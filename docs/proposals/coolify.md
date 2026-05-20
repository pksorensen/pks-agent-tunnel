# Coolify integration — drop `:18080` from public URLs

- Status: Proposed
- Date: 2026-05-20
- Track: 3 (Coolify + Traefik)
- Related: Track 2 (native ACME, separate proposal)

## Context

`registry.kjeldager.io/agent-tunnel-server:0.2.0` speaks plain HTTP on `:8080`
(public frontend) and `:7080` (control plane). Today the user runs it directly
on a Hetzner box and publishes `host:18080` — public URLs look like
`https://ws-relay--tun1.tunnels.agentics.dk:18080/`. The `:18080` suffix is
both ugly and a giveaway that this isn't a real production deploy.

The user already runs **Coolify** on the same host, and Coolify ships
**Traefik v3.1** as its built-in reverse proxy (`coolify-proxy`). The Traefik
container watches the `coolify` Docker network with
`--providers.docker.exposedbydefault=false` — i.e. any container on that
network can opt in to TLS termination by setting `traefik.enable=true` plus a
router rule. Traefik holds a single `acme.json` with ~25 live certs for the
user's other apps (HTTP-01 only).

This proposal sketches the **Coolify-based delivery** of `agent-tunnel-server`
and weighs it against **Track 2 (native ACME inside the server)**. Tracks 2
and 3 reach the same outcome — `https://ws-relay--tun1.tunnels.agentics.dk/`
with no port suffix — by different means.

## Decision

**Ship Track 2 (native ACME) first.** Document Track 3 (Coolify + Traefik) as
a recipe in `docs/deploy/coolify.md` for users who already run Traefik and
prefer to keep cert management in one place. Do not build a Coolify
one-click service template (the user-defined Docker Compose flow already
covers it; the 1000-stars bar on the official catalogue rules out the curated
list anyway).

Coolify integration is **documentation, not code** — the server image already
emits port-suffix-free URLs when `PUBLIC_HTTP_PORT=443`, and the existing
`--tls` flag flips `PublicScheme()` to `https` so the emitted URLs match what
Traefik serves.

## Two paths comparison (Coolify vs native ACME)

|                                 | Track 2 — Native ACME               | Track 3 — Coolify + Traefik           |
|---------------------------------|--------------------------------------|----------------------------------------|
| New code in server              | ACME client, cert cache, SNI router  | None                                   |
| Operator dependencies           | Just the Docker image + DNS-01 token | Coolify, Traefik, Coolify's `acme.json`|
| Works on a bare VM              | Yes                                  | No — requires Coolify+Traefik          |
| Wildcard cert source            | DNS-01 baked in                      | Requires Traefik DNS-01 resolver       |
| Affects unrelated apps          | No                                   | Yes — shares the Traefik static config |
| Time to "no port suffix"        | Implementation work                  | Recipe + Traefik config tweak          |
| Control plane TLS               | Server terminates on `:443`/`:7443`  | Traefik terminates, proxies HTTP/WS    |
| Slot URL TLS                    | Server terminates with SNI lookup    | Traefik terminates on wildcard         |
| Future portability              | High — runs anywhere                 | Low — tied to user's Coolify host      |

**Recommendation:** Build Track 2. It's strictly more general — Coolify users
can still front the server with Traefik if they prefer, by setting `TLS=false`
on the server and using the Track 3 recipe. The reverse is not true: a bare
VM operator cannot use the Coolify recipe without installing Coolify.

Use Track 3 today as an **interim deploy** on the Hetzner host so the user
can stop showing `:18080` URLs in demos, while Track 2 is being built.

## Recipe (docker run + labels)

This is the interim deployment recipe. It assumes:

- Coolify is already installed and `coolify-proxy` (Traefik) is healthy.
- The `coolify` Docker network exists (it does — Coolify creates it).
- A DNS-01 resolver named `letsencrypt-dns` has been added to Traefik (see
  next section). Until then, fall back to `letsencrypt` (HTTP-01) and accept
  the rate-limit risk for the slot frontend.
- DNS records `tunnels.agentics.dk` and `*.tunnels.agentics.dk` both point at
  the Hetzner box's public IP.

