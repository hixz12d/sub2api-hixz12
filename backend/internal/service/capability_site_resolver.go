package service

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

type CapabilityKeys interface {
	GetByID(context.Context, int64) (*APIKey, error)
}
type CapabilityUsers interface {
	GetByID(context.Context, int64) (*User, error)
}

// This resolver sends through the site's normal gateway, not directly to an
// upstream account. Gateway authentication, routing and billing remain in force.
type SiteCapabilityResolver struct {
	Keys             CapabilityKeys
	Users            CapabilityUsers
	Groups           MonitorProbeGroups
	Flags            MonitorRuntimeFlags
	BaseURL          string
	CampaignID       string
	AllowedKeyIDs    []int64
	Prices           map[string]CapabilityPriceCeiling
	GlobalDailyLimit int64
	UserDailyLimit   int64
}

func (r *SiteCapabilityResolver) Resolve(ctx context.Context, job *CapabilityJob, index int) (*CapabilityBinding, error) {
	if r == nil || r.Keys == nil || r.Users == nil || r.Groups == nil || r.Flags == nil || !r.Flags.GetMonitorFeatureFlags(ctx).UserTestingAllowed(true) || job == nil || job.OwnerID == nil || *job.OwnerID <= 0 || index < 0 || index >= len(job.Snapshot.Targets) || index >= len(job.Snapshot.Benchmarks) || r.CampaignID == "" || r.GlobalDailyLimit <= 0 || r.UserDailyLimit <= 0 {
		return nil, ErrMonitorOutboundDenied
	}
	target := job.Snapshot.Targets[index]
	if target.Source != MonitorSourceSiteAPIKey || target.SiteAPIKeyID == nil || !slices.Contains(r.AllowedKeyIDs, *target.SiteAPIKeyID) || target.BaseURL != "" {
		return nil, ErrMonitorOutboundDenied
	}
	base, err := url.Parse(r.BaseURL)
	if err != nil || base.Scheme != "https" || base.Hostname() == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" || base.Opaque != "" {
		return nil, ErrMonitorOutboundDenied
	}
	key, err := r.Keys.GetByID(ctx, *target.SiteAPIKeyID)
	if err != nil || key == nil || key.ID != *target.SiteAPIKeyID || key.UserID != *job.OwnerID || !key.IsActive() || key.IsExpired() || key.IsQuotaExhausted() || key.Key == "" || key.GroupID == nil || key.Quota <= 0 || math.IsNaN(key.Quota) || math.IsInf(key.Quota, 0) || math.IsNaN(key.QuotaUsed) || math.IsInf(key.QuotaUsed, 0) || key.QuotaUsed < 0 {
		return nil, ErrMonitorOutboundDenied
	}
	// Never substitute the worker's address for a caller's IP restriction.
	if len(key.IPWhitelist) > 0 || len(key.IPBlacklist) > 0 {
		return nil, ErrMonitorOutboundDenied
	}
	user, err := r.Users.GetByID(ctx, *job.OwnerID)
	if err != nil || user == nil || user.ID != *job.OwnerID || !user.IsActive() || user.DeletedAt != nil {
		return nil, ErrMonitorOutboundDenied
	}
	group, err := r.Groups.GetByID(ctx, *key.GroupID)
	if err != nil || group == nil || group.ID != *key.GroupID || !group.IsActive() || !user.CanBindGroup(group.ID, group.IsExclusive) {
		return nil, ErrMonitorOutboundDenied
	}
	if target.GroupID != nil && *target.GroupID != group.ID {
		return nil, ErrMonitorOutboundDenied
	}
	if key.RateLimit5h > 0 && key.EffectiveUsage5h() >= key.RateLimit5h || key.RateLimit1d > 0 && key.EffectiveUsage1d() >= key.RateLimit1d || key.RateLimit7d > 0 && key.EffectiveUsage7d() >= key.RateLimit7d {
		return nil, ErrMonitorOutboundDenied
	}
	price, ok := r.Prices[target.Target.RequestModel]
	if !ok {
		return nil, ErrMonitorOutboundDenied
	}
	// The release mode is independently validated by the engine before dispatch.
	var endpoint string
	switch group.Platform {
	case PlatformOpenAI:
		endpoint = buildOpenAIResponsesURL(strings.TrimRight(r.BaseURL, "/"))
	case PlatformAnthropic:
		endpoint = strings.TrimRight(r.BaseURL, "/")
		if !strings.HasSuffix(endpoint, "/v1") {
			endpoint += "/v1"
		}
		endpoint += "/messages"
	default:
		return nil, ErrMonitorOutboundDenied
	}
	headers := http.Header{"Authorization": []string{"Bearer " + key.Key}, "Content-Type": []string{"application/json"}, "Accept": []string{"text/event-stream"}}
	if group.Platform == PlatformAnthropic {
		headers.Set("anthropic-version", "2023-06-01")
	}
	revision, err := json.Marshal([]any{key.ID, key.GroupID, key.Key, user.ID, user.TokenVersion, group.Platform, group.UpdatedAt, price})
	if err != nil {
		return nil, ErrMonitorOutboundDenied
	}
	binding := &CapabilityBinding{CredentialRevision: benchmarkSHA256(revision), Endpoint: endpoint, Headers: headers, Secrets: []string{key.Key}, CampaignID: r.CampaignID, Concurrency: 1, Limits: []MonitorBudgetLimit{{Scope: "global", RequestLimit: r.GlobalDailyLimit}, {Scope: "user", ScopeID: *job.OwnerID, RequestLimit: r.UserDailyLimit}}, UpperBound: price.UpperBound}
	binding.Mode = "gpt"
	if group.Platform == PlatformAnthropic {
		binding.Mode = "claude"
	}
	return binding, nil
}
func (r *SiteCapabilityResolver) Check(ctx context.Context, job *CapabilityJob, index int, binding *CapabilityBinding) error {
	if binding == nil {
		return ErrMonitorOutboundDenied
	}
	fresh, err := r.Resolve(ctx, job, index)
	if err != nil {
		return err
	}
	if fresh.Endpoint != binding.Endpoint || fresh.CampaignID != binding.CampaignID || fresh.CredentialRevision != binding.CredentialRevision || subtle.ConstantTimeCompare([]byte(fresh.Headers.Get("Authorization")), []byte(binding.Headers.Get("Authorization"))) != 1 {
		return ErrMonitorOutboundDenied
	}
	return nil
}
