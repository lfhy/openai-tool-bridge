package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestServerBridgesRequestAndNormalizesNonStreamResponse(t *testing.T) {
	var requestBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requestBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-1",
			"object":"chat.completion",
			"created":1,
			"model":"demo-upstream",
			"choices":[{
				"index":0,
				"message":{
					"role":"assistant",
					"content":"Thinking...\n> plan: create a page\n> final: Create a ZCode page\n\nCreate a ZCode page"
				},
				"finish_reason":"stop"
			}]
		}`))
	}))
	defer upstream.Close()

	server := NewServer(Config{
		ListenAddr:      ":0",
		UpstreamBaseURL: upstream.URL + "/v1",
		UpstreamModel:   "demo-upstream",
	}, upstream.Client())

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{
		"model":"demo",
		"tools":[{"type":"function","function":{"name":"write_file","description":"Write a file","parameters":{"type":"object"}}}],
		"messages":[{"role":"user","content":"create a zcode page"}]
	}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: got %d body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(requestBody, `"tools"`) {
		t.Fatalf("expected upstream body to remove native tools, got %s", requestBody)
	}
	if !strings.Contains(requestBody, "工具提示桥接模式") {
		t.Fatalf("expected upstream body to contain bridge prompt, got %s", requestBody)
	}
	if !strings.Contains(w.Body.String(), `"reasoning_content":"plan: create a page\nfinal: Create a ZCode page"`) {
		t.Fatalf("expected reasoning_content in downstream response, got %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"content":"Create a ZCode page"`) {
		t.Fatalf("expected visible content in downstream response, got %s", w.Body.String())
	}
}

func TestServerNormalizesStreamResponse(t *testing.T) {
	upstreamStream := strings.Join([]string{
		`data: {"id":"chatcmpl-bridge","object":"chat.completion.chunk","created":1,"model":"demo","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		``,
		`data: {"id":"chatcmpl-bridge","object":"chat.completion.chunk","created":1,"model":"demo","choices":[{"index":0,"delta":{"content":"Thinking...\n> "},"finish_reason":null}]}`,
		``,
		`data: {"id":"chatcmpl-bridge","object":"chat.completion.chunk","created":1,"model":"demo","choices":[{"index":0,"delta":{"content":"Plan the file write.\n> "},"finish_reason":null}]}`,
		``,
		`data: {"id":"chatcmpl-bridge","object":"chat.completion.chunk","created":1,"model":"demo","choices":[{"index":0,"delta":{"content":"\n\nI will create the page.\n\n<"},"finish_reason":null}]}`,
		``,
		`data: {"id":"chatcmpl-bridge","object":"chat.completion.chunk","created":1,"model":"demo","choices":[{"index":0,"delta":{"content":"|tool_use|>"},"finish_reason":null}]}`,
		``,
		`data: {"id":"chatcmpl-bridge","object":"chat.completion.chunk","created":1,"model":"demo","choices":[{"index":0,"delta":{"content":"\n{\"name\":\"write_file\",\"arguments\":{\"path\":\"index.html\",\"content\":\"<html>ok</html>\"}}"},"finish_reason":null}]}`,
		``,
		`data: {"id":"chatcmpl-bridge","object":"chat.completion.chunk","created":1,"model":"demo","choices":[{"index":0,"delta":{"content":"\n</|tool_use|>"},"finish_reason":null}]}`,
		``,
		`data: {"id":"chatcmpl-bridge","object":"chat.completion.chunk","created":1,"model":"demo","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, upstreamStream)
	}))
	defer upstream.Close()

	server := NewServer(Config{
		ListenAddr:      ":0",
		UpstreamBaseURL: upstream.URL + "/v1",
		UpstreamModel:   "demo-upstream",
	}, upstream.Client())

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{
		"model":"demo",
		"stream":true,
		"tools":[{"type":"function","function":{"name":"write_file","description":"Write a file","parameters":{"type":"object"}}}],
		"messages":[{"role":"user","content":"create a zcode page"}]
	}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: got %d body=%s", w.Code, w.Body.String())
	}
	got := w.Body.String()
	if strings.Contains(got, "Thinking...") || strings.Contains(got, "|tool_use|") {
		t.Fatalf("expected stream output to suppress raw bridge text, got %s", got)
	}
	if !strings.Contains(got, `"reasoning_content":"Plan the file write."`) {
		t.Fatalf("expected reasoning_content in stream output, got %s", got)
	}
	if !strings.Contains(got, `"tool_calls":[`) || !strings.Contains(got, `"name":"write_file"`) {
		t.Fatalf("expected tool_calls in stream output, got %s", got)
	}
}

func TestServerFailsOverAcrossAPIKeys(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			if got := r.Header.Get("Authorization"); got != "Bearer key-a" {
				t.Fatalf("unexpected first auth header: %s", got)
			}
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"rate limited"}}`))
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer key-b" {
			t.Fatalf("unexpected second auth header: %s", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-1",
			"object":"chat.completion",
			"created":1,
			"model":"demo-upstream",
			"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]
		}`))
	}))
	defer upstream.Close()

	server := NewServer(Config{
		ListenAddr:      ":0",
		UpstreamBaseURL: upstream.URL + "/v1",
		UpstreamModel:   "demo-upstream",
		UpstreamAPIKeys: []string{"key-a", "key-b"},
		AuthMode:        "static",
	}, upstream.Client())

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"demo","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: got %d body=%s", w.Code, w.Body.String())
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected two upstream attempts, got %d", got)
	}
}

func TestServerPassesThroughClientAuthorization(t *testing.T) {
	var authHeader string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-1",
			"object":"chat.completion",
			"created":1,
			"model":"demo-upstream",
			"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]
		}`))
	}))
	defer upstream.Close()

	server := NewServer(Config{
		ListenAddr:      ":0",
		UpstreamBaseURL: upstream.URL + "/v1",
		UpstreamModel:   "demo-upstream",
		UpstreamAPIKeys: []string{"server-key"},
		AuthMode:        "passthrough",
	}, upstream.Client())

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"demo","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer client-key")
	w := httptest.NewRecorder()

	server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: got %d body=%s", w.Code, w.Body.String())
	}
	if authHeader != "Bearer client-key" {
		t.Fatalf("expected passthrough auth header, got %s", authHeader)
	}
}

func TestServerPrefersClientAuthorizationWhenConfigured(t *testing.T) {
	var authHeader string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-1",
			"object":"chat.completion",
			"created":1,
			"model":"demo-upstream",
			"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]
		}`))
	}))
	defer upstream.Close()

	server := NewServer(Config{
		ListenAddr:      ":0",
		UpstreamBaseURL: upstream.URL + "/v1",
		UpstreamModel:   "demo-upstream",
		UpstreamAPIKeys: []string{"server-key"},
		AuthMode:        "prefer_client",
	}, upstream.Client())

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"demo","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer client-key")
	w := httptest.NewRecorder()

	server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: got %d body=%s", w.Code, w.Body.String())
	}
	if authHeader != "Bearer client-key" {
		t.Fatalf("expected client auth header to win, got %s", authHeader)
	}
}

func TestServerProxyModelsWhenNoStaticModelConfigured(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"qwen3.7-max-t","object":"model"}]}`))
	}))
	defer upstream.Close()

	server := NewServer(Config{
		ListenAddr:      ":0",
		UpstreamBaseURL: upstream.URL + "/v1",
	}, upstream.Client())

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	w := httptest.NewRecorder()

	server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"qwen3.7-max-t"`) {
		t.Fatalf("expected passthrough model list, got %s", w.Body.String())
	}
}
