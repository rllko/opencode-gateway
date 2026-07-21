package gateway

import (
	"encoding/json"
	"strings"
)

var stopReason = map[string]string{
	"stop": "end_turn", "length": "max_tokens",
	"content_filter": "end_turn", "tool_calls": "tool_use",
}

// textOf flattens an Anthropic content field (string OR array of blocks) to text.
func textOf(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) == nil {
		var b strings.Builder
		for _, bl := range blocks {
			if bl.Type == "text" {
				b.WriteString(bl.Text)
			}
		}
		return b.String()
	}
	return ""
}

// toOAIContent converts an Anthropic message content field to OpenAI content.
// Text-only content (a bare string, or blocks that are all text) collapses to a
// plain string — the same shape textOf produced, so text/streaming paths are
// unchanged. If the message carries any image block, it returns a []oaiPart
// multimodal array instead: text blocks become {type:"text"} parts and image
// blocks become {type:"image_url"} parts. Base64 sources are wrapped as a
// data: URI; url sources pass the https URL through directly.
func toOAIContent(raw json.RawMessage) any {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []struct {
		Type   string `json:"type"`
		Text   string `json:"text"`
		Source struct {
			Type      string `json:"type"`
			MediaType string `json:"media_type"`
			Data      string `json:"data"`
			URL       string `json:"url"`
		} `json:"source"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	hasImage := false
	for _, bl := range blocks {
		if bl.Type == "image" {
			hasImage = true
			break
		}
	}
	if !hasImage {
		return textOf(raw) // text-only: keep the simple string shape
	}
	var parts []oaiPart
	for _, bl := range blocks {
		switch bl.Type {
		case "text":
			parts = append(parts, oaiPart{Type: "text", Text: bl.Text})
		case "image":
			url := ""
			switch bl.Source.Type {
			case "base64":
				url = "data:" + bl.Source.MediaType + ";base64," + bl.Source.Data
			case "url":
				url = bl.Source.URL
			}
			if url != "" {
				parts = append(parts, oaiPart{Type: "image_url", ImageURL: &oaiImageURL{URL: url}})
			}
		}
	}
	return parts
}

// convTools maps the Anthropic tools array to OpenAI function tools. Each
// Anthropic tool {name,description,input_schema} becomes
// {"type":"function","function":{name,description,parameters:<input_schema>}}.
// Returns nil (so oaiReq omits it) when there are no tools.
func convTools(raw json.RawMessage) []oaiTool {
	if len(raw) == 0 {
		return nil
	}
	var in []struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		InputSchema json.RawMessage `json:"input_schema"`
	}
	if json.Unmarshal(raw, &in) != nil {
		return nil
	}
	var out []oaiTool
	for _, t := range in {
		if t.Name == "" {
			continue
		}
		out = append(out, oaiTool{Type: "function", Function: oaiToolFunc{
			Name: t.Name, Description: t.Description, Parameters: t.InputSchema}})
	}
	return out
}

// convToolChoice maps Anthropic tool_choice to the OpenAI form:
//
//	{"type":"auto"}          -> "auto"
//	{"type":"any"}           -> "required"
//	{"type":"tool","name":x} -> {"type":"function","function":{"name":x}}
//
// Returns nil (field omitted) when absent or unrecognized.
func convToolChoice(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var tc struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}
	if json.Unmarshal(raw, &tc) != nil {
		return nil
	}
	switch tc.Type {
	case "auto":
		return "auto"
	case "any":
		return "required"
	case "tool":
		if tc.Name != "" {
			return map[string]any{"type": "function",
				"function": map[string]any{"name": tc.Name}}
		}
	}
	return nil
}

// convMsg expands one Anthropic message into one or more OpenAI messages.
//   - Plain string / text / image content -> a single message (via toOAIContent).
//   - Assistant content with tool_use blocks -> one assistant message carrying any
//     text plus a tool_calls array (each tool_use.input serialized to an args string).
//   - User content with tool_result blocks -> one role:"tool" message per result
//     (content flattened to text); any stray text becomes a trailing user message.
func convMsg(m anthMsg) []oaiMsg {
	var blocks []anthBlock
	if json.Unmarshal(m.Content, &blocks) != nil {
		// Not a block array (bare string, null, or unparsable) — keep the simple path.
		return []oaiMsg{{Role: m.Role, Content: toOAIContent(m.Content)}}
	}
	hasToolUse, hasToolResult := false, false
	for _, b := range blocks {
		switch b.Type {
		case "tool_use":
			hasToolUse = true
		case "tool_result":
			hasToolResult = true
		}
	}
	if !hasToolUse && !hasToolResult {
		// text and/or image only — unchanged behavior.
		return []oaiMsg{{Role: m.Role, Content: toOAIContent(m.Content)}}
	}
	if hasToolUse {
		var text strings.Builder
		var calls []oaiToolCall
		for _, b := range blocks {
			switch b.Type {
			case "text":
				text.WriteString(b.Text)
			case "tool_use":
				args := string(b.Input)
				if strings.TrimSpace(args) == "" {
					args = "{}"
				}
				calls = append(calls, oaiToolCall{ID: b.ID, Type: "function",
					Function: oaiFuncCall{Name: b.Name, Arguments: args}})
			}
		}
		am := oaiMsg{Role: "assistant", ToolCalls: calls}
		if t := text.String(); t != "" {
			am.Content = t // omit content entirely when the tool call carries no text
		}
		return []oaiMsg{am}
	}
	// hasToolResult: one tool message per result, plus any leftover text as a user msg.
	var res []oaiMsg
	var text strings.Builder
	for _, b := range blocks {
		switch b.Type {
		case "tool_result":
			res = append(res, oaiMsg{Role: "tool", ToolCallID: b.ToolUseID,
				Content: textOf(b.Content)})
		case "text":
			text.WriteString(b.Text)
		}
	}
	if text.Len() > 0 {
		res = append(res, oaiMsg{Role: "user", Content: text.String()})
	}
	return res
}

func (s *Server) toOpenAI(a anthReq) (real string, out oaiReq) {
	real = s.alias[a.Model]
	if real == "" {
		real = s.cfg.DefaultModel
	}
	var msgs []oaiMsg
	if sys := textOf(a.System); sys != "" {
		msgs = append(msgs, oaiMsg{Role: "system", Content: sys})
	}
	for _, m := range a.Messages {
		msgs = append(msgs, convMsg(m)...)
	}
	max := a.MaxTokens
	if max == 0 {
		max = 4096
	}
	out = oaiReq{
		Model: real, Messages: msgs, MaxTokens: max, Stream: a.Stream,
		Temperature: a.Temperature, TopP: a.TopP,
		Tools: convTools(a.Tools), ToolChoice: convToolChoice(a.ToolChoice),
	}
	// Anthropic effort -> OpenAI reasoning_effort. zen's providers accept
	// low/medium/high (verified against every catalog model); the Anthropic-only
	// xhigh/max clamp to high; anything unrecognized is dropped.
	if a.OutputConfig != nil {
		switch a.OutputConfig.Effort {
		case "low", "medium", "high":
			out.ReasoningEffort = a.OutputConfig.Effort
		case "xhigh", "max":
			out.ReasoningEffort = "high"
		}
	}
	if a.Stream {
		out.StreamOptions = &streamOpts{IncludeUsage: true} // get real usage in a final chunk
	}
	return real, out
}

// buildMessageResponse converts a non-streaming OpenAI response into the
// Anthropic message object: a text block first (if any), then a tool_use block
// per tool call (arguments parsed back into an input object), plus the mapped
// stop_reason and the usage totals.
func buildMessageResponse(o oaiResp, real string) map[string]any {
	text, finish := "", "stop"
	reasoning := ""
	var toolCalls []oaiToolCall
	if len(o.Choices) > 0 {
		text = o.Choices[0].Message.Content
		reasoning = o.Choices[0].Message.ReasoningContent
		toolCalls = o.Choices[0].Message.ToolCalls
		finish = o.Choices[0].FinishReason
	}
	stop := stopReason[finish]
	if stop == "" {
		stop = "end_turn"
	}
	var content []any
	if reasoning != "" { // thinking blocks precede the visible answer
		content = append(content, map[string]any{"type": "thinking", "thinking": reasoning})
	}
	if text != "" {
		content = append(content, map[string]any{"type": "text", "text": text})
	}
	for _, tc := range toolCalls {
		var input any
		if json.Unmarshal([]byte(tc.Function.Arguments), &input) != nil || input == nil {
			input = map[string]any{}
		}
		content = append(content, map[string]any{
			"type": "tool_use", "id": tc.ID, "name": tc.Function.Name, "input": input})
	}
	if len(content) == 0 { // no text and no tools — keep an empty text block
		content = append(content, map[string]any{"type": "text", "text": ""})
	}
	in, out, cache, _ := o.Usage.parts()
	return map[string]any{
		"id": firstNonEmpty(o.ID, newMsgID()), "type": "message", "role": "assistant",
		"model": real, "content": content,
		"stop_reason": stop, "stop_sequence": nil,
		"usage": map[string]any{
			"input_tokens": in, "output_tokens": out, "cache_read_input_tokens": cache},
	}
}
