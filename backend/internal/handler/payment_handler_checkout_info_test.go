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
	plansErr                       error
	planResponses                  []service.SubscriptionPlanResponse
	alipayMobilePrecreateDeepLink  bool
	officialAlipayVisibleMethod    bool
	officialAlipayVisibleMethodErr error
}

func (s *checkoutInfoConfigServiceStub) GetPaymentConfig(_ context.Context) (*service.PaymentConfig, error) {
	return &service.PaymentConfig{
		BalanceDisabled:               false,
		BalanceRechargeMultiplier:     1,
		RechargeFeeRate:               0,
		AlipayMobilePrecreateDeepLink: s.alipayMobilePrecreateDeepLink,
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

func (s *checkoutInfoConfigServiceStub) ListPlanResponsesForSale(_ context.Context) ([]service.SubscriptionPlanResponse, error) {
	return s.planResponses, s.plansErr
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

func (s *checkoutInfoConfigServiceStub) UsesOfficialAlipayVisibleMethod(_ context.Context) (bool, error) {
	return s.officialAlipayVisibleMethod, s.officialAlipayVisibleMethodErr
}

func TestGetCheckoutInfoReturnsErrorWhenListPlansForSaleFails(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	h := NewPaymentHandler(nil, &checkoutInfoConfigServiceStub{
		plansErr: errors.New("plan query failed"),
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/payment/checkout-info", nil)

	h.GetCheckoutInfo(ctx)

	require.Equal(t, http.StatusInternalServerError, recorder.Code)

	var resp response.Response
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, http.StatusInternalServerError, resp.Code)
}

func TestGetCheckoutInfoReturnsFullPlanGroups(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	h := NewPaymentHandler(nil, &checkoutInfoConfigServiceStub{
		planResponses: []service.SubscriptionPlanResponse{
			{
				ID:              7,
				GroupID:         10,
				GroupIDs:        []int64{10, 11},
				Groups:          []service.PlanGroupInfo{{ID: 10, Name: "A", Platform: service.PlatformOpenAI, RateMultiplier: 1}, {ID: 11, Name: "B", Platform: service.PlatformAnthropic, RateMultiplier: 2}},
				GroupPlatform:   service.PlatformOpenAI,
				GroupName:       "A",
				RateMultiplier:  1,
				Name:            "Pro",
				Description:     "full description",
				Price:           10,
				ValidityDays:    30,
				ValidityUnit:    "day",
				MonthlyLimitUSD: ptrFloat(100),
				OveragePolicy:   service.SubscriptionEntitlementOverageBlock,
				Features:        "[]",
				ForSale:         true,
			},
		},
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/payment/checkout-info", nil)

	h.GetCheckoutInfo(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var resp struct {
		Data struct {
			Plans []struct {
				GroupIDs []int64 `json:"group_ids"`
				Groups   []struct {
					ID   int64  `json:"id"`
					Name string `json:"name"`
				} `json:"groups"`
				Features []string `json:"features"`
			} `json:"plans"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Len(t, resp.Data.Plans, 1)
	require.Equal(t, []int64{10, 11}, resp.Data.Plans[0].GroupIDs)
	require.Len(t, resp.Data.Plans[0].Groups, 2)
	require.Equal(t, "B", resp.Data.Plans[0].Groups[1].Name)
	require.Empty(t, resp.Data.Plans[0].Features)
}

func TestGetCheckoutInfoExposesAlipayMobilePrecreateDeepLinkForOfficialMethod(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	h := NewPaymentHandler(nil, &checkoutInfoConfigServiceStub{
		alipayMobilePrecreateDeepLink: true,
		officialAlipayVisibleMethod:   true,
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/payment/checkout-info", nil)

	h.GetCheckoutInfo(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var resp struct {
		Data struct {
			AlipayMobilePrecreateDeepLink bool `json:"alipay_mobile_precreate_deep_link"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.True(t, resp.Data.AlipayMobilePrecreateDeepLink)
}

func ptrFloat(value float64) *float64 { return &value }
