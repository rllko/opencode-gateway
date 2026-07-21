package gateway

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sseStream joins data lines into a valid SSE body (each line followed by a
// blank line, then a trailing terminator).
func sseStream(lines ...string) string {
	return strings.Join(lines, "\n\n") + "\n\n"
}

func TestTranslateStreamText(t *testing.T) {
	stream := sseStream(
		`data: {"choices":[{"delta":{"content":"Hel"}}]}`,
		`data: {"choices":[{"delta":{"content":"lo"}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		`data: {"choices":[],"usage":{"prompt_tokens":4,"completion_tokens":2}}`,
		`data: [DONE]`,
	)

	var events []string
	var text strings.Builder
	var usage map[string]any

	translateStream(strings.NewReader(stream),
		"deepseek-v4-flash",
		func(e string, d any) {
			events = append(events, e)

			m := d.(map[string]any)
			if e == "content_block_delta" {
				delta := m["delta"].(map[string]any)
				if delta["type"] == "text_delta" {
					text.WriteString(delta["text"].(string))
				}
			}

			if e == "message_delta" {
				usage = m["usage"].(map[string]any)
			}
		})

	// Test: the Anthropic event order for a text-only stream
	assert.Equal(t, []string{
		"message_start",
		"content_block_start",
		"content_block_delta",
		"content_block_delta",
		"content_block_stop",
		"message_delta",
		"message_stop",
	}, events)

	// Test: the text deltas reassemble to the full answer
	assert.Equal(t, "Hello", text.String())

	// Test: the real upstream usage is passed through, not the delta count
	assert.Equal(t, 4, usage["input_tokens"])
	assert.Equal(t, 2, usage["output_tokens"])
}

func TestTranslateStreamThinking(t *testing.T) {
	stream := sseStream(
		`data: {"choices":[{"delta":{"content":null,"reasoning_content":"Let me"}}]}`,
		`data: {"choices":[{"delta":{"content":null,"reasoning_content":" think."}}]}`,
		`data: {"choices":[{"delta":{"content":"Hi"}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		`data: [DONE]`,
	)

	var events []string
	var thinking, text strings.Builder
	startType := map[int]string{} // block index -> content_block type

	translateStream(strings.NewReader(stream),
		"deepseek-v4-flash",
		func(e string, d any) {
			events = append(events, e)
			m := d.(map[string]any)
			switch e {
			case "content_block_start":
				cb := m["content_block"].(map[string]any)
				startType[m["index"].(int)] = cb["type"].(string)
			case "content_block_delta":
				delta := m["delta"].(map[string]any)
				switch delta["type"] {
				case "thinking_delta":
					thinking.WriteString(delta["thinking"].(string))
				case "text_delta":
					text.WriteString(delta["text"].(string))
				}
			}
		})

	// Test: the thinking block opens, drains, and closes before the text block
	assert.Equal(t, []string{
		"message_start",
		"content_block_start", "content_block_delta", "content_block_delta", // thinking
		"content_block_stop",
		"content_block_start", "content_block_delta", // text
		"content_block_stop",
		"message_delta", "message_stop",
	}, events)

	// Test: thinking is block 0, the visible answer is block 1
	assert.Equal(t, map[int]string{0: "thinking", 1: "text"}, startType)

	// Test: the reasoning deltas reassemble separately from the answer
	assert.Equal(t, "Let me think.", thinking.String())
	assert.Equal(t, "Hi", text.String())
}

func TestTranslateStreamTools(t *testing.T) {
	stream := sseStream(
		`data: {"choices":[{"delta":{"content":"Let me check."}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"get_weather","arguments":"{\"ci"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"ty\":\"Paris\"}"}}]}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: {"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5}}`,
		`data: [DONE]`,
	)

	var events []string
	var argJSON strings.Builder
	var toolName, gotStop string
	translateStream(strings.NewReader(stream), "deepseek-v4-pro", func(e string, d any) {
		events = append(events, e)
		m := d.(map[string]any)
		if e == "content_block_start" {
			if cb, ok := m["content_block"].(map[string]any); ok && cb["type"] == "tool_use" {
				toolName = cb["name"].(string)
			}
		}
		if e == "content_block_delta" {
			delta := m["delta"].(map[string]any)
			if delta["type"] == "input_json_delta" {
				argJSON.WriteString(delta["partial_json"].(string))
			}
		}
		if e == "message_delta" {
			gotStop = m["delta"].(map[string]any)["stop_reason"].(string)
		}
	})

	// Test: the text block is closed before the tool_use block opens
	assert.Equal(t, []string{
		"message_start", "content_block_start", "content_block_delta", // text
		"content_block_stop",                                                // close text
		"content_block_start", "content_block_delta", "content_block_delta", // tool_use + two arg fragments
		"content_block_stop", // close tool
		"message_delta", "message_stop",
	}, events)

	// Test: the tool name is carried on the tool_use block
	assert.Equal(t, "get_weather", toolName)

	// Test: the input_json_delta fragments reassemble to valid JSON
	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(argJSON.String()), &parsed))
	assert.Equal(t, "Paris", parsed["city"])

	// Test: stop_reason is tool_use
	assert.Equal(t, "tool_use", gotStop)
}
