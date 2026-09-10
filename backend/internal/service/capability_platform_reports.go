package service

import (
	"context"
	"time"
)

type PlatformCapabilityReport struct {
	JobID        string                 `json:"job_id"`
	AccountID    int64                  `json:"account_id"`
	RequestModel string                 `json:"request_model"`
	Evidence     DetectorReportEvidence `json:"evidence"`
	CreatedAt    time.Time              `json:"created_at"`
}
type PlatformCapabilityReports interface {
	PolicyReports(context.Context, int64, int, int) ([]PlatformCapabilityReport, error)
}
