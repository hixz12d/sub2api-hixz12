package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpsErrorLoggerTrustedRequestOrigin(t *testing.T) {
	for _, origin := range []service.RequestOrigin{service.RequestOriginBusiness, service.RequestOriginAvailabilityProbe, service.RequestOriginCapabilityDetector, service.RequestOriginUserDetector} {
		for _, kind := range []string{"failure", "recovered", "stream_failure"} {
			t.Run(string(origin)+"/"+kind, func(t *testing.T) {
				setupOpsErrorLogTestQueue(t, 2)
				repo := &ingressRejectOpsRepo{}
				ops := service.NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
				router := gin.New()
				router.Use(OpsErrorLoggerMiddleware(ops))
				router.POST("/v1/responses", func(c *gin.Context) {
					switch kind {
					case "failure":
						c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"type": "upstream_error", "message": "failed"}})
					case "recovered":
						c.Set(service.OpsUpstreamErrorsKey, []*service.OpsUpstreamErrorEvent{{UpstreamStatusCode: 429, Message: "retry"}})
						c.JSON(http.StatusOK, gin.H{"status": "completed"})
					case "stream_failure":
						c.Header("Content-Type", "text/event-stream")
						c.String(http.StatusOK, "data: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\",\"error\":{\"type\":\"upstream_error\",\"message\":\"failed\"}}}\n\n")
					}
				})
				request := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
				request.Header.Set("X-Request-Origin", "capability_probe")
				if origin != service.RequestOriginBusiness {
					ctx, err := service.WithRequestOrigin(request.Context(), origin)
					require.NoError(t, err)
					request = request.WithContext(ctx)
				}
				router.ServeHTTP(httptest.NewRecorder(), request)
				require.Equal(t, int64(1), OpsErrorLogQueueLength())
				job := <-opsErrorLogQueue
				require.Equal(t, origin, job.entry.RequestOrigin)
				flushOpsErrorLogBatch([]opsErrorLogJob{job})
				require.Len(t, repo.entries, 1)
				require.Equal(t, origin, repo.entries[0].RequestOrigin, "background queue must preserve attribution")
			})
		}
	}
}
