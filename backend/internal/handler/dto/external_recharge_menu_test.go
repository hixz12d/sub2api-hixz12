package dto

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExternalRechargeMenuSettingsRoundTrip(t *testing.T) {
	raw := `[{"id":"ldxp-recharge","label":"Shop","url":"https://example.com/shop","visibility":"user","placement":"recharge","enabled":false},{"id":"docs","label":"Docs","url":"md:docs","visibility":"user"}]`
	items := ParseCustomMenuItems(raw)
	require.Len(t, items, 2)
	require.NotNil(t, items[0].Enabled)
	require.False(t, *items[0].Enabled)
	require.Equal(t, "recharge", items[0].Placement)
	require.Nil(t, items[1].Enabled)
	encoded, err := json.Marshal(items)
	require.NoError(t, err)
	restored := ParseCustomMenuItems(string(encoded))
	require.Equal(t, items, restored)
	visible := ParseUserVisibleMenuItems(string(encoded))
	require.Len(t, visible, 1)
	require.Equal(t, "docs", visible[0].ID)
	*items[0].Enabled = true
	encoded, err = json.Marshal(items)
	require.NoError(t, err)
	require.Len(t, ParseUserVisibleMenuItems(string(encoded)), 2)
}
