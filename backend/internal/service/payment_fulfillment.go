package service

import (
	"context"
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
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
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
		// Fallback: try legacy format (sub2_N where N is DB ID)
		trimmed := strings.TrimPrefix(n.OrderID, orderIDPrefix)
		if oid, parseErr := strconv.ParseInt(trimmed, 10, 64); parseErr == nil {
			return s.confirmPayment(ctx, oid, n.TradeNo, n.Amount, pk)
		}
		return fmt.Errorf("order not found for out_trade_no: %s", n.OrderID)
	}
	return s.confirmPayment(ctx, order.ID, n.TradeNo, n.Amount, pk)
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
	if o == nil || o.OrderType != payment.OrderTypeBalance {
		return nil
	}

	reversalAmount := normalizeExternalReversalAmount(o, n.Amount)
	apply := func(txCtx context.Context) error {
		currentOrder, err := s.getPaymentOrderByID(txCtx, o.ID)
		if err != nil {
			return err
		}
		if currentOrder.OrderType != payment.OrderTypeBalance {
			return nil
		}

		if err := s.syncExternalReferralReversal(txCtx, currentOrder, reversalAmount, n.AmountSemantic, chargeback); err != nil {
			return err
		}

		creditedDelta, refundAmountTotal := computeExternalCreditedRefund(currentOrder, reversalAmount, n.AmountSemantic)
		if creditedDelta > 0 {
			if err := s.userRepo.DeductBalance(txCtx, currentOrder.UserID, creditedDelta); err != nil {
				return err
			}
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
	if s.referralRefundSvc == nil || o == nil || o.OrderType != payment.OrderTypeBalance {
		return nil
	}

	providerKey := payment.GetBasePaymentType(o.PaymentType)
	if providerKey == "" {
		providerKey = o.PaymentType
	}

	rechargeOrder, err := s.referralRefundSvc.rechargeRepo.GetByProviderAndExternalOrderID(ctx, strings.TrimSpace(providerKey), strings.TrimSpace(o.OutTradeNo))
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

func (s *PaymentService) confirmPayment(ctx context.Context, oid int64, tradeNo string, paid float64, pk string) error {
	o, err := s.entClient.PaymentOrder.Get(ctx, oid)
	if err != nil {
		slog.Error("order not found", "orderID", oid)
		return nil
	}
	// Skip amount check when paid=0 (e.g. QueryOrder doesn't return amount).
	// Also skip if paid is NaN/Inf (malformed provider data).
	if paid > 0 && !math.IsNaN(paid) && !math.IsInf(paid, 0) {
		if math.Abs(paid-o.PayAmount) > amountToleranceCNY {
			s.writeAuditLog(ctx, o.ID, "PAYMENT_AMOUNT_MISMATCH", pk, map[string]any{"expected": o.PayAmount, "paid": paid, "tradeNo": tradeNo})
			return fmt.Errorf("amount mismatch: expected %.2f, got %.2f", o.PayAmount, paid)
		}
	}
	// Use order's expected amount when provider didn't report one
	if paid <= 0 || math.IsNaN(paid) || math.IsInf(paid, 0) {
		paid = o.PayAmount
	}
	return s.toPaid(ctx, o, tradeNo, paid, pk)
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
	return s.markCompleted(ctx, o, "RECHARGE_SUCCESS")
}

func (s *PaymentService) markCompleted(ctx context.Context, o *dbent.PaymentOrder, auditAction string) error {
	if o != nil && o.OrderType == payment.OrderTypeBalance {
		if err := s.syncReferralReward(ctx, o); err != nil {
			return fmt.Errorf("sync referral reward: %w", err)
		}
	}

	now := time.Now()
	_, err := s.entClient.PaymentOrder.Update().Where(paymentorder.IDEQ(o.ID), paymentorder.StatusEQ(OrderStatusRecharging)).SetStatus(OrderStatusCompleted).SetCompletedAt(now).Save(ctx)
	if err != nil {
		return fmt.Errorf("mark completed: %w", err)
	}
	s.writeAuditLog(ctx, o.ID, auditAction, "system", map[string]any{"rechargeCode": o.RechargeCode, "amount": o.Amount})
	return nil
}

func (s *PaymentService) syncReferralReward(ctx context.Context, o *dbent.PaymentOrder) error {
	if s.referralRewardSvc == nil || o == nil || o.OrderType != payment.OrderTypeBalance {
		return nil
	}

	providerKey := payment.GetBasePaymentType(o.PaymentType)
	if providerKey == "" {
		providerKey = o.PaymentType
	}

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
		Provider:              providerKey,
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
	// Idempotency: check audit log to see if subscription was already assigned.
	// Prevents double-extension on retry after markCompleted fails.
	if s.hasAuditLog(ctx, o.ID, "SUBSCRIPTION_SUCCESS") {
		slog.Info("subscription already assigned for order, skipping", "orderID", o.ID, "groupID", gid)
		return s.markCompleted(ctx, o, "SUBSCRIPTION_SUCCESS")
	}
	orderNote := fmt.Sprintf("payment order %d", o.ID)
	_, _, err = s.subscriptionSvc.AssignOrExtendSubscription(ctx, &AssignSubscriptionInput{UserID: o.UserID, GroupID: gid, ValidityDays: days, AssignedBy: 0, Notes: orderNote})
	if err != nil {
		return fmt.Errorf("assign subscription: %w", err)
	}
	return s.markCompleted(ctx, o, "SUBSCRIPTION_SUCCESS")
}

func (s *PaymentService) hasAuditLog(ctx context.Context, orderID int64, action string) bool {
	oid := strconv.FormatInt(orderID, 10)
	c, _ := s.entClient.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(oid), paymentauditlog.ActionEQ(action)).
		Limit(1).Count(ctx)
	return c > 0
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
