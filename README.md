# openai-tool-bridge

OpenAI-compatible proxy that bridges native tool calling into prompt-based tool use for models without tool-call support.

## English

`openai-tool-bridge` is a lightweight OpenAI-compatible proxy for models that do not support native `tools/tool_calls/tool` messaging.

It rewrites tool metadata into the system prompt before forwarding requests upstream, and then normalizes upstream responses back into OpenAI-style tool calls. It also cleans prompt-bridge thinking output in both streaming and non-streaming responses.

### Features

- Bridges native `tools/tool_calls/tool` into prompt-based tool use
- Rewrites tool metadata into the system message before proxying upstream
- Normalizes pseudo tool calls such as `<tool_call>` and `<|tool_use|>` back to `tool_calls`
- Extracts bridge thinking like `Thinking...\n> ...` into `reasoning_content`
- Handles both streaming and non-streaming chat completions
- Supports TOML config with `-c`

### Configuration

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

When multiple `api_keys` are configured, the proxy uses round-robin load balancing and fails over to the next key on `401`, `403`, `429`, or `5xx` upstream responses.

### Run

```bash
cp config.example.toml config.toml
go run . -c config.toml
```

### Build

```bash
go build ./...
go test ./...
```

## 中文

`openai-tool-bridge` 是一个轻量的 OpenAI 兼容代理，专门用于适配“不支持原生 `tools/tool_calls/tool` 协议”的模型。

它会在把请求转发给上游之前，将工具定义抽到 system prompt 中；拿到上游响应后，再把伪工具调用还原成 OpenAI 风格的 `tool_calls`。同时，它也会处理桥接模式下流式和非流式响应里的 `Thinking...\n> ...` 思考文本，把它清洗到 `reasoning_content`。

### 功能

- 将原生 `tools/tool_calls/tool` 桥接为 prompt 工具调用
- 转发上游前把工具信息抽到 system 消息中
- 将 `<tool_call>`、`<|tool_use|>` 等伪工具调用还原为 `tool_calls`
- 将 `Thinking...\n> ...` 这类桥接思考提取到 `reasoning_content`
- 同时支持流式和非流式 `chat/completions`
- 支持通过 `-c` 指定 TOML 配置文件

### 配置

程序默认从 `./config.toml` 读取配置，也可以通过 `-c` 显式指定。

示例：

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

鉴权模式：

- `static`：使用配置里的 `api_keys` / `api_key`
- `passthrough`：直接透传客户端传入的 `Authorization`
- `prefer_client`：优先使用客户端 `Authorization`，没有时再回退到配置里的 key

当配置了多个 `api_keys` 时，代理会按轮询做负载均衡；如果上游返回 `401`、`403`、`429` 或 `5xx`，会自动切到下一个 key 做故障转移。

### 启动

```bash
cp config.example.toml config.toml
go run . -c config.toml
```

### 校验

```bash
go build ./...
go test ./...
```
