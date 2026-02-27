package catalog

import "sync"

type HealthState struct {
	enabled bool
	mu      sync.RWMutex
	lastErr string
}

func NewHealthState(enabled bool) *HealthState {
	return &HealthState{enabled: enabled}
}

func (s *HealthState) Set(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil {
		s.lastErr = ""
		return
	}
	s.lastErr = err.Error()
}

func (s *HealthState) Snapshot() (bool, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.enabled, s.lastErr
}
