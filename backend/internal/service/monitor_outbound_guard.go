package service

import (
	"context"
	"errors"
	"net/http"
)

var ErrMonitorOutboundDenied = errors.New("monitor outbound request denied")

type monitorOutboundGuardKey struct{}

// MonitorOutboundGuard is server-owned. HTTP headers never create this marker.
// Authorize persists the dispatch/budget fence immediately before RoundTrip and
// must stop the execution after a denial or an ambiguous commit.
type MonitorOutboundGuard interface {
	Authorize(context.Context, int64, *http.Request) error
}

func WithMonitorOutboundGuard(ctx context.Context, guard MonitorOutboundGuard) context.Context {
	return context.WithValue(ctx, monitorOutboundGuardKey{}, guard)
}

func MonitorOutboundGuardFromContext(ctx context.Context) MonitorOutboundGuard {
	if ctx == nil {
		return nil
	}
	guard, _ := ctx.Value(monitorOutboundGuardKey{}).(MonitorOutboundGuard)
	return guard
}
