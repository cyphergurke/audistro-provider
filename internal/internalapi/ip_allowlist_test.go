package internalapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"audistro-provider/internal/config"
)

func TestIPAllowlistLoopbackAllowed(t *testing.T) {
	mw := NewIPAllowlistMiddleware(config.Config{
		InternalAllowedCIDRs: "127.0.0.1/32,::1/128",
	}, nil)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/internal/rescan", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
}

func TestIPAllowlistRejectsNonAllowedIP(t *testing.T) {
	mw := NewIPAllowlistMiddleware(config.Config{
		InternalAllowedCIDRs: "127.0.0.1/32,::1/128",
	}, nil)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/internal/rescan", nil)
	req.RemoteAddr = "1.2.3.4:12345"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", rr.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if body["error"] != "forbidden" {
		t.Fatalf("expected error=forbidden, got %q", body["error"])
	}
}
