package service

import (
	"context"
	"net/http"
	"strings"
)

type openAIStreamProxyURLContextKey struct{}

// The transport binds outcomes to a specific response, not a mutable proxy policy.
type openAIHTTP2ResponseOutcomeReporter interface {
	RecordOpenAIHTTP2ResponseOutcome(resp *http.Response, err error)
}

func (s *OpenAIGatewayService) recordOpenAIHTTP2StreamSuccess(resp *http.Response, terminal string) {
	switch terminal {
	case "response.completed", "response.done", "[DONE]":
	default:
		return
	}
	if s == nil || s.httpUpstream == nil {
		return
	}
	if reporter, ok := s.httpUpstream.(openAIHTTP2ResponseOutcomeReporter); ok {
		reporter.RecordOpenAIHTTP2ResponseOutcome(resp, nil)
	}
}

func withOpenAIStreamProxyURL(ctx context.Context, proxyURL string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, openAIStreamProxyURLContextKey{}, strings.TrimSpace(proxyURL))
}

func openAIStreamProxyURL(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	proxyURL, _ := ctx.Value(openAIStreamProxyURLContextKey{}).(string)
	return strings.TrimSpace(proxyURL)
}

func (s *OpenAIGatewayService) recordOpenAIHTTP2StreamFailure(ctx context.Context, resp *http.Response, err error) {
	if s == nil || err == nil || s.httpUpstream == nil {
		return
	}
	if reporter, ok := s.httpUpstream.(openAIHTTP2ResponseOutcomeReporter); ok {
		reporter.RecordOpenAIHTTP2ResponseOutcome(resp, err)
		return
	}
	proxyURL := openAIStreamProxyURL(ctx)
	if proxyURL == "" {
		return
	}
	reporter, ok := s.httpUpstream.(OpenAIHTTP2StreamFailureReporter)
	if !ok {
		return
	}
	reporter.RecordOpenAIHTTP2StreamFailure(proxyURL, err)
}
