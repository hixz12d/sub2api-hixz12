package service

import (
	"context"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// RequestTiming 记录一次请求在网关内的关键时刻，用于拆分使用记录里的
// 接收 / 网关处理 / 上游等待 三段耗时。一个请求一个对象，同时挂在
// gin context 和 c.Request 的 context 上。
type RequestTiming struct {
	mu sync.Mutex

	bodyReadStart time.Time // 开始读请求体
	bodyReadEnd   time.Time // 请求体读完
	forwardStart  time.Time // Forward 内部 startTime（first_token_ms 的起点），以最后一次为准
	upstreamSend  time.Time // 最后一次发出上游请求
}

const requestTimingKey = "request_timing"

type requestTimingContextKey struct{}

var requestTimingCtxKey = requestTimingContextKey{}

func requestTimingFromGin(c *gin.Context) *RequestTiming {
	if c == nil {
		return nil
	}
	if v, ok := c.Get(requestTimingKey); ok {
		if t, ok := v.(*RequestTiming); ok && t != nil {
			return t
		}
	}
	if c.Request != nil {
		if t, ok := c.Request.Context().Value(requestTimingCtxKey).(*RequestTiming); ok && t != nil {
			return t
		}
	}
	return nil
}

func requestTimingFromContext(ctx context.Context) *RequestTiming {
	if ctx == nil {
		return nil
	}
	if c, ok := ctx.(*gin.Context); ok {
		return requestTimingFromGin(c)
	}
	t, _ := ctx.Value(requestTimingCtxKey).(*RequestTiming)
	return t
}

// EnsureRequestTiming 返回当前请求的计时对象；没有就新建并挂到 gin context
// 和 c.Request 的 context 上。c 或 c.Request 为 nil 时返回 nil。
func EnsureRequestTiming(c *gin.Context) *RequestTiming {
	if c == nil || c.Request == nil {
		return nil
	}
	t := requestTimingFromGin(c)
	if t == nil {
		t = &RequestTiming{}
	}
	if v, ok := c.Get(requestTimingKey); !ok || v != t {
		c.Set(requestTimingKey, t)
	}
	if existing, _ := c.Request.Context().Value(requestTimingCtxKey).(*RequestTiming); existing != t {
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), requestTimingCtxKey, t))
	}
	return t
}

// MarkRequestBodyRead 记录读请求体的起止时刻。只认第一次：预读中间件先读，
// handler 后面从内存再读一次时不覆盖。
func MarkRequestBodyRead(c *gin.Context, start, end time.Time) {
	t := EnsureRequestTiming(c)
	if t == nil || start.IsZero() || end.IsZero() {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.bodyReadEnd.IsZero() {
		return
	}
	t.bodyReadStart = start
	t.bodyReadEnd = end
}

// MarkOpenAIForwardStart 记录 Forward 内部的 startTime（first_token_ms 的起点）。
// 每次覆盖，换号重试或转到其他 forward 函数时以最后一次为准。
func MarkOpenAIForwardStart(c *gin.Context, at time.Time) {
	t := EnsureRequestTiming(c)
	if t == nil || at.IsZero() {
		return
	}
	t.mu.Lock()
	t.forwardStart = at
	t.mu.Unlock()
}

// MarkUpstreamSend 记录“发出上游请求”的时刻（time.Now()）。从 ctx 找计时对象，
// 找不到就什么也不做。每次覆盖。
func MarkUpstreamSend(ctx context.Context) {
	t := requestTimingFromContext(ctx)
	if t == nil {
		return
	}
	now := time.Now()
	t.mu.Lock()
	t.upstreamSend = now
	t.mu.Unlock()
}

// ApplyRequestTiming 按记录的时刻填写 result 里还是 nil 的三个耗时字段，已有值不覆盖：
//   - RequestReceiveMs = 读完请求体 − 开始读
//   - GatewayMs        = 最后一次发上游 − 读完请求体
//   - UpstreamWaitMs   = (Forward 起点 + FirstTokenMs) − 最后一次发上游
//
// 负数按 0 记。
func ApplyRequestTiming(c *gin.Context, result *OpenAIForwardResult) {
	if result == nil {
		return
	}
	t := requestTimingFromGin(c)
	if t == nil {
		return
	}
	t.mu.Lock()
	bodyStart, bodyEnd := t.bodyReadStart, t.bodyReadEnd
	forwardStart, upstreamSend := t.forwardStart, t.upstreamSend
	t.mu.Unlock()

	if result.RequestReceiveMs == nil && !bodyStart.IsZero() && !bodyEnd.IsZero() {
		result.RequestReceiveMs = requestTimingMs(bodyEnd.Sub(bodyStart))
	}
	if result.GatewayMs == nil && !bodyEnd.IsZero() && !upstreamSend.IsZero() {
		result.GatewayMs = requestTimingMs(upstreamSend.Sub(bodyEnd))
	}
	if result.UpstreamWaitMs == nil && result.FirstTokenMs != nil && !forwardStart.IsZero() && !upstreamSend.IsZero() {
		firstToken := forwardStart.Add(time.Duration(*result.FirstTokenMs) * time.Millisecond)
		result.UpstreamWaitMs = requestTimingMs(firstToken.Sub(upstreamSend))
	}
}

func requestTimingMs(d time.Duration) *int {
	ms := int(d / time.Millisecond)
	if ms < 0 {
		ms = 0
	}
	return &ms
}
