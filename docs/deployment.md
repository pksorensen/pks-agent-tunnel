# Deployment

> **Heads-up — v0.1 has no native ACME yet.** For the working v0.1 install, see the [Caddy-fronted recipe in the top-level README](../README.md#vps-installation-guide-v01--caddy-front-for-tls). The rest of this document describes the **v0.2+ native-ACME design** that drops the Caddy hop. Env vars listed here are aspirational until v0.2 lands.

The server ships as a Docker image to `ghcr.io/pksorensen/agent-tunnel-server`. Deploy on a single VPS (Hetzner / Coolify) with:

- A public IPv4 (the server terminates TLS itself).
- An apex domain or subdomain pointed at it with a wildcard A/AAAA record (`*.tunnels.example.com`).
- A DNS API token for the DNS provider (used for ACME DNS-01 wildcard issuance).
- Firewall rules permitting 80, 443, 7443, and the TCP pool range (10000–19999 default).

## DNS

```
A  *.tunnels.agentics.dk  → <vps-ip>
A  tunnels.agentics.dk    → <vps-ip>
```

The wildcard handles every `<slot>--<tunnel>.tunnels.agentics.dk`; the apex `tunnels.agentics.dk` is where the control plane lives (`https://tunnels.agentics.dk:7443/v1/control`).

## TLS

certmagic + Let's Encrypt DNS-01 — set:

```
TLS_DOMAIN=tunnels.agentics.dk
ACME_EMAIL=ops@agentics.dk
ACME_DNS_PROVIDER=cloudflare
ACME_DNS_TOKEN=<api-token>
```

State (account keys, certs, locks) lives under `$USER_DATA_DIR/tls/` — persisted with the data volume.

## Firewall

```bash
ufw allow 80/tcp
ufw allow 443/tcp
ufw allow 7443/tcp
ufw allow 10000:19999/tcp
```

## docker run

```bash
docker stop agent-tunnel && docker rm agent-tunnel

docker run -d \
  --name agent-tunnel \
  --restart unless-stopped \
  -p 80:80 -p 443:443 -p 7443:7443 -p 10000-19999:10000-19999 \
  -v agent-tunnel-data:/data \
  -e TLS_DOMAIN=tunnels.agentics.dk \
  -e ACME_EMAIL=ops@agentics.dk \
  -e ACME_DNS_PROVIDER=cloudflare \
  -e ACME_DNS_TOKEN=<api-token> \
  -e AUTH_MODE=token \
  ghcr.io/pksorensen/agent-tunnel-server:latest
```

To update:

```bash
docker pull ghcr.io/pksorensen/agent-tunnel-server:latest
docker restart agent-tunnel
```

## Coolify

1. Add service → Docker Image → `ghcr.io/pksorensen/agent-tunnel-server:latest`.
2. Set the env vars above.
3. Add a persistent volume mounted at `/data`.
4. **Port bypass**: Coolify normally routes HTTP/HTTPS through Traefik. This server terminates TLS itself, so 443, 7443, and the TCP pool must bind directly to the host — *not* through the proxy. Map them as raw port bindings.

## Env var reference

| Variable             | Default          | Purpose                                                  |
|----------------------|------------------|----------------------------------------------------------|
| `USER_DATA_DIR`      | `/data`          | Root for all persisted state.                            |
| `LISTEN_HTTPS`       | `:443`           | Public TLS frontend.                                     |
| `LISTEN_HTTP`        | `:80`            | ACME + redirect-to-https.                                |
| `LISTEN_CONTROL`     | `:7443`          | Control-plane WSS for the CLI.                           |
| `TCP_POOL_MIN`       | `10000`          | First port in the raw-TCP slot pool.                     |
| `TCP_POOL_MAX`       | `19999`          | Last port in the raw-TCP slot pool (inclusive).          |
| `TLS_DOMAIN`         | —                | Wildcard apex. Required.                                 |
| `ACME_EMAIL`         | —                | Required.                                                |
| `ACME_DNS_PROVIDER`  | `cloudflare`     | certmagic DNS-01 provider.                               |
| `ACME_DNS_TOKEN`     | —                | API token for the DNS provider.                          |
| `AUTH_MODE`          | `anonymous`      | `anonymous` or `token`.                                  |

## Dev mode (no TLS, no DNS)

Set `--listen-http :8080` and use `*.localtest.me` (which resolves `*.localtest.me` → 127.0.0.1) as the wildcard. No certmagic needed:

```bash
go run ./src/agent-tunnel-server \
  --listen-http :8080 \
  --control :7080 \
  --user-data-dir ./app/user-data \
  --no-tls
```

Then `curl -H 'Host: ws-relay--agentic.localtest.me' http://localhost:8080/` (or just hit `http://ws-relay--agentic.localtest.me:8080/` directly — `localtest.me` resolves to 127.0.0.1).
