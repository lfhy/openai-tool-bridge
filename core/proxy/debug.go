package proxy

import (
	"io"
	"log"
	"strings"
)

type debugLogger struct {
	enabled bool
}

func newDebugLogger(enabled bool) *debugLogger {
	return &debugLogger{enabled: enabled}
}

func (l *debugLogger) Enabled() bool {
	return l != nil && l.enabled
}

func (l *debugLogger) Printf(format string, args ...any) {
	if !l.Enabled() {
		return
	}
	log.Printf("[debug] "+format, args...)
}

func (l *debugLogger) DumpText(label string, text string) {
	if !l.Enabled() {
		return
	}
	log.Printf("[debug] %s:\n%s", label, text)
}

func (l *debugLogger) DumpBytes(label string, data []byte) {
	if !l.Enabled() {
		return
	}
	l.DumpText(label, string(data))
}

type debugWriter struct {
	writer io.Writer
	logger *debugLogger
	label  string
}

func (w *debugWriter) Write(p []byte) (int, error) {
	if w.logger != nil && w.logger.Enabled() && len(p) > 0 {
		w.logger.DumpBytes(w.label, p)
	}
	return w.writer.Write(p)
}

func maskAuthorization(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if index := strings.IndexByte(value, ' '); index >= 0 {
		return value[:index] + " " + maskToken(strings.TrimSpace(value[index+1:]))
	}
	return maskToken(value)
}

func maskToken(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 8 {
		return "***"
	}
	return value[:4] + "..." + value[len(value)-4:]
}
