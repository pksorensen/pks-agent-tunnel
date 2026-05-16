# Protocol

The CLI keeps one long-lived WebSocket open to the server at `wss://<server>/v1/control`. Inside that WS, we run a [yamux](https://github.com/hashicorp/yamux) session to get cheap multiplexed streams without re-implementing flow control.

## Stream 0 — control channel

Carries newline-delimited JSON frames. All frames have a `type`.

### `register` — CLI → server

```json
{
  "type": "register",
  "tunnel": "agentic-tunnel",
  "owner": "agentics",
  "slots": [
    { "name": "ws-relay",            "kind": "http", "upstreamHint": "ws-relay:3000",            "anonymous": true },
    { "name": "plugin-marketplace",  "kind": "http", "upstreamHint": "plugin-marketplace:40145", "anonymous": true }
  ],
  "token": "tk_..."
}
```

### `register_ack` — server → CLI

```json
{
  "type": "register_ack",
  "tunnel": "agentic-tunnel",
  "slots": [
    { "name": "ws-relay",           "kind": "http", "url": "https://ws-relay--agentic-tunnel.tunnels.agentics.dk" },
    { "name": "plugin-marketplace", "kind": "http", "url": "https://plugin-marketplace--agentic-tunnel.tunnels.agentics.dk" }
  ]
}
```

### `error` — server → CLI

```json
{ "type": "error", "code": "auth_required", "message": "...", "slot": "..." }
```

### `ping` / `pong`

5-second keepalive in both directions.

## Other streams — proxied bytes

For each inbound public connection, the server opens a new yamux stream on the CLI's session. Before any bytes flow, the **server** writes a single line of JSON metadata terminated by `\n`:

```
{"slot":"ws-relay","remote":"203.0.113.7:54321","tls":false}\n
```

After that header, the stream is raw bytes in both directions. For HTTP slots the bytes are the already-TLS-terminated HTTP/1.1 / HTTP/2 wire content; the CLI does not re-terminate TLS.

## Subdomain → slot resolution

Server parses `Host: <slot>--<tunnel>.<TLS_DOMAIN>` (case-insensitive). The `--` separator is reserved; slot and tunnel names match `^[a-z0-9][a-z0-9-]*$`.

## TCP slots

For `kind: "tcp"` slots, no subdomain. The server allocates a public port from the pool (10000–19999 default) and returns it in `register_ack`:

```json
{ "name": "ssh", "kind": "tcp", "publicPort": 10042 }
```

A sticky preference is recorded in the slot sidecar — the same slot prefers the same public port across restarts.

## TUNNEL_READY (CLI stdout)

After a successful `register_ack`, the CLI prints a single line to stdout that the Aspire extension parses:

```
TUNNEL_READY {"slots":[{"name":"ws-relay","kind":"http","url":"https://..."},{"name":"plugin-marketplace","kind":"http","url":"https://..."}]}
```

Subsequent reconnects emit `TUNNEL_RECONNECTED` with the same payload; failures emit `TUNNEL_ERROR`.
