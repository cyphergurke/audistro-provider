package httpserver

import (
	_ "embed"
	"fmt"
	"net/http"
)

const openAPISpecPath = "/openapi.yaml"

//go:embed openapi.yaml
var openAPISpec []byte

func openAPISpecHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(openAPISpec)
	})
}

func scalarDocsHandler(specPath string) http.Handler {
	if specPath == "" {
		specPath = openAPISpecPath
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
