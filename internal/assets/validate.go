package assets

import (
	"errors"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	assetIDRegex  = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)
	filenameRegex = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,256}$`)
)

func validateAssetID(assetID string) error {
	if !assetIDRegex.MatchString(assetID) {
		return errors.New("invalid asset_id")
	}
	return nil
}

func validateFilename(filename string) error {
	if filename == "" {
		return errors.New("empty filename")
	}
	if strings.Contains(filename, "..") {
		return errors.New("invalid filename")
	}
	if strings.Contains(filename, "/") || strings.Contains(filename, `\`) {
		return errors.New("invalid filename")
	}
	if !filenameRegex.MatchString(filename) {
		return errors.New("invalid filename")
	}
	return nil
}

func validateExtension(filename string) (string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".m3u8", ".ts", ".m4s":
		return ext, nil
	case ".mp4":
		base := strings.ToLower(filepath.Base(filename))
		if base == "init.mp4" {
			return ext, nil
		}
		if strings.HasPrefix(base, "init-") && len(base) > len("init-.mp4") {
			return ext, nil
		}
		return "", errors.New("invalid mp4 filename")
	default:
		return "", errors.New("unsupported extension")
	}
}
