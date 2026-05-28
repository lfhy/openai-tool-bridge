package proxy

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	lfhyflag "github.com/lfhy/flag"
)

type Config struct {
	ConfigPath      string
	ListenAddr      string
	Debug           bool
	UpstreamBaseURL string
	UpstreamModel   string
	UpstreamAPIKey  string
	UpstreamAPIKeys []string
	UpstreamProxy   string
	AuthMode        string
	Timeout         time.Duration
}

func defaultConfigPath() string {
	return filepath.Join(".", "config.toml")
}

func LoadConfig() (Config, error) {
	var cfg Config
	configPathPtr := lfhyflag.String("c", defaultConfigPath(), "config file path")
	lfhyflag.StringConfigVar(&cfg.ListenAddr, "listen", "server", "listen", ":8080", "listen address")
	lfhyflag.BoolConfigVar(&cfg.Debug, "debug", "server", "debug", false, "enable debug logging")
	lfhyflag.StringConfigVar(&cfg.UpstreamBaseURL, "upstream-base-url", "upstream", "base_url", "", "upstream base url")
	lfhyflag.StringConfigVar(&cfg.UpstreamModel, "upstream-model", "upstream", "model", "", "upstream model override")
	lfhyflag.StringConfigVar(&cfg.UpstreamAPIKey, "upstream-api-key", "upstream", "api_key", "", "upstream api key override")
	lfhyflag.StringConfigVar(&cfg.UpstreamProxy, "upstream-proxy", "upstream", "proxy", "", "upstream proxy url")
	lfhyflag.StringConfigVar(&cfg.AuthMode, "auth-mode", "upstream", "auth_mode", "static", "auth mode: static|passthrough|prefer_client")
	timeoutSeconds := 0
	lfhyflag.IntConfigVar(&timeoutSeconds, "timeout-seconds", "upstream", "timeout_seconds", 300, "upstream timeout in seconds")
	lfhyflag.Parse()

	if configPathPtr != nil {
		cfg.ConfigPath = strings.TrimSpace(*configPathPtr)
	}
	if cfg.ConfigPath == "" {
		cfg.ConfigPath = defaultConfigPath()
	}
	if cfg.UpstreamBaseURL == "" {
		return Config{}, fmt.Errorf("missing upstream.base_url in %s", cfg.ConfigPath)
	}
	cfg.Timeout = time.Duration(timeoutSeconds) * time.Second
	cfg.UpstreamAPIKeys = readStringSlice("upstream", "api_keys")
	if len(cfg.UpstreamAPIKeys) == 0 && strings.TrimSpace(cfg.UpstreamAPIKey) != "" {
		cfg.UpstreamAPIKeys = []string{strings.TrimSpace(cfg.UpstreamAPIKey)}
	}
	if _, err := os.Stat(cfg.ConfigPath); err != nil && !os.IsNotExist(err) {
		return Config{}, err
	}
	return cfg, nil
}

func readStringSlice(section string, key string) []string {
	cfg := lfhyflag.GetConfig()
	if cfg == nil {
		return nil
	}
	values := cfg.ReadConfigToAnySlice(section, key)
	out := make([]string, 0, len(values))
	for _, value := range values {
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" || text == "<nil>" {
			continue
		}
		out = append(out, text)
	}
	return out
}

func NewHTTPClient(cfg Config) (*http.Client, error) {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	if proxyURL := strings.TrimSpace(cfg.UpstreamProxy); proxyURL != "" {
		parsed, err := url.Parse(proxyURL)
		if err != nil {
			return nil, err
		}
		transport.Proxy = http.ProxyURL(parsed)
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}, nil
}
