package service

import (
	"strconv"
	"time"

	"github.com/tidwall/gjson"
)

// Attempt-local and single-consumer. Retains counters, not response text.
type openAIVisibleStreamObserver struct {
	origin    RequestOrigin
	started   time.Time
	first     time.Time
	last      time.Time
	deltas    int
	input     *int64
	cached    *int64
	visible   *int64
	terminal  bool
	completed bool
	invalid   bool
}

func newOpenAIVisibleStreamObserver(origin RequestOrigin, started time.Time) *openAIVisibleStreamObserver {
	return &openAIVisibleStreamObserver{origin: origin, started: started}
}

func (o *openAIVisibleStreamObserver) observe(data []byte, eventType string, receivedAt time.Time) {
	if o == nil || string(data) == "[DONE]" {
		return
	}
	if !gjson.ValidBytes(data) {
		o.invalid = true
		return
	}
	root := gjson.ParseBytes(data)
	switch eventType {
	case "response.output_text.delta":
		if o.terminal {
			o.invalid = true
			return
		}
		delta := root.Get("delta")
		if delta.Type != gjson.String {
			o.invalid = true
			return
		}
		if delta.Str == "" {
			return
		}
		if receivedAt.Before(o.started) || (!o.last.IsZero() && receivedAt.Before(o.last)) {
			o.invalid = true
			return
		}
		if o.deltas == 0 {
			o.first = receivedAt
		}
		o.last = receivedAt
		o.deltas++
	case "response.output_item.added", "response.output_item.done":
		if !openAIObservationTextItem(root.Get("item")) {
			o.invalid = true
		}
	case "response.content_part.added", "response.content_part.done":
		if root.Get("part.type").String() != "output_text" {
			o.invalid = true
		}
	case "response.completed", "response.done":
		if o.terminal {
			o.invalid = true
			return
		}
		o.terminal = true
		response := root.Get("response")
		if response.Get("status").String() != "completed" {
			return
		}
		o.completed = true
		usage := response.Get("usage")
		o.input = openAIObservationCount(usage.Get("input_tokens"))
		cached := openAIObservationCount(usage.Get("input_tokens_details.cached_tokens"))
		if o.input != nil && cached != nil && *cached <= *o.input {
			o.cached = cached
		}
		output := response.Get("output")
		if !output.IsArray() {
			return
		}
		hasText := false
		for _, item := range output.Array() {
			if !openAIObservationTextItem(item) {
				o.invalid = true
				return
			}
			if item.Get("type").String() == "message" {
				for _, part := range item.Get("content").Array() {
					text := part.Get("text")
					if text.Type == gjson.String && text.Str != "" {
						hasText = true
					}
				}
			}
		}
		if !hasText {
			return
		}
		total := openAIObservationCount(usage.Get("output_tokens"))
		reasoning := openAIObservationCount(usage.Get("output_tokens_details.reasoning_tokens"))
		// An absent reasoning detail is unknown, not a zero-token reasoning phase.
		if total == nil || reasoning == nil || *reasoning > *total {
			return
		}
		usage.Get("output_tokens_details").ForEach(func(key, value gjson.Result) bool {
			if key.Str != "reasoning_tokens" {
				n := openAIObservationCount(value)
				if n == nil || *n != 0 {
					o.invalid = true
				}
			}
			return true
		})
		visible := *total - *reasoning
		if visible > 0 {
			o.visible = &visible
		}
	case "response.created", "response.in_progress", "response.queued", "response.output_text.done",
		"response.reasoning_summary_part.added", "response.reasoning_summary_part.done",
		"response.reasoning_summary_text.delta", "response.reasoning_summary_text.done",
		"response.reasoning_text.delta", "response.reasoning_text.done":
		// Metadata and reasoning are not visible text deltas.
	default:
		// Includes failures, tools, refusal, audio, images and unknown event kinds.
		o.invalid = true
	}
}

func openAIObservationTextItem(item gjson.Result) bool {
	switch item.Get("type").String() {
	case "reasoning":
		return true
	case "message":
		role := item.Get("role")
		if role.Exists() && role.String() != "assistant" {
			return false
		}
		content := item.Get("content")
		if !content.IsArray() {
			return false
		}
		for _, part := range content.Array() {
			if part.Get("type").String() != "output_text" || part.Get("text").Type != gjson.String {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func openAIObservationCount(value gjson.Result) *int64 {
	if value.Type != gjson.Number {
		return nil
	}
	count, err := strconv.ParseInt(value.Raw, 10, 64)
	if err != nil || count < 0 {
		return nil
	}
	return &count
}

func (o *openAIVisibleStreamObserver) snapshot(delivered bool) *VisibleOutputObservation {
	if o == nil {
		return nil
	}
	observation := &VisibleOutputObservation{
		Origin: o.origin, RequestStartedAt: o.started, TextDeltaCount: o.deltas,
		Version: VisibleStreamObservationVersion, Method: VisibleStreamObservationMethod,
		OutputComplete: delivered && o.completed && !o.invalid,
	}
	if o.deltas > 0 {
		first, last := o.first, o.last
		observation.FirstVisibleTextAt, observation.LastVisibleTextAt = &first, &last
	}
	if o.input != nil {
		value := *o.input
		observation.InputTokensTotal = &value
	}
	if o.cached != nil {
		value := *o.cached
		observation.CacheReadTokens = &value
	}
	if o.visible != nil && !o.invalid {
		value := *o.visible
		observation.VisibleOutputTokens = &value
	}
	return observation
}
