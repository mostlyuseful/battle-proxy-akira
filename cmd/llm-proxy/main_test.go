package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"battle-proxy-akira/internal/config"
)

func TestNewHTTPServerAppliesConfiguredTimeouts(t *testing.T) {
	t.Parallel()

	cfg := config.ServerConfig{
		Addr:                "127.0.0.1:0",
		ReadTimeoutSeconds:  3,
		WriteTimeoutSeconds: 0,
		IdleTimeoutSeconds:  7,
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	server := newHTTPServer(cfg, handler)

	if server.Addr != cfg.Addr {
		t.Fatalf("Addr = %q, want %q", server.Addr, cfg.Addr)
	}
	if server.ReadTimeout != 3*time.Second {
		t.Fatalf("ReadTimeout = %v, want 3s", server.ReadTimeout)
	}
	if server.WriteTimeout != 0 {
		t.Fatalf("WriteTimeout = %v, want 0 for streaming-friendly config", server.WriteTimeout)
	}
	if server.IdleTimeout != 7*time.Second {
		t.Fatalf("IdleTimeout = %v, want 7s", server.IdleTimeout)
	}
	if server.Handler == nil {
		t.Fatal("Handler is nil")
	}
}

func TestLoadRuntimeConfigWithVerbose(t *testing.T) {
	// Default config (empty path).
	cfg, err := loadRuntimeConfigWithVerbose(runtimeFlags{}, false, nil)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Server.Addr != config.DefaultAddr {
		t.Fatalf("Addr = %q, want default %q", cfg.Server.Addr, config.DefaultAddr)
	}

	// Config from file.
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"server":{"addr":"127.0.0.1:9090"}}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err = loadRuntimeConfigWithVerbose(runtimeFlags{configPath: path}, true, nil)
	if err != nil {
		t.Fatalf("load config file: %v", err)
	}
	if cfg.Server.Addr != "127.0.0.1:9090" {
		t.Fatalf("Addr = %q, want configured addr", cfg.Server.Addr)
	}

	// Flag addr overrides config.
	cfg, err = loadRuntimeConfigWithVerbose(runtimeFlags{configPath: path, addr: "0.0.0.0:3000"}, true, nil)
	if err != nil {
		t.Fatalf("load config with addr flag: %v", err)
	}
	if cfg.Server.Addr != "0.0.0.0:3000" {
		t.Fatalf("Addr = %q, want overridden addr", cfg.Server.Addr)
	}
}

func TestWaitForShutdownAllowsInFlightRequestToFinish(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	server := newHTTPServer(config.ServerConfig{Addr: "127.0.0.1:0"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() { close(started) })
		<-release
		w.WriteHeader(http.StatusNoContent)
	}))
	listenErr, addr := startTestServer(t, server)

	clientDone := make(chan error, 1)
	go func() {
		resp, err := http.Get("http://" + addr + "/healthz")
		if err == nil {
			_ = resp.Body.Close()
		}
		clientDone <- err
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not start")
	}

	signals := make(chan os.Signal, 1)
	shutdownDone := make(chan error, 1)
	go func() {
		shutdownDone <- waitForShutdown(context.Background(), server, signals, 2*time.Second, listenErr)
	}()
	signals <- syscall.SIGTERM

	select {
	case err := <-shutdownDone:
		t.Fatalf("shutdown returned before in-flight request completed: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Fatalf("shutdown returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown did not complete after in-flight request finished")
	}
	if err := <-clientDone; err != nil {
		t.Fatalf("client request error: %v", err)
	}
}

func TestWaitForShutdownBoundsInFlightRequests(t *testing.T) {
	started := make(chan struct{})
	var once sync.Once
	server := newHTTPServer(config.ServerConfig{Addr: "127.0.0.1:0"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() { close(started) })
		<-r.Context().Done()
	}))
	listenErr, addr := startTestServer(t, server)

	clientDone := make(chan error, 1)
	go func() {
		resp, err := http.Get("http://" + addr + "/healthz")
		if err == nil {
			_ = resp.Body.Close()
		}
		clientDone <- err
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not start")
	}

	signals := make(chan os.Signal, 1)
	shutdownDone := make(chan error, 1)
	go func() {
		shutdownDone <- waitForShutdown(context.Background(), server, signals, 50*time.Millisecond, listenErr)
	}()
	signals <- syscall.SIGTERM

	select {
	case err := <-shutdownDone:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("shutdown error = %v, want context deadline exceeded", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown did not honor bounded timeout")
	}
	<-clientDone
}

func startTestServer(t *testing.T, server *http.Server) (<-chan error, string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	listenErr := make(chan error, 1)
	go func() {
		err := server.Serve(ln)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		listenErr <- err
	}()
	return listenErr, addr
}

func TestRunReloadLoopInvokesReloadOnSignal(t *testing.T) {
	t.Parallel()

	signals := make(chan os.Signal, 1)
	done := make(chan struct{})
	var calls int32
	reload := func() error {
		n := atomic.AddInt32(&calls, 1)
		if n == 2 {
			return errors.New("simulated reload failure")
		}
		return nil
	}
	go func() {
		runReloadLoop(signals, reload)
		close(done)
	}()

	signals <- syscall.SIGHUP
	signals <- syscall.SIGHUP
	close(signals)
	<-done

	if calls != 2 {
		t.Fatalf("reload calls = %d, want 2", calls)
	}
}
