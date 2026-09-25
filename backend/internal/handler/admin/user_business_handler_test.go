//go:build unit

package admin

import (
	"context"
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

type businessHandlerRepo struct {
	calls int
	rate  float64
}

func (r *businessHandlerRepo) Report(_ context.Context, q service.UserBusinessQuery) (json.RawMessage, error) {
	r.calls++
	r.rate = q.USDCNY
	return json.RawMessage(`{"items":[],"total":0}`), nil
}
func (r *businessHandlerRepo) Detail(ctx context.Context, q service.UserBusinessQuery) (json.RawMessage, error) {
	return r.Report(ctx, q)
}
func TestUserBusinessCacheAndBadFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &businessHandlerRepo{}
	h := NewUserBusinessHandler(service.NewUserBusinessService(repo, nil))
	router := gin.New()
	router.GET("/report", h.List)
	request := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		return w
	}
	for _, path := range []string{"/report?usd_cny=NaN", "/report?sort=DROP", "/report?start_date=nope", "/report?page=bad", "/report?usd_cny=-1"} {
		require.Equal(t, 400, request(path).Code)
	}
	require.Zero(t, repo.calls)
	w := request("/report?usd_cny=7")
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), `"items":[]`)
	require.Equal(t, "miss", w.Header().Get("X-Snapshot-Cache"))
	w = request("/report?usd_cny=7")
	require.Equal(t, "hit", w.Header().Get("X-Snapshot-Cache"))
	require.Equal(t, 1, repo.calls)
	require.Equal(t, 200, request("/report?usd_cny=6").Code)
	require.Equal(t, 2, repo.calls)
	require.Equal(t, 6.0, repo.rate)
}
