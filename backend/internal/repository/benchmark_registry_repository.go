package repository

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
)

var ErrBenchmarkRelease = service.ErrBenchmarkRelease

type BenchmarkPackageInput = service.BenchmarkPackageInput
type BenchmarkRelease = service.BenchmarkRelease

type BenchmarkRegistryRepository struct{ db *sql.DB }

func NewBenchmarkRegistryRepository(db *sql.DB) *BenchmarkRegistryRepository {
	return &BenchmarkRegistryRepository{db: db}
}
func benchmarkDigest(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
func benchmarkLabel(s string, max int) bool {
	if s == "" || len(s) > max || strings.TrimSpace(s) != s {
		return false
	}
	for _, r := range s {
		if r < 32 || r == 127 {
			return false
		}
	}
	return true
}
func benchmarkJSONObject(raw []byte) bool {
	var value map[string]json.RawMessage
	return len(raw) > 0 && len(raw) <= 65536 && json.Unmarshal(raw, &value) == nil && value != nil
}

func (r *BenchmarkRegistryRepository) Stage(ctx context.Context, input BenchmarkPackageInput, actor int64) (string, error) {
	if actor <= 0 || !benchmarkLabel(input.ID, 200) || !benchmarkLabel(input.Version, 100) || !validMonitorConfigurationHash(input.SHA256) || !validMonitorConfigurationHash(input.ContentSHA256) || len(input.Payload) == 0 || len(input.Payload) > 33554432 || benchmarkDigest(input.Payload) != input.SHA256 || !benchmarkJSONObject(input.EngineLock) || (input.Mode != "gpt" && input.Mode != "claude" && input.Mode != "chat") {
		return "", ErrBenchmarkRelease
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `INSERT INTO monitor_benchmark_packages(benchmark_id,benchmark_version,sha256,content_sha256,mode,payload) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(benchmark_id,benchmark_version) DO NOTHING`, input.ID, input.Version, input.SHA256, input.ContentSHA256, input.Mode, input.Payload)
	if err != nil {
		return "", err
	}
	var packageID int64
	var digest, content, mode string
	var raw []byte
	err = tx.QueryRowContext(ctx, `SELECT id,sha256,content_sha256,mode,payload FROM monitor_benchmark_packages WHERE benchmark_id=$1 AND benchmark_version=$2 FOR UPDATE`, input.ID, input.Version).Scan(&packageID, &digest, &content, &mode, &raw)
	if err != nil {
		return "", err
	}
	if digest != input.SHA256 || content != input.ContentSHA256 || mode != input.Mode || !bytes.Equal(raw, input.Payload) {
		return "", ErrBenchmarkRelease
	}
	engineHash := benchmarkDigest(input.EngineLock)
	id := uuid.NewString()
	result, err := tx.ExecContext(ctx, `INSERT INTO monitor_benchmark_releases(id,package_id,engine_lock_sha256,engine_lock) VALUES($1,$2,$3,$4) ON CONFLICT(package_id,engine_lock_sha256) DO NOTHING`, id, packageID, engineHash, []byte(input.EngineLock))
	if err != nil {
		return "", err
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return "", err
	}
	if inserted == 0 {
		err = tx.QueryRowContext(ctx, `SELECT id FROM monitor_benchmark_releases WHERE package_id=$1 AND engine_lock_sha256=$2`, packageID, engineHash).Scan(&id)
	} else {
		_, err = tx.ExecContext(ctx, `INSERT INTO monitor_benchmark_audit(actor_id,operation,release_id) VALUES($1,'stage',$2)`, actor, id)
	}
	if err != nil {
		return "", err
	}
	return id, tx.Commit()
}

func (r *BenchmarkRegistryRepository) Get(ctx context.Context, id string) (*BenchmarkRelease, error) {
	if !validDetectorResourceID(id) {
		return nil, ErrBenchmarkRelease
	}
	var b BenchmarkRelease
	var receipt, engine []byte
	err := r.db.QueryRowContext(ctx, `SELECT r.id,p.benchmark_id,p.benchmark_version,p.sha256,p.content_sha256,p.mode,r.engine_lock_sha256,r.state,p.payload,r.engine_lock,r.validation_receipt FROM monitor_benchmark_releases r JOIN monitor_benchmark_packages p ON p.id=r.package_id WHERE r.id=$1`, id).Scan(&b.ID, &b.BenchmarkID, &b.Version, &b.SHA256, &b.ContentSHA256, &b.Mode, &b.EngineLockSHA256, &b.State, &b.Payload, &engine, &receipt)
	if err != nil {
		return nil, err
	}
	b.EngineLock, b.Receipt = engine, receipt
	if benchmarkDigest(b.Payload) != b.SHA256 || benchmarkDigest(engine) != b.EngineLockSHA256 {
		return nil, ErrBenchmarkRelease
	}
	return &b, nil
}

// ApproveValidated persists only a trusted offline validator's receipt. This is
// an internal repository operation, not an endpoint accepting client receipts.
func (r *BenchmarkRegistryRepository) ApproveValidated(ctx context.Context, id, packageHash, engineHash string, receipt json.RawMessage, actor int64) error {
	if actor <= 0 || !validDetectorResourceID(id) || !benchmarkJSONObject(receipt) {
		return ErrBenchmarkRelease
	}
	var binding struct {
		BenchmarkSHA256  string `json:"benchmark_sha256"`
		EngineLockSHA256 string `json:"engine_lock_sha256"`
		AdapterSHA256    string `json:"adapter_sha256"`
		CalibrationValid bool   `json:"calibration_valid"`
	}
	if json.Unmarshal(receipt, &binding) != nil || binding.BenchmarkSHA256 != packageHash || binding.EngineLockSHA256 != engineHash || !validMonitorConfigurationHash(binding.AdapterSHA256) || !binding.CalibrationValid {
		return ErrBenchmarkRelease
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var state string
	err = tx.QueryRowContext(ctx, `SELECT r.state FROM monitor_benchmark_releases r JOIN monitor_benchmark_packages p ON p.id=r.package_id WHERE r.id=$1 AND p.sha256=$2 AND r.engine_lock_sha256=$3 FOR UPDATE OF r`, id, packageHash, engineHash).Scan(&state)
	if err != nil {
		return err
	}
	if state == "withdrawn" {
		return ErrBenchmarkRelease
	}
	if state == "approved" {
		return tx.Commit()
	}
	_, err = tx.ExecContext(ctx, `UPDATE monitor_benchmark_releases SET state='approved',validation_receipt=$2 WHERE id=$1`, id, []byte(receipt))
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO monitor_benchmark_audit(actor_id,operation,release_id) VALUES($1,'approve',$2)`, actor, id)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (r *BenchmarkRegistryRepository) Activate(ctx context.Context, channel, id string, expected, actor int64) (int64, error) {
	if !benchmarkLabel(channel, 200) || !validDetectorResourceID(id) || expected < 0 || expected == 9223372036854775807 || actor <= 0 {
		return 0, ErrBenchmarkRelease
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	var state string
	err = tx.QueryRowContext(ctx, `SELECT state FROM monitor_benchmark_releases WHERE id=$1 FOR SHARE`, id).Scan(&state)
	if err != nil {
		return 0, err
	}
	if state != "approved" {
		return 0, ErrBenchmarkRelease
	}
	var revision int64
	if expected == 0 {
		err = tx.QueryRowContext(ctx, `INSERT INTO monitor_benchmark_channels(name,release_id,revision) VALUES($1,$2,1) ON CONFLICT(name) DO NOTHING RETURNING revision`, channel, id).Scan(&revision)
	} else {
		err = tx.QueryRowContext(ctx, `UPDATE monitor_benchmark_channels SET release_id=$2,revision=revision+1,updated_at=clock_timestamp() WHERE name=$1 AND revision=$3 RETURNING revision`, channel, id, expected).Scan(&revision)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrBenchmarkRelease
	}
	if err != nil {
		return 0, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO monitor_benchmark_audit(actor_id,operation,release_id,channel,revision) VALUES($1,'activate',$2,$3,$4)`, actor, id, channel, revision)
	if err != nil {
		return 0, err
	}
	// Channel revision is frozen separately from policy execution configuration.
	// Queued jobs fail their channel guard; running jobs retain the frozen release.
	_, err = tx.ExecContext(ctx, `UPDATE channel_monitor_group_policies p SET version=version+1,updated_by=$2,updated_at=clock_timestamp(),next_capability_at=CASE WHEN p.enabled AND p.capability_config @> '{"enabled":true}' AND p.active_capability_job_id IS NULL THEN clock_timestamp() ELSE p.next_capability_at END WHERE p.deleted_at IS NULL AND EXISTS(SELECT 1 FROM jsonb_array_elements(COALESCE(p.capability_config->'targets','[]'::jsonb)) t WHERE t->>'benchmark_channel'=$1)`, channel, actor)
	if err != nil {
		return 0, err
	}
	return revision, tx.Commit()
}

// Freeze returns immutable identity plus the current channel revision. It is not
// a send permit: creation and every physical dispatch must recheck release state.
func (r *BenchmarkRegistryRepository) Freeze(ctx context.Context, channel string) (string, int64, error) {
	if !benchmarkLabel(channel, 200) {
		return "", 0, ErrBenchmarkRelease
	}
	var id string
	var revision int64
	err := r.db.QueryRowContext(ctx, `SELECT c.release_id,c.revision FROM monitor_benchmark_channels c JOIN monitor_benchmark_releases r ON r.id=c.release_id WHERE c.name=$1 AND r.state='approved'`, channel).Scan(&id, &revision)
	return id, revision, err
}
func (r *BenchmarkRegistryRepository) Withdraw(ctx context.Context, id, reason string, actor int64) error {
	if !validDetectorResourceID(id) || !benchmarkLabel(reason, 500) || actor <= 0 {
		return ErrBenchmarkRelease
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var state string
	err = tx.QueryRowContext(ctx, `SELECT state FROM monitor_benchmark_releases WHERE id=$1 FOR UPDATE`, id).Scan(&state)
	if err != nil {
		return err
	}
	if state == "withdrawn" {
		return tx.Commit()
	}
	_, err = tx.ExecContext(ctx, `UPDATE monitor_benchmark_releases SET state='withdrawn' WHERE id=$1`, id)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO monitor_benchmark_audit(actor_id,operation,release_id,reason) VALUES($1,'withdraw',$2,$3)`, actor, id, reason)
	if err != nil {
		return err
	}
	// Keep channel revisions as tombstones, avoiding CAS ABA and implicit fallback.
	return tx.Commit()
}
