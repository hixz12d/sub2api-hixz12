package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRequestOriginContract(t *testing.T) {
	origins := []RequestOrigin{RequestOriginBusiness, RequestOriginAvailabilityProbe, RequestOriginCapabilityDetector, RequestOriginUserDetector, RequestOriginLegacyUnknown}
	require.Equal(t, []RequestOrigin{"real_traffic", "availability_probe", "capability_probe", "user_detector", "legacy_unknown"}, origins)
	for _, origin := range origins {
		t.Run(string(origin), func(t *testing.T) {
			ctx, err := WithRequestOrigin(context.Background(), origin)
			require.NoError(t, err)
			require.Equal(t, origin, RequestOriginFromContext(context.WithoutCancel(ctx)))
			start := time.Unix(100, 0)
			first, last := start.Add(time.Second), start.Add(3*time.Second)
			tokens := int64(80)
			o := VisibleOutputObservation{Origin: origin, RequestStartedAt: start, FirstVisibleTextAt: &first, LastVisibleTextAt: &last, VisibleOutputTokens: &tokens, OutputComplete: true, TextDeltaCount: 2, Version: VisibleStreamObservationVersion, Method: VisibleStreamObservationMethod}
			rate, err := o.OutputTPSMilli()
			require.NoError(t, err)
			if origin == RequestOriginBusiness {
				require.NotNil(t, rate)
			} else {
				require.Nil(t, rate)
			}
		})
	}
	for _, invalid := range []RequestOrigin{"", "business", "capability_detector", "REAL_TRAFFIC", "spoofed"} {
		_, err := WithRequestOrigin(context.Background(), invalid)
		require.Error(t, err)
	}
	require.Equal(t, RequestOriginBusiness, RequestOriginFromContext(nil))
}
