//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/ent/setting"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/dgraph-io/ristretto"
	"github.com/stretchr/testify/require"
)

func TestInvalidateFinalizedRefundSubscriptionCaches(t *testing.T) {
	cache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 100,
		MaxCost:     100,
		BufferItems: 64,
	})
	require.NoError(t, err)
	t.Cleanup(cache.Close)

	const (
		userID  = int64(41)
		groupID = int64(73)
	)
	subscription := &UserSubscription{
		ID:        19,
		UserID:    userID,
		GroupID:   groupID,
		UpdatedAt: time.Now(),
	}
	require.True(t, cache.Set(subCacheKey(userID, groupID), subscription, 1))
	require.True(t, cache.Set(activeSubscriptionsCacheKey(userID), []*UserSubscription{subscription}, 1))
	cache.Wait()

	subscriptionSvc := &SubscriptionService{subCacheL1: cache}
	paymentSvc := &PaymentService{subscriptionSvc: subscriptionSvc}
	paymentSvc.invalidateFinalizedRefundSubscriptionCaches(&RefundPlan{
		DeductionType:        payment.DeductionTypeSubscription,
		SubDaysToDeduct:      3,
		SubscriptionID:       subscription.ID,
		SubscriptionSnapshot: subscription,
	})

	_, subscriptionCached := cache.Get(subCacheKey(userID, groupID))
	_, activeListCached := cache.Get(activeSubscriptionsCacheKey(userID))
	require.False(t, subscriptionCached)
	require.False(t, activeListCached)
}

type refundProviderStub struct{}

func (s *refundProviderStub) Name() string { return "refund-stub" }

func (s *refundProviderStub) ProviderKey() string { return payment.TypeStripe }

func (s *refundProviderStub) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeStripe}
}

func (s *refundProviderStub) CreatePayment(context.Context, payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	return nil, errors.New("unexpected CreatePayment")
}

func (s *refundProviderStub) QueryOrder(context.Context, string) (*payment.QueryOrderResponse, error) {
	return nil, errors.New("unexpected QueryOrder")
}

func (s *refundProviderStub) VerifyNotification(context.Context, string, map[string]string) (*payment.PaymentNotification, error) {
	return nil, errors.New("unexpected VerifyNotification")
}

func (s *refundProviderStub) Refund(context.Context, payment.RefundRequest) (*payment.RefundResponse, error) {
	return &payment.RefundResponse{RefundID: "refund_stub_trade_no", Status: "success"}, nil
}

type failingRefundProviderStub struct{}

func (s *failingRefundProviderStub) Name() string { return "refund-failing-stub" }

func (s *failingRefundProviderStub) ProviderKey() string { return payment.TypeStripe }

func (s *failingRefundProviderStub) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeStripe}
}

func (s *failingRefundProviderStub) CreatePayment(context.Context, payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	return nil, errors.New("unexpected CreatePayment")
}

func (s *failingRefundProviderStub) QueryOrder(context.Context, string) (*payment.QueryOrderResponse, error) {
	return nil, errors.New("unexpected QueryOrder")
}

func (s *failingRefundProviderStub) VerifyNotification(context.Context, string, map[string]string) (*payment.PaymentNotification, error) {
	return nil, errors.New("unexpected VerifyNotification")
}

func (s *failingRefundProviderStub) Refund(context.Context, payment.RefundRequest) (*payment.RefundResponse, error) {
	return nil, errors.New("gateway unavailable")
}

type callbackRefundProviderStub struct {
	onRefund func()
	response *payment.RefundResponse
	err      error
}

func (s *callbackRefundProviderStub) Name() string { return "refund-callback-stub" }

func (s *callbackRefundProviderStub) ProviderKey() string { return payment.TypeStripe }

func (s *callbackRefundProviderStub) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeStripe}
}

func (s *callbackRefundProviderStub) CreatePayment(context.Context, payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	return nil, errors.New("unexpected CreatePayment")
}

func (s *callbackRefundProviderStub) QueryOrder(context.Context, string) (*payment.QueryOrderResponse, error) {
	return nil, errors.New("unexpected QueryOrder")
}

func (s *callbackRefundProviderStub) VerifyNotification(context.Context, string, map[string]string) (*payment.PaymentNotification, error) {
	return nil, errors.New("unexpected VerifyNotification")
}

func (s *callbackRefundProviderStub) Refund(context.Context, payment.RefundRequest) (*payment.RefundResponse, error) {
	if s.onRefund != nil {
		s.onRefund()
	}
	return s.response, s.err
}

type refundRollbackSubscriptionRepoStub struct {
	userSubRepoNoop

	byID        map[int64]*UserSubscription
	byUserGroup map[string]*UserSubscription
}

func newRefundRollbackSubscriptionRepoStub() *refundRollbackSubscriptionRepoStub {
	return &refundRollbackSubscriptionRepoStub{
		byID:        make(map[int64]*UserSubscription),
		byUserGroup: make(map[string]*UserSubscription),
	}
}

func (s *refundRollbackSubscriptionRepoStub) key(userID, groupID int64) string {
	return fmt.Sprintf("%d:%d", userID, groupID)
}

func (s *refundRollbackSubscriptionRepoStub) seed(sub *UserSubscription) {
	if sub == nil {
		return
	}
	cp := *sub
	s.byID[cp.ID] = &cp
	s.byUserGroup[s.key(cp.UserID, cp.GroupID)] = &cp
}

func (s *refundRollbackSubscriptionRepoStub) Create(_ context.Context, sub *UserSubscription) error {
	if sub == nil {
		return nil
	}
	cp := *sub
	if cp.ID == 0 {
		cp.ID = int64(len(s.byID) + 1)
	}
	sub.ID = cp.ID
	s.byID[cp.ID] = &cp
	s.byUserGroup[s.key(cp.UserID, cp.GroupID)] = &cp
	return nil
}

func (s *refundRollbackSubscriptionRepoStub) GetByID(_ context.Context, id int64) (*UserSubscription, error) {
	sub := s.byID[id]
	if sub == nil {
		return nil, ErrSubscriptionNotFound
	}
	cp := *sub
	return &cp, nil
}

func (s *refundRollbackSubscriptionRepoStub) GetByUserIDAndGroupID(_ context.Context, userID, groupID int64) (*UserSubscription, error) {
	sub := s.byUserGroup[s.key(userID, groupID)]
	if sub == nil {
		return nil, ErrSubscriptionNotFound
	}
	cp := *sub
	return &cp, nil
}

func (s *refundRollbackSubscriptionRepoStub) GetActiveByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*UserSubscription, error) {
	return s.GetByUserIDAndGroupID(ctx, userID, groupID)
}

func (s *refundRollbackSubscriptionRepoStub) ExtendExpiry(_ context.Context, subscriptionID int64, newExpiresAt time.Time) error {
	sub := s.byID[subscriptionID]
	if sub == nil {
		return ErrSubscriptionNotFound
	}
	if !newExpiresAt.After(time.Now()) {
		return ErrAdjustWouldExpire
	}
	sub.ExpiresAt = newExpiresAt
	return nil
}

func (s *refundRollbackSubscriptionRepoStub) UpdateStatus(_ context.Context, subscriptionID int64, status string) error {
	sub := s.byID[subscriptionID]
	if sub == nil {
		return ErrSubscriptionNotFound
	}
	sub.Status = status
	return nil
}

func (s *refundRollbackSubscriptionRepoStub) Delete(_ context.Context, subscriptionID int64) error {
	sub := s.byID[subscriptionID]
	if sub == nil {
		return ErrSubscriptionNotFound
	}
	delete(s.byID, subscriptionID)
	delete(s.byUserGroup, s.key(sub.UserID, sub.GroupID))
	return nil
}

func TestExecuteRefund_PartialSubscriptionRefundDeductsProportionalDays(t *testing.T) {
	ctx := context.Background()
	client := newPaymentServiceEntClient(t)
	user := createPaymentRefundTestUser(t, ctx, client, 0)

	groupID := int64(77)
	paidAt := time.Now().Add(-2 * time.Hour)
	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeStripe).
		SetName("stripe-subscription-refund").
		SetConfig(encryptWebhookProviderConfig(t, map[string]string{"secretKey": "sk_test_subscription_refund"})).
		SetSupportedTypes("stripe").
		SetEnabled(true).
		SetRefundEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	instID := strconv.FormatInt(inst.ID, 10)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(90).
		SetPayAmount(90).
		SetRechargeCode("sub-partial-refund").
		SetOutTradeNo("subscription_partial_refund_order").
		SetPaymentType(payment.TypeStripe).
		SetPaymentTradeNo("").
		SetOrderType(payment.OrderTypeSubscription).
		SetStatus(OrderStatusCompleted).
		SetSubscriptionGroupID(groupID).
		SetSubscriptionDays(30).
		SetExpiresAt(time.Now().Add(24 * time.Hour)).
		SetPaidAt(paidAt).
		SetClientIP("127.0.0.1").
		SetSrcHost("example.com").
		SetProviderInstanceID(instID).
		SetProviderKey(payment.TypeStripe).
		SetProviderSnapshot(map[string]any{
			"schema_version":       1,
			"provider_instance_id": instID,
			"provider_key":         payment.TypeStripe,
		}).
		Save(ctx)
	require.NoError(t, err)

	subRepo := newSubscriptionUserSubRepoStub()
	originalExpiry := time.Now().Add(29 * 24 * time.Hour)
	subRepo.seed(&UserSubscription{
		ID:        701,
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
		loadBalancer:    newWebhookProviderTestLoadBalancer(client),
		subscriptionSvc: subscriptionSvc,
	}

	plan, earlyResult, err := service.PrepareRefund(ctx, order.ID, 45, "partial refund", false, true)
	require.NoError(t, err)
	require.Nil(t, earlyResult)
	require.NotNil(t, plan)
	require.Equal(t, 15, plan.SubDaysToDeduct)
	require.Equal(t, int64(701), plan.SubscriptionID)

	result, err := service.ExecuteRefund(ctx, plan)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)
	require.Equal(t, 15, result.SubDaysDeducted)

	updatedOrder, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusPartiallyRefunded, updatedOrder.Status)
	require.Equal(t, 45.0, updatedOrder.RefundAmount)

	updatedSub, err := subRepo.GetByID(ctx, 701)
	require.NoError(t, err)
	require.WithinDuration(t, originalExpiry.AddDate(0, 0, -15), updatedSub.ExpiresAt, time.Second)
}

type refundWebhookRaceFixture struct {
	ctx              context.Context
	client           *dbent.Client
	svc              *PaymentService
	order            *dbent.PaymentOrder
	plan             *RefundPlan
	webhookErr       error
	initialDeducted  float64
	webhookDeducted  float64
	rollbackRestored float64
}

