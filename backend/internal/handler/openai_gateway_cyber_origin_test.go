package handler

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestDedicatedCyberOpsPreservesOrigin(t *testing.T) {
	for _, origin := range []service.RequestOrigin{service.RequestOriginBusiness, service.RequestOriginAvailabilityProbe, service.RequestOriginCapabilityDetector, service.RequestOriginUserDetector} {
		for _, local := range []bool{true, false} {
			t.Run(string(origin)+map[bool]string{true: "/local", false: "/upstream"}[local], func(t *testing.T) {
				setupOpsErrorLogTestQueue(t, 2)
				repo := &ingressRejectOpsRepo{}
				h := &OpenAIGatewayHandler{opsService: service.NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)}
				c := newTestGinContext()
				c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
				ctx, err := service.WithRequestOrigin(c.Request.Context(), origin)
				require.NoError(t, err)
				c.Request = c.Request.WithContext(ctx)
				c.Request.Header.Set("X-Request-Origin", "real_traffic")
				if local {
					h.enqueueCyberSessionBlockedOpsEntry(c, &service.APIKey{ID: 1}, "fixture-model", "fixture-session")
				} else {
					service.MarkOpsCyberPolicy(c, service.CyberPolicyMark{Message: "fixture", UpstreamStatus: 400})
					h.recordCyberPolicyIfMarked(c, nil, nil, nil, "fixture-model", false, nil, service.ChannelUsageFields{}, "")
				}
				select {
				case job := <-opsErrorLogQueue:
					require.Equal(t, origin, job.entry.RequestOrigin)
					flushOpsErrorLogBatch([]opsErrorLogJob{job})
					require.Len(t, repo.entries, 1)
					require.Equal(t, origin, repo.entries[0].RequestOrigin)
				case <-time.After(2 * time.Second):
					t.Fatal("dedicated ops entry was not queued")
				}
				require.Eventually(t, func() bool { return OpsErrorLogEnqueuedTotal() == 1 }, time.Second, time.Millisecond)
			})
		}
	}
}
