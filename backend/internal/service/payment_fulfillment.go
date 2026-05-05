package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// ErrOrderNotFound is returned by HandlePaymentNotification when the webhook
// references an out_trade_no that does not exist in our DB. Callers (webhook
// handlers) should treat this as a terminal, non-retryable condition and still
// respond with a 2xx success to the provider — otherwise the provider will keep
// retrying forever (e.g. when a foreign environment's webhook endpoint is
// misconfigured to point at us, or when our orders table has been wiped).
var ErrOrderNotFound = errors.New("payment order not found")

const (
	paymentAuditActionReferralRewardSyncFailed    = "REFERRAL_REWARD_SYNC_FAILED"
	paymentAuditActionReferralRewardSyncRecovered = "REFERRAL_REWARD_SYNC_RECOVERED"
	paymentAuditActionSubscriptionAssigned        = "SUBSCRIPTION_ASSIGNED"
	paymentAuditActionSubscriptionSuccess         = "SUBSCRIPTION_SUCCESS"

	defaultFailedReferralRewardRecoveryLimit = 50
	maxFailedReferralRewardRecoveryScan      = 500
)

// --- Payment Notification & Fulfillment ---

func (s *PaymentService) HandlePaymentNotification(ctx context.Context, n *payment.PaymentNotification, pk string) error {
	if n == nil {
		return nil
	}
	switch n.Status {
	case payment.NotificationStatusSuccess:
		return s.handlePaymentSuccessNotification(ctx, n, pk)
	case payment.NotificationStatusRefunded:
		return s.handleExternalRefundNotification(ctx, n, pk)
	case payment.NotificationStatusChargeback:
		return s.handleChargebackNotification(ctx, n, pk)
	default:
		return nil
	}
}

func (s *PaymentService) handlePaymentSuccessNotification(ctx context.Context, n *payment.PaymentNotification, pk string) error {
	// Look up order by out_trade_no (the external order ID we sent to the provider)
	order, err := s.entClient.PaymentOrder.Query().Where(paymentorder.OutTradeNo(n.OrderID)).Only(ctx)
	if err != nil {
		// Fallback only for true legacy "sub2_N" DB-ID payloads when the
		// current out_trade_no lookup genuinely did not find an order.
		if oid, ok := parseLegacyPaymentOrderID(n.OrderID, err); ok {
			return s.confirmPayment(ctx, oid, n.TradeNo, n.Amount, pk, n.Metadata)
		}
		if dbent.IsNotFound(err) {
			return fmt.Errorf("%w: out_trade_no=%s", ErrOrderNotFound, n.OrderID)
		}
		return fmt.Errorf("lookup order failed for out_trade_no %s: %w", n.OrderID, err)
	}
	return s.confirmPayment(ctx, order.ID, n.TradeNo, n.Amount, pk, n.Metadata)
}

func (s *PaymentService) handleExternalRefundNotification(ctx context.Context, n *payment.PaymentNotification, pk string) error {
	return s.handleExternalRefundOrChargeback(ctx, n, pk, false)
}

func (s *PaymentService) handleChargebackNotification(ctx context.Context, n *payment.PaymentNotification, pk string) error {
	return s.handleExternalRefundOrChargeback(ctx, n, pk, true)
}

