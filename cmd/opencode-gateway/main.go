//go:build !tray

// Plain headless entrypoint (default build): run the gateway and block.
// Used for the Linux/WSL build and testing. The Windows tray app is built
// with `-tags tray` (see tray.go).
package main

import (
	"log"
	"net/http"

	"opencode-gateway/internal/gateway"
)

func main() {
	cfg := gateway.DefaultConfig()
	key := gateway.LoadAPIKey()
	srv := gateway.New(cfg, key)
	if key == "" {
		log.Printf("WARNING: no API key found — requests will 401 until one is set")
	}
	log.Printf("opencode-gateway on http://%s -> %s (%d models)", cfg.Addr, cfg.UpstreamURL, srv.ModelCount())
	log.Fatal(http.ListenAndServe(cfg.Addr, srv.Handler()))
}
