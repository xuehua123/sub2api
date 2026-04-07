package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func RegisterLobeHubRoutes(
	v1 *gin.RouterGroup,
	h *handler.Handlers,
	jwtAuth middleware.JWTAuthMiddleware,
	settingService *service.SettingService,
) {
	v1.GET("/lobehub/bridge", h.LobeHub.Bridge)
	v1.GET("/lobehub/oidc/.well-known/openid-configuration", h.LobeHub.Discovery)
	v1.GET("/lobehub/oidc/jwks", h.LobeHub.JWKS)
	v1.GET("/lobehub/oidc/authorize", h.LobeHub.Authorize)
	v1.POST("/lobehub/oidc/token", h.LobeHub.Token)
	v1.GET("/lobehub/oidc/userinfo", h.LobeHub.UserInfo)
	v1.POST("/lobehub/bootstrap-exchange", h.LobeHub.BootstrapExchange)
	v1.POST("/lobehub/config-probe/compare", h.LobeHub.CompareCurrentConfig)
	v1.GET("/lobehub/bootstrap/consume", h.LobeHub.ConsumeBootstrap)

	authenticated := v1.Group("")
	authenticated.Use(gin.HandlerFunc(jwtAuth))
	authenticated.Use(middleware.BackendModeUserGuard(settingService))
	{
		authenticated.POST("/lobehub/launch-ticket", h.LobeHub.CreateLaunchTicket)
		authenticated.POST("/lobehub/oidc-web-session", h.LobeHub.CreateOIDCWebSession)
	}
}
