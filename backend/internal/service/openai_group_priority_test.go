package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestShuffleOpenAIWithinSortGroupsPreservesBindingPriorityTiers(t *testing.T) {
	groupID := int64(38)
	lastUsed := time.Unix(1_700_000_000, 0)
	accounts := []accountWithLoad{
		{account: &Account{ID: 1, Priority: 1, LastUsedAt: &lastUsed, AccountGroups: []AccountGroup{{GroupID: groupID, Priority: 1}}}, loadInfo: &AccountLoadInfo{LoadRate: 10}},
		{account: &Account{ID: 2, Priority: 1, LastUsedAt: &lastUsed, AccountGroups: []AccountGroup{{GroupID: groupID, Priority: 1}}}, loadInfo: &AccountLoadInfo{LoadRate: 10}},
		{account: &Account{ID: 3, Priority: 1, LastUsedAt: &lastUsed, AccountGroups: []AccountGroup{{GroupID: groupID, Priority: 50}}}, loadInfo: &AccountLoadInfo{LoadRate: 10}},
		{account: &Account{ID: 4, Priority: 1, LastUsedAt: &lastUsed, AccountGroups: []AccountGroup{{GroupID: groupID, Priority: 50}}}, loadInfo: &AccountLoadInfo{LoadRate: 10}},
	}

	for range 100 {
		shuffled := append([]accountWithLoad(nil), accounts...)
		shuffleOpenAIWithinSortGroups(shuffled, &groupID, OpenAIAccountPriorityModeBinding)
		for index, expectedPriority := range []int{1, 1, 50, 50} {
			priority, ok := openAIAccountBindingPriority(shuffled[index].account, &groupID)
			require.True(t, ok)
			require.Equal(t, expectedPriority, priority)
		}
	}
}
