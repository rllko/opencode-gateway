package gateway

import "encoding/json"

type anthReq struct {
	Model        string            `json:"model"`
	System       json.RawMessage   `json:"system"`
	Messages     []anthMsg         `json:"messages"`
	MaxTokens    int               `json:"max_tokens"`
	Stream       bool              `json:"stream"`
	Temperature  *float64          `json:"temperature,omitempty"`
	TopP         *float64          `json:"top_p,omitempty"`
	Tools        json.RawMessage   `json:"tools,omitempty"`       // raw so we can reshape to OpenAI
	ToolChoice   json.RawMessage   `json:"tool_choice,omitempty"` // raw so we can reshape to OpenAI
	OutputConfig *anthOutputConfig `json:"output_config,omitempty"`
}

// anthOutputConfig carries the effort level Claude Desktop's picker sends.
type anthOutputConfig struct {
	Effort string `json:"effort"` // "low"|"medium"|"high"|"xhigh"|"max"
}

type anthMsg struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// anthBlock is one element of an Anthropic message content array, wide enough to
// carry every block kind we care about (text, tool_use, tool_result). Fields for
// kinds a given block isn't are simply left zero.
type anthBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`        // text
	ID        string          `json:"id"`          // tool_use
	Name      string          `json:"name"`        // tool_use
	Input     json.RawMessage `json:"input"`       // tool_use (already a JSON object)
	ToolUseID string          `json:"tool_use_id"` // tool_result
	Content   json.RawMessage `json:"content"`     // tool_result (string OR []text blocks)
}
