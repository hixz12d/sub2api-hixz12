//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOAuthLegacyUnauthorizedPostgres(t *testing.T) {
	for _, permanent := range []bool{false, true} {
		for _, scenario := range []string{"current", "reauthorized", "outbox_failure", "manual_error"} {
			t.Run(fmt.Sprintf("permanent=%t/%s", permanent, scenario), func(t *testing.T) {
				_, before, _ := durableSyncPostgresFixture(t)
				ctx := context.Background()
				_, err := integrationDB.ExecContext(ctx, `UPDATE accounts SET status='active', error_message=NULL WHERE id=$1`, before.ID)
				require.NoError(t, err)
				accounts := newAccountRepositoryWithSQL(integrationEntClient, integrationDB, nil)
				before, err = accounts.GetByID(ctx, before.ID)
				require.NoError(t, err)
				require.Zero(t, before.GetCredentialAsInt64("_token_version"))
				switch scenario {
				case "reauthorized":
					_, err = integrationDB.ExecContext(ctx, `UPDATE accounts SET credentials=credentials || '{"access_token":"fixture-new","_token_version":1}'::jsonb WHERE id=$1`, before.ID)
				case "manual_error":
					_, err = integrationDB.ExecContext(ctx, `UPDATE accounts SET status='error', error_message='manual hold' WHERE id=$1`, before.ID)
				case "outbox_failure":
					constraint := fmt.Sprintf("legacy_401_outbox_%d", before.ID)
					_, err = integrationDB.ExecContext(ctx, fmt.Sprintf("ALTER TABLE scheduler_outbox ADD CONSTRAINT %s CHECK (account_id <> %d)", constraint, before.ID))
					t.Cleanup(func() {
						_, dropErr := integrationDB.ExecContext(ctx, "ALTER TABLE scheduler_outbox DROP CONSTRAINT "+constraint)
						require.NoError(t, dropErr)
					})
				}
				require.NoError(t, err)
				until := time.Now().Add(10 * time.Minute).Truncate(time.Microsecond)
				err = accounts.RecordOAuthUnauthorized(ctx, before, "fixture-old", permanent, until)
				if scenario == "outbox_failure" {
					require.Error(t, err)
					require.Contains(t, err.Error(), fmt.Sprintf("legacy_401_outbox_%d", before.ID))
				} else {
					require.NoError(t, err)
				}
				after, err := accounts.GetByID(ctx, before.ID)
				require.NoError(t, err)
				require.False(t, after.Schedulable, "preserve manual scheduling setting")
				require.Equal(t, before.RateLimitResetAt, after.RateLimitResetAt)
				var evidence, outbox int
				require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT count(*) FROM oauth_auth_errors WHERE account_id=$1", before.ID).Scan(&evidence))
				require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT count(*) FROM scheduler_outbox WHERE account_id=$1", before.ID).Scan(&outbox))
				require.Zero(t, evidence, "unversioned credentials must not create automatic-recovery evidence")
				if scenario == "current" {
					require.Equal(t, 1, outbox, "publish legacy state changes even without versioned evidence")
					require.Equal(t, before.Credentials, after.Credentials)
					if permanent {
						require.Equal(t, service.StatusError, after.Status)
						require.Contains(t, after.ErrorMessage, "unversioned credential")
					} else {
						require.Equal(t, service.StatusActive, after.Status)
						require.NotNil(t, after.TempUnschedulableUntil)
						require.True(t, until.Equal(*after.TempUnschedulableUntil))
						require.Contains(t, after.TempUnschedulableReason, "unversioned credential")
					}
				} else {
					require.Zero(t, outbox)
					require.Nil(t, after.TempUnschedulableUntil)
					if scenario == "manual_error" {
						require.Equal(t, service.StatusError, after.Status)
						require.Equal(t, "manual hold", after.ErrorMessage)
					} else {
						require.Equal(t, service.StatusActive, after.Status)
						require.Empty(t, after.ErrorMessage)
					}
					if scenario == "reauthorized" {
						require.Equal(t, "fixture-new", after.GetCredential("access_token"))
					}
				}
			})
		}
	}
}
