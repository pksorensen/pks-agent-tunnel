package proxyproto

import (
	"io"
	"net"
	"testing"

	pp "github.com/pires/go-proxyproto"
)

// dialThrough connects to ln, optionally writing a PROXY v2 header claiming
// claimed, and returns the RemoteAddr the server side saw (or "" when the
// server refused the connection).
func dialThrough(t *testing.T, ln net.Listener, claimed string) string {
	t.Helper()
	seen := make(chan string, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			seen <- ""
			return
		}
		defer c.Close()
		buf := make([]byte, 4)
		if _, err := io.ReadFull(c, buf); err != nil {
			seen <- ""
			return
		}
		seen <- c.RemoteAddr().String()
	}()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if claimed != "" {
		h := pp.HeaderProxyFromAddrs(2, &net.TCPAddr{IP: net.ParseIP(claimed), Port: 40000}, ln.Addr())
		if _, err := h.WriteTo(conn); err != nil {
			t.Fatal(err)
		}
	}
	_, _ = conn.Write([]byte("ping"))
	return <-seen
}

func listen(t *testing.T, trusted string) net.Listener {
	t.Helper()
	nets, err := ParseTrusted(trusted)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ln := Wrap(raw, nets)
	t.Cleanup(func() { ln.Close() })
	return ln
}

func TestTrustedProxyHeaderIsUsed(t *testing.T) {
	ln := listen(t, "127.0.0.0/8")
	if got := dialThrough(t, ln, "203.0.113.9"); got != "203.0.113.9:40000" {
		t.Fatalf("RemoteAddr = %q, want the claimed client", got)
	}
}

func TestTrustedProxyHeaderIsOptional(t *testing.T) {
	ln := listen(t, "127.0.0.1")
	if got := dialThrough(t, ln, ""); got == "" || got[:10] != "127.0.0.1:" {
		t.Fatalf("RemoteAddr = %q, want the direct peer", got)
	}
}

func TestUntrustedPeerCannotClaimAnAddress(t *testing.T) {
	ln := listen(t, "10.0.1.0/24")
	if got := dialThrough(t, ln, "203.0.113.9"); got != "" {
		t.Fatalf("an untrusted header was accepted: RemoteAddr = %q", got)
	}
	if got := dialThrough(t, ln, ""); got == "" {
		t.Fatal("a direct client without a header must still be served")
	}
}

func TestOffWhenNothingIsTrusted(t *testing.T) {
	raw, _ := net.Listen("tcp", "127.0.0.1:0")
	defer raw.Close()
	if Wrap(raw, nil) != raw {
		t.Fatal("expected the listener unchanged")
	}
	if _, err := ParseTrusted("nope/99"); err == nil {
		t.Fatal("expected an error for a bad entry")
	}
}