func (s *PaymentService) handleExternalRefundOrChargeback(ctx context.Context, n *payment.PaymentNotification, pk string, chargeback bool) error {
	o, err := s.findOrderForRefundNotification(ctx, n)
	if err != nil {
		return err
	}
	if o == nil {
		return nil
	}

	reversalAmount := normalizeExternalReversalAmount(o, n.Amount)
	apply := func(txCtx context.Context) error {
		currentOrder, err := s.getPaymentOrderByID(txCtx, o.ID)
		if err != nil {
			return err
		}
		refundAmountTotal := computeExternalRefundAmountTotal(currentOrder, reversalAmount, n.AmountSemantic)
		creditedDelta := 0.0

		switch currentOrder.OrderType {
		case payment.OrderTypeBalance:
			if err := s.syncExternalReferralReversal(txCtx, currentOrder, reversalAmount, n.AmountSemantic, chargeback); err != nil {
				return err
			}
			creditedDelta, refundAmountTotal = computeExternalCreditedRefund(currentOrder, reversalAmount, n.AmountSemantic)
			if creditedDelta > 0 {
				if err := s.userRepo.DeductBalance(txCtx, currentOrder.UserID, creditedDelta); err != nil {
					return err
				}
			}
		case payment.OrderTypeSubscription:
			if err := s.syncExternalReferralReversal(txCtx, currentOrder, reversalAmount, n.AmountSemantic, chargeback); err != nil {
				return err
			}
			if err := s.syncExternalSubscriptionReversal(txCtx, currentOrder, refundAmountTotal); err != nil {
				return err
			}
		default:
			return nil
		}

		status := OrderStatusRefunded
		if refundAmountTotal < roundMoney(currentOrder.Amount) {
			status = OrderStatusPartiallyRefunded
		}
		now := time.Now()
		if _, err := s.paymentOrderClient(txCtx).PaymentOrder.UpdateOneID(currentOrder.ID).
			SetStatus(status).
			SetRefundAmount(refundAmountTotal).
			SetRefundAt(now).
			Save(txCtx); err != nil {
			return err
		}

		action := "EXTERNAL_REFUND_SYNCED"
		if chargeback {
			action = "EXTERNAL_CHARGEBACK_SYNCED"
		}
		s.writeAuditLog(txCtx, currentOrder.ID, action, pk, map[string]any{
			"gatewayAmount":     reversalAmount,
			"amountSemantic":    n.AmountSemantic,
			"creditedDelta":     creditedDelta,
			"refundAmountTotal": refundAmountTotal,
			"subscriptionDays":  subscriptionDaysRefundDelta(currentOrder, refundAmountTotal),
			"tradeNo":           n.TradeNo,
			"status":            n.Status,
		})
		return nil
	}

	if s.entClient == nil || dbent.TxFromContext(ctx) != nil {
		return apply(ctx)
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := apply(dbent.NewTxContext(ctx, tx)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *PaymentService) findOrderForRefundNotification(ctx context.Context, n *payment.PaymentNotification) (*dbent.PaymentOrder, error) {
	if n == nil {
		return nil, nil
	}
	if orderID := strings.TrimSpace(n.OrderID); orderID != "" {
		o, err := s.entClient.PaymentOrder.Query().Where(paymentorder.OutTradeNo(orderID)).Only(ctx)
		if err == nil {
			return o, nil
		}
		trimmed := strings.TrimPrefix(orderID, orderIDPrefix)
		if oid, parseErr := strconv.ParseInt(trimmed, 10, 64); parseErr == nil {
			return s.entClient.PaymentOrder.Get(ctx, oid)
		}
	}
	if tradeNo := strings.TrimSpace(n.TradeNo); tradeNo != "" {
		o, err := s.entClient.PaymentOrder.Query().Where(paymentorder.PaymentTradeNoEQ(tradeNo)).Only(ctx)
		if err == nil {
			return o, nil
		}
		return nil, fmt.Errorf("refund notification order lookup failed for trade_no %q: %w", tradeNo, err)
	}
	return nil, fmt.Errorf("refund notification missing order identifier")
}

func (s *PaymentService) syncExternalReferralReversal(ctx context.Context, o *dbent.PaymentOrder, amount float64, amountSemantic string, chargeback bool) error {
	if s.referralRefundSvc == nil || o == nil {
		return nil
	}
	if o.OrderType != payment.OrderTypeBalance && o.OrderType != payment.OrderTypeSubscription {
		return nil
	}

	rechargeOrder, err := s.referralRefundSvc.rechargeRepo.GetByProviderAndExternalOrderID(ctx, strings.TrimSpace(paymentReferralProviderKey(o)), strings.TrimSpace(o.OutTradeNo))
	if err != nil {
		if errors.Is(err, ErrRechargeOrderNotFound) {
			return nil
		}
		return err
	}

	reversalAmount := roundMoney(amount)
	if reversalAmount <= 0 {
		return nil
	}

	refundedAmount := roundMoney(rechargeOrder.RefundedAmount)
	chargebackAmount := roundMoney(rechargeOrder.ChargebackAmount)
	paidAmount := roundMoney(rechargeOrder.PaidAmount)
	if chargeback {
		chargebackAmount = reconcileExternalReversalAmount(chargebackAmount, reversalAmount, paidAmount-refundedAmount, amountSemantic)
	} else {
		refundedAmount = reconcileExternalReversalAmount(refundedAmount, reversalAmount, paidAmount-chargebackAmount, amountSemantic)
	}
	_, _, err = s.referralRefundSvc.ApplyRefund(ctx, &RechargeRefundInput{
		RechargeOrderID:  rechargeOrder.ID,
		RefundedAmount:   refundedAmount,
		ChargebackAmount: chargebackAmount,
	})
	return err
}

func (s *PaymentService) getPaymentOrderByID(ctx context.Context, orderID int64) (*dbent.PaymentOrder, error) {
	return s.paymentOrderClient(ctx).PaymentOrder.Get(ctx, orderID)
}

func (s *PaymentService) paymentOrderClient(ctx context.Context) *dbent.Client {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		return tx.Client()
	}
	return s.entClient
}

func normalizeExternalReversalAmount(o *dbent.PaymentOrder, amount float64) float64 {
	if o == nil {
		return 0
	}
	if amount <= 0 || math.IsNaN(amount) || math.IsInf(amount, 0) {
		amount = o.PayAmount
	}
	if amount <= 0 {
		amount = o.Amount
	}
	return roundMoney(amount)
}

func computeExternalCreditedRefund(o *dbent.PaymentOrder, gatewayAmount float64, amountSemantic string) (float64, float64) {
	if o == nil {
		return 0, 0
	}
	creditedTotal := roundMoney(o.Amount)
	if creditedTotal <= 0 {
		return 0, 0
	}
	paidTotal := roundMoney(o.PayAmount)
	if paidTotal <= 0 {
		paidTotal = creditedTotal
	}
	existingRefundAmount := roundMoney(o.RefundAmount)
	if existingRefundAmount < 0 {
		existingRefundAmount = 0
	}
	if existingRefundAmount > creditedTotal {
		existingRefundAmount = creditedTotal
	}
	if gatewayAmount <= 0 {
		return 0, existingRefundAmount
	}

	targetGatewayTotal := gatewayAmount
	if amountSemantic != payment.NotificationAmountTotal {
		targetGatewayTotal = roundMoney(existingRefundAmount)
		if paidTotal > 0 && math.Abs(paidTotal-creditedTotal) > amountToleranceCNY {
			targetGatewayTotal = roundMoney(existingRefundAmount * paidTotal / creditedTotal)
		}
		targetGatewayTotal = roundMoney(targetGatewayTotal + gatewayAmount)
	}
	if paidTotal > 0 && targetGatewayTotal > paidTotal {
		targetGatewayTotal = paidTotal
	}

	targetCreditedTotal := targetGatewayTotal
	if paidTotal > 0 && math.Abs(paidTotal-creditedTotal) > amountToleranceCNY {
		targetCreditedTotal = targetGatewayTotal * creditedTotal / paidTotal
	}
	targetCreditedTotal = roundMoney(targetCreditedTotal)
	if targetCreditedTotal < 0 {
		targetCreditedTotal = 0
	}
	if targetCreditedTotal > creditedTotal {
		targetCreditedTotal = creditedTotal
	}
	if targetCreditedTotal <= existingRefundAmount {
		return 0, existingRefundAmount
	}
	creditedDelta := roundMoney(targetCreditedTotal - existingRefundAmount)
	return creditedDelta, targetCreditedTotal
}

func computeExternalRefundAmountTotal(o *dbent.PaymentOrder, gatewayAmount float64, amountSemantic string) float64 {
	_, refundAmountTotal := computeExternalCreditedRefund(o, gatewayAmount, amountSemantic)
	return refundAmountTotal
}

func subscriptionRefundBaseAmount(o *dbent.PaymentOrder) float64 {
	if o == nil {
		return 0
	}
	baseAmount := roundMoney(o.Amount)
	if baseAmount > 0 {
		return baseAmount
	}
	return roundMoney(o.PayAmount)
}

func proportionalSubscriptionDays(totalDays int, baseAmount float64, refundAmount float64) int {
	if totalDays <= 0 || baseAmount <= 0 || refundAmount <= 0 {
		return 0
	}
	baseAmount = roundMoney(baseAmount)
	refundAmount = roundMoney(refundAmount)
	if refundAmount > baseAmount {
		refundAmount = baseAmount
	}
	days := int(math.Round(float64(totalDays) * refundAmount / baseAmount))
	if days < 0 {
		return 0
	}
	if days > totalDays {
		return totalDays
	}
	return days
}

func subscriptionDaysRefundDelta(o *dbent.PaymentOrder, refundAmountTotal float64) int {
	if o == nil || o.SubscriptionDays == nil || *o.SubscriptionDays <= 0 {
		return 0
	}
	baseAmount := subscriptionRefundBaseAmount(o)
	targetDays := proportionalSubscriptionDays(*o.SubscriptionDays, baseAmount, refundAmountTotal)
	existingDays := proportionalSubscriptionDays(*o.SubscriptionDays, baseAmount, roundMoney(o.RefundAmount))
	if targetDays <= existingDays {
		return 0
	}
	return targetDays - existingDays
}

func (s *PaymentService) syncExternalSubscriptionReversal(ctx context.Context, o *dbent.PaymentOrder, refundAmountTotal float64) error {
	if s.subscriptionSvc == nil || o == nil || o.OrderType != payment.OrderTypeSubscription {
		return nil
	}
	if o.SubscriptionGroupID == nil || o.SubscriptionDays == nil || *o.SubscriptionDays <= 0 {
		return nil
	}
	deltaDays := subscriptionDaysRefundDelta(o, refundAmountTotal)
	if deltaDays <= 0 {
		return nil
	}

	sub, err := s.subscriptionSvc.GetActiveSubscription(ctx, o.UserID, *o.SubscriptionGroupID)
	if err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			return nil
		}
		return err
	}
	if sub == nil {
		return nil
	}

	if _, err := s.subscriptionSvc.ExtendSubscription(ctx, sub.ID, -deltaDays); err != nil {
		if errors.Is(err, ErrAdjustWouldExpire) {
			return s.subscriptionSvc.RevokeSubscription(ctx, sub.ID)
		}
		if errors.Is(err, ErrSubscriptionNotFound) {
			return nil
		}
		return err
	}
	return nil
}

