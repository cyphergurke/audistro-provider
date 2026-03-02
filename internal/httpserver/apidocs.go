package httpserver

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/getkin/kin-openapi/openapi3"
)

const (
	openAPIYAMLPath = "/openapi.yaml"
	openAPIJSONPath = "/openapi.json"
)

var (
	// Go embed cannot traverse to ../../api, so this package embeds a synced copy.
	// The canonical manual spec lives at services/audistro-provider/api/openapi.v1.yaml.
	//go:embed api/openapi.v1.yaml
	openAPIYAML []byte

	loadOpenAPISpecOnce sync.Once
	loadedOpenAPISpec   *openapi3.T
	loadedOpenAPIJSON   []byte
	loadOpenAPISpecErr  error
)

func LoadSpec() (*openapi3.T, error) {
	loadOpenAPISpecOnce.Do(func() {
		loader := openapi3.NewLoader()
		spec, err := loader.LoadFromData(openAPIYAML)
		if err != nil {
			loadOpenAPISpecErr = err
			return
		}
		if err := spec.Validate(loader.Context); err != nil {
			loadOpenAPISpecErr = err
			return
		}
		jsonSpec, err := json.MarshalIndent(spec, "", "  ")
		if err != nil {
			loadOpenAPISpecErr = err
			return
		}
		loadedOpenAPISpec = spec
		loadedOpenAPIJSON = jsonSpec
	})
	return loadedOpenAPISpec, loadOpenAPISpecErr
}

func OpenAPIYAMLHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(openAPIYAML)
	})
}

func OpenAPIJSONHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := LoadSpec()
		if err != nil {
			http.Error(w, fmt.Sprintf("load openapi spec: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(loadedOpenAPIJSON)
	})
}

func DocsHandler(specPath string) http.Handler {
	if specPath == "" {
		specPath = openAPIJSONPath
	}
	html := fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>audistro-provider API Docs</title>
  <style>
    body { margin: 0; }
  </style>
</head>
<body>
  <script
    id="api-reference"
    data-url="%s"
    data-configuration='{"theme":"deepSpace"}'></script>
  <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
</body>
</html>`, specPath)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(html))
	})
}

func canonicalOpenAPIYAML() ([]byte, error) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return nil, fmt.Errorf("locate current file")
	}
	return os.ReadFile(filepath.Join(filepath.Dir(currentFile), "..", "..", "api", "openapi.v1.yaml"))
}

func embeddedOpenAPIYAMLMatchesCanonical() error {
	canonical, err := canonicalOpenAPIYAML()
	if err != nil {
		return err
	}
	if !bytes.Equal(canonical, openAPIYAML) {
		return fmt.Errorf("embedded spec is out of sync with canonical spec")
	}
	compatPath := filepath.Join(filepath.Dir(mustCurrentFile()), "openapi.yaml")
	compat, err := os.ReadFile(compatPath)
	if err != nil {
		return err
	}
	if !bytes.Equal(canonical, compat) {
		return fmt.Errorf("compatibility spec is out of sync with canonical spec")
	}
	return nil
}

func mustCurrentFile() string {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("locate current file")
	}
	return currentFile
}
