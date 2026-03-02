package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	legacyrouter "github.com/getkin/kin-openapi/routers/legacy"
)

type validationErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

func OpenAPIValidation(next http.Handler) http.Handler {
	spec, err := loadOpenAPISpec()
	if err != nil {
		panic(err)
	}
	router, err := legacyrouter.NewRouter(spec)
	if err != nil {
		panic(err)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !shouldValidateOpenAPIRequest(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		route, pathParams, err := router.FindRoute(r)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		if !requestContentTypeAllowed(route, r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(validationErrorResponse{Error: "invalid_request", Message: "content type does not match OpenAPI contract"})
			return
		}
		input := &openapi3filter.RequestValidationInput{
			Request:    r,
			PathParams: pathParams,
			Route:      route,
			Options: &openapi3filter.Options{
				AuthenticationFunc: func(context.Context, *openapi3filter.AuthenticationInput) error { return nil },
			},
		}
		if err := openapi3filter.ValidateRequest(r.Context(), input); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(validationErrorResponse{Error: "invalid_request", Message: err.Error()})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func shouldValidateOpenAPIRequest(path string) bool {
	switch {
	case path == "/healthz", path == "/readyz", path == "/metrics":
		return false
	case path == openAPIYAMLPath, path == openAPIJSONPath, path == "/docs", path == "/docs/":
		return false
	case strings.HasPrefix(path, "/assets/"):
		return false
	case strings.HasPrefix(path, "/internal/"):
		return true
	default:
		return false
	}
}

func requestContentTypeAllowed(route *routers.Route, r *http.Request) bool {
	if route == nil || route.Operation == nil || route.Operation.RequestBody == nil || route.Operation.RequestBody.Value == nil {
		return true
	}
	contentTypes := route.Operation.RequestBody.Value.Content
	if len(contentTypes) == 0 {
		return true
	}
	contentType := strings.TrimSpace(strings.ToLower(strings.Split(r.Header.Get("Content-Type"), ";")[0]))
	if contentType == "" {
		return false
	}
	_, ok := contentTypes[contentType]
	return ok
}
