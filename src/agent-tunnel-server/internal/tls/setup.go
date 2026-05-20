// Package tls wires ACME / Let's Encrypt issuance into the server.
//
// Setup returns a *tls.Config that picks issued certs from certmagic's
// cache, plus an HTTP handler that solves ACME HTTP-01 challenges and
// 301-redirects everything else to https. The same *tls.Config is shared
// by the public frontend (:443) and the control plane (:7443) so a single
// wildcard cert covers both.
package tls

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"

	"github.com/caddyserver/certmagic"
	"github.com/libdns/cloudflare"

	"github.com/pksorensen/pks-agent-tunnel/src/agent-tunnel-server/internal/config"
)

// Setup configures certmagic for the wildcard cert covering
// cfg.PublicDomain + *.cfg.PublicDomain and kicks off async issuance.
//
// Until the first cert lands (~30–90s for fresh installs) TLS handshakes
// on the public listeners will fail; that is documented in the proposal.
func Setup(ctx context.Context, cfg config.Config, log *slog.Logger) (*tls.Config, http.Handler, error) {
	if cfg.ACME.DNSProvider != "cloudflare" {
		return nil, nil, fmt.Errorf("unsupported DNS provider %q (v0.2 ships cloudflare only)", cfg.ACME.DNSProvider)
	}

	cm := certmagic.NewDefault()
	cm.Storage = &certmagic.FileStorage{Path: filepath.Join(cfg.UserDataDir, "tls")}

	acme := certmagic.NewACMEIssuer(cm, certmagic.ACMEIssuer{
		CA:     cfg.ACME.Directory,
		Email:  cfg.ACME.Email,
		Agreed: true,
		DNS01Solver: &certmagic.DNS01Solver{
			DNSManager: certmagic.DNSManager{
				DNSProvider: &cloudflare.Provider{APIToken: cfg.ACME.DNSToken},
			},
		},
	})
	cm.Issuers = []certmagic.Issuer{acme}

	domains := []string{cfg.PublicDomain, "*." + cfg.PublicDomain}
	log.Info("ACME: issuing initial cert", "domains", domains, "directory", cfg.ACME.Directory)
	if err := cm.ManageAsync(ctx, domains); err != nil {
		return nil, nil, fmt.Errorf("certmagic manage: %w", err)
	}

	tlsCfg := cm.TLSConfig()
	tlsCfg.NextProtos = append([]string{"h2", "http/1.1"}, tlsCfg.NextProtos...)

	return tlsCfg, acme.HTTPChallengeHandler(redirectToHTTPS()), nil
}

// redirectToHTTPS is the fallback handler chained behind the ACME HTTP-01
// solver: anything that is not a challenge URL gets 301'd to https.
func redirectToHTTPS() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := "https://" + r.Host + r.URL.RequestURI()
		http.Redirect(w, r, target, http.StatusMovedPermanently)
	})
}
