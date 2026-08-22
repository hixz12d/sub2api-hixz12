package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChannelMonitorIntervalCapMigration(t *testing.T) {
	content, err := FS.ReadFile("229_channel_monitor_interval_cap.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")

	require.Contains(t, sql, "channel_monitors_interval_check")
	require.Contains(t, sql, "DROP CONSTRAINT IF EXISTS channel_monitors_interval_check")
	require.Contains(t, sql, "CHECK (interval_seconds BETWEEN 15 AND 9600)")
	require.Contains(t, sql, "position('9600' IN constraint_def) = 0")
}
