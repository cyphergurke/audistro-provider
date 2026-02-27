package identity

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadOrCreateRoundTrip(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "provider_identity.json")
	store := &FileStore{Path: path}

	created, wasCreated, err := LoadOrCreate(t.Context(), store)
	if err != nil {
		t.Fatalf("LoadOrCreate(create) returned error: %v", err)
	}
	if !wasCreated {
		t.Fatal("expected identity to be created")
	}

	loaded, wasCreated, err := LoadOrCreate(t.Context(), store)
	if err != nil {
		t.Fatalf("LoadOrCreate(load) returned error: %v", err)
	}
	if wasCreated {
		t.Fatal("expected identity to be loaded")
	}

	if created.ProviderID != loaded.ProviderID {
		t.Fatalf("provider_id mismatch: %s != %s", created.ProviderID, loaded.ProviderID)
	}
	if created.PublicKeyHexCompressed() != loaded.PublicKeyHexCompressed() {
		t.Fatalf("public key mismatch")
	}
	if created.PrivateKey.Key != loaded.PrivateKey.Key {
		t.Fatalf("private key mismatch")
	}
}

func TestLoadOrCreateFailsOnTamperedPublicKey(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "provider_identity.json")
	store := &FileStore{Path: path}
	id, _, err := LoadOrCreate(t.Context(), store)
	if err != nil {
		t.Fatalf("LoadOrCreate(create) returned error: %v", err)
	}

	doc := map[string]string{
		"provider_id": id.ProviderID.String(),
		"public_key":  "03aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"private_key": hex.EncodeToString(id.PrivateKey.Serialize()),
		"created_at":  id.CreatedAt.UTC().Format(time.RFC3339),
	}

	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal tampered doc: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write tampered doc: %v", err)
	}

	if _, _, err := LoadOrCreate(t.Context(), store); err == nil {
		t.Fatal("expected validation error for tampered public_key")
	}
}

func TestLoadOrCreateFailsOnInvalidPrivateKeyLength(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "provider_identity.json")
	store := &FileStore{Path: path}
	id, _, err := LoadOrCreate(t.Context(), store)
	if err != nil {
		t.Fatalf("LoadOrCreate(create) returned error: %v", err)
	}

	doc := map[string]string{
		"provider_id": id.ProviderID.String(),
		"public_key":  id.PublicKeyHexCompressed(),
		"private_key": "abcd",
		"created_at":  id.CreatedAt.UTC().Format(time.RFC3339),
	}

	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal invalid doc: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write invalid doc: %v", err)
	}

	if _, _, err := LoadOrCreate(t.Context(), store); err == nil {
		t.Fatal("expected validation error for invalid private_key length")
	}
}

func TestLoadOrCreateFailsOnInvalidUUID(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "provider_identity.json")
	store := &FileStore{Path: path}
	id, _, err := LoadOrCreate(t.Context(), store)
	if err != nil {
		t.Fatalf("LoadOrCreate(create) returned error: %v", err)
	}

	doc := map[string]string{
		"provider_id": "not-a-uuid",
		"public_key":  id.PublicKeyHexCompressed(),
		"private_key": hex.EncodeToString(id.PrivateKey.Serialize()),
		"created_at":  id.CreatedAt.UTC().Format(time.RFC3339),
	}

	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal invalid uuid doc: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write invalid uuid doc: %v", err)
	}

	if _, _, err := LoadOrCreate(t.Context(), store); err == nil {
		t.Fatal("expected validation error for invalid uuid")
	}
}
