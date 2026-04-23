//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type referralHandlerSettingRepoStub struct {
	values map[string]string
}

func (s *referralHandlerSettingRepoStub) Get(context.Context, string) (*service.Setting, error) {
	return nil, service.ErrSettingNotFound
}

func (s *referralHandlerSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	value, ok := s.values[key]
	if !ok {
		return "", service.ErrSettingNotFound
	}
	return value, nil
}

func (s *referralHandlerSettingRepoStub) Set(context.Context, string, string) error {
	return nil
}

func (s *referralHandlerSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		result[key] = s.values[key]
	}
	return result, nil
}

func (s *referralHandlerSettingRepoStub) SetMultiple(context.Context, map[string]string) error {
	return nil
}

func (s *referralHandlerSettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	return s.values, nil
}

func (s *referralHandlerSettingRepoStub) Delete(context.Context, string) error {
	return nil
}

type referralHandlerRelationRepoStub struct{}

func (s *referralHandlerRelationRepoStub) CountInvitees(context.Context, int64) (*service.ReferralInviteeCounts, error) {
	return &service.ReferralInviteeCounts{}, nil
}

func (s *referralHandlerRelationRepoStub) ListInvitees(context.Context, int64, pagination.PaginationParams) ([]service.ReferralInvitee, *pagination.PaginationResult, error) {
	return nil, &pagination.PaginationResult{}, nil
}

type referralHandlerCommissionRepoStub struct {
	userID       int64
	sourceUserID int64
	rewards      []service.UserInviteeReward
}

func (s *referralHandlerCommissionRepoStub) SumUserBucketAmount(context.Context, int64, string) (float64, error) {
	return 0, nil
}

func (s *referralHandlerCommissionRepoStub) ListLedgerEntriesByUser(context.Context, int64, pagination.PaginationParams) ([]service.CommissionLedger, *pagination.PaginationResult, error) {
	return nil, &pagination.PaginationResult{}, nil
}

func (s *referralHandlerCommissionRepoStub) ListWithdrawalsByUser(context.Context, int64, pagination.PaginationParams) ([]service.CommissionWithdrawal, *pagination.PaginationResult, error) {
	return nil, &pagination.PaginationResult{}, nil
}

func (s *referralHandlerCommissionRepoStub) ListPayoutAccountsByUser(context.Context, int64) ([]service.CommissionPayoutAccount, error) {
	return nil, nil
}

func (s *referralHandlerCommissionRepoStub) ListRewardsByUserAndSource(_ context.Context, userID int64, sourceUserID int64) ([]service.UserInviteeReward, error) {
	s.userID = userID
	s.sourceUserID = sourceUserID
	return s.rewards, nil
}

func TestReferralHandler_GetInviteeRewards_UsesRouteSourceUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	settingService := service.NewSettingService(&referralHandlerSettingRepoStub{values: map[string]string{
		service.SettingKeyReferralEnabled:            "true",
		service.SettingKeyReferralSettlementCurrency: service.ReferralSettlementCurrencyCNY,
	}}, nil)
	baseService := service.NewReferralService(nil, nil, nil, settingService)
	commissionRepo := &referralHandlerCommissionRepoStub{
		rewards: []service.UserInviteeReward{{ID: 9, RechargeOrderID: 11}},
	}
	centerService := service.NewReferralCenterService(baseService, &referralHandlerRelationRepoStub{}, commissionRepo, nil)
	h := NewReferralHandler(nil, centerService, nil)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/user/referral/invitees/123/rewards", nil)
	c.Params = gin.Params{{Key: "source_user_id", Value: "123"}}
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})

	h.GetInviteeRewards(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, int64(42), commissionRepo.userID)
	require.Equal(t, int64(123), commissionRepo.sourceUserID)

	var resp struct {
		Code int                         `json:"code"`
		Data []service.UserInviteeReward `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Len(t, resp.Data, 1)
	require.Equal(t, int64(9), resp.Data[0].ID)
}
