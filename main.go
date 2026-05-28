package main

import (
	"log"
	"net/http"

	"github.com/lfhy/openai-tool-bridge/core/proxy"
)

func main() {
	cfg, err := proxy.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}

	client, err := proxy.NewHTTPClient(cfg)
	if err != nil {
		log.Fatal(err)
	}
	server := proxy.NewServer(cfg, client)
	log.Printf("openai-tool-bridge listening on %s", cfg.ListenAddr)
	if err := http.ListenAndServe(cfg.ListenAddr, server.Handler()); err != nil {
		log.Fatal(err)
	}
}
