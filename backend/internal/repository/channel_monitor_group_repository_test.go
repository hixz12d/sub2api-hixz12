//go:build unit

package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func monitorPolicyFixtureRows(t *testing.T, version int64) *sqlmock.Rows {
	t.Helper()
	probe, err := json.Marshal(service.DefaultGroupProbeConfig())
	require.NoError(t, err)
	capability, err := json.Marshal(service.DefaultGroupCapabilityConfig())
	require.NoError(t, err)
	return sqlmock.NewRows(strings.Split(monitorPolicyColumns, ",")).AddRow(
		int64(1), int64(1), "display", "model-a", `[]`, false, string(probe), string(capability), version, strings.Repeat("a", 64),
		nil, nil, nil, nil, int64(1), int64(1), time.Now(), time.Now(), nil)
}

func TestMonitorGroupRepositoryInactiveCreateAndCAS(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	r := NewChannelMonitorGroupRepository(db)
	p := service.ChannelMonitorGroupPolicy{ID: 1, GroupID: 1, PrimaryModel: "model-a", ProbeConfig: service.DefaultGroupProbeConfig(), CapabilityConfig: service.DefaultGroupCapabilityConfig()}
	mock.ExpectQuery("INSERT INTO channel_monitor_group_policies").WillReturnRows(monitorPolicyFixtureRows(t, 1))
	created, err := r.CreateInactivePolicy(context.Background(), p, 1)
	require.NoError(t, err)
	require.Equal(t, int64(1), created.Version)
	mock.ExpectQuery("INSERT INTO channel_monitor_group_policies").WillReturnError(&pq.Error{Code: "23505", Constraint: "channel_monitor_group_policies_live_group_uq"})
	_, err = r.CreateInactivePolicy(context.Background(), p, 1)
	require.ErrorIs(t, err, service.ErrMonitorPolicyExists)
	mock.ExpectQuery(`(?s)UPDATE channel_monitor_group_policies.*p.version=\$10`).WillReturnRows(monitorPolicyFixtureRows(t, 2))
	updated, err := r.UpdateInactivePolicy(context.Background(), p, 1, 1)
	require.NoError(t, err)
	require.Equal(t, int64(2), updated.Version)
	mock.ExpectQuery(`(?s)UPDATE channel_monitor_group_policies.*p.version=\$10`).WillReturnError(sql.ErrNoRows)
	_, err = r.UpdateInactivePolicy(context.Background(), p, 1, 1)
	require.ErrorIs(t, err, service.ErrMonitorPolicyConflict)
	p.ProbeConfig.Enabled = true
	_, err = r.CreateInactivePolicy(context.Background(), p, 1)
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
