VERSION ?= $(shell git describe --tags --exact-match 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X 'audistro-provider/internal/buildinfo.Version=$(VERSION)' \
	-X 'audistro-provider/internal/buildinfo.Commit=$(COMMIT)' \
	-X 'audistro-provider/internal/buildinfo.BuildTime=$(BUILD_TIME)'

SBOM_TOOL := $(CURDIR)/bin/cyclonedx-gomod

.PHONY: test vet build release clean

test:
	go test ./...

vet:
	go vet ./...

build:
	mkdir -p bin
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/audistro-provider ./cmd/audistro-provider

release:
	mkdir -p dist bin
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/audistro-provider_linux_amd64 ./cmd/audistro-provider
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/audistro-provider_linux_arm64 ./cmd/audistro-provider
	sha256sum dist/audistro-provider_linux_amd64 dist/audistro-provider_linux_arm64 > dist/checksums.txt
	GOBIN=$(CURDIR)/bin go install github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@latest
	$(SBOM_TOOL) mod -licenses -json -output dist/sbom.cdx.json

clean:
	rm -rf bin dist
