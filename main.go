package main

import (
	"log"
	"net/http"
	"os"

	"github.com/lfhy/openai-tool-bridge/core/proxy"
)

func main() {
	cfg := proxy.Config{
		ListenAddr:      valueOrDefault("OTB_LISTEN", ":8080"),
		UpstreamBaseURL: os.Getenv("OTB_UPSTREAM_BASE_URL"),
		UpstreamModel:   os.Getenv("OTB_UPSTREAM_MODEL"),
		UpstreamAPIKey:  os.Getenv("OTB_UPSTREAM_API_KEY"),
	}

	server := proxy.NewServer(cfg, http.DefaultClient)
	log.Printf("openai-tool-bridge listening on %s", cfg.ListenAddr)
	if err := http.ListenAndServe(cfg.ListenAddr, server.Handler()); err != nil {
		log.Fatal(err)
	}
}

func valueOrDefault(value string, fallback string) string {
	if current := os.Getenv(value); current != "" {
		return current
	}
	return fallback
}
