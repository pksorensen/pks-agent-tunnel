---
title: "Concepts"
description: "Slots, owner/tunnel/slot naming, and how the public hostname is derived."
tags: [concepts, naming, slots]
category: agents
status: beta
type: cli
---

# Concepts

## Slots

A **slot** is one forwarding from a public subdomain to a local upstream,
declared with `--http <slot>=<host:port>`. A single `agent-tunnel host` process
can register multiple slots, each getting its own public URL over one shared
connection.

Today the client exposes **HTTP slots**. The slot's `kind` is reported in the
`TUNNEL_READY` line so tooling can distinguish slot types as more land.

## Owner, tunnel, and slot names

Three identifiers combine to address a tunnel:

| Identifier | Flag | Default | Role |
|---|---|---|---|
| owner | `--owner` | `agentics` | Account/namespace the tunnel belongs to. |
| tunnel | `--name` | `agentic` | A named tunnel within the owner. |
| slot | `--http <slot>=…` | — | One upstream within the tunnel. |

All three must match `^[a-z0-9][a-z0-9-]*$` — lowercase letters, digits, and
hyphens, starting with a letter or digit. The sequence `--` is **reserved** (it
separates slot from tunnel in the hostname) and cannot appear in a name.

## Public hostname formula

The server publishes each HTTP slot at:

```
<slot>--<tunnel>.<server-domain>
```

For the live server (`tunnels.agentics.dk`), `--owner agentics --name demo
--http app=…` yields `https://app--demo.tunnels.agentics.dk`. Because the
hostname is derived purely from the names — not a random token — the URL is
**stable across restarts**. Re-running the same command reclaims the same
hostname.

> **v0.1 ownership:** the server currently accepts anonymous registrations and
> does not enforce ownership, so any client can claim a given `(owner, tunnel)`.
> Token-based auth and ownership enforcement arrive in a later release; the
> `--token` flag is already wired for that.

## The TUNNEL_READY line

Once all slots are registered the client writes a single machine-readable line to
stdout:

```
TUNNEL_READY {"slots":[{"name":"app","kind":"http","url":"https://app--demo.tunnels.agentics.dk:8443"}]}
```

Automation (such as the Aspire hosting extension, or a container entrypoint) can
parse this to discover the public URL programmatically instead of hardcoding the
formula.
