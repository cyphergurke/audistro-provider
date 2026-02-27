# Reverse Proxy Reference (TLS Termination)

`audistro-provider` should run behind a reverse proxy that terminates TLS.
Run the provider itself on loopback/private bind only.

Reference configs:

- `deploy/caddy/Caddyfile`
- `deploy/nginx/audistro-provider.conf`

## Provider Runtime Settings Behind Proxy

Recommended baseline:

```env
PROVIDER_HTTP_ADDR=127.0.0.1:8080
PROVIDER_PUBLIC_BASE_URL=https://provider.example
PROVIDER_TRUST_PROXY_HEADERS=false
```

If you must enable trusted proxy headers:

```env
PROVIDER_TRUST_PROXY_HEADERS=true
PROVIDER_TRUSTED_PROXY_CIDRS=127.0.0.1/32,10.0.0.0/24
```

Only include CIDRs for your own proxy layer. Never trust arbitrary client-provided `X-Forwarded-For`.

## Routing and Exposure Policy

Public routes:

- `/assets/*`
- `/healthz`
- `/readyz`

Not publicly exposed by default:

- `/internal/*` blocked (`403`)
- `/metrics` hidden (`404`)

Expose `/metrics` only on private networks or behind auth/VPN.

## Streaming and Range Guidance

For HLS/fMP4 streaming and byte-range support:

- Pass `Range` requests through unchanged (Caddy/Nginx defaults support this).
- Disable compression for media routes (`/assets/*`) to avoid range/content-encoding issues.
- Keep long enough upstream/proxy timeouts for segment transfer and range reads.

## Header Forwarding Policy

Forward only needed headers to provider:

- `Host`
- `X-Forwarded-Proto`
- `X-Forwarded-For`

Do not add broad trust of forwarded headers in app config unless proxy CIDRs are strictly pinned.

## Browser Player and CORS

Provider CORS is disabled by default.

Enable only when needed and restrict origins:

```env
PROVIDER_ENABLE_CORS=true
PROVIDER_CORS_ALLOWED_ORIGINS=https://app.example
PROVIDER_CORS_ALLOW_CREDENTIALS=false
```

Avoid wildcard origins for production browser playback.
