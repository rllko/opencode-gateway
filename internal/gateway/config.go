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
	LogSpec      string        // GATEWAY_LOG: "", "1", or a log file path
}

// DefaultConfig returns the built-in configuration. The port comes from
// $GATEWAY_PORT (default 3458); request logging from $GATEWAY_LOG (off by
// default — "1" writes gateway.log next to the executable); the upstream chat
// endpoint from $GATEWAY_UPSTREAM (default: OpenCode Zen).
//
// Zen (opencode.ai/zen) is used instead of Zen-Go (opencode.ai/zen/go): the
// same account key authenticates both, but the plain Zen catalog is a superset
// (57 vs 22 models) and is the only one carrying the free rotating models
// (big-pickle, *-free). Set GATEWAY_UPSTREAM back to the /zen/go/ URL to revert.
func DefaultConfig() Config {
	p := os.Getenv("GATEWAY_PORT")
	if p == "" {
		p = "3458"
	}
	up := os.Getenv("GATEWAY_UPSTREAM")
	if up == "" {
		up = "https://opencode.ai/zen/v1/chat/completions"
	}
	return Config{
		Addr:         "127.0.0.1:" + p,
		UpstreamURL:  up,
		DefaultModel: "deepseek-v4-pro",
		HTTPTimeout:  10 * time.Minute,
		LogSpec:      os.Getenv("GATEWAY_LOG"),
	}
}
