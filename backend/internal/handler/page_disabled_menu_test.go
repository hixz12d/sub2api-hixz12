package handler

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type disabledMenuRepo struct {
	service.SettingRepository
	raw string
}

func (r *disabledMenuRepo) GetValue(context.Context, string) (string, error) { return r.raw, nil }

func TestDisabledCustomMenuPageIsUnavailable(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want bool
	}{
		{`[{"url":"md:shop","visibility":"user"}]`, true},
		{`[{"url":"md:shop","visibility":"user","enabled":true}]`, true},
		{`[{"url":"md:shop","visibility":"user","enabled":false}]`, false},
	} {
		repo := &disabledMenuRepo{raw: tc.raw}
		h := NewPageHandler(t.TempDir(), service.NewSettingService(repo, &config.Config{}))
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/api/v1/pages/shop", nil)
		_, found := h.findSlugVisibility(c, "shop")
		require.Equal(t, tc.want, found)
	}
}
