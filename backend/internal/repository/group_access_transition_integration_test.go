//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGroupAccessTransition(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := newGroupRepositoryWithSQL(client, integrationDB)
	suffix := time.Now().UnixNano()
	g := &service.Group{Name: fmt.Sprintf("access-transition-%d", suffix), Platform: service.PlatformOpenAI,
		RateMultiplier: 0.125, Status: service.StatusActive, SubscriptionType: service.SubscriptionTypeStandard}
	require.NoError(t, repo.Create(ctx, g))
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM scheduler_outbox WHERE group_id = $1", g.ID)
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM user_allowed_groups WHERE group_id = $1", g.ID)
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM groups WHERE id = $1", g.ID)
	})
	createUser := func(name string, restricted bool, createdAt time.Time, deleted bool) int64 {
		t.Helper()
		var id int64
		var deletedAt any
		if deleted {
			deletedAt = time.Now()
		}
		require.NoError(t, integrationDB.QueryRowContext(ctx, `INSERT INTO users
			(email, password_hash, restrict_public_groups, created_at, deleted_at)
			VALUES ($1, 'unused', $2, $3, $4) RETURNING id`, fmt.Sprintf("access-%d-%s@example.test", suffix, name), restricted, createdAt, deletedAt).Scan(&id))
		t.Cleanup(func() { _, _ = integrationDB.ExecContext(ctx, "DELETE FROM users WHERE id = $1", id) })
		return id
	}
	old := time.Now().Add(-time.Hour)
	eligible := createUser("eligible-no-key", false, old, false)
	restricted := createUser("restricted", true, old, false)
	granted := createUser("granted", true, old, false)
	deleted := createUser("deleted", false, old, true)
	future := createUser("future", false, time.Now().Add(time.Hour), false)
	_, err := integrationDB.ExecContext(ctx, "INSERT INTO user_allowed_groups (user_id, group_id, created_at) VALUES ($1, $2, $3)", granted, g.ID, old)
	require.NoError(t, err)

	// Force a later failure: access grants and exclusivity must both roll back.
	functionName := fmt.Sprintf("fail_access_outbox_%d", suffix)
	_, err = integrationDB.ExecContext(ctx, fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN IF NEW.group_id = %d THEN RAISE EXCEPTION 'forced access outbox failure'; END IF; RETURN NEW; END; $$`, functionName, g.ID))
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, fmt.Sprintf("CREATE TRIGGER %s BEFORE INSERT ON scheduler_outbox FOR EACH ROW EXECUTE FUNCTION %s()", functionName, functionName))
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, fmt.Sprintf("DROP TRIGGER IF EXISTS %s ON scheduler_outbox", functionName))
		_, _ = integrationDB.ExecContext(ctx, fmt.Sprintf("DROP FUNCTION IF EXISTS %s()", functionName))
	})

	g, err = repo.GetByID(ctx, g.ID)
	require.NoError(t, err)
	expected := g.UpdatedAt
	g.IsExclusive = true
	err = repo.UpdatePreservingUserAccess(ctx, g, expected)
	require.ErrorContains(t, err, "forced access outbox failure")
	persisted, err := repo.GetByID(ctx, g.ID)
	require.NoError(t, err)
	require.False(t, persisted.IsExclusive)
	var count int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM user_allowed_groups WHERE group_id = $1", g.ID).Scan(&count))
	require.Equal(t, 1, count, "only the original explicit grant survives rollback")
	_, err = integrationDB.ExecContext(ctx, fmt.Sprintf("DROP TRIGGER %s ON scheduler_outbox", functionName))
	require.NoError(t, err)

	// Stale edits cannot accidentally enroll a different cohort.
	require.ErrorIs(t, repo.UpdatePreservingUserAccess(ctx, g, expected.Add(-time.Second)), service.ErrGroupAccessTransitionConflict)
	require.NoError(t, repo.UpdatePreservingUserAccess(ctx, g, expected))
	persisted, err = repo.GetByID(ctx, g.ID)
	require.NoError(t, err)
	require.True(t, persisted.IsExclusive)
	require.Equal(t, 0.125, persisted.RateMultiplier)

	for _, tc := range []struct {
		id      int64
		allowed bool
	}{{eligible, true}, {restricted, false}, {granted, true}, {deleted, false}, {future, false}} {
		require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM user_allowed_groups WHERE user_id = $1 AND group_id = $2", tc.id, g.ID).Scan(&count))
		require.Equal(t, tc.allowed, count == 1, "user %d", tc.id)
	}
	newUser := createUser("after-transition", false, time.Now(), false)
	require.ErrorIs(t, repo.UpdatePreservingUserAccess(ctx, g, g.UpdatedAt), service.ErrGroupAccessTransitionConflict)
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM user_allowed_groups WHERE user_id = $1 AND group_id = $2", newUser, g.ID).Scan(&count))
	require.Zero(t, count, "retry must not grant access to a new user")
	var grantedAt time.Time
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT created_at FROM user_allowed_groups WHERE user_id = $1 AND group_id = $2", granted, g.ID).Scan(&grantedAt))
	require.WithinDuration(t, old, grantedAt, time.Microsecond)
}
