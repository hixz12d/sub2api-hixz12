package repository

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUserErrorCategoryFilter_OtherRetainsOwnershipAndStatus(t *testing.T) {
	userID := int64(42)
	where, args := buildOpsErrorLogsWhere(&service.OpsErrorLogFilter{UserID: &userID, UserErrorCategory: "other", View: "all"})
	require.Contains(t, where, "e.user_id = $")
	require.Contains(t, where, "COALESCE(e.status_code, 0) >= 400")
	require.Contains(t, where, "CASE e.error_phase")
	require.Contains(t, where, "WHEN 'request' THEN CASE e.error_type")
	require.Contains(t, where, "ELSE 'other' END) = $")
	require.Equal(t, []any{userID, "other"}, args)
	unfiltered, _ := buildOpsErrorLogsWhere(&service.OpsErrorLogFilter{UserID: &userID, View: "all"})
	require.NotEqual(t, unfiltered, where)
}

func TestUserErrorCategoryFilter_BindsCategoryInsteadOfSQL(t *testing.T) {
	value := "other' OR true --"
	where, args := buildOpsErrorLogsWhere(&service.OpsErrorLogFilter{UserErrorCategory: value})
	require.NotContains(t, where, value)
	require.Contains(t, args, value)
}
