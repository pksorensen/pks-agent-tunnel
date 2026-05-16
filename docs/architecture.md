# Architecture

```
                    ┌─────────────────────────────────┐
                    │  Public Internet                │
                    │  Browser / runner container     │
                    └────────────┬────────────────────┘
                                 │ https://<slot>--<tunnel>.tunnels.example.com
                                 ▼
            ┌────────────────────────────────────────────────────┐
            │  agent-tunnel-server  (VPS, wildcard TLS)          │
            │   :443  HTTPS frontend  :7443  control WSS         │
            │   :80   ACME / redirect :T_n   raw TCP per slot    │
            │   Folder-backed registry under $USER_DATA_DIR      │
            └────────────────────────┬───────────────────────────┘
                                     │ wss://.../v1/control
                                     │ yamux-multiplexed streams
                                     ▼
            ┌────────────────────────────────────────────────────┐
            │  agent-tunnel  (Go CLI in dev container)            │
            │   reads tunnel.yml, opens control WS, registers     │
            │   slots, forwards inbound streams → local ports     │
            └────────────────────────┬───────────────────────────┘
                                     │ 127.0.0.1:<port>
                                     ▼
                          ┌──────────────────────┐
                          │  Local app process    │
                          └──────────────────────┘
```

## Components

### Server (`src/agent-tunnel-server`)

Listens on three planes:

| Port  | Purpose                                                                |
|-------|------------------------------------------------------------------------|
| 443   | Public HTTPS/WSS. Server-terminated TLS using the wildcard cert.       |
| 80    | ACME HTTP-01 + 301 redirect to https.                                  |
| 7443  | Control plane (WSS). CLI keeps a long-lived connection here.           |
| 10000–19999 | Pool for raw-TCP slots; one port per registered TCP slot.        |

Resolves the slot from the request `Host` (HTTP) or the listening port (TCP), opens a yamux stream on the owning CLI's control session, and pipes bytes.

### CLI (`src/agent-tunnel`)

Opens a WSS connection to the server, sends a `register` frame per slot, then accepts yamux streams the server pushes for inbound connections — each gets piped into the configured local upstream (`127.0.0.1:<port>` or a unix socket).

### Aspire extension (`src/aspire/Aspire.Hosting.AgentTunnel`)

Spawns the CLI as a child process inside the Aspire AppHost, parses its `TUNNEL_READY` line to learn the allocated public URLs, and surfaces them via `EndpointAnnotation` so they resolve through the normal `GetEndpoint(resource, "http")` API.

## Subdomain scheme

For HTTP slots: `<slot-name>--<tunnel-name>.<TLS_DOMAIN>`. The same `(owner, tunnel, slot)` triple deterministically maps to the same subdomain — no `-{hash}` churn between restarts. See ADR 0002.

## State

No database. Everything persists as YAML-frontmatter `.md` sidecars under `$USER_DATA_DIR`. See [storage.md](storage.md) and ADR 0006.
