package gateway

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// streamBack wires the HTTP response into translateStream: it sets the SSE
// headers and passes an emit closure that marshals each event and flushes.
func (s *Server) streamBack(w http.ResponseWriter, resp *http.Response, real string) {
	fl, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, 500, errObj("no_flush", "streaming unsupported"))
		return
	}
	h := w.Header()
	h.Set("content-type", "text/event-stream")
	h.Set("cache-control", "no-cache")
	h.Set("connection", "keep-alive")
	w.WriteHeader(200)

	emit := func(event string, data any) {
		b, _ := json.Marshal(data)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
		fl.Flush()
	}
	translateStream(resp.Body, real, emit)
}

// translateStream reads an OpenAI SSE stream and calls emit for each Anthropic
// event, in order. It is kept separate from the HTTP plumbing so it can be
// unit-tested with a synthetic stream and a capturing emit.
func translateStream(body io.Reader, real string, emit func(event string, data any)) {
	emit("message_start", map[string]any{"type": "message_start",
		"message": map[string]any{"id": "msg_stream", "type": "message",
			"role": "assistant", "model": real, "content": []any{},
			"stop_reason": nil, "stop_sequence": nil,
			"usage": map[string]any{"input_tokens": 0, "output_tokens": 0}}})
	emit("content_block_start", map[string]any{"type": "content_block_start",
		"index": 0, "content_block": map[string]any{"type": "text", "text": ""}})

	stop, counted := "end_turn", 0
	var inTok, outTok, cacheTok int
	// Block index 0 is the (eagerly opened) text block. Tool calls each get their
	// own block starting at index 1; toolBlock maps an OpenAI tool_call index to
	// its Anthropic block index. Blocks are kept strictly sequential: the text
	// block is closed when the first tool call arrives, and each tool block is
	// closed when the next one opens (or at end of stream).
	textStopped := false
	sawTool := false
	toolBlock := map[int]int{}
	nextBlock := 1
	curOpenTool := -1 // Anthropic index of the currently open tool block, -1 = none
	closeText := func() {
		if !textStopped {
			emit("content_block_stop", map[string]any{"type": "content_block_stop", "index": 0})
			textStopped = true
		}
	}
	closeTool := func() {
		if curOpenTool >= 0 {
			emit("content_block_stop", map[string]any{"type": "content_block_stop", "index": curOpenTool})
			curOpenTool = -1
		}
	}
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(line[5:])
		if payload == "[DONE]" {
			break
		}
		var c oaiChunk
		if json.Unmarshal([]byte(payload), &c) != nil {
			continue
		}
		if c.Usage != nil { // final usage chunk (choices is empty here)
			inTok, outTok, cacheTok, _ = c.Usage.parts()
		}
		if len(c.Choices) == 0 {
			continue
		}
		if txt := c.Choices[0].Delta.Content; txt != "" && !textStopped {
			counted++
			emit("content_block_delta", map[string]any{"type": "content_block_delta",
				"index": 0, "delta": map[string]any{"type": "text_delta", "text": txt}})
		}
		for _, tc := range c.Choices[0].Delta.ToolCalls {
			sawTool = true
			bidx, seen := toolBlock[tc.Index]
			if !seen {
				closeText() // finish the text block before the first tool block opens
				closeTool() // finish the previous tool block before this one opens
				bidx = nextBlock
				nextBlock++
				toolBlock[tc.Index] = bidx
				curOpenTool = bidx
				emit("content_block_start", map[string]any{"type": "content_block_start",
					"index": bidx, "content_block": map[string]any{"type": "tool_use",
						"id": tc.ID, "name": tc.Function.Name, "input": map[string]any{}}})
			}
			if frag := tc.Function.Arguments; frag != "" {
				emit("content_block_delta", map[string]any{"type": "content_block_delta",
					"index": bidx, "delta": map[string]any{"type": "input_json_delta",
						"partial_json": frag}})
			}
		}
		if fr := c.Choices[0].FinishReason; fr != nil {
			if mapped := stopReason[*fr]; mapped != "" {
				stop = mapped
			}
		}
	}
	if outTok == 0 { // upstream didn't report usage — fall back to the delta count
		outTok = counted
	}
	if sawTool {
		stop = "tool_use"
	}
	closeTool() // close the last open tool block, if any
	closeText() // close the text block (no-op if already closed by a tool)
	emit("message_delta", map[string]any{"type": "message_delta",
		"delta": map[string]any{"stop_reason": stop, "stop_sequence": nil},
		"usage": map[string]any{"input_tokens": inTok, "output_tokens": outTok,
			"cache_read_input_tokens": cacheTok}})
	emit("message_stop", map[string]any{"type": "message_stop"})
}
