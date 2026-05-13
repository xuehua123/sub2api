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
		adminIssues.POST("/:id/comments/:comment_id/hide", h.Admin.SupportIssue.HideComment)
		adminIssues.POST("/:id/attachments/:attachment_id/hide", h.Admin.SupportIssue.HideAttachment)
		adminIssues.GET("/:id/events", h.Admin.SupportIssue.Events)
	}
}
