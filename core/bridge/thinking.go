package bridge

import "strings"

const promptBridgeThinkingPrefix = "Thinking...\n> "

func ExtractPromptBridgeThinkingContent(content string) (string, string, bool) {
	if content == "" {
		return content, "", false
	}
	start := findPromptBridgeThinkingStart(content)
	if start < 0 {
		return content, "", false
	}
	visiblePrefix := content[:start]
	suffix := content[start:]
	if reasoning, visibleSuffix, complete := extractPromptBridgeThinking(suffix); complete {
		return visiblePrefix + visibleSuffix, reasoning, true
	}
	if reasoning := finalizePromptBridgeThinking(suffix); reasoning != "" {
		return visiblePrefix, reasoning, true
	}
	return content, "", false
}

func ConsumeStreamPromptBridgeThinking(content string, pending *stringBuilder) (string, string, bool) {
	aggregate := content
	if pending != nil && pending.Len() > 0 {
		aggregate = pending.String() + content
		pending.Reset()
	}

	if start := findPromptBridgeThinkingStart(aggregate); start >= 0 {
		visiblePrefix := aggregate[:start]
		suffix := aggregate[start:]
		if reasoning, visibleSuffix, complete := extractPromptBridgeThinking(suffix); complete {
			return visiblePrefix + visibleSuffix, reasoning, false
		}
		if looksLikePromptBridgeThinkingPrefix(suffix) {
			if pending != nil {
				pending.WriteString(suffix)
			}
			return visiblePrefix, "", true
		}
	}

	if looksLikePromptBridgeThinkingPrefix(aggregate) {
		if pending != nil {
			pending.WriteString(aggregate)
		}
		return "", "", true
	}

	return aggregate, "", false
}

func ConsumeStreamPromptBridgeThinkingIncremental(content string, pending *stringBuilder, emitted *string) (string, string, bool, bool) {
	aggregate := content
	if pending != nil && pending.Len() > 0 {
		aggregate = pending.String() + content
		pending.Reset()
	}

	if start := findPromptBridgeThinkingStart(aggregate); start >= 0 {
		visiblePrefix := aggregate[:start]
		suffix := aggregate[start:]
		if reasoning, visibleSuffix, complete := extractPromptBridgeThinking(suffix); complete {
			return visiblePrefix + visibleSuffix, diffPromptBridgeReasoning(emitted, reasoning), false, true
		}
		if looksLikePromptBridgeThinkingPrefix(suffix) {
			if pending != nil {
				pending.WriteString(suffix)
			}
			return visiblePrefix, diffPromptBridgeReasoning(emitted, finalizePromptBridgeThinking(suffix)), true, false
		}
	}

	if looksLikePromptBridgeThinkingPrefix(aggregate) {
		if pending != nil {
			pending.WriteString(aggregate)
		}
		return "", diffPromptBridgeReasoning(emitted, finalizePromptBridgeThinking(aggregate)), true, false
	}

	return aggregate, "", false, false
}

func FinalizePromptBridgeThinking(content string) string {
	return finalizePromptBridgeThinking(content)
}

func diffPromptBridgeReasoning(emitted *string, current string) string {
	current = strings.TrimSpace(current)
	if current == "" {
		return ""
	}
	if emitted == nil {
		return current
	}
	prev := strings.TrimSpace(*emitted)
	if prev == "" {
		*emitted = current
		return current
	}
	if strings.HasPrefix(current, prev) {
		delta := current[len(prev):]
		*emitted = current
		return delta
	}
	*emitted = current
	return current
}

func looksLikePromptBridgeThinkingPrefix(content string) bool {
	if content == "" {
		return false
	}
	trimmed := strings.TrimLeft(content, "\r\n\t ")
	if trimmed == "" {
		return false
	}
	lowerTrimmed := strings.ToLower(trimmed)
	lowerPrefix := strings.ToLower(promptBridgeThinkingPrefix)
	if strings.HasPrefix(lowerTrimmed, lowerPrefix) {
		return true
	}
	if len(lowerTrimmed) > len(lowerPrefix) {
		return false
	}
	return strings.HasPrefix(lowerPrefix, lowerTrimmed)
}

func findPromptBridgeThinkingStart(content string) int {
	for index := 0; index < len(content); index++ {
		suffix := content[index:]
		if looksLikePromptBridgeThinkingPrefix(suffix) {
			return index
		}
	}
	return -1
}

func extractPromptBridgeThinking(content string) (string, string, bool) {
	trimmed := strings.TrimLeft(content, "\r\n\t ")
	if len(trimmed) < len(promptBridgeThinkingPrefix) || !strings.EqualFold(trimmed[:len(promptBridgeThinkingPrefix)], promptBridgeThinkingPrefix) {
		return "", "", false
	}
	body := trimmed[len(promptBridgeThinkingPrefix):]
	if end := findPromptBridgeThinkingEnd(body); end >= 0 {
		return normalizePromptBridgeThinking(body[:end]), body[end:], true
	}
	return "", "", false
}

func finalizePromptBridgeThinking(content string) string {
	trimmed := strings.TrimLeft(content, "\r\n\t ")
	if len(trimmed) < len(promptBridgeThinkingPrefix) || !strings.EqualFold(trimmed[:len(promptBridgeThinkingPrefix)], promptBridgeThinkingPrefix) {
		return ""
	}
	body := trimmed[len(promptBridgeThinkingPrefix):]
	if end := findPromptBridgeThinkingEnd(body); end >= 0 {
		body = body[:end]
	}
	return normalizePromptBridgeThinking(body)
}

func findPromptBridgeThinkingEnd(body string) int {
	searchFrom := 0
	for {
		offset := strings.Index(body[searchFrom:], "\n\n")
		if offset < 0 {
			break
		}
		candidate := searchFrom + offset
		remainder := strings.TrimLeft(body[candidate+2:], " \r\t")
		if strings.HasPrefix(remainder, ">") {
			searchFrom = candidate + 2
			continue
		}
		return candidate + 2
	}
	if toolStart := findUnquotedPseudoToolCallStart(body); toolStart >= 0 {
		return toolStart
	}
	return -1
}

func findUnquotedPseudoToolCallStart(content string) int {
	lineStart := 0
	for lineStart < len(content) {
		lineEnd := strings.Index(content[lineStart:], "\n")
		if lineEnd < 0 {
			lineEnd = len(content) - lineStart
		}
		line := content[lineStart : lineStart+lineEnd]
		trimmed := strings.TrimLeft(line, "\r\t ")
		if trimmed != "" && !strings.HasPrefix(trimmed, ">") {
			prefixLen := len(line) - len(trimmed)
			if start := FindPseudoToolCallStart(trimmed); start >= 0 && strings.TrimSpace(trimmed[:start]) == "" {
				return lineStart + prefixLen + start
			}
		}
		lineStart += lineEnd + 1
	}
	return -1
}

func normalizePromptBridgeThinking(raw string) string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	normalized := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "> ")
		if line == ">" {
			line = ""
		}
		normalized = append(normalized, line)
	}
	return strings.TrimSpace(strings.Join(normalized, "\n"))
}
