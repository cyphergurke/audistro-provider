package originauth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type NonceSource interface {
	Bytes(n int) ([]byte, error)
}

type cryptoNonceSource struct{}

func (cryptoNonceSource) Bytes(n int) ([]byte, error) {
	if n <= 0 {
		return nil, fmt.Errorf("invalid nonce size")
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

type HMACSigner struct {
	keyID        string
	secret       []byte
	providerID   string
	includeQuery bool
	nonceSource  NonceSource
}

func NewHMACSigner(keyID string, secret []byte, providerID string, includeQuery bool) *HMACSigner {
	return NewHMACSignerWithNonceSource(keyID, secret, providerID, includeQuery, cryptoNonceSource{})
}

func NewHMACSignerWithNonceSource(keyID string, secret []byte, providerID string, includeQuery bool, nonceSource NonceSource) *HMACSigner {
	if strings.TrimSpace(keyID) == "" {
		keyID = "v1"
	}
	if nonceSource == nil {
		nonceSource = cryptoNonceSource{}
	}
	clonedSecret := make([]byte, len(secret))
	copy(clonedSecret, secret)
	return &HMACSigner{
		keyID:        keyID,
		secret:       clonedSecret,
		providerID:   providerID,
		includeQuery: includeQuery,
		nonceSource:  nonceSource,
	}
}

func (s *HMACSigner) Sign(req *http.Request, now time.Time) error {
	if req == nil {
		return fmt.Errorf("request is nil")
	}
	nonceBytes, err := s.nonceSource.Bytes(16)
	if err != nil {
		return fmt.Errorf("generate nonce: %w", err)
	}
	return s.signWithNonce(req, now, nonceBytes)
}

func (s *HMACSigner) signWithNonce(req *http.Request, now time.Time, nonceBytes []byte) error {
	switch req.Method {
	case http.MethodGet, http.MethodHead:
	default:
		return fmt.Errorf("unsupported method %q", req.Method)
	}
	if len(nonceBytes) != 16 {
		return fmt.Errorf("nonce must be 16 bytes")
	}

	ts := strconv.FormatInt(now.Unix(), 10)
	nonce := hex.EncodeToString(nonceBytes)
	pathWithQuery := req.URL.EscapedPath()
	if pathWithQuery == "" {
		pathWithQuery = "/"
	}
	if s.includeQuery && req.URL.RawQuery != "" {
		pathWithQuery += "?" + req.URL.RawQuery
	}

	canonical := req.Method + "\n" + pathWithQuery + "\n" + ts + "\n" + nonce + "\n" + s.providerID
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(canonical))
	signature := hex.EncodeToString(mac.Sum(nil))

	req.Header.Set("X-AudistroProvider-KeyId", s.keyID)
	req.Header.Set("X-AudistroProvider-Timestamp", ts)
	req.Header.Set("X-AudistroProvider-Nonce", nonce)
	req.Header.Set("X-AudistroProvider-Signature", signature)
	req.Header.Set("X-AudistroProvider-ProviderId", s.providerID)
	return nil
}
