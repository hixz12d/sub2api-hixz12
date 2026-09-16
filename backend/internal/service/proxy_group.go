package service

import (
	"context"
	"strings"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

var (
	ErrProxyGroupNotFound = infraerrors.NotFound("PROXY_GROUP_NOT_FOUND", "代理分组不存在")
	ErrProxyGroupFull     = infraerrors.Conflict("PROXY_GROUP_FULL", "代理分组没有可用容量，请增加代理或调整每个代理的账号上限")
	ErrProxyGroupCapacity = infraerrors.Conflict("PROXY_GROUP_CAPACITY", "代理已达到分组账号上限")
)

// ProxyGroup is an egress pool, unrelated to account routing groups.
type ProxyGroup struct {
	ID                  int64   `json:"id"`
	Name                string  `json:"name"`
	MaxAccountsPerProxy int     `json:"max_accounts_per_proxy"`
	ProxyIDs            []int64 `json:"proxy_ids"`
	AvailableProxyIDs   []int64 `json:"available_proxy_ids"`
}

type ProxyGroupRepository interface {
	ListProxyGroups(context.Context) ([]ProxyGroup, error)
	SaveProxyGroup(context.Context, *ProxyGroup) error
	DeleteProxyGroup(context.Context, int64) error
	ListWithGroupFilterAndAccountCount(context.Context, pagination.PaginationParams, string, string, string, *int64) ([]ProxyWithAccountCount, *pagination.PaginationResult, error)
}

func (s *adminServiceImpl) proxyGroups() (ProxyGroupRepository, error) {
	repo, ok := s.proxyRepo.(ProxyGroupRepository)
	if !ok {
		return nil, infraerrors.ServiceUnavailable("PROXY_GROUPS_UNAVAILABLE", "代理分组暂不可用")
	}
	return repo, nil
}
func (s *adminServiceImpl) ListProxyGroups(ctx context.Context) ([]ProxyGroup, error) {
	repo, err := s.proxyGroups()
	if err != nil {
		return nil, err
	}
	return repo.ListProxyGroups(ctx)
}
func (s *adminServiceImpl) SaveProxyGroup(ctx context.Context, group *ProxyGroup) error {
	group.Name = strings.TrimSpace(group.Name)
	if group.ProxyIDs == nil {
		group.ProxyIDs = []int64{}
	}
	if group.Name == "" || utf8.RuneCountInString(group.Name) > 100 || group.MaxAccountsPerProxy < 1 || group.MaxAccountsPerProxy > 10000 || len(group.ProxyIDs) > 10000 {
		return infraerrors.BadRequest("PROXY_GROUP_INVALID", "分组名称须为 1–100 字，单代理账号上限须为 1–10000")
	}
	seen := make(map[int64]bool, len(group.ProxyIDs))
	for _, id := range group.ProxyIDs {
		if id <= 0 || seen[id] {
			return infraerrors.BadRequest("PROXY_GROUP_MEMBERS_INVALID", "代理列表包含无效或重复的代理")
		}
		seen[id] = true
	}
	repo, err := s.proxyGroups()
	if err != nil {
		return err
	}
	return repo.SaveProxyGroup(ctx, group)
}
func (s *adminServiceImpl) ListProxiesByGroup(ctx context.Context, page, pageSize int, protocol, status, search, sortBy, sortOrder string, groupID int64) ([]ProxyWithAccountCount, int64, error) {
	repo, err := s.proxyGroups()
	if err != nil {
		return nil, 0, err
	}
	proxies, result, err := repo.ListWithGroupFilterAndAccountCount(ctx, pagination.PaginationParams{Page: page, PageSize: pageSize, SortBy: sortBy, SortOrder: sortOrder}, protocol, status, search, &groupID)
	if err != nil {
		return nil, 0, err
	}
	s.attachProxyLatency(ctx, proxies)
	return proxies, result.Total, nil
}

func (s *adminServiceImpl) DeleteProxyGroup(ctx context.Context, id int64) error {
	repo, err := s.proxyGroups()
	if err != nil {
		return err
	}
	return repo.DeleteProxyGroup(ctx, id)
}
