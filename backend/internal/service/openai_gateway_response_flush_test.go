package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type openAIResponseFlushRecorder struct {
	header          http.Header
	mu              sync.Mutex
	body            bytes.Buffer
	status          int
	writes          int
	failAfterWrites int
	flushSnapshots  []string
	flushEvents     chan int
	blockFlush      int
	flushBlocked    chan struct{}
	releaseFlush    <-chan struct{}
}

func newOpenAIResponseFlushRecorder() *openAIResponseFlushRecorder {
	return &openAIResponseFlushRecorder{
		header:          make(http.Header),
		failAfterWrites: -1,
		flushEvents:     make(chan int, 16),
	}
}

func (w *openAIResponseFlushRecorder) Header() http.Header {
	return w.header
}

func (w *openAIResponseFlushRecorder) WriteHeader(statusCode int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.status == 0 {
		w.status = statusCode
	}
}

func (w *openAIResponseFlushRecorder) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.failAfterWrites >= 0 && w.writes >= w.failAfterWrites {
		return 0, errors.New("client disconnected")
	}
	w.writes++
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(data)
}

func (w *openAIResponseFlushRecorder) Flush() {
	w.mu.Lock()
	w.flushSnapshots = append(w.flushSnapshots, w.body.String())
	count := len(w.flushSnapshots)
	w.mu.Unlock()
	w.flushEvents <- count
	if count == w.blockFlush {
		close(w.flushBlocked)
		<-w.releaseFlush
	}
}

func (w *openAIResponseFlushRecorder) snapshot() (string, []string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.body.String(), append([]string(nil), w.flushSnapshots...)
}

type stagedOpenAISSEReadCloser struct {
	segments   [][]byte
	gates      []<-chan struct{}
	waiting    []chan struct{}
	eofReached chan struct{}
	current    []byte
	index      int
}

func (r *stagedOpenAISSEReadCloser) Read(data []byte) (int, error) {
	if len(r.current) == 0 {
		if r.index >= len(r.segments) {
			if r.eofReached != nil {
				close(r.eofReached)
				r.eofReached = nil
			}
			return 0, io.EOF
		}
		index := r.index
		r.index++
		if index < len(r.waiting) && r.waiting[index] != nil {
			close(r.waiting[index])
		}
		if index < len(r.gates) && r.gates[index] != nil {
			<-r.gates[index]
		}
		r.current = r.segments[index]
	}
	n := copy(data, r.current)
	r.current = r.current[n:]
	return n, nil
}

func (r *stagedOpenAISSEReadCloser) Close() error { return nil }

type openAIResponseFlushReadError struct {
	payload []byte
	err     error
	sent    bool
}

func (r *openAIResponseFlushReadError) Read(data []byte) (int, error) {
	if !r.sent {
		r.sent = true
		return copy(data, r.payload), nil
	}
	if r.err != nil {
		return 0, r.err
	}
	return 0, io.ErrUnexpectedEOF
}

func (r *openAIResponseFlushReadError) Close() error { return nil }

func TestOpenAIResponseFlush_SlowEventsFlushOnceAtBoundaries(t *testing.T) {
	events := []string{
		`data: {"type":"response.output_text.delta","delta":"a"}`,
		`data: {"type":"response.output_text.delta","delta":"b"}`,
		`data: {"type":"response.output_text.delta","delta":"c"}`,
		`data: [DONE]`,
	}
	body := strings.Join(events, "\n\n") + "\n\n"
	recorder := newOpenAIResponseFlushRecorder()

	result, err := runOpenAIResponseFlushTest(recorder, io.NopCloser(strings.NewReader(body)), config.GatewayConfig{})

	require.NoError(t, err)
	require.NotNil(t, result)
	gotBody, flushes := recorder.snapshot()
	require.Equal(t, body, gotBody)
	require.Len(t, flushes, len(events))
	for _, flushed := range flushes {
		require.True(t, strings.HasSuffix(flushed, "\n\n"), "flush must occur after a complete SSE event")
	}
}

