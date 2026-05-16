# 0003 — Control plane: WebSocket + yamux

- Status: Accepted
- Date: 2026-05-16

## Context

The CLI is behind NAT; the server has a public IP. The CLI needs to:

1. Keep one long-lived connection through which it can receive inbound public requests on demand.
2. Multiplex many concurrent inbound streams over that connection.
3. Survive NAT/firewall traversal (most networks allow outbound 443).

## Decision

- Use WebSocket over TLS (`wss://`) as the carrier — outbound, port 443, indistinguishable from a normal browser/web app connection from the network's point of view.
- Use [hashicorp/yamux](https://github.com/hashicorp/yamux) on top of the WebSocket for cheap stream multiplexing with built-in flow control.
- Reserve `stream 0` for control-plane JSON frames (`register`, `register_ack`, `event`, `error`, `ping`, `pong`). All other streams carry proxied bytes.

## Alternatives considered

- **HTTP/2 server-push** — only ever push from server-to-client, can't initiate streams back; needs a separate channel anyway.
- **gRPC bidirectional streams** — usable, but adds heavyweight tooling for what is essentially "pipe these bytes".
- **Raw TCP + TLS** — possible, but WebSocket re-uses an HTTPS endpoint and doesn't need a separate firewall hole.
- **QUIC** — interesting future direction, but Go's HTTP/3 stack is still maturing and Hetzner edge networks vary in UDP handling.

## Consequences

- `+` Single port for both control and data plane.
- `+` `yamux` is battle-tested (used by HashiCorp Consul/Nomad) and the API is dead simple.
- `+` WebSocket framing handles the keepalive (ping/pong) at the layer below the protocol's own ping/pong.
- `−` Per-stream latency adds the WS frame overhead vs raw TCP; negligible for the use case.
