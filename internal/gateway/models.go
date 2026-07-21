package gateway

const createdAt = "2025-01-01T00:00:00Z"

// The two opencode upstreams. Every model below points at one of these. To add a
// model: copy a row and pick goAPI or zenAPI — nothing else to touch.
const (
	goAPI  = "https://opencode.ai/zen/go/v1/chat/completions" // Go subscription (paid catalog)
	zenAPI = "https://opencode.ai/zen/v1/chat/completions"    // credit balance (free rotating models)
)

// Model maps a Desktop-facing alias to a real opencode model + picker label,
// with the model's real context and max-output token limits (from opencode's
// models.json) so Desktop's context meter and output caps are accurate.
// Aliases are typo'd on purpose to slip past Desktop's third-party-brand filter.
type Model struct {
	Alias, Real, Label string
	MaxIn, MaxOut      int    // context window, max output tokens
	Vision             bool   // opencode metadata lists image input for this model
	API                string // upstream chat endpoint: goAPI or zenAPI
}

// Aliases are intentionally typo'd (deepsek/kimm/qwenn/gllm/grokk/…). Desktop's
// discovery filter rejects IDs containing recognizable third-party brand names,
// so the misspellings are what let them through; the clean name shows via display_name.
//
// Last column is the upstream: goAPI keeps paid models on the Go subscription so
// they don't burn credit balance; zenAPI is where the free models live. Endpoint
// availability verified against both /v1/models catalogs.
var models = []Model{
	{"claude-deepsek", "deepseek-v4-pro", "DeepSeek V4 Pro", 1000000, 384000, false, goAPI},
	{"claude-deepsek-flash", "deepseek-v4-flash", "DeepSeek V4 Flash", 1000000, 384000, false, goAPI},
	{"claude-kimm", "kimi-k3", "Kimi K3", 1048576, 131072, true, goAPI},
	{"claude-kimm-code", "kimi-k2.7-code", "Kimi K2.7 Code", 262144, 262144, true, goAPI},
	{"claude-qwwenn", "qwen3.7-max", "Qwen3.7 Max", 1000000, 65536, false, goAPI},
	{"claude-qwwenn-plus", "qwen3.7-plus", "Qwen3.7 Plus", 1000000, 65536, true, goAPI},
	{"claude-gllm", "glm-5", "GLM-5", 202752, 32768, false, goAPI},
	{"claude-gllm52", "glm-5.2", "GLM-5.2", 1000000, 131072, false, goAPI},
	{"claude-grookk", "grok-4.5", "Grok 4.5", 500000, 500000, true, goAPI},
	{"claude-minmax", "minimax-m3", "MiniMax M3", 1000000, 131072, true, goAPI},
	{"claude-mmimo", "mimo-v2.5-pro", "MiMo v2.5 Pro", 1048576, 128000, false, goAPI},

	// OpenCode Zen free-tier models (rotating; may disappear). See
	// https://opencode.ai/docs/zen/. TODO(verify): big-pickle and north-mini-code
	// have no known paid twin — the token limits below are conservative guesses,
	// confirm against opencode's models.json before trusting Desktop's context meter.
	{"claude-bigpickle", "big-pickle", "Big Pickle (free)", 131072, 8192, false, zenAPI},
	{"claude-deepsek-flash-free", "deepseek-v4-flash-free", "DeepSeek V4 Flash (free)", 1000000, 384000, false, zenAPI},
	{"claude-mmimo-free", "mimo-v2.5-free", "MiMo v2.5 (free)", 1048576, 128000, false, zenAPI},
	{"claude-north-mini-code", "north-mini-code-free", "North Mini Code (free)", 262144, 32768, false, zenAPI},
}
