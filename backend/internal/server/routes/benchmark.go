package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/gin-gonic/gin"
)

func registerBenchmarkRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	if h.Admin.Account != nil {
		admin.GET("/question-assessments", h.Admin.Account.QuestionAssessments)
		admin.GET("/monitor-policies", h.Admin.Account.MonitorPolicies)
		admin.GET("/monitor-policies/:policy/reports", h.Admin.Account.MonitorPolicyReports)
		admin.POST("/monitor-policies", h.Admin.Account.SaveMonitorPolicy)
		admin.PUT("/monitor-policies/:policy", h.Admin.Account.SaveMonitorPolicy)
		admin.DELETE("/monitor-policies/:policy", h.Admin.Account.DeleteMonitorPolicy)
		admin.POST("/monitor-policies/:policy/disable", h.Admin.Account.DisableMonitorPolicy)
		admin.POST("/detector/plans", h.Admin.Account.DetectorPlan)
		admin.POST("/detector/jobs", h.Admin.Account.DetectorCreate)
		admin.GET("/detector/jobs/:job", h.Admin.Account.DetectorJob)
		admin.POST("/detector/jobs/:job/cancel", h.Admin.Account.DetectorCancel)
	}
	if h.Admin.Benchmark == nil {
		return
	}
	benchmarks := admin.Group("/monitor-benchmarks")
	benchmarks.GET("", h.Admin.Benchmark.List)
	benchmarks.POST("", h.Admin.Benchmark.Stage)
	benchmarks.POST("/:id/approve", h.Admin.Benchmark.Approve)
	benchmarks.POST("/:id/activate", h.Admin.Benchmark.Activate)
	benchmarks.POST("/:id/withdraw", h.Admin.Benchmark.Withdraw)
}
