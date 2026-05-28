APP := openai-tool-bridge
CONFIG ?= config.toml

.PHONY: fmt test build run

fmt:
	go fmt ./...

test:
	go test ./...

build:
	go build ./...

run:
	go run . -c $(CONFIG)
