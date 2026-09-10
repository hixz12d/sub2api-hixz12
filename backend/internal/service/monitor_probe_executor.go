package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	mathrand "math/rand/v2"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
)

var ErrMonitorProbeUnsupported = errors.New("monitor probe target is unsupported or unavailable")

type MonitorProbeAccounts interface {
	GetByID(context.Context, int64) (*Account, error)
	ListSchedulableByGroupID(context.Context, int64) ([]Account, error)
}
type MonitorProbeGroups interface {
	GetByID(context.Context, int64) (*Group, error)
}
type MonitorProbeConcurrency interface {
	AcquireAccountSlot(context.Context, int64, int) (*AcquireResult, error)
}

// This executor is for administrator-owned platform probes only. External user
// targets must use a separately audited public-network transport and Key policy.
type MonitorProbeExecutor struct {
	store       MonitorProbeStore
	accounts    MonitorProbeAccounts
	groups      MonitorProbeGroups
	upstream    HTTPUpstream
	flags       MonitorRuntimeFlags
	slots       MonitorProbeConcurrency
	globalLimit int64
}

func NewMonitorProbeExecutor(store MonitorProbeStore, accounts MonitorProbeAccounts, groups MonitorProbeGroups, upstream HTTPUpstream, flags MonitorRuntimeFlags, slots MonitorProbeConcurrency, globalLimit int64) *MonitorProbeExecutor {
	return &MonitorProbeExecutor{store: store, accounts: accounts, groups: groups, upstream: upstream, flags: flags, slots: slots, globalLimit: globalLimit}
}

func (e *MonitorProbeExecutor) Ready() bool {
	return e != nil && e.store != nil && e.accounts != nil && e.groups != nil && e.upstream != nil && e.flags != nil && e.slots != nil && e.globalLimit > 0 && e.globalLimit <= 1000000000
}

func (e *MonitorProbeExecutor) Execute(ctx context.Context, lease MonitorJobLease) error {
	if !e.Ready() || lease.Kind != MonitorJobAvailability || lease.Source != MonitorSourcePlatformGroup || !e.flags.GetMonitorFeatureFlags(ctx).GroupProbeAllowed() {
		return ErrMonitorOutboundDenied
	}
	job, err := e.store.GetProbeJob(ctx, lease)
	if err != nil {
		return err
	}
	if job == nil || job.Snapshot.ProbeConfig == nil {
		return ErrMonitorProbeUnsupported
	}
	ctx, cancel := context.WithDeadline(ctx, job.Deadline)
	defer cancel()
	ctx, err = WithRequestOrigin(ctx, RequestOriginAvailabilityProbe)
	if err != nil {
		return err
	}
	group, err := e.groups.GetByID(ctx, job.GroupID)
	if err != nil {
		return ErrMonitorProbeUnsupported
	}
	if group == nil || !group.IsActive() || group.Platform != PlatformOpenAI || group.RequireOAuthOnly || group.ProfitControlEnabled {
		return ErrMonitorProbeUnsupported
	}
	candidates, err := e.accounts.ListSchedulableByGroupID(ctx, job.GroupID)
	if err != nil {
		return ErrMonitorProbeUnsupported
	}
	eligible := make([]Account, 0, len(candidates))
	for _, account := range candidates {
		if monitorProbeAccountAllowed(group, &account, job.Snapshot.Targets) {
			eligible = append(eligible, account)
		}
	}
	cfg := job.Snapshot.ProbeConfig
	selected := make([]Account, 0, cfg.SampleSize)
	if cfg.SelectionMode == MonitorSelectionFixed {
		for _, id := range cfg.FixedAccountIDs {
			index := slices.IndexFunc(eligible, func(a Account) bool { return a.ID == id })
			if index < 0 {
				return ErrMonitorProbeUnsupported
			}
			selected = append(selected, eligible[index])
		}
	} else {
		if len(eligible) < cfg.SampleSize {
			return ErrMonitorProbeUnsupported
		}
		mathrand.Shuffle(len(eligible), func(i, j int) { eligible[i], eligible[j] = eligible[j], eligible[i] })
		selected = append(selected, eligible[:cfg.SampleSize]...)
	}
	for targetIndex, target := range job.Snapshot.Targets {
		for sampleIndex := range selected {
			if err := ctx.Err(); err != nil {
				return err
			}
			if !e.flags.GetMonitorFeatureFlags(ctx).GroupProbeAllowed() {
				return ErrMonitorOutboundDenied
			}
			if err := e.executeSample(ctx, lease, job, &selected[sampleIndex], targetIndex, sampleIndex, target.Target.RequestModel); err != nil {
				return err
			}
		}
	}
	return nil
}

