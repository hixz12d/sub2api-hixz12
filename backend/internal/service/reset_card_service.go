package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// 重置卡（消费返还卡）：管理员按批次发卡，用户用卡时把卡适用分组里
// "最近 7 天、上次返还之后"的消费按比例退回余额，退回部分是限时额度，到期回收没用完的部分。

var (
	ErrResetCardDisabled        = infraerrors.NotFound("RESET_CARD_DISABLED", "reset cards are disabled")
	ErrResetCardNotAvailable    = infraerrors.Conflict("RESET_CARD_NOT_AVAILABLE", "reset card is not available")
	ErrResetCardExpired         = infraerrors.Conflict("RESET_CARD_EXPIRED", "reset card has expired")
	ErrResetCardNothingToRefund = infraerrors.Conflict("RESET_CARD_NOTHING_TO_REFUND", "no new spend to refund since the last reset card")
	ErrResetCardBusy            = infraerrors.TooManyRequests("RESET_CARD_BUSY", "reset card is being processed, please try again")
	ErrResetCardNotFound        = infraerrors.NotFound("RESET_CARD_NOT_FOUND", "reset card not found")
	ErrResetCardBatchNotFound   = infraerrors.NotFound("RESET_CARD_BATCH_NOT_FOUND", "reset card batch not found")
	ErrResetCardNotRevocable    = infraerrors.Conflict("RESET_CARD_NOT_REVOCABLE", "only unused reset cards can be revoked")
	ErrResetCardNoRecipients    = infraerrors.BadRequest("RESET_CARD_NO_RECIPIENTS", "no eligible users for this batch")
)

const (
	ResetCardStatusUnused  = "unused"
	ResetCardStatusUsed    = "used"
	ResetCardStatusExpired = "expired"
	ResetCardStatusRevoked = "revoked"

	ResetCardCreditStatusActive  = "active"
	ResetCardCreditStatusSettled = "settled"

	ResetCardTargetAll   = "all"
	ResetCardTargetUsers = "users"

	resetCardSpendWindow       = 7 * 24 * time.Hour
	resetCardSpendDays         = 7
	resetCardUseLockTTL        = 10 * time.Second
	resetCardMaxUserIDs        = 2000
	resetCardMaxGroupIDs       = 50
	resetCardMaxPopupIDs       = 500
	resetCardUserListLimit     = 500
	resetCardMaxAmountUpper    = 1e11
	resetCardBalanceRecordCode = "RSC"
)

// ==================== 仓储层数据 ====================

// ResetCardCreditRecord 是 reset_card_credits 的一行。
type ResetCardCreditRecord struct {
	ID             int64
	CardID         int64
	UserID         int64
	GroupIDs       []int64
	Amount         float64
	StartsAt       time.Time
	ExpiresAt      time.Time
	Status         string
	UsedAmount     *float64
	ClawbackAmount *float64
	SettledAt      *time.Time
	// BatchName 只在到期回收查询里填充，用作余额记录备注。
	BatchName string
}

// ResetCardRecord 是一张卡及其批次信息。
type ResetCardRecord struct {
	ID              int64
	BatchID         int64
	UserID          int64
	BatchName       string
	GroupIDs        []int64
	Percent         float64
	MaxAmount       float64
	CreditValidDays int
	Status          string
	ExpiresAt       time.Time
	CreatedAt       time.Time
	UsedAt          *time.Time
	RefundAmount    *float64
	PopupSeenAt     *time.Time
	Credit          *ResetCardCreditRecord
}

// ResetCardBatchRecord 是一个批次及其统计。
type ResetCardBatchRecord struct {
	ID              int64
	Name            string
	GroupIDs        []int64
	Percent         float64
	MaxAmount       float64
	CardValidDays   int
	CreditValidDays int
	TargetMode      string
	OnlyActive      bool
	CreatedBy       *int64
	CreatedAt       time.Time
	Stats           ResetCardBatchStats
}

// AdminResetCardRecord 是管理端卡列表的一行。
type AdminResetCardRecord struct {
	ID           int64
	UserID       int64
	Email        string
	Username     string
	Status       string
	ExpiresAt    time.Time
	CreatedAt    time.Time
	UsedAt       *time.Time
	RefundAmount *float64
	Credit       *ResetCardCreditRecord
}

// ResetCardAdminCardFilter 管理端卡列表筛选。Status 取 unused/used/expired/revoked 或空。
type ResetCardAdminCardFilter struct {
	Status string
	Search string
}

