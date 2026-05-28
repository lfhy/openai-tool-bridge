package bridge

type Message struct {
	Role             string      `json:"role,omitempty"`
	Content          any         `json:"content,omitempty"`
	ReasoningContent string      `json:"reasoning_content,omitempty"`
	Reasoning        string      `json:"reasoning,omitempty"`
	ToolCalls        []*ToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string      `json:"tool_call_id,omitempty"`
}

type ToolDefinition struct {
	Type     string              `json:"type,omitempty"`
	Function *FunctionDefinition `json:"function,omitempty"`
}

type FunctionDefinition struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
}

type ToolCall struct {
	Index    int           `json:"index,omitempty"`
	ID       string        `json:"id,omitempty"`
	Type     string        `json:"type,omitempty"`
	Function *ToolFunction `json:"function,omitempty"`
}

type ToolFunction struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type ChatCompletionResponse struct {
	ID                string            `json:"id,omitempty"`
	Object            string            `json:"object,omitempty"`
	Created           int64             `json:"created,omitempty"`
	Model             string            `json:"model,omitempty"`
	Choices           []*ResponseChoice `json:"choices,omitempty"`
	SystemFingerprint any               `json:"system_fingerprint,omitempty"`
	Usage             any               `json:"usage,omitempty"`
}

type ResponseChoice struct {
	Index        int      `json:"index,omitempty"`
	Message      *Message `json:"message,omitempty"`
	FinishReason any      `json:"finish_reason,omitempty"`
}

type ChatCompletionChunk struct {
	ID                string         `json:"id,omitempty"`
	Object            string         `json:"object,omitempty"`
	Created           int64          `json:"created,omitempty"`
	Model             string         `json:"model,omitempty"`
	Choices           []*ChunkChoice `json:"choices,omitempty"`
	SystemFingerprint any            `json:"system_fingerprint,omitempty"`
	Usage             any            `json:"usage,omitempty"`
}

type ChunkChoice struct {
	Index        int   `json:"index,omitempty"`
	Delta        Delta `json:"delta,omitempty"`
	FinishReason any   `json:"finish_reason,omitempty"`
	Logprobs     any   `json:"logprobs,omitempty"`
	MatchedStop  any   `json:"matched_stop,omitempty"`
}

type Delta struct {
	Role             string      `json:"role,omitempty"`
	Content          any         `json:"content,omitempty"`
	ReasoningContent string      `json:"reasoning_content,omitempty"`
	Reasoning        string      `json:"reasoning,omitempty"`
	ToolCalls        []*ToolCall `json:"tool_calls,omitempty"`
	ExtraContent     *ExtraBody  `json:"extra_content,omitempty"`
}

type ExtraBody struct {
	Google *GoogleExtra `json:"google,omitempty"`
}

type GoogleExtra struct {
	Thought bool `json:"thought,omitempty"`
}

type streamToolCallMeta struct {
	ID   string
	Type string
	Name string
}

type streamState struct {
	pseudoToolContent    stringBuilder
	promptBridgeThinking stringBuilder
	emittedReasoning     string
	toolCallMeta         map[int]*streamToolCallMeta
	collectedToolCalls   []*ToolCall
	template             ChatCompletionChunk
}

type stringBuilder struct {
	value string
}

func (b *stringBuilder) WriteString(value string) {
	b.value += value
}

func (b *stringBuilder) String() string {
	return b.value
}

func (b *stringBuilder) Len() int {
	return len(b.value)
}

func (b *stringBuilder) Reset() {
	b.value = ""
}
