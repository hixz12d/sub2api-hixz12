//go:build unit

package service

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

type platformAccounts struct{ account *Account }

func (f platformAccounts) GetByID(context.Context, int64) (*Account, error) { return f.account, nil }
func (f platformAccounts) ListSchedulableByGroupID(context.Context, int64) ([]Account, error) {
	return []Account{*f.account}, nil
}

type platformFlags struct{}

func (platformFlags) GetMonitorFeatureFlags(context.Context) MonitorFeatureFlags {
	return MonitorFeatureFlags{ChannelMonitorEnabled: true, ChannelMonitorV2: true, DetectorEnabled: true, ScheduledEnabled: true, EngineAllowed: true}
}
func TestPlatformCapabilityAdmission(t *testing.T) {
	group := &Group{ID: 1, Platform: PlatformOpenAI, Status: StatusActive}
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, GroupIDs: []int64{1}, Credentials: map[string]any{"api_key": "fixture", "base_url": "https://fixture.invalid"}}
	cfg := DefaultGroupCapabilityConfig()
	cfg.Enabled = true
	r := &PlatformCapabilityResolver{Accounts: platformAccounts{account}, Groups: siteCapabilityGroups{group}, Flags: platformFlags{}, CampaignID: "campaign", AllowedGroupIDs: []int64{1}, Prices: map[string]CapabilityPriceCeiling{"model": {InputTokenLimit: 1000, InputMicrosPerToken: 1, OutputMicrosPerToken: 1}}, GlobalDailyLimit: 100}
	job := &CapabilityJob{Snapshot: MonitorJobSnapshot{CapabilityConfig: &cfg, Targets: []DetectorTargetSpec{{Source: MonitorSourcePlatformGroup, GroupID: &group.ID, AccountID: &account.ID, Target: DetectorTarget{RequestModel: "model"}}}}}
	binding, err := r.Resolve(context.Background(), job, 0)
	require.NoError(t, err)
	require.Equal(t, DetectorContractExact, binding.ContractStatus)
	require.NoError(t, r.Check(context.Background(), job, 0, binding))
	account.Credentials["api_key"] = "rotated"
	require.Error(t, r.Check(context.Background(), job, 0, binding))
	account.Credentials["api_key"] = "fixture"
	account.Type = AccountTypeOAuth
	_, err = r.Resolve(context.Background(), job, 0)
	require.Error(t, err)
	account.Type = AccountTypeAPIKey
	account.Schedulable = false
	_, err = r.Resolve(context.Background(), job, 0)
	require.Error(t, err)
	account.Schedulable = true
	account.Extra = map[string]any{"openai_responses_mode": "force_chat_completions"}
	_, err = r.Resolve(context.Background(), job, 0)
	require.Error(t, err)
	account.Extra = nil
	account.GroupIDs = nil
	_, err = r.Resolve(context.Background(), job, 0)
	require.Error(t, err)
}
