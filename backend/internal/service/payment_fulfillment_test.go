//go:build unit

package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

type paymentFulfillmentTestProvider struct {
	key            string
	supportedTypes []payment.PaymentType
}

func (p paymentFulfillmentTestProvider) Name() string        { return p.key }
func (p paymentFulfillmentTestProvider) ProviderKey() string { return p.key }
func (p paymentFulfillmentTestProvider) SupportedTypes() []payment.PaymentType {
	return p.supportedTypes
}
func (p paymentFulfillmentTestProvider) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	panic("unexpected call")
}
func (p paymentFulfillmentTestProvider) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	panic("unexpected call")
}
func (p paymentFulfillmentTestProvider) VerifyNotification(ctx context.Context, rawBody string, headers map[string]string) (*payment.PaymentNotification, error) {
	panic("unexpected call")
}
func (p paymentFulfillmentTestProvider) Refund(ctx context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	panic("unexpected call")
}

// ---------------------------------------------------------------------------
// resolveRedeemAction — pure idempotency decision logic
// ---------------------------------------------------------------------------

func TestResolveRedeemAction_CodeNotFound(t *testing.T) {
	t.Parallel()
	action := resolveRedeemAction(nil, nil)
	assert.Equal(t, redeemActionCreate, action, "nil code with nil error should create")
}

func TestResolveRedeemAction_LookupError(t *testing.T) {
	t.Parallel()
	action := resolveRedeemAction(nil, errors.New("db connection lost"))
	assert.Equal(t, redeemActionCreate, action, "lookup error should fall back to create")
}

func TestResolveRedeemAction_LookupErrorWithNonNilCode(t *testing.T) {
	t.Parallel()
	// Edge case: both code and error are non-nil (shouldn't happen in practice,
	// but the function should still treat error as authoritative)
	code := &RedeemCode{Status: StatusUnused}
	action := resolveRedeemAction(code, errors.New("partial error"))
	assert.Equal(t, redeemActionCreate, action, "non-nil error should always result in create regardless of code")
}

func TestResolveRedeemAction_CodeExistsAndUsed(t *testing.T) {
	t.Parallel()
	code := &RedeemCode{
		Code:   "test-code-123",
		Status: StatusUsed,
		Type:   RedeemTypeBalance,
		Value:  10.0,
	}
	action := resolveRedeemAction(code, nil)
	assert.Equal(t, redeemActionSkipCompleted, action, "used code should skip to completed")
}

func TestResolveRedeemAction_CodeExistsAndUnused(t *testing.T) {
	t.Parallel()
	code := &RedeemCode{
		Code:   "test-code-456",
		Status: StatusUnused,
		Type:   RedeemTypeBalance,
		Value:  25.0,
	}
	action := resolveRedeemAction(code, nil)
	assert.Equal(t, redeemActionRedeem, action, "unused code should skip creation and proceed to redeem")
}

func TestResolveRedeemAction_CodeExistsWithExpiredStatus(t *testing.T) {
	t.Parallel()
	// A code with a non-standard status (neither "unused" nor "used")
	// should NOT be treated as used, so it falls through to redeemActionRedeem.
	code := &RedeemCode{
		Code:   "expired-code",
		Status: StatusExpired,
	}
	action := resolveRedeemAction(code, nil)
	assert.Equal(t, redeemActionRedeem, action, "expired-status code is not IsUsed(), should redeem")
}

// ---------------------------------------------------------------------------
// Table-driven comprehensive test
// ---------------------------------------------------------------------------

func TestResolveRedeemAction_Table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		code     *RedeemCode
		err      error
		expected redeemAction
	}{
		{
			name:     "nil code, nil error — first run",
			code:     nil,
			err:      nil,
			expected: redeemActionCreate,
		},
		{
			name:     "nil code, lookup error — treat as not found",
			code:     nil,
			err:      ErrRedeemCodeNotFound,
			expected: redeemActionCreate,
		},
		{
			name:     "nil code, generic DB error — treat as not found",
			code:     nil,
			err:      errors.New("connection refused"),
			expected: redeemActionCreate,
		},
		{
			name:     "code exists, used — previous run completed redeem",
			code:     &RedeemCode{Status: StatusUsed},
			err:      nil,
			expected: redeemActionSkipCompleted,
		},
		{
			name:     "code exists, unused — previous run created code but crashed before redeem",
			code:     &RedeemCode{Status: StatusUnused},
			err:      nil,
			expected: redeemActionRedeem,
		},
		{
			name:     "code exists but error also set — error takes precedence",
			code:     &RedeemCode{Status: StatusUsed},
			err:      errors.New("unexpected"),
			expected: redeemActionCreate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := resolveRedeemAction(tt.code, tt.err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// ---------------------------------------------------------------------------
// redeemAction enum value sanity
// ---------------------------------------------------------------------------

func TestRedeemAction_DistinctValues(t *testing.T) {
	t.Parallel()
	// Ensure the three actions have distinct values (iota correctness)
	assert.NotEqual(t, redeemActionCreate, redeemActionRedeem)
	assert.NotEqual(t, redeemActionCreate, redeemActionSkipCompleted)
	assert.NotEqual(t, redeemActionRedeem, redeemActionSkipCompleted)
}

// ---------------------------------------------------------------------------
// RedeemCode.IsUsed / CanUse interaction with resolveRedeemAction
// ---------------------------------------------------------------------------

func TestComputeExternalCreditedRefund_UsesCreditRatioForPartialGatewayRefund(t *testing.T) {
	t.Parallel()

	creditedDelta, refundTotal := computeExternalCreditedRefund(&dbent.PaymentOrder{
		Amount:       120,
		PayAmount:    100,
		RefundAmount: 0,
	}, 50, payment.NotificationAmountDelta)

	assert.Equal(t, 60.0, creditedDelta)
	assert.Equal(t, 60.0, refundTotal)
}

func TestComputeExternalCreditedRefund_CapsAtRemainingCreditedBalance(t *testing.T) {
	t.Parallel()

	creditedDelta, refundTotal := computeExternalCreditedRefund(&dbent.PaymentOrder{
		Amount:       120,
		PayAmount:    100,
		RefundAmount: 100,
	}, 50, payment.NotificationAmountDelta)

	assert.Equal(t, 20.0, creditedDelta)
	assert.Equal(t, 120.0, refundTotal)
}

func TestComputeExternalCreditedRefund_UsesTotalSemanticAsCumulativeGatewayAmount(t *testing.T) {
	t.Parallel()

	creditedDelta, refundTotal := computeExternalCreditedRefund(&dbent.PaymentOrder{
		Amount:       120,
		PayAmount:    100,
		RefundAmount: 24,
	}, 50, payment.NotificationAmountTotal)

	assert.Equal(t, 36.0, creditedDelta)
	assert.Equal(t, 60.0, refundTotal)
}

func TestAccumulateExternalReversalAmount_AddsIncrementallyAndCapsTotal(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 70.0, accumulateExternalReversalAmount(40, 30, 100))
	assert.Equal(t, 100.0, accumulateExternalReversalAmount(80, 50, 100))
	assert.Equal(t, 100.0, accumulateExternalReversalAmount(120, 10, 100))
}

func TestReconcileExternalReversalAmount_UsesTotalSemanticAsCumulativeAmount(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 70.0, reconcileExternalReversalAmount(40, 70, 100, payment.NotificationAmountTotal))
	assert.Equal(t, 100.0, reconcileExternalReversalAmount(40, 120, 100, payment.NotificationAmountTotal))
}

type paymentRefundUserRepoStub struct {
	*withdrawalUserRepoStub
	deductedBalances map[int64]float64
	deductErr        error
}

func newPaymentRefundUserRepoStub() *paymentRefundUserRepoStub {
	return &paymentRefundUserRepoStub{withdrawalUserRepoStub: &withdrawalUserRepoStub{}}
}

func (s *paymentRefundUserRepoStub) DeductBalance(ctx context.Context, id int64, amount float64) error {
	if s.deductErr != nil {
		return s.deductErr
	}
	if s.deductedBalances == nil {
		s.deductedBalances = map[int64]float64{}
	}
	s.deductedBalances[id] += amount
	return nil
}

func (s *paymentRefundUserRepoStub) CreateWithEmailAliasGuard(ctx context.Context, user *User) error {
	return s.Create(ctx, user)
}

func (s *paymentRefundUserRepoStub) ExistsByEmailAlias(context.Context, string) (bool, error) {
	panic("unexpected ExistsByEmailAlias")
}

func (s *paymentRefundUserRepoStub) BatchUpdateLimits(context.Context, []int64, *int, *int) (int, error) {
	panic("unexpected BatchUpdateLimits")
}

type failingRechargeOrderRepoStub struct {
	*rechargeOrderRepoStub
	err error
}

func (s *failingRechargeOrderRepoStub) GetByProviderAndExternalOrderID(ctx context.Context, provider, externalOrderID string) (*RechargeOrder, error) {
	return nil, s.err
}

func newPaymentServiceEntClient(t *testing.T) *dbent.Client {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() {
		require.NoError(t, client.Close())
		require.NoError(t, db.Close())
	})
	return client
}

func createPaymentRefundTestUser(t *testing.T, ctx context.Context, client *dbent.Client, balance float64) *dbent.User {
	t.Helper()

	user, err := client.User.Create().
		SetEmail(fmt.Sprintf("%s@example.com", strings.ToLower(strings.ReplaceAll(t.Name(), "/", "_")))).
		SetPasswordHash("hash").
		SetRole("user").
		SetUsername("refund-test").
		SetBalance(balance).
		Save(ctx)
	require.NoError(t, err)
	return user
}

func newPaymentEntitlementsSettingService(enabled bool) *SettingService {
	repo := newMockSettingRepo()
	_ = repo.Set(context.Background(), SettingKeySubscriptionEntitlementsV2Enabled, strconv.FormatBool(enabled))
	_ = repo.Set(context.Background(), SettingKeySub2PaymentPageLegacyMappingEnabled, "false")
	return NewSettingService(repo, &config.Config{})
}

func TestShouldUseSubscriptionEntitlementV2HonorsCheckoutSnapshot(t *testing.T) {
	planID := int64(101)
	service := &PaymentService{settingSvc: newPaymentEntitlementsSettingService(false)}

	require.True(t, service.shouldUseSubscriptionEntitlementV2(context.Background(), &dbent.PaymentOrder{
		PlanID:           &planID,
		ProviderSnapshot: map[string]any{paymentOrderSnapshotEntitlementV2Enabled: true},
	}))

	service.settingSvc = newPaymentEntitlementsSettingService(true)
	require.False(t, service.shouldUseSubscriptionEntitlementV2(context.Background(), &dbent.PaymentOrder{
		PlanID:           &planID,
		ProviderSnapshot: map[string]any{paymentOrderSnapshotEntitlementV2Enabled: false},
	}))

	require.True(t, service.shouldUseSubscriptionEntitlementV2(context.Background(), &dbent.PaymentOrder{PlanID: &planID}), "historical orders without a snapshot retain the legacy runtime fallback")
}

func TestResolveReferralSettlementUsesConfiguredUSDSubscriptionRate(t *testing.T) {
	svc := &PaymentService{configService: &PaymentConfigService{settingRepo: &paymentFulfillmentSettingRepoStub{values: map[string]string{
		SettingSubscriptionUSDToCNYRate: "7.2",
	}}}}
	order := &dbent.PaymentOrder{
		OrderType: payment.OrderTypeSubscription,
		ProviderSnapshot: map[string]any{
			"currency": "USD",
		},
	}

	settlement, skipReason, err := svc.resolveReferralSettlement(context.Background(), order)
	require.NoError(t, err)
	require.Empty(t, skipReason)
	require.Equal(t, "USD", settlement.sourceCurrency)
	require.InDelta(t, 7.2, settlement.rate, 0.000001)
	require.InDelta(t, 71.28, settlement.amount(9.9), 0.000001)
}

func TestResolveReferralSettlementPrefersOrderSnapshotRate(t *testing.T) {
	svc := &PaymentService{configService: &PaymentConfigService{settingRepo: &paymentFulfillmentSettingRepoStub{values: map[string]string{
		SettingSubscriptionUSDToCNYRate: "8.0",
	}}}}
	order := &dbent.PaymentOrder{
		OrderType: payment.OrderTypeSubscription,
		ProviderSnapshot: map[string]any{
			"currency": "USD",
			paymentOrderSnapshotSubscriptionUSDToCNYRate: 7.25,
		},
	}

	settlement, skipReason, err := svc.resolveReferralSettlement(context.Background(), order)
	require.NoError(t, err)
	require.Empty(t, skipReason)
	require.InDelta(t, 7.25, settlement.rate, 0.000001)
}

func TestSyncSubscriptionReferralRewardSettlesUSDPaymentInCNY(t *testing.T) {
	ctx := context.Background()
	rechargeRepo := newRechargeOrderRepoStub()
	rewardSvc := newReferralRewardServiceForTest(rechargeRepo, &commissionRepoStub{}, newRewardUserRepoStub(), newReferralRepoStub(), map[string]string{
		SettingKeyReferralEnabled: "false",
	})
	svc := &PaymentService{
		referralRewardSvc: rewardSvc,
		configService: &PaymentConfigService{settingRepo: &paymentFulfillmentSettingRepoStub{values: map[string]string{
			SettingSubscriptionUSDToCNYRate: "8.0",
		}}},
	}
	order := &dbent.PaymentOrder{
		ID:          71,
		UserID:      100,
		OutTradeNo:  "usd-subscription-referral",
		PaymentType: payment.TypeStripe,
		OrderType:   payment.OrderTypeSubscription,
		Amount:      10,
		PayAmount:   10,
		ProviderSnapshot: map[string]any{
			"currency": "USD",
			paymentOrderSnapshotSubscriptionUSDToCNYRate: 7.25,
		},
	}

	require.NoError(t, svc.syncReferralReward(ctx, order))
	rechargeOrder, err := rechargeRepo.GetByProviderAndExternalOrderID(ctx, payment.TypeStripe, order.OutTradeNo)
	require.NoError(t, err)
	require.Equal(t, ReferralSettlementCurrencyCNY, rechargeOrder.Currency)
	require.InDelta(t, 72.5, rechargeOrder.GrossAmount, 0.000001)
	require.InDelta(t, 72.5, rechargeOrder.PaidAmount, 0.000001)
	require.NotNil(t, rechargeOrder.MetadataJSON)
	require.Contains(t, *rechargeOrder.MetadataJSON, `"settlement_rate":7.25`)
}

