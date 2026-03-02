package httpserver

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"audistro-provider/internal/config"
	"audistro-provider/internal/identity"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/google/uuid"
)

func TestOpenAPISpecEndpoint(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil)
	rr := httptest.NewRecorder()

	srv.HTTP.Handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "application/yaml") {
		t.Fatalf("unexpected content type: %q", ct)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "openapi: 3.0.3") {
		t.Fatalf("expected openapi version in response")
	}
	if !strings.Contains(body, "/internal/announce:") {
		t.Fatalf("expected internal announce path in response")
	}

	jsonReq := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	jsonRR := httptest.NewRecorder()
	srv.HTTP.Handler.ServeHTTP(jsonRR, jsonReq)
	if jsonRR.Code != http.StatusOK {
		t.Fatalf("expected json spec status 200, got %d", jsonRR.Code)
	}
	if ct := jsonRR.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("unexpected json content type: %q", ct)
	}
	var doc map[string]any
	if err := json.Unmarshal(jsonRR.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode json spec: %v", err)
	}
	if doc["openapi"] != "3.0.3" {
		t.Fatalf("unexpected openapi version: %#v", doc["openapi"])
	}
}

func TestScalarDocsEndpoint(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/docs", nil)
	rr := httptest.NewRecorder()

	srv.HTTP.Handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("unexpected content type: %q", ct)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "@scalar/api-reference") {
		t.Fatalf("expected scalar script include in docs html")
	}
	if !strings.Contains(body, "data-url=\"/openapi.json\"") {
		t.Fatalf("expected openapi spec URL in docs html")
	}
}

func newTestServer(t *testing.T) *Server {
	t.Helper()

	cfg := config.Config{
		HTTPAddr:                    ":0",
		DataPath:                    t.TempDir(),
		AssetsSubdir:                "assets",
		StorageMode:                 config.StorageModeFilesystem,
		MaxSegmentBytes:             1024,
		RateLimitRPS:                10,
		RateLimitBurst:              20,
		ProxyMaxUpstreamConcurrency: 8,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	id := newTestIdentity(t)
	return New(cfg, logger, id, "test")
}

func newTestIdentity(t *testing.T) *identity.Identity {
	t.Helper()

	priv, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatalf("create private key: %v", err)
	}
	return &identity.Identity{
		ProviderID: uuid.New(),
		PrivateKey: priv,
		PublicKey:  priv.PubKey(),
		CreatedAt:  time.Now().UTC(),
	}
}
