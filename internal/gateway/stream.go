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

	stop, counted := "end_turn", 0
	var inTok, outTok, cacheTok int
	// Blocks open lazily and run strictly sequentially: reasoning_content opens
	// a thinking block, content a text block, each tool call a tool_use block.
	// The open block is closed when the next kind starts (or at end of stream),
	// so the thinking always precedes the visible answer, as Anthropic expects.
	// toolBlock maps an OpenAI tool_call index to its Anthropic block index.
	const (
		blkNone = iota
		blkThinking
		blkText
		blkTool
	)
	open, openIdx := blkNone, -1 // kind + Anthropic index of the open block
	nextBlock := 0
	sawTool := false
	toolBlock := map[int]int{}
	closeCur := func() {
		if open == blkNone {
			return
		}
		emit("content_block_stop", map[string]any{"type": "content_block_stop", "index": openIdx})
		open, openIdx = blkNone, -1
	}
	startBlock := func(kind int, block map[string]any) int {
		closeCur()
		idx := nextBlock
		nextBlock++
		emit("content_block_start", map[string]any{"type": "content_block_start",
			"index": idx, "content_block": block})
		open, openIdx = kind, idx
		return idx
	}
	delta := func(idx int, d map[string]any) {
		emit("content_block_delta", map[string]any{"type": "content_block_delta",
			"index": idx, "delta": d})
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
		d := c.Choices[0].Delta
		if r := d.ReasoningContent; r != "" {
			if open != blkThinking {
				startBlock(blkThinking, map[string]any{"type": "thinking", "thinking": ""})
			}
			delta(openIdx, map[string]any{"type": "thinking_delta", "thinking": r})
		}
		if txt := d.Content; txt != "" {
			counted++
			if open != blkText {
				startBlock(blkText, map[string]any{"type": "text", "text": ""})
			}
			delta(openIdx, map[string]any{"type": "text_delta", "text": txt})
		}
		for _, tc := range d.ToolCalls {
			sawTool = true
			bidx, seen := toolBlock[tc.Index]
			if !seen {
				bidx = startBlock(blkTool, map[string]any{"type": "tool_use",
					"id": tc.ID, "name": tc.Function.Name, "input": map[string]any{}})
				toolBlock[tc.Index] = bidx
			}
			if frag := tc.Function.Arguments; frag != "" {
				delta(bidx, map[string]any{"type": "input_json_delta",
					"partial_json": frag})
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
	closeCur()
	if nextBlock == 0 { // upstream sent nothing — keep one empty text block
		startBlock(blkText, map[string]any{"type": "text", "text": ""})
		closeCur()
	}
	emit("message_delta", map[string]any{"type": "message_delta",
		"delta": map[string]any{"stop_reason": stop, "stop_sequence": nil},
		"usage": map[string]any{"input_tokens": inTok, "output_tokens": outTok,
			"cache_read_input_tokens": cacheTok}})
	emit("message_stop", map[string]any{"type": "message_stop"})
}
