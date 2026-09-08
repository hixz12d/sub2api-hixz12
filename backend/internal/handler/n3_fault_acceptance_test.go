//go:build unit

package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestN3MessagesUnknownHostedEventsForbidReplay(t *testing.T) {
	for _, tc := range []struct{ name, event string }{
		{"unknown", `{"type":"response.future_effect"}`},
		{"hosted", `{"type":"response.web_search_call.in_progress","item_id":"hosted"}`},
		{"unknown_item", `{"type":"response.output_item.added","item":{"id":"item","type":"future_tool"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, u, h := blueprintPhaseHandlerServer(t, "/v1/messages", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, blueprintPhasePreamble+"data: "+tc.event+"\n\ndata: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"server_error\",\"message\":\"upstream overloaded\"}}}\n\n")
			}, 10)
			h.maxAccountSwitches = 2
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			resp := blueprintPhaseRequest(t, ctx, server, "/v1/messages")
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.EqualValues(t, 1, u.calls.Load(), string(body))
			require.NotContains(t, string(body), "event: message_start")
		})
	}
}

type n3OwnershipRepo struct {
	service.OpenAIAffinityRepository
	mu       sync.Mutex
	accounts []int64
	fail     bool
}

func (r *n3OwnershipRepo) ResolveSession(context.Context, string, string, string, string, []string, time.Time, time.Duration, time.Duration) (*service.OpenAISessionBinding, error) {
	return nil, service.ErrOpenAIAffinityNotFound
}
func (r *n3OwnershipRepo) ResolveResponse(context.Context, string, string, string, time.Time, time.Duration, time.Duration) (*service.OpenAIResponseBinding, error) {
	return nil, service.ErrOpenAIAffinityNotFound
}
func (r *n3OwnershipRepo) CreateOrGetSession(_ context.Context, id service.SessionIdentity, accountID int64, expiry time.Time) (*service.OpenAISessionBinding, bool, error) {
	return &service.OpenAISessionBinding{ID: 1, AccountID: accountID, Strength: service.AffinityWeak, ExpiresAt: expiry}, true, nil
}
func (r *n3OwnershipRepo) BindResponseAndUpgrade(_ context.Context, _ service.SessionIdentity, _ string, accountID int64, _, _ time.Time) (*service.OpenAIResponseBinding, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.accounts = append(r.accounts, accountID)
	if r.fail {
		return nil, errors.New("injected ownership transaction failure")
	}
	return &service.OpenAIResponseBinding{AccountID: accountID}, nil
}
func TestN3MessagesOwnershipCommitAndFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		name := "candidate_not_persisted"
		if fail {
			name = "failure_before_public_write"
		}
		t.Run(name, func(t *testing.T) {
			repo := &n3OwnershipRepo{fail: fail}
			var first atomic.Bool
			server, u, h := blueprintPhaseHandlerServer(t, "/v1/messages", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				if !fail && first.CompareAndSwap(false, true) {
					_, _ = io.WriteString(w, blueprintPhasePreamble+"data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"server_error\",\"message\":\"upstream overloaded\"}}}\n\n")
					return
				}
				_, _ = io.WriteString(w, blueprintPhasePreamble+blueprintPhaseText+blueprintPhaseCompleted)
			}, 10)
			h.maxAccountSwitches = 2
			h.cfg.Gateway.OpenAIAffinity = config.GatewayOpenAIAffinityConfig{Enabled: true, WritesEnabled: true, Secret: "0123456789abcdef0123456789abcdef"}
			h.gatewayService.SetOpenAIAffinityRepository(repo)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			resp := blueprintPhaseRequest(t, ctx, server, "/v1/messages")
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			repo.mu.Lock()
			accounts := append([]int64(nil), repo.accounts...)
			repo.mu.Unlock()
			if fail {
				require.EqualValues(t, 1, u.calls.Load())
				require.Equal(t, []int64{1}, accounts)
				require.NotContains(t, string(body), "event: message_start")
				require.NotContains(t, string(body), "phase_visible_text")
			} else {
				require.EqualValues(t, 2, u.calls.Load(), string(body))
				require.Equal(t, []int64{2}, accounts)
				require.Contains(t, string(body), "phase_visible_text")
			}
		})
	}
}