func TestOpenAIResponseFlush_DataQueuedButBlankDrainsFlushesOnce(t *testing.T) {
	first := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"first\"}\n\n"
	second := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"second\"}\n\n"
	terminal := "data: [DONE]\n\n"
	allowSecond := make(chan struct{})
	allowTerminal := make(chan struct{})
	terminalWaiting := make(chan struct{})
	reader := &stagedOpenAISSEReadCloser{
		segments: [][]byte{[]byte(first), []byte(second), []byte(terminal)},
		gates:    []<-chan struct{}{nil, allowSecond, allowTerminal},
		waiting:  []chan struct{}{nil, nil, terminalWaiting},
	}
	releaseFirstFlush := make(chan struct{})
	recorder := newOpenAIResponseFlushRecorder()
	recorder.blockFlush = 1
	recorder.flushBlocked = make(chan struct{})
	recorder.releaseFlush = releaseFirstFlush
	resultCh, errCh := runOpenAIResponseFlushTestAsync(recorder, reader, config.GatewayConfig{StreamDataIntervalTimeout: 30})

	waitOpenAIResponseFlushSignal(t, recorder.flushBlocked)
	close(allowSecond)
	waitOpenAIResponseFlushSignal(t, terminalWaiting)
	close(releaseFirstFlush)
	waitOpenAIResponseFlushCount(t, recorder, 2)
	close(allowTerminal)

	require.NoError(t, <-errCh)
	require.NotNil(t, <-resultCh)
	gotBody, flushes := recorder.snapshot()
	require.Equal(t, first+second+terminal, gotBody)
	require.Len(t, flushes, 3)
	require.Equal(t, first, flushes[0])
	require.Equal(t, first+second, flushes[1], "blank line that drains the queue must flush the complete event exactly once")
}

func TestOpenAIResponseFlush_BurstDoesNotIncreaseFlushes(t *testing.T) {
	first := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"first\"}\n\n"
	burst := strings.Join([]string{
		`data: {"type":"response.output_text.delta","delta":"second"}`,
		`data: {"type":"response.output_text.delta","delta":"third"}`,
		`data: [DONE]`,
	}, "\n\n") + "\n\n"
	allowBurst := make(chan struct{})
	eofReached := make(chan struct{})
	reader := &stagedOpenAISSEReadCloser{
		segments:   [][]byte{[]byte(first), []byte(burst)},
		gates:      []<-chan struct{}{nil, allowBurst},
		eofReached: eofReached,
	}
	releaseFirstFlush := make(chan struct{})
	recorder := newOpenAIResponseFlushRecorder()
	recorder.blockFlush = 1
	recorder.flushBlocked = make(chan struct{})
	recorder.releaseFlush = releaseFirstFlush
	resultCh, errCh := runOpenAIResponseFlushTestAsync(recorder, reader, config.GatewayConfig{StreamDataIntervalTimeout: 30})

	waitOpenAIResponseFlushSignal(t, recorder.flushBlocked)
	close(allowBurst)
	waitOpenAIResponseFlushSignal(t, eofReached)
	close(releaseFirstFlush)

	require.NoError(t, <-errCh)
	require.NotNil(t, <-resultCh)
	gotBody, flushes := recorder.snapshot()
	require.Equal(t, first+burst, gotBody)
	require.Len(t, flushes, 2, "queued burst must remain batched until its drained event boundary")
	require.Equal(t, first, flushes[0])
	require.Equal(t, first+burst, flushes[1])
}

func TestOpenAIResponseFlush_CommentAndEOFOnlyFlushCompleteResidual(t *testing.T) {
	body := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"a\"}\n\n" +
		": upstream-comment\n\n" +
		"data: [DONE]\n"
	recorder := newOpenAIResponseFlushRecorder()

	result, err := runOpenAIResponseFlushTest(recorder, io.NopCloser(strings.NewReader(body)), config.GatewayConfig{})

	require.NoError(t, err)
	require.NotNil(t, result)
	gotBody, flushes := recorder.snapshot()
	require.Equal(t, body, gotBody)
	require.Len(t, flushes, 3)
	require.True(t, strings.HasSuffix(flushes[0], "\n\n"))
	require.True(t, strings.HasSuffix(flushes[1], "\n\n"))
	require.True(t, strings.HasSuffix(flushes[2], "data: [DONE]\n"), "EOF must flush only the remaining bytes")
}

func TestOpenAIResponseFlush_TerminalReadErrorFlushesResidual(t *testing.T) {
	body := "data: [DONE]\n"
	recorder := newOpenAIResponseFlushRecorder()

	result, err := runOpenAIResponseFlushTest(recorder, &openAIResponseFlushReadError{payload: []byte(body)}, config.GatewayConfig{})

	require.NoError(t, err)
	require.NotNil(t, result)
	gotBody, flushes := recorder.snapshot()
	require.Equal(t, body, gotBody)
	require.Equal(t, []string{body}, flushes)
}

