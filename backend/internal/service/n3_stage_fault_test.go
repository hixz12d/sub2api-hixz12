package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestN3MessagesSpoolCreationFailureIsLocal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows deliberately uses memory-only staging; Unix pipeline is checked in Linux")
	}
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	account := messagesStageTestAccount()
	account.Type = AccountTypeOAuth
	EnsureOpenAIRetryBudget(c, account, []byte(`{"stream":true,"input":"hello"}`))
	reporter := &messagesStageH2Reporter{}
	svc := &OpenAIGatewayService{cfg: messagesStageTestConfig(), httpUpstream: reporter}
	payload := "data: {\"type\":\"response.created\",\"response\":{\"id\":\"" + strings.Repeat("a", 70000) + "\"}}\n\n"
	resp := &http.Response{StatusCode: 200, ProtoMajor: 2, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(payload))}
	result, err := svc.handleAnthropicStreamingResponse(context.Background(), resp, c, account, "gpt-5.4", "gpt-5.4", "gpt-5.4", time.Now())
	require.ErrorIs(t, err, errOpenAILocalOutputFailure)
	var failover *UpstreamFailoverError
	require.False(t, errors.As(err, &failover))
	require.False(t, result.ClientDisconnect)
	require.Empty(t, rec.Body.String())
	require.Empty(t, reporter.failures)
	require.False(t, NewCodexCommitGuard(c).Snapshot().ReplaySafe)
	require.False(t, svc.ReportOpenAIAccountScheduleResult(account, "gpt-5.4", false, nil, err))
	require.Nil(t, svc.openaiScheduler, "local failure must not even initialize the scheduler")
}

func TestN3StageIOFaultsAndCleanup(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		stage := newDefaultOpenAIFirstOutputStage()
		defer func() { _ = stage.Close() }()
		stage.memoryOnly = false
		stage.createTemp = func() (*os.File, error) { return nil, os.ErrPermission }
		_, err := stage.WriteString(strings.Repeat("x", 70000))
		require.ErrorIs(t, err, os.ErrPermission)
		require.Zero(t, stage.Buffered())
	})
	for _, mode := range []string{"write", "seek", "read"} {
		t.Run(mode, func(t *testing.T) {
			file, err := os.CreateTemp(t.TempDir(), "stage-*")
			require.NoError(t, err)
			_, err = file.WriteString("a")
			require.NoError(t, err)
			stage := newDefaultOpenAIFirstOutputStage()
			stage.tempFile, stage.tempPath, stage.size = file, file.Name(), 2
			if mode != "read" {
				require.NoError(t, file.Close())
			}
			var out bytes.Buffer
			if mode == "write" {
				_, err = stage.WriteString("b")
				require.Error(t, err)
			} else {
				err = stage.CommitTo(&out)
				require.ErrorIs(t, err, errOpenAILocalOutputFailure)
			}
			if mode == "seek" {
				require.Empty(t, out.String())
			}
			if mode == "read" {
				require.Equal(t, "a", out.String())
			}
			_ = stage.Close()
			_, statErr := os.Stat(file.Name())
			require.ErrorIs(t, statErr, os.ErrNotExist)
			require.NoError(t, stage.Close())
		})
	}
}
