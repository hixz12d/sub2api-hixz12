//go:build integration

package repository

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func monitorBudgetTestDB(t *testing.T) *sql.DB {
	t.Helper()
	schema := "monitor_budget_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, err := integrationDB.Exec(`CREATE SCHEMA ` + pq.QuoteIdentifier(schema))
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := integrationDB.Exec(`DROP SCHEMA ` + pq.QuoteIdentifier(schema) + ` CASCADE`)
		require.NoError(t, err)
	})
	dsn, err := url.Parse(integrationDSN)
	require.NoError(t, err)
	query := dsn.Query()
	query.Set("search_path", schema)
	query.Set("TimeZone", "Pacific/Honolulu")
	dsn.RawQuery = query.Encode()
	db, err := sql.Open("postgres", dsn.String())
	require.NoError(t, err)
	db.SetMaxOpenConns(12)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	_, err = db.Exec(`CREATE TABLE settings(key TEXT PRIMARY KEY,value TEXT NOT NULL);
 CREATE TABLE users(id BIGINT PRIMARY KEY); CREATE TABLE groups(id BIGINT PRIMARY KEY);
 CREATE TABLE accounts(id BIGINT PRIMARY KEY); CREATE TABLE api_keys(id BIGINT PRIMARY KEY);
 INSERT INTO users VALUES(1),(2); INSERT INTO groups VALUES(1),(2);`)
	require.NoError(t, err)
	for _, name := range []string{"238_channel_monitor_group_foundation.sql", "239_monitor_plan_scope_constraints.sql", "244_monitor_probe_dispatches.sql", "245_monitor_benchmark_registry.sql", "246_monitor_spend_campaigns.sql"} {
		raw, err := migrations.FS.ReadFile(name)
		require.NoError(t, err)
		_, err = db.Exec(string(raw))
		require.NoError(t, err)
	}
	return db
}

func monitorBudgetTestJob(t *testing.T, db *sql.DB, owner, ceiling int64) string {
	t.Helper()
	plan, job := uuid.NewString(), uuid.NewString()
	manifest := monitorTestApprovedManifest(t, db, owner)
	_, err := db.Exec(`INSERT INTO llm_detector_plans(id,owner_user_id,source,normalized_target_spec,benchmark_manifest,configuration_hash,
 planned_base_requests,retry_budget_requests,maximum_outbound_requests,expires_at)
 VALUES($1,$2,'site_api_key','{}',$4,repeat('a',64),$3,0,$3,clock_timestamp()+interval '5 minutes')`, plan, owner, ceiling, manifest)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO monitor_jobs(id,kind,source,plan_id,owner_user_id,idempotency_scope,idempotency_key_hash,payload_hash,
 config_snapshot,deadline_at,queue_deadline_at,base_requests_planned,outbound_reserved)
 VALUES($1,'capability','site_api_key',$2,$3,$5,repeat('a',64),repeat('b',64),jsonb_build_object('benchmarks',jsonb_build_array($6::jsonb)),clock_timestamp()+interval '1 hour',clock_timestamp()+interval '5 minutes',$4,$4)`, job, plan, owner, ceiling, job, manifest)
	require.NoError(t, err)
	return job
}

func monitorBudgetTestRunning(t *testing.T, db *sql.DB, job string) {
	t.Helper()
	_, err := db.Exec(`UPDATE monitor_jobs SET state='running',lease_owner='worker',lease_generation=1,
 lease_expires_at=clock_timestamp()+interval '90 seconds' WHERE id=$1`, job)
	require.NoError(t, err)
}

func monitorBudgetTestLimits(owner, global, user int64) []MonitorBudgetLimit {
	return []MonitorBudgetLimit{{Scope: "user", ScopeID: owner, RequestLimit: user}, {Scope: "global", RequestLimit: global}}
}

func monitorBudgetTestBucket(t *testing.T, db *sql.DB, scope string, offset int, reserved, consumed int64) {
	t.Helper()
	var gotReserved, gotConsumed int64
	require.NoError(t, db.QueryRow(`SELECT reserved,consumed FROM monitor_budget_buckets
 WHERE scope=$1 AND utc_day=(clock_timestamp() AT TIME ZONE 'UTC')::date+$2::integer`, scope, offset).Scan(&gotReserved, &gotConsumed))
	require.Equal(t, reserved, gotReserved, scope)
	require.Equal(t, consumed, gotConsumed, scope)
}

func monitorBudgetTestYesterday(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO monitor_budget_buckets(scope,scope_id,utc_day,request_limit,reserved,consumed)
 SELECT scope,scope_id,utc_day-1,request_limit,reserved,consumed FROM monitor_budget_buckets;
 UPDATE monitor_budget_reservations SET utc_day=utc_day-1;
 DELETE FROM monitor_budget_buckets WHERE utc_day=(clock_timestamp() AT TIME ZONE 'UTC')::date;`)
	require.NoError(t, err)
}

