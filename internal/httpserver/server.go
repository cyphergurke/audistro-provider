package httpserver

import (
	"context"
	"log/slog"
	"net/http"
	"path/filepath"
	"time"

	"audistro-provider/internal/assets"
	"audistro-provider/internal/config"
	"audistro-provider/internal/health"
	"audistro-provider/internal/identity"
	"audistro-provider/internal/metrics"
	"audistro-provider/internal/proxyguard"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Server struct {
	HTTP     *http.Server
	Identity *identity.Identity
}

func New(cfg config.Config, logger *slog.Logger, id *identity.Identity, version string) *Server {
	return NewWithDeps(cfg, logger, id, version, nil, nil, nil, nil, nil, nil, nil, nil, nil)
}

func NewWithDeps(
	cfg config.Config,
	logger *slog.Logger,
	id *identity.Identity,
	version string,
	activeAssetsCounter health.ActiveAssetsCounter,
	catalogStatus health.CatalogStatusProvider,
	jobsStatus health.JobsStatusProvider,
	readinessChecker health.ReadinessChecker,
	originSigner assets.OriginSigner,
	rescanHandler http.Handler,
	announceHandler http.Handler,
	metricsRegistry *prometheus.Registry,
	metricsCollector *metrics.Metrics,
) *Server {
	if metricsCollector == nil {
		metricsCollector = metrics.New(metricsRegistry)
	}
	if metricsRegistry == nil {
		metricsRegistry = metricsCollector.Registry
	}

	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.HandlerFor(metricsRegistry, promhttp.HandlerOpts{}))

	mux.Handle("GET /healthz", health.Handler(
		"audistro-provider",
		version,
		cfg.Region,
		cfg.PublicBaseURL,
		id.ProviderID.String(),
		id.PublicKeyHexCompressed(),
		activeAssetsCounter,
		catalogStatus,
		jobsStatus,
	))
	mux.Handle("GET /readyz", health.ReadyHandler(readinessChecker))
	if cfg.InternalEnable {
		if rescanHandler != nil {
			mux.Handle("POST /internal/rescan", rescanHandler)
		}
		if announceHandler != nil {
			mux.Handle("POST /internal/announce", announceHandler)
		}
	}

	upstreamSemaphore := proxyguard.NewSemaphore(cfg.ProxyMaxUpstreamConcurrency)
	assetsHandler := assets.NewHandler(assets.HandlerConfig{
		AssetsRoot:                  filepath.Join(cfg.DataPath, cfg.AssetsSubdir),
		MaxSegmentBytes:             cfg.MaxSegmentBytes,
		CORSEnabled:                 cfg.EnableCORS,
		StorageMode:                 string(cfg.StorageMode),
		OriginBaseURL:               cfg.OriginBaseURL,
		OriginAuthMode:              cfg.OriginAuthMode,
		OriginSigner:                originSigner,
		ProxyMaxUpstreamConcurrency: cfg.ProxyMaxUpstreamConcurrency,
		ProxySemaphore:              upstreamSemaphore,
		Logger:                      logger,
		Metrics:                     metricsCollector,
	})
	if cfg.EnableCORS {
		assetsHandler = assets.NewCORSMiddleware(assets.CORSConfig{
			AllowedOrigins:   assets.ParseAllowedOrigins(cfg.CORSAllowedOrigins),
			AllowCredentials: cfg.CORSAllowCredentials,
		})(assetsHandler)
	}
	assetsHandler = assets.NewRateLimitMiddleware(assets.RateLimitConfig{
		RPS:                 cfg.RateLimitRPS,
		Burst:               cfg.RateLimitBurst,
		TrustProxyAddresses: cfg.TrustProxyAddresses,
	})(assetsHandler)
	mux.Handle("/assets/", assetsHandler)

	handler := Chain(mux,
		HTTPMetrics(metricsCollector),
		AccessLog(logger),
		Recover(logger),
		RequestID,
	)

	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	return &Server{
		HTTP:     httpServer,
		Identity: id,
	}
}

func (s *Server) ListenAndServe() error {
	return s.HTTP.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.HTTP.Shutdown(ctx)
}

func (s *Server) Close() error {
	return s.HTTP.Close()
}

func Chain(next http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		next = mws[i](next)
	}
	return next
}
