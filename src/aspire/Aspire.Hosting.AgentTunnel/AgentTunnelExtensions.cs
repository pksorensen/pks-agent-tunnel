using System.Diagnostics;
using System.Text.Json;
using Aspire.Hosting.ApplicationModel;
using Aspire.Hosting.Eventing;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Logging;

namespace Aspire.Hosting.AgentTunnel;

/// <summary>
/// Public surface for the pks-agent-tunnel Aspire integration. Method names
/// mirror <c>Aspire.Hosting.DevTunnels</c> so flipping is a one-line <c>using</c>
/// change.
/// </summary>
public static class AgentTunnelExtensions
{
    /// <summary>
    /// Adds a tunnel resource. Method name mirrors <c>Aspire.Hosting.DevTunnels.AddDevTunnel</c>;
    /// see <see cref="AddAgentTunnel"/> for the accurately-named alias.
    /// </summary>
    public static IResourceBuilder<DevTunnelResource> AddDevTunnel(
        this IDistributedApplicationBuilder builder,
        string name)
        => builder.AddDevTunnel(name, existingId: null);

    /// <summary>
    /// Drop-in overload matching <c>AddDevTunnel(name, existingId)</c> from
    /// Aspire.Hosting.DevTunnels. <paramref name="existingId"/> is accepted-and-ignored:
    /// pks-agent-tunnel keys its registry by <c>(owner, tunnel name)</c>, not by
    /// an opaque cloud-side id, so there is nothing to reuse.
    /// </summary>
    public static IResourceBuilder<DevTunnelResource> AddDevTunnel(
        this IDistributedApplicationBuilder builder,
        string name,
        string? existingId)
    {
        _ = existingId; // accepted for API compat; see XML doc above.

        var resource = new DevTunnelResource(name);
        var rb = builder.AddResource(resource);

        // Subscribe to BeforeStartEvent for this specific tunnel. The CLI is a
        // *transport*, not an app resource — we deliberately spawn it outside the
        // Aspire resource lifecycle so the dashboard's Stop/Restart commands
        // can't tear it down and break still-running references.
        var hooks = new AgentTunnelHooks();
        builder.Eventing.Subscribe<BeforeStartEvent>(async (evt, ct) =>
        {
            var log = evt.Services.GetRequiredService<ILoggerFactory>().CreateLogger("AgentTunnel");
            await hooks.StartCliAsync(resource, log, ct).ConfigureAwait(false);
        });
        builder.Services.AddSingleton(hooks); // disposed by DI on shutdown

        return rb;
    }

    /// <summary>
    /// Alias for <see cref="AddDevTunnel(IDistributedApplicationBuilder, string)"/>
    /// with the accurate name. Use this in new code; <c>AddDevTunnel</c> remains
    /// for drop-in compatibility.
    /// </summary>
    public static IResourceBuilder<DevTunnelResource> AddAgentTunnel(
        this IDistributedApplicationBuilder builder,
        string name)
        => AddDevTunnel(builder, name);

    /// <summary>
    /// Registers an Aspire resource with the tunnel. The resource's <c>http</c>
    /// endpoint will be reachable from the public internet at the slot's URL.
    /// Slot name = <paramref name="source"/>'s resource name.
    /// </summary>
    public static IResourceBuilder<DevTunnelResource> WithReference<T>(
        this IResourceBuilder<DevTunnelResource> builder,
        IResourceBuilder<T> source,
        string endpointName = "http")
        where T : IResourceWithEndpoints
    {
        var slotName = source.Resource.Name;
        builder.Resource.SlotsByResource[slotName] = new AgentTunnelResource.SlotBinding(slotName, source.Resource, endpointName);
        return builder;
    }

    /// <summary>
    /// Flags the tunnel as anonymous-access. Persisted in the tunnel sidecar on
    /// the server side. Mirrors <c>Aspire.Hosting.DevTunnels.WithAnonymousAccess</c>.
    /// (Anonymous is already the default; this method exists for API parity.)
    /// </summary>
    public static IResourceBuilder<DevTunnelResource> WithAnonymousAccess(
        this IResourceBuilder<DevTunnelResource> builder)
    {
        builder.Resource.Anonymous = true;
        return builder;
    }

