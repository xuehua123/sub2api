//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type checkoutInfoConfigServiceStub struct {
	plansErr error
}

func (s *checkoutInfoConfigServiceStub) GetPaymentConfig(_ context.Context) (*service.PaymentConfig, error) {
	return &service.PaymentConfig{
		BalanceDisabled:           false,
		BalanceRechargeMultiplier: 1,
		RechargeFeeRate:           0,
	}, nil
}

func (s *checkoutInfoConfigServiceStub) GetAvailableMethodLimits(_ context.Context) (*service.MethodLimitsResponse, error) {
	return &service.MethodLimitsResponse{
		Methods:   map[string]service.MethodLimits{},
		GlobalMin: 1,
		GlobalMax: 100,
	}, nil
}

func (s *checkoutInfoConfigServiceStub) ListPlansForSale(_ context.Context) ([]*dbent.SubscriptionPlan, error) {
	return nil, s.plansErr
}

func (s *checkoutInfoConfigServiceStub) GetGroupInfoMap(_ context.Context, _ []*dbent.SubscriptionPlan) map[int64]service.PlanGroupInfo {
	return nil
}

func (s *checkoutInfoConfigServiceStub) GetGroupPlatformMap(_ context.Context, _ []*dbent.SubscriptionPlan) map[int64]string {
	return nil
}

func (s *checkoutInfoConfigServiceStub) GetUserRefundEligibleInstanceIDs(_ context.Context) ([]string, error) {
	return nil, nil
}

func TestGetCheckoutInfoReturnsErrorWhenListPlansForSaleFails(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	h := NewPaymentHandler(nil, &checkoutInfoConfigServiceStub{
		plansErr: errors.New("plan query failed"),
	}, nil)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/payment/checkout-info", nil)

	h.GetCheckoutInfo(ctx)

	require.Equal(t, http.StatusInternalServerError, recorder.Code)

	var resp response.Response
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, http.StatusInternalServerError, resp.Code)
}
