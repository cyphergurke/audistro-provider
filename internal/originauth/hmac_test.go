package originauth

import (
	"net/http"
	"net/url"
	"testing"
	"time"
)

type fixedNonceSource struct {
	nonce []byte
}

func (f fixedNonceSource) Bytes(n int) ([]byte, error) {
	out := make([]byte, len(f.nonce))
	copy(out, f.nonce)
	return out, nil
}

func TestHMACSignerSignDeterministic(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	providerID := "11111111-2222-3333-4444-555555555555"
	nonce := []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}

	signer := NewHMACSignerWithNonceSource("v1", secret, providerID, true, fixedNonceSource{nonce: nonce})
	req := &http.Request{
		Method: http.MethodGet,
		URL: &url.URL{
			Path:     "/assets/asset1/seg_0001.m4s",
			RawQuery: "token=abc&v=1",
		},
		Header: make(http.Header),
	}
	now := time.Unix(1700000000, 0).UTC()

	if err := signer.Sign(req, now); err != nil {
		t.Fatalf("sign failed: %v", err)
	}

	if got := req.Header.Get("X-AudistroProvider-KeyId"); got != "v1" {
		t.Fatalf("unexpected key id: %q", got)
	}
	if got := req.Header.Get("X-AudistroProvider-Timestamp"); got != "1700000000" {
		t.Fatalf("unexpected timestamp: %q", got)
	}
	if got := req.Header.Get("X-AudistroProvider-Nonce"); got != "00112233445566778899aabbccddeeff" {
		t.Fatalf("unexpected nonce: %q", got)
	}
	if got := req.Header.Get("X-AudistroProvider-ProviderId"); got != providerID {
		t.Fatalf("unexpected provider id: %q", got)
	}
	const wantSig = "43f55125833fbb8d9413a0c6d797070abbb7c25312b3a7ce5344a2ba16ff1d90"
	if got := req.Header.Get("X-AudistroProvider-Signature"); got != wantSig {
		t.Fatalf("unexpected signature: want %q got %q", wantSig, got)
	}
}