    /// <summary>
    /// Returns a <see cref="ReferenceExpression"/> for the public URL of <paramref name="resource"/>
    /// through this tunnel. Resolution awaits the CLI's first <c>TUNNEL_READY</c>
    /// emission, so the value is only materialised when an app actually needs it
    /// (typically because another resource has it in its environment).
    /// </summary>
    public static ReferenceExpression GetEndpoint<T>(
        this IResourceBuilder<DevTunnelResource> tunnel,
        IResourceBuilder<T> resource,
        string endpointName = "http")
        where T : IResourceWithEndpoints
    {
        _ = endpointName; // single-endpoint-per-slot in v0.1; reserved for multi-endpoint resources.
        var slotName = resource.Resource.Name;

        // Lazy: when the env-var is resolved, look up PublicUrls. If the slot
        // hasn't been registered yet (BeforeStart not fired), block until it is.
        return ReferenceExpression.Create($"{new TunnelUrlValueProvider(tunnel.Resource, slotName)}");
    }

    private sealed class TunnelUrlValueProvider : IValueProvider, IManifestExpressionProvider
    {
        private readonly DevTunnelResource _tunnel;
        private readonly string _slot;

        public TunnelUrlValueProvider(DevTunnelResource tunnel, string slot)
        {
            _tunnel = tunnel;
            _slot = slot;
        }

        public async ValueTask<string?> GetValueAsync(CancellationToken cancellationToken)
        {
            await _tunnel.Ready.Task.WaitAsync(cancellationToken).ConfigureAwait(false);
            return _tunnel.PublicUrls.TryGetValue(_slot, out var url) ? url : null;
        }

        // Manifest representation — used by Aspire publish to externalise the
        // reference into deployment manifests. We can't know the final URL at
        // publish time, so we emit a stable placeholder.
        public string ValueExpression => $"{{{_tunnel.Name}.tunnel.{_slot}.url}}";
    }
}

/// <summary>
/// Owns the <c>agent-tunnel host</c> child processes for every tunnel registered
/// via <see cref="AgentTunnelExtensions.AddDevTunnel(IDistributedApplicationBuilder,string)"/>.
/// One instance per AppHost, registered in DI so the runtime disposes it on
/// shutdown (which kills the children).
/// </summary>
internal sealed class AgentTunnelHooks : IAsyncDisposable
{
    private readonly List<Process> _processes = new();

    public Task StartCliAsync(AgentTunnelResource tunnel, ILogger logger, CancellationToken ct)
    {
        if (tunnel.SlotsByResource.Count == 0)
        {
            logger.LogWarning("AgentTunnel '{Tunnel}' has no slots registered via WithReference — skipping CLI spawn.", tunnel.TunnelName);
            tunnel.Ready.TrySetResult();
            return Task.CompletedTask;
        }
        return StartCliInternalAsync(tunnel, logger, ct);
    }

    private Task StartCliInternalAsync(AgentTunnelResource tunnel, ILogger logger, CancellationToken ct)
    {
        var binary = ResolveCliBinary()
            ?? throw new InvalidOperationException(
                "agent-tunnel CLI not found. Set PKS_AGENT_TUNNEL_BIN, place a binary next to the package, or install the CLI on PATH.");

        var args = new List<string>
        {
            "host",
            "--server", tunnel.ServerUrl,
            "--owner",  tunnel.Owner,
            "--name",   tunnel.TunnelName,
        };
        foreach (var (slot, binding) in tunnel.SlotsByResource)
        {
            // For v0.1 we assume each referenced resource exposes an HTTP endpoint
            // on a known port. Aspire's endpoint allocation runs before BeforeStart,
            // so binding.Source has a port by now. We forward to localhost:<port>
            // since both the CLI and the Aspire-managed processes share a host.
            var port = binding.Source.Annotations.OfType<EndpointAnnotation>()
                .FirstOrDefault(e => e.Name == binding.EndpointName)?.AllocatedEndpoint?.Port;
            if (port is null)
            {
                logger.LogWarning("AgentTunnel slot '{Slot}': resource has no allocated '{Endpoint}' endpoint yet; skipping.", slot, binding.EndpointName);
                continue;
            }
            args.Add("--http");
            args.Add($"{slot}=127.0.0.1:{port}");
        }

        var psi = new ProcessStartInfo
        {
            FileName = binary,
            RedirectStandardOutput = true,
            RedirectStandardError = true,
            UseShellExecute = false,
            CreateNoWindow = true,
        };
        foreach (var a in args) psi.ArgumentList.Add(a);

        var proc = new Process { StartInfo = psi, EnableRaisingEvents = true };
        proc.OutputDataReceived += (_, e) => HandleStdout(tunnel, logger, e.Data);
        proc.ErrorDataReceived  += (_, e) => { if (!string.IsNullOrEmpty(e.Data)) logger.LogInformation("agent-tunnel[{Tunnel}]: {Line}", tunnel.TunnelName, e.Data); };
        proc.Start();
        proc.BeginOutputReadLine();
        proc.BeginErrorReadLine();
        _processes.Add(proc);

        logger.LogInformation("Spawned agent-tunnel for '{Tunnel}' (pid {Pid}, {Slots} slots).", tunnel.TunnelName, proc.Id, tunnel.SlotsByResource.Count);
        return Task.CompletedTask;
    }

