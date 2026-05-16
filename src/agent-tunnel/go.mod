module github.com/pksorensen/pks-agent-tunnel/src/agent-tunnel

go 1.24

require (
	github.com/coder/websocket v1.8.13
	github.com/hashicorp/yamux v0.1.2
	github.com/pksorensen/pks-agent-tunnel/src/protocol v0.0.0-00010101000000-000000000000
)

replace github.com/pksorensen/pks-agent-tunnel/src/protocol => ../protocol
