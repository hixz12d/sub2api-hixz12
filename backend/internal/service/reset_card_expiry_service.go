package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/google/uuid"
)

const (
	resetCardExpiryInterval = 5 * time.Minute
	// resetCardExpiryLeaderLockKey 保证两个实例不会同时跑回收。
	resetCardExpiryLeaderLockKey = "reset_card_expiry"
	// resetCardExpiryLeaderLockTTL 必须大于单轮最长耗时（resetCardExpiryRunTimeout）。
	resetCardExpiryLeaderLockTTL = 10 * time.Minute
	resetCardExpiryRunTimeout    = 4 * time.Minute
	resetCardExpiryItemTimeout   = 30 * time.Second
	resetCardExpiryBatchLimit    = 200
)

// ResetCardExpiryService 定时回收到期限时额度里没用完的部分。
//
// 防重复扣：每轮先拿 leader 锁；每笔额度再用条件更新 status='active'→'settled'，
// 影响 0 行说明别的实例已处理过，直接跳过。扣余额走 DeductAvailableBalance，最多扣到 0。
type ResetCardExpiryService struct {
	repo                 ResetCardRepository
	deductor             availableBalanceDeductor
	entClient            *dbent.Client
	authCacheInvalidator APIKeyAuthCacheInvalidator
	billingCacheService  *BillingCacheService

	lockCache  LeaderLockCache
	db         *sql.DB
	instanceID string

	interval time.Duration
	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

// NewResetCardExpiryService 创建到期回收任务。
func NewResetCardExpiryService(
	repo ResetCardRepository,
	userRepo UserRepository,
	entClient *dbent.Client,
	authCacheInvalidator APIKeyAuthCacheInvalidator,
	billingCacheService *BillingCacheService,
	lockCache LeaderLockCache,
	db *sql.DB,
) *ResetCardExpiryService {
	deductor, _ := userRepo.(availableBalanceDeductor)
	return &ResetCardExpiryService{
		repo:                 repo,
		deductor:             deductor,
		entClient:            entClient,
		authCacheInvalidator: authCacheInvalidator,
		billingCacheService:  billingCacheService,
		lockCache:            lockCache,
		db:                   db,
		instanceID:           uuid.NewString(),
		interval:             resetCardExpiryInterval,
		stopCh:               make(chan struct{}),
	}
}

// Start 启动定时任务。
func (s *ResetCardExpiryService) Start() {
	if s == nil || s.repo == nil || s.deductor == nil || s.entClient == nil || s.interval <= 0 {
		if s != nil && s.deductor == nil {
			slog.Warn("[ResetCardExpiry] user repository does not support available balance deduction; job disabled")
		}
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		s.runOnce()
		for {
			select {
			case <-ticker.C:
				s.runOnce()
			case <-s.stopCh:
				return
			}
		}
	}()
}

// Stop 停止定时任务并等待当前一轮结束。
func (s *ResetCardExpiryService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
	s.wg.Wait()
}

func (s *ResetCardExpiryService) runOnce() {
	lockCtx, lockCancel := context.WithTimeout(context.Background(), 2*time.Second)
	release, ok := tryAcquireSingletonLeaderLock(lockCtx, s.lockCache, s.db, resetCardExpiryLeaderLockKey, s.instanceID, resetCardExpiryLeaderLockTTL)
	lockCancel()
	if !ok {
		return
	}
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), resetCardExpiryRunTimeout)
	defer cancel()

	now := time.Now().UTC()
	due, err := s.repo.ListDueCredits(ctx, now, resetCardExpiryBatchLimit)
	if err != nil {
		slog.Error("[ResetCardExpiry] failed to list due credits", "error", err)
		return
	}
	settled, clawedBack := 0, 0.0
	for i := range due {
		select {
		case <-s.stopCh:
			return
		default:
		}
		if ctx.Err() != nil {
			return
		}
		deducted, done, err := s.settleOne(ctx, &due[i])
		if err != nil {
			slog.Error("[ResetCardExpiry] failed to settle credit", "credit_id", due[i].ID, "user_id", due[i].UserID, "error", err)
			continue
		}
		if done {
			settled++
			clawedBack += deducted
		}
	}
	if settled > 0 {
		slog.Info("[ResetCardExpiry] settled expired credits", "count", settled, "clawback_total", roundResetCardAmount8(clawedBack))
	}
}

// settleOne 结算一笔到期额度。返回实际扣回金额、是否由本次结算。
func (s *ResetCardExpiryService) settleOne(parent context.Context, credit *ResetCardCreditRecord) (float64, bool, error) {
	ctx, cancel := context.WithTimeout(parent, resetCardExpiryItemTimeout)
	defer cancel()

	now := time.Now().UTC()
	credits, err := s.repo.ListUserCredits(ctx, credit.UserID)
	if err != nil {
		return 0, false, fmt.Errorf("list user credits: %w", err)
	}
	usage, err := computeResetCardCreditUsage(ctx, s.repo, credit.UserID, credits, map[int64]struct{}{credit.ID: {}}, now)
	if err != nil {
		return 0, false, err
	}
	used := usage[credit.ID]
	if used > credit.Amount {
		used = credit.Amount
	}
	used = roundResetCardAmount8(used)
	unused := roundResetCardAmount8(credit.Amount - used)

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return 0, false, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)

	ok, err := s.repo.SettleCredit(txCtx, credit.ID, used, now)
	if err != nil {
		return 0, false, fmt.Errorf("settle credit: %w", err)
	}
	if !ok {
		// 已被其他实例结算，不重复扣
		return 0, false, nil
	}

	deducted := 0.0
	if unused > 0 {
		deducted, err = s.deductor.DeductAvailableBalance(txCtx, credit.UserID, unused)
		if err != nil {
			if !errors.Is(err, ErrUserNotFound) {
				return 0, false, fmt.Errorf("deduct balance: %w", err)
			}
			// 用户已删除：只结算，不扣款
			deducted = 0
		}
		deducted = roundResetCardAmount8(deducted)
	}
	if err := s.repo.SetCreditClawback(txCtx, credit.ID, deducted); err != nil {
		return 0, false, fmt.Errorf("record clawback: %w", err)
	}
	if deducted > 0 {
		code, err := generateResetCardRecordCode()
		if err != nil {
			return 0, false, err
		}
		if err := s.repo.InsertBalanceRecord(txCtx, credit.UserID, RedeemTypeResetCardClawback, code, -deducted, credit.BatchName, now); err != nil {
			return 0, false, fmt.Errorf("create balance record: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, false, fmt.Errorf("commit transaction: %w", err)
	}
	if deducted > 0 {
		invalidateResetCardBalanceCaches(parent, s.authCacheInvalidator, s.billingCacheService, credit.UserID)
	}
	return deducted, true, nil
}