```bash
docker run -d \
  --name agent-tunnel-server \
  --restart unless-stopped \
  --network coolify \
  -v agent-tunnel-data:/data \
  -e TLS_DOMAIN=tunnels.agentics.dk \
  -e PUBLIC_HTTP_PORT=443 \
  -e LISTEN_HTTP=:8080 \
  -e LISTEN_CONTROL=:7080 \
  -e AUTH_MODE=anonymous \
  --label traefik.enable=true \
  \
  `# --- public HTTP frontend: <slot>--<tunnel>.tunnels.agentics.dk ---` \
  --label 'traefik.http.routers.agent-tunnel-public.rule=HostRegexp(`{sub:[a-z0-9-]+--[a-z0-9-]+}.tunnels.agentics.dk`)' \
  --label traefik.http.routers.agent-tunnel-public.entrypoints=https \
  --label traefik.http.routers.agent-tunnel-public.tls=true \
  --label traefik.http.routers.agent-tunnel-public.tls.certresolver=letsencrypt-dns \
  --label 'traefik.http.routers.agent-tunnel-public.tls.domains[0].main=tunnels.agentics.dk' \
  --label 'traefik.http.routers.agent-tunnel-public.tls.domains[0].sans=*.tunnels.agentics.dk' \
  --label traefik.http.routers.agent-tunnel-public.service=agent-tunnel-public \
  --label traefik.http.services.agent-tunnel-public.loadbalancer.server.port=8080 \
  \
  `# --- control plane: tunnels.agentics.dk/v1/control (WSS upgrade) ---` \
  --label 'traefik.http.routers.agent-tunnel-control.rule=Host(`tunnels.agentics.dk`) && PathPrefix(`/v1/control`)' \
  --label traefik.http.routers.agent-tunnel-control.entrypoints=https \
  --label traefik.http.routers.agent-tunnel-control.tls=true \
  --label traefik.http.routers.agent-tunnel-control.tls.certresolver=letsencrypt-dns \
  --label traefik.http.routers.agent-tunnel-control.service=agent-tunnel-control \
  --label traefik.http.services.agent-tunnel-control.loadbalancer.server.port=7080 \
  \
  registry.kjeldager.io/agent-tunnel-server:latest --tls
```

Equivalent Coolify **Docker Compose Empty** resource (paste under
*Resources → New → Docker Compose Empty*):

```yaml
services:
  agent-tunnel-server:
    image: registry.kjeldager.io/agent-tunnel-server:latest
    command: ["--tls"]
    restart: unless-stopped
    networks: [coolify]
    volumes:
      - agent-tunnel-data:/data
    environment:
      TLS_DOMAIN: tunnels.agentics.dk
      PUBLIC_HTTP_PORT: "443"
      LISTEN_HTTP: ":8080"
      LISTEN_CONTROL: ":7080"
      AUTH_MODE: anonymous
    labels:
      - traefik.enable=true
      # public HTTP slot frontend (wildcard host)
      - "traefik.http.routers.agent-tunnel-public.rule=HostRegexp(`{sub:[a-z0-9-]+--[a-z0-9-]+}.tunnels.agentics.dk`)"
      - traefik.http.routers.agent-tunnel-public.entrypoints=https
      - traefik.http.routers.agent-tunnel-public.tls=true
      - traefik.http.routers.agent-tunnel-public.tls.certresolver=letsencrypt-dns
      - "traefik.http.routers.agent-tunnel-public.tls.domains[0].main=tunnels.agentics.dk"
      - "traefik.http.routers.agent-tunnel-public.tls.domains[0].sans=*.tunnels.agentics.dk"
      - traefik.http.routers.agent-tunnel-public.service=agent-tunnel-public
      - traefik.http.services.agent-tunnel-public.loadbalancer.server.port=8080
      # control plane (apex + /v1/control)
      - "traefik.http.routers.agent-tunnel-control.rule=Host(`tunnels.agentics.dk`) && PathPrefix(`/v1/control`)"
      - traefik.http.routers.agent-tunnel-control.entrypoints=https
      - traefik.http.routers.agent-tunnel-control.tls=true
      - traefik.http.routers.agent-tunnel-control.tls.certresolver=letsencrypt-dns
      - traefik.http.routers.agent-tunnel-control.service=agent-tunnel-control
      - traefik.http.services.agent-tunnel-control.loadbalancer.server.port=7080

networks:
  coolify:
    external: true

volumes:
  agent-tunnel-data:
```

### Why these specific labels

- `HostRegexp(...)` matches `<slot>--<tunnel>.tunnels.agentics.dk` and rejects
  anything that isn't the slot pattern (so the apex hostname doesn't
  collide with the slot router). The `--` separator is enforced by the
  control plane (ADR 0002), so the regex character class is the cheapest
  guard.
- Two routers, two services — one points at the container's `:8080`, the
  other at `:7080`. Traefik treats them independently for cert renewal and
  rate limits.
- `tls.domains[0].main` + `sans` is what asks Traefik to issue the
  **wildcard** cert via DNS-01 ahead of any matched host. Without it,
  Traefik would issue per-leaf certs on first match (HTTP-01) — which is
  exactly the rate-limit pitfall in the next section.
- `--tls` flag flips the server's emitted scheme to `https://`. `PUBLIC_HTTP_PORT=443`
  drops the suffix (the existing `publicPortSuffix` function returns ""
  for 443/80).
