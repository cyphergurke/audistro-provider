package httpserver

import (
	"net/http"
	"strings"
	"time"

	"audistro-provider/internal/metrics"
)

func HTTPMetrics(m *metrics.Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			route := routeLabel(r.URL.Path)
			done := m.IncInFlight(route)
			defer done()

			start := time.Now()
			mw := &metricsResponseWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(mw, r)

			m.ObserveHTTPRequest(route, r.Method, mw.status, time.Since(start), mw.bytes)
		})
	}
}

func routeLabel(path string) string {
	switch {
	case path == "/healthz":
		return "healthz"
	case path == "/readyz":
		return "readyz"
	case path == "/metrics":
		return "metrics"
	case strings.HasPrefix(path, "/assets/"):
		return "assets"
	case path == "/internal/rescan":
		return "internal_rescan"
	case path == "/internal/announce":
		return "internal_announce"
	default:
		return "other"
	}
}

type metricsResponseWriter struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (w *metricsResponseWriter) WriteHeader(statusCode int) {
	w.status = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *metricsResponseWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.bytes += int64(n)
	return n, err
}
