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
	require.Equal(t, 40.0, reloaded.RefundAmount)
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
				userRepo: &mockUserRepo{deductBalanceFn: func(ctx context.Context, id int64, amount float64) error {
					deducted += amount
					return nil
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
		userRepo: &mockUserRepo{deductBalanceFn: func(_ context.Context, _ int64, amount float64) error {
			deducted += amount
			return nil
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
	now := time.Date(2026, 7, 24, 10, 0, 0, 0, time.UTC)
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
		userRepo: &mockUserRepo{deductBalanceFn: func(ctx context.Context, id int64, amount float64) error {
			deducted += amount
			return nil
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
