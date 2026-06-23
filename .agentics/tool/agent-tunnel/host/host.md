---
title: "host command"
description: "Reference for `agent-tunnel host` — flags, the published-port gotcha, and output."
tags: [reference, cli, host]
category: agents
status: beta
type: cli
---

# `agent-tunnel host`

Connect to a tunnel server, register one or more slots, and proxy public traffic
to local upstreams. Runs in the foreground until interrupted (`Ctrl-C`).

```bash
agent-tunnel host \
  --server wss://tunnels.agentics.dk:17443 \
  --owner agentics \
  --name demo \
  --http app=127.0.0.1:3000
```

## Flags

| Flag | Default | Description |
|---|---|---|
| `--server` | `ws://localhost:7080` | Control-plane URL, `ws://` or `wss://`. Use the **published** host port (see below). |
| `--owner` | `agentics` | Owner identifier. `[a-z0-9][a-z0-9-]*`. |
| `--name` | `agentic` | Tunnel name. `[a-z0-9][a-z0-9-]*`. |
| `--http` | — | HTTP slot, repeatable: `--http <slot>=<host:port>`. At least one is required. |
| `--token` | (empty) | Bearer token, for servers running with token auth. Optional on anonymous (v0.1) servers. |

## The published-port gotcha

The server's container-internal control port (`:7443`) is usually **not** the
port you connect to. On a deployment where a reverse proxy already owns `:443`/
`:7443`, the control plane is republished on a different host port. On the live
`tunnels.agentics.dk` server:

| Role | Container port | Connect to |
|---|---|---|
| Control plane (WSS) | `:7443` | `wss://tunnels.agentics.dk:17443` |
| Public TLS frontend | `:443` | `https://<slot>--<tunnel>.tunnels.agentics.dk:8443` |

So the working invocation against the live server uses `--server
wss://tunnels.agentics.dk:17443`, and the resulting public URLs are served on
`:8443`. Connecting to `:7443` on the public host will **not** reach the control
plane.

## Output

On success the client emits a single `TUNNEL_READY` line to stdout with the
resolved public URL of each slot (see
[Concepts](/tools/agent-tunnel/concepts#the-tunnel_ready-line)), then logs
forwarded traffic to stderr until stopped.

## Exit behavior

`host` runs until it receives `SIGINT`/`SIGTERM` (e.g. `Ctrl-C`), then closes the
tunnel cleanly. A non-zero exit indicates a registration or connection error
(printed to stderr as `host: <error>`).
