package jobs

import (
	"context"
	"database/sql"
	"encoding/hex"
	"path/filepath"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/google/uuid"

	"audistro-provider/internal/catalog"
	"audistro-provider/internal/config"
	"audistro-provider/internal/db"
	"audistro-provider/internal/identity"
	"audistro-provider/internal/repository"
)

type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	return c.now
}

type fakeAnnouncer struct {
	errs  []error
	calls int
}

func (a *fakeAnnouncer) Announce(_ context.Context, _ *identity.Identity, _ catalog.AnnounceRequest) error {
	a.calls++
	if len(a.errs) == 0 {
		return nil
	}
	err := a.errs[0]
	a.errs = a.errs[1:]
	return err
}

func TestRunAnnounceSweepBackoffAndStatusUpdate(t *testing.T) {
	sqlDB := openJobsTestDB(t)
	now := time.Unix(1700000000, 0).UTC()

	if err := repository.UpsertAssets(context.Background(), sqlDB, []repository.AssetRecord{{
		AssetID:            "asset1",
		MasterPlaylistPath: "asset1/master.m3u8",
		ContentHash:        "hash1",
		FileCount:          3,
		TotalSizeBytes:     1024,
		LastScannedAt:      now,
	}}, now); err != nil {
		t.Fatalf("seed assets failed: %v", err)
	}

	cfg := config.Config{
		PublicBaseURL:              "https://provider.example",
		Transport:                  "https",
		AnnouncePriority:           10,
		AnnounceExpiresSeconds:     600,
		ReannounceThresholdSeconds: 60,
	}
	id := testIdentity(t)
	clock := &fakeClock{now: now}
	backoff := NewBackoffState(BackoffConfig{
		Base:              2 * time.Second,
		Max:               60 * time.Second,
		RejectedRetry:     24 * time.Hour,
		UnauthorizedRetry: time.Hour,
	}, fixedJitter{value: 0})
	announcer := &fakeAnnouncer{errs: []error{catalog.ErrServer, nil}}

	first, err := RunAnnounceSweep(context.Background(), sqlDB, announcer, id, cfg, backoff, clock, nil)
	if err != nil {
		t.Fatalf("first sweep failed: %v", err)
	}
	if first.Attempted != 1 || first.Failed != 1 || first.Deferred != 0 {
		t.Fatalf("unexpected first sweep result: %+v", first)
	}

	row, err := repository.GetAnnouncement(context.Background(), sqlDB, "asset1")
	if err != nil {
		t.Fatalf("get announcement after first sweep: %v", err)
	}
	if row == nil || row.Status != "failed" {
		t.Fatalf("expected failed announcement row, got %+v", row)
	}

	second, err := RunAnnounceSweep(context.Background(), sqlDB, announcer, id, cfg, backoff, clock, nil)
	if err != nil {
		t.Fatalf("second sweep failed: %v", err)
	}
	if second.Attempted != 0 || second.Deferred != 1 {
		t.Fatalf("unexpected second sweep result: %+v", second)
	}
	if announcer.calls != 1 {
		t.Fatalf("expected no second announce call while deferred, got %d", announcer.calls)
	}

	clock.now = clock.now.Add(3 * time.Second)
	third, err := RunAnnounceSweep(context.Background(), sqlDB, announcer, id, cfg, backoff, clock, nil)
	if err != nil {
		t.Fatalf("third sweep failed: %v", err)
	}
	if third.Attempted != 1 || third.OK != 1 || third.Deferred != 0 {
		t.Fatalf("unexpected third sweep result: %+v", third)
	}

	row, err = repository.GetAnnouncement(context.Background(), sqlDB, "asset1")
	if err != nil {
		t.Fatalf("get announcement after third sweep: %v", err)
	}
	if row == nil || row.Status != "ok" {
		t.Fatalf("expected ok announcement row, got %+v", row)
	}
}

func openJobsTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dataPath := t.TempDir()
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

func testIdentity(t *testing.T) *identity.Identity {
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
