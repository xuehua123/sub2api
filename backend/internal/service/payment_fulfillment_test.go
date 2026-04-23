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
	"github.com/Wei-Shaw/sub2api/internal/payment"
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
	delete(s.byID, subscriptionID)
	delete(s.byUserGroup, s.key(sub.UserID, sub.GroupID))
	return nil
}

func requireAuditActionsForOrder(t *testing.T, ctx context.Context, client *dbent.Client, orderID int64, expectedActions ...string) []*dbent.PaymentAuditLog {
	t.Helper()

	logs, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(orderID, 10))).
		Order(paymentauditlog.ByCreatedAt()).
		All(ctx)
	require.NoError(t, err)
	require.Len(t, logs, len(expectedActions))
	for i, action := range expectedActions {
		require.Equal(t, action, logs[i].Action)
	}
	return logs
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
