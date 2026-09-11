package repository

import (
	"context"
	"errors"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"
)

func TestClientProfileSettingCASUsesConditionalSQL(t *testing.T) {
	for _, scenario := range []string{"insert", "duplicate", "update", "stale"} {
		t.Run(scenario, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
			t.Cleanup(func() { _ = client.Close() })
			repo := &settingRepository{client: client}
			var expected *string
			if scenario == "insert" || scenario == "duplicate" {
				query := mock.ExpectQuery(`INSERT INTO "settings" .* RETURNING "id"`)
				if scenario == "duplicate" {
					query.WillReturnError(errors.New(`duplicate key value violates unique constraint "settings_key_key"`))
				} else {
					query.WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
				}
			} else {
				old := "old"
				expected = &old
				count := int64(1)
				if scenario == "stale" {
					count = 0
				}
				mock.ExpectExec(`UPDATE "settings" SET .* WHERE "settings"\."key" = \$3 AND "settings"\."value" = \$4`).
					WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "client_profile", "old").WillReturnResult(sqlmock.NewResult(0, count))
			}
			won, err := repo.CompareAndSwapSetting(context.Background(), "client_profile", expected, "new")
			require.NoError(t, err)
			require.Equal(t, scenario == "insert" || scenario == "update", won)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