// ResetCardRepository 重置卡仓储。支持在 dbent 事务 context 中调用。
type ResetCardRepository interface {
	// 用户端
	ListUserCards(ctx context.Context, userID int64, now time.Time, limit int) ([]ResetCardRecord, error)
	GetUserCard(ctx context.Context, userID, cardID int64) (*ResetCardRecord, error)
	// LockUserCardForUse 必须在事务中调用：先加该用户的事务级咨询锁，再 FOR UPDATE 锁住本人未使用的卡；查不到返回 nil, nil。
	LockUserCardForUse(ctx context.Context, userID, cardID int64) (*ResetCardRecord, error)
	// SpendableDaily 返回"可统计消费"按 24 小时分段的合计，从旧到新共 7 个数。
	SpendableDaily(ctx context.Context, userID int64, groupIDs []int64, now time.Time) ([]float64, error)
	MarkCardUsed(ctx context.Context, cardID int64, usedAt time.Time, refundAmount, baseSpend float64) (bool, error)
	UpsertWatermarks(ctx context.Context, userID int64, groupIDs []int64, until time.Time) error
	InsertCredit(ctx context.Context, credit *ResetCardCreditRecord) error
	InsertBalanceRecord(ctx context.Context, userID int64, recordType string, code string, value float64, notes string, at time.Time) error
	GetUserBalance(ctx context.Context, userID int64) (float64, error)
	MarkPopupSeen(ctx context.Context, userID int64, cardIDs []int64, at time.Time) (int64, error)
	ListUserCredits(ctx context.Context, userID int64) ([]ResetCardCreditRecord, error)
	HourlySpend(ctx context.Context, userID int64, groupIDs []int64, from, to time.Time) ([]ResetCardSpendBucket, error)
	// GroupNames 返回未删除分组的名称。
	GroupNames(ctx context.Context, groupIDs []int64) (map[int64]string, error)

	// 管理端
	ResolveRecipients(ctx context.Context, targetMode string, onlyActive bool, groupIDs, userIDs []int64, since time.Time) ([]int64, error)
	SpendableByUser(ctx context.Context, groupIDs, userIDs []int64, since, now time.Time) (map[int64]float64, error)
	CreateBatch(ctx context.Context, batch *ResetCardBatchRecord, userIDs []int64, cardExpiresAt time.Time) error
	ListBatches(ctx context.Context, offset, limit int, now time.Time) ([]ResetCardBatchRecord, int64, error)
	GetBatch(ctx context.Context, batchID int64, now time.Time) (*ResetCardBatchRecord, error)
	ListBatchCards(ctx context.Context, batchID int64, filter ResetCardAdminCardFilter, offset, limit int, now time.Time) ([]AdminResetCardRecord, int64, error)
	RevokeBatchCards(ctx context.Context, batchID int64, at time.Time) (int64, error)
	RevokeCard(ctx context.Context, cardID int64, at time.Time) (int64, error)
	CardExists(ctx context.Context, cardID int64) (bool, error)

	// 到期回收
	ListDueCredits(ctx context.Context, now time.Time, limit int) ([]ResetCardCreditRecord, error)
	SettleCredit(ctx context.Context, creditID int64, usedAmount float64, at time.Time) (bool, error)
	SetCreditClawback(ctx context.Context, creditID int64, clawbackAmount float64) error
}

// ==================== 接口返回 ====================

type ResetCardGroupRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type ResetCardEstimate struct {
	Amount    float64   `json:"amount"`
	BaseSpend float64   `json:"base_spend"`
	Daily     []float64 `json:"daily"`
	Capped    bool      `json:"capped"`
}

type ResetCardCreditView struct {
	Amount          float64   `json:"amount"`
	UsedAmount      float64   `json:"used_amount"`
	RemainingAmount float64   `json:"remaining_amount"`
	StartsAt        time.Time `json:"starts_at"`
	ExpiresAt       time.Time `json:"expires_at"`
	Status          string    `json:"status"`
	ClawbackAmount  *float64  `json:"clawback_amount"`
}

type ResetCardView struct {
	ID        int64               `json:"id"`
	BatchName string              `json:"batch_name"`
	Groups    []ResetCardGroupRef `json:"groups"`
	Percent   float64             `json:"percent"`
	MaxAmount float64             `json:"max_amount"`
	// CreditValidDays 批次设定的返还额度有效天数，卡面"到账后 N 天内用完"使用
	CreditValidDays int                  `json:"credit_valid_days"`
	Status          string               `json:"status"`
	ExpiresAt       time.Time            `json:"expires_at"`
	CreatedAt       time.Time            `json:"created_at"`
	UsedAt          *time.Time           `json:"used_at"`
	RefundAmount    *float64             `json:"refund_amount"`
	Estimate        *ResetCardEstimate   `json:"estimate"`
	Credit          *ResetCardCreditView `json:"credit"`
}

type ResetCardListResult struct {
	Cards                  []ResetCardView `json:"cards"`
	UnusedCount            int             `json:"unused_count"`
	LimitedCreditRemaining float64         `json:"limited_credit_remaining"`
	PopupCardIDs           []int64         `json:"popup_card_ids"`
}

type ResetCardUseResult struct {
	RefundAmount    float64   `json:"refund_amount"`
	CreditExpiresAt time.Time `json:"credit_expires_at"`
	Balance         float64   `json:"balance"`
}

type ResetCardBatchStats struct {
	Issued        int64   `json:"issued"`
	Unused        int64   `json:"unused"`
	Used          int64   `json:"used"`
	Expired       int64   `json:"expired"`
	Revoked       int64   `json:"revoked"`
	RefundedTotal float64 `json:"refunded_total"`
	ClawbackTotal float64 `json:"clawback_total"`
}