- **TCP slot ports (`10000–19999`) are NOT in this recipe.** Coolify's
  Traefik isn't a TCP load balancer for arbitrary port ranges. If TCP slots
  matter for this deploy, publish those ports on the host (`-p 10000-19999:10000-19999`)
  and document that the TCP frontend stays on the host IP, not behind
  Traefik. For most demo use the HTTP slot is enough.

## Coolify Traefik config changes

Coolify v4 stores the proxy compose at `/data/coolify/proxy/docker-compose.yml`
and the ACME state at `/data/coolify/proxy/acme.json`. There is **no UI panel**
for editing the static Traefik config — operators edit the file directly and
run `docker compose up -d` in that directory.

Add these flags to the `command:` list of the `traefik` service (keep the
existing `letsencrypt` resolver untouched so the other 25+ apps keep
renewing):

```yaml
    command:
      # ...existing flags (letsencrypt HTTP-01, providers, etc.)...

      # New DNS-01 wildcard resolver for tunnels.agentics.dk
      - '--certificatesresolvers.letsencrypt-dns.acme.email=poul@kjeldager.com'
      - '--certificatesresolvers.letsencrypt-dns.acme.storage=/traefik/acme-dns.json'
      - '--certificatesresolvers.letsencrypt-dns.acme.dnschallenge=true'
      - '--certificatesresolvers.letsencrypt-dns.acme.dnschallenge.provider=cloudflare'
      - '--certificatesresolvers.letsencrypt-dns.acme.dnschallenge.resolvers=1.1.1.1:53,1.0.0.1:53'
      # For staging while testing — comment out for prod:
      # - '--certificatesresolvers.letsencrypt-dns.acme.caserver=https://acme-staging-v02.api.letsencrypt.org/directory'
    environment:
      # Scoped Cloudflare API token: Zone.DNS:Edit + Zone.Zone:Read for the
      # agentics.dk zone only. Do NOT reuse the global API key.
      CF_DNS_API_TOKEN: ${CF_DNS_API_TOKEN}
```

Key choices:

- **Separate `acme-dns.json`** rather than reusing `acme.json`. Keeps the
  blast radius small: if the new resolver misbehaves it can't scribble on
  the production ACME state for the other 25+ apps.
- **Scoped Cloudflare token** (not the global key). Permissions: `Zone:Read`
  and `DNS:Edit` for `agentics.dk` only.
- **`CF_DNS_API_TOKEN`** is the legacy-friendly env var name; Traefik also
  accepts `CLOUDFLARE_DNS_API_TOKEN`. Both work — pick one and stay
  consistent.
- DNS resolvers `1.1.1.1` and `1.0.0.1` are explicit so Traefik isn't at the
  mercy of whichever stub resolver Docker injects (sometimes Docker's
  embedded resolver caches negatively and the DNS-01 propagation check
  fails).

### Testing strategy — do not flip prod blindly

Touching the production Traefik static config affects 25+ live cert renewals.
Roll it out in three steps:

1. **Staging CA first.** Uncomment the `caserver=` line above and bring up
   `letsencrypt-dns` against Let's Encrypt staging. Deploy the agent tunnel
   server pointing at `letsencrypt-dns`. Confirm the wildcard `*.tunnels.agentics.dk`
   cert appears in `acme-dns.json` and that browsers see the Fake LE Intermediate.
2. **Production CA, agent tunnel only.** Comment the staging line out and
   `docker compose up -d` Traefik. Wait for the production wildcard to land
   in `acme-dns.json`. Confirm both routers serve real certs. **Other apps
   keep using `letsencrypt` (HTTP-01) on the original `acme.json`** —
   nothing changes for them.
3. **Watch for 24h.** If renewal of an unrelated app hiccups, the staging
   step caught nothing because the new resolver name and storage file are
   isolated — investigate Traefik startup, not ACME.

If the Cloudflare token isn't available, **skip Track 3 entirely** and wait
for Track 2.

## HTTP-01 fallback (no wildcard)

If the user refuses to add DNS-01, Track 3 still works in degraded mode: drop
the `tls.domains[*]` lines and let Traefik issue a per-leaf cert on first
match. **This means each new slot URL triggers an HTTP-01 issuance.**

