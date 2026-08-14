package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type OpenAIAffinityMigrationPlan struct {
	FromAccountID int64                              `json:"from_account_id"`
	ToAccountID   int64                              `json:"to_account_id"`
	Reason        string                             `json:"reason"`
	GeneratedAt   time.Time                          `json:"generated_at"`
	Candidates    []OpenAIAffinityMigrationCandidate `json:"candidates"`
	Digest        string                             `json:"digest"`
}

type OpenAIAffinityMigrationTool struct {
	repo        OpenAIAffinityRepository
	accountRepo AccountRepository
	now         func() time.Time
}

func NewOpenAIAffinityMigrationTool(repo OpenAIAffinityRepository, accountRepos ...AccountRepository) *OpenAIAffinityMigrationTool {
	tool := &OpenAIAffinityMigrationTool{repo: repo, now: func() time.Time { return time.Now().UTC() }}
	if len(accountRepos) > 0 {
		tool.accountRepo = accountRepos[0]
	}
	return tool
}

func (t *OpenAIAffinityMigrationTool) Preview(ctx context.Context, fromAccountID, toAccountID int64, reason string, includeExpired bool) (*OpenAIAffinityMigrationPlan, error) {
	if t == nil || t.repo == nil {
		return nil, errors.New("openai affinity migration repository unavailable")
	}
	reason = normalizeOpenAIAffinityReason(reason)
	if fromAccountID <= 0 || toAccountID <= 0 || fromAccountID == toAccountID || reason == "" {
		return nil, errors.New("distinct source/target accounts and explicit reason are required")
	}
	if t.accountRepo != nil {
		target, err := t.accountRepo.GetByID(ctx, toAccountID)
		if err != nil || target == nil {
			return nil, errors.New("migration target account is unavailable")
		}
		if target.Status != StatusActive || !target.IsOpenAIOAuth() {
			return nil, errors.New("migration target must be an active OpenAI OAuth account")
		}
	}
	now := t.now()
	// One binding per plan keeps the confirmation and repository CAS operation
	// atomic; operators explicitly preview the next binding after every change.
	candidates, err := t.repo.ListMigrationCandidates(ctx, fromAccountID, includeExpired, 1, now)
	if err != nil {
		return nil, err
	}
	plan := &OpenAIAffinityMigrationPlan{FromAccountID: fromAccountID, ToAccountID: toAccountID, Reason: reason, GeneratedAt: now, Candidates: candidates}
	plan.Digest, err = openAIAffinityMigrationPlanDigest(plan)
	if err != nil {
		return nil, err
	}
	return plan, nil
}

func openAIAffinityMigrationPlanDigest(plan *OpenAIAffinityMigrationPlan) (string, error) {
	if plan == nil {
		return "", errors.New("nil migration plan")
	}
	copyPlan := *plan
	copyPlan.Digest = ""
	payload, err := json.Marshal(copyPlan)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func (t *OpenAIAffinityMigrationTool) Apply(ctx context.Context, plan *OpenAIAffinityMigrationPlan, confirmDigest string) error {
	if t == nil || t.repo == nil || plan == nil {
		return errors.New("openai affinity migration plan unavailable")
	}
	digest, err := openAIAffinityMigrationPlanDigest(plan)
	if err != nil {
		return err
	}
	if strings.TrimSpace(confirmDigest) == "" || !strings.EqualFold(strings.TrimSpace(confirmDigest), digest) || !strings.EqualFold(plan.Digest, digest) {
		return errors.New("migration confirmation digest mismatch; generate a fresh preview")
	}
	if len(plan.Candidates) != 1 {
		return errors.New("migration apply requires exactly one previewed binding")
	}
	candidate := plan.Candidates[0]
	return t.repo.MigrateBindingCAS(ctx, candidate.BindingID, plan.FromAccountID, plan.ToAccountID,
		candidate.Version, plan.Reason, t.now())
}