func TestResolveReferralSettlementSkipsUnsupportedForeignCurrency(t *testing.T) {
	svc := &PaymentService{configService: &PaymentConfigService{settingRepo: &paymentFulfillmentSettingRepoStub{values: map[string]string{
		SettingSubscriptionUSDToCNYRate: "7.2",
	}}}}
	order := &dbent.PaymentOrder{
		OrderType: payment.OrderTypeSubscription,
		ProviderSnapshot: map[string]any{
			"currency": "EUR",
		},
	}

	settlement, skipReason, err := svc.resolveReferralSettlement(context.Background(), order)
	require.NoError(t, err)
	require.Empty(t, settlement.sourceCurrency)
	require.Contains(t, skipReason, "EUR")
}

func TestReferralSettlementReversalAmountUsesPersistedCNYRatio(t *testing.T) {
	order := &dbent.PaymentOrder{
		PayAmount: 10,
		ProviderSnapshot: map[string]any{
			"currency": "USD",
		},
	}
	rechargeOrder := &RechargeOrder{PaidAmount: 72.5}

	require.InDelta(t, 36.25, referralSettlementReversalAmount(order, rechargeOrder, 5), 0.000001)
}

func createPaymentOrderForRefundTest(t *testing.T, ctx context.Context, client *dbent.Client, user *dbent.User, amount float64, payAmount float64, refundAmount float64, status string, outTradeNo string, paymentTradeNo string) *dbent.PaymentOrder {
	t.Helper()

	paidAt := time.Now().Add(-time.Hour)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(amount).
		SetPayAmount(payAmount).
		SetRechargeCode("code-" + strings.ReplaceAll(outTradeNo, "_", "-")).
		SetOutTradeNo(outTradeNo).
		SetPaymentType(payment.TypeStripe).
		SetPaymentTradeNo(paymentTradeNo).
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(status).
		SetRefundAmount(refundAmount).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(paidAt).
		SetClientIP("127.0.0.1").
		SetSrcHost("example.com").
		Save(ctx)
	require.NoError(t, err)
	return order
}

type paymentQueryProviderStub struct {
	providerKey    string
	supportedTypes []payment.PaymentType
	queryTradeNos  []string
	queryResponse  *payment.QueryOrderResponse
}

func (s *paymentQueryProviderStub) Name() string { return "query-stub" }

func (s *paymentQueryProviderStub) ProviderKey() string { return s.providerKey }

func (s *paymentQueryProviderStub) SupportedTypes() []payment.PaymentType {
	return append([]payment.PaymentType(nil), s.supportedTypes...)
}

func (s *paymentQueryProviderStub) CreatePayment(context.Context, payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	return nil, errors.New("unexpected CreatePayment")
}

func (s *paymentQueryProviderStub) QueryOrder(_ context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	s.queryTradeNos = append(s.queryTradeNos, tradeNo)
	if s.queryResponse == nil {
		return nil, errors.New("missing query response")
	}
	resp := *s.queryResponse
	return &resp, nil
}

func (s *paymentQueryProviderStub) VerifyNotification(context.Context, string, map[string]string) (*payment.PaymentNotification, error) {
	return nil, errors.New("unexpected VerifyNotification")
}

func (s *paymentQueryProviderStub) Refund(context.Context, payment.RefundRequest) (*payment.RefundResponse, error) {
	return nil, errors.New("unexpected Refund")
}

type paymentGroupRepoStub struct {
	group *Group
}

func (s *paymentGroupRepoStub) Create(context.Context, *Group) error { panic("unexpected Create") }
func (s *paymentGroupRepoStub) GetByID(context.Context, int64) (*Group, error) {
	return s.group, nil
}
func (s *paymentGroupRepoStub) GetByIDLite(context.Context, int64) (*Group, error) {
	panic("unexpected GetByIDLite")
}
func (s *paymentGroupRepoStub) Update(context.Context, *Group) error { panic("unexpected Update") }
func (s *paymentGroupRepoStub) Delete(context.Context, int64) error  { panic("unexpected Delete") }
func (s *paymentGroupRepoStub) DeleteCascade(context.Context, int64) ([]int64, error) {
	panic("unexpected DeleteCascade")
}
func (s *paymentGroupRepoStub) List(context.Context, pagination.PaginationParams) ([]Group, *pagination.PaginationResult, error) {
	panic("unexpected List")
}
func (s *paymentGroupRepoStub) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string, *bool) ([]Group, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters")
}
func (s *paymentGroupRepoStub) ListActive(context.Context) ([]Group, error) {
	panic("unexpected ListActive")
}
func (s *paymentGroupRepoStub) ListActiveByPlatform(context.Context, string) ([]Group, error) {
	panic("unexpected ListActiveByPlatform")
}
func (s *paymentGroupRepoStub) ExistsByName(context.Context, string) (bool, error) {
	panic("unexpected ExistsByName")
}
func (s *paymentGroupRepoStub) GetAccountCount(context.Context, int64) (int64, int64, error) {
	panic("unexpected GetAccountCount")
}
func (s *paymentGroupRepoStub) DeleteAccountGroupsByGroupID(context.Context, int64) (int64, error) {
	panic("unexpected DeleteAccountGroupsByGroupID")
}
func (s *paymentGroupRepoStub) GetAccountIDsByGroupIDs(context.Context, []int64) ([]int64, error) {
	panic("unexpected GetAccountIDsByGroupIDs")
}
func (s *paymentGroupRepoStub) BindAccountsToGroup(context.Context, int64, []int64) error {
	panic("unexpected BindAccountsToGroup")
}
func (s *paymentGroupRepoStub) UpdateSortOrders(context.Context, []GroupSortOrderUpdate) error {
	panic("unexpected UpdateSortOrders")
}

type paymentLoadBalancerStub struct {
	configs           map[int64]map[string]string
	requestedConfig   []int64
	selectInstance    *payment.InstanceSelection
	selectInstanceErr error
}

func (s *paymentLoadBalancerStub) GetInstanceConfig(ctx context.Context, instanceID int64) (map[string]string, error) {
	s.requestedConfig = append(s.requestedConfig, instanceID)
	if cfg, ok := s.configs[instanceID]; ok {
		cloned := make(map[string]string, len(cfg))
		for k, v := range cfg {
			cloned[k] = v
		}
		return cloned, nil
	}
	return nil, fmt.Errorf("missing config for instance %d", instanceID)
}

func (s *paymentLoadBalancerStub) SelectInstance(ctx context.Context, providerKey string, paymentType payment.PaymentType, strategy payment.Strategy, orderAmount float64) (*payment.InstanceSelection, error) {
	if s.selectInstanceErr != nil {
		return nil, s.selectInstanceErr
	}
	if s.selectInstance == nil {
		return nil, fmt.Errorf("unexpected SelectInstance call")
	}
	result := *s.selectInstance
	if result.Config != nil {
		cloned := make(map[string]string, len(result.Config))
		for k, v := range result.Config {
			cloned[k] = v
		}
		result.Config = cloned
	}
	return &result, nil
}

func (s *subscriptionUserSubRepoStub) GetActiveByUserIDAndGroupID(_ context.Context, userID, groupID int64) (*UserSubscription, error) {
	return s.GetByUserIDAndGroupID(context.Background(), userID, groupID)
}

func (s *subscriptionUserSubRepoStub) ExtendExpiry(_ context.Context, subscriptionID int64, newExpiresAt time.Time) error {
	sub := s.byID[subscriptionID]
	if sub == nil {
		return ErrSubscriptionNotFound
	}
	sub.ExpiresAt = newExpiresAt
	return nil
}

func (s *subscriptionUserSubRepoStub) UpdateStatus(_ context.Context, subscriptionID int64, status string) error {
	sub := s.byID[subscriptionID]
	if sub == nil {
		return ErrSubscriptionNotFound
	}
	sub.Status = status
	return nil
}

func (s *subscriptionUserSubRepoStub) Delete(_ context.Context, subscriptionID int64) error {
	sub := s.byID[subscriptionID]
	if sub == nil {
		return ErrSubscriptionNotFound
	}
	s.deleteCalls++
	delete(s.byID, subscriptionID)
	delete(s.byUserGroup, s.key(sub.UserID, sub.GroupID))
	return nil
}

func requireAuditActionsForOrder(t *testing.T, ctx context.Context, client *dbent.Client, orderID int64, expectedActions ...string) []*dbent.PaymentAuditLog {
	t.Helper()

	logs, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(orderID, 10))).
		Where(paymentauditlog.ActionIn(expectedActions...)).
		Order(paymentauditlog.ByCreatedAt(), paymentauditlog.ByID()).
		All(ctx)
	require.NoError(t, err)
	require.Len(t, logs, len(expectedActions))
	for i, action := range expectedActions {
		require.Equal(t, action, logs[i].Action)
	}
	return logs
}

func TestExecuteSubscriptionFulfillment_SyncsReferralRewardWithoutCreditingBalance(t *testing.T) {
	ctx := context.Background()
	client := newPaymentServiceEntClient(t)
	user := createPaymentRefundTestUser(t, ctx, client, 0)

	paidAt := time.Now().Add(-time.Minute)
	groupID := int64(88)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(9.90).
		SetPayAmount(9.90).
		SetRechargeCode("sub-referral-no-balance").
		SetOutTradeNo("subscription_referral_order").
		SetPaymentType(payment.TypeAlipayDirect).
		SetPaymentTradeNo("trade-sub-referral").
		SetOrderType(payment.OrderTypeSubscription).
		SetStatus(OrderStatusPaid).
		SetSubscriptionGroupID(groupID).
		SetSubscriptionDays(1).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(paidAt).
		SetClientIP("127.0.0.1").
		SetSrcHost("example.com").
		Save(ctx)
	require.NoError(t, err)

	subRepo := newSubscriptionUserSubRepoStub()
	subscriptionSvc := NewSubscriptionService(
		&paymentGroupRepoStub{group: &Group{ID: groupID, Status: payment.EntityStatusActive, SubscriptionType: SubscriptionTypeSubscription}},
		subRepo,
		nil,
		nil,
		nil,
	)

	rechargeRepo := newRechargeOrderRepoStub()
	commissionRepo := &commissionRepoStub{}
	userRepo := newRewardUserRepoStub()
	userRepo.users[200] = &User{ID: 200, ReferralEnabled: true}
	referralRepo := newReferralRepoStub()
	referralRepo.relationsByUser[user.ID] = &ReferralRelation{
		UserID:         user.ID,
		ReferrerUserID: 200,
		BindSource:     ReferralBindSourceLink,
	}
	rewardSvc := newReferralRewardServiceForTest(rechargeRepo, commissionRepo, userRepo, referralRepo, map[string]string{
		SettingKeyReferralEnabled:             "false",
		SettingKeyReferralLevel1Enabled:       "true",
		SettingKeyReferralLevel1Rate:          "0.05",
		SettingKeyReferralRewardMode:          ReferralRewardModeEveryPaidOrder,
		SettingKeyReferralSettlementDelayDays: "7",
	})

	service := &PaymentService{
		entClient:         client,
		subscriptionSvc:   subscriptionSvc,
		groupRepo:         &paymentGroupRepoStub{group: &Group{ID: groupID, Status: payment.EntityStatusActive, SubscriptionType: SubscriptionTypeSubscription}},
		referralRewardSvc: rewardSvc,
	}

	err = service.ExecuteSubscriptionFulfillment(ctx, order.ID)
	require.NoError(t, err)

	updatedOrder, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusCompleted, updatedOrder.Status)
	require.Empty(t, userRepo.updateBalanceCalls, "subscription referral sync must not credit user balance")

	rechargeOrder, err := rechargeRepo.GetByProviderAndExternalOrderID(ctx, payment.TypeAlipay, order.OutTradeNo)
	require.NoError(t, err)
	require.Equal(t, user.ID, rechargeOrder.UserID)
	require.Equal(t, 9.90, rechargeOrder.PaidAmount)
	require.Equal(t, 0.0, rechargeOrder.CreditedBalanceAmount)
	require.Equal(t, RechargeOrderStatusCredited, rechargeOrder.Status)

	require.Len(t, commissionRepo.rewards, 1)
	require.Equal(t, int64(200), commissionRepo.rewards[0].UserID)
	require.Equal(t, user.ID, commissionRepo.rewards[0].SourceUserID)
	require.Equal(t, 0.05, commissionRepo.rewards[0].RateSnapshot)
	require.Equal(t, 0.495, commissionRepo.rewards[0].RewardAmount)
	require.Len(t, commissionRepo.ledgers, 1)
	require.Equal(t, CommissionLedgerEntryRewardPendingCredit, commissionRepo.ledgers[0].EntryType)
}

func TestExecuteSubscriptionFulfillment_CompletesWhenReferralRewardSyncFails(t *testing.T) {
	ctx := context.Background()
	client := newPaymentServiceEntClient(t)
	user := createPaymentRefundTestUser(t, ctx, client, 0)

	groupID := int64(89)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(9.90).
		SetPayAmount(9.90).
		SetRechargeCode("sub-referral-fails").
		SetOutTradeNo("subscription_referral_fails_order").
		SetPaymentType(payment.TypeAlipayDirect).
		SetPaymentTradeNo("trade-sub-referral-fails").
		SetOrderType(payment.OrderTypeSubscription).
		SetStatus(OrderStatusPaid).
		SetSubscriptionGroupID(groupID).
		SetSubscriptionDays(1).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now().Add(-time.Minute)).
		SetClientIP("127.0.0.1").
		SetSrcHost("example.com").
		Save(ctx)
	require.NoError(t, err)

	groupRepo := &paymentGroupRepoStub{group: &Group{ID: groupID, Status: payment.EntityStatusActive, SubscriptionType: SubscriptionTypeSubscription}}
	subRepo := newSubscriptionUserSubRepoStub()
	rewardSvc := newReferralRewardServiceForTest(
		&failingRechargeOrderRepoStub{rechargeOrderRepoStub: newRechargeOrderRepoStub(), err: errors.New("referral store unavailable")},
		&commissionRepoStub{},
		newRewardUserRepoStub(),
		newReferralRepoStub(),
		map[string]string{SettingKeyReferralEnabled: "true"},
	)
	service := &PaymentService{
		entClient:         client,
		subscriptionSvc:   NewSubscriptionService(groupRepo, subRepo, nil, nil, nil),
		groupRepo:         groupRepo,
		referralRewardSvc: rewardSvc,
	}

	err = service.ExecuteSubscriptionFulfillment(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, 1, subRepo.createCalls)

	updatedOrder, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusCompleted, updatedOrder.Status)
	requireAuditActionsForOrder(t, ctx, client, order.ID, "SUBSCRIPTION_ASSIGNED", "REFERRAL_REWARD_SYNC_FAILED", "SUBSCRIPTION_SUCCESS")
}

