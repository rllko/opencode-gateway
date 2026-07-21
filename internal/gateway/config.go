package gateway

import (
	"os"
	"time"
)

// Config holds the gateway's runtime settings.
type Config struct {
	Addr         string        // listen address, e.g. "127.0.0.1:3458"
	UpstreamURL  string        // opencode OpenAI-compatible chat endpoint
	DefaultModel string        // used when Desktop sends an unknown alias
	HTTPTimeout  time.Duration // upstream request timeout
}

// DefaultConfig returns the built-in configuration. The port comes from
// $GATEWAY_PORT (default 3458).
func DefaultConfig() Config {
	p := os.Getenv("GATEWAY_PORT")
	if p == "" {
		p = "3458"
	}
	return Config{
		Addr:         "127.0.0.1:" + p,
		UpstreamURL:  "https://opencode.ai/zen/go/v1/chat/completions",
		DefaultModel: "deepseek-v4-pro",
		HTTPTimeout:  10 * time.Minute,
	}
}
