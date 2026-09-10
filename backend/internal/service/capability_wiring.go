package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"path/filepath"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/google/uuid"
)

type SiteCapabilityRuntimeConfig struct {
	AllowedGroupIDs  []int64                           `json:"allowed_group_ids,omitempty"`
	Enabled          bool                              `json:"enabled"`
	BaseURL          string                            `json:"base_url"`
	CampaignID       string                            `json:"campaign_id"`
	AllowedKeyIDs    []int64                           `json:"allowed_key_ids"`
	Prices           map[string]CapabilityPriceCeiling `json:"prices"`
	GlobalDailyLimit int64                             `json:"global_daily_limit"`
	UserDailyLimit   int64                             `json:"user_daily_limit"`
}

// No configured file means no executor. Invalid explicit admission fails startup.
// The file contains IDs and prices only, never Key material.
func ProvideCapabilityExecutor(cfg *config.Config, store CapabilityStore, releases BenchmarkRegistryStore, keys APIKeyRepository, users UserRepository, groups GroupRepository, accounts AccountRepository, flags *SettingService, upstream HTTPUpstream, slots *ConcurrencyService) (*CapabilityExecutor, error) {
	if cfg == nil || cfg.LLMDetectorRuntimeConfig == "" {
		return nil, nil
	}
	if !filepath.IsAbs(cfg.LLMDetectorRuntimeConfig) {
		return nil, ErrMonitorOutboundDenied
	}
	raw, err := readBenchmarkFile(cfg.LLMDetectorRuntimeConfig, 65536)
	if err != nil {
		return nil, err
	}
	var options SiteCapabilityRuntimeConfig
	if decodeCapabilityRuntimeConfig(raw, &options) != nil {
		return nil, ErrMonitorOutboundDenied
	}
	if !options.Enabled {
		return nil, nil
	}
	if !cfg.LLMDetectorEngineAllowed || uuid.Validate(options.CampaignID) != nil || (len(options.AllowedKeyIDs) == 0 && len(options.AllowedGroupIDs) == 0) || len(options.AllowedGroupIDs) > 100 || len(options.AllowedKeyIDs) > 100 || len(options.Prices) == 0 || len(options.Prices) > 100 || options.GlobalDailyLimit < 1 || options.GlobalDailyLimit > 1000000000 || options.UserDailyLimit < 1 || options.UserDailyLimit > options.GlobalDailyLimit {
		return nil, ErrMonitorOutboundDenied
	}
	seen := map[int64]bool{}
	for _, id := range options.AllowedKeyIDs {
		if id <= 0 || seen[id] {
			return nil, ErrMonitorOutboundDenied
		}
		seen[id] = true
	}
	groupSeen := map[int64]bool{}
	for _, id := range options.AllowedGroupIDs {
		if id <= 0 || groupSeen[id] {
			return nil, ErrMonitorOutboundDenied
		}
		groupSeen[id] = true
	}
	for model, price := range options.Prices {
		if model == "" || len(model) > 200 {
			return nil, ErrMonitorOutboundDenied
		}
		if _, err := price.UpperBound([]byte(`{"max_output_tokens":1}`)); err != nil {
			return nil, err
		}
	}
	engine := NewPinnedCapabilityEngine(cfg)
	if _, err = engine.validator.EngineLock(context.Background()); err != nil {
		return nil, err
	}
	resolver := &SiteCapabilityResolver{Keys: keys, Users: users, Groups: groups, Flags: flags, BaseURL: options.BaseURL, CampaignID: options.CampaignID, AllowedKeyIDs: options.AllowedKeyIDs, Prices: options.Prices, GlobalDailyLimit: options.GlobalDailyLimit, UserDailyLimit: options.UserDailyLimit}
	executor := &CapabilityExecutor{Store: store, Engine: engine, Releases: releases, Targets: resolver, Upstream: upstream, Slots: slots}
	if len(options.AllowedGroupIDs) > 0 {
		queue, ok := store.(PlatformCapabilityQueue)
		if !ok {
			return nil, ErrMonitorOutboundDenied
		}
		platform := &PlatformCapabilityResolver{Accounts: accounts, Groups: groups, Flags: flags, CampaignID: options.CampaignID, AllowedGroupIDs: options.AllowedGroupIDs, Prices: options.Prices, GlobalDailyLimit: options.GlobalDailyLimit}
		executor.Targets = &CombinedCapabilityResolver{Site: resolver, Platform: platform}
		executor.Scheduler = &PlatformCapabilityScheduler{Queue: queue, Accounts: accounts, Resolver: platform, Engine: engine, Registry: releases}
	}
	return executor, nil
}

func decodeCapabilityRuntimeConfig(raw []byte, options *SiteCapabilityRuntimeConfig) error {
	if len(raw) == 0 || len(raw) > 65536 || len(bytes.TrimSpace(raw)) == 0 || bytes.TrimSpace(raw)[0] != '{' {
		return ErrMonitorOutboundDenied
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(options) != nil || decoder.Decode(new(any)) != io.EOF {
		return ErrMonitorOutboundDenied
	}
	return nil
}

func (e *CapabilityExecutor) AllowsPlatformJobs() bool { return e != nil && e.Scheduler != nil }
func (e *CapabilityExecutor) SiteJobsOnly() bool       { return !e.AllowsPlatformJobs() }
func (e *CapabilityExecutor) Schedule(ctx context.Context) error {
	if !e.AllowsPlatformJobs() {
		return nil
	}
	return e.Scheduler.Schedule(ctx)
}