func TestExecuteSubscriptionFulfillment_RetryDoesNotExtendAlreadyAssignedSubscription(t *testing.T) {
	ctx := context.Background()
	client := newPaymentServiceEntClient(t)
	user := createPaymentRefundTestUser(t, ctx, client, 0)

	group, err := client.Group.Create().
		SetName("paid-subscription-idempotency").
		SetStatus(payment.EntityStatusActive).
		SetSubscriptionType(SubscriptionTypeSubscription).
		Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(9.90).
		SetPayAmount(9.90).
		SetRechargeCode("sub-already-assigned").
		SetOutTradeNo("subscription_already_assigned_order").
		SetPaymentType(payment.TypeAlipayDirect).
		SetPaymentTradeNo("trade-sub-already-assigned").
		SetOrderType(payment.OrderTypeSubscription).
		SetStatus(OrderStatusFailed).
		SetSubscriptionGroupID(group.ID).
		SetSubscriptionDays(30).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now().Add(-time.Minute)).
		SetClientIP("127.0.0.1").
		SetSrcHost("example.com").
		Save(ctx)
	require.NoError(t, err)

	expiresAt := time.Now().Add(10 * 24 * time.Hour).Truncate(time.Second)
	_, err = client.UserSubscription.Create().
		SetUserID(user.ID).
		SetGroupID(group.ID).
		SetStartsAt(time.Now().Add(-24 * time.Hour)).
		SetExpiresAt(expiresAt).
		SetStatus(SubscriptionStatusActive).
		SetNotes(fmt.Sprintf("payment_order_id=%d", order.ID)).
		Save(ctx)
	require.NoError(t, err)

	service := &PaymentService{
		entClient: client,
		groupRepo: &paymentGroupRepoStub{
			group: &Group{ID: group.ID, Status: payment.EntityStatusActive, SubscriptionType: SubscriptionTypeSubscription},
		},
	}

	err = service.ExecuteSubscriptionFulfillment(ctx, order.ID)
	require.NoError(t, err)

	updatedOrder, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusCompleted, updatedOrder.Status)

	sub, err := client.UserSubscription.Query().Only(ctx)
	require.NoError(t, err)
	require.WithinDuration(t, expiresAt, sub.ExpiresAt, time.Second)
}

func TestExecuteSubscriptionFulfillment_V2FlagDisabledKeepsLegacyPlanOrder(t *testing.T) {
	ctx := context.Background()
	client := newPaymentServiceEntClient(t)
	user := createPaymentRefundTestUser(t, ctx, client, 0)
	now := time.Now().Truncate(time.Second)
	primaryGroup := createPaymentConfigPlanTestGroup(t, client, "legacy-v2-off-primary", PlatformOpenAI, 0)
	secondGroup := createPaymentConfigPlanTestGroup(t, client, "legacy-v2-off-second", PlatformAnthropic, 1)
	plan, err := client.SubscriptionPlan.Create().
		SetGroupID(primaryGroup.ID).
		SetName("legacy-v2-off-multi-group-plan").
		SetDescription("paid before V2 rollout").
		SetPrice(9.90).
		SetValidityDays(30).
		SetValidityUnit("day").
		SetAccessScope(PlanAccessScopeExplicit).
		SetOveragePolicy(SubscriptionEntitlementOverageBlock).
		SetForSale(true).
		Save(ctx)
	require.NoError(t, err)
	for sortOrder, groupID := range []int64{primaryGroup.ID, secondGroup.ID} {
		_, err = client.SubscriptionPlanGroup.Create().
			SetPlanID(plan.ID).
			SetGroupID(groupID).
			SetSortOrder(sortOrder).
			SetEnabled(true).
			Save(ctx)
		require.NoError(t, err)
	}

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(9.90).
		SetPayAmount(9.90).
		SetRechargeCode("sub-legacy-flag-off").
		SetOutTradeNo("subscription_legacy_flag_off_order").
		SetPaymentType(payment.TypeAlipayDirect).
		SetPaymentTradeNo("trade-sub-legacy-flag-off").
		SetOrderType(payment.OrderTypeSubscription).
		SetStatus(OrderStatusPaid).
		SetPlanID(plan.ID).
		SetSubscriptionGroupID(primaryGroup.ID).
		SetSubscriptionDays(30).
		SetExpiresAt(now.Add(time.Hour)).
		SetPaidAt(now).
		SetClientIP("127.0.0.1").
		SetSrcHost("example.com").
		Save(ctx)
	require.NoError(t, err)

	groupRepo := &paymentGroupRepoStub{group: &Group{ID: primaryGroup.ID, Status: payment.EntityStatusActive, SubscriptionType: SubscriptionTypeSubscription}}
	subRepo := newSubscriptionUserSubRepoStub()
	entRepo := newFakeSubscriptionEntitlementRepo(now)
	entSvc := NewSubscriptionEntitlementService(entRepo, &fakeSubscriptionEntitlementPlanRepo{
		plans: map[int64]*SubscriptionEntitlementPlan{
			plan.ID: testEntitlementPlan(plan.ID, []int64{primaryGroup.ID, secondGroup.ID}, nil),
		},
	})
	entSvc.SetNowFunc(func() time.Time { return now })
	service := &PaymentService{
		entClient:                  client,
		groupRepo:                  groupRepo,
		subscriptionSvc:            NewSubscriptionService(groupRepo, subRepo, nil, nil, nil),
		subscriptionEntitlementSvc: entSvc,
		configService:              &PaymentConfigService{entClient: client},
		settingSvc:                 newPaymentEntitlementsSettingService(false),
	}

	err = service.ExecuteSubscriptionFulfillment(ctx, order.ID)
	require.NoError(t, err)

	updatedOrder, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusCompleted, updatedOrder.Status)
	require.Nil(t, updatedOrder.SubscriptionEntitlementID)
	require.Equal(t, 1, subRepo.createCalls)
	require.Equal(t, 0, entRepo.createCount)
	require.Equal(t, 0, entRepo.updateTermCount)
	require.Equal(t, 0, entRepo.eventCount)
	require.Empty(t, entRepo.fulfillments)
}

func TestExecuteSubscriptionFulfillment_V2AssignsEntitlementWithPlanGroups(t *testing.T) {
	ctx := context.Background()
	client := newPaymentServiceEntClient(t)
	user := createPaymentRefundTestUser(t, ctx, client, 0)
	now := time.Now().Truncate(time.Second)
	planID := int64(7101)
	groupIDs := []int64{8101, 8102}

	_, err := client.SubscriptionEntitlement.Create().
		SetUserID(user.ID).
		SetName("placeholder entitlement").
		SetSourceType("test").
		SetStatus(SubscriptionStatusActive).
		SetStartsAt(now).
		SetExpiresAt(now.AddDate(0, 0, 30)).
		SetOveragePolicy(SubscriptionEntitlementOverageBlock).
		SetPlanSnapshot(map[string]any{}).
		SetAssignedAt(now).
		Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(90).
		SetPayAmount(90).
		SetRechargeCode("sub-v2-create").
		SetOutTradeNo("subscription_v2_create_order").
		SetPaymentType(payment.TypeStripe).
		SetPaymentTradeNo("trade_subscription_v2_create").
		SetOrderType(payment.OrderTypeSubscription).
		SetStatus(OrderStatusPaid).
		SetPlanID(planID).
		SetSubscriptionGroupID(groupIDs[0]).
		SetSubscriptionDays(30).
		SetProviderSnapshot(map[string]any{
			"currency": "USD",
			paymentOrderSnapshotSubscriptionUSDToCNYRate: 7.2,
		}).
		SetExpiresAt(now.Add(time.Hour)).
		SetPaidAt(now).
		SetClientIP("127.0.0.1").
		SetSrcHost("example.com").
		Save(ctx)
	require.NoError(t, err)

	entRepo := newFakeSubscriptionEntitlementRepo(now)
	entSvc := NewSubscriptionEntitlementService(entRepo, &fakeSubscriptionEntitlementPlanRepo{
		plans: map[int64]*SubscriptionEntitlementPlan{
			planID: testEntitlementPlan(planID, groupIDs, nil),
		},
	})
	entSvc.SetNowFunc(func() time.Time { return now })
	service := &PaymentService{
		entClient:                  client,
		subscriptionEntitlementSvc: entSvc,
		settingSvc:                 newPaymentEntitlementsSettingService(true),
	}

	err = service.ExecuteSubscriptionFulfillment(ctx, order.ID)
	require.NoError(t, err)

	updatedOrder, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusCompleted, updatedOrder.Status)
	require.NotNil(t, updatedOrder.SubscriptionEntitlementID)
	require.Equal(t, int64(1), *updatedOrder.SubscriptionEntitlementID)
	require.Len(t, entRepo.createGroups, 1)
	require.Equal(t, groupIDs, entRepo.createGroups[0])
	entitlement, err := entRepo.GetByID(ctx, *updatedOrder.SubscriptionEntitlementID)
	require.NoError(t, err)
	require.Equal(t, "USD", entitlement.PlanSnapshot["purchase_currency"])
	require.Equal(t, 90.0, entitlement.PlanSnapshot["purchase_price"])
	require.Equal(t, 7.2, entitlement.PlanSnapshot["purchase_cny_per_usd_rate"])
	require.Len(t, entRepo.fulfillments, 1)
	for _, fulfillment := range entRepo.fulfillments {
		require.NotNil(t, fulfillment.SourceID)
		require.Equal(t, order.ID, *fulfillment.SourceID)
		require.NotNil(t, fulfillment.SourceExternalID)
		require.Equal(t, order.OutTradeNo, *fulfillment.SourceExternalID)
	}
}

func TestExecuteSubscriptionFulfillment_V2RenewalReusesEntitlementAndPreservesUsage(t *testing.T) {
	ctx := context.Background()
	client := newPaymentServiceEntClient(t)
	user := createPaymentRefundTestUser(t, ctx, client, 0)
	now := time.Now().Truncate(time.Second)
	planID := int64(7102)
	groupIDs := []int64{8201, 8202}

	_, err := client.SubscriptionEntitlement.Create().
		SetUserID(user.ID).
		SetName("placeholder entitlement").
		SetSourceType("test").
		SetStatus(SubscriptionStatusActive).
		SetStartsAt(now).
		SetExpiresAt(now.AddDate(0, 0, 30)).
		SetOveragePolicy(SubscriptionEntitlementOverageBlock).
		SetPlanSnapshot(map[string]any{}).
		SetAssignedAt(now).
		Save(ctx)
	require.NoError(t, err)

	entRepo := newFakeSubscriptionEntitlementRepo(now)
	entSvc := NewSubscriptionEntitlementService(entRepo, &fakeSubscriptionEntitlementPlanRepo{
		plans: map[int64]*SubscriptionEntitlementPlan{
			planID: testEntitlementPlan(planID, groupIDs, nil),
		},
	})
	entSvc.SetNowFunc(func() time.Time { return now })
	service := &PaymentService{
		entClient:                  client,
		subscriptionEntitlementSvc: entSvc,
		settingSvc:                 newPaymentEntitlementsSettingService(true),
	}

	firstOrder, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(90).
		SetPayAmount(90).
		SetRechargeCode("sub-v2-renew-first").
		SetOutTradeNo("subscription_v2_renew_first").
		SetPaymentType(payment.TypeStripe).
		SetPaymentTradeNo("trade_subscription_v2_renew_first").
		SetOrderType(payment.OrderTypeSubscription).
		SetStatus(OrderStatusPaid).
		SetPlanID(planID).
		SetSubscriptionGroupID(groupIDs[0]).
		SetSubscriptionDays(30).
		SetExpiresAt(now.Add(time.Hour)).
		SetPaidAt(now).
		SetClientIP("127.0.0.1").
		SetSrcHost("example.com").
		Save(ctx)
	require.NoError(t, err)
	require.NoError(t, service.ExecuteSubscriptionFulfillment(ctx, firstOrder.ID))

	entRepo.entitlements[1].MonthlyUsageUSD = 12.5
	firstExpiry := entRepo.entitlements[1].ExpiresAt
	secondOrder, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(90).
		SetPayAmount(90).
		SetRechargeCode("sub-v2-renew-second").
		SetOutTradeNo("subscription_v2_renew_second").
		SetPaymentType(payment.TypeStripe).
		SetPaymentTradeNo("trade_subscription_v2_renew_second").
		SetOrderType(payment.OrderTypeSubscription).
		SetStatus(OrderStatusPaid).
		SetPlanID(planID).
		SetSubscriptionGroupID(groupIDs[0]).
		SetSubscriptionDays(30).
		SetExpiresAt(now.Add(time.Hour)).
		SetPaidAt(now).
		SetClientIP("127.0.0.1").
		SetSrcHost("example.com").
		Save(ctx)
	require.NoError(t, err)
	require.NoError(t, service.ExecuteSubscriptionFulfillment(ctx, secondOrder.ID))

	updatedOrder, err := client.PaymentOrder.Get(ctx, secondOrder.ID)
	require.NoError(t, err)
	require.NotNil(t, updatedOrder.SubscriptionEntitlementID)
	require.Equal(t, int64(1), *updatedOrder.SubscriptionEntitlementID)
	ent := entRepo.entitlements[1]
	require.Equal(t, 12.5, ent.MonthlyUsageUSD)
	require.True(t, ent.ExpiresAt.After(firstExpiry))
	require.Len(t, entRepo.fulfillments, 2)
}

