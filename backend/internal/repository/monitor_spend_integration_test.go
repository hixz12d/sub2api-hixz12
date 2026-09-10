//go:build integration

package repository

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestMonitorSpendCampaignPostgres(t *testing.T) {
	db := monitorBudgetTestDB(t)
	ctx := context.Background()
	plan := monitorJobTestPlan(t, db, 1)
	var hash string
	require.NoError(t, db.QueryRow(`SELECT configuration_hash FROM llm_detector_plans WHERE id=$1`, plan).Scan(&hash))
	job, err := NewMonitorJobRepository(db).CreateFromPlan(ctx, CreateMonitorJobInput{PlanID: plan, OwnerID: 1, IdempotencyKey: "spend-test", ConfigurationHash: hash, Limits: monitorBudgetTestLimits(1, 100, 100)})
	require.NoError(t, err)
	for _, tc := range []struct {
		name                       string
		requests, usd, bound, want int64
	}{{"request_ceiling", 3, 100, 1, 3}, {"dollar_ceiling", 100, 10, 3, 3}, {"exact_dollar_ceiling", 100, 12, 4, 3}} {
		t.Run(tc.name, func(t *testing.T) {
			campaign := uuid.NewString()
			_, err = db.Exec(`INSERT INTO monitor_spend_campaigns(id,request_limit,usd_limit_micros,enabled) VALUES($1,$2,$3,true)`, campaign, tc.requests, tc.usd)
			require.NoError(t, err)
			var passed atomic.Int64
			var wg sync.WaitGroup
			failures := make(chan error, 20)
			for i := 0; i < 20; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					tx, err := db.BeginTx(ctx, nil)
					if err != nil {
						failures <- err
						return
					}
					defer tx.Rollback()
					err = chargeMonitorSpendTx(ctx, tx, campaign, job, uuid.NewString(), tc.bound)
					if err == ErrMonitorSpendDenied {
						return
					}
					if err != nil {
						failures <- err
						return
					}
					if err = tx.Commit(); err != nil {
						failures <- err
						return
					}
					passed.Add(1)
				}()
			}
			wg.Wait()
			close(failures)
			for err := range failures {
				require.NoError(t, err)
			}
			require.Equal(t, tc.want, passed.Load())
			var requests, usd int64
			require.NoError(t, db.QueryRow(`SELECT requests_charged,usd_charged_micros FROM monitor_spend_campaigns WHERE id=$1`, campaign).Scan(&requests, &usd))
			require.Equal(t, tc.want, requests)
			require.Equal(t, tc.want*tc.bound, usd)
		})
	}
	campaign := uuid.NewString()
	_, err = db.Exec(`INSERT INTO monitor_spend_campaigns(id,request_limit,usd_limit_micros,enabled) VALUES($1,10,100,true)`, campaign)
	require.NoError(t, err)
	outbound := uuid.NewString()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	require.NoError(t, chargeMonitorSpendTx(ctx, tx, campaign, job, outbound, 7))
	require.NoError(t, tx.Rollback())
	var requests int64
	require.NoError(t, db.QueryRow(`SELECT requests_charged FROM monitor_spend_campaigns WHERE id=$1`, campaign).Scan(&requests))
	require.Zero(t, requests)
	for i := 0; i < 2; i++ {
		tx, err = db.BeginTx(ctx, nil)
		require.NoError(t, err)
		require.NoError(t, chargeMonitorSpendTx(ctx, tx, campaign, job, outbound, 7))
		require.NoError(t, tx.Commit())
	}
	require.NoError(t, db.QueryRow(`SELECT requests_charged FROM monitor_spend_campaigns WHERE id=$1`, campaign).Scan(&requests))
	require.EqualValues(t, 1, requests)
	tx, err = db.BeginTx(ctx, nil)
	require.NoError(t, err)
	require.ErrorIs(t, chargeMonitorSpendTx(ctx, tx, campaign, job, outbound, 8), ErrMonitorSpendDenied)
	require.NoError(t, tx.Rollback())
	_, err = db.Exec(`UPDATE monitor_spend_campaigns SET enabled=false WHERE id=$1`, campaign)
	require.NoError(t, err)
	tx, err = db.BeginTx(ctx, nil)
	require.NoError(t, err)
	require.ErrorIs(t, chargeMonitorSpendTx(ctx, tx, campaign, job, uuid.NewString(), 7), ErrMonitorSpendDenied)
	require.NoError(t, tx.Rollback())
}
