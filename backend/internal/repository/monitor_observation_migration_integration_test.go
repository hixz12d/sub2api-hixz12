//go:build integration

package repository

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestMonitorObservationMigrationPostgres(t *testing.T) {
	ctx := context.Background()
	tx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	schema := "monitor_p2_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, err = tx.ExecContext(ctx, `CREATE SCHEMA `+schema+`; SET LOCAL search_path TO `+schema+`; CREATE TABLE usage_logs(id BIGSERIAL PRIMARY KEY); CREATE TABLE ops_error_logs(id BIGSERIAL PRIMARY KEY); INSERT INTO usage_logs DEFAULT VALUES;`)
	require.NoError(t, err)
	apply := func(name string) {
		t.Helper()
		raw, err := migrations.FS.ReadFile(name)
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, string(raw))
		require.NoError(t, err, name)
	}
	apply("240_usage_monitor_observations.sql")
	_, err = tx.ExecContext(ctx, `INSERT INTO usage_logs(request_origin,monitor_observation_version,monitor_visible_output_tokens,monitor_generation_ms,monitor_output_tps_milli,monitor_tps_method) VALUES ('business',1,80,2000,40000,'visible_stream_v1'); INSERT INTO usage_logs(request_origin) VALUES ('capability_detector');`)
	require.NoError(t, err)
	apply("241_monitor_request_origins.sql")
	apply("241_monitor_request_origins.sql")
	var origin sql.NullString
	require.NoError(t, tx.QueryRow(`SELECT request_origin FROM usage_logs WHERE id=1`).Scan(&origin))
	require.False(t, origin.Valid, "historical NULL must not become measured real traffic")
	require.NoError(t, tx.QueryRow(`SELECT request_origin FROM usage_logs WHERE id=2`).Scan(&origin))
	require.Equal(t, "real_traffic", origin.String)
	require.NoError(t, tx.QueryRow(`SELECT request_origin FROM usage_logs WHERE id=3`).Scan(&origin))
	require.Equal(t, "capability_probe", origin.String)
	for _, origin := range []string{"real_traffic", "availability_probe", "capability_probe", "user_detector", "legacy_unknown"} {
		_, err = tx.Exec(`INSERT INTO usage_logs(request_origin) VALUES($1);`, origin)
		require.NoError(t, err)
		_, err = tx.Exec(`INSERT INTO ops_error_logs(request_origin) VALUES($1);`, origin)
		require.NoError(t, err)
	}
	for _, query := range []string{
		`INSERT INTO usage_logs(request_origin) VALUES('business')`,
		`INSERT INTO ops_error_logs(request_origin) VALUES('spoofed')`,
		`INSERT INTO usage_logs(monitor_input_tokens_total,monitor_cache_read_tokens) VALUES(100,101)`,
		`INSERT INTO usage_logs(monitor_cache_read_tokens) VALUES(0)`,
		`INSERT INTO usage_logs(monitor_output_tps_milli) VALUES(40000)`,
		`INSERT INTO usage_logs(request_origin,monitor_observation_version,monitor_visible_output_tokens,monitor_generation_ms,monitor_output_tps_milli,monitor_tps_method) VALUES ('user_detector',1,80,2000,40000,'visible_stream_v1')`,
	} {
		rejected := mustRejectMonitorSQL(t, tx, query)
		var pgerr *pq.Error
		require.ErrorAs(t, rejected, &pgerr)
		require.Equal(t, pq.ErrorCode("23514"), pgerr.Code)
	}
	_, err = tx.Exec(`INSERT INTO usage_logs DEFAULT VALUES; INSERT INTO ops_error_logs DEFAULT VALUES; INSERT INTO usage_logs(monitor_input_tokens_total,monitor_cache_read_tokens) VALUES(100,0);`)
	require.NoError(t, err, "old writers and explicit zero cache remain supported")
}