func TestRetryFailedSubscriptionReferralRewards_ReplaysCompletedSubscription(t *testing.T) {
	ctx := context.Background()
	client := newPaymentServiceEntClient(t)
	user := createPaymentRefundTestUser(t, ctx, client, 0)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(9.90).
		SetPayAmount(9.90).
		SetRechargeCode("sub-referral-replay").
		SetOutTradeNo("subscription_referral_replay_order").
		SetPaymentType(payment.TypeAlipayDirect).
		SetPaymentTradeNo("trade-sub-referral-replay").
		SetOrderType(payment.OrderTypeSubscription).
		SetStatus(OrderStatusCompleted).
		SetSubscriptionGroupID(90).
		SetSubscriptionDays(1).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now().Add(-time.Minute)).
		SetClientIP("127.0.0.1").
		SetSrcHost("example.com").
		Save(ctx)
	require.NoError(t, err)
	client.PaymentAuditLog.Create().
		SetOrderID(strconv.FormatInt(order.ID, 10)).
		SetAction("REFERRAL_REWARD_SYNC_FAILED").
		SetOperator("system").
		SetDetail(`{"detail":"referral store unavailable"}`).
		ExecX(ctx)

	rechargeRepo := newRechargeOrderRepoStub()
	commissionRepo := &commissionRepoStub{}
	userRepo := newRewardUserRepoStub()
	userRepo.users[200] = &User{ID: 200, ReferralEnabled: true}
	referralRepo := newReferralRepoStub()
	referralRepo.relationsByUser[user.ID] = &ReferralRelation{
		UserID:         user.ID,
		ReferrerUserID: 200,
		BindSource:     ReferralBindSourceLink,
	}
	rewardSvc := newReferralRewardServiceForTest(rechargeRepo, commissionRepo, userRepo, referralRepo, map[string]string{
		SettingKeyReferralEnabled:             "false",
		SettingKeyReferralLevel1Enabled:       "true",
		SettingKeyReferralLevel1Rate:          "0.05",
		SettingKeyReferralRewardMode:          ReferralRewardModeEveryPaidOrder,
		SettingKeyReferralSettlementDelayDays: "7",
	})

	service := &PaymentService{entClient: client, referralRewardSvc: rewardSvc}
	recovered, err := service.RetryFailedSubscriptionReferralRewards(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, 1, recovered)

	rechargeOrder, err := rechargeRepo.GetByProviderAndExternalOrderID(ctx, payment.TypeAlipay, order.OutTradeNo)
	require.NoError(t, err)
	require.Equal(t, 9.90, rechargeOrder.PaidAmount)
	require.Len(t, commissionRepo.rewards, 1)
	require.Equal(t, int64(200), commissionRepo.rewards[0].UserID)
	requireAuditActionsForOrder(t, ctx, client, order.ID, "REFERRAL_REWARD_SYNC_RECOVERED")
}

func TestCheckPaid_UsesProviderTradeNoWhenFallbackQuerySucceeds(t *testing.T) {
	ctx := context.Background()
	client := newPaymentServiceEntClient(t)
	user := createPaymentRefundTestUser(t, ctx, client, 0)

	groupRepo := &paymentGroupRepoStub{
		group: &Group{ID: 88, Status: payment.EntityStatusActive, SubscriptionType: SubscriptionTypeSubscription},
	}
	subRepo := newSubscriptionUserSubRepoStub()
	subscriptionSvc := NewSubscriptionService(groupRepo, subRepo, nil, nil, nil)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(99).
		SetPayAmount(99).
		SetRechargeCode("sub-check-paid").
		SetOutTradeNo("sub_check_paid_order").
		SetPaymentType(payment.TypeStripe).
		SetPaymentTradeNo("").
		SetOrderType(payment.OrderTypeSubscription).
		SetStatus(OrderStatusPending).
		SetSubscriptionGroupID(88).
		SetSubscriptionDays(30).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("example.com").
		Save(ctx)
	require.NoError(t, err)

	registry := payment.NewRegistry()
	providerStub := &paymentQueryProviderStub{
		providerKey:    payment.TypeStripe,
		supportedTypes: []payment.PaymentType{payment.TypeStripe},
		queryResponse: &payment.QueryOrderResponse{
			TradeNo: "pi_real_trade_no",
			Status:  payment.ProviderStatusPaid,
			Amount:  99,
		},
	}
	registry.Register(providerStub)

	service := &PaymentService{
		entClient:       client,
		registry:        registry,
		subscriptionSvc: subscriptionSvc,
		groupRepo:       groupRepo,
	}

	result := service.checkPaid(ctx, order)
	require.Equal(t, checkPaidResultAlreadyPaid, result)
	require.Equal(t, []string{"sub_check_paid_order"}, providerStub.queryTradeNos)

	updatedOrder, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusCompleted, updatedOrder.Status)
	require.Equal(t, "pi_real_trade_no", updatedOrder.PaymentTradeNo)
}

