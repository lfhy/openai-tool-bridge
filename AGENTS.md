# Repository Guidelines

## Project Structure
- Entrypoint is `main.go`.
- Core protocol rewrite logic lives under `core/bridge/`.
- HTTP proxy wiring lives under `core/proxy/`.
- Keep the proxy layer thin; request/response rewriting belongs in `core/bridge/`.

## Build And Test
- `go build ./...` — compile the project.
- `go test ./...` — run tests.

## Coding Style
- Follow standard Go formatting with `gofmt`.
- Keep changes surgical and focused on OpenAI-compatible proxy behavior.
- Prefer explicit structs for wire protocol where practical, but keep unknown fields preserved when rewriting payloads.

## Scope
- This project is an OpenAI-compatible proxy focused on tool-call prompt bridging.
- Preserve client-visible protocol details such as SSE framing, `finish_reason`, and `tool_calls` layout.
- Keep request bridge, reasoning cleanup, and pseudo tool-call normalization consistent across streaming and non-streaming paths.
