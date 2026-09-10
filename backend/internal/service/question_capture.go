package service

import (
	"strings"
	"sync"
	"unicode/utf8"
)

// Captures only server-produced events, never browser-supplied response text.
// Errors, headers, media and credentials are not part of the persisted answer.
type QuestionCapture struct {
	mu      sync.Mutex
	record  QuestionRecord
	secrets []string
	invalid bool
	ended   bool
}

func NewQuestionCapture(account, actor int64, model, prompt string, secrets []string) *QuestionCapture {
	return &QuestionCapture{record: QuestionRecord{AccountID: account, CreatedBy: actor, RequestModel: model, Prompt: prompt, TransportState: "incomplete"}, secrets: append([]string(nil), secrets...)}
}
func (c *QuestionCapture) Observe(event TestEvent) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ended {
		return
	}
	switch event.Type {
	case "content":
		if len(c.record.Answer)+len(event.Text) > 32768 || !utf8.ValidString(event.Text) {
			c.invalid = true
			c.record.Answer = ""
			return
		}
		if !c.invalid {
			c.record.Answer += event.Text
		}
	case "error":
		c.record.TransportState = "failed"
		c.ended = true
	case "test_complete":
		if event.Success && !c.invalid {
			c.record.TransportState = "completed"
		}
		c.ended = true
	}
}
func (c *QuestionCapture) Record() QuestionRecord {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := c.record
	for _, secret := range c.secrets {
		if secret != "" && (strings.Contains(result.Answer, secret) || strings.Contains(result.Prompt, secret)) {
			result.Answer = ""
			result.Prompt = "[redacted credential echo]"
			result.TransportState = "incomplete"
		}
	}
	if c.invalid {
		result.Answer = ""
		result.TransportState = "incomplete"
	}
	return result
}
