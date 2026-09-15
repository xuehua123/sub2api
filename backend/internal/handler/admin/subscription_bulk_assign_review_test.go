package admin

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type bulkAssignReviewGroupRepo struct {
	service.GroupRepository
	calls  int
	cancel context.CancelFunc
	detail string
}

func (repo *bulkAssignReviewGroupRepo) GetByID(ctx context.Context, id int64) (*service.Group, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	repo.calls++
	if repo.cancel != nil {
		repo.cancel()
	}
	return nil, fmt.Errorf("group %d unavailable: %s", id, repo.detail)
}

func TestBulkPlanAssignHandler_RequiresDurableKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.SetDefaultIdempotencyCoordinator(nil)
	t.Cleanup(func() { service.SetDefaultIdempotencyCoordinator(nil) })
	router := gin.New()
	path := "/api/v1/admin/subscriptions/bulk-assign"
	router.POST(path, NewSubscriptionHandler(nil).BulkAssign)
	body := `{"user_ids":[1],"plan_id":9,"group_id":1}`
	for _, key := range []string{"", "   "} {
		require.Equal(t, http.StatusBadRequest, bulkActionHandlerRequest(router, path, body, key).Code)
	}
	require.Equal(t, http.StatusServiceUnavailable, bulkActionHandlerRequest(router, path, body, "plan-operation").Code)
}

func TestBulkPlanAssignHandler_ReplaysAfterClientCancellation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &cancellationAwareAdminIdempotencyRepo{memoryIdempotencyRepoStub: newMemoryIdempotencyRepoStub()}
	service.SetDefaultIdempotencyCoordinator(service.NewIdempotencyCoordinator(store, service.DefaultIdempotencyConfig()))
	t.Cleanup(func() { service.SetDefaultIdempotencyCoordinator(nil) })
	requestCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	repo := &bulkAssignReviewGroupRepo{cancel: cancel, detail: strings.Repeat("x", 70*1024)}
	svc := service.NewSubscriptionService(repo, nil, nil, nil, nil)
	t.Cleanup(svc.Stop)
	router := gin.New()
	path := "/api/v1/admin/subscriptions/bulk-assign"
	router.Use(func(ctx *gin.Context) { ctx.Request = ctx.Request.WithContext(requestCtx) })
	router.POST(path, NewSubscriptionHandler(svc).BulkAssign)
	body := `{"user_ids":[1,2,1],"plan_id":9,"group_id":1}`
	first := bulkActionHandlerRequest(router, path, body, "plan-operation")
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	require.Greater(t, first.Body.Len(), 64*1024)
	require.Equal(t, 2, repo.calls)
	require.Equal(t, 1, store.saves)
	replayed := bulkActionHandlerRequest(router, path, body, "plan-operation")
	require.Equal(t, http.StatusOK, replayed.Code, replayed.Body.String())
	require.Equal(t, "true", replayed.Header().Get("X-Idempotency-Replayed"))
	require.JSONEq(t, first.Body.String(), replayed.Body.String())
	require.Equal(t, 2, repo.calls)
	conflict := bulkActionHandlerRequest(router, path, `{"user_ids":[1],"plan_id":10,"group_id":1}`, "plan-operation")
	require.Equal(t, http.StatusConflict, conflict.Code)
	require.Equal(t, 2, repo.calls)
	fresh := bulkActionHandlerRequest(router, path, body, "new-plan-operation")
	require.Equal(t, http.StatusOK, fresh.Code, fresh.Body.String())
	require.Equal(t, 4, repo.calls)
}
