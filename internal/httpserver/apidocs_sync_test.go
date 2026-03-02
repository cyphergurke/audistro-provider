package httpserver

import "testing"

func TestEmbeddedSpecMatchesCanonicalSpec(t *testing.T) {
	if err := embeddedOpenAPIYAMLMatchesCanonical(); err != nil {
		t.Fatal(err)
	}
}
