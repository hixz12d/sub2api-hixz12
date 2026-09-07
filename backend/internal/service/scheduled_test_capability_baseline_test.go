//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type capabilityBaselineResultRepo struct {
	ScheduledTestResultRepository
	createErr error
	saved     *ScheduledTestResult
	pruned    bool
	planID    int64
	keep      int
}

func (r *capabilityBaselineResultRepo) Create(_ context.Context, result *ScheduledTestResult) (*ScheduledTestResult, error) {
	r.saved = result
	return result, r.createErr
}

func (r *capabilityBaselineResultRepo) PruneOldResults(_ context.Context, planID int64, keep int) error {
	r.pruned, r.planID, r.keep = true, planID, keep
	return nil
}

func TestScheduledTestCapabilityBaselineSaveBeforePrune(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "saved", true: "save failed"}[failed], func(t *testing.T) {
			repo := &capabilityBaselineResultRepo{}
			if failed {
				repo.createErr = errors.New("storage unavailable")
			}
			svc := NewScheduledTestService(nil, repo)
			result := &ScheduledTestResult{Status: "success", ResponseText: "29"}
			err := svc.SaveResult(context.Background(), 7, 50, result)
			require.Equal(t, int64(7), repo.saved.PlanID)
			require.Equal(t, "success", repo.saved.Status)
			require.Equal(t, "29", repo.saved.ResponseText)
			if failed {
				require.ErrorIs(t, err, repo.createErr)
				require.False(t, repo.pruned)
			} else {
				require.NoError(t, err)
				require.True(t, repo.pruned)
				require.Equal(t, int64(7), repo.planID)
				require.Equal(t, 50, repo.keep)
			}
		})
	}
}

func TestScheduledTestCapabilityBaselineCron(t *testing.T) {
	from := time.Date(2026, 1, 1, 12, 1, 0, 0, time.UTC)
	next, err := computeNextRun("0 */2 * * *", from)
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 1, 1, 14, 0, 0, 0, time.UTC), next)
	_, err = computeNextRun("not a cron", from)
	require.Error(t, err)
}

func TestScheduledTestCapabilityBaselineSSEIsNotSemanticJudging(t *testing.T) {
	text, message := parseTestSSEOutput("data: {\"type\":\"content\",\"text\":\"29\"}\n\ndata: {\"type\":\"error\",\"error\":\"upstream unavailable\"}\n")
	require.Equal(t, "29", text)
	require.Equal(t, "upstream unavailable", message)
}
