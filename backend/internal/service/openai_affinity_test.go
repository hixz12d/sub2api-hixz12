package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type memoryOpenAIAffinityRepo struct {
	mu             sync.Mutex
	nextID         int64
	sessions       map[string]*OpenAISessionBinding
	aliases        map[string]*OpenAISessionBinding
	responses      map[string]*OpenAIResponseBinding
	migrations     []OpenAIAffinityMigrationCandidate
	migratedFrom   int64
	migratedTo     int64
	onBindResponse func()
}

func newMemoryOpenAIAffinityRepo() *memoryOpenAIAffinityRepo {
	return &memoryOpenAIAffinityRepo{
		sessions:  make(map[string]*OpenAISessionBinding),
		aliases:   make(map[string]*OpenAISessionBinding),
		responses: make(map[string]*OpenAIResponseBinding),
	}
}

func affinitySessionMapKey(owner, provider, namespace, hash string) string {
	return owner + "|" + provider + "|" + namespace + "|" + hash
}

func affinityResponseMapKey(owner, provider, hash string) string {
	return owner + "|" + provider + "|" + hash
}

func cloneOpenAISessionBinding(value *OpenAISessionBinding) *OpenAISessionBinding {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func cloneOpenAIResponseBinding(value *OpenAIResponseBinding) *OpenAIResponseBinding {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func (r *memoryOpenAIAffinityRepo) ResolveResponse(_ context.Context, owner, provider, hash string, now time.Time, refreshTTL, _ time.Duration) (*OpenAIResponseBinding, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	value := r.responses[affinityResponseMapKey(owner, provider, hash)]
	if value == nil || !value.ExpiresAt.After(now) {
		return nil, ErrOpenAIAffinityNotFound
	}
	if refreshTTL > 0 {
		value.LastHitAt = now
		value.ExpiresAt = now.Add(refreshTTL)
		value.Version++
	}
	return cloneOpenAIResponseBinding(value), nil
}

func (r *memoryOpenAIAffinityRepo) ResolveSession(_ context.Context, owner, provider, namespace, primary string, aliases []string, now time.Time, refreshTTL, _ time.Duration) (*OpenAISessionBinding, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	value := r.sessions[affinitySessionMapKey(owner, provider, namespace, primary)]
	if value == nil {
		for _, alias := range aliases {
			value = r.aliases[affinitySessionMapKey(owner, provider, namespace, alias)]
			if value != nil {
				break
			}
		}
	}
	if value == nil || !value.ExpiresAt.After(now) {
		return nil, ErrOpenAIAffinityNotFound
	}
	if refreshTTL > 0 {
		value.LastHitAt = now
		value.ExpiresAt = now.Add(refreshTTL)
		value.Version++
	}
	return cloneOpenAISessionBinding(value), nil
}

func (r *memoryOpenAIAffinityRepo) CreateOrGetSession(_ context.Context, identity SessionIdentity, accountID int64, expiresAt time.Time) (*OpenAISessionBinding, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := affinitySessionMapKey(identity.OwnerScopeHash, identity.Provider, identity.NamespaceHash, identity.PrimaryHash)
	if value := r.sessions[key]; value != nil {
		return cloneOpenAISessionBinding(value), false, nil
	}
	for _, alias := range identity.Aliases {
		if value := r.aliases[affinitySessionMapKey(identity.OwnerScopeHash, identity.Provider, identity.NamespaceHash, alias)]; value != nil {
			return cloneOpenAISessionBinding(value), false, nil
		}
	}
	r.nextID++
	now := time.Now().UTC()
	value := &OpenAISessionBinding{ID: r.nextID, OwnerScopeHash: identity.OwnerScopeHash, Provider: identity.Provider,
		NamespaceHash: identity.NamespaceHash, PrimaryHash: identity.PrimaryHash, AccountID: accountID,
		Strength: identity.Strength, Source: identity.Source, Stateful: identity.Stateful,
		Capability: identity.Capability, CreatedAt: now, UpdatedAt: now, LastHitAt: now,
		ExpiresAt: expiresAt, Version: 1}
	r.sessions[key] = value
	for _, alias := range identity.Aliases {
		r.aliases[affinitySessionMapKey(identity.OwnerScopeHash, identity.Provider, identity.NamespaceHash, alias)] = value
	}
	return cloneOpenAISessionBinding(value), true, nil
}

func (r *memoryOpenAIAffinityRepo) BindResponseAndUpgrade(_ context.Context, identity SessionIdentity, responseHash string, accountID int64, responseExpiresAt, strongExpiresAt time.Time) (*OpenAIResponseBinding, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.onBindResponse != nil {
		r.onBindResponse()
	}
	key := affinityResponseMapKey(identity.OwnerScopeHash, identity.Provider, responseHash)
	if value := r.responses[key]; value != nil && value.AccountID != accountID {
		return nil, ErrOpenAIAffinityConflict
	}
	session := r.sessions[affinitySessionMapKey(identity.OwnerScopeHash, identity.Provider, identity.NamespaceHash, identity.PrimaryHash)]
	if session == nil {
		for _, alias := range identity.Aliases {
			session = r.aliases[affinitySessionMapKey(identity.OwnerScopeHash, identity.Provider, identity.NamespaceHash, alias)]
			if session != nil {
				break
			}
		}
	}
	if session != nil {
		if session.AccountID != accountID {
			return nil, ErrOpenAIAffinityConflict
		}
		session.Strength = AffinityStrong
		session.Stateful = true
		session.ExpiresAt = strongExpiresAt
		session.Version++
	}
	r.nextID++
	now := time.Now().UTC()
	value := &OpenAIResponseBinding{ID: r.nextID, OwnerScopeHash: identity.OwnerScopeHash, Provider: identity.Provider,
		ResponseKeyHash: responseHash, AccountID: accountID, Capability: identity.Capability,
		CreatedAt: now, LastHitAt: now, ExpiresAt: responseExpiresAt, Version: 1}
	if session != nil {
		id := session.ID
		value.SessionBindingID = &id
	}
	r.responses[key] = value
	return cloneOpenAIResponseBinding(value), nil
}

func (r *memoryOpenAIAffinityRepo) ListMigrationCandidates(_ context.Context, fromAccountID int64, _ bool, limit int, _ time.Time) ([]OpenAIAffinityMigrationCandidate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]OpenAIAffinityMigrationCandidate, 0, limit)
	for _, candidate := range r.migrations {
		if candidate.AccountID == fromAccountID && len(out) < limit {
			out = append(out, candidate)
		}
	}
	return out, nil
}

