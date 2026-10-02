// Package proxyproto lets the public TLS frontend sit behind an L4 proxy
// (Traefik's TCP router with TLS passthrough on the Coolify box) without
// losing the client address: the proxy prefixes each connection with a PROXY
// protocol header, and RemoteAddr becomes the real client again.
package proxyproto

import (
	"fmt"
	"net"
	"strings"
	"time"

	pp "github.com/pires/go-proxyproto"
)

// ParseTrusted turns "10.0.1.0/24, 10.0.4.13" into networks. Empty input
// means PROXY protocol is off.
func ParseTrusted(list string) ([]*net.IPNet, error) {
	var nets []*net.IPNet
	for _, raw := range strings.Split(list, ",") {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		if !strings.Contains(entry, "/") {
			if ip := net.ParseIP(entry); ip != nil && ip.To4() != nil {
				entry += "/32"
			} else {
				entry += "/128"
			}
		}
		_, n, err := net.ParseCIDR(entry)
		if err != nil {
			return nil, fmt.Errorf("PROXY_PROTOCOL_TRUSTED: %q: %w", raw, err)
		}
		nets = append(nets, n)
	}
	return nets, nil
}

// Policy reads the header only from a trusted proxy, and there it is
// optional, so the proxy and this server can be switched over in either
// order. Anyone else sending a header is refused: otherwise a client
// reaching the published port directly could claim any address.
func Policy(trusted []*net.IPNet) pp.ConnPolicyFunc {
	return func(opts pp.ConnPolicyOptions) (pp.Policy, error) {
		addr, ok := opts.Upstream.(*net.TCPAddr)
		if !ok {
			return pp.REJECT, nil
		}
		for _, n := range trusted {
			if n.Contains(addr.IP) {
				return pp.USE, nil
			}
		}
		return pp.REJECT, nil
	}
}

// Wrap returns ln unchanged when trusted is empty.
func Wrap(ln net.Listener, trusted []*net.IPNet) net.Listener {
	if len(trusted) == 0 {
		return ln
	}
	return &pp.Listener{Listener: ln, ConnPolicy: Policy(trusted), ReadHeaderTimeout: 5 * time.Second}
}
