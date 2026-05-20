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

## VPS installation guide (v0.1 — plain HTTP, no TLS)

v0.1 speaks plain HTTP. Stand it up on a Hetzner box with two `docker run` lines and you're done — same operational pattern as `pks-agent-ftp` / `pks-agent-inbox`. v0.2 adds native ACME + wildcard TLS so the URLs lose the port suffix; until then, the tunnel works for everything that's happy with `http://` and `ws://` (curl, the vibecast Go CLI, scripts, the Aspire integration).

The image lives at `registry.kjeldager.io/agent-tunnel-server:latest` — built and pushed by the PKS self-hosted runner via a unix-socket credential helper, so no GitHub secrets are needed.

### Prerequisites

- A VPS with a public IPv4 (Hetzner CX22 is plenty).
- A domain whose DNS you control (e.g. `tunnels.example.com`).
- Docker installed on the VPS (no Compose required).
- A registry pull token for `registry.kjeldager.io` (already in place if you also run other `pks-agent-*` services).

### Step 1 — DNS

Point a wildcard and apex at the VPS:

```
A   *.tunnels.example.com   → <vps-ip>
A   tunnels.example.com     → <vps-ip>
```

### Step 2 — Firewall

Pick the host ports you want to expose. The image listens on `:8080` (public HTTP) and `:7080` (control plane) inside the container — host-map them to whatever's free. The worked example below uses `18080`/`17080`:

```bash
sudo ufw allow 22/tcp
sudo ufw allow 18080/tcp     # public HTTP frontend
sudo ufw allow 17080/tcp     # control plane (plain WS for the CLI)
sudo ufw enable
```

If `:8080`/`:7080` are free on the host, just map straight (`-p 8080:8080 -p 7080:7080`) and skip the `PUBLIC_HTTP_PORT` env var below.

### Step 3 — Run the server

```bash
docker pull registry.kjeldager.io/agent-tunnel-server:latest

docker stop agent-tunnel 2>/dev/null; docker rm agent-tunnel 2>/dev/null

docker run -d \
  --name agent-tunnel \
  --restart unless-stopped \
  -p 18080:8080 \
  -p 17080:7080 \
  -v agent-tunnel-data:/data \
  -e TLS_DOMAIN=tunnels.example.com \
  -e PUBLIC_HTTP_PORT=18080 \
  registry.kjeldager.io/agent-tunnel-server:latest
```

What the env vars do (both optional, both purely cosmetic — they affect the URL the CLI prints, not the routing):

- `TLS_DOMAIN` — wildcard apex baked into emitted URLs. Misnamed for v0.1 (we'll rename it `PUBLIC_DOMAIN` alongside the v0.2 ACME work); for now it sets the host part of every `lastSeenPublicUrl`.
- `PUBLIC_HTTP_PORT` — overrides the port shown in emitted URLs. Set this when the host-bound port differs from the container-internal `:8080`.

### Step 4 — Smoke-test

From any machine:

```bash
# 1. Control plane healthcheck
curl http://tunnels.example.com:17080/healthz
# expect: ok

# 2. A non-existent slot returns 502 — that's correct (frontend works,
# subdomain parsed, no client bound for that slot)
curl -i http://nothing--agentic.tunnels.example.com:18080/

# 3. Connect a CLI from a separate machine and tunnel a local upstream
python3 -m http.server 9000 &
agent-tunnel host \
  --server ws://tunnels.example.com:17080 \
  --owner agentics --name agentic \
  --http demo=127.0.0.1:9000

# 4. Hit the public URL — body is whatever the upstream served
curl http://demo--agentic.tunnels.example.com:18080/
```

### Step 5 — Updates

```bash
docker pull registry.kjeldager.io/agent-tunnel-server:latest
docker stop agent-tunnel && docker rm agent-tunnel
# re-run the docker run from Step 3
```

### Backups

State is one folder: the `agent-tunnel-data` named volume, mounted at `/data` in the container. Full backup:

```bash
docker run --rm -v agent-tunnel-data:/data alpine tar czf - /data > backup.tgz
```

No database, no migrations — see [docs/storage.md](docs/storage.md) and [ADR 0006](docs/adr/0006-folder-based-storage-no-database.md).

See [docs/deployment.md](docs/deployment.md) for the full env-var matrix and the v0.2 TLS plan.

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
