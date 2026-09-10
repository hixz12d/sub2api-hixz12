package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	ErrMonitorPolicyNotFound = errors.New("monitor policy not found")
	ErrMonitorPolicyConflict = errors.New("monitor policy version conflict")
	ErrMonitorPolicyExists   = errors.New("monitor policy already exists for group")
)

type MonitorSelectionMode string

const (
	MonitorSelectionRandom MonitorSelectionMode = "random"
	MonitorSelectionFixed  MonitorSelectionMode = "fixed"
)

type GroupProbeConfig struct {
	Enabled            bool                 `json:"enabled"`
	IntervalSeconds    int                  `json:"interval_seconds"`
	JitterSeconds      int                  `json:"jitter_seconds"`
	TimeoutSeconds     int                  `json:"timeout_seconds"`
	SelectionMode      MonitorSelectionMode `json:"selection_mode"`
	FixedAccountIDs    []int64              `json:"fixed_account_ids"`
	SampleSize         int                  `json:"sample_size"`
	IncludeExtraModels bool                 `json:"include_extra_models"`
	DailyRequestLimit  int                  `json:"daily_request_limit"`
}

type DetectorTarget struct {
	BenchmarkChannel string `json:"benchmark_channel,omitempty"`
	RequestModel     string `json:"request_model"`
	ClaimedModel     string `json:"claimed_model"`
	BenchmarkID      string `json:"benchmark_id"`
	BenchmarkVersion string `json:"benchmark_version"`
	BenchmarkSHA256  string `json:"benchmark_sha256"`
}

type GroupCapabilityConfig struct {
	Enabled                 bool                 `json:"enabled"`
	IntervalSeconds         int                  `json:"interval_seconds"`
	JitterSeconds           int                  `json:"jitter_seconds"`
	SelectionMode           MonitorSelectionMode `json:"selection_mode"`
	FixedAccountIDs         []int64              `json:"fixed_account_ids"`
	SampleSize              int                  `json:"sample_size"`
	Tier                    string               `json:"tier"`
	ExecutionTimeoutSeconds int                  `json:"execution_timeout_seconds"`
	ResultTTLSeconds        int                  `json:"result_ttl_seconds"`
	DailyRequestLimit       int                  `json:"daily_request_limit"`
	RetestOnMismatch        bool                 `json:"retest_on_mismatch"`
	RetestDelaySeconds      int                  `json:"retest_delay_seconds"`
	RetestLimitPerExecution int                  `json:"retest_limit_per_execution"`
	Targets                 []DetectorTarget     `json:"targets"`
}

// Internal/admin model. Never serialize this type as a public group card.
type ChannelMonitorGroupPolicy struct {
	ID                    int64                 `json:"id"`
	GroupID               int64                 `json:"group_id"`
	DisplayName           string                `json:"display_name"`
	PrimaryModel          string                `json:"primary_model"`
	ExtraModels           []string              `json:"extra_models"`
	Enabled               bool                  `json:"enabled"`
	ProbeConfig           GroupProbeConfig      `json:"probe_config"`
	CapabilityConfig      GroupCapabilityConfig `json:"capability_config"`
	Version               int64                 `json:"version"`
	EvaluationRevision    string                `json:"evaluation_revision"`
	NextProbeAt           *time.Time            `json:"next_probe_at"`
	NextCapabilityAt      *time.Time            `json:"next_capability_at"`
	ActiveProbeJobID      *string               `json:"-"`
	ActiveCapabilityJobID *string               `json:"-"`
	CreatedBy             int64                 `json:"created_by"`
	UpdatedBy             int64                 `json:"updated_by"`
	CreatedAt             time.Time             `json:"created_at"`
	UpdatedAt             time.Time             `json:"updated_at"`
	DeletedAt             *time.Time            `json:"-"`
}

func DefaultGroupProbeConfig() GroupProbeConfig {
	return GroupProbeConfig{IntervalSeconds: 60, JitterSeconds: 15, TimeoutSeconds: 45,
		SelectionMode: MonitorSelectionRandom, FixedAccountIDs: []int64{}, SampleSize: 1, DailyRequestLimit: 2000}
}

func DefaultGroupCapabilityConfig() GroupCapabilityConfig {
	return GroupCapabilityConfig{IntervalSeconds: 43200, JitterSeconds: 6480,
		SelectionMode: MonitorSelectionRandom, FixedAccountIDs: []int64{}, SampleSize: 1,
		Tier: "low", ExecutionTimeoutSeconds: 1800, ResultTTLSeconds: 86400,
		DailyRequestLimit: 1000, RetestOnMismatch: true, RetestDelaySeconds: 1800,
		RetestLimitPerExecution: 1, Targets: []DetectorTarget{}}
}