func TestHandlePaymentNotification_RefundedSubscriptionOrderRevokesAccess(t *testing.T) {
	ctx := context.Background()
	client := newPaymentServiceEntClient(t)
	user := createPaymentRefundTestUser(t, ctx, client, 0)

	groupID := int64(66)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(90).
		SetPayAmount(90).
		SetRechargeCode("sub-refund").
		SetOutTradeNo("subscription_refund_order").
		SetPaymentType(payment.TypeStripe).
		SetPaymentTradeNo("trade_subscription_refund").
		SetOrderType(payment.OrderTypeSubscription).
		SetStatus(OrderStatusCompleted).
		SetSubscriptionGroupID(groupID).
		SetSubscriptionDays(30).
		SetExpiresAt(time.Now().Add(24 * time.Hour)).
		SetPaidAt(time.Now().Add(-time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("example.com").
		Save(ctx)
	require.NoError(t, err)

	subRepo := newSubscriptionUserSubRepoStub()
	subRepo.seed(&UserSubscription{
		ID:        501,
		UserID:    user.ID,
		GroupID:   groupID,
		Status:    SubscriptionStatusActive,
		StartsAt:  time.Now().Add(-24 * time.Hour),
		ExpiresAt: time.Now().Add(29 * 24 * time.Hour),
	})
	subscriptionSvc := NewSubscriptionService(
		&paymentGroupRepoStub{group: &Group{ID: groupID, Status: payment.EntityStatusActive, SubscriptionType: SubscriptionTypeSubscription}},
		subRepo,
		nil,
		nil,
		nil,
	)

	service := &PaymentService{
		entClient:       client,
		subscriptionSvc: subscriptionSvc,
	}

	err = service.HandlePaymentNotification(ctx, &payment.PaymentNotification{
		OrderID:        order.OutTradeNo,
		TradeNo:        order.PaymentTradeNo,
		Amount:         90,
		AmountSemantic: payment.NotificationAmountTotal,
		Status:         payment.NotificationStatusRefunded,
	}, payment.TypeStripe)
	require.NoError(t, err)

	updatedOrder, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefunded, updatedOrder.Status)
	require.Equal(t, 90.0, updatedOrder.RefundAmount)

	_, err = subRepo.GetByID(ctx, 501)
	require.ErrorIs(t, err, ErrSubscriptionNotFound)

	logs := requireAuditActionsForOrder(t, ctx, client, order.ID, "EXTERNAL_REFUND_SYNCED")
	require.Contains(t, logs[0].Detail, `"tradeNo":"trade_subscription_refund"`)
}

func TestHandlePaymentNotification_RefundedSubscriptionOrderSyncsReferralState(t *testing.T) {
	ctx := context.Background()
	client := newPaymentServiceEntClient(t)
	user := createPaymentRefundTestUser(t, ctx, client, 0)

	groupID := int64(166)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(90).
		SetPayAmount(90).
		SetRechargeCode("sub-refund-referral").
		SetOutTradeNo("subscription_refund_referral_order").
		SetPaymentType(payment.TypeStripe).
		SetPaymentTradeNo("trade_subscription_refund_referral").
		SetOrderType(payment.OrderTypeSubscription).
		SetStatus(OrderStatusCompleted).
		SetSubscriptionGroupID(groupID).
		SetSubscriptionDays(30).
		SetExpiresAt(time.Now().Add(24 * time.Hour)).
		SetPaidAt(time.Now().Add(-time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("example.com").
		Save(ctx)
	require.NoError(t, err)

	subRepo := newSubscriptionUserSubRepoStub()
	subRepo.seed(&UserSubscription{
		ID:        601,
		UserID:    user.ID,
		GroupID:   groupID,
		Status:    SubscriptionStatusActive,
		StartsAt:  time.Now().Add(-24 * time.Hour),
		ExpiresAt: time.Now().Add(29 * 24 * time.Hour),
	})
	subscriptionSvc := NewSubscriptionService(
		&paymentGroupRepoStub{group: &Group{ID: groupID, Status: payment.EntityStatusActive, SubscriptionType: SubscriptionTypeSubscription}},
		subRepo,
		nil,
		nil,
		nil,
	)

	rechargeRepo := newRechargeOrderRepoStub()
	rechargeRepo.orders["stripe::subscription_refund_referral_order"] = &RechargeOrder{
		ID:              301,
		UserID:          user.ID,
		Provider:        payment.TypeStripe,
		ExternalOrderID: order.OutTradeNo,
		PaidAmount:      90,
		Status:          RechargeOrderStatusCredited,
		Currency:        ReferralSettlementCurrencyCNY,
	}
	rewardID := int64(401)
	commissionRepo := &commissionRepoStub{
		rewards: []CommissionReward{{
			ID:              rewardID,
			UserID:          200,
			SourceUserID:    user.ID,
			RechargeOrderID: 301,
			RewardAmount:    4.5,
			Currency:        ReferralSettlementCurrencyCNY,
			Status:          CommissionRewardStatusPending,
		}},
		ledgers: []CommissionLedger{{
			ID:              501,
			UserID:          200,
			RewardID:        &rewardID,
			RechargeOrderID: int64ValuePtr(301),
			EntryType:       CommissionLedgerEntryRewardPendingCredit,
			Bucket:          CommissionLedgerBucketPending,
			Amount:          4.5,
			Currency:        ReferralSettlementCurrencyCNY,
		}},
	}

	service := &PaymentService{
		entClient:         client,
		subscriptionSvc:   subscriptionSvc,
		referralRefundSvc: NewReferralRefundService(rechargeRepo, commissionRepo, nil, nil),
	}

	err = service.HandlePaymentNotification(ctx, &payment.PaymentNotification{
		OrderID:        order.OutTradeNo,
		TradeNo:        order.PaymentTradeNo,
		Amount:         90,
		AmountSemantic: payment.NotificationAmountTotal,
		Status:         payment.NotificationStatusRefunded,
	}, payment.TypeStripe)
	require.NoError(t, err)

	updatedRecharge, err := rechargeRepo.GetByProviderAndExternalOrderID(ctx, payment.TypeStripe, order.OutTradeNo)
	require.NoError(t, err)
	require.Equal(t, RechargeOrderStatusRefunded, updatedRecharge.Status)
	require.Equal(t, 90.0, updatedRecharge.RefundedAmount)
	require.Len(t, commissionRepo.ledgers, 2)
	require.Equal(t, CommissionLedgerEntryRefundReverse, commissionRepo.ledgers[1].EntryType)
	require.Equal(t, -4.5, commissionRepo.ledgers[1].Amount)
	require.Equal(t, CommissionRewardStatusReversed, commissionRepo.rewards[0].Status)
}

func TestMarkRefundOk_SubscriptionOrderSyncsReferralState(t *testing.T) {
	ctx := context.Background()
	client := newPaymentServiceEntClient(t)
	user := createPaymentRefundTestUser(t, ctx, client, 0)

	groupID := int64(167)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(90).
		SetPayAmount(90).
		SetRechargeCode("sub-admin-refund-referral").
		SetOutTradeNo("subscription_admin_refund_referral_order").
		SetPaymentType(payment.TypeStripe).
		SetPaymentTradeNo("trade_subscription_admin_refund_referral").
		SetOrderType(payment.OrderTypeSubscription).
		SetStatus(OrderStatusRefunding).
		SetSubscriptionGroupID(groupID).
		SetSubscriptionDays(30).
		SetExpiresAt(time.Now().Add(24 * time.Hour)).
		SetPaidAt(time.Now().Add(-time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("example.com").
		Save(ctx)
	require.NoError(t, err)

	rechargeRepo := newRechargeOrderRepoStub()
	rechargeRepo.orders["stripe::subscription_admin_refund_referral_order"] = &RechargeOrder{
		ID:              302,
		UserID:          user.ID,
		Provider:        payment.TypeStripe,
		ExternalOrderID: order.OutTradeNo,
		PaidAmount:      90,
		Status:          RechargeOrderStatusCredited,
		Currency:        ReferralSettlementCurrencyCNY,
	}
	rewardID := int64(402)
	commissionRepo := &commissionRepoStub{
		rewards: []CommissionReward{{
			ID:              rewardID,
			UserID:          200,
			SourceUserID:    user.ID,
			RechargeOrderID: 302,
			RewardAmount:    4.5,
			Currency:        ReferralSettlementCurrencyCNY,
			Status:          CommissionRewardStatusPending,
		}},
		ledgers: []CommissionLedger{{
			ID:              502,
			UserID:          200,
			RewardID:        &rewardID,
			RechargeOrderID: int64ValuePtr(302),
			EntryType:       CommissionLedgerEntryRewardPendingCredit,
			Bucket:          CommissionLedgerBucketPending,
			Amount:          4.5,
			Currency:        ReferralSettlementCurrencyCNY,
		}},
	}
	service := &PaymentService{
		entClient:         client,
		referralRefundSvc: NewReferralRefundService(rechargeRepo, commissionRepo, nil, nil),
	}

	result, err := service.markRefundOk(ctx, &RefundPlan{
		OrderID:       order.ID,
		Order:         order,
		RefundAmount:  90,
		GatewayAmount: 90,
		Reason:        "admin refund",
	})
	require.NoError(t, err)
	require.True(t, result.Success)

	updatedRecharge, err := rechargeRepo.GetByProviderAndExternalOrderID(ctx, payment.TypeStripe, order.OutTradeNo)
	require.NoError(t, err)
	require.Equal(t, RechargeOrderStatusRefunded, updatedRecharge.Status)
	require.Equal(t, 90.0, updatedRecharge.RefundedAmount)
	require.Len(t, commissionRepo.ledgers, 2)
	require.Equal(t, CommissionLedgerEntryRefundReverse, commissionRepo.ledgers[1].EntryType)
	require.Equal(t, -4.5, commissionRepo.ledgers[1].Amount)
	require.Equal(t, CommissionRewardStatusReversed, commissionRepo.rewards[0].Status)
}

func TestHandlePaymentNotification_PartialSubscriptionRefundDeductsProportionalDays(t *testing.T) {
	ctx := context.Background()
	client := newPaymentServiceEntClient(t)
	user := createPaymentRefundTestUser(t, ctx, client, 0)

	groupID := int64(67)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(90).
		SetPayAmount(90).
		SetRechargeCode("sub-partial-refund").
		SetOutTradeNo("subscription_partial_refund_notification_order").
		SetPaymentType(payment.TypeStripe).
		SetPaymentTradeNo("trade_subscription_partial_notification").
		SetOrderType(payment.OrderTypeSubscription).
		SetStatus(OrderStatusCompleted).
		SetSubscriptionGroupID(groupID).
		SetSubscriptionDays(30).
		SetExpiresAt(time.Now().Add(24 * time.Hour)).
		SetPaidAt(time.Now().Add(-time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("example.com").
		Save(ctx)
	require.NoError(t, err)

	subRepo := newSubscriptionUserSubRepoStub()
	originalExpiry := time.Now().Add(29 * 24 * time.Hour)
	subRepo.seed(&UserSubscription{
		ID:        502,
		UserID:    user.ID,
		GroupID:   groupID,
		Status:    SubscriptionStatusActive,
		StartsAt:  time.Now().Add(-24 * time.Hour),
		ExpiresAt: originalExpiry,
	})
	subscriptionSvc := NewSubscriptionService(
		&paymentGroupRepoStub{group: &Group{ID: groupID, Status: payment.EntityStatusActive, SubscriptionType: SubscriptionTypeSubscription}},
		subRepo,
		nil,
		nil,
		nil,
	)

	service := &PaymentService{
		entClient:       client,
		subscriptionSvc: subscriptionSvc,
	}

	err = service.HandlePaymentNotification(ctx, &payment.PaymentNotification{
		OrderID:        order.OutTradeNo,
		TradeNo:        order.PaymentTradeNo,
		Amount:         45,
		AmountSemantic: payment.NotificationAmountTotal,
		Status:         payment.NotificationStatusRefunded,
	}, payment.TypeStripe)
	require.NoError(t, err)

	updatedOrder, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusPartiallyRefunded, updatedOrder.Status)
	require.Equal(t, 45.0, updatedOrder.RefundAmount)
	require.NotNil(t, updatedOrder.RefundAt)

	updatedSub, err := subRepo.GetByID(ctx, 502)
	require.NoError(t, err)
	require.WithinDuration(t, originalExpiry.AddDate(0, 0, -15), updatedSub.ExpiresAt, time.Second)

	logs := requireAuditActionsForOrder(t, ctx, client, order.ID, "EXTERNAL_REFUND_SYNCED")
	require.Contains(t, logs[0].Detail, `"refundAmountTotal":45`)
}

func TestHandlePaymentNotification_V2SubscriptionRefundDeductsEntitlementDays(t *testing.T) {
	ctx := context.Background()
	client := newPaymentServiceEntClient(t)
	user := createPaymentRefundTestUser(t, ctx, client, 0)
	now := time.Now().Truncate(time.Second)
	placeholder, err := client.SubscriptionEntitlement.Create().
		SetUserID(user.ID).
		SetName("webhook refund entitlement").
		SetSourceType("test").
		SetStatus(SubscriptionStatusActive).
		SetStartsAt(now.Add(-24 * time.Hour)).
		SetExpiresAt(now.Add(30 * 24 * time.Hour)).
		SetOveragePolicy(SubscriptionEntitlementOverageBlock).
		SetPlanSnapshot(map[string]any{}).
		SetAssignedAt(now).
		Save(ctx)
	require.NoError(t, err)
	entitlementID := placeholder.ID

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(90).
		SetPayAmount(90).
		SetRechargeCode("sub-v2-refund-webhook").
		SetOutTradeNo("subscription_v2_refund_webhook_order").
		SetPaymentType(payment.TypeStripe).
		SetPaymentTradeNo("trade_subscription_v2_refund_webhook").
		SetOrderType(payment.OrderTypeSubscription).
		SetStatus(OrderStatusCompleted).
		SetPlanID(9901).
		SetSubscriptionEntitlementID(entitlementID).
		SetSubscriptionDays(30).
		SetExpiresAt(now.Add(time.Hour)).
		SetPaidAt(now.Add(-time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("example.com").
		Save(ctx)
	require.NoError(t, err)

	originalExpiry := now.Add(30 * 24 * time.Hour)
	entRepo := newFakeSubscriptionEntitlementRepo(now)
	entRepo.entitlements[entitlementID] = &SubscriptionEntitlement{
		ID:        entitlementID,
		UserID:    user.ID,
		PlanID:    int64ValuePtr(9901),
		Name:      "webhook refund entitlement",
		Status:    SubscriptionStatusActive,
		StartsAt:  now.Add(-24 * time.Hour),
		ExpiresAt: originalExpiry,
	}
	entSvc := NewSubscriptionEntitlementService(entRepo, &fakeSubscriptionEntitlementPlanRepo{})
	entSvc.SetNowFunc(func() time.Time { return now })

	service := &PaymentService{
		entClient:                  client,
		subscriptionEntitlementSvc: entSvc,
	}

	err = service.HandlePaymentNotification(ctx, &payment.PaymentNotification{
		OrderID:        order.OutTradeNo,
		TradeNo:        order.PaymentTradeNo,
		Amount:         45,
		AmountSemantic: payment.NotificationAmountTotal,
		Status:         payment.NotificationStatusRefunded,
	}, payment.TypeStripe)
	require.NoError(t, err)

	updatedOrder, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusPartiallyRefunded, updatedOrder.Status)
	require.Equal(t, 45.0, updatedOrder.RefundAmount)

	ent, err := entRepo.GetByID(ctx, entitlementID)
	require.NoError(t, err)
	require.WithinDuration(t, originalExpiry.AddDate(0, 0, -15), ent.ExpiresAt, time.Second)
	require.Equal(t, 1, entRepo.updateTermCount)
}

func TestGetWebhookProvider_RejectsAmbiguousProviderInstancesWithoutOrderHint(t *testing.T) {
	ctx := context.Background()
	client := newPaymentServiceEntClient(t)

	_, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeStripe).
		SetName("stripe-a").
		SetConfig("{}").
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeStripe).
		SetName("stripe-b").
		SetConfig("{}").
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	service := &PaymentService{
		entClient: client,
		registry:  payment.NewRegistry(),
	}

	_, err = service.GetWebhookProvider(ctx, payment.TypeStripe, "", "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "ambiguous")
}

func TestGetWebhookProvider_UsesInstanceHintBeforeVerification(t *testing.T) {
	ctx := context.Background()
	client := newPaymentServiceEntClient(t)

	first, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeStripe).
		SetName("stripe-a").
		SetConfig("{}").
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)
	second, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeStripe).
		SetName("stripe-b").
		SetConfig("{}").
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	loadBalancer := &paymentLoadBalancerStub{
		configs: map[int64]map[string]string{
			first.ID:  {"secretKey": "sk_test_first"},
			second.ID: {"secretKey": "sk_test_second"},
		},
	}
	service := &PaymentService{
		entClient:    client,
		registry:     payment.NewRegistry(),
		loadBalancer: loadBalancer,
	}

	provider, err := service.GetWebhookProvider(ctx, payment.TypeStripe, "", strconv.FormatInt(second.ID, 10))
	require.NoError(t, err)
	require.NotNil(t, provider)
	require.Equal(t, []int64{second.ID}, loadBalancer.requestedConfig)
}

func TestBuildNotifyURLWithInstanceHint_AppendsQueryParameter(t *testing.T) {
	got := buildNotifyURLWithInstanceHint("https://api.example.com/api/v1/payment/webhook/wxpay?foo=bar", "42")
	require.Equal(t, "https://api.example.com/api/v1/payment/webhook/wxpay?foo=bar&instance_id=42", got)
}

func TestHandlePaymentNotification_RefundedTotalSyncsOrderBalanceAndReferralState(t *testing.T) {
	ctx := context.Background()
	client := newPaymentServiceEntClient(t)
	user := createPaymentRefundTestUser(t, ctx, client, 0)
	order := createPaymentOrderForRefundTest(t, ctx, client, user, 120, 100, 24, OrderStatusCompleted, "refund_total_order", "trade_refund_total")

	userRepo := newPaymentRefundUserRepoStub()
	rechargeRepo := newRechargeOrderRepoStub()
	rechargeRepo.orders["stripe::refund_total_order"] = &RechargeOrder{
		ID:              101,
		UserID:          user.ID,
		Provider:        payment.TypeStripe,
		ExternalOrderID: order.OutTradeNo,
		PaidAmount:      100,
		RefundedAmount:  20,
		Status:          RechargeOrderStatusCredited,
		Currency:        ReferralSettlementCurrencyCNY,
	}
	service := &PaymentService{
		entClient:         client,
		userRepo:          userRepo,
		referralRefundSvc: NewReferralRefundService(rechargeRepo, &commissionRepoStub{}, nil, nil),
	}

	err := service.HandlePaymentNotification(ctx, &payment.PaymentNotification{
		OrderID:        order.OutTradeNo,
		TradeNo:        order.PaymentTradeNo,
		Amount:         50,
		AmountSemantic: payment.NotificationAmountTotal,
		Status:         payment.NotificationStatusRefunded,
	}, payment.TypeStripe)
	require.NoError(t, err)

	updatedOrder, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusPartiallyRefunded, updatedOrder.Status)
	require.Equal(t, 60.0, updatedOrder.RefundAmount)
	require.NotNil(t, updatedOrder.RefundAt)
	require.Equal(t, 36.0, userRepo.deductedBalances[user.ID])

	updatedRecharge, err := rechargeRepo.GetByProviderAndExternalOrderID(ctx, payment.TypeStripe, order.OutTradeNo)
	require.NoError(t, err)
	require.Equal(t, 50.0, updatedRecharge.RefundedAmount)
	require.Equal(t, 0.0, updatedRecharge.ChargebackAmount)
	require.Equal(t, RechargeOrderStatusPartiallyRefunded, updatedRecharge.Status)

	logs := requireAuditActionsForOrder(t, ctx, client, order.ID, "EXTERNAL_REFUND_SYNCED")
	require.Contains(t, logs[0].Detail, `"amountSemantic":"total"`)
}

func TestHandlePaymentNotification_RefundRequestedDoesNotTreatRequestedAmountAsSettled(t *testing.T) {
	ctx := context.Background()
	client := newPaymentServiceEntClient(t)
	user := createPaymentRefundTestUser(t, ctx, client, 0)
	order := createPaymentOrderForRefundTest(t, ctx, client, user, 100, 100, 100, OrderStatusRefundRequested, "refund_requested_baseline", "trade_refund_requested_baseline")
	userRepo := newPaymentRefundUserRepoStub()
	service := &PaymentService{entClient: client, userRepo: userRepo}

	err := service.HandlePaymentNotification(ctx, &payment.PaymentNotification{
		OrderID:        order.OutTradeNo,
		TradeNo:        order.PaymentTradeNo,
		Amount:         20,
		AmountSemantic: payment.NotificationAmountDelta,
		Status:         payment.NotificationStatusRefunded,
		RawData:        "refund-requested-provider-event",
	}, payment.TypeStripe)
	require.NoError(t, err)
	require.Equal(t, 20.0, userRepo.deductedBalances[user.ID])

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusPartiallyRefunded, reloaded.Status)
	require.Equal(t, 20.0, reloaded.RefundAmount)
}

func TestHandlePaymentNotification_DifferentRefundIDAfterSuccessfulRefundIsApplied(t *testing.T) {
	ctx := context.Background()
	client := newPaymentServiceEntClient(t)
	user := createPaymentRefundTestUser(t, ctx, client, 0)
	order := createPaymentOrderForRefundTest(t, ctx, client, user, 100, 100, 50, OrderStatusPartiallyRefunded, "refund_new_id_order", "trade_refund_new_id")
	_, err := client.PaymentOrder.UpdateOneID(order.ID).
		SetProviderSnapshot(map[string]any{
			paymentOrderSnapshotRefundInFlight: map[string]any{
				"refundAmount":                50,
				"providerRefundID":            "old-refund-id",
				"previousSettledRefundAmount": 0,
			},
		}).
		Save(ctx)
	require.NoError(t, err)

	userRepo := newPaymentRefundUserRepoStub()
	service := &PaymentService{entClient: client, userRepo: userRepo}
	err = service.HandlePaymentNotification(ctx, &payment.PaymentNotification{
		OrderID:        order.OutTradeNo,
		TradeNo:        order.PaymentTradeNo,
		RefundID:       "new-refund-id",
		Amount:         25,
		AmountSemantic: payment.NotificationAmountDelta,
		Status:         payment.NotificationStatusRefunded,
		RawData:        "new-refund-provider-event",
	}, payment.TypeStripe)
	require.NoError(t, err)
	require.Equal(t, 25.0, userRepo.deductedBalances[user.ID])

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusPartiallyRefunded, reloaded.Status)
	require.Equal(t, 75.0, reloaded.RefundAmount)
}

func TestHandlePaymentNotification_EventAuditRemainsIdempotentAfterSnapshotEviction(t *testing.T) {
	ctx := context.Background()
	client := newPaymentServiceEntClient(t)
	user := createPaymentRefundTestUser(t, ctx, client, 0)
	order := createPaymentOrderForRefundTest(t, ctx, client, user, 100, 100, 0, OrderStatusCompleted, "refund_event_audit_order", "trade_refund_event_audit")
	notification := &payment.PaymentNotification{
		OrderID:        order.OutTradeNo,
		TradeNo:        order.PaymentTradeNo,
		Amount:         25,
		AmountSemantic: payment.NotificationAmountDelta,
		Status:         payment.NotificationStatusRefunded,
		RawData:        "evicted-refund-provider-event",
	}
	fingerprint := refundNotificationFingerprint(notification, payment.TypeStripe, false)
	_, err := client.PaymentOrder.UpdateOneID(order.ID).
		SetProviderSnapshot(map[string]any{
			paymentOrderSnapshotRefundNotificationFingerprints: []string{
				"old-01", "old-02", "old-03", "old-04", "old-05", "old-06", "old-07", "old-08",
				"old-09", "old-10", "old-11", "old-12", "old-13", "old-14", "old-15", "old-16",
				"old-17", "old-18", "old-19", "old-20", "old-21", "old-22", "old-23", "old-24",
				"old-25", "old-26", "old-27", "old-28", "old-29", "old-30", "old-31", "old-32",
			},
		}).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.PaymentAuditLog.Create().
		SetOrderID(strconv.FormatInt(order.ID, 10)).
		SetAction(refundNotificationAuditAction(fingerprint, false)).
		SetOperator(payment.TypeStripe).
		SetDetail(`{"fingerprint":"` + fingerprint + `"}`).
		Save(ctx)
	require.NoError(t, err)

	userRepo := newPaymentRefundUserRepoStub()
	service := &PaymentService{entClient: client, userRepo: userRepo}
	require.NoError(t, service.HandlePaymentNotification(ctx, notification, payment.TypeStripe))
	require.Zero(t, userRepo.deductedBalances[user.ID])

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusCompleted, reloaded.Status)
	require.Equal(t, 0.0, reloaded.RefundAmount)
}

