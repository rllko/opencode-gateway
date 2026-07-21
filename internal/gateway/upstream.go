package gateway

import (
	"bytes"
	"encoding/json"
	"net/http"
)

func (s *Server) callUpstream(url string, body oaiReq) (*http.Response, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("authorization", "Bearer "+s.apiKey)
	req.Header.Set("content-type", "application/json")
	req.Header.Set("accept", "application/json")
	// Cloudflare (error 1010) blocks the default Go/Python client UA.
	req.Header.Set("user-agent", "Mozilla/5.0 (opencode-gateway)")
	return s.client.Do(req)
}
