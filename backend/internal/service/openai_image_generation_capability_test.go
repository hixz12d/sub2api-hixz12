//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestIsOpenAIImageGenerationCapabilityError(t *testing.T) {
	tests := []struct {
		name         string
		message      string
		responseBody string
		want         bool
	}{
		{
			name:    "direct upstream message",
			message: "Image generation is not enabled for this group.",
			want:    true,
		},
		{
			name:         "nested permission error",
			responseBody: `{"error":{"type":"permission_error","message":"IMAGE GENERATION IS NOT ENABLED FOR THIS GROUP"}}`,
			want:         true,
		},
		{
			name:         "responses failed detail",
			responseBody: `{"type":"response.failed","response":{"error":{"detail":"Image generation is not enabled for this group"}}}`,
			want:         true,
		},
		{
			name:         "unrelated request text does not match",
			responseBody: `{"input":"Image generation is not enabled for this group"}`,
			want:         false,
		},
		{
			name:         "ordinary permission error",
			responseBody: `{"error":{"type":"permission_error","message":"You do not have permission to use this model"}}`,
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, IsOpenAIImageGenerationCapabilityError(tt.message, []byte(tt.responseBody)))
		})
	}
}

func TestRateLimitService_ImageGenerationCapabilityErrorSkipsAccountState(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	counter := &countingOpenAI403CounterCache{openAI403CounterCacheStub: openAI403CounterCacheStub{counts: []int64{1}}}
	blocker := &runtimeBlockRecorder{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	svc.SetOpenAI403CounterCache(counter)
	svc.SetAccountRuntimeBlocker(blocker)
	account := &Account{
		ID:       601,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"temp_unschedulable_enabled": true,
			"temp_unschedulable_rules": []any{map[string]any{
				"error_code":       float64(http.StatusForbidden),
				"keywords":         []any{"image generation is not enabled"},
				"duration_minutes": float64(30),
			}},
		},
	}
	body := []byte(`{"error":{"type":"permission_error","message":"Image generation is not enabled for this group"}}`)

	shouldDisable := svc.HandleUpstreamError(context.Background(), account, http.StatusForbidden, http.Header{}, body, "gpt-image-2")

	require.False(t, shouldDisable)
	require.Zero(t, repo.setErrorCalls)
	require.Zero(t, repo.tempCalls)
	require.Zero(t, counter.increments)
	require.Empty(t, blocker.accounts)
}

func TestOpenAIGatewayService_ImageGenerationCapabilityErrorSkipsFastPathState(t *testing.T) {
	repo := &modelNotFoundAccountRepoStub{}
	svc := &OpenAIGatewayService{
		rateLimitService: NewRateLimitService(repo, nil, &config.Config{}, nil, nil),
	}
	account := &Account{
		ID:       602,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"temp_unschedulable_enabled": true,
			"temp_unschedulable_rules": []any{map[string]any{
				"error_code":       float64(http.StatusForbidden),
				"keywords":         []any{"image generation is not enabled"},
				"duration_minutes": float64(30),
			}},
		},
	}
	body := []byte(`{"error":{"type":"permission_error","message":"Image generation is not enabled for this group"}}`)

	shouldDisable := svc.handleOpenAIAccountUpstreamError(context.Background(), account, http.StatusForbidden, http.Header{}, body, "gpt-image-2")

	require.False(t, shouldDisable)
	require.Zero(t, repo.tempCalls)
	require.Empty(t, repo.modelRateLimitCalls)
}