func TestOpenAIResponseFlush_OutputWithoutTerminalFlushesResidualWithoutFailover(t *testing.T) {
	body := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n"
	recorder := newOpenAIResponseFlushRecorder()

	result, err := runOpenAIResponseFlushTest(recorder, io.NopCloser(strings.NewReader(body)), config.GatewayConfig{})

	require.ErrorContains(t, err, "missing terminal event")
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.NotNil(t, result)
	gotBody, flushes := recorder.snapshot()
	require.Equal(t, body, gotBody)
	require.Equal(t, []string{body}, flushes)
}

func TestOpenAIResponseFlush_PreambleWithoutTerminalRemainsBufferedForFailover(t *testing.T) {
	body := "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\"}}\n"
	recorder := newOpenAIResponseFlushRecorder()

	result, err := runOpenAIResponseFlushTest(recorder, io.NopCloser(strings.NewReader(body)), config.GatewayConfig{})

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.NotNil(t, result)
	gotBody, flushes := recorder.snapshot()
	require.Empty(t, gotBody)
	require.Empty(t, flushes)
}

func TestOpenAIResponseFlush_CanceledAfterOutputFlushesResidualWithoutErrorEvent(t *testing.T) {
	body := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n"
	recorder := newOpenAIResponseFlushRecorder()

	result, err := runOpenAIResponseFlushTest(recorder, &openAIResponseFlushReadError{payload: []byte(body), err: context.Canceled}, config.GatewayConfig{})

	require.ErrorIs(t, err, context.Canceled)
	require.NotNil(t, result)
	gotBody, flushes := recorder.snapshot()
	require.Equal(t, body, gotBody)
	require.Equal(t, []string{body}, flushes)
	require.NotContains(t, gotBody, "stream_read_error")
}

func TestOpenAIResponseFlush_HTTP2ReadErrorAfterOutputReturnsSanitizedError(t *testing.T) {
	body := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n"
	recorder := newOpenAIResponseFlushRecorder()
	streamErr := errors.New("stream error: stream ID 87; INTERNAL_ERROR; received from peer")

	result, err := runOpenAIResponseFlushTest(
		recorder,
		&openAIResponseFlushReadError{payload: []byte(body), err: streamErr},
		config.GatewayConfig{},
	)

	require.Error(t, err)
	require.NotNil(t, result)
	code, message, ok := OpenAIUpstreamStreamReadErrorDetails(err)
	require.True(t, ok)
	require.Equal(t, OpenAIUpstreamHTTP2StreamErrorCode, code)
	require.Equal(t, "Upstream HTTP/2 stream failed", message)
	gotBody, _ := recorder.snapshot()
	require.Equal(t, body, gotBody)
	require.NotContains(t, gotBody, `"type":"error"`)
	require.NotContains(t, gotBody, "stream ID")
	require.NotContains(t, gotBody, "INTERNAL_ERROR")
}

