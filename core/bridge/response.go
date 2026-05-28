package bridge

import (
	"fmt"
)

func NormalizeNonStreamResponse(data []byte) ([]byte, error) {
	var resp ChatCompletionResponse
	if err := unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if len(resp.Choices) == 0 || resp.Choices[0] == nil || resp.Choices[0].Message == nil {
		return marshal(resp)
	}

	message := resp.Choices[0].Message
	if message.Reasoning != "" && message.ReasoningContent == "" {
		message.ReasoningContent = message.Reasoning
	}
	if content, ok := message.Content.(string); ok {
		if visibleContent, reasoning, extracted := ExtractPromptBridgeThinkingContent(content); extracted {
			if message.ReasoningContent == "" {
				message.ReasoningContent = reasoning
				message.Reasoning = reasoning
			}
			message.Content = visibleContent
		}
		if retainedContent, parsedCalls, parsed := ExtractPseudoToolCallsFromContent(message.Content.(string)); parsed && len(message.ToolCalls) == 0 {
			message.ToolCalls = parsedCalls
			message.Content = retainedContent
			if resp.Choices[0].FinishReason == nil {
				resp.Choices[0].FinishReason = "tool_calls"
			}
		}
	}
	normalizeToolCalls(message.ToolCalls)
	return marshal(resp)
}

func normalizeToolCalls(calls []*ToolCall) {
	for index, call := range calls {
		if call == nil {
			continue
		}
		if call.ID == "" {
			call.ID = fmt.Sprintf("tool_%d", index)
		}
		if call.Type == "" {
			call.Type = "function"
		}
	}
}