func newRefundWebhookRaceFixture(
	t *testing.T,
	response *payment.RefundResponse,
	refundErr error,
	amountSemantic string,
	rawData string,
) *refundWebhookRaceFixture {
	t.Helper()
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	user, err := client.User.Create().
		SetEmail("refund-webhook-race@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-webhook-race").
		SetBalance(100).
		Save(ctx)
	require.NoError(t, err)
	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeStripe).
		SetName("stripe-refund-webhook-race").
		SetConfig("{}").
		SetSupportedTypes("stripe").
		SetEnabled(true).
		SetRefundEnabled(true).
		Save(ctx)
	require.NoError(t, err)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(100).
		SetPayAmount(100).
		SetRechargeCode("refund-webhook-race").
		SetOutTradeNo("refund_webhook_race_order").
		SetPaymentType(payment.TypeStripe).
		SetPaymentTradeNo("trade_refund_webhook_race").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetProviderInstanceID(strconv.FormatInt(inst.ID, 10)).
		Save(ctx)
	require.NoError(t, err)

	fixture := &refundWebhookRaceFixture{ctx: ctx, client: client, order: order}
	userRepo := &mockUserRepo{
		getByIDUser: &User{ID: user.ID, Balance: 100},
		deductAvailableBalanceFn: func(_ context.Context, _ int64, amount float64) (float64, error) {
			fixture.initialDeducted += amount
			return amount, nil
		},
		deductBalanceFn: func(_ context.Context, _ int64, amount float64) error {
			fixture.webhookDeducted += amount
			return nil
		},
		updateBalanceFn: func(_ context.Context, _ int64, amount float64) error {
			fixture.rollbackRestored += amount
			return nil
		},
	}
	svc := &PaymentService{
		entClient:    client,
		loadBalancer: newWebhookProviderTestLoadBalancer(client),
		userRepo:     userRepo,
	}
	fixture.svc = svc
	provider := &callbackRefundProviderStub{
		response: response,
		err:      refundErr,
		onRefund: func() {
			fixture.webhookErr = svc.HandlePaymentNotification(ctx, &payment.PaymentNotification{
				OrderID:        order.OutTradeNo,
				TradeNo:        order.PaymentTradeNo,
				Amount:         50,
				AmountSemantic: amountSemantic,
				Status:         payment.NotificationStatusRefunded,
				RawData:        rawData,
			}, payment.TypeStripe)
		},
	}
	restore := replacePaymentProviderFactoryForTest(t, provider)
	t.Cleanup(restore)

	plan, early, err := svc.PrepareRefund(ctx, order.ID, 50, "webhook race", false, true)
	require.NoError(t, err)
	require.Nil(t, early)
	fixture.plan = plan
	return fixture
}

func TestExecuteRefundWebhookSuccessDoesNotDeductBalanceTwice(t *testing.T) {
	fixture := newRefundWebhookRaceFixture(
		t,
		&payment.RefundResponse{RefundID: "rf_race", Status: payment.ProviderStatusSuccess},
		nil,
		payment.NotificationAmountTotal,
		"stripe-total-refund-event",
	)

	result, err := fixture.svc.ExecuteRefund(fixture.ctx, fixture.plan)
	require.NoError(t, err)
	require.NoError(t, fixture.webhookErr)
	require.True(t, result.Success)
	require.Equal(t, 50.0, fixture.initialDeducted)
	require.Zero(t, fixture.webhookDeducted, "the webhook must not repeat a deduction already applied by the admin refund")
	require.Zero(t, fixture.rollbackRestored)

	reloaded, err := fixture.client.PaymentOrder.Get(fixture.ctx, fixture.order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusPartiallyRefunded, reloaded.Status)
	require.Equal(t, 50.0, reloaded.RefundAmount)
}

func TestExecuteRefundDeltaWebhookDoesNotDeductBalanceTwice(t *testing.T) {
	fixture := newRefundWebhookRaceFixture(
		t,
		&payment.RefundResponse{RefundID: "rf_delta_race", Status: payment.ProviderStatusSuccess},
		nil,
		payment.NotificationAmountDelta,
		"alipay-like-delta-refund-event",
	)

	result, err := fixture.svc.ExecuteRefund(fixture.ctx, fixture.plan)
	require.NoError(t, err)
	require.NoError(t, fixture.webhookErr)
	require.True(t, result.Success)
	require.Equal(t, 50.0, fixture.initialDeducted)
	require.Zero(t, fixture.webhookDeducted, "a delta webhook must confirm the in-flight admin refund without deducting it again")
	require.Zero(t, fixture.rollbackRestored)

	reloaded, err := fixture.client.PaymentOrder.Get(fixture.ctx, fixture.order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusPartiallyRefunded, reloaded.Status)
	require.Equal(t, 50.0, reloaded.RefundAmount)
}

func TestExecuteRefundWebhookSuccessWinsOverPendingProviderResponse(t *testing.T) {
	fixture := newRefundWebhookRaceFixture(
		t,
		&payment.RefundResponse{RefundID: "rf_race", Status: payment.ProviderStatusPending},
		nil,
		payment.NotificationAmountTotal,
		"stripe-pending-race-event",
	)

	result, err := fixture.svc.ExecuteRefund(fixture.ctx, fixture.plan)
	require.NoError(t, err)
	require.NoError(t, fixture.webhookErr)
	require.NotNil(t, result)
	require.True(t, result.Success)
	require.Equal(t, 50.0, fixture.initialDeducted)
	require.Zero(t, fixture.webhookDeducted)
	require.Zero(t, fixture.rollbackRestored, "a late pending response must not restore a refund already confirmed by webhook")

	reloaded, err := fixture.client.PaymentOrder.Get(fixture.ctx, fixture.order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusPartiallyRefunded, reloaded.Status)
	require.Equal(t, 50.0, reloaded.RefundAmount)
}

func TestMarkRefundPendingRequiresExternalCompletionAudit(t *testing.T) {
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
		OrderStatusPartiallyRefunded,
		"refund_pending_requires_external_audit",
		"trade_refund_pending_requires_external_audit",
	)
	svc := &PaymentService{entClient: client}
	plan := &RefundPlan{
		OrderID:       order.ID,
		Order:         order,
		RefundAmount:  50,
		GatewayAmount: 50,
		Reason:        "late pending response",
	}

	result, err := svc.markRefundPending(ctx, plan, &payment.RefundResponse{
		RefundID: "rf_without_external_confirmation",
		Status:   payment.ProviderStatusPending,
	})
	require.Nil(t, result)
	require.Error(t, err)
	require.Equal(t, "REFUND_PENDING_CONFLICT", infraerrors.Reason(err))
}

func TestExecuteRefundWebhookSuccessWinsOverProviderError(t *testing.T) {
	fixture := newRefundWebhookRaceFixture(
		t,
		nil,
		errors.New("provider response lost"),
		payment.NotificationAmountTotal,
		"stripe-error-race-event",
	)

	result, err := fixture.svc.ExecuteRefund(fixture.ctx, fixture.plan)
	require.NoError(t, err)
	require.NoError(t, fixture.webhookErr)
	require.NotNil(t, result)
	require.True(t, result.Success)
	require.Equal(t, 50.0, fixture.initialDeducted)
	require.Zero(t, fixture.webhookDeducted)
	require.Zero(t, fixture.rollbackRestored, "a late transport error must not restore a refund already confirmed by webhook")

	reloaded, err := fixture.client.PaymentOrder.Get(fixture.ctx, fixture.order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusPartiallyRefunded, reloaded.Status)
	require.Equal(t, 50.0, reloaded.RefundAmount)
}

func TestExecuteRefund_ConcurrentChargebackPreservesClaimAndCombinesOnSuccess(t *testing.T) {
	fixture := newRefundWebhookRaceFixture(
		t,
		&payment.RefundResponse{RefundID: "unused", Status: payment.ProviderStatusSuccess},
		nil,
		payment.NotificationAmountTotal,
		"unused-refund-event",
	)
	rechargeRepo := newRechargeOrderRepoStub()
	rechargeRepo.orders["stripe::"+fixture.order.OutTradeNo] = &RechargeOrder{
		ID:              910,
		UserID:          fixture.order.UserID,
		Provider:        payment.TypeStripe,
		ExternalOrderID: fixture.order.OutTradeNo,
		PaidAmount:      100,
		Status:          RechargeOrderStatusCredited,
		Currency:        ReferralSettlementCurrencyCNY,
	}
	fixture.svc.referralRefundSvc = NewReferralRefundService(rechargeRepo, &commissionRepoStub{}, nil, nil)
	affiliateRepo := &paymentFulfillmentAffiliateRepoStub{}
	fixture.svc.affiliateService = NewAffiliateService(affiliateRepo, nil, nil, nil)
	fixture.svc.affiliateReversalEnabled = true
	_, err := fixture.client.PaymentAuditLog.Create().
		SetOrderID(strconv.FormatInt(fixture.order.ID, 10)).
		SetAction("AFFILIATE_REBATE_APPLIED").
		SetDetail(`{"rebateAmount":10}`).
		SetOperator("system").
		Save(fixture.ctx)
	require.NoError(t, err)

	claimPreserved := false
	provider := &callbackRefundProviderStub{
		response: &payment.RefundResponse{RefundID: "rf_manual_success", Status: payment.ProviderStatusSuccess},
		onRefund: func() {
			fixture.webhookErr = fixture.svc.HandlePaymentNotification(fixture.ctx, &payment.PaymentNotification{
				EventID:        "evt_chargeback_during_manual_success",
				OrderID:        fixture.order.OutTradeNo,
				TradeNo:        fixture.order.PaymentTradeNo,
				Amount:         20,
				AmountSemantic: payment.NotificationAmountTotal,
				Status:         payment.NotificationStatusChargeback,
			}, payment.TypeStripe)
			current, err := fixture.client.PaymentOrder.Get(fixture.ctx, fixture.order.ID)
			if err == nil {
				_, claimPreserved = refundInFlightDetailFromOrder(current)
				claimPreserved = claimPreserved && current.Status == OrderStatusRefunding
			}
		},
	}
	restore := replacePaymentProviderFactoryForTest(t, provider)
	t.Cleanup(restore)

	result, err := fixture.svc.ExecuteRefund(fixture.ctx, fixture.plan)
	require.NoError(t, err)
	require.NoError(t, fixture.webhookErr)
	require.True(t, result.Success)
	require.True(t, claimPreserved, "a chargeback must not clear or finalize the unrelated in-flight refund claim")
	require.Equal(t, 50.0, fixture.initialDeducted)
	require.Equal(t, 20.0, fixture.webhookDeducted)
	require.Zero(t, fixture.rollbackRestored)

	updated, err := fixture.client.PaymentOrder.Get(fixture.ctx, fixture.order.ID)
	require.NoError(t, err)
	require.Equal(t, 50.0, updated.ProviderRefundAmount)
	require.Equal(t, 20.0, updated.ChargebackAmount)
	require.Equal(t, 70.0, updated.RefundAmount)
	require.Equal(t, OrderStatusPartiallyRefunded, updated.Status)

	recharge, err := rechargeRepo.GetByProviderAndExternalOrderID(fixture.ctx, payment.TypeStripe, fixture.order.OutTradeNo)
	require.NoError(t, err)
	require.Equal(t, 50.0, recharge.RefundedAmount)
	require.Equal(t, 20.0, recharge.ChargebackAmount)
	require.Len(t, affiliateRepo.reverseCalls, 2)
	require.InDelta(t, 0.2, affiliateRepo.reverseCalls[0].cumulativeRefundRatio, 1e-9)
	require.InDelta(t, 0.7, affiliateRepo.reverseCalls[1].cumulativeRefundRatio, 1e-9)
}