func reconcileExternalReversalAmount(existingAmount float64, notificationAmount float64, maxTotalAmount float64, amountSemantic string) float64 {
	existingAmount = roundMoney(existingAmount)
	notificationAmount = roundMoney(notificationAmount)
	maxTotalAmount = roundMoney(maxTotalAmount)
	if existingAmount < 0 {
		existingAmount = 0
	}
	if maxTotalAmount < 0 {
		maxTotalAmount = 0
	}
	if notificationAmount <= 0 {
		if existingAmount > maxTotalAmount {
			return maxTotalAmount
		}
		return existingAmount
	}

	if amountSemantic == payment.NotificationAmountTotal {
		if notificationAmount > maxTotalAmount {
			notificationAmount = maxTotalAmount
		}
		return notificationAmount
	}
	return accumulateExternalReversalAmount(existingAmount, notificationAmount, maxTotalAmount)
}

func accumulateExternalReversalAmount(existingAmount float64, deltaAmount float64, maxTotalAmount float64) float64 {
	existingAmount = roundMoney(existingAmount)
	deltaAmount = roundMoney(deltaAmount)
	maxTotalAmount = roundMoney(maxTotalAmount)
	if existingAmount < 0 {
		existingAmount = 0
	}
	if maxTotalAmount < 0 {
		maxTotalAmount = 0
	}
	if existingAmount >= maxTotalAmount || deltaAmount <= 0 {
		if existingAmount > maxTotalAmount {
			return maxTotalAmount
		}
		return existingAmount
	}
	remainingAmount := roundMoney(maxTotalAmount - existingAmount)
	if deltaAmount > remainingAmount {
		deltaAmount = remainingAmount
	}
	return roundMoney(existingAmount + deltaAmount)
}

func parseLegacyPaymentOrderID(orderID string, lookupErr error) (int64, bool) {
	if !dbent.IsNotFound(lookupErr) {
		return 0, false
	}
	orderID = strings.TrimSpace(orderID)
	if !strings.HasPrefix(orderID, orderIDPrefix) {
		return 0, false
	}
	trimmed := strings.TrimPrefix(orderID, orderIDPrefix)
	if trimmed == "" || trimmed == orderID {
		return 0, false
	}
	oid, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil || oid <= 0 {
		return 0, false
	}
	return oid, true
}

