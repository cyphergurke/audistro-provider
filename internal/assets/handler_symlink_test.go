package assets

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestGetSegmentSymlinkBlocked(t *testing.T) {
	dataPath := t.TempDir()
	assetDir := filepath.Join(dataPath, "assets", "asset1")
	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		t.Fatalf("mkdir asset dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(assetDir, "master.m3u8"), []byte("#EXTM3U\n"), 0o644); err != nil {
		t.Fatalf("write master: %v", err)
	}

	outside := filepath.Join(dataPath, "outside.ts")
	if err := os.WriteFile(outside, []byte("outside-content"), 0o644); err != nil {
		t.Fatalf("write outside: %v", err)
	}

	linkPath := filepath.Join(assetDir, "seg_0001.ts")
	if err := os.Symlink(outside, linkPath); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	h := NewHandler(HandlerConfig{
		AssetsRoot:      filepath.Join(dataPath, "assets"),
		MaxSegmentBytes: 67108864,
	})

	req := httptest.NewRequest(http.MethodGet, "/assets/asset1/seg_0001.ts", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code == http.StatusOK {
		t.Fatalf("expected symlink request to be blocked, got status %d", rr.Code)
	}
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rr.Code)
	}
}
