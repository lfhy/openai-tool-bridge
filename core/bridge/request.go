package bridge

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

func ApplyToolPromptBridge(payload map[string]any) (bool, error) {
	if !payloadHasBridgeInputs(payload) {
		return false, nil
	}

	msgs, err := payloadMessages(payload)
	if err != nil {
		return false, fmt.Errorf("rewrite messages: %w", err)
	}
	tools, err := payloadTools(payload["tools"])
	if err != nil {
		return false, fmt.Errorf("rewrite tools: %w", err)
	}

	payload["messages"] = bridgeMessages(msgs, tools)
	delete(payload, "tools")
	delete(payload, "tool_choice")
	delete(payload, "parallel_tool_calls")
	return true, nil
}

func payloadHasBridgeInputs(payload map[string]any) bool {
	if payload == nil {
		return false
	}
	if rawTools, ok := payload["tools"]; ok {
		if tools, err := payloadTools(rawTools); err == nil && len(tools) > 0 {
			return true
		}
	}
	msgs, err := payloadMessages(payload)
	if err != nil {
		return false
	}
	for _, msg := range msgs {
		if msg == nil {
			continue
		}
		if hasToolCalls(msg.ToolCalls) || strings.EqualFold(strings.TrimSpace(msg.Role), "tool") {
			return true
		}
	}
	return false
}

func payloadMessages(payload map[string]any) ([]*Message, error) {
	raw, ok := payload["messages"]
	if !ok {
		return nil, nil
	}
	data, err := marshal(raw)
	if err != nil {
		return nil, err
	}
	var msgs []*Message
	if err := unmarshal(data, &msgs); err != nil {
		return nil, err
	}
	return msgs, nil
}

func payloadTools(raw any) ([]*ToolDefinition, error) {
	if raw == nil {
		return nil, nil
	}
	data, err := marshal(raw)
	if err != nil {
		return nil, err
	}
	var tools []*ToolDefinition
	if err := unmarshal(data, &tools); err != nil {
		return nil, err
	}
	return tools, nil
}

func bridgeMessages(msgs []*Message, tools []*ToolDefinition) []*Message {
	bridged := make([]*Message, 0, len(msgs)+1)
	toolNames := make(map[string]string)
	insertedPrompt := false

	for _, msg := range msgs {
		if msg == nil {
			continue
		}
		next := &Message{
			Role:             strings.TrimSpace(msg.Role),
			Content:          msg.Content,
			ReasoningContent: msg.ReasoningContent,
			Reasoning:        msg.Reasoning,
			ToolCalls:        msg.ToolCalls,
			ToolCallID:       strings.TrimSpace(msg.ToolCallID),
		}
		if next.Role == "" {
			next.Role = "user"
		}
		if !insertedPrompt && promptRole(next.Role) {
			next.Content = prependText(next.Content, buildPrompt(tools))
			insertedPrompt = true
		}
		if hasToolCalls(next.ToolCalls) {
			encoded, names := encodeAssistantToolCalls(next.ToolCalls)
			for key, value := range names {
				toolNames[key] = value
			}
			next.Content = appendText(next.Content, encoded)
			next.ToolCalls = nil
		}
		if strings.EqualFold(next.Role, "tool") {
			next.Role = "user"
			next.Content = encodeToolResult(next.ToolCallID, toolNames[next.ToolCallID], next.Content)
			next.ToolCallID = ""
		}
		bridged = append(bridged, next)
	}

	if !insertedPrompt {
		bridged = append([]*Message{{
			Role:    "system",
			Content: buildPrompt(tools),
		}}, bridged...)
	}
	return bridged
}

func promptRole(role string) bool {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "system", "developer", "user":
		return true
	default:
		return false
	}
}

