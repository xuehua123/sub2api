package admin

import (
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
)

func TestSubscriptionListRejectsInvalidExtendedFilters(t *testing.T) {
	for _, query := range []string{"plan_id=-1", "plan_id=abc", "source=unknown", "monthly_quota=any", "expires_within_days=-1", "expires_within_days=36500"} {
		t.Run(query, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/api/v1/admin/subscriptions?"+query, nil)
			(&SubscriptionHandler{}).List(c)
			require.Equal(t, 400, w.Code)
		})
	}
}
