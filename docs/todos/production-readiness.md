# Production Readiness TODOs (audistro-provider)

This document tracks remaining gaps before `audistro-provider` can be considered production-ready.
Items are grouped by priority.

## P0 — Production Blockers (must-have)

### Security
- [ ] Ensure filesystem escape protections are enabled and tested (no symlink traversal, no path escape).
- [ ] Internal endpoints exposure model is safe by default:
  - [ ] `/internal/*` protected by CIDR allowlist (loopback by default) OR not mounted unless explicitly enabled.
  - [ ] If `TRUST_PROXY_HEADERS=true`, require `TRUSTED_PROXY_CIDRS` and document the risk (no blind XFF trust).

### Proxy Mode (if enabled)
- [ ] Define and implement origin authentication/authorization for proxy mode (mTLS and/or signed origin requests).
- [ ] Add basic abuse limits for proxy mode (max upstream concurrency / connection limits) to protect origin and provider.

### Deployment
- [ ] Provide a hardened reverse-proxy reference config (Nginx/Caddy/Traefik) with:
  - TLS termination
  - sane timeouts for HLS/range streaming
  - request size limits
  - header forwarding rules (XFF policy)

## P1 — Strong Hardening (recommended)

### Security
- [ ] Add optional authenticated authorization for `/internal/rescan` and `/internal/announce` (token-based), in addition to CIDR allowlist.

### Reliability
- [ ] Improve graceful shutdown behavior for long-running streams (document operational drain strategy; optionally add max drain time & connection tracking).

### Testing/CI
- [ ] Run `go test -race ./...` in CI (linux/amd64).
- [ ] Add tests for identity file permissions and atomic-write behavior.

## P2 — Optional Hardening / Enterprise

### Security / Key Management
- [ ] Encrypt identity private key at rest (optional; v1 stores plaintext key).
- [ ] Add optional KMS/HSM-backed key management.
- [ ] Add key rotation and recovery process (only needed if provider identity becomes policy/reputation critical).

### Observability
- [ ] Add structured error taxonomy and alert-friendly log fields.
- [ ] Add distributed tracing (OpenTelemetry).

### Quality Gates
- [ ] Add linting/static analysis gates (golangci-lint) if desired.
- [ ] Add integration tests for HTTP server startup/shutdown behavior.
- [ ] Add coverage targets in CI.
