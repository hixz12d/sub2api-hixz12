package service

import "time"

type MonitorJobKind string
type MonitorJobSource string
type MonitorJobState string
type DetectorVerdict string
type DetectorContractStatus string
type MonitorDispatchState string

const (
	MonitorJobAvailability      MonitorJobKind         = "availability"
	MonitorJobCapability        MonitorJobKind         = "capability"
	MonitorSourcePlatformGroup  MonitorJobSource       = "platform_group"
	MonitorSourceExternalAPI    MonitorJobSource       = "external_api"
	MonitorSourceSiteAPIKey     MonitorJobSource       = "site_api_key"
	MonitorJobQueued            MonitorJobState        = "queued"
	MonitorJobRunning           MonitorJobState        = "running"
	MonitorJobCancelling        MonitorJobState        = "cancelling"
	MonitorJobCompleted         MonitorJobState        = "completed"
	MonitorJobFailed            MonitorJobState        = "failed"
	MonitorJobCancelled         MonitorJobState        = "cancelled"
	MonitorJobInterrupted       MonitorJobState        = "interrupted"
	MonitorJobSkipped           MonitorJobState        = "skipped"
	DetectorMatch               DetectorVerdict        = "match"
	DetectorMismatch            DetectorVerdict        = "mismatch"
	DetectorInsufficient        DetectorVerdict        = "insufficient"
	DetectorNotEvaluated        DetectorVerdict        = "not_evaluated"
	DetectorContractExact       DetectorContractStatus = "exact"
	DetectorContractMutated     DetectorContractStatus = "mutated"
	DetectorContractUnsupported DetectorContractStatus = "unsupported"
	DetectorContractUnknown     DetectorContractStatus = "unknown"
	MonitorDispatchReserved     MonitorDispatchState   = "reserved"
	MonitorDispatchSending      MonitorDispatchState   = "dispatching"
	MonitorDispatchCompleted    MonitorDispatchState   = "completed"
	MonitorDispatchUncertain    MonitorDispatchState   = "uncertain"
	MonitorDispatchCancelled    MonitorDispatchState   = "cancelled_before_dispatch"
)

func (s MonitorJobState) Terminal() bool {
	switch s {
	case MonitorJobCompleted, MonitorJobFailed, MonitorJobCancelled, MonitorJobInterrupted, MonitorJobSkipped:
		return true
	default:
		return false
	}
}

// No credential, arbitrary header or arbitrary request body belongs in a snapshot.
type DetectorTargetSpec struct {
	AccountID    *int64           `json:"account_id,omitempty"`
	Source       MonitorJobSource `json:"source"`
	BaseURL      string           `json:"base_url,omitempty"`
	SiteAPIKeyID *int64           `json:"site_api_key_id,omitempty"`
	GroupID      *int64           `json:"group_id,omitempty"`
	Target       DetectorTarget   `json:"target"`
	Tier         string           `json:"tier"`
}

type DetectorBenchmarkManifest struct {
	Channel             string `json:"channel,omitempty"`
	ChannelRevision     int64  `json:"channel_revision,omitempty"`
	ReleaseID           string `json:"release_id,omitempty"`
	EngineLockSHA256    string `json:"engine_lock_sha256,omitempty"`
	ID                  string `json:"id"`
	Version             string `json:"version"`
	SHA256              string `json:"sha256"`
	EngineCommit        string `json:"engine_commit"`
	EngineVersion       string `json:"engine_version"`
	ScoringVersion      string `json:"scoring_version"`
	SamplePolicyVersion string `json:"sample_policy_version"`
	RequestContractHash string `json:"request_contract_hash"`
}

type DetectorPlan struct {
	ID                      string
	OwnerUserID             *int64
	Source                  MonitorJobSource
	PolicyID                *int64
	EvaluationRevision      *string
	TargetSpec              DetectorTargetSpec
	Benchmark               DetectorBenchmarkManifest
	ConfigurationHash       string
	PlannedBaseRequests     int64
	RetryBudgetRequests     int64
	MaximumOutboundRequests int64
	EstimatedTokens         *int64
	EstimatedCost           *string
	EstimateCurrency        *string
	EstimateStatus          string
	ExpiresAt               time.Time
	ConsumedJobID           *string // Derived from monitor_jobs.plan_id, not a second writable link.
	CreatedAt               time.Time
}

type MonitorJobSnapshot struct {
	RetestOf         string                      `json:"retest_of,omitempty"`
	PolicyVersion    int64                       `json:"policy_version,omitempty"`
	Targets          []DetectorTargetSpec        `json:"targets"`
	Benchmarks       []DetectorBenchmarkManifest `json:"benchmarks"`
	ProbeConfig      *GroupProbeConfig           `json:"probe_config,omitempty"`
	CapabilityConfig *GroupCapabilityConfig      `json:"capability_config,omitempty"`
}