func TestExecuteRefund_ConcurrentChargebackSurvivesGatewayFailureRollback(t *testing.T) {
	fixture := newRefundWebhookRaceFixture(
		t,
		nil,
		errors.New("unused"),
		payment.NotificationAmountTotal,
		"unused-refund-event",
	)
	rechargeRepo := newRechargeOrderRepoStub()
	rechargeRepo.orders["stripe::"+fixture.order.OutTradeNo] = &RechargeOrder{
		ID:              911,
		UserID:          fixture.order.UserID,
		Provider:        payment.TypeStripe,
		ExternalOrderID: fixture.order.OutTradeNo,
		PaidAmount:      100,
		Status:          RechargeOrderStatusCredited,
		Currency:        ReferralSettlementCurrencyCNY,
	}
	fixture.svc.referralRefundSvc = NewReferralRefundService(rechargeRepo, &commissionRepoStub{}, nil, nil)
	affiliateRepo := &paymentFulfillmentAffiliateRepoStub{}
	fixture.svc.affiliateService = NewAffiliateService(affiliateRepo, nil, nil, nil)
	fixture.svc.affiliateReversalEnabled = true
	_, err := fixture.client.PaymentAuditLog.Create().
		SetOrderID(strconv.FormatInt(fixture.order.ID, 10)).
		SetAction("AFFILIATE_REBATE_APPLIED").
		SetDetail(`{"rebateAmount":10}`).
		SetOperator("system").
		Save(fixture.ctx)
	require.NoError(t, err)

	provider := &callbackRefundProviderStub{
		err: errors.New("provider unavailable after chargeback callback"),
		onRefund: func() {
			fixture.webhookErr = fixture.svc.HandlePaymentNotification(fixture.ctx, &payment.PaymentNotification{
				EventID:        "evt_chargeback_during_manual_failure",
				OrderID:        fixture.order.OutTradeNo,
				TradeNo:        fixture.order.PaymentTradeNo,
				Amount:         20,
				AmountSemantic: payment.NotificationAmountTotal,
				Status:         payment.NotificationStatusChargeback,
			}, payment.TypeStripe)
		},
	}
	restore := replacePaymentProviderFactoryForTest(t, provider)
	t.Cleanup(restore)

	result, err := fixture.svc.ExecuteRefund(fixture.ctx, fixture.plan)
	require.NoError(t, err)
	require.NoError(t, fixture.webhookErr)
	require.False(t, result.Success)
	require.Equal(t, 50.0, fixture.initialDeducted)
	require.Equal(t, 20.0, fixture.webhookDeducted)
	require.Equal(t, 50.0, fixture.rollbackRestored)

	updated, err := fixture.client.PaymentOrder.Get(fixture.ctx, fixture.order.ID)
	require.NoError(t, err)
	require.Zero(t, updated.ProviderRefundAmount)
	require.Equal(t, 20.0, updated.ChargebackAmount)
	require.Equal(t, 20.0, updated.RefundAmount)
	require.Equal(t, OrderStatusPartiallyRefunded, updated.Status)
	_, hasClaim := refundInFlightDetailFromOrder(updated)
	require.False(t, hasClaim)

	recharge, err := rechargeRepo.GetByProviderAndExternalOrderID(fixture.ctx, payment.TypeStripe, fixture.order.OutTradeNo)
	require.NoError(t, err)
	require.Zero(t, recharge.RefundedAmount)
	require.Equal(t, 20.0, recharge.ChargebackAmount)
	require.Len(t, affiliateRepo.reverseCalls, 1)
	require.InDelta(t, 0.2, affiliateRepo.reverseCalls[0].cumulativeRefundRatio, 1e-9)
}

func TestExecuteRefund_DeductsSubscriptionEntitlementDays(t *testing.T) {
	ctx := context.Background()
	client := newPaymentServiceEntClient(t)
	user := createPaymentRefundTestUser(t, ctx, client, 0)
	now := time.Now().Truncate(time.Second)

	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeStripe).
		SetName("stripe-entitlement-refund").
		SetConfig("{}").
		SetSupportedTypes("stripe").
		SetEnabled(true).
		SetRefundEnabled(true).
		Save(ctx)
	require.NoError(t, err)
	placeholder, err := client.SubscriptionEntitlement.Create().
		SetUserID(user.ID).
		SetName("manual refund entitlement").
		SetSourceType("test").
		SetStatus(SubscriptionStatusActive).
		SetStartsAt(now.Add(-24 * time.Hour)).
		SetExpiresAt(now.Add(30 * 24 * time.Hour)).
		SetOveragePolicy(SubscriptionEntitlementOverageBlock).
		SetPlanSnapshot(map[string]any{}).
		SetAssignedAt(now).
		Save(ctx)
	require.NoError(t, err)

	instID := strconv.FormatInt(inst.ID, 10)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(90).
		SetPayAmount(90).
		SetRechargeCode("sub-entitlement-refund").
		SetOutTradeNo("subscription_entitlement_refund_order").
		SetPaymentType(payment.TypeStripe).
		SetPaymentTradeNo("").
		SetOrderType(payment.OrderTypeSubscription).
		SetStatus(OrderStatusCompleted).
		SetPlanID(9902).
		SetSubscriptionEntitlementID(placeholder.ID).
		SetSubscriptionDays(30).
		SetExpiresAt(now.Add(time.Hour)).
		SetPaidAt(now.Add(-time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("example.com").
		SetProviderInstanceID(instID).
		SetProviderKey(payment.TypeStripe).
		SetProviderSnapshot(map[string]any{
			"schema_version":       1,
			"provider_instance_id": instID,
			"provider_key":         payment.TypeStripe,
		}).
		Save(ctx)
	require.NoError(t, err)

	originalExpiry := now.Add(30 * 24 * time.Hour)
	entRepo := newFakeSubscriptionEntitlementRepo(now)
	entRepo.entitlements[placeholder.ID] = &SubscriptionEntitlement{
		ID:        placeholder.ID,
		UserID:    user.ID,
		PlanID:    int64ValuePtr(9902),
		Name:      "manual refund entitlement",
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

	plan, earlyResult, err := service.PrepareRefund(ctx, order.ID, 45, "partial entitlement refund", false, true)
	require.NoError(t, err)
	require.Nil(t, earlyResult)
	require.NotNil(t, plan)
	require.Equal(t, 15, plan.SubDaysToDeduct)
	require.Equal(t, placeholder.ID, plan.EntitlementID)

	result, err := service.ExecuteRefund(ctx, plan)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)
	require.Equal(t, 15, result.SubDaysDeducted)

	ent, err := entRepo.GetByID(ctx, placeholder.ID)
	require.NoError(t, err)
	require.WithinDuration(t, originalExpiry.AddDate(0, 0, -15), ent.ExpiresAt, time.Second)
}

func TestExecuteRefund_FailedGatewayRefundRestoresRevokedSubscription(t *testing.T) {
	ctx := context.Background()
	client := newPaymentServiceEntClient(t)
	user := createPaymentRefundTestUser(t, ctx, client, 0)

	groupID := int64(78)
	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("alipay-subscription-refund").
		SetConfig(encryptWebhookProviderConfig(t, map[string]string{
			"appId":      "runtime-alipay-app",
			"privateKey": "runtime-private-key",
		})).
		SetSupportedTypes("alipay").
		SetEnabled(true).
		SetRefundEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	instID := strconv.FormatInt(inst.ID, 10)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(90).
		SetPayAmount(90).
		SetRechargeCode("sub-revoke-rollback").
		SetOutTradeNo("subscription_revoke_rollback_order").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade_subscription_revoke_rollback").
		SetOrderType(payment.OrderTypeSubscription).
		SetStatus(OrderStatusCompleted).
		SetSubscriptionGroupID(groupID).
		SetSubscriptionDays(30).
		SetExpiresAt(time.Now().Add(24 * time.Hour)).
		SetPaidAt(time.Now().Add(-time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("example.com").
		SetProviderInstanceID(instID).
		SetProviderKey(payment.TypeAlipay).
		SetProviderSnapshot(map[string]any{
			"schema_version":       2,
			"provider_instance_id": instID,
			"provider_key":         payment.TypeAlipay,
			"merchant_app_id":      "expected-alipay-app",
		}).
		Save(ctx)
	require.NoError(t, err)

	subRepo := newRefundRollbackSubscriptionRepoStub()
	originalExpiry := time.Now().Add(12 * time.Hour)
	subRepo.seed(&UserSubscription{
		ID:        801,
		UserID:    user.ID,
		GroupID:   groupID,
		Status:    SubscriptionStatusActive,
		StartsAt:  time.Now().Add(-24 * time.Hour),
		ExpiresAt: originalExpiry,
		Notes:     "payment order restore target",
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
		loadBalancer:    newWebhookProviderTestLoadBalancer(client),
		subscriptionSvc: subscriptionSvc,
	}

	plan, earlyResult, err := service.PrepareRefund(ctx, order.ID, 45, "partial refund", false, true)
	require.NoError(t, err)
	require.Nil(t, earlyResult)
	require.NotNil(t, plan)
	require.Equal(t, 15, plan.SubDaysToDeduct)
	require.Equal(t, int64(801), plan.SubscriptionID)

	result, err := service.ExecuteRefund(ctx, plan)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.Success)
	require.Contains(t, result.Warning, "gateway failed")

	restoredSub, err := subRepo.GetByUserIDAndGroupID(ctx, user.ID, groupID)
	require.NoError(t, err)
	require.Equal(t, SubscriptionStatusActive, restoredSub.Status)
	require.WithinDuration(t, originalExpiry, restoredSub.ExpiresAt, time.Second)

	updatedOrder, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusCompleted, updatedOrder.Status)
}

func TestExecuteRefund_FailedGatewayRefundRestoresSubscriptionEntitlement(t *testing.T) {
	ctx := context.Background()
	client := newPaymentServiceEntClient(t)
	user := createPaymentRefundTestUser(t, ctx, client, 0)
	now := time.Now().Truncate(time.Second)

	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("alipay-entitlement-refund").
		SetConfig(encryptWebhookProviderConfig(t, map[string]string{
			"appId":      "runtime-alipay-app",
			"privateKey": "runtime-private-key",
		})).
		SetSupportedTypes("alipay").
		SetEnabled(true).
		SetRefundEnabled(true).
		Save(ctx)
	require.NoError(t, err)
	placeholder, err := client.SubscriptionEntitlement.Create().
		SetUserID(user.ID).
		SetName("rollback entitlement").
		SetSourceType("test").
		SetStatus(SubscriptionStatusActive).
		SetStartsAt(now.Add(-24 * time.Hour)).
		SetExpiresAt(now.Add(12 * time.Hour)).
		SetOveragePolicy(SubscriptionEntitlementOverageBlock).
		SetPlanSnapshot(map[string]any{}).
		SetAssignedAt(now).
		Save(ctx)
	require.NoError(t, err)

	instID := strconv.FormatInt(inst.ID, 10)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(90).
		SetPayAmount(90).
		SetRechargeCode("sub-entitlement-rollback").
		SetOutTradeNo("subscription_entitlement_rollback_order").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade_subscription_entitlement_rollback").
		SetOrderType(payment.OrderTypeSubscription).
		SetStatus(OrderStatusCompleted).
		SetPlanID(9903).
		SetSubscriptionEntitlementID(placeholder.ID).
		SetSubscriptionDays(30).
		SetExpiresAt(now.Add(time.Hour)).
		SetPaidAt(now.Add(-time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("example.com").
		SetProviderInstanceID(instID).
		SetProviderKey(payment.TypeAlipay).
		SetProviderSnapshot(map[string]any{
			"schema_version":       2,
			"provider_instance_id": instID,
			"provider_key":         payment.TypeAlipay,
			"merchant_app_id":      "expected-alipay-app",
		}).
		Save(ctx)
	require.NoError(t, err)

	originalExpiry := now.Add(12 * time.Hour)
	entRepo := newFakeSubscriptionEntitlementRepo(now)
	entRepo.entitlements[placeholder.ID] = &SubscriptionEntitlement{
		ID:        placeholder.ID,
		UserID:    user.ID,
		PlanID:    int64ValuePtr(9903),
		Name:      "rollback entitlement",
		Status:    SubscriptionStatusActive,
		StartsAt:  now.Add(-24 * time.Hour),
		ExpiresAt: originalExpiry,
		Notes:     "restore target",
	}
	entSvc := NewSubscriptionEntitlementService(entRepo, &fakeSubscriptionEntitlementPlanRepo{})
	entSvc.SetNowFunc(func() time.Time { return now })

	service := &PaymentService{
		entClient:                  client,
		loadBalancer:               newWebhookProviderTestLoadBalancer(client),
		subscriptionEntitlementSvc: entSvc,
	}

	plan, earlyResult, err := service.PrepareRefund(ctx, order.ID, 45, "partial entitlement refund", false, true)
	require.NoError(t, err)
	require.Nil(t, earlyResult)
	require.NotNil(t, plan)
	require.Equal(t, 15, plan.SubDaysToDeduct)
	require.Equal(t, placeholder.ID, plan.EntitlementID)

	result, err := service.ExecuteRefund(ctx, plan)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.Success)
	require.Contains(t, result.Warning, "gateway failed")

	ent, err := entRepo.GetByID(ctx, placeholder.ID)
	require.NoError(t, err)
	require.Equal(t, SubscriptionStatusActive, ent.Status)
	require.WithinDuration(t, originalExpiry, ent.ExpiresAt, time.Second)
	require.Equal(t, "restore target", ent.Notes)

	updatedOrder, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusCompleted, updatedOrder.Status)
}

func TestValidateRefundRequestRejectsLegacyGuessedProviderInstance(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("refund-legacy@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-legacy-user").
		Save(ctx)
	require.NoError(t, err)

	_, err = client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("alipay-refund-instance").
		SetConfig("{}").
		SetSupportedTypes("alipay").
		SetEnabled(true).
		SetAllowUserRefund(true).
		SetRefundEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(88).
		SetPayAmount(88).
		SetFeeRate(0).
		SetRechargeCode("REFUND-LEGACY-ORDER").
		SetOutTradeNo("sub2_refund_legacy_order").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-legacy-refund").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{
		entClient: client,
	}

	_, err = svc.validateRefundRequest(ctx, order.ID, user.ID)
	require.Error(t, err)
	require.Equal(t, "USER_REFUND_DISABLED", infraerrors.Reason(err))
}

func TestPrepareRefundRejectsLegacyGuessedProviderInstance(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("refund-legacy-admin@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-legacy-admin-user").
		Save(ctx)
	require.NoError(t, err)

	_, err = client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("alipay-refund-admin-instance").
		SetConfig("{}").
		SetSupportedTypes("alipay").
		SetEnabled(true).
		SetAllowUserRefund(true).
		SetRefundEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(188).
		SetPayAmount(188).
		SetFeeRate(0).
		SetRechargeCode("REFUND-LEGACY-ADMIN-ORDER").
		SetOutTradeNo("sub2_refund_legacy_admin_order").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-legacy-admin-refund").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{
		entClient: client,
	}

	plan, result, err := svc.PrepareRefund(ctx, order.ID, 0, "", false, false)
	require.Nil(t, plan)
	require.Nil(t, result)
	require.Error(t, err)
	require.Equal(t, "REFUND_DISABLED", infraerrors.Reason(err))
}

