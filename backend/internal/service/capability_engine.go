package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

type CapabilityCell struct {
	ID      string `json:"cell_id"`
	Count   int    `json:"count"`
	Payload []byte `json:"payload_base64"`
	SHA256  string `json:"payload_sha256"`
}
type CapabilityPlan struct {
	ContractHash    string           `json:"contract_hash"`
	BenchmarkSHA256 string           `json:"benchmark_sha256"`
	Mode            string           `json:"mode"`
	PlannedSamples  int              `json:"planned_samples"`
	Cells           []CapabilityCell `json:"cells"`
}
type CapabilitySample struct {
	CellID      string `json:"cell_id"`
	SampleIndex int    `json:"sample_index"`
	Answer      string `json:"answer"`
}
type CapabilityDecoded struct {
	Answer string               `json:"answer"`
	Usage  DetectorUsageSummary `json:"usage"`
}
type CapabilityEngine interface {
	Plan(context.Context, *BenchmarkRelease, DetectorTargetSpec) (*CapabilityPlan, error)
	Decode(context.Context, *BenchmarkRelease, []byte) (*CapabilityDecoded, error)
	Score(context.Context, *BenchmarkRelease, DetectorTargetSpec, string, DetectorContractStatus, []CapabilitySample) (*DetectorReportEvidence, error)
}

type PinnedCapabilityEngine struct{ validator *PinnedMeowValidator }

