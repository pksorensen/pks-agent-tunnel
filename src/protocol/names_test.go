package protocol

import "testing"

func TestParseSubdomain(t *testing.T) {
	cases := []struct {
		host    string
		slot    string
		tunnel  string
		domain  string
		ok      bool
	}{
		{"ws-relay--agentic.localtest.me", "ws-relay", "agentic", "localtest.me", true},
		{"ws-relay--agentic.tunnels.agentics.dk:8080", "ws-relay", "agentic", "tunnels.agentics.dk", true},
		{"foo--bar.example.com", "foo", "bar", "example.com", true},

		{"localhost", "", "", "", false},                       // no dot
		{"plain.example.com", "", "", "", false},               // no separator
		{"a--b--c.example.com", "", "", "", false}, // tunnel would be "b--c" — contains "--", rejected
		{"--bar.example.com", "", "", "", false},               // empty slot
		{"foo--.example.com", "", "", "", false},               // empty tunnel
		{"Foo--Bar.example.com", "foo", "bar", "example.com", true}, // lowercased
	}
	for _, tc := range cases {
		slot, tunnel, domain, ok := ParseSubdomain(tc.host)
		if ok != tc.ok || slot != tc.slot || tunnel != tc.tunnel || domain != tc.domain {
			t.Errorf("ParseSubdomain(%q) = (%q,%q,%q,%v); want (%q,%q,%q,%v)",
				tc.host, slot, tunnel, domain, ok, tc.slot, tc.tunnel, tc.domain, tc.ok)
		}
	}
}

func TestValidateName(t *testing.T) {
	good := []string{"a", "ws-relay", "agentic", "abc123", "1abc"}
	bad := []string{"", "A", "ws_relay", "ws--relay", "-abc", "abc def"}
	for _, n := range good {
		if err := ValidateName(n); err != nil {
			t.Errorf("ValidateName(%q) returned %v; want nil", n, err)
		}
	}
	for _, n := range bad {
		if err := ValidateName(n); err == nil {
			t.Errorf("ValidateName(%q) returned nil; want error", n)
		}
	}
}
