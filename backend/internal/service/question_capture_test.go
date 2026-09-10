//go:build unit

package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQuestionCapture(t *testing.T) {
	c := NewQuestionCapture(1, 2, "model", " original prompt ", []string{"secret-token"})
	c.Observe(TestEvent{Type: "content", Text: "answer"})
	c.Observe(TestEvent{Type: "test_complete", Success: true})
	c.Observe(TestEvent{Type: "content", Text: "late"})
	result := c.Record()
	require.Equal(t, " original prompt ", result.Prompt)
	require.Equal(t, "answer", result.Answer)
	require.Equal(t, "completed", result.TransportState)
	c = NewQuestionCapture(1, 2, "model", "prompt", []string{"secret-token"})
	c.Observe(TestEvent{Type: "content", Text: "secret-"})
	c.Observe(TestEvent{Type: "content", Text: "token"})
	c.Observe(TestEvent{Type: "test_complete", Success: true})
	result = c.Record()
	require.Empty(t, result.Answer)
	require.Equal(t, "incomplete", result.TransportState)
	c = NewQuestionCapture(1, 2, "model", "prompt", nil)
	c.Observe(TestEvent{Type: "content", Text: strings.Repeat("a", 32769)})
	c.Observe(TestEvent{Type: "test_complete", Success: true})
	require.Equal(t, "incomplete", c.Record().TransportState)
	require.Empty(t, c.Record().Answer)
	c = NewQuestionCapture(1, 2, "model", "prompt", nil)
	c.Observe(TestEvent{Type: "error", Error: "secret raw error"})
	require.Equal(t, "failed", c.Record().TransportState)
	require.Empty(t, c.Record().Answer)
}