func (s *PaymentService) confirmPayment(ctx context.Context, oid int64, tradeNo string, paid float64, pk string, metadata map[string]string) error {
	o, err := s.entClient.PaymentOrder.Get(ctx, oid)
	if err != nil {
		slog.Error("order not found", "orderID", oid)
		return nil
	}
	instanceProviderKey := ""
	if inst, instErr := s.getOrderProviderInstance(ctx, o); instErr == nil && inst != nil {
		instanceProviderKey = inst.ProviderKey
	}
	expectedProviderKey := expectedNotificationProviderKeyForOrder(s.registry, o, instanceProviderKey)
	if expectedProviderKey != "" && strings.TrimSpace(pk) != "" && !strings.EqualFold(expectedProviderKey, strings.TrimSpace(pk)) {
		s.writeAuditLog(ctx, o.ID, "PAYMENT_PROVIDER_MISMATCH", pk, map[string]any{
			"expectedProvider": expectedProviderKey,
			"actualProvider":   pk,
			"tradeNo":          tradeNo,
		})
		return fmt.Errorf("provider mismatch: expected %s, got %s", expectedProviderKey, pk)
	}
	if err := validateProviderNotificationMetadata(o, pk, metadata); err != nil {
		s.writeAuditLog(ctx, o.ID, "PAYMENT_PROVIDER_METADATA_MISMATCH", pk, map[string]any{
			"detail":  err.Error(),
			"tradeNo": tradeNo,
		})
		return err
	}
	if !isValidProviderAmount(paid) {
		s.writeAuditLog(ctx, o.ID, "PAYMENT_INVALID_AMOUNT", pk, map[string]any{
			"expected": o.PayAmount,
			"paid":     paid,
			"tradeNo":  tradeNo,
		})
		return fmt.Errorf("invalid paid amount from provider: %v", paid)
	}
	if math.Abs(paid-o.PayAmount) > amountToleranceCNY {
		s.writeAuditLog(ctx, o.ID, "PAYMENT_AMOUNT_MISMATCH", pk, map[string]any{"expected": o.PayAmount, "paid": paid, "tradeNo": tradeNo})
		return fmt.Errorf("amount mismatch: expected %.2f, got %.2f", o.PayAmount, paid)
	}
	return s.toPaid(ctx, o, tradeNo, paid, pk)
}

func isValidProviderAmount(amount float64) bool {
	return amount > 0 && !math.IsNaN(amount) && !math.IsInf(amount, 0)
}

func validateProviderNotificationMetadata(order *dbent.PaymentOrder, providerKey string, metadata map[string]string) error {
	return validateProviderSnapshotMetadata(order, providerKey, metadata)
}

func expectedNotificationProviderKey(registry *payment.Registry, orderPaymentType string, orderProviderKey string, instanceProviderKey string) string {
	if key := strings.TrimSpace(instanceProviderKey); key != "" {
		return key
	}
	if key := strings.TrimSpace(orderProviderKey); key != "" {
		return key
	}
	if registry != nil {
		if key := strings.TrimSpace(registry.GetProviderKey(payment.PaymentType(orderPaymentType))); key != "" {
			return key
		}
	}
	return strings.TrimSpace(orderPaymentType)
}

func (s *PaymentService) toPaid(ctx context.Context, o *dbent.PaymentOrder, tradeNo string, paid float64, pk string) error {
	previousStatus := o.Status
	now := time.Now()
	grace := now.Add(-paymentGraceMinutes * time.Minute)
	c, err := s.entClient.PaymentOrder.Update().Where(
		paymentorder.IDEQ(o.ID),
		paymentorder.Or(
			paymentorder.StatusEQ(OrderStatusPending),
			paymentorder.StatusEQ(OrderStatusCancelled),
			paymentorder.And(
				paymentorder.StatusEQ(OrderStatusExpired),
				paymentorder.UpdatedAtGTE(grace),
			),
		),
	).SetStatus(OrderStatusPaid).SetPayAmount(paid).SetPaymentTradeNo(tradeNo).SetPaidAt(now).ClearFailedAt().ClearFailedReason().Save(ctx)
	if err != nil {
		return fmt.Errorf("update to PAID: %w", err)
	}
	if c == 0 {
		return s.alreadyProcessed(ctx, o)
	}
	if previousStatus == OrderStatusCancelled || previousStatus == OrderStatusExpired {
		slog.Info("order recovered from webhook payment success",
			"orderID", o.ID,
			"previousStatus", previousStatus,
			"tradeNo", tradeNo,
			"provider", pk,
		)
		s.writeAuditLog(ctx, o.ID, "ORDER_RECOVERED", pk, map[string]any{
			"previous_status": previousStatus,
			"tradeNo":         tradeNo,
			"paidAmount":      paid,
			"reason":          "webhook payment success received after order " + previousStatus,
		})
	}
	s.writeAuditLog(ctx, o.ID, "ORDER_PAID", pk, map[string]any{"tradeNo": tradeNo, "paidAmount": paid})
	return s.executeFulfillment(ctx, o.ID)
}

func (s *PaymentService) alreadyProcessed(ctx context.Context, o *dbent.PaymentOrder) error {
	cur, err := s.entClient.PaymentOrder.Get(ctx, o.ID)
	if err != nil {
		return nil
	}
	switch cur.Status {
	case OrderStatusCompleted, OrderStatusRefunded:
		return nil
	case OrderStatusFailed:
		return s.executeFulfillment(ctx, o.ID)
	case OrderStatusPaid, OrderStatusRecharging:
		return fmt.Errorf("order %d is being processed", o.ID)
	case OrderStatusExpired:
		slog.Warn("webhook payment success for expired order beyond grace period",
			"orderID", o.ID,
			"status", cur.Status,
			"updatedAt", cur.UpdatedAt,
		)
		s.writeAuditLog(ctx, o.ID, "PAYMENT_AFTER_EXPIRY", "system", map[string]any{
			"status":    cur.Status,
			"updatedAt": cur.UpdatedAt,
			"reason":    "payment arrived after expiry grace period",
		})
		return nil
	default:
		return nil
	}
}

func (s *PaymentService) executeFulfillment(ctx context.Context, oid int64) error {
	o, err := s.entClient.PaymentOrder.Get(ctx, oid)
	if err != nil {
		return fmt.Errorf("get order: %w", err)
	}
	if o.OrderType == payment.OrderTypeSubscription {
		return s.ExecuteSubscriptionFulfillment(ctx, oid)
	}
	return s.ExecuteBalanceFulfillment(ctx, oid)
}

