# 0002 — Wildcard subdomain routing on a single TLS frontend

- Status: Accepted
- Date: 2026-05-16

## Context

Each HTTP/WS slot needs a stable, browser-reachable public URL. Three options:

1. **Per-slot path** under one hostname (`tunnels.example.com/<tunnel>/<slot>/...`).
2. **Per-slot subdomain** under one wildcard (`<slot>--<tunnel>.tunnels.example.com`).
3. **End-to-end TLS** with SNI passthrough — server doesn't decrypt, just forwards bytes.

## Decision

Use **per-slot subdomain on a wildcard cert**, with server-side TLS termination.

Slot subdomains are deterministic: `<slot-name>--<tunnel-name>.<TLS_DOMAIN>`. No hashes — the same triple `(owner, tunnel, slot)` maps to the same URL across every restart.

## Consequences

- `+` Apps run at site root (`/`), no `<base href>` rewriting, no path-prefix surprises.
- `+` Cookies, OAuth callbacks, CORS rules all work the same as on the real domain.
- `+` Stable URLs across restarts — the original motivation, the antithesis of devtunnels' `-{hash}` churn.
- `+` Server-side TLS termination keeps the CLI simple (no per-slot cert distribution).
- `−` Requires a wildcard cert (DNS-01 issuance), not just HTTP-01.
- `−` `--` is a reserved separator in slot/tunnel names; the resolver rejects names containing `--`.
- We can add SNI-passthrough later for raw-TLS use cases (e.g. hosting your own HTTPS upstream) as a separate slot kind — it does not affect the routing for `kind: "http"`.
