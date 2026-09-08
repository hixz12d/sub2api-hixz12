//go:build integration

package repository

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestN3BillingRollbackAndConcurrentDedup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)
	user := mustCreateUser(t, client, &service.User{Email: uuid.NewString() + "@example.com", PasswordHash: "fixture", Balance: 100})
	key := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "fixture-" + uuid.NewString(), Name: "isolated", Quota: 100})
	cmd := service.UsageBillingCommand{RequestID: uuid.NewString(), UserID: user.ID, APIKeyID: key.ID, AccountID: -999, AccountType: service.AccountTypeOAuth, BalanceCost: 1.25, APIKeyQuotaCost: 1.25, AccountQuotaCost: 1}
	_, err := repo.Apply(ctx, &cmd)
	require.Error(t, err, "missing account must fail after balance and key updates")
	check := func(balance, used float64, count int) {
		t.Helper()
		var gotBalance, gotUsed float64
		var gotCount int
		require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance FROM users WHERE id=$1", user.ID).Scan(&gotBalance))
		require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT quota_used FROM api_keys WHERE id=$1", key.ID).Scan(&gotUsed))
		require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT count(*) FROM usage_billing_dedup WHERE request_id=$1 AND api_key_id=$2", cmd.RequestID, key.ID).Scan(&gotCount))
		require.InDelta(t, balance, gotBalance, 0.000001)
		require.InDelta(t, used, gotUsed, 0.000001)
		require.Equal(t, count, gotCount)
	}
	check(100, 0, 0)
	cmd.AccountQuotaCost = 0
	cmd.RequestFingerprint = ""
	type outcome struct {
		applied bool
		err     error
	}
	results := make(chan outcome, 12)
	start := make(chan struct{})
	for range 12 {
		go func() {
			<-start
			own := cmd
			result, err := repo.Apply(ctx, &own)
			results <- outcome{applied: result != nil && result.Applied, err: err}
		}()
	}
	close(start)
	applied := 0
	for range 12 {
		result := <-results
		require.NoError(t, result.err)
		if result.applied {
			applied++
		}
	}
	require.Equal(t, 1, applied)
	check(98.75, 1.25, 1)
}

func TestN3AffinityConflictRollsBackSessionUpgrade(t *testing.T) {
	ctx := context.Background()
	repo := NewOpenAIAffinityRepository(integrationDB)
	client := testEntClient(t)
	a := mustCreateAccount(t, client, &service.Account{Name: uuid.NewString(), Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth})
	b := mustCreateAccount(t, client, &service.Account{Name: uuid.NewString(), Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth})
	id := service.SessionIdentity{OwnerScopeHash: uuid.NewString(), Provider: "openai", NamespaceHash: uuid.NewString(), PrimaryHash: uuid.NewString(), Strength: service.AffinityWeak, Source: "acceptance"}
	expiry := time.Now().Add(time.Hour)
	session, _, err := repo.CreateOrGetSession(ctx, id, b.ID, expiry)
	require.NoError(t, err)
	responseHash := uuid.NewString()
	withoutSession := id
	withoutSession.PrimaryHash = ""
	_, err = repo.BindResponseAndUpgrade(ctx, withoutSession, responseHash, a.ID, expiry, expiry)
	require.NoError(t, err)
	_, err = repo.BindResponseAndUpgrade(ctx, id, responseHash, b.ID, expiry, expiry)
	require.ErrorIs(t, err, service.ErrOpenAIAffinityConflict)
	current, err := repo.ResolveSession(ctx, id.OwnerScopeHash, id.Provider, id.NamespaceHash, id.PrimaryHash, nil, time.Now(), 0, 0)
	require.NoError(t, err)
	require.Equal(t, session.Version, current.Version)
	require.Equal(t, session.Strength, current.Strength)
	require.False(t, current.Stateful)
	response, err := repo.ResolveResponse(ctx, id.OwnerScopeHash, id.Provider, responseHash, time.Now(), 0, 0)
	require.NoError(t, err)
	require.Equal(t, a.ID, response.AccountID)
}

func TestN3AffinityConcurrentDifferentOwnersFailClosed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	repo := NewOpenAIAffinityRepository(integrationDB)
	client := testEntClient(t)
	a := mustCreateAccount(t, client, &service.Account{Name: uuid.NewString(), Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth})
	b := mustCreateAccount(t, client, &service.Account{Name: uuid.NewString(), Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth})
	id := service.SessionIdentity{OwnerScopeHash: uuid.NewString(), Provider: "openai", NamespaceHash: uuid.NewString()}
	responseHash := uuid.NewString()
	name := "n3_delay_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	// Widen only this fixture's insert window so both transactions observe no row.
	_, err := integrationDB.ExecContext(ctx, fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_sleep(0.3); RETURN NEW; END $$`, name))
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = integrationDB.ExecContext(context.Background(), "DROP FUNCTION "+name+"() CASCADE") })
	_, err = integrationDB.ExecContext(ctx, fmt.Sprintf(`CREATE TRIGGER %s BEFORE INSERT ON gateway_response_bindings FOR EACH ROW WHEN (NEW.response_key_hash = '%s') EXECUTE FUNCTION %s()`, name, responseHash, name))
	require.NoError(t, err)
	type outcome struct {
		requested int64
		binding   *service.OpenAIResponseBinding
		err       error
	}
	results := make(chan outcome, 2)
	start := make(chan struct{})
	for _, accountID := range []int64{a.ID, b.ID} {
		go func(accountID int64) {
			<-start
			expiry := time.Now().Add(time.Hour)
			binding, err := repo.BindResponseAndUpgrade(ctx, id, responseHash, accountID, expiry, expiry)
			results <- outcome{accountID, binding, err}
		}(accountID)
	}
	close(start)
	successes := 0
	for range 2 {
		result := <-results
		if result.err != nil {
			require.ErrorIs(t, result.err, service.ErrOpenAIAffinityConflict)
			continue
		}
		successes++
		require.Equal(t, result.requested, result.binding.AccountID, "never report another account's binding as success")
	}
	require.Equal(t, 1, successes)
}
