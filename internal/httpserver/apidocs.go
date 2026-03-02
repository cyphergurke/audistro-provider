package httpserver

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/getkin/kin-openapi/openapi3"
)

const (
	openAPIYAMLPath = "/openapi.yaml"
	openAPIJSONPath = "/openapi.json"
)

var (
	//go:embed openapi.yaml
	openAPIYAML []byte

	loadOpenAPISpecOnce sync.Once
	loadedOpenAPISpec   *openapi3.T
	loadedOpenAPIJSON   []byte
	loadOpenAPISpecErr  error
)

func loadOpenAPISpec() (*openapi3.T, error) {
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

func openAPISpecYAMLHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(openAPIYAML)
	})
}

func openAPISpecJSONHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := loadOpenAPISpec()
		if err != nil {
			http.Error(w, fmt.Sprintf("load openapi spec: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(loadedOpenAPIJSON)
	})
}

func scalarDocsHandler(specPath string) http.Handler {
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
