package repository

import (
	"context"
	"database/sql"
	"errors"
)

var ErrMonitorSpendDenied = errors.New("monitor campaign spending denied")

// Call after the job/attempt locks and before committing the dispatch permit.
// Unknown outcomes keep the full upper bound charged; no background refund exists.
func chargeMonitorSpendTx(ctx context.Context, tx *sql.Tx, campaignID, jobID, outboundID string, upperBoundMicros int64) error {
	if !validDetectorResourceID(campaignID) || !validDetectorResourceID(jobID) || !validDetectorResourceID(outboundID) || upperBoundMicros <= 0 || upperBoundMicros > 1000000000000 {
		return ErrMonitorSpendDenied
	}
	var enabled bool
	var requestLimit, usdLimit, requests, usd int64
	err := tx.QueryRowContext(ctx, `SELECT enabled,request_limit,usd_limit_micros,requests_charged,usd_charged_micros FROM monitor_spend_campaigns WHERE id=$1 FOR UPDATE`, campaignID).Scan(&enabled, &requestLimit, &usdLimit, &requests, &usd)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrMonitorSpendDenied
	}
	if err != nil {
		return err
	}
	var previousCampaign, previousJob string
	var previousBound int64
	err = tx.QueryRowContext(ctx, `SELECT campaign_id,job_id,upper_bound_micros FROM monitor_spend_charges WHERE outbound_request_id=$1`, outboundID).Scan(&previousCampaign, &previousJob, &previousBound)
	if err == nil {
		if previousCampaign != campaignID || previousJob != jobID || previousBound != upperBoundMicros {
			return ErrMonitorSpendDenied
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if !enabled || requests >= requestLimit || upperBoundMicros > usdLimit-usd {
		return ErrMonitorSpendDenied
	}
	if _, err = tx.ExecContext(ctx, `UPDATE monitor_spend_campaigns SET requests_charged=requests_charged+1,usd_charged_micros=usd_charged_micros+$2 WHERE id=$1`, campaignID, upperBoundMicros); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO monitor_spend_charges(outbound_request_id,campaign_id,job_id,upper_bound_micros) VALUES($1,$2,$3,$4)`, outboundID, campaignID, jobID, upperBoundMicros)
	return err
}
