//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type benchmarkServiceStore struct {
	BenchmarkRegistryStore
	approved int
	release  *BenchmarkRelease
	receipt  json.RawMessage
}

func (s *benchmarkServiceStore) Get(context.Context, string) (*BenchmarkRelease, error) {
	return s.release, nil
}
func (s *benchmarkServiceStore) ApproveValidated(_ context.Context, _, _, _ string, receipt json.RawMessage, _ int64) error {
	s.approved++
	s.receipt = receipt
	return nil
}

type benchmarkServiceUsers struct{ role string }

func (u *benchmarkServiceUsers) GetByID(_ context.Context, id int64) (*User, error) {
	return &User{ID: id, Role: u.role, Status: StatusActive}, nil
}

type benchmarkServiceValidator struct {
	BenchmarkValidator
	users  *benchmarkServiceUsers
	fail   bool
	revoke bool
}

func (v *benchmarkServiceValidator) Validate(context.Context, *BenchmarkRelease) (json.RawMessage, error) {
	if v.revoke {
		v.users.role = RoleUser
	}
	if v.fail {
		return nil, errors.New("untrusted error")
	}
	return json.RawMessage(`{"trusted":true}`), nil
}
func TestBenchmarkServiceApproval(t *testing.T) {
	for _, mode := range []string{"success", "user", "validator_failure", "role_revoked", "withdrawn"} {
		t.Run(mode, func(t *testing.T) {
			users := &benchmarkServiceUsers{role: RoleAdmin}
			store := &benchmarkServiceStore{release: &BenchmarkRelease{State: "candidate"}}
			validator := &benchmarkServiceValidator{users: users, fail: mode == "validator_failure", revoke: mode == "role_revoked"}
			if mode == "user" {
				users.role = RoleUser
			}
			if mode == "withdrawn" {
				store.release.State = "withdrawn"
			}
			svc := NewBenchmarkService(store, validator, users)
			err := svc.Approve(context.Background(), 1, "fixture")
			if mode == "success" {
				require.NoError(t, err)
				require.Equal(t, 1, store.approved)
				require.JSONEq(t, `{"trusted":true}`, string(store.receipt))
			} else {
				require.Error(t, err)
				require.Zero(t, store.approved)
			}
		})
	}
}
func TestBenchmarkValidatorFailsClosed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	v := &PinnedMeowValidator{}
	_, err := v.EngineLock(context.Background())
	require.ErrorIs(t, err, ErrBenchmarkValidator)
	_, err = v.EngineLock(ctx)
	require.ErrorIs(t, err, ErrBenchmarkValidator)
	_, err = v.Validate(context.Background(), nil)
	require.ErrorIs(t, err, ErrBenchmarkValidator)
	var out benchmarkBoundedOutput
	_, err = out.Write([]byte(strings.Repeat("x", 65536)))
	require.NoError(t, err)
	_, err = out.Write([]byte("overflow"))
	require.ErrorIs(t, err, ErrBenchmarkValidator)
	require.Equal(t, 65536, out.Len())
	var copied benchmarkBoundedOutput
	_, err = io.Copy(&copied, io.LimitReader(strings.NewReader(strings.Repeat("x", 65537)), 65537))
	require.ErrorIs(t, err, ErrBenchmarkValidator)
	require.LessOrEqual(t, copied.Len(), 65536)
}