func monitorProbeAccountAllowed(group *Group, account *Account, targets []DetectorTargetSpec) bool {
	if account == nil || group == nil || !group.IsActive() || group.Platform != PlatformOpenAI || group.RequireOAuthOnly || group.ProfitControlEnabled || account.Platform != PlatformOpenAI || account.Type != AccountTypeAPIKey || account.ParentAccountID != nil ||
		account.IsTLSFingerprintEnabled() || account.IsHeaderOverrideEnabled() || account.GetCredential("api_key") == "" || !slices.Contains(account.GroupIDs, group.ID) {
		return false
	}
	if group.RequirePrivacySet && !account.IsPrivacySet() {
		return false
	}
	if account.ProxyID != nil && (account.Proxy == nil || !account.Proxy.IsActive() || account.Proxy.IsExpired(time.Now())) {
		return false
	}
	for _, target := range targets {
		if target.GroupID == nil || *target.GroupID != group.ID || !account.isSchedulableForModel(target.Target.RequestModel) || !account.IsModelSupported(target.Target.RequestModel) {
			return false
		}
	}
	return len(targets) > 0
}

func monitorProbeAccountFingerprint(account *Account) string {
	proxy := ""
	if account.Proxy != nil {
		proxy = account.Proxy.URL()
	}
	raw, _ := json.Marshal(struct {
		Credentials map[string]any
		Extra       map[string]any
		Proxy       string
		Platform    string
		Type        string
	}{account.Credentials, account.Extra, proxy, account.Platform, account.Type})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func monitorProbeRequest(account *Account, model, challenge string) (string, []byte, bool, error) {
	base := account.GetOpenAIBaseURL()
	if base == "" {
		base = "https://api.openai.com"
	}
	parsed, err := url.Parse(base)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", nil, false, ErrMonitorProbeUnsupported
	}
	// Same account model mapping and endpoint helpers as the production gateway.
	mapped := account.GetMappedModel(model)
	prompt := "Reply with exactly this token and nothing else: " + challenge
	chat := shouldForwardOpenAIResponsesViaRawChatCompletions(account)
	endpoint := buildOpenAIResponsesURL(base)
	payload := map[string]any{"model": mapped, "input": prompt, "stream": false, "store": false, "max_output_tokens": 64}
	if chat {
		endpoint = buildOpenAIChatCompletionsURL(base)
		payload = map[string]any{"model": mapped, "messages": []map[string]string{{"role": "user", "content": prompt}}, "stream": false, "max_completion_tokens": 64}
	}
	raw, err := json.Marshal(payload)
	return endpoint, raw, chat, err
}

func (e *MonitorProbeExecutor) executeSample(ctx context.Context, lease MonitorJobLease, job *MonitorProbeJob, account *Account, targetIndex, sampleIndex int, model string) error {
	requestCtx, cancel := context.WithTimeout(ctx, time.Duration(job.Snapshot.ProbeConfig.TimeoutSeconds)*time.Second)
	defer cancel()
	slot, err := e.slots.AcquireAccountSlot(requestCtx, account.ID, account.Concurrency)
	if err != nil || slot == nil || !slot.Acquired || slot.ReleaseFunc == nil {
		return ErrMonitorProbeUnsupported
	}
	defer slot.ReleaseFunc()
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	challenge := hex.EncodeToString(nonce)
	endpoint, body, chat, err := monitorProbeRequest(account, model, challenge)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(body)
	guard := &monitorProbeGuard{executor: e, job: job, account: account, lease: lease, targetIndex: targetIndex, sampleIndex: sampleIndex, model: model,
		endpoint: endpoint, bodyHash: hex.EncodeToString(sum[:]), fingerprint: monitorProbeAccountFingerprint(account), cancel: cancel}
	requestCtx = WithMonitorOutboundGuard(requestCtx, guard)
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return ErrMonitorProbeUnsupported
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+account.GetCredential("api_key"))
	proxy := ""
	if account.Proxy != nil {
		proxy = account.Proxy.URL()
	}
	response, sendErr := e.upstream.Do(req, proxy, account.ID, account.Concurrency)
	id := guard.dispatchID()
	if id == "" {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		return ErrMonitorOutboundDenied
	}
	outcome := MonitorProbeOutcome{DispatchID: id, TransportState: "failed", ChallengeState: "not_evaluated", ErrorCategory: "transport_error"}
	if response != nil && response.Body != nil {
		raw, readErr := io.ReadAll(io.LimitReader(response.Body, 65537))
		_ = response.Body.Close()
		switch {
		case sendErr != nil || readErr != nil:
			outcome.TransportState = "incomplete"
		case len(raw) > 65536:
			outcome.TransportState = "incomplete"
			outcome.ErrorCategory = "response_too_large"
		case response.StatusCode != http.StatusOK:
			outcome.ErrorCategory = "http_error"
		case bytes.Contains(raw, []byte(account.GetCredential("api_key"))):
			outcome.TransportState = "incomplete"
			outcome.ErrorCategory = "invalid_response"
		default:
			outcome = classifyMonitorProbeResponse(raw, chat, challenge)
			outcome.DispatchID = id
		}
	}
	// Never retain raw model output or use response-header arrival as visible TTFT.
	finishCtx, finishCancel := context.WithTimeout(ctx, 5*time.Second)
	defer finishCancel()
	return e.store.CompleteProbeDispatch(finishCtx, lease, outcome)
}