Let's Encrypt limits: **50 certs per registered domain per week**
(`agentics.dk`, including all subs). The user's existing Coolify already
consumes ~25 of that quota in the rolling 7-day window during initial
rollouts. So:

| Slot count fanout per week | Verdict (HTTP-01 fallback)                           |
|----------------------------|------------------------------------------------------|
| ≤ 5                        | Fine — well under the budget.                        |
| 5 – 20                     | OK if other apps aren't churning certs.              |
| 20 – 50                    | Risky — competes with the other 25 apps.             |
| > 50                       | Will be rate-limited; renewals fail for ~7 days.     |

For the user's actual workload (ws-relay + plugin-marketplace, 2 slots, very
rare changes) HTTP-01 fallback is fine forever. **Still prefer DNS-01** — the
wildcard means a slot URL change doesn't trigger a new cert at all, and the
guarantee scales for free if slot counts ever grow.

## DNS records

Set these in Cloudflare for `agentics.dk` (proxy: **DNS only**, grey cloud —
not orange — so SNI passes through):

| Type | Name                  | Value             | TTL  |
|------|-----------------------|-------------------|------|
| A    | `tunnels`             | `<hetzner-ipv4>`  | Auto |
| A    | `*.tunnels`           | `<hetzner-ipv4>`  | Auto |

Optional, for IPv6 reachability:

| Type | Name                  | Value             | TTL  |
|------|-----------------------|-------------------|------|
| AAAA | `tunnels`             | `<hetzner-ipv6>`  | Auto |
| AAAA | `*.tunnels`           | `<hetzner-ipv6>`  | Auto |

These records are already documented in `README.md`. No change for the
Coolify deploy specifically — Traefik resolves names the same way the
bare-VM setup does.

## Gotchas

- **`--tls` flag is mandatory** for the server even though Traefik terminates
  TLS. Without it, the server emits `http://...` URLs that browsers will
  upgrade-and-then-mismatch under HSTS, or just look wrong. The flag only
  flips the scheme in emitted URLs — the listeners stay plain HTTP inside
  the container.
- **No `TLS=true` env var exists.** Either pass `--tls` via `command:` (compose)
  or as a trailing arg to `docker run`. (Future: add a `TLS=true` env shim.)
- **`PUBLIC_HTTP_PORT=443`** is what drops the `:18080` suffix. Without it,
  emitted URLs include `:8080` from the listener address. The
  `publicPortSuffix` function already collapses `:443` and `:80` to empty.
- **Yamux tunnel over WSS.** The control plane upgrades to WebSocket and
  then runs yamux multiplexing on top. Traefik handles WebSocket upgrades
  natively — no `Connection: upgrade` workaround needed in modern Traefik
  v3, but **do not** put any `compress` middleware on the control router.
- **Long-lived control connections.** Traefik's default idle timeout will
  drop the yamux session. Add `--entrypoints.https.transport.respondingTimeouts.idleTimeout=0`
  or apply a router-scoped timeout middleware. Test before declaring done.
- **Coolify network gotcha.** Raw `docker run` containers must be on the
  `coolify` network OR you must add `--providers.docker.network=coolify`
  to Traefik's static config. The recipe uses `--network coolify` for
  the no-config-change path.
- **Coolify rebuilds.** If the user later imports this resource into the
  Coolify UI and redeploys, Coolify rewrites container names with a UUID
  suffix. Update the `traefik.http.services.agent-tunnel-*.loadbalancer.server.port`
  labels only — Traefik resolves services by the container's network alias,
  not name.
- **No TCP slot frontend.** Coolify's Traefik isn't configured as a TCP
  router. TCP slots stay on host-published ports. If TCP slots become a
  product requirement, that's a Track 2 argument, not a Track 3 fix.
- **No one-click template.** Coolify's curated catalogue requires ≥1000
  GitHub stars on the source repo, which `pks-agent-tunnel` does not meet.
  Users get the Docker Compose Empty path, not a one-click install. This
  is fine — the compose snippet above is the entire onboarding doc.

## Sources

- Coolify Services overview — https://coolify.io/docs/services/introduction
- Coolify Docker Compose build pack — https://coolify.io/docs/applications/build-packs/docker-compose
- Traefik ACME (DNS challenge, Cloudflare) — https://doc.traefik.io/traefik/https/acme/
- Traefik docker-compose ACME-DNS user guide — https://doc.traefik.io/traefik/user-guides/docker-compose/acme-dns/
- Reference compose for Traefik+Cloudflare DNS-01 — https://github.com/eingress/docker-compose-traefik-letsencrypt-cloudflare