func TestMonitorBudgetPostgres(t *testing.T) {
	ctx := context.Background()
	t.Run("concurrent reservation respects global ceiling", func(t *testing.T) {
		db := monitorBudgetTestDB(t)
		r := NewMonitorBudgetRepository(db)
		jobs := make([]string, 16)
		for i := range jobs {
			jobs[i] = monitorBudgetTestJob(t, db, int64(i%2+1), 2)
		}
		results := make(chan error, len(jobs))
		var wg sync.WaitGroup
		for i, job := range jobs {
			wg.Add(1)
			go func(owner int64, id string) {
				defer wg.Done()
				results <- r.Reserve(ctx, id, monitorBudgetTestLimits(owner, 5, 10))
			}(int64(i%2+1), job)
		}
		wg.Wait()
		close(results)
		successes := 0
		for err := range results {
			if err == nil {
				successes++
			} else {
				require.ErrorIs(t, err, ErrMonitorBudgetExhausted)
			}
		}
		require.Equal(t, 2, successes)
		monitorBudgetTestBucket(t, db, "global", 0, 4, 0)
		var count int
		require.NoError(t, db.QueryRow(`SELECT count(*) FROM monitor_budget_reservations`).Scan(&count))
		require.Equal(t, 4, count)
	})
	t.Run("idempotent reserve and dispatch CAS", func(t *testing.T) {
		db := monitorBudgetTestDB(t)
		r := NewMonitorBudgetRepository(db)
		job := monitorBudgetTestJob(t, db, 1, 3)
		limits := monitorBudgetTestLimits(1, 10, 10)
		results := make(chan error, 16)
		var wg sync.WaitGroup
		for i := 0; i < 16; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); results <- r.Reserve(ctx, job, limits) }()
		}
		wg.Wait()
		for i := 0; i < 16; i++ {
			require.NoError(t, <-results)
		}
		monitorBudgetTestBucket(t, db, "global", 0, 3, 0)
		monitorBudgetTestRunning(t, db, job)
		for i := 0; i < 16; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); results <- r.Consume(ctx, job, "worker", 1, 0, limits) }()
		}
		wg.Wait()
		successes := 0
		for i := 0; i < 16; i++ {
			err := <-results
			if err == nil {
				successes++
			} else {
				require.ErrorIs(t, err, ErrMonitorBudgetConflict)
			}
		}
		require.Equal(t, 1, successes)
		monitorBudgetTestBucket(t, db, "global", 0, 2, 1)
		require.NoError(t, r.Consume(ctx, job, "worker", 1, 1, limits))
		require.NoError(t, r.Consume(ctx, job, "worker", 1, 2, limits))
		require.ErrorIs(t, r.Consume(ctx, job, "worker", 1, 3, limits), ErrMonitorBudgetExhausted)
		monitorBudgetTestBucket(t, db, "user", 0, 0, 3)
	})
	t.Run("scope denial and all or nothing rollback", func(t *testing.T) {
		db := monitorBudgetTestDB(t)
		r := NewMonitorBudgetRepository(db)
		job := monitorBudgetTestJob(t, db, 1, 3)
		require.ErrorIs(t, r.Reserve(ctx, job, monitorBudgetTestLimits(2, 10, 10)), ErrMonitorBudgetInput)
		require.ErrorIs(t, r.Reserve(ctx, job, monitorBudgetTestLimits(1, 10, 2)), ErrMonitorBudgetExhausted)
		var count int
		require.NoError(t, db.QueryRow(`SELECT count(*) FROM monitor_budget_buckets`).Scan(&count))
		require.Zero(t, count)
		require.NoError(t, r.Reserve(ctx, job, monitorBudgetTestLimits(1, 10, 10)))
		require.ErrorIs(t, r.Reserve(ctx, job, monitorBudgetTestLimits(1, 11, 10)), ErrMonitorBudgetConflict)
		changed := append(monitorBudgetTestLimits(1, 10, 10), MonitorBudgetLimit{Scope: "credential", ScopeID: 1, RequestLimit: 10})
		require.ErrorIs(t, r.Reserve(ctx, job, changed), ErrMonitorBudgetConflict)
		monitorBudgetTestBucket(t, db, "global", 0, 3, 0)
	})
	t.Run("fencing cancellation and terminal guards", func(t *testing.T) {
		db := monitorBudgetTestDB(t)
		r := NewMonitorBudgetRepository(db)
		job := monitorBudgetTestJob(t, db, 1, 3)
		limits := monitorBudgetTestLimits(1, 10, 10)
		require.NoError(t, r.Reserve(ctx, job, limits))
		monitorBudgetTestRunning(t, db, job)
		require.ErrorIs(t, r.Consume(ctx, job, "stale-worker", 1, 0, limits), ErrMonitorBudgetLease)
		require.ErrorIs(t, r.Consume(ctx, job, "worker", 2, 0, limits), ErrMonitorBudgetLease)
		require.ErrorIs(t, r.Settle(ctx, job), ErrMonitorBudgetLease)
		_, err := db.Exec(`UPDATE monitor_jobs SET lease_expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, job)
		require.NoError(t, err)
		require.ErrorIs(t, r.Consume(ctx, job, "worker", 1, 0, limits), ErrMonitorBudgetLease)
		_, err = db.Exec(`UPDATE monitor_jobs SET lease_expires_at=clock_timestamp()+interval '90 seconds',cancel_requested_at=clock_timestamp() WHERE id=$1`, job)
		require.NoError(t, err)
		require.ErrorIs(t, r.Consume(ctx, job, "worker", 1, 0, limits), ErrMonitorBudgetLease)
		monitorBudgetTestBucket(t, db, "global", 0, 3, 0)
	})
	t.Run("concurrent terminal settlement preserves consumption", func(t *testing.T) {
		db := monitorBudgetTestDB(t)
		r := NewMonitorBudgetRepository(db)
		job := monitorBudgetTestJob(t, db, 1, 3)
		limits := monitorBudgetTestLimits(1, 10, 10)
		require.NoError(t, r.Reserve(ctx, job, limits))
		monitorBudgetTestRunning(t, db, job)
		require.NoError(t, r.Consume(ctx, job, "worker", 1, 0, limits))
		_, err := db.Exec(`UPDATE monitor_jobs SET state='interrupted',finished_at=clock_timestamp() WHERE id=$1`, job)
		require.NoError(t, err)
		results := make(chan error, 16)
		var wg sync.WaitGroup
		for i := 0; i < 16; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); results <- r.Settle(ctx, job) }()
		}
		wg.Wait()
		for i := 0; i < 16; i++ {
			require.NoError(t, <-results)
		}
		monitorBudgetTestBucket(t, db, "global", 0, 0, 1)
		monitorBudgetTestBucket(t, db, "user", 0, 0, 1)
		var settled int
		require.NoError(t, db.QueryRow(`SELECT count(*) FROM monitor_budget_reservations WHERE settled_at IS NOT NULL AND consumed=1 AND released=2`).Scan(&settled))
		require.Equal(t, 2, settled)
	})
	t.Run("UTC rollover keeps old consumption and charges new day", func(t *testing.T) {
		db := monitorBudgetTestDB(t)
		r := NewMonitorBudgetRepository(db)
		job := monitorBudgetTestJob(t, db, 1, 3)
		limits := monitorBudgetTestLimits(1, 10, 10)
		require.NoError(t, r.Reserve(ctx, job, limits))
		monitorBudgetTestRunning(t, db, job)
		require.NoError(t, r.Consume(ctx, job, "worker", 1, 0, limits))
		monitorBudgetTestYesterday(t, db)
		require.NoError(t, r.Consume(ctx, job, "worker", 1, 1, limits))
		monitorBudgetTestBucket(t, db, "global", -1, 0, 1)
		monitorBudgetTestBucket(t, db, "global", 0, 1, 1)
		monitorBudgetTestBucket(t, db, "user", -1, 0, 1)
		monitorBudgetTestBucket(t, db, "user", 0, 1, 1)
		var released int64
		require.NoError(t, db.QueryRow(`SELECT sum(released) FROM monitor_budget_reservations WHERE utc_day=(clock_timestamp() AT TIME ZONE 'UTC')::date-1`).Scan(&released))
		require.EqualValues(t, 4, released)
	})
	t.Run("UTC rollover denial rolls back every scope", func(t *testing.T) {
		db := monitorBudgetTestDB(t)
		r := NewMonitorBudgetRepository(db)
		job := monitorBudgetTestJob(t, db, 1, 3)
		limits := monitorBudgetTestLimits(1, 10, 10)
		require.NoError(t, r.Reserve(ctx, job, limits))
		monitorBudgetTestRunning(t, db, job)
		monitorBudgetTestYesterday(t, db)
		_, err := db.Exec(`INSERT INTO monitor_budget_buckets(scope,scope_id,utc_day,request_limit,consumed) VALUES('user',1,(clock_timestamp() AT TIME ZONE 'UTC')::date,10,10)`)
		require.NoError(t, err)
		require.ErrorIs(t, r.Consume(ctx, job, "worker", 1, 0, limits), ErrMonitorBudgetExhausted)
		monitorBudgetTestBucket(t, db, "global", -1, 3, 0)
		monitorBudgetTestBucket(t, db, "user", -1, 3, 0)
		var count int
		require.NoError(t, db.QueryRow(`SELECT count(*) FROM monitor_budget_reservations WHERE settled_at IS NOT NULL`).Scan(&count))
		require.Zero(t, count)
		_, err = db.Exec(`UPDATE monitor_jobs SET state='cancelled',finished_at=clock_timestamp() WHERE id=$1`, job)
		require.NoError(t, err)
		require.NoError(t, r.Settle(ctx, job))
		monitorBudgetTestBucket(t, db, "global", -1, 0, 0)
	})
	t.Run("queued rollover and expired queue", func(t *testing.T) {
		db := monitorBudgetTestDB(t)
		r := NewMonitorBudgetRepository(db)
		job := monitorBudgetTestJob(t, db, 1, 3)
		limits := monitorBudgetTestLimits(1, 10, 10)
		require.NoError(t, r.Reserve(ctx, job, limits))
		monitorBudgetTestYesterday(t, db)
		require.NoError(t, r.Reserve(ctx, job, limits))
		monitorBudgetTestBucket(t, db, "global", -1, 0, 0)
		monitorBudgetTestBucket(t, db, "global", 0, 3, 0)
		_, err := db.Exec(`UPDATE monitor_jobs SET created_at=clock_timestamp()-interval '10 minutes',queue_deadline_at=clock_timestamp()-interval '1 second' WHERE id=$1`, job)
		require.NoError(t, err)
		require.ErrorIs(t, r.Reserve(ctx, job, limits), ErrMonitorBudgetLease)
	})
	t.Run("SQL failure rolls back partial consumption", func(t *testing.T) {
		db := monitorBudgetTestDB(t)
		r := NewMonitorBudgetRepository(db)
		job := monitorBudgetTestJob(t, db, 1, 3)
		limits := monitorBudgetTestLimits(1, 10, 10)
		require.NoError(t, r.Reserve(ctx, job, limits))
		monitorBudgetTestRunning(t, db, job)
		_, err := db.Exec(`CREATE FUNCTION reject_budget_test() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.scope='user' AND NEW.consumed>OLD.consumed THEN RAISE EXCEPTION 'budget fault fixture'; END IF; RETURN NEW; END $$;
 CREATE TRIGGER reject_budget_test BEFORE UPDATE ON monitor_budget_buckets FOR EACH ROW EXECUTE FUNCTION reject_budget_test();`)
		require.NoError(t, err)
		err = r.Consume(ctx, job, "worker", 1, 0, limits)
		require.Error(t, err)
		var pgErr *pq.Error
		require.True(t, errors.As(err, &pgErr))
		require.Equal(t, pq.ErrorCode("P0001"), pgErr.Code)
		monitorBudgetTestBucket(t, db, "global", 0, 3, 0)
		monitorBudgetTestBucket(t, db, "user", 0, 3, 0)
		_, err = db.Exec(`DROP TRIGGER reject_budget_test ON monitor_budget_buckets`)
		require.NoError(t, err)
		require.NoError(t, r.Consume(ctx, job, "worker", 1, 0, limits))
	})
	t.Run("lease expires while bucket lock is held", func(t *testing.T) {
		db := monitorBudgetTestDB(t)
		r := NewMonitorBudgetRepository(db)
		job := monitorBudgetTestJob(t, db, 1, 3)
		limits := monitorBudgetTestLimits(1, 10, 10)
		require.NoError(t, r.Reserve(ctx, job, limits))
		monitorBudgetTestRunning(t, db, job)
		_, err := db.Exec(`UPDATE monitor_jobs SET lease_expires_at=clock_timestamp()+interval '150 milliseconds' WHERE id=$1`, job)
		require.NoError(t, err)
		blocker, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer blocker.Rollback()
		_, err = blocker.Exec(`SELECT 1 FROM monitor_budget_buckets WHERE scope='global' FOR UPDATE`)
		require.NoError(t, err)
		done := make(chan error, 1)
		go func() { done <- r.Consume(ctx, job, "worker", 1, 0, limits) }()
		time.Sleep(250 * time.Millisecond)
		require.NoError(t, blocker.Rollback())
		require.ErrorIs(t, <-done, ErrMonitorBudgetLease)
		monitorBudgetTestBucket(t, db, "global", 0, 3, 0)
	})
}
