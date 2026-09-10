package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const observationTerminal = `{"type":"response.completed","response":{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hello world"}]}],"usage":{"input_tokens":100,"input_tokens_details":{"cached_tokens":40},"output_tokens":100,"output_tokens_details":{"reasoning_tokens":20}}}}`

func TestOpenAIStreamObservationReliableUsage(t *testing.T) {
	start := time.Unix(100, 0)
	o := newOpenAIVisibleStreamObserver(RequestOriginBusiness, start)
	o.observe([]byte(`{"type":"response.created"}`), "response.created", start)
	o.observe([]byte(`{"delta":"reasoning"}`), "response.reasoning_text.delta", start.Add(time.Second))
	o.observe([]byte(`{"delta":"hello"}`), "response.output_text.delta", start.Add(2*time.Second))
	o.observe([]byte(`{"delta":"world"}`), "response.output_text.delta", start.Add(6*time.Second))
	o.observe([]byte(observationTerminal), "response.completed", start.Add(20*time.Second))
	snapshot := o.snapshot(true)
	milli, err := snapshot.OutputTPSMilli()
	require.NoError(t, err)
	require.NotNil(t, milli)
	require.Equal(t, int64(20000), *milli)
	require.Equal(t, int64(80), *snapshot.VisibleOutputTokens)
	require.Equal(t, int64(100), *snapshot.InputTokensTotal)
	require.Equal(t, int64(40), *snapshot.CacheReadTokens)
	require.Equal(t, int64(2000), *snapshot.FirstVisibleOutputMs())
	require.Equal(t, VisibleStreamObservationMethod, snapshot.Method)
	*snapshot.VisibleOutputTokens = 999
	require.Equal(t, int64(80), *o.snapshot(true).VisibleOutputTokens, "snapshots must own their values")
	milli, err = o.snapshot(false).OutputTPSMilli()
	require.NoError(t, err)
	require.Nil(t, milli, "disconnect cannot become a successful sample")
}

func TestOpenAIStreamObservationExcludesUnreliableSamples(t *testing.T) {
	for _, test := range []struct {
		name, terminal, extra string
		deltas                int
		gap                   time.Duration
	}{
		{"missing reasoning", strings.Replace(observationTerminal, `"output_tokens_details":{"reasoning_tokens":20}`, `"output_tokens_details":{}`, 1), "", 2, time.Second},
		{"negative reasoning", strings.Replace(observationTerminal, `"reasoning_tokens":20`, `"reasoning_tokens":-1`, 1), "", 2, time.Second},
		{"reasoning exceeds output", strings.Replace(observationTerminal, `"reasoning_tokens":20`, `"reasoning_tokens":101`, 1), "", 2, time.Second},
		{"fractional count", strings.Replace(observationTerminal, `"output_tokens":100`, `"output_tokens":100.5`, 1), "", 2, time.Second},
		{"incomplete status", strings.Replace(observationTerminal, `"status":"completed"`, `"status":"incomplete"`, 1), "", 2, time.Second},
		{"audio usage", strings.Replace(observationTerminal, `"reasoning_tokens":20`, `"reasoning_tokens":20,"audio_tokens":5`, 1), "", 2, time.Second},
		{"tool event", observationTerminal, "response.function_call_arguments.delta", 2, time.Second},
		{"audio event", observationTerminal, "response.audio.delta", 2, time.Second},
		{"refusal", observationTerminal, "response.refusal.delta", 2, time.Second},
		{"failed then completed", observationTerminal, "response.failed", 2, time.Second},
		{"unknown event", observationTerminal, "response.new_modality.delta", 2, time.Second},
		{"single delta", observationTerminal, "", 1, time.Second},
		{"too fast", observationTerminal, "", 2, 199 * time.Millisecond},
		{"too few tokens", strings.Replace(observationTerminal, `"output_tokens":100`, `"output_tokens":35`, 1), "", 2, time.Second},
		{"no terminal", "", "", 2, time.Second},
		{"tool terminal", strings.Replace(observationTerminal, `"type":"message"`, `"type":"function_call"`, 1), "", 2, time.Second},
	} {
		t.Run(test.name, func(t *testing.T) {
			start := time.Unix(100, 0)
			o := newOpenAIVisibleStreamObserver(RequestOriginBusiness, start)
			for i := 0; i < test.deltas; i++ {
				o.observe([]byte(`{"delta":"text"}`), "response.output_text.delta", start.Add(time.Second+time.Duration(i)*test.gap))
			}
			if test.extra != "" {
				o.observe([]byte(`{}`), test.extra, start.Add(3*time.Second))
			}
			if test.terminal != "" {
				o.observe([]byte(test.terminal), "response.completed", start.Add(4*time.Second))
			}
			rate, err := o.snapshot(true).OutputTPSMilli()
			require.NoError(t, err)
			require.Nil(t, rate)
		})
	}
}

