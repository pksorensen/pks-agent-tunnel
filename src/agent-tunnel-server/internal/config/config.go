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
	TLS           bool   // false in v0.1
	AuthMode      string // "anonymous" or "token"
}

func (c Config) PublicScheme() string {
	if c.TLS {
		return "https"
	}
	return "http"
}

func Load() Config {
	c := Config{
		UserDataDir:   envOr("USER_DATA_DIR", "./app/user-data"),
		ListenHTTP:    envOr("LISTEN_HTTP", ":8080"),
		ListenControl: envOr("LISTEN_CONTROL", ":7080"),
		PublicDomain:  envOr("TLS_DOMAIN", "localtest.me"),
		AuthMode:      envOr("AUTH_MODE", "anonymous"),
	}

	flag.StringVar(&c.UserDataDir, "user-data-dir", c.UserDataDir, "Root for persisted state ($USER_DATA_DIR)")
	flag.StringVar(&c.ListenHTTP, "listen-http", c.ListenHTTP, "Public HTTP frontend listen address")
	flag.StringVar(&c.ListenControl, "control", c.ListenControl, "Control-plane WSS listen address")
	flag.StringVar(&c.PublicDomain, "public-domain", c.PublicDomain, "Wildcard parent domain (e.g. tunnels.example.com)")
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
