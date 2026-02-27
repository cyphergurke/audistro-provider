# audistro-provider

`audistro-provider` serves HLS assets (`master.m3u8`, segments, keys passthrough paths) and optionally announces availability to `audicatalog`.

It supports:

- local filesystem asset serving
- proxy/hybrid origin fetching
- provider identity (persisted keypair)
- SQLite asset index
- internal rescan/announce endpoints
- health/readiness/metrics
- optional background jobs for rescan + reannounce

## Requirements

- Go `1.26+`
- writable data directory

## Quick Start (Local)

```bash
go run ./cmd/audistro-provider
```

Defaults:

- HTTP listen: `:8080`
- data path: `./data`
- assets dir: `./data/assets`
- identity file: `./data/provider_identity.json`
- DB file: `./data/provider.db`

Health checks:

```bash
curl -sS http://localhost:8080/healthz
curl -sS http://localhost:8080/readyz
curl -sS http://localhost:8080/metrics
```

## Docker (Service-Only)

```bash
cp .env.example .env
docker compose up -d --build
```

Stop:

```bash
docker compose down
```

## Asset Paths

- `GET|HEAD /assets/{assetId}/master.m3u8`
- `GET|HEAD /assets/{assetId}/{filename}`

Filesystem mode expects files under:

`$PROVIDER_DATA_PATH/$PROVIDER_ASSETS_SUBDIR/{assetId}/...`

## API Endpoints

- `GET /healthz`
- `GET /readyz`
- `GET /metrics`
- `GET /docs`
- `GET /openapi.yaml`
- `POST /internal/rescan` (if `PROVIDER_INTERNAL_ENABLE=true`)
- `POST /internal/announce` (if `PROVIDER_INTERNAL_ENABLE=true`)

Internal endpoints are intended for trusted/private networks only.

## Environment Variables

### Core Server / Paths

- `PROVIDER_HTTP_ADDR` (default `:8080`)
- `PROVIDER_DATA_PATH` (default `./data`)
- `PROVIDER_ASSETS_SUBDIR` (default `assets`)
- `PROVIDER_IDENTITY_PATH` (default `${PROVIDER_DATA_PATH}/provider_identity.json`)
- `PROVIDER_DB_PATH` (default `${PROVIDER_DATA_PATH}/provider.db`)
- `PROVIDER_SHUTDOWN_TIMEOUT_SECONDS` (default `15`)

### Asset Serving / Security

- `PROVIDER_MAX_SEGMENT_BYTES` (default `67108864`)
- `PROVIDER_RATE_LIMIT_RPS` (default `20`)
- `PROVIDER_RATE_LIMIT_BURST` (default `40`)
- `PROVIDER_ENFORCE_NO_SYMLINKS` (default `true`)

### CORS

- `PROVIDER_ENABLE_CORS` (default `false`)
- `PROVIDER_CORS_ALLOWED_ORIGINS` (required if CORS enabled, explicit origins only, no `*`)
- `PROVIDER_CORS_ALLOW_CREDENTIALS` (default `false`)

### Storage Mode / Origin Fetch

- `PROVIDER_STORAGE_MODE` (default `filesystem`; `filesystem|proxy|hybrid`)
- `PROVIDER_ORIGIN_BASE_URL` (required for `proxy` and `hybrid`)
- `PROVIDER_ORIGIN_AUTH_MODE` (`none|hmac`, default `none`)
- `PROVIDER_ORIGIN_HMAC_SECRET_PATH` (required if auth mode `hmac`)
- `PROVIDER_ORIGIN_HMAC_KEY_ID` (default `v1`)
- `PROVIDER_ORIGIN_AUTH_INCLUDE_QUERY` (default `true`)
- `PROVIDER_PROXY_MAX_UPSTREAM_CONCURRENCY` (default `64`)

### Catalog Announce

- `PROVIDER_PUBLIC_BASE_URL` (required when catalog integration is enabled)
- `PROVIDER_ALLOW_INSECURE_PUBLIC_URL` (default `false`; allows `http` public URL for dev)
- `PROVIDER_TRANSPORT` (default `https`; use `http` in dev where required)
- `PROVIDER_REGION` (optional)
- `PROVIDER_CATALOG_BASE_URL` (optional; enables catalog client/jobs)
- `PROVIDER_CATALOG_TIMEOUT_SECONDS` (default `10`)
- `PROVIDER_ANNOUNCE_PRIORITY` (default `10`)
- `PROVIDER_ANNOUNCE_EXPIRES_SECONDS` (default `604800`)
- `PROVIDER_ANNOUNCE_INTERVAL` (optional override)

