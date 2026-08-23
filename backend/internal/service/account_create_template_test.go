//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccountCreateTemplateCRUD(t *testing.T) {
	svc := NewSettingService(newMockSettingRepo(), nil)
	ctx := context.Background()

	created, err := svc.CreateAccountCreateTemplate(ctx, AccountCreateTemplateInput{
		Name:          " Team 轮转 ",
		Platform:      PlatformOpenAI,
		Type:          AccountTypeOAuth,
		IsDefault:     true,
		IncludeGroups: true,
		Values: AccountCreateTemplateValues{
			Concurrency:          3,
			Priority:             2,
			RateMultiplier:       1,
			GroupIDs:             []int64{38, 38, 41, 0},
			OpenAIWSMode:         "ctx_pool",
			CodexFingerprintMode: "session",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "Team 轮转", created.Name)
	require.True(t, created.IsDefault)
	require.Equal(t, []int64{38, 41}, created.Values.GroupIDs)
	require.Equal(t, "ctx_pool", created.Values.OpenAIWSMode)
	require.Equal(t, "session", created.Values.CodexFingerprintMode)

	second, err := svc.CreateAccountCreateTemplate(ctx, AccountCreateTemplateInput{
		Name:      "Pro 固定",
		Platform:  PlatformOpenAI,
		Type:      AccountTypeOAuth,
		IsDefault: true,
		Values: AccountCreateTemplateValues{
			Concurrency: 1,
		},
	})
	require.NoError(t, err)
	require.True(t, second.IsDefault)

	listed, err := svc.ListAccountCreateTemplates(ctx, PlatformOpenAI, AccountTypeOAuth)
	require.NoError(t, err)
	require.Len(t, listed, 2)
	require.False(t, listed[0].IsDefault)
	require.True(t, listed[1].IsDefault)

	_, err = svc.CreateAccountCreateTemplate(ctx, AccountCreateTemplateInput{
		Name:     "team 轮转",
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
	})
	require.ErrorIs(t, err, ErrAccountCreateTemplateNameDuplicate)

	updated, err := svc.UpdateAccountCreateTemplate(ctx, created.ID, AccountCreateTemplateInput{
		Name:          "Team 轮转",
		Platform:      PlatformOpenAI,
		Type:          AccountTypeOAuth,
		IsDefault:     false,
		IncludeGroups: false,
		Values: AccountCreateTemplateValues{
			Concurrency:          4,
			GroupIDs:             []int64{50},
			OpenAIWSMode:         "http_bridge",
			CodexFingerprintMode: "window",
		},
	})
	require.NoError(t, err)
	require.False(t, updated.IncludeGroups)
	require.Empty(t, updated.Values.GroupIDs)
	require.Equal(t, 4, updated.Values.Concurrency)

	require.NoError(t, svc.DeleteAccountCreateTemplate(ctx, created.ID))
	listed, err = svc.ListAccountCreateTemplates(ctx, "", "")
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, second.ID, listed[0].ID)
}

func TestAccountCreateTemplateListEmptyAndInvalidJSON(t *testing.T) {
	repo := newMockSettingRepo()
	svc := NewSettingService(repo, nil)
	ctx := context.Background()

	items, err := svc.ListAccountCreateTemplates(ctx, "", "")
	require.NoError(t, err)
	require.Empty(t, items)

	repo.data[SettingKeyAccountCreateTemplates] = "not-json"
	items, err = svc.ListAccountCreateTemplates(ctx, "", "")
	require.NoError(t, err)
	require.Empty(t, items)
}

func TestAccountCreateTemplateNormalizesValues(t *testing.T) {
	svc := NewSettingService(newMockSettingRepo(), nil)
	proxyID := int64(-3)
	loadFactor := 0
	quota := 0.0
	created, err := svc.CreateAccountCreateTemplate(context.Background(), AccountCreateTemplateInput{
		Name:     "defaults",
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Values: AccountCreateTemplateValues{
			ProxyID:              &proxyID,
			Concurrency:          0,
			LoadFactor:           &loadFactor,
			Priority:             0,
			RateMultiplier:       -1,
			OpenAIWSMode:         "nope",
			OpenAICompactMode:    "maybe",
			CodexFingerprintMode: "zzz",
			QuotaLimit:           &quota,
		},
	})
	require.NoError(t, err)
	require.Nil(t, created.Values.ProxyID)
	require.Equal(t, 1, created.Values.Concurrency)
	require.Nil(t, created.Values.LoadFactor)
	require.Equal(t, 1, created.Values.Priority)
	require.Equal(t, 1.0, created.Values.RateMultiplier)
	require.Equal(t, "off", created.Values.OpenAIWSMode)
	require.Equal(t, "auto", created.Values.OpenAICompactMode)
	require.Equal(t, "off", created.Values.CodexFingerprintMode)
	require.Nil(t, created.Values.QuotaLimit)
}

func TestAccountCreateTemplateRejectsInvalidScopeAndMissing(t *testing.T) {
	svc := NewSettingService(newMockSettingRepo(), nil)
	ctx := context.Background()

	_, err := svc.CreateAccountCreateTemplate(ctx, AccountCreateTemplateInput{
		Name:     "bad",
		Platform: "nope",
		Type:     AccountTypeOAuth,
	})
	require.ErrorIs(t, err, ErrAccountCreateTemplateScopeInvalid)

	_, err = svc.CreateAccountCreateTemplate(ctx, AccountCreateTemplateInput{
		Name:     "",
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
	})
	require.ErrorIs(t, err, ErrAccountCreateTemplateNameInvalid)

	_, err = svc.UpdateAccountCreateTemplate(ctx, "missing", AccountCreateTemplateInput{
		Name:     "x",
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
	})
	require.ErrorIs(t, err, ErrAccountCreateTemplateNotFound)
	require.ErrorIs(t, svc.DeleteAccountCreateTemplate(ctx, "missing"), ErrAccountCreateTemplateNotFound)
}

func TestAccountCreateTemplatePersistsJSONDocument(t *testing.T) {
	repo := newMockSettingRepo()
	svc := NewSettingService(repo, nil)
	created, err := svc.CreateAccountCreateTemplate(context.Background(), AccountCreateTemplateInput{
		Name:     "persist",
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Values: AccountCreateTemplateValues{
			Concurrency: 8,
		},
	})
	require.NoError(t, err)

	raw := repo.data[SettingKeyAccountCreateTemplates]
	var store accountCreateTemplateStore
	require.NoError(t, json.Unmarshal([]byte(raw), &store))
	require.Len(t, store.Items, 1)
	require.Equal(t, created.ID, store.Items[0].ID)
	require.Equal(t, PlatformAnthropic, store.Items[0].Platform)
}