func (r *memoryOpenAIAffinityRepo) MigrateBindingCAS(_ context.Context, bindingID, fromAccountID, toAccountID, expectedVersion int64, reason string, _ time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if reason == "" {
		return errors.New("reason required")
	}
	for i := range r.migrations {
		candidate := &r.migrations[i]
		if candidate.BindingID == bindingID && candidate.AccountID == fromAccountID && candidate.Version == expectedVersion {
			candidate.AccountID = toAccountID
			candidate.Version++
			r.migratedFrom = fromAccountID
			r.migratedTo = toAccountID
			return nil
		}
	}
	return ErrOpenAIAffinityCAS
}

func newOpenAIAffinityTestContext(t *testing.T, userID, groupID int64, path string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", path, nil)
	c.Set("api_key", &APIKey{ID: userID*1000 + groupID, UserID: userID, GroupID: &groupID})
	return c
}

func openAIAffinityTestConfig() *config.Config {
	return &config.Config{Gateway: config.GatewayConfig{OpenAIAffinity: config.GatewayOpenAIAffinityConfig{
		Enabled: true, WritesEnabled: true, Secret: "0123456789abcdef0123456789abcdef",
	}}}
}

func TestOpenAIAffinityIdentityOwnerNamespaceAndState(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: openAIAffinityTestConfig()}
	body := []byte(`{"previous_response_id":"resp-secret","prompt_cache_key":"cache-secret","input":[{"type":"function_call_output"}]}`)
	first := newOpenAIAffinityTestContext(t, 7, 11, "/v1/responses")
	first.Header("session_id", "session-secret")
	identity, err := svc.resolveOpenAISessionIdentity(first, body, "legacy-secret")
	require.NoError(t, err)
	require.Equal(t, AffinityStrong, identity.Strength)
	require.True(t, identity.Stateful)
	require.False(t, identity.ReplaySafe)
	require.NotEmpty(t, identity.PreviousResponseHash)
	require.NotContains(t, identity.PreviousResponseHash, "resp-secret")
	require.LessOrEqual(t, len(identity.Aliases), openAIAffinityMaxAliases)

	otherOwner := newOpenAIAffinityTestContext(t, 8, 11, "/v1/responses")
	ownerIdentity, err := svc.resolveOpenAISessionIdentity(otherOwner, body, "legacy-secret")
	require.NoError(t, err)
	require.NotEqual(t, identity.OwnerScopeHash, ownerIdentity.OwnerScopeHash)
	require.NotEqual(t, identity.PreviousResponseHash, ownerIdentity.PreviousResponseHash)

	otherGroup := newOpenAIAffinityTestContext(t, 7, 12, "/v1/responses")
	groupIdentity, err := svc.resolveOpenAISessionIdentity(otherGroup, body, "legacy-secret")
	require.NoError(t, err)
	require.NotEqual(t, identity.OwnerScopeHash, groupIdentity.OwnerScopeHash)
	require.NotEqual(t, identity.NamespaceHash, groupIdentity.NamespaceHash)
}

