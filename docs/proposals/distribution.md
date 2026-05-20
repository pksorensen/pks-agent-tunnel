# Track 1 — CLI binaries on GitHub Releases, Aspire integration on NuGet.org

- Status: Proposal
- Date: 2026-05-20

## Context

Today the project ships exactly one consumable artefact: the `agent-tunnel-server` Docker image at `registry.kjeldager.io/agent-tunnel-server:<version>`. Everything else requires a `git clone`:

- The `agent-tunnel` CLI is only available as Go source — anyone wanting to use it has to install Go 1.24+ and `go build`. Hostile to non-Go users (and to bootstrap scripts).
- The `Aspire.Hosting.AgentTunnel` extension is only consumable via a `ProjectReference` to a path inside this repo (see `external/agentic-live-www/src/apps/apphost/apphost.csproj`). That blocks the drop-in story promised by [ADR 0005](../adr/0005-aspire-extension-dropin-shape.md): a user wanting to swap `Aspire.Hosting.DevTunnels` for ours cannot do it with a `<PackageReference>` line.

`release-please` already cuts a GitHub Release on every version tag (`.github/workflows/release.yml`), and the existing `build-push` job is gated on `release-please.outputs.release_created == 'true'`. We need to bolt two more jobs onto that same gate.

## Decision

| Question | Choice |
|---|---|
| Where do CLI binaries live? | **GitHub Releases** as release assets, attached to the `agent-tunnel-v<X.Y.Z>` tag release-please already creates. Not the container registry — `registry.kjeldager.io` is a Docker registry; binaries don't fit. |
| How are they built? | **GoReleaser** in a new `release-cli` workflow job, on `ubuntu-latest`. Pure-Go cross-compile, no need for a self-hosted runner. |
| What gets published per tag? | Five binaries (`linux-amd64`, `linux-arm64`, `darwin-amd64`, `darwin-arm64`, `windows-amd64.exe`) plus a `checksums.txt` (SHA256). No archives — single static binaries are easier for `curl \| sh` and for the NuGet pack step to consume by HTTP. |
| How does the NuGet embed them? | A `release-nuget` job that runs *after* `release-cli`, downloads the five assets from the just-published GitHub Release, and `dotnet pack`s them into `tools/<rid>/agent-tunnel[.exe]` inside the package via `<Content … PackagePath="tools/$(RID)/…" />` items. |
| How does `ResolveCliBinary` pick a RID at runtime? | Extend the resolver to try `tools/<RuntimeInformation.RuntimeIdentifier>/agent-tunnel[.exe]` *before* the existing single-binary path. |
| Where is the NuGet published? | **nuget.org**. Public, anonymous pull, default feed — matches the drop-in promise. GitHub Packages requires `GITHUB_TOKEN` auth even for public packages, which is needless friction for an integration meant to be one `<PackageReference>` away. |

The existing `build-push` job stays exactly as it is; the Docker image remains the canonical server distribution.

## Implementation

### 1. CI flow

```
release-please ──► build-push        (existing, server image)
              ├──► release-cli       (NEW: goreleaser → GitHub Release assets)
              └──► release-nuget     (NEW: needs release-cli; pack + push to nuget.org)
```

All three jobs are gated on `needs.release-please.outputs.release_created == 'true'`, same as today. `release-nuget` additionally `needs: [release-please, release-cli]`.

### 2. GoReleaser configuration

A single `.goreleaser.yml` at repo root. Sketch:

```yaml
version: 2
project_name: agent-tunnel
builds:
  - id: agent-tunnel
    main: ./src/agent-tunnel
    binary: agent-tunnel
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]
    ignore:
      - { goos: windows, goarch: arm64 }   # not in the matrix we promised
    env: [CGO_ENABLED=0]
    ldflags:
      - -s -w -X main.version={{.Version}}
archives:
  - format: binary                          # no tar.gz / zip — raw binaries
    name_template: "agent-tunnel-{{ .Os }}-{{ .Arch }}"
checksum:
  name_template: "checksums.txt"
release:
  github:
    owner: pksorensen
    name: pks-agent-tunnel
  # release-please already created the release; just upload assets to it.
  mode: append
```

GoReleaser's `--clean` + `mode: append` is the right shape when the GitHub Release was created by another tool. The job sets `GITHUB_TOKEN` and `GORELEASER_CURRENT_TAG=${{ needs.release-please.outputs.tag_name }}` and runs `goreleaser release --clean`.

### 3. NuGet pack

Two new files in `src/aspire/Aspire.Hosting.AgentTunnel/`:

`build/download-cli.ps1` (cross-platform PowerShell — runs on `ubuntu-latest` via `pwsh`):

