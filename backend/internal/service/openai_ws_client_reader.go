package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
)

const openAIWSClientReaderGinKey = "openai_ws_client_reader"

type openAIWSClientReaderContextKey struct{}

// openAIWSClientReader belongs to the inbound connection, not an account
// attempt. Bridge turns must keep reading control frames while HTTP is busy.
// A single queued application message bounds read-ahead; it is never executed
// concurrently with the current turn.
type openAIWSClientReader struct {
	conn     *coderws.Conn
	parent   context.Context
	ctx      context.Context
	cancel   context.CancelCauseFunc
	messages chan openAIWSClientReadResult
	done     chan struct{}
	once     sync.Once
	started  atomic.Bool
}

func openAIWSClientReaderForIngress(ctx context.Context, c *gin.Context, conn *coderws.Conn) *openAIWSClientReader {
	if cached, ok := c.Get(openAIWSClientReaderGinKey); ok {
		if reader, ok := cached.(*openAIWSClientReader); ok && reader.conn == conn {
			return reader
		}
	}
	parent := ctx
	if parent == nil {
		parent = context.Background()
	}
	readerCtx, cancel := context.WithCancelCause(context.Background())
	reader := &openAIWSClientReader{
		conn: conn, parent: parent, ctx: readerCtx, cancel: cancel,
		messages: make(chan openAIWSClientReadResult, 1), done: make(chan struct{}),
	}
	c.Set(openAIWSClientReaderGinKey, reader)
	return reader
}

func openAIWSClientReaderFromContext(ctx context.Context) *openAIWSClientReader {
	if ctx == nil {
		return nil
	}
	reader, _ := ctx.Value(openAIWSClientReaderContextKey{}).(*openAIWSClientReader)
	if reader == nil || !reader.started.Load() {
		return nil
	}
	return reader
}

func (r *openAIWSClientReader) start() {
	r.once.Do(func() {
		r.started.Store(true)
		go r.run()
	})
}

func (r *openAIWSClientReader) run() {
	defer close(r.done)
	defer close(r.messages)
	stopParent := context.AfterFunc(r.parent, func() {
		status, reason := openAIWSClientControlClose(r.parent)
		r.close(status, reason, context.Cause(r.parent))
	})
	defer stopParent()
	for {
		messageType, payload, err := r.conn.Read(context.Background())
		if err != nil {
			r.cancel(NewOpenAIWSClientCloseError(coderws.StatusGoingAway, "websocket client disconnected", err))
			return
		}
		select {
		case r.messages <- openAIWSClientReadResult{messageType: messageType, payload: payload}:
		case <-r.ctx.Done():
			return
		default:
			// Do not let pipelined requests block the reader and prevent Close
			// or Ping processing indefinitely, or retain unbounded request bodies.
			r.close(coderws.StatusPolicyViolation, "too many pending websocket messages", nil)
			return
		}
	}
}

func (r *openAIWSClientReader) close(status coderws.StatusCode, reason string, cause error) {
	r.cancel(NewOpenAIWSClientCloseError(status, reason, cause))
	_ = r.conn.Close(status, reason)
	_ = r.conn.CloseNow()
}

func openAIWSClientControlClose(ctx context.Context) (coderws.StatusCode, string) {
	if errors.Is(context.Cause(ctx), ErrOpenAIWSIngressLeaseLost) {
		return coderws.StatusTryAgainLater, "websocket ingress capacity lease lost; please reconnect"
	}
	return coderws.StatusGoingAway, "websocket request canceled"
}

// OpenAIWSClientWaitContext binds admission work to the actual client connection.
// Callers must bind acquired-slot cleanup to the turn lifetime, not this short
// wait context, which is canceled as soon as admission finishes.
func OpenAIWSClientWaitContext(parent context.Context, c *gin.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancelCause(parent)
	stop := func() bool { return true }
	if c != nil {
		if value, ok := c.Get(openAIWSClientReaderGinKey); ok {
			if reader, ok := value.(*openAIWSClientReader); ok && reader.started.Load() {
				stop = context.AfterFunc(reader.ctx, func() { cancel(context.Cause(reader.ctx)) })
				if reader.ctx.Err() != nil {
					cancel(context.Cause(reader.ctx))
				}
			}
		}
	}
	return ctx, func() { stop(); cancel(context.Canceled) }
}
