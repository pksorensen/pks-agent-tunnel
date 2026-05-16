# 0001 — Language: Go for both server and CLI

- Status: Accepted
- Date: 2026-05-16

## Context

We need a single static binary for both the server (deployed as a Docker image) and the CLI (shipped into devcontainers and runner containers). The Aspire AppHost already builds and runs a Go binary (`external/vibecast`), so the toolchain is in place.

## Decision

Write the server, CLI, and shared protocol module in Go 1.24+. Use `go.work` to glue them into one workspace.

The Aspire hosting extension is in C# / .NET 10 because it has to integrate with `Aspire.Hosting`.

## Consequences

- `+` Single static binary per platform, trivially cross-compiled (linux/amd64, linux/arm64, darwin, windows).
- `+` Cheap goroutines for the per-connection mux model.
- `+` Strong stdlib for net/http + WebSocket via `gorilla/websocket`.
- `+` Matches `vibecast` / `pks-agent-ftp` / `pks-agent-inbox` toolchain.
- `−` Two languages in the repo (Go + C#) — the boundary is the CLI's `TUNNEL_READY` stdout contract, which keeps coupling small.
