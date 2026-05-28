APP := openai-tool-bridge
CONFIG ?= config.toml

.PHONY: all fmt test build run

all: build

fmt:
	go fmt ./...

test:
	go test ./...

build:
	go build -o $(APP) main.go

run:
	go run . -c $(CONFIG)
