//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type accessTransitionRepoStub struct {
	groupPlatformRepoStub
	calls    int
	expected time.Time
	err      error
}

func (r *accessTransitionRepoStub) UpdatePreservingUserAccess(_ context.Context, g *Group, expected time.Time) error {
	r.calls++
	r.expected = expected
	if r.err != nil {
		return r.err
	}
	r.updated = g
	return nil
}

func TestUpdateGroupPreservesExistingUsers(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "transaction failure"}[fail], func(t *testing.T) {
			updatedAt := time.Now().Add(-time.Hour)
			repo := &accessTransitionRepoStub{groupPlatformRepoStub: groupPlatformRepoStub{group: &Group{
				ID: 7, Name: "Legacy", Platform: PlatformOpenAI, RateMultiplier: 0.125,
				SubscriptionType: SubscriptionTypeStandard, UpdatedAt: updatedAt,
			}}}
			if fail {
				repo.err = errors.New("transaction failed")
			}
			cache := &authCacheInvalidatorStub{}
			svc := &adminServiceImpl{groupRepo: repo, authCacheInvalidator: cache}
			exclusive := true
			got, err := svc.UpdateGroup(context.Background(), 7, &UpdateGroupInput{IsExclusive: &exclusive, PreserveExistingUsers: true})
			require.Equal(t, 1, repo.calls)
			require.Equal(t, updatedAt, repo.expected)
			if fail {
				require.ErrorIs(t, err, repo.err)
				require.Nil(t, got)
				require.Empty(t, cache.groupIDs)
			} else {
				require.NoError(t, err)
				require.True(t, got.IsExclusive)
				require.Equal(t, 0.125, got.RateMultiplier)
				require.Equal(t, []int64{7}, cache.groupIDs)
			}
		})
	}
}

func TestUpdateGroupRejectsInvalidAccessPreservation(t *testing.T) {
	for _, tc := range []struct {
		name                                     string
		exclusive, subscription, targetExclusive bool
	}{
		{"already exclusive", true, false, true},
		{"subscription", false, true, true},
		{"remains public", false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			group := &Group{ID: 7, Platform: PlatformOpenAI, IsExclusive: tc.exclusive, SubscriptionType: SubscriptionTypeStandard}
			if tc.subscription {
				group.SubscriptionType = SubscriptionTypeSubscription
			}
			repo := &accessTransitionRepoStub{groupPlatformRepoStub: groupPlatformRepoStub{group: group}}
			svc := &adminServiceImpl{groupRepo: repo}
			_, err := svc.UpdateGroup(context.Background(), 7, &UpdateGroupInput{IsExclusive: &tc.targetExclusive, PreserveExistingUsers: true})
			require.Error(t, err)
			require.Zero(t, repo.calls)
			require.Nil(t, repo.updated)
		})
	}
}

func TestUpdateGroupWithoutPreservationUsesNormalUpdate(t *testing.T) {
	repo := &accessTransitionRepoStub{groupPlatformRepoStub: groupPlatformRepoStub{group: &Group{ID: 7, Platform: PlatformOpenAI}}}
	exclusive := true
	_, err := (&adminServiceImpl{groupRepo: repo}).UpdateGroup(context.Background(), 7, &UpdateGroupInput{IsExclusive: &exclusive})
	require.NoError(t, err)
	require.Zero(t, repo.calls)
	require.True(t, repo.updated.IsExclusive)
}
