// Package gateway bridges Claude Desktop (Anthropic API) to opencode's OpenAI-
// compatible "zen/go" endpoint:
//
//	Claude Desktop --Anthropic--> gateway --OpenAI--> opencode.ai/zen/go
//
// Endpoints:
//
//	GET  /v1/models   -> Anthropic-format model list (so Desktop discovery works)
//	POST /v1/messages -> translate Anthropic req -> OpenAI, call upstream,
//	                     translate the reply (streaming or not) back to Anthropic
//
// The cmd/opencode-gateway entry points construct a Server with New and serve
// Server.Handler(); the API key is loaded with LoadAPIKey. Stdlib only.
package gateway

import (
	"encoding/json"
	"io"
	"net/http"
)

// Server holds the gateway's dependencies; the HTTP handlers are methods on it.
type Server struct {
	cfg    Config
	apiKey string
	client *http.Client
	models []Model
	alias  map[string]string // Desktop alias -> real opencode model
}

// New builds a Server and its alias index from the model registry.
func New(cfg Config, apiKey string) *Server {
	alias := make(map[string]string, len(models))
	for _, m := range models {
		alias[m.Alias] = m.Real
	}
	return &Server{
		cfg:    cfg,
		apiKey: apiKey,
		client: &http.Client{Timeout: cfg.HTTPTimeout},
		models: models,
		alias:  alias,
	}
}

// Handler builds the HTTP mux with the gateway's routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", s.handleModels)
	mux.HandleFunc("POST /v1/messages", s.handleMessages)
	mux.HandleFunc("POST /v1/messages/count_tokens", s.handleCountTokens)
	return mux
}

// HasKey reports whether an opencode API key was loaded.
func (s *Server) HasKey() bool { return s.apiKey != "" }

// ModelCount returns the number of models the gateway serves.
func (s *Server) ModelCount() int { return len(s.models) }

func writeJSON(w http.ResponseWriter, code int, v any) {
	b, _ := json.Marshal(v)
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(code)
	w.Write(b)
}

func errObj(kind, msg string) any {
	return map[string]any{"type": "error",
		"error": map[string]any{"type": kind, "message": msg}}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func (s *Server) handleModels(w http.ResponseWriter, _ *http.Request) {
	unsupported := map[string]any{"supported": false}
	var data []any
	for _, m := range s.models {
		imageInput := map[string]any{"supported": m.Vision}
		data = append(data, map[string]any{
			"type": "model", "id": m.Alias, "display_name": m.Label,
			"created_at": createdAt, "max_input_tokens": m.MaxIn, "max_tokens": m.MaxOut,
			"capabilities": map[string]any{
				"batch": unsupported, "citations": unsupported, "code_execution": unsupported,
				"context_management": map[string]any{"supported": false},
				"effort":             map[string]any{"supported": false},
				"image_input":        imageInput, "pdf_input": unsupported, "structured_outputs": unsupported,
				"thinking": map[string]any{"supported": false,
					"types": map[string]any{"adaptive": unsupported, "enabled": unsupported}},
			},
		})
	}
	writeJSON(w, 200, map[string]any{
		"data": data, "has_more": false,
		"first_id": s.models[0].Alias, "last_id": s.models[len(s.models)-1].Alias,
	})
}

func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	if s.apiKey == "" {
		writeJSON(w, 401, errObj("no_api_key",
			"no API key: set OPENCODE_API_KEY or put opencode-key.txt next to the executable"))
		return
	}
	var a anthReq
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
		writeJSON(w, 400, errObj("invalid_request", err.Error()))
		return
	}
	real, oreq := s.toOpenAI(a)
	resp, err := s.callUpstream(oreq)
	if err != nil {
		writeJSON(w, 502, errObj("connect_error", err.Error()))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		msg, _ := io.ReadAll(resp.Body)
		writeJSON(w, resp.StatusCode, errObj("upstream_error", string(msg)))
		return
	}
	if oreq.Stream {
		s.streamBack(w, resp, real)
		return
	}
	var o oaiResp
	if err := json.NewDecoder(resp.Body).Decode(&o); err != nil {
		writeJSON(w, 502, errObj("decode_error", err.Error()))
		return
	}
	writeJSON(w, 200, buildMessageResponse(o, real))
}

// handleCountTokens emulates POST /v1/messages/count_tokens with a heuristic
// estimate (~4 chars/token). Good enough for Desktop's pre-send display; the
// authoritative count still comes back in each response's usage.
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
