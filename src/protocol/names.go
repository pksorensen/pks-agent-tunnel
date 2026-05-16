package protocol

import (
	"fmt"
	"regexp"
	"strings"
)

// SlotSeparator is the literal that separates slot from tunnel in a
// public subdomain: <slot>--<tunnel>.<TLS_DOMAIN>. Reserved — no slot or
// tunnel name may contain it.
const SlotSeparator = "--"

var nameRx = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// ValidateName returns nil if name is a valid slot/tunnel/owner identifier.
// Lowercase, digits, and hyphens; must start with a letter or digit; and
// must not contain the slot-separator "--".
func ValidateName(name string) error {
	if !nameRx.MatchString(name) {
		return fmt.Errorf("invalid name %q: must match [a-z0-9][a-z0-9-]*", name)
	}
	if strings.Contains(name, SlotSeparator) {
		return fmt.Errorf("invalid name %q: reserved separator %q", name, SlotSeparator)
	}
	return nil
}

// SubdomainFor builds the public subdomain for an HTTP slot. It does not
// include the TLS_DOMAIN suffix — callers add that.
func SubdomainFor(slot, tunnel string) string {
	return slot + SlotSeparator + tunnel
}

// ParseSubdomain splits a host header value of the form
//
//	<slot>--<tunnel>.<rest...>
//
// into its slot and tunnel components. The trailing domain is returned as
// the third value (without leading dot). Returns ok=false if host does not
// match the pattern.
func ParseSubdomain(host string) (slot, tunnel, domain string, ok bool) {
	// Strip any port suffix.
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	host = strings.ToLower(host)
	dot := strings.IndexByte(host, '.')
	if dot < 0 {
		return "", "", "", false
	}
	prefix, rest := host[:dot], host[dot+1:]
	sep := strings.Index(prefix, SlotSeparator)
	if sep < 0 {
		return "", "", "", false
	}
	slot = prefix[:sep]
	tunnel = prefix[sep+len(SlotSeparator):]
	if slot == "" || tunnel == "" || ValidateName(slot) != nil || ValidateName(tunnel) != nil {
		return "", "", "", false
	}
	return slot, tunnel, rest, true
}
