package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

func RegisterIssueRoutes(
	v1 *gin.RouterGroup,
	h *handler.Handlers,
	jwtAuth middleware.JWTAuthMiddleware,
	adminAuth middleware.AdminAuthMiddleware,
	settingService *service.SettingService,
) {
	issues := v1.Group("/issues")
	{
		issues.GET("", h.SupportIssue.List)
		issues.GET("/mine", gin.HandlerFunc(jwtAuth), middleware.BackendModeUserGuard(settingService), h.SupportIssue.Mine)
		issues.GET("/notifications", gin.HandlerFunc(jwtAuth), middleware.BackendModeUserGuard(settingService), h.SupportIssue.Notifications)
		issues.GET("/trending", h.SupportIssue.Trending)
		issues.GET("/attachments/:id/file", h.SupportIssue.ServeAttachmentFile)
		issues.GET("/:id", h.SupportIssue.Get)
	}

	authenticated := v1.Group("/issues")
	authenticated.Use(gin.HandlerFunc(jwtAuth))
	authenticated.Use(middleware.BackendModeUserGuard(settingService))
	{
		authenticated.POST("", h.SupportIssue.Create)
		authenticated.POST("/attachments", h.SupportIssue.UploadAttachment)
		authenticated.POST("/search-suggestions", h.SupportIssue.SearchSuggestions)
		authenticated.POST("/:id/comments", h.SupportIssue.AddComment)
		authenticated.PATCH("/:id/resolve", h.SupportIssue.Resolve)
	}

	adminIssues := v1.Group("/admin/issues")
	adminIssues.Use(gin.HandlerFunc(adminAuth))
	{
		adminIssues.GET("", h.Admin.SupportIssue.List)
		adminIssues.GET("/:id", h.Admin.SupportIssue.Get)
		adminIssues.PATCH("/:id/status", h.Admin.SupportIssue.UpdateStatus)
		adminIssues.POST("/:id/reopen", h.Admin.SupportIssue.Reopen)
		adminIssues.POST("/:id/hide", h.Admin.SupportIssue.HideIssue)
		adminIssues.POST("/:id/restore", h.Admin.SupportIssue.RestoreIssue)
		adminIssues.POST("/:id/pin", h.Admin.SupportIssue.PinIssue)
		adminIssues.POST("/:id/unpin", h.Admin.SupportIssue.UnpinIssue)
		adminIssues.POST("/:id/solution", h.Admin.SupportIssue.MarkSolution)
		adminIssues.POST("/:id/solution/clear", h.Admin.SupportIssue.ClearSolution)
		adminIssues.POST("/:id/related-issue", h.Admin.SupportIssue.SetRelatedIssue)
		adminIssues.POST("/:id/related-issue/clear", h.Admin.SupportIssue.ClearRelatedIssue)
		adminIssues.POST("/:id/comments/:comment_id/hide", h.Admin.SupportIssue.HideComment)
		adminIssues.POST("/:id/attachments/:attachment_id/hide", h.Admin.SupportIssue.HideAttachment)
		adminIssues.GET("/:id/events", h.Admin.SupportIssue.Events)
	}
}
