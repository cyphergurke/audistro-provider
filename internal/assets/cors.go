package assets

import (
	"net/http"
	"strings"
)

type CORSConfig struct {
	AllowedOrigins   map[string]struct{}
	AllowCredentials bool
}

func ParseAllowedOrigins(csv string) map[string]struct{} {
	origins := make(map[string]struct{})
	for _, part := range strings.Split(csv, ",") {
		origin := strings.TrimSpace(part)
		if origin == "" {
			continue
		}
		origins[origin] = struct{}{}
	}
	return origins
}

func NewCORSMiddleware(cfg CORSConfig) func(http.Handler) http.Handler {
	allowMethods := "GET, HEAD, OPTIONS"
	allowHeaders := "Range, If-Modified-Since, If-None-Match, Origin, Accept, Content-Type"
	exposeHeaders := "Content-Length, Content-Range, Accept-Ranges, ETag, Last-Modified"

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			_, allowed := cfg.AllowedOrigins[origin]

			if r.Method == http.MethodOptions {
				if !allowed {
					writeJSONError(w, http.StatusForbidden, "cors_origin_forbidden")
					return
				}
				setCORSHeaders(w, origin, cfg.AllowCredentials, allowMethods, allowHeaders, exposeHeaders)
				w.WriteHeader(http.StatusNoContent)
				return
			}

			if allowed {
				setCORSHeaders(w, origin, cfg.AllowCredentials, allowMethods, allowHeaders, exposeHeaders)
			}
			next.ServeHTTP(w, r)
		})
	}
}

func setCORSHeaders(w http.ResponseWriter, origin string, allowCredentials bool, allowMethods, allowHeaders, exposeHeaders string) {
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", origin)
	h.Add("Vary", "Origin")
	if allowCredentials {
		h.Set("Access-Control-Allow-Credentials", "true")
	}
	h.Set("Access-Control-Allow-Methods", allowMethods)
	h.Set("Access-Control-Allow-Headers", allowHeaders)
	h.Set("Access-Control-Expose-Headers", exposeHeaders)
}
