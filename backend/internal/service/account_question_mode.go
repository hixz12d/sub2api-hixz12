package service

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"
)

const AccountTestModeQuestion = "question"

type accountQuestionModeKey struct{}

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

func applyAccountQuestionPayload(payload map[string]any, prompt string, chat, oauth bool) {
	if chat {
		payload["messages"] = []map[string]any{{"role": "user", "content": prompt}}
		payload["max_tokens"] = 1024
	} else {
		payload["input"] = []map[string]any{{"role": "user", "content": []map[string]any{{"type": "input_text", "text": prompt}}}}
		if !oauth {
			payload["max_output_tokens"] = 1024
		}
	}
	payload["tools"] = []any{}
}
