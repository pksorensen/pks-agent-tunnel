// Command agent-tunnel is the client CLI. It connects to an
// agent-tunnel-server control endpoint, registers one or more slots, and
// proxies inbound public traffic to local upstreams.
//
// v0.1: only the `host` subcommand. Quick mode:
//
//	agent-tunnel host \
//	  --server ws://localhost:7080 \
//	  --owner agentics \
//	  --name agentic \
//	  --http ws-relay=127.0.0.1:9000 \
//	  --http plugin-marketplace=127.0.0.1:40145
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/pksorensen/pks-agent-tunnel/src/agent-tunnel/internal/host"
)

// version is stamped at build time via -ldflags "-X main.version=<semver>".
// Defaults to "dev" for local `go run`/`go build` without the flag.
var version = "dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "host":
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		if err := host.Run(ctx, os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "host:", err)
			os.Exit(1)
		}
	case "version", "--version", "-v":
		fmt.Println("agent-tunnel", version)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintln(os.Stderr, "unknown command:", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `agent-tunnel — self-hosted devtunnel client

Subcommands:
  host      Connect to a tunnel server and expose local upstreams.
  version   Print build version.
  help      Show this help.

Run `+"`agent-tunnel host -h`"+` for host options.`)
}
