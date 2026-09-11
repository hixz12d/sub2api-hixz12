//go:build integration

package repository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// Exercise the old and new SQL write contracts against the migrated schema.
// This is a column-level rolling-compatibility test, not a two-binary rollout.
func TestGroupModelPolicyRollingWritesStayIndependent(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()
	var id int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO groups (name, platform, rate_multiplier, status, models_list_config)
VALUES ('model-policy-rolling', 'openai', 1, 'active', '{"enabled":true,"models":["gpt-5.5"]}'::jsonb)
RETURNING id`).Scan(&id))

	assertPolicies := func(display, admission string) {
		t.Helper()
		var actualDisplay, actualAdmission string
		require.NoError(t, tx.QueryRowContext(ctx,
			"SELECT models_list_config::text, model_allowlist::text FROM groups WHERE id = $1", id).
			Scan(&actualDisplay, &actualAdmission))
		require.JSONEq(t, display, actualDisplay)
		require.JSONEq(t, admission, actualAdmission)
	}
	assertPolicies(`{"enabled":true,"models":["gpt-5.5"]}`, `{}`)

	_, err := tx.ExecContext(ctx, `UPDATE groups SET model_allowlist = '{"enabled":true,"models":["gpt-5.4"]}'::jsonb WHERE id = $1`, id)
	require.NoError(t, err)
	assertPolicies(`{"enabled":true,"models":["gpt-5.5"]}`, `{"enabled":true,"models":["gpt-5.4"]}`)

	// A late write by the old binary must neither erase nor enable admission.
	_, err = tx.ExecContext(ctx, `UPDATE groups SET models_list_config = '{"enabled":true,"models":["gpt-6-astra"]}'::jsonb WHERE id = $1`, id)
	require.NoError(t, err)
	assertPolicies(`{"enabled":true,"models":["gpt-6-astra"]}`, `{"enabled":true,"models":["gpt-5.4"]}`)

	// Repair/replay and disabling the new feature must preserve old display data.
	applyGroupModelAllowlistRepair(ctx, t, tx)
	assertPolicies(`{"enabled":true,"models":["gpt-6-astra"]}`, `{"enabled":true,"models":["gpt-5.4"]}`)
	_, err = tx.ExecContext(ctx, `UPDATE groups SET model_allowlist = '{}'::jsonb WHERE id = $1`, id)
	require.NoError(t, err)
	assertPolicies(`{"enabled":true,"models":["gpt-6-astra"]}`, `{}`)
}
