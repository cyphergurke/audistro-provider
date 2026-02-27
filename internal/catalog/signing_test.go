package catalog

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/google/uuid"

	"audistro-provider/internal/config"
	"audistro-provider/internal/identity"
)

func TestBuildAnnouncementSignatureDeterministic(t *testing.T) {
	privHex := "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
	privBytes, err := hex.DecodeString(privHex)
	if err != nil {
		t.Fatalf("decode priv: %v", err)
	}
	priv, pub := btcec.PrivKeyFromBytes(privBytes)
	providerID := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	id := &identity.Identity{
		ProviderID: providerID,
		PrivateKey: priv,
		PublicKey:  pub,
		CreatedAt:  time.Unix(1700000000, 0).UTC(),
	}

	cfg := config.Config{
		PublicBaseURL:          "https://provider.example",
		Transport:              "https",
		AnnouncePriority:       10,
		AnnounceExpiresSeconds: 604800,
	}

	fixedNow := time.Unix(1700000000, 0).UTC()
	oldReader := nonceReader
	nonceReader = bytes.NewReader([]byte{0x10, 0x32, 0x54, 0x76, 0x98, 0xba, 0xdc, 0xfe, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77})
	t.Cleanup(func() { nonceReader = oldReader })

	req, err := BuildAnnouncement(id, cfg, "asset1", fixedNow)
	if err != nil {
		t.Fatalf("BuildAnnouncement failed: %v", err)
	}

	if req.BaseURL != "https://provider.example/assets/asset1" {
		t.Fatalf("unexpected base_url: %q", req.BaseURL)
	}
	if req.ExpiresAt != fixedNow.Add(604800*time.Second).Unix() {
		t.Fatalf("unexpected expires_at: %d", req.ExpiresAt)
	}
	if req.Nonce != "1032547698badcfe0011223344556677" {
		t.Fatalf("unexpected nonce: %q", req.Nonce)
	}

	message := buildSignatureMessage(providerID.String(), "asset1", req.Transport, req.BaseURL, req.ExpiresAt, req.Nonce)
	hash := sha256.Sum256([]byte(message))
	sigBytes, err := hex.DecodeString(req.Signature)
	if err != nil {
		t.Fatalf("decode sig: %v", err)
	}
	sig, err := schnorr.ParseSignature(sigBytes)
	if err != nil {
		t.Fatalf("parse sig: %v", err)
	}
	if !sig.Verify(hash[:], pub) {
		t.Fatal("signature verification failed")
	}

	const expectedSignatureHex = "fc16782ad64a74ae4db90617bac606d5a639d00e5c7fea51f01679430fb55ab65021cbdddde016cab58cc39ab9e53b3ab1076e70a1d476784330c0585451cf6f"
	if req.Signature != expectedSignatureHex {
		t.Fatalf("unexpected signature hex:\nwant %s\n got %s", expectedSignatureHex, req.Signature)
	}
}
