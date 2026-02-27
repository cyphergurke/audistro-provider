package catalog

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/btcsuite/btcd/btcec/v2/schnorr"

	"audistro-provider/internal/config"
	"audistro-provider/internal/identity"
)

var nonceReader = rand.Reader

func NonceReaderForTest(r io.Reader) func() {
	prev := nonceReader
	nonceReader = r
	return func() {
		nonceReader = prev
	}
}

func BuildAnnouncement(id *identity.Identity, cfg config.Config, assetID string, now time.Time) (AnnounceRequest, error) {
	if strings.TrimSpace(cfg.PublicBaseURL) == "" {
		return AnnounceRequest{}, fmt.Errorf("public base url is required")
	}
	if strings.TrimSpace(assetID) == "" {
		return AnnounceRequest{}, fmt.Errorf("asset_id is required")
	}

	nonceBytes := make([]byte, 16)
	if _, err := nonceReader.Read(nonceBytes); err != nil {
		return AnnounceRequest{}, fmt.Errorf("generate nonce: %w", err)
	}
	nonceHex := hex.EncodeToString(nonceBytes)

	baseURL := strings.TrimRight(cfg.PublicBaseURL, "/") + "/assets/" + assetID
	expiresAt := now.UTC().Add(time.Duration(cfg.AnnounceExpiresSeconds) * time.Second).Unix()

	message := buildSignatureMessage(
		id.ProviderID.String(),
		assetID,
		cfg.Transport,
		baseURL,
		expiresAt,
		nonceHex,
	)
	hash := sha256.Sum256([]byte(message))

	sig, err := schnorr.Sign(id.PrivateKey, hash[:])
	if err != nil {
		return AnnounceRequest{}, fmt.Errorf("sign announcement: %w", err)
	}

	return AnnounceRequest{
		AssetID:          assetID,
		Transport:        cfg.Transport,
		BaseURL:          baseURL,
		Priority:         cfg.AnnouncePriority,
		ExpiresInSeconds: cfg.AnnounceExpiresSeconds,
		ExpiresAt:        expiresAt,
		Nonce:            nonceHex,
		Signature:        hex.EncodeToString(sig.Serialize()),
	}, nil
}

func buildSignatureMessage(providerID, assetID, transport, baseURL string, expiresAt int64, nonce string) string {
	return fmt.Sprintf("%s|%s|%s|%s|%d|%s", providerID, assetID, transport, baseURL, expiresAt, nonce)
}