func TestHandlePaymentNotification_ChargebackDeltaSyncsOrderBalanceAndReferralState(t *testing.T) {
	ctx := context.Background()
	client := newPaymentServiceEntClient(t)
	user := createPaymentRefundTestUser(t, ctx, client, 0)
	order := createPaymentOrderForRefundTest(t, ctx, client, user, 100, 100, 30, OrderStatusCompleted, "chargeback_delta_order", "trade_chargeback_delta")

	userRepo := newPaymentRefundUserRepoStub()
	rechargeRepo := newRechargeOrderRepoStub()
	rechargeRepo.orders["stripe::chargeback_delta_order"] = &RechargeOrder{
		ID:               102,
		UserID:           user.ID,
		Provider:         payment.TypeStripe,
		ExternalOrderID:  order.OutTradeNo,
		PaidAmount:       100,
		RefundedAmount:   30,
		ChargebackAmount: 0,
		Status:           RechargeOrderStatusPartiallyRefunded,
		Currency:         ReferralSettlementCurrencyCNY,
	}
	service := &PaymentService{
		entClient:         client,
		userRepo:          userRepo,
		referralRefundSvc: NewReferralRefundService(rechargeRepo, &commissionRepoStub{}, nil, nil),
	}

	err := service.HandlePaymentNotification(ctx, &payment.PaymentNotification{
		OrderID:        order.OutTradeNo,
		TradeNo:        order.PaymentTradeNo,
		Amount:         20,
		AmountSemantic: payment.NotificationAmountDelta,
		Status:         payment.NotificationStatusChargeback,
	}, payment.TypeStripe)
	require.NoError(t, err)

	updatedOrder, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusPartiallyRefunded, updatedOrder.Status)
	require.Equal(t, 50.0, updatedOrder.RefundAmount)
	require.NotNil(t, updatedOrder.RefundAt)
	require.Equal(t, 20.0, userRepo.deductedBalances[user.ID])

	updatedRecharge, err := rechargeRepo.GetByProviderAndExternalOrderID(ctx, payment.TypeStripe, order.OutTradeNo)
	require.NoError(t, err)
	require.Equal(t, 30.0, updatedRecharge.RefundedAmount)
	require.Equal(t, 20.0, updatedRecharge.ChargebackAmount)
	require.Equal(t, RechargeOrderStatusChargeback, updatedRecharge.Status)

	logs := requireAuditActionsForOrder(t, ctx, client, order.ID, "EXTERNAL_CHARGEBACK_SYNCED")
	require.Contains(t, logs[0].Detail, `"amountSemantic":"delta"`)
}

func TestHandlePaymentNotification_RefundedFallsBackToTradeNoLookup(t *testing.T) {
	ctx := context.Background()
	client := newPaymentServiceEntClient(t)
	user := createPaymentRefundTestUser(t, ctx, client, 0)
	order := createPaymentOrderForRefundTest(t, ctx, client, user, 120, 100, 0, OrderStatusCompleted, "refund_trade_lookup_order", "trade_refund_lookup")

	userRepo := newPaymentRefundUserRepoStub()
	rechargeRepo := newRechargeOrderRepoStub()
	rechargeRepo.orders["stripe::refund_trade_lookup_order"] = &RechargeOrder{
		ID:              103,
		UserID:          user.ID,
		Provider:        payment.TypeStripe,
		ExternalOrderID: order.OutTradeNo,
		PaidAmount:      100,
		RefundedAmount:  0,
		Status:          RechargeOrderStatusCredited,
		Currency:        ReferralSettlementCurrencyCNY,
	}
	service := &PaymentService{
		entClient:         client,
		userRepo:          userRepo,
		referralRefundSvc: NewReferralRefundService(rechargeRepo, &commissionRepoStub{}, nil, nil),
	}

	err := service.HandlePaymentNotification(ctx, &payment.PaymentNotification{
		TradeNo:        order.PaymentTradeNo,
		Amount:         50,
		AmountSemantic: payment.NotificationAmountTotal,
		Status:         payment.NotificationStatusRefunded,
	}, payment.TypeStripe)
	require.NoError(t, err)

	updatedOrder, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusPartiallyRefunded, updatedOrder.Status)
	require.Equal(t, 60.0, updatedOrder.RefundAmount)
	require.Equal(t, 60.0, userRepo.deductedBalances[user.ID])

	updatedRecharge, err := rechargeRepo.GetByProviderAndExternalOrderID(ctx, payment.TypeStripe, order.OutTradeNo)
	require.NoError(t, err)
	require.Equal(t, 50.0, updatedRecharge.RefundedAmount)

	logs := requireAuditActionsForOrder(t, ctx, client, order.ID, "EXTERNAL_REFUND_SYNCED")
	require.Contains(t, logs[0].Detail, `"tradeNo":"trade_refund_lookup"`)
}

func TestHandlePaymentNotification_PendingRefundDeductionFollowsRollbackOutcome(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		deductBalance        bool
		deductionRollbackOK  bool
		expectedDeductionUSD float64
	}{
		{name: "rollback succeeded", deductBalance: true, deductionRollbackOK: true, expectedDeductionUSD: 50},
		{name: "rollback failed", deductBalance: true, deductionRollbackOK: false, expectedDeductionUSD: 0},
		{name: "deduction disabled", deductBalance: false, deductionRollbackOK: true, expectedDeductionUSD: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			client := newPaymentServiceEntClient(t)
			user := createPaymentRefundTestUser(t, ctx, client, 0)
			order := createPaymentOrderForRefundTest(
				t,
				ctx,
				client,
				user,
				100,
				100,
				50,
				OrderStatusRefundPending,
				"pending_refund_"+strings.ReplaceAll(tc.name, " ", "_"),
				"trade_pending_refund_"+strings.ReplaceAll(tc.name, " ", "_"),
			)
			_, err := client.PaymentOrder.UpdateOneID(order.ID).
				SetProviderSnapshot(map[string]any{
					paymentOrderSnapshotRefundPending: map[string]any{
						"refundID":                    "rf_pending",
						"refundAmount":                50,
						"gatewayAmount":               50,
						"deductBalance":               tc.deductBalance,
						"deductionType":               payment.DeductionTypeBalance,
						"deductionRollbackOK":         tc.deductionRollbackOK,
						"previousSettledRefundAmount": 0,
					},
				}).
				Save(ctx)
			require.NoError(t, err)

			userRepo := newPaymentRefundUserRepoStub()
			svc := &PaymentService{entClient: client, userRepo: userRepo}
			notification := &payment.PaymentNotification{
				OrderID:        order.OutTradeNo,
				TradeNo:        order.PaymentTradeNo,
				Amount:         50,
				AmountSemantic: payment.NotificationAmountDelta,
				Status:         payment.NotificationStatusRefunded,
				RawData:        "pending-refund-event-" + tc.name,
			}

			require.NoError(t, svc.HandlePaymentNotification(ctx, notification, payment.TypeAlipay))
			require.Equal(t, tc.expectedDeductionUSD, userRepo.deductedBalances[user.ID])

			reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
			require.NoError(t, err)
			require.Equal(t, OrderStatusPartiallyRefunded, reloaded.Status)
			require.Equal(t, 50.0, reloaded.RefundAmount)
		})
	}
}

func TestHandlePaymentNotification_DeltaRefundRetriesAreIdempotent(t *testing.T) {
	ctx := context.Background()
	client := newPaymentServiceEntClient(t)
	user := createPaymentRefundTestUser(t, ctx, client, 0)
	order := createPaymentOrderForRefundTest(t, ctx, client, user, 100, 100, 0, OrderStatusCompleted, "delta_retry_order", "trade_delta_retry")
	userRepo := newPaymentRefundUserRepoStub()
	svc := &PaymentService{entClient: client, userRepo: userRepo}

	first := &payment.PaymentNotification{
		OrderID:        order.OutTradeNo,
		TradeNo:        order.PaymentTradeNo,
		Amount:         50,
		AmountSemantic: payment.NotificationAmountDelta,
		Status:         payment.NotificationStatusRefunded,
		RawData:        "provider-refund-event-1",
	}
	require.NoError(t, svc.HandlePaymentNotification(ctx, first, payment.TypeAlipay))
	require.NoError(t, svc.HandlePaymentNotification(ctx, first, payment.TypeAlipay))
	require.Equal(t, 50.0, userRepo.deductedBalances[user.ID], "the same delta webhook retry must be applied once")

	second := *first
	second.RawData = "provider-refund-event-2"
	require.NoError(t, svc.HandlePaymentNotification(ctx, &second, payment.TypeAlipay))
	require.Equal(t, 100.0, userRepo.deductedBalances[user.ID], "a distinct equal-sized provider refund must still be applied")

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefunded, reloaded.Status)
	require.Equal(t, 100.0, reloaded.RefundAmount)
}

func TestMarkRefundOk_PersistsSuccessWhenReferralSyncFails(t *testing.T) {
	ctx := context.Background()
	client := newPaymentServiceEntClient(t)
	user := createPaymentRefundTestUser(t, ctx, client, 0)
	order := createPaymentOrderForRefundTest(t, ctx, client, user, 50, 50, 0, OrderStatusRefunding, "mark_refund_ok_order", "trade_mark_refund_ok")

	service := &PaymentService{
		entClient: client,
		referralRefundSvc: NewReferralRefundService(
			&failingRechargeOrderRepoStub{rechargeOrderRepoStub: newRechargeOrderRepoStub(), err: errors.New("referral lookup failed")},
			&commissionRepoStub{},
			nil,
			nil,
		),
	}

	result, err := service.markRefundOk(ctx, &RefundPlan{
		OrderID:       order.ID,
		Order:         order,
		RefundAmount:  50,
		GatewayAmount: 50,
		Reason:        "gateway success",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)
	require.Contains(t, result.Warning, "referral sync failed")

	updatedOrder, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefunded, updatedOrder.Status)
	require.Equal(t, 50.0, updatedOrder.RefundAmount)
	require.NotNil(t, updatedOrder.RefundAt)
	require.NotNil(t, updatedOrder.RefundReason)
	require.Equal(t, "gateway success", *updatedOrder.RefundReason)

	requireAuditActionsForOrder(t, ctx, client, order.ID, "REFUND_SUCCESS", "REFUND_REFERRAL_SYNC_FAILED")
}

func TestExpectedNotificationProviderKeyPrefersOrderInstanceProvider(t *testing.T) {
	t.Parallel()

	registry := payment.NewRegistry()
	registry.Register(paymentFulfillmentTestProvider{
		key:            payment.TypeAlipay,
		supportedTypes: []payment.PaymentType{payment.TypeAlipay},
	})

	assert.Equal(t,
		payment.TypeEasyPay,
		expectedNotificationProviderKey(registry, payment.TypeAlipay, "", payment.TypeEasyPay),
	)
}

func TestExpectedNotificationProviderKeyUsesRegistryMappingForLegacyOrders(t *testing.T) {
	t.Parallel()

	registry := payment.NewRegistry()
	registry.Register(paymentFulfillmentTestProvider{
		key:            payment.TypeEasyPay,
		supportedTypes: []payment.PaymentType{payment.TypeAlipay},
	})

	assert.Equal(t,
		payment.TypeEasyPay,
		expectedNotificationProviderKey(registry, payment.TypeAlipay, "", ""),
	)
}

func TestExpectedNotificationProviderKeyFallsBackToPaymentType(t *testing.T) {
	t.Parallel()

	assert.Equal(t,
		payment.TypeWxpay,
		expectedNotificationProviderKey(nil, payment.TypeWxpay, "", ""),
	)
}

func TestExpectedNotificationProviderKeyPrefersOrderSnapshotProviderKey(t *testing.T) {
	t.Parallel()

	registry := payment.NewRegistry()
	registry.Register(paymentFulfillmentTestProvider{
		key:            payment.TypeAlipay,
		supportedTypes: []payment.PaymentType{payment.TypeAlipay},
	})

	assert.Equal(t,
		payment.TypeEasyPay,
		expectedNotificationProviderKey(registry, payment.TypeAlipay, payment.TypeEasyPay, ""),
	)
}

func TestExpectedNotificationProviderKeyForOrderUsesSnapshotProviderKey(t *testing.T) {
	t.Parallel()

	registry := payment.NewRegistry()
	registry.Register(paymentFulfillmentTestProvider{
		key:            payment.TypeAlipay,
		supportedTypes: []payment.PaymentType{payment.TypeAlipay},
	})

	order := &dbent.PaymentOrder{
		PaymentType: payment.TypeAlipay,
		ProviderSnapshot: map[string]any{
			"schema_version": 1,
			"provider_key":   payment.TypeEasyPay,
		},
	}

	assert.Equal(t,
		payment.TypeEasyPay,
		expectedNotificationProviderKeyForOrder(registry, order, ""),
	)
}

func TestValidateProviderNotificationMetadataRejectsWxpaySnapshotMismatch(t *testing.T) {
	t.Parallel()

	order := &dbent.PaymentOrder{
		PaymentType: payment.TypeWxpay,
		ProviderSnapshot: map[string]any{
			"schema_version":  1,
			"merchant_app_id": "wx-app-expected",
			"merchant_id":     "mch-expected",
			"currency":        "CNY",
		},
	}

	err := validateProviderNotificationMetadata(order, payment.TypeWxpay, map[string]string{
		"appid":       "wx-app-other",
		"mchid":       "mch-expected",
		"currency":    "CNY",
		"trade_state": "SUCCESS",
	})
	assert.ErrorContains(t, err, "wxpay appid mismatch")
}

