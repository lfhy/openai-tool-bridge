package bridge

import (
	"bytes"
	"strings"
	"testing"
)

func TestApplyToolPromptBridge(t *testing.T) {
	payload := map[string]any{
		"model": "demo",
		"tools": []map[string]any{
			{
				"type": "function",
				"function": map[string]any{
					"name":        "write_file",
					"description": "Write a file",
					"parameters":  map[string]any{"type": "object"},
				},
			},
		},
		"tool_choice": "auto",
		"messages": []*Message{
			{Role: "system", Content: "You are helpful."},
			{Role: "user", Content: "create a page"},
			{
				Role: "assistant",
				ToolCalls: []*ToolCall{{
					ID:   "call_1",
					Type: "function",
					Function: &ToolFunction{
						Name:      "write_file",
						Arguments: `{"path":"index.html"}`,
					},
				}},
			},
			{Role: "tool", ToolCallID: "call_1", Content: "ok"},
		},
	}

	changed, err := ApplyToolPromptBridge(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Fatal("expected payload to change")
	}
	if _, ok := payload["tools"]; ok {
		t.Fatalf("expected tools to be removed, got %#v", payload["tools"])
	}
	msgs, err := payloadMessages(payload)
	if err != nil {
		t.Fatalf("unexpected message error: %v", err)
	}
	if first, _ := msgs[0].Content.(string); !strings.Contains(first, "工具提示桥接模式") || !strings.Contains(first, "<tool_call>") {
		t.Fatalf("expected bridge prompt in first message, got %q", first)
	}
	if content, _ := msgs[2].Content.(string); !strings.Contains(content, "<function=write_file>") {
		t.Fatalf("expected assistant tool call to be encoded as xml, got %q", content)
	}
	if msgs[3].Role != "user" {
		t.Fatalf("expected tool message to become user, got %q", msgs[3].Role)
	}
}

func TestNormalizeNonStreamResponse(t *testing.T) {
	raw := []byte(`{
		"id":"chatcmpl-1",
		"object":"chat.completion",
		"created":1,
		"model":"demo",
		"choices":[
			{
				"index":0,
				"message":{
					"role":"assistant",
					"content":"Thinking...\n> task: create a page title\n> final: Create a ZCode intro page\n\nCreate a ZCode intro page"
				},
				"finish_reason":"stop"
			}
		]
	}`)

	normalized, err := NormalizeNonStreamResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(string(normalized), "Thinking...") {
		t.Fatalf("expected thinking to be removed from visible content, got %s", string(normalized))
	}
	if !strings.Contains(string(normalized), `"reasoning_content":"task: create a page title\nfinal: Create a ZCode intro page"`) {
		t.Fatalf("expected reasoning_content in normalized response, got %s", string(normalized))
	}
	if !strings.Contains(string(normalized), `"content":"Create a ZCode intro page"`) {
		t.Fatalf("expected visible content to remain, got %s", string(normalized))
	}
}

func TestRewriteStream(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"id":"chatcmpl-bridge","object":"chat.completion.chunk","created":1,"model":"demo","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		``,
		`data: {"id":"chatcmpl-bridge","object":"chat.completion.chunk","created":1,"model":"demo","choices":[{"index":0,"delta":{"content":"Thinking...\n> "},"finish_reason":null}]}`,
		``,
		`data: {"id":"chatcmpl-bridge","object":"chat.completion.chunk","created":1,"model":"demo","choices":[{"index":0,"delta":{"content":"Plan the file write.\n> "},"finish_reason":null}]}`,
		``,
		`data: {"id":"chatcmpl-bridge","object":"chat.completion.chunk","created":1,"model":"demo","choices":[{"index":0,"delta":{"content":"\n\nI will create the page.\n\n<"},"finish_reason":null}]}`,
		``,
		`data: {"id":"chatcmpl-bridge","object":"chat.completion.chunk","created":1,"model":"demo","choices":[{"index":0,"delta":{"content":"|tool_use|>"},"finish_reason":null}]}`,
		``,
		`data: {"id":"chatcmpl-bridge","object":"chat.completion.chunk","created":1,"model":"demo","choices":[{"index":0,"delta":{"content":"\n{\"name\":\"write_file\",\"arguments\":{\"path\":\"index.html\",\"content\":\"<html>ok</html>\"}}"},"finish_reason":null}]}`,
		``,
		`data: {"id":"chatcmpl-bridge","object":"chat.completion.chunk","created":1,"model":"demo","choices":[{"index":0,"delta":{"content":"\n</|tool_use|>"},"finish_reason":null}]}`,
		``,
		`data: {"id":"chatcmpl-bridge","object":"chat.completion.chunk","created":1,"model":"demo","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		``,
		`data: [DONE]`,
		"",
	}, "\n")

	var out bytes.Buffer
	if err := RewriteStream(&out, strings.NewReader(stream)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	if strings.Contains(got, "Thinking...") || strings.Contains(got, "|tool_use|") {
		t.Fatalf("expected raw bridge content to be suppressed, got %s", got)
	}
	if !strings.Contains(got, `"reasoning_content":"Plan the file write."`) {
		t.Fatalf("expected reasoning chunk, got %s", got)
	}
	if !strings.Contains(got, `"content":"I will create the page.\n\n"`) {
		t.Fatalf("expected visible answer chunk, got %s", got)
	}
	if !strings.Contains(got, `"tool_calls":[`) || !strings.Contains(got, `"name":"write_file"`) || !strings.Contains(got, `index.html`) {
		t.Fatalf("expected normalized tool call chunk, got %s", got)
	}
	if !strings.Contains(got, `"finish_reason":"tool_calls"`) {
		t.Fatalf("expected tool_calls finish reason, got %s", got)
	}
	if !strings.Contains(got, "data: [DONE]") {
		t.Fatalf("expected done marker, got %s", got)
	}
}

func TestRewriteStreamRecoversMalformedToolCall(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"id":"chatcmpl-bad-tool","object":"chat.completion.chunk","created":1,"model":"demo","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		``,
		`data: {"id":"chatcmpl-bad-tool","object":"chat.completion.chunk","created":1,"model":"demo","choices":[{"index":0,"delta":{"content":"<tool_call>"},"finish_reason":null}]}`,
		``,
		`data: {"id":"chatcmpl-bad-tool","object":"chat.completion.chunk","created":1,"model":"demo","choices":[{"index":0,"delta":{"content":"Write "},"finish_reason":null}]}`,
		``,
		`data: {"id":"chatcmpl-bad-tool","object":"chat.completion.chunk","created":1,"model":"demo","choices":[{"index":0,"delta":{"content":"content=<!DOCTYPE html><html>ok</html>"},"finish_reason":null}]}`,
		``,
		`data: {"id":"chatcmpl-bad-tool","object":"chat.completion.chunk","created":1,"model":"demo","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		``,
		`data: [DONE]`,
		"",
	}, "\n")

	var out bytes.Buffer
	if err := RewriteStream(&out, strings.NewReader(stream)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	if strings.Contains(got, `"<tool_call>`) || strings.Contains(got, `Write content=`) {
		t.Fatalf("expected malformed tool call to be normalized, got %s", got)
	}
	if !strings.Contains(got, `"tool_calls":[`) || !strings.Contains(got, `"name":"Write"`) {
		t.Fatalf("expected recovered tool_calls output, got %s", got)
	}
	if !strings.Contains(got, `DOCTYPE html`) {
		t.Fatalf("expected recovered content argument in tool call, got %s", got)
	}
	if !strings.Contains(got, `"finish_reason":"tool_calls"`) {
		t.Fatalf("expected tool_calls finish reason, got %s", got)
	}
}
