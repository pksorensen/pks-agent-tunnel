// Package router implements the public-facing HTTP frontend. It looks up
// the slot from the request Host, opens a yamux stream on the owning CLI
// session, writes a StreamMeta line, and reverse-proxies the request.
package router

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/pksorensen/pks-agent-tunnel/src/agent-tunnel-server/internal/config"
	"github.com/pksorensen/pks-agent-tunnel/src/agent-tunnel-server/internal/sessions"
	"github.com/pksorensen/pks-agent-tunnel/src/protocol"
)

// NewHTTP returns an http.Handler that resolves Host → slot → CLI session
// and reverse-proxies through a yamux stream.
func NewHTTP(log *slog.Logger, reg *sessions.Registry, _ config.Config) http.Handler {
	return &httpFrontend{log: log, reg: reg}
}

type httpFrontend struct {
	log *slog.Logger
	reg *sessions.Registry
}

func (h *httpFrontend) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	slot, tunnel, _, ok := protocol.ParseSubdomain(r.Host)
	if !ok {
		http.Error(w, "tunnel: host does not match <slot>--<tunnel>.<domain>", http.StatusNotFound)
		return
	}
	entry := h.reg.Lookup(tunnel, slot)
	if entry == nil {
		http.Error(w, fmt.Sprintf("tunnel: no client connected for %s--%s", slot, tunnel), http.StatusBadGateway)
		return
	}

	// One yamux stream per inbound request. Reverse-proxy via a transport
	// whose DialContext returns the stream wrapped so the StreamMeta header
	// is written before any HTTP bytes leave the box.
	dial := func(ctx context.Context, _, _ string) (net.Conn, error) {
		stream, err := entry.Session.OpenStream()
		if err != nil {
			return nil, err
		}
		_ = stream.SetWriteDeadline(time.Now().Add(5 * time.Second))
		meta, _ := json.Marshal(protocol.StreamMeta{Slot: slot, Remote: r.RemoteAddr})
		if _, err := stream.Write(append(meta, '\n')); err != nil {
			stream.Close()
			return nil, err
		}
		_ = stream.SetWriteDeadline(time.Time{})
		return stream, nil
	}

	// httputil.ReverseProxy wants a target URL — we use a sentinel since
	// the actual dial returns our yamux stream regardless of host.
	target, _ := url.Parse("http://upstream.invalid")

	// Original scheme as seen by the public client. The tunnel terminates
	// TLS, so r.TLS != nil means the user came in over https; we forward to
	// the upstream over plain http and rely on X-Forwarded-Proto for the
	// upstream framework (Next.js, NextAuth, etc.) to know the real scheme
	// when it builds absolute redirect URLs.
	originalProto := "http"
	if r.TLS != nil {
		originalProto = "https"
	}

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.Host = r.Host
			req.Header.Set("X-Forwarded-Proto", originalProto)
			req.Header.Set("X-Forwarded-Host", r.Host)
			// X-Forwarded-For is set automatically by httputil.ReverseProxy.
		},
		Transport: &http.Transport{
			DialContext:           dial,
			MaxIdleConnsPerHost:   -1, // every request gets a fresh stream
			DisableKeepAlives:     true,
			ResponseHeaderTimeout: 30 * time.Second,
		},
	}
	proxy.ServeHTTP(w, r)
}
