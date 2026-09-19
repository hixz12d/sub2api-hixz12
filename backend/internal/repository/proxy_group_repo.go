package repository

import (
	"context"
	"encoding/json"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// All writers use the same transaction-scoped lock as the capacity trigger.
func lockProxyAllocation(ctx context.Context, exec sqlExecutor) error {
	_, err := exec.ExecContext(ctx, `SELECT pg_advisory_xact_lock(731204, 1)`)
	return err
}

func (r *proxyRepository) ListProxyGroups(ctx context.Context) ([]service.ProxyGroup, error) {
	rows, err := r.sql.QueryContext(ctx, `
 SELECT g.id, g.name, g.max_accounts_per_proxy,
 COALESCE(jsonb_agg(p.id ORDER BY p.id) FILTER (WHERE p.id IS NOT NULL), '[]'),
 COALESCE(jsonb_agg(p.id ORDER BY p.id) FILTER (WHERE p.id IS NOT NULL
   AND p.status = 'active' AND (p.expires_at IS NULL OR p.expires_at > NOW())
   AND (SELECT count(*) FROM accounts a WHERE a.proxy_id = p.id AND a.deleted_at IS NULL
        AND a.parent_account_id IS NULL) < g.max_accounts_per_proxy), '[]')
 FROM proxy_groups g LEFT JOIN proxy_group_members m ON m.group_id = g.id
 LEFT JOIN proxies p ON p.id = m.proxy_id AND p.deleted_at IS NULL
 GROUP BY g.id ORDER BY g.name, g.id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	groups := make([]service.ProxyGroup, 0)
	for rows.Next() {
		var g service.ProxyGroup
		var members, available []byte
		if err := rows.Scan(&g.ID, &g.Name, &g.MaxAccountsPerProxy, &members, &available); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(members, &g.ProxyIDs); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(available, &g.AvailableProxyIDs); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

func (r *proxyRepository) SaveProxyGroup(ctx context.Context, g *service.ProxyGroup) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	exec := tx.Client()
	if err := lockProxyAllocation(ctx, exec); err != nil {
		return err
	}
	members, err := json.Marshal(g.ProxyIDs)
	if err != nil {
		return err
	}
	var count int
	if err := scanSingleRow(ctx, exec, `SELECT count(*) FROM proxies WHERE deleted_at IS NULL
   AND id IN (SELECT value::bigint FROM jsonb_array_elements_text($1::jsonb))`, []any{string(members)}, &count); err != nil {
		return err
	}
	if count != len(g.ProxyIDs) {
		return service.ErrProxyNotFound
	}
	if err := scanSingleRow(ctx, exec, `SELECT count(*) FROM (
   SELECT proxy_id FROM accounts WHERE deleted_at IS NULL AND parent_account_id IS NULL
   AND proxy_id IN (SELECT value::bigint FROM jsonb_array_elements_text($1::jsonb))
   GROUP BY proxy_id HAVING count(*) > $2) full_proxies`, []any{string(members), g.MaxAccountsPerProxy}, &count); err != nil {
		return err
	}
	if count > 0 {
		return service.ErrProxyGroupCapacity
	}
	if g.ID == 0 {
		err = scanSingleRow(ctx, exec, `INSERT INTO proxy_groups(name, max_accounts_per_proxy) VALUES ($1,$2) RETURNING id`, []any{g.Name, g.MaxAccountsPerProxy}, &g.ID)
	} else {
		var res interface{ RowsAffected() (int64, error) }
		res, err = exec.ExecContext(ctx, `UPDATE proxy_groups SET name=$2, max_accounts_per_proxy=$3 WHERE id=$1`, g.ID, g.Name, g.MaxAccountsPerProxy)
		if err == nil {
			n, e := res.RowsAffected()
			if e != nil {
				return e
			}
			if n == 0 {
				return service.ErrProxyGroupNotFound
			}
		}
	}
	if err != nil {
		return translatePersistenceError(err, nil, infraerrors.Conflict("PROXY_GROUP_NAME_EXISTS", "代理分组名称已存在"))
	}
	if _, err := exec.ExecContext(ctx, `DELETE FROM proxy_group_members WHERE group_id=$1`, g.ID); err != nil {
		return err
	}
	if _, err := exec.ExecContext(ctx, `INSERT INTO proxy_group_members(proxy_id, group_id)
 SELECT value::bigint, $2 FROM jsonb_array_elements_text($1::jsonb)
 ON CONFLICT(proxy_id) DO UPDATE SET group_id=EXCLUDED.group_id`, string(members), g.ID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *proxyRepository) DeleteProxyGroup(ctx context.Context, id int64) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockProxyAllocation(ctx, tx.Client()); err != nil {
		return err
	}
	res, err := tx.Client().ExecContext(ctx, `DELETE FROM proxy_groups WHERE id=$1`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return service.ErrProxyGroupNotFound
	}
	return tx.Commit()
}

// resolveAccountProxyGroup runs inside the same transaction as the account write.
// A preview proxy may be reused (OAuth login); a full/stale preview is replaced.
func resolveAccountProxyGroup(ctx context.Context, client *dbent.Client, account *service.Account) error {
	if account.ProxyGroupID == nil {
		return nil
	}
	if *account.ProxyGroupID <= 0 {
		return service.ErrProxyGroupNotFound
	}
	if err := lockProxyAllocation(ctx, client); err != nil {
		return err
	}
	var exists bool
	if err := scanSingleRow(ctx, client, `SELECT EXISTS(SELECT 1 FROM proxy_groups WHERE id=$1)`, []any{*account.ProxyGroupID}, &exists); err != nil {
		return err
	}
	if !exists {
		return service.ErrProxyGroupNotFound
	}
	rows, err := client.QueryContext(ctx, `SELECT p.id FROM proxies p
 JOIN proxy_group_members m ON m.proxy_id=p.id JOIN proxy_groups g ON g.id=m.group_id
 WHERE g.id=$1 AND p.deleted_at IS NULL AND p.status='active'
 AND (p.expires_at IS NULL OR p.expires_at > NOW())
 AND (SELECT count(*) FROM accounts a WHERE a.proxy_id=p.id AND a.deleted_at IS NULL
      AND a.parent_account_id IS NULL AND a.id <> $2) < g.max_accounts_per_proxy
 ORDER BY (p.id IS NOT DISTINCT FROM $3::bigint) DESC, random() LIMIT 1`, *account.ProxyGroupID, account.ID, account.ProxyID)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return err
		}
		return service.ErrProxyGroupFull
	}
	var id int64
	if err := rows.Scan(&id); err != nil {
		return err
	}
	account.ProxyID = &id
	return rows.Err()
}