func (s *PaymentService) ExecuteBalanceFulfillment(ctx context.Context, oid int64) error {
	o, err := s.entClient.PaymentOrder.Get(ctx, oid)
	if err != nil {
		return infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if o.Status == OrderStatusCompleted {
		return nil
	}
	if psIsRefundStatus(o.Status) {
		return infraerrors.BadRequest("INVALID_STATUS", "refund-related order cannot fulfill")
	}
	if o.Status != OrderStatusPaid && o.Status != OrderStatusFailed {
		return infraerrors.BadRequest("INVALID_STATUS", "order cannot fulfill in status "+o.Status)
	}
	c, err := s.entClient.PaymentOrder.Update().Where(paymentorder.IDEQ(oid), paymentorder.StatusIn(OrderStatusPaid, OrderStatusFailed)).SetStatus(OrderStatusRecharging).Save(ctx)
	if err != nil {
		return fmt.Errorf("lock: %w", err)
	}
	if c == 0 {
		return nil
	}
	if err := s.doBalance(ctx, o); err != nil {
		s.markFailed(ctx, oid, err)
		return err
	}
	return nil
}

// redeemAction represents the idempotency decision for balance fulfillment.
type redeemAction int

const (
	// redeemActionCreate: code does not exist — create it, then redeem.
	redeemActionCreate redeemAction = iota
	// redeemActionRedeem: code exists but is unused — skip creation, redeem only.
	redeemActionRedeem
	// redeemActionSkipCompleted: code exists and is already used — skip to mark completed.
	redeemActionSkipCompleted
)

// resolveRedeemAction decides the idempotency action based on an existing redeem code lookup.
// existing is the result of GetByCode; lookupErr is the error from that call.
func resolveRedeemAction(existing *RedeemCode, lookupErr error) redeemAction {
	if existing == nil || lookupErr != nil {
		return redeemActionCreate
	}
	if existing.IsUsed() {
		return redeemActionSkipCompleted
	}
	return redeemActionRedeem
}

func (s *PaymentService) doBalance(ctx context.Context, o *dbent.PaymentOrder) error {
	// Idempotency: check if redeem code already exists (from a previous partial run)
	existing, lookupErr := s.redeemService.GetByCode(ctx, o.RechargeCode)
	action := resolveRedeemAction(existing, lookupErr)

	switch action {
	case redeemActionSkipCompleted:
		if err := s.applyAffiliateRebateForOrder(ctx, o); err != nil {
			return err
		}
		// Code already created and redeemed — just mark completed
		return s.markCompleted(ctx, o, "RECHARGE_SUCCESS")
	case redeemActionCreate:
		rc := &RedeemCode{Code: o.RechargeCode, Type: RedeemTypeBalance, Value: o.Amount, Status: StatusUnused}
		if err := s.redeemService.CreateCode(ctx, rc); err != nil {
			return fmt.Errorf("create redeem code: %w", err)
		}
	case redeemActionRedeem:
		// Code exists but unused — skip creation, proceed to redeem
	}
	if _, err := s.redeemService.Redeem(ctx, o.UserID, o.RechargeCode); err != nil {
		return fmt.Errorf("redeem balance: %w", err)
	}
	if err := s.applyAffiliateRebateForOrder(ctx, o); err != nil {
		return err
	}
	return s.markCompleted(ctx, o, "RECHARGE_SUCCESS")
}

func (s *PaymentService) markCompleted(ctx context.Context, o *dbent.PaymentOrder, auditAction string) error {
	var referralErr error
	if o != nil {
		referralErr = s.syncReferralReward(ctx, o)
		if referralErr != nil && o.OrderType != payment.OrderTypeSubscription {
			return fmt.Errorf("sync referral reward: %w", referralErr)
		}
	}

	now := time.Now()
	_, err := s.entClient.PaymentOrder.Update().Where(paymentorder.IDEQ(o.ID), paymentorder.StatusEQ(OrderStatusRecharging)).SetStatus(OrderStatusCompleted).SetCompletedAt(now).Save(ctx)
	if err != nil {
		return fmt.Errorf("mark completed: %w", err)
	}
	if referralErr != nil {
		s.writeAuditLog(ctx, o.ID, paymentAuditActionReferralRewardSyncFailed, "system", map[string]any{
			"detail": referralErr.Error(),
		})
	}
	s.writeAuditLog(ctx, o.ID, auditAction, "system", map[string]any{
		"rechargeCode":   o.RechargeCode,
		"creditedAmount": o.Amount,
		"payAmount":      o.PayAmount,
	})
	return nil
}

func (s *PaymentService) syncReferralReward(ctx context.Context, o *dbent.PaymentOrder) error {
	if s.referralRewardSvc == nil || o == nil {
		return nil
	}

	switch o.OrderType {
	case payment.OrderTypeBalance:
		return s.syncBalanceReferralReward(ctx, o)
	case payment.OrderTypeSubscription:
		return s.syncSubscriptionReferralReward(ctx, o)
	default:
		return nil
	}
}

func paymentReferralProviderKey(o *dbent.PaymentOrder) string {
	if o == nil {
		return ""
	}
	providerKey := payment.GetBasePaymentType(o.PaymentType)
	if providerKey == "" {
		providerKey = o.PaymentType
	}
	return providerKey
}

func (s *PaymentService) syncBalanceReferralReward(ctx context.Context, o *dbent.PaymentOrder) error {
	paidAmount := roundMoney(o.PayAmount)
	if paidAmount <= 0 {
		paidAmount = roundMoney(o.Amount)
	}
	creditedBalanceAmount := roundMoney(o.Amount)
	giftBalanceAmount := 0.0
	grossAmount := creditedBalanceAmount
	discountAmount := 0.0
	if creditedBalanceAmount > paidAmount {
		giftBalanceAmount = roundMoney(creditedBalanceAmount - paidAmount)
		discountAmount = giftBalanceAmount
	} else if paidAmount > creditedBalanceAmount {
		grossAmount = paidAmount
	}

	_, err := s.referralRewardSvc.CreditRechargeOrder(ctx, &RechargeCreditInput{
		UserID:                o.UserID,
		ExternalOrderID:       o.OutTradeNo,
		Provider:              paymentReferralProviderKey(o),
		Channel:               o.PaymentType,
		Currency:              ReferralSettlementCurrencyCNY,
		GrossAmount:           grossAmount,
		DiscountAmount:        discountAmount,
		PaidAmount:            paidAmount,
		GiftBalanceAmount:     giftBalanceAmount,
		CreditedBalanceAmount: creditedBalanceAmount,
		SkipBalanceCredit:     true,
		PaidAt:                o.PaidAt,
		Notes:                 fmt.Sprintf("payment order %d", o.ID),
	})
	return err
}

func (s *PaymentService) syncSubscriptionReferralReward(ctx context.Context, o *dbent.PaymentOrder) error {
	paidAmount := roundMoney(o.PayAmount)
	if paidAmount <= 0 {
		paidAmount = roundMoney(o.Amount)
	}
	if paidAmount <= 0 {
		return nil
	}

	grossAmount := roundMoney(o.Amount)
	if grossAmount <= 0 || grossAmount < paidAmount {
		grossAmount = paidAmount
	}

	metadata := map[string]any{
		"order_type":       payment.OrderTypeSubscription,
		"payment_order_id": o.ID,
		"payment_trade_no": o.PaymentTradeNo,
	}
	if o.PlanID != nil {
		metadata["plan_id"] = *o.PlanID
	}
	if o.SubscriptionGroupID != nil {
		metadata["subscription_group_id"] = *o.SubscriptionGroupID
	}
	if o.SubscriptionDays != nil {
		metadata["subscription_days"] = *o.SubscriptionDays
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return err
	}

	_, err = s.referralRewardSvc.CreditRechargeOrder(ctx, &RechargeCreditInput{
		UserID:                o.UserID,
		ExternalOrderID:       o.OutTradeNo,
		Provider:              paymentReferralProviderKey(o),
		Channel:               o.PaymentType,
		Currency:              ReferralSettlementCurrencyCNY,
		GrossAmount:           grossAmount,
		DiscountAmount:        0,
		PaidAmount:            paidAmount,
		GiftBalanceAmount:     0,
		CreditedBalanceAmount: 0,
		SkipBalanceCredit:     true,
		PaidAt:                o.PaidAt,
		MetadataJSON:          string(metadataJSON),
		Notes:                 fmt.Sprintf("subscription payment order %d", o.ID),
	})
	return err
}

func (s *PaymentService) RetryFailedSubscriptionReferralRewards(ctx context.Context, limit int) (int, error) {
	if s == nil || s.entClient == nil || s.referralRewardSvc == nil {
		return 0, nil
	}
	if limit <= 0 {
		limit = defaultFailedReferralRewardRecoveryLimit
	}
	scanLimit := limit * 4
	if scanLimit < defaultFailedReferralRewardRecoveryLimit {
		scanLimit = defaultFailedReferralRewardRecoveryLimit
	}
	if scanLimit > maxFailedReferralRewardRecoveryScan {
		scanLimit = maxFailedReferralRewardRecoveryScan
	}

	logs, err := s.entClient.PaymentAuditLog.Query().
		Where(paymentauditlog.ActionEQ(paymentAuditActionReferralRewardSyncFailed)).
		Order(paymentauditlog.ByCreatedAt()).
		Limit(scanLimit).
		All(ctx)
	if err != nil {
		return 0, err
	}

	recovered := 0
	for _, log := range logs {
		if recovered >= limit {
			break
		}
		orderID, err := strconv.ParseInt(strings.TrimSpace(log.OrderID), 10, 64)
		if err != nil {
			slog.Warn("skip invalid failed referral sync audit", "orderID", log.OrderID, "error", err)
			continue
		}
		if s.hasAuditLog(ctx, orderID, paymentAuditActionReferralRewardSyncRecovered) {
			continue
		}

		order, err := s.entClient.PaymentOrder.Get(ctx, orderID)
		if err != nil {
			slog.Warn("skip failed referral sync audit with missing order", "orderID", orderID, "error", err)
			continue
		}
		if order.OrderType != payment.OrderTypeSubscription || order.Status != OrderStatusCompleted {
			continue
		}

		if err := s.syncSubscriptionReferralReward(ctx, order); err != nil {
			slog.Warn("retry subscription referral reward sync failed", "orderID", orderID, "error", err)
			continue
		}
		s.markReferralRewardSyncRecovered(ctx, orderID)
		recovered++
	}

	return recovered, nil
}

func (s *PaymentService) markReferralRewardSyncRecovered(ctx context.Context, orderID int64) {
	if s == nil || s.entClient == nil {
		return
	}
	oid := strconv.FormatInt(orderID, 10)
	detailJSON, _ := json.Marshal(map[string]any{
		"source_action": paymentAuditActionReferralRewardSyncFailed,
	})
	updated, err := s.currentClient(ctx).PaymentAuditLog.Update().
		Where(
			paymentauditlog.OrderIDEQ(oid),
			paymentauditlog.ActionEQ(paymentAuditActionReferralRewardSyncFailed),
		).
		SetAction(paymentAuditActionReferralRewardSyncRecovered).
		SetDetail(string(detailJSON)).
		SetOperator("system").
		Save(ctx)
	if err != nil {
		slog.Error("mark referral reward sync recovered failed", "orderID", orderID, "error", err)
		return
	}
	if updated == 0 {
		s.writeAuditLog(ctx, orderID, paymentAuditActionReferralRewardSyncRecovered, "system", map[string]any{
			"source_action": paymentAuditActionReferralRewardSyncFailed,
		})
	}
}

func (s *PaymentService) ExecuteSubscriptionFulfillment(ctx context.Context, oid int64) error {
	o, err := s.entClient.PaymentOrder.Get(ctx, oid)
	if err != nil {
		return infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if o.Status == OrderStatusCompleted {
		return nil
	}
	if psIsRefundStatus(o.Status) {
		return infraerrors.BadRequest("INVALID_STATUS", "refund-related order cannot fulfill")
	}
	if o.Status != OrderStatusPaid && o.Status != OrderStatusFailed {
		return infraerrors.BadRequest("INVALID_STATUS", "order cannot fulfill in status "+o.Status)
	}
	if o.SubscriptionGroupID == nil || o.SubscriptionDays == nil {
		return infraerrors.BadRequest("INVALID_STATUS", "missing subscription info")
	}
	c, err := s.entClient.PaymentOrder.Update().Where(paymentorder.IDEQ(oid), paymentorder.StatusIn(OrderStatusPaid, OrderStatusFailed)).SetStatus(OrderStatusRecharging).Save(ctx)
	if err != nil {
		return fmt.Errorf("lock: %w", err)
	}
	if c == 0 {
		return nil
	}
	if err := s.doSub(ctx, o); err != nil {
		s.markFailed(ctx, oid, err)
		return err
	}
	return nil
}

func (s *PaymentService) doSub(ctx context.Context, o *dbent.PaymentOrder) error {
	gid := *o.SubscriptionGroupID
	days := *o.SubscriptionDays
	g, err := s.groupRepo.GetByID(ctx, gid)
	if err != nil || g.Status != payment.EntityStatusActive {
		return fmt.Errorf("group %d no longer exists or inactive", gid)
	}
	// Idempotency: check audit/notes to see if subscription was already assigned.
	// Prevents double-extension on retry after markCompleted fails.
	if s.hasAuditLog(ctx, o.ID, paymentAuditActionSubscriptionSuccess) ||
		s.hasAuditLog(ctx, o.ID, paymentAuditActionSubscriptionAssigned) ||
		s.hasSubscriptionAssignmentMarker(ctx, o, gid) {
		slog.Info("subscription already assigned for order, skipping", "orderID", o.ID, "groupID", gid)
		return s.markCompleted(ctx, o, paymentAuditActionSubscriptionSuccess)
	}
	orderNote := subscriptionAssignmentMarker(o.ID)
	_, _, err = s.subscriptionSvc.AssignOrExtendSubscription(ctx, &AssignSubscriptionInput{UserID: o.UserID, GroupID: gid, ValidityDays: days, AssignedBy: 0, Notes: orderNote})
	if err != nil {
		return fmt.Errorf("assign subscription: %w", err)
	}
	s.writeAuditLog(ctx, o.ID, paymentAuditActionSubscriptionAssigned, "system", map[string]any{
		"groupID":      gid,
		"validityDays": days,
		"marker":       orderNote,
	})
	return s.markCompleted(ctx, o, paymentAuditActionSubscriptionSuccess)
}

func (s *PaymentService) hasAuditLog(ctx context.Context, orderID int64, action string) bool {
	oid := strconv.FormatInt(orderID, 10)
	c, _ := s.entClient.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(oid), paymentauditlog.ActionEQ(action)).
		Limit(1).Count(ctx)
	return c > 0
}

func subscriptionAssignmentMarker(orderID int64) string {
	return fmt.Sprintf("payment_order_id=%d", orderID)
}

func legacySubscriptionAssignmentMarker(orderID int64) string {
	return fmt.Sprintf("payment order %d", orderID)
}

func subscriptionNotesContainAssignmentMarker(notes string, orderID int64) bool {
	for _, line := range strings.FieldsFunc(notes, func(r rune) bool {
		return r == '\n' || r == '\r'
	}) {
		line = strings.TrimSpace(line)
		if line == subscriptionAssignmentMarker(orderID) || line == legacySubscriptionAssignmentMarker(orderID) {
			return true
		}
	}
	return false
}

func (s *PaymentService) hasSubscriptionAssignmentMarker(ctx context.Context, o *dbent.PaymentOrder, groupID int64) bool {
	if s == nil || s.entClient == nil || o == nil {
		return false
	}
	sub, err := s.entClient.UserSubscription.Query().
		Where(usersubscription.UserIDEQ(o.UserID), usersubscription.GroupIDEQ(groupID)).
		Only(ctx)
	if err != nil || sub == nil || sub.Notes == nil {
		return false
	}
	return subscriptionNotesContainAssignmentMarker(*sub.Notes, o.ID)
}

func (s *PaymentService) applyAffiliateRebateForOrder(ctx context.Context, o *dbent.PaymentOrder) error {
	if o == nil || o.OrderType != payment.OrderTypeBalance || o.Amount <= 0 {
		return nil
	}
	if s.affiliateService == nil {
		return nil
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		s.writeAuditLog(ctx, o.ID, "AFFILIATE_REBATE_FAILED", "system", map[string]any{
			"error": fmt.Sprintf("begin affiliate rebate tx: %v", err),
		})
		return fmt.Errorf("begin affiliate rebate tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	txCtx := dbent.NewTxContext(ctx, tx)
	claimed, err := s.tryClaimAffiliateRebateAudit(txCtx, tx.Client(), o.ID, o.Amount)
	if err != nil {
		s.writeAuditLog(ctx, o.ID, "AFFILIATE_REBATE_FAILED", "system", map[string]any{
			"error": err.Error(),
		})
		return fmt.Errorf("claim affiliate rebate audit: %w", err)
	}
	if !claimed {
		return nil
	}

	sourceOrderID := o.ID
	rebateAmount, err := s.affiliateService.AccrueInviteRebateForOrder(txCtx, o.UserID, o.Amount, &sourceOrderID)
	if err != nil {
		s.writeAuditLog(ctx, o.ID, "AFFILIATE_REBATE_FAILED", "system", map[string]any{
			"error": err.Error(),
		})
		return fmt.Errorf("accrue affiliate rebate: %w", err)
	}

	if rebateAmount <= 0 {
		if err := s.updateClaimedAffiliateRebateAudit(txCtx, tx.Client(), o.ID, "AFFILIATE_REBATE_SKIPPED", map[string]any{
			"baseAmount": o.Amount,
			"reason":     "no inviter bound or rebate amount <= 0",
		}); err != nil {
			s.writeAuditLog(ctx, o.ID, "AFFILIATE_REBATE_FAILED", "system", map[string]any{
				"error": err.Error(),
			})
			return fmt.Errorf("update affiliate rebate skipped audit: %w", err)
		}
		if err := tx.Commit(); err != nil {
			s.writeAuditLog(ctx, o.ID, "AFFILIATE_REBATE_FAILED", "system", map[string]any{
				"error": fmt.Sprintf("commit affiliate rebate tx: %v", err),
			})
			return fmt.Errorf("commit affiliate rebate tx: %w", err)
		}
		return nil
	}

	if err := s.updateClaimedAffiliateRebateAudit(txCtx, tx.Client(), o.ID, "AFFILIATE_REBATE_APPLIED", map[string]any{
		"baseAmount":   o.Amount,
		"rebateAmount": rebateAmount,
	}); err != nil {
		s.writeAuditLog(ctx, o.ID, "AFFILIATE_REBATE_FAILED", "system", map[string]any{
			"error": err.Error(),
		})
		return fmt.Errorf("update affiliate rebate applied audit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		s.writeAuditLog(ctx, o.ID, "AFFILIATE_REBATE_FAILED", "system", map[string]any{
			"error": fmt.Sprintf("commit affiliate rebate tx: %v", err),
		})
		return fmt.Errorf("commit affiliate rebate tx: %w", err)
	}
	return nil
}

func (s *PaymentService) tryClaimAffiliateRebateAudit(ctx context.Context, client *dbent.Client, orderID int64, baseAmount float64) (bool, error) {
	if client == nil {
		return false, errors.New("nil payment client")
	}
	oid := strconv.FormatInt(orderID, 10)
	detail, _ := json.Marshal(map[string]any{
		"baseAmount": baseAmount,
		"status":     "reserved",
	})
	rows, err := client.QueryContext(ctx, `
INSERT INTO payment_audit_logs (order_id, action, detail, operator, created_at)
SELECT $1::text, 'AFFILIATE_REBATE_APPLIED', $2::text, 'system', NOW()
WHERE NOT EXISTS (
	SELECT 1
	FROM payment_audit_logs
	WHERE order_id = $1::text
	  AND action IN ('AFFILIATE_REBATE_APPLIED', 'AFFILIATE_REBATE_SKIPPED')
)
ON CONFLICT (order_id, action) DO NOTHING
RETURNING id`, oid, string(detail))
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return false, err
		}
		return false, nil
	}
	var claimID int64
	if err := rows.Scan(&claimID); err != nil {
		return false, err
	}
	return true, nil
}

