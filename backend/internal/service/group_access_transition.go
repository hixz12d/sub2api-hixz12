package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var ErrGroupAccessTransitionConflict = infraerrors.Conflict("GROUP_ACCESS_TRANSITION_CONFLICT", "group changed; reload before preserving existing user access")

// GroupAccessTransitionRepository converts a public standard group to exclusive
// and grants its existing eligible users access in the same transaction.
// It preserves explicit restrictions and never changes API key bindings.
type GroupAccessTransitionRepository interface {
	UpdatePreservingUserAccess(ctx context.Context, group *Group, expectedUpdatedAt time.Time) error
}
