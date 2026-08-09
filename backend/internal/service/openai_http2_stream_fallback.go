package service

import (
	"context"
	"strings"
)

type openAIStreamProxyURLContextKey struct{}

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

func (s *OpenAIGatewayService) recordOpenAIHTTP2StreamFailure(ctx context.Context, err error) {
	if s == nil || err == nil || s.httpUpstream == nil {
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
