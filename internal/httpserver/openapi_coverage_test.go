package httpserver

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestOpenAPISpecCoversRegisteredEndpoints(t *testing.T) {
	registered, err := registeredAPIEndpoints()
	if err != nil {
		t.Fatalf("read registered endpoints: %v", err)
	}
	specOps, err := documentedOperations()
	if err != nil {
		t.Fatalf("load openapi operations: %v", err)
	}

	missing := make([]string, 0)
	for _, ep := range registered {
		methods := specOps[ep.Path]
		if methods == nil || !methods[ep.Method] {
			missing = append(missing, fmt.Sprintf("%s %s", ep.Method, ep.Path))
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("openapi spec is missing endpoint(s):\n%s", strings.Join(missing, "\n"))
	}
}

type endpoint struct {
	Method string
	Path   string
}

func registeredAPIEndpoints() ([]endpoint, error) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return nil, fmt.Errorf("locate current file")
	}
	serverFile := filepath.Join(filepath.Dir(currentFile), "server.go")
	content, err := os.ReadFile(serverFile)
	if err != nil {
		return nil, err
	}
	src := string(content)

	found := make(map[endpoint]struct{})
	methodRoute := regexp.MustCompile(`mux\.Handle\("([A-Z]+) ([^"]+)"`)
	for _, m := range methodRoute.FindAllStringSubmatch(src, -1) {
		if len(m) != 3 {
			continue
		}
		p := normalizePath(m[2])
		if strings.HasPrefix(p, "/assets/") || p == "/docs" || p == "/docs/" || p == "/openapi.yaml" || p == "/openapi.json" || p == "/metrics" {
			continue
		}
		found[endpoint{Method: m[1], Path: p}] = struct{}{}
	}

	out := make([]endpoint, 0, len(found))
	for ep := range found {
		out = append(out, ep)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path == out[j].Path {
			return out[i].Method < out[j].Method
		}
		return out[i].Path < out[j].Path
	})
	return out, nil
}

func normalizePath(path string) string {
	if path == "/" {
		return path
	}
	return strings.TrimSuffix(path, "/")
}

func documentedOperations() (map[string]map[string]bool, error) {
	spec, err := loadCanonicalOpenAPISpec()
	if err != nil {
		return nil, err
	}
	ops := make(map[string]map[string]bool, len(spec.Paths.Map()))
	for path, item := range spec.Paths.Map() {
		normalized := normalizePath(path)
		if strings.HasPrefix(normalized, "/assets/") || normalized == "/docs" || normalized == "/openapi.yaml" || normalized == "/openapi.json" || normalized == "/metrics" {
			continue
		}
		methods := make(map[string]bool)
		if item.Get != nil {
			methods["GET"] = true
		}
		if item.Post != nil {
			methods["POST"] = true
		}
		if item.Put != nil {
			methods["PUT"] = true
		}
		if item.Patch != nil {
			methods["PATCH"] = true
		}
		if item.Delete != nil {
			methods["DELETE"] = true
		}
		if item.Head != nil {
			methods["HEAD"] = true
		}
		if len(methods) > 0 {
			ops[normalized] = methods
		}
	}
	return ops, nil
}

func loadCanonicalOpenAPISpec() (*openapi3.T, error) {
	canonical, err := canonicalOpenAPIYAML()
	if err != nil {
		return nil, err
	}
	loader := openapi3.NewLoader()
	spec, err := loader.LoadFromData(canonical)
	if err != nil {
		return nil, err
	}
	if err := spec.Validate(loader.Context); err != nil {
		return nil, err
	}
	return spec, nil
}
