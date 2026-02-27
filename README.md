# audistro-provider

Step 8 service baseline for `audistro-provider` with identity, static/proxy HLS serving, SQLite index/scanner, catalog integration, background jobs, metrics, readiness, and deployment hardening defaults.

## Requirements

- Go 1.26+

## Environment variables

- `PROVIDER_HTTP_ADDR` (default `:8080`)
- `PROVIDER_DATA_PATH` (default `./data`)
- `PROVIDER_ASSETS_SUBDIR` (default `assets`)
- `PROVIDER_IDENTITY_PATH` (optional; default `path.Join(PROVIDER_DATA_PATH, "provider_identity.json")`)
- `PROVIDER_DB_PATH` (optional; default `path.Join(PROVIDER_DATA_PATH, "provider.db")`)
- `PROVIDER_SCAN_ON_STARTUP` (default `true`)
- `PROVIDER_STORAGE_MODE` (default `filesystem`; `filesystem|proxy|hybrid`)
- `PROVIDER_ORIGIN_BASE_URL` (required for `proxy` and `hybrid`)
- `PROVIDER_ORIGIN_AUTH_MODE` (default `none`; `none|hmac`)
- `PROVIDER_ORIGIN_HMAC_SECRET_PATH` (required when `PROVIDER_ORIGIN_AUTH_MODE=hmac`)
- `PROVIDER_ORIGIN_HMAC_KEY_ID` (default `v1`)
- `PROVIDER_ORIGIN_AUTH_INCLUDE_QUERY` (default `true`)
- `PROVIDER_PROXY_MAX_UPSTREAM_CONCURRENCY` (default `64`)
- `PROVIDER_MAX_SEGMENT_BYTES` (default `67108864`)
- `PROVIDER_RATE_LIMIT_RPS` (default `20`)
- `PROVIDER_RATE_LIMIT_BURST` (default `40`)
- `PROVIDER_ENABLE_CORS` (default `false`)
- `PROVIDER_CORS_ALLOWED_ORIGINS` (optional, comma-separated exact origins; no `*`)
- `PROVIDER_CORS_ALLOW_CREDENTIALS` (default `false`)
- `PROVIDER_ENFORCE_NO_SYMLINKS` (default `true`)
- `PROVIDER_PUBLIC_BASE_URL` (required for announcements when catalog integration is enabled)
- `PROVIDER_ALLOW_INSECURE_PUBLIC_URL` (default `false`)
- `PROVIDER_CATALOG_BASE_URL` (optional; enables catalog integration)
- `PROVIDER_CATALOG_TIMEOUT_SECONDS` (default `10`)
- `PROVIDER_TRANSPORT` (default `https`)
- `PROVIDER_ANNOUNCE_PRIORITY` (default `10`)
- `PROVIDER_ANNOUNCE_EXPIRES_SECONDS` (default `604800`)
- `PROVIDER_ENABLE_JOBS` (default `true`)
- `PROVIDER_RESCAN_INTERVAL_SECONDS` (default `300`)
- `PROVIDER_ANNOUNCE_SWEEP_INTERVAL_SECONDS` (default `300`)
- `PROVIDER_REANNOUNCE_THRESHOLD_SECONDS` (default `86400`)
- `PROVIDER_BACKOFF_BASE_MILLIS` (default `2000`)
- `PROVIDER_BACKOFF_MAX_SECONDS` (default `600`)
- `PROVIDER_REJECTED_RETRY_SECONDS` (default `86400`)
- `PROVIDER_UNAUTHORIZED_RETRY_SECONDS` (default `3600`)
- `PROVIDER_ANNOUNCEMENT_EXPIRY_GRACE_SECONDS` (default `86400`)
- `PROVIDER_INTERNAL_ENABLE` (default `true`)
- `PROVIDER_INTERNAL_ALLOWED_CIDRS` (default `127.0.0.1/32,::1/128`)
- `PROVIDER_TRUST_PROXY_HEADERS` (default `false`)
- `PROVIDER_TRUSTED_PROXY_CIDRS` (default empty)
- `PROVIDER_SHUTDOWN_TIMEOUT_SECONDS` (default `15`)
- `PROVIDER_ANNOUNCE_INTERVAL` (optional)
- `PROVIDER_REGION` (optional)
- `PROVIDER_PRIVATE_KEY_PATH` (optional)

## Identity file behavior

- Private key is not encrypted on disk in v1.
- On startup, identity is loaded from `PROVIDER_IDENTITY_PATH` or created if missing.
- File format:

```json
{
  "provider_id": "uuid-string",
  "public_key": "hex-compressed-33-bytes",
  "private_key": "hex-32-bytes",
  "created_at": "RFC3339 timestamp"
}
```

## Endpoints

- `GET /healthz`
- `GET /readyz`
- `GET /metrics`
- `GET|HEAD /assets/{assetId}/master.m3u8`
- `GET|HEAD /assets/{assetId}/{filename}`
- `POST /internal/rescan` (only mounted if `PROVIDER_INTERNAL_ENABLE=true`)
- `POST /internal/announce` (only mounted if `PROVIDER_INTERNAL_ENABLE=true`)

## Run

```bash
go run ./cmd/audistro-provider
```

## Run with Docker Compose

```bash
cp .env.example .env
docker compose up --build -d
```

Stop:

```bash
docker compose down
```

## Build and release artifacts

```bash
make test
make vet
make build
make release
```

`make release` creates:

- `dist/audistro-provider_linux_amd64`
- `dist/audistro-provider_linux_arm64`
- `dist/checksums.txt`
- `dist/sbom.cdx.json`

## Example checks

```bash
curl -s http://localhost:8080/healthz
curl -s http://localhost:8080/readyz
curl -s http://localhost:8080/metrics
```

## Deployment examples

- Docker: `Dockerfile`
- Docker Compose: `docker-compose.yml`
- systemd: `deploy/systemd/audistro-provider.service`
- Kubernetes snippet: `deploy/k8s/deployment.yaml`
- Reverse proxy reference: `docs/reverse-proxy.md`

## Production checklist

- Put `audistro-provider` behind a TLS reverse proxy; use `docs/reverse-proxy.md` as the baseline.
- TLS termination is required in front of this service. `audistro-provider` itself serves plain HTTP.
- `/internal/*` endpoints should stay private and are CIDR-protected; keep defaults (loopback only) unless you intentionally expose internal networks.
- Keep `/metrics` private by default; expose it only on trusted networks or behind auth/VPN.
- Enable CORS only when a browser HLS player needs direct asset access; configure explicit allowed origins.
- Persist and protect `PROVIDER_DATA_PATH` as a writable volume; identity and DB files should remain private to the service user.

## TODOs

All known remaining non-production-ready items are tracked in:

- `docs/todos/production-readiness.md`
