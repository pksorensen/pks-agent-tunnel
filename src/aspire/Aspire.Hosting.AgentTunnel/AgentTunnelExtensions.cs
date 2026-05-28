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

        // The CLI is a *transport*, not an app resource — we spawn it outside the
        // Aspire resource lifecycle so the dashboard's Stop/Restart commands can't
        // tear it down and break still-running references. The spawn itself is
        // triggered by WithReference: each call subscribes to its source's
        // ResourceEndpointsAllocatedEvent, and the CLI fires once the last source
        // has had its endpoint port allocated. AfterEndpointsAllocatedEvent is
        // deprecated in 13.x and doesn't fire reliably anymore.
        builder.Services.AddSingleton(new AgentTunnelHooks()); // disposed by DI on shutdown

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
        var tunnel = builder.Resource;
        tunnel.SlotsByResource[slotName] = new AgentTunnelResource.SlotBinding(slotName, source.Resource, endpointName);

        // Add a child resource per slot so it renders in the dashboard nested
        // under the tunnel — matches the per-port rows Aspire.Hosting.DevTunnels
        // displays. The URL is filled in once TUNNEL_READY arrives.
        var slotResource = new AgentTunnelSlotResource(
            name: $"{tunnel.Name}-{slotName}",
            parent: tunnel,
            slotName: slotName,
            source: source.Resource,
            endpointName: endpointName);
        builder.ApplicationBuilder.AddResource(slotResource);
        tunnel.SlotResources[slotName] = slotResource;

        // Subscribe to this source's ResourceEndpointsAllocatedEvent. When the
        // last referenced source fires (count matches), spawn the CLI — by then
        // every slot has an allocated port available via EndpointAnnotation.
        source.OnResourceEndpointsAllocated(async (_, evt, ct) =>
        {
            await tunnel.StartGate.WaitAsync(ct).ConfigureAwait(false);
            try
            {
                tunnel.SlotEndpointsAllocated.Add(slotName);
                if (tunnel.CliStarted) return;
                if (tunnel.SlotEndpointsAllocated.Count < tunnel.SlotsByResource.Count) return;
                tunnel.CliStarted = true;
            }
            finally { tunnel.StartGate.Release(); }

            var hooks = evt.Services.GetRequiredService<AgentTunnelHooks>();
            var log = evt.Services.GetRequiredService<ILoggerFactory>().CreateLogger("AgentTunnel");
            var notifier = evt.Services.GetRequiredService<ResourceNotificationService>();
            await hooks.StartCliAsync(tunnel, log, notifier, ct).ConfigureAwait(false);
        });
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
    /// Rewrites the scheme and/or port of every URL the server reports in
    /// <c>TUNNEL_READY</c>. Use when the deployed <c>agent-tunnel-server</c> is
    /// behind external TLS termination (e.g. a reverse proxy at <c>:8443</c>
    /// fronting plain HTTP at <c>:18080</c>) — the server doesn't know about the
    /// proxy, so it emits <c>http://…:18080</c> URLs that don't resolve for
    /// callers. Pass <c>scheme: "https", port: 8443</c> to fix them up locally.
    /// Pass <c>null</c> to leave that part untouched.
    /// </summary>
    public static IResourceBuilder<DevTunnelResource> WithPublicUrlOverride(
        this IResourceBuilder<DevTunnelResource> builder,
        string? scheme = null,
        int? port = null)
    {
        builder.Resource.PublicUrlOverride = (scheme, port);
        return builder;
    }

    /// <summary>
    /// Points the tunnel at a specific server. Example:
    /// <c>.WithServer("ws://tunnels.agentics.dk:17080")</c>. Defaults to
    /// <c>ws://localhost:7080</c> when not set — useful when the AppHost
    /// also spawns an <c>agent-tunnel-server</c> locally.
    /// </summary>
    public static IResourceBuilder<DevTunnelResource> WithServer(
        this IResourceBuilder<DevTunnelResource> builder,
        string url)
    {
        builder.Resource.ServerUrl = url;
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

    public async Task StartCliAsync(AgentTunnelResource tunnel, ILogger logger, ResourceNotificationService notifier, CancellationToken ct)
    {
        if (tunnel.SlotsByResource.Count == 0)
        {
            logger.LogWarning("AgentTunnel '{Tunnel}' has no slots registered via WithReference — skipping CLI spawn.", tunnel.TunnelName);
            await notifier.PublishUpdateAsync(tunnel, s => s with
            {
                State = new ResourceStateSnapshot("No slots", KnownResourceStateStyles.Warn),
            }).ConfigureAwait(false);
            tunnel.Ready.TrySetResult();
            return;
        }

        // Initial dashboard state: tunnel + each slot "Starting".
        await notifier.PublishUpdateAsync(tunnel, s => s with
        {
            State = new ResourceStateSnapshot("Starting", KnownResourceStateStyles.Info),
            Properties = s.Properties.Add(new ResourcePropertySnapshot("server", tunnel.ServerUrl)),
        }).ConfigureAwait(false);
        foreach (var slot in tunnel.SlotResources.Values)
        {
            await notifier.PublishUpdateAsync(slot, s => s with
            {
                State = new ResourceStateSnapshot("Starting", KnownResourceStateStyles.Info),
                Properties = s.Properties
                    .Add(new ResourcePropertySnapshot("source", slot.Source.Name))
                    .Add(new ResourcePropertySnapshot("endpoint", slot.EndpointName)),
            }).ConfigureAwait(false);
        }

        await StartCliInternalAsync(tunnel, logger, notifier, ct).ConfigureAwait(false);
    }

    private Task StartCliInternalAsync(AgentTunnelResource tunnel, ILogger logger, ResourceNotificationService notifier, CancellationToken ct)
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
            // Aspire's endpoint allocation has fired by the time WithReference's
            // OnResourceEndpointsAllocated callback runs StartCliAsync.
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
        proc.OutputDataReceived += (_, e) => _ = HandleStdoutAsync(tunnel, logger, notifier, e.Data);
        proc.ErrorDataReceived  += (_, e) => { if (!string.IsNullOrEmpty(e.Data)) logger.LogInformation("agent-tunnel[{Tunnel}]: {Line}", tunnel.TunnelName, e.Data); };
        proc.Exited += async (_, _) =>
        {
            try
            {
                var stopped = new ResourceStateSnapshot("Exited", KnownResourceStateStyles.Error);
                await notifier.PublishUpdateAsync(tunnel, s => s with { State = stopped }).ConfigureAwait(false);
                foreach (var slot in tunnel.SlotResources.Values)
                    await notifier.PublishUpdateAsync(slot, s => s with { State = stopped }).ConfigureAwait(false);
            }
            catch { /* shutting down */ }
        };
        proc.Start();
        proc.BeginOutputReadLine();
        proc.BeginErrorReadLine();
        _processes.Add(proc);

        logger.LogInformation("Spawned agent-tunnel for '{Tunnel}' (pid {Pid}, {Slots} slots).", tunnel.TunnelName, proc.Id, tunnel.SlotsByResource.Count);
        return Task.CompletedTask;
    }

    private async Task HandleStdoutAsync(AgentTunnelResource tunnel, ILogger logger, ResourceNotificationService notifier, string? line)
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
                if (string.IsNullOrEmpty(s.Name) || string.IsNullOrEmpty(s.Url)) continue;
                var url = ApplyPublicUrlOverride(s.Url, tunnel.PublicUrlOverride, logger, tunnel.TunnelName);
                tunnel.PublicUrls[s.Name] = url;
                logger.LogInformation("agent-tunnel[{Tunnel}]: {Slot} → {Url}", tunnel.TunnelName, s.Name, url);

                if (tunnel.SlotResources.TryGetValue(s.Name, out var slotResource))
                {
                    await notifier.PublishUpdateAsync(slotResource, snap => snap with
                    {
                        State = new ResourceStateSnapshot("Running", KnownResourceStateStyles.Success),
                        Urls  = snap.Urls.Add(new UrlSnapshot(Name: "public", Url: url, IsInternal: false)),
                    }).ConfigureAwait(false);
                }
            }

            await notifier.PublishUpdateAsync(tunnel, snap => snap with
            {
                State = new ResourceStateSnapshot("Running", KnownResourceStateStyles.Success),
            }).ConfigureAwait(false);
            tunnel.Ready.TrySetResult();
        }
        catch (Exception ex)
        {
            logger.LogWarning(ex, "Failed to parse TUNNEL_READY line: {Line}", line);
        }
    }

    private static string ApplyPublicUrlOverride(string raw, (string? Scheme, int? Port) ov, ILogger logger, string tunnelName)
    {
        if (ov.Scheme is null && ov.Port is null) return raw;
        try
        {
            var b = new UriBuilder(raw);
            if (ov.Scheme is { } scheme) b.Scheme = scheme;
            if (ov.Port is { } port) b.Port = port;
            return b.Uri.ToString();
        }
        catch (Exception ex)
        {
            logger.LogWarning(ex, "agent-tunnel[{Tunnel}]: failed to apply public URL override to {Url}", tunnelName, raw);
            return raw;
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