func (s *PaymentService) updateClaimedAffiliateRebateAudit(ctx context.Context, client *dbent.Client, orderID int64, action string, detail map[string]any) error {
	if client == nil {
		return errors.New("nil payment client")
	}
	oid := strconv.FormatInt(orderID, 10)
	detailJSON, _ := json.Marshal(detail)
	updated, err := client.PaymentAuditLog.Update().
		Where(
			paymentauditlog.OrderIDEQ(oid),
			paymentauditlog.ActionEQ("AFFILIATE_REBATE_APPLIED"),
		).
		SetAction(action).
		SetDetail(string(detailJSON)).
		SetOperator("system").
		Save(ctx)
	if err != nil {
		return err
	}
	if updated == 0 {
		return errors.New("affiliate rebate claim log not found")
	}
	return nil
}

func (s *PaymentService) markFailed(ctx context.Context, oid int64, cause error) {
	now := time.Now()
	r := psErrMsg(cause)
	// Only mark FAILED if still in RECHARGING state — prevents overwriting
	// a COMPLETED order when markCompleted failed but fulfillment succeeded.
	c, e := s.entClient.PaymentOrder.Update().
		Where(paymentorder.IDEQ(oid), paymentorder.StatusEQ(OrderStatusRecharging)).
		SetStatus(OrderStatusFailed).SetFailedAt(now).SetFailedReason(r).Save(ctx)
	if e != nil {
		slog.Error("mark FAILED", "orderID", oid, "error", e)
	}
	if c > 0 {
		s.writeAuditLog(ctx, oid, "FULFILLMENT_FAILED", "system", map[string]any{"reason": r})
	}
}