func TestValidateProviderNotificationMetadataAllowsLegacyOrdersWithoutSnapshotFields(t *testing.T) {
	t.Parallel()

	order := &dbent.PaymentOrder{
		PaymentType: payment.TypeWxpay,
		ProviderSnapshot: map[string]any{
			"schema_version":       1,
			"provider_instance_id": "9",
			"provider_key":         payment.TypeWxpay,
		},
	}

	err := validateProviderNotificationMetadata(order, payment.TypeWxpay, map[string]string{
		"appid":       "wx-app-runtime",
		"mchid":       "mch-runtime",
		"currency":    "CNY",
		"trade_state": "SUCCESS",
	})
	assert.NoError(t, err)
}

func TestParseLegacyPaymentOrderID(t *testing.T) {
	t.Parallel()

	oid, ok := parseLegacyPaymentOrderID("sub2_42", &dbent.NotFoundError{})
	assert.True(t, ok)
	assert.EqualValues(t, 42, oid)

	_, ok = parseLegacyPaymentOrderID("42", &dbent.NotFoundError{})
	assert.False(t, ok)

	_, ok = parseLegacyPaymentOrderID("sub2_42", errors.New("db down"))
	assert.False(t, ok)
}

func TestIsValidProviderAmount(t *testing.T) {
	t.Parallel()

	assert.True(t, isValidProviderAmount(0.01))
	assert.False(t, isValidProviderAmount(0))
	assert.False(t, isValidProviderAmount(-1))
	assert.False(t, isValidProviderAmount(math.NaN()))
	assert.False(t, isValidProviderAmount(math.Inf(1)))
}

func TestValidateProviderNotificationMetadataRejectsAlipaySnapshotMismatch(t *testing.T) {
	t.Parallel()

	order := &dbent.PaymentOrder{
		PaymentType: payment.TypeAlipay,
		ProviderSnapshot: map[string]any{
			"schema_version":  2,
			"merchant_app_id": "alipay-app-expected",
		},
	}

	err := validateProviderNotificationMetadata(order, payment.TypeAlipay, map[string]string{
		"app_id": "alipay-app-other",
	})
	assert.ErrorContains(t, err, "alipay app_id mismatch")
}

func TestValidateProviderNotificationMetadataRejectsEasyPaySnapshotMismatch(t *testing.T) {
	t.Parallel()

	order := &dbent.PaymentOrder{
		PaymentType: payment.TypeAlipay,
		ProviderSnapshot: map[string]any{
			"schema_version": 2,
			"merchant_id":    "pid-expected",
		},
	}

	err := validateProviderNotificationMetadata(order, payment.TypeEasyPay, map[string]string{
		"pid": "pid-other",
	})
	assert.ErrorContains(t, err, "easypay pid mismatch")
}

func TestValidateProviderNotificationMetadataRejectsAirwallexSnapshotMismatch(t *testing.T) {
	t.Parallel()

	order := &dbent.PaymentOrder{
		PaymentType: payment.TypeAirwallex,
		ProviderSnapshot: map[string]any{
			"schema_version": 2,
			"merchant_id":    "acct_expected",
			"currency":       "CNY",
		},
	}

	err := validateProviderNotificationMetadata(order, payment.TypeAirwallex, map[string]string{
		"account_id": "acct_other",
		"currency":   "CNY",
		"status":     "SUCCEEDED",
	})
	assert.ErrorContains(t, err, "airwallex account_id mismatch")

	err = validateProviderNotificationMetadata(order, payment.TypeAirwallex, map[string]string{
		"account_id": "acct_expected",
		"currency":   "USD",
		"status":     "SUCCEEDED",
	})
	assert.ErrorContains(t, err, "airwallex currency mismatch")
}

func TestValidateProviderNotificationMetadataRejectsStripeCurrencyMismatch(t *testing.T) {
	t.Parallel()

	order := &dbent.PaymentOrder{
		PaymentType: payment.TypeStripe,
		ProviderSnapshot: map[string]any{
			"schema_version": 2,
			"currency":       "HKD",
		},
	}

	err := validateProviderNotificationMetadata(order, payment.TypeStripe, map[string]string{
		"currency": "USD",
	})
	assert.ErrorContains(t, err, "stripe currency mismatch")
}

func TestPaymentAmountToleranceForThreeDecimalCurrency(t *testing.T) {
	t.Parallel()

	assert.Equal(t, amountToleranceCNY, paymentAmountToleranceForCurrency("CNY"))
	assert.Equal(t, amountToleranceCNY, paymentAmountToleranceForCurrency("JPY"))
	assert.InDelta(t, 0.0005, paymentAmountToleranceForCurrency("KWD"), 1e-12)
}

func ensurePaymentAuditOrderActionUniqueIndex(t *testing.T, ctx context.Context, client *dbent.Client) {
	t.Helper()
	_, err := client.ExecContext(ctx, `
		CREATE UNIQUE INDEX IF NOT EXISTS idx_payment_audit_logs_order_action_uniq
		ON payment_audit_logs (order_id, action)
	`)
	require.NoError(t, err)
}

func TestRetryFulfillmentRejectsFreshRechargingLease(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPaymentFulfillmentSubscriptionOrder(t, ctx, client, OrderStatusRecharging, time.Now())

	svc := &PaymentService{entClient: client}
	err := svc.RetryFulfillment(ctx, order.ID)
	require.Error(t, err)
	require.Equal(t, "CONFLICT", infraerrors.Reason(err))

	reloaded, getErr := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, getErr)
	require.Equal(t, OrderStatusRecharging, reloaded.Status)
}

func TestAlreadyProcessedRecoversStaleRechargingLease(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	ensurePaymentAuditOrderActionUniqueIndex(t, ctx, client)
	order := createPaymentFulfillmentSubscriptionOrder(
		t,
		ctx,
		client,
		OrderStatusRecharging,
		time.Now().Add(-paymentFulfillmentLeaseDuration-time.Minute),
	)
	_, err := client.PaymentAuditLog.Create().
		SetOrderID(strconv.FormatInt(order.ID, 10)).
		SetAction("SUBSCRIPTION_ASSIGNED").
		SetDetail(`{"groupID":7,"validityDays":30}`).
		SetOperator("system").
		Save(ctx)
	require.NoError(t, err)

	groupRepo := &subscriptionGroupRepoStub{
		group: &Group{ID: 7, Status: payment.EntityStatusActive, SubscriptionType: SubscriptionTypeSubscription},
	}
	svc := &PaymentService{
		entClient:       client,
		groupRepo:       groupRepo,
		subscriptionSvc: NewSubscriptionService(groupRepo, userSubRepoNoop{}, nil, nil, nil),
	}

	require.NoError(t, svc.alreadyProcessed(ctx, order))
	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusCompleted, reloaded.Status)
}

func TestFulfillmentLeaseVersionRejectsStaleWorker(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	staleAt := time.Now().Add(-paymentFulfillmentLeaseDuration - time.Minute)
	order := createPaymentFulfillmentSubscriptionOrder(t, ctx, client, OrderStatusRecharging, staleAt)
	svc := &PaymentService{entClient: client}

	firstLease, err := svc.acquirePaymentFulfillmentLease(ctx, order)
	require.NoError(t, err)
	require.NotNil(t, firstLease)

	_, err = client.PaymentOrder.UpdateOneID(order.ID).SetUpdatedAt(staleAt).Save(ctx)
	require.NoError(t, err)
	time.Sleep(time.Millisecond)
	staleOrder, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	secondLease, err := svc.acquirePaymentFulfillmentLease(ctx, staleOrder)
	require.NoError(t, err)
	require.NotNil(t, secondLease)
	require.False(t, firstLease.version.Equal(secondLease.version))

	err = svc.markCompleted(ctx, order, firstLease, "SUBSCRIPTION_SUCCESS")
	require.Error(t, err)
	require.Equal(t, "CONFLICT", infraerrors.Reason(err))
	svc.markFailed(ctx, order.ID, firstLease, errors.New("stale worker failure"))

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRecharging, reloaded.Status)
	require.NoError(t, svc.markCompleted(ctx, order, secondLease, "SUBSCRIPTION_SUCCESS"))
}

func TestExecuteBalanceFulfillmentRecoversAfterRedeemWithoutCreditingAgain(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	ensurePaymentAuditOrderActionUniqueIndex(t, ctx, client)
	staleAt := time.Now().Add(-paymentFulfillmentLeaseDuration - time.Minute)
	order := createPaymentFulfillmentSubscriptionOrder(t, ctx, client, OrderStatusRecharging, staleAt)
	order, err := client.PaymentOrder.UpdateOneID(order.ID).
		SetOrderType(payment.OrderTypeBalance).
		ClearPlanID().
		ClearSubscriptionGroupID().
		ClearSubscriptionDays().
		SetUpdatedAt(staleAt).
		Save(ctx)
	require.NoError(t, err)

	redeemRepo := &redeemCodeRepoStub{codesByCode: map[string]*RedeemCode{
		order.RechargeCode: {
			ID:     101,
			Code:   order.RechargeCode,
			Type:   RedeemTypeBalance,
			Value:  order.Amount,
			Status: StatusUsed,
		},
	}}
	svc := &PaymentService{
		entClient:     client,
		redeemService: &RedeemService{redeemRepo: redeemRepo},
	}

	require.NoError(t, svc.ExecuteBalanceFulfillment(ctx, order.ID))
	require.Empty(t, redeemRepo.useCalls, "an already-used order code must not be redeemed again")
	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusCompleted, reloaded.Status)
}

func TestDuplicatePaymentNotificationDoesNotReprocessCompletedBalanceOrder(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPaymentFulfillmentSubscriptionOrder(t, ctx, client, OrderStatusCompleted, time.Now())
	order, err := client.PaymentOrder.UpdateOneID(order.ID).
		SetOrderType(payment.OrderTypeBalance).
		ClearPlanID().
		ClearSubscriptionGroupID().
		ClearSubscriptionDays().
		Save(ctx)
	require.NoError(t, err)

	redeemRepo := &redeemCodeRepoStub{codesByCode: map[string]*RedeemCode{
		order.RechargeCode: {
			ID:     102,
			Code:   order.RechargeCode,
			Type:   RedeemTypeBalance,
			Value:  order.Amount,
			Status: StatusUnused,
		},
	}}
	svc := &PaymentService{
		entClient:     client,
		redeemService: &RedeemService{redeemRepo: redeemRepo},
	}
	notification := &payment.PaymentNotification{
		TradeNo: "alipay-trade-replayed",
		OrderID: order.OutTradeNo,
		Amount:  order.PayAmount,
		Status:  payment.NotificationStatusSuccess,
	}
	require.NoError(t, svc.HandlePaymentNotification(ctx, notification, payment.TypeAlipay))
	require.NoError(t, svc.HandlePaymentNotification(ctx, notification, payment.TypeAlipay))

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusCompleted, reloaded.Status)
	require.Empty(t, redeemRepo.useCalls, "a duplicate notification must not redeem the balance code again")
}

func TestPaymentNotificationRejectsAmountMismatchBeforeFulfillment(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPaymentFulfillmentSubscriptionOrder(t, ctx, client, OrderStatusPending, time.Now())
	order, err := client.PaymentOrder.UpdateOneID(order.ID).
		SetOrderType(payment.OrderTypeBalance).
		ClearPlanID().
		ClearSubscriptionGroupID().
		ClearSubscriptionDays().
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{entClient: client}
	err = svc.HandlePaymentNotification(ctx, &payment.PaymentNotification{
		TradeNo: "alipay-trade-wrong-amount",
		OrderID: order.OutTradeNo,
		Amount:  order.PayAmount - 1,
		Status:  payment.NotificationStatusSuccess,
	}, payment.TypeAlipay)
	require.ErrorContains(t, err, "amount mismatch")

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusPending, reloaded.Status)
}

func TestExecuteSubscriptionFulfillmentRecoversCommittedAssignmentWithoutExtendingAgain(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	ensurePaymentAuditOrderActionUniqueIndex(t, ctx, client)
	staleAt := time.Now().Add(-paymentFulfillmentLeaseDuration - time.Minute)
	order := createPaymentFulfillmentSubscriptionOrder(t, ctx, client, OrderStatusRecharging, staleAt)

	expiresAt := time.Now().Add(30 * 24 * time.Hour).Truncate(time.Second)
	subRepo := newSubscriptionUserSubRepoStub()
	subRepo.seed(&UserSubscription{
		ID:        99,
		UserID:    order.UserID,
		GroupID:   *order.SubscriptionGroupID,
		StartsAt:  time.Now().Add(-time.Hour),
		ExpiresAt: expiresAt,
		Status:    SubscriptionStatusActive,
		Notes:     "manual note\n" + legacySubscriptionAssignmentMarker(order.ID) + "\nretained note",
	})
	groupRepo := &subscriptionGroupRepoStub{
		group: &Group{ID: 7, Status: payment.EntityStatusActive, SubscriptionType: SubscriptionTypeSubscription},
	}
	svc := &PaymentService{
		entClient:       client,
		groupRepo:       groupRepo,
		subscriptionSvc: NewSubscriptionService(groupRepo, subRepo, nil, nil, nil),
	}

	require.NoError(t, svc.ExecuteSubscriptionFulfillment(ctx, order.ID))
	assertPaymentSubscriptionExpiry(t, subRepo, order, expiresAt)

	assignmentAuditCount, err := client.PaymentAuditLog.Query().
		Where(
			paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)),
			paymentauditlog.ActionEQ("SUBSCRIPTION_ASSIGNED"),
		).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, assignmentAuditCount)

	// Simulate another stale recovery attempt after completion. The durable audit
	// must make replay a no-op for the subscription entitlement.
	_, err = client.PaymentOrder.UpdateOneID(order.ID).
		SetStatus(OrderStatusRecharging).
		SetUpdatedAt(staleAt).
		ClearCompletedAt().
		Save(ctx)
	require.NoError(t, err)
	require.NoError(t, svc.ExecuteSubscriptionFulfillment(ctx, order.ID))
	assertPaymentSubscriptionExpiry(t, subRepo, order, expiresAt)

	assignmentAuditCount, err = client.PaymentAuditLog.Query().
		Where(
			paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)),
			paymentauditlog.ActionEQ("SUBSCRIPTION_ASSIGNED"),
		).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, assignmentAuditCount)
}

