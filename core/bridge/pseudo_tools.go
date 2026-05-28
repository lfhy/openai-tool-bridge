package bridge

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var malformedToolArgPattern = regexp.MustCompile(`(?i)(^|[\s\r\n\t])([a-z_][a-z0-9_.-]*)=`)

func ExtractPseudoToolCallsFromContent(content string) (string, []*ToolCall, bool) {
	return extractPseudoToolCallsFromContent(content, true)
}

func ExtractPseudoToolCallsFromStreamingContent(content string) (string, []*ToolCall, bool) {
	return extractPseudoToolCallsFromContent(content, false)
}

func extractPseudoToolCallsFromContent(content string, allowMalformed bool) (string, []*ToolCall, bool) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return content, nil, false
	}
	if calls, ok := parsePseudoToolCalls(trimmed, allowMalformed); ok {
		return "", calls, true
	}

	lowerContent := strings.ToLower(content)
	firstToolCallIndex := strings.Index(lowerContent, "<tool_call>")
	if firstToolCallIndex < 0 {
		firstToolCallIndex = strings.Index(lowerContent, "<function_calls>")
	}
	if firstToolCallIndex < 0 {
		firstToolCallIndex = strings.Index(lowerContent, "<function_call>")
	}
	if firstToolCallIndex < 0 {
		firstToolCallIndex = strings.Index(lowerContent, "<|tool_use|>")
	}
	if firstToolCallIndex < 0 {
		return content, nil, false
	}

	prefix := strings.TrimRight(content[:firstToolCallIndex], " \r\n\t")
	toolSegment := strings.TrimSpace(content[firstToolCallIndex:])
	if toolSegment == "" {
		return content, nil, false
	}
	calls, ok := parsePseudoToolCalls(toolSegment, allowMalformed)
	if !ok {
		return content, nil, false
	}
	return prefix, calls, true
}

func ParsePseudoToolCalls(content string) ([]*ToolCall, bool) {
	return parsePseudoToolCalls(content, true)
}

func parsePseudoToolCalls(content string, allowMalformed bool) ([]*ToolCall, bool) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return nil, false
	}

	if strings.HasPrefix(trimmed, "<Function_Go_Start") {
		if end := strings.Index(trimmed, "/>"); end >= 0 {
			trimmed = strings.TrimSpace(trimmed[end+2:])
		}
	}
	if calls, ok := parseToolUseBlocks(trimmed); ok {
		return calls, true
	}
	if calls, ok := parseXMLStyleToolCalls(trimmed); ok {
		return calls, true
	}
	if allowMalformed {
		if calls, ok := parseMalformedXMLStyleToolCall(trimmed); ok {
			return calls, true
		}
		if calls, ok := parseMalformedToolCall(trimmed); ok {
			return calls, true
		}
	}
	return parseLegacyFunctionCalls(trimmed)
}

func ParsePseudoToolCallsStreaming(content string) ([]*ToolCall, bool) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return nil, false
	}
	if strings.HasPrefix(trimmed, "<Function_Go_Start") {
		if end := strings.Index(trimmed, "/>"); end >= 0 {
			trimmed = strings.TrimSpace(trimmed[end+2:])
		}
	}
	if calls, ok := parseToolUseBlocks(trimmed); ok {
		return calls, true
	}
	if calls, ok := parseXMLStyleToolCalls(trimmed); ok {
		return calls, true
	}
	return parseLegacyFunctionCalls(trimmed)
}

func LooksLikePseudoToolCallContent(content string) bool {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return false
	}
	lower := strings.ToLower(trimmed)
	return strings.HasPrefix(lower, "<function") ||
		strings.Contains(lower, "<function_go_start") ||
		strings.Contains(lower, "<function_calls") ||
		strings.Contains(lower, "<function_call") ||
		strings.Contains(lower, "<|tool_use|>") ||
		strings.Contains(lower, "<tool_call>") ||
		strings.Contains(lower, "<function=")
}

