package identity

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/google/uuid"
)

var ErrNotFound = errors.New("identity not found")

type IdentityStore interface {
	Load(ctx context.Context) (*Identity, error)
	Save(ctx context.Context, id *Identity) error
}

type Identity struct {
	ProviderID uuid.UUID
	PrivateKey *btcec.PrivateKey
	PublicKey  *btcec.PublicKey
	CreatedAt  time.Time
}

func (i *Identity) PublicKeyHexCompressed() string {
	return hex.EncodeToString(i.PublicKey.SerializeCompressed())
}

func (i *Identity) Fingerprint() string {
	sum := sha256.Sum256(i.PublicKey.SerializeCompressed())
	return hex.EncodeToString(sum[:])[:8]
}

func LoadOrCreate(ctx context.Context, store IdentityStore) (*Identity, bool, error) {
	id, err := store.Load(ctx)
	if err == nil {
		return id, false, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, false, fmt.Errorf("load identity: %w", err)
	}

	id, err = newIdentity()
	if err != nil {
		return nil, false, fmt.Errorf("create identity: %w", err)
	}

	if err := store.Save(ctx, id); err != nil {
		return nil, false, fmt.Errorf("save identity: %w", err)
	}

	return id, true, nil
}

func newIdentity() (*Identity, error) {
	priv, err := btcec.NewPrivateKey()
	if err != nil {
		return nil, err
	}

	return &Identity{
		ProviderID: uuid.New(),
		PrivateKey: priv,
		PublicKey:  priv.PubKey(),
		CreatedAt:  time.Now().UTC(),
	}, nil
}

func parseProviderID(value string) (uuid.UUID, error) {
	providerID, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid provider_id: %w", err)
	}
	return providerID, nil
}

func parseCreatedAt(value string) (time.Time, error) {
	createdAt, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid created_at: %w", err)
	}
	return createdAt.UTC(), nil
}

func parsePrivateKeyHex(value string) (*btcec.PrivateKey, *btcec.PublicKey, error) {
	privBytes, err := hex.DecodeString(value)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid private_key hex: %w", err)
	}
	if len(privBytes) != 32 {
		return nil, nil, fmt.Errorf("invalid private_key length: expected 32 bytes, got %d", len(privBytes))
	}

	priv, derivedPub := btcec.PrivKeyFromBytes(privBytes)
	if priv.Key.IsZero() {
		return nil, nil, errors.New("invalid private_key: zero scalar")
	}
	return priv, derivedPub, nil
}

func parsePublicKeyHexCompressed(value string) ([]byte, error) {
	pubBytes, err := hex.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("invalid public_key hex: %w", err)
	}
	if len(pubBytes) != 33 {
		return nil, fmt.Errorf("invalid public_key length: expected 33 bytes, got %d", len(pubBytes))
	}
	if _, err := btcec.ParsePubKey(pubBytes); err != nil {
		return nil, fmt.Errorf("invalid public_key: %w", err)
	}
	return pubBytes, nil
}

func validatePubKeyMatchesPrivate(derivedPub *btcec.PublicKey, storedPub []byte) error {
	if !bytes.Equal(derivedPub.SerializeCompressed(), storedPub) {
		return errors.New("public_key does not match private_key")
	}
	return nil
}
