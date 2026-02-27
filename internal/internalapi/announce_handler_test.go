package internalapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/google/uuid"

	"audistro-provider/internal/catalog"
	"audistro-provider/internal/config"
	"audistro-provider/internal/db"
	"audistro-provider/internal/identity"
	"audistro-provider/internal/repository"
)

func TestAnnounceHandler(t *testing.T) {
	var providerReq catalog.EnsureProviderRequest
	var announceReq catalog.AnnounceRequest

	catalogServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/providers":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if err := json.NewDecoder(r.Body).Decode(&providerReq); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v1/providers/") && strings.HasSuffix(r.URL.Path, "/announce"):
			if err := json.NewDecoder(r.Body).Decode(&announceReq); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer catalogServer.Close()

	dataPath := t.TempDir()
	cfg := config.Config{
		DataPath:               dataPath,
		DBPath:                 filepath.Join(dataPath, "provider.db"),
		PublicBaseURL:          "https://provider.example",
		Transport:              "https",
		AnnouncePriority:       10,
		AnnounceExpiresSeconds: 604800,
		CatalogBaseURL:         catalogServer.URL,
		CatalogTimeoutSeconds:  5,
	}

	sqlDB := openTestDB(t, cfg)
	id := newTestIdentity(t)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	client, err := catalog.NewClient(cfg.CatalogBaseURL, 5*time.Second, logger, nil)
	if err != nil {
		t.Fatalf("new catalog client failed: %v", err)
	}
	if err := client.EnsureProvider(context.Background(), id, cfg); err != nil {
		t.Fatalf("ensure provider failed: %v", err)
	}

	fixedNow := time.Unix(1700000000, 0).UTC()
	oldNow := announceNow
	announceNow = func() time.Time { return fixedNow }
	t.Cleanup(func() { announceNow = oldNow })

	nonceRestore := catalog.NonceReaderForTest(bytes.NewReader([]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99}))
	t.Cleanup(nonceRestore)

	if err := repository.UpsertAssets(context.Background(), sqlDB, []repository.AssetRecord{{
		AssetID:            "asset1",
		MasterPlaylistPath: "asset1/master.m3u8",
		ContentHash:        "abc",
		FileCount:          2,
		TotalSizeBytes:     12,
		LastScannedAt:      fixedNow,
	}}, fixedNow); err != nil {
		t.Fatalf("seed active assets failed: %v", err)
	}

	health := catalog.NewHealthState(true)
	h := NewAnnounceHandler(sqlDB, id, cfg, client, logger, health)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/internal/announce", nil)
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	if providerReq.ProviderID != id.ProviderID.String() {
		t.Fatalf("provider registration payload mismatch: %q", providerReq.ProviderID)
	}
	if announceReq.BaseURL != "https://provider.example/assets/asset1" {
		t.Fatalf("unexpected announce base_url: %q", announceReq.BaseURL)
	}
	wantExpires := fixedNow.Add(time.Duration(cfg.AnnounceExpiresSeconds) * time.Second).Unix()
	if announceReq.ExpiresAt != wantExpires {
		t.Fatalf("unexpected expires_at: want %d got %d", wantExpires, announceReq.ExpiresAt)
	}

	msg := id.ProviderID.String() + "|asset1|" + announceReq.Transport + "|" + announceReq.BaseURL + "|" + strconv.FormatInt(announceReq.ExpiresAt, 10) + "|" + announceReq.Nonce
	hash := sha256.Sum256([]byte(msg))
	sigBytes, err := hex.DecodeString(announceReq.Signature)
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	sig, err := schnorr.ParseSignature(sigBytes)
	if err != nil {
		t.Fatalf("parse signature: %v", err)
	}
	if !sig.Verify(hash[:], id.PublicKey) {
		t.Fatal("announce signature verification failed")
	}

	var summary map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &summary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if summary["attempted"].(float64) != 1 || summary["ok"].(float64) != 1 || summary["rejected"].(float64) != 0 || summary["failed"].(float64) != 0 {
		t.Fatalf("unexpected summary: %v", summary)
	}
}

func openTestDB(t *testing.T, cfg config.Config) *sql.DB {
	t.Helper()
	sqlDB, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("open db failed: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.Migrate(sqlDB); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}
	return sqlDB
}

func newTestIdentity(t *testing.T) *identity.Identity {
	t.Helper()
	privHex := "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
	privBytes, err := hex.DecodeString(privHex)
	if err != nil {
		t.Fatalf("decode private key: %v", err)
	}
	priv, pub := btcec.PrivKeyFromBytes(privBytes)
	return &identity.Identity{
		ProviderID: uuid.MustParse("11111111-2222-3333-4444-555555555555"),
		PrivateKey: priv,
		PublicKey:  pub,
		CreatedAt:  time.Unix(1700000000, 0).UTC(),
	}
}
