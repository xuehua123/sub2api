package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestModelPriceOperationsHaveSpecificAuditActions(t *testing.T) {
	h := &ModelPriceHandler{}
	for _, tc := range []struct {
		action string
		call   func(*gin.Context)
	}{
		{"visibility.groups", h.UpdateHiddenGroups}, {"visibility.models", h.UpdateHiddenModel}, {"display_price", h.UpdateCustomPrice}, {"sync_catalog", h.SyncCatalog},
	} {
		t.Run(tc.action, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/api/v1/admin/model-prices/test", strings.NewReader("{"))
			c.Request.Header.Set("Content-Type", "application/json")
			tc.call(c)
			action, ok := c.Get("audit_action")
			require.True(t, ok)
			require.Equal(t, "admin.model_prices."+tc.action, action)
		})
	}
}
