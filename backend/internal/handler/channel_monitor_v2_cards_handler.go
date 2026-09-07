package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *ChannelMonitorV2Handler) Cards(c *gin.Context)           { h.cards(c, false, false) }
func (h *ChannelMonitorV2Handler) AdminCards(c *gin.Context)      { h.cards(c, true, false) }
func (h *ChannelMonitorV2Handler) CardDetail(c *gin.Context)      { h.cards(c, false, true) }
func (h *ChannelMonitorV2Handler) AdminCardDetail(c *gin.Context) { h.cards(c, true, true) }

func (h *ChannelMonitorV2Handler) cards(c *gin.Context, admin, detail bool) {
	groups, err := parseChannelMonitorV2GroupIDs(queryList(c, "group_id"))
	if err != nil {
		response.BadRequest(c, "invalid group_id")
		return
	}
	filter := service.ChannelMonitorV2Filter{Platforms: queryList(c, "platform"), GroupIDs: groups, Models: queryList(c, "model")}
	if detail && (len(filter.Platforms) != 1 || len(groups) != 1 || len(filter.Models) != 1) {
		response.BadRequest(c, "card detail requires one platform, group_id and model")
		return
	}
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil {
		response.BadRequest(c, "invalid page")
		return
	}
	size, err := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if err != nil {
		response.BadRequest(c, "invalid page_size")
		return
	}
	q := service.ChannelMonitorV2CardsQuery{Filter: filter, Page: page, PageSize: size, IncludeAdmin: admin}
	if raw := c.Query("as_of"); raw != "" {
		at, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil || !strings.HasSuffix(raw, "Z") {
			response.BadRequest(c, "as_of must be an ISO8601 UTC timestamp")
			return
		}
		q.AsOf = &at
	}
	if !h.scopeFilter(c, &q.Filter, admin) {
		return
	}
	if detail {
		q.Page, q.PageSize = 1, 1
	}
	result, err := h.service.Cards(c.Request.Context(), q)
	if err != nil {
		if errors.Is(err, service.ErrChannelMonitorV2InvalidRange) {
			response.BadRequest(c, err.Error())
			return
		}
		response.ErrorFrom(c, err)
		return
	}
	if detail && len(result.Items) == 0 {
		response.Error(c, http.StatusNotFound, "status card not found")
		return
	}
	// Detail uses the same envelope so as_of, freshness and coverage stay explicit.
	if admin {
		response.Success(c, result.AdminResponse())
		return
	}
	response.Success(c, result)
}
