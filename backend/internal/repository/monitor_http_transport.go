package repository

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// This wrapper sits beneath explicit fallback transports, so each fallback also
// needs a fresh permit. Cached shared clients and ordinary traffic are unchanged.
func httpClientWithMonitorGuard(client *http.Client, req *http.Request, accountID int64) *http.Client {
	if client == nil || req == nil || service.MonitorOutboundGuardFromContext(req.Context()) == nil {
		return client
	}
	clone := *client
	base := clone.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	clone.Transport = &monitorGuardTransport{base: base, accountID: accountID}
	clone.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &clone
}

type monitorGuardTransport struct {
	base      http.RoundTripper
	accountID int64
}

func (t *monitorGuardTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	guard := service.MonitorOutboundGuardFromContext(req.Context())
	if guard == nil || req.Method != http.MethodPost || req.Body == nil || req.Body == http.NoBody {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, service.ErrMonitorOutboundDenied
	}
	if err := req.Context().Err(); err != nil {
		_ = req.Body.Close()
		return nil, err
	}
	if err := guard.Authorize(req.Context(), t.accountID, req); err != nil {
		_ = req.Body.Close()
		return nil, service.ErrMonitorOutboundDenied
	}
	// Prevent net/http from silently replaying a POST below our accounting hook.
	// Explicit higher-level fallback still passes through this RoundTripper again.
	clone := req.Clone(req.Context())
	clone.GetBody = nil
	return t.base.RoundTrip(clone)
}
