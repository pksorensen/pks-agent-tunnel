# pks-agent-tunnel — Development Guide

Self-hosted devtunnel replacement. Three Go modules + one .NET project under a Go workspace.

## Layout

```
src/protocol/                              shared Go types (control frames, mux header)
src/agent-tunnel-server/                   server binary, Docker image, folder-backed state
src/agent-tunnel/                          CLI binary
src/aspire/Aspire.Hosting.AgentTunnel/     Aspire hosting extension (.NET)
```

`go.work` glues the three Go modules. The .NET project is independent.

## Build / run

```bash
# Build all Go modules
go build ./src/...

# Run the server (plain HTTP, no auth — dev only)
go run ./src/agent-tunnel-server --listen :8080 --control :7080 \
  --user-data-dir ./app/user-data

# Run the CLI
go run ./src/agent-tunnel host --server ws://localhost:7080 \
  --name agentic --http ws-relay=127.0.0.1:9000

# Build the Aspire extension
dotnet build src/aspire/Aspire.Hosting.AgentTunnel/
```

## Storage Convention

No database. State lives under `$USER_DATA_DIR` (default `./app/user-data` dev, `/data` container) as YAML-frontmatter `.md` sidecars. Atomic writes via `<file>.tmp` + `rename`. See `docs/storage.md`.

If you need a new entity type, add a new subfolder and a frontmatter schema in `docs/storage.md`. **Do not** add a database.

## Aspire surface — drop-in rules

The extension must remain a drop-in for `Aspire.Hosting.DevTunnels`:
- Public methods keep the same names: `AddDevTunnel`, `WithReference`, `WithAnonymousAccess`, `GetEndpoint`.
- The type `DevTunnelResource` is exposed (as a type alias for `AgentTunnelResource`) so existing user code referencing the type by name keeps compiling.
- New features ship as additional opt-in methods, never by repurposing existing ones.

When in doubt, compare with `external/agentic-live-www/src/apps/apphost/AppHost.cs:507–528` — that's the reference call pattern that must keep working.

## Releases

`release-please` watches `main`. Conventional Commits in PR titles drive version bumps. On release, CI builds and pushes Docker images to `registry.kjeldager.io/agent-tunnel-server:<version>` and `:latest`. Aspire extension NuGet packaging is added at v0.4.

## ADRs

Decision records live under `docs/adr/`, numbered from 0001. Add a new ADR for every non-obvious decision; don't litter ADRs through the parent agentic-live-www repo.