func TestOpenAIStreamObservationNativeHandler(t *testing.T) {
	for _, asynchronous := range []bool{false, true} {
		for _, completed := range []bool{false, true} {
			name := "sync"
			if asynchronous {
				name = "async"
			}
			if completed {
				name += "/completed"
			} else {
				name += "/truncated"
			}
			t.Run(name, func(t *testing.T) {
				cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}
				if asynchronous {
					cfg.Gateway.StreamKeepaliveInterval = 10
				}
				svc := &OpenAIGatewayService{cfg: cfg}
				reader, writer := io.Pipe()
				t.Cleanup(func() { _ = reader.Close() })
				done := make(chan struct{})
				go func() {
					defer close(done)
					defer writer.Close()
					_, _ = io.WriteString(writer, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n")
					time.Sleep(220 * time.Millisecond)
					_, _ = io.WriteString(writer, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"world\"}\n\n")
					if completed {
						_, _ = io.WriteString(writer, "data: "+observationTerminal+"\n\n")
					}
				}()
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
				c.Request.Header.Set("X-Request-Origin", "capability_detector")
				resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: reader}
				result, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI}, time.Now(), "model", "model")
				if completed {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
				}
				require.NotNil(t, result)
				require.NotNil(t, result.monitorObservation)
				require.Equal(t, RequestOriginBusiness, result.monitorObservation.Origin, "client headers cannot set origin")
				rate, rateErr := result.monitorObservation.OutputTPSMilli()
				require.NoError(t, rateErr)
				if completed {
					require.NotNil(t, rate)
					require.Equal(t, 100, result.usage.OutputTokens, "billing usage is untouched")
					require.Equal(t, int64(80), *result.monitorObservation.VisibleOutputTokens)
				} else {
					require.Nil(t, rate)
				}
				require.Contains(t, recorder.Body.String(), `"delta":"hello"`)
				select {
				case <-done:
				case <-time.After(time.Second):
					t.Fatal("writer did not stop")
				}
			})
		}
	}
}

func TestOpenAIStreamObservationNativeCancellationAndSource(t *testing.T) {
	body := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n" +
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"world\"}\n\n" + "data: " + observationTerminal + "\n\n"
	for _, mode := range []string{"cancelled", "write_failed", "detector", "unsupported"} {
		t.Run(mode, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			ctx := context.Background()
			if mode == "cancelled" {
				cancelled, cancel := context.WithCancel(c.Request.Context())
				cancel()
				c.Request = c.Request.WithContext(cancelled)
			}
			if mode == "write_failed" {
				c.Writer = &observationFailedWriter{ResponseWriter: c.Writer}
			}
			if mode == "detector" {
				var err error
				ctx, err = WithRequestOrigin(ctx, RequestOriginCapabilityDetector)
				require.NoError(t, err)
			}
			platform := PlatformOpenAI
			if mode == "unsupported" {
				platform = PlatformGrok
			}
			svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}}
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
			result, err := svc.handleStreamingResponse(ctx, resp, c, &Account{ID: 1, Platform: platform}, time.Now(), "model", "model")
			require.NoError(t, err, "monitoring must not change existing drain/billing behavior")
			require.Equal(t, 100, result.usage.OutputTokens)
			if mode == "unsupported" {
				require.Nil(t, result.monitorObservation)
				return
			}
			require.NotNil(t, result.monitorObservation)
			if mode == "detector" {
				require.Equal(t, RequestOriginCapabilityDetector, result.monitorObservation.Origin)
			} else {
				require.False(t, result.monitorObservation.OutputComplete)
			}
			rate, err := result.monitorObservation.OutputTPSMilli()
			require.NoError(t, err)
			require.Nil(t, rate)
		})
	}
}

type observationFailedWriter struct{ gin.ResponseWriter }

func (w *observationFailedWriter) Write(p []byte) (int, error)       { return 0, io.ErrClosedPipe }
func (w *observationFailedWriter) WriteString(p string) (int, error) { return 0, io.ErrClosedPipe }

func TestVisibleOutputObservationBoundariesAndTrustedOrigin(t *testing.T) {
	start := time.Unix(100, 0)
	first, last := start.Add(time.Second), start.Add(1200*time.Millisecond)
	tokens := int64(16)
	o := VisibleOutputObservation{Origin: RequestOriginBusiness, RequestStartedAt: start, FirstVisibleTextAt: &first, LastVisibleTextAt: &last, VisibleOutputTokens: &tokens, OutputComplete: true, TextDeltaCount: 2, Version: 1, Method: VisibleStreamObservationMethod}
	rate, err := o.OutputTPSMilli()
	require.NoError(t, err)
	require.Equal(t, int64(80000), *rate)
	tokens = 1<<63 - 1
	_, err = o.OutputTPSMilli()
	require.Error(t, err)
	ctx, err := WithRequestOrigin(context.Background(), RequestOriginCapabilityDetector)
	require.NoError(t, err)
	require.Equal(t, RequestOriginCapabilityDetector, RequestOriginFromContext(context.WithoutCancel(ctx)))
	_, err = WithRequestOrigin(ctx, "untrusted")
	require.Error(t, err)
	require.Equal(t, RequestOriginBusiness, RequestOriginFromContext(nil))
	// Valid JSON fixtures are asserted independently of observer filtering.
	require.True(t, json.Valid([]byte(observationTerminal)))
}