func TestGwRefundRejectsAlipayMerchantIdentitySnapshotMismatch(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("refund-snapshot-mismatch@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-snapshot-mismatch-user").
		Save(ctx)
	require.NoError(t, err)

	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("alipay-refund-mismatch-instance").
		SetConfig(encryptWebhookProviderConfig(t, map[string]string{
			"appId":      "runtime-alipay-app",
			"privateKey": "runtime-private-key",
		})).
		SetSupportedTypes("alipay").
		SetEnabled(true).
		SetRefundEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	instID := strconv.FormatInt(inst.ID, 10)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(88).
		SetPayAmount(88).
		SetFeeRate(0).
		SetRechargeCode("REFUND-SNAPSHOT-MISMATCH-ORDER").
		SetOutTradeNo("sub2_refund_snapshot_mismatch_order").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-refund-snapshot-mismatch").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetProviderInstanceID(instID).
		SetProviderKey(payment.TypeAlipay).
		SetProviderSnapshot(map[string]any{
			"schema_version":       2,
			"provider_instance_id": instID,
			"provider_key":         payment.TypeAlipay,
			"merchant_app_id":      "expected-alipay-app",
		}).
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{
		entClient:    client,
		loadBalancer: newWebhookProviderTestLoadBalancer(client),
	}

	_, err = svc.gwRefund(ctx, &RefundPlan{
		OrderID:       order.ID,
		Order:         order,
		RefundAmount:  order.Amount,
		GatewayAmount: order.Amount,
		Reason:        "snapshot mismatch",
	})
	require.ErrorContains(t, err, "alipay app_id mismatch")
}

func TestCalculateGatewayRefundAmountUsesCurrencyPrecision(t *testing.T) {
	require.InDelta(t, 6.173, calculateGatewayRefundAmount(100, 12.345, 50, "KWD"), 1e-12)
	require.InDelta(t, 12.345, calculateGatewayRefundAmount(100, 12.345, 100, "KWD"), 1e-12)
	require.InDelta(t, 52, calculateGatewayRefundAmount(100, 103, 50, "JPY"), 1e-12)
}

func TestFormatGatewayRefundAmountUsesOrderCurrency(t *testing.T) {
	order := &dbent.PaymentOrder{
		ProviderSnapshot: map[string]any{
			"currency": "KWD",
		},
	}

	require.Equal(t, "12.345", formatGatewayRefundAmount(12.345, order))
}

func TestValidateRefundProviderResponseAcceptsPending(t *testing.T) {
	require.NoError(t, validateRefundProviderResponse(&payment.RefundResponse{Status: payment.ProviderStatusPending}))
	require.NoError(t, validateRefundProviderResponse(&payment.RefundResponse{Status: payment.ProviderStatusSuccess}))
	require.Error(t, validateRefundProviderResponse(&payment.RefundResponse{Status: payment.ProviderStatusFailed}))
	require.Error(t, validateRefundProviderResponse(nil))
}

func TestFinishRefundPendingMarksOrderPendingAndRollsBackDeduction(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("refund-pending@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-pending-user").
		Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(100).
		SetPayAmount(100).
		SetFeeRate(0).
		SetRechargeCode("REFUND-PENDING-ORDER").
		SetOutTradeNo("sub2_refund_pending_order").
		SetPaymentType(payment.TypeStripe).
		SetPaymentTradeNo("pi_refund_pending").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusRefunding).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
	require.NoError(t, err)

	var rolledBack float64
	userRepo := &mockUserRepo{}
	userRepo.updateBalanceFn = func(ctx context.Context, id int64, amount float64) error {
		require.Equal(t, user.ID, id)
		rolledBack += amount
		return nil
	}
	svc := &PaymentService{
		entClient: client,
		userRepo:  userRepo,
	}
	plan := &RefundPlan{
		OrderID:         order.ID,
		Order:           order,
		RefundAmount:    40,
		GatewayAmount:   40,
		Reason:          "gateway accepted but not final",
		Force:           true,
		DeductionType:   payment.DeductionTypeBalance,
		BalanceToDeduct: 40,
	}

	result, err := svc.finishRefund(ctx, plan, &payment.RefundResponse{
		RefundID: "refund_stub_trade_no",
		Status:   payment.ProviderStatusPending,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.Success)
	require.Contains(t, result.Warning, "pending confirmation")
	require.Equal(t, 40.0, rolledBack)
	require.Zero(t, plan.BalanceToDeduct)

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefundPending, reloaded.Status)
	require.Zero(t, reloaded.ProviderRefundAmount)
	require.Zero(t, reloaded.ChargebackAmount)
	require.Zero(t, reloaded.RefundAmount, "a pending provider operation is not a settled reversal component")
	require.NotNil(t, reloaded.RefundReason)
	require.Equal(t, "gateway accepted but not final", *reloaded.RefundReason)
	require.Nil(t, reloaded.RefundAt)

	pendingAudits, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, pendingAudits)
	successAudits, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_SUCCESS")).
		Count(ctx)
	require.NoError(t, err)
	require.Zero(t, successAudits)

	_, err = client.PaymentAuditLog.Delete().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
		Exec(ctx)
	require.NoError(t, err)
	pendingDetail := svc.latestRefundPendingDetail(ctx, reloaded)
	require.Equal(t, "refund_stub_trade_no", pendingDetail.RefundID)
	require.True(t, pendingDetail.DeductionRollbackOK)
	require.NotNil(t, pendingDetail.BalanceRolledBack)
	require.Equal(t, 40.0, *pendingDetail.BalanceRolledBack)
}