    private void HandleStdout(AgentTunnelResource tunnel, ILogger logger, string? line)
    {
        if (string.IsNullOrEmpty(line)) return;
        const string prefix = "TUNNEL_READY ";
        if (!line.StartsWith(prefix, StringComparison.Ordinal))
        {
            logger.LogTrace("agent-tunnel[{Tunnel}]: {Line}", tunnel.TunnelName, line);
            return;
        }

        try
        {
            var payload = JsonSerializer.Deserialize<TunnelReadyPayload>(line[prefix.Length..]);
            if (payload?.Slots is null) return;
            foreach (var s in payload.Slots)
            {
                if (!string.IsNullOrEmpty(s.Name) && !string.IsNullOrEmpty(s.Url))
                {
                    tunnel.PublicUrls[s.Name] = s.Url;
                    logger.LogInformation("agent-tunnel[{Tunnel}]: {Slot} → {Url}", tunnel.TunnelName, s.Name, s.Url);
                }
            }
            tunnel.Ready.TrySetResult();
        }
        catch (Exception ex)
        {
            logger.LogWarning(ex, "Failed to parse TUNNEL_READY line: {Line}", line);
        }
    }

    private static string? ResolveCliBinary()
    {
        // 1) Explicit env var.
        var fromEnv = Environment.GetEnvironmentVariable("PKS_AGENT_TUNNEL_BIN");
        if (!string.IsNullOrEmpty(fromEnv) && File.Exists(fromEnv)) return fromEnv;

        // 2) `tools/` co-located with the assembly (NuGet pack ships the binary).
        var asmDir = Path.GetDirectoryName(typeof(AgentTunnelExtensions).Assembly.Location);
        if (asmDir is not null)
        {
            var name = OperatingSystem.IsWindows() ? "agent-tunnel.exe" : "agent-tunnel";
            var candidate = Path.Combine(asmDir, "tools", name);
            if (File.Exists(candidate)) return candidate;
        }

        // 3) PATH lookup.
        return WhichOnPath(OperatingSystem.IsWindows() ? "agent-tunnel.exe" : "agent-tunnel");
    }

    private static string? WhichOnPath(string name)
    {
        var path = Environment.GetEnvironmentVariable("PATH") ?? "";
        foreach (var dir in path.Split(Path.PathSeparator))
        {
            if (string.IsNullOrEmpty(dir)) continue;
            var candidate = Path.Combine(dir, name);
            if (File.Exists(candidate)) return candidate;
        }
        return null;
    }

    public async ValueTask DisposeAsync()
    {
        foreach (var p in _processes)
        {
            try
            {
                if (!p.HasExited)
                {
                    p.Kill(entireProcessTree: true);
                    await p.WaitForExitAsync().ConfigureAwait(false);
                }
            }
            catch { /* shutting down */ }
            finally { p.Dispose(); }
        }
    }

    private sealed class TunnelReadyPayload
    {
        [System.Text.Json.Serialization.JsonPropertyName("slots")]
        public List<TunnelReadySlot>? Slots { get; set; }
    }

    private sealed class TunnelReadySlot
    {
        [System.Text.Json.Serialization.JsonPropertyName("name")]
        public string? Name { get; set; }

        [System.Text.Json.Serialization.JsonPropertyName("kind")]
        public string? Kind { get; set; }

        [System.Text.Json.Serialization.JsonPropertyName("url")]
        public string? Url { get; set; }

        [System.Text.Json.Serialization.JsonPropertyName("publicPort")]
        public int? PublicPort { get; set; }
    }
}
