//go:build unit

package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type rechargeOrderHandlerSettingRepoStub struct {
	values map[string]string
}

func (s *rechargeOrderHandlerSettingRepoStub) Get(ctx context.Context, key string) (*service.Setting, error) {
	if value, ok := s.values[key]; ok {
		return &service.Setting{Key: key, Value: value}, nil
	}
	return nil, service.ErrSettingNotFound
}

func (s *rechargeOrderHandlerSettingRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	if value, ok := s.values[key]; ok {
		return value, nil
	}
	return "", service.ErrSettingNotFound
}

func (s *rechargeOrderHandlerSettingRepoStub) Set(ctx context.Context, key, value string) error {
	return nil
}
func (s *rechargeOrderHandlerSettingRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			result[key] = value
		}
	}
	return result, nil
}
func (s *rechargeOrderHandlerSettingRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	return nil
}
func (s *rechargeOrderHandlerSettingRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	result := make(map[string]string, len(s.values))
	for key, value := range s.values {
		result[key] = value
	}
	return result, nil
}
func (s *rechargeOrderHandlerSettingRepoStub) Delete(ctx context.Context, key string) error {
	return nil
}

type rechargeOrderHandlerUserRepoStub struct {
	balances map[int64]float64
}

func (s *rechargeOrderHandlerUserRepoStub) Create(ctx context.Context, user *service.User) error {
	panic("unexpected Create")
}
func (s *rechargeOrderHandlerUserRepoStub) GetByID(ctx context.Context, id int64) (*service.User, error) {
	return &service.User{ID: id, Balance: s.balances[id]}, nil
}
func (s *rechargeOrderHandlerUserRepoStub) GetByEmail(ctx context.Context, email string) (*service.User, error) {
	panic("unexpected GetByEmail")
}
func (s *rechargeOrderHandlerUserRepoStub) GetFirstAdmin(ctx context.Context) (*service.User, error) {
	panic("unexpected GetFirstAdmin")
}
func (s *rechargeOrderHandlerUserRepoStub) Update(ctx context.Context, user *service.User) error {
	panic("unexpected Update")
}
func (s *rechargeOrderHandlerUserRepoStub) Delete(ctx context.Context, id int64) error {
	panic("unexpected Delete")
}
func (s *rechargeOrderHandlerUserRepoStub) GetUserAvatar(context.Context, int64) (*service.UserAvatar, error) {
	panic("unexpected GetUserAvatar")
}
func (s *rechargeOrderHandlerUserRepoStub) UpsertUserAvatar(context.Context, int64, service.UpsertUserAvatarInput) (*service.UserAvatar, error) {
	panic("unexpected UpsertUserAvatar")
}
func (s *rechargeOrderHandlerUserRepoStub) DeleteUserAvatar(context.Context, int64) error {
	panic("unexpected DeleteUserAvatar")
}
func (s *rechargeOrderHandlerUserRepoStub) List(ctx context.Context, params pagination.PaginationParams) ([]service.User, *pagination.PaginationResult, error) {
	panic("unexpected List")
}
func (s *rechargeOrderHandlerUserRepoStub) ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters service.UserListFilters) ([]service.User, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters")
}
func (s *rechargeOrderHandlerUserRepoStub) UpdateBalance(ctx context.Context, id int64, amount float64) error {
	s.balances[id] += amount
	return nil
}
func (s *rechargeOrderHandlerUserRepoStub) DeductBalance(ctx context.Context, id int64, amount float64) error {
	panic("unexpected DeductBalance")
}
func (s *rechargeOrderHandlerUserRepoStub) UpdateConcurrency(ctx context.Context, id int64, amount int) error {
	panic("unexpected UpdateConcurrency")
}
func (s *rechargeOrderHandlerUserRepoStub) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	panic("unexpected ExistsByEmail")
}
func (s *rechargeOrderHandlerUserRepoStub) RemoveGroupFromAllowedGroups(ctx context.Context, groupID int64) (int64, error) {
	panic("unexpected RemoveGroupFromAllowedGroups")
}
func (s *rechargeOrderHandlerUserRepoStub) AddGroupToAllowedGroups(ctx context.Context, userID int64, groupID int64) error {
	panic("unexpected AddGroupToAllowedGroups")
}
func (s *rechargeOrderHandlerUserRepoStub) RemoveGroupFromUserAllowedGroups(ctx context.Context, userID int64, groupID int64) error {
	panic("unexpected RemoveGroupFromUserAllowedGroups")
}
func (s *rechargeOrderHandlerUserRepoStub) GetLatestUsedAtByUserIDs(context.Context, []int64) (map[int64]*time.Time, error) {
	panic("unexpected GetLatestUsedAtByUserIDs")
}
func (s *rechargeOrderHandlerUserRepoStub) GetLatestUsedAtByUserID(context.Context, int64) (*time.Time, error) {
	panic("unexpected GetLatestUsedAtByUserID")
}
func (s *rechargeOrderHandlerUserRepoStub) UpdateUserLastActiveAt(context.Context, int64, time.Time) error {
	panic("unexpected UpdateUserLastActiveAt")
}
func (s *rechargeOrderHandlerUserRepoStub) UpdateDefaultChatAPIKeyID(ctx context.Context, userID int64, apiKeyID *int64) error {
	panic("unexpected UpdateDefaultChatAPIKeyID")
}
func (s *rechargeOrderHandlerUserRepoStub) ListUserAuthIdentities(context.Context, int64) ([]service.UserAuthIdentityRecord, error) {
	panic("unexpected ListUserAuthIdentities")
}
func (s *rechargeOrderHandlerUserRepoStub) UnbindUserAuthProvider(context.Context, int64, string) error {
	panic("unexpected UnbindUserAuthProvider")
}
func (s *rechargeOrderHandlerUserRepoStub) UpdateTotpSecret(ctx context.Context, userID int64, encryptedSecret *string) error {
	panic("unexpected UpdateTotpSecret")
}
func (s *rechargeOrderHandlerUserRepoStub) EnableTotp(ctx context.Context, userID int64) error {
	panic("unexpected EnableTotp")
}
func (s *rechargeOrderHandlerUserRepoStub) DisableTotp(ctx context.Context, userID int64) error {
	panic("unexpected DisableTotp")
}