func TestOpenAIResponseFlush_KeepaliveFlushesImmediately(t *testing.T) {
	recorder := newOpenAIResponseFlushRecorder()
	reader, writer := io.Pipe()
	resultCh, errCh := runOpenAIResponseFlushTestAsync(recorder, reader, config.GatewayConfig{StreamKeepaliveInterval: 1})

	waitOpenAIResponseFlushCount(t, recorder, 1)
	_, flushes := recorder.snapshot()
	require.Equal(t, ":\n\n", flushes[0])
	_, err := writer.Write([]byte("data: [DONE]\n\n"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	require.NoError(t, <-errCh)
	require.NotNil(t, <-resultCh)
	gotBody, flushes := recorder.snapshot()
	require.Equal(t, ":\n\ndata: [DONE]\n\n", gotBody)
	require.Len(t, flushes, 2)
}

func TestOpenAIResponseFlush_KeepalivePreservesCapacityFailoverBeforeSemanticOutput(t *testing.T) {
	recorder := newOpenAIResponseFlushRecorder()
	reader, writer := io.Pipe()
	resultCh, errCh := runOpenAIResponseFlushTestAsync(
		recorder,
		reader,
		config.GatewayConfig{StreamKeepaliveInterval: 1},
		http.Header{"X-Request-Id": []string{"rid-failed-attempt"}},
	)

	preamble := "event: response.created\n" +
		`data: {"type":"response.created","response":{"id":"resp_capacity"}}` + "\n\n" +
		"event: response.in_progress\n" +
		`data: {"type":"response.in_progress","response":{"id":"resp_capacity"}}` + "\n\n"
	_, err := writer.Write([]byte(preamble))
	require.NoError(t, err)
	waitOpenAIResponseFlushCount(t, recorder, 1)
	bodyAfterKeepalive, _ := recorder.snapshot()

	failedEvent := "event: response.failed\n" +
		`data: {"type":"response.failed","response":{"id":"resp_capacity","error":{"code":"server_is_overloaded","message":"Our servers are currently overloaded. Please try again later."}}}` + "\n\n"
	_, err = writer.Write([]byte(failedEvent))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	result := <-resultCh
	forwardErr := <-errCh
	require.NotNil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, forwardErr, &failoverErr)
	require.True(t, failoverErr.SafeToFailoverAfterWrite)
	require.Equal(t, ":\n\n", bodyAfterKeepalive)
	require.Empty(t, recorder.Header().Get("X-Request-Id"))
	body, flushes := recorder.snapshot()
	require.Equal(t, ":\n\n", body)
	require.Equal(t, []string{":\n\n"}, flushes)
	require.NotContains(t, body, "response.created")
	require.NotContains(t, body, "response.failed")
}

func TestOpenAIResponseFlush_OversizedPreambleDisablesReplaySafely(t *testing.T) {
	preamble := `data: {"type":"response.created","response":{"id":"resp_large","metadata":{"padding":"` +
		strings.Repeat("x", 8*1024) + `"}}}` + "\n\n"
	failedEvent := `data: {"type":"response.failed","response":{"error":{"code":"server_is_overloaded","message":"Please retry later."}}}` + "\n\n"
	recorder := newOpenAIResponseFlushRecorder()

	result, err := runOpenAIResponseFlushTest(
		recorder,
		io.NopCloser(strings.NewReader(preamble+failedEvent)),
		config.GatewayConfig{},
	)

	require.NotNil(t, result)
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	body, _ := recorder.snapshot()
	require.Contains(t, body, "response.created")
	require.Contains(t, body, "response.failed")
}

func TestOpenAIResponseFlush_KeepaliveDoesNotSplitOversizedPreamble(t *testing.T) {
	preamble := `data: {"type":"response.created","response":{"id":"resp_large_keepalive","metadata":{"padding":"` +
		strings.Repeat("x", 8*1024) + `"}}}` + "\n\n"
	failedEvent := `data: {"type":"response.failed","response":{"error":{"code":"server_is_overloaded","message":"Please retry later."}}}` + "\n\n"
	recorder := newOpenAIResponseFlushRecorder()
	reader, writer := io.Pipe()
	resultCh, errCh := runOpenAIResponseFlushTestAsync(recorder, reader, config.GatewayConfig{StreamKeepaliveInterval: 1})

	_, err := writer.Write([]byte(preamble))
	require.NoError(t, err)
	waitOpenAIResponseFlushCount(t, recorder, 2)
	_, err = writer.Write([]byte(failedEvent))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	result := <-resultCh
	forwardErr := <-errCh
	require.NotNil(t, result)
	require.Error(t, forwardErr)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(forwardErr, &failoverErr))
	body, flushes := recorder.snapshot()
	forwardedFailedEvent := strings.Replace(failedEvent, `"code":"server_is_overloaded"`, `"code":"server_error"`, 1)
	require.Equal(t, preamble+":\n\n"+forwardedFailedEvent, body)
	require.GreaterOrEqual(t, len(flushes), 3)
}

func TestOpenAIStreamClientOutputStartedUsesAttemptWriterBaseline(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	_, err := c.Writer.Write([]byte(":\n\n"))
	require.NoError(t, err)
	attemptWriterSizeBefore := OpenAICompactKeepaliveAdjustedWrittenSize(c)
	require.False(t, openAIStreamClientOutputStarted(c, false, attemptWriterSizeBefore, 0))

	n, err := c.Writer.Write([]byte(":\n\n"))
	require.NoError(t, err)
	require.False(t, openAIStreamClientOutputStarted(c, false, attemptWriterSizeBefore, n))

	_, err = c.Writer.Write([]byte("data: semantic\n\n"))
	require.NoError(t, err)
	require.True(t, openAIStreamClientOutputStarted(c, false, attemptWriterSizeBefore, n))
}

