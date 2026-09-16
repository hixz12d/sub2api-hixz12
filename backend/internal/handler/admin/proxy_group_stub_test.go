package admin

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (s *stubAdminService) ListProxyGroups(context.Context) ([]service.ProxyGroup, error) {
	return []service.ProxyGroup{}, nil
}
func (s *stubAdminService) SaveProxyGroup(context.Context, *service.ProxyGroup) error { return nil }
func (s *stubAdminService) DeleteProxyGroup(context.Context, int64) error             { return nil }
func (s *stubAdminService) ListProxiesByGroup(context.Context, int, int, string, string, string, string, string, int64) ([]service.ProxyWithAccountCount, int64, error) {
	return nil, 0, nil
}