func (s *PaymentService) RetryFulfillment(ctx context.Context, oid int64) error {
	o, err := s.entClient.PaymentOrder.Get(ctx, oid)
	if err != nil {
		return infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if o.PaidAt == nil {
		return infraerrors.BadRequest("INVALID_STATUS", "order is not paid")
	}
	if psIsRefundStatus(o.Status) {
		return infraerrors.BadRequest("INVALID_STATUS", "refund-related order cannot retry")
	}
	if o.Status == OrderStatusRecharging {
		return infraerrors.Conflict("CONFLICT", "order is being processed")
	}
	if o.Status == OrderStatusCompleted {
		return infraerrors.BadRequest("INVALID_STATUS", "order already completed")
	}
	if o.Status != OrderStatusFailed && o.Status != OrderStatusPaid {
		return infraerrors.BadRequest("INVALID_STATUS", "only paid and failed orders can retry")
	}
	_, err = s.entClient.PaymentOrder.Update().Where(paymentorder.IDEQ(oid), paymentorder.StatusIn(OrderStatusFailed, OrderStatusPaid)).SetStatus(OrderStatusPaid).ClearFailedAt().ClearFailedReason().Save(ctx)
	if err != nil {
		return fmt.Errorf("reset for retry: %w", err)
	}
	s.writeAuditLog(ctx, oid, "RECHARGE_RETRY", "admin", map[string]any{"detail": "admin manual retry"})
	return s.executeFulfillment(ctx, oid)
}