func TestOpenAIAffinityColdStartHundredRequestsChooseOneWinner(t *testing.T) {
	repo := newMemoryOpenAIAffinityRepo()
	accounts := []Account{
		{ID: 101, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 100},
		{ID: 202, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 100},
	}
	svc := &OpenAIGatewayService{openAIAffinityRepo: repo, accountRepo: stubOpenAIAccountRepo{accounts: accounts}}
	scheduler := &defaultOpenAIAccountScheduler{service: svc}
	identity := SessionIdentity{OwnerScopeHash: "owner", NamespaceHash: "namespace", PrimaryHash: "primary",
		Provider: openAIAffinityProvider, Capability: "responses", Source: "explicit_session", Strength: AffinityExplicit}
	value := openAIAffinityContextValue{Identity: identity, Enabled: true, Writable: true}
	ctx := context.WithValue(context.Background(), openAIAffinityContextKey, value)

	const count = 100
	results := make(chan int64, count)
	errs := make(chan error, count)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		candidate := &accounts[i%len(accounts)]
		wg.Add(1)
		go func(account *Account) {
			defer wg.Done()
			selection, err := scheduler.finalizeOpenAIAffinityColdWinner(ctx, OpenAIAccountScheduleRequest{Platform: PlatformOpenAI},
				&AccountSelectionResult{Account: account, Acquired: true, ReleaseFunc: func() {}})
			if err != nil {
				errs <- err
				return
			}
			results <- selection.Account.ID
		}(candidate)
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	var winner int64
	for accountID := range results {
		if winner == 0 {
			winner = accountID
		}
		require.Equal(t, winner, accountID)
	}
	require.Contains(t, []int64{101, 202}, winner)
}

func TestOpenAIAffinityResponseBindingUpgradesAndPrecedes(t *testing.T) {
	repo := newMemoryOpenAIAffinityRepo()
	svc := &OpenAIGatewayService{cfg: openAIAffinityTestConfig(), openAIAffinityRepo: repo}
	c := newOpenAIAffinityTestContext(t, 7, 11, "/v1/responses")
	identity, err := svc.resolveOpenAISessionIdentity(c, []byte(`{"prompt_cache_key":"cache"}`), "legacy")
	require.NoError(t, err)
	attachOpenAIAffinityIdentity(c, identity, true, true)
	ctx := c.Request.Context()
	binding, created, err := svc.createOrGetPersistentOpenAISession(ctx, 101)
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, AffinityWeak, binding.Strength)

	account := &Account{ID: 101, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	require.NoError(t, svc.bindPersistentOpenAIResponse(ctx, c, account, "resp-1"))
	resolvedSession, err := repo.ResolveSession(ctx, identity.OwnerScopeHash, identity.Provider, identity.NamespaceHash,
		identity.PrimaryHash, identity.Aliases, time.Now(), time.Hour, time.Minute)
	require.NoError(t, err)
	require.Equal(t, AffinityStrong, resolvedSession.Strength)

	continuation := newOpenAIAffinityTestContext(t, 7, 11, "/v1/responses")
	continuationIdentity, err := svc.resolveOpenAISessionIdentity(continuation, []byte(`{"previous_response_id":"resp-1"}`), "")
	require.NoError(t, err)
	attachOpenAIAffinityIdentity(continuation, continuationIdentity, true, true)
	responseBinding, err := svc.resolvePersistentOpenAIResponse(continuation.Request.Context())
	require.NoError(t, err)
	require.Equal(t, int64(101), responseBinding.AccountID)
}

