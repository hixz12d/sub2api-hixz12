package service

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"
)

var errOpenAIWSHTTPBridgeReadTimeout = errors.New("websocket HTTP bridge upstream inactivity timeout")

type openAIWSHTTPBridgeReplaySafeKey struct{}

// The activity deadline covers header wait and body reads. A separate optional
// first-output deadline cannot be extended by SSE comments or metadata. These
// timers are attempt-local; cancellation always reaches the actual HTTP request.
type openAIWSHTTPBridgeWatchdog struct {
	ctx             context.Context
	cancel          context.CancelCauseFunc
	mu              sync.Mutex
	idle            time.Duration
	deadline        time.Time
	timer           *time.Timer
	stopped         bool
	firstOutput     *openAIFirstOutputHeaderGuard
	outputStarted   bool
	disconnected    bool
	disconnectCause error
	drainTimer      *time.Timer
}

func newOpenAIWSHTTPBridgeWatchdog(parent context.Context, idle, firstOutput time.Duration) *openAIWSHTTPBridgeWatchdog {
	ctx, cancel := context.WithCancelCause(parent)
	g := &openAIWSHTTPBridgeWatchdog{ctx: ctx, cancel: cancel, idle: idle}
	if firstOutput > 0 {
		g.ctx, g.firstOutput = newOpenAIFirstOutputHeaderGuard(ctx, nil, time.Now().Add(firstOutput))
	}
	if idle > 0 {
		g.deadline = time.Now().Add(idle)
		g.timer = time.AfterFunc(idle, func() {
			g.mu.Lock()
			defer g.mu.Unlock()
			if !g.stopped {
				if remaining := time.Until(g.deadline); remaining > 0 {
					g.timer.Reset(remaining)
					return
				}
				g.stopped = true
				g.cancel(errOpenAIWSHTTPBridgeReadTimeout)
			}
		})
	}
	return g
}

func (g *openAIWSHTTPBridgeWatchdog) activity() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.stopped && g.timer != nil {
		g.deadline = time.Now().Add(g.idle)
		g.timer.Reset(g.idle)
	}
}

func (g *openAIWSHTTPBridgeWatchdog) semanticOutput() {
	if g.firstOutput != nil {
		g.firstOutput.stopHeaderWait()
	}
}

func (g *openAIWSHTTPBridgeWatchdog) clientOutput() {
	g.mu.Lock()
	g.outputStarted = true
	g.mu.Unlock()
}

func (g *openAIWSHTTPBridgeWatchdog) clientClosed(cause error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.stopped || g.disconnected {
		return
	}
	g.disconnected = true
	g.disconnectCause = cause
	if !g.outputStarted {
		g.cancel(cause)
		return
	}
	// Preserve terminal usage for a response already sent to the client, but
	// never retain a disconnected turn indefinitely, even if data keeps arriving.
	drain := 30 * time.Second
	if g.idle > 0 && g.idle < drain {
		drain = g.idle
	}
	g.drainTimer = time.AfterFunc(drain, func() { g.cancel(cause) })
}

func (g *openAIWSHTTPBridgeWatchdog) clientDisconnected() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.disconnected
}

func (g *openAIWSHTTPBridgeWatchdog) disconnectError() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.disconnectCause
}

func (g *openAIWSHTTPBridgeWatchdog) close() {
	g.mu.Lock()
	g.stopped = true
	if g.timer != nil {
		g.timer.Stop()
	}
	if g.drainTimer != nil {
		g.drainTimer.Stop()
	}
	g.mu.Unlock()
	if g.firstOutput != nil {
		g.firstOutput.close()
	}
	g.cancel(context.Canceled)
}

type openAIWSHTTPBridgeActivityBody struct {
	io.ReadCloser
	watchdog *openAIWSHTTPBridgeWatchdog
}

func (b *openAIWSHTTPBridgeActivityBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		b.watchdog.activity()
	}
	return n, err
}
