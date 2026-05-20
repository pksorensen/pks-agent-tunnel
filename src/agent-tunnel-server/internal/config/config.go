// Package config resolves runtime config from flags + env. Flags win over
// env when both are set.
package config

import (
	"flag"
	"os"
)

type Config struct {
	UserDataDir   string
	ListenHTTP    string
	ListenHTTPS   string
	ListenControl string
	PublicDomain  string // e.g. localtest.me (dev) or tunnels.agentics.dk (prod)
	// PublicHTTPPort overrides the port emitted in public URLs (TUNNEL_READY,
	// lastSeenPublicUrl). Defaults to the listener port, but in Docker the
	// listener is on the container-internal port (e.g. :8080) while the host
	// maps it to a different port (e.g. :18080) — set PUBLIC_HTTP_PORT=18080
	// so the URLs the CLI emits actually resolve.
	PublicHTTPPort string
	AuthMode       string // "anonymous" or "token"
	ACME           ACMEConfig
}

// ACMEConfig holds ACME / Let's Encrypt settings. Empty Email = ACME off
// (v0.1 plain-HTTP behaviour).
type ACMEConfig struct {
	Email       string
	DNSProvider string
	DNSToken    string
	Directory   string
}

func (a ACMEConfig) Enabled() bool { return a.Email != "" }

func (c Config) PublicScheme() string {
	if c.ACME.Enabled() {
		return "https"
	}
	return "http"
}

func Load() Config {
	acme := ACMEConfig{
		Email:       os.Getenv("ACME_EMAIL"),
		DNSProvider: envOr("ACME_DNS_PROVIDER", "cloudflare"),
		DNSToken:    os.Getenv("ACME_DNS_TOKEN"),
		Directory:   envOr("ACME_DIRECTORY", "https://acme-v02.api.letsencrypt.org/directory"),
	}

	// Listener defaults flip when ACME is on: :80 (challenge+redirect),
	// :443 (TLS frontend), :7443 (TLS control plane). Otherwise keep the
	// dev-friendly v0.1 defaults.
	defaultHTTP := ":8080"
	defaultControl := ":7080"
	if acme.Enabled() {
		defaultHTTP = ":80"
		defaultControl = ":7443"
	}

	c := Config{
		UserDataDir:    envOr("USER_DATA_DIR", "./app/user-data"),
		ListenHTTP:     envOr("LISTEN_HTTP", defaultHTTP),
		ListenHTTPS:    envOr("LISTEN_HTTPS", ":443"),
		ListenControl:  envOr("LISTEN_CONTROL", defaultControl),
		PublicDomain:   envOr("TLS_DOMAIN", "localtest.me"),
		PublicHTTPPort: os.Getenv("PUBLIC_HTTP_PORT"),
		AuthMode:       envOr("AUTH_MODE", "anonymous"),
		ACME:           acme,
	}

	flag.StringVar(&c.UserDataDir, "user-data-dir", c.UserDataDir, "Root for persisted state ($USER_DATA_DIR)")
	flag.StringVar(&c.ListenHTTP, "listen-http", c.ListenHTTP, "Public HTTP frontend listen address (ACME: challenge + redirect)")
	flag.StringVar(&c.ListenHTTPS, "listen-https", c.ListenHTTPS, "Public TLS frontend listen address (ACME mode only)")
	flag.StringVar(&c.ListenControl, "control", c.ListenControl, "Control-plane listen address (WSS in ACME mode, WS otherwise)")
	flag.StringVar(&c.PublicDomain, "public-domain", c.PublicDomain, "Wildcard parent domain (e.g. tunnels.example.com)")
	flag.StringVar(&c.PublicHTTPPort, "public-http-port", c.PublicHTTPPort, "Port number shown in public URLs (overrides listener port — set when host port differs from container port)")
	flag.StringVar(&c.AuthMode, "auth-mode", c.AuthMode, "anonymous | token")
	flag.StringVar(&c.ACME.Email, "acme-email", c.ACME.Email, "Email for ACME account; empty disables ACME")
	flag.StringVar(&c.ACME.DNSProvider, "acme-dns-provider", c.ACME.DNSProvider, "libdns provider name (only 'cloudflare' supported)")
	flag.StringVar(&c.ACME.DNSToken, "acme-dns-token", c.ACME.DNSToken, "DNS provider API token")
	flag.StringVar(&c.ACME.Directory, "acme-directory", c.ACME.Directory, "ACME directory URL (default: Let's Encrypt prod)")
	flag.Parse()
	return c
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
