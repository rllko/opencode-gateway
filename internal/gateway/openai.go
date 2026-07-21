package gateway

import "encoding/json"

type oaiReq struct {
	Model           string      `json:"model"`
	Messages        []oaiMsg    `json:"messages"`
	MaxTokens       int         `json:"max_tokens"`
	Stream          bool        `json:"stream"`
	StreamOptions   *streamOpts `json:"stream_options,omitempty"`
	Temperature     *float64    `json:"temperature,omitempty"`
	TopP            *float64    `json:"top_p,omitempty"`
	ReasoningEffort string      `json:"reasoning_effort,omitempty"` // "low"|"medium"|"high"
	Tools           []oaiTool   `json:"tools,omitempty"`
	ToolChoice      any         `json:"tool_choice,omitempty"` // "auto"|"required"|{function object}
}

type streamOpts struct {
	IncludeUsage bool `json:"include_usage"`
}

type oaiMsg struct {
	Role       string        `json:"role"`
	Content    any           `json:"content,omitempty"`      // string (text-only) or []oaiPart (multimodal)
	ToolCalls  []oaiToolCall `json:"tool_calls,omitempty"`   // assistant messages that call tools
	ToolCallID string        `json:"tool_call_id,omitempty"` // role:"tool" result messages
}

// oaiTool is an OpenAI function tool: {"type":"function","function":{name,description,parameters}}.
type oaiTool struct {
	Type     string      `json:"type"`
	Function oaiToolFunc `json:"function"`
}
type oaiToolFunc struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// oaiToolCall is a completed function call carried on an assistant message.
type oaiToolCall struct {
	ID       string      `json:"id"`
	Type     string      `json:"type"`
	Function oaiFuncCall `json:"function"`
}
type oaiFuncCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON object encoded as a string
}

type oaiPart struct {
	Type     string       `json:"type"`
	Text     string       `json:"text,omitempty"`
	ImageURL *oaiImageURL `json:"image_url,omitempty"`
}
type oaiImageURL struct {
	URL string `json:"url"`
}

// oaiUsage covers the OpenAI usage object incl. cached-input and reasoning details.
type oaiUsage struct {
	PromptTokens        int `json:"prompt_tokens"`
	CompletionTokens    int `json:"completion_tokens"`
	PromptTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
	CompletionTokensDetails struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
}

func (u oaiUsage) parts() (in, out, cache, reasoning int) {
	return u.PromptTokens,
		u.CompletionTokens,
		u.PromptTokensDetails.CachedTokens,
		u.CompletionTokensDetails.ReasoningTokens
}

type oaiResp struct {
	ID      string `json:"id"`
	Choices []struct {
		Message struct {
			Content          string        `json:"content"`
			ReasoningContent string        `json:"reasoning_content"` // reasoning models (DeepSeek, Kimi, GLM…)
			ToolCalls        []oaiToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage oaiUsage `json:"usage"`
}

// oaiToolCallDelta is the incremental tool-call shape in streamed
type oaiToolCallDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type oaiChunk struct {
	Choices []struct {
		Delta struct {
			Content          string             `json:"content"`
			ReasoningContent string             `json:"reasoning_content"`
			ToolCalls        []oaiToolCallDelta `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *oaiUsage `json:"usage"`
}
