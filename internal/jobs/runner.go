package jobs

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"
	"time"

	"audistro-provider/internal/config"
	"audistro-provider/internal/identity"
	"audistro-provider/internal/metrics"
	"audistro-provider/internal/repository"
	"audistro-provider/internal/scanner"
)

type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time {
	return time.Now().UTC()
}

type StatusSnapshot struct {
	JobsEnabled            bool
	LastRescanAt           time.Time
	LastAnnounceSweepAt    time.Time
	DeferredAnnouncesCount int
}

type Status struct {
	mu                     sync.RWMutex
	jobsEnabled            bool
	lastRescanAt           time.Time
	lastAnnounceSweepAt    time.Time
	deferredAnnouncesCount int
}

func NewStatus(enabled bool) *Status {
	return &Status{jobsEnabled: enabled}
}

func (s *Status) Snapshot() StatusSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return StatusSnapshot{
		JobsEnabled:            s.jobsEnabled,
		LastRescanAt:           s.lastRescanAt,
		LastAnnounceSweepAt:    s.lastAnnounceSweepAt,
		DeferredAnnouncesCount: s.deferredAnnouncesCount,
	}
}

func (s *Status) setLastRescanAt(t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastRescanAt = t
}

func (s *Status) setLastAnnounceSweep(t time.Time, deferred int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastAnnounceSweepAt = t
	s.deferredAnnouncesCount = deferred
}

type Runner struct {
	cfg       config.Config
	logger    *slog.Logger
	db        *sql.DB
	scanner   *scanner.Scanner
	announcer CatalogAnnouncer
	identity  *identity.Identity
	clock     Clock
	backoff   *BackoffState
	status    *Status
	metrics   *metrics.Metrics

	wg sync.WaitGroup
}

func NewRunner(
	cfg config.Config,
	logger *slog.Logger,
	db *sql.DB,
	scanner *scanner.Scanner,
	announcer CatalogAnnouncer,
	id *identity.Identity,
	status *Status,
	metricsCollector *metrics.Metrics,
) *Runner {
	if status == nil {
		status = NewStatus(cfg.EnableJobs)
	}
	return &Runner{
		cfg:       cfg,
		logger:    logger,
		db:        db,
		scanner:   scanner,
		announcer: announcer,
		identity:  id,
		clock:     realClock{},
		backoff:   NewBackoffStateFromConfig(cfg, nil),
		status:    status,
		metrics:   metricsCollector,
	}
}

func (r *Runner) SetClock(clock Clock) {
	if clock == nil {
		return
	}
	r.clock = clock
}

func (r *Runner) SetBackoffState(state *BackoffState) {
	if state == nil {
		return
	}
	r.backoff = state
}

func (r *Runner) Start(ctx context.Context) {
	if !r.cfg.EnableJobs {
		return
	}

	r.wg.Add(3)
	go r.rescanLoop(ctx)
	go r.announceSweepLoop(ctx)
	go r.cleanupLoop(ctx)
}

func (r *Runner) Wait() {
	r.wg.Wait()
}

func (r *Runner) rescanLoop(ctx context.Context) {
	defer r.wg.Done()
	interval := time.Duration(r.cfg.RescanIntervalSeconds) * time.Second
	r.runPeriodic(ctx, interval, func(runCtx context.Context) {
		result, err := RunRescanJob(runCtx, r.db, r.scanner, r.clock)
		if err != nil {
			r.logger.Error("rescan job failed", slog.String("error", err.Error()))
			return
		}
		if r.metrics != nil {
			r.metrics.SetActiveAssets(result.ActiveAssets)
			missing, countErr := repository.CountMissing(runCtx, r.db)
			if countErr == nil {
				r.metrics.SetMissingAssets(missing)
			}
		}
		r.status.setLastRescanAt(r.clock.Now().UTC())
	})
}

func (r *Runner) announceSweepLoop(ctx context.Context) {
	defer r.wg.Done()
	interval := time.Duration(r.cfg.AnnounceSweepIntervalSeconds) * time.Second
	r.runPeriodic(ctx, interval, func(runCtx context.Context) {
		_, err := RunAnnounceSweep(runCtx, r.db, r.announcer, r.identity, r.cfg, r.backoff, r.clock, r.metrics)
		if err != nil {
			r.logger.Error("announce sweep failed", slog.String("error", err.Error()))
			if r.metrics != nil {
				r.metrics.ObserveAnnounceSweep("error")
			}
			return
		}
		deferred := r.backoff.CountDeferred(r.clock.Now().UTC())
		r.status.setLastAnnounceSweep(r.clock.Now().UTC(), deferred)
		if r.metrics != nil {
			r.metrics.SetDeferredAnnounces(deferred)
			r.metrics.ObserveAnnounceSweep("ok")
		}
	})
}

func (r *Runner) cleanupLoop(ctx context.Context) {
	defer r.wg.Done()
	interval := time.Duration(r.cfg.AnnounceSweepIntervalSeconds) * time.Second
	r.runPeriodic(ctx, interval, func(runCtx context.Context) {
		_, err := RunCleanupJob(runCtx, r.db, r.cfg.AnnouncementExpiryGraceSeconds, r.clock)
		if err != nil {
			r.logger.Error("cleanup job failed", slog.String("error", err.Error()))
		}
	})
}

func (r *Runner) runPeriodic(ctx context.Context, interval time.Duration, fn func(context.Context)) {
	if interval <= 0 {
		interval = 5 * time.Minute
	}

	fn(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fn(ctx)
		}
	}
}