func TestOpenAIResponseFlush_PreviousAttemptKeepaliveCapsNextPreOutputFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	_, err := c.Writer.Write([]byte(":\n\n"))
	require.NoError(t, err)
	c.Writer.Flush()

	svc := &OpenAIGatewayService{
		cfg:           &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
		toolCorrector: NewCodexToolCorrector(),
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(
			"event: response.failed\n" +
				`data: {"type":"response.failed","response":{"error":{"code":"server_is_overloaded","message":"Please retry later."}}}` + "\n\n",
		)),
	}

	result, forwardErr := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 2, Platform: PlatformOpenAI}, time.Now(), "gpt-5", "gpt-5")
	require.NotNil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, forwardErr, &failoverErr)
	require.True(t, failoverErr.SafeToFailoverAfterWrite)
	require.Equal(t, ":\n\n", recorder.Body.String())
}

func TestOpenAIPassthrough_PreviousNativeKeepaliveCapsPreOutputFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	_, err := c.Writer.Write([]byte(":\n\n"))
	require.NoError(t, err)
	c.Writer.Flush()

	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(
			"event: response.failed\n" +
				`data: {"type":"response.failed","response":{"error":{"code":"server_is_overloaded","message":"Please retry later."}}}` + "\n\n",
		)),
	}

	result, forwardErr := svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, &Account{ID: 2, Platform: PlatformOpenAI}, time.Now(), "gpt-5", "gpt-5")
	require.NotNil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, forwardErr, &failoverErr)
	require.True(t, failoverErr.SafeToFailoverAfterWrite)
	require.Equal(t, ":\n\n", recorder.Body.String())
}

func TestOpenAIPassthroughFailedAttemptHeadersDoNotLeakIntoNativeSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	svc := &OpenAIGatewayService{
		cfg:           &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
		toolCorrector: NewCodexToolCorrector(),
	}

	failedResp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
			"X-Request-Id": []string{"rid-passthrough-failed"},
		},
		Body: io.NopCloser(strings.NewReader(
			"event: response.created\n" +
				`data: {"type":"response.created","response":{"id":"resp_failed"}}` + "\n\n" +
				"event: response.failed\n" +
				`data: {"type":"response.failed","response":{"error":{"code":"server_is_overloaded","message":"Please retry later."}}}` + "\n\n",
		)),
	}

	_, firstErr := svc.handleStreamingResponsePassthrough(c.Request.Context(), failedResp, c, &Account{ID: 1, Platform: PlatformOpenAI}, time.Now(), "gpt-5", "gpt-5")
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, firstErr, &failoverErr)
	require.False(t, c.Writer.Written())
	require.Empty(t, c.Writer.Header().Values("X-Request-Id"))

	successResp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
			"X-Request-Id": []string{"rid-native-success"},
		},
		Body: io.NopCloser(strings.NewReader(
			`data: {"type":"response.output_text.delta","delta":"ok"}` + "\n\n" +
				`data: {"type":"response.completed","response":{"id":"resp_success","usage":{"input_tokens":1,"output_tokens":1}}}` + "\n\n",
		)),
	}

	result, secondErr := svc.handleStreamingResponse(c.Request.Context(), successResp, c, &Account{ID: 2, Platform: PlatformOpenAI}, time.Now(), "gpt-5", "gpt-5")
	require.NoError(t, secondErr)
	require.NotNil(t, result)
	require.Equal(t, []string{"rid-native-success"}, recorder.Header().Values("X-Request-Id"))
	require.NotContains(t, recorder.Body.String(), "resp_failed")
	require.Contains(t, recorder.Body.String(), "resp_success")
}

