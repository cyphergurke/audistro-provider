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
)

func TestOpenAPISpecCoversRegisteredEndpoints(t *testing.T) {
	registered, err := registeredAPIEndpoints()
	if err != nil {
		t.Fatalf("read registered endpoints: %v", err)
	}
	specOps, err := parseOpenAPIOperations(string(openAPISpec))
	if err != nil {
		t.Fatalf("parse openapi operations: %v", err)
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
		found[endpoint{Method: m[1], Path: p}] = struct{}{}
	}

	if strings.Contains(src, `mux.Handle("/assets/", assetsHandler)`) {
		found[endpoint{Method: "GET", Path: "/assets/{assetId}/master.m3u8"}] = struct{}{}
		found[endpoint{Method: "HEAD", Path: "/assets/{assetId}/master.m3u8"}] = struct{}{}
		found[endpoint{Method: "GET", Path: "/assets/{assetId}/{filename}"}] = struct{}{}
		found[endpoint{Method: "HEAD", Path: "/assets/{assetId}/{filename}"}] = struct{}{}
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

func parseOpenAPIOperations(spec string) (map[string]map[string]bool, error) {
	lines := strings.Split(spec, "\n")
	pathLine := regexp.MustCompile(`^\s{2}(/[^:]*):\s*$`)
	methodLine := regexp.MustCompile(`^\s{4}(get|post|put|patch|delete|head|options|trace):\s*$`)

	inPaths := false
	currentPath := ""
	out := make(map[string]map[string]bool)

	for _, line := range lines {
		switch {
		case strings.TrimSpace(line) == "paths:":
			inPaths = true
			currentPath = ""
			continue
		case strings.TrimSpace(line) == "components:":
			inPaths = false
			currentPath = ""
			continue
		}
		if !inPaths {
			continue
		}

		if m := pathLine.FindStringSubmatch(line); len(m) == 2 {
			currentPath = normalizePath(m[1])
			if out[currentPath] == nil {
				out[currentPath] = make(map[string]bool)
			}
			continue
		}
		if m := methodLine.FindStringSubmatch(line); len(m) == 2 && currentPath != "" {
			out[currentPath][strings.ToUpper(m[1])] = true
		}
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("no operations found")
	}
	return out, nil
}
