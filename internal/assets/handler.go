package assets

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"audistro-provider/internal/fssecure"
	"audistro-provider/internal/metrics"
	"audistro-provider/internal/proxyguard"
)

type OriginSigner interface {
	Sign(req *http.Request, now time.Time) error
}

type HandlerConfig struct {
	AssetsRoot                  string
	MaxSegmentBytes             int64
	CORSEnabled                 bool
	StorageMode                 string
	OriginBaseURL               string
	OriginAuthMode              string
	OriginSigner                OriginSigner
	ProxyMaxUpstreamConcurrency int
	ProxySemaphore              *proxyguard.Semaphore
	Logger                      *slog.Logger
	Metrics                     *metrics.Metrics
}

func NewHandler(cfg HandlerConfig) http.Handler {
	sem := cfg.ProxySemaphore
	if sem == nil && cfg.ProxyMaxUpstreamConcurrency > 0 {
		sem = proxyguard.NewSemaphore(cfg.ProxyMaxUpstreamConcurrency)
	}
	h := &handler{
		cfg:               cfg,
		mode:              normalizeStorageMode(cfg.StorageMode),
		originAuthMode:    normalizeOriginAuthMode(cfg.OriginAuthMode),
		originSigner:      cfg.OriginSigner,
		upstreamSemaphore: sem,
		nowFn:             time.Now,
	}
	h.initProxy()
	return http.HandlerFunc(h.serveHTTP)
}

func parseAssetPath(path string) (string, string, bool) {
	const prefix = "/assets/"
	if !strings.HasPrefix(path, prefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(path, prefix)
	if rest == "" {
		return "", "", false
	}
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

type handler struct {
	cfg               HandlerConfig
	mode              string
	originAuthMode    string
	originSigner      OriginSigner
	upstreamSemaphore *proxyguard.Semaphore
	nowFn             func() time.Time
	proxy             *httputil.ReverseProxy
}

type assetRequest struct {
	assetID  string
	filename string
	ext      string
	root     string
	target   string
}

func (h *handler) serveHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodHead:
	case http.MethodOptions:
		if h.cfg.CORSEnabled {
			writeJSONError(w, http.StatusForbidden, "cors_origin_forbidden")
			return
		}
		writeJSONError(w, http.StatusNotFound, "not_found")
		return
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}

	req, status, err := h.parseRequest(r.URL.Path)
	if err != nil {
		h.writeErrorStatus(w, status)
		return
	}
	setAccessLogField(w, "mode", h.mode)

	switch h.mode {
	case "proxy":
		h.ServeProxy(w, r, req)
		return
	case "hybrid":
		served, localStatus, localErr := h.TryServeLocal(w, r, req)
		if served {
			return
		}
		if localErr != nil && localStatus != http.StatusNotFound {
			h.writeErrorStatus(w, localStatus)
			return
		}
		h.ServeProxy(w, r, req)
		return
	default:
		served, localStatus, _ := h.TryServeLocal(w, r, req)
		if served {
			return
		}
		h.writeErrorStatus(w, localStatus)
		return
	}
}

func (h *handler) parseRequest(pathValue string) (assetRequest, int, error) {
	assetID, filename, ok := parseAssetPath(pathValue)
	if !ok {
		return assetRequest{}, http.StatusNotFound, errors.New("not_found")
	}
	if err := validateAssetID(assetID); err != nil {
		return assetRequest{}, http.StatusBadRequest, err
	}
	if filename == "" {
		return assetRequest{}, http.StatusNotFound, errors.New("not_found")
	}
	if err := validateFilename(filename); err != nil {
		return assetRequest{}, http.StatusBadRequest, err
	}
	ext, err := validateExtension(filename)
	if err != nil {
		return assetRequest{}, http.StatusBadRequest, err
	}

	root := filepath.Join(h.cfg.AssetsRoot, assetID)
	target := filepath.Join(root, filename)
	if err := ensureWithinRoot(root, target); err != nil {
		return assetRequest{}, http.StatusBadRequest, err
	}

	return assetRequest{
		assetID:  assetID,
		filename: filename,
		ext:      ext,
		root:     root,
		target:   target,
	}, http.StatusOK, nil
}