func LooksLikePseudoToolCallPrefix(content string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(content))
	if trimmed == "" {
		return false
	}
	for _, opener := range []string{
		"<function_go_start",
		"<function_calls>",
		"<function_call>",
		"<|tool_use|>",
		"<tool_call>",
		"<function=",
	} {
		if strings.HasPrefix(opener, trimmed) {
			return true
		}
	}
	return false
}

func FindPseudoToolCallStart(content string) int {
	for index := 0; index < len(content); index++ {
		if content[index] != '<' {
			continue
		}
		suffix := content[index:]
		if LooksLikePseudoToolCallContent(suffix) || LooksLikePseudoToolCallPrefix(suffix) {
			return index
		}
	}
	return -1
}

func parseToolUseBlocks(content string) ([]*ToolCall, bool) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return nil, false
	}

	const openTag = "<|tool_use|>"
	const closeTag = "</|tool_use|>"

	var calls []*ToolCall
	for strings.TrimSpace(trimmed) != "" {
		trimmed = strings.TrimSpace(trimmed)
		if !strings.HasPrefix(strings.ToLower(trimmed), openTag) {
			return nil, false
		}
		trimmed = strings.TrimSpace(trimmed[len(openTag):])
		end := strings.Index(strings.ToLower(trimmed), closeTag)
		if end < 0 {
			return nil, false
		}
		block := strings.TrimSpace(trimmed[:end])
		trimmed = strings.TrimSpace(trimmed[end+len(closeTag):])
		call, ok := parseToolUseBlock(block, len(calls))
		if !ok {
			return nil, false
		}
		calls = append(calls, call)
	}
	return calls, len(calls) > 0
}

func parseToolUseBlock(block string, index int) (*ToolCall, bool) {
	var payload struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal([]byte(block), &payload); err != nil {
		return nil, false
	}
	name := strings.TrimSpace(payload.Name)
	if name == "" {
		return nil, false
	}
	args := strings.TrimSpace(string(payload.Arguments))
	if args == "" {
		args = "{}"
	}
	return &ToolCall{
		Index: index,
		Type:  "function",
		Function: &ToolFunction{
			Name:      name,
			Arguments: args,
		},
	}, true
}

func parseLegacyFunctionCalls(trimmed string) ([]*ToolCall, bool) {
	const openCallsTag = "<function_calls>"
	const closeCallsTag = "</function_calls>"
	const openCallTag = "<function_call>"
	const closeCallTag = "</function_call>"

	if !strings.HasPrefix(trimmed, openCallsTag) || !strings.HasSuffix(trimmed, closeCallsTag) {
		return nil, false
	}
	body := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, openCallsTag), closeCallsTag))
	if body == "" {
		return nil, false
	}

	var calls []*ToolCall
	for strings.TrimSpace(body) != "" {
		body = strings.TrimSpace(body)
		if !strings.HasPrefix(body, openCallTag) {
			return nil, false
		}
		body = strings.TrimSpace(body[len(openCallTag):])
		name, rest, ok := consumePseudoTag(body, "name")
		if !ok {
			return nil, false
		}
		args, rest, ok := consumePseudoTag(rest, "args_json")
		if !ok {
			return nil, false
		}
		rest = strings.TrimSpace(rest)
		if !strings.HasPrefix(rest, closeCallTag) {
			return nil, false
		}
		body = strings.TrimSpace(rest[len(closeCallTag):])
		calls = append(calls, &ToolCall{
			Index: len(calls),
			Type:  "function",
			Function: &ToolFunction{
				Name:      strings.TrimSpace(name),
				Arguments: strings.TrimSpace(args),
			},
		})
	}
	return calls, len(calls) > 0
}

func parseXMLStyleToolCalls(content string) ([]*ToolCall, bool) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return nil, false
	}
	const openCallTag = "<tool_call>"
	const closeCallTag = "</tool_call>"

	var calls []*ToolCall
	for strings.TrimSpace(trimmed) != "" {
		trimmed = strings.TrimSpace(trimmed)
		if !strings.HasPrefix(trimmed, openCallTag) {
			return nil, false
		}
		trimmed = strings.TrimSpace(trimmed[len(openCallTag):])
		end := strings.Index(strings.ToLower(trimmed), closeCallTag)
		if end < 0 {
			return nil, false
		}
		block := strings.TrimSpace(trimmed[:end])
		rest := strings.TrimSpace(trimmed[end+len(closeCallTag):])
		call, ok := parseXMLStyleToolCallBlock(block, len(calls))
		if !ok {
			return nil, false
		}
		calls = append(calls, call)
		trimmed = rest
	}
	return calls, len(calls) > 0
}