func TestFinishRefundSuccessStatusesFinalize(t *testing.T) {
	for _, status := range []string{payment.ProviderStatusSuccess, payment.ProviderStatusRefunded} {
		t.Run(status, func(t *testing.T) {
			ctx := context.Background()
			client := newPaymentConfigServiceTestClient(t)

			user, err := client.User.Create().
				SetEmail("refund-success-" + status + "@example.com").
				SetPasswordHash("hash").
				SetUsername("refund-success-" + status).
				Save(ctx)
			require.NoError(t, err)

			order, err := client.PaymentOrder.Create().
				SetUserID(user.ID).
				SetUserEmail(user.Email).
				SetUserName(user.Username).
				SetAmount(100).
				SetPayAmount(100).
				SetFeeRate(0).
				SetRechargeCode("REFUND-SUCCESS-" + status).
				SetOutTradeNo("sub2_refund_success_" + status).
				SetPaymentType(payment.TypeStripe).
				SetPaymentTradeNo("pi_refund_success_" + status).
				SetOrderType(payment.OrderTypeBalance).
				SetStatus(OrderStatusRefunding).
				SetExpiresAt(time.Now().Add(time.Hour)).
				SetPaidAt(time.Now()).
				SetClientIP("127.0.0.1").
				SetSrcHost("api.example.com").
				Save(ctx)
			require.NoError(t, err)

			svc := &PaymentService{entClient: client}
			plan := &RefundPlan{
				OrderID:         order.ID,
				Order:           order,
				RefundAmount:    100,
				GatewayAmount:   100,
				Reason:          "final success",
				DeductionType:   payment.DeductionTypeBalance,
				BalanceToDeduct: 100,
			}

			result, err := svc.finishRefund(ctx, plan, &payment.RefundResponse{Status: status})
			require.NoError(t, err)
			require.NotNil(t, result)
			require.True(t, result.Success)
			require.Equal(t, 100.0, result.BalanceDeducted)

			reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
			require.NoError(t, err)
			require.Equal(t, OrderStatusRefunded, reloaded.Status)
			require.NotNil(t, reloaded.RefundAt)

			successAudits, err := client.PaymentAuditLog.Query().
				Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_SUCCESS")).
				Count(ctx)
			require.NoError(t, err)
			require.Equal(t, 1, successAudits)
			pendingAudits, err := client.PaymentAuditLog.Query().
				Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
				Count(ctx)
			require.NoError(t, err)
			require.Zero(t, pendingAudits)
		})
	}
}

func TestQueryAndFinalizeRefundFinalizesProviderStatuses(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     string
		wantStatus string
		wantDeduct float64
	}{
		{name: "success", status: payment.ProviderStatusSuccess, wantStatus: OrderStatusRefunded, wantDeduct: 100},
		{name: "failed", status: payment.ProviderStatusFailed, wantStatus: OrderStatusRefundFailed},
		{name: "pending", status: payment.ProviderStatusPending, wantStatus: OrderStatusRefundPending},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			client := newPaymentConfigServiceTestClient(t)
			order := createPendingRefundOrderForTest(t, ctx, client, "query-finalize-"+tc.name)

			var deducted float64
			svc := &PaymentService{
				entClient:    client,
				loadBalancer: &captureLoadBalancer{},
				userRepo: &mockUserRepo{deductAvailableBalanceFn: func(ctx context.Context, id int64, amount float64) (float64, error) {
					deducted += amount
					return amount, nil
				}},
			}
			restore := replacePaymentProviderFactoryForTest(t, &refundQueryProviderTestDouble{
				refundResponse: &payment.RefundResponse{RefundID: "rf_test", Status: tc.status},
			})
			defer restore()

			result, err := svc.QueryAndFinalizeRefund(ctx, order.ID)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, tc.status == payment.ProviderStatusSuccess, result.Success)
			require.Equal(t, tc.wantDeduct, deducted)

			reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
			require.NoError(t, err)
			require.Equal(t, tc.wantStatus, reloaded.Status)
		})
	}
}

func TestQueryAndFinalizeRefundDeductsSubscriptionEntitlementDays(t *testing.T) {
	ctx := context.Background()
	client := newPaymentServiceEntClient(t)
	user := createPaymentRefundTestUser(t, ctx, client, 0)
	now := time.Now().Truncate(time.Second)

	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeStripe).
		SetName("pending-entitlement-refund-provider").
		SetConfig("{}").
		SetSupportedTypes("stripe").
		SetEnabled(true).
		SetRefundEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	placeholder, err := client.SubscriptionEntitlement.Create().
		SetUserID(user.ID).
		SetName("pending refund entitlement").
		SetSourceType("test").
		SetStatus(SubscriptionStatusActive).
		SetStartsAt(now.Add(-24 * time.Hour)).
		SetExpiresAt(now.Add(30 * 24 * time.Hour)).
		SetOveragePolicy(SubscriptionEntitlementOverageBlock).
		SetPlanSnapshot(map[string]any{}).
		SetAssignedAt(now).
		Save(ctx)
	require.NoError(t, err)

	instID := strconv.FormatInt(inst.ID, 10)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(90).
		SetPayAmount(90).
		SetRechargeCode("pending-entitlement-refund").
		SetOutTradeNo("pending_entitlement_refund_order").
		SetPaymentType(payment.TypeStripe).
		SetPaymentTradeNo("pi_pending_entitlement_refund").
		SetOrderType(payment.OrderTypeSubscription).
		SetStatus(OrderStatusRefundPending).
		SetPlanID(9904).
		SetSubscriptionEntitlementID(placeholder.ID).
		SetSubscriptionDays(30).
		SetRefundAmount(45).
		SetRefundReason("pending partial entitlement refund").
		SetExpiresAt(now.Add(time.Hour)).
		SetPaidAt(now.Add(-time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetProviderInstanceID(instID).
		SetProviderKey(payment.TypeStripe).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.PaymentAuditLog.Create().
		SetOrderID(strconv.FormatInt(order.ID, 10)).
		SetAction("REFUND_PENDING").
		SetOperator("admin").
		SetDetail(`{"refundID":"rf_test","deductionRollbackOK":true,"subDaysRolledBack":15}`).
		Save(ctx)
	require.NoError(t, err)

	originalExpiry := now.Add(30 * 24 * time.Hour)
	entRepo := newFakeSubscriptionEntitlementRepo(now)
	entRepo.entitlements[placeholder.ID] = &SubscriptionEntitlement{
		ID:        placeholder.ID,
		UserID:    user.ID,
		PlanID:    int64ValuePtr(9904),
		Name:      "pending refund entitlement",
		Status:    SubscriptionStatusActive,
		StartsAt:  now.Add(-24 * time.Hour),
		ExpiresAt: originalExpiry,
	}
	entSvc := NewSubscriptionEntitlementService(entRepo, &fakeSubscriptionEntitlementPlanRepo{})
	entSvc.SetNowFunc(func() time.Time { return now })
	svc := &PaymentService{
		entClient:                  client,
		loadBalancer:               &captureLoadBalancer{},
		subscriptionEntitlementSvc: entSvc,
	}
	restore := replacePaymentProviderFactoryForTest(t, &refundQueryProviderTestDouble{
		refundResponse: &payment.RefundResponse{RefundID: "rf_test", Status: payment.ProviderStatusSuccess},
	})
	defer restore()

	result, err := svc.QueryAndFinalizeRefund(ctx, order.ID)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)
	require.Equal(t, 15, result.SubDaysDeducted)

	ent, err := entRepo.GetByID(ctx, placeholder.ID)
	require.NoError(t, err)
	require.WithinDuration(t, originalExpiry.AddDate(0, 0, -15), ent.ExpiresAt, time.Second)

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusPartiallyRefunded, reloaded.Status)
}

func TestQueryAndFinalizeRefundUsesPendingRollbackBalanceAmount(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "query-finalize-balance-rollback")
	order, err := client.PaymentOrder.UpdateOneID(order.ID).
		SetRefundAmount(60).
		Save(ctx)
	require.NoError(t, err)
	pendingAudit, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
		Only(ctx)
	require.NoError(t, err)
	_, err = client.PaymentAuditLog.UpdateOneID(pendingAudit.ID).
		SetDetail(`{"refundID":"rf_test","deductionRollbackOK":true,"balanceRolledBack":20}`).
		Save(ctx)
	require.NoError(t, err)

	var deducted float64
	svc := &PaymentService{
		entClient:    client,
		loadBalancer: &captureLoadBalancer{},
		userRepo: &mockUserRepo{deductAvailableBalanceFn: func(_ context.Context, _ int64, amount float64) (float64, error) {
			deducted += amount
			return amount, nil
		}},
	}
	restore := replacePaymentProviderFactoryForTest(t, &refundQueryProviderTestDouble{
		refundResponse: &payment.RefundResponse{RefundID: "rf_test", Status: payment.ProviderStatusSuccess},
	})
	defer restore()

	result, err := svc.QueryAndFinalizeRefund(ctx, order.ID)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)
	require.Equal(t, 20.0, result.BalanceDeducted)
	require.Equal(t, 20.0, deducted)
}

func TestApplyRefundFinalDeductionShortensSubscriptionEntitlement(t *testing.T) {
	ctx := context.Background()
	// Keep the entitlement in the future relative to the real clock. A fixed
	// 2026 date eventually crossed the service's "never expire in the past"
	// clamp and made this otherwise deterministic test time-bomb.
	now := time.Now().UTC().Truncate(time.Second)
	client := newPaymentConfigServiceTestClient(t)
	user, err := client.User.Create().
		SetEmail("pending-entitlement-refund@example.com").
		SetPasswordHash("hash").
		SetUsername("pending-entitlement-refund").
		Save(ctx)
	require.NoError(t, err)

	entitlementID := int64(71)
	originalExpiry := now.AddDate(0, 0, 30)
	entRepo := newFakeSubscriptionEntitlementRepo(now)
	entRepo.entitlements[entitlementID] = &SubscriptionEntitlement{
		ID:        entitlementID,
		UserID:    user.ID,
		Status:    SubscriptionStatusActive,
		StartsAt:  now.Add(-24 * time.Hour),
		ExpiresAt: originalExpiry,
	}
	entSvc := NewSubscriptionEntitlementService(entRepo, &fakeSubscriptionEntitlementPlanRepo{})
	entSvc.SetNowFunc(func() time.Time { return now })

	svc := &PaymentService{entClient: client, subscriptionEntitlementSvc: entSvc}
	plan := &RefundPlan{
		OrderID:         9876,
		Order:           &dbent.PaymentOrder{UserID: user.ID, OrderType: payment.OrderTypeSubscription},
		DeductionType:   payment.DeductionTypeSubscription,
		EntitlementID:   entitlementID,
		SubDaysToDeduct: 10,
	}

	require.NoError(t, svc.applyRefundFinalDeduction(ctx, plan))
	entitlement, err := entRepo.GetByID(ctx, entitlementID)
	require.NoError(t, err)
	require.WithinDuration(t, originalExpiry.AddDate(0, 0, -10), entitlement.ExpiresAt, time.Second)
}