func (h *handler) TryServeLocal(w http.ResponseWriter, r *http.Request, req assetRequest) (bool, int, error) {
	file, st, err := fssecure.OpenFileNoSymlinks(req.root, req.filename)
	if err != nil {
		if os.IsNotExist(err) {
			return false, http.StatusNotFound, err
		}
		if errors.Is(err, fssecure.ErrSymlink) {
			if h.cfg.Logger != nil {
				h.cfg.Logger.Warn("asset request blocked",
					slog.String("reason", "symlink_blocked"),
					slog.String("asset_id", req.assetID),
					slog.String("filename", req.filename),
				)
			}
			return false, http.StatusNotFound, err
		}
		if errors.Is(err, fssecure.ErrNotRegular) || errors.Is(err, fssecure.ErrInvalidFilename) {
			return false, http.StatusNotFound, err
		}
		return false, http.StatusInternalServerError, err
	}
	defer file.Close()

	if isSegmentLike(req.ext) && h.cfg.MaxSegmentBytes > 0 && st.Size() > h.cfg.MaxSegmentBytes {
		return false, http.StatusRequestEntityTooLarge, errors.New("segment_too_large")
	}

	setContentHeaders(w, req.ext)
	http.ServeContent(w, r, filepath.Base(req.filename), st.ModTime(), file)
	return true, http.StatusOK, nil
}

func (h *handler) ServeProxy(w http.ResponseWriter, r *http.Request, req assetRequest) {
	start := time.Now()
	if h.proxy == nil {
		writeJSONError(w, http.StatusInternalServerError, "proxy_not_configured")
		if h.cfg.Metrics != nil {
			h.cfg.Metrics.ObserveProxy(http.StatusInternalServerError, time.Since(start), "error")
		}
		return
	}
	if h.originAuthMode == "hmac" && h.originSigner == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "upstream_auth_unavailable")
		if h.cfg.Metrics != nil {
			h.cfg.Metrics.ObserveProxy(http.StatusServiceUnavailable, time.Since(start), "upstream_auth_unavailable")
		}
		return
	}
	if h.upstreamSemaphore != nil {
		if !h.upstreamSemaphore.Acquire(r.Context()) {
			writeJSONError(w, http.StatusServiceUnavailable, "upstream_busy")
			setAccessLogField(w, "upstream_status", http.StatusServiceUnavailable)
			setAccessLogField(w, "upstream_latency_ms", time.Since(start).Milliseconds())
			if h.cfg.Metrics != nil {
				h.cfg.Metrics.IncProxyUpstreamBusy()
				h.cfg.Metrics.ObserveProxy(http.StatusServiceUnavailable, time.Since(start), "upstream_busy")
			}
			return
		}
		defer h.upstreamSemaphore.Release()
		if h.cfg.Metrics != nil {
			done := h.cfg.Metrics.IncProxyUpstreamInFlight()
			defer done()
		}
	}

	meta := proxyRequestMeta{
		ext:             req.ext,
		maxSegmentBytes: h.cfg.MaxSegmentBytes,
		start:           start,
		requestPath:     r.URL.Path,
	}
	r = r.WithContext(context.WithValue(r.Context(), proxyRequestMetaKey{}, meta))

	capture := &statusCaptureWriter{ResponseWriter: w}
	h.proxy.ServeHTTP(capture, r)

	setAccessLogField(w, "upstream_status", capture.Status())
	setAccessLogField(w, "upstream_latency_ms", time.Since(meta.start).Milliseconds())
	if h.cfg.Metrics != nil {
		h.cfg.Metrics.ObserveProxy(capture.Status(), time.Since(start), proxyResult(capture.Status()))
	}
}

func (h *handler) writeErrorStatus(w http.ResponseWriter, status int) {
	switch status {
	case http.StatusBadRequest:
		writeJSONError(w, http.StatusBadRequest, "invalid_request")
	case http.StatusNotFound:
		writeJSONError(w, http.StatusNotFound, "not_found")
	case http.StatusRequestEntityTooLarge:
		writeJSONError(w, http.StatusRequestEntityTooLarge, "segment_too_large")
	default:
		writeJSONError(w, http.StatusInternalServerError, "internal_error")
	}
}