```pwsh
param([string]$Version, [string]$OutDir)
$base = "https://github.com/pksorensen/pks-agent-tunnel/releases/download/agent-tunnel-v$Version"
$rids = @{
  "linux-x64"   = "agent-tunnel-linux-amd64"
  "linux-arm64" = "agent-tunnel-linux-arm64"
  "osx-x64"     = "agent-tunnel-darwin-amd64"
  "osx-arm64"   = "agent-tunnel-darwin-arm64"
  "win-x64"     = "agent-tunnel-windows-amd64.exe"
}
foreach ($rid in $rids.Keys) {
  $name = if ($rid -eq "win-x64") { "agent-tunnel.exe" } else { "agent-tunnel" }
  $dst  = Join-Path $OutDir "$rid/$name"
  New-Item -ItemType Directory -Force (Split-Path $dst) | Out-Null
  Invoke-WebRequest "$base/$($rids[$rid])" -OutFile $dst
  if ($rid -notlike "win*") { chmod +x $dst }
}
```

`Aspire.Hosting.AgentTunnel.csproj` additions:

```xml
<PropertyGroup>
  <Version>$(ReleaseVersion)</Version>
  <CliToolsDir>$(IntermediateOutputPath)tools</CliToolsDir>
  <GeneratePackageOnBuild>false</GeneratePackageOnBuild>
  <PackageReadmeFile>README.md</PackageReadmeFile>
</PropertyGroup>

<Target Name="DownloadCliBinaries" BeforeTargets="GenerateNuspec">
  <Exec Command="pwsh -File build/download-cli.ps1 -Version $(Version) -OutDir $(CliToolsDir)" />
</Target>

<ItemGroup>
  <Content Include="$(CliToolsDir)/**/*"
           Pack="true"
           PackagePath="tools/%(RecursiveDir)%(Filename)%(Extension)" />
  <None Include="README.md" Pack="true" PackagePath="\" />
</ItemGroup>
```

Result: `Aspire.Hosting.AgentTunnel.<version>.nupkg` ships:

```
lib/net10.0/Aspire.Hosting.AgentTunnel.dll
tools/linux-x64/agent-tunnel
tools/linux-arm64/agent-tunnel
tools/osx-x64/agent-tunnel
tools/osx-arm64/agent-tunnel
tools/win-x64/agent-tunnel.exe
```

This is the same shape `Aspire.Hosting.NodeJs` uses to ship `node`/`npm` shims, and the same shape `dotnet-ef` uses to ship its tool binaries.

### 4. `ResolveCliBinary` upgrade

Current implementation (`AgentTunnelExtensions.cs:330`) only checks one `tools/agent-tunnel`. New lookup order, lowest-priority last:

1. `PKS_AGENT_TUNNEL_BIN` env var (unchanged — escape hatch).
2. `tools/<RuntimeInformation.RuntimeIdentifier>/agent-tunnel[.exe]` co-located with the assembly. **New.** This is the NuGet path.
3. `tools/agent-tunnel[.exe]` co-located with the assembly. Existing fallback — keeps the single-binary in-repo dev path working (where someone drops one cross-compile next to the DLL).
4. `PATH` lookup (unchanged).

RID mapping is `RuntimeInformation.RuntimeIdentifier` directly — but that returns OS-version-pinned values (`ubuntu.22.04-x64`). Two safe options:

- Use `RuntimeInformation.OSArchitecture` + `RuntimeInformation.IsOSPlatform(...)` to derive a *generic* RID (`linux-x64`, `osx-arm64`, `win-x64`). Recommended; predictable and matches our package layout.
- Walk the RID graph fallback chain. More plumbing, no real benefit since we only ship five RIDs.

### 5. NuGet publish

`release-nuget` job (`ubuntu-latest`):

```yaml
release-nuget:
  needs: [release-please, release-cli]
  if: needs.release-please.outputs.release_created == 'true'
  runs-on: ubuntu-latest
  steps:
    - uses: actions/checkout@v4
    - uses: actions/setup-dotnet@v4
      with: { dotnet-version: '10.0.x' }
    - name: Pack
      run: |
        dotnet pack src/aspire/Aspire.Hosting.AgentTunnel/Aspire.Hosting.AgentTunnel.csproj \
          -c Release \
          -p:ReleaseVersion=${{ needs.release-please.outputs.version }} \
          -o ./nupkg
    - name: Push
      run: |
        dotnet nuget push ./nupkg/*.nupkg \
          --source https://api.nuget.org/v3/index.json \
          --api-key ${{ secrets.NUGET_API_KEY }} \
          --skip-duplicate
```

`NUGET_API_KEY` is a new repository secret — owner-scoped key from nuget.org for the `Aspire.Hosting.AgentTunnel` package id (or a glob over `Aspire.Hosting.AgentTunnel*` if we ever ship a `.Server` variant).

### 6. Consumer story after this lands

