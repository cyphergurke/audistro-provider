package jobs

import (
	"testing"
	"time"
)

type fixedJitter struct {
	value int64
}

func (j fixedJitter) Int63n(n int64) int64 {
	if n <= 0 {
		return 0
	}
	if j.value < 0 {
		return 0
	}
	if j.value >= n {
		return n - 1
	}
	return j.value
}

func TestBackoffNextAttemptExponentialClamp(t *testing.T) {
	state := NewBackoffState(BackoffConfig{
		Base:              2 * time.Second,
		Max:               10 * time.Second,
		RejectedRetry:     24 * time.Hour,
		UnauthorizedRetry: time.Hour,
	}, fixedJitter{value: 0})

	start := time.Unix(1700000000, 0).UTC()

	state.Record("asset1", ClassTransient, start)
	entry := state.entries["asset1"]
	if got, want := entry.FailCount, 1; got != want {
		t.Fatalf("fail count mismatch: got %d want %d", got, want)
	}
	if got, want := entry.NextAttemptAt, start.Add(2*time.Second); !got.Equal(want) {
		t.Fatalf("next attempt mismatch: got %s want %s", got, want)
	}

	second := start.Add(3 * time.Second)
	state.Record("asset1", ClassTransient, second)
	entry = state.entries["asset1"]
	if got, want := entry.NextAttemptAt, second.Add(4*time.Second); !got.Equal(want) {
		t.Fatalf("next attempt mismatch second failure: got %s want %s", got, want)
	}

	third := second.Add(5 * time.Second)
	state.Record("asset1", ClassTransient, third)
	entry = state.entries["asset1"]
	if got, want := entry.NextAttemptAt, third.Add(8*time.Second); !got.Equal(want) {
		t.Fatalf("next attempt mismatch third failure: got %s want %s", got, want)
	}

	fourth := third.Add(9 * time.Second)
	state.Record("asset1", ClassTransient, fourth)
	entry = state.entries["asset1"]
	if got, want := entry.NextAttemptAt, fourth.Add(10*time.Second); !got.Equal(want) {
		t.Fatalf("next attempt mismatch clamped failure: got %s want %s", got, want)
	}
}

func TestBackoffRejectedAndUnauthorizedDelays(t *testing.T) {
	state := NewBackoffState(BackoffConfig{
		Base:              2 * time.Second,
		Max:               10 * time.Second,
		RejectedRetry:     24 * time.Hour,
		UnauthorizedRetry: time.Hour,
	}, fixedJitter{value: 0})

	now := time.Unix(1700000000, 0).UTC()

	state.Record("asset-rejected", ClassRejected, now)
	rejected := state.entries["asset-rejected"]
	if got, want := rejected.NextAttemptAt, now.Add(24*time.Hour); !got.Equal(want) {
		t.Fatalf("rejected retry mismatch: got %s want %s", got, want)
	}

	state.Record("asset-unauthorized", ClassUnauthorized, now)
	unauthorized := state.entries["asset-unauthorized"]
	if got, want := unauthorized.NextAttemptAt, now.Add(time.Hour); !got.Equal(want) {
		t.Fatalf("unauthorized retry mismatch: got %s want %s", got, want)
	}
}