func TestPrepSubscriptionDeductExplicitV2OrderNeverFallsBackToLegacySubscription(t *testing.T) {
	now := time.Now()
	const (
		userID        = int64(41)
		groupID       = int64(73)
		entitlementID = int64(91)
	)
	entRepo := newFakeSubscriptionEntitlementRepo(now)
	entRepo.entitlements[entitlementID] = &SubscriptionEntitlement{
		ID:        entitlementID,
		UserID:    userID,
		Status:    SubscriptionStatusExpired,
		StartsAt:  now.Add(-48 * time.Hour),
		ExpiresAt: now.Add(-24 * time.Hour),
	}
	legacyRepo := newRefundRollbackSubscriptionRepoStub()
	legacyRepo.seed(&UserSubscription{
		ID:        19,
		UserID:    userID,
		GroupID:   groupID,
		Status:    SubscriptionStatusActive,
		StartsAt:  now.Add(-24 * time.Hour),
		ExpiresAt: now.Add(30 * 24 * time.Hour),
	})
	svc := &PaymentService{
		subscriptionEntitlementSvc: NewSubscriptionEntitlementService(entRepo, nil),
		subscriptionSvc:            NewSubscriptionService(nil, legacyRepo, nil, nil, nil),
	}
	order := &dbent.PaymentOrder{
		UserID:                    userID,
		OrderType:                 payment.OrderTypeSubscription,
		SubscriptionGroupID:       int64ValuePtr(groupID),
		SubscriptionEntitlementID: int64ValuePtr(entitlementID),
		ProviderSnapshot: map[string]any{
			paymentOrderSnapshotEntitlementV2Enabled: true,
		},
	}
	plan := &RefundPlan{}

	result := svc.prepSubscriptionDeduct(context.Background(), order, plan, true, 10)

	require.Nil(t, result)
	require.Zero(t, plan.SubscriptionID)
	require.Nil(t, plan.SubscriptionSnapshot)

	nonForcePlan := &RefundPlan{}
	result = svc.prepSubscriptionDeduct(context.Background(), order, nonForcePlan, false, 10)
	require.NotNil(t, result)
	require.True(t, result.RequireForce)
	require.Zero(t, nonForcePlan.SubscriptionID)
	require.Nil(t, nonForcePlan.SubscriptionSnapshot)
}

func TestPrepareRefundRejectsPendingRefundOrder(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "prepare-rejects-pending")
	svc := &PaymentService{entClient: client}

	plan, result, err := svc.PrepareRefund(ctx, order.ID, 100, "retry pending", false, true)

	require.Nil(t, plan)
	require.Nil(t, result)
	require.Error(t, err)
	require.Equal(t, "REFUND_PENDING", infraerrors.Reason(err))
}

func TestExecuteRefundDoesNotRestartPendingRefundFromStalePlan(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "execute-rejects-pending")
	svc := &PaymentService{entClient: client}

	result, err := svc.ExecuteRefund(ctx, &RefundPlan{
		OrderID:       order.ID,
		Order:         order,
		RefundAmount:  100,
		GatewayAmount: 100,
		Reason:        "stale retry",
	})

	require.Nil(t, result)
	require.Error(t, err)
	require.Equal(t, "CONFLICT", infraerrors.Reason(err))

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefundPending, reloaded.Status)
}

func TestQueryAndFinalizeRefundDoesNotDeductWhenFinalizeClaimLost(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "query-finalize-claim-lost")

	var deducted float64
	svc := &PaymentService{
		entClient:    client,
		loadBalancer: &captureLoadBalancer{},
		userRepo: &mockUserRepo{deductAvailableBalanceFn: func(ctx context.Context, id int64, amount float64) (float64, error) {
			deducted += amount
			return amount, nil
		}},
	}
	restore := replacePaymentProviderFactoryForTest(t, &refundQueryProviderTestDouble{
		refundResponse: &payment.RefundResponse{RefundID: "rf_test", Status: payment.ProviderStatusSuccess},
		onQuery: func() {
			_, err := client.PaymentOrder.UpdateOneID(order.ID).SetStatus(OrderStatusRefunding).Save(ctx)
			require.NoError(t, err)
		},
	})
	defer restore()

	result, err := svc.QueryAndFinalizeRefund(ctx, order.ID)

	require.Nil(t, result)
	require.Error(t, err)
	require.Equal(t, "REFUND_FINALIZE_CONFLICT", infraerrors.Reason(err))
	require.Zero(t, deducted)

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefunding, reloaded.Status)
}

func TestFinalizePendingRefundSuccessRejectsStaleCallerBeforeSecondDeduction(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "finalize-stale")

	deductions := 0
	svc := &PaymentService{
		entClient: client,
		userRepo: &mockUserRepo{deductAvailableBalanceFn: func(ctx context.Context, id int64, amount float64) (float64, error) {
			require.NotNil(t, dbent.TxFromContext(ctx))
			deductions++
			return amount, nil
		}},
	}

	first, err := svc.finalizePendingRefundSuccess(ctx, svc.refundFinalizePlan(order))
	require.NoError(t, err)
	require.True(t, first.Success)

	second, err := svc.finalizePendingRefundSuccess(ctx, svc.refundFinalizePlan(order))
	require.Nil(t, second)
	require.Error(t, err)
	require.Equal(t, "REFUND_FINALIZE_CONFLICT", infraerrors.Reason(err))
	require.Equal(t, 1, deductions)

	successAudits, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_SUCCESS")).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, successAudits)
}

func TestFinalizePendingRefundSuccessRollsBackPostDeductionFailure(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "finalize-rollback")
	_, err := client.User.UpdateOneID(order.UserID).SetBalance(100).Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{
		entClient: client,
		userRepo: &mockUserRepo{deductAvailableBalanceFn: func(ctx context.Context, id int64, amount float64) (float64, error) {
			tx := dbent.TxFromContext(ctx)
			require.NotNil(t, tx)
			if _, updateErr := tx.Client().User.UpdateOneID(id).AddBalance(-amount).Save(ctx); updateErr != nil {
				return 0, updateErr
			}
			return 0, errors.New("injected failure after deduction")
		}},
	}

	result, err := svc.finalizePendingRefundSuccess(ctx, svc.refundFinalizePlan(order))
	require.Nil(t, result)
	require.ErrorContains(t, err, "injected failure after deduction")

	user, err := client.User.Get(ctx, order.UserID)
	require.NoError(t, err)
	require.Equal(t, 100.0, user.Balance)
	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefundPending, reloaded.Status)
	successAudits, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_SUCCESS")).
		Count(ctx)
	require.NoError(t, err)
	require.Zero(t, successAudits)
}

func TestQueryAndFinalizeRefundUnsupportedProviderReturnsClearError(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "query-finalize-unsupported")
	svc := &PaymentService{entClient: client, loadBalancer: &captureLoadBalancer{}}
	restore := replacePaymentProviderFactoryForTest(t, refundProviderTestDouble{})
	defer restore()

	result, err := svc.QueryAndFinalizeRefund(ctx, order.ID)
	require.Nil(t, result)
	require.Error(t, err)
	require.Equal(t, "REFUND_QUERY_UNSUPPORTED", infraerrors.Reason(err))
}

