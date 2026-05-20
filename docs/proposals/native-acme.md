# Native ACME — built-in Let's Encrypt for v0.2

- Status: Accepted (superseded by [ADR 0007](../adr/0007-native-acme.md) for the canonical record)
- Date: 2026-05-20
- Tracks: Track 2 (drop the Caddy/Traefik TLS dependency)

## Who this is for

Operators who **do not** already run a TLS-terminating reverse proxy on the same box. Concretely:

- ✅ Someone deploying `agent-tunnel-server` on a bare VM (Hetzner CX22, Linode, a Raspberry Pi at home, anything).
- ✅ Someone who wants `docker run` to be the entire install — no Caddy, no Traefik, no Nginx to configure first.
- ❌ Someone whose box already runs Caddy / Traefik / Coolify with healthy ACME on `:80`/`:443`. Those users should follow [`coolify.md`](coolify.md) (or write the equivalent Caddyfile) — putting two ACME stacks on the same machine fights over the same ports.

The two paths reach the same outcome (TLS-terminated clean URLs). This proposal is the **server-owns-its-own-TLS** path so the project doesn't require a reverse proxy to ship a sensible default install. See `coolify.md`'s comparison table for the full trade-off.

## Port reality — `:443` is convention, not requirement

DNS-01 doesn't touch any port on your box:

- **Issuance**: certmagic writes a TXT record via the Cloudflare API; LE reads it via public DNS. No inbound connection to your host.
- **Renewal**: same loop every ~60 days. No inbound port needed.
- **OCSP stapling**: outbound to LE. Outbound only.

So `:443` is conventional, not required. The cert certmagic issues works on whatever port you bind. This means **native ACME can run side-by-side with Coolify/Traefik/Caddy** on the same box, as long as you give the tunnel server unused ports:

| Port pattern                                     | Coexists with another proxy? | URL shape                                                    |
|--------------------------------------------------|:----------------------------:|--------------------------------------------------------------|
| `:443` + `:7443`                                  | ❌ — needs `:443` to itself   | `https://*--*.tunnels.example.com/` (no suffix)              |
| `:8443` + `:7443`                                 | ✅                            | `https://*--*.tunnels.example.com:8443/`                     |
| Anything in `:1024–65535` not already in use      | ✅                            | `https://*--*.tunnels.example.com:<port>/`                   |

`:80` is **only** needed for HTTP-01 fallback. With DNS-01 set up there is no `:80` listener — the server doesn't bind it, and ACME-issued certs renew without it.

### When `:80`/`:443` are conventionally desirable

- You want zero port-suffix URLs in shared links → bind `:443`. Requires the box to have nothing else on `:443`.
- You want browser-friendly URLs that paste cleanly into chat → bind `:443`.

### When alternative ports are fine

- The tunnel URLs are mostly clicked from an Aspire dashboard or other tooling that handles ports transparently → use `:8443`/`:7443` (or any free pair).
- The box already runs Coolify/Caddy/Traefik on `:80`/`:443` → use alternative ports, coexist peacefully.
- You're testing native ACME without committing to a new deployment topology → use alternative ports, swap to `:443` later if you want.

The implementation supports both; pick at deploy time via `LISTEN_HTTPS` and `LISTEN_CONTROL` env vars.

## Context

v0.1 of `agent-tunnel-server` speaks plain HTTP on `:8080` and expects an external reverse proxy (Caddy / Traefik / Coolify) to terminate TLS. That works, but it pushes a non-trivial config (wildcard cert + DNS-01 + per-host SNI) onto the operator and defeats the single-binary install story the project is selling.

ADR 0002 already commits us to **server-side TLS termination on a wildcard cert** (`*.<TLS_DOMAIN>`). The only choice left is which library issues and renews that cert, and how it plugs into the server's listener wiring. `docs/deployment.md` and the Dockerfile already advertise the env-var shape (`ACME_EMAIL`, `ACME_DNS_PROVIDER`, `ACME_DNS_TOKEN`) — this proposal nails down the implementation.

Constraints carried over from v0.1:

- Single Go binary, no extra processes.
- All state under `$USER_DATA_DIR` so `tar czf` is the backup.
- Plain-HTTP dev mode must keep working (LISTEN_HTTP=:8080, no certmagic).
- `publicPortSuffix` already drops `:80` / `:443` from emitted URLs — confirmed at `src/agent-tunnel-server/internal/control/handler.go:188-190`.

## Decision

Use **`github.com/caddyserver/certmagic`** for ACME issuance, renewal, and OCSP stapling, with **libdns providers** for DNS-01 wildcard challenges.

