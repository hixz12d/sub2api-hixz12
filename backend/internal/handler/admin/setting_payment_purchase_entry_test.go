package admin

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPurchaseEntrySettingsAreRecognizedAsPaymentUpdates(t *testing.T) {
	for _, payload := range []string{
		`{"payment_purchase_entry_enabled":false}`,
		`{"payment_purchase_entry_enabled":true}`,
		`{"payment_purchase_entry_perpay_linked":false}`,
		`{"payment_purchase_entry_perpay_linked":true}`,
	} {
		t.Run(payload, func(t *testing.T) {
			var req UpdateSettingsRequest
			require.NoError(t, json.Unmarshal([]byte(payload), &req))
			require.True(t, hasPaymentFields(req))
		})
	}
	require.False(t, hasPaymentFields(UpdateSettingsRequest{}))
}