func parseMalformedToolCall(content string) ([]*ToolCall, bool) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" || !strings.HasPrefix(strings.ToLower(trimmed), "<tool_call>") {
		return nil, false
	}
	hasClosingTag := strings.Contains(strings.ToLower(trimmed), "</tool_call>")
	body := strings.TrimSpace(trimmed[len("<tool_call>"):])
	body = strings.TrimSpace(strings.TrimSuffix(body, "</tool_call>"))
	if body == "" {
		return nil, false
	}
	if strings.HasPrefix(strings.ToLower(body), "<function=") {
		return nil, false
	}

	name, rest, ok := consumeMalformedToolName(body)
	if !ok {
		return nil, false
	}
	if !hasClosingTag && !strings.Contains(rest, "=") {
		return nil, false
	}
	args := parseMalformedToolArguments(rest)
	rawArgs, err := json.Marshal(args)
	if err != nil {
		return nil, false
	}
	return []*ToolCall{{
		Index: 0,
		Type:  "function",
		Function: &ToolFunction{
			Name:      name,
			Arguments: string(rawArgs),
		},
	}}, true
}

func parseMalformedXMLStyleToolCall(content string) ([]*ToolCall, bool) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" || !strings.HasPrefix(strings.ToLower(trimmed), "<tool_call>") {
		return nil, false
	}
	body := strings.TrimSpace(trimmed[len("<tool_call>"):])
	if body == "" {
		return nil, false
	}

	funcName, rest, ok := consumeXMLFunctionOpen(body)
	if !ok {
		return nil, false
	}

	params := make(map[string]any)
	consumedParam := false
	for {
		rest = strings.TrimSpace(rest)
		switch {
		case rest == "":
			args, ok := marshalMalformedXMLStyleArguments(params)
			if !ok {
				return nil, false
			}
			return []*ToolCall{{
				Index: 0,
				Type:  "function",
				Function: &ToolFunction{
					Name:      strings.TrimSpace(funcName),
					Arguments: args,
				},
			}}, consumedParam
		case strings.HasPrefix(strings.ToLower(rest), "</function>"):
			rest = rest[len("</function>"):]
			continue
		case strings.HasPrefix(strings.ToLower(rest), "</tool_call>"):
			rest = rest[len("</tool_call>"):]
			continue
		}

		key, value, nextRest, ok := consumeMalformedXMLParameter(rest)
		if !ok {
			return nil, false
		}
		params[key] = value
		consumedParam = true
		rest = nextRest
	}
}

func consumeMalformedToolName(body string) (string, string, bool) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", "", false
	}
	end := 0
	for end < len(body) {
		ch := body[end]
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '.' || ch == '-' {
			end++
			continue
		}
		break
	}
	if end == 0 {
		return "", "", false
	}
	return strings.TrimSpace(body[:end]), strings.TrimSpace(body[end:]), true
}

func parseMalformedToolArguments(rest string) map[string]any {
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return map[string]any{}
	}
	matches := malformedToolArgPattern.FindAllStringSubmatchIndex(rest, -1)
	if len(matches) == 0 {
		return map[string]any{
			"input": rest,
		}
	}

	args := make(map[string]any, len(matches))
	leading := strings.TrimSpace(rest[:matches[0][0]])
	if leading != "" {
		args["input"] = leading
	}
	for index, match := range matches {
		keyStart := match[4]
		keyEnd := match[5]
		valueStart := match[1]
		if valueStart < 0 || valueStart > len(rest) {
			continue
		}
		valueEnd := len(rest)
		if index+1 < len(matches) {
			valueEnd = matches[index+1][0]
		}
		key := strings.TrimSpace(rest[keyStart:keyEnd])
		if key == "" {
			continue
		}
		args[key] = normalizeMalformedToolValue(rest[valueStart:valueEnd])
	}
	if len(args) == 0 {
		args["input"] = rest
	}
	return args
}

