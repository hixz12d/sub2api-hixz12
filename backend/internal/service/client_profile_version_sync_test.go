package service

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/clientprofile"
	"github.com/stretchr/testify/require"
)

type clientUpdateRepoStub struct {
	SettingRepository
	mu       sync.Mutex
	values   map[string]string
	readErr  error
	writeErr error
}

func (r *clientUpdateRepoStub) GetValue(_ context.Context, key string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.readErr != nil {
		return "", r.readErr
	}
	value, ok := r.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}
func (r *clientUpdateRepoStub) GetMultiple(_ context.Context, _ []string) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return maps.Clone(r.values), r.readErr
}
func (r *clientUpdateRepoStub) CompareAndSwapSetting(_ context.Context, key string, expected *string, value string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.writeErr != nil {
		return false, r.writeErr
	}
	old, exists := r.values[key]
	if expected == nil && exists || expected != nil && (!exists || old != *expected) {
		return false, nil
	}
	if r.values == nil {
		r.values = make(map[string]string)
	}
	r.values[key] = value
	return true, nil
}
func (r *clientUpdateRepoStub) put(t *testing.T, family string, state ClientProfileUpdate) {
	t.Helper()
	data, err := json.Marshal(state)
	require.NoError(t, err)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.values == nil {
		r.values = make(map[string]string)
	}
	r.values[clientProfileUpdateKey(family)] = string(data)
}

type clientUpdateSourceFunc func(context.Context, string, string) (ClientReleaseObservation, error)

func (f clientUpdateSourceFunc) FetchClientProfileRelease(ctx context.Context, family, etag string) (ClientReleaseObservation, error) {
	return f(ctx, family, etag)
}
func testClientRelease(family, version string) *clientprofile.CompatibleRelease {
	contract, _ := clientprofile.ClientReleaseContract(family)
	return &clientprofile.CompatibleRelease{Family: family, Version: version, Tag: "v" + version, Commit: contract.BaselineCommit, ContractDigest: contract.Digest()}
}
func clientUpdateObservation(family, version string) ClientReleaseObservation {
	return ClientReleaseObservation{LatestVersion: version, ETag: `"release-v1"`, Release: testClientRelease(family, version)}
}
func readTestClientUpdate(t *testing.T, repo SettingRepository, family string) ClientProfileUpdate {
	t.Helper()
	state, _, err := readClientProfileUpdate(context.Background(), repo, family)
	require.NoError(t, err)
	return state
}

func TestClientProfileSyncPublishesAndRetainsLastGood(t *testing.T) {
	for _, family := range []string{"pi", "opencode"} {
		t.Run(family, func(t *testing.T) {
			ctx := context.Background()
			repo := &clientUpdateRepoStub{}
			settings := &SettingService{settingRepo: repo}
			svc := &OpenAICodexVersionSyncService{settingRepo: repo, settingService: settings}
			calls := 0
			source := clientUpdateSourceFunc(func(context.Context, string, string) (ClientReleaseObservation, error) {
				calls++
				return clientUpdateObservation(family, "9.2.3"), nil
			})
			svc.syncClientProfile(ctx, repo, source, family)
			state := readTestClientUpdate(t, repo, family)
			require.Equal(t, "9.2.3", state.Active.Version)
			require.NotNil(t, state.Previous)
			require.Equal(t, "current", state.Status)
			svc.syncClientProfile(ctx, repo, source, family)
			require.Equal(t, 1, calls, "restart/minute polling must not repeat a fresh check")
			for _, outcome := range []string{"failure", "needs_review", "older", "not_modified"} {
				state.CheckedAt = time.Now().Add(-7 * time.Hour)
				repo.put(t, family, state)
				source = func(_ context.Context, _ string, etag string) (ClientReleaseObservation, error) {
					switch outcome {
					case "failure":
						return ClientReleaseObservation{}, errors.New("offline")
					case "needs_review":
						return ClientReleaseObservation{LatestVersion: "9.9.0", Reason: "source_changed", ETag: `"changed"`}, nil
					case "older":
						return clientUpdateObservation(family, "9.1.0"), nil
					default:
						require.NotEmpty(t, etag)
						return ClientReleaseObservation{NotModified: true}, nil
					}
				}
				svc.syncClientProfile(ctx, repo, source, family)
				state = readTestClientUpdate(t, repo, family)
				require.Equal(t, "9.2.3", state.Active.Version, outcome)
				if outcome == "needs_review" {
					require.Equal(t, "needs_review", state.Status)
					require.Equal(t, "9.9.0", state.LatestVersion)
				}
			}
			view := settings.ClientProfileUpdates(ctx)
			view[family].Active.Version = "corrupted"
			require.Equal(t, "9.2.3", settings.ClientProfileUpdates(ctx)[family].Active.Version)
			repo.readErr = errors.New("database unavailable")
			settings.clientProfileExpires = time.Time{}
			require.Equal(t, "9.2.3", settings.ClientProfileUpdates(ctx)[family].Active.Version)
			require.Equal(t, "storage_unavailable", settings.ClientProfileUpdates(ctx)[family].Status)
		})
	}
}

