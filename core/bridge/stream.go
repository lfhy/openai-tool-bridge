package bridge

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"
)

func RewriteStream(dst io.Writer, src io.Reader) error {
	return RewriteStreamWithHooks(dst, src, nil)
}

func RewriteStreamWithHooks(dst io.Writer, src io.Reader, hooks *StreamDebugHooks) error {
	reader := newSSEReader(src)
	state := &streamState{
		toolCallMeta: make(map[int]*streamToolCallMeta),
	}

	for {
		frame, err := reader.Next()
		if frame.raw != "" {
			if hooks != nil && hooks.OnInputFrame != nil {
				hooks.OnInputFrame(frame.raw)
			}
			if stop, processErr := processStreamFrame(dst, state, frame); processErr != nil {
				return processErr
			} else if stop {
				break
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
	}

	if state.promptBridgeThinking.Len() > 0 {
		if reasoning := diffPromptBridgeReasoning(&state.emittedReasoning, FinalizePromptBridgeThinking(state.promptBridgeThinking.String())); reasoning != "" {
			chunk := state.template
			chunk.Choices = []*ChunkChoice{{
				Index: 0,
				Delta: Delta{
					Role:             "assistant",
					ReasoningContent: reasoning,
					Reasoning:        reasoning,
				},
			}}
			if err := writeSSEJSON(dst, chunk); err != nil {
				return err
			}
		}
	}

	if state.pseudoToolContent.Len() > 0 {
		chunk := state.template
		chunk.Choices = []*ChunkChoice{{
			Index: 0,
			Delta: Delta{
				Role:    "assistant",
				Content: state.pseudoToolContent.String(),
			},
		}}
		if err := writeSSEJSON(dst, chunk); err != nil {
			return err
		}
	}

	if len(state.collectedToolCalls) > 0 {
		normalizeToolCalls(state.collectedToolCalls)
		chunk := state.template
		chunk.Choices = []*ChunkChoice{{
			Index: 0,
			Delta: Delta{
				Role:      "assistant",
				ToolCalls: state.collectedToolCalls,
			},
		}}
		if err := writeSSEJSON(dst, chunk); err != nil {
			return err
		}
		stop := state.template
		stop.Choices = []*ChunkChoice{{
			Index:        0,
			Delta:        Delta{Role: "assistant"},
			FinishReason: "tool_calls",
		}}
		if err := writeSSEJSON(dst, stop); err != nil {
			return err
		}
	}

	return writeSSEDone(dst)
}

func processStreamFrame(dst io.Writer, state *streamState, frame sseFrame) (bool, error) {
	if !frame.hasData {
		return false, nil
	}
	if frame.done {
		return true, nil
	}

	var chunk ChatCompletionChunk
	if err := unmarshal([]byte(frame.data), &chunk); err != nil {
		return false, fmt.Errorf("decode chunk: %w", err)
	}
	if chunk.ID != "" {
		state.template.ID = chunk.ID
	}
	if chunk.Object != "" {
		state.template.Object = chunk.Object
	}
	if chunk.Created != 0 {
		state.template.Created = chunk.Created
	}
	if chunk.Model != "" {
		state.template.Model = chunk.Model
	}
	if chunk.SystemFingerprint != nil {
		state.template.SystemFingerprint = chunk.SystemFingerprint
	}
	if chunk.Usage != nil && len(chunk.Choices) == 0 {
		return false, writeSSEJSON(dst, chunk)
	}
	if len(chunk.Choices) == 0 || chunk.Choices[0] == nil {
		return false, nil
	}

	choice := chunk.Choices[0]
	if choice.Delta.Role == "" {
		choice.Delta.Role = "assistant"
	}
	if choice.Delta.Reasoning != "" && choice.Delta.ReasoningContent == "" {
		choice.Delta.ReasoningContent = choice.Delta.Reasoning
	}

	content, _ := choice.Delta.Content.(string)
	if content != "" {
		visibleContent, reasoning, buffering, completed := ConsumeStreamPromptBridgeThinkingIncremental(content, &state.promptBridgeThinking, &state.emittedReasoning)
		if reasoning != "" {
			choice.Delta.ReasoningContent = joinReasoning(choice.Delta.ReasoningContent, reasoning)
			choice.Delta.Reasoning = choice.Delta.ReasoningContent
		}
		if buffering && strings.TrimSpace(visibleContent) == "" {
			visibleContent = ""
		}
		if visibleContent != content {
			if visibleContent == "" {
				choice.Delta.Content = nil
			} else {
				choice.Delta.Content = visibleContent
			}
			content = visibleContent
		}
		if completed {
			state.emittedReasoning = ""
		}
	}

	if content != "" {
		visibleContent, parsedCalls, buffering := consumeStreamPseudoToolContent(content, &state.pseudoToolContent)
		if len(parsedCalls) > 0 {
			normalizeStreamToolCalls(parsedCalls, state.toolCallMeta)
			normalizeToolCalls(parsedCalls)
			state.collectedToolCalls = append(state.collectedToolCalls, parsedCalls...)
		}
		if buffering && strings.TrimSpace(visibleContent) == "" {
			visibleContent = ""
		}
		if visibleContent != content {
			if visibleContent == "" {
				choice.Delta.Content = nil
			} else {
				choice.Delta.Content = visibleContent
			}
			content = visibleContent
		}
	}

	if len(choice.Delta.ToolCalls) > 0 {
		normalizeStreamToolCalls(choice.Delta.ToolCalls, state.toolCallMeta)
		normalizeToolCalls(choice.Delta.ToolCalls)
		state.collectedToolCalls = append(state.collectedToolCalls, choice.Delta.ToolCalls...)
	}

	if isEmptyContent(choice.Delta.Content) && choice.Delta.ReasoningContent == "" && len(choice.Delta.ToolCalls) == 0 {
		return false, nil
	}

	chunk.Choices = []*ChunkChoice{choice}
	return false, writeSSEJSON(dst, chunk)
}

func consumeStreamPseudoToolContent(content string, pending *stringBuilder) (string, []*ToolCall, bool) {
	aggregate := content
	if pending != nil && pending.Len() > 0 {
		aggregate = pending.String() + content
		pending.Reset()
	}

	if retained, calls, parsed := ExtractPseudoToolCallsFromContent(aggregate); parsed {
		return retained, calls, false
	}
	if start := FindPseudoToolCallStart(aggregate); start >= 0 {
		visiblePrefix := aggregate[:start]
		suffix := aggregate[start:]
		if retained, calls, parsed := ExtractPseudoToolCallsFromContent(suffix); parsed {
			return visiblePrefix + retained, calls, false
		}
		if LooksLikePseudoToolCallContent(suffix) || LooksLikePseudoToolCallPrefix(suffix) {
			if pending != nil {
				pending.WriteString(suffix)
			}
			return visiblePrefix, nil, true
		}
	}
	if LooksLikePseudoToolCallContent(aggregate) || LooksLikePseudoToolCallPrefix(aggregate) {
		if pending != nil {
			pending.WriteString(aggregate)
		}
		return "", nil, true
	}
	return aggregate, nil, false
}

func normalizeStreamToolCalls(calls []*ToolCall, meta map[int]*streamToolCallMeta) {
	for _, call := range calls {
		if call == nil {
			continue
		}
		state := meta[call.Index]
		if state == nil {
			state = &streamToolCallMeta{}
			meta[call.Index] = state
		}
		if strings.TrimSpace(call.ID) != "" {
			state.ID = call.ID
		} else if state.ID != "" {
			call.ID = state.ID
		}
		if strings.TrimSpace(call.Type) != "" {
			state.Type = call.Type
		} else if state.Type != "" {
			call.Type = state.Type
		} else if call.Function != nil {
			call.Type = "function"
			state.Type = call.Type
		}
		if call.Function == nil {
			continue
		}
		if strings.TrimSpace(call.Function.Name) != "" {
			state.Name = call.Function.Name
		}
	}
}

func joinReasoning(current string, extra string) string {
	if current == "" {
		return extra
	}
	return current + extra
}

type sseFrame struct {
	raw     string
	data    string
	hasData bool
	done    bool
}

type sseReader struct {
	reader *bufio.Reader
}

func newSSEReader(r io.Reader) *sseReader {
	return &sseReader{
		reader: bufio.NewReaderSize(r, 1024*1024),
	}
}

func (r *sseReader) Next() (sseFrame, error) {
	var raw bytes.Buffer
	var dataLines []string
	var hasData bool

	for {
		line, err := r.reader.ReadString('\n')
		if line != "" {
			raw.WriteString(line)
		}
		if err != nil && err != io.EOF {
			return sseFrame{raw: raw.String()}, err
		}

		trimmed := strings.TrimRight(line, "\r\n")
		if trimmed == "" {
			frame := sseFrame{
				raw:     raw.String(),
				data:    strings.Join(dataLines, "\n"),
				hasData: hasData,
			}
			if strings.TrimSpace(frame.data) == "[DONE]" {
				frame.done = true
			}
			return frame, err
		}
		if strings.HasPrefix(trimmed, "data:") {
			hasData = true
			dataLines = append(dataLines, strings.TrimSpace(trimmed[len("data:"):]))
		}
		if err == io.EOF {
			frame := sseFrame{
				raw:     raw.String(),
				data:    strings.Join(dataLines, "\n"),
				hasData: hasData,
			}
			if strings.TrimSpace(frame.data) == "[DONE]" {
				frame.done = true
			}
			return frame, io.EOF
		}
	}
}

func writeSSEJSON(w io.Writer, payload any) error {
	data, err := marshal(payload)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", data)
	return err
}

func writeSSEDone(w io.Writer) error {
	_, err := io.WriteString(w, "data: [DONE]\n\n")
	return err
}
