---
title: "Quickstart"
description: "Install agent-tunnel and open your first public tunnel in two commands."
tags: [cli, install, quickstart]
category: agents
status: beta
type: cli
---

# Quickstart

## Install

**Linux / macOS**

```bash
curl -fsSL https://agentics.dk/install/agent-tunnel.sh | bash
```

**Windows (PowerShell)**

```powershell
irm https://agentics.dk/install/agent-tunnel.ps1 | iex
```

The script detects your OS/architecture, verifies the sha256 checksum, installs
the `agent-tunnel` binary to `~/.local/bin` (or `%LOCALAPPDATA%\Agentics\bin` on
Windows), and ensures it is on your `PATH`.

Pin a version or customize the install:

```bash
curl -fsSL https://agentics.dk/install/agent-tunnel.sh | VERSION=0.5.0 bash
curl -fsSL https://agentics.dk/install/agent-tunnel.sh | INSTALL_DIR=~/bin NO_MODIFY_PATH=1 bash
```

Verify:

```bash
agent-tunnel version
```

## Open your first tunnel

Suppose a local web server is running on port `3000`. Expose it publicly:

```bash
agent-tunnel host \
  --server wss://tunnels.agentics.dk:17443 \
  --owner agentics \
  --name demo \
  --http app=127.0.0.1:3000
```

The client prints a ready line listing each slot's public URL:

```
TUNNEL_READY {"slots":[{"name":"app","kind":"http","url":"https://app--demo.tunnels.agentics.dk:8443"}]}
```

Open `https://app--demo.tunnels.agentics.dk:8443` — requests are forwarded to
`127.0.0.1:3000` on your machine. Press `Ctrl-C` to close the tunnel.

## Multiple slots

One process can expose several upstreams — repeat `--http`:

```bash
agent-tunnel host \
  --server wss://tunnels.agentics.dk:17443 \
  --owner agentics --name demo \
  --http web=127.0.0.1:3000 \
  --http api=127.0.0.1:8080
# → https://web--demo.tunnels.agentics.dk:8443
# → https://api--demo.tunnels.agentics.dk:8443
```

## Next steps

- [Concepts](/tools/agent-tunnel/concepts) — slots, naming, and the public URL formula.
- [`host` command](/tools/agent-tunnel/host) — every flag, including the published-port gotcha.
