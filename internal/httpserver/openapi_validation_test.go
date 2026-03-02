package httpserver

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"audistro-provider/internal/config"
	"audistro-provider/internal/identity"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/google/uuid"
)

func TestOpenAPIValidationRejectsWrongContentTypeForInternalAnnounce(t *testing.T) {
	cfg := config.Config{
		HTTPAddr:                    ":0",
		DataPath:                    t.TempDir(),
		AssetsSubdir:                "assets",
		StorageMode:                 config.StorageModeFilesystem,
		MaxSegmentBytes:             1024,
		RateLimitRPS:                10,
		RateLimitBurst:              20,
		ProxyMaxUpstreamConcurrency: 8,
		InternalEnable:              true,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	priv, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatalf("create private key: %v", err)
	}
	id := &identity.Identity{ProviderID: uuid.New(), PrivateKey: priv, PublicKey: priv.PubKey()}
	srv := NewWithDeps(
		cfg,
		logger,
		id,
		"test",
		nil,
		nil,
		nil,
		nil,
		nil,
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }),
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }),
		nil,
		nil,
	)

	req := httptest.NewRequest(http.MethodPost, "/internal/announce", bytes.NewReader([]byte(`{"asset_id":"asset1"}`)))
	req.Header.Set("Content-Type", "text/plain")
	rr := httptest.NewRecorder()
	srv.HTTP.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	var out validationErrorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Error != "invalid_request" {
		t.Fatalf("unexpected error response: %#v", out)
	}
}
