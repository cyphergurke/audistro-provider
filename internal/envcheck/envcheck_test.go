package envcheck

import "testing"

func TestValidateSkipsWhenModeUnset(t *testing.T) {
	t.Setenv("AUDISTRO_ENV", "")

	if err := Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateFailsWhenProdEnvMissing(t *testing.T) {
	t.Setenv("AUDISTRO_ENV", "prod")
	t.Setenv("PROVIDER_IDENTITY_PATH", "")
	t.Setenv("PROVIDER_PUBLIC_BASE_URL", "https://provider-eu-1.example.com")
	t.Setenv("PROVIDER_CATALOG_BASE_URL", "http://audistro-catalog:8080")

	if err := Validate(); err == nil || err.Error() != "envcheck: missing required env: PROVIDER_IDENTITY_PATH" {
		t.Fatalf("expected missing identity path error, got %v", err)
	}
}

func TestValidateAcceptsProdEnv(t *testing.T) {
	t.Setenv("AUDISTRO_ENV", "prod")
	t.Setenv("PROVIDER_IDENTITY_PATH", "/var/lib/audistro-provider/provider_identity.json")
	t.Setenv("PROVIDER_PUBLIC_BASE_URL", "https://provider-eu-1.example.com")
	t.Setenv("PROVIDER_CATALOG_BASE_URL", "http://audistro-catalog:8080")

	if err := Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}
