package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestModelListingFilterDoesNotEnableAdmission(t *testing.T) {
	group := &Group{ModelsListConfig: GroupModelsListConfig{Enabled: true, Models: []string{"gpt-5.5"}}}
	require.Equal(t, []string{"gpt-5.5"}, group.ModelListingFilter().FilterForListing([]string{"gpt-5.5", "gpt-5.4"}))
	require.False(t, group.ModelAllowlistEnabled())
	require.True(t, group.ModelAllowlist.Allows("gpt-5.4"))
	require.Empty(t, group.ModelAllowlist.Models)
}

func TestModelListingFilterIntersectsIndependentAdmission(t *testing.T) {
	group := &Group{
		ModelsListConfig: GroupModelsListConfig{Enabled: true, Models: []string{"gpt-5.5", "gpt-5.4"}},
		ModelAllowlist:   GroupModelAllowlist{Enabled: true, Models: []string{"gpt-5.4", "claude-*"}},
	}
	require.Equal(t, []string{"gpt-5.4"}, group.ModelListingFilter().Models)
	require.True(t, group.ModelAllowlist.Allows("claude-sonnet-4-6"))
	require.False(t, group.ModelAllowlist.Allows("gpt-5.5"))
	require.Equal(t, []string{"gpt-5.5", "gpt-5.4"}, group.ModelsListConfig.Models)
	group.ModelAllowlist.Models = []string{"claude-*"}
	filter := group.ModelListingFilter()
	require.True(t, filter.Enabled)
	require.Empty(t, filter.FilterForListing([]string{"gpt-5.5", "claude-sonnet-4-6"}))
}

func TestModelListingFilterFallsBackToAdmission(t *testing.T) {
	group := &Group{ModelAllowlist: GroupModelAllowlist{Enabled: true, Models: []string{"gpt-*"}}}
	require.Equal(t, group.ModelAllowlist, group.ModelListingFilter())
	var missing *Group
	require.False(t, missing.ModelListingFilter().Enabled)
}