```xml
<!-- agentic-live-www/src/apps/apphost/apphost.csproj -->
<PackageReference Include="Aspire.Hosting.AgentTunnel" Version="0.2.0" />
```

```cs
using Aspire.Hosting.AgentTunnel;   // was: Aspire.Hosting.DevTunnels
```

Zero other changes. The CLI binary for the current RID is unpacked under `bin/Debug/net10.0/tools/linux-x64/agent-tunnel`, and `ResolveCliBinary` finds it.

## Files to add/modify

| File | Change |
|---|---|
| `.goreleaser.yml` | **New.** Build matrix + checksum + GitHub Release append. |
| `.github/workflows/release.yml` | **Modify.** Add `release-cli` and `release-nuget` jobs gated on `release_created`. |
| `src/aspire/Aspire.Hosting.AgentTunnel/Aspire.Hosting.AgentTunnel.csproj` | **Modify.** `<Version>` from `$(ReleaseVersion)`, download-target, `<Content … Pack="true" />` items, `PackageReadmeFile`. |
| `src/aspire/Aspire.Hosting.AgentTunnel/build/download-cli.ps1` | **New.** Fetches the five binaries from the GitHub Release into `obj/tools/<rid>/`. |
| `src/aspire/Aspire.Hosting.AgentTunnel/README.md` | **New.** Short consumer-facing readme — required by nuget.org for a good package page. |
| `src/aspire/Aspire.Hosting.AgentTunnel/AgentTunnelExtensions.cs` | **Modify.** `ResolveCliBinary` learns the `tools/<rid>/` layout; add a `GetGenericRid()` helper. |
| `docs/adr/0007-distribution.md` | **New.** Promote this proposal to an ADR once accepted. |

## Prerequisites

- `NUGET_API_KEY` repo secret — new. Owner-scoped API key from nuget.org for `Aspire.Hosting.AgentTunnel`.
- Reserve the package id `Aspire.Hosting.AgentTunnel` on nuget.org before the first publish (push as a prerelease `0.2.0-alpha` from a manual workflow run to claim the name, or use nuget.org's reserve-prefix flow).
- `pwsh` is preinstalled on `ubuntu-latest` GitHub-hosted runners — no extra setup.
- The `release-cli` and `release-nuget` jobs run on `ubuntu-latest`, not the self-hosted devcontainer runner. The self-hosted runner is only needed by `build-push` (Docker into the private registry).

## Gotchas

- **Tag format.** release-please tags the Go module `agent-tunnel-v0.2.0`, not `v0.2.0` (see the `component` in `release-please-config.json`). GoReleaser and the NuGet download URL must use the full tag. Pass `GORELEASER_CURRENT_TAG: ${{ needs.release-please.outputs.tag_name }}` and build the asset URL as `…/releases/download/${{ tag_name }}/…`.
- **Version drift between CLI and NuGet.** Both ship under the same release-please version today. If we ever decouple them (e.g. NuGet 0.3.0 ships CLI 0.2.5), the download URL has to know which CLI version to pull. For now, keep them lockstep — same version, same tag — and revisit when a real reason to decouple appears.
- **Generic RID, not RuntimeIdentifier.** `RuntimeInformation.RuntimeIdentifier` returns `ubuntu.22.04-x64` on Ubuntu hosts, which won't match our `linux-x64` layout. Derive the generic RID from `OSArchitecture` + `IsOSPlatform`. Tested in the resolver.
- **macOS Gatekeeper.** Unsigned `darwin` binaries downloaded by users will be quarantined and refused on first run unless they `xattr -d com.apple.quarantine`. For the AppHost-via-NuGet path this doesn't trigger (no quarantine on files extracted by `dotnet restore`). Document the manual unblock step in the README for direct CLI users; full notarisation is out of scope for v0.2.
- **Windows arm64.** Excluded from the matrix — Aspire on Windows arm64 is niche enough to defer. Add later if a consumer asks.
- **`goreleaser` requires the tag to exist before it runs.** release-please creates the tag *and* the release in the same workflow run, so the `release-cli` job will see both. If you ever move `release-please` to a separate workflow, re-check `needs:` wiring.
- **NuGet symbols / source-link.** Skipped for v0.2. The package is mostly a binary container; add `Microsoft.SourceLink.GitHub` later if anyone needs to debug into the extension.
- **`--skip-duplicate` on `dotnet nuget push`.** Makes the job idempotent if release-please re-runs the workflow on the same tag. Without it, a re-run fails noisily.
- **`tools/` collides with .NET tool packs.** `tools/<tfm>/<rid>/` is the *dotnet-tool* convention; we use `tools/<rid>/` because this is *not* a `DotnetCliTool` package — it's an Aspire integration that happens to ship binaries. The `ResolveCliBinary` lookup is the authoritative reader, so the layout is whatever we want it to be. Just don't add `<PackageType>DotnetTool</PackageType>`.
