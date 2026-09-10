package service

import (
	"context"
	"time"
)

// MonitorJobLease identifies the database fence, never a credential or API token.
type MonitorJobLease struct {
	JobID      string
	InstanceID string
	Generation int64
	ExpiresAt  time.Time
	Kind       MonitorJobKind
	Source     MonitorJobSource
}

// Limits come from trusted deployment/policy configuration, not request bodies.
type MonitorBudgetLimit struct {
	Scope        string
	ScopeID      int64
	RequestLimit int64
}

type MonitorJobStore interface {
	ListDueProbePolicyIDs(context.Context, int) ([]int64, error)
	EnqueueProbe(context.Context, int64, int64) (string, error)
	ClaimEligible(context.Context, MonitorJobKind, string, bool, bool) (*MonitorJobLease, error)
	CancelDisabledNext(context.Context, bool, bool, bool) (bool, error)
	Renew(context.Context, MonitorJobLease) (time.Time, error)
	Finish(context.Context, MonitorJobLease, MonitorJobState) error
	RecoverNext(context.Context) (bool, error)
}

type MonitorJobExecutor interface {
	Ready() bool
	// Execute must honor cancellation and persist fenced request/result evidence.
	// A nil error means execution ended, not that model capability matched.
	Execute(context.Context, MonitorJobLease) error
}
