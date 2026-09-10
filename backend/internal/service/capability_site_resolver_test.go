//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type siteCapabilityKeys struct{ key *APIKey }

func (f siteCapabilityKeys) GetByID(context.Context, int64) (*APIKey, error) { return f.key, nil }

type siteCapabilityUsers struct{ user *User }

func (f siteCapabilityUsers) GetByID(context.Context, int64) (*User, error) { return f.user, nil }

type siteCapabilityGroups struct{ group *Group }

func (f siteCapabilityGroups) GetByID(context.Context, int64) (*Group, error) { return f.group, nil }

type siteCapabilityFlags struct{ allowed bool }

func (f *siteCapabilityFlags) GetMonitorFeatureFlags(context.Context) MonitorFeatureFlags {
	return MonitorFeatureFlags{DetectorEnabled: f.allowed, UserTestingEnabled: f.allowed, EngineAllowed: f.allowed}
}

func TestSiteCapabilityResolver(t *testing.T) {
	for _, scenario := range []string{"allowed", "foreign_owner", "disabled", "expired", "quota", "unlimited_key", "group_revoked", "unlisted_key", "arbitrary_url", "http_url", "flags_disabled", "rotated", "group_changed", "price_changed", "ip_restricted"} {
		t.Run(scenario, func(t *testing.T) {
			owner, keyID, groupID := int64(1), int64(2), int64(3)
			key := &APIKey{ID: keyID, UserID: owner, Key: "fixture-key", Status: StatusActive, GroupID: &groupID, Quota: 20}
			user := &User{ID: owner, Status: StatusActive}
			group := &Group{ID: groupID, Status: StatusActive, Platform: PlatformOpenAI}
			flags := &siteCapabilityFlags{allowed: true}
			r := &SiteCapabilityResolver{Keys: siteCapabilityKeys{key}, Users: siteCapabilityUsers{user}, Groups: siteCapabilityGroups{group}, Flags: flags, BaseURL: "https://sub2api.example.test", CampaignID: "campaign", AllowedKeyIDs: []int64{keyID}, GlobalDailyLimit: 1000, UserDailyLimit: 1000, Prices: map[string]CapabilityPriceCeiling{"test": {InputTokenLimit: 1000, InputMicrosPerToken: 1, OutputMicrosPerToken: 1}}}
			job := &CapabilityJob{OwnerID: &owner, Snapshot: MonitorJobSnapshot{Targets: []DetectorTargetSpec{{Source: MonitorSourceSiteAPIKey, SiteAPIKeyID: &keyID, Target: DetectorTarget{RequestModel: "test"}}}, Benchmarks: []DetectorBenchmarkManifest{{}}}}
			original, err := r.Resolve(context.Background(), job, 0)
			require.NoError(t, err)
			switch scenario {
			case "foreign_owner":
				key.UserID = 99
			case "disabled":
				key.Status = StatusAPIKeyDisabled
			case "expired":
				past := time.Now().Add(-time.Hour)
				key.ExpiresAt = &past
			case "quota":
				key.QuotaUsed = 20
			case "unlimited_key":
				key.Quota = 0
			case "group_revoked":
				group.IsExclusive = true
			case "unlisted_key":
				r.AllowedKeyIDs = nil
			case "arbitrary_url":
				job.Snapshot.Targets[0].BaseURL = "https://attacker.example.test"
			case "http_url":
				r.BaseURL = "http://sub2api.example.test"
			case "flags_disabled":
				flags.allowed = false
			case "rotated":
				key.Key = "rotated"
			case "group_changed":
				group.UpdatedAt = time.Now()
			case "price_changed":
				r.Prices["test"] = CapabilityPriceCeiling{InputTokenLimit: 2000, InputMicrosPerToken: 1, OutputMicrosPerToken: 1}
			case "ip_restricted":
				key.IPWhitelist = []string{"192.0.2.1"}
			}
			err = r.Check(context.Background(), job, 0, original)
			if scenario == "allowed" {
				require.NoError(t, err)
				require.Equal(t, "https://sub2api.example.test/v1/responses", original.Endpoint)
			} else {
				require.ErrorIs(t, err, ErrMonitorOutboundDenied)
			}
		})
	}
}