func TestSubscriptionNotesContainAssignmentMarkerRequiresIndependentExactLine(t *testing.T) {
	t.Parallel()
	require.True(t, subscriptionNotesContainAssignmentMarker("before\r\npayment order 42\r\nafter", 42))
	require.True(t, subscriptionNotesContainAssignmentMarker("before\npayment_order_id=42\nafter", 42))
	require.False(t, subscriptionNotesContainAssignmentMarker("payment order 420", 42))
	require.False(t, subscriptionNotesContainAssignmentMarker("prefix payment order 42 suffix", 42))
}

func createPaymentFulfillmentSubscriptionOrder(
	t *testing.T,
	ctx context.Context,
	client *dbent.Client,
	status string,
	updatedAt time.Time,
) *dbent.PaymentOrder {
	t.Helper()
	user, err := client.User.Create().
		SetEmail("fulfillment-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.com").
		SetPasswordHash("hash").
		SetUsername("payment-fulfillment-user").
		Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(80).
		SetPayAmount(80).
		SetFeeRate(0).
		SetRechargeCode("PAY-SUB-" + strconv.FormatInt(time.Now().UnixNano(), 10)).
		SetOutTradeNo("sub2_fulfillment_" + strconv.FormatInt(time.Now().UnixNano(), 10)).
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-fulfillment").
		SetOrderType(payment.OrderTypeSubscription).
		SetPlanID(100).
		SetSubscriptionGroupID(7).
		SetSubscriptionDays(30).
		SetStatus(status).
		SetPaidAt(time.Now().Add(-time.Hour)).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetUpdatedAt(updatedAt).
		Save(ctx)
	require.NoError(t, err)
	return order
}

func assertPaymentSubscriptionExpiry(t *testing.T, repo *subscriptionUserSubRepoStub, order *dbent.PaymentOrder, expected time.Time) {
	t.Helper()
	sub, err := repo.GetByUserIDAndGroupID(context.Background(), order.UserID, *order.SubscriptionGroupID)
	require.NoError(t, err)
	require.True(t, sub.ExpiresAt.Equal(expected), "subscription expiry changed from %s to %s", expected, sub.ExpiresAt)
}

func TestExecuteSubscriptionFulfillmentAppliesAffiliateRebate(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	ensurePaymentAuditOrderActionUniqueIndex(t, ctx, client)

	user, err := client.User.Create().
		SetEmail("subscription-affiliate@example.com").
		SetPasswordHash("hash").
		SetUsername("subscription-affiliate-user").
		Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(9.99).
		SetPayAmount(71.36).
		SetFeeRate(0).
		SetRechargeCode("PAY-SUB-AFFILIATE").
		SetOutTradeNo("sub2_subscription_affiliate").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-sub-affiliate").
		SetOrderType(payment.OrderTypeSubscription).
		SetPlanID(99).
		SetSubscriptionGroupID(7).
		SetSubscriptionDays(30).
		SetStatus(OrderStatusPaid).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
	require.NoError(t, err)

	inviterID := int64(9001)
	affiliateRepo := &paymentFulfillmentAffiliateRepoStub{
		inviteeSummary: &AffiliateSummary{
			UserID:    user.ID,
			AffCode:   "INVITEE",
			InviterID: &inviterID,
			CreatedAt: time.Now().Add(-24 * time.Hour),
		},
		inviterSummary: &AffiliateSummary{
			UserID:    inviterID,
			AffCode:   "INVITER",
			CreatedAt: time.Now().Add(-48 * time.Hour),
		},
	}
	settingSvc := NewSettingService(&paymentFulfillmentSettingRepoStub{values: map[string]string{
		SettingKeyAffiliateEnabled:           "true",
		SettingKeyAffiliateRebateRate:        "15",
		SettingKeyAffiliateRebateFreezeHours: "0",
	}}, nil)
	subRepo := newSubscriptionUserSubRepoStub()
	subscriptionSvc := NewSubscriptionService(&subscriptionGroupRepoStub{
		group: &Group{ID: 7, Status: payment.EntityStatusActive, SubscriptionType: SubscriptionTypeSubscription},
	}, subRepo, nil, nil, nil)
	svc := &PaymentService{
		entClient:        client,
		groupRepo:        &subscriptionGroupRepoStub{group: &Group{ID: 7, Status: payment.EntityStatusActive, SubscriptionType: SubscriptionTypeSubscription}},
		subscriptionSvc:  subscriptionSvc,
		affiliateService: NewAffiliateService(affiliateRepo, settingSvc, nil, nil),
	}

	err = svc.ExecuteSubscriptionFulfillment(ctx, order.ID)
	require.NoError(t, err)

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusCompleted, reloaded.Status)
	require.Len(t, affiliateRepo.accrueCalls, 1)
	require.Equal(t, inviterID, affiliateRepo.accrueCalls[0].inviterID)
	require.Equal(t, user.ID, affiliateRepo.accrueCalls[0].inviteeUserID)
	require.InDelta(t, 1.4985, affiliateRepo.accrueCalls[0].amount, 0.00000001)
	require.NotNil(t, affiliateRepo.accrueCalls[0].sourceOrderID)
	require.Equal(t, order.ID, *affiliateRepo.accrueCalls[0].sourceOrderID)
	require.Equal(t, 1, subRepo.createCalls)

	applied, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("AFFILIATE_REBATE_APPLIED")).
		Only(ctx)
	require.NoError(t, err)
	require.Contains(t, applied.Detail, `"baseAmount":9.99`)
	require.Contains(t, applied.Detail, `"rebateAmount":1.4985`)
}

func TestExecuteSubscriptionFulfillmentDoesNotDuplicateWorkAfterLegacySuccessAudit(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	ensurePaymentAuditOrderActionUniqueIndex(t, ctx, client)

	user, err := client.User.Create().
		SetEmail("subscription-affiliate-idempotent@example.com").
		SetPasswordHash("hash").
		SetUsername("subscription-affiliate-idempotent-user").
		Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(80).
		SetPayAmount(80).
		SetFeeRate(0).
		SetRechargeCode("PAY-SUB-AFFILIATE-IDEMPOTENT").
		SetOutTradeNo("sub2_subscription_affiliate_idempotent").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-sub-affiliate-idempotent").
		SetOrderType(payment.OrderTypeSubscription).
		SetPlanID(100).
		SetSubscriptionGroupID(7).
		SetSubscriptionDays(30).
		SetStatus(OrderStatusPaid).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
	require.NoError(t, err)
	_, err = client.PaymentAuditLog.Create().
		SetOrderID(strconv.FormatInt(order.ID, 10)).
		SetAction("SUBSCRIPTION_SUCCESS").
		SetDetail(`{"groupID":7,"validityDays":30}`).
		SetOperator("system").
		Save(ctx)
	require.NoError(t, err)
	_, err = client.PaymentAuditLog.Create().
		SetOrderID(strconv.FormatInt(order.ID, 10)).
		SetAction("AFFILIATE_REBATE_APPLIED").
		SetDetail(`{"baseAmount":80,"rebateAmount":16}`).
		SetOperator("system").
		Save(ctx)
	require.NoError(t, err)

	inviterID := int64(9001)
	affiliateRepo := &paymentFulfillmentAffiliateRepoStub{
		inviteeSummary: &AffiliateSummary{
			UserID:    user.ID,
			AffCode:   "INVITEE",
			InviterID: &inviterID,
			CreatedAt: time.Now().Add(-24 * time.Hour),
		},
		inviterSummary: &AffiliateSummary{
			UserID:    inviterID,
			AffCode:   "INVITER",
			CreatedAt: time.Now().Add(-48 * time.Hour),
		},
	}
	settingSvc := NewSettingService(&paymentFulfillmentSettingRepoStub{values: map[string]string{
		SettingKeyAffiliateEnabled:    "true",
		SettingKeyAffiliateRebateRate: "20",
	}}, nil)
	subRepo := newSubscriptionUserSubRepoStub()
	subscriptionSvc := NewSubscriptionService(&subscriptionGroupRepoStub{
		group: &Group{ID: 7, Status: payment.EntityStatusActive, SubscriptionType: SubscriptionTypeSubscription},
	}, subRepo, nil, nil, nil)
	svc := &PaymentService{
		entClient:        client,
		groupRepo:        &subscriptionGroupRepoStub{group: &Group{ID: 7, Status: payment.EntityStatusActive, SubscriptionType: SubscriptionTypeSubscription}},
		subscriptionSvc:  subscriptionSvc,
		affiliateService: NewAffiliateService(affiliateRepo, settingSvc, nil, nil),
	}

	err = svc.ExecuteSubscriptionFulfillment(ctx, order.ID)
	require.NoError(t, err)

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusCompleted, reloaded.Status)
	require.Empty(t, affiliateRepo.accrueCalls)
	require.Zero(t, subRepo.createCalls)
}

type paymentFulfillmentAffiliateAccrueCall struct {
	inviterID     int64
	inviteeUserID int64
	amount        float64
	freezeHours   int
	sourceOrderID *int64
}

type paymentFulfillmentAffiliateRepoStub struct {
	inviteeSummary *AffiliateSummary
	inviterSummary *AffiliateSummary
	accrued        float64
	accrueCalls    []paymentFulfillmentAffiliateAccrueCall
}

func (s *paymentFulfillmentAffiliateRepoStub) EnsureUserAffiliate(ctx context.Context, userID int64) (*AffiliateSummary, error) {
	if s.inviteeSummary != nil && s.inviteeSummary.UserID == userID {
		return s.inviteeSummary, nil
	}
	if s.inviterSummary != nil && s.inviterSummary.UserID == userID {
		return s.inviterSummary, nil
	}
	return &AffiliateSummary{UserID: userID, CreatedAt: time.Now()}, nil
}

func (s *paymentFulfillmentAffiliateRepoStub) GetAffiliateByCode(ctx context.Context, code string) (*AffiliateSummary, error) {
	return nil, nil
}

func (s *paymentFulfillmentAffiliateRepoStub) BindInviter(ctx context.Context, userID, inviterID int64) (bool, error) {
	return false, nil
}

func (s *paymentFulfillmentAffiliateRepoStub) AccrueQuota(ctx context.Context, inviterID, inviteeUserID int64, amount float64, freezeHours int, sourceOrderID *int64) (bool, error) {
	s.accrueCalls = append(s.accrueCalls, paymentFulfillmentAffiliateAccrueCall{
		inviterID:     inviterID,
		inviteeUserID: inviteeUserID,
		amount:        amount,
		freezeHours:   freezeHours,
		sourceOrderID: sourceOrderID,
	})
	return true, nil
}

func (s *paymentFulfillmentAffiliateRepoStub) GetAccruedRebateFromInvitee(ctx context.Context, inviterID, inviteeUserID int64) (float64, error) {
	return s.accrued, nil
}

func (s *paymentFulfillmentAffiliateRepoStub) ThawFrozenQuota(ctx context.Context, userID int64) (float64, error) {
	return 0, nil
}

func (s *paymentFulfillmentAffiliateRepoStub) TransferQuotaToBalance(ctx context.Context, userID int64) (float64, float64, error) {
	return 0, 0, nil
}

func (s *paymentFulfillmentAffiliateRepoStub) ListInvitees(ctx context.Context, inviterID int64, limit int) ([]AffiliateInvitee, error) {
	return nil, nil
}

func (s *paymentFulfillmentAffiliateRepoStub) UpdateUserAffCode(ctx context.Context, userID int64, newCode string) error {
	return nil
}

func (s *paymentFulfillmentAffiliateRepoStub) ResetUserAffCode(ctx context.Context, userID int64) (string, error) {
	return "", nil
}

func (s *paymentFulfillmentAffiliateRepoStub) SetUserRebateRate(ctx context.Context, userID int64, ratePercent *float64) error {
	return nil
}

func (s *paymentFulfillmentAffiliateRepoStub) BatchSetUserRebateRate(ctx context.Context, userIDs []int64, ratePercent *float64) error {
	return nil
}

func (s *paymentFulfillmentAffiliateRepoStub) ListUsersWithCustomSettings(ctx context.Context, filter AffiliateAdminFilter) ([]AffiliateAdminEntry, int64, error) {
	return nil, 0, nil
}

func (s *paymentFulfillmentAffiliateRepoStub) ListAffiliateInviteRecords(ctx context.Context, filter AffiliateRecordFilter) ([]AffiliateInviteRecord, int64, error) {
	return nil, 0, nil
}

func (s *paymentFulfillmentAffiliateRepoStub) ListAffiliateRebateRecords(ctx context.Context, filter AffiliateRecordFilter) ([]AffiliateRebateRecord, int64, error) {
	return nil, 0, nil
}

func (s *paymentFulfillmentAffiliateRepoStub) ListAffiliateTransferRecords(ctx context.Context, filter AffiliateRecordFilter) ([]AffiliateTransferRecord, int64, error) {
	return nil, 0, nil
}

func (s *paymentFulfillmentAffiliateRepoStub) GetAffiliateUserOverview(ctx context.Context, userID int64) (*AffiliateUserOverview, error) {
	return nil, nil
}

type paymentFulfillmentSettingRepoStub struct {
	values map[string]string
}

func (s *paymentFulfillmentSettingRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	value, ok := s.values[key]
	if !ok {
		return nil, errors.New("setting not found")
	}
	return &Setting{Key: key, Value: value}, nil
}

func (s *paymentFulfillmentSettingRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	value, ok := s.values[key]
	if !ok {
		return "", errors.New("setting not found")
	}
	return value, nil
}

func (s *paymentFulfillmentSettingRepoStub) Set(ctx context.Context, key, value string) error {
	s.values[key] = value
	return nil
}

func (s *paymentFulfillmentSettingRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (s *paymentFulfillmentSettingRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	if s.values == nil {
		s.values = make(map[string]string, len(settings))
	}
	for key, value := range settings {
		s.values[key] = value
	}
	return nil
}

func (s *paymentFulfillmentSettingRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	out := make(map[string]string, len(s.values))
	for key, value := range s.values {
		out[key] = value
	}
	return out, nil
}

func (s *paymentFulfillmentSettingRepoStub) Delete(ctx context.Context, key string) error {
	delete(s.values, key)
	return nil
}

var _ AffiliateRepository = (*paymentFulfillmentAffiliateRepoStub)(nil)
var _ SettingRepository = (*paymentFulfillmentSettingRepoStub)(nil)