func TestOpenAIAffinityMigrationRequiresExactPreviewDigest(t *testing.T) {
	repo := newMemoryOpenAIAffinityRepo()
	repo.migrations = []OpenAIAffinityMigrationCandidate{{BindingID: 9, AccountID: 101, Strength: AffinityStrong, Version: 3, ExpiresAt: time.Now().Add(time.Hour)}}
	tool := NewOpenAIAffinityMigrationTool(repo)
	plan, err := tool.Preview(context.Background(), 101, 202, "operator-approved drain", false)
	require.NoError(t, err)
	require.Len(t, plan.Candidates, 1)
	require.NotEmpty(t, plan.Digest)
	require.Error(t, tool.Apply(context.Background(), plan, "wrong"))
	require.NoError(t, tool.Apply(context.Background(), plan, plan.Digest))
	require.Equal(t, int64(101), repo.migratedFrom)
	require.Equal(t, int64(202), repo.migratedTo)
	require.ErrorIs(t, tool.Apply(context.Background(), plan, plan.Digest), ErrOpenAIAffinityCAS)
}

func TestOpenAIAffinityDefaultOffDoesNotCreateIdentity(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	c := newOpenAIAffinityTestContext(t, 7, 11, "/v1/responses")
	require.NoError(t, svc.prepareOpenAIAffinityIdentity(c, []byte(`{"prompt_cache_key":"cache"}`), "legacy"))
	_, enabled := openAIAffinityFromGin(c)
	require.False(t, enabled)
}

func TestOpenAIAffinityNonStreamingBindsBeforeClientOutput(t *testing.T) {
	repo := newMemoryOpenAIAffinityRepo()
	svc := &OpenAIGatewayService{cfg: openAIAffinityTestConfig(), openAIAffinityRepo: repo}
	c := newOpenAIAffinityTestContext(t, 7, 11, "/v1/responses")
	identity, err := svc.resolveOpenAISessionIdentity(c, []byte(`{"prompt_cache_key":"cache"}`), "legacy")
	require.NoError(t, err)
	attachOpenAIAffinityIdentity(c, identity, true, true)
	_, _, err = svc.createOrGetPersistentOpenAISession(c.Request.Context(), 101)
	require.NoError(t, err)
	repo.onBindResponse = func() {
		require.False(t, c.Writer.Written(), "ownership must persist before the first client-visible byte")
	}
	body := []byte(`{"id":"resp-before-output","model":"gpt-5.4","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`)
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewReader(body))}
	account := &Account{ID: 101, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	result, err := svc.handleNonStreamingResponse(c.Request.Context(), resp, c, account, "gpt-5.4", "gpt-5.4")
	require.NoError(t, err)
	require.Equal(t, "resp-before-output", result.responseID)
	require.True(t, c.Writer.Written())
}

func TestOpenAIAffinityStateConstrainsRetryBudgetBeforeReserve(t *testing.T) {
	c := newOpenAIAffinityTestContext(t, 7, 11, "/v1/responses")
	identity := SessionIdentity{Strength: AffinityStrong, Stateful: true, ReplaySafe: false}
	attachOpenAIAffinityIdentity(c, identity, true, true)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{openAIRetryBudgetV2ExtraKey: true}}
	budget := EnsureOpenAIRetryBudget(c, account, []byte(`{"input":"otherwise stateless"}`))
	require.NotNil(t, budget)
	snapshot := budget.Snapshot()
	require.Equal(t, 1, snapshot.MaxDistinctAccounts)
	require.NoError(t, budget.Reserve(101))
	require.ErrorIs(t, budget.Reserve(202), ErrOpenAIRetryBudgetExhausted)
}

func TestOpenAIAffinityStatefulLegacyReadAtomicallyAdoptsOwner(t *testing.T) {
	repo := newMemoryOpenAIAffinityRepo()
	cache := &stubGatewayCache{sessionBindings: map[string]int64{"openai:legacy-session": 101}}
	svc := &OpenAIGatewayService{openAIAffinityRepo: repo, cache: cache}
	identity := SessionIdentity{OwnerScopeHash: "owner", NamespaceHash: "namespace", PrimaryHash: "primary",
		Provider: openAIAffinityProvider, Capability: "responses", Source: "function_call_output", Strength: AffinityStrong, Stateful: true}
	ctx := context.WithValue(context.Background(), openAIAffinityContextKey,
		openAIAffinityContextValue{Identity: identity, Enabled: true, Writable: true})
	accountID, err := svc.getStickySessionAccountID(ctx, nil, "legacy-session")
	require.NoError(t, err)
	require.Equal(t, int64(101), accountID)
	binding, err := repo.ResolveSession(ctx, identity.OwnerScopeHash, identity.Provider, identity.NamespaceHash,
		identity.PrimaryHash, nil, time.Now(), time.Hour, time.Minute)
	require.NoError(t, err)
	require.Equal(t, int64(101), binding.AccountID)
	require.Equal(t, AffinityStrong, binding.Strength)
}