func NewPinnedCapabilityEngine(cfg *config.Config) *PinnedCapabilityEngine {
	v := &PinnedMeowValidator{slots: make(chan struct{}, 1)}
	if cfg != nil {
		v.python = cfg.LLMDetectorPython
		v.adapter = cfg.LLMDetectorAdapter
		v.engineRoot = cfg.LLMDetectorEngineRoot
		v.adapterSHA256 = cfg.LLMDetectorAdapterSHA256
		v.allowed = cfg.LLMDetectorEngineAllowed
	}
	return &PinnedCapabilityEngine{validator: v}
}
func capabilityEngineRequest(operation string, release *BenchmarkRelease, target DetectorTargetSpec) map[string]any {
	return map[string]any{"operation": operation, "benchmark_id": release.BenchmarkID, "benchmark_version": release.Version, "tier": target.Tier, "claimed_model": target.Target.ClaimedModel, "request_model": target.Target.RequestModel}
}
func (e *PinnedCapabilityEngine) invoke(ctx context.Context, release *BenchmarkRelease, request any) ([]byte, error) {
	if e == nil || e.validator == nil || release == nil || release.State != "approved" || len(release.Payload) == 0 || len(release.Payload) > 32*1024*1024 || benchmarkSHA256(release.Payload) != release.SHA256 || benchmarkSHA256(release.EngineLock) != release.EngineLockSHA256 {
		return nil, ErrBenchmarkValidator
	}
	v := e.validator
	lock, err := v.EngineLock(ctx)
	if err != nil || !bytes.Equal(lock, release.EngineLock) || v.slots == nil {
		return nil, ErrBenchmarkValidator
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	select {
	case v.slots <- struct{}{}:
		defer func() { <-v.slots }()
	case <-ctx.Done():
		return nil, ErrBenchmarkValidator
	}
	input, err := json.Marshal(map[string]any{"payload": release.Payload, "engine_lock": []byte(release.EngineLock), "manifest": map[string]string{"id": release.BenchmarkID, "version": release.Version, "sha256": release.SHA256, "content_sha256": release.ContentSHA256, "mode": release.Mode}, "request": request})
	if err != nil || len(input) > 48*1024*1024 {
		return nil, ErrBenchmarkValidator
	}
	cmd := exec.CommandContext(ctx, v.python, "-I", "-B", v.adapter, "--engine-root", v.engineRoot, "--admitted", "--execute-frozen")
	cmd.Env = []string{"PYTHONDONTWRITEBYTECODE=1", "PYTHONUTF8=1"}
	for _, key := range []string{"SystemRoot", "WINDIR", "TEMP", "TMP", "PATH"} {
		if value, ok := os.LookupEnv(key); ok {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
	}
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stderr = io.Discard
	cmd.WaitDelay = 2 * time.Second
	out := &benchmarkBoundedOutput{limit: 4 * 1024 * 1024}
	cmd.Stdout = out
	if cmd.Run() != nil || ctx.Err() != nil {
		return nil, ErrBenchmarkValidator
	}
	return bytes.Clone(out.Bytes()), nil
}
func (e *PinnedCapabilityEngine) Plan(ctx context.Context, release *BenchmarkRelease, target DetectorTargetSpec) (*CapabilityPlan, error) {
	if release == nil {
		return nil, ErrBenchmarkValidator
	}
	raw, err := e.invoke(ctx, release, capabilityEngineRequest("plan", release, target))
	if err != nil {
		return nil, err
	}
	var plan CapabilityPlan
	if json.Unmarshal(raw, &plan) != nil || plan.BenchmarkSHA256 != release.SHA256 || plan.Mode != release.Mode || len(plan.ContractHash) != 64 || plan.PlannedSamples < 1 || plan.PlannedSamples > 300 || len(plan.Cells) < 1 || len(plan.Cells) > 300 {
		return nil, ErrBenchmarkValidator
	}
	seen := map[string]bool{}
	total := 0
	for _, cell := range plan.Cells {
		if cell.ID == "" || len(cell.ID) > 200 || seen[cell.ID] || cell.Count < 1 || cell.Count > 300 || len(cell.Payload) == 0 || len(cell.Payload) > 65536 || !json.Valid(cell.Payload) || benchmarkSHA256(cell.Payload) != cell.SHA256 {
			return nil, ErrBenchmarkValidator
		}
		var payload struct {
			Model  string `json:"model"`
			Stream bool   `json:"stream"`
		}
		if json.Unmarshal(cell.Payload, &payload) != nil || payload.Model != target.Target.RequestModel || !payload.Stream {
			return nil, ErrBenchmarkValidator
		}
		seen[cell.ID] = true
		total += cell.Count
	}
	if total != plan.PlannedSamples {
		return nil, ErrBenchmarkValidator
	}
	return &plan, nil
}
func (e *PinnedCapabilityEngine) Decode(ctx context.Context, release *BenchmarkRelease, stream []byte) (*CapabilityDecoded, error) {
	if len(stream) == 0 || len(stream) > 1024*1024 || !utf8.Valid(stream) {
		return nil, ErrBenchmarkValidator
	}
	raw, err := e.invoke(ctx, release, map[string]any{"operation": "decode", "stream": stream})
	if err != nil {
		return nil, err
	}
	var result CapabilityDecoded
	if json.Unmarshal(raw, &result) != nil || len(result.Answer) > 8192 || !utf8.ValidString(result.Answer) {
		return nil, ErrBenchmarkValidator
	}
	return &result, nil
}
func (e *PinnedCapabilityEngine) Score(ctx context.Context, release *BenchmarkRelease, target DetectorTargetSpec, hash string, status DetectorContractStatus, samples []CapabilitySample) (*DetectorReportEvidence, error) {
	if release == nil || len(samples) > 300 {
		return nil, ErrBenchmarkValidator
	}
	if samples == nil {
		samples = []CapabilitySample{}
	}
	request := capabilityEngineRequest("score", release, target)
	request["contract_hash"] = hash
	request["contract_status"] = status
	request["samples"] = samples
	raw, err := e.invoke(ctx, release, request)
	if err != nil {
		return nil, err
	}
	var result DetectorReportEvidence
	if json.Unmarshal(raw, &result) != nil || result.ContractStatus != status {
		return nil, ErrBenchmarkValidator
	}
	switch result.Verdict {
	case DetectorMatch, DetectorMismatch:
		if status != DetectorContractExact {
			return nil, ErrBenchmarkValidator
		}
	case DetectorInsufficient, DetectorNotEvaluated:
	default:
		return nil, ErrBenchmarkValidator
	}
	result.TransportMode = release.Mode
	return &result, nil
}
