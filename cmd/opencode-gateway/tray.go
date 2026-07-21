//go:build tray

// System-tray entrypoint (Windows): shows a taskbar icon with Pause/Resume/Quit.
// Build:  GOOS=windows GOARCH=amd64 go build -tags tray -o opencode-gateway.exe
// The HTTP server runs in a goroutine; Pause calls Shutdown, Resume restarts it.
package main

import (
	"context"
	_ "embed"
	"log"
	"net/http"
	"sync"
	"time"

	"fyne.io/systray"

	"opencode-gateway/internal/gateway"
)

//go:embed icon.ico
var iconData []byte

var (
	mu  sync.Mutex
	srv *http.Server
	gw  *gateway.Server
	cfg gateway.Config
)

func startServer() {
	mu.Lock()
	defer mu.Unlock()
	if srv != nil {
		return
	}
	srv = &http.Server{Addr: cfg.Addr, Handler: gw.Handler()}
	go func(s *http.Server) {
		if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("server error: %v", err)
		}
	}(srv)
}

func stopServer() {
	mu.Lock()
	s := srv
	srv = nil
	mu.Unlock()
	if s != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.Shutdown(ctx)
	}
}

func main() {
	cfg = gateway.DefaultConfig()
	gw = gateway.New(cfg, gateway.LoadAPIKey())
	systray.Run(onReady, stopServer)
}

func onReady() {
	systray.SetIcon(iconData)
	systray.SetTitle("opencode-gateway")
	systray.SetTooltip("opencode-gateway — running on " + cfg.Addr)

	mStatus := systray.AddMenuItem("Running on "+cfg.Addr, "")
	mStatus.Disable()
	if !gw.HasKey() {
		mStatus.SetTitle("⚠ No API key (add opencode-key.txt next to the exe)")
	}
	systray.AddSeparator()
	mToggle := systray.AddMenuItem("Pause", "Pause or resume the gateway")
	mQuit := systray.AddMenuItem("Quit", "Stop the gateway and exit")

	startServer()
	running := true

	for {
		select {
		case <-mToggle.ClickedCh:
			if running {
				stopServer()
				mToggle.SetTitle("Resume")
				mStatus.SetTitle("Paused")
				systray.SetTooltip("opencode-gateway — paused")
			} else {
				startServer()
				mToggle.SetTitle("Pause")
				mStatus.SetTitle("Running on " + cfg.Addr)
				systray.SetTooltip("opencode-gateway — running on " + cfg.Addr)
			}
			running = !running
		case <-mQuit.ClickedCh:
			systray.Quit()
			return
		}
	}
}