type rechargeOrderHandlerRechargeRepoStub struct {
	orders map[string]*service.RechargeOrder
	nextID int64
}

func (s *rechargeOrderHandlerRechargeRepoStub) GetByProviderAndExternalOrderID(ctx context.Context, provider, externalOrderID string) (*service.RechargeOrder, error) {
	if order, ok := s.orders[provider+"::"+externalOrderID]; ok {
		cloned := *order
		return &cloned, nil
	}
	return nil, service.ErrRechargeOrderNotFound
}

func (s *rechargeOrderHandlerRechargeRepoStub) GetByID(ctx context.Context, id int64) (*service.RechargeOrder, error) {
	for _, order := range s.orders {
		if order.ID == id {
			cloned := *order
			return &cloned, nil
		}
	}
	return nil, service.ErrRechargeOrderNotFound
}

func (s *rechargeOrderHandlerRechargeRepoStub) Create(ctx context.Context, order *service.RechargeOrder) error {
	if s.nextID == 0 {
		s.nextID = 1
	}
	order.ID = s.nextID
	s.nextID++
	cloned := *order
	s.orders[order.Provider+"::"+order.ExternalOrderID] = &cloned
	return nil
}

func (s *rechargeOrderHandlerRechargeRepoStub) Update(ctx context.Context, order *service.RechargeOrder) error {
	cloned := *order
	s.orders[order.Provider+"::"+order.ExternalOrderID] = &cloned
	return nil
}

func (s *rechargeOrderHandlerRechargeRepoStub) CountPaidOrdersByUser(ctx context.Context, userID int64) (int, error) {
	count := 0
	for _, order := range s.orders {
		if order.UserID == userID {
			count++
		}
	}
	return count, nil
}

func (s *rechargeOrderHandlerRechargeRepoStub) HasRefundOrChargeback(ctx context.Context, rechargeOrderID int64) (bool, error) {
	order, err := s.GetByID(ctx, rechargeOrderID)
	if err != nil {
		return false, err
	}
	return order.RefundedAmount > 0 || order.ChargebackAmount > 0, nil
}

type rechargeOrderHandlerCommissionRepoStub struct {
	rewards []service.CommissionReward
	ledgers []service.CommissionLedger
}

func (s *rechargeOrderHandlerCommissionRepoStub) CreateReward(ctx context.Context, reward *service.CommissionReward) error {
	reward.ID = int64(len(s.rewards) + 1)
	s.rewards = append(s.rewards, *reward)
	return nil
}