func TestOpenAIResponseFlush_KeepaliveDoesNotSplitOpenEvent(t *testing.T) {
	const dataLine = `data: {"type":"response.output_text.delta","delta":"a"}`
	// Filling the 16-slot scan queue proves the main loop processed data before the reader reaches the gated blank.
	dataLines := make([]string, 17)
	for i := range dataLines {
		dataLines[i] = dataLine
	}
	partialEvent := strings.Join(dataLines, "\n") + "\n"
	completeEvent := partialEvent + "\n"
	terminal := "data: [DONE]\n\n"
	allowBlank := make(chan struct{})
	allowTerminal := make(chan struct{})
	blankWaiting := make(chan struct{})
	terminalWaiting := make(chan struct{})
	reader := &stagedOpenAISSEReadCloser{
		segments: [][]byte{[]byte(partialEvent), []byte("\n"), []byte(terminal)},
		gates:    []<-chan struct{}{nil, allowBlank, allowTerminal},
		waiting:  []chan struct{}{nil, blankWaiting, terminalWaiting},
	}
	recorder := newOpenAIResponseFlushRecorder()
	resultCh, errCh := runOpenAIResponseFlushTestAsync(recorder, reader, config.GatewayConfig{StreamKeepaliveInterval: 1})

	waitOpenAIResponseFlushSignal(t, blankWaiting)
	timer := time.NewTimer(1250 * time.Millisecond)
	select {
	case count := <-recorder.flushEvents:
		timer.Stop()
		t.Fatalf("keepalive flushed open event before its blank boundary: flush %d", count)
	case <-timer.C:
	}

	close(allowBlank)
	waitOpenAIResponseFlushSignal(t, terminalWaiting)
	waitOpenAIResponseFlushCount(t, recorder, 1)
	gotBody, flushes := recorder.snapshot()
	require.Equal(t, completeEvent, gotBody)
	require.Equal(t, []string{completeEvent}, flushes)

	close(allowTerminal)
	require.NoError(t, <-errCh)
	require.NotNil(t, <-resultCh)
	gotBody, flushes = recorder.snapshot()
	require.Equal(t, completeEvent+terminal, gotBody)
	require.Len(t, flushes, 2)
	require.Equal(t, completeEvent+terminal, flushes[1])
}

func TestOpenAIResponseFlush_FailedAndErrorEventsFlushAtBoundaries(t *testing.T) {
	t.Run("failed at EOF", func(t *testing.T) {
		body := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"a\"}\n\n" +
			"data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"safety_error\",\"message\":\"blocked\"},\"usage\":{\"input_tokens\":3,\"output_tokens\":1}}}\n"
		recorder := newOpenAIResponseFlushRecorder()

		result, err := runOpenAIResponseFlushTest(recorder, io.NopCloser(strings.NewReader(body)), config.GatewayConfig{})

		require.Error(t, err)
		require.NotNil(t, result)
		require.Equal(t, 3, result.usage.InputTokens)
		gotBody, flushes := recorder.snapshot()
		expectedBody := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"a\"}\n\n" +
			"data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"safety_error\",\"message\":\"blocked\"}}}\n"
		require.Equal(t, expectedBody, gotBody)
		require.Len(t, flushes, 2)
		require.Contains(t, flushes[1], "response.failed")
	})

	t.Run("bare error synthesizes failed before done", func(t *testing.T) {
		body := "data: {\"type\":\"error\",\"error\":{\"message\":\"failed\"}}\n\n" +
			"data: [DONE]\n\n"
		recorder := newOpenAIResponseFlushRecorder()

		result, err := runOpenAIResponseFlushTest(recorder, io.NopCloser(strings.NewReader(body)), config.GatewayConfig{})

		require.Error(t, err)
		require.NotNil(t, result)
		gotBody, flushes := recorder.snapshot()
		require.NotContains(t, gotBody, `"type":"error"`)
		require.Equal(t, 1, strings.Count(gotBody, `"type":"response.failed"`))
		require.Contains(t, gotBody, `"status":"failed"`)
		require.NotContains(t, gotBody, "[DONE]")
		require.Len(t, flushes, 1)
	})

	t.Run("bare error forwards following failed and drains terminal usage", func(t *testing.T) {
		errorEvent := "data: {\"type\":\"error\",\"error\":{\"code\":\"invalid_request\",\"message\":\"bad request\"}}\n\n"
		body := errorEvent +
			"data: {\"type\":\"response.failed\",\"response\":{\"id\":\"resp_failed\",\"usage\":{\"input_tokens\":9,\"output_tokens\":2},\"error\":{\"code\":\"invalid_request\",\"message\":\"bad request\"}}}\n\n" +
			"data: [DONE]\n\n"
		recorder := newOpenAIResponseFlushRecorder()

		result, err := runOpenAIResponseFlushTest(recorder, io.NopCloser(strings.NewReader(body)), config.GatewayConfig{})

		require.Error(t, err)
		require.NotNil(t, result)
		require.Equal(t, 9, result.usage.InputTokens)
		require.Equal(t, 2, result.usage.OutputTokens)
		gotBody, flushes := recorder.snapshot()
		require.NotContains(t, gotBody, `"type":"error"`)
		require.Equal(t, 1, strings.Count(gotBody, `"type":"response.failed"`))
		require.Contains(t, gotBody, `"id":"resp_failed"`)
		require.NotContains(t, gotBody, "[DONE]")
		require.Len(t, flushes, 1)
	})

	t.Run("non-retryable error event flushes at boundary", func(t *testing.T) {
		body := "data: {\"type\":\"error\",\"error\":{\"code\":\"invalid_request\",\"message\":\"bad request\"}}\n\n" +
			"data: [DONE]\n\n"
		recorder := newOpenAIResponseFlushRecorder()

		result, err := runOpenAIResponseFlushTest(recorder, io.NopCloser(strings.NewReader(body)), config.GatewayConfig{})

		require.Error(t, err)
		require.NotNil(t, result)
		gotBody, flushes := recorder.snapshot()
		require.NotContains(t, gotBody, `"type":"error"`)
		require.Equal(t, 1, strings.Count(gotBody, `"type":"response.failed"`))
		require.Len(t, flushes, 1)
	})
}

