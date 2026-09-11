package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

type PinnedMeowValidator struct {
	python, adapter, engineRoot, adapterSHA256 string
	allowed                                    bool
	slots                                      chan struct{}
}

func ProvideBenchmarkService(cfg *config.Config, store BenchmarkRegistryStore, users UserRepository) *BenchmarkService {
	validator := &PinnedMeowValidator{slots: make(chan struct{}, 1)}
	if cfg != nil {
		validator.python = cfg.LLMDetectorPython
		validator.adapter = cfg.LLMDetectorAdapter
		validator.engineRoot = cfg.LLMDetectorEngineRoot
		validator.adapterSHA256 = cfg.LLMDetectorAdapterSHA256
		validator.allowed = cfg.LLMDetectorEngineAllowed
	}
	return NewBenchmarkService(store, validator, users)
}

func readBenchmarkFile(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, ErrBenchmarkValidator
	}
	defer func() { _ = file.Close() }()
	raw, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(raw)) > limit {
		return nil, ErrBenchmarkValidator
	}
	return raw, nil
}

func (v *PinnedMeowValidator) EngineLock(ctx context.Context) (json.RawMessage, error) {
	if ctx.Err() != nil || v == nil || !v.allowed || !filepath.IsAbs(v.python) || !filepath.IsAbs(v.adapter) || !filepath.IsAbs(v.engineRoot) || len(v.adapterSHA256) != 64 {
		return nil, ErrBenchmarkValidator
	}
	raw, err := readBenchmarkFile(v.adapter, 1024*1024)
	if err != nil || benchmarkSHA256(raw) != v.adapterSHA256 {
		return nil, ErrBenchmarkValidator
	}
	lock, err := readBenchmarkFile(filepath.Join(filepath.Dir(v.adapter), "meow.lock.json"), 65536)
	if err != nil {
		return nil, err
	}
	var document map[string]any
	if json.Unmarshal(lock, &document) != nil {
		return nil, ErrBenchmarkValidator
	}
	canonicalLock, err := json.Marshal(document)
	if err != nil || benchmarkSHA256(canonicalLock) != "d65a40e3f460e554f36c5a23be37d851772089e5739ca503fd917b53f267da6b" {
		return nil, ErrBenchmarkValidator
	}
	var metadata struct {
		Commit  string `json:"engine_commit"`
		Version string `json:"engine_version"`
	}
	if json.Unmarshal(lock, &metadata) != nil || metadata.Commit != "fdb89c99852e0d5558551168835387b835265942" || metadata.Version != "4.5.2" {
		return nil, ErrBenchmarkValidator
	}
	return lock, nil
}

type benchmarkBoundedOutput struct {
	buffer bytes.Buffer
	limit  int
}

func (b *benchmarkBoundedOutput) Len() int       { return b.buffer.Len() }
func (b *benchmarkBoundedOutput) Bytes() []byte  { return b.buffer.Bytes() }
func (b *benchmarkBoundedOutput) String() string { return b.buffer.String() }

func (b *benchmarkBoundedOutput) Write(raw []byte) (int, error) {
	limit := b.limit
	if limit == 0 {
		limit = 65536
	}
	if limit < 0 || limit > 4*1024*1024 || len(raw) > limit-b.Len() {
		return 0, ErrBenchmarkValidator
	}
	return b.buffer.Write(raw)
}

func (v *PinnedMeowValidator) Validate(ctx context.Context, release *BenchmarkRelease) (json.RawMessage, error) {
	if release == nil || len(release.Payload) == 0 || len(release.Payload) > 32*1024*1024 || benchmarkSHA256(release.Payload) != release.SHA256 || benchmarkSHA256(release.EngineLock) != release.EngineLockSHA256 {
		return nil, ErrBenchmarkValidator
	}
	lock, err := v.EngineLock(ctx)
	if err != nil || !bytes.Equal(lock, release.EngineLock) {
		return nil, ErrBenchmarkValidator
	}
	if v.slots == nil {
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
	input, err := json.Marshal(map[string]any{
		"payload": release.Payload, "engine_lock": []byte(release.EngineLock),
		"manifest": map[string]string{"id": release.BenchmarkID, "version": release.Version, "sha256": release.SHA256, "content_sha256": release.ContentSHA256, "mode": release.Mode},
	})
	if err != nil {
		return nil, ErrBenchmarkValidator
	}
	// Fixed executable/arguments only. Never inherit DB passwords, cloud keys or proxy credentials.
	cmd := exec.CommandContext(ctx, v.python, "-I", "-B", v.adapter, "--engine-root", v.engineRoot, "--admitted", "--validate-candidate")
	cmd.Env = []string{"PYTHONDONTWRITEBYTECODE=1", "PYTHONUTF8=1"}
	for _, key := range []string{"SystemRoot", "WINDIR", "TEMP", "TMP", "PATH"} {
		if value, ok := os.LookupEnv(key); ok {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
	}
	cmd.WaitDelay = 2 * time.Second
	cmd.Stdin = bytes.NewReader(input)
	var output benchmarkBoundedOutput
	cmd.Stdout = &output
	cmd.Stderr = io.Discard
	if cmd.Run() != nil || ctx.Err() != nil {
		return nil, ErrBenchmarkValidator
	}
	var receipt struct {
		BenchmarkSHA256  string `json:"benchmark_sha256"`
		EngineLockSHA256 string `json:"engine_lock_sha256"`
		AdapterSHA256    string `json:"adapter_sha256"`
		CalibrationValid bool   `json:"calibration_valid"`
	}
	if json.Unmarshal(output.Bytes(), &receipt) != nil || receipt.BenchmarkSHA256 != release.SHA256 || receipt.EngineLockSHA256 != release.EngineLockSHA256 || receipt.AdapterSHA256 != v.adapterSHA256 || !receipt.CalibrationValid {
		return nil, ErrBenchmarkValidator
	}
	return json.RawMessage(strings.TrimSpace(output.String())), nil
}
