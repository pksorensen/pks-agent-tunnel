// Package control implements the control-plane HTTP handler: accepts WSS
// upgrades at /v1/control, runs a yamux server over the connection, reads
// the RegisterFrame, persists/binds slots, and replies with RegisterAckFrame.
package control

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/hashicorp/yamux"

	"github.com/pksorensen/pks-agent-tunnel/src/agent-tunnel-server/internal/config"
	"github.com/pksorensen/pks-agent-tunnel/src/agent-tunnel-server/internal/sessions"
	"github.com/pksorensen/pks-agent-tunnel/src/agent-tunnel-server/internal/store"
	"github.com/pksorensen/pks-agent-tunnel/src/protocol"
)

// Handler bundles the dependencies the control-plane router needs.
type Handler struct {
	log *slog.Logger
	st  *store.Store
	reg *sessions.Registry
	cfg config.Config
}

func NewHandler(log *slog.Logger, st *store.Store, reg *sessions.Registry, cfg config.Config) http.Handler {
	mux := http.NewServeMux()
	h := &Handler{log: log, st: st, reg: reg, cfg: cfg}
	mux.HandleFunc("/v1/control", h.handleControl)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	return mux
}

func (h *Handler) handleControl(w http.ResponseWriter, r *http.Request) {
	wsConn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// Dev: we run the CLI locally and there is no browser origin.
		InsecureSkipVerify: true,
	})
	if err != nil {
		h.log.Warn("ws accept", "err", err)
		return
	}
	defer wsConn.Close(websocket.StatusNormalClosure, "")

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	nc := websocket.NetConn(ctx, wsConn, websocket.MessageBinary)
	sess, err := yamux.Server(nc, nil)
	if err != nil {
		h.log.Warn("yamux server", "err", err)
		return
	}
	defer sess.Close()
	defer h.reg.UnbindSession(sess)

	// Stream 0 — control channel.
	ctrl, err := sess.AcceptStream()
	if err != nil {
		h.log.Warn("accept control stream", "err", err)
		return
	}
	defer ctrl.Close()

	br := bufio.NewReader(ctrl)
	line, err := br.ReadBytes('\n')
	if err != nil {
		h.log.Warn("read register frame", "err", err)
		return
	}
	var reg protocol.RegisterFrame
	if err := json.Unmarshal(line, &reg); err != nil {
		writeError(ctrl, "bad_frame", err.Error(), "")
		return
	}
	if reg.Type != protocol.FrameRegister {
		writeError(ctrl, "bad_frame", "first frame must be type=register", "")
		return
	}
	if err := protocol.ValidateName(reg.Owner); err != nil {
		writeError(ctrl, "bad_name", err.Error(), "")
		return
	}
	if err := protocol.ValidateName(reg.Tunnel); err != nil {
		writeError(ctrl, "bad_name", err.Error(), "")
		return
	}

	// Persist tunnel.
	if err := h.st.PutTunnel(store.Tunnel{Owner: reg.Owner, Name: reg.Tunnel}); err != nil {
		writeError(ctrl, "store", err.Error(), "")
		return
	}

	acks := make([]protocol.SlotAck, 0, len(reg.Slots))
	for _, s := range reg.Slots {
		if err := protocol.ValidateName(s.Name); err != nil {
			writeError(ctrl, "bad_name", err.Error(), s.Name)
			return
		}
		switch s.Kind {
		case protocol.SlotKindHTTP:
			subdomain := protocol.SubdomainFor(s.Name, reg.Tunnel)
			url := fmt.Sprintf("%s://%s.%s%s", h.cfg.PublicScheme(), subdomain, h.cfg.PublicDomain, publicPortSuffix(h.cfg))
			if err := h.st.PutSlot(reg.Owner, reg.Tunnel, store.Slot{
				Name: s.Name, Kind: s.Kind, Subdomain: subdomain,
				UpstreamHint: s.UpstreamHint, Anonymous: s.Anonymous,
				LastSeenPublicURL: url,
			}); err != nil {
				writeError(ctrl, "store", err.Error(), s.Name)
				return
			}
			h.reg.Bind(&sessions.Entry{Tunnel: reg.Tunnel, Slot: s.Name, Session: sess, OwnerKey: reg.Owner + "/" + reg.Tunnel})
			acks = append(acks, protocol.SlotAck{Name: s.Name, Kind: s.Kind, URL: url})
		case protocol.SlotKindTCP:
			// TCP slots: not in v0.1.
			writeError(ctrl, "unsupported", "tcp slots ship in v0.3", s.Name)
			return
		default:
			writeError(ctrl, "bad_kind", string(s.Kind), s.Name)
			return
		}
	}

	ack := protocol.RegisterAckFrame{Type: protocol.FrameRegisterAck, Tunnel: reg.Tunnel, Slots: acks}
	if err := writeFrame(ctrl, ack); err != nil {
		h.log.Warn("write ack", "err", err)
		return
	}
	h.log.Info("registered", "owner", reg.Owner, "tunnel", reg.Tunnel, "slots", len(reg.Slots))

	// Drain the control channel until the CLI hangs up.
	for {
		line, err := br.ReadBytes('\n')
		if err != nil {
			return
		}
		ftype, err := protocol.DecodeEnvelope(line)
		if err != nil {
			continue
		}
		switch ftype {
		case protocol.FramePing:
			var p protocol.PingFrame
			_ = json.Unmarshal(line, &p)
			_ = writeFrame(ctrl, protocol.PongFrame{Type: protocol.FramePong, Nonce: p.Nonce})
		}
	}
}

func writeFrame(w net.Conn, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_ = w.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_, err = w.Write(b)
	return err
}

func writeError(w net.Conn, code, msg, slot string) {
	_ = writeFrame(w, protocol.ErrorFrame{Type: protocol.FrameError, Code: code, Message: msg, Slot: slot})
}

// publicPortSuffix renders the URL port suffix for emitted public URLs.
// Prefers cfg.PublicHTTPPort when set (Docker host-port mapping case);
// otherwise falls back to cfg.ListenHTTP's port. Returns empty for the
// scheme-default ports (":80" / ":443") so the URL is clean.
func publicPortSuffix(cfg config.Config) string {
	var port string
	switch {
	case cfg.PublicHTTPPort != "":
		port = ":" + strings.TrimPrefix(cfg.PublicHTTPPort, ":")
	default:
		if idx := strings.LastIndexByte(cfg.ListenHTTP, ':'); idx >= 0 {
			port = cfg.ListenHTTP[idx:]
		}
	}
	if port == ":443" || port == ":80" {
		return ""
	}
	return port
}
