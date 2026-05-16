# pks-agent-tunnel

A self-hosted devtunnel replacement. Exposes local TCP/HTTP/WebSocket services on the public internet over a single persistent control connection from a Go CLI to a Go server. Includes an Aspire hosting extension that is a drop-in replacement for `Aspire.Hosting.DevTunnels`.

> **Why this exists**: months of pain with Microsoft's `devtunnel.ms` integration in Aspire — Azure split-brain across regions, per-port ACE race in Aspire 13.3, opaque CLI 404s. `pks-agent-tunnel` is one server we control, with stable public URLs and zero "scorched-earth cleanup" required between runs.

## Components

| Path                                          | Purpose                                                          |
|-----------------------------------------------|------------------------------------------------------------------|
| `src/protocol/`                               | Shared Go types for control frames + mux stream headers.         |
| `src/agent-tunnel-server/`                    | The server. Public TLS frontend, WSS control plane, TCP pool. Deploys as Docker image. |
| `src/agent-tunnel/`                           | The CLI. Connects to the server and forwards public traffic to local upstreams. |
| `src/aspire/Aspire.Hosting.AgentTunnel/`      | Aspire hosting extension. Drop-in replacement for `Aspire.Hosting.DevTunnels`. |

## Storage Model

The server is **database-free**. All state lives under `$USER_DATA_DIR` (default `./app/user-data` for dev, `/data` in the Docker image) as YAML-frontmatter `.md` sidecars — same convention as `pks-agent-ftp` and `pks-agent-inbox`. See [docs/storage.md](docs/storage.md).

```
$USER_DATA_DIR/
├── tunnels/<owner>/<tunnel>/tunnel.md
├── tunnels/<owner>/<tunnel>/slots/<slot>.md
├── tokens/<token-id>.md
├── ports/allocated.md
├── tls/                       # certmagic state
└── runtime/server.pid, server.lock
```

`tar czf` of the directory is a complete backup.

## Quick Start (local dev, no TLS)

```bash
# Terminal 1: server (binds :8080 plain HTTP, no auth, in-process state)
go run ./src/agent-tunnel-server \
  --listen :8080 \
  --control :7080 \
  --user-data-dir ./app/user-data

# Terminal 2: an upstream to expose
python3 -m http.server 9000

# Terminal 3: CLI registers an http slot pointing at :9000
go run ./src/agent-tunnel host \
  --server ws://localhost:7080 \
  --name agentic \
  --http ws-relay=127.0.0.1:9000

# Terminal 4: curl through the tunnel
curl -H 'Host: ws-relay--agentic.localtest.me' http://localhost:8080/
```

## VPS installation guide (v0.1 — Caddy front for TLS)

v0.1 ships **without native ACME** — front the server with [Caddy](https://caddyserver.com) for wildcard TLS termination. v0.2 will add native ACME and the Caddy hop drops out.

The image lives at `ghcr.io/pksorensen/agent-tunnel-server:latest` (built by the [release workflow](.github/workflows/release.yml) on every release-please tag). The package is public — no `docker login` needed to pull. To also push to `registry.kjeldager.io`, set the repo variable `PUSH_TO_KJELDAGER_REGISTRY=true` and add the `REGISTRY_KJELDAGER_USERNAME` / `REGISTRY_KJELDAGER_PASSWORD` secrets; the release workflow will then push to both registries.

### Prerequisites

- A VPS with a public IPv4 (a Hetzner CX22 is plenty).
- A domain whose DNS you control (e.g. `tunnels.example.com`).
- The DNS provider's API token for the [Caddy DNS plugin](https://caddyserver.com/docs/automatic-https#dns-challenge) — used for wildcard DNS-01 issuance. Cloudflare is the worked example below.
- Docker + Docker Compose installed on the VPS.
- A registry pull token for `registry.kjeldager.io` (already in place if you also run other `pks-agent-*` services).

### Step 1 — DNS

Point a wildcard and apex at the VPS:

```
A   *.tunnels.example.com   → <vps-ip>
A   tunnels.example.com     → <vps-ip>
```

### Step 2 — Firewall

```bash
sudo ufw allow 22/tcp
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw allow 7443/tcp                # control plane (WSS for the CLI)
sudo ufw allow 10000:19999/tcp         # TCP slot pool — open in v0.3
sudo ufw enable
```

### Step 3 — Compose file

`/srv/agent-tunnel/docker-compose.yml`:

```yaml
services:
  agent-tunnel:
    image: ghcr.io/pksorensen/agent-tunnel-server:latest
    container_name: agent-tunnel
    restart: unless-stopped
    expose: ["8080", "7080"]            # plain HTTP behind Caddy
    environment:
      USER_DATA_DIR: /data
      LISTEN_HTTP: ":8080"
      LISTEN_CONTROL: ":7080"
      AUTH_MODE: anonymous              # switch to "token" in v0.3
    volumes:
      - agent-tunnel-data:/data
    networks: [edge]

  caddy:
    image: caddy:2
    container_name: agent-tunnel-caddy
    restart: unless-stopped
    ports: ["80:80", "443:443", "7443:7443"]
    environment:
      CLOUDFLARE_API_TOKEN: ${CLOUDFLARE_API_TOKEN}
      TLS_DOMAIN: tunnels.example.com
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile:ro
      - caddy-data:/data
      - caddy-config:/config
    networks: [edge]
    depends_on: [agent-tunnel]

networks:
  edge:

volumes:
  agent-tunnel-data:
  caddy-data:
  caddy-config:
```

> **Cloudflare plugin**: `caddy:2` does not include the Cloudflare DNS plugin by default. Either swap the image for `slothcroissant/caddy-cloudflaredns:latest` (drop-in, includes the plugin) or build your own via `xcaddy build --with github.com/caddy-dns/cloudflare`.

`/srv/agent-tunnel/Caddyfile`:

```caddyfile
{
    email ops@example.com
}

# Public wildcard frontend — TLS terminated here, plain HTTP forwarded to the server.
*.tunnels.example.com, tunnels.example.com {
    tls {
        dns cloudflare {env.CLOUDFLARE_API_TOKEN}
    }
    reverse_proxy agent-tunnel:8080
}

# Control plane on :7443 — wss:// for the CLI.
:7443 {
    tls {
        dns cloudflare {env.CLOUDFLARE_API_TOKEN}
    }
    reverse_proxy agent-tunnel:7080
}
```

### Step 4 — Bring it up

```bash
cd /srv/agent-tunnel
echo 'CLOUDFLARE_API_TOKEN=<your-token>' > .env

docker login registry.kjeldager.io     # if not already logged in
docker compose pull
docker compose up -d
docker compose logs -f --tail=50
```

### Step 5 — Smoke-test

From any machine:

```bash
# 1. Control plane healthcheck (should reply "ok" with a valid cert)
curl https://tunnels.example.com:7443/healthz

# 2. A non-existent slot returns 404 — that's correct (frontend works)
curl -i https://nothing--agentic.tunnels.example.com/

# 3. Connect a CLI from a separate machine and tunnel a local upstream
python3 -m http.server 9000 &
agent-tunnel host \
  --server wss://tunnels.example.com:7443 \
  --owner agentics --name agentic \
  --http demo=127.0.0.1:9000

# 4. From a third machine, hit the public URL
curl https://demo--agentic.tunnels.example.com/
# expect: the http.server directory listing
```

### Step 6 — Updates

```bash
docker compose pull
docker compose up -d
```

### Backups

State is one folder: `agent-tunnel-data` (mounted at `/data` in the container). `docker run --rm -v agent-tunnel-data:/data alpine tar czf - /data > backup.tgz` is a complete backup. No database, no migrations — see [docs/storage.md](docs/storage.md) and [ADR 0006](docs/adr/0006-folder-based-storage-no-database.md).

See [docs/deployment.md](docs/deployment.md) for the full env-var matrix and a Coolify deployment recipe.

## Aspire Drop-in

```diff
- using Aspire.Hosting.DevTunnels;
+ using Aspire.Hosting.AgentTunnel;

  var tunnel = builder.AddDevTunnel("agentic-tunnel")
      .WithReference(wsRelay)
      .WithReference(pluginMarketplace)
      .WithAnonymousAccess();

  nextjs.WithEnvironment("WS_RELAY_PUBLIC_URL", tunnel.GetEndpoint(wsRelay, "http"));
```

The `AddDevTunnel` / `WithReference` / `WithAnonymousAccess` / `GetEndpoint` surface matches `Aspire.Hosting.DevTunnels` exactly, and the type `DevTunnelResource` is exposed as an alias so existing code keeps compiling. See [src/aspire/Aspire.Hosting.AgentTunnel/README.md](src/aspire/Aspire.Hosting.AgentTunnel/README.md).

## Docs

- [docs/architecture.md](docs/architecture.md) — components and data flow.
- [docs/protocol.md](docs/protocol.md) — wire format (control frames, yamux mux).
- [docs/storage.md](docs/storage.md) — `$USER_DATA_DIR` folder layout + frontmatter schema.
- [docs/deployment.md](docs/deployment.md) — Hetzner + Coolify deploy, DNS, TLS, firewall.
- [docs/adr/](docs/adr) — architecture decisions (0001+).