func (s *rechargeOrderHandlerCommissionRepoStub) ListRewardsByRechargeOrder(ctx context.Context, rechargeOrderID int64) ([]service.CommissionReward, error) {
	var result []service.CommissionReward
	for _, reward := range s.rewards {
		if reward.RechargeOrderID == rechargeOrderID {
			result = append(result, reward)
		}
	}
	return result, nil
}

func (s *rechargeOrderHandlerCommissionRepoStub) ListPendingRewardsReady(ctx context.Context, readyAt time.Time) ([]service.CommissionReward, error) {
	return nil, nil
}

func (s *rechargeOrderHandlerCommissionRepoStub) ListRewardsByUser(ctx context.Context, userID int64, statuses []string) ([]service.CommissionReward, error) {
	return nil, nil
}

func (s *rechargeOrderHandlerCommissionRepoStub) UpdateReward(ctx context.Context, reward *service.CommissionReward) error {
	return nil
}

func (s *rechargeOrderHandlerCommissionRepoStub) CreateLedgerEntries(ctx context.Context, entries []service.CommissionLedger) error {
	s.ledgers = append(s.ledgers, entries...)
	return nil
}

func (s *rechargeOrderHandlerCommissionRepoStub) SumRewardBucketAmount(ctx context.Context, rewardID int64, bucket string) (float64, error) {
	return 0, nil
}

func (s *rechargeOrderHandlerCommissionRepoStub) GetRewardByID(ctx context.Context, rewardID int64) (*service.CommissionReward, error) {
	for _, reward := range s.rewards {
		if reward.ID == rewardID {
			cloned := reward
			return &cloned, nil
		}
	}
	return nil, service.ErrCommissionWithdrawalNotFound
}

func (s *rechargeOrderHandlerCommissionRepoStub) SumRewardBucketAmountForUpdate(ctx context.Context, rewardID int64, bucket string, forUpdate bool) (float64, error) {
	return s.SumRewardBucketAmount(ctx, rewardID, bucket)
}

func (s *rechargeOrderHandlerCommissionRepoStub) CreateWithdrawal(ctx context.Context, withdrawal *service.CommissionWithdrawal) error {
	return nil
}

func (s *rechargeOrderHandlerCommissionRepoStub) GetWithdrawalByID(ctx context.Context, id int64) (*service.CommissionWithdrawal, error) {
	return nil, service.ErrCommissionWithdrawalNotFound
}

func (s *rechargeOrderHandlerCommissionRepoStub) UpdateWithdrawal(ctx context.Context, withdrawal *service.CommissionWithdrawal) error {
	return nil
}

func (s *rechargeOrderHandlerCommissionRepoStub) CreateWithdrawalItems(ctx context.Context, items []service.CommissionWithdrawalItem) error {
	return nil
}

func (s *rechargeOrderHandlerCommissionRepoStub) ListWithdrawalItemsByWithdrawal(ctx context.Context, withdrawalID int64) ([]service.CommissionWithdrawalItem, error) {
	return nil, nil
}

func (s *rechargeOrderHandlerCommissionRepoStub) UpdateWithdrawalItem(ctx context.Context, item *service.CommissionWithdrawalItem) error {
	return nil
}

func (s *rechargeOrderHandlerCommissionRepoStub) CountWithdrawalsByUserSince(ctx context.Context, userID int64, since time.Time) (int, error) {
	return 0, nil
}

func (s *rechargeOrderHandlerCommissionRepoStub) ListPayoutAccountsByUser(ctx context.Context, userID int64) ([]service.CommissionPayoutAccount, error) {
	return nil, nil
}

func (s *rechargeOrderHandlerCommissionRepoStub) UpsertPayoutAccount(ctx context.Context, account *service.CommissionPayoutAccount) error {
	return nil
}

func newRechargeOrderHandlerForTest() (*RechargeOrderHandler, *rechargeOrderHandlerUserRepoStub, *rechargeOrderHandlerRechargeRepoStub, *rechargeOrderHandlerCommissionRepoStub) {
	cfg := &config.Config{}
	settingSvc := service.NewSettingService(&rechargeOrderHandlerSettingRepoStub{values: map[string]string{
		service.SettingKeyReferralEnabled:    "true",
		service.SettingKeyReferralRewardMode: service.ReferralRewardModeEveryPaidOrder,
	}}, cfg)
	userRepo := &rechargeOrderHandlerUserRepoStub{balances: map[int64]float64{123: 0}}
	rechargeRepo := &rechargeOrderHandlerRechargeRepoStub{orders: make(map[string]*service.RechargeOrder)}
	commissionRepo := &rechargeOrderHandlerCommissionRepoStub{}
	settlementService := service.NewReferralSettlementService(commissionRepo, rechargeRepo, nil)
	rewardService := service.NewReferralRewardService(rechargeRepo, commissionRepo, userRepo, newReferralRepositoryStubForHandler(), nil, settingSvc, settlementService)
	return NewRechargeOrderHandler(rewardService), userRepo, rechargeRepo, commissionRepo
}