type ResetCardBatchView struct {
	ID              int64               `json:"id"`
	Name            string              `json:"name"`
	Groups          []ResetCardGroupRef `json:"groups"`
	Percent         float64             `json:"percent"`
	MaxAmount       float64             `json:"max_amount"`
	CardValidDays   int                 `json:"card_valid_days"`
	CreditValidDays int                 `json:"credit_valid_days"`
	TargetMode      string              `json:"target_mode"`
	OnlyActive      bool                `json:"only_active"`
	CreatedAt       time.Time           `json:"created_at"`
	Stats           ResetCardBatchStats `json:"stats"`
}

type AdminResetCardUser struct {
	ID       int64  `json:"id"`
	Email    string `json:"email"`
	Username string `json:"username"`
}

type AdminResetCardCreditView struct {
	Amount          float64   `json:"amount"`
	UsedAmount      float64   `json:"used_amount"`
	RemainingAmount float64   `json:"remaining_amount"`
	ExpiresAt       time.Time `json:"expires_at"`
	Status          string    `json:"status"`
	ClawbackAmount  *float64  `json:"clawback_amount"`
}

type AdminResetCardView struct {
	ID           int64                     `json:"id"`
	User         AdminResetCardUser        `json:"user"`
	Status       string                    `json:"status"`
	ExpiresAt    time.Time                 `json:"expires_at"`
	CreatedAt    time.Time                 `json:"created_at"`
	UsedAt       *time.Time                `json:"used_at"`
	RefundAmount *float64                  `json:"refund_amount"`
	Credit       *AdminResetCardCreditView `json:"credit"`
}

type ResetCardBatchPreviewResult struct {
	RecipientCount   int     `json:"recipient_count"`
	EstimatedMaxCost float64 `json:"estimated_max_cost"`
}

// ResetCardBatchInput 发卡 / 发卡预估的输入。预估时 Name 和两个有效期不校验。
type ResetCardBatchInput struct {
	Name            string
	GroupIDs        []int64
	Percent         float64
	MaxAmount       float64
	CardValidDays   int
	CreditValidDays int
	TargetMode      string
	OnlyActive      bool
	UserIDs         []int64
	CreatedBy       int64
}

// ==================== 服务 ====================

// ResetCardService 重置卡业务。
type ResetCardService struct {
	repo                 ResetCardRepository
	redeemUserRepo       RedeemUserAdjustmentRepository
	cache                RedeemCache
	entClient            *dbent.Client
	settingService       *SettingService
	authCacheInvalidator APIKeyAuthCacheInvalidator
	billingCacheService  *BillingCacheService
}

// NewResetCardService 创建重置卡服务。
func NewResetCardService(
	repo ResetCardRepository,
	userRepo UserRepository,
	cache RedeemCache,
	entClient *dbent.Client,
	settingService *SettingService,
	authCacheInvalidator APIKeyAuthCacheInvalidator,
	billingCacheService *BillingCacheService,
) *ResetCardService {
	redeemUserRepo, _ := userRepo.(RedeemUserAdjustmentRepository)
	return &ResetCardService{
		repo:                 repo,
		redeemUserRepo:       redeemUserRepo,
		cache:                cache,
		entClient:            entClient,
		settingService:       settingService,
		authCacheInvalidator: authCacheInvalidator,
		billingCacheService:  billingCacheService,
	}
}

func (s *ResetCardService) enabled(ctx context.Context) bool {
	return s.settingService != nil && s.settingService.IsResetCardEnabled(ctx)
}

// ---------- 用户端 ----------

