package service

import (
	"errors"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

var errOpenAILocalOutputFailure = errors.New("local OpenAI output preparation failed")

func localOpenAIOutputFailure(err error) error {
	return fmt.Errorf("%w: %w", errOpenAILocalOutputFailure, err)
}

// Deny replay without claiming that downstream semantic bytes were delivered.
func denyOpenAIMessagesReplay(c *gin.Context) {
	if state := OpenAIAttemptStateFromContext(c); state != nil {
		state.ReplaySafe = false
	}
	if budget := OpenAIRetryBudgetFromContext(c); budget != nil {
		budget.mu.Lock()
		budget.replaySafe = false
		budget.mu.Unlock()
	}
}

// Unknown/provider-hosted actions are not known to be replayable. This list
// only admits protocol structure and locally converted, non-hosted output.
func openAIMessagesEventAllowsUncommittedReplay(payload, eventType string) bool {
	switch eventType {
	case "response.created", "response.queued", "response.in_progress", "response.completed", "response.incomplete", "response.failed", "error":
		return true
	case "response.output_item.added", "response.output_item.done":
		switch gjson.Get(payload, "item.type").String() {
		case "message", "reasoning", "function_call":
			return true
		}
	case "response.content_part.added", "response.content_part.done", "response.reasoning_summary_part.added", "response.reasoning_summary_part.done":
		switch gjson.Get(payload, "part.type").String() {
		case "output_text", "refusal", "reasoning_text", "summary_text":
			return true
		}
	case "response.output_text.delta", "response.output_text.done", "response.refusal.delta", "response.refusal.done",
		"response.function_call_arguments.delta", "response.function_call_arguments.done",
		"response.reasoning_summary_text.delta", "response.reasoning_summary_text.done",
		"response.reasoning_text.delta", "response.reasoning_text.done":
		return true
	}
	return false
}
