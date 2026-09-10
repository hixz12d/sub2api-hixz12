//go:build unit

package service

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestQuestionCaptureReceivesServerEvents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(writer)
	capture := NewQuestionCapture(1, 2, "model", " exact prompt ", nil)
	ctx.Set("account_question_capture", capture)
	svc := &AccountTestService{}
	svc.sendEvent(ctx, TestEvent{Type: "content", Text: "first "})
	svc.sendEvent(ctx, TestEvent{Type: "content", Text: "second"})
	svc.sendEvent(ctx, TestEvent{Type: "test_complete", Success: true})
	record := capture.Record()
	require.Equal(t, "first second", record.Answer)
	require.Equal(t, " exact prompt ", record.Prompt)
	require.Equal(t, "completed", record.TransportState)
	require.Contains(t, writer.Body.String(), "test_complete")
}
