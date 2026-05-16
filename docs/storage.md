# Storage

The server is **database-free**. All state lives under `$USER_DATA_DIR`:

| Environment    | Default        |
|----------------|----------------|
| Local dev      | `./app/user-data` |
| Docker image   | `/data`        |

Same convention as `pks-agent-ftp` and `pks-agent-inbox`. See ADR 0006 for the reasoning.

## Layout

```
$USER_DATA_DIR/
├── tunnels/
│   └── <owner>/<tunnel-name>/
│       ├── tunnel.md
│       └── slots/
│           └── <slot-name>.md
├── tokens/
│   └── <token-id>.md
├── ports/
│   └── allocated.md
├── tls/                       # certmagic state (issued certs, account keys, locks)
└── runtime/
    ├── server.pid
    └── server.lock
```

`tar czf` of this directory is a complete backup of server state.

## Sidecar format

Every entity is a Markdown file whose **frontmatter** is the structured data. Body is optional, intended for human notes:

```markdown
---
name: ws-relay
kind: http
ownerTunnel: agentics/agentic-tunnel
subdomain: ws-relay--agentic-tunnel
anonymous: true
upstreamHint: ws-relay:3000
stickyPort: null
lastSeenPublicUrl: https://ws-relay--agentic-tunnel.tunnels.agentics.dk
createdAt: 2026-05-16T11:30:00Z
---

Optional human notes go here.
```

## Schemas

### `tunnels/<owner>/<tunnel>/tunnel.md`

| Field          | Type     | Notes                                                       |
|----------------|----------|-------------------------------------------------------------|
| `name`         | string   | Tunnel name (matches folder).                               |
| `owner`        | string   | Owner identifier (matches parent folder).                   |
| `anonymous`    | bool     | If true, slots default to anonymous access.                 |
| `createdAt`    | RFC3339  | First-seen timestamp.                                       |
| `description`  | string?  | Free text.                                                  |

### `tunnels/<owner>/<tunnel>/slots/<slot>.md`

| Field              | Type    | Notes                                                        |
|--------------------|---------|--------------------------------------------------------------|
| `name`             | string  | Slot name (matches file basename).                           |
| `kind`             | enum    | `http` or `tcp`.                                             |
| `ownerTunnel`      | string  | `<owner>/<tunnel>` for cross-reference.                      |
| `subdomain`        | string  | For HTTP: `<slot>--<tunnel>`. Empty for TCP.                 |
| `stickyPort`       | int?    | For TCP: remembered port from previous run.                  |
| `upstreamHint`     | string  | Optional, server-side hint of what the CLI forwards to (display only — server never connects to upstream directly). |
| `anonymous`        | bool    | Per-slot override of tunnel-level anonymous flag.            |
| `lastSeenPublicUrl`| string  | Last URL emitted to the CLI.                                 |
| `createdAt`        | RFC3339 |                                                              |

### `tokens/<token-id>.md`

| Field          | Type       | Notes                                                       |
|----------------|------------|-------------------------------------------------------------|
| `id`           | string     | Matches filename.                                           |
| `scope`        | enum       | `owner:<name>` or `tunnel:<owner>/<name>`.                  |
| `sha256`       | hex string | sha256 of the secret. Plaintext never stored.               |
| `createdAt`    | RFC3339    |                                                              |
| `expiresAt`    | RFC3339?   |                                                              |

### `ports/allocated.md`

Single file holding the TCP port pool state, mutated under an advisory file lock. Frontmatter:

```yaml
pool:
  min: 10000
  max: 19999
allocations:
  10000: agentics/agentic-tunnel/ssh
  10001: agentics/agentic-tunnel/db
```

## Atomic writes

Every mutation writes `<file>.tmp`, `fsync`s, then `rename`s over the destination. This is crash-safe on every common filesystem.

## Concurrency

Per-instance, all mutations happen on the single control-plane goroutine, so no in-process locking is needed beyond `runtime/server.lock` (a file lock that prevents two server processes sharing `$USER_DATA_DIR`).