v0.2 ships **Cloudflare-only**. Other providers added by request as separate blank-imports — each provider is a few MB of dependencies and we don't want to drag in route53 / azure / gcp by default.

When `ACME_EMAIL` is unset the server retains v0.1 plain-HTTP behaviour on `:8080` (dev mode). When set, the public listener moves to `:443` (TLS) with `:80` for ACME HTTP-01 challenge response + 301 redirect, and the control plane moves from `:7080` to `:7443` (WSS over the same cert).

HTTP-01 single-host fallback (for users without a DNS provider) is **deferred** — see Gotchas.

### Why certmagic over alternatives

| Library | Verdict | Reason |
|---|---|---|
| `caddyserver/certmagic` | **Pick** | Battle-tested in Caddy. Handles ACME + renewal + OCSP stapling + locking + storage abstraction in one `tls.Config`-returning call. libdns plugin ecosystem already exists for every provider we care about. |
| `golang.org/x/crypto/acme/autocert` | No | HTTP-01 only, no DNS-01, no wildcard. Would force per-slot certs and break ADR 0002. |
| `go-acme/lego` | No | More control, but we'd reimplement certmagic's cache/locking/stapling layer. Not worth it for a single-binary server. |

## Implementation

### Library wiring

```go
import (
    "github.com/caddyserver/certmagic"
    "github.com/libdns/cloudflare"
)

cm := certmagic.NewDefault()
cm.Storage = &certmagic.FileStorage{Path: filepath.Join(cfg.UserDataDir, "tls")}

acme := certmagic.NewACMEIssuer(cm, certmagic.ACMEIssuer{
    CA:     cfg.ACMEDirectory,      // LE prod by default; LE staging via env
    Email:  cfg.ACMEEmail,
    Agreed: true,
    DNS01Solver: &certmagic.DNS01Solver{
        DNSManager: certmagic.DNSManager{
            DNSProvider: &cloudflare.Provider{APIToken: cfg.ACMEDNSToken},
        },
    },
})
cm.Issuers = []certmagic.Issuer{acme}

domains := []string{cfg.PublicDomain, "*." + cfg.PublicDomain}
if err := cm.ManageAsync(ctx, domains); err != nil { /* fatal */ }

tlsCfg := cm.TLSConfig()
tlsCfg.NextProtos = append([]string{"h2", "http/1.1"}, tlsCfg.NextProtos...)
```

Both the public frontend (`:443`) and control-plane WSS (`:7443`) share `cm` — same storage, same renewal loop, one cert. The HTTP-01 challenge listener on `:80` is provided by `certmagic.HTTPChallengeHandler(redirectHandler)` wrapping a 301-to-https handler (kept around even though we use DNS-01, because LE periodically prefers HTTP-01 for renewal and certmagic handles the switching internally).

### Listener changes in `main.go`

Today (`src/agent-tunnel-server/main.go:42-48`):

```go
ctrlSrv := &http.Server{Addr: cfg.ListenControl, Handler: ctrlHandler}
httpSrv := &http.Server{Addr: cfg.ListenHTTP,    Handler: httpHandler}
```

After (sketched):

```go
if cfg.ACME.Enabled() {
    // :80 — ACME HTTP-01 + redirect to https
    challengeSrv := &http.Server{Addr: ":80",
        Handler: certmagic.HTTPChallengeHandler(redirectToHTTPS, cm.Issuers[0].(*certmagic.ACMEIssuer))}
    go runServer(log, "http-challenge", challengeSrv)

    // :443 — public frontend, TLS terminated
    httpsSrv := &http.Server{Addr: cfg.ListenHTTPS, Handler: httpHandler, TLSConfig: tlsCfg}
    go runServerTLS(log, "https", httpsSrv)

    // :7443 — control-plane WSS, same cert
    ctrlSrv := &http.Server{Addr: cfg.ListenControl, Handler: ctrlHandler, TLSConfig: tlsCfg}
    go runServerTLS(log, "control-tls", ctrlSrv)
} else {
    // v0.1 dev mode: plain HTTP on :8080, plain WS control on :7080.
    go runServer(log, "http",    &http.Server{Addr: cfg.ListenHTTP,    Handler: httpHandler})
    go runServer(log, "control", &http.Server{Addr: cfg.ListenControl, Handler: ctrlHandler})
}
```

`runServerTLS` calls `ListenAndServeTLS("", "")` — empty strings tell `net/http` to use the `TLSConfig.GetCertificate` certmagic populated. `cfg.TLS` becomes a computed property: `cfg.ACME.Enabled() && cfg.PublicScheme()=="https"`.

