package service

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

var (
	ErrOpenAIFirstOutputTimeout    = errors.New("openai first output timeout")
	ErrOpenAIStreamIntervalTimeout = errors.New("stream data interval timeout")
	ErrOpenAIStreamMissingTerminal = errors.New("stream ended before terminal event")
)

func withOpenAIUnderlyingError(failover *UpstreamFailoverError, err error) *UpstreamFailoverError {
	if failover != nil && err != nil {
		failover.Err = err
		if failover.Cause == "" {
			failover.Cause = classifyOpenAIStreamScanCause(err)
		}
	}
	return failover
}

const (
	OpenAIFailurePhasePreOutput  = "pre_output"
	OpenAIFailurePhasePostOutput = "post_output"
	OpenAIFailurePhaseTransport  = "transport"
	OpenAIFailurePhaseTimeout    = "timeout"

	OpenAIFailureCauseStreamEOF          = "stream_eof"
	OpenAIFailureCauseStreamRead         = "stream_read"
	OpenAIFailureCauseMissingTerminal    = "missing_terminal"
	OpenAIFailureCauseIntervalTimeout    = "interval_timeout"
	OpenAIFailureCauseFirstOutputTimeout = "first_output_timeout"
	OpenAIFailureCauseCapacity           = "capacity"
	OpenAIFailureCauseUnknown            = "unknown"

	OpenAIRetryDecisionFailoverOtherAccount = "failover_other_account"
	OpenAIRetryDecisionFailClosed           = "fail_closed"
	OpenAIRetryDecisionClientCanceled       = "client_canceled"
)

// annotateOpenAIPreOutputFailover attaches C2 structured attribution and wire ledger
// fields without changing eligibility decisions already encoded on the error.
func annotateOpenAIPreOutputFailover(c *gin.Context, err *UpstreamFailoverError, cause, decision string) *UpstreamFailoverError {
	if err == nil {
		return nil
	}
	if strings.TrimSpace(err.FailurePhase) == "" {
		err.FailurePhase = OpenAIFailurePhasePreOutput
	}
	if strings.TrimSpace(err.Cause) == "" {
		err.Cause = strings.TrimSpace(cause)
		if err.Cause == "" {
			err.Cause = OpenAIFailureCauseUnknown
		}
	}
	if strings.TrimSpace(err.RetryDecisionReason) == "" {
		err.RetryDecisionReason = strings.TrimSpace(decision)
		if !err.ShouldRetryNextAccount() {
			err.RetryDecisionReason = OpenAIRetryDecisionFailClosed
		} else if err.RetryDecisionReason == "" {
			err.RetryDecisionReason = OpenAIRetryDecisionFailoverOtherAccount
		}
	}
	MarkOpenAIAttemptFailureAttribution(c, err.FailurePhase, err.Cause, err.RetryDecisionReason)
	return err
}

func classifyOpenAIStreamScanCause(err error) string {
	if err == nil {
		return OpenAIFailureCauseUnknown
	}
	switch {
	case errors.Is(err, ErrOpenAIFirstOutputTimeout):
		return OpenAIFailureCauseFirstOutputTimeout
	case errors.Is(err, ErrOpenAIStreamIntervalTimeout):
		return OpenAIFailureCauseIntervalTimeout
	case errors.Is(err, ErrOpenAIStreamMissingTerminal):
		return OpenAIFailureCauseMissingTerminal
	}
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return OpenAIFailureCauseStreamEOF
	}
	lower := strings.ToLower(err.Error())
	switch {
	case strings.Contains(lower, "unexpected eof"), strings.Contains(lower, "stream ended before terminal"):
		return OpenAIFailureCauseStreamEOF
	case strings.Contains(lower, "interval timeout"):
		return OpenAIFailureCauseIntervalTimeout
	default:
		return OpenAIFailureCauseStreamRead
	}
}

func noteOpenAIAttemptProtoFromResponse(c *gin.Context, resp *http.Response) {
	if resp == nil {
		return
	}
	MarkOpenAIAttemptProtoMajor(c, resp.ProtoMajor)
}