func TestClientProfileSyncCASRejectsLatePublication(t *testing.T) {
	ctx := context.Background()
	repo := &clientUpdateRepoStub{}
	settings := &SettingService{settingRepo: repo}
	svc := &OpenAICodexVersionSyncService{settingRepo: repo, settingService: settings}
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		svc.syncClientProfile(ctx, repo, clientUpdateSourceFunc(func(context.Context, string, string) (ClientReleaseObservation, error) {
			close(started)
			<-release
			return clientUpdateObservation("pi", "9.2.3"), nil
		}), "pi")
	}()
	<-started
	svc.syncClientProfile(ctx, repo, clientUpdateSourceFunc(func(context.Context, string, string) (ClientReleaseObservation, error) {
		return clientUpdateObservation("pi", "9.3.0"), nil
	}), "pi")
	require.NoError(t, settings.SetClientProfileUpdatePolicy(ctx, "pi", "hold"))
	close(release)
	<-done
	state := readTestClientUpdate(t, repo, "pi")
	require.Equal(t, "9.3.0", state.Active.Version)
	require.Equal(t, "hold", state.Mode)
}

type clientUpdateCombinedSource struct {
	GitHubReleaseClient
	ClientProfileReleaseClient
}

func TestClientProfileSyncStopsActiveFetch(t *testing.T) {
	repo := &clientUpdateRepoStub{}
	started := make(chan struct{})
	source := clientUpdateSourceFunc(func(ctx context.Context, _, _ string) (ClientReleaseObservation, error) {
		close(started)
		<-ctx.Done()
		return ClientReleaseObservation{}, ctx.Err()
	})
	svc := NewOpenAICodexVersionSyncService(repo, &SettingService{settingRepo: repo}, &clientUpdateCombinedSource{ClientProfileReleaseClient: source}, time.Hour)
	svc.wg.Add(1)
	go func() { defer svc.wg.Done(); svc.syncClientProfiles() }()
	<-started
	svc.Stop()
	require.Empty(t, repo.values, "shutdown cancellation must not persist a failed publication")
}

func TestClientProfileSyncRejectsCorruptStoredRecords(t *testing.T) {
	for _, raw := range []string{"null", "{}", "[]", `{"schema":2,"mode":"auto"}`, `{"schema":1,"mode":"auto","active":{"family":"pi","version":"unknown"}}`} {
		_, err := decodeClientProfileUpdate("pi", raw)
		require.Error(t, err, raw)
	}
	repo := &clientUpdateRepoStub{}
	good := defaultClientProfileUpdate()
	good.Active = testClientRelease("pi", "9.2.3")
	repo.put(t, "pi", good)
	settings := &SettingService{settingRepo: repo}
	require.Equal(t, "9.2.3", settings.ClientProfileUpdates(context.Background())["pi"].Active.Version)
	repo.values[clientProfileUpdateKey("pi")] = "null"
	settings.clientProfileExpires = time.Time{}
	require.Equal(t, "9.2.3", settings.ClientProfileUpdates(context.Background())["pi"].Active.Version)
}

func TestClientProfileSyncRollbackHoldAndResume(t *testing.T) {
	ctx := context.Background()
	repo := &clientUpdateRepoStub{}
	settings := &SettingService{settingRepo: repo}
	state := defaultClientProfileUpdate()
	state.Active, state.Previous = testClientRelease("opencode", "9.2.3"), testClientRelease("opencode", "9.1.0")
	repo.put(t, "opencode", state)
	require.NoError(t, settings.SetClientProfileUpdatePolicy(ctx, "opencode", "rollback"))
	svc := &OpenAICodexVersionSyncService{settingRepo: repo, settingService: settings}
	svc.syncClientProfile(ctx, repo, clientUpdateSourceFunc(func(context.Context, string, string) (ClientReleaseObservation, error) {
		t.Fatal("held release must not fetch")
		return ClientReleaseObservation{}, nil
	}), "opencode")
	state = readTestClientUpdate(t, repo, "opencode")
	require.Equal(t, "9.1.0", state.Active.Version)
	require.Equal(t, "hold", state.Mode)
	require.NoError(t, settings.SetClientProfileUpdatePolicy(ctx, "opencode", "resume"))
	state = readTestClientUpdate(t, repo, "opencode")
	require.Equal(t, "auto", state.Mode)
	require.True(t, state.CheckedAt.IsZero())
	require.Empty(t, state.ETag)
	repo.writeErr = errors.New("write failed")
	svc.syncClientProfile(ctx, repo, clientUpdateSourceFunc(func(context.Context, string, string) (ClientReleaseObservation, error) {
		return clientUpdateObservation("opencode", "9.3.0"), nil
	}), "opencode")
	require.Equal(t, "9.1.0", readTestClientUpdate(t, repo, "opencode").Active.Version)
}
