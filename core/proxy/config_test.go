package proxy

import (
	"net/http"
	"testing"
)

func TestNewHTTPClientUsesConfiguredProxy(t *testing.T) {
	client, err := NewHTTPClient(Config{
		UpstreamProxy: "http://127.0.0.1:7890",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", client.Transport)
	}
	req, err := http.NewRequest(http.MethodGet, "https://api.openai.com/v1/models", nil)
	if err != nil {
		t.Fatalf("unexpected request build error: %v", err)
	}
	proxyURL, err := transport.Proxy(req)
	if err != nil {
		t.Fatalf("unexpected proxy resolution error: %v", err)
	}
	if proxyURL == nil || proxyURL.String() != "http://127.0.0.1:7890" {
		t.Fatalf("expected configured proxy url, got %v", proxyURL)
	}
}

func TestNewHTTPClientRejectsInvalidProxyURL(t *testing.T) {
	if _, err := NewHTTPClient(Config{UpstreamProxy: "://bad proxy"}); err == nil {
		t.Fatalf("expected invalid proxy url to fail")
	}
}
