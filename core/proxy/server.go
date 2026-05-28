package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync/atomic"

	"github.com/lfhy/openai-tool-bridge/core/bridge"
)

type Server struct {
	cfg    Config
	client *http.Client
	mux    *http.ServeMux
	rr     atomic.Uint64
}

func NewServer(cfg Config, client *http.Client) *Server {
	if client == nil {
		client = http.DefaultClient
	}
	s := &Server{
		cfg:    cfg,
		client: client,
		mux:    http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("/healthz", s.handleHealthz)
	s.mux.HandleFunc("/v1/models", s.handleModels)
	s.mux.HandleFunc("/v1/chat/completions", s.handleChatCompletions)
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
	})
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	if s.cfg.UpstreamModel != "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"object": "list",
			"data": []map[string]any{
				{
					"id":       s.cfg.UpstreamModel,
					"object":   "model",
					"owned_by": "openai-tool-bridge",
				},
			},
		})
		return
	}
	s.proxyPassthrough(w, r)
}

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{
			"error": map[string]any{
				"message": "method not allowed",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": map[string]any{
				"message": err.Error(),
				"type":    "invalid_request_error",
			},
		})
		return
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": map[string]any{
				"message": "invalid json body",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	if s.cfg.UpstreamModel != "" {
		payload["model"] = s.cfg.UpstreamModel
	}
	if _, err := bridge.ApplyToolPromptBridge(payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": map[string]any{
				"message": err.Error(),
				"type":    "invalid_request_error",
			},
		})
		return
	}

	stream, _ := payload["stream"].(bool)
	upstreamBody, err := json.Marshal(payload)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": map[string]any{
				"message": err.Error(),
				"type":    "server_error",
			},
		})
		return
	}

	resp, err := s.doUpstream(r, http.MethodPost, r.URL.Path, "application/json", upstreamBody)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error": map[string]any{
				"message": err.Error(),
				"type":    "upstream_error",
			},
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		copyHeaders(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
		return
	}

	if stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, _ := w.(http.Flusher)
		writer := &flushWriter{writer: w, flusher: flusher}
		if err := bridge.RewriteStream(writer, resp.Body); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{
				"error": map[string]any{
					"message": err.Error(),
					"type":    "upstream_error",
				},
			})
		}
		return
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error": map[string]any{
				"message": err.Error(),
				"type":    "upstream_error",
			},
		})
		return
	}
	normalized, err := bridge.NormalizeNonStreamResponse(raw)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error": map[string]any{
				"message": err.Error(),
				"type":    "upstream_error",
			},
		})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(normalized)
}

func (s *Server) proxyPassthrough(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": map[string]any{
				"message": err.Error(),
				"type":    "server_error",
			},
		})
		return
	}
	resp, err := s.doUpstream(r, r.Method, r.URL.Path, r.Header.Get("Content-Type"), body)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error": map[string]any{
				"message": err.Error(),
				"type":    "upstream_error",
			},
		})
		return
	}
	defer resp.Body.Close()
	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (s *Server) doUpstream(r *http.Request, method string, requestPath string, contentType string, body []byte) (*http.Response, error) {
	attempts := s.buildAuthAttempts(strings.TrimSpace(r.Header.Get("Authorization")))
	if len(attempts) == 0 {
		attempts = []string{""}
	}

	var lastErr error
	for index, authHeader := range attempts {
		req, err := http.NewRequestWithContext(r.Context(), method, joinUpstreamURL(s.cfg.UpstreamBaseURL, requestPath), strings.NewReader(string(body)))
		if err != nil {
			return nil, err
		}
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		if accept := strings.TrimSpace(r.Header.Get("Accept")); accept != "" {
			req.Header.Set("Accept", accept)
		}
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}

		resp, err := s.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if shouldFailoverStatus(resp.StatusCode) && index < len(attempts)-1 {
			lastErr = fmt.Errorf("upstream status %d", resp.StatusCode)
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			continue
		}
		return resp, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("upstream request failed")
	}
	return nil, lastErr
}

func (s *Server) buildAuthAttempts(incomingAuth string) []string {
	mode := strings.ToLower(strings.TrimSpace(s.cfg.AuthMode))
	switch mode {
	case "passthrough":
		if incomingAuth == "" {
			return []string{""}
		}
		return []string{incomingAuth}
	case "prefer_client":
		if incomingAuth != "" {
			return []string{incomingAuth}
		}
	}

	keys := s.cfg.UpstreamAPIKeys
	if len(keys) == 0 {
		if incomingAuth != "" {
			return []string{incomingAuth}
		}
		return nil
	}

	start := int(s.rr.Add(1)-1) % len(keys)
	attempts := make([]string, 0, len(keys))
	for offset := 0; offset < len(keys); offset++ {
		key := strings.TrimSpace(keys[(start+offset)%len(keys)])
		if key == "" {
			continue
		}
		attempts = append(attempts, normalizeAuthHeader(key))
	}
	return attempts
}

func normalizeAuthHeader(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.Contains(value, " ") {
		return value
	}
	return "Bearer " + value
}

func shouldFailoverStatus(status int) bool {
	return status == http.StatusUnauthorized ||
		status == http.StatusForbidden ||
		status == http.StatusTooManyRequests ||
		status >= http.StatusInternalServerError
}

func joinUpstreamURL(baseURL string, requestPath string) string {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return strings.TrimRight(baseURL, "/") + requestPath
	}
	basePath := strings.TrimSuffix(parsed.Path, "/")
	if strings.HasSuffix(basePath, "/v1") && strings.HasPrefix(requestPath, "/v1/") {
		requestPath = strings.TrimPrefix(requestPath, "/v1")
	}
	parsed.Path = path.Clean(strings.TrimRight(basePath, "/") + "/" + strings.TrimLeft(requestPath, "/"))
	return parsed.String()
}

func copyHeaders(dst http.Header, src http.Header) {
	for key, values := range src {
		dst.Del(key)
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

type flushWriter struct {
	writer  io.Writer
	flusher http.Flusher
}

func (w *flushWriter) Write(p []byte) (int, error) {
	n, err := w.writer.Write(p)
	if err == nil && w.flusher != nil {
		w.flusher.Flush()
	}
	return n, err
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
