// Package store is the folder-backed registry. Every entity is a Markdown
// file whose YAML frontmatter is the structured data. Atomic writes via
// <file>.tmp + rename.
package store

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pksorensen/pks-agent-tunnel/src/protocol"
)

// Tunnel matches tunnels/<owner>/<name>/tunnel.md frontmatter.
type Tunnel struct {
	Name        string    `yaml:"name"`
	Owner       string    `yaml:"owner"`
	Anonymous   bool      `yaml:"anonymous"`
	Description string    `yaml:"description,omitempty"`
	CreatedAt   time.Time `yaml:"createdAt"`
}

// Slot matches tunnels/<owner>/<tunnel>/slots/<name>.md frontmatter.
type Slot struct {
	Name              string             `yaml:"name"`
	Kind              protocol.SlotKind  `yaml:"kind"`
	OwnerTunnel       string             `yaml:"ownerTunnel"`
	Subdomain         string             `yaml:"subdomain,omitempty"`
	StickyPort        int                `yaml:"stickyPort,omitempty"`
	UpstreamHint      string             `yaml:"upstreamHint,omitempty"`
	Anonymous         bool               `yaml:"anonymous"`
	LastSeenPublicURL string             `yaml:"lastSeenPublicUrl,omitempty"`
	CreatedAt         time.Time          `yaml:"createdAt"`
}

// Store is a thin handle over the root directory.
type Store struct {
	Root string
}

// Open ensures the root directory exists and returns a handle.
func Open(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", root, err)
	}
	for _, sub := range []string{"tunnels", "tokens", "ports", "runtime", "tls"} {
		if err := os.MkdirAll(filepath.Join(root, sub), 0o755); err != nil {
			return nil, fmt.Errorf("mkdir %s: %w", sub, err)
		}
	}
	return &Store{Root: root}, nil
}

// Close is a no-op today; reserved for future flushes/lock release.
func (s *Store) Close() error { return nil }

func (s *Store) tunnelPath(owner, name string) string {
	return filepath.Join(s.Root, "tunnels", owner, name)
}

func (s *Store) slotPath(owner, tunnel, slot string) string {
	return filepath.Join(s.tunnelPath(owner, tunnel), "slots", slot+".md")
}

// PutTunnel writes (or updates) a tunnel sidecar.
func (s *Store) PutTunnel(t Tunnel) error {
	if err := protocol.ValidateName(t.Owner); err != nil {
		return fmt.Errorf("owner: %w", err)
	}
	if err := protocol.ValidateName(t.Name); err != nil {
		return fmt.Errorf("tunnel: %w", err)
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now().UTC()
	}
	dir := s.tunnelPath(t.Owner, t.Name)
	if err := os.MkdirAll(filepath.Join(dir, "slots"), 0o755); err != nil {
		return err
	}
	return writeFrontmatter(filepath.Join(dir, "tunnel.md"), t, "")
}

// PutSlot writes (or updates) a slot sidecar.
func (s *Store) PutSlot(owner, tunnel string, slot Slot) error {
	if err := protocol.ValidateName(slot.Name); err != nil {
		return fmt.Errorf("slot: %w", err)
	}
	if slot.CreatedAt.IsZero() {
		slot.CreatedAt = time.Now().UTC()
	}
	slot.OwnerTunnel = owner + "/" + tunnel
	dir := filepath.Join(s.tunnelPath(owner, tunnel), "slots")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return writeFrontmatter(s.slotPath(owner, tunnel, slot.Name), slot, "")
}

// GetSlot loads a slot sidecar; returns os.ErrNotExist if missing.
func (s *Store) GetSlot(owner, tunnel, slot string) (Slot, error) {
	var sl Slot
	if err := readFrontmatter(s.slotPath(owner, tunnel, slot), &sl); err != nil {
		return Slot{}, err
	}
	return sl, nil
}
