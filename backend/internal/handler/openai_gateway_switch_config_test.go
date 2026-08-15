package handler

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestNewOpenAIGatewayHandlerHonorsZeroAccountSwitches(t *testing.T) {
	zero := &config.Config{}
	zero.Gateway.MaxAccountSwitches = 0
	handler := NewOpenAIGatewayHandler(nil, nil, nil, nil, nil, nil, nil, nil, zero)
	require.Zero(t, handler.maxAccountSwitches)

	configured := &config.Config{}
	configured.Gateway.MaxAccountSwitches = 4
	handler = NewOpenAIGatewayHandler(nil, nil, nil, nil, nil, nil, nil, nil, configured)
	require.Equal(t, 4, handler.maxAccountSwitches)
}