// ListUserCards 列出本人的卡：可用在前，然后按创建时间倒序。
func (s *ResetCardService) ListUserCards(ctx context.Context, userID int64) (*ResetCardListResult, error) {
	if !s.enabled(ctx) {
		return nil, ErrResetCardDisabled
	}
	now := time.Now().UTC()
	records, err := s.repo.ListUserCards(ctx, userID, now, resetCardUserListLimit)
	if err != nil {
		return nil, fmt.Errorf("list reset cards: %w", err)
	}
	names, err := s.repo.GroupNames(ctx, collectResetCardGroupIDs(records))
	if err != nil {
		return nil, fmt.Errorf("load group names: %w", err)
	}

	creditUsage := map[int64]float64{}
	hasActiveCredit := false
	for i := range records {
		if c := records[i].Credit; c != nil && c.Status == ResetCardCreditStatusActive {
			hasActiveCredit = true
			break
		}
	}
	if hasActiveCredit {
		creditUsage, err = s.activeCreditUsage(ctx, userID, now)
		if err != nil {
			return nil, err
		}
	}

	result := &ResetCardListResult{
		Cards:        make([]ResetCardView, 0, len(records)),
		PopupCardIDs: []int64{},
	}
	for i := range records {
		rec := &records[i]
		view := ResetCardView{
			ID:           rec.ID,
			BatchName:    rec.BatchName,
			Groups:       resetCardGroupRefs(rec.GroupIDs, names),
			Percent:      rec.Percent,
			MaxAmount:    rec.MaxAmount,
			Status:       resetCardDisplayStatus(rec.Status, rec.ExpiresAt, now),
			ExpiresAt:    rec.ExpiresAt,
			CreatedAt:    rec.CreatedAt,
			UsedAt:       rec.UsedAt,
			RefundAmount: rec.RefundAmount,
		}
		view.CreditValidDays = rec.CreditValidDays
		switch view.Status {
		case ResetCardStatusUnused:
			result.UnusedCount++
			if rec.PopupSeenAt == nil {
				result.PopupCardIDs = append(result.PopupCardIDs, rec.ID)
			}
			daily, err := s.repo.SpendableDaily(ctx, userID, rec.GroupIDs, now)
			if err != nil {
				return nil, fmt.Errorf("estimate reset card: %w", err)
			}
			est := buildResetCardEstimate(daily, rec.Percent, rec.MaxAmount)
			view.Estimate = &est
		case ResetCardStatusUsed:
			if rec.Credit != nil {
				cv := resetCardCreditView(rec.Credit, creditUsage)
				view.Credit = &cv
				if cv.Status == ResetCardCreditStatusActive && rec.Credit.ExpiresAt.After(now) {
					result.LimitedCreditRemaining += cv.RemainingAmount
				}
			}
		}
		result.Cards = append(result.Cards, view)
	}
	result.LimitedCreditRemaining = roundResetCardAmount8(result.LimitedCreditRemaining)
	return result, nil
}

// PreviewUserCard 实时重算一张可用卡的预估返还。
func (s *ResetCardService) PreviewUserCard(ctx context.Context, userID, cardID int64) (*ResetCardEstimate, error) {
	if !s.enabled(ctx) {
		return nil, ErrResetCardDisabled
	}
	rec, err := s.repo.GetUserCard(ctx, userID, cardID)
	if err != nil {
		return nil, fmt.Errorf("get reset card: %w", err)
	}
	if rec == nil || rec.Status != ResetCardStatusUnused {
		return nil, ErrResetCardNotAvailable
	}
	now := time.Now().UTC()
	if !rec.ExpiresAt.After(now) {
		return nil, ErrResetCardExpired
	}
	daily, err := s.repo.SpendableDaily(ctx, userID, rec.GroupIDs, now)
	if err != nil {
		return nil, fmt.Errorf("estimate reset card: %w", err)
	}
	est := buildResetCardEstimate(daily, rec.Percent, rec.MaxAmount)
	return &est, nil
}