func TestMarkRefundOkExistingTxRollsBackPartialReferralSyncBeforeWarning(t *testing.T) {
	ctx := context.Background()
	client := newPaymentServiceEntClient(t)
	user := createPaymentRefundTestUser(t, ctx, client, 0)
	order := createPaymentOrderForRefundTest(
		t,
		ctx,
		client,
		user,
		50,
		50,
		0,
		OrderStatusRefunding,
		"mark_refund_ok_savepoint",
		"trade_mark_refund_ok_savepoint",
	)
	recharge, err := client.RechargeOrder.Create().
		SetUserID(user.ID).
		SetExternalOrderID(order.OutTradeNo).
		SetProvider(payment.TypeStripe).
		SetCurrency(ReferralSettlementCurrencyCNY).
		SetPaidAmount(50).
		SetStatus(RechargeOrderStatusCredited).
		Save(ctx)
	require.NoError(t, err)

	rechargeRepo := &txAwareRefundRechargeRepo{client: client, orderID: recharge.ID}
	commissionRepo := &failingRefundCommissionRepo{
		commissionRepoStub: &commissionRepoStub{},
		err:                errors.New("reward lookup failed after recharge update"),
	}
	svc := &PaymentService{
		entClient: client,
		referralRefundSvc: NewReferralRefundService(
			rechargeRepo,
			commissionRepo,
			nil,
			nil,
		),
	}

	tx, err := client.Tx(ctx)
	require.NoError(t, err)
	txCtx := dbent.NewTxContext(ctx, tx)
	result, err := svc.markRefundOk(txCtx, &RefundPlan{
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
	require.NoError(t, tx.Commit())

	updatedOrder, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefunded, updatedOrder.Status)

	updatedRecharge, err := client.RechargeOrder.Get(ctx, recharge.ID)
	require.NoError(t, err)
	require.Equal(t, RechargeOrderStatusCredited, updatedRecharge.Status)
	require.Zero(t, updatedRecharge.RefundedAmount)
	require.Nil(t, updatedRecharge.RefundedAt)
	requireAuditActionsForOrder(t, ctx, client, order.ID, "REFUND_SUCCESS", "REFUND_REFERRAL_SYNC_FAILED")
}

func TestMarkRefundOkIsolatesReferralAndAffiliateSyncFailures(t *testing.T) {
	tests := []struct {
		name            string
		referralErr     error
		affiliateErr    error
		expectedActions []string
		expectedWarning []string
	}{
		{
			name:            "referral failure does not roll back affiliate",
			referralErr:     errors.New("referral unavailable"),
			expectedActions: []string{paymentAuditActionReferralRefundSyncFailed},
			expectedWarning: []string{"referral sync failed"},
		},
		{
			name:            "affiliate failure does not roll back referral",
			affiliateErr:    errors.New("affiliate unavailable"),
			expectedActions: []string{paymentAuditActionAffiliateRefundSyncFailed},
			expectedWarning: []string{"affiliate sync failed"},
		},
		{
			name:            "both failures are recorded",
			referralErr:     errors.New("referral unavailable"),
			affiliateErr:    errors.New("affiliate unavailable"),
			expectedActions: []string{paymentAuditActionReferralRefundSyncFailed, paymentAuditActionAffiliateRefundSyncFailed},
			expectedWarning: []string{"referral sync failed", "affiliate sync failed"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			client := newPaymentServiceEntClient(t)
			ensurePaymentAuditOrderActionUniqueIndex(t, ctx, client)
			user := createPaymentRefundTestUser(t, ctx, client, 0)
			order := createPaymentOrderForRefundTest(
				t,
				ctx,
				client,
				user,
				100,
				100,
				0,
				OrderStatusRefunding,
				"isolated_reward_sync_"+strconv.FormatInt(time.Now().UnixNano(), 10),
				"trade_isolated_reward_sync_"+strconv.FormatInt(time.Now().UnixNano(), 10),
			)
			_, err := client.PaymentAuditLog.Create().
				SetOrderID(strconv.FormatInt(order.ID, 10)).
				SetAction("AFFILIATE_REBATE_APPLIED").
				SetDetail(`{"rebateAmount":10}`).
				SetOperator("system").
				Save(ctx)
			require.NoError(t, err)

			affiliateRepo := &paymentFulfillmentAffiliateRepoStub{reverseErr: tt.affiliateErr}
			svc := &PaymentService{
				entClient:                client,
				affiliateService:         NewAffiliateService(affiliateRepo, nil, nil, nil),
				affiliateReversalEnabled: true,
			}
			if tt.referralErr != nil {
				svc.referralRefundSvc = NewReferralRefundService(
					&failingRechargeOrderRepoStub{rechargeOrderRepoStub: newRechargeOrderRepoStub(), err: tt.referralErr},
					&commissionRepoStub{},
					nil,
					nil,
				)
			}

			result, err := svc.markRefundOk(ctx, &RefundPlan{
				OrderID:       order.ID,
				Order:         order,
				RefundAmount:  100,
				GatewayAmount: 100,
				Reason:        "gateway success",
			})
			require.NoError(t, err)
			require.True(t, result.Success)
			for _, warning := range tt.expectedWarning {
				require.Contains(t, result.Warning, warning)
			}
			require.Len(t, affiliateRepo.reverseCalls, 1, "affiliate sync must be attempted independently")
			require.InDelta(t, 1, affiliateRepo.reverseCalls[0].cumulativeRefundRatio, 1e-9)
			requireAuditActionsForOrder(t, ctx, client, order.ID, append([]string{"REFUND_SUCCESS"}, tt.expectedActions...)...)

			for _, action := range []string{paymentAuditActionReferralRefundSyncFailed, paymentAuditActionAffiliateRefundSyncFailed} {
				count, countErr := client.PaymentAuditLog.Query().
					Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ(action)).
					Count(ctx)
				require.NoError(t, countErr)
				require.Equal(t, boolToInt(containsExpectedAuditAction(tt.expectedActions, action)), count)
			}
		})
	}
}

func TestRetryFailedRefundRewardSyncsIsIdempotentAcrossFailureCycles(t *testing.T) {
	ctx := context.Background()
	client := newPaymentServiceEntClient(t)
	ensurePaymentAuditOrderActionUniqueIndex(t, ctx, client)
	user := createPaymentRefundTestUser(t, ctx, client, 0)
	order := createPaymentOrderForRefundTest(
		t,
		ctx,
		client,
		user,
		100,
		100,
		50,
		OrderStatusPartiallyRefunded,
		"affiliate_recovery",
		"trade_affiliate_recovery",
	)
	for _, action := range []string{"AFFILIATE_REBATE_APPLIED", paymentAuditActionAffiliateRefundSyncFailed} {
		_, err := client.PaymentAuditLog.Create().
			SetOrderID(strconv.FormatInt(order.ID, 10)).
			SetAction(action).
			SetDetail(`{"test":true}`).
			SetOperator("system").
			Save(ctx)
		require.NoError(t, err)
	}

	affiliateRepo := &paymentFulfillmentAffiliateRepoStub{}
	svc := &PaymentService{
		entClient:                client,
		affiliateService:         NewAffiliateService(affiliateRepo, nil, nil, nil),
		affiliateReversalEnabled: true,
	}

	recovered, err := svc.RetryFailedRefundRewardSyncs(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, 1, recovered)
	require.Len(t, affiliateRepo.reverseCalls, 1)
	require.InDelta(t, 0.5, affiliateRepo.reverseCalls[0].cumulativeRefundRatio, 1e-9)
	requireAuditActionsForOrder(t, ctx, client, order.ID, paymentAuditActionAffiliateRefundSyncRecovered)
	requirePaymentAuditActionCount(t, ctx, client, order.ID, paymentAuditActionAffiliateRefundSyncFailed, 0)

	recovered, err = svc.RetryFailedRefundRewardSyncs(ctx, 10)
	require.NoError(t, err)
	require.Zero(t, recovered)
	require.Len(t, affiliateRepo.reverseCalls, 1, "recovery without a failed marker must be a no-op")

	_, err = client.PaymentOrder.UpdateOneID(order.ID).SetRefundAmount(75).Save(ctx)
	require.NoError(t, err)
	_, err = client.PaymentAuditLog.Create().
		SetOrderID(strconv.FormatInt(order.ID, 10)).
		SetAction(paymentAuditActionAffiliateRefundSyncFailed).
		SetDetail(`{"test":"second cycle"}`).
		SetOperator("system").
		Save(ctx)
	require.NoError(t, err)

	recovered, err = svc.RetryFailedRefundRewardSyncs(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, 1, recovered)
	require.Len(t, affiliateRepo.reverseCalls, 2)
	require.InDelta(t, 0.75, affiliateRepo.reverseCalls[1].cumulativeRefundRatio, 1e-9)
	requirePaymentAuditActionCount(t, ctx, client, order.ID, paymentAuditActionAffiliateRefundSyncFailed, 0)
	requirePaymentAuditActionCount(t, ctx, client, order.ID, paymentAuditActionAffiliateRefundSyncRecovered, 1)
}

func TestRetryFailedRefundRewardSyncsDefersAffiliateWhileGateDisabled(t *testing.T) {
	ctx := context.Background()
	client := newPaymentServiceEntClient(t)
	ensurePaymentAuditOrderActionUniqueIndex(t, ctx, client)
	user := createPaymentRefundTestUser(t, ctx, client, 0)
	order := createPaymentOrderForRefundTest(
		t,
		ctx,
		client,
		user,
		100,
		100,
		50,
		OrderStatusPartiallyRefunded,
		"deferred_affiliate_recovery",
		"trade_deferred_affiliate_recovery",
	)
	for _, action := range []string{
		"AFFILIATE_REBATE_APPLIED",
		paymentAuditActionAffiliateRefundSyncFailed,
		paymentAuditActionReferralRefundSyncFailed,
	} {
		_, err := client.PaymentAuditLog.Create().
			SetOrderID(strconv.FormatInt(order.ID, 10)).
			SetAction(action).
			SetDetail(`{"test":true}`).
			SetOperator("system").
			Save(ctx)
		require.NoError(t, err)
	}

	rechargeRepo := newRechargeOrderRepoStub()
	rechargeOrder := &RechargeOrder{
		UserID:          order.UserID,
		ExternalOrderID: order.OutTradeNo,
		Provider:        payment.TypeStripe,
		Currency:        ReferralSettlementCurrencyCNY,
		PaidAmount:      100,
		Status:          RechargeOrderStatusPaid,
	}
	require.NoError(t, rechargeRepo.Create(ctx, rechargeOrder))

	affiliateRepo := &paymentFulfillmentAffiliateRepoStub{}
	svc := &PaymentService{
		entClient:         client,
		referralRefundSvc: NewReferralRefundService(rechargeRepo, &commissionRepoStub{}, nil, nil),
		affiliateService:  NewAffiliateService(affiliateRepo, nil, nil, nil),
		// The zero value intentionally represents the first deployment phase.
	}

	recovered, err := svc.RetryFailedRefundRewardSyncs(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, 1, recovered)
	require.Empty(t, affiliateRepo.reverseCalls)
	requirePaymentAuditActionCount(t, ctx, client, order.ID, paymentAuditActionReferralRefundSyncFailed, 0)
	requirePaymentAuditActionCount(t, ctx, client, order.ID, paymentAuditActionReferralRefundSyncRecovered, 1)
	requirePaymentAuditActionCount(t, ctx, client, order.ID, paymentAuditActionAffiliateRefundSyncFailed, 1)
	requirePaymentAuditActionCount(t, ctx, client, order.ID, paymentAuditActionAffiliateRefundSyncRecovered, 0)

	recovered, err = svc.RetryFailedRefundRewardSyncs(ctx, 10)
	require.NoError(t, err)
	require.Zero(t, recovered, "disabled affiliate markers must not be retried repeatedly")
	require.Empty(t, affiliateRepo.reverseCalls)

	svc.affiliateReversalEnabled = true
	recovered, err = svc.RetryFailedRefundRewardSyncs(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, 1, recovered)
	require.Len(t, affiliateRepo.reverseCalls, 1)
	requirePaymentAuditActionCount(t, ctx, client, order.ID, paymentAuditActionAffiliateRefundSyncFailed, 0)
	requirePaymentAuditActionCount(t, ctx, client, order.ID, paymentAuditActionAffiliateRefundSyncRecovered, 1)
}

