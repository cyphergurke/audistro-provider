package repository_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"audistro-provider/internal/config"
	"audistro-provider/internal/db"
	"audistro-provider/internal/repository"
	"audistro-provider/internal/scanner"
)

func TestUpsertAndMarkMissing(t *testing.T) {
	ctx := context.Background()
	dataPath := t.TempDir()
	assetsRoot := filepath.Join(dataPath, "assets")
	assetDir := filepath.Join(assetsRoot, "asset1")
	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		t.Fatalf("mkdir asset dir: %v", err)
	}
	mustWriteFile(t, filepath.Join(assetDir, "master.m3u8"), "#EXTM3U\n")
	mustWriteFile(t, filepath.Join(assetDir, "seg_0001.ts"), "segment")

	sqlDB := openTestDB(t, dataPath)
	s := &scanner.Scanner{AssetsRoot: assetsRoot}

	res, err := s.Scan(ctx)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	records := toRecords(res.Assets)
	seen := seenIDs(res.Assets)
	now := time.Now().UTC()

	if err := repository.UpsertAssets(ctx, sqlDB, records, now); err != nil {
		t.Fatalf("upsert failed: %v", err)
	}
	missing, err := repository.MarkMissing(ctx, sqlDB, seen, now)
	if err != nil {
		t.Fatalf("mark missing failed: %v", err)
	}
	if missing != 0 {
		t.Fatalf("expected missing 0, got %d", missing)
	}

	active, err := repository.CountActive(ctx, sqlDB)
	if err != nil {
		t.Fatalf("count active failed: %v", err)
	}
	if active != 1 {
		t.Fatalf("expected active 1, got %d", active)
	}

	if err := os.RemoveAll(assetDir); err != nil {
		t.Fatalf("remove asset dir: %v", err)
	}
	res2, err := s.Scan(ctx)
	if err != nil {
		t.Fatalf("scan 2 failed: %v", err)
	}
	if err := repository.UpsertAssets(ctx, sqlDB, toRecords(res2.Assets), now.Add(time.Second)); err != nil {
		t.Fatalf("upsert 2 failed: %v", err)
	}
	missing, err = repository.MarkMissing(ctx, sqlDB, seenIDs(res2.Assets), now.Add(time.Second))
	if err != nil {
		t.Fatalf("mark missing 2 failed: %v", err)
	}
	if missing != 1 {
		t.Fatalf("expected missing 1, got %d", missing)
	}
	active, err = repository.CountActive(ctx, sqlDB)
	if err != nil {
		t.Fatalf("count active 2 failed: %v", err)
	}
	if active != 0 {
		t.Fatalf("expected active 0, got %d", active)
	}
}

func openTestDB(t *testing.T, dataPath string) *sql.DB {
	t.Helper()
	cfg := config.Config{
		DataPath: dataPath,
	}
	dbPath := filepath.Join(dataPath, "provider.db")
	_ = dbPath
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

func toRecords(in []scanner.Asset) []repository.AssetRecord {
	out := make([]repository.AssetRecord, 0, len(in))
	for _, a := range in {
		out = append(out, repository.AssetRecord{
			AssetID:            a.AssetID,
			MasterPlaylistPath: a.MasterPlaylistPath,
			ContentHash:        a.ContentHash,
			FileCount:          a.FileCount,
			TotalSizeBytes:     a.TotalSizeBytes,
			LastScannedAt:      a.LastScannedAt,
		})
	}
	return out
}

func seenIDs(in []scanner.Asset) []string {
	out := make([]string, 0, len(in))
	for _, a := range in {
		out = append(out, a.AssetID)
	}
	return out
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}
