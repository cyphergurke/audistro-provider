package assets

import (
	"net"
	"net/http"
	"sync"
	"time"
)

type RateLimitConfig struct {
	RPS                 int
	Burst               int
	TrustProxyAddresses bool
}

type ipRateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rps     float64
	burst   float64
	nowFn   func() time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

func NewRateLimitMiddleware(cfg RateLimitConfig) func(http.Handler) http.Handler {
	limiter := &ipRateLimiter{
		buckets: make(map[string]*bucket),
		rps:     float64(max(1, cfg.RPS)),
		burst:   float64(max(1, cfg.Burst)),
		nowFn:   time.Now,
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiter.allow(clientIP(r.RemoteAddr)) {
				writeJSONError(w, http.StatusTooManyRequests, "rate_limited")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (l *ipRateLimiter) allow(ip string) bool {
	now := l.nowFn()

	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.buckets[ip]
	if !ok {
		l.buckets[ip] = &bucket{tokens: l.burst - 1, last: now}
		return true
	}

	elapsed := now.Sub(b.last).Seconds()
	if elapsed > 0 {
		b.tokens = min(l.burst, b.tokens+elapsed*l.rps)
		b.last = now
	}

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func clientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
