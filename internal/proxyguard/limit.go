package proxyguard

import "context"

type Semaphore struct {
	ch chan struct{}
}

func NewSemaphore(size int) *Semaphore {
	if size <= 0 {
		return nil
	}
	return &Semaphore{ch: make(chan struct{}, size)}
}

func (s *Semaphore) Acquire(ctx context.Context) bool {
	if s == nil || s.ch == nil {
		return true
	}
	select {
	case <-ctx.Done():
		return false
	default:
	}
	select {
	case s.ch <- struct{}{}:
		return true
	default:
		return false
	}
}

func (s *Semaphore) Release() {
	if s == nil || s.ch == nil {
		return
	}
	select {
	case <-s.ch:
	default:
	}
}
