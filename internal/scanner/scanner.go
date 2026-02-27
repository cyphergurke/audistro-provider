package scanner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"audistro-provider/internal/fssecure"
)

var assetIDRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)

type Scanner struct {
	AssetsRoot string
	Metrics    ScanMetrics
}

type ScanMetrics interface {
	ObserveScan(result string, duration time.Duration)
}

type Asset struct {
	AssetID            string
	MasterPlaylistPath string
	ContentHash        string
	FileCount          int
	TotalSizeBytes     int64
	LastScannedAt      time.Time
}

type ScanResult struct {
	Assets        []Asset
	ScannedAssets int
	Duration      time.Duration
}

type fileMeta struct {
	name    string
	size    int64
	modUnix int64
}

func (s *Scanner) Scan(ctx context.Context) (ScanResult, error) {
	start := time.Now()
	resultLabel := "ok"
	defer func() {
		if s.Metrics != nil {
			s.Metrics.ObserveScan(resultLabel, time.Since(start))
		}
	}()
	if err := os.MkdirAll(s.AssetsRoot, 0o700); err != nil {
		resultLabel = "error"
		return ScanResult{}, fmt.Errorf("ensure assets root: %w", err)
	}

	entries, err := os.ReadDir(s.AssetsRoot)
	if err != nil {
		resultLabel = "error"
		return ScanResult{}, fmt.Errorf("read assets root: %w", err)
	}

	assets := make([]Asset, 0)
	for _, entry := range entries {
		select {
		case <-ctx.Done():
			resultLabel = "error"
			return ScanResult{}, ctx.Err()
		default:
		}

		if entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		if !entry.IsDir() {
			continue
		}
		assetID := entry.Name()
		if !assetIDRegex.MatchString(assetID) {
			continue
		}

		asset, ok, err := s.scanAssetDir(assetID)
		if err != nil {
			resultLabel = "error"
			return ScanResult{}, err
		}
		if ok {
			assets = append(assets, asset)
		}
	}

	sort.Slice(assets, func(i, j int) bool {
		return assets[i].AssetID < assets[j].AssetID
	})

	return ScanResult{
		Assets:        assets,
		ScannedAssets: len(assets),
		Duration:      time.Since(start),
	}, nil
}

func (s *Scanner) scanAssetDir(assetID string) (Asset, bool, error) {
	assetDir := filepath.Join(s.AssetsRoot, assetID)
	assetDirInfo, err := os.Lstat(assetDir)
	if err != nil {
		return Asset{}, false, fmt.Errorf("lstat asset dir %q: %w", assetID, err)
	}
	if fssecure.IsSymlink(assetDirInfo) {
		return Asset{}, false, nil
	}

	entries, err := os.ReadDir(assetDir)
	if err != nil {
		return Asset{}, false, fmt.Errorf("read asset dir %q: %w", assetID, err)
	}

	hasMaster := false
	hasPlayable := false
	files := make([]fileMeta, 0)
	var totalSize int64

	for _, entry := range entries {
		name := entry.Name()
		if entry.Type()&os.ModeSymlink != 0 {
			return Asset{}, false, nil
		}

		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(name))
		if !isAllowedFile(name, ext) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			return Asset{}, false, fmt.Errorf("stat file %q in asset %q: %w", name, assetID, err)
		}

		if strings.EqualFold(name, "master.m3u8") {
			hasMaster = true
		}
		if ext == ".ts" || ext == ".m4s" || isAllowedInitMP4(name) {
			hasPlayable = true
		}

		files = append(files, fileMeta{
			name:    name,
			size:    info.Size(),
			modUnix: info.ModTime().UTC().UnixNano(),
		})
		totalSize += info.Size()
	}

	if !hasMaster || !hasPlayable {
		return Asset{}, false, nil
	}

	hash := computeContentHash(files)

	return Asset{
		AssetID:            assetID,
		MasterPlaylistPath: filepath.ToSlash(filepath.Join(assetID, "master.m3u8")),
		ContentHash:        hash,
		FileCount:          len(files),
		TotalSizeBytes:     totalSize,
		LastScannedAt:      time.Now().UTC(),
	}, true, nil
}

func isAllowedFile(name, ext string) bool {
	switch ext {
	case ".m3u8", ".ts", ".m4s":
		return true
	case ".mp4":
		return isAllowedInitMP4(name)
	default:
		return false
	}
}

func isAllowedInitMP4(name string) bool {
	lower := strings.ToLower(name)
	if lower == "init.mp4" {
		return true
	}
	if strings.HasPrefix(lower, "init-") && strings.HasSuffix(lower, ".mp4") && len(lower) > len("init-.mp4") {
		return true
	}
	return false
}

func computeContentHash(files []fileMeta) string {
	sort.Slice(files, func(i, j int) bool {
		return files[i].name < files[j].name
	})

	h := sha256.New()
	for _, f := range files {
		_, _ = fmt.Fprintf(h, "%s|%d|%d\n", f.name, f.size, f.modUnix)
	}
	return hex.EncodeToString(h.Sum(nil))
}
