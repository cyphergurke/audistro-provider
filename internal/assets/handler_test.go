package assets

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestGetMasterPlaylist(t *testing.T) {
	dataPath := writeAssetsFixture(t)
	h := NewHandler(HandlerConfig{AssetsRoot: filepath.Join(dataPath, "assets"), MaxSegmentBytes: 67108864})

	req := httptest.NewRequest(http.MethodGet, "/assets/asset1/master.m3u8", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if rr.Header().Get("Content-Type") != "application/vnd.apple.mpegurl" {
		t.Fatalf("unexpected content-type: %q", rr.Header().Get("Content-Type"))
	}
	if rr.Header().Get("Cache-Control") != "public, max-age=5" {
		t.Fatalf("unexpected cache-control: %q", rr.Header().Get("Cache-Control"))
	}
}

func TestGetSegmentM4S(t *testing.T) {
	dataPath := writeAssetsFixture(t)
	h := NewHandler(HandlerConfig{AssetsRoot: filepath.Join(dataPath, "assets"), MaxSegmentBytes: 67108864})

	req := httptest.NewRequest(http.MethodGet, "/assets/asset1/seg_0001.m4s", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if rr.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatalf("unexpected cache-control: %q", rr.Header().Get("Cache-Control"))
	}
}

func TestGetInitMP4(t *testing.T) {
	dataPath := writeAssetsFixture(t)
	h := NewHandler(HandlerConfig{AssetsRoot: filepath.Join(dataPath, "assets"), MaxSegmentBytes: 67108864})

	req := httptest.NewRequest(http.MethodGet, "/assets/asset1/init.mp4", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
}

func TestGetOtherMP4Rejected(t *testing.T) {
	dataPath := writeAssetsFixture(t)
	h := NewHandler(HandlerConfig{AssetsRoot: filepath.Join(dataPath, "assets"), MaxSegmentBytes: 67108864})

	req := httptest.NewRequest(http.MethodGet, "/assets/asset1/other.mp4", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
}

func TestRangeRequestOnInitMP4(t *testing.T) {
	dataPath := writeAssetsFixture(t)
	h := NewHandler(HandlerConfig{AssetsRoot: filepath.Join(dataPath, "assets"), MaxSegmentBytes: 67108864})

	req := httptest.NewRequest(http.MethodGet, "/assets/asset1/init.mp4", nil)
	req.Header.Set("Range", "bytes=0-9")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusPartialContent {
		t.Fatalf("expected status 206, got %d", rr.Code)
	}
	if rr.Header().Get("Content-Range") == "" {
		t.Fatal("expected Content-Range header")
	}
}

func TestInvalidAssetID(t *testing.T) {
	dataPath := writeAssetsFixture(t)
	h := NewHandler(HandlerConfig{AssetsRoot: filepath.Join(dataPath, "assets"), MaxSegmentBytes: 67108864})

	req := httptest.NewRequest(http.MethodGet, "/assets/asset!/master.m3u8", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
}

func TestTraversalFilenameRejected(t *testing.T) {
	dataPath := writeAssetsFixture(t)
	h := NewHandler(HandlerConfig{AssetsRoot: filepath.Join(dataPath, "assets"), MaxSegmentBytes: 67108864})

	req := httptest.NewRequest(http.MethodGet, "/assets/asset1/../x", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
}

func TestMissingFile(t *testing.T) {
	dataPath := writeAssetsFixture(t)
	h := NewHandler(HandlerConfig{AssetsRoot: filepath.Join(dataPath, "assets"), MaxSegmentBytes: 67108864})

	req := httptest.NewRequest(http.MethodGet, "/assets/asset1/missing.ts", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rr.Code)
	}
}

func TestMaxSegmentSizeExceeded(t *testing.T) {
	dataPath := writeAssetsFixture(t)
	h := NewHandler(HandlerConfig{AssetsRoot: filepath.Join(dataPath, "assets"), MaxSegmentBytes: 4})

	req := httptest.NewRequest(http.MethodGet, "/assets/asset1/seg_0001.ts", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected status 413, got %d", rr.Code)
	}
}

func TestRateLimitExceeded(t *testing.T) {
	dataPath := writeAssetsFixture(t)
	base := NewHandler(HandlerConfig{AssetsRoot: filepath.Join(dataPath, "assets"), MaxSegmentBytes: 67108864})
	h := NewRateLimitMiddleware(RateLimitConfig{RPS: 1, Burst: 1})(base)

	req1 := httptest.NewRequest(http.MethodGet, "/assets/asset1/master.m3u8", nil)
	req1.RemoteAddr = "203.0.113.1:12345"
	rr1 := httptest.NewRecorder()
	h.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("expected first request status 200, got %d", rr1.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/assets/asset1/master.m3u8", nil)
	req2.RemoteAddr = "203.0.113.1:12345"
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second request status 429, got %d", rr2.Code)
	}
}

func TestCORSDisabledByDefault(t *testing.T) {
	dataPath := writeAssetsFixture(t)
	h := NewHandler(HandlerConfig{AssetsRoot: filepath.Join(dataPath, "assets"), MaxSegmentBytes: 67108864})

	req := httptest.NewRequest(http.MethodGet, "/assets/asset1/master.m3u8", nil)
	req.Header.Set("Origin", "https://app.example.com")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("expected no CORS header, got %q", rr.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestCORSEnabledAllowedOriginGET(t *testing.T) {
	dataPath := writeAssetsFixture(t)
	base := NewHandler(HandlerConfig{AssetsRoot: filepath.Join(dataPath, "assets"), MaxSegmentBytes: 67108864, CORSEnabled: true})
	h := NewCORSMiddleware(CORSConfig{AllowedOrigins: ParseAllowedOrigins("https://app.example.com")})(base)

	req := httptest.NewRequest(http.MethodGet, "/assets/asset1/master.m3u8", nil)
	req.Header.Set("Origin", "https://app.example.com")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if rr.Header().Get("Access-Control-Allow-Origin") != "https://app.example.com" {
		t.Fatalf("unexpected ACAO: %q", rr.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestCORSEnabledPreflightAllowed(t *testing.T) {
	dataPath := writeAssetsFixture(t)
	base := NewHandler(HandlerConfig{AssetsRoot: filepath.Join(dataPath, "assets"), MaxSegmentBytes: 67108864, CORSEnabled: true})
	h := NewCORSMiddleware(CORSConfig{AllowedOrigins: ParseAllowedOrigins("https://app.example.com"), AllowCredentials: true})(base)

	req := httptest.NewRequest(http.MethodOptions, "/assets/asset1/master.m3u8", nil)
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", rr.Code)
	}
	if rr.Header().Get("Access-Control-Allow-Origin") != "https://app.example.com" {
		t.Fatalf("unexpected ACAO: %q", rr.Header().Get("Access-Control-Allow-Origin"))
	}
	if rr.Header().Get("Access-Control-Allow-Methods") != "GET, HEAD, OPTIONS" {
		t.Fatalf("unexpected allow methods: %q", rr.Header().Get("Access-Control-Allow-Methods"))
	}
	if rr.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("unexpected allow credentials: %q", rr.Header().Get("Access-Control-Allow-Credentials"))
	}
}

func TestCORSEnabledPreflightDisallowed(t *testing.T) {
	dataPath := writeAssetsFixture(t)
	base := NewHandler(HandlerConfig{AssetsRoot: filepath.Join(dataPath, "assets"), MaxSegmentBytes: 67108864, CORSEnabled: true})
	h := NewCORSMiddleware(CORSConfig{AllowedOrigins: ParseAllowedOrigins("https://app.example.com")})(base)

	req := httptest.NewRequest(http.MethodOptions, "/assets/asset1/master.m3u8", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", rr.Code)
	}
	if rr.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("expected no ACAO header, got %q", rr.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestProxyModeRangeRequest(t *testing.T) {
	origin, _ := newOriginServer(t)
	h := NewHandler(HandlerConfig{
		AssetsRoot:      t.TempDir(),
		MaxSegmentBytes: 67108864,
		StorageMode:     "proxy",
		OriginBaseURL:   origin.URL,
	})

	req := httptest.NewRequest(http.MethodGet, "/assets/a/init.mp4", nil)
	req.Header.Set("Range", "bytes=0-9")
	req.RemoteAddr = "203.0.113.9:34567"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusPartialContent {
		t.Fatalf("expected status 206, got %d", rr.Code)
	}
	if rr.Header().Get("Content-Range") == "" {
		t.Fatal("expected Content-Range header")
	}
}

func TestHybridModeLocalThenProxyFallback(t *testing.T) {
	origin, hits := newOriginServer(t)
	dataPath := t.TempDir()
	assetDir := filepath.Join(dataPath, "assets", "a")
	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		t.Fatalf("mkdir asset dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(assetDir, "master.m3u8"), []byte("#EXTM3U\n"), 0o644); err != nil {
		t.Fatalf("write local master: %v", err)
	}

	h := NewHandler(HandlerConfig{
		AssetsRoot:      filepath.Join(dataPath, "assets"),
		MaxSegmentBytes: 67108864,
		StorageMode:     "hybrid",
		OriginBaseURL:   origin.URL,
	})

	reqLocal := httptest.NewRequest(http.MethodGet, "/assets/a/master.m3u8", nil)
	rrLocal := httptest.NewRecorder()
	h.ServeHTTP(rrLocal, reqLocal)
	if rrLocal.Code != http.StatusOK {
		t.Fatalf("expected local status 200, got %d", rrLocal.Code)
	}
	if got := hits.Load(); got != 0 {
		t.Fatalf("expected no origin hit for local file, got %d", got)
	}

	reqProxy := httptest.NewRequest(http.MethodGet, "/assets/a/seg_remote.m4s", nil)
	rrProxy := httptest.NewRecorder()
	h.ServeHTTP(rrProxy, reqProxy)
	if rrProxy.Code != http.StatusOK {
		t.Fatalf("expected proxied status 200, got %d", rrProxy.Code)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("expected one origin hit after fallback, got %d", got)
	}
}

func TestProxyRemovesHopByHopHeaders(t *testing.T) {
	origin, _ := newOriginServer(t)
	h := NewHandler(HandlerConfig{
		AssetsRoot:      t.TempDir(),
		MaxSegmentBytes: 67108864,
		StorageMode:     "proxy",
		OriginBaseURL:   origin.URL,
	})

	req := httptest.NewRequest(http.MethodGet, "/assets/a/init.mp4", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	for _, header := range []string{
		"Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"TE",
		"Trailer",
		"Transfer-Encoding",
		"Upgrade",
	} {
		if got := rr.Header().Get(header); got != "" {
			t.Fatalf("expected %s to be removed, got %q", header, got)
		}
	}
}

func newOriginServer(t *testing.T) (*httptest.Server, *atomic.Int64) {
	t.Helper()

	var hits atomic.Int64
	files := map[string][]byte{
		"/assets/a/init.mp4":       []byte("0123456789abcdefghij"),
		"/assets/a/seg_remote.m4s": []byte("remote-segment-data"),
		"/assets/a/master.m3u8":    []byte("#EXTM3U\n#origin\n"),
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		data, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Connection", "close")
		w.Header().Set("Keep-Alive", "timeout=5")
		w.Header().Set("Proxy-Authenticate", "Basic realm=\"x\"")
		w.Header().Set("Proxy-Authorization", "Basic aGVsbG86d29ybGQ=")
		w.Header().Set("TE", "trailers")
		w.Header().Set("Trailer", "X-Trailer")
		w.Header().Set("Upgrade", "h2c")
		modTime := time.Unix(1700000000, 0).UTC()
		http.ServeContent(w, r, filepath.Base(r.URL.Path), modTime, bytes.NewReader(data))
	}))
	t.Cleanup(srv.Close)

	return srv, &hits
}

func writeAssetsFixture(t *testing.T) string {
	t.Helper()

	dataPath := t.TempDir()
	assetDir := filepath.Join(dataPath, "assets", "asset1")
	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		t.Fatalf("mkdir asset dir: %v", err)
	}

	writeFile := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(assetDir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write file %s: %v", name, err)
		}
	}

	writeFile("master.m3u8", "#EXTM3U\n#EXT-X-VERSION:3\n")
	writeFile("seg_0001.ts", "segment-ts-data")
	writeFile("seg_0001.m4s", "segment-m4s-data")
	writeFile("init.mp4", "0123456789abcdefghij")
	writeFile("init-abc.mp4", "0123456789abcdefghij")
	writeFile("other.mp4", "0123456789abcdefghij")

	return dataPath
}
