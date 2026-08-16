package admin

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestShouldScheduleOpenAIResponsesProbe(t *testing.T) {
	tests := []struct {
		name    string
		account *service.Account
		want    bool
	}{
		{
			name:    "nil account",
			account: nil,
			want:    false,
		},
		{
			name: "openai oauth",
			account: &service.Account{
				Platform: service.PlatformOpenAI,
				Type:     service.AccountTypeOAuth,
			},
			want: false,
		},
		{
			name: "openai api key auto",
			account: &service.Account{
				Platform: service.PlatformOpenAI,
				Type:     service.AccountTypeAPIKey,
			},
			want: true,
		},
		{
			name: "force responses",
			account: &service.Account{
				Platform: service.PlatformOpenAI,
				Type:     service.AccountTypeAPIKey,
				Extra: map[string]any{
					openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceResponses),
				},
			},
			want: false,
		},
		{
			name: "force chat completions",
			account: &service.Account{
				Platform: service.PlatformOpenAI,
				Type:     service.AccountTypeAPIKey,
				Extra: map[string]any{
					openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions),
				},
			},
			want: false,
		},
		{
			name: "invalid mode keeps auto detection",
			account: &service.Account{
				Platform: service.PlatformOpenAI,
				Type:     service.AccountTypeAPIKey,
				Extra: map[string]any{
					openai_compat.ExtraKeyResponsesMode: "unsupported",
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldScheduleOpenAIResponsesProbe(tt.account); got != tt.want {
				t.Fatalf("shouldScheduleOpenAIResponsesProbe() = %v, want %v", got, tt.want)
			}
		})
	}
}
