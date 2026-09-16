//go:build integration

package repository

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func proxyGroupFixture(t *testing.T, capacity, proxyCount int) (*proxyRepository, *accountRepository, *service.ProxyGroup) {
	t.Helper()
	ctx := context.Background()
	pr := newProxyRepositoryWithSQL(testEntClient(t), integrationDB)
	ar := newAccountRepositoryWithSQL(testEntClient(t), integrationDB, nil)
	group := &service.ProxyGroup{Name: fmt.Sprintf("proxy-pool-%d", time.Now().UnixNano()), MaxAccountsPerProxy: capacity, ProxyIDs: []int64{}}
	for i := 0; i < proxyCount; i++ {
		p := &service.Proxy{Name: fmt.Sprintf("egress-%d", i), Protocol: "socks5", Host: "127.0.0.1", Port: 18000 + i, Status: service.StatusActive, FallbackMode: service.FallbackModeNone, ExpiryWarnDays: 7}
		require.NoError(t, pr.Create(ctx, p))
		group.ProxyIDs = append(group.ProxyIDs, p.ID)
	}
	require.NoError(t, pr.SaveProxyGroup(ctx, group))
	t.Cleanup(func() {
		for _, id := range group.ProxyIDs {
			_, _ = integrationDB.Exec(`DELETE FROM accounts WHERE proxy_id=$1`, id)
		}
		_, _ = integrationDB.Exec(`DELETE FROM proxy_groups WHERE id=$1`, group.ID)
		for _, id := range group.ProxyIDs {
			_, _ = integrationDB.Exec(`DELETE FROM proxies WHERE id=$1`, id)
		}
	})
	return pr, ar, group
}
func proxyGroupAccount(g *service.ProxyGroup) *service.Account {
	return &service.Account{Name: "pool-account", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{}, Extra: map[string]any{}, Status: service.StatusActive, Concurrency: 1, Priority: 1, Schedulable: true, ProxyGroupID: &g.ID}
}

func TestProxyGroupConcurrentAllocation(t *testing.T) {
	_, repo, group := proxyGroupFixture(t, 2, 2)
	ctx := context.Background()
	var wg sync.WaitGroup
	results := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- repo.Create(ctx, proxyGroupAccount(group)) }()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else {
			require.True(t, errors.Is(err, service.ErrProxyGroupFull), "%v", err)
		}
	}
	require.Equal(t, 4, success)
	for _, id := range group.ProxyIDs {
		var count int
		require.NoError(t, integrationDB.QueryRow(`SELECT count(*) FROM accounts WHERE proxy_id=$1`, id).Scan(&count))
		require.Equal(t, 2, count)
	}
	var outboxCount int
	require.NoError(t, integrationDB.QueryRow(`SELECT count(*) FROM scheduler_outbox o JOIN accounts a ON a.id=o.account_id WHERE a.proxy_id IN ($1,$2) AND o.event_type=$3`, group.ProxyIDs[0], group.ProxyIDs[1], service.SchedulerOutboxEventAccountChanged).Scan(&outboxCount))
	require.Equal(t, 4, outboxCount)
}

func TestProxyGroupManualWritesCannotBypassCapacity(t *testing.T) {
	_, repo, group := proxyGroupFixture(t, 2, 1)
	ctx := context.Background()
	var wg sync.WaitGroup
	results := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a := proxyGroupAccount(group)
			a.ProxyGroupID = nil
			a.ProxyID = &group.ProxyIDs[0]
			results <- repo.Create(ctx, a)
		}()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else {
			require.ErrorIs(t, err, service.ErrProxyGroupCapacity)
		}
	}
	require.Equal(t, 2, success)
}

func TestProxyGroupCapacityAndMembership(t *testing.T) {
	pr, repo, group := proxyGroupFixture(t, 1, 2)
	ctx := context.Background()
	filtered, page, err := pr.ListWithGroupFilterAndAccountCount(ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, "", "active", "", &group.ID)
	require.NoError(t, err)
	require.Len(t, filtered, 2)
	require.EqualValues(t, 2, page.Total)
	ungrouped := int64(0)
	filtered, _, err = pr.ListWithGroupFilterAndAccountCount(ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, "", "active", "", &ungrouped)
	require.NoError(t, err)
	require.Empty(t, filtered)

	first := proxyGroupAccount(group)
	first.ProxyID = &group.ProxyIDs[0]
	require.NoError(t, repo.Create(ctx, first))
	require.Equal(t, group.ProxyIDs[0], *first.ProxyID)
	second := proxyGroupAccount(group)
	second.ProxyID = &group.ProxyIDs[0]
	require.NoError(t, repo.Create(ctx, second))
	require.Equal(t, group.ProxyIDs[1], *second.ProxyID)
	first.ProxyGroupID = &group.ID
	require.NoError(t, repo.Update(ctx, first), "editing at capacity excludes the account itself")
	manual := proxyGroupAccount(group)
	manual.ProxyGroupID = nil
	manual.ProxyID = &group.ProxyIDs[0]
	require.ErrorIs(t, repo.Create(ctx, manual), service.ErrProxyGroupCapacity)
	listed, err := pr.ListProxyGroups(ctx)
	require.NoError(t, err)
	for _, g := range listed {
		if g.ID == group.ID {
			require.Empty(t, g.AvailableProxyIDs)
			require.Len(t, g.ProxyIDs, 2)
		}
	}
	require.NoError(t, pr.DeleteProxyGroup(ctx, group.ID))
	saved, err := repo.GetByID(ctx, first.ID)
	require.NoError(t, err)
	require.Equal(t, first.ProxyID, saved.ProxyID)
	require.NoError(t, repo.Create(ctx, manual), "deleting only the group removes its limit")
}

func TestProxyGroupSkipsUnavailableAndRollsBack(t *testing.T) {
	_, repo, group := proxyGroupFixture(t, 2, 3)
	ctx := context.Background()
	_, err := integrationDB.Exec(`UPDATE proxies SET status='inactive' WHERE id=$1`, group.ProxyIDs[0])
	require.NoError(t, err)
	_, err = integrationDB.Exec(`UPDATE proxies SET expires_at=NOW()-INTERVAL '1 hour' WHERE id=$1`, group.ProxyIDs[1])
	require.NoError(t, err)
	a := proxyGroupAccount(group)
	a.ProxyID = &group.ProxyIDs[0]
	require.NoError(t, repo.Create(ctx, a))
	require.Equal(t, group.ProxyIDs[2], *a.ProxyID)
	tx, err := testEntClient(t).Tx(ctx)
	require.NoError(t, err)
	txRepo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	b := proxyGroupAccount(group)
	require.NoError(t, txRepo.Create(ctx, b))
	require.NoError(t, tx.Rollback())
	c := proxyGroupAccount(group)
	require.NoError(t, repo.Create(ctx, c), "rolled back writes must release capacity")
	require.ErrorIs(t, repo.Create(ctx, proxyGroupAccount(group)), service.ErrProxyGroupFull)
	missing := proxyGroupAccount(group)
	badID := int64(9223372036854775807)
	missing.ProxyGroupID = &badID
	require.ErrorIs(t, repo.Create(ctx, missing), service.ErrProxyGroupNotFound)
}
