package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
	"strconv"
	"strings"
)

func (h *AccountHandler) QuestionAssessments(c *gin.Context) {
	actor, ok := questionActor(c)
	if !ok {
		return
	}
	raw := c.Query("ids")
	if len(raw) > 2100 {
		response.BadRequest(c, "Invalid assessment scope")
		return
	}
	parts := strings.Split(raw, ",")
	if len(parts) > 100 {
		response.BadRequest(c, "Invalid assessment scope")
		return
	}
	ids := make([]int64, 0, len(parts))
	for _, part := range parts {
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid assessment scope")
			return
		}
		ids = append(ids, id)
	}
	items, err := h.questionReviews.Summaries(c.Request.Context(), actor, c.Query("scope"), ids)
	if err != nil {
		questionError(c, err)
		return
	}
	response.Success(c, gin.H{"items": items})
}