// Persistence model only: user responses must be explicit, owner-scoped projections.
type MonitorJob struct {
	ID                    string
	Kind                  MonitorJobKind
	Source                MonitorJobSource
	PolicyID              *int64
	EvaluationRevision    *string
	PlanID                *string
	OwnerUserID           *int64
	IdempotencyScope      string
	IdempotencyKeyHash    string
	PayloadHash           string
	ScheduleOccurrenceID  *string
	State                 MonitorJobState
	ConfigSnapshot        MonitorJobSnapshot
	CreatedAt             time.Time
	StartedAt             *time.Time
	FinishedAt            *time.Time
	DeadlineAt            time.Time
	QueueDeadlineAt       time.Time
	LeaseOwner            *string
	LeaseExpiresAt        *time.Time
	LeaseGeneration       int64
	SecretOwnerInstanceID *string
	CancelRequestedAt     *time.Time
	FailureCode           *string
	BaseRequestsPlanned   int64
	OutboundReserved      int64
	OutboundDispatched    int64
	OutboundCompleted     int64
}

type DetectorSelectionSnapshot struct {
	AccountIDs                []int64 `json:"account_ids"`
	CredentialSubjectRevision string  `json:"credential_subject_revision"`
	SampleSize                int     `json:"sample_size"`
	Scope                     string  `json:"scope"`
}

type DetectorExecution struct {
	ID                    string
	JobID                 string
	Source                MonitorJobSource
	TargetIndex           int
	GroupID               *int64
	EffectiveGroupID      *int64
	AccountID             *int64
	CredentialRevision    *string
	SiteAPIKeyID          *int64
	RequestModel          string
	ResolvedUpstreamModel *string
	ClaimedModel          string
	Benchmark             DetectorBenchmarkManifest
	EffectiveContractHash *string
	ContractStatus        DetectorContractStatus
	Tier                  string
	BaseRequestCount      int64
	RetryBudget           int64
	SelectionMode         *MonitorSelectionMode
	SelectionSnapshot     DetectorSelectionSnapshot
	State                 MonitorJobState
	StartedAt             *time.Time
	FinishedAt            *time.Time
	ExpiresAt             *time.Time
	Verdict               DetectorVerdict
	FailureCode           *string
	ValidSamples          int64
	PlannedSamples        int64
}

type DetectorUsageSummary struct {
	InputTokens     *int64 `json:"input_tokens"`
	OutputTokens    *int64 `json:"output_tokens"`
	CacheReadTokens *int64 `json:"cache_read_tokens"`
}

type DetectorAttempt struct {
	ID                 string
	ExecutionID        string
	CellID             string
	SampleIndex        int
	AttemptIndex       int
	OutboundRequestID  string
	LeaseGeneration    int64
	DispatchState      MonitorDispatchState
	HTTPStatus         *int
	ErrorCategory      *string
	StartedAt          *time.Time
	CompletedAt        *time.Time
	NormalizedAnswer   *string
	AnswerCategory     *string
	UsageSummary       *DetectorUsageSummary
	ElapsedMs          *int64
	EffectiveAccountID *int64
	ContractStatus     DetectorContractStatus
}

type DetectorCellReport struct {
	CellID    string `json:"cell_id"`
	Planned   int64  `json:"planned"`
	Minimum   int64  `json:"minimum"`
	Valid     int64  `json:"valid"`
	Completed int64  `json:"completed"`
}

type DetectorReportEvidence struct {
	Verdict        DetectorVerdict           `json:"verdict"`
	Winner         *string                   `json:"winner"`
	Reasons        []string                  `json:"reasons"`
	Cells          []DetectorCellReport      `json:"cells"`
	Matches        map[string]float64        `json:"matches"`
	Scores         map[string]float64        `json:"scores"`
	Thresholds     map[string]float64        `json:"thresholds"`
	Benchmark      DetectorBenchmarkManifest `json:"benchmark"`
	ContractStatus DetectorContractStatus    `json:"contract_status"`
	TransportMode  string                    `json:"transport_mode"`
}

type DetectorReport struct {
	ID          string
	ExecutionID string
	Version     int
	Evidence    DetectorReportEvidence
	CreatedAt   time.Time
	DeletedAt   *time.Time
}

type ChannelMonitorProbeResult struct {
	ID             string
	JobID          string
	PolicyID       int64
	GroupID        int64
	RequestModel   string
	AccountID      *int64
	CheckedAt      time.Time
	TransportState string
	ChallengeState string
	TTFTMs         *int64
	ErrorCategory  *string
	Source         string
}

type MonitorBudgetBucket struct {
	Scope        string
	ScopeID      int64
	UTCDay       time.Time
	RequestLimit int64
	Reserved     int64
	Consumed     int64
	UpdatedAt    time.Time
}

type MonitorBudgetReservation struct {
	ID        string
	JobID     string
	Scope     string
	ScopeID   int64
	UTCDay    time.Time
	Reserved  int64
	Consumed  int64
	Released  int64
	CreatedAt time.Time
	SettledAt *time.Time
}
