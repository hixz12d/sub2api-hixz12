//go:build unit

package admin

import (
	"testing"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/require"
)

func monitorBindingValidator(t *testing.T) *validator.Validate {
	t.Helper()
	engine, ok := binding.Validator.Engine().(*validator.Validate)
	require.True(t, ok)
	return engine
}

func TestChannelMonitorCreateRequest_AllowsRaisedIntervalCap(t *testing.T) {
	validate := monitorBindingValidator(t)

	req := channelMonitorCreateRequest{
		Name:            "probe",
		Provider:        "openai",
		PrimaryModel:    "gpt-5.6-terra",
		IntervalSeconds: 9600,
		JitterSeconds:   100,
	}
	require.NoError(t, validate.Struct(req))

	req.IntervalSeconds = 9601
	require.Error(t, validate.Struct(req))

	req.IntervalSeconds = 9600
	req.JitterSeconds = 9585
	require.NoError(t, validate.Struct(req))

	req.JitterSeconds = 9586
	require.Error(t, validate.Struct(req))
}

func TestChannelMonitorUpdateRequest_AllowsRaisedIntervalCap(t *testing.T) {
	validate := monitorBindingValidator(t)

	interval := 9600
	jitter := 100
	req := channelMonitorUpdateRequest{
		IntervalSeconds: &interval,
		JitterSeconds:   &jitter,
	}
	require.NoError(t, validate.Struct(req))

	tooHigh := 9601
	req.IntervalSeconds = &tooHigh
	require.Error(t, validate.Struct(req))
}
