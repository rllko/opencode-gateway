//go:build !tray

// Plain headless entrypoint (default build): run the gateway and block.
// Used for the Linux/WSL build and testing. The Windows tray app is built
// with `-tags tray` (see tray.go).
package main

import (
	"log/slog"
	"net/http"
	"os"

	"opencode-gateway/internal/gateway"
)

func main() {
	cfg := gateway.DefaultConfig()
	key := gateway.LoadAPIKey()
	srv := gateway.New(cfg, key)
	defer srv.Close()
	if key == "" {
		slog.Warn("no API key found — requests will 401 until one is set")
	}
	slog.Info("opencode-gateway starting",
		"addr", cfg.Addr, "models", srv.ModelCount())
	if err := http.ListenAndServe(cfg.Addr, srv.Handler()); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}
