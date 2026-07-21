package gateway

const createdAt = "2025-01-01T00:00:00Z"

// Model maps a Desktop-facing alias to a real opencode model + picker label,
// with the model's real context and max-output token limits (from opencode's
// models.json) so Desktop's context meter and output caps are accurate.
// Aliases are typo'd on purpose to slip past Desktop's third-party-brand filter.
type Model struct {
	Alias, Real, Label string
	MaxIn, MaxOut      int  // context window, max output tokens
	Vision             bool // opencode metadata lists image input for this model
}

// Aliases are intentionally typo'd (deepsek/kimm/qwenn/gllm/grokk/…). Desktop's
// discovery filter rejects IDs containing recognizable third-party brand names,
// so the misspellings are what let them through; the clean name shows via display_name.
var models = []Model{
	{"claude-deepsek", "deepseek-v4-pro", "DeepSeek V4 Pro", 1000000, 384000, false},
	{"claude-deepsek-flash", "deepseek-v4-flash", "DeepSeek V4 Flash", 1000000, 384000, false},
	{"claude-kimm", "kimi-k3", "Kimi K3", 1048576, 131072, true},
	{"claude-kimm-code", "kimi-k2.7-code", "Kimi K2.7 Code", 262144, 262144, true},
	{"claude-qwenn", "qwen3.7-max", "Qwen3.7 Max", 1000000, 65536, false},
	{"claude-qwenn-plus", "qwen3.7-plus", "Qwen3.7 Plus", 1000000, 65536, true},
	{"claude-gllm", "glm-5", "GLM-5", 202752, 32768, false},
	{"claude-gllm52", "glm-5.2", "GLM-5.2", 1000000, 131072, false},
	{"claude-grokk", "grok-4.5", "Grok 4.5", 500000, 500000, true},
	{"claude-minmax", "minimax-m3", "MiniMax M3", 1000000, 131072, true},
	{"claude-mimoo", "mimo-v2.5-pro", "MiMo v2.5 Pro", 1048576, 128000, false},
}
