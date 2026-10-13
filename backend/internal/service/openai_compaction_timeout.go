package service

import (
	"time"

	"github.com/gin-gonic/gin"
)

// Native HTTP compaction uses a separate, fixed pre-output deadline. Merely
// replaying a historical compaction item, using a high effort, or sending a
// large body must not enable it: the ingress handler marks compaction_trigger.
func (s *OpenAIGatewayService) openAICompactionFirstOutputTimeout(c *gin.Context) time.Duration {
	if s == nil || s.cfg == nil || !isOpenAINativeCompactionV2(c) {
		return 0
	}
	seconds := s.cfg.Gateway.OpenAICompactionFirstOutputTimeoutSeconds
	if seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

// Use the logical ingress time so neither account retries nor progress events
// can grant another full compaction window. Older internal callers without an
// ingress timestamp get one fixed start, recorded on their first attempt.
func openAICompactionOutputDeadline(c *gin.Context, timeout time.Duration, fallback time.Time) time.Time {
	if c == nil {
		return fallback.Add(timeout)
	}
	RecordOpenAILogicalStart(c, fallback)
	if raw, ok := c.Get(openAILogicalStartKey); ok {
		if started, ok := raw.(time.Time); ok && !started.IsZero() {
			return started.Add(timeout)
		}
	}
	return fallback.Add(timeout)
}
