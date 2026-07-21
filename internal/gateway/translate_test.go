package gateway

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTextOf(t *testing.T) {
	// Test: bare string
	assert.Equal(t, "hello", textOf(json.RawMessage(`"hello"`)))

	// Test: all-text blocks are concatenated
	assert.Equal(t, "ab", textOf(json.RawMessage(`[{"type":"text","text":"a"},{"type":"text","text":"b"}]`)))

	// Test: empty input
	assert.Equal(t, "", textOf(json.RawMessage(``)))

	// Test: non-text blocks are ignored
	assert.Equal(t, "x", textOf(json.RawMessage(`[{"type":"text","text":"x"},{"type":"image"}]`)))
}

func TestToOAIContent(t *testing.T) {
	// Test: bare string collapses to a string
	assert.Equal(t, "hi", toOAIContent(json.RawMessage(`"hi"`)))

	// Test: all-text blocks collapse to a string
	assert.Equal(t, "ab", toOAIContent(json.RawMessage(`[{"type":"text","text":"a"},{"type":"text","text":"b"}]`)))

	// Test: a message with images becomes a []oaiPart with a data URI and a passthrough URL
	got := toOAIContent(json.RawMessage(`[{"type":"text","text":"look"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AAAA"}},{"type":"image","source":{"type":"url","url":"https://x/y.png"}}]`))
	parts, ok := got.([]oaiPart)
	require.True(t, ok)
	require.Len(t, parts, 3)
	assert.Equal(t, "text", parts[0].Type)
	assert.Equal(t, "look", parts[0].Text)
	assert.Equal(t, "image_url", parts[1].Type)
	require.NotNil(t, parts[1].ImageURL)
	assert.Equal(t, "data:image/png;base64,AAAA", parts[1].ImageURL.URL)
	assert.Equal(t, "https://x/y.png", parts[2].ImageURL.URL)
}

func TestConvTools(t *testing.T) {
	// Test: no tools -> nil
	assert.Nil(t, convTools(nil))

	// Test: one tool becomes an OpenAI function tool with the schema verbatim
	out := convTools(json.RawMessage(`[{"name":"get_weather","description":"d","input_schema":{"type":"object"}}]`))
	require.Len(t, out, 1)
	assert.Equal(t, "function", out[0].Type)
	assert.Equal(t, "get_weather", out[0].Function.Name)
	assert.Equal(t, "d", out[0].Function.Description)
	assert.JSONEq(t, `{"type":"object"}`, string(out[0].Function.Parameters))

	// Test: a nameless tool is skipped
	assert.Nil(t, convTools(json.RawMessage(`[{"description":"x"}]`)))
}

func TestConvToolChoice(t *testing.T) {
	// Test: absent -> nil
	assert.Nil(t, convToolChoice(nil))

	// Test: auto -> "auto"
	assert.Equal(t, "auto", convToolChoice(json.RawMessage(`{"type":"auto"}`)))

	// Test: any -> "required"
	assert.Equal(t, "required", convToolChoice(json.RawMessage(`{"type":"any"}`)))

	// Test: tool -> function object
	got := convToolChoice(json.RawMessage(`{"type":"tool","name":"foo"}`))
	assert.Equal(t, map[string]any{"type": "function", "function": map[string]any{"name": "foo"}}, got)
}

func TestConvMsg(t *testing.T) {
	// Test: text-only content -> a single message with a string body
	out := convMsg(anthMsg{Role: "user", Content: json.RawMessage(`"hello"`)})
	require.Len(t, out, 1)
	assert.Equal(t, "user", out[0].Role)
	assert.Equal(t, "hello", out[0].Content)

	// Test: assistant tool_use -> one assistant message with tool_calls
	out = convMsg(anthMsg{Role: "assistant", Content: json.RawMessage(`[{"type":"text","text":"sure"},{"type":"tool_use","id":"t1","name":"get_weather","input":{"city":"Paris"}}]`)})
	require.Len(t, out, 1)
	assert.Equal(t, "assistant", out[0].Role)
	assert.Equal(t, "sure", out[0].Content)
	require.Len(t, out[0].ToolCalls, 1)
	assert.Equal(t, "t1", out[0].ToolCalls[0].ID)
	assert.Equal(t, "get_weather", out[0].ToolCalls[0].Function.Name)
	assert.JSONEq(t, `{"city":"Paris"}`, out[0].ToolCalls[0].Function.Arguments)

	// Test: user tool_result -> a role:"tool" message
	out = convMsg(anthMsg{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"t1","content":"18C sunny"}]`)})
	require.Len(t, out, 1)
	assert.Equal(t, "tool", out[0].Role)
	assert.Equal(t, "t1", out[0].ToolCallID)
	assert.Equal(t, "18C sunny", out[0].Content)

	// Test: tool_result plus trailing text -> tool message + user message
	out = convMsg(anthMsg{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"t1","content":"x"},{"type":"text","text":"and now?"}]`)})
	require.Len(t, out, 2)
	assert.Equal(t, "tool", out[0].Role)
	assert.Equal(t, "user", out[1].Role)
	assert.Equal(t, "and now?", out[1].Content)
}

