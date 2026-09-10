package repository

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// Changing only the policy avoids reversing the job->policy lock order used
// during settlement. Dispatch/renew reject disabled policies; recovery settles
// each affected job independently and preserves uncertain paid attempts.
func (r *ChannelMonitorGroupRepository) DisablePolicy(ctx context.Context, id, revision, actor int64) error {
	if id <= 0 || revision <= 0 || actor <= 0 {
		return service.ErrMonitorPolicyConflict
	}
	result, err := r.db.ExecContext(ctx, `UPDATE channel_monitor_group_policies SET enabled=false,probe_config=jsonb_set(probe_config,'{enabled}','false'),capability_config=jsonb_set(capability_config,'{enabled}','false'),next_probe_at=NULL,next_capability_at=NULL,updated_by=$3,updated_at=clock_timestamp(),version=version+1 WHERE id=$1 AND version=$2 AND deleted_at IS NULL`, id, revision, actor)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return service.ErrMonitorPolicyConflict
	}
	return nil
}
