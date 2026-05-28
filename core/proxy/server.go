package proxy

import (
	"encoding/json"
	"net/http"
)

type Config struct {
	ListenAddr      string
	UpstreamBaseURL string
	UpstreamModel   string
	UpstreamAPIKey  string
}

type Server struct {
	cfg    Config
	client *http.Client
	mux    *http.ServeMux
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
	s.mux.HandleFunc("/v1/chat/completions", s.handleChatCompletions)
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
	})
}

func (s *Server) handleChatCompletions(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusNotImplemented, map[string]any{
		"error": map[string]any{
			"message": "chat completions bridge is not implemented yet",
			"type":    "not_implemented",
		},
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
