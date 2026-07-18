// Package host implements the `agent-tunnel host` subcommand.
package host

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/hashicorp/yamux"

	"github.com/pksorensen/pks-agent-tunnel/src/protocol"
)

// Run executes the host subcommand. args excludes the program name and
// "host" verb.
func Run(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("host", flag.ContinueOnError)
	var (
		server  = fs.String("server", "ws://localhost:7080", "Control plane URL (ws:// or wss://)")
		owner   = fs.String("owner", "agentics", "Owner identifier")
		name    = fs.String("name", "agentic", "Tunnel name")
		token   = fs.String("token", "", "Bearer token (when --auth-mode=token on the server)")
		httpDefs multiFlag
	)
	fs.Var(&httpDefs, "http", `HTTP slot, repeated: --http <slot>=<host:port>`)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(httpDefs) == 0 {
		return fmt.Errorf("at least one --http slot required")
	}

	slots := make([]slotConfig, 0, len(httpDefs))
	for _, def := range httpDefs {
		slot, upstream, ok := strings.Cut(def, "=")
		if !ok || slot == "" || upstream == "" {
			return fmt.Errorf("--http: expected <slot>=<host:port>, got %q", def)
		}
		if err := protocol.ValidateName(slot); err != nil {
			return err
		}
		slots = append(slots, slotConfig{Name: slot, Kind: protocol.SlotKindHTTP, Upstream: upstream})
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	c := connector{
		log:      log,
		serverURL: *server,
		owner:    *owner,
		tunnel:   *name,
		token:    *token,
		slots:    slots,
	}
	return c.serve(ctx)
}

// serve runs the control connection and reconnects with capped backoff whenever
// it drops. c.run returns when the control session ends — a server redeploy, a
// network blip, the peer going away — and without this loop the process would
// simply exit, leaving every slot dead (502 "no client connected") until
// something restarts it. Retrying in-process makes a server restart self-heal.
func (c *connector) serve(ctx context.Context) error {
	const maxBackoff = 30 * time.Second
	backoff := time.Second
	for {
		start := time.Now()
		err := c.run(ctx)
		if ctx.Err() != nil {
			return nil // clean shutdown (context cancelled)
		}
		if time.Since(start) > maxBackoff {
			backoff = time.Second // stayed up a while → fresh drop, reset backoff
		}
		c.log.Warn("tunnel control session ended; reconnecting", "err", err, "retry_in", backoff)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

type slotConfig struct {
	Name     string
	Kind     protocol.SlotKind
	Upstream string
}

type connector struct {
	log       *slog.Logger
	serverURL string
	owner     string
	tunnel    string
	token     string
	slots     []slotConfig
}

func (c *connector) run(ctx context.Context) error {
	wsURL := strings.TrimRight(c.serverURL, "/") + "/v1/control"

	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("dial %s: %w", wsURL, err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	nc := websocket.NetConn(ctx, conn, websocket.MessageBinary)
	sess, err := yamux.Client(nc, nil)
	if err != nil {
		return fmt.Errorf("yamux client: %w", err)
	}
	defer sess.Close()

	ctrl, err := sess.OpenStream()
	if err != nil {
		return fmt.Errorf("open control stream: %w", err)
	}

	// Register
	specs := make([]protocol.SlotSpec, 0, len(c.slots))
	for _, s := range c.slots {
		specs = append(specs, protocol.SlotSpec{
			Name:         s.Name,
			Kind:         s.Kind,
			UpstreamHint: s.Upstream,
			Anonymous:    true,
		})
	}
	regBytes, _ := json.Marshal(protocol.RegisterFrame{
		Type:   protocol.FrameRegister,
		Tunnel: c.tunnel,
		Owner:  c.owner,
		Slots:  specs,
		Token:  c.token,
	})
	if _, err := ctrl.Write(append(regBytes, '\n')); err != nil {
		return fmt.Errorf("write register: %w", err)
	}

	br := bufio.NewReader(ctrl)
	ackLine, err := br.ReadBytes('\n')
	if err != nil {
		return fmt.Errorf("read register_ack: %w", err)
	}
	ftype, _ := protocol.DecodeEnvelope(ackLine)
	if ftype == protocol.FrameError {
		var ef protocol.ErrorFrame
		_ = json.Unmarshal(ackLine, &ef)
		return fmt.Errorf("server error: %s (%s)", ef.Message, ef.Code)
	}
	if ftype != protocol.FrameRegisterAck {
		return fmt.Errorf("expected register_ack, got %s", ftype)
	}
	var ack protocol.RegisterAckFrame
	if err := json.Unmarshal(ackLine, &ack); err != nil {
		return fmt.Errorf("parse register_ack: %w", err)
	}

	// Emit TUNNEL_READY to stdout — the Aspire extension parses this.
	emitReady(ack)
	c.log.Info("registered", "tunnel", ack.Tunnel, "slots", len(ack.Slots))

	// Index upstream addresses by slot name for quick lookup on inbound streams.
	upstreams := make(map[string]string, len(c.slots))
	for _, s := range c.slots {
		upstreams[s.Name] = s.Upstream
	}

	// Accept inbound mux streams from the server and forward to upstreams.
	acceptLoop := make(chan error, 1)
	go func() {
		for {
			stream, err := sess.AcceptStream()
			if err != nil {
				acceptLoop <- err
				return
			}
			go handleInbound(c.log, stream, upstreams)
		}
	}()

	select {
	case <-ctx.Done():
		return nil
	case err := <-acceptLoop:
		return fmt.Errorf("session ended: %w", err)
	}
}

func emitReady(ack protocol.RegisterAckFrame) {
	type slotOut struct {
		Name string `json:"name"`
		Kind string `json:"kind"`
		URL  string `json:"url,omitempty"`
		Port int    `json:"publicPort,omitempty"`
	}
	out := struct {
		Slots []slotOut `json:"slots"`
	}{}
	for _, s := range ack.Slots {
		out.Slots = append(out.Slots, slotOut{Name: s.Name, Kind: string(s.Kind), URL: s.URL, Port: s.PublicPort})
	}
	b, _ := json.Marshal(out)
	fmt.Printf("TUNNEL_READY %s\n", b)
}

func handleInbound(log *slog.Logger, stream net.Conn, upstreams map[string]string) {
	defer stream.Close()

	br := bufio.NewReader(stream)
	line, err := br.ReadBytes('\n')
	if err != nil {
		log.Warn("read stream meta", "err", err)
		return
	}
	var meta protocol.StreamMeta
	if err := json.Unmarshal(line[:len(line)-1], &meta); err != nil {
		log.Warn("parse stream meta", "err", err)
		return
	}
	addr, ok := upstreams[meta.Slot]
	if !ok {
		log.Warn("unknown slot in stream meta", "slot", meta.Slot)
		_ = writeBadGateway(stream, fmt.Sprintf("slot %q not configured", meta.Slot))
		return
	}

	d := net.Dialer{Timeout: 5 * time.Second}
	up, err := d.Dial("tcp", addr)
	if err != nil {
		log.Warn("dial upstream", "addr", addr, "err", err)
		_ = writeBadGateway(stream, fmt.Sprintf("upstream dial: %v", err))
		return
	}
	defer up.Close()

	// Drain anything buffered in br to the upstream before splicing raw.
	if buf, _ := br.Peek(br.Buffered()); len(buf) > 0 {
		if _, err := up.Write(buf); err != nil {
			return
		}
		_, _ = br.Discard(len(buf))
	}

	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(up, stream); done <- struct{}{} }()
	go func() { _, _ = io.Copy(stream, up); done <- struct{}{} }()
	<-done
}

func writeBadGateway(w io.Writer, msg string) error {
	resp := &http.Response{
		StatusCode: http.StatusBadGateway,
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     http.Header{"Content-Type": []string{"text/plain; charset=utf-8"}},
		Body:       io.NopCloser(strings.NewReader(msg)),
	}
	return resp.Write(w)
}

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }
