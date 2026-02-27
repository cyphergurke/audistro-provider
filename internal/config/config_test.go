package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateInvalidStorageMode(t *testing.T) {
	cfg := validConfigForTest(t)
	cfg.StorageMode = StorageMode("invalid")

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "invalid PROVIDER_STORAGE_MODE") {
		t.Fatalf("expected invalid storage mode error, got %v", err)
	}
}

func TestValidateProxyModeRequiresOrigin(t *testing.T) {
	cfg := validConfigForTest(t)
	cfg.StorageMode = StorageModeProxy
	cfg.OriginBaseURL = ""

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "PROVIDER_ORIGIN_BASE_URL is required") {
		t.Fatalf("expected missing origin error, got %v", err)
	}
}

func TestValidateCORSEnabledRequiresOrigins(t *testing.T) {
	cfg := validConfigForTest(t)
	cfg.EnableCORS = true
	cfg.CORSAllowedOrigins = ""

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "PROVIDER_CORS_ALLOWED_ORIGINS must not be empty") {
		t.Fatalf("expected cors origins error, got %v", err)
	}
}

func TestValidatePublicBaseURLHTTPSRequired(t *testing.T) {
	cfg := validConfigForTest(t)
	cfg.PublicBaseURL = "http://provider.example"
	cfg.AllowInsecurePublicURL = false

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "must use https") {
		t.Fatalf("expected https required error, got %v", err)
	}
}

func TestValidateInvalidOriginAuthMode(t *testing.T) {
	cfg := validConfigForTest(t)
	cfg.OriginAuthMode = "invalid"

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "invalid PROVIDER_ORIGIN_AUTH_MODE") {
		t.Fatalf("expected invalid origin auth mode error, got %v", err)
	}
}

func TestValidateHMACRequiresSecretPath(t *testing.T) {
	cfg := validConfigForTest(t)
	cfg.OriginAuthMode = "hmac"
	cfg.OriginHMACSecretPath = ""

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "PROVIDER_ORIGIN_HMAC_SECRET_PATH is required") {
		t.Fatalf("expected missing hmac secret path error, got %v", err)
	}
}

func TestValidateHMACSecretTooShort(t *testing.T) {
	cfg := validConfigForTest(t)
	cfg.OriginAuthMode = "hmac"
	secretPath := filepath.Join(t.TempDir(), "hmac.secret")
	if err := os.WriteFile(secretPath, []byte("short"), 0o600); err != nil {
		t.Fatalf("write secret file: %v", err)
	}
	cfg.OriginHMACSecretPath = secretPath

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "secret must be at least 16 bytes") {
		t.Fatalf("expected short secret error, got %v", err)
	}
}

func validConfigForTest(t *testing.T) Config {
	t.Helper()
	return Config{
		DataPath:                    t.TempDir(),
		AssetsSubdir:                "assets",
		StorageMode:                 StorageModeFilesystem,
		OriginAuthMode:              "none",
		MaxSegmentBytes:             1,
		RateLimitRPS:                1,
		RateLimitBurst:              1,
		ProxyMaxUpstreamConcurrency: 1,
		ShutdownTimeoutSeconds:      15,
		InternalEnable:              true,
		InternalAllowedCIDRs:        "127.0.0.1/32,::1/128",
	}
}