func ensureWithinRoot(root, target string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(absTarget, absRoot+string(os.PathSeparator)) {
		return fmt.Errorf("target outside root")
	}
	return nil
}

func setContentHeaders(w http.ResponseWriter, ext string) {
	h := w.Header()
	applyContentHeaders(h, ext, true)
}

func isSegmentLike(ext string) bool {
	return ext == ".ts" || ext == ".m4s" || ext == ".mp4"
}

func normalizeStorageMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "proxy":
		return "proxy"
	case "hybrid":
		return "hybrid"
	default:
		return "filesystem"
	}
}

func normalizeOriginAuthMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "hmac":
		return "hmac"
	default:
		return "none"
	}
}

var hopByHopHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"TE",
	"Trailer",
	"Transfer-Encoding",
	"Upgrade",
}

func removeHopByHopHeaders(header http.Header) {
	connection := header.Get("Connection")
	for _, key := range hopByHopHeaders {
		header.Del(key)
	}
	if connection == "" {
		return
	}
	for _, field := range strings.Split(connection, ",") {
		if trimmed := strings.TrimSpace(field); trimmed != "" {
			header.Del(trimmed)
		}
	}
}

func (h *handler) initProxy() {
	if h.mode != "proxy" && h.mode != "hybrid" {
		return
	}
	origin, err := url.Parse(strings.TrimSpace(h.cfg.OriginBaseURL))
	if err != nil || origin.Scheme == "" || origin.Host == "" {
		return
	}

	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          128,
		MaxIdleConnsPerHost:   32,
		IdleConnTimeout:       60 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	upstreamTransport := http.RoundTripper(transport)
	if h.originAuthMode == "hmac" {
		upstreamTransport = &signingRoundTripper{
			base:   transport,
			signer: h.originSigner,
			nowFn:  h.nowFn,
		}
	}

	h.proxy = &httputil.ReverseProxy{
		Transport: upstreamTransport,
		Rewrite: func(pr *httputil.ProxyRequest) {
			in := pr.In
			out := pr.Out

			out.URL.Scheme = origin.Scheme
			out.URL.Host = origin.Host
			out.URL.Path = singleJoiningSlash(origin.Path, in.URL.Path)
			out.URL.RawPath = out.URL.Path
			out.URL.RawQuery = in.URL.RawQuery
			out.Host = origin.Host

			removeHopByHopHeaders(out.Header)
			out.Header.Set("X-Forwarded-For", remoteHost(in.RemoteAddr))
			if in.TLS != nil {
				out.Header.Set("X-Forwarded-Proto", "https")
			} else {
				out.Header.Set("X-Forwarded-Proto", "http")
			}
		},
		ModifyResponse: func(resp *http.Response) error {
			removeHopByHopHeaders(resp.Header)
			resp.Trailer = nil

			meta, ok := proxyMetaFromContext(resp.Request.Context())
			if ok {
				applyContentHeaders(resp.Header, meta.ext, false)
				if isSegmentLike(meta.ext) && meta.maxSegmentBytes > 0 {
					if resp.ContentLength > meta.maxSegmentBytes {
						return errProxySegmentTooLarge
					}
					if resp.ContentLength < 0 {
						resp.Body = &maxBytesReadCloser{
							body:        resp.Body,
							reader:      io.LimitReader(resp.Body, meta.maxSegmentBytes+1),
							max:         meta.maxSegmentBytes,
							logger:      h.cfg.Logger,
							requestPath: meta.requestPath,
						}
						resp.ContentLength = -1
						resp.Header.Del("Content-Length")
					}
				}
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			if errors.Is(err, errOriginAuthUnavailable) {
				writeJSONError(w, http.StatusServiceUnavailable, "upstream_auth_unavailable")
				if h.cfg.Logger != nil {
					h.cfg.Logger.Error("proxy upstream auth unavailable",
						slog.String("path", r.URL.Path),
					)
				}
				return
			}
			if errors.Is(err, errProxySegmentTooLarge) || errors.Is(err, errProxyBodyTooLarge) {
				writeJSONError(w, http.StatusRequestEntityTooLarge, "segment_too_large")
				if h.cfg.Logger != nil {
					h.cfg.Logger.Error("proxy segment too large",
						slog.String("path", r.URL.Path),
						slog.String("error", err.Error()),
					)
				}
				return
			}
			writeJSONError(w, http.StatusBadGateway, "upstream_error")
			if h.cfg.Logger != nil {
				h.cfg.Logger.Error("proxy upstream error",
					slog.String("path", r.URL.Path),
					slog.String("error", err.Error()),
				)
			}
		},
	}
}

func applyContentHeaders(h http.Header, ext string, setContentType bool) {
	switch ext {
	case ".m3u8":
		if setContentType {
			h.Set("Content-Type", "application/vnd.apple.mpegurl")
		}
		h.Set("Cache-Control", "public, max-age=5")
	case ".ts":
		if setContentType {
			h.Set("Content-Type", "video/mp2t")
		}
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
	case ".m4s":
		if setContentType {
			h.Set("Content-Type", "video/iso.segment")
		}
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
	case ".mp4":
		if setContentType {
			h.Set("Content-Type", "video/mp4")
		}
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
	}
}

func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	default:
		return a + b
	}
}

type statusCaptureWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusCaptureWriter) WriteHeader(statusCode int) {
	w.status = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *statusCaptureWriter) Status() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

type accessLogFieldSetter interface {
	SetAccessLogField(key string, value any)
}

func setAccessLogField(w http.ResponseWriter, key string, value any) {
	setter, ok := w.(accessLogFieldSetter)
	if !ok {
		return
	}
	setter.SetAccessLogField(key, value)
}

type proxyRequestMetaKey struct{}

type proxyRequestMeta struct {
	ext             string
	maxSegmentBytes int64
	start           time.Time
	requestPath     string
}

func proxyResult(status int) string {
	switch {
	case status >= 200 && status < 400:
		return "ok"
	case status >= 400 && status < 500:
		return "client_error"
	case status >= 500:
		return "upstream_error"
	default:
		return "error"
	}
}

func proxyMetaFromContext(ctx context.Context) (proxyRequestMeta, bool) {
	meta, ok := ctx.Value(proxyRequestMetaKey{}).(proxyRequestMeta)
	return meta, ok
}

var (
	errProxySegmentTooLarge  = errors.New("proxy segment too large")
	errProxyBodyTooLarge     = errors.New("proxy body too large")
	errOriginAuthUnavailable = errors.New("origin auth unavailable")
)

type signingRoundTripper struct {
	base   http.RoundTripper
	signer OriginSigner
	nowFn  func() time.Time
}

func (s *signingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if s.signer == nil {
		return nil, errOriginAuthUnavailable
	}
	if s.nowFn == nil {
		s.nowFn = time.Now
	}
	if err := s.signer.Sign(req, s.nowFn().UTC()); err != nil {
		return nil, err
	}
	return s.base.RoundTrip(req)
}

type maxBytesReadCloser struct {
	body        io.ReadCloser
	reader      io.Reader
	max         int64
	read        int64
	overflow    bool
	logger      *slog.Logger
	requestPath string
}

func (r *maxBytesReadCloser) Read(p []byte) (int, error) {
	if r.overflow {
		return 0, errProxyBodyTooLarge
	}
	n, err := r.reader.Read(p)
	if n > 0 {
		next := r.read + int64(n)
		if next > r.max {
			allowed := int(r.max - r.read)
			if allowed < 0 {
				allowed = 0
			}
			r.read = r.max
			r.overflow = true
			if r.logger != nil {
				r.logger.Error("proxy response exceeded segment limit",
					slog.String("path", r.requestPath),
					slog.Int64("max_segment_bytes", r.max),
				)
			}
			if allowed > 0 {
				return allowed, nil
			}
			return 0, errProxyBodyTooLarge
		}
		r.read = next
	}
	if err != nil {
		return n, err
	}
	return n, nil
}

func (r *maxBytesReadCloser) Close() error {
	return r.body.Close()
}

func remoteHost(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}
