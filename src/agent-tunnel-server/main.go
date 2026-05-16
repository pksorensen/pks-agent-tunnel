// Command agent-tunnel-server runs the public-facing tunnel server.
//
// Defaults are dev-friendly (plain HTTP on :8080, control plane on :7080,
// no auth, no TLS). Production deploys override via env vars — see
// docs/deployment.md.
package main

import (
	"context"
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

	// Control plane
	ctrlHandler := control.NewHandler(log, st, reg, cfg)
	ctrlSrv := &http.Server{Addr: cfg.ListenControl, Handler: ctrlHandler}
	go runServer(log, "control", ctrlSrv)

	// Public HTTP frontend (TLS termination + subdomain routing)
	httpHandler := router.NewHTTP(log, reg, cfg)
	httpSrv := &http.Server{Addr: cfg.ListenHTTP, Handler: httpHandler}
	go runServer(log, "http", httpSrv)

	log.Info("agent-tunnel-server up",
		"user_data_dir", cfg.UserDataDir,
		"listen_http", cfg.ListenHTTP,
		"listen_control", cfg.ListenControl,
		"public_domain", cfg.PublicDomain,
		"public_scheme", cfg.PublicScheme(),
	)

	<-ctx.Done()
	log.Info("shutting down")
	shutdown(log, "control", ctrlSrv)
	shutdown(log, "http", httpSrv)
}

func runServer(log *slog.Logger, name string, s *http.Server) {
	if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("listener failed", "name", name, "addr", s.Addr, "err", err)
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
