# 0006 — Folder-based storage, no database

- Status: Accepted
- Date: 2026-05-16

## Context

The server needs to persist: tunnel definitions, slot configs (kind, subdomain, sticky port), auth tokens (sha256'd), and the TCP port allocator. The "obvious" choice is embedded SQLite; the existing sibling projects `pks-agent-ftp` and `pks-agent-inbox` went a different way.

## Decision

No database. Every entity is a Markdown file whose **YAML frontmatter** is the structured data. Layout:

```
$USER_DATA_DIR/
├── tunnels/<owner>/<tunnel>/tunnel.md
├── tunnels/<owner>/<tunnel>/slots/<slot>.md
├── tokens/<token-id>.md
├── ports/allocated.md
├── tls/                       # certmagic state
└── runtime/server.pid, server.lock
```

Atomic writes via `<file>.tmp` + `fsync` + `rename`. Inter-process safety via a single `runtime/server.lock` advisory file lock — only one server instance per `$USER_DATA_DIR`.

## Alternatives considered

- **SQLite**: powerful, but invites a schema-migration story and a backup story that's heavier than the data deserves. The data is naturally hierarchical and small.
- **BoltDB / Badger**: opaque to `cat`/`grep`, single-file lock model fights with `tar`-style backup.
- **JSON files**: equivalent on disk, but `.md` with frontmatter is what `pks-agent-ftp` / `pks-agent-inbox` already use, and the body is a free spot for human notes.

## Consequences

- `+` `cat $USER_DATA_DIR/tunnels/.../slots/ws-relay.md` is the debug tool.
- `+` `tar czf` is a full backup. No vacuum, no checkpoint, no migration.
- `+` Adding a field = tolerate-missing in old files. No migrations.
- `+` Matches the existing platform pattern (`pks-agent-ftp`, `pks-agent-inbox`).
- `−` We can't do cross-entity transactions. Acceptable: mutations are serialised through the control plane and individually atomic.
- `−` Many small files. Fine until ~10⁵ tunnels — well past anything we'll hit.
