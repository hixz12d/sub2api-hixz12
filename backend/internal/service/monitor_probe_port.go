package service

import (
	"context"
	"time"
)

type MonitorProbeJob struct {
	ID       string
	PolicyID int64
	GroupID  int64
	Snapshot MonitorJobSnapshot
	Deadline time.Time
}

type MonitorProbeDispatch struct {
	Lease         MonitorJobLease
	TargetIndex   int
	SampleIndex   int
	AccountID     int64
	RequestModel  string
	RequestSHA256 string
}

type MonitorProbeOutcome struct {
	DispatchID     string
	TransportState string
	ChallengeState string
	ErrorCategory  string
	TTFTMs         *int64
}

type MonitorProbeStore interface {
	GetProbeJob(context.Context, MonitorJobLease) (*MonitorProbeJob, error)
	BeginProbeDispatch(context.Context, MonitorProbeDispatch, int64) (string, error)
	CompleteProbeDispatch(context.Context, MonitorJobLease, MonitorProbeOutcome) error
}
