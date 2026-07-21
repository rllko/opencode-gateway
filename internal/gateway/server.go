// Package gateway bridges Claude Desktop (Anthropic API) to opencode's OpenAI-
// compatible "zen/go" endpoint:
//
//	Claude Desktop --Anthropic--> gateway --OpenAI--> opencode.ai/zen/go
//
// endpoints:
//
//	GET  /v1/models   -> Anthropic-format model list (so Desktop discovery works)
//	POST /v1/messages -> translate Anthropic req -> OpenAI, call upstream,
//	                     translate the reply (streaming or not) back to Anthropic
package gateway

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Server struct {
	cfg     Config
	apiKey  string
	client  *http.Client
	models  []Model
	byAlias map[string]Model // Desktop alias -> model (real name + upstream API)
	log     *slog.Logger     // nil unless GATEWAY_LOG is set
	logC    io.Closer        // underlying log file; nil unless GATEWAY_LOG is set
}

func New(cfg Config, apiKey string) *Server {
	byAlias := make(map[string]Model, len(models))
	for _, m := range models {
		byAlias[m.Alias] = m
	}
	lg, lc := openLogger(cfg.LogSpec)
	return &Server{
		cfg:     cfg,
		apiKey:  apiKey,
		client:  &http.Client{Timeout: cfg.HTTPTimeout},
		models:  models,
		byAlias: byAlias,
		log:     lg,
		logC:    lc,
	}
}

func (s *Server) Close() error {
	if s.logC != nil {
		return s.logC.Close()
	}
	return nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", s.handleModels)
	mux.HandleFunc("POST /v1/messages", s.handleMessages)
	mux.HandleFunc("POST /v1/messages/count_tokens", s.handleCountTokens)
	return mux
}

func (s *Server) HasKey() bool { return s.apiKey != "" }

func (s *Server) ModelCount() int { return len(s.models) }

func writeJSON(w http.ResponseWriter, code int, v any) {
	b, _ := json.Marshal(v)
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(code)
	w.Write(b)
}

func errObj(kind, msg string) any {
	return map[string]any{
		"type":  "error",
		"error": map[string]any{"type": kind, "message": msg},
	}
}

// newMsgID returns a unique Anthropic-style message id (msg_<random hex>).
// Each response must carry its own id: a constant one (the old "msg_stream")
func newMsgID() string {
	b := make([]byte, 12)

	_, err := rand.Read(b)
	if err != nil {
		return "msg_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}

	return "msg_" + hex.EncodeToString(b)
}

// logf writes one interception line when logging is enabled, e.g.:
//
//	POST /v1/messages status=200 dur=1.9s model=claude-gllm real=glm-5 stream=true effort=low msgs=5
func (s *Server) logf(r *http.Request, status int, start time.Time, format string, args ...any) {
	if s.log == nil {
		return
	}

	msg := ""
	if format != "" {
		msg = fmt.Sprintf(format, args...)
	}

	s.log.Info(
		msg,
		"method", r.Method,
		"path", r.URL.Path,
		"status", status,
		"dur", time.Since(start).Round(time.Millisecond),
	)
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() { s.logf(r, 200, start, "") }()

	unsupported := map[string]any{
		"supported": false,
	}
	supported := map[string]any{
		"supported": true,
	}

	// there was an attempt here, the desktop app doesnt detect it, fix later
	effort := map[string]any{
		"supported": true,
		"low":       supported,
		"medium":    supported,
		"high":      supported,
		"max":       supported,
		"xhigh":     supported,
	}

	var data []any
	for _, m := range s.models {
		imageInput := map[string]any{"supported": m.Vision}

		data = append(data,
			map[string]any{
				"type":             "model",
				"id":               m.Alias,
				"display_name":     m.Label,
				"created_at":       createdAt,
				"max_input_tokens": m.MaxIn,
				"max_tokens":       m.MaxOut,
				"capabilities": map[string]any{
					"batch":          unsupported,
					"citations":      unsupported,
					"code_execution": unsupported,
					"context_management": map[string]any{
						"supported": false,
					},
					"effort":             effort,
					"image_input":        imageInput,
					"pdf_input":          unsupported,
					"structured_outputs": unsupported,
					"thinking": map[string]any{
						"supported": true,
						"types": map[string]any{
							"adaptive": supported,
							"enabled":  supported,
						},
					},
				},
			})
	}

	writeJSON(
		w,
		200,
		map[string]any{
			"data":     data,
			"has_more": false,
			"first_id": s.models[0].Alias,
			"last_id":  s.models[len(s.models)-1].Alias,
		},
	)
}

func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// log boilerplate
	status := 200
	var detail string
	defer func() { s.logf(r, status, start, "%s", detail) }()

	if s.apiKey == "" {
		status = 401
		writeJSON(w,
			status,
			errObj("no_api_key",
				"no API key: set OPENCODE_API_KEY or put opencode-key.txt next to the executable"))
		return
	}

	var a anthReq
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
		status = 400
		detail = "decode=" + err.Error()
		writeJSON(
			w,
			status,
			errObj("invalid_request", err.Error()),
		)
		return
	}

	real, oreq := s.toOpenAI(a)

	var route string
	up := s.byAlias[a.Model].API

	switch up {
	case zenAPI:
		route = "zen"
	default:
		up = goAPI
		route = "go"
	}

	detail = fmt.Sprintf("model=%s real=%s route=%s stream=%v effort=%s msgs=%d",
		a.Model, real, route, a.Stream, oreq.ReasoningEffort, len(a.Messages))

	resp, err := s.callUpstream(up, oreq)
	if err != nil {
		status = 502
		detail += " connect=" + err.Error()
		writeJSON(w, status, errObj("connect_error", err.Error()))
		return
	}

	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		status = resp.StatusCode
		msg, _ := io.ReadAll(resp.Body)

		snip := strings.TrimSpace(string(msg))
		if len(snip) > 300 {
			snip = snip[:300] + "…"
		}

		detail += " upstream=" + snip
		writeJSON(w, status, errObj("upstream_error", string(msg)))
		return
	}

	if oreq.Stream {
		s.streamBack(w, resp, real)
		return
	}

	var o oaiResp
	err = json.NewDecoder(resp.Body).Decode(&o)
	if err != nil {
		status = 502
		detail += " decode=" + err.Error()
		writeJSON(w, status, errObj("decode_error", err.Error()))
		return
	}

	writeJSON(w, 200, buildMessageResponse(o, real))
}

// handleCountTokens emulates POST /v1/messages/count_tokens with a heuristic
// estimate (~4 chars/token). good enough for Desktop's pre-send display
func (s *Server) handleCountTokens(w http.ResponseWriter, r *http.Request) {
	var a anthReq
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
		writeJSON(w, 400, errObj("invalid_request", err.Error()))
		return
	}
	chars := len(textOf(a.System))
	for _, m := range a.Messages {
		chars += len(textOf(m.Content))
	}
	writeJSON(w, 200, map[string]any{"input_tokens": (chars + 3) / 4})
}
