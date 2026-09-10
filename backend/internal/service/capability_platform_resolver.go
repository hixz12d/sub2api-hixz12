package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
)

// Platform targets use frozen account IDs, never a new random selection on retry.
type PlatformCapabilityResolver struct {
	Accounts         MonitorProbeAccounts
	Groups           MonitorProbeGroups
	Flags            MonitorRuntimeFlags
	CampaignID       string
	AllowedGroupIDs  []int64
	Prices           map[string]CapabilityPriceCeiling
	GlobalDailyLimit int64
}

func (r *PlatformCapabilityResolver) Resolve(ctx context.Context, job *CapabilityJob, index int) (*CapabilityBinding, error) {
	if r == nil || r.Accounts == nil || r.Groups == nil || r.Flags == nil || !r.Flags.GetMonitorFeatureFlags(ctx).ScheduledDetectionAllowed(true) || job == nil || job.OwnerID != nil || job.Snapshot.CapabilityConfig == nil || index < 0 || index >= len(job.Snapshot.Targets) || r.CampaignID == "" || r.GlobalDailyLimit < 1 {
		return nil, ErrMonitorOutboundDenied
	}
	target := job.Snapshot.Targets[index]
	if target.Source != MonitorSourcePlatformGroup || target.GroupID == nil || target.AccountID == nil || !slices.Contains(r.AllowedGroupIDs, *target.GroupID) || target.SiteAPIKeyID != nil || target.BaseURL != "" {
		return nil, ErrMonitorOutboundDenied
	}
	group, err := r.Groups.GetByID(ctx, *target.GroupID)
	if err != nil {
		return nil, ErrMonitorOutboundDenied
	}
	account, err := r.Accounts.GetByID(ctx, *target.AccountID)
	if err != nil || account == nil || account.ID != *target.AccountID || !monitorProbeAccountAllowed(group, account, []DetectorTargetSpec{target}) || shouldForwardOpenAIResponsesViaRawChatCompletions(account) || account.GetMappedModel(target.Target.RequestModel) != target.Target.RequestModel {
		return nil, ErrMonitorOutboundDenied
	}
	base := account.GetOpenAIBaseURL()
	if base == "" {
		base = "https://api.openai.com"
	}
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, ErrMonitorOutboundDenied
	}
	price, ok := r.Prices[target.Target.RequestModel]
	if !ok {
		return nil, ErrMonitorOutboundDenied
	}
	proxy := ""
	if account.Proxy != nil {
		proxy = account.Proxy.URL()
	}
	revision, err := json.Marshal([]any{monitorProbeAccountFingerprint(account), group.UpdatedAt, price})
	if err != nil {
		return nil, ErrMonitorOutboundDenied
	}
	return &CapabilityBinding{Mode: "gpt", ContractStatus: DetectorContractExact, AccountID: target.AccountID, CredentialRevision: benchmarkSHA256(revision), Endpoint: buildOpenAIResponsesURL(base), Headers: http.Header{"Authorization": []string{"Bearer " + account.GetCredential("api_key")}, "Content-Type": []string{"application/json"}, "Accept": []string{"text/event-stream"}}, ProxyURL: proxy, Concurrency: account.Concurrency, Secrets: []string{account.GetCredential("api_key")}, CampaignID: r.CampaignID, Limits: []MonitorBudgetLimit{{Scope: "global", RequestLimit: r.GlobalDailyLimit}, {Scope: "group", ScopeID: group.ID, RequestLimit: int64(job.Snapshot.CapabilityConfig.DailyRequestLimit)}}, UpperBound: price.UpperBound}, nil
}
func (r *PlatformCapabilityResolver) Check(ctx context.Context, job *CapabilityJob, index int, b *CapabilityBinding) error {
	if b == nil {
		return ErrMonitorOutboundDenied
	}
	fresh, err := r.Resolve(ctx, job, index)
	if err != nil {
		return err
	}
	if fresh.CredentialRevision != b.CredentialRevision || fresh.Endpoint != b.Endpoint || fresh.CampaignID != b.CampaignID || fresh.AccountID == nil || b.AccountID == nil || *fresh.AccountID != *b.AccountID {
		return ErrMonitorOutboundDenied
	}
	return nil
}

type CombinedCapabilityResolver struct {
	Site     *SiteCapabilityResolver
	Platform *PlatformCapabilityResolver
}

func (r *CombinedCapabilityResolver) Resolve(ctx context.Context, job *CapabilityJob, index int) (*CapabilityBinding, error) {
	if job == nil || index < 0 || index >= len(job.Snapshot.Targets) {
		return nil, ErrMonitorOutboundDenied
	}
	if job.Snapshot.Targets[index].Source == MonitorSourcePlatformGroup {
		return r.Platform.Resolve(ctx, job, index)
	}
	return r.Site.Resolve(ctx, job, index)
}
func (r *CombinedCapabilityResolver) Check(ctx context.Context, job *CapabilityJob, index int, b *CapabilityBinding) error {
	if job == nil || index < 0 || index >= len(job.Snapshot.Targets) {
		return ErrMonitorOutboundDenied
	}
	if job.Snapshot.Targets[index].Source == MonitorSourcePlatformGroup {
		return r.Platform.Check(ctx, job, index, b)
	}
	return r.Site.Check(ctx, job, index, b)
}
