# Aspire.Hosting.AgentTunnel

Aspire hosting integration for [pks-agent-tunnel](https://github.com/pksorensen/pks-agent-tunnel). A drop-in replacement for `Aspire.Hosting.DevTunnels` — same public surface, no Azure dependency, predictable URLs.

## Install

```xml
<PackageReference Include="Aspire.Hosting.AgentTunnel" Version="0.0.*" />
```

## Drop-in

```diff
- using Aspire.Hosting.DevTunnels;
+ using Aspire.Hosting.AgentTunnel;

  var tunnel = builder.AddDevTunnel("agentic-tunnel")
      .WithReference(wsRelay)
      .WithReference(pluginMarketplace)
      .WithAnonymousAccess();

  nextjs.WithEnvironment("WS_RELAY_PUBLIC_URL", tunnel.GetEndpoint(wsRelay, "http"));
```

The type `DevTunnelResource` is preserved (as an alias for `AgentTunnelResource`) so code that names the type by hand keeps compiling.

Workaround helpers from the original — `WithDevTunnelCleanup()`, `WithTunnelHealing(...)` — are **not** exposed: pks-agent-tunnel has no Azure split-brain to defend against.

## Configuration

The extension spawns the `agent-tunnel host` CLI as a child process and parses its `TUNNEL_READY` stdout line. Locate the binary via:

1. `PKS_AGENT_TUNNEL_BIN` env var (absolute path).
2. A `tools/` folder co-located with the package (NuGet pack ships the binary there).
3. `PATH` lookup.

Set `AgentTunnelResource.ServerUrl` to point at the right server:

```csharp
var tunnel = builder.AddAgentTunnel("agentic-tunnel");
tunnel.Resource.ServerUrl = "wss://tunnels.agentics.dk:7443";
```

## Server

Run the [pks-agent-tunnel-server](../../agent-tunnel-server/) — locally or as a Docker image at `registry.kjeldager.io/agent-tunnel-server:latest`.

## Status

v0.1 — single-tunnel, HTTP slots only, anonymous-only access. TCP slots and token auth ship in v0.3.