type monitorProbeGuard struct {
	executor                               *MonitorProbeExecutor
	job                                    *MonitorProbeJob
	account                                *Account
	lease                                  MonitorJobLease
	targetIndex, sampleIndex               int
	model, endpoint, bodyHash, fingerprint string
	cancel                                 context.CancelFunc
	mu                                     sync.Mutex
	id                                     string
}

func (g *monitorProbeGuard) dispatchID() string { g.mu.Lock(); defer g.mu.Unlock(); return g.id }

func (g *monitorProbeGuard) Authorize(ctx context.Context, accountID int64, req *http.Request) (err error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	defer func() {
		if err != nil {
			g.cancel()
		}
	}()
	if g.id != "" || accountID != g.account.ID || req.URL.String() != g.endpoint || req.GetBody == nil || req.Header.Get("Authorization") != "Bearer "+g.account.GetCredential("api_key") ||
		RequestOriginFromContext(ctx) != RequestOriginAvailabilityProbe {
		return ErrMonitorOutboundDenied
	}
	copy, err := req.GetBody()
	if err != nil {
		return ErrMonitorOutboundDenied
	}
	raw, err := io.ReadAll(io.LimitReader(copy, 65537))
	_ = copy.Close()
	if err != nil || len(raw) > 65536 {
		return ErrMonitorOutboundDenied
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != g.bodyHash {
		return ErrMonitorOutboundDenied
	}
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if !g.executor.flags.GetMonitorFeatureFlags(dbCtx).GroupProbeAllowed() {
		return ErrMonitorOutboundDenied
	}
	group, err := g.executor.groups.GetByID(dbCtx, g.job.GroupID)
	if err != nil || group == nil || !group.IsActive() || group.RequireOAuthOnly || group.Platform != PlatformOpenAI {
		return ErrMonitorOutboundDenied
	}
	fresh, err := g.executor.accounts.GetByID(dbCtx, accountID)
	if err != nil || !monitorProbeAccountAllowed(group, fresh, g.job.Snapshot.Targets) || monitorProbeAccountFingerprint(fresh) != g.fingerprint {
		return ErrMonitorOutboundDenied
	}
	g.id, err = g.executor.store.BeginProbeDispatch(dbCtx, MonitorProbeDispatch{Lease: g.lease, TargetIndex: g.targetIndex, SampleIndex: g.sampleIndex, AccountID: accountID, RequestModel: g.model, RequestSHA256: g.bodyHash}, g.executor.globalLimit)
	if err != nil {
		return ErrMonitorOutboundDenied
	}
	return nil
}

func classifyMonitorProbeResponse(raw []byte, chat bool, challenge string) MonitorProbeOutcome {
	result := MonitorProbeOutcome{TransportState: "incomplete", ChallengeState: "not_evaluated", ErrorCategory: "invalid_response"}
	var payload struct {
		Object string `json:"object"`
		Status string `json:"status"`
		Output []struct {
			Type    string `json:"type"`
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Role      string            `json:"role"`
				Content   string            `json:"content"`
				Refusal   *string           `json:"refusal"`
				ToolCalls []json.RawMessage `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		return result
	}
	text := ""
	if chat {
		if payload.Object != "chat.completion" || len(payload.Choices) != 1 {
			return result
		}
		choice := payload.Choices[0]
		if choice.FinishReason != "stop" || choice.Message.Role != "assistant" || choice.Message.Refusal != nil || len(choice.Message.ToolCalls) > 0 {
			return result
		}
		text = choice.Message.Content
	} else {
		if payload.Object != "response" || payload.Status != "completed" {
			return result
		}
		for _, item := range payload.Output {
			if item.Type == "reasoning" {
				continue
			}
			if item.Type != "message" || item.Role != "assistant" {
				return result
			}
			for _, part := range item.Content {
				if part.Type != "output_text" {
					return result
				}
				text += part.Text
			}
		}
	}
	if text == "" {
		return result
	}
	result.TransportState = "passed"
	result.ChallengeState = "failed"
	result.ErrorCategory = "challenge_mismatch"
	if strings.TrimSpace(text) == challenge {
		result.ChallengeState = "passed"
		result.ErrorCategory = ""
	}
	return result
}
