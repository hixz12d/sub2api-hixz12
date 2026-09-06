//go:build integration

package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestBulkUpdateLongContextBillingInheritanceIntegration(t *testing.T) {
	tests := []struct {
		name, targets       string
		parentValue         any
		requested, expected bool
	}{
		{name: "parent updates unselected shadow", targets: "parent", parentValue: false, requested: true, expected: true},
		{name: "parent and shadow update together", targets: "both", parentValue: true, requested: false, expected: false},
		{name: "shadow ignores conflicting request", targets: "shadow", parentValue: true, requested: false, expected: true},
		{name: "missing parent flag inherits false", targets: "shadow", requested: true, expected: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := testEntTx(t)
			ctx := dbent.NewTxContext(context.Background(), tx)
			repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
			create := func(name string, parentID *int64, extra map[string]any) *service.Account {
				account := &service.Account{Name: name, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
					Status: service.StatusActive, Extra: extra, ParentAccountID: parentID}
				if parentID != nil {
					account.QuotaDimension = service.QuotaDimensionSpark
				}
				require.NoError(t, repo.Create(ctx, account))
				stored, err := repo.GetByID(ctx, account.ID)
				require.NoError(t, err)
				return stored
			}
			parentExtra := map[string]any{"owner": "parent"}
			if tt.parentValue != nil {
				parentExtra[openAILongContextBillingExtraKey] = tt.parentValue
			}
			parent := create("parent", nil, parentExtra)
			shadow := create("shadow", &parent.ID, map[string]any{openAILongContextBillingExtraKey: !tt.expected, "owner": "shadow"})
			otherParent := create("other parent", nil, map[string]any{openAILongContextBillingExtraKey: true})
			otherShadow := create("other shadow", &otherParent.ID, map[string]any{openAILongContextBillingExtraKey: false})
			marker, err := tx.QueryContext(ctx, `SELECT COALESCE(MAX(id), 0) FROM scheduler_outbox`)
			require.NoError(t, err)
			require.True(t, marker.Next())
			var lastEventID int64
			require.NoError(t, marker.Scan(&lastEventID))
			require.NoError(t, marker.Close())

			ids := []int64{parent.ID}
			if tt.targets == "shadow" {
				ids = []int64{shadow.ID}
			}
			if tt.targets == "both" {
				ids = append(ids, shadow.ID)
			}
			disabled := service.StatusDisabled
			affected, err := repo.BulkUpdate(ctx, ids, service.AccountBulkUpdate{
				Status: &disabled, Extra: map[string]any{openAILongContextBillingExtraKey: tt.requested, "batch_mark": true},
			})
			require.NoError(t, err)
			require.EqualValues(t, len(ids), affected)
			storedParent, err := repo.GetByID(ctx, parent.ID)
			require.NoError(t, err)
			storedShadow, err := repo.GetByID(ctx, shadow.ID)
			require.NoError(t, err)
			require.Equal(t, tt.expected, storedShadow.IsOpenAILongContextBillingEnabled())
			require.Equal(t, "shadow", storedShadow.Extra["owner"])
			if tt.targets == "shadow" {
				require.Equal(t, parent.Extra, storedParent.Extra, "a shadow must not change its parent")
				require.Equal(t, service.StatusActive, storedParent.Status)
			} else {
				require.Equal(t, tt.requested, storedParent.IsOpenAILongContextBillingEnabled())
				require.Equal(t, service.StatusDisabled, storedParent.Status)
			}
			if tt.targets == "parent" {
				require.NotContains(t, storedShadow.Extra, "batch_mark", "unrelated fields must not propagate")
				require.Equal(t, service.StatusActive, storedShadow.Status)
			} else {
				require.Equal(t, true, storedShadow.Extra["batch_mark"])
				require.Equal(t, service.StatusDisabled, storedShadow.Status)
			}
			unrelated, err := repo.GetByID(ctx, otherShadow.ID)
			require.NoError(t, err)
			require.Equal(t, otherShadow.Extra, unrelated.Extra, "unrelated trees must remain untouched")
			require.Equal(t, service.StatusActive, unrelated.Status)

			// Parent propagation uses account_changed; the primary write uses account_bulk_changed.
			events, err := tx.QueryContext(ctx, `SELECT event_type, account_id, payload FROM scheduler_outbox WHERE id > $1`, lastEventID)
			require.NoError(t, err)
			defer events.Close()
			notified := map[int64]bool{}
			for events.Next() {
				var eventType string
				var accountID sql.NullInt64
				var payload []byte
				require.NoError(t, events.Scan(&eventType, &accountID, &payload))
				if eventType == service.SchedulerOutboxEventAccountChanged && accountID.Valid {
					notified[accountID.Int64] = true
				}
				if eventType == service.SchedulerOutboxEventAccountBulkChanged {
					var event struct {
						AccountIDs []int64 `json:"account_ids"`
					}
					require.NoError(t, json.Unmarshal(payload, &event))
					require.ElementsMatch(t, ids, event.AccountIDs)
					for _, id := range event.AccountIDs {
						notified[id] = true
					}
				}
			}
			require.NoError(t, events.Err())
			expectedIDs := map[int64]bool{shadow.ID: true}
			if tt.targets != "shadow" {
				expectedIDs[parent.ID] = true
			}
			require.Equal(t, expectedIDs, notified, "both selected accounts and inherited shadows need cache notifications")
		})
	}
}

func TestBulkUpdateLongContextBillingRejectsMalformedIntegration(t *testing.T) {
	tx := testEntTx(t)
	ctx := dbent.NewTxContext(context.Background(), tx)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	parent := &service.Account{Name: "malformed billing parent", Platform: service.PlatformOpenAI,
		Type: service.AccountTypeOAuth, Status: service.StatusActive, Extra: map[string]any{openAILongContextBillingExtraKey: false}}
	require.NoError(t, repo.Create(ctx, parent))
	_, err := tx.ExecContext(ctx, "SAVEPOINT before_malformed_billing")
	require.NoError(t, err)
	_, err = repo.BulkUpdate(ctx, []int64{parent.ID}, service.AccountBulkUpdate{
		Extra: map[string]any{openAILongContextBillingExtraKey: "true"},
	})
	require.ErrorContains(t, err, "must be a boolean")
	_, err = tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT before_malformed_billing")
	require.NoError(t, err)
	stored, err := repo.GetByID(ctx, parent.ID)
	require.NoError(t, err)
	require.False(t, stored.IsOpenAILongContextBillingEnabled())
}
