// Command agent-tunnel-server runs the public-facing tunnel server.
//
// Defaults are dev-friendly (plain HTTP on :8080, control plane on :7080,
// no auth, no TLS). Production deploys flip on ACME via ACME_EMAIL — see
// docs/deployment.md and docs/adr/0007-native-acme.md.
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/pksorensen/pks-agent-tunnel/src/agent-tunnel-server/internal/config"
	"github.com/pksorensen/pks-agent-tunnel/src/agent-tunnel-server/internal/control"
	"github.com/pksorensen/pks-agent-tunnel/src/agent-tunnel-server/internal/router"
	"github.com/pksorensen/pks-agent-tunnel/src/agent-tunnel-server/internal/sessions"
	"github.com/pksorensen/pks-agent-tunnel/src/agent-tunnel-server/internal/store"
	tlssetup "github.com/pksorensen/pks-agent-tunnel/src/agent-tunnel-server/internal/tls"
)

func main() {
	cfg := config.Load()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	st, err := store.Open(cfg.UserDataDir)
	if err != nil {
		log.Error("open store", "err", err, "dir", cfg.UserDataDir)
		os.Exit(1)
	}
	defer st.Close()

	reg := sessions.New()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	ctrlHandler := control.NewHandler(log, st, reg, cfg)
	httpHandler := router.NewHTTP(log, reg, cfg)

	servers := make([]*http.Server, 0, 3)

	if cfg.ACME.Enabled() {
		tlsCfg, challengeHandler, err := tlssetup.Setup(ctx, cfg, log)
		if err != nil {
			log.Error("acme setup", "err", err)
			os.Exit(1)
		}

		challengeSrv := &http.Server{Addr: cfg.ListenHTTP, Handler: challengeHandler}
		httpsSrv := &http.Server{Addr: cfg.ListenHTTPS, Handler: httpHandler, TLSConfig: tlsCfg}
		ctrlSrv := &http.Server{Addr: cfg.ListenControl, Handler: ctrlHandler, TLSConfig: tlsCfg}

		go runServer(log, "http-challenge", challengeSrv)
		go runServerTLS(log, "https", httpsSrv)
		go runServerTLS(log, "control-tls", ctrlSrv)

		servers = append(servers, challengeSrv, httpsSrv, ctrlSrv)
	} else {
		ctrlSrv := &http.Server{Addr: cfg.ListenControl, Handler: ctrlHandler}
		httpSrv := &http.Server{Addr: cfg.ListenHTTP, Handler: httpHandler}

		go runServer(log, "control", ctrlSrv)
		go runServer(log, "http", httpSrv)

		servers = append(servers, ctrlSrv, httpSrv)
	}

	log.Info("agent-tunnel-server up",
		"user_data_dir", cfg.UserDataDir,
		"listen_http", cfg.ListenHTTP,
		"listen_https", cfg.ListenHTTPS,
		"listen_control", cfg.ListenControl,
		"public_domain", cfg.PublicDomain,
		"public_scheme", cfg.PublicScheme(),
		"acme", cfg.ACME.Enabled(),
	)

	<-ctx.Done()
	log.Info("shutting down")
	for _, s := range servers {
		shutdown(log, s.Addr, s)
	}
}

func runServer(log *slog.Logger, name string, s *http.Server) {
	if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("listener failed", "name", name, "addr", s.Addr, "err", err)
		os.Exit(1)
	}
}

// runServerTLS serves over TLS. The empty cert/key strings tell net/http
// to fall through to TLSConfig.GetCertificate, which certmagic populates.
func runServerTLS(log *slog.Logger, name string, s *http.Server) {
	if s.TLSConfig == nil {
		s.TLSConfig = &tls.Config{}
	}
	if err := s.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("tls listener failed", "name", name, "addr", s.Addr, "err", err)
		os.Exit(1)
	}
}

func shutdown(log *slog.Logger, name string, s *http.Server) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Shutdown(ctx); err != nil {
		log.Warn("shutdown", "name", name, "err", err)
	}
}