// UseCard 使用一张卡：按"现在"计算可统计消费，加余额并生成限时额度。可返还为 0 时卡不会被消耗。
func (s *ResetCardService) UseCard(ctx context.Context, userID, cardID int64) (*ResetCardUseResult, error) {
	if !s.enabled(ctx) {
		return nil, ErrResetCardDisabled
	}
	if s.redeemUserRepo == nil || s.entClient == nil {
		return nil, errors.New("reset card service is not fully configured")
	}

	lockKey := fmt.Sprintf("reset_card:use:%d", userID)
	if !s.acquireUseLock(ctx, lockKey) {
		return nil, ErrResetCardBusy
	}
	defer s.releaseUseLock(ctx, lockKey)

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)

	card, err := s.repo.LockUserCardForUse(txCtx, userID, cardID)
	if err != nil {
		return nil, fmt.Errorf("lock reset card: %w", err)
	}
	if card == nil {
		return nil, ErrResetCardNotAvailable
	}
	now := time.Now().UTC()
	if !card.ExpiresAt.After(now) {
		return nil, ErrResetCardExpired
	}

	daily, err := s.repo.SpendableDaily(txCtx, userID, card.GroupIDs, now)
	if err != nil {
		return nil, fmt.Errorf("compute spendable usage: %w", err)
	}
	est := buildResetCardEstimate(daily, card.Percent, card.MaxAmount)
	if est.Amount <= 0 {
		return nil, ErrResetCardNothingToRefund
	}

	ok, err := s.repo.MarkCardUsed(txCtx, card.ID, now, est.Amount, est.BaseSpend)
	if err != nil {
		return nil, fmt.Errorf("mark reset card used: %w", err)
	}
	if !ok {
		return nil, ErrResetCardNotAvailable
	}
	if err := s.repo.UpsertWatermarks(txCtx, userID, card.GroupIDs, now); err != nil {
		return nil, fmt.Errorf("update rebate watermark: %w", err)
	}
	validDays := card.CreditValidDays
	if validDays <= 0 {
		validDays = 7
	}
	credit := &ResetCardCreditRecord{
		CardID:    card.ID,
		UserID:    userID,
		GroupIDs:  card.GroupIDs,
		Amount:    est.Amount,
		StartsAt:  now,
		ExpiresAt: now.AddDate(0, 0, validDays),
		Status:    ResetCardCreditStatusActive,
	}
	if err := s.repo.InsertCredit(txCtx, credit); err != nil {
		return nil, fmt.Errorf("create reset card credit: %w", err)
	}
	if err := s.redeemUserRepo.ApplyRedeemBalanceAdjustment(txCtx, userID, est.Amount); err != nil {
		return nil, fmt.Errorf("update user balance: %w", err)
	}
	code, err := generateResetCardRecordCode()
	if err != nil {
		return nil, err
	}
	if err := s.repo.InsertBalanceRecord(txCtx, userID, RedeemTypeResetCard, code, est.Amount, card.BatchName, now); err != nil {
		return nil, fmt.Errorf("create balance record: %w", err)
	}
	balance, err := s.repo.GetUserBalance(txCtx, userID)
	if err != nil {
		return nil, fmt.Errorf("read user balance: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	s.invalidateBalanceCaches(ctx, userID)
	return &ResetCardUseResult{
		RefundAmount:    est.Amount,
		CreditExpiresAt: credit.ExpiresAt,
		Balance:         balance,
	}, nil
}

// MarkPopupSeen 把本人的这些卡标记为已弹过窗。
func (s *ResetCardService) MarkPopupSeen(ctx context.Context, userID int64, cardIDs []int64) (int64, error) {
	if !s.enabled(ctx) {
		return 0, ErrResetCardDisabled
	}
	ids := dedupePositiveIDs(cardIDs)
	if len(ids) == 0 {
		return 0, nil
	}
	if len(ids) > resetCardMaxPopupIDs {
		return 0, infraerrors.BadRequest("RESET_CARD_INVALID_REQUEST", "too many card ids")
	}
	return s.repo.MarkPopupSeen(ctx, userID, ids, time.Now().UTC())
}

// ---------- 管理端 ----------

// ListBatches 分页列出批次（带统计）。
func (s *ResetCardService) ListBatches(ctx context.Context, page, pageSize int) ([]ResetCardBatchView, int64, error) {
	now := time.Now().UTC()
	records, total, err := s.repo.ListBatches(ctx, (page-1)*pageSize, pageSize, now)
	if err != nil {
		return nil, 0, fmt.Errorf("list reset card batches: %w", err)
	}
	groupIDs := make([]int64, 0)
	for i := range records {
		groupIDs = append(groupIDs, records[i].GroupIDs...)
	}
	names, err := s.repo.GroupNames(ctx, dedupePositiveIDs(groupIDs))
	if err != nil {
		return nil, 0, fmt.Errorf("load group names: %w", err)
	}
	out := make([]ResetCardBatchView, 0, len(records))
	for i := range records {
		out = append(out, resetCardBatchView(&records[i], names))
	}
	return out, total, nil
}

// GetBatch 返回单个批次（带统计）。
func (s *ResetCardService) GetBatch(ctx context.Context, batchID int64) (*ResetCardBatchView, error) {
	rec, err := s.repo.GetBatch(ctx, batchID, time.Now().UTC())
	if err != nil {
		return nil, fmt.Errorf("get reset card batch: %w", err)
	}
	if rec == nil {
		return nil, ErrResetCardBatchNotFound
	}
	names, err := s.repo.GroupNames(ctx, rec.GroupIDs)
	if err != nil {
		return nil, fmt.Errorf("load group names: %w", err)
	}
	view := resetCardBatchView(rec, names)
	return &view, nil
}

// PreviewBatch 发卡前预估收卡人数与最大成本。
func (s *ResetCardService) PreviewBatch(ctx context.Context, input ResetCardBatchInput) (*ResetCardBatchPreviewResult, error) {
	normalized, err := s.validateBatchInput(ctx, input, false)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	since := now.Add(-resetCardSpendWindow)
	recipients, err := s.repo.ResolveRecipients(ctx, normalized.TargetMode, normalized.OnlyActive, normalized.GroupIDs, normalized.UserIDs, since)
	if err != nil {
		return nil, fmt.Errorf("resolve recipients: %w", err)
	}
	var scope []int64
	if normalized.TargetMode == ResetCardTargetUsers {
		scope = recipients
	}
	spend, err := s.repo.SpendableByUser(ctx, normalized.GroupIDs, scope, since, now)
	if err != nil {
		return nil, fmt.Errorf("estimate spend: %w", err)
	}
	total := 0.0
	for _, uid := range recipients {
		total += resetCardRefundAmount(spend[uid], normalized.Percent, normalized.MaxAmount)
	}
	return &ResetCardBatchPreviewResult{
		RecipientCount:   len(recipients),
		EstimatedMaxCost: math.Round(total*100) / 100,
	}, nil
}

// CreateBatch 创建批次并发卡。
func (s *ResetCardService) CreateBatch(ctx context.Context, input ResetCardBatchInput) (*ResetCardBatchView, error) {
	normalized, err := s.validateBatchInput(ctx, input, true)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	recipients, err := s.repo.ResolveRecipients(ctx, normalized.TargetMode, normalized.OnlyActive, normalized.GroupIDs, normalized.UserIDs, now.Add(-resetCardSpendWindow))
	if err != nil {
		return nil, fmt.Errorf("resolve recipients: %w", err)
	}
	if len(recipients) == 0 {
		return nil, ErrResetCardNoRecipients
	}
	batch := &ResetCardBatchRecord{
		Name:            normalized.Name,
		GroupIDs:        normalized.GroupIDs,
		Percent:         normalized.Percent,
		MaxAmount:       normalized.MaxAmount,
		CardValidDays:   normalized.CardValidDays,
		CreditValidDays: normalized.CreditValidDays,
		TargetMode:      normalized.TargetMode,
		OnlyActive:      normalized.OnlyActive,
	}
	if normalized.CreatedBy > 0 {
		createdBy := normalized.CreatedBy
		batch.CreatedBy = &createdBy
	}
	if err := s.repo.CreateBatch(ctx, batch, recipients, now.AddDate(0, 0, normalized.CardValidDays)); err != nil {
		return nil, fmt.Errorf("create reset card batch: %w", err)
	}
	return s.GetBatch(ctx, batch.ID)
}

// ListBatchCards 批次内的卡。
func (s *ResetCardService) ListBatchCards(ctx context.Context, batchID int64, filter ResetCardAdminCardFilter, page, pageSize int) ([]AdminResetCardView, int64, error) {
	switch filter.Status {
	case "", ResetCardStatusUnused, ResetCardStatusUsed, ResetCardStatusExpired, ResetCardStatusRevoked:
	default:
		return nil, 0, infraerrors.BadRequest("RESET_CARD_INVALID_REQUEST", "invalid status filter")
	}
	filter.Search = strings.TrimSpace(filter.Search)
	if utf8.RuneCountInString(filter.Search) > 100 {
		filter.Search = string([]rune(filter.Search)[:100])
	}
	now := time.Now().UTC()
	batch, err := s.repo.GetBatch(ctx, batchID, now)
	if err != nil {
		return nil, 0, fmt.Errorf("get reset card batch: %w", err)
	}
	if batch == nil {
		return nil, 0, ErrResetCardBatchNotFound
	}
	records, total, err := s.repo.ListBatchCards(ctx, batchID, filter, (page-1)*pageSize, pageSize, now)
	if err != nil {
		return nil, 0, fmt.Errorf("list reset cards: %w", err)
	}

	usageByUser := make(map[int64]map[int64]float64)
	for i := range records {
		c := records[i].Credit
		if c == nil || c.Status != ResetCardCreditStatusActive {
			continue
		}
		if _, done := usageByUser[records[i].UserID]; done {
			continue
		}
		usage, err := s.activeCreditUsage(ctx, records[i].UserID, now)
		if err != nil {
			return nil, 0, err
		}
		usageByUser[records[i].UserID] = usage
	}

	out := make([]AdminResetCardView, 0, len(records))
	for i := range records {
		rec := &records[i]
		view := AdminResetCardView{
			ID:           rec.ID,
			User:         AdminResetCardUser{ID: rec.UserID, Email: rec.Email, Username: rec.Username},
			Status:       resetCardDisplayStatus(rec.Status, rec.ExpiresAt, now),
			ExpiresAt:    rec.ExpiresAt,
			CreatedAt:    rec.CreatedAt,
			UsedAt:       rec.UsedAt,
			RefundAmount: rec.RefundAmount,
		}
		if rec.Credit != nil {
			cv := resetCardCreditView(rec.Credit, usageByUser[rec.UserID])
			view.Credit = &AdminResetCardCreditView{
				Amount:          cv.Amount,
				UsedAmount:      cv.UsedAmount,
				RemainingAmount: cv.RemainingAmount,
				ExpiresAt:       cv.ExpiresAt,
				Status:          cv.Status,
				ClawbackAmount:  cv.ClawbackAmount,
			}
		}
		out = append(out, view)
	}
	return out, total, nil
}

// RevokeBatch 作废批次里所有未使用的卡。
func (s *ResetCardService) RevokeBatch(ctx context.Context, batchID int64) (int64, error) {
	batch, err := s.repo.GetBatch(ctx, batchID, time.Now().UTC())
	if err != nil {
		return 0, fmt.Errorf("get reset card batch: %w", err)
	}
	if batch == nil {
		return 0, ErrResetCardBatchNotFound
	}
	return s.repo.RevokeBatchCards(ctx, batchID, time.Now().UTC())
}

// RevokeCard 作废单张未使用的卡；已使用的卡返回错误，已产生的限时额度不受影响。
func (s *ResetCardService) RevokeCard(ctx context.Context, cardID int64) (int64, error) {
	n, err := s.repo.RevokeCard(ctx, cardID, time.Now().UTC())
	if err != nil {
		return 0, fmt.Errorf("revoke reset card: %w", err)
	}
	if n > 0 {
		return n, nil
	}
	exists, err := s.repo.CardExists(ctx, cardID)
	if err != nil {
		return 0, fmt.Errorf("get reset card: %w", err)
	}
	if !exists {
		return 0, ErrResetCardNotFound
	}
	return 0, ErrResetCardNotRevocable
}

func (s *ResetCardService) validateBatchInput(ctx context.Context, input ResetCardBatchInput, forCreate bool) (*ResetCardBatchInput, error) {
	bad := func(msg string) error { return infraerrors.BadRequest("RESET_CARD_INVALID_REQUEST", msg) }
	out := input
	out.Name = strings.TrimSpace(input.Name)
	if forCreate {
		if out.Name == "" || utf8.RuneCountInString(out.Name) > 100 {
			return nil, bad("name is required and must be at most 100 characters")
		}
		if out.CardValidDays < 1 || out.CardValidDays > 90 {
			return nil, bad("card_valid_days must be between 1 and 90")
		}
		if out.CreditValidDays < 1 || out.CreditValidDays > 30 {
			return nil, bad("credit_valid_days must be between 1 and 30")
		}
	}
	out.GroupIDs = dedupePositiveIDs(input.GroupIDs)
	if len(out.GroupIDs) == 0 {
		return nil, bad("group_ids must not be empty")
	}
	if len(out.GroupIDs) > resetCardMaxGroupIDs {
		return nil, bad("too many group_ids")
	}
	if math.IsNaN(input.Percent) || input.Percent <= 0 || input.Percent > 100 {
		return nil, bad("percent must be in (0, 100]")
	}
	out.Percent = math.Round(input.Percent*100) / 100
	if out.Percent <= 0 {
		return nil, bad("percent must be in (0, 100]")
	}
	if math.IsNaN(input.MaxAmount) || input.MaxAmount <= 0 || input.MaxAmount > resetCardMaxAmountUpper {
		return nil, bad("max_amount must be greater than 0")
	}
	switch input.TargetMode {
	case ResetCardTargetAll:
		out.UserIDs = nil
	case ResetCardTargetUsers:
		out.UserIDs = dedupePositiveIDs(input.UserIDs)
		if len(out.UserIDs) == 0 {
			return nil, bad("user_ids is required when target_mode is users")
		}
		if len(out.UserIDs) > resetCardMaxUserIDs {
			return nil, bad("user_ids must contain at most 2000 users")
		}
	default:
		return nil, bad("target_mode must be all or users")
	}
	names, err := s.repo.GroupNames(ctx, out.GroupIDs)
	if err != nil {
		return nil, fmt.Errorf("load groups: %w", err)
	}
	if len(names) != len(out.GroupIDs) {
		return nil, infraerrors.BadRequest("RESET_CARD_INVALID_GROUP", "some groups do not exist")
	}
	return &out, nil
}

// ---------- 限时额度 ----------

// activeCreditUsage 现场计算该用户生效中额度的已用金额（按分摊规则）。
func (s *ResetCardService) activeCreditUsage(ctx context.Context, userID int64, now time.Time) (map[int64]float64, error) {
	credits, err := s.repo.ListUserCredits(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list reset card credits: %w", err)
	}
	targets := make(map[int64]struct{})
	for i := range credits {
		if credits[i].Status == ResetCardCreditStatusActive {
			targets[credits[i].ID] = struct{}{}
		}
	}
	return computeResetCardCreditUsage(ctx, s.repo, userID, credits, targets, now)
}

// computeResetCardCreditUsage 对目标额度跑一次分摊，返回分摊范围内每笔额度的已用金额。
func computeResetCardCreditUsage(ctx context.Context, repo ResetCardRepository, userID int64, credits []ResetCardCreditRecord, targets map[int64]struct{}, now time.Time) (map[int64]float64, error) {
	if len(targets) == 0 {
		return map[int64]float64{}, nil
	}
	all := make([]ResetCardAllocCredit, 0, len(credits))
	for i := range credits {
		all = append(all, ResetCardAllocCredit{
			ID:        credits[i].ID,
			GroupIDs:  credits[i].GroupIDs,
			Amount:    credits[i].Amount,
			StartsAt:  credits[i].StartsAt,
			ExpiresAt: credits[i].ExpiresAt,
		})
	}
	scope, from, ok := resetCardAllocationScope(all, targets)
	if !ok {
		return map[int64]float64{}, nil
	}
	to := now
	var latest time.Time
	groupIDs := make([]int64, 0)
	for _, c := range scope {
		if c.ExpiresAt.After(latest) {
			latest = c.ExpiresAt
		}
		groupIDs = append(groupIDs, c.GroupIDs...)
	}
	if latest.Before(to) {
		to = latest
	}
	var buckets []ResetCardSpendBucket
	if to.After(from) {
		var err error
		buckets, err = repo.HourlySpend(ctx, userID, dedupePositiveIDs(groupIDs), from, to)
		if err != nil {
			return nil, fmt.Errorf("load hourly spend: %w", err)
		}
	}
	return AllocateResetCardCredits(scope, buckets), nil
}

func resetCardCreditView(c *ResetCardCreditRecord, liveUsage map[int64]float64) ResetCardCreditView {
	view := ResetCardCreditView{
		Amount:    c.Amount,
		StartsAt:  c.StartsAt,
		ExpiresAt: c.ExpiresAt,
		Status:    c.Status,
	}
	if c.Status == ResetCardCreditStatusSettled {
		if c.UsedAmount != nil {
			view.UsedAmount = *c.UsedAmount
		}
		clawback := 0.0
		if c.ClawbackAmount != nil {
			clawback = *c.ClawbackAmount
		}
		view.ClawbackAmount = &clawback
	} else {
		view.UsedAmount = liveUsage[c.ID]
	}
	if view.UsedAmount > c.Amount {
		view.UsedAmount = c.Amount
	}
	view.RemainingAmount = roundResetCardAmount8(c.Amount - view.UsedAmount)
	return view
}

// ---------- 缓存与锁 ----------

func (s *ResetCardService) acquireUseLock(ctx context.Context, key string) bool {
	if s.cache == nil {
		return true
	}
	ok, err := s.cache.AcquireRedeemLock(ctx, key, resetCardUseLockTTL)
	if err != nil {
		// Redis 出错时不阻止操作，依赖数据库咨询锁和条件更新
		return true
	}
	return ok
}

func (s *ResetCardService) releaseUseLock(ctx context.Context, key string) {
	if s.cache == nil {
		return
	}
	_ = s.cache.ReleaseRedeemLock(ctx, key)
}

func (s *ResetCardService) invalidateBalanceCaches(ctx context.Context, userID int64) {
	invalidateResetCardBalanceCaches(ctx, s.authCacheInvalidator, s.billingCacheService, userID)
}

// invalidateResetCardBalanceCaches 与兑换码一致：认证缓存同步失效，余额计费缓存异步失效。
func invalidateResetCardBalanceCaches(ctx context.Context, auth APIKeyAuthCacheInvalidator, billing *BillingCacheService, userID int64) {
	if auth != nil {
		auth.InvalidateAuthCacheByUserID(ctx, userID)
	}
	if billing == nil {
		return
	}
	go func() {
		cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := billing.InvalidateUserBalance(cacheCtx, userID); err != nil {
			logger.LegacyPrintf("service.reset_card", "invalidate user balance cache failed: user_id=%d err=%v", userID, err)
		}
	}()
}

// ---------- 小工具 ----------

func buildResetCardEstimate(daily []float64, percent, maxAmount float64) ResetCardEstimate {
	out := ResetCardEstimate{Daily: make([]float64, resetCardSpendDays)}
	base := 0.0
	for i := 0; i < resetCardSpendDays && i < len(daily); i++ {
		v := daily[i]
		if v < 0 {
			v = 0
		}
		out.Daily[i] = roundResetCardAmount8(v)
		base += v
	}
	out.BaseSpend = roundResetCardAmount8(base)
	raw := base * percent / 100
	out.Capped = raw > maxAmount
	out.Amount = resetCardRefundAmount(base, percent, maxAmount)
	return out
}

// resetCardRefundAmount = min(消费 × 比例, 上限)，四舍五入到 0.01，且不超过上限。
func resetCardRefundAmount(spend, percent, maxAmount float64) float64 {
	if spend <= 0 || percent <= 0 || maxAmount <= 0 {
		return 0
	}
	raw := spend * percent / 100
	if raw > maxAmount {
		raw = maxAmount
	}
	amount := math.Round(raw*100) / 100
	if amount > maxAmount {
		amount = math.Floor(maxAmount*100) / 100
	}
	if amount < 0 {
		return 0
	}
	return amount
}

func resetCardDisplayStatus(status string, expiresAt, now time.Time) string {
	if status == ResetCardStatusUnused && !expiresAt.After(now) {
		return ResetCardStatusExpired
	}
	return status
}

func resetCardGroupRefs(ids []int64, names map[int64]string) []ResetCardGroupRef {
	out := make([]ResetCardGroupRef, 0, len(ids))
	for _, id := range ids {
		out = append(out, ResetCardGroupRef{ID: id, Name: names[id]})
	}
	return out
}

func resetCardBatchView(rec *ResetCardBatchRecord, names map[int64]string) ResetCardBatchView {
	return ResetCardBatchView{
		ID:              rec.ID,
		Name:            rec.Name,
		Groups:          resetCardGroupRefs(rec.GroupIDs, names),
		Percent:         rec.Percent,
		MaxAmount:       rec.MaxAmount,
		CardValidDays:   rec.CardValidDays,
		CreditValidDays: rec.CreditValidDays,
		TargetMode:      rec.TargetMode,
		OnlyActive:      rec.OnlyActive,
		CreatedAt:       rec.CreatedAt,
		Stats:           rec.Stats,
	}
}

func collectResetCardGroupIDs(records []ResetCardRecord) []int64 {
	ids := make([]int64, 0)
	for i := range records {
		ids = append(ids, records[i].GroupIDs...)
	}
	return dedupePositiveIDs(ids)
}

func dedupePositiveIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// generateResetCardRecordCode 生成余额记录的 code：RSC + 24 位十六进制，共 27 位（redeem_codes.code 上限 32）。
func generateResetCardRecordCode() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate record code: %w", err)
	}
	return resetCardBalanceRecordCode + strings.ToUpper(hex.EncodeToString(b)), nil
}