func TestToOpenAI(t *testing.T) {
	srv := New(DefaultConfig(), "test-key")

	// Test: a known alias maps to its real model
	real, out := srv.toOpenAI(anthReq{Model: "claude-gllm", Messages: []anthMsg{{Role: "user", Content: json.RawMessage(`"hi"`)}}})
	assert.Equal(t, "glm-5", real)
	assert.Equal(t, "glm-5", out.Model)

	// Test: an unknown alias falls back to the default model
	real, _ = srv.toOpenAI(anthReq{Model: "nope"})
	assert.Equal(t, "deepseek-v4-pro", real)

	// Test: the system prompt is prepended as a system message
	_, out = srv.toOpenAI(anthReq{Model: "claude-gllm", System: json.RawMessage(`"be brief"`), Messages: []anthMsg{{Role: "user", Content: json.RawMessage(`"hi"`)}}})
	require.Len(t, out.Messages, 2)
	assert.Equal(t, "system", out.Messages[0].Role)
	assert.Equal(t, "be brief", out.Messages[0].Content)

	// Test: max_tokens defaults to 4096 when absent
	_, out = srv.toOpenAI(anthReq{Model: "claude-gllm"})
	assert.Equal(t, 4096, out.MaxTokens)

	// Test: streaming sets stream_options.include_usage
	_, out = srv.toOpenAI(anthReq{Model: "claude-gllm", Stream: true})
	require.NotNil(t, out.StreamOptions)
	assert.True(t, out.StreamOptions.IncludeUsage)
}

func TestBuildMessageResponse(t *testing.T) {
	// Test: a text-only response
	var o oaiResp
	require.NoError(t, json.Unmarshal([]byte(`{"id":"x","choices":[{"message":{"content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":3}}`), &o))
	got := buildMessageResponse(o, "glm-5")
	assert.Equal(t, "message", got["type"])
	assert.Equal(t, "glm-5", got["model"])
	assert.Equal(t, "end_turn", got["stop_reason"])
	content := got["content"].([]any)
	require.Len(t, content, 1)
	assert.Equal(t, "text", content[0].(map[string]any)["type"])
	assert.Equal(t, "hello", content[0].(map[string]any)["text"])
	usage := got["usage"].(map[string]any)
	assert.Equal(t, 5, usage["input_tokens"])
	assert.Equal(t, 3, usage["output_tokens"])

	// Test: tool_calls become tool_use blocks with stop_reason tool_use
	o = oaiResp{}
	require.NoError(t, json.Unmarshal([]byte(`{"choices":[{"message":{"content":"","tool_calls":[{"id":"t1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"Paris\"}"}}]},"finish_reason":"tool_calls"}]}`), &o))
	got = buildMessageResponse(o, "deepseek-v4-pro")
	assert.Equal(t, "tool_use", got["stop_reason"])
	content = got["content"].([]any)
	require.Len(t, content, 1)
	block := content[0].(map[string]any)
	assert.Equal(t, "tool_use", block["type"])
	assert.Equal(t, "get_weather", block["name"])
	assert.Equal(t, map[string]any{"city": "Paris"}, block["input"])

	// Test: an empty message keeps a single empty text block
	o = oaiResp{}
	require.NoError(t, json.Unmarshal([]byte(`{"choices":[{"message":{"content":""},"finish_reason":"stop"}]}`), &o))
	got = buildMessageResponse(o, "glm-5")
	content = got["content"].([]any)
	require.Len(t, content, 1)
	assert.Equal(t, "", content[0].(map[string]any)["text"])
}
