package internalapi_test

import (
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"audistro-provider/internal/config"
	"audistro-provider/internal/db"
	"audistro-provider/internal/internalapi"
	"audistro-provider/internal/scanner"
)

func TestRescanHandler(t *testing.T) {
	dataPath := t.TempDir()
	assetDir := filepath.Join(dataPath, "assets", "asset1")
	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		t.Fatalf("mkdir asset dir: %v", err)
	}
	mustWriteFile(t, filepath.Join(assetDir, "master.m3u8"), "#EXTM3U\n")
	mustWriteFile(t, filepath.Join(assetDir, "init.mp4"), "init")

	sqlDB := openTestDB(t, dataPath)
	sc := &scanner.Scanner{AssetsRoot: filepath.Join(dataPath, "assets")}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	h := internalapi.NewRescanHandler(sqlDB, sc, logger)

	req := httptest.NewRequest(http.MethodPost, "/internal/rescan", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["scanned_assets"].(float64) != 1 {
		t.Fatalf("expected scanned_assets=1, got %v", body["scanned_assets"])
	}
	if body["active_assets"].(float64) != 1 {
		t.Fatalf("expected active_assets=1, got %v", body["active_assets"])
	}
}

func openTestDB(t *testing.T, dataPath string) *sql.DB {
	t.Helper()
	cfg := config.Config{DataPath: dataPath, DBPath: filepath.Join(dataPath, "provider.db")}
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

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}