### Config changes

`config.go` grows an embedded ACME struct (kept flat in env, grouped in Go for readability):

```go
type ACMEConfig struct {
    Email       string  // ACME_EMAIL — required to enable
    DNSProvider string  // ACME_DNS_PROVIDER, default "cloudflare"
    DNSToken    string  // ACME_DNS_TOKEN
    Directory   string  // ACME_DIRECTORY, default LE prod
}
func (a ACMEConfig) Enabled() bool { return a.Email != "" }
```

`config.Load()` populates it from env. `PublicScheme()` returns `"https"` when `ACME.Enabled()`.

### Storage layout (under `$USER_DATA_DIR/tls/`)

certmagic's `FileStorage` lays out:

```
$USER_DATA_DIR/tls/
├── acme/
│   └── acme-v02.api.letsencrypt.org-directory/
│       ├── users/<email-hash>/<email>.json        # ACME account key + reg
│       └── sites/<domain>/<domain>.{crt,key,json} # issued cert + meta
├── ocsp/<hash>                                    # cached OCSP responses
└── locks/<resource>.lock                          # cross-process locks
```

Confirmed in certmagic README: `FileStorage{Path}` is the documented hook; default would be `$XDG_DATA_HOME/certmagic` which we override. `tar czf backup.tgz $USER_DATA_DIR` captures everything — including the ACME account key, which is the one piece you really don't want to lose (rate limits per registered account, not just per domain).

## Env var matrix

| Variable | Default | Required | Used | Purpose |
|---|---|---|---|---|
| `ACME_EMAIL` | — | yes (to enable ACME) | v0.2 | Sets up the ACME account. Empty = ACME off, v0.1 plain-HTTP behaviour. |
| `TLS_DOMAIN` | `localtest.me` | yes when ACME on | v0.1 cosmetic, v0.2 binding | v0.1: only emitted into URLs. v0.2: also the apex bound to the issued wildcard (`*.<TLS_DOMAIN>`). |
| `ACME_DNS_PROVIDER` | `cloudflare` | no | v0.2 | libdns provider name. Only `cloudflare` ships in v0.2. |
| `ACME_DNS_TOKEN` | — | yes when ACME on | v0.2 | Provider API token. Cloudflare: scoped token with `Zone:Read` + `Zone.DNS:Write` on the target zone. |
| `ACME_DIRECTORY` | `https://acme-v02.api.letsencrypt.org/directory` | no | v0.2 | Override to `https://acme-staging-v02.api.letsencrypt.org/directory` for testing without burning quota. |
| `LISTEN_HTTP` | `:80` when ACME on, `:8080` otherwise | no | v0.1/v0.2 | ACME mode: HTTP-01 challenge + redirect. Dev mode: plain HTTP frontend. |
| `LISTEN_HTTPS` | `:443` | no | v0.2 | Public TLS frontend. Only used when ACME on. |
| `LISTEN_CONTROL` | `:7443` when ACME on, `:7080` otherwise | no | v0.1/v0.2 | Control-plane (WSS in ACME mode, WS otherwise). |
| `PUBLIC_HTTP_PORT` | — | no | v0.1/v0.2 | Override port in emitted URLs (Docker host/container port mismatch). With ACME on and host-mapped to `:443`, leave unset — `publicPortSuffix` already drops `:443`. |
| `USER_DATA_DIR` | `/data` (container), `./app/user-data` (dev) | no | v0.1/v0.2 | TLS state goes to `<USER_DATA_DIR>/tls/`. |

### v0.1 → v0.2 behaviour of `TLS_DOMAIN`

- **v0.1**: `TLS_DOMAIN` is decorative — it's only the parent domain stamped into emitted URLs. The server doesn't bind to it, doesn't validate it.
- **v0.2 with ACME off**: same as v0.1 (so `localtest.me` keeps working for dev).
- **v0.2 with ACME on**: `TLS_DOMAIN` becomes the apex passed to `cm.ManageAsync([TLS_DOMAIN, "*."+TLS_DOMAIN])`. Required, validated as a real FQDN at startup, must match the DNS record the user pointed at the box.

## Files to add/modify

