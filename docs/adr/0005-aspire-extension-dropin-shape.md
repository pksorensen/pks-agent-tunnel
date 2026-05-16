# 0005 — Aspire extension is a drop-in for Aspire.Hosting.DevTunnels

- Status: Accepted
- Date: 2026-05-16

## Context

The motivation for `pks-agent-tunnel` is to replace `Aspire.Hosting.DevTunnels` in agentic-live-www. That codebase has a non-trivial integration spanning `AppHost.cs:507–528` (and `DevTunnelCleanupExtensions.cs` — which goes away). We want flipping over to be a one-line change.

## Decision

`Aspire.Hosting.AgentTunnel` mirrors the public surface of `Aspire.Hosting.DevTunnels`:

| Method                                                                                  | Behaviour                                                                |
|-----------------------------------------------------------------------------------------|--------------------------------------------------------------------------|
| `IDistributedApplicationBuilder.AddDevTunnel(string name)`                              | Registers an `AgentTunnelResource`.                                      |
| `IDistributedApplicationBuilder.AddDevTunnel(string name, string? existingId)`          | Same; `existingId` is accepted-and-ignored (registry is keyed by name).  |
| `IResourceBuilder<DevTunnelResource>.WithReference(IResourceBuilder<IResourceWithEndpoints>)` | Registers an HTTP slot for that resource's `http` endpoint.        |
| `IResourceBuilder<DevTunnelResource>.WithAnonymousAccess()`                             | Flag; persisted in `tunnel.md` frontmatter.                              |
| `tunnel.GetEndpoint(resource, "http")` → `ReferenceExpression`                          | Resolves to the slot's allocated public URL.                             |

The exported type `DevTunnelResource` is preserved — implemented as a type alias for `AgentTunnelResource` — so existing user code that names the type by hand keeps compiling.

Methods that exist only as workarounds for devtunnels bugs are **not** ported:

- `WithDevTunnelCleanup()` — no scorched-earth cleanup is needed; we don't have a split-brain to defend against.
- `WithTunnelHealing(...)` — no per-port ACE race; we never had one.

## Consequences

- `+` Flipping from devtunnels to agent-tunnel is `using Aspire.Hosting.DevTunnels;` → `using Aspire.Hosting.AgentTunnel;` plus deleting the two workaround calls.
- `+` We can swap underlying mechanism (e.g. add gRPC later) without breaking the user surface.
- `−` Method names lie a little: `AddDevTunnel` does not add a Microsoft devtunnel. Trade-off for the drop-in. (We expose `AddAgentTunnel` as an alias for users who'd rather use the accurate name.)
