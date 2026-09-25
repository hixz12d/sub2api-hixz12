package service

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

const AccountTestModeQuestion = "question"

// Question answers keep the historical small cap; explicit reasoning needs
// room for hidden reasoning tokens before any visible answer is produced.
const (
	accountQuestionMaxOutputTokens          = 1024
	accountQuestionReasoningMaxOutputTokens = 16384
	accountQuestionTimeout                  = 45 * time.Second
	accountQuestionReasoningTimeout         = 110 * time.Second
)

type accountQuestionModeKey struct{}
type accountQuestionEffortKey struct{}

// NormalizeAccountQuestionReasoningEffort accepts an empty value (upstream
// default) or one of the OpenAI reasoning effort levels.
func NormalizeAccountQuestionReasoningEffort(effort string) (string, bool) {
	effort = strings.ToLower(strings.TrimSpace(effort))
	switch effort {
	case "", "none", "minimal", "low", "medium", "high", "xhigh":
		return effort, true
	}
	return "", false
}

func withAccountQuestionReasoningEffort(ctx context.Context, effort string) context.Context {
	return context.WithValue(ctx, accountQuestionEffortKey{}, effort)
}

func accountQuestionReasoningEffort(ctx context.Context) string {
	value, _ := ctx.Value(accountQuestionEffortKey{}).(string)
	return value
}

// IsAccountQuestionTest distinguishes human review from connectivity recovery.
func IsAccountQuestionTest(mode string) bool {
	return strings.EqualFold(strings.TrimSpace(mode), AccountTestModeQuestion)
}

func accountQuestionMode(ctx context.Context) bool {
	value, _ := ctx.Value(accountQuestionModeKey{}).(bool)
	return value
}

func validateAccountQuestion(account *Account, model, prompt string) error {
	if account == nil || account.Platform != PlatformOpenAI ||
		(account.Type != AccountTypeAPIKey && account.Type != AccountTypeOAuth) ||
		account.IsCredentialShadow() || account.IsOpenAIAgentIdentity() {
		return errors.New("question mode requires a direct OpenAI API key or OAuth account")
	}
	if strings.TrimSpace(prompt) == "" || len(prompt) > 4096 || !utf8.ValidString(prompt) || strings.ContainsRune(prompt, 0) {
		return errors.New("question must contain 1 to 4096 UTF-8 bytes")
	}
	if !validMonitorIdentifier(model, 200) || isOpenAIImageModel(account.GetMappedModel(model)) {
		return errors.New("question mode requires an explicit text model")
	}
	return nil
}

func applyAccountQuestionPayload(payload map[string]any, prompt string, chat, oauth bool, effort string) {
	maxTokens := accountQuestionMaxOutputTokens
	if effort != "" {
		maxTokens = accountQuestionReasoningMaxOutputTokens
	}
	if chat {
		payload["messages"] = []map[string]any{{"role": "user", "content": prompt}}
		payload["max_tokens"] = maxTokens
		if effort != "" {
			payload["reasoning_effort"] = effort
		}
	} else {
		payload["input"] = []map[string]any{{"role": "user", "content": []map[string]any{{"type": "input_text", "text": prompt}}}}
		if !oauth {
			payload["max_output_tokens"] = maxTokens
		}
		if effort != "" {
			payload["reasoning"] = map[string]any{"effort": effort}
		}
	}
	payload["tools"] = []any{}
}
