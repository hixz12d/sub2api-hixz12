package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
)

var (
	ErrBenchmarkRelease   = errors.New("benchmark release unavailable or conflicting")
	ErrBenchmarkAdmin     = errors.New("benchmark administrator required")
	ErrBenchmarkValidator = errors.New("benchmark validator unavailable or rejected candidate")
)

type BenchmarkPackageInput struct {
	ID, Version, SHA256, ContentSHA256, Mode string
	Payload                                  []byte
	EngineLock                               json.RawMessage
}

type BenchmarkRelease struct {
	ID, BenchmarkID, Version, SHA256, ContentSHA256, Mode, EngineLockSHA256, State string
	Payload                                                                        []byte
	EngineLock, Receipt                                                            json.RawMessage
}

type BenchmarkReleaseSummary struct {
	ID               string `json:"id"`
	BenchmarkID      string `json:"benchmark_id"`
	Version          string `json:"version"`
	SHA256           string `json:"sha256"`
	Mode             string `json:"mode"`
	EngineLockSHA256 string `json:"engine_lock_sha256"`
	State            string `json:"state"`
}

type BenchmarkChannel struct {
	Name      string `json:"name"`
	ReleaseID string `json:"release_id"`
	Revision  int64  `json:"revision"`
	State     string `json:"state"`
}

type BenchmarkRegistryStore interface {
	Stage(context.Context, BenchmarkPackageInput, int64) (string, error)
	Get(context.Context, string) (*BenchmarkRelease, error)
	List(context.Context, int, int) ([]BenchmarkReleaseSummary, error)
	Channels(context.Context) ([]BenchmarkChannel, error)
	ApproveValidated(context.Context, string, string, string, json.RawMessage, int64) error
	Activate(context.Context, string, string, int64, int64) (int64, error)
	Withdraw(context.Context, string, string, int64) error
}

type BenchmarkValidator interface {
	EngineLock(context.Context) (json.RawMessage, error)
	Validate(context.Context, *BenchmarkRelease) (json.RawMessage, error)
}

type BenchmarkAdminUsers interface {
	GetByID(context.Context, int64) (*User, error)
}

type BenchmarkService struct {
	store     BenchmarkRegistryStore
	validator BenchmarkValidator
	users     BenchmarkAdminUsers
}

func NewBenchmarkService(store BenchmarkRegistryStore, validator BenchmarkValidator, users BenchmarkAdminUsers) *BenchmarkService {
	return &BenchmarkService{store: store, validator: validator, users: users}
}

func (s *BenchmarkService) authorize(ctx context.Context, actor int64) error {
	if s == nil || s.store == nil || s.users == nil || actor <= 0 {
		return ErrBenchmarkAdmin
	}
	user, err := s.users.GetByID(ctx, actor)
	if err != nil || user == nil || user.Role != RoleAdmin || user.Status != StatusActive {
		return ErrBenchmarkAdmin
	}
	return nil
}

func (s *BenchmarkService) List(ctx context.Context, actor int64, limit, offset int) ([]BenchmarkReleaseSummary, []BenchmarkChannel, error) {
	if err := s.authorize(ctx, actor); err != nil {
		return nil, nil, err
	}
	if limit < 1 || limit > 100 || offset < 0 || offset > 100000 {
		return nil, nil, ErrBenchmarkRelease
	}
	items, err := s.store.List(ctx, limit, offset)
	if err != nil {
		return nil, nil, err
	}
	channels, err := s.store.Channels(ctx)
	return items, channels, err
}

func (s *BenchmarkService) Stage(ctx context.Context, actor int64, input BenchmarkPackageInput) (string, error) {
	if err := s.authorize(ctx, actor); err != nil {
		return "", err
	}
	if s.validator == nil || len(input.EngineLock) != 0 {
		return "", ErrBenchmarkValidator
	}
	lock, err := s.validator.EngineLock(ctx)
	if err != nil {
		return "", ErrBenchmarkValidator
	}
	input.EngineLock = lock
	return s.store.Stage(ctx, input, actor)
}

func (s *BenchmarkService) Approve(ctx context.Context, actor int64, id string) error {
	if err := s.authorize(ctx, actor); err != nil {
		return err
	}
	if s.validator == nil {
		return ErrBenchmarkValidator
	}
	release, err := s.store.Get(ctx, id)
	if err != nil {
		return err
	}
	if release == nil || release.State == "withdrawn" {
		return ErrBenchmarkRelease
	}
	receipt, err := s.validator.Validate(ctx, release)
	if err != nil {
		return ErrBenchmarkValidator
	}
	// Recheck the actor after the bounded but potentially slow offline validation.
	if err := s.authorize(ctx, actor); err != nil {
		return err
	}
	return s.store.ApproveValidated(ctx, id, release.SHA256, release.EngineLockSHA256, receipt, actor)
}

func (s *BenchmarkService) Activate(ctx context.Context, actor int64, channel, id string, revision int64) (int64, error) {
	if err := s.authorize(ctx, actor); err != nil {
		return 0, err
	}
	if revision < 0 || revision >= 9007199254740991 {
		return 0, ErrBenchmarkRelease
	}
	return s.store.Activate(ctx, channel, id, revision, actor)
}

func (s *BenchmarkService) Withdraw(ctx context.Context, actor int64, id, reason string) error {
	if err := s.authorize(ctx, actor); err != nil {
		return err
	}
	return s.store.Withdraw(ctx, id, reason, actor)
}

func benchmarkSHA256(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