func normalizeMalformedToolValue(raw string) any {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if (strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`)) || (strings.HasPrefix(value, `'`) && strings.HasSuffix(value, `'`)) {
		value = strings.TrimSpace(value[1 : len(value)-1])
	}
	var decoded any
	if err := json.Unmarshal([]byte(value), &decoded); err == nil {
		return decoded
	}
	return value
}

func consumeMalformedXMLParameter(content string) (string, string, string, bool) {
	content = strings.TrimSpace(content)
	const prefix = "<parameter="
	if !strings.HasPrefix(strings.ToLower(content), prefix) {
		return "", "", content, false
	}
	nameEnd := strings.Index(content, ">")
	if nameEnd < 0 {
		return "", "", content, false
	}
	name := strings.TrimSpace(content[len(prefix):nameEnd])
	if name == "" {
		return "", "", content, false
	}
	body := content[nameEnd+1:]
	lowerBody := strings.ToLower(body)
	if valueEnd := strings.Index(lowerBody, "</parameter>"); valueEnd >= 0 {
		return name, strings.TrimSpace(body[:valueEnd]), body[valueEnd+len("</parameter>"):], true
	}
	if functionCloseIndex := strings.Index(lowerBody, "</function>"); functionCloseIndex >= 0 {
		return name, strings.TrimSpace(body[:functionCloseIndex]), body[functionCloseIndex:], true
	}
	if toolCallCloseIndex := strings.Index(lowerBody, "</tool_call>"); toolCallCloseIndex >= 0 {
		return name, strings.TrimSpace(body[:toolCallCloseIndex]), body[toolCallCloseIndex:], true
	}
	return name, strings.TrimSpace(body), "", true
}

func parseXMLStyleToolCallBlock(block string, index int) (*ToolCall, bool) {
	block = strings.TrimSpace(block)
	if !strings.HasPrefix(strings.ToLower(block), "<function=") {
		return nil, false
	}

	funcName, rest, ok := consumeXMLFunctionOpen(block)
	if !ok {
		return nil, false
	}
	params := make(map[string]any)
	for {
		rest = strings.TrimSpace(rest)
		if rest == "" {
			return nil, false
		}
		if strings.HasPrefix(strings.ToLower(rest), "</function>") {
			rest = strings.TrimSpace(rest[len("</function>"):])
			break
		}
		key, value, nextRest, ok := consumeXMLParameter(rest)
		if !ok {
			return nil, false
		}
		params[key] = value
		rest = nextRest
	}
	if strings.TrimSpace(rest) != "" {
		return nil, false
	}

	argsValue := normalizeXMLStyleArguments(params)
	args, err := json.Marshal(argsValue)
	if err != nil {
		return nil, false
	}
	return &ToolCall{
		Index: index,
		Type:  "function",
		Function: &ToolFunction{
			Name:      strings.TrimSpace(funcName),
			Arguments: string(args),
		},
	}, true
}

func normalizeXMLStyleArguments(params map[string]any) any {
	if len(params) != 1 {
		return params
	}
	for key, value := range params {
		lowerKey := strings.ToLower(strings.TrimSpace(key))
		if lowerKey != "arguments_json" && lowerKey != "args_json" {
			return params
		}
		raw := strings.TrimSpace(fmt.Sprint(value))
		if raw == "" {
			return map[string]any{}
		}
		var decoded any
		if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
			return params
		}
		return decoded
	}
	return params
}

func marshalMalformedXMLStyleArguments(params map[string]any) (string, bool) {
	argsValue := normalizeMalformedXMLStyleArguments(params)
	args, err := json.Marshal(argsValue)
	if err != nil {
		return "", false
	}
	return string(args), true
}

func normalizeMalformedXMLStyleArguments(params map[string]any) any {
	if len(params) != 1 {
		return params
	}
	for key, value := range params {
		lowerKey := strings.ToLower(strings.TrimSpace(key))
		if lowerKey != "arguments_json" && lowerKey != "args_json" {
			return params
		}
		raw := strings.TrimSpace(fmt.Sprint(value))
		if raw == "" {
			return map[string]any{}
		}
		decoded, ok := decodePossiblyPartialJSON(raw)
		if !ok {
			return params
		}
		return decoded
	}
	return params
}

func decodePossiblyPartialJSON(raw string) (any, bool) {
	var decoded any
	if err := json.Unmarshal([]byte(raw), &decoded); err == nil {
		return decoded, true
	}
	repaired := repairPartialJSON(raw)
	if repaired == raw {
		return nil, false
	}
	if err := json.Unmarshal([]byte(repaired), &decoded); err != nil {
		return nil, false
	}
	return decoded, true
}

func repairPartialJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	switch raw[0] {
	case '{', '[', '"':
	default:
		return raw
	}

	var builder strings.Builder
	stack := make([]byte, 0, 8)
	inString := false
	escape := false
	for index := 0; index < len(raw); index++ {
		ch := raw[index]
		builder.WriteByte(ch)
		if inString {
			if escape {
				escape = false
				continue
			}
			if ch == '\\' {
				escape = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}

		switch ch {
		case '"':
			inString = true
		case '{':
			stack = append(stack, '}')
		case '[':
			stack = append(stack, ']')
		case '}', ']':
			if len(stack) > 0 && stack[len(stack)-1] == ch {
				stack = stack[:len(stack)-1]
			}
		}
	}

	repaired := strings.TrimRight(builder.String(), " \r\n\t")
	if escape && strings.HasSuffix(repaired, `\`) {
		repaired = repaired[:len(repaired)-1]
	}
	if inString {
		repaired += `"`
	}
	repaired = strings.TrimRight(repaired, " \r\n\t")
	for strings.HasSuffix(repaired, ",") || strings.HasSuffix(repaired, ":") {
		repaired = strings.TrimRight(strings.TrimSuffix(strings.TrimSuffix(repaired, ","), ":"), " \r\n\t")
	}
	for index := len(stack) - 1; index >= 0; index-- {
		repaired += string(stack[index])
	}
	return repaired
}

