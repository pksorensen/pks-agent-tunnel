package control

import (
	"testing"

	"github.com/pksorensen/pks-agent-tunnel/src/agent-tunnel-server/internal/config"
)

func TestPublicPortSuffix(t *testing.T) {
	cases := []struct {
		name string
		cfg  config.Config
		want string
	}{
		{"listener default", config.Config{ListenHTTP: ":8080"}, ":8080"},
		{"listener host-bound", config.Config{ListenHTTP: "0.0.0.0:8080"}, ":8080"},
		{"listener 443 → empty", config.Config{ListenHTTP: ":443", TLS: true}, ""},
		{"listener 80 → empty", config.Config{ListenHTTP: ":80"}, ""},
		{"public-port overrides listener", config.Config{ListenHTTP: ":8080", PublicHTTPPort: "18080"}, ":18080"},
		{"public-port with leading colon", config.Config{ListenHTTP: ":8080", PublicHTTPPort: ":18080"}, ":18080"},
		{"public-port 443 → empty", config.Config{ListenHTTP: ":8080", PublicHTTPPort: "443"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := publicPortSuffix(tc.cfg); got != tc.want {
				t.Errorf("publicPortSuffix() = %q; want %q", got, tc.want)
			}
		})
	}
}
