package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScanFindsValidAssets(t *testing.T) {
	root := filepath.Join(t.TempDir(), "assets")
	assetDir := filepath.Join(root, "asset1")
	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		t.Fatalf("mkdir asset dir: %v", err)
	}
	mustWriteFile(t, filepath.Join(assetDir, "master.m3u8"), "#EXTM3U\n")
	mustWriteFile(t, filepath.Join(assetDir, "seg_0001.m4s"), "segment")
	mustWriteFile(t, filepath.Join(assetDir, "ignore.txt"), "nope")

	s := &Scanner{AssetsRoot: root}
	res, err := s.Scan(context.Background())
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	if res.ScannedAssets != 1 {
		t.Fatalf("expected 1 scanned asset, got %d", res.ScannedAssets)
	}
	if len(res.Assets) != 1 {
		t.Fatalf("expected 1 asset record, got %d", len(res.Assets))
	}
}

func TestContentHashChangesOnSizeOrMTime(t *testing.T) {
	root := filepath.Join(t.TempDir(), "assets")
	assetDir := filepath.Join(root, "asset1")
	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		t.Fatalf("mkdir asset dir: %v", err)
	}
	masterPath := filepath.Join(assetDir, "master.m3u8")
	segPath := filepath.Join(assetDir, "seg_0001.ts")
	mustWriteFile(t, masterPath, "#EXTM3U\n")
	mustWriteFile(t, segPath, "abc")

	s := &Scanner{AssetsRoot: root}
	res1, err := s.Scan(context.Background())
	if err != nil {
		t.Fatalf("scan 1 failed: %v", err)
	}
	h1 := res1.Assets[0].ContentHash

	mustWriteFile(t, segPath, "abcdef")
	res2, err := s.Scan(context.Background())
	if err != nil {
		t.Fatalf("scan 2 failed: %v", err)
	}
	h2 := res2.Assets[0].ContentHash
	if h1 == h2 {
		t.Fatal("expected content hash to change after size change")
	}

	fi, err := os.Stat(masterPath)
	if err != nil {
		t.Fatalf("stat master: %v", err)
	}
	newTime := fi.ModTime().Add(2 * time.Second)
	if err := os.Chtimes(masterPath, newTime, newTime); err != nil {
		t.Fatalf("chtimes master: %v", err)
	}

	res3, err := s.Scan(context.Background())
	if err != nil {
		t.Fatalf("scan 3 failed: %v", err)
	}
	h3 := res3.Assets[0].ContentHash
	if h2 == h3 {
		t.Fatal("expected content hash to change after mtime change")
	}
}

func TestScanSkipsAssetWithAllowlistedSymlink(t *testing.T) {
	root := filepath.Join(t.TempDir(), "assets")
	assetDir := filepath.Join(root, "asset1")
	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		t.Fatalf("mkdir asset dir: %v", err)
	}
	mustWriteFile(t, filepath.Join(assetDir, "master.m3u8"), "#EXTM3U\n")
	realSegment := filepath.Join(root, "real.ts")
	mustWriteFile(t, realSegment, "segment")
	if err := os.Symlink(realSegment, filepath.Join(assetDir, "seg_0001.ts")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	s := &Scanner{AssetsRoot: root}
	res, err := s.Scan(context.Background())
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	if res.ScannedAssets != 0 {
		t.Fatalf("expected 0 scanned assets, got %d", res.ScannedAssets)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}
