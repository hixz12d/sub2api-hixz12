//go:build integration

package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// Uses the repository suite's isolated PostgreSQL. A rollback-only schema keeps
// constraint fixtures independent of application data and other integration tests.
func TestMonitorFoundationMigrationPostgres(t *testing.T) {
	ctx := context.Background()
	tx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	schema := "monitor_p1_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, err = tx.ExecContext(ctx, `CREATE SCHEMA `+schema)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `SET LOCAL search_path TO `+schema)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `CREATE TABLE settings(key TEXT PRIMARY KEY,value TEXT NOT NULL);
 CREATE TABLE users(id BIGINT PRIMARY KEY);
 CREATE TABLE groups(id BIGINT PRIMARY KEY);
 CREATE TABLE accounts(id BIGINT PRIMARY KEY);
 CREATE TABLE api_keys(id BIGINT PRIMARY KEY);
 INSERT INTO users VALUES (1),(2); INSERT INTO groups VALUES (1),(2); INSERT INTO accounts VALUES (1); INSERT INTO api_keys VALUES (1);`)
	require.NoError(t, err)
	for _, migration := range []string{"238_channel_monitor_group_foundation.sql", "239_monitor_plan_scope_constraints.sql"} {
		raw, err := migrations.FS.ReadFile(migration)
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, string(raw))
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, string(raw))
		require.NoError(t, err, "migration must be repeatable: %s", migration)
	}
	var enabledCount int
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM settings WHERE value <> 'false'`).Scan(&enabledCount))
	require.Zero(t, enabledCount)
	var policyID int64
	var enabled bool
	var probe, capability []byte
	require.NoError(t, tx.QueryRowContext(ctx, `INSERT INTO channel_monitor_group_policies(group_id,primary_model,evaluation_revision,created_by,updated_by)
 VALUES(1,'fixture-model',repeat('a',64),1,1) RETURNING id,enabled,probe_config,capability_config`).Scan(&policyID, &enabled, &probe, &capability))
	require.False(t, enabled)
	var probeConfig service.GroupProbeConfig
	var capabilityConfig service.GroupCapabilityConfig
	require.NoError(t, json.Unmarshal(probe, &probeConfig))
	require.NoError(t, json.Unmarshal(capability, &capabilityConfig))
	require.Equal(t, service.DefaultGroupProbeConfig(), probeConfig)
	require.Equal(t, service.DefaultGroupCapabilityConfig(), capabilityConfig)
	mustRejectMonitorSQL(t, tx, `INSERT INTO channel_monitor_group_policies(group_id,primary_model,evaluation_revision,created_by,updated_by) VALUES(1,'other',repeat('b',64),1,1)`)

	jobID := uuid.NewString()
	jobSQL := `INSERT INTO monitor_jobs(id,kind,source,policy_id,evaluation_revision,idempotency_scope,idempotency_key_hash,payload_hash,config_snapshot,deadline_at,queue_deadline_at,base_requests_planned,outbound_reserved)
 VALUES($1,'capability','platform_group',$2,repeat('a',64),'fixture',repeat('b',64),repeat('c',64),'{}',NOW()+INTERVAL '1 hour',NOW()+INTERVAL '5 minutes',1,2)`
	_, err = tx.ExecContext(ctx, jobSQL, jobID, policyID)
	require.NoError(t, err)
	mustRejectMonitorSQL(t, tx, `UPDATE monitor_jobs SET outbound_dispatched=3 WHERE id=$1`, jobID)
	mustRejectMonitorSQL(t, tx, `UPDATE monitor_jobs SET state='running' WHERE id=$1`, jobID)
	mustRejectMonitorSQL(t, tx, `UPDATE monitor_jobs SET state='completed' WHERE id=$1`, jobID)
	mustRejectMonitorSQL(t, tx, `INSERT INTO monitor_jobs(id,kind,source,policy_id,evaluation_revision,idempotency_scope,idempotency_key_hash,payload_hash,config_snapshot,deadline_at,queue_deadline_at,base_requests_planned,outbound_reserved)
 VALUES($1,'capability','platform_group',$2,repeat('a',64),'different',repeat('d',64),repeat('c',64),'{}',NOW()+INTERVAL '1 hour',NOW()+INTERVAL '5 minutes',1,2)`, uuid.NewString(), policyID)
	_, err = tx.ExecContext(ctx, `INSERT INTO monitor_budget_buckets(scope,scope_id,utc_day,request_limit) VALUES('global',0,CURRENT_DATE,2)`)
	require.NoError(t, err)
	mustRejectMonitorSQL(t, tx, `UPDATE monitor_budget_buckets SET reserved=3`)
	mustRejectMonitorSQL(t, tx, `UPDATE monitor_budget_buckets SET consumed=-1`)

	planID := uuid.NewString()
	_, err = tx.ExecContext(ctx, `INSERT INTO llm_detector_plans(id,owner_user_id,source,normalized_target_spec,benchmark_manifest,configuration_hash,planned_base_requests,retry_budget_requests,maximum_outbound_requests,expires_at)
 VALUES($1,1,'site_api_key','{}','{}',repeat('e',64),1,1,2,NOW()+INTERVAL '5 minutes')`, planID)
	require.NoError(t, err)
	privateJobSQL := `INSERT INTO monitor_jobs(id,kind,source,plan_id,owner_user_id,idempotency_scope,idempotency_key_hash,payload_hash,config_snapshot,deadline_at,queue_deadline_at,base_requests_planned,outbound_reserved)
 VALUES($1,'capability','site_api_key',$2,$3,$4,repeat('f',64),repeat('a',64),'{}',NOW()+INTERVAL '1 hour',NOW()+INTERVAL '5 minutes',1,2)`
	mustRejectMonitorSQL(t, tx, privateJobSQL, uuid.NewString(), planID, 2, "user:2")
	_, err = tx.ExecContext(ctx, privateJobSQL, uuid.NewString(), planID, 1, "user:1")
	require.NoError(t, err)
	mustRejectMonitorSQL(t, tx, privateJobSQL, uuid.NewString(), planID, 1, "user:1:repeat")
	testMonitorPlanScopeConstraints(t, tx, jobID, policyID)
}

