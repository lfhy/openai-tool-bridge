package bridge

import (
	"encoding/json"
	"strings"
)

func marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func marshalString(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(data)
}

func unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func isEmptyContent(content any) bool {
	switch value := content.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(value) == ""
	default:
		return strings.TrimSpace(marshalString(value)) == ""
	}
}
