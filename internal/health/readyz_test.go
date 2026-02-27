package health_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"audistro-provider/internal/config"
	"audistro-provider/internal/db"
	"audistro-provider/internal/health"
)

func TestReadyzHandlerReady(t *testing.T) {
	dataPath := t.TempDir()
	assetsRoot := filepath.Join(dataPath, "assets")
	if err := os.MkdirAll(assetsRoot, 0o700); err != nil {
		t.Fatalf("mkdir assets root: %v", err)
	}

	sqlDB := openReadyDB(t, dataPath)
	checker := func(ctx context.Context) error {
		if err := sqlDB.PingContext(ctx); err != nil {
			return fmt.Errorf("db_unavailable")
		}
		info, err := os.Stat(assetsRoot)
		if err != nil {
			return fmt.Errorf("assets_root_unavailable")
		}
		if !info.IsDir() {
			return fmt.Errorf("assets_root_not_directory")
		}
		return nil
	}

	h := health.ReadyHandler(checker)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if body["status"] != "ready" {
		t.Fatalf("expected status=ready, got %v", body["status"])
	}
}

func openReadyDB(t *testing.T, dataPath string) *sql.DB {
	t.Helper()
	cfg := config.Config{
		DataPath: dataPath,
		DBPath:   filepath.Join(dataPath, "provider.db"),
	}
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
