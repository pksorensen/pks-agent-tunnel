# 0004 — Slot types: http and tcp

- Status: Accepted
- Date: 2026-05-16

## Context

The original Aspire-driven use case (ws-relay, plugin-marketplace) is HTTP + WebSocket. But limiting v1 to HTTP would block obvious follow-ups (SSH, Postgres, MQTT). The user explicitly wants "arbitrary TCP" in v1.

## Decision

Each slot has a `kind` of either `http` or `tcp`:

- `http` slots are routed by `Host` header on the wildcard frontend. Server terminates TLS, then ships plain HTTP/WS bytes to the CLI over a yamux stream.
- `tcp` slots get a public port allocated from `TCP_POOL_MIN..TCP_POOL_MAX` (default `10000..19999`). Inbound TCP connections on that port get forwarded byte-for-byte (still over the WS+yamux control session — the *carrier* is unchanged, only the *resolution* differs).

## Sticky ports

For `tcp` slots, the slot sidecar persists `stickyPort`. On register, the server prefers that port if still free; if taken, it picks the lowest free one and updates the sidecar. This gives the same TCP stability HTTP gets for free via subdomain.

## Consequences

- `+` Generalises beyond HTTP without changing the carrier protocol.
- `+` Sticky ports give the same restart-stability story TCP needs.
- `−` Operators must open the entire pool range on the firewall, not just :443. Documented in `deployment.md`.
- `−` Per-slot public ports leak slot count to anyone scanning the box; trade-off accepted for v1.
