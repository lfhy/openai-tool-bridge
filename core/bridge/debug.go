package bridge

type StreamDebugHooks struct {
	OnInputFrame func(raw string)
}
