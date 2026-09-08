package service

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

type responseBoundOutcomeStub struct {
	HTTPUpstream
	response    *http.Response
	err         error
	calls       int
	legacyCalls int
}

func (s *responseBoundOutcomeStub) RecordOpenAIHTTP2ResponseOutcome(resp *http.Response, err error) {
	s.response, s.err = resp, err
	s.calls++
}

func (s *responseBoundOutcomeStub) RecordOpenAIHTTP2StreamFailure(string, error) { s.legacyCalls++ }

func TestHTTP2ResponseOutcomeUsesResponseInsteadOfCurrentProxy(t *testing.T) {
	stub := &responseBoundOutcomeStub{}
	svc := &OpenAIGatewayService{httpUpstream: stub}
	resp := &http.Response{ProtoMajor: 2}
	ctx := withOpenAIStreamProxyURL(context.Background(), "http://different-proxy.example:8080")
	svc.recordOpenAIHTTP2StreamFailure(ctx, resp, io.ErrUnexpectedEOF)
	require.Same(t, resp, stub.response)
	require.ErrorIs(t, stub.err, io.ErrUnexpectedEOF)
	require.Equal(t, 1, stub.calls)
	require.Zero(t, stub.legacyCalls)
}

func TestHTTP2ResponseOutcomeSuccessRequiresSuccessfulTerminal(t *testing.T) {
	stub := &responseBoundOutcomeStub{}
	svc := &OpenAIGatewayService{httpUpstream: stub}
	resp := &http.Response{ProtoMajor: 2}
	for _, terminal := range []string{"", "response.created", "response.failed", "response.incomplete", "response.cancelled"} {
		svc.recordOpenAIHTTP2StreamSuccess(resp, terminal)
	}
	require.Zero(t, stub.calls)
	for _, terminal := range []string{"response.completed", "response.done", "[DONE]"} {
		svc.recordOpenAIHTTP2StreamSuccess(resp, terminal)
	}
	require.Equal(t, 3, stub.calls)
	require.Same(t, resp, stub.response)
	require.NoError(t, stub.err)
}