### Jobs / Backoff

- `PROVIDER_ENABLE_JOBS` (default `true`)
- `PROVIDER_SCAN_ON_STARTUP` (default `true`)
- `PROVIDER_RESCAN_INTERVAL_SECONDS` (default `300`)
- `PROVIDER_ANNOUNCE_SWEEP_INTERVAL_SECONDS` (default `300`)
- `PROVIDER_REANNOUNCE_THRESHOLD_SECONDS` (default `86400`)
- `PROVIDER_BACKOFF_BASE_MILLIS` (default `2000`)
- `PROVIDER_BACKOFF_MAX_SECONDS` (default `600`)
- `PROVIDER_REJECTED_RETRY_SECONDS` (default `86400`)
- `PROVIDER_UNAUTHORIZED_RETRY_SECONDS` (default `3600`)
- `PROVIDER_ANNOUNCEMENT_EXPIRY_GRACE_SECONDS` (default `86400`)

### Internal Endpoint Access Control

- `PROVIDER_INTERNAL_ENABLE` (default `true`)
- `PROVIDER_INTERNAL_ALLOWED_CIDRS` (default `127.0.0.1/32,::1/128`)
- `PROVIDER_TRUST_PROXY_HEADERS` (default `false`)
- `PROVIDER_TRUSTED_PROXY_CIDRS` (optional)

### Optional Key Input

- `PROVIDER_PRIVATE_KEY_PATH` (optional fixed private key source)

## Identity Behavior

- Identity is loaded from disk if present; otherwise generated once.
- File contains provider UUID + secp256k1 keypair and creation timestamp.
- Keep this file persistent to avoid provider identity churn.

## Monorepo Integration Notes

When running with the top-level compose stack, common settings are:

- `PROVIDER_PUBLIC_BASE_URL=http://localhost:18082` (or `:18083`, `:18084` per instance)
- `PROVIDER_CATALOG_BASE_URL=http://audicatalog:8080`
- `PROVIDER_TRANSPORT=http` for dev stacks that allow insecure transport

For multiple providers, use separate data volumes/paths per instance.

## Build / Test / Release

```bash
make test
make vet
make build
make release
```

Artifacts:

- `dist/audistro-provider_linux_amd64`
- `dist/audistro-provider_linux_arm64`
- `dist/checksums.txt`
- `dist/sbom.cdx.json`

## Troubleshooting

### `readyz` unhealthy

- Check DB/data path writable:
  - `PROVIDER_DATA_PATH`
  - `PROVIDER_DB_PATH`
- Check scanner found assets (if expected) and startup scan enabled.
- If catalog integration is enabled, verify `PROVIDER_CATALOG_BASE_URL`.

### announce failing

- Verify `PROVIDER_PUBLIC_BASE_URL` format and scheme policy.
- Verify `PROVIDER_TRANSPORT` matches public URL scheme (`http` vs `https`).
- Verify catalog reachable and provider signing identity stable.

### CORS blocked in browser

- Set `PROVIDER_ENABLE_CORS=true`
- Set exact origin list in `PROVIDER_CORS_ALLOWED_ORIGINS`
- Do not use `*`

### origin proxy failures (proxy/hybrid)

- Verify `PROVIDER_ORIGIN_BASE_URL`
- If HMAC auth enabled, verify `PROVIDER_ORIGIN_HMAC_SECRET_PATH` and key length
- Increase `PROVIDER_PROXY_MAX_UPSTREAM_CONCURRENCY` cautiously if upstream is healthy

## Related Files

- Dockerfile: [`Dockerfile`](/audistro-provider/Dockerfile)
- Service-only compose: [`docker-compose.yml`](/audistro-provider/docker-compose.yml)
- systemd unit: [`deploy/systemd/audistro-provider.service`](/audistro-provider/deploy/systemd/audistro-provider.service)
- k8s example: [`deploy/k8s/deployment.yaml`](/audistro-provider/deploy/k8s/deployment.yaml)
- reverse proxy notes: [`docs/reverse-proxy.md`](/audistro-provider/docs/reverse-proxy.md)
- production TODOs: [`docs/todos/production-readiness.md`](/audistro-provider/docs/todos/production-readiness.md)
