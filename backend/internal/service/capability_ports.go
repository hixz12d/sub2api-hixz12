package service

import (
	"context"
	"time"
)

type CapabilityJob struct {
	ID       string
	OwnerID  *int64
	Snapshot MonitorJobSnapshot
	Deadline time.Time
}
type CapabilityExecutionInput struct {
	Lease              MonitorJobLease
	TargetIndex        int
	AccountID          *int64
	CredentialRevision string
	Plan               CapabilityPlan
}
type CapabilityDispatch struct {
	Lease            MonitorJobLease
	ExecutionID      string
	CellID           string
	SampleIndex      int
	RequestSHA256    string
	CampaignID       string
	UpperBoundMicros int64
	Limits           []MonitorBudgetLimit
}
type CapabilityOutcome struct {
	AttemptID     string
	HTTPStatus    *int
	ErrorCategory string
	Answer        *string
	Usage         *DetectorUsageSummary
	ElapsedMs     int64
}
type CapabilityStore interface {
	GetCapabilityJob(context.Context, MonitorJobLease) (*CapabilityJob, error)
	BeginCapabilityExecution(context.Context, CapabilityExecutionInput) (string, error)
	BeginCapabilityDispatch(context.Context, CapabilityDispatch) (string, error)
	CompleteCapabilityDispatch(context.Context, MonitorJobLease, CapabilityOutcome) error
	SaveCapabilityReport(context.Context, MonitorJobLease, string, DetectorReportEvidence) error
}
