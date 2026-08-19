package service

import (
	"strings"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// normalizeGrokResponsesPayloadForClient fills the Responses response timestamp
// that strict clients require. xAI occasionally omits created_at from the
// nested response object in SSE events, or from a non-streaming response body.
// Preserve any upstream value and leave unrelated event payloads unchanged.
func normalizeGrokResponsesPayloadForClient(payload []byte, fallbackCreatedAt int64) ([]byte, bool) {
	if len(payload) == 0 || !gjson.ValidBytes(payload) {
		return payload, false
	}
	if fallbackCreatedAt <= 0 {
		fallbackCreatedAt = time.Now().Unix()
	}

	eventType := strings.TrimSpace(gjson.GetBytes(payload, "type").String())
	if response := gjson.GetBytes(payload, "response"); strings.HasPrefix(eventType, "response.") && response.Exists() && response.IsObject() {
		createdAt := response.Get("created_at")
		if createdAt.Exists() && createdAt.Type != gjson.Null {
			return payload, false
		}
		updated, err := sjson.SetBytes(payload, "response.created_at", fallbackCreatedAt)
		if err != nil {
			return payload, false
		}
		return updated, true
	}

	// A non-streaming Responses document has the response fields at the root.
	// Require a Responses-specific marker so Chat Completions payloads with a
	// root id are not modified.
	objectType := strings.TrimSpace(gjson.GetBytes(payload, "object").String())
	if objectType != "response" && !gjson.GetBytes(payload, "output").Exists() {
		return payload, false
	}
	createdAt := gjson.GetBytes(payload, "created_at")
	if createdAt.Exists() && createdAt.Type != gjson.Null {
		return payload, false
	}
	updated, err := sjson.SetBytes(payload, "created_at", fallbackCreatedAt)
	if err != nil {
		return payload, false
	}
	return updated, true
}

// normalizeGrokResponsesSSEBodyForClient applies the same compatibility fix to
// buffered SSE responses. It preserves event fields, separators, and line
// endings while rewriting only data payloads that contain a Responses object.
func normalizeGrokResponsesSSEBodyForClient(body []byte, fallbackCreatedAt int64) []byte {
	if len(body) == 0 {
		return body
	}
	lines := strings.Split(string(body), "\n")
	changed := false
	for i, line := range lines {
		lineWithoutCR := strings.TrimSuffix(line, "\r")
		data, ok := extractOpenAISSEDataLine(lineWithoutCR)
		if !ok {
			continue
		}
		normalized, normalizedChanged := normalizeGrokResponsesPayloadForClient([]byte(data), fallbackCreatedAt)
		if !normalizedChanged {
			continue
		}
		dataStart := strings.Index(lineWithoutCR, data)
		if dataStart < 0 {
			continue
		}
		lines[i] = lineWithoutCR[:dataStart] + string(normalized)
		if strings.HasSuffix(line, "\r") {
			lines[i] += "\r"
		}
		changed = true
	}
	if !changed {
		return body
	}
	return []byte(strings.Join(lines, "\n"))
}
