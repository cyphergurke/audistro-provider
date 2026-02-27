package fssecure

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenFileNoSymlinksRegularFile(t *testing.T) {
	root := filepath.Join(t.TempDir(), "asset1")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}
	target := filepath.Join(root, "seg_0001.ts")
	if err := os.WriteFile(target, []byte("segment"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}

	f, info, err := OpenFileNoSymlinks(root, "seg_0001.ts")
	if err != nil {
		t.Fatalf("open file: %v", err)
	}
	_ = f.Close()

	if info == nil || !info.Mode().IsRegular() {
		t.Fatalf("expected regular file info, got %#v", info)
	}
}

func TestOpenFileNoSymlinksRejectsSymlink(t *testing.T) {
	root := filepath.Join(t.TempDir(), "asset1")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}
	outside := filepath.Join(t.TempDir(), "outside.ts")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatalf("write outside: %v", err)
	}
	link := filepath.Join(root, "seg_0001.ts")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	_, _, err := OpenFileNoSymlinks(root, "seg_0001.ts")
	if err == nil {
		t.Fatal("expected symlink rejection error")
	}
	if err != ErrSymlink {
		t.Fatalf("expected ErrSymlink, got %v", err)
	}
}

func TestAuditNoSymlinksDetectsSymlinkedDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "assets")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}

	realDir := filepath.Join(t.TempDir(), "real_asset")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatalf("mkdir real dir: %v", err)
	}
	if err := os.Symlink(realDir, filepath.Join(root, "asset_link")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	found, err := AuditNoSymlinks(root, func(name string) bool { return true })
	if err != nil {
		t.Fatalf("audit failed: %v", err)
	}
	if !found {
		t.Fatal("expected symlink to be detected")
	}
}
