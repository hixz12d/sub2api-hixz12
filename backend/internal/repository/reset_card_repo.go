package repository

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

// resetCardRepository 重置卡仓储（原生 SQL，无 ent schema）。
// 所有方法都通过 clientFromContext 取执行器，可以加入调用方用 dbent.NewTxContext 开的事务。
type resetCardRepository struct {
	client *dbent.Client
}

// NewResetCardRepository 创建重置卡仓储。
func NewResetCardRepository(client *dbent.Client) service.ResetCardRepository {
	return &resetCardRepository{client: client}
}

// resetCardUseLockClass 是用卡时事务级咨询锁的第一个键，第二个键是用户 ID。
const resetCardUseLockClass = 731205

// resetCardInsertChunk 批量插卡每批条数。
const resetCardInsertChunk = 500

func (r *resetCardRepository) exec(ctx context.Context) sqlExecutor {
	return clientFromContext(ctx, r.client)
}

// spendable 条件：只统计余额计费、在所选分组、晚于"已返还到"时间点的消费。
const resetCardSpendableJoin = `
	FROM usage_logs ul
	LEFT JOIN reset_card_watermarks w ON w.user_id = ul.user_id AND w.group_id = ul.group_id
	WHERE ul.billing_type = 0
	  AND ul.group_id = ANY($GROUPS)
	  AND ul.created_at > $SINCE AND ul.created_at <= $NOW
	  AND (w.rebated_until IS NULL OR ul.created_at > w.rebated_until)`

func resetCardSpendableClause(groups, since, now string) string {
	return strings.NewReplacer("$GROUPS", groups, "$SINCE", since, "$NOW", now).Replace(resetCardSpendableJoin)
}

const resetCardSelectColumns = `
	c.id, c.batch_id, c.user_id, b.name, b.group_ids, b.percent, b.max_amount, b.credit_valid_days,
	c.status, c.expires_at, c.created_at, c.used_at, c.refund_amount, c.popup_seen_at`

func scanResetCardRecord(scan func(dest ...any) error, extra ...any) (service.ResetCardRecord, error) {
	var (
		rec         service.ResetCardRecord
		groupIDs    pq.Int64Array
		usedAt      sql.NullTime
		refund      sql.NullFloat64
		popupSeenAt sql.NullTime
	)
	dest := []any{
		&rec.ID, &rec.BatchID, &rec.UserID, &rec.BatchName, &groupIDs, &rec.Percent, &rec.MaxAmount, &rec.CreditValidDays,
		&rec.Status, &rec.ExpiresAt, &rec.CreatedAt, &usedAt, &refund, &popupSeenAt,
	}
	dest = append(dest, extra...)
	if err := scan(dest...); err != nil {
		return rec, err
	}
	rec.GroupIDs = []int64(groupIDs)
	rec.UsedAt = nullTimePtr(usedAt)
	rec.RefundAmount = nullFloatPtr(refund)
	rec.PopupSeenAt = nullTimePtr(popupSeenAt)
	return rec, nil
}

// creditScanTarget 用于 LEFT JOIN credits 时扫描可空列。
type creditScanTarget struct {
	id        sql.NullInt64
	cardID    sql.NullInt64
	userID    sql.NullInt64
	groupIDs  pq.Int64Array
	amount    sql.NullFloat64
	startsAt  sql.NullTime
	expiresAt sql.NullTime
	status    sql.NullString
	used      sql.NullFloat64
	clawback  sql.NullFloat64
	settledAt sql.NullTime
}

const resetCardCreditColumns = `cr.id, cr.card_id, cr.user_id, cr.group_ids, cr.amount, cr.starts_at, cr.expires_at, cr.status, cr.used_amount, cr.clawback_amount, cr.settled_at`

func (t *creditScanTarget) dest() []any {
	return []any{&t.id, &t.cardID, &t.userID, &t.groupIDs, &t.amount, &t.startsAt, &t.expiresAt, &t.status, &t.used, &t.clawback, &t.settledAt}
}

