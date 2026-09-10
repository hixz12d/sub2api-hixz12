package repository

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

var _ service.BenchmarkRegistryStore = (*BenchmarkRegistryRepository)(nil)

func (r *BenchmarkRegistryRepository) List(ctx context.Context, limit, offset int) ([]service.BenchmarkReleaseSummary, error) {
	if limit < 1 || limit > 100 || offset < 0 || offset > 100000 {
		return nil, ErrBenchmarkRelease
	}
	rows, err := r.db.QueryContext(ctx, `SELECT r.id,p.benchmark_id,p.benchmark_version,p.sha256,p.mode,r.engine_lock_sha256,r.state
 FROM monitor_benchmark_releases r JOIN monitor_benchmark_packages p ON p.id=r.package_id
 ORDER BY r.created_at DESC,r.id DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]service.BenchmarkReleaseSummary, 0)
	for rows.Next() {
		var b service.BenchmarkReleaseSummary
		if err := rows.Scan(&b.ID, &b.BenchmarkID, &b.Version, &b.SHA256, &b.Mode, &b.EngineLockSHA256, &b.State); err != nil {
			return nil, err
		}
		result = append(result, b)
	}
	return result, rows.Err()
}

func (r *BenchmarkRegistryRepository) Channels(ctx context.Context) ([]service.BenchmarkChannel, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT c.name,c.release_id,c.revision,r.state FROM monitor_benchmark_channels c
 JOIN monitor_benchmark_releases r ON r.id=c.release_id ORDER BY c.name LIMIT 101`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]service.BenchmarkChannel, 0)
	for rows.Next() {
		var c service.BenchmarkChannel
		if err := rows.Scan(&c.Name, &c.ReleaseID, &c.Revision, &c.State); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	if len(result) > 100 {
		return nil, ErrBenchmarkRelease
	}
	return result, rows.Err()
}
