package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildRecordUsageLogPreservesRequestOrigin(t *testing.T) {
	for _, origin := range []RequestOrigin{RequestOriginBusiness, RequestOriginAvailabilityProbe, RequestOriginCapabilityDetector, RequestOriginUserDetector} {
		t.Run(string(origin), func(t *testing.T) {
			ctx, err := WithRequestOrigin(context.Background(), origin)
			require.NoError(t, err)
			result := &ForwardResult{Model: "fixture-model"}
			result.Usage.InputTokens = 100
			result.Usage.OutputTokens = 80
			cost := &CostBreakdown{InputCost: 0.1, OutputCost: 0.2, TotalCost: 0.3, ActualCost: 0.6}
			s := &GatewayService{}
			row := s.buildRecordUsageLog(ctx, &recordUsageCoreInput{}, result, &APIKey{ID: 1}, &User{ID: 2}, &Account{ID: 3}, nil, "fixture-model", 2, 1, 1, 0, false, cost)
			require.NotNil(t, row.RequestOrigin)
			require.Equal(t, string(origin), *row.RequestOrigin)
			require.Equal(t, 100, row.InputTokens)
			require.Equal(t, 80, row.OutputTokens)
			require.Equal(t, 0.3, row.TotalCost)
			require.Equal(t, 0.6, row.ActualCost)
			require.Nil(t, row.MonitorObservationVersion)
			require.Nil(t, row.MonitorOutputTPSMilli)
			require.Nil(t, row.MonitorInputTokensTotal)
			require.Nil(t, row.MonitorCacheReadTokens)
		})
	}
}
