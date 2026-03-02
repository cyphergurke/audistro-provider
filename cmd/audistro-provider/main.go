package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"audistro-provider/internal/buildinfo"
	"audistro-provider/internal/catalog"
	"audistro-provider/internal/config"
	"audistro-provider/internal/db"
	"audistro-provider/internal/envcheck"
	"audistro-provider/internal/fssecure"
	"audistro-provider/internal/health"
	"audistro-provider/internal/httpserver"
	"audistro-provider/internal/identity"
	"audistro-provider/internal/internalapi"
	"audistro-provider/internal/jobs"
	"audistro-provider/internal/logging"
	"audistro-provider/internal/metrics"
	"audistro-provider/internal/originauth"
	"audistro-provider/internal/repository"
	"audistro-provider/internal/scanner"

	"github.com/prometheus/client_golang/prometheus"
)

func main() {
	envcheck.MustValidate()

	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		slog.Error("invalid configuration", slog.String("error", err.Error()))
		os.Exit(1)
	}

	info := buildinfo.Info()
	logger := logging.New("audistro-provider", info.Version, info.Commit, info.BuildTime)
	metricsRegistry := prometheus.NewRegistry()
	metricsCollector := metrics.New(metricsRegistry)
	metricsCollector.SetDeferredAnnounces(0)
	logger.Info("build_metadata")
	logger.Info("starting audistro-provider",
		slog.String("http_addr", cfg.HTTPAddr),
		slog.String("data_path", cfg.DataPath),
		slog.String("identity_path", cfg.IdentityPath),
		slog.String("db_path", cfg.DBPath),
		slog.String("catalog_base_url", cfg.CatalogBaseURL),
		slog.String("region", cfg.Region),
		slog.String("public_base_url", cfg.PublicBaseURL),
		slog.String("storage_mode", string(cfg.StorageMode)),
		slog.String("origin_base_url", cfg.OriginBaseURL),
	)

	if err := os.MkdirAll(cfg.DataPath, 0o700); err != nil {
		logger.Error("failed to create data path", slog.String("error", err.Error()))
		os.Exit(1)
	}
	assetsRoot := filepath.Join(cfg.DataPath, cfg.AssetsSubdir)
	if err := os.MkdirAll(assetsRoot, 0o700); err != nil {
		logger.Error("failed to create assets path", slog.String("error", err.Error()))
		os.Exit(1)
	}
	allowlistedAssetName := func(name string) bool {
		lower := strings.ToLower(name)
		ext := strings.ToLower(filepath.Ext(lower))
		switch ext {
		case ".m3u8", ".ts", ".m4s":
			return true
		case ".mp4":
			return lower == "init.mp4" || (strings.HasPrefix(lower, "init-") && strings.HasSuffix(lower, ".mp4") && len(lower) > len("init-.mp4"))
		default:
			return false
		}
	}
	if cfg.EnforceNoSymlinks {
		found, err := fssecure.AuditNoSymlinks(assetsRoot, allowlistedAssetName)
		if err != nil {
			logger.Warn("startup symlink audit failed", slog.String("error", err.Error()))
		} else if found {
			logger.Warn("startup symlink audit detected symlink", slog.String("assets_root", assetsRoot))
		}
	}

	store := &identity.FileStore{Path: cfg.IdentityPath}
	id, created, err := identity.LoadOrCreate(context.Background(), store)
	if err != nil {
		logger.Error("failed to load identity", slog.String("error", err.Error()))
		os.Exit(1)
	}

	event := "identity_loaded"
	if created {
		event = "identity_created"
	}
	logger.Info(event,
		slog.String("provider_id", id.ProviderID.String()),
		slog.String("pubkey_fingerprint", id.Fingerprint()),
	)

	originAuthMode := strings.ToLower(strings.TrimSpace(cfg.OriginAuthMode))
	originAuthReady := true
	var originSigner *originauth.HMACSigner
	if originAuthMode == "hmac" {
		secretBytes, err := os.ReadFile(cfg.OriginHMACSecretPath)
		if err != nil {
			originAuthReady = false
			logger.Error("failed to load origin hmac secret", slog.String("error", err.Error()))
		} else {
			secretBytes = bytes.TrimRight(secretBytes, "\r\n")
			if len(secretBytes) < 16 {
				originAuthReady = false
				logger.Error("origin hmac secret too short", slog.Int("length", len(secretBytes)))
			} else {
				originSigner = originauth.NewHMACSigner(
					cfg.OriginHMACKeyID,
					secretBytes,
					id.ProviderID.String(),
					cfg.OriginAuthIncludeQuery,
				)
			}
		}
	}

	sqlDB, err := db.Open(cfg)
	if err != nil {
		logger.Error("failed to open db", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer func() {
		_ = sqlDB.Close()
	}()

	if err := db.Migrate(sqlDB); err != nil {
		logger.Error("failed to migrate db", slog.String("error", err.Error()))
		os.Exit(1)
	}

	sc := &scanner.Scanner{AssetsRoot: assetsRoot, Metrics: metricsCollector}
	if cfg.ScanOnStartup {
		res, err := internalapi.RunRescan(context.Background(), sqlDB, sc)
		if err != nil {
			logger.Error("startup scan failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		logger.Info("startup scan completed",
			slog.Int("scanned_assets", res.ScannedAssets),
			slog.Int("active_assets", res.ActiveAssets),
			slog.Int("missing_assets", res.MissingAssets),
			slog.Int64("duration_ms", res.DurationMS),
		)
		metricsCollector.SetActiveAssets(res.ActiveAssets)
		missing, countErr := repository.CountMissing(context.Background(), sqlDB)
		if countErr == nil {
			metricsCollector.SetMissingAssets(missing)
		}
	}

	var catalogClient *catalog.Client
	catalogHealth := catalog.NewHealthState(strings.TrimSpace(cfg.CatalogBaseURL) != "")
	if strings.TrimSpace(cfg.CatalogBaseURL) != "" {
		client, err := catalog.NewClient(cfg.CatalogBaseURL, time.Duration(cfg.CatalogTimeoutSeconds)*time.Second, logger, metricsCollector)
		if err != nil {
			catalogHealth.Set(err)
			logger.Error("catalog client initialization failed", slog.String("error", err.Error()))
		} else {
			catalogClient = client
			if err := catalogClient.EnsureProvider(context.Background(), id, cfg); err != nil {
				catalogHealth.Set(err)
				logger.Error("catalog provider ensure failed", slog.String("error", err.Error()))
			} else {
				catalogHealth.Set(nil)
				logger.Info("catalog provider ensured",
					slog.String("provider_id", id.ProviderID.String()),
					slog.String("pubkey_fingerprint", id.Fingerprint()),
				)
			}
		}
	}

	rescanHandler := internalapi.NewRescanHandler(sqlDB, sc, logger)
	announceHandler := internalapi.NewAnnounceHandler(sqlDB, id, cfg, catalogClient, logger, catalogHealth)
	if cfg.InternalEnable {
		internalAllowlist := internalapi.NewIPAllowlistMiddleware(cfg, logger)
		rescanHandler = internalAllowlist(rescanHandler)
		announceHandler = internalAllowlist(announceHandler)
	}
	readinessChecker := func(ctx context.Context) error {
		if err := sqlDB.PingContext(ctx); err != nil {
			return fmt.Errorf("db_unavailable")
		}
		info, err := os.Stat(assetsRoot)
		if err != nil {
			return fmt.Errorf("assets_root_unavailable")
		}
		if !info.IsDir() {
			return fmt.Errorf("assets_root_not_directory")
		}
		if cfg.EnforceNoSymlinks {
			found, err := fssecure.AuditNoSymlinks(assetsRoot, allowlistedAssetName)
			if err != nil {
				return fmt.Errorf("symlink_audit_failed")
			}
			if found {
				return fmt.Errorf("symlink_detected")
			}
		}
		if originAuthMode == "hmac" && !originAuthReady {
			return fmt.Errorf("origin_auth_not_ready")
		}
		return nil
	}

	activeAssetsCounter := func(ctx context.Context) (int, error) {
		return repository.CountActive(ctx, sqlDB)
	}
	catalogStatus := func() (bool, string) {
		return catalogHealth.Snapshot()
	}
	jobsStatusState := jobs.NewStatus(cfg.EnableJobs)
	jobsStatus := func() health.JobsStatusSnapshot {
		snapshot := jobsStatusState.Snapshot()
		return health.JobsStatusSnapshot{
			JobsEnabled:            snapshot.JobsEnabled,
			LastRescanAt:           snapshot.LastRescanAt,
			LastAnnounceSweepAt:    snapshot.LastAnnounceSweepAt,
			DeferredAnnouncesCount: snapshot.DeferredAnnouncesCount,
		}
	}

	srv := httpserver.NewWithDeps(
		cfg,
		logger,
		id,
		info.Version,
		activeAssetsCounter,
		catalogStatus,
		jobsStatus,
		readinessChecker,
		originSigner,
		rescanHandler,
		announceHandler,
		metricsRegistry,
		metricsCollector,
	)

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	jobsCtx, jobsCancel := context.WithCancel(context.Background())
	var jobRunner *jobs.Runner
	if cfg.EnableJobs {
		jobRunner = jobs.NewRunner(cfg, logger, sqlDB, sc, jobs.NewCatalogAnnouncer(catalogClient), id, jobsStatusState, metricsCollector)
		jobRunner.Start(jobsCtx)
		logger.Info("background jobs started",
			slog.Int("rescan_interval_seconds", cfg.RescanIntervalSeconds),
			slog.Int("announce_sweep_interval_seconds", cfg.AnnounceSweepIntervalSeconds),
			slog.Int("reannounce_threshold_seconds", cfg.ReannounceThresholdSeconds),
		)
	}
	if !cfg.ScanOnStartup {
		active, err := repository.CountActive(context.Background(), sqlDB)
		if err == nil {
			metricsCollector.SetActiveAssets(active)
		}
		missing, err := repository.CountMissing(context.Background(), sqlDB)
		if err == nil {
			metricsCollector.SetMissingAssets(missing)
		}
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	serverFailed := false
	select {
	case sig := <-sigCh:
		logger.Info("shutdown signal received", slog.String("signal", sig.String()))
	case err := <-errCh:
		logger.Error("server failed", slog.String("error", err.Error()))
		serverFailed = true
	}
	jobsCancel()
	if jobRunner != nil {
		jobRunner.Wait()
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.ShutdownTimeoutSeconds)*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("graceful shutdown failed", slog.String("error", err.Error()))
		if closeErr := srv.Close(); closeErr != nil {
			logger.Error("server close failed", slog.String("error", closeErr.Error()))
		}
		os.Exit(1)
	}

	logger.Info("server stopped")
	if serverFailed {
		os.Exit(1)
	}
}