| File | Change |
|---|---|
| `src/agent-tunnel-server/go.mod` | + `github.com/caddyserver/certmagic`, `github.com/libdns/cloudflare`. |
| `src/agent-tunnel-server/internal/config/config.go` | Add `ACME` struct + env wiring. `PublicScheme()` returns `https` when enabled. Default `ListenHTTP`/`ListenControl` flip when enabled. |
| `src/agent-tunnel-server/internal/tls/` (new) | `Setup(ctx, cfg) (*tls.Config, http.Handler, error)` returning the `tls.Config` and the `:80` challenge handler. Encapsulates certmagic wiring so `main.go` stays small. |
| `src/agent-tunnel-server/main.go` | Branch on `cfg.ACME.Enabled()` for listener wiring (see sketch above). |
| `src/agent-tunnel-server/Dockerfile` | `EXPOSE 80 443 7443` in addition to existing 8080/7080 — keep both so the same image runs in dev and prod. Drop the `# v0.2 will…` comment. |
| `docs/adr/0007-native-acme.md` (new) | Promote this proposal to an accepted ADR once implementation merges. |
| `docs/deployment.md` | Drop the "aspirational" banner. Keep the existing env-var matrix; it's already aligned with this proposal. |
| `README.md` | Replace "VPS install with Caddy front" recipe with the new single-container recipe; keep Caddy recipe in an appendix for users who already have a Caddy install. |
| `src/agent-tunnel-server/internal/control/handler.go` | **No change** — `publicPortSuffix` already returns empty for `:443`. Confirmed at `handler.go:188-190`. |

## Prerequisites

- DNS A/AAAA records: `tunnels.example.com` and `*.tunnels.example.com` both pointing at the box. (CNAME for the wildcard is fine; the apex must be A/AAAA.)
- A Cloudflare API token scoped to the target zone (`Zone:Read` + `Zone.DNS:Write`).
- Firewall opens `:80`, `:443`, `:7443`, and the TCP pool range.
- Box has reachable IPv4 — Let's Encrypt's validation servers must be able to read the TXT record DNS-01 writes. (For DNS-01 the box itself doesn't need to be reachable for cert issuance; for renewal of HTTP-01 fallback it does.)
- `ACME_EMAIL` is a real address — LE sends expiry warnings there if renewal stalls.
- Persistent volume on `/data`. Losing the account key means re-registering, which costs you a slot in LE's per-account rate limit pool.

## Gotchas

- **Let's Encrypt rate limits.** 50 certificates per registered domain per week (a wildcard counts as one), 5 duplicate certs per week, 5 failed validations per account per hostname per hour. For wildcard issuance via DNS-01 we issue **one** cert covering `apex + *.apex` — well below any limit. Use LE staging (`ACME_DIRECTORY=<staging>`) when iterating on the integration; staging has the same shape but doesn't share quota.
- **Cloudflare API token scope.** Must be a *token*, not a legacy global API key. Minimum scope is `Zone:Read` + `Zone.DNS:Write` on the specific zone (`tunnels.example.com`). A zone-wide write token is overkill but works.
- **First-issue blocks startup briefly.** `ManageAsync` returns immediately, but until the first cert lands `tlsCfg.GetCertificate` returns an error and `:443` connections fail. The server should log "ACME: issuing initial cert for X, Y…" and `:443` requests get a TLS handshake failure for ~30–90s. Document this.
- **Clock skew.** ACME signatures are time-sensitive. If the host clock is more than a few minutes off, every issuance fails with a confusing `urn:ietf:params:acme:error:malformed` — check NTP in the install guide.
- **HTTP-01 single-host fallback — deferred to v0.3.** Tempting for users without a DNS provider, but: (1) LE's 50-certs-per-registered-domain-per-week means a power user with 50+ slot subdomains hits the cap; (2) we'd need on-demand issuance hooks for cold-cache subdomains, increasing first-request latency; (3) the design conflicts with the wildcard story in ADR 0002. Document as a non-goal for v0.2; revisit if a real user asks.
- **Multiple servers sharing `$USER_DATA_DIR`.** certmagic's lock files in `tls/locks/` are designed for the cluster case, but ADR 0006 already mandates single-instance via `runtime/server.lock`. No change here; just don't accidentally point two servers at one volume.
- **OCSP stapling silently fails closed by certmagic.** If LE's OCSP responder is unreachable at renewal time, certmagic logs and serves the cert without the stapled response — fine, browsers handle it. Worth a log-level note, not a blocker.
- **`PublicScheme()` switches based on ACME enable.** Any code path that built a URL by string-concatenating `http://...` instead of calling `cfg.PublicScheme()` is now wrong. The control handler at `handler.go:112` does it right; grep the repo for `"http://"` before merging.
- **Aspire extension uplift.** The extension currently assumes `http`. Once v0.2 lands, `GetEndpoint` returns an `https://` URL — confirm `Aspire.Hosting.AgentTunnel` doesn't hard-code the scheme. Out of scope for this proposal but track as a follow-up.
