package repository

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type openAIHTTP2OutcomeKey struct{}

func isOpenAIHTTP2FallbackProxyKey(key string) bool {
	return isHTTPProxyKey(key) || strings.HasPrefix(key, "socks5://") || strings.HasPrefix(key, "socks5h://")
}

// One outcome belongs to one dispatch, even if the proxy policy changes while
// its response is being read. The response context also survives body wrappers.
type openAIHTTP2Outcome struct {
	once       sync.Once
	state      *openAIHTTP2FallbackState
	generation uint64
	settings   openAIHTTP2Settings
	ctx        context.Context
	protoMajor int
	statusCode int
}

func (s *httpUpstreamService) newOpenAIHTTP2Outcome(req *http.Request, entry *upstreamClientEntry, profile service.HTTPUpstreamProfile) *openAIHTTP2Outcome {
	settings := s.resolveOpenAIHTTP2Settings()
	if profile != service.HTTPUpstreamProfileOpenAI || entry.protocolMode != upstreamProtocolModeOpenAIH2 ||
		!settings.enabled || !settings.allowProxyFallbackToHTTP1 || !isOpenAIHTTP2FallbackProxyKey(entry.proxyKey) {
		return nil
	}
	outcome := &openAIHTTP2Outcome{ctx: req.Context()}
	state := s.getOrCreateOpenAIHTTP2FallbackState(entry.proxyKey)
	state.mu.Lock()
	outcome.state = state
	outcome.generation = state.generation
	outcome.settings = settings
	state.mu.Unlock()
	return outcome
}

func (o *openAIHTTP2Outcome) report(err error) {
	if o == nil || o.state == nil || o.protoMajor != 2 || o.ctx.Err() != nil {
		return
	}
	if err != nil && !isOpenAIHTTP2StreamFailure(err) {
		return
	}
	if err == nil && (o.statusCode < 200 || o.statusCode >= 300) {
		return
	}
	o.once.Do(func() {
		s := o.state
		s.mu.Lock()
		defer s.mu.Unlock()
		// An old H2 completion cannot clear or extend a newer quarantine.
		if s.generation != o.generation || !s.fallbackUntil.IsZero() {
			return
		}
		if err == nil {
			s.expireErrorWindowLocked(time.Now(), o.settings.fallbackWindow)
			return
		}
		activated, until := s.recordFailureLocked(time.Now(), o.settings.fallbackErrorThreshold, o.settings.fallbackWindow, o.settings.fallbackTTL)
		if activated {
			// Do not log proxy URLs: they can contain authentication credentials.
			slog.Warn("openai_http2_stream_fallback_activated", "fallback_until", until.Format(time.RFC3339))
		}
	})
}

func (o *openAIHTTP2Outcome) bindResponse(req *http.Request, resp *http.Response) {
	if o == nil {
		return
	}
	o.protoMajor, o.statusCode = resp.ProtoMajor, resp.StatusCode
	responseRequest := resp.Request
	if responseRequest == nil {
		responseRequest = req
	}
	resp.Request = responseRequest.WithContext(context.WithValue(responseRequest.Context(), openAIHTTP2OutcomeKey{}, o))
	if !isOpenAIStreamingHTTPResponse(req, resp) {
		o.report(nil)
	}
}

// RecordOpenAIHTTP2ResponseOutcome uses only metadata bound by Do/DoWithTLS.
// Synthetic responses cannot be attributed by consulting the current policy.
func (s *httpUpstreamService) RecordOpenAIHTTP2ResponseOutcome(resp *http.Response, err error) {
	if resp == nil || resp.Request == nil {
		return
	}
	outcome, _ := resp.Request.Context().Value(openAIHTTP2OutcomeKey{}).(*openAIHTTP2Outcome)
	outcome.report(err)
}
