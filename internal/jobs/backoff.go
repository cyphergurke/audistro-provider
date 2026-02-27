package jobs

import (
	"errors"
	"math/rand"
	"sync"
	"time"

	"audistro-provider/internal/catalog"
	"audistro-provider/internal/config"
)

type ErrorClass string

const (
	ClassOK           ErrorClass = "ok"
	ClassRejected     ErrorClass = "rejected"
	ClassUnauthorized ErrorClass = "unauthorized"
	ClassTransient    ErrorClass = "transient"
	ClassPermanent    ErrorClass = "permanent"
)

type Jitter interface {
	Int63n(n int64) int64
}

type BackoffConfig struct {
	Base              time.Duration
	Max               time.Duration
	RejectedRetry     time.Duration
	UnauthorizedRetry time.Duration
}

type BackoffEntry struct {
	FailCount     int
	NextAttemptAt time.Time
}

type BackoffState struct {
	mu      sync.RWMutex
	entries map[string]BackoffEntry
	cfg     BackoffConfig
	jitter  Jitter
}

func NewBackoffState(cfg BackoffConfig, jitter Jitter) *BackoffState {
	if cfg.Base <= 0 {
		cfg.Base = 2 * time.Second
	}
	if cfg.Max <= 0 {
		cfg.Max = 10 * time.Minute
	}
	if cfg.RejectedRetry <= 0 {
		cfg.RejectedRetry = 24 * time.Hour
	}
	if cfg.UnauthorizedRetry <= 0 {
		cfg.UnauthorizedRetry = time.Hour
	}
	if jitter == nil {
		jitter = &defaultJitter{rnd: rand.New(rand.NewSource(time.Now().UnixNano()))}
	}
	return &BackoffState{
		entries: make(map[string]BackoffEntry),
		cfg:     cfg,
		jitter:  jitter,
	}
}

func NewBackoffStateFromConfig(cfg config.Config, jitter Jitter) *BackoffState {
	return NewBackoffState(BackoffConfig{
		Base:              time.Duration(cfg.BackoffBaseMillis) * time.Millisecond,
		Max:               time.Duration(cfg.BackoffMaxSeconds) * time.Second,
		RejectedRetry:     time.Duration(cfg.RejectedRetrySeconds) * time.Second,
		UnauthorizedRetry: time.Duration(cfg.UnauthorizedRetrySeconds) * time.Second,
	}, jitter)
}

func (b *BackoffState) CanAttempt(assetID string, now time.Time) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	entry, ok := b.entries[assetID]
	if !ok {
		return true
	}
	return !entry.NextAttemptAt.After(now)
}

func (b *BackoffState) CountDeferred(now time.Time) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	count := 0
	for _, entry := range b.entries {
		if entry.NextAttemptAt.After(now) {
			count++
		}
	}
	return count
}

func (b *BackoffState) Record(assetID string, class ErrorClass, now time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch class {
	case ClassOK:
		delete(b.entries, assetID)
	case ClassRejected:
		b.entries[assetID] = BackoffEntry{FailCount: 0, NextAttemptAt: now.Add(b.cfg.RejectedRetry)}
	case ClassUnauthorized:
		b.entries[assetID] = BackoffEntry{FailCount: 0, NextAttemptAt: now.Add(b.cfg.UnauthorizedRetry)}
	case ClassPermanent:
		entry := b.entries[assetID]
		entry.FailCount++
		entry.NextAttemptAt = now.Add(b.cfg.Max)
		b.entries[assetID] = entry
	case ClassTransient:
		fallthrough
	default:
		entry := b.entries[assetID]
		entry.FailCount++
		entry.NextAttemptAt = b.NextAttempt(now, entry.FailCount)
		b.entries[assetID] = entry
	}
}

func (b *BackoffState) NextAttempt(now time.Time, failCount int) time.Time {
	if failCount < 1 {
		failCount = 1
	}

	delay := b.cfg.Base
	for i := 1; i < failCount; i++ {
		if delay >= b.cfg.Max {
			delay = b.cfg.Max
			break
		}
		delay *= 2
		if delay > b.cfg.Max {
			delay = b.cfg.Max
			break
		}
	}

	jitterWindow := delay / 4
	if jitterWindow > 0 {
		extra := time.Duration(b.jitter.Int63n(int64(jitterWindow) + 1))
		delay += extra
	}
	if delay > b.cfg.Max {
		delay = b.cfg.Max
	}
	return now.Add(delay)
}

func ClassifyError(err error) ErrorClass {
	if err == nil {
		return ClassOK
	}
	if errors.Is(err, catalog.ErrNotFound) {
		return ClassRejected
	}
	if errors.Is(err, catalog.ErrUnauthorized) {
		return ClassUnauthorized
	}
	if errors.Is(err, catalog.ErrBadRequest) || errors.Is(err, catalog.ErrConflict) {
		return ClassPermanent
	}
	if errors.Is(err, catalog.ErrServer) || errors.Is(err, catalog.ErrUnexpected) {
		return ClassTransient
	}
	return ClassTransient
}

type defaultJitter struct {
	mu  sync.Mutex
	rnd *rand.Rand
}

func (j *defaultJitter) Int63n(n int64) int64 {
	if n <= 0 {
		return 0
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.rnd.Int63n(n)
}
