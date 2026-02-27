package fssecure

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var ErrSymlink = errors.New("symlink not allowed")
var ErrInvalidFilename = errors.New("invalid filename")
var ErrNotRegular = errors.New("not a regular file")
var errAuditFoundSymlink = errors.New("symlink found")

func IsSymlink(fi os.FileInfo) bool {
	if fi == nil {
		return false
	}
	return fi.Mode()&os.ModeSymlink != 0
}

func OpenFileNoSymlinks(rootDir string, filename string) (*os.File, os.FileInfo, error) {
	if filename == "" || filename != filepath.Base(filename) || strings.Contains(filename, "/") || strings.Contains(filename, `\`) {
		return nil, nil, ErrInvalidFilename
	}

	rootInfo, err := os.Lstat(rootDir)
	if err != nil {
		return nil, nil, err
	}
	if IsSymlink(rootInfo) {
		return nil, nil, ErrSymlink
	}
	if !rootInfo.IsDir() {
		return nil, nil, ErrNotRegular
	}

	targetPath := filepath.Join(rootDir, filename)
	targetInfo, err := os.Lstat(targetPath)
	if err != nil {
		return nil, nil, err
	}
	if IsSymlink(targetInfo) {
		return nil, nil, ErrSymlink
	}
	if !targetInfo.Mode().IsRegular() {
		return nil, nil, ErrNotRegular
	}

	f, err := os.Open(targetPath)
	if err != nil {
		return nil, nil, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		_ = f.Close()
		return nil, nil, ErrNotRegular
	}
	if !os.SameFile(targetInfo, info) {
		_ = f.Close()
		return nil, nil, ErrSymlink
	}
	return f, info, nil
}

func AuditNoSymlinks(root string, allowedExt func(name string) bool) (bool, error) {
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.Type()&os.ModeSymlink != 0 {
			return errAuditFoundSymlink
		}
		if !d.IsDir() && allowedExt != nil {
			_ = allowedExt(d.Name())
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, errAuditFoundSymlink) {
			return true, nil
		}
		return false, err
	}
	return false, nil
}
