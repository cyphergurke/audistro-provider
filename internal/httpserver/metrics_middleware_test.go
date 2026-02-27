package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"audistro-provider/internal/metrics"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestHTTPMetricsMiddlewareHealthzCounter(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := metrics.New(registry)

	h := HTTPMetrics(m)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	got := testutil.ToFloat64(m.HTTPRequestsTotal.WithLabelValues("healthz", http.MethodGet, "200"))
	if got != 1 {
		t.Fatalf("expected request counter 1, got %v", got)
	}
}
