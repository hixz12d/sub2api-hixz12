//go:build unit

package service

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCapabilityRuntimeModelKeys(t *testing.T) {
	var config SiteCapabilityRuntimeConfig
	require.NoError(t, decodeCapabilityRuntimeConfig([]byte(`{"prices":{"gpt-5.4":{"input_token_limit":1000,"input_micros_per_token":1,"output_micros_per_token":2,"fixed_micros":0}}}`), &config))
	require.EqualValues(t, 1000, config.Prices["gpt-5.4"].InputTokenLimit)
	for _, raw := range []string{`null`, `{} {}`, `{"secret":"value"}`, `{"prices":{"gpt-5.4":{"arbitrary":1}}}`} {
		require.Error(t, decodeCapabilityRuntimeConfig([]byte(raw), &config))
	}
}
