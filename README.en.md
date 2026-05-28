# openai-tool-bridge

[中文](./README.md) | English

`openai-tool-bridge` is a lightweight OpenAI-compatible proxy for models that do not support native `tools/tool_calls/tool` messaging.

It rewrites tool metadata into the system prompt before forwarding requests upstream, and then normalizes upstream responses back into OpenAI-style tool calls. It also cleans prompt-bridge thinking output in both streaming and non-streaming responses.

## Features

- Bridges native `tools/tool_calls/tool` into prompt-based tool use
- Rewrites tool metadata into the system message before proxying upstream
- Normalizes pseudo tool calls such as `<tool_call>` and `<|tool_use|>` back to `tool_calls`
- Extracts bridge thinking like `Thinking...\n> ...` into `reasoning_content`
- Handles both streaming and non-streaming chat completions
- Supports TOML config with `-c`
- Supports round-robin load balancing and failover across multiple upstream API keys
- Supports connecting to upstream through an HTTP proxy
- Supports runtime `Authorization` passthrough mode

## Configuration

The proxy reads TOML config from `-c`, defaulting to `./config.toml`.

Example:

```toml
[server]
listen = ":8080"

[upstream]
base_url = "https://api.openai.com/v1"
model = "qwen3.7-max"
api_key = ""
api_keys = ["sk-key-a", "sk-key-b"]
auth_mode = "static"
proxy = ""
timeout_seconds = 300
```

Auth modes:

- `static`: use configured `api_keys` / `api_key`
- `passthrough`: forward the incoming `Authorization` header directly
- `prefer_client`: use client `Authorization` when present, otherwise fall back to configured keys

Multiple keys and failover:

- When multiple `api_keys` are configured, the proxy uses round-robin load balancing
- It fails over to the next key on `401`, `403`, `429`, or `5xx` upstream responses

Proxying to upstream:

- Set `upstream.proxy = "http://127.0.0.1:7890"` to route requests through a proxy
- If unset, the client falls back to environment-based proxy behavior

Runtime key passthrough:

- Set `auth_mode = "passthrough"` to forward client `Authorization` directly
- Use `prefer_client` if you want to prefer the client header and fall back to configured keys

## Run

```bash
cp config.example.toml config.toml
make run
```

Or:

```bash
go run . -c config.toml
```

## Common Commands

```bash
make fmt
make test
make build
make
make run
```

## Validation

```bash
go test ./...
go build ./...
```