func consumeXMLFunctionOpen(content string) (string, string, bool) {
	content = strings.TrimSpace(content)
	const prefix = "<function="
	if !strings.HasPrefix(strings.ToLower(content), prefix) {
		return "", content, false
	}
	end := strings.Index(content, ">")
	if end < 0 {
		return "", content, false
	}
	name := strings.TrimSpace(content[len(prefix):end])
	return name, content[end+1:], name != ""
}

func consumeXMLParameter(content string) (string, string, string, bool) {
	content = strings.TrimSpace(content)
	const prefix = "<parameter="
	if !strings.HasPrefix(strings.ToLower(content), prefix) {
		return "", "", content, false
	}
	nameEnd := strings.Index(content, ">")
	if nameEnd < 0 {
		return "", "", content, false
	}
	name := strings.TrimSpace(content[len(prefix):nameEnd])
	if name == "" {
		return "", "", content, false
	}
	body := content[nameEnd+1:]
	closeTag := "</parameter>"
	lowerBody := strings.ToLower(body)
	valueEnd := strings.Index(lowerBody, closeTag)
	if valueEnd >= 0 {
		return name, strings.TrimSpace(body[:valueEnd]), body[valueEnd+len(closeTag):], true
	}
	functionCloseTag := "</function>"
	functionCloseIndex := strings.Index(lowerBody, functionCloseTag)
	if functionCloseIndex < 0 {
		return "", "", content, false
	}
	return name, strings.TrimSpace(body[:functionCloseIndex]), body[functionCloseIndex:], true
}

func consumePseudoTag(content, tag string) (string, string, bool) {
	openTag := "<" + tag + ">"
	closeTag := "</" + tag + ">"
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, openTag) {
		return "", content, false
	}
	content = content[len(openTag):]
	end := strings.Index(content, closeTag)
	if end < 0 {
		return "", content, false
	}
	value := content[:end]
	rest := content[end+len(closeTag):]
	return value, rest, true
}
