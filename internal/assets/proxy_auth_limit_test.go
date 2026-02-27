package assets

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"audistro-provider/internal/originauth"
)

type fixedNonce struct {
	value []byte
}

func (f fixedNonce) Bytes(n int) ([]byte, error) {
	out := make([]byte, len(f.value))
	copy(out, f.value)
	return out, nil
}

func TestProxyModeHMACAuthHeaders(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	providerID := "11111111-2222-3333-4444-555555555555"
	keyID := "v1"
	nonceSource := fixedNonce{value: []byte{0x10, 0x32, 0x54, 0x76, 0x98, 0xba, 0xdc, 0xfe, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77}}
	signer := originauth.NewHMACSignerWithNonceSource(keyID, secret, providerID, true, nonceSource)

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-AudistroProvider-KeyId"); got != keyID {
			http.Error(w, "bad_keyid", http.StatusUnauthorized)
			return
		}
		ts := r.Header.Get("X-AudistroProvider-Timestamp")
		nonce := r.Header.Get("X-AudistroProvider-Nonce")
		signature := r.Header.Get("X-AudistroProvider-Signature")
		if got := r.Header.Get("X-AudistroProvider-ProviderId"); got != providerID {
			http.Error(w, "bad_provider_id", http.StatusUnauthorized)
			return
		}
		if ts == "" || nonce == "" || signature == "" {
			http.Error(w, "missing_auth_headers", http.StatusUnauthorized)
			return
		}

		pathWithQuery := r.URL.EscapedPath()
		if r.URL.RawQuery != "" {
			pathWithQuery += "?" + r.URL.RawQuery
		}
		canonical := r.Method + "\n" + pathWithQuery + "\n" + ts + "\n" + nonce + "\n" + providerID
		mac := hmac.New(sha256.New, secret)
		_, _ = mac.Write([]byte(canonical))
		expected := hex.EncodeToString(mac.Sum(nil))
		if expected != signature {
			http.Error(w, "bad_signature", http.StatusUnauthorized)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer origin.Close()

	h := NewHandler(HandlerConfig{
		AssetsRoot:                  t.TempDir(),
		MaxSegmentBytes:             67108864,
		StorageMode:                 "proxy",
		OriginBaseURL:               origin.URL,
		OriginAuthMode:              "hmac",
		OriginSigner:                signer,
		ProxyMaxUpstreamConcurrency: 4,
	})

	req := httptest.NewRequest(http.MethodGet, "/assets/a/seg_0001.m4s?token=abc", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (%s)", rr.Code, rr.Body.String())
	}
}

func TestProxyModeUpstreamConcurrencyLimit(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer origin.Close()

	h := NewHandler(HandlerConfig{
		AssetsRoot:                  t.TempDir(),
		MaxSegmentBytes:             67108864,
		StorageMode:                 "proxy",
		OriginBaseURL:               origin.URL,
		ProxyMaxUpstreamConcurrency: 1,
	})

	firstDone := make(chan int, 1)
	go func() {
		req1 := httptest.NewRequest(http.MethodGet, "/assets/a/seg_0001.m4s", nil)
		rr1 := httptest.NewRecorder()
		h.ServeHTTP(rr1, req1)
		firstDone <- rr1.Code
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for first request to reach origin")
	}

	req2 := httptest.NewRequest(http.MethodGet, "/assets/a/seg_0002.m4s", nil)
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503 for second request, got %d", rr2.Code)
	}
	if !strings.Contains(rr2.Body.String(), "upstream_busy") {
		t.Fatalf("expected upstream_busy error body, got %q", rr2.Body.String())
	}

	close(release)

	select {
	case code := <-firstDone:
		if code != http.StatusOK {
			t.Fatalf("expected first request to finish with 200, got %d", code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for first request to finish")
	}
}