// DecodeMonitorConfig rejects unknown fields, fractional integers and trailing JSON.
// A caller may initialize dst with defaults before decoding an older stored object.
func DecodeMonitorConfig(raw []byte, dst any) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || len(raw) > 64*1024 || trimmed[0] != '{' || !utf8.Valid(raw) {
		return errors.New("monitor config must be a bounded JSON object")
	}
	shape := json.NewDecoder(bytes.NewReader(raw))
	if err := validateMonitorJSONValue(shape, 0); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("trailing monitor config data")
	}
	return nil
}

// Reject nulls and duplicate keys before decoding into default-initialized structs.
func validateMonitorJSONValue(decoder *json.Decoder, depth int) error {
	if depth > 32 {
		return errors.New("monitor config nesting exceeds limit")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if token == nil {
		return errors.New("monitor config fields cannot be null")
	}
	switch token {
	case json.Delim('{'):
		seen := map[string]bool{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return errors.New("duplicate or invalid monitor config key")
			}
			seen[name] = true
			// encoding/json otherwise accepts case-folded aliases of tagged fields.
			for _, ch := range name {
				if !(ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || ch == '_') {
					return errors.New("monitor config keys must use lowercase ASCII names")
				}
			}
			if err := validateMonitorJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
	case json.Delim('['):
		for decoder.More() {
			if err := validateMonitorJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
	}
	return err
}

func validateMonitorSelection(mode MonitorSelectionMode, ids []int64, size int) error {
	if size < 1 || size > 3 {
		return errors.New("sample_size must be between 1 and 3")
	}
	switch mode {
	case MonitorSelectionRandom:
		if len(ids) != 0 {
			return errors.New("random selection cannot contain fixed accounts")
		}
	case MonitorSelectionFixed:
		if len(ids) != size {
			return errors.New("fixed sample_size must equal account count")
		}
		seen := make(map[int64]bool, len(ids))
		for _, id := range ids {
			if id <= 0 || seen[id] {
				return errors.New("fixed accounts must be positive and unique")
			}
			seen[id] = true
		}
	default:
		return errors.New("invalid selection_mode")
	}
	return nil
}

func (c GroupProbeConfig) Validate() error {
	if c.IntervalSeconds < 15 || c.IntervalSeconds > 9600 || c.JitterSeconds < 0 || c.JitterSeconds > c.IntervalSeconds-15 {
		return errors.New("invalid probe interval or jitter")
	}
	if c.TimeoutSeconds < 1 || c.TimeoutSeconds > 300 || c.DailyRequestLimit < 1 || c.DailyRequestLimit > 100000 {
		return errors.New("invalid probe timeout or request limit")
	}
	return validateMonitorSelection(c.SelectionMode, c.FixedAccountIDs, c.SampleSize)
}

func (c GroupCapabilityConfig) Validate(models []string) error {
	if c.IntervalSeconds < 3600 || c.IntervalSeconds > 604800 || c.JitterSeconds < 0 || c.JitterSeconds > c.IntervalSeconds-3600 {
		return errors.New("invalid capability interval or jitter")
	}
	if c.ExecutionTimeoutSeconds < 60 || c.ExecutionTimeoutSeconds > 3600 ||
		c.ResultTTLSeconds < c.IntervalSeconds+c.JitterSeconds+c.ExecutionTimeoutSeconds || c.ResultTTLSeconds > 14*86400 {
		return errors.New("invalid capability timeout or result TTL")
	}
	if c.DailyRequestLimit < 1 || c.DailyRequestLimit > 100000 ||
		c.RetestDelaySeconds < 60 || c.RetestDelaySeconds > 604800 || c.RetestLimitPerExecution < 0 || c.RetestLimitPerExecution > 1 {
		return errors.New("invalid capability budget or retest settings")
	}
	if c.Tier != "low" && c.Tier != "medium" && c.Tier != "high" {
		return errors.New("invalid detection tier")
	}
	if err := validateMonitorSelection(c.SelectionMode, c.FixedAccountIDs, c.SampleSize); err != nil {
		return err
	}
	if len(c.Targets) > 8 || (c.Enabled && len(c.Targets) == 0) {
		return errors.New("capability requires 1 to 8 targets when enabled")
	}
	allowed := make(map[string]bool, len(models))
	for _, model := range models {
		allowed[model] = true
	}
	seen := make(map[string]bool, len(c.Targets))
	for _, target := range c.Targets {
		if target.BenchmarkChannel != "" && !validMonitorIdentifier(target.BenchmarkChannel, 200) {
			return errors.New("invalid benchmark channel")
		}
		if !allowed[target.RequestModel] || seen[target.RequestModel] {
			return errors.New("target must be a unique configured model")
		}
		seen[target.RequestModel] = true
		if target.ClaimedModel == "reference-only:other" || !validMonitorIdentifier(target.ClaimedModel, 200) ||
			!validMonitorIdentifier(target.RequestModel, 200) || !validMonitorIdentifier(target.BenchmarkID, 200) ||
			!validMonitorIdentifier(target.BenchmarkVersion, 100) {
			return errors.New("invalid benchmark reference")
		}
		if !validMonitorHash(target.BenchmarkSHA256) {
			return errors.New("benchmark_sha256 must be a lowercase SHA256 digest")
		}
	}
	return nil
}

// Byte limits are intentionally conservative relative to PostgreSQL VARCHAR.
func validMonitorIdentifier(value string, maxBytes int) bool {
	return value != "" && len(value) <= maxBytes && utf8.ValidString(value) &&
		strings.TrimSpace(value) == value && strings.IndexFunc(value, unicode.IsControl) < 0
}

func validMonitorHash(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

// Normalize validates structural constraints only. Group/account membership and
// installed benchmark admission must be checked by the service before activation.
func (p *ChannelMonitorGroupPolicy) Normalize() error {
	if p.GroupID <= 0 {
		return errors.New("group_id must be positive")
	}
	p.DisplayName = strings.TrimSpace(p.DisplayName)
	p.PrimaryModel = strings.TrimSpace(p.PrimaryModel)
	if len(p.DisplayName) > 200 || !utf8.ValidString(p.DisplayName) || strings.IndexFunc(p.DisplayName, unicode.IsControl) >= 0 || !validMonitorIdentifier(p.PrimaryModel, 200) {
		return errors.New("invalid monitor name or primary model")
	}
	seen := map[string]bool{p.PrimaryModel: true}
	extra := make([]string, 0, len(p.ExtraModels))
	for _, raw := range p.ExtraModels {
		model := strings.TrimSpace(raw)
		if !validMonitorIdentifier(model, 200) {
			return errors.New("invalid extra model")
		}
		if !seen[model] {
			extra = append(extra, model)
			seen[model] = true
		}
	}
	if len(extra) > 19 {
		return errors.New("at most 20 monitored models are allowed")
	}
	p.ExtraModels = extra
	if p.ProbeConfig.FixedAccountIDs == nil {
		p.ProbeConfig.FixedAccountIDs = []int64{}
	}
	if p.CapabilityConfig.FixedAccountIDs == nil {
		p.CapabilityConfig.FixedAccountIDs = []int64{}
	}
	if p.CapabilityConfig.Targets == nil {
		p.CapabilityConfig.Targets = []DetectorTarget{}
	}
	if err := p.ProbeConfig.Validate(); err != nil {
		return fmt.Errorf("probe_config: %w", err)
	}
	if err := p.CapabilityConfig.Validate(append([]string{p.PrimaryModel}, extra...)); err != nil {
		return fmt.Errorf("capability_config: %w", err)
	}
	return nil
}

// Policy revision excludes presentation, schedules and probe settings. Executions
// additionally freeze resolved model, benchmark, scoring and request contract hashes.
func (p ChannelMonitorGroupPolicy) CapabilityRevision() (string, error) {
	cfg := p.CapabilityConfig
	cfg.FixedAccountIDs = append([]int64(nil), cfg.FixedAccountIDs...)
	cfg.Targets = append([]DetectorTarget(nil), cfg.Targets...)
	sort.Slice(cfg.FixedAccountIDs, func(i, j int) bool { return cfg.FixedAccountIDs[i] < cfg.FixedAccountIDs[j] })
	sort.Slice(cfg.Targets, func(i, j int) bool { return cfg.Targets[i].RequestModel < cfg.Targets[j].RequestModel })
	raw, err := json.Marshal(struct {
		Version int                   `json:"version"`
		GroupID int64                 `json:"group_id"`
		Config  GroupCapabilityConfig `json:"config"`
	}{Version: 1, GroupID: p.GroupID, Config: cfg})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
