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
	requestlog "battle-proxy-akira/internal/logging"
	"battle-proxy-akira/internal/runtime"
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

	manager, err := runtime.NewManager(loadRuntimeConfig, nil)
	if err != nil {
		slog.Error("build runtime", "error", err)
		os.Exit(1)
	}

	clientAuth, err := api.NewClientAuthMiddleware(cfg.ClientAuth)
	if err != nil {
		slog.Error("build client auth", "error", err)
		os.Exit(1)
	}
	logger, err := requestlog.New(cfg.Logging)
	if err != nil {
		slog.Error("build request logger", "error", err)
		os.Exit(1)
	}

	handler := api.NewServer(
		api.WithChatRouter(manager),
		api.WithModelLister(api.ModelListerFunc(manager.Models)),
		api.WithClientAuth(clientAuth),
		api.WithRequestLogger(logger),
		api.WithServerConfig(cfg.Server),
	)
	server := newHTTPServer(cfg.Server, handler)

	shutdownSignals := make(chan os.Signal, 1)
	signal.Notify(shutdownSignals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(shutdownSignals)

	reloadSignals := make(chan os.Signal, 1)
	signal.Notify(reloadSignals, syscall.SIGHUP)
	defer signal.Stop(reloadSignals)
	go runReloadLoop(reloadSignals, manager.Reload)

	slog.Info("starting llm proxy", "addr", server.Addr)
	if err := serve(context.Background(), server, shutdownSignals, defaultShutdownTimeout); err != nil {
		slog.Error("server stopped with error", "error", err)
		os.Exit(1)
	}
}

// runReloadLoop triggers a config reload for every received signal until the
// channel is closed. Reload failures are logged but never terminate the proxy.
func runReloadLoop(signals <-chan os.Signal, reload func() error) {
	for range signals {
		if err := reload(); err != nil {
			slog.Error("config reload failed", "error", err)
			continue
		}
		slog.Info("config reloaded")
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
