//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type monitorProbeIntegrationFlags struct{}

func (monitorProbeIntegrationFlags) GetMonitorFeatureFlags(context.Context) service.MonitorFeatureFlags {
	return service.MonitorFeatureFlags{ChannelMonitorEnabled: true, ChannelMonitorV2: true, GroupProbeEnabled: true}
}

func TestMonitorProbeExecutorHTTPPostgres(t *testing.T) {
	ctx := context.Background()
	db, client := testMonitorDatabase(t)
	user := mustCreateUser(t, client, &service.User{Email: "probe-" + uuid.NewString() + "@example.com"})
	accounts := NewAccountRepository(client, db, nil)
	groups := NewGroupRepository(client, db)
	jobs := NewMonitorJobRepository(db)
	slots := service.NewConcurrencyService(NewConcurrencyCache(testRedis(t), 5, 30))
	for _, test := range []struct {
		name                           string
		chat                           bool
		transport, challenge, category string
	}{
		{"responses", false, "passed", "passed", ""},
		{"chat_completions", true, "passed", "passed", ""},
		{"challenge_mismatch", false, "passed", "failed", "challenge_mismatch"},
		{"html_200", false, "incomplete", "not_evaluated", "invalid_response"},
		{"empty_200", false, "incomplete", "not_evaluated", "invalid_response"},
		{"incomplete_json", false, "incomplete", "not_evaluated", "invalid_response"},
		{"credential_echo", false, "incomplete", "not_evaluated", "invalid_response"},
		{"too_large", false, "incomplete", "not_evaluated", "response_too_large"},
		{"http_error", false, "failed", "not_evaluated", "http_error"},
		{"redirect", false, "failed", "not_evaluated", "http_error"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var hits, decoyHits atomic.Int64
			key := "fixture-not-a-real-key-" + uuid.NewString()
			var jobID string
			var selectedID int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/redirected" {
					decoyHits.Add(1)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				hits.Add(1)
				if r.Header.Get("Authorization") != "Bearer "+key {
					t.Error("wrong account credential used")
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				expectedPath := "/v1/responses"
				if test.chat {
					expectedPath = "/v1/chat/completions"
				}
				if r.URL.Path != expectedPath {
					t.Errorf("wrong route: %s", r.URL.Path)
				}
				var before int
				err := db.QueryRow(`SELECT count(*) FROM monitor_probe_dispatches WHERE job_id=$1 AND account_id=$2 AND state='dispatching'`, jobID, selectedID).Scan(&before)
				if err != nil || before != 1 {
					t.Errorf("dispatch must be durable before actual HTTP: count=%d error=%v", before, err)
				}
				var request struct {
					Model    string `json:"model"`
					Input    string `json:"input"`
					Messages []struct {
						Content string `json:"content"`
					} `json:"messages"`
				}
				if json.NewDecoder(r.Body).Decode(&request) != nil {
					t.Error("invalid request JSON")
					w.WriteHeader(400)
					return
				}
				if request.Model != "upstream-fixture" {
					t.Errorf("model mapping was not used: %s", request.Model)
				}
				prompt := request.Input
				if test.chat && len(request.Messages) == 1 {
					prompt = request.Messages[0].Content
				}
				challenge := strings.TrimPrefix(prompt, "Reply with exactly this token and nothing else: ")
				if len(challenge) != 32 {
					t.Error("missing random challenge")
				}
				switch test.name {
				case "html_200":
					_, _ = io.WriteString(w, "<html>ok</html>")
					return
				case "empty_200":
					return
				case "incomplete_json":
					_, _ = io.WriteString(w, `{"object":"response","status":"incomplete","output":[]}`)
					return
				case "credential_echo":
					_, _ = io.WriteString(w, key)
					return
				case "too_large":
					_, _ = io.WriteString(w, strings.Repeat("x", 65537))
					return
				case "http_error":
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				case "redirect":
					http.Redirect(w, r, "/redirected", http.StatusTemporaryRedirect)
					return
				case "challenge_mismatch":
					challenge = "wrong challenge"
				}
				if test.chat {
					_ = json.NewEncoder(w).Encode(map[string]any{"object": "chat.completion", "choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": challenge}}}})
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"object": "response", "status": "completed", "output": []any{map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": challenge}}}}})
			}))
			defer server.Close()
			decoy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				decoyHits.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer decoy.Close()
			var groupID int64
			require.NoError(t, db.QueryRow(`INSERT INTO groups(name,platform,status) VALUES($1,'openai','active') RETURNING id`, "probe-"+test.name).Scan(&groupID))
			mode := "force_responses"
			if test.chat {
				mode = "force_chat_completions"
			}
			selected := mustCreateAccount(t, client, &service.Account{Name: "selected-" + test.name, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
				Credentials: map[string]any{"base_url": server.URL, "api_key": key, "model_mapping": map[string]any{"fixture-model": "upstream-fixture"}}, Extra: map[string]any{"openai_responses_mode": mode}})
			selectedID = selected.ID
			other := mustCreateAccount(t, client, &service.Account{Name: "decoy-" + test.name, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Credentials: map[string]any{"base_url": decoy.URL, "api_key": "fixture-decoy"}})
			require.NoError(t, accounts.BindGroups(ctx, selected.ID, []int64{groupID}))
			require.NoError(t, accounts.BindGroups(ctx, other.ID, []int64{groupID}))
			cfg := service.DefaultGroupProbeConfig()
			cfg.Enabled = true
			cfg.SelectionMode = service.MonitorSelectionFixed
			cfg.FixedAccountIDs = []int64{selected.ID}
			raw, err := json.Marshal(cfg)
			require.NoError(t, err)
			var policy int64
			require.NoError(t, db.QueryRow(`INSERT INTO channel_monitor_group_policies(group_id,primary_model,evaluation_revision,created_by,updated_by,enabled,probe_config,next_probe_at)
 VALUES($1,'fixture-model',repeat('a',64),$2,$2,true,$3,clock_timestamp()-interval '1 second') RETURNING id`, groupID, user.ID, raw).Scan(&policy))
			jobID, err = jobs.EnqueueProbe(ctx, policy, 10000)
			require.NoError(t, err)
			lease, err := jobs.Claim(ctx, service.MonitorJobAvailability, "http-fixture")
			require.NoError(t, err)
			require.Equal(t, jobID, lease.JobID)
			executor := service.NewMonitorProbeExecutor(jobs, accounts, groups, NewHTTPUpstream(nil), monitorProbeIntegrationFlags{}, slots, 10000)
			require.True(t, executor.Ready())
			require.NoError(t, executor.Execute(ctx, *lease))
			require.NoError(t, jobs.Finish(ctx, *lease, service.MonitorJobCompleted))
			require.EqualValues(t, 1, hits.Load())
			require.Zero(t, decoyHits.Load())
			var transport, challenge, category string
			var accountID int64
			require.NoError(t, db.QueryRow(`SELECT transport_state,challenge_state,COALESCE(error_category,''),account_id FROM channel_monitor_probe_results WHERE job_id=$1`, jobID).Scan(&transport, &challenge, &category, &accountID))
			require.Equal(t, test.transport, transport)
			require.Equal(t, test.challenge, challenge)
			require.Equal(t, test.category, category)
			require.Equal(t, selectedID, accountID)
			var businessRows int
			require.NoError(t, db.QueryRow(`SELECT count(*) FROM usage_logs WHERE account_id=$1`, selectedID).Scan(&businessRows))
			require.Zero(t, businessRows)
		})
	}
}