func mustRejectMonitorSQL(t *testing.T, tx *sql.Tx, query string, args ...any) error {
	t.Helper()
	_, err := tx.Exec(`SAVEPOINT monitor_rejection`)
	require.NoError(t, err)
	_, rejected := tx.Exec(query, args...)
	require.Error(t, rejected)
	_, err = tx.Exec(`ROLLBACK TO SAVEPOINT monitor_rejection`)
	require.NoError(t, err)
	_, err = tx.Exec(`RELEASE SAVEPOINT monitor_rejection`)
	require.NoError(t, err)
	return rejected
}

func testMonitorPlanScopeConstraints(t *testing.T, tx *sql.Tx, jobID string, policyID int64) {
	t.Helper()
	var otherPolicyID int64
	require.NoError(t, tx.QueryRow(`INSERT INTO channel_monitor_group_policies(group_id,primary_model,evaluation_revision,created_by,updated_by)
 VALUES(2,'fixture-model',repeat('a',64),1,1) RETURNING id`).Scan(&otherPolicyID))
	for _, test := range []struct {
		name     string
		source   string
		owner    any
		policy   any
		revision any
		allowed  bool
	}{
		{"private plan with null job owner", "site_api_key", int64(1), nil, nil, false},
		{"different platform policy", "platform_group", nil, otherPolicyID, strings.Repeat("a", 64), false},
		{"different platform revision", "platform_group", nil, policyID, strings.Repeat("b", 64), false},
		{"matching platform plan", "platform_group", nil, policyID, strings.Repeat("a", 64), true},
	} {
		t.Run(test.name, func(t *testing.T) {
			planID := uuid.NewString()
			_, err := tx.Exec(`INSERT INTO llm_detector_plans(id,owner_user_id,source,policy_id,evaluation_revision,normalized_target_spec,benchmark_manifest,configuration_hash,planned_base_requests,retry_budget_requests,maximum_outbound_requests,expires_at)
 VALUES($1,$2,$3,$4,$5,'{}','{}',repeat('e',64),1,1,2,NOW()+INTERVAL '5 minutes')`, planID, test.owner, test.source, test.policy, test.revision)
			require.NoError(t, err)
			query := `UPDATE monitor_jobs SET plan_id=$1 WHERE id=$2`
			if test.allowed {
				_, err = tx.Exec(query, planID, jobID)
				require.NoError(t, err)
				return
			}
			err = mustRejectMonitorSQL(t, tx, query, planID, jobID)
			var constraint *pq.Error
			require.ErrorAs(t, err, &constraint)
			require.Equal(t, pq.ErrorCode("23503"), constraint.Code)
			require.Contains(t, []string{"monitor_jobs_plan_source_fk", "monitor_jobs_plan_policy_revision_fk"}, constraint.Constraint)
		})
	}
}
