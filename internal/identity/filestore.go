package identity

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

type FileStore struct {
	Path string
}

type fileIdentity struct {
	ProviderID string `json:"provider_id"`
	PublicKey  string `json:"public_key"`
	PrivateKey string `json:"private_key"`
	CreatedAt  string `json:"created_at"`
}

func (s *FileStore) Load(ctx context.Context) (*Identity, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	raw, err := os.ReadFile(s.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	var stored fileIdentity
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&stored); err != nil {
		return nil, fmt.Errorf("decode identity json: %w", err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("invalid identity json: trailing data")
	}

	providerID, err := parseProviderID(stored.ProviderID)
	if err != nil {
		return nil, err
	}
	createdAt, err := parseCreatedAt(stored.CreatedAt)
	if err != nil {
		return nil, err
	}
	priv, derivedPub, err := parsePrivateKeyHex(stored.PrivateKey)
	if err != nil {
		return nil, err
	}
	pubBytes, err := parsePublicKeyHexCompressed(stored.PublicKey)
	if err != nil {
		return nil, err
	}
	if err := validatePubKeyMatchesPrivate(derivedPub, pubBytes); err != nil {
		return nil, err
	}

	return &Identity{
		ProviderID: providerID,
		PrivateKey: priv,
		PublicKey:  derivedPub,
		CreatedAt:  createdAt.UTC(),
	}, nil
}

func (s *FileStore) Save(ctx context.Context, id *Identity) (err error) {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create identity dir: %w", err)
	}

	stored := fileIdentity{
		ProviderID: id.ProviderID.String(),
		PublicKey:  id.PublicKeyHexCompressed(),
		PrivateKey: hex.EncodeToString(id.PrivateKey.Serialize()),
		CreatedAt:  id.CreatedAt.UTC().Format(time.RFC3339),
	}

	encoded, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal identity json: %w", err)
	}
	encoded = append(encoded, '\n')

	tmp, err := os.CreateTemp(dir, ".provider_identity-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp identity file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := enforce0600(tmp.Name(), tmp); err != nil {
		_ = tmp.Close()
		return err
	}

	if _, err := tmp.Write(encoded); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp identity file: %w", err)
	}

	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temp identity file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp identity file: %w", err)
	}

	if err := os.Rename(tmpPath, s.Path); err != nil {
		return fmt.Errorf("rename temp identity file: %w", err)
	}

	if err := enforce0600(s.Path, nil); err != nil {
		return err
	}

	return nil
}

func enforce0600(path string, file *os.File) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	if file != nil {
		if err := file.Chmod(0o600); err != nil {
			return fmt.Errorf("chmod identity file: %w", err)
		}
		return nil
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("chmod identity file: %w", err)
	}
	return nil
}