func TestOpenAIResponseFlush_BareErrorFollowedByCompletedUsesCompletedTerminal(t *testing.T) {
	body := "data: {\"type\":\"error\",\"error\":{\"code\":\"transient\",\"message\":\"retrying\"}}\n\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_recovered\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":7,\"output_tokens\":3}}}\n\n"
	recorder := newOpenAIResponseFlushRecorder()

	result, err := runOpenAIResponseFlushTest(recorder, io.NopCloser(strings.NewReader(body)), config.GatewayConfig{})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 7, result.usage.InputTokens)
	require.Equal(t, 3, result.usage.OutputTokens)
	gotBody, _ := recorder.snapshot()
	require.NotContains(t, gotBody, `"type":"error"`)
	require.NotContains(t, gotBody, `"type":"response.failed"`)
	require.Contains(t, gotBody, `"type":"response.completed"`)
}

func TestOpenAIResponseFlush_CompatibleAPIKeyDoesNotUseCodexBareErrorSynthesis(t *testing.T) {
	body := "data: {\"type\":\"error\",\"error\":{\"code\":\"provider_error\",\"message\":\"provider failed\"}}\n\n"
	recorder := newOpenAIResponseFlushRecorder()
	account := &Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	result, err := runOpenAIResponseFlushTestWithAccount(recorder, io.NopCloser(strings.NewReader(body)), config.GatewayConfig{}, account)

	require.Error(t, err)
	require.NotNil(t, result)
	gotBody, _ := recorder.snapshot()
	require.Contains(t, gotBody, `"type":"error"`)
	require.NotContains(t, gotBody, `"type":"response.failed"`)
}

func TestOpenAIResponseFlush_RecentBareErrorAllowsCompletedBeforeIdleTimeout(t *testing.T) {
	reader, writer := io.Pipe()
	defer func() { _ = writer.Close() }()
	recorder := newOpenAIResponseFlushRecorder()
	resultCh, errCh := runOpenAIResponseFlushTestAsync(recorder, reader, config.GatewayConfig{StreamDataIntervalTimeout: 1})

	// Place the bare error shortly before the first ticker firing. It is fresh
	// data, so the ticker must leave the stream open for an authoritative event.
	time.Sleep(700 * time.Millisecond)
	_, err := io.WriteString(writer, "data: {\"type\":\"error\",\"error\":{\"code\":\"transient\",\"message\":\"retrying\"}}\n\n")
	require.NoError(t, err)
	time.Sleep(500 * time.Millisecond)
	_, err = io.WriteString(writer, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_late\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":5,\"output_tokens\":1}}}\n\n")
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	require.NoError(t, <-errCh)
	result := <-resultCh
	require.NotNil(t, result)
	require.Equal(t, 5, result.usage.InputTokens)
	gotBody, _ := recorder.snapshot()
	require.Contains(t, gotBody, `"type":"response.completed"`)
	require.NotContains(t, gotBody, `"type":"response.failed"`)
}

