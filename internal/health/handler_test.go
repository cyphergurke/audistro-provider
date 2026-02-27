package health_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"audistro-provider/internal/health"
)

func TestHealthzHandler(t *testing.T) {
	h := health.Handler(
		"audistro-provider",
		"v0.0.0-test",
		"us-east-1",
		"https://provider.example.com",
		"89e4ee5f-d005-48b7-a677-6dbc07324e8e",
		"03aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		func(context.Context) (int, error) { return 7, nil },
		func() (bool, string) { return true, "" },
		func() health.JobsStatusSnapshot {
			return health.JobsStatusSnapshot{
				JobsEnabled:            true,
				LastRescanAt:           time.Unix(1700000000, 0).UTC(),
				LastAnnounceSweepAt:    time.Unix(1700000060, 0).UTC(),
				DeferredAnnouncesCount: 3,
			}
		},
	)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("expected valid json: %v", err)
	}

	for _, key := range []string{"status", "service", "version", "time", "region", "public_base_url", "provider_id", "public_key", "active_assets_count", "catalog_enabled", "metrics_enabled", "jobs_enabled", "last_rescan_at", "last_announce_sweep_at", "deferred_announces_count"} {
		if _, ok := body[key]; !ok {
			t.Fatalf("expected key %q in response", key)
		}
	}

	if body["status"] != "ok" {
		t.Fatalf("expected status=ok, got %v", body["status"])
	}
	if body["metrics_enabled"] != true {
		t.Fatalf("expected metrics_enabled=true, got %v", body["metrics_enabled"])
	}
}

func TestHealthzHandlerWarningOnActiveCountError(t *testing.T) {
	h := health.Handler(
		"audistro-provider",
		"v0.0.0-test",
		"",
		"",
		"89e4ee5f-d005-48b7-a677-6dbc07324e8e",
		"03aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		func(context.Context) (int, error) { return 0, errors.New("db down") },
		func() (bool, string) { return true, "catalog down" },
		func() health.JobsStatusSnapshot {
			return health.JobsStatusSnapshot{
				JobsEnabled:            false,
				DeferredAnnouncesCount: 0,
			}
		},
	)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("expected valid json: %v", err)
	}
	if body["warning"] != "active_assets_count_unavailable" {
		t.Fatalf("expected warning field, got %v", body["warning"])
	}
	if _, ok := body["active_assets_count"]; ok {
		t.Fatal("did not expect active_assets_count when counter fails")
	}
	if body["catalog_last_error"] != "catalog down" {
		t.Fatalf("expected catalog_last_error field, got %v", body["catalog_last_error"])
	}
	if body["jobs_enabled"] != false {
		t.Fatalf("expected jobs_enabled=false, got %v", body["jobs_enabled"])
	}
}
