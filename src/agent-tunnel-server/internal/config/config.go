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
	ListenControl string
	PublicDomain  string // e.g. localtest.me (dev) or tunnels.agentics.dk (prod)
	// PublicHTTPPort overrides the port emitted in public URLs (TUNNEL_READY,
	// lastSeenPublicUrl). Defaults to the listener port, but in Docker the
	// listener is on the container-internal port (e.g. :8080) while the host
	// maps it to a different port (e.g. :18080) — set PUBLIC_HTTP_PORT=18080
	// so the URLs the CLI emits actually resolve.
	PublicHTTPPort string
	TLS            bool   // false in v0.1
	AuthMode       string // "anonymous" or "token"
}

func (c Config) PublicScheme() string {
	if c.TLS {
		return "https"
	}
	return "http"
}

func Load() Config {
	c := Config{
		UserDataDir:    envOr("USER_DATA_DIR", "./app/user-data"),
		ListenHTTP:     envOr("LISTEN_HTTP", ":8080"),
		ListenControl:  envOr("LISTEN_CONTROL", ":7080"),
		PublicDomain:   envOr("TLS_DOMAIN", "localtest.me"),
		PublicHTTPPort: os.Getenv("PUBLIC_HTTP_PORT"),
		AuthMode:       envOr("AUTH_MODE", "anonymous"),
	}

	flag.StringVar(&c.UserDataDir, "user-data-dir", c.UserDataDir, "Root for persisted state ($USER_DATA_DIR)")
	flag.StringVar(&c.ListenHTTP, "listen-http", c.ListenHTTP, "Public HTTP frontend listen address")
	flag.StringVar(&c.ListenControl, "control", c.ListenControl, "Control-plane WSS listen address")
	flag.StringVar(&c.PublicDomain, "public-domain", c.PublicDomain, "Wildcard parent domain (e.g. tunnels.example.com)")
	flag.StringVar(&c.PublicHTTPPort, "public-http-port", c.PublicHTTPPort, "Port number shown in public URLs (overrides listener port — set when host port differs from container port)")
	flag.BoolVar(&c.TLS, "tls", c.TLS, "Terminate TLS on the public frontend")
	flag.StringVar(&c.AuthMode, "auth-mode", c.AuthMode, "anonymous | token")
	flag.Parse()
	return c
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