func buildPrompt(tools []*ToolDefinition) string {
	if len(tools) == 0 {
		return `You are running in tool prompt bridge mode. Do not use native tool_calls or tool messages.

If you need to call a tool, output one or more XML blocks exactly like this:
<tool_call>
<function=tool_name>
<parameter=arguments_json>{"arg":"value"}</parameter>
</function>
</tool_call>

If no tool is needed, answer normally. If later user messages contain <|tool_result|>...</|tool_result|>, treat them as tool execution results and continue the task.`
	}

	sections := make([]string, 0, len(tools))
	for _, tool := range tools {
		if tool == nil || tool.Function == nil {
			continue
		}
		params := "{}"
		if tool.Function.Parameters != nil {
			params = marshalString(tool.Function.Parameters)
		}
		description := strings.TrimSpace(tool.Function.Description)
		if description == "" {
			description = "No additional description."
		}
		sections = append(sections, fmt.Sprintf("### %s\n%s\nParameter JSON Schema:\n%s", strings.TrimSpace(tool.Function.Name), description, params))
	}

	return fmt.Sprintf(`You are running in tool prompt bridge mode. The target model does not support native tool_calls or tool messages.

If you need to call a tool, output one or more XML blocks exactly like this:
<tool_call>
<function=tool_name>
<parameter=arguments_json>{"arg":"value"}</parameter>
</function>
</tool_call>

If no tool is needed, answer normally. If later user messages contain <|tool_result|>...</|tool_result|>, treat them as tool execution results and continue the task.

Available tools:

%s`, strings.Join(sections, "\n\n"))
}

func prependText(content any, text string) any {
	text = strings.TrimSpace(text)
	if text == "" {
		return content
	}
	switch value := content.(type) {
	case nil:
		return text
	case string:
		if strings.TrimSpace(value) == "" {
			return text
		}
		return text + "\n\n" + value
	default:
		existing := extractContentText(content)
		if existing == "" {
			return text
		}
		return text + "\n\n" + existing
	}
}

func appendText(content any, text string) any {
	text = strings.TrimSpace(text)
	if text == "" {
		return content
	}
	switch value := content.(type) {
	case nil:
		return text
	case string:
		if strings.TrimSpace(value) == "" {
			return text
		}
		return value + "\n\n" + text
	default:
		existing := extractContentText(content)
		if existing == "" {
			return text
		}
		return existing + "\n\n" + text
	}
}

func extractContentText(content any) string {
	switch value := content.(type) {
	case nil:
		return ""
	case string:
		return value
	default:
		return strings.TrimSpace(marshalString(value))
	}
}

func encodeAssistantToolCalls(calls []*ToolCall) (string, map[string]string) {
	if len(calls) == 0 {
		return "", nil
	}
	blocks := make([]string, 0, len(calls))
	names := make(map[string]string, len(calls))
	for _, call := range calls {
		if call == nil || call.Function == nil {
			continue
		}
		name := strings.TrimSpace(call.Function.Name)
		if name == "" {
			continue
		}
		if call.ID != "" {
			names[strings.TrimSpace(call.ID)] = name
		}
		blocks = append(blocks, encodeXMLToolCall(name, call.Function.Arguments))
	}
	return strings.Join(blocks, "\n\n"), names
}

func encodeXMLToolCall(name string, arguments string) string {
	return fmt.Sprintf("<tool_call>\n<function=%s>\n%s\n</function>\n</tool_call>", strings.TrimSpace(name), encodeXMLParameters(arguments))
}

func encodeXMLParameters(arguments string) string {
	normalized := strings.TrimSpace(arguments)
	if normalized == "" {
		return "<parameter=arguments_json>{}</parameter>"
	}

	var object map[string]any
	if err := json.Unmarshal([]byte(normalized), &object); err != nil {
		return fmt.Sprintf("<parameter=arguments_json>%s</parameter>", normalizeArguments(arguments))
	}

	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		value := object[key]
		switch typed := value.(type) {
		case string:
			lines = append(lines, fmt.Sprintf("<parameter=%s>%s</parameter>", key, typed))
		default:
			lines = append(lines, fmt.Sprintf("<parameter=%s>%s</parameter>", key, marshalString(typed)))
		}
	}
	return strings.Join(lines, "\n")
}

func normalizeArguments(arguments string) string {
	arguments = strings.TrimSpace(arguments)
	if arguments == "" {
		return "{}"
	}
	var value any
	if err := json.Unmarshal([]byte(arguments), &value); err == nil {
		return marshalString(value)
	}
	return marshalString(arguments)
}

func encodeToolResult(toolCallID string, toolName string, content any) string {
	payload := map[string]any{
		"tool_call_id": strings.TrimSpace(toolCallID),
		"name":         strings.TrimSpace(toolName),
		"content":      extractContentText(content),
	}
	return fmt.Sprintf("<|tool_result|>\n%s\n</|tool_result|>", marshalString(payload))
}

func hasToolCalls(calls []*ToolCall) bool {
	return len(calls) > 0
}
