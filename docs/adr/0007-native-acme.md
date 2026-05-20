# 0007 — Native ACME for built-in TLS

- Status: Accepted
- Date: 2026-05-20

## Context

ADR 0002 commits us to server-side TLS termination on a wildcard cert
(`*.<TLS_DOMAIN>`). v0.1 punted on issuance — operators had to front the
server with Caddy/Traefik/Coolify. That defeats the single-binary install
story for users on a bare VM, and forces non-trivial reverse-proxy config
(wildcard DNS-01 + per-host SNI) on every new deploy.

We need built-in ACME so `docker run` is the whole install, while still
letting operators with an existing TLS proxy skip the built-in path.

## Decision

Use **`github.com/caddyserver/certmagic`** for ACME issuance, renewal, and
OCSP stapling, with **libdns providers** for DNS-01 wildcard challenges.
v0.2 ships **Cloudflare-only** (`github.com/libdns/cloudflare`); other
providers add later as opt-in blank-imports.

A single `certmagic.Config` issues one cert covering
`{apex, *.apex}` and is shared by the public frontend (`:443`) and the
control plane (`:7443`). HTTP-01 fallback handler stays bound to `:80`
even though we issue via DNS-01 — certmagic handles the switching
internally and the listener also serves the 301-to-https redirect.

`ACME_EMAIL` is the master switch:
- **Empty** → v0.1 plain-HTTP behaviour (`:8080` HTTP, `:7080` WS). Dev mode.
- **Set** → ACME on. Listeners flip to `:80` / `:443` / `:7443`.
  `PublicScheme()` returns `https`. State lives under `$USER_DATA_DIR/tls/`.

HTTP-01 single-host fallback (for users without a DNS provider) is
deferred — see the proposal's Gotchas section.

## Consequences

- `+` Single binary, single `docker run`, no reverse proxy needed.
- `+` Wildcard issuance via DNS-01 — every slot subdomain works without
  per-slot certs.
- `+` All TLS state (account key, certs, OCSP, locks) sits under
  `$USER_DATA_DIR` — `tar czf` is still the backup.
- `+` Same `tls.Config` powers `:443` and `:7443`, so one renewal cycle
  covers both planes.
- `−` Binds `:80` + `:443` — conflicts with any other proxy already on
  those ports. Operators with Coolify/Caddy/Traefik should keep using
  the reverse-proxy path (see `coolify.md`).
- `−` First-cert issuance takes 30–90s; `:443` handshakes fail until
  the cert lands.
- `−` Loss of `$USER_DATA_DIR/tls/acme/.../users/` costs a slot in
  Let's Encrypt's per-account rate limit pool — back up the volume.
- `−` Each libdns provider drags in a few MB of deps; we keep the
  default lean by shipping cloudflare only and adding others on
  request.

See `docs/proposals/native-acme.md` for the full design, env-var matrix,
storage layout, and gotchas.
