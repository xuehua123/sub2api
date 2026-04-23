//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

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
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(90).
		SetPayAmount(90).
		SetRechargeCode("sub-partial-refund").
		SetOutTradeNo("subscription_partial_refund_order").
		SetPaymentType(payment.TypeStripe).
		SetPaymentTradeNo("trade_subscription_partial_refund").
		SetOrderType(payment.OrderTypeSubscription).
		SetStatus(OrderStatusCompleted).
		SetSubscriptionGroupID(groupID).
		SetSubscriptionDays(30).
		SetExpiresAt(time.Now().Add(24 * time.Hour)).
		SetPaidAt(paidAt).
		SetClientIP("127.0.0.1").
		SetSrcHost("example.com").
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

	registry := payment.NewRegistry()
	registry.Register(&refundProviderStub{})

	service := &PaymentService{
		entClient:       client,
		registry:        registry,
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

func TestExecuteRefund_FailedGatewayRefundRestoresRevokedSubscription(t *testing.T) {
	ctx := context.Background()
	client := newPaymentServiceEntClient(t)
	user := createPaymentRefundTestUser(t, ctx, client, 0)

	groupID := int64(78)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(90).
		SetPayAmount(90).
		SetRechargeCode("sub-revoke-rollback").
		SetOutTradeNo("subscription_revoke_rollback_order").
		SetPaymentType(payment.TypeStripe).
		SetPaymentTradeNo("trade_subscription_revoke_rollback").
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

	registry := payment.NewRegistry()
	registry.Register(&failingRefundProviderStub{})

	service := &PaymentService{
		entClient:       client,
		registry:        registry,
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