func TestRetryFailedRefundRewardSyncsRotatesPastPoisonScanWindow(t *testing.T) {
	ctx := context.Background()
	client := newPaymentServiceEntClient(t)
	ensurePaymentAuditOrderActionUniqueIndex(t, ctx, client)
	user := createPaymentRefundTestUser(t, ctx, client, 0)

	for i := 0; i < defaultFailedRefundSyncRecoveryLimit; i++ {
		order := createPaymentOrderForRefundTest(
			t,
			ctx,
			client,
			user,
			100,
			100,
			50,
			OrderStatusCompleted,
			fmt.Sprintf("poison_refund_recovery_%d", i),
			fmt.Sprintf("trade_poison_refund_recovery_%d", i),
		)
		_, err := client.PaymentAuditLog.Create().
			SetOrderID(strconv.FormatInt(order.ID, 10)).
			SetAction(paymentAuditActionAffiliateRefundSyncFailed).
			SetDetail(`{"test":"poison"}`).
			SetOperator("system").
			Save(ctx)
		require.NoError(t, err)
	}

	recoverable := createPaymentOrderForRefundTest(
		t,
		ctx,
		client,
		user,
		100,
		100,
		50,
		OrderStatusPartiallyRefunded,
		"recoverable_after_poison_window",
		"trade_recoverable_after_poison_window",
	)
	for _, action := range []string{"AFFILIATE_REBATE_APPLIED", paymentAuditActionAffiliateRefundSyncFailed} {
		_, err := client.PaymentAuditLog.Create().
			SetOrderID(strconv.FormatInt(recoverable.ID, 10)).
			SetAction(action).
			SetDetail(`{"test":"recoverable"}`).
			SetOperator("system").
			Save(ctx)
		require.NoError(t, err)
	}

	affiliateRepo := &paymentFulfillmentAffiliateRepoStub{}
	svc := &PaymentService{
		entClient:                client,
		affiliateService:         NewAffiliateService(affiliateRepo, nil, nil, nil),
		affiliateReversalEnabled: true,
	}

	recovered, err := svc.RetryFailedRefundRewardSyncs(ctx, 1)
	require.NoError(t, err)
	require.Zero(t, recovered)
	require.Empty(t, affiliateRepo.reverseCalls)

	// A replacement process must resume after the persisted poison window rather
	// than starting over from the first failed audit rows.
	restartedSvc := &PaymentService{
		entClient:                client,
		affiliateService:         NewAffiliateService(affiliateRepo, nil, nil, nil),
		affiliateReversalEnabled: true,
	}
	recovered, err = restartedSvc.RetryFailedRefundRewardSyncs(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, 1, recovered)
	require.Len(t, affiliateRepo.reverseCalls, 1)
	require.Equal(t, recoverable.ID, affiliateRepo.reverseCalls[0].sourceOrderID)
}

func TestLoadFailedRefundSyncRecoveryCursorRepairsInvalidValue(t *testing.T) {
	ctx := context.Background()
	client := newPaymentServiceEntClient(t)
	_, err := client.Setting.Create().
		SetKey(failedRefundSyncRecoveryCursorSettingKey).
		SetValue("not-an-audit-id").
		Save(ctx)
	require.NoError(t, err)
	svc := &PaymentService{entClient: client}

	cursor, err := svc.loadFailedRefundSyncRecoveryCursor(ctx)

	require.NoError(t, err)
	require.Zero(t, cursor)
	settingEntry, err := client.Setting.Query().
		Where(setting.KeyEQ(failedRefundSyncRecoveryCursorSettingKey)).
		Only(ctx)
	require.NoError(t, err)
	require.Equal(t, "0", settingEntry.Value)
}

func TestSyncReferralRefundToSettledTotalSubtractsExistingChargeback(t *testing.T) {
	ctx := context.Background()
	rechargeRepo := newRechargeOrderRepoStub()
	commissionRepo := &commissionRepoStub{}
	recharge := &RechargeOrder{
		UserID:           42,
		ExternalOrderID:  "chargeback-cap",
		Provider:         payment.TypeStripe,
		Currency:         ReferralSettlementCurrencyCNY,
		PaidAmount:       100,
		RefundedAmount:   20,
		ChargebackAmount: 30,
		Status:           RechargeOrderStatusChargeback,
	}
	require.NoError(t, rechargeRepo.Create(ctx, recharge))
	svc := &PaymentService{
		referralRefundSvc: NewReferralRefundService(rechargeRepo, commissionRepo, nil, nil),
	}
	order := &dbent.PaymentOrder{
		ID:                   99,
		Amount:               100,
		PayAmount:            100,
		RefundAmount:         80,
		ProviderRefundAmount: 50,
		ChargebackAmount:     30,
		OrderType:            payment.OrderTypeBalance,
		PaymentType:          payment.TypeStripe,
		OutTradeNo:           recharge.ExternalOrderID,
	}

	require.NoError(t, svc.syncReferralRefundToSettledTotal(ctx, order))
	updated, err := rechargeRepo.GetByID(ctx, recharge.ID)
	require.NoError(t, err)
	require.InDelta(t, 50, updated.RefundedAmount, 1e-9)
	require.InDelta(t, 30, updated.ChargebackAmount, 1e-9)

	require.NoError(t, svc.syncReferralRefundToSettledTotal(ctx, order))
	updatedAgain, err := rechargeRepo.GetByID(ctx, recharge.ID)
	require.NoError(t, err)
	require.InDelta(t, 50, updatedAgain.RefundedAmount, 1e-9, "repeated cumulative recovery must be idempotent")

	updatedAgain.PaidAmount = 0
	require.NoError(t, rechargeRepo.Update(ctx, updatedAgain))
	err = svc.syncReferralRefundToSettledTotal(ctx, order)
	require.ErrorContains(t, err, "invalid referral reversal base")
}

func requirePaymentAuditActionCount(t *testing.T, ctx context.Context, client *dbent.Client, orderID int64, action string, expected int) {
	t.Helper()
	count, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(orderID, 10)), paymentauditlog.ActionEQ(action)).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, expected, count)
}

func containsExpectedAuditAction(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

type txAwareRefundRechargeRepo struct {
	client  *dbent.Client
	orderID int64
}

func (r *txAwareRefundRechargeRepo) currentClient(ctx context.Context) *dbent.Client {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		return tx.Client()
	}
	return r.client
}

func (r *txAwareRefundRechargeRepo) GetByProviderAndExternalOrderID(ctx context.Context, _, _ string) (*RechargeOrder, error) {
	return r.GetByID(ctx, r.orderID)
}

func (r *txAwareRefundRechargeRepo) GetByID(ctx context.Context, id int64) (*RechargeOrder, error) {
	order, err := r.currentClient(ctx).RechargeOrder.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return &RechargeOrder{
		ID:                    order.ID,
		UserID:                order.UserID,
		ExternalOrderID:       order.ExternalOrderID,
		Provider:              order.Provider,
		Currency:              order.Currency,
		GrossAmount:           order.GrossAmount,
		DiscountAmount:        order.DiscountAmount,
		PaidAmount:            order.PaidAmount,
		GiftBalanceAmount:     order.GiftBalanceAmount,
		CreditedBalanceAmount: order.CreditedBalanceAmount,
		RefundedAmount:        order.RefundedAmount,
		ChargebackAmount:      order.ChargebackAmount,
		Status:                order.Status,
		PaidAt:                order.PaidAt,
		CreditedAt:            order.CreditedAt,
		RefundedAt:            order.RefundedAt,
		ChargebackAt:          order.ChargebackAt,
	}, nil
}

func (r *txAwareRefundRechargeRepo) Create(context.Context, *RechargeOrder) error {
	panic("unexpected Create call")
}

func (r *txAwareRefundRechargeRepo) Update(ctx context.Context, order *RechargeOrder) error {
	update := r.currentClient(ctx).RechargeOrder.UpdateOneID(order.ID).
		SetStatus(order.Status).
		SetRefundedAmount(order.RefundedAmount).
		SetChargebackAmount(order.ChargebackAmount)
	if order.RefundedAt != nil {
		update.SetRefundedAt(*order.RefundedAt)
	} else {
		update.ClearRefundedAt()
	}
	if order.ChargebackAt != nil {
		update.SetChargebackAt(*order.ChargebackAt)
	} else {
		update.ClearChargebackAt()
	}
	_, err := update.Save(ctx)
	return err
}

func (r *txAwareRefundRechargeRepo) CountPaidOrdersByUser(context.Context, int64) (int, error) {
	panic("unexpected CountPaidOrdersByUser call")
}

func (r *txAwareRefundRechargeRepo) HasRefundOrChargeback(context.Context, int64) (bool, error) {
	panic("unexpected HasRefundOrChargeback call")
}

type failingRefundCommissionRepo struct {
	*commissionRepoStub
	err error
}

func (r *failingRefundCommissionRepo) ListRewardsByRechargeOrderForUpdate(context.Context, int64) ([]CommissionReward, error) {
	return nil, r.err
}

func createPendingRefundOrderForTest(t *testing.T, ctx context.Context, client *dbent.Client, suffix string) *dbent.PaymentOrder {
	t.Helper()

	user, err := client.User.Create().
		SetEmail(suffix + "@example.com").
		SetPasswordHash("hash").
		SetUsername(suffix).
		Save(ctx)
	require.NoError(t, err)

	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeStripe).
		SetName(suffix + "-provider").
		SetConfig("{}").
		SetSupportedTypes("stripe").
		SetEnabled(true).
		SetRefundEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(100).
		SetPayAmount(100).
		SetFeeRate(0).
		SetRechargeCode("REFUND-" + suffix).
		SetOutTradeNo("sub2_" + suffix).
		SetPaymentType(payment.TypeStripe).
		SetPaymentTradeNo("pi_" + suffix).
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusRefundPending).
		SetRefundAmount(100).
		SetRefundReason("pending refund").
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetProviderInstanceID(strconv.FormatInt(inst.ID, 10)).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.PaymentAuditLog.Create().
		SetOrderID(strconv.FormatInt(order.ID, 10)).
		SetAction("REFUND_PENDING").
		SetOperator("admin").
		SetDetail(`{"refundID":"rf_test","deductionRollbackOK":true}`).
		Save(ctx)
	require.NoError(t, err)
	return order
}

func replacePaymentProviderFactoryForTest(t *testing.T, prov payment.Provider) func() {
	t.Helper()
	original := createPaymentProviderFromInstance
	createPaymentProviderFromInstance = func(providerKey, instanceID string, config map[string]string) (payment.Provider, error) {
		return prov, nil
	}
	return func() { createPaymentProviderFromInstance = original }
}

type refundProviderTestDouble struct{}

func (refundProviderTestDouble) Name() string { return "refund-test" }
func (refundProviderTestDouble) ProviderKey() string {
	return payment.TypeStripe
}
func (refundProviderTestDouble) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeStripe}
}
func (refundProviderTestDouble) CreatePayment(context.Context, payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	return nil, nil
}
func (refundProviderTestDouble) QueryOrder(context.Context, string) (*payment.QueryOrderResponse, error) {
	return nil, nil
}
func (refundProviderTestDouble) VerifyNotification(context.Context, string, map[string]string) (*payment.PaymentNotification, error) {
	return nil, nil
}
func (refundProviderTestDouble) Refund(context.Context, payment.RefundRequest) (*payment.RefundResponse, error) {
	return nil, nil
}

type refundQueryProviderTestDouble struct {
	refundProviderTestDouble
	refundResponse *payment.RefundResponse
	onQuery        func()
}

func (p *refundQueryProviderTestDouble) QueryRefund(context.Context, payment.RefundQueryRequest) (*payment.RefundResponse, error) {
	if p.onQuery != nil {
		p.onQuery()
	}
	return p.refundResponse, nil
}
