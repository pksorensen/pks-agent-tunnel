---
title: "Agent Tunnel"
description: "Self-hosted tunnel client — expose a local port at a stable public https URL on tunnels.agentics.dk."
tags: [cli, tunnel, networking, devtunnel]
category: agents
status: beta
type: cli
icon: globe
author: Poul Kjeldager
component: agent-tunnel
usage: "agent-tunnel host --server <url> --owner <owner> --name <tunnel> --http <slot>=<host:port>"
examples:
  - command: "agent-tunnel host --server wss://tunnels.agentics.dk:17443 --owner agentics --name demo --http app=127.0.0.1:3000"
    description: "Expose local port 3000 at https://app--demo.tunnels.agentics.dk:8443"
---

# Agent Tunnel

`agent-tunnel` is the client for the self-hosted Agentics tunnel — a drop-in
alternative to dev tunnels. It connects to a tunnel server over a single
WebSocket, registers one or more **slots**, and proxies inbound public traffic
to local upstreams. A running tunnel gives a local service a **stable public
HTTPS URL**, so you can share a dev server, test a webhook, or let a teammate hit
something running on your machine.

The public hostname is derived deterministically from the owner + tunnel + slot
names, so the URL is the same every run — no random subdomain churn between
restarts.

## Quick start

Install the CLI, then open a tunnel to a local port:

```bash
curl -fsSL https://agentics.dk/install/agent-tunnel.sh | bash

agent-tunnel host \
  --server wss://tunnels.agentics.dk:17443 \
  --owner agentics \
  --name demo \
  --http app=127.0.0.1:3000
# → https://app--demo.tunnels.agentics.dk:8443
```

See [Quickstart](/tools/agent-tunnel/quickstart) for install options and a full
first-run walkthrough.

## How it works

- You run `agent-tunnel host` and declare HTTP **slots** (`--http <slot>=<host:port>`).
- The client dials the server's control plane (WSS) and registers your
  `(owner, tunnel, slot)` identifiers.
- The server publishes each slot at `https://<slot>--<tunnel>.<server-domain>`
  and forwards public requests back through the tunnel to your local upstream.
- The connection is multiplexed, so one process can expose several slots at once.

Read [Concepts](/tools/agent-tunnel/concepts) for slots, naming, and the public
URL formula, and the [`host` command](/tools/agent-tunnel/host) for the complete
flag reference.

## Common uses

- Expose a local web app or API at a shareable HTTPS URL.
- Receive webhooks on a service running on your laptop or in a container.
- Give a container (e.g. a packaged app image) a public endpoint for testing,
  by running the client as a sidecar process.

> **Status:** beta. v0.1 servers accept anonymous registrations; token auth and
> ownership enforcement arrive in a later release.
