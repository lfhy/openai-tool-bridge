# openai-tool-bridge

中文 | [English](./README.en.md)

`openai-tool-bridge` 是一个轻量的 OpenAI 兼容代理，专门用于适配“不支持原生 `tools/tool_calls/tool` 协议”的模型。

它会在把请求转发给上游之前，将工具定义抽到 system prompt 中；拿到上游响应后，再把伪工具调用还原成 OpenAI 风格的 `tool_calls`。同时，它也会处理桥接模式下流式和非流式响应里的 `Thinking...\n> ...` 思考文本，把它清洗到 `reasoning_content`。

详细英文文档见 [README.en.md](/Users/3000y/openai-tool-bridge/README.en.md)。

## 功能

- 将原生 `tools/tool_calls/tool` 桥接为 prompt 工具调用
- 转发上游前把工具信息抽到 system 消息中
- 将 `<tool_call>`、`<|tool_use|>` 等伪工具调用还原为 `tool_calls`
- 将 `Thinking...\n> ...` 这类桥接思考提取到 `reasoning_content`
- 同时支持流式和非流式 `chat/completions`
- 支持通过 `-c` 指定 TOML 配置文件
- 支持多个上游 key 的轮询负载均衡与故障转移
- 支持通过代理连接上游
- 支持运行时直传客户端 `Authorization`

## 配置

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

多个 key 与故障转移：

- 当配置了多个 `api_keys` 时，代理会按轮询做负载均衡
- 如果上游返回 `401`、`403`、`429` 或 `5xx`，会自动切到下一个 key 做故障转移

代理连接上游：

- 设置 `upstream.proxy = "http://127.0.0.1:7890"` 即可
- 未设置时默认沿用环境变量代理规则

运行时 key 直传：

- 把 `auth_mode` 设为 `passthrough`，代理会直接转发客户端的 `Authorization`
- 如果希望“客户端有 key 就用客户端，没有就回退到服务端配置”，使用 `prefer_client`

## 启动

```bash
cp config.example.toml config.toml
make run
```

或：

```bash
go run . -c config.toml
```

## 常用命令

```bash
make fmt
make test
make build
make
make run
```

## 校验

```bash
go test ./...
go build ./...
```
