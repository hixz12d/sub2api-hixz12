package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
)

// CreateMonitorJobInput contains a previously admitted private plan, not a raw
// target, Key, or benchmark. Authorization/engine readiness must be rechecked by
// the application before this persistence operation; nothing here starts a worker.
type CreateMonitorJobInput = service.DetectorJobInput

func validMonitorConfigurationHash(hash string) bool {
	if len(hash) != 64 || strings.ToLower(hash) != hash {
		return false
	}
	_, err := hex.DecodeString(hash)
	return err == nil
}

// CreateFromPlan commits plan consumption, job identity and all budget scopes
// together. Repeating the same key returns the same job without reserving again;
// another plan/configuration with the same key is always a conflict.
func (r *MonitorJobRepository) CreateFromPlan(ctx context.Context, input CreateMonitorJobInput) (string, error) {
	if input.OwnerID <= 0 || !validDetectorResourceID(input.PlanID) || len(input.IdempotencyKey) == 0 || len(input.IdempotencyKey) > 200 || strings.TrimSpace(input.IdempotencyKey) != input.IdempotencyKey ||
		!validMonitorConfigurationHash(input.ConfigurationHash) || len(input.SecretOwnerInstanceID) > 200 {
		return "", ErrMonitorBudgetInput
	}
	input.PlanID = strings.ToLower(input.PlanID)
	limits, err := normalizeMonitorBudgetLimits(input.Limits)
	if err != nil {
		return "", err
	}
	scope := fmt.Sprintf("user:%d:detector", input.OwnerID)
	keySum := sha256.Sum256([]byte(input.IdempotencyKey))
	keyHash := hex.EncodeToString(keySum[:])
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	existing := func() (string, error) {
		var id, plan, hash string
		err := tx.QueryRowContext(ctx, `SELECT id,plan_id,payload_hash FROM monitor_jobs WHERE owner_user_id=$1
 AND kind='capability' AND source IN ('external_api','site_api_key') AND idempotency_scope=$2 AND idempotency_key_hash=$3`, input.OwnerID, scope, keyHash).Scan(&id, &plan, &hash)
		if err != nil {
			return "", err
		}
		if plan != input.PlanID || hash != input.ConfigurationHash {
			return "", ErrMonitorBudgetConflict
		}
		return id, nil
	}
	if id, err := existing(); err == nil {
		return id, tx.Commit()
	} else if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	var source service.MonitorJobSource
	var targetRaw, benchmarkRaw []byte
	var configHash string
	var base, ceiling int64
	// Serializes different idempotency keys trying to consume the same plan.
	err = tx.QueryRowContext(ctx, `SELECT source,normalized_target_spec,benchmark_manifest,configuration_hash,planned_base_requests,maximum_outbound_requests
 FROM llm_detector_plans WHERE id=$1 AND owner_user_id=$2 AND source IN ('external_api','site_api_key') FOR UPDATE`, input.PlanID, input.OwnerID).Scan(
		&source, &targetRaw, &benchmarkRaw, &configHash, &base, &ceiling)
	if err != nil {
		return "", err
	}
	if id, err := existing(); err == nil {
		return id, tx.Commit()
	} else if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if configHash != input.ConfigurationHash {
		return "", ErrMonitorBudgetConflict
	}
	var consumed bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM monitor_jobs WHERE plan_id=$1)`, input.PlanID).Scan(&consumed); err != nil {
		return "", err
	}
	if consumed {
		return "", ErrMonitorBudgetConflict
	}
	if (source == service.MonitorSourceExternalAPI && strings.TrimSpace(input.SecretOwnerInstanceID) == "") ||
		(source == service.MonitorSourceSiteAPIKey && input.SecretOwnerInstanceID != "") {
		return "", ErrMonitorBudgetInput
	}
	var target service.DetectorTargetSpec
	var benchmark service.DetectorBenchmarkManifest
	if err := service.DecodeMonitorConfig(targetRaw, &target); err != nil {
		return "", ErrMonitorBudgetInput
	}
	if err := service.DecodeMonitorConfig(benchmarkRaw, &benchmark); err != nil {
		return "", ErrMonitorBudgetInput
	}
	if target.Source != source {
		return "", ErrMonitorBudgetInput
	}
	if err := lockApprovedBenchmarkTx(ctx, tx, benchmark); err != nil {
		return "", err
	}
	if err := lockCurrentBenchmarkChannelTx(ctx, tx, benchmark); err != nil {
		return "", err
	}
	snapshot, err := json.Marshal(service.MonitorJobSnapshot{Targets: []service.DetectorTargetSpec{target}, Benchmarks: []service.DetectorBenchmarkManifest{benchmark}})
	if err != nil {
		return "", err
	}
	id := uuid.NewString()
	var created string
	err = tx.QueryRowContext(ctx, `INSERT INTO monitor_jobs(id,kind,source,plan_id,owner_user_id,idempotency_scope,idempotency_key_hash,payload_hash,
 config_snapshot,deadline_at,queue_deadline_at,base_requests_planned,outbound_reserved,secret_owner_instance_id)
 VALUES($1,'capability',$2,$3,$4,$5,$6,$7,$8,transaction_timestamp()+interval '2 hours',transaction_timestamp()+interval '5 minutes',$9,$10,NULLIF($11,''))
 ON CONFLICT(idempotency_scope,idempotency_key_hash) DO NOTHING RETURNING id`, id, source, input.PlanID, input.OwnerID, scope, keyHash, configHash, snapshot, base, ceiling, input.SecretOwnerInstanceID).Scan(&created)
	if errors.Is(err, sql.ErrNoRows) {
		id, err := existing()
		if err != nil {
			return "", err
		}
		return id, tx.Commit()
	}
	if err != nil {
		return "", err
	}
	if err := reserveMonitorBudgetTx(ctx, tx, created, limits); err != nil {
		return "", err
	}
	var valid bool
	if err := tx.QueryRowContext(ctx, `SELECT expires_at>clock_timestamp() FROM llm_detector_plans WHERE id=$1`, input.PlanID).Scan(&valid); err != nil {
		return "", err
	}
	if !valid {
		return "", ErrMonitorBudgetLease
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return created, nil
}
