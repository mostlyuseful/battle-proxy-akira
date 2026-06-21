package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"battle-proxy-akira/internal/api"
	"battle-proxy-akira/internal/config"
	requestlog "battle-proxy-akira/internal/logging"
	"battle-proxy-akira/internal/metrics"
	"battle-proxy-akira/internal/runtime"
)

const (
	defaultShutdownTimeout = 10 * time.Second
)

// runtimeFlags bundles command-line flags for building a runtime config.
type runtimeFlags struct {
	configPath string
	addr       string
}

func main() {
	flags := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	verbose := flags.Bool("verbose", false, "log informational and debug messages")
	help := flags.Bool("help", false, "show usage information")
	cfgPath := flags.String("config", "", "path to JSON config file")
	addr := flags.String("addr", "", "server listen address (overrides config)")
	if err := flags.Parse(os.Args[1:]); err != nil {
		slog.Error("parse flags", "error", err)
		os.Exit(1)
	}
	if *help {
		flags.SetOutput(os.Stdout)
		flags.Usage()
		os.Exit(0)
	}

	rf := runtimeFlags{configPath: *cfgPath, addr: *addr}

	cfg, err := loadRuntimeConfigWithVerbose(rf, *verbose, slog.Default())
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}

	manager, err := runtime.NewManager(func() (*config.Config, error) {
		return loadRuntimeConfig(rf)
	}, nil)
	if err != nil {
		slog.Error("build runtime", "error", err)
		os.Exit(1)
	}
	if *verbose {
		slog.Default().Info("runtime manager built", "providers", len(manager.Current().Providers))
	}

	metricsCollector := metrics.NewCollector()
	if *verbose {
		slog.Default().Info("metrics collector initialized")
	}

	clientAuth, err := api.NewClientAuthMiddleware(cfg.ClientAuth)
	if err != nil {
		slog.Error("build client auth", "error", err)
		os.Exit(1)
	}
	if *verbose {
		slog.Default().Info("client auth configured", "mode", cfg.ClientAuth.Mode)
	}

	logger, err := requestlog.New(cfg.Logging)
	if err != nil {
		slog.Error("build request logger", "error", err)
		os.Exit(1)
	}
	if *verbose {
		slog.Default().Info("request logger configured", "enabled", cfg.Logging.Enabled, "mode", cfg.Logging.Mode, "path", cfg.Logging.Path)
	}

	apiOptions := []api.Option{
		api.WithChatRouter(manager),
		api.WithModelLister(api.ModelListerFunc(manager.Models)),
		api.WithClientAuth(clientAuth),
		api.WithRequestLogger(logger),
		api.WithServerConfig(cfg.Server),
		api.WithMetrics(metricsCollector),
	}
	if *verbose {
		apiOptions = append(apiOptions, api.WithLogger(slog.Default()))
	}
	handler := api.NewServer(apiOptions...)
	if *verbose {
		slog.Default().Info("api server built", "addr", cfg.Server.Addr, "max_body_bytes", cfg.Server.MaxBodyBytes)
	}
	server := newHTTPServer(cfg.Server, handler)
	if *verbose {
		slog.Default().Info("http server configured", "addr", server.Addr, "read_timeout", secondsDuration(cfg.Server.ReadTimeoutSeconds), "write_timeout", secondsDuration(cfg.Server.WriteTimeoutSeconds), "idle_timeout", secondsDuration(cfg.Server.IdleTimeoutSeconds))
	}

	shutdownSignals := make(chan os.Signal, 1)
	signal.Notify(shutdownSignals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(shutdownSignals)

	reloadSignals := make(chan os.Signal, 1)
	signal.Notify(reloadSignals, syscall.SIGHUP)
	defer signal.Stop(reloadSignals)
	go runReloadLoop(reloadSignals, manager.Reload)
	if *verbose {
		slog.Default().Info("reload signal handler configured", "signal", "SIGHUP")
	}

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

func loadRuntimeConfig(rf runtimeFlags) (*config.Config, error) {
	return loadRuntimeConfigWithVerbose(rf, false, nil)
}

func loadRuntimeConfigWithVerbose(rf runtimeFlags, verbose bool, logger *slog.Logger) (*config.Config, error) {
	cfg, err := config.Load(rf.configPath)
	if err != nil {
		return nil, err
	}
	if verbose && logger != nil {
		if rf.configPath == "" {
			logger.Info("using default configuration", "config_path", "")
		} else {
			logger.Info("loaded configuration file", "config_path", rf.configPath)
		}
	}
	if rf.addr != "" {
		cfg.Server.Addr = rf.addr
		if verbose && logger != nil {
			logger.Info("overrode config server address from flag", "addr", rf.addr)
		}
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
