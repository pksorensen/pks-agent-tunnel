using System.Collections.Concurrent;
using Aspire.Hosting.ApplicationModel;

namespace Aspire.Hosting.AgentTunnel;

/// <summary>
/// Aspire resource that owns one logical tunnel — the public-facing analogue of
/// <c>Aspire.Hosting.DevTunnels.DevTunnelResource</c>. Holds the slot registrations
/// added via <see cref="AgentTunnelExtensions.WithReference"/> and the per-slot
/// public URLs the CLI reports back via the <c>TUNNEL_READY</c> stdout line.
/// </summary>
public class AgentTunnelResource : Resource
{
    /// <summary>Owner identifier sent to the server. Defaults to <c>agentics</c>.</summary>
    public string Owner { get; }

    /// <summary>Tunnel name (e.g. <c>agentic-tunnel</c>). Matches the user-visible name.</summary>
    public string TunnelName { get; }

    /// <summary>Server control endpoint (e.g. <c>ws://localhost:7080</c> or <c>wss://tunnels.example.com:7443</c>).</summary>
    public string ServerUrl { get; set; }

    /// <summary>When true (default), slots are registered with <c>anonymous: true</c>.</summary>
    public bool Anonymous { get; internal set; } = true;

    /// <summary>
    /// Slot registrations, keyed by the underlying resource. Populated by
    /// <see cref="AgentTunnelExtensions.WithReference"/> at construction time.
    /// </summary>
    internal Dictionary<string, SlotBinding> SlotsByResource { get; } = new(StringComparer.Ordinal);

    /// <summary>
    /// Public URLs reported by the CLI after a successful <c>register_ack</c>.
    /// Keyed by slot name.
    /// </summary>
    internal ConcurrentDictionary<string, string> PublicUrls { get; } = new(StringComparer.Ordinal);

    /// <summary>
    /// TaskCompletionSource that completes when the CLI prints its first
    /// <c>TUNNEL_READY</c> line. <see cref="GetEndpoint"/> awaits on this.
    /// </summary>
    internal TaskCompletionSource Ready { get; } = new(TaskCreationOptions.RunContinuationsAsynchronously);

    public AgentTunnelResource(string name, string owner = "agentics", string serverUrl = "ws://localhost:7080") : base(name)
    {
        TunnelName = name;
        Owner = owner;
        ServerUrl = serverUrl;
    }

    internal sealed record SlotBinding(string SlotName, IResource Source, string EndpointName);
}

/// <summary>
/// Type alias for drop-in compatibility with <c>Aspire.Hosting.DevTunnels.DevTunnelResource</c>:
/// existing code that names the type by hand continues to compile after the
/// <c>using</c> change. Identical to <see cref="AgentTunnelResource"/>.
/// </summary>
public class DevTunnelResource(string name, string owner = "agentics", string serverUrl = "ws://localhost:7080")
    : AgentTunnelResource(name, owner, serverUrl);
