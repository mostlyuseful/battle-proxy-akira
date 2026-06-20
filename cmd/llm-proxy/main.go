package main

import (
	"battle-proxy-akira/internal/api"
	"log/slog"
	"net/http"
	"os"
)

func main() {
	addr := os.Getenv("LLM_PROXY_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}

	server := &http.Server{
		Addr:    addr,
		Handler: api.NewServer(),
	}

	slog.Info("starting llm proxy", "addr", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server stopped with error", "error", err)
		os.Exit(1)
	}
}
