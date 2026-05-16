package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pksorensen/pks-agent-tunnel/src/protocol"
)

func TestPutAndGetSlot(t *testing.T) {
	root := t.TempDir()
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if err := s.PutTunnel(Tunnel{Owner: "agentics", Name: "agentic", Anonymous: true}); err != nil {
		t.Fatal(err)
	}
	in := Slot{
		Name:         "ws-relay",
		Kind:         protocol.SlotKindHTTP,
		Subdomain:    "ws-relay--agentic",
		UpstreamHint: "ws-relay:3000",
		Anonymous:    true,
	}
	if err := s.PutSlot("agentics", "agentic", in); err != nil {
		t.Fatal(err)
	}

	out, err := s.GetSlot("agentics", "agentic", "ws-relay")
	if err != nil {
		t.Fatal(err)
	}
	if out.Name != "ws-relay" || out.Kind != protocol.SlotKindHTTP || out.Subdomain != "ws-relay--agentic" {
		t.Errorf("slot round-trip mismatch: %+v", out)
	}
	if out.OwnerTunnel != "agentics/agentic" {
		t.Errorf("OwnerTunnel = %q; want agentics/agentic", out.OwnerTunnel)
	}
	if out.CreatedAt.IsZero() {
		t.Errorf("CreatedAt should have been auto-set")
	}

	raw, _ := os.ReadFile(filepath.Join(root, "tunnels", "agentics", "agentic", "slots", "ws-relay.md"))
	if !strings.HasPrefix(string(raw), "---\n") {
		t.Errorf("expected frontmatter, got: %q", raw[:30])
	}
}

func TestPutSlotRejectsBadName(t *testing.T) {
	root := t.TempDir()
	s, _ := Open(root)
	defer s.Close()

	err := s.PutSlot("agentics", "agentic", Slot{Name: "ws--relay", Kind: protocol.SlotKindHTTP})
	if err == nil {
		t.Fatal("expected error for slot name containing '--'")
	}
}