func TestOpenAIResponseFlush_BareErrorTimeoutSynthesizesFailed(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.GatewayConfig
	}{
		{
			name: "stream interval timeout",
			cfg:  config.GatewayConfig{StreamDataIntervalTimeout: 1},
		},
		{
			name: "first output timeout",
			cfg:  config.GatewayConfig{OpenAIFirstOutputTimeoutSeconds: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader, writer := io.Pipe()
			defer func() { _ = writer.Close() }()
			recorder := newOpenAIResponseFlushRecorder()
			resultCh, errCh := runOpenAIResponseFlushTestAsync(recorder, reader, tt.cfg)

			_, writeErr := io.WriteString(writer, "data: {\"type\":\"error\",\"error\":{\"code\":\"invalid_request\",\"message\":\"bad request\"}}\n\n")
			require.NoError(t, writeErr)

			select {
			case err := <-errCh:
				require.Error(t, err)
				require.Contains(t, err.Error(), "upstream response failed")
			case <-time.After(3 * time.Second):
				t.Fatal("timed out waiting for bare error terminal synthesis")
			}
			require.NotNil(t, <-resultCh)
			body, flushes := recorder.snapshot()
			require.NotContains(t, body, `"type":"error"`)
			require.Equal(t, 1, strings.Count(body, `"type":"response.failed"`))
			require.Len(t, flushes, 1)
		})
	}
}

func TestOpenAIResponseFlush_ReusedTypeKeepsSSEBytesAndTerminalSemantics(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		flushCount int
	}{
		{
			name:       "whitespace around done",
			body:       "data: \t[DONE]  \n\n",
			flushCount: 1,
		},
		{
			name:       "invalid JSON before done",
			body:       "data: {\"type\":\n\ndata: [DONE]\n\n",
			flushCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := newOpenAIResponseFlushRecorder()

			result, err := runOpenAIResponseFlushTest(recorder, io.NopCloser(strings.NewReader(tt.body)), config.GatewayConfig{})

			require.NoError(t, err)
			require.NotNil(t, result)
			gotBody, flushes := recorder.snapshot()
			require.Equal(t, tt.body, gotBody)
			require.Len(t, flushes, tt.flushCount)
		})
	}
}

func TestOpenAIResponseFlush_ClientDisconnectStillDrainsUsage(t *testing.T) {
	first := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"a\"}\n\n"
	terminal := "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":7,\"output_tokens\":5,\"input_tokens_details\":{\"cached_tokens\":2}}}}\n\n"
	recorder := newOpenAIResponseFlushRecorder()
	recorder.failAfterWrites = 1

	result, err := runOpenAIResponseFlushTest(recorder, io.NopCloser(strings.NewReader(first+terminal)), config.GatewayConfig{})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 7, result.usage.InputTokens)
	require.Equal(t, 5, result.usage.OutputTokens)
	require.Equal(t, 2, result.usage.CacheReadInputTokens)
	gotBody, flushes := recorder.snapshot()
	require.Equal(t, first, gotBody)
	require.Len(t, flushes, 1)
}

func runOpenAIResponseFlushTest(recorder *openAIResponseFlushRecorder, body io.ReadCloser, gatewayCfg config.GatewayConfig, responseHeaders ...http.Header) (*openaiStreamingResult, error) {
	return runOpenAIResponseFlushTestWithAccount(recorder, body, gatewayCfg, &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, responseHeaders...)
}

func runOpenAIResponseFlushTestWithAccount(recorder *openAIResponseFlushRecorder, body io.ReadCloser, gatewayCfg config.GatewayConfig, account *Account, responseHeaders ...http.Header) (*openaiStreamingResult, error) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	svc := &OpenAIGatewayService{
		cfg:           &config.Config{Gateway: gatewayCfg},
		toolCorrector: NewCodexToolCorrector(),
	}
	header := http.Header{"Content-Type": []string{"text/event-stream"}}
	if len(responseHeaders) > 0 && responseHeaders[0] != nil {
		header = responseHeaders[0].Clone()
		if header.Get("Content-Type") == "" {
			header.Set("Content-Type", "text/event-stream")
		}
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     header,
		Body:       body,
	}
	return svc.handleStreamingResponse(context.Background(), resp, c, account, time.Now(), "gpt-5", "gpt-5")
}

func runOpenAIResponseFlushTestAsync(recorder *openAIResponseFlushRecorder, body io.ReadCloser, gatewayCfg config.GatewayConfig, responseHeaders ...http.Header) (<-chan *openaiStreamingResult, <-chan error) {
	resultCh := make(chan *openaiStreamingResult, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := runOpenAIResponseFlushTest(recorder, body, gatewayCfg, responseHeaders...)
		resultCh <- result
		errCh <- err
	}()
	return resultCh, errCh
}

func waitOpenAIResponseFlushCount(t *testing.T, recorder *openAIResponseFlushRecorder, want int) {
	t.Helper()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for {
		select {
		case count := <-recorder.flushEvents:
			if count >= want {
				return
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for flush %d", want)
		}
	}
}

func waitOpenAIResponseFlushSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for stream signal")
	}
}
