// Package sessions tracks live CLI sessions by (tunnel, slot). Lookup is
// used by the HTTP frontend to find which yamux session owns a given slot.
package sessions

import (
	"sync"

	"github.com/hashicorp/yamux"
)

// Entry is one live slot binding.
type Entry struct {
	Tunnel   string
	Slot     string
	Session  *yamux.Session
	OwnerKey string // "owner/tunnel" — used for cleanup on close
}

// Registry is a goroutine-safe map of slot bindings.
type Registry struct {
	mu sync.RWMutex
	// keyed by "tunnel/slot" (no owner — slot names are owner-scoped in
	// storage but resolution from a public subdomain only carries tunnel
	// and slot, so two owners cannot host the same tunnel name at once).
	m map[string]*Entry
}

func New() *Registry {
	return &Registry{m: make(map[string]*Entry)}
}

func key(tunnel, slot string) string { return tunnel + "/" + slot }

// Bind sets (or replaces) the entry for (tunnel, slot). The previous entry,
// if any, is returned so the caller can decide whether to close its
// session (typical: yes — last writer wins).
func (r *Registry) Bind(e *Entry) *Entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	prev := r.m[key(e.Tunnel, e.Slot)]
	r.m[key(e.Tunnel, e.Slot)] = e
	return prev
}

// Lookup returns the entry for (tunnel, slot) or nil.
func (r *Registry) Lookup(tunnel, slot string) *Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.m[key(tunnel, slot)]
}

// UnbindSession removes every entry pointing at s. Used on WS close.
func (r *Registry) UnbindSession(s *yamux.Session) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, e := range r.m {
		if e.Session == s {
			delete(r.m, k)
		}
	}
}