func (t *creditScanTarget) record() *service.ResetCardCreditRecord {
	if !t.id.Valid {
		return nil
	}
	return &service.ResetCardCreditRecord{
		ID:             t.id.Int64,
		CardID:         t.cardID.Int64,
		UserID:         t.userID.Int64,
		GroupIDs:       []int64(t.groupIDs),
		Amount:         t.amount.Float64,
		StartsAt:       t.startsAt.Time,
		ExpiresAt:      t.expiresAt.Time,
		Status:         t.status.String,
		UsedAmount:     nullFloatPtr(t.used),
		ClawbackAmount: nullFloatPtr(t.clawback),
		SettledAt:      nullTimePtr(t.settledAt),
	}
}

// ==================== 用户端 ====================

func (r *resetCardRepository) ListUserCards(ctx context.Context, userID int64, now time.Time, limit int) ([]service.ResetCardRecord, error) {
	rows, err := r.exec(ctx).QueryContext(ctx, `
		SELECT`+resetCardSelectColumns+`, `+resetCardCreditColumns+`
		FROM reset_cards c
		JOIN reset_card_batches b ON b.id = c.batch_id
		LEFT JOIN reset_card_credits cr ON cr.card_id = c.id
		WHERE c.user_id = $1
		ORDER BY (c.status = 'unused' AND c.expires_at > $2) DESC, c.created_at DESC, c.id DESC
		LIMIT $3`, userID, now, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]service.ResetCardRecord, 0)
	for rows.Next() {
		var credit creditScanTarget
		rec, err := scanResetCardRecord(rows.Scan, credit.dest()...)
		if err != nil {
			return nil, err
		}
		rec.Credit = credit.record()
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (r *resetCardRepository) GetUserCard(ctx context.Context, userID, cardID int64) (*service.ResetCardRecord, error) {
	rows, err := r.exec(ctx).QueryContext(ctx, `
		SELECT`+resetCardSelectColumns+`
		FROM reset_cards c
		JOIN reset_card_batches b ON b.id = c.batch_id
		WHERE c.id = $1 AND c.user_id = $2`, cardID, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return nil, rows.Err()
	}
	rec, err := scanResetCardRecord(rows.Scan)
	if err != nil {
		return nil, err
	}
	return &rec, rows.Err()
}

func (r *resetCardRepository) LockUserCardForUse(ctx context.Context, userID, cardID int64) (*service.ResetCardRecord, error) {
	if dbent.TxFromContext(ctx) == nil {
		return nil, errors.New("LockUserCardForUse must run inside a transaction")
	}
	exec := r.exec(ctx)
	// 同一用户的用卡串行化（即使 Redis 不可用），保证水位读写不会交错。
	if _, err := exec.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1::int, ($2::bigint % 2147483647)::int)`, resetCardUseLockClass, userID); err != nil {
		return nil, err
	}
	rows, err := exec.QueryContext(ctx, `
		SELECT`+resetCardSelectColumns+`
		FROM reset_cards c
		JOIN reset_card_batches b ON b.id = c.batch_id
		WHERE c.id = $1 AND c.user_id = $2 AND c.status = 'unused'
		FOR UPDATE OF c`, cardID, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return nil, rows.Err()
	}
	rec, err := scanResetCardRecord(rows.Scan)
	if err != nil {
		return nil, err
	}
	return &rec, rows.Err()
}

func (r *resetCardRepository) SpendableDaily(ctx context.Context, userID int64, groupIDs []int64, now time.Time) ([]float64, error) {
	daily := make([]float64, 7)
	if len(groupIDs) == 0 {
		return daily, nil
	}
	since := now.Add(-7 * 24 * time.Hour)
	rows, err := r.exec(ctx).QueryContext(ctx, `
		SELECT FLOOR(EXTRACT(EPOCH FROM ($3::timestamptz - ul.created_at)) / 86400)::int AS bucket,
		       COALESCE(SUM(ul.actual_cost), 0)
		`+resetCardSpendableClause("$2", "$4", "$3")+`
		  AND ul.user_id = $1
		GROUP BY 1`, userID, pq.Array(groupIDs), now, since)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var bucket int
		var sum float64
		if err := rows.Scan(&bucket, &sum); err != nil {
			return nil, err
		}
		if bucket < 0 {
			bucket = 0
		}
		if bucket > 6 {
			bucket = 6
		}
		// bucket 0 = 最近 24 小时；daily 从旧到新
		daily[6-bucket] += sum
	}
	return daily, rows.Err()
}

func (r *resetCardRepository) MarkCardUsed(ctx context.Context, cardID int64, usedAt time.Time, refundAmount, baseSpend float64) (bool, error) {
	res, err := r.exec(ctx).ExecContext(ctx, `
		UPDATE reset_cards
		SET status = 'used', used_at = $2, refund_amount = $3, base_spend = $4
		WHERE id = $1 AND status = 'unused'`, cardID, usedAt, refundAmount, baseSpend)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (r *resetCardRepository) UpsertWatermarks(ctx context.Context, userID int64, groupIDs []int64, until time.Time) error {
	if len(groupIDs) == 0 {
		return nil
	}
	_, err := r.exec(ctx).ExecContext(ctx, `
		INSERT INTO reset_card_watermarks (user_id, group_id, rebated_until)
		SELECT $1, g, $3 FROM unnest($2::bigint[]) AS g
		ON CONFLICT (user_id, group_id)
		DO UPDATE SET rebated_until = GREATEST(reset_card_watermarks.rebated_until, EXCLUDED.rebated_until)`,
		userID, pq.Array(groupIDs), until)
	return err
}

func (r *resetCardRepository) InsertCredit(ctx context.Context, credit *service.ResetCardCreditRecord) error {
	status := credit.Status
	if status == "" {
		status = service.ResetCardCreditStatusActive
	}
	return scanSingleRow(ctx, r.exec(ctx), `
		INSERT INTO reset_card_credits (card_id, user_id, group_ids, amount, starts_at, expires_at, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id`,
		[]any{credit.CardID, credit.UserID, pq.Array(credit.GroupIDs), credit.Amount, credit.StartsAt, credit.ExpiresAt, status},
		&credit.ID)
}

func (r *resetCardRepository) InsertBalanceRecord(ctx context.Context, userID int64, recordType string, code string, value float64, notes string, at time.Time) error {
	var notesArg any
	if notes != "" {
		notesArg = notes
	}
	_, err := r.exec(ctx).ExecContext(ctx, `
		INSERT INTO redeem_codes (code, type, value, status, used_by, used_at, notes, created_at, validity_days)
		VALUES ($1, $2, $3, 'used', $4, $5, $6, $5, 0)`,
		code, recordType, value, userID, at, notesArg)
	return err
}

func (r *resetCardRepository) GetUserBalance(ctx context.Context, userID int64) (float64, error) {
	var balance float64
	err := scanSingleRow(ctx, r.exec(ctx), `SELECT balance FROM users WHERE id = $1`, []any{userID}, &balance)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, service.ErrUserNotFound
	}
	return balance, err
}

func (r *resetCardRepository) MarkPopupSeen(ctx context.Context, userID int64, cardIDs []int64, at time.Time) (int64, error) {
	if len(cardIDs) == 0 {
		return 0, nil
	}
	res, err := r.exec(ctx).ExecContext(ctx, `
		UPDATE reset_cards SET popup_seen_at = $3
		WHERE user_id = $1 AND id = ANY($2) AND popup_seen_at IS NULL`, userID, pq.Array(cardIDs), at)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *resetCardRepository) ListUserCredits(ctx context.Context, userID int64) ([]service.ResetCardCreditRecord, error) {
	rows, err := r.exec(ctx).QueryContext(ctx, `
		SELECT `+resetCardCreditColumns+`
		FROM reset_card_credits cr
		WHERE cr.user_id = $1
		ORDER BY cr.starts_at, cr.id`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]service.ResetCardCreditRecord, 0)
	for rows.Next() {
		var t creditScanTarget
		if err := rows.Scan(t.dest()...); err != nil {
			return nil, err
		}
		if rec := t.record(); rec != nil {
			out = append(out, *rec)
		}
	}
	return out, rows.Err()
}

func (r *resetCardRepository) HourlySpend(ctx context.Context, userID int64, groupIDs []int64, from, to time.Time) ([]service.ResetCardSpendBucket, error) {
	if len(groupIDs) == 0 || !to.After(from) {
		return nil, nil
	}
	rows, err := r.exec(ctx).QueryContext(ctx, `
		SELECT ul.group_id, FLOOR(EXTRACT(EPOCH FROM ul.created_at) / 3600)::bigint AS hour_idx, COALESCE(SUM(ul.actual_cost), 0)
		FROM usage_logs ul
		WHERE ul.user_id = $1
		  AND ul.created_at >= $3 AND ul.created_at < $4
		  AND ul.group_id = ANY($2)
		  AND ul.billing_type = 0
		GROUP BY 1, 2`, userID, pq.Array(groupIDs), from, to)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]service.ResetCardSpendBucket, 0)
	for rows.Next() {
		var b service.ResetCardSpendBucket
		var hourIdx int64
		if err := rows.Scan(&b.GroupID, &hourIdx, &b.Amount); err != nil {
			return nil, err
		}
		b.Hour = time.Unix(hourIdx*3600, 0).UTC()
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *resetCardRepository) GroupNames(ctx context.Context, groupIDs []int64) (map[int64]string, error) {
	out := make(map[int64]string, len(groupIDs))
	if len(groupIDs) == 0 {
		return out, nil
	}
	rows, err := r.exec(ctx).QueryContext(ctx, `SELECT id, name FROM groups WHERE id = ANY($1) AND deleted_at IS NULL`, pq.Array(groupIDs))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out[id] = name
	}
	return out, rows.Err()
}

// ==================== 管理端 ====================

func (r *resetCardRepository) ResolveRecipients(ctx context.Context, targetMode string, onlyActive bool, groupIDs, userIDs []int64, since time.Time) ([]int64, error) {
	var (
		query string
		args  []any
	)
	switch {
	case targetMode == service.ResetCardTargetUsers:
		query = `SELECT u.id FROM users u WHERE u.id = ANY($1) AND u.deleted_at IS NULL AND u.status = 'active' ORDER BY u.id`
		args = []any{pq.Array(userIDs)}
	case onlyActive:
		query = `
			SELECT u.id FROM users u
			WHERE u.deleted_at IS NULL AND u.status = 'active'
			  AND EXISTS (
			    SELECT 1 FROM usage_logs ul
			    WHERE ul.user_id = u.id AND ul.created_at > $2
			      AND ul.group_id = ANY($1) AND ul.billing_type = 0 AND ul.actual_cost > 0
			  )
			ORDER BY u.id`
		args = []any{pq.Array(groupIDs), since}
	default:
		query = `SELECT u.id FROM users u WHERE u.deleted_at IS NULL AND u.status = 'active' ORDER BY u.id`
	}
	rows, err := r.exec(ctx).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (r *resetCardRepository) SpendableByUser(ctx context.Context, groupIDs, userIDs []int64, since, now time.Time) (map[int64]float64, error) {
	out := make(map[int64]float64)
	if len(groupIDs) == 0 {
		return out, nil
	}
	query := `SELECT ul.user_id, COALESCE(SUM(ul.actual_cost), 0)` + resetCardSpendableClause("$1", "$2", "$3")
	args := []any{pq.Array(groupIDs), since, now}
	if userIDs != nil {
		if len(userIDs) == 0 {
			return out, nil
		}
		query += ` AND ul.user_id = ANY($4)`
		args = append(args, pq.Array(userIDs))
	}
	query += ` GROUP BY ul.user_id`
	rows, err := r.exec(ctx).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int64
		var sum float64
		if err := rows.Scan(&id, &sum); err != nil {
			return nil, err
		}
		out[id] = sum
	}
	return out, rows.Err()
}

func (r *resetCardRepository) CreateBatch(ctx context.Context, batch *service.ResetCardBatchRecord, userIDs []int64, cardExpiresAt time.Time) error {
	txCtx, exec, tx, err := beginRepositoryTx(ctx, r.client)
	if err != nil {
		return err
	}
	if tx != nil {
		defer func() { _ = tx.Rollback() }()
	}
	var createdBy any
	if batch.CreatedBy != nil {
		createdBy = *batch.CreatedBy
	}
	if err := scanSingleRow(txCtx, exec, `
		INSERT INTO reset_card_batches
		  (name, group_ids, percent, max_amount, card_valid_days, credit_valid_days, target_mode, only_active, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at`,
		[]any{batch.Name, pq.Array(batch.GroupIDs), batch.Percent, batch.MaxAmount, batch.CardValidDays, batch.CreditValidDays,
			batch.TargetMode, batch.OnlyActive, createdBy},
		&batch.ID, &batch.CreatedAt); err != nil {
		return err
	}
	for start := 0; start < len(userIDs); start += resetCardInsertChunk {
		end := start + resetCardInsertChunk
		if end > len(userIDs) {
			end = len(userIDs)
		}
		if _, err := exec.ExecContext(txCtx, `
			INSERT INTO reset_cards (batch_id, user_id, expires_at)
			SELECT $1, u, $3 FROM unnest($2::bigint[]) AS u
			ON CONFLICT (batch_id, user_id) DO NOTHING`,
			batch.ID, pq.Array(userIDs[start:end]), cardExpiresAt); err != nil {
			return err
		}
	}
	if tx != nil {
		return tx.Commit()
	}
	return nil
}

const resetCardBatchColumns = `b.id, b.name, b.group_ids, b.percent, b.max_amount, b.card_valid_days, b.credit_valid_days,
	b.target_mode, b.only_active, b.created_by, b.created_at,
	s.issued, s.unused, s.used, s.expired, s.revoked, s.refunded_total, s.clawback_total`

// resetCardBatchStatsLateral 按批次汇总统计；$NOW 用于判断卡是否过期。
const resetCardBatchStatsLateral = `
	LEFT JOIN LATERAL (
		SELECT COUNT(c.id) AS issued,
		       COUNT(c.id) FILTER (WHERE c.status = 'unused' AND c.expires_at > $NOW) AS unused,
		       COUNT(c.id) FILTER (WHERE c.status = 'used') AS used,
		       COUNT(c.id) FILTER (WHERE c.status = 'unused' AND c.expires_at <= $NOW) AS expired,
		       COUNT(c.id) FILTER (WHERE c.status = 'revoked') AS revoked,
		       COALESCE(SUM(c.refund_amount), 0) AS refunded_total,
		       COALESCE(SUM(cr.clawback_amount), 0) AS clawback_total
		FROM reset_cards c
		LEFT JOIN reset_card_credits cr ON cr.card_id = c.id
		WHERE c.batch_id = b.id
	) s ON TRUE`

func scanResetCardBatch(scan func(dest ...any) error) (service.ResetCardBatchRecord, error) {
	var (
		rec       service.ResetCardBatchRecord
		groupIDs  pq.Int64Array
		createdBy sql.NullInt64
	)
	err := scan(&rec.ID, &rec.Name, &groupIDs, &rec.Percent, &rec.MaxAmount, &rec.CardValidDays, &rec.CreditValidDays,
		&rec.TargetMode, &rec.OnlyActive, &createdBy, &rec.CreatedAt,
		&rec.Stats.Issued, &rec.Stats.Unused, &rec.Stats.Used, &rec.Stats.Expired, &rec.Stats.Revoked,
		&rec.Stats.RefundedTotal, &rec.Stats.ClawbackTotal)
	if err != nil {
		return rec, err
	}
	rec.GroupIDs = []int64(groupIDs)
	if createdBy.Valid {
		v := createdBy.Int64
		rec.CreatedBy = &v
	}
	return rec, nil
}

func (r *resetCardRepository) ListBatches(ctx context.Context, offset, limit int, now time.Time) ([]service.ResetCardBatchRecord, int64, error) {
	exec := r.exec(ctx)
	var total int64
	if err := scanSingleRow(ctx, exec, `SELECT COUNT(*) FROM reset_card_batches`, nil, &total); err != nil {
		return nil, 0, err
	}
	rows, err := exec.QueryContext(ctx, `
		SELECT `+resetCardBatchColumns+`
		FROM (SELECT * FROM reset_card_batches ORDER BY created_at DESC, id DESC LIMIT $1 OFFSET $2) b`+
		strings.ReplaceAll(resetCardBatchStatsLateral, "$NOW", "$3")+`
		ORDER BY b.created_at DESC, b.id DESC`, limit, offset, now)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]service.ResetCardBatchRecord, 0)
	for rows.Next() {
		rec, err := scanResetCardBatch(rows.Scan)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, rec)
	}
	return out, total, rows.Err()
}

func (r *resetCardRepository) GetBatch(ctx context.Context, batchID int64, now time.Time) (*service.ResetCardBatchRecord, error) {
	rows, err := r.exec(ctx).QueryContext(ctx, `
		SELECT `+resetCardBatchColumns+`
		FROM reset_card_batches b`+
		strings.ReplaceAll(resetCardBatchStatsLateral, "$NOW", "$2")+`
		WHERE b.id = $1`, batchID, now)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return nil, rows.Err()
	}
	rec, err := scanResetCardBatch(rows.Scan)
	if err != nil {
		return nil, err
	}
	return &rec, rows.Err()
}

func (r *resetCardRepository) ListBatchCards(ctx context.Context, batchID int64, filter service.ResetCardAdminCardFilter, offset, limit int, now time.Time) ([]service.AdminResetCardRecord, int64, error) {
	args := []any{}
	arg := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}
	where := []string{"c.batch_id = " + arg(batchID)}
	switch filter.Status {
	case service.ResetCardStatusUnused:
		where = append(where, "c.status = 'unused' AND c.expires_at > "+arg(now))
	case service.ResetCardStatusExpired:
		where = append(where, "c.status = 'unused' AND c.expires_at <= "+arg(now))
	case service.ResetCardStatusUsed:
		where = append(where, "c.status = 'used'")
	case service.ResetCardStatusRevoked:
		where = append(where, "c.status = 'revoked'")
	}
	if filter.Search != "" {
		p := arg("%" + escapeResetCardLike(filter.Search) + "%")
		where = append(where, "(u.email ILIKE "+p+" OR u.username ILIKE "+p+")")
	}
	whereSQL := strings.Join(where, " AND ")

	exec := r.exec(ctx)
	var total int64
	if err := scanSingleRow(ctx, exec, `SELECT COUNT(*) FROM reset_cards c JOIN users u ON u.id = c.user_id WHERE `+whereSQL, args, &total); err != nil {
		return nil, 0, err
	}
	limitParam := arg(limit)
	offsetParam := arg(offset)
	rows, err := exec.QueryContext(ctx, `
		SELECT c.id, u.id, u.email, u.username, c.status, c.expires_at, c.created_at, c.used_at, c.refund_amount, `+resetCardCreditColumns+`
		FROM reset_cards c
		JOIN users u ON u.id = c.user_id
		LEFT JOIN reset_card_credits cr ON cr.card_id = c.id
		WHERE `+whereSQL+`
		ORDER BY c.id DESC
		LIMIT `+limitParam+` OFFSET `+offsetParam, args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]service.AdminResetCardRecord, 0)
	for rows.Next() {
		var (
			rec    service.AdminResetCardRecord
			usedAt sql.NullTime
			refund sql.NullFloat64
			credit creditScanTarget
		)
		dest := append([]any{&rec.ID, &rec.UserID, &rec.Email, &rec.Username, &rec.Status, &rec.ExpiresAt, &rec.CreatedAt, &usedAt, &refund}, credit.dest()...)
		if err := rows.Scan(dest...); err != nil {
			return nil, 0, err
		}
		rec.UsedAt = nullTimePtr(usedAt)
		rec.RefundAmount = nullFloatPtr(refund)
		rec.Credit = credit.record()
		out = append(out, rec)
	}
	return out, total, rows.Err()
}

func (r *resetCardRepository) RevokeBatchCards(ctx context.Context, batchID int64, at time.Time) (int64, error) {
	res, err := r.exec(ctx).ExecContext(ctx, `
		UPDATE reset_cards SET status = 'revoked', revoked_at = $2
		WHERE batch_id = $1 AND status = 'unused'`, batchID, at)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *resetCardRepository) RevokeCard(ctx context.Context, cardID int64, at time.Time) (int64, error) {
	res, err := r.exec(ctx).ExecContext(ctx, `
		UPDATE reset_cards SET status = 'revoked', revoked_at = $2
		WHERE id = $1 AND status = 'unused'`, cardID, at)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *resetCardRepository) CardExists(ctx context.Context, cardID int64) (bool, error) {
	var exists bool
	err := scanSingleRow(ctx, r.exec(ctx), `SELECT EXISTS (SELECT 1 FROM reset_cards WHERE id = $1)`, []any{cardID}, &exists)
	return exists, err
}

// ==================== 到期回收 ====================

func (r *resetCardRepository) ListDueCredits(ctx context.Context, now time.Time, limit int) ([]service.ResetCardCreditRecord, error) {
	rows, err := r.exec(ctx).QueryContext(ctx, `
		SELECT `+resetCardCreditColumns+`, b.name
		FROM reset_card_credits cr
		JOIN reset_cards c ON c.id = cr.card_id
		JOIN reset_card_batches b ON b.id = c.batch_id
		WHERE cr.status = 'active' AND cr.expires_at <= $1
		ORDER BY cr.expires_at, cr.id
		LIMIT $2`, now, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]service.ResetCardCreditRecord, 0)
	for rows.Next() {
		var t creditScanTarget
		var batchName string
		if err := rows.Scan(append(t.dest(), &batchName)...); err != nil {
			return nil, err
		}
		if rec := t.record(); rec != nil {
			rec.BatchName = batchName
			out = append(out, *rec)
		}
	}
	return out, rows.Err()
}

func (r *resetCardRepository) SettleCredit(ctx context.Context, creditID int64, usedAmount float64, at time.Time) (bool, error) {
	res, err := r.exec(ctx).ExecContext(ctx, `
		UPDATE reset_card_credits
		SET status = 'settled', used_amount = $2, settled_at = $3
		WHERE id = $1 AND status = 'active'`, creditID, usedAmount, at)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (r *resetCardRepository) SetCreditClawback(ctx context.Context, creditID int64, clawbackAmount float64) error {
	_, err := r.exec(ctx).ExecContext(ctx, `UPDATE reset_card_credits SET clawback_amount = $2 WHERE id = $1`, creditID, clawbackAmount)
	return err
}

// ==================== 小工具 ====================

func nullTimePtr(v sql.NullTime) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time
	return &t
}

func nullFloatPtr(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	f := v.Float64
	return &f
}

func escapeResetCardLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
