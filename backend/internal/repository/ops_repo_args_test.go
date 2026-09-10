//go:build unit

package repository

import (
	"database/sql"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpsInsertErrorLogArgsPreservesExplicitZeroUpstreamStatus(t *testing.T) {
	zero := 0
	args := opsInsertErrorLogArgs(&service.OpsInsertErrorLogInput{UpstreamStatusCode: &zero})

	require.Len(t, args, 39)
	encoded, ok := args[27].(sql.NullInt64)
	require.True(t, ok)
	require.True(t, encoded.Valid)
	require.Zero(t, encoded.Int64)
}

func TestOpsRequestOriginArgument(t *testing.T) {
	for _, origin := range []service.RequestOrigin{"", service.RequestOriginBusiness, service.RequestOriginAvailabilityProbe, service.RequestOriginCapabilityDetector, service.RequestOriginUserDetector, service.RequestOriginLegacyUnknown} {
		args := opsInsertErrorLogArgs(&service.OpsInsertErrorLogInput{RequestOrigin: origin})
		require.Len(t, args, 39)
		value := args[38].(sql.NullString)
		require.Equal(t, origin != "", value.Valid)
		require.Equal(t, string(origin), value.String)
	}
	require.Contains(t, insertOpsErrorLogSQL, "request_origin")
	require.Contains(t, insertOpsErrorLogSQL, "$39")
}

func TestOpsNullableIntPointerDistinguishesNilZeroAndStatus(t *testing.T) {
	missing := opsNullableIntPointer(nil).(sql.NullInt64)
	require.False(t, missing.Valid)

	zeroValue := 0
	zero := opsNullableIntPointer(&zeroValue).(sql.NullInt64)
	require.True(t, zero.Valid)
	require.Zero(t, zero.Int64)

	statusValue := 503
	status := opsNullableIntPointer(&statusValue).(sql.NullInt64)
	require.True(t, status.Valid)
	require.EqualValues(t, 503, status.Int64)
}
