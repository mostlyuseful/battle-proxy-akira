package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"battle-proxy-akira/internal/api"
	"battle-proxy-akira/internal/config"
)

const (
	configPathEnv          = "LLM_PROXY_CONFIG"
	addrOverrideEnv        = "LLM_PROXY_ADDR"
	defaultShutdownTimeout = 10 * time.Second
)

func main() {
	cfg, err := loadRuntimeConfig()
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}

	server := newHTTPServer(cfg.Server, api.NewServer(api.WithServerConfig(cfg.Server)))
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)

	slog.Info("starting llm proxy", "addr", server.Addr)
	if err := serve(context.Background(), server, signals, defaultShutdownTimeout); err != nil {
		slog.Error("server stopped with error", "error", err)
		os.Exit(1)
	}
}

func loadRuntimeConfig() (*config.Config, error) {
	cfg, err := config.Load(os.Getenv(configPathEnv))
	if err != nil {
		return nil, err
	}
	// Preserve the early development env override while allowing JSON config to be the default source of truth.
	if addr := os.Getenv(addrOverrideEnv); addr != "" {
		cfg.Server.Addr = addr
	}
	return cfg, nil
}

func newHTTPServer(cfg config.ServerConfig, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:         cfg.Addr,
		Handler:      handler,
		ReadTimeout:  secondsDuration(cfg.ReadTimeoutSeconds),
		WriteTimeout: secondsDuration(cfg.WriteTimeoutSeconds),
		IdleTimeout:  secondsDuration(cfg.IdleTimeoutSeconds),
	}
}

func secondsDuration(seconds int) time.Duration {
	if seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

func serve(ctx context.Context, server *http.Server, signals <-chan os.Signal, shutdownTimeout time.Duration) error {
	listenErr := make(chan error, 1)
	go func() {
		err := server.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		listenErr <- err
	}()
	return waitForShutdown(ctx, server, signals, shutdownTimeout, listenErr)
}

func waitForShutdown(ctx context.Context, server *http.Server, signals <-chan os.Signal, shutdownTimeout time.Duration, listenErr <-chan error) error {
	select {
	case err := <-listenErr:
		return err
	case <-ctx.Done():
		slog.Info("shutting down llm proxy", "reason", ctx.Err())
		return shutdownServer(server, shutdownTimeout, listenErr)
	case sig := <-signals:
		slog.Info("shutting down llm proxy", "signal", sig.String())
		return shutdownServer(server, shutdownTimeout, listenErr)
	}
}

func shutdownServer(server *http.Server, shutdownTimeout time.Duration, listenErr <-chan error) error {
	if shutdownTimeout <= 0 {
		shutdownTimeout = defaultShutdownTimeout
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
		return err
	}
	return <-listenErr
}