type referralRepositoryStubForHandler struct{}

func newReferralRepositoryStubForHandler() *referralRepositoryStubForHandler {
	return &referralRepositoryStubForHandler{}
}

func (s *referralRepositoryStubForHandler) GetDefaultCodeByUserID(ctx context.Context, userID int64) (*service.ReferralCode, error) {
	return nil, service.ErrReferralCodeNotFound
}
func (s *referralRepositoryStubForHandler) GetCodeByCode(ctx context.Context, code string) (*service.ReferralCode, error) {
	return nil, service.ErrReferralCodeNotFound
}
func (s *referralRepositoryStubForHandler) CreateCode(ctx context.Context, code *service.ReferralCode) error {
	return nil
}
func (s *referralRepositoryStubForHandler) GetRelationByUserID(ctx context.Context, userID int64) (*service.ReferralRelation, error) {
	return nil, service.ErrReferralRelationNotFound
}
func (s *referralRepositoryStubForHandler) CreateRelation(ctx context.Context, relation *service.ReferralRelation) error {
	return nil
}
func (s *referralRepositoryStubForHandler) CreateRelationHistory(ctx context.Context, history *service.ReferralRelationHistory) error {
	return nil
}
func (s *referralRepositoryStubForHandler) HasPaidRecharge(ctx context.Context, userID int64) (bool, error) {
	return false, nil
}

func fakeAdminAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 999})
		c.Set(string(middleware2.ContextKeyUserRole), "admin")
		c.Next()
	}
}

func TestRechargeOrderHandler_Credit_SuccessAndIdempotent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, userRepo, _, commissionRepo := newRechargeOrderHandlerForTest()

	router := gin.New()
	router.Use(fakeAdminAuth())
	router.POST("/credit", handler.Credit)

	body := `{"external_order_id":"order-1","provider":"sub2apipay","currency":"CNY","user_id":123,"paid_amount":100,"credited_balance_amount":100}`
	req := httptest.NewRequest(http.MethodPost, "/credit", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, float64(100), userRepo.balances[123])

	req2 := httptest.NewRequest(http.MethodPost, "/credit", strings.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusOK, w2.Code)
	require.Equal(t, float64(100), userRepo.balances[123])
	require.Len(t, commissionRepo.rewards, 0)

	var payload struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &payload))
	require.Equal(t, 0, payload.Code)
}

func TestRechargeOrderHandler_Credit_AllowsZeroCreditedBalance(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, userRepo, rechargeRepo, _ := newRechargeOrderHandlerForTest()

	router := gin.New()
	router.Use(fakeAdminAuth())
	router.POST("/credit", handler.Credit)

	body := `{"external_order_id":"sub-order-1","provider":"sub2apipay","currency":"CNY","user_id":123,"paid_amount":9.9,"credited_balance_amount":0}`
	req := httptest.NewRequest(http.MethodPost, "/credit", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, 0.0, userRepo.balances[123])
	order, err := rechargeRepo.GetByProviderAndExternalOrderID(context.Background(), "sub2apipay", "sub-order-1")
	require.NoError(t, err)
	require.Equal(t, 9.9, order.PaidAmount)
	require.Equal(t, 0.0, order.CreditedBalanceAmount)
}

func TestRechargeOrderHandler_Credit_RejectsNonCNY(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, _, _, _ := newRechargeOrderHandlerForTest()

	router := gin.New()
	router.Use(fakeAdminAuth())
	router.POST("/credit", handler.Credit)

	body := `{"external_order_id":"order-2","provider":"sub2apipay","currency":"USD","user_id":123,"paid_amount":100,"credited_balance_amount":100}`
	req := httptest.NewRequest(http.MethodPost, "/credit", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "RECHARGE_ORDER_CURRENCY_INVALID")
}
