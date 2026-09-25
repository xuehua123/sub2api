//go:build unit

package middleware

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUserBusinessRequiresAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "test-secret", ExpireHour: 1}}
	auth := service.NewAuthService(nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	user := &service.User{ID: 123, Email: "business@test.invalid", Role: service.RoleUser, Status: service.StatusActive, Concurrency: 1}
	repo := &stubUserRepo{getByID: func(context.Context, int64) (*service.User, error) { return user, nil }}
	users := service.NewUserService(repo, nil, nil, nil)
	router := gin.New()
	admin := router.Group("/api/v1/admin", gin.HandlerFunc(NewAdminAuthMiddleware(auth, users, nil, nil)))
	called := false
	for _, path := range []string{"/user-business", "/user-business/fx-rates", "/user-business/:id"} {
		admin.GET(path, func(c *gin.Context) { called = true; c.Status(200) })
	}
	admin.PUT("/user-business/fx-rates", func(c *gin.Context) { called = true; c.Status(200) })
	for _, path := range []string{"/api/v1/admin/user-business", "/api/v1/admin/user-business/fx-rates", "/api/v1/admin/user-business/123"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, 401, w.Code)
		token, err := auth.GenerateToken(context.Background(), user)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, 403, w.Code)
	}
	tokenUser, err := auth.GenerateToken(context.Background(), user)
	require.NoError(t, err)
	reqWrite := httptest.NewRequest(http.MethodPut, "/api/v1/admin/user-business/fx-rates", nil)
	reqWrite.Header.Set("Authorization", "Bearer "+tokenUser)
	writeResult := httptest.NewRecorder()
	router.ServeHTTP(writeResult, reqWrite)
	require.Equal(t, 403, writeResult.Code)
	require.False(t, called)
	user.Role = service.RoleAdmin
	token, err := auth.GenerateToken(context.Background(), user)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/user-business", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code)
}
