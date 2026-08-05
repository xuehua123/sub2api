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

	"entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/ent/paymentproviderinstance"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/payment/provider"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/servertiming"
)

// --- Refund Flow ---

var createPaymentProviderFromInstance = provider.CreateProvider

// getOrderProviderInstance looks up the provider instance that processed this order.
// For legacy orders without provider_instance_id, it resolves only when the
// historical instance is uniquely identifiable from the stored order fields.
func (s *PaymentService) getOrderProviderInstance(ctx context.Context, o *dbent.PaymentOrder) (*dbent.PaymentProviderInstance, error) {
	if s == nil || s.entClient == nil || o == nil {
		return nil, nil
	}

	if snapshot := psOrderProviderSnapshot(o); snapshot != nil {
		return s.resolveSnapshotOrderProviderInstance(ctx, o, snapshot)
	}

	instIDStr := strings.TrimSpace(psStringValue(o.ProviderInstanceID))
	if instIDStr == "" {
		return s.resolveUniqueLegacyOrderProviderInstance(ctx, o)
	}

	instID, err := strconv.ParseInt(instIDStr, 10, 64)
	if err != nil {
		return nil, nil
	}
	return s.entClient.PaymentProviderInstance.Get(ctx, instID)
}

// getRefundOrderProviderInstance resolves the provider instance for refund paths.
// Refunds must be pinned to an explicit historical binding, so legacy
// "best-effort" provider guessing is intentionally not allowed here.
func (s *PaymentService) getRefundOrderProviderInstance(ctx context.Context, o *dbent.PaymentOrder) (*dbent.PaymentProviderInstance, error) {
	if s == nil || s.entClient == nil || o == nil {
		return nil, nil
	}

	if snapshot := psOrderProviderSnapshot(o); snapshot != nil {
		return s.resolveSnapshotOrderProviderInstance(ctx, o, snapshot)
	}

	instIDStr := strings.TrimSpace(psStringValue(o.ProviderInstanceID))
	if instIDStr == "" {
		return nil, nil
	}

	instID, err := strconv.ParseInt(instIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("order %d refund provider instance id is invalid: %s", o.ID, instIDStr)
	}
	inst, err := s.entClient.PaymentProviderInstance.Get(ctx, instID)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, fmt.Errorf("order %d refund provider instance %s is missing", o.ID, instIDStr)
		}
		return nil, err
	}
	return inst, nil
}

func (s *PaymentService) resolveUniqueLegacyOrderProviderInstance(ctx context.Context, o *dbent.PaymentOrder) (*dbent.PaymentProviderInstance, error) {
	paymentType := payment.GetBasePaymentType(strings.TrimSpace(o.PaymentType))
	providerKey := strings.TrimSpace(psStringValue(o.ProviderKey))
	if providerKey != "" {
		instances, err := s.entClient.PaymentProviderInstance.Query().
			Where(paymentproviderinstance.ProviderKeyEQ(providerKey)).
			All(ctx)
		if err != nil {
			return nil, err
		}
		matched := psFilterLegacyOrderProviderInstances(paymentType, instances)
		if len(matched) == 1 {
			return matched[0], nil
		}
		return nil, nil
	}

	if paymentType == "" {
		return nil, nil
	}

	instances, err := s.entClient.PaymentProviderInstance.Query().
		All(ctx)
	if err != nil {
		return nil, err
	}

	matched := psFilterLegacyOrderProviderInstances(paymentType, instances)
	if len(matched) == 1 {
		return matched[0], nil
	}
	return nil, nil
}

func psFilterLegacyOrderProviderInstances(orderPaymentType string, instances []*dbent.PaymentProviderInstance) []*dbent.PaymentProviderInstance {
	if len(instances) == 0 {
		return nil
	}
	if strings.TrimSpace(orderPaymentType) == "" {
		return instances
	}
	var matched []*dbent.PaymentProviderInstance
	for _, inst := range instances {
		if psLegacyOrderMatchesInstance(orderPaymentType, inst) {
			matched = append(matched, inst)
		}
	}
	return matched
}

func psLegacyOrderMatchesInstance(orderPaymentType string, inst *dbent.PaymentProviderInstance) bool {
	if inst == nil {
		return false
	}

	baseType := payment.GetBasePaymentType(strings.TrimSpace(orderPaymentType))
	instanceProviderKey := strings.TrimSpace(inst.ProviderKey)
	if baseType == "" {
		return false
	}

	if baseType == payment.TypeStripe {
		return instanceProviderKey == payment.TypeStripe
	}
	if instanceProviderKey == payment.TypeStripe {
		return false
	}
	if instanceProviderKey == baseType {
		return true
	}
	return payment.InstanceSupportsType(inst.SupportedTypes, baseType)
}

func (s *PaymentService) RequestRefund(ctx context.Context, oid, uid int64, reason string) error {
	o, err := s.validateRefundRequest(ctx, oid, uid)
	if err != nil {
		return err
	}
	u, err := s.userRepo.GetByID(ctx, o.UserID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}
	if u.Balance < o.Amount {
		return infraerrors.BadRequest("BALANCE_NOT_ENOUGH", "refund amount exceeds balance")
	}
	nr := strings.TrimSpace(reason)
	now := time.Now()
	by := fmt.Sprintf("%d", uid)
	c, err := s.entClient.PaymentOrder.Update().Where(paymentorder.IDEQ(oid), paymentorder.UserIDEQ(uid), paymentorder.StatusEQ(OrderStatusCompleted), paymentorder.OrderTypeEQ(payment.OrderTypeBalance)).SetStatus(OrderStatusRefundRequested).SetRefundRequestedAt(now).SetRefundRequestReason(nr).SetRefundRequestedBy(by).SetRefundAmount(o.Amount).Save(ctx)
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}
	if c == 0 {
		return infraerrors.Conflict("CONFLICT", "order status changed")
	}
	s.writeAuditLog(ctx, oid, "REFUND_REQUESTED", fmt.Sprintf("user:%d", uid), map[string]any{"amount": o.Amount, "reason": nr})
	return nil
}

func (s *PaymentService) validateRefundRequest(ctx context.Context, oid, uid int64) (*dbent.PaymentOrder, error) {
	o, err := s.entClient.PaymentOrder.Get(ctx, oid)
	if err != nil {
		return nil, infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if o.UserID != uid {
		return nil, infraerrors.Forbidden("FORBIDDEN", "no permission")
	}
	if o.OrderType != payment.OrderTypeBalance {
		return nil, infraerrors.BadRequest("INVALID_ORDER_TYPE", "only balance orders can request refund")
	}
	if o.Status != OrderStatusCompleted {
		return nil, infraerrors.BadRequest("INVALID_STATUS", "only completed orders can request refund")
	}
	// Check provider instance allows user refund
	inst, err := s.getRefundOrderProviderInstance(ctx, o)
	if err != nil || inst == nil {
		return nil, infraerrors.Forbidden("USER_REFUND_DISABLED", "refund is not available for this order")
	}
	if !inst.AllowUserRefund {
		return nil, infraerrors.Forbidden("USER_REFUND_DISABLED", "user refund is not enabled for this provider")
	}
	return o, nil
}

func (s *PaymentService) PrepareRefund(ctx context.Context, oid int64, amt float64, reason string, force, deduct bool) (*RefundPlan, *RefundResult, error) {
	o, err := s.entClient.PaymentOrder.Get(ctx, oid)
	if err != nil {
		return nil, nil, infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if o.Status == OrderStatusRefundPending {
		return nil, nil, infraerrors.Conflict("REFUND_PENDING", "refund is pending confirmation; query refund status instead")
	}
	ok := []string{OrderStatusCompleted, OrderStatusRefundRequested, OrderStatusRefundFailed}
	if !psSliceContains(ok, o.Status) {
		return nil, nil, infraerrors.BadRequest("INVALID_STATUS", "order status does not allow refund")
	}
	// Check provider instance allows admin refund
	inst, instErr := s.getRefundOrderProviderInstance(ctx, o)
	if instErr != nil {
		slog.Warn("refund: provider instance lookup failed", "orderID", oid, "error", instErr)
		return nil, nil, infraerrors.InternalServer("PROVIDER_LOOKUP_FAILED", "failed to look up payment provider for this order")
	}
	if inst == nil {
		// Legacy order without provider_instance_id — block refund
		return nil, nil, infraerrors.Forbidden("REFUND_DISABLED", "refund is not available for this order")
	}
	if !inst.RefundEnabled {
		return nil, nil, infraerrors.Forbidden("REFUND_DISABLED", "refund is not enabled for this provider")
	}
	if math.IsNaN(amt) || math.IsInf(amt, 0) {
		return nil, nil, infraerrors.BadRequest("INVALID_AMOUNT", "invalid refund amount")
	}
	if amt <= 0 {
		amt = o.Amount
	}
	orderCurrency := PaymentOrderCurrency(o)
	if amt-o.Amount > paymentAmountToleranceForCurrency(orderCurrency) {
		return nil, nil, infraerrors.BadRequest("REFUND_AMOUNT_EXCEEDED", "refund amount exceeds recharge")
	}
	ga := calculateGatewayRefundAmount(o.Amount, o.PayAmount, amt, orderCurrency)
	rr := strings.TrimSpace(reason)
	if rr == "" && o.RefundRequestReason != nil {
		rr = *o.RefundRequestReason
	}
	if rr == "" {
		rr = fmt.Sprintf("refund order:%d", o.ID)
	}
	p := &RefundPlan{OrderID: oid, Order: o, RefundAmount: amt, GatewayAmount: ga, Reason: rr, Force: force, DeductBalance: deduct, DeductionType: payment.DeductionTypeNone}
	if deduct {
		if er := s.prepDeduct(ctx, o, p, force); er != nil {
			return nil, er, nil
		}
	}
	return p, nil, nil
}

func (s *PaymentService) prepDeduct(ctx context.Context, o *dbent.PaymentOrder, p *RefundPlan, force bool) *RefundResult {
	if o.OrderType == payment.OrderTypeSubscription {
		return s.prepSubscriptionDeduct(ctx, o, p, force, subscriptionDaysRefundDelta(o, p.RefundAmount))
	}
	u, err := s.userRepo.GetByID(ctx, o.UserID)
	if err != nil {
		if !force {
			return &RefundResult{Success: false, Warning: "cannot fetch user balance, use force", RequireForce: true}
		}
		return nil
	}
	p.DeductionType = payment.DeductionTypeBalance
	if u.Balance < p.RefundAmount && !force {
		return &RefundResult{Success: false, Warning: "user balance is insufficient for deduction, use force", RequireForce: true}
	}
	p.BalanceToDeduct = math.Max(0, math.Min(p.RefundAmount, u.Balance))
	return nil
}

func (s *PaymentService) prepSubscriptionDeduct(ctx context.Context, o *dbent.PaymentOrder, p *RefundPlan, force bool, daysToDeduct int) *RefundResult {
	p.DeductionType = payment.DeductionTypeSubscription
	p.SubDaysToDeduct = daysToDeduct
	if p.SubDaysToDeduct <= 0 {
		return nil
	}
	if o.SubscriptionEntitlementID != nil && *o.SubscriptionEntitlementID > 0 && s.subscriptionEntitlementSvc != nil {
		snapshot, err := s.subscriptionEntitlementSvc.GetRefundSnapshot(ctx, *o.SubscriptionEntitlementID, time.Now())
		if err == nil && snapshot != nil {
			p.EntitlementID = snapshot.ID
			p.EntitlementSnapshot = snapshot
			return nil
		}
		if err != nil && !errors.Is(err, ErrSubscriptionEntitlementNotFound) && !errors.Is(err, ErrSubscriptionEntitlementExpired) && !force {
			return &RefundResult{Success: false, Warning: "cannot fetch active entitlement for deduction, use force", RequireForce: true}
		}
	}
	if o.SubscriptionGroupID != nil {
		sub, err := s.subscriptionSvc.GetActiveSubscription(ctx, o.UserID, *o.SubscriptionGroupID)
		if err == nil && sub != nil {
			p.SubscriptionID = sub.ID
			snapshot := *sub
			p.SubscriptionSnapshot = &snapshot
		} else if !force {
			return &RefundResult{Success: false, Warning: "cannot find active subscription for deduction, use force", RequireForce: true}
		}
	}
	if p.SubscriptionID > 0 {
		return nil
	}
	if !force {
		return &RefundResult{Success: false, Warning: "cannot find active subscription entitlement for deduction, use force", RequireForce: true}
	}
	return nil
}

type availableBalanceDeductor interface {
	DeductAvailableBalance(ctx context.Context, id int64, amount float64) (float64, error)
}

func (s *PaymentService) deductAvailableBalance(ctx context.Context, userID int64, amount float64) (float64, error) {
	repo, ok := s.userRepo.(availableBalanceDeductor)
	if !ok {
		return 0, errors.New("user repository does not support available balance deduction")
	}
	return repo.DeductAvailableBalance(ctx, userID, amount)
}

func (s *PaymentService) ExecuteRefund(ctx context.Context, p *RefundPlan) (*RefundResult, error) {
	if err := s.claimRefundAndApplyDeduction(ctx, p); err != nil {
		return nil, err
	}
	resp, err := s.gwRefund(ctx, p)
	if err != nil {
		return s.handleGwFail(ctx, p, err)
	}
	return s.finishRefund(ctx, p, resp)
}

const paymentOrderSnapshotRefundInFlight = "refund_inflight"

type refundInFlightDetail struct {
	RefundAmount                float64 `json:"refundAmount"`
	GatewayAmount               float64 `json:"gatewayAmount"`
	ProviderRefundID            string  `json:"providerRefundID"`
	PreviousRefundAmount        float64 `json:"previousRefundAmount"`
	PreviousSettledRefundAmount float64 `json:"previousSettledRefundAmount"`
	DeductBalance               bool    `json:"deductBalance"`
	DeductionType               string  `json:"deductionType"`
	BalanceDeducted             float64 `json:"balanceDeducted"`
	SubDaysDeducted             int     `json:"subDaysDeducted"`
}

func (s *PaymentService) claimRefundAndApplyDeduction(ctx context.Context, p *RefundPlan) (err error) {
	if s == nil || s.entClient == nil || p == nil || p.Order == nil {
		return errors.New("payment database is not configured")
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin refund claim: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	txCtx := dbent.NewTxContext(ctx, tx)

	claimed, err := tx.PaymentOrder.Update().
		Where(
			paymentorder.IDEQ(p.OrderID),
			paymentorder.StatusIn(OrderStatusCompleted, OrderStatusRefundRequested, OrderStatusRefundFailed),
		).
		SetStatus(OrderStatusRefunding).
		Save(txCtx)
	if err != nil {
		return fmt.Errorf("lock: %w", err)
	}
	if claimed == 0 {
		return infraerrors.Conflict("CONFLICT", "order status changed")
	}

	if err := s.applyPreparedRefundDeduction(txCtx, p); err != nil {
		return err
	}

	providerSnapshot := copyMap(p.Order.ProviderSnapshot)
	if providerSnapshot == nil {
		providerSnapshot = make(map[string]any, 1)
	}
	previousSettledRefundAmount := settledRefundAmount(p.Order)
	providerSnapshot[paymentOrderSnapshotRefundInFlight] = refundInFlightDetail{
		RefundAmount:                p.RefundAmount,
		GatewayAmount:               p.GatewayAmount,
		ProviderRefundID:            p.ProviderRefundID,
		PreviousRefundAmount:        p.Order.RefundAmount,
		PreviousSettledRefundAmount: previousSettledRefundAmount,
		DeductBalance:               p.DeductBalance,
		DeductionType:               p.DeductionType,
		BalanceDeducted:             p.BalanceToDeduct,
		SubDaysDeducted:             p.SubDaysToDeduct,
	}
	if _, err := tx.PaymentOrder.UpdateOneID(p.OrderID).
		SetRefundReason(p.Reason).
		SetForceRefund(p.Force).
		SetProviderSnapshot(providerSnapshot).
		ClearRefundAt().
		ClearFailedAt().
		ClearFailedReason().
		Save(txCtx); err != nil {
		return fmt.Errorf("persist refund claim: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit refund claim: %w", err)
	}
	p.Order.ProviderSnapshot = providerSnapshot
	s.invalidateFinalizedRefundSubscriptionCaches(p)
	return nil
}

func settledRefundAmount(order *dbent.PaymentOrder) float64 {
	if order == nil {
		return 0
	}
	switch order.Status {
	case OrderStatusCompleted, OrderStatusRefundFailed, OrderStatusPartiallyRefunded, OrderStatusRefunded:
		return roundMoney(order.RefundAmount)
	case OrderStatusRefundRequested:
		// A local refund request records the requested amount before the
		// provider has actually settled it. External notifications must not
		// treat that request as money already refunded.
		return 0
	default:
		return 0
	}
}

func (s *PaymentService) applyPreparedRefundDeduction(ctx context.Context, p *RefundPlan) error {
	if p.DeductionType == payment.DeductionTypeBalance && p.BalanceToDeduct > 0 {
		// Skip balance deduction on retry if previous attempt already deducted
		// but failed to roll back (REFUND_ROLLBACK_FAILED in audit log).
		if !s.hasAuditLog(ctx, p.OrderID, "REFUND_ROLLBACK_FAILED") {
			deducted, err := s.deductAvailableBalance(ctx, p.Order.UserID, p.BalanceToDeduct)
			if err != nil {
				return fmt.Errorf("deduction: %w", err)
			}
			p.BalanceToDeduct = deducted
		} else {
			slog.Warn("skipping balance deduction on retry (previous rollback failed)", "orderID", p.OrderID)
			p.BalanceToDeduct = 0
		}
	}
	if p.DeductionType == payment.DeductionTypeSubscription && p.SubDaysToDeduct > 0 && p.SubscriptionID > 0 {
		if !s.hasAuditLog(ctx, p.OrderID, "REFUND_ROLLBACK_FAILED") {
			_, err := s.subscriptionSvc.ExtendSubscription(ctx, p.SubscriptionID, -p.SubDaysToDeduct)
			if err != nil {
				if errors.Is(err, ErrAdjustWouldExpire) {
					// Deduction would expire the subscription, so revoke it entirely.
					// Keep the snapshot in the plan so a later gateway failure can
					// restore the user's access instead of leaving the row deleted.
					slog.Info("subscription deduction would expire, revoking", "orderID", p.OrderID, "subID", p.SubscriptionID, "days", p.SubDaysToDeduct)
					if revokeErr := s.subscriptionSvc.RevokeSubscription(ctx, p.SubscriptionID); revokeErr != nil {
						return fmt.Errorf("revoke subscription: %w", revokeErr)
					}
				} else {
					// Other errors (DB failure, not found) — abort refund
					return fmt.Errorf("deduct subscription days: %w", err)
				}
				p.SubRevoked = true
			}
		} else {
			slog.Warn("skipping subscription deduction on retry (previous rollback failed)", "orderID", p.OrderID)
			p.SubDaysToDeduct = 0
		}
	}
	if p.DeductionType == payment.DeductionTypeSubscription && p.SubDaysToDeduct > 0 && p.EntitlementID > 0 {
		if !s.hasAuditLog(ctx, p.OrderID, "REFUND_ROLLBACK_FAILED") {
			if s.subscriptionEntitlementSvc == nil {
				return fmt.Errorf("deduct subscription entitlement days: entitlement service is not configured")
			}
			adjustment, err := s.subscriptionEntitlementSvc.ShortenForRefund(ctx, p.EntitlementID, p.SubDaysToDeduct, time.Now())
			if err != nil {
				return fmt.Errorf("deduct subscription entitlement days: %w", err)
			}
			p.EntitlementSnapshot = adjustment.Snapshot
			p.EntitlementRevoked = adjustment.Revoked
			p.EntitlementUpdatedAt = adjustment.UpdatedAt
		} else {
			slog.Warn("skipping subscription entitlement deduction on retry (previous rollback failed)", "orderID", p.OrderID)
			p.SubDaysToDeduct = 0
		}
	}
	return nil
}

func (s *PaymentService) gwRefund(ctx context.Context, p *RefundPlan) (*payment.RefundResponse, error) {
	if p.Order.PaymentTradeNo == "" {
		s.writeAuditLog(ctx, p.Order.ID, "REFUND_NO_TRADE_NO", "admin", map[string]any{"detail": "skipped"})
		return &payment.RefundResponse{Status: payment.ProviderStatusSuccess}, nil
	}

	// Use the exact provider instance that created this order, not a random one
	// from the registry. Each instance has its own merchant credentials.
	prov, err := s.getRefundProvider(ctx, p.Order)
	if err != nil {
		return nil, fmt.Errorf("get refund provider: %w", err)
	}
	if err := validateProviderSnapshotMetadata(p.Order, prov.ProviderKey(), providerMerchantIdentityMetadata(prov)); err != nil {
		s.writeAuditLog(ctx, p.Order.ID, "REFUND_PROVIDER_METADATA_MISMATCH", "admin", map[string]any{
			"detail": err.Error(),
		})
		return nil, err
	}
	finishProviderCall := servertiming.ObserveDependency(ctx, "payment")
	resp, err := prov.Refund(ctx, payment.RefundRequest{
		TradeNo: p.Order.PaymentTradeNo,
		OrderID: p.Order.OutTradeNo,
		Amount:  formatGatewayRefundAmount(p.GatewayAmount, p.Order),
		Reason:  p.Reason,
	})
	finishProviderCall()
	if err != nil {
		if resp != nil && strings.TrimSpace(resp.Status) == payment.ProviderStatusPending {
			return resp, nil
		}
		return nil, err
	}
	if err := validateRefundProviderResponse(resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func formatGatewayRefundAmount(amount float64, order *dbent.PaymentOrder) string {
	return payment.FormatAmountForCurrency(amount, PaymentOrderCurrency(order))
}

func validateRefundProviderResponse(resp *payment.RefundResponse) error {
	if resp == nil {
		return fmt.Errorf("payment refund response missing")
	}
	status := strings.TrimSpace(resp.Status)
	switch status {
	case payment.ProviderStatusSuccess, payment.ProviderStatusRefunded, payment.ProviderStatusPending:
		return nil
	case payment.ProviderStatusFailed:
		return fmt.Errorf("payment refund failed: status %s", status)
	default:
		return fmt.Errorf("payment refund returned unknown status: %s", status)
	}
}

func (s *PaymentService) finishRefund(ctx context.Context, p *RefundPlan, resp *payment.RefundResponse) (*RefundResult, error) {
	if err := validateRefundProviderResponse(resp); err != nil {
		return s.handleGwFail(ctx, p, err)
	}
	if p != nil {
		p.ProviderRefundID = refundResponseID(resp)
	}
	switch strings.TrimSpace(resp.Status) {
	case payment.ProviderStatusSuccess, payment.ProviderStatusRefunded:
		return s.markRefundOk(ctx, p)
	case payment.ProviderStatusPending:
		return s.markRefundPending(ctx, p, resp)
	default:
		return s.handleGwFail(ctx, p, fmt.Errorf("payment refund returned unknown status: %s", strings.TrimSpace(resp.Status)))
	}
}

func (s *PaymentService) QueryAndFinalizeRefund(ctx context.Context, oid int64) (*RefundResult, error) {
	o, err := s.entClient.PaymentOrder.Get(ctx, oid)
	if err != nil {
		return nil, infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if o.Status != OrderStatusRefundPending {
		return nil, infraerrors.BadRequest("INVALID_STATUS", "only refund pending orders can be finalized")
	}

	prov, err := s.getRefundProvider(ctx, o)
	if err != nil {
		return nil, fmt.Errorf("get refund provider: %w", err)
	}
	queryProvider, ok := prov.(payment.RefundQueryProvider)
	if !ok {
		return nil, infraerrors.BadRequest("REFUND_QUERY_UNSUPPORTED", "this payment provider does not support refund status query; please verify manually")
	}

	pendingDetail := s.latestRefundPendingDetail(ctx, o)
	finishProviderCall := servertiming.ObserveDependency(ctx, "payment")
	resp, err := queryProvider.QueryRefund(ctx, payment.RefundQueryRequest{
		TradeNo:  o.PaymentTradeNo,
		OrderID:  o.OutTradeNo,
		RefundID: pendingDetail.RefundID,
		Amount:   formatGatewayRefundAmount(o.RefundAmount, o),
	})
	finishProviderCall()
	if err != nil {
		return nil, fmt.Errorf("query refund: %w", err)
	}
	if err := validateRefundProviderResponse(resp); err != nil {
		return s.finalizeRefundFailed(ctx, o, err)
	}

	plan := s.refundFinalizePlan(o)
	plan.ProviderRefundID = refundResponseID(resp)
	deductBalance := true
	if pendingDetail.DeductBalance != nil {
		deductBalance = *pendingDetail.DeductBalance
	}
	plan.DeductBalance = deductBalance
	if !deductBalance {
		plan.DeductionType = payment.DeductionTypeNone
		plan.BalanceToDeduct = 0
		plan.SubDaysToDeduct = 0
	}
	if !pendingDetail.DeductionRollbackOK {
		plan.BalanceToDeduct = 0
		plan.SubDaysToDeduct = 0
	} else if deductBalance && o.OrderType == payment.OrderTypeSubscription {
		daysToDeduct := subscriptionDaysRefundDelta(o, plan.RefundAmount)
		if pendingDetail.SubDaysRolledBack != nil {
			daysToDeduct = *pendingDetail.SubDaysRolledBack
			if daysToDeduct < 0 {
				daysToDeduct = 0
			}
		}
		if early := s.prepSubscriptionDeduct(ctx, o, plan, true, daysToDeduct); early != nil {
			return early, nil
		}
	} else if deductBalance && pendingDetail.BalanceRolledBack != nil {
		plan.BalanceToDeduct = roundMoney(*pendingDetail.BalanceRolledBack)
		if plan.BalanceToDeduct < 0 {
			plan.BalanceToDeduct = 0
		}
	}
	switch strings.TrimSpace(resp.Status) {
	case payment.ProviderStatusSuccess, payment.ProviderStatusRefunded:
		return s.finalizePendingRefundSuccess(ctx, plan)
	case payment.ProviderStatusPending:
		return s.observePendingRefundQuery(ctx, plan, resp)
	default:
		return s.finalizeRefundFailed(ctx, o, fmt.Errorf("payment refund returned unknown status: %s", strings.TrimSpace(resp.Status)))
	}
}

func (s *PaymentService) finalizePendingRefundSuccess(ctx context.Context, p *RefundPlan) (_ *RefundResult, err error) {
	if s == nil || s.entClient == nil {
		return nil, errors.New("payment database is not configured")
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin refund finalization: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	txCtx := dbent.NewTxContext(ctx, tx)

	claimed, err := tx.PaymentOrder.Update().
		Where(paymentorder.IDEQ(p.OrderID), paymentorder.StatusEQ(OrderStatusRefundPending)).
		SetStatus(OrderStatusRefunding).
		Save(txCtx)
	if err != nil {
		return nil, fmt.Errorf("claim pending refund: %w", err)
	}
	if claimed == 0 {
		current, getErr := tx.PaymentOrder.Get(txCtx, p.OrderID)
		if getErr == nil && s.refundCompletedExternally(txCtx, current, p) {
			if err = tx.Commit(); err != nil {
				return nil, fmt.Errorf("commit completed refund observation: %w", err)
			}
			return refundResultFromPlan(p), nil
		}
		return nil, infraerrors.Conflict("REFUND_FINALIZE_CONFLICT", "refund status changed")
	}

	if err := s.applyRefundFinalDeduction(txCtx, p); err != nil {
		return nil, err
	}
	result, err := s.markRefundOk(txCtx, p)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit refund finalization: %w", err)
	}
	s.invalidateFinalizedRefundSubscriptionCaches(p)
	return result, nil
}

func (s *PaymentService) invalidateFinalizedRefundSubscriptionCaches(p *RefundPlan) {
	if s == nil ||
		s.subscriptionSvc == nil ||
		p == nil ||
		p.DeductionType != payment.DeductionTypeSubscription ||
		p.SubDaysToDeduct <= 0 ||
		p.SubscriptionID <= 0 ||
		p.SubscriptionSnapshot == nil {
		return
	}
	subscription := p.SubscriptionSnapshot
	s.subscriptionSvc.bestEffortInvalidateSubscriptionCachesBefore(
		"finalize pending refund",
		subscription.UserID,
		subscription.GroupID,
		subscriptionCacheVersion(subscription),
	)
}
func (s *PaymentService) refundFinalizePlan(o *dbent.PaymentOrder) *RefundPlan {
	refundAmount := o.RefundAmount
	reason := strings.TrimSpace(psStringValue(o.RefundReason))
	if reason == "" {
		reason = fmt.Sprintf("refund order:%d", o.ID)
	}
	return &RefundPlan{
		OrderID:       o.ID,
		Order:         o,
		RefundAmount:  refundAmount,
		GatewayAmount: calculateGatewayRefundAmount(o.Amount, o.PayAmount, refundAmount, PaymentOrderCurrency(o)),
		Reason:        reason,
		Force:         o.ForceRefund,
		DeductBalance: true,
		DeductionType: payment.DeductionTypeBalance,
		BalanceToDeduct: func() float64 {
			if o.OrderType == payment.OrderTypeBalance {
				return refundAmount
			}
			return 0
		}(),
	}
}

func (s *PaymentService) applyRefundFinalDeduction(ctx context.Context, p *RefundPlan) error {
	if p.DeductionType == payment.DeductionTypeBalance && p.BalanceToDeduct > 0 {
		deducted, err := s.deductAvailableBalance(ctx, p.Order.UserID, p.BalanceToDeduct)
		if err != nil {
			return fmt.Errorf("deduction: %w", err)
		}
		p.BalanceToDeduct = deducted
	}
	if p.DeductionType == payment.DeductionTypeSubscription && p.SubDaysToDeduct > 0 && p.SubscriptionID > 0 {
		if _, err := s.subscriptionSvc.ExtendSubscription(ctx, p.SubscriptionID, -p.SubDaysToDeduct); err != nil {
			if errors.Is(err, ErrAdjustWouldExpire) {
				if revokeErr := s.subscriptionSvc.RevokeSubscription(ctx, p.SubscriptionID); revokeErr != nil {
					return fmt.Errorf("revoke subscription: %w", revokeErr)
				}
			} else {
				return fmt.Errorf("deduct subscription days: %w", err)
			}
		}
	}
	if p.DeductionType == payment.DeductionTypeSubscription && p.SubDaysToDeduct > 0 && p.EntitlementID > 0 {
		if s.subscriptionEntitlementSvc == nil {
			return fmt.Errorf("deduct subscription entitlement days: entitlement service is not configured")
		}
		adjustment, err := s.subscriptionEntitlementSvc.ShortenForRefund(ctx, p.EntitlementID, p.SubDaysToDeduct, time.Now())
		if err != nil {
			return fmt.Errorf("deduct subscription entitlement days: %w", err)
		}
		p.EntitlementSnapshot = adjustment.Snapshot
		p.EntitlementRevoked = adjustment.Revoked
		p.EntitlementUpdatedAt = adjustment.UpdatedAt
	}
	return nil
}

func (s *PaymentService) finalizeRefundFailed(ctx context.Context, o *dbent.PaymentOrder, gErr error) (*RefundResult, error) {
	if s == nil || s.entClient == nil || o == nil {
		return nil, errors.New("payment database is not configured")
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin refund failure finalization: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)
	current, err := s.getPaymentOrderByIDForUpdate(txCtx, o.ID)
	if err != nil {
		return nil, err
	}
	plan := s.refundFinalizePlan(o)
	if s.refundCompletedExternally(txCtx, current, plan) {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit completed refund observation: %w", err)
		}
		return refundResultFromPlan(plan), nil
	}
	if current.Status != OrderStatusRefundPending {
		return nil, infraerrors.Conflict("REFUND_FINALIZE_CONFLICT", "refund status changed")
	}
	now := time.Now()
	if _, err := tx.PaymentOrder.Update().
		Where(paymentorder.IDEQ(o.ID), paymentorder.StatusEQ(OrderStatusRefundPending)).
		SetStatus(OrderStatusRefundFailed).
		SetFailedAt(now).
		SetFailedReason(psErrMsg(gErr)).
		Save(txCtx); err != nil {
		return nil, err
	}
	s.writeAuditLog(txCtx, o.ID, "REFUND_FAILED", "admin", map[string]any{"detail": psErrMsg(gErr)})
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit refund failure finalization: %w", err)
	}
	return &RefundResult{Success: false, Warning: "gateway refund failed: " + psErrMsg(gErr)}, nil
}

func (s *PaymentService) observePendingRefundQuery(ctx context.Context, p *RefundPlan, resp *payment.RefundResponse) (*RefundResult, error) {
	if s == nil || s.entClient == nil || p == nil {
		return nil, errors.New("payment database is not configured")
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)
	current, err := s.getPaymentOrderByIDForUpdate(txCtx, p.OrderID)
	if err != nil {
		return nil, err
	}
	if s.refundCompletedExternally(txCtx, current, p) {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return refundResultFromPlan(p), nil
	}
	if current.Status != OrderStatusRefundPending {
		return nil, infraerrors.Conflict("REFUND_FINALIZE_CONFLICT", "refund status changed")
	}
	s.writeAuditLog(txCtx, p.OrderID, "REFUND_QUERY_PENDING", "admin", map[string]any{"refundID": resp.RefundID})
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &RefundResult{Success: false, Warning: "gateway refund is still pending confirmation"}, nil
}

type refundPendingAuditDetail struct {
	RefundID                    string   `json:"refundID"`
	RefundAmount                float64  `json:"refundAmount"`
	GatewayAmount               float64  `json:"gatewayAmount"`
	DeductionRollbackOK         bool     `json:"deductionRollbackOK"`
	DeductBalance               *bool    `json:"deductBalance"`
	DeductionType               string   `json:"deductionType"`
	PreviousSettledRefundAmount float64  `json:"previousSettledRefundAmount"`
	BalanceRolledBack           *float64 `json:"balanceRolledBack"`
	SubDaysRolledBack           *int     `json:"subDaysRolledBack"`
}

const paymentOrderSnapshotRefundPending = "refund_pending"

func (s *PaymentService) latestRefundPendingDetail(ctx context.Context, o *dbent.PaymentOrder) refundPendingAuditDetail {
	if o == nil {
		return refundPendingAuditDetail{DeductionRollbackOK: true}
	}
	if o.ProviderSnapshot != nil {
		if raw, ok := o.ProviderSnapshot[paymentOrderSnapshotRefundPending]; ok {
			detail := refundPendingAuditDetail{}
			encoded, marshalErr := json.Marshal(raw)
			if marshalErr == nil && json.Unmarshal(encoded, &detail) == nil {
				detail.RefundID = strings.TrimSpace(detail.RefundID)
				return detail
			}
		}
	}
	oid := o.ID
	logEntry, err := s.entClient.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(oid, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
		Order(paymentauditlog.ByCreatedAt(sql.OrderDesc())).
		First(ctx)
	if err != nil || logEntry == nil {
		return refundPendingAuditDetail{DeductionRollbackOK: true}
	}
	detail := refundPendingAuditDetail{DeductionRollbackOK: true}
	_ = json.Unmarshal([]byte(logEntry.Detail), &detail)
	detail.RefundID = strings.TrimSpace(detail.RefundID)
	return detail
}

func refundPendingDetailFromOrder(o *dbent.PaymentOrder) (refundPendingAuditDetail, bool) {
	if o == nil || o.ProviderSnapshot == nil {
		return refundPendingAuditDetail{}, false
	}
	raw, ok := o.ProviderSnapshot[paymentOrderSnapshotRefundPending]
	if !ok {
		return refundPendingAuditDetail{}, false
	}
	detail := refundPendingAuditDetail{}
	encoded, err := json.Marshal(raw)
	if err != nil || json.Unmarshal(encoded, &detail) != nil {
		return refundPendingAuditDetail{}, false
	}
	detail.RefundID = strings.TrimSpace(detail.RefundID)
	return detail, true
}

// getRefundProvider creates a provider using the order's original instance config.
// Delegates to getOrderProvider which handles instance lookup and fallback.
func (s *PaymentService) getRefundProvider(ctx context.Context, o *dbent.PaymentOrder) (payment.Provider, error) {
	inst, err := s.getRefundOrderProviderInstance(ctx, o)
	if err != nil {
		return nil, err
	}
	if inst == nil {
		return nil, fmt.Errorf("refund provider instance is unavailable for order %d", o.ID)
	}
	return s.createProviderFromInstance(ctx, inst)
}

func (s *PaymentService) handleGwFail(ctx context.Context, p *RefundPlan, gErr error) (*RefundResult, error) {
	if s == nil || s.entClient == nil {
		return nil, errors.New("payment database is not configured")
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin refund failure handling: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)
	current, err := s.getPaymentOrderByIDForUpdate(txCtx, p.OrderID)
	if err != nil {
		return nil, err
	}
	if s.refundCompletedExternally(txCtx, current, p) {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit completed refund observation: %w", err)
		}
		return refundResultFromPlan(p), nil
	}
	if current.Status != OrderStatusRefunding {
		return nil, infraerrors.Conflict("CONFLICT", "refund status changed")
	}

	if s.RollbackRefund(txCtx, p, gErr) {
		if err := s.restoreRefundClaim(txCtx, current, p); err != nil {
			return nil, err
		}
		s.writeAuditLog(txCtx, p.OrderID, "REFUND_GATEWAY_FAILED", "admin", map[string]any{"detail": psErrMsg(gErr)})
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit refund rollback: %w", err)
		}
		s.invalidateFinalizedRefundSubscriptionCaches(p)
		return &RefundResult{Success: false, Warning: "gateway failed: " + psErrMsg(gErr) + ", rolled back"}, nil
	}

	now := time.Now()
	if _, err := tx.PaymentOrder.Update().
		Where(paymentorder.IDEQ(p.OrderID), paymentorder.StatusEQ(OrderStatusRefunding)).
		SetStatus(OrderStatusRefundFailed).
		SetFailedAt(now).
		SetFailedReason(psErrMsg(gErr)).
		Save(txCtx); err != nil {
		return nil, err
	}
	s.writeAuditLog(txCtx, p.OrderID, "REFUND_FAILED", "admin", map[string]any{"detail": psErrMsg(gErr)})
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit refund failure: %w", err)
	}
	return nil, infraerrors.InternalServer("REFUND_FAILED", psErrMsg(gErr))
}

func (s *PaymentService) markRefundOk(ctx context.Context, p *RefundPlan) (*RefundResult, error) {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		return s.markRefundOkLocked(ctx, p)
	}
	if s == nil || s.entClient == nil {
		return nil, errors.New("payment database is not configured")
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := s.markRefundOkLocked(dbent.NewTxContext(ctx, tx), p)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *PaymentService) markRefundOkLocked(ctx context.Context, p *RefundPlan) (*RefundResult, error) {
	current, err := s.getPaymentOrderByIDForUpdate(ctx, p.OrderID)
	if err != nil {
		return nil, err
	}
	if s.refundCompletedExternally(ctx, current, p) {
		return refundResultFromPlan(p), nil
	}
	if current.Status != OrderStatusRefunding {
		return nil, infraerrors.Conflict("CONFLICT", "refund status changed")
	}

	result := refundResultFromPlan(p)
	finalStatus := refundSuccessStatus(p)
	const referralSavepoint = "refund_referral_sync"
	txClient := s.paymentOrderClient(ctx)
	if _, err := txClient.ExecContext(ctx, "SAVEPOINT "+referralSavepoint); err != nil {
		return nil, fmt.Errorf("create referral refund savepoint: %w", err)
	}
	if syncErr := s.syncReferralRefund(ctx, p); syncErr != nil {
		if _, rollbackErr := txClient.ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+referralSavepoint); rollbackErr != nil {
			return nil, fmt.Errorf("sync referral refund: %w; rollback savepoint: %v", syncErr, rollbackErr)
		}
		if _, releaseErr := txClient.ExecContext(ctx, "RELEASE SAVEPOINT "+referralSavepoint); releaseErr != nil {
			return nil, fmt.Errorf("sync referral refund: %w; release savepoint: %v", syncErr, releaseErr)
		}
		if err := s.persistRefundSuccess(ctx, current, p, finalStatus); err != nil {
			return nil, fmt.Errorf("sync referral refund: %w; mark refund: %v", syncErr, err)
		}
		s.writeAuditLog(ctx, p.OrderID, "REFUND_REFERRAL_SYNC_FAILED", "admin", map[string]any{"detail": psErrMsg(syncErr)})
		result.Warning = "refund completed but referral sync failed: " + psErrMsg(syncErr)
		return result, nil
	}
	if _, err := txClient.ExecContext(ctx, "RELEASE SAVEPOINT "+referralSavepoint); err != nil {
		return nil, fmt.Errorf("release referral refund savepoint: %w", err)
	}
	if err := s.persistRefundSuccess(ctx, current, p, finalStatus); err != nil {
		return nil, err
	}
	return result, nil
}

func refundSuccessStatus(p *RefundPlan) string {
	if p != nil && p.Order != nil && p.RefundAmount < p.Order.Amount {
		return OrderStatusPartiallyRefunded
	}
	return OrderStatusRefunded
}

func refundResultFromPlan(p *RefundPlan) *RefundResult {
	if p == nil {
		return &RefundResult{Success: true}
	}
	return &RefundResult{
		Success:         true,
		BalanceDeducted: p.BalanceToDeduct,
		SubDaysDeducted: p.SubDaysToDeduct,
	}
}

func refundOrderCoversPlan(order *dbent.PaymentOrder, p *RefundPlan) bool {
	if order == nil || p == nil || !psIsSuccessfulRefundStatus(order.Status) {
		return false
	}
	return roundMoney(order.RefundAmount)+paymentAmountToleranceForCurrency(PaymentOrderCurrency(order)) >= roundMoney(p.RefundAmount)
}

func (s *PaymentService) refundCompletedExternally(ctx context.Context, order *dbent.PaymentOrder, p *RefundPlan) bool {
	return refundOrderCoversPlan(order, p) && s.hasAuditLog(ctx, order.ID, "EXTERNAL_REFUND_SYNCED")
}

func refundSnapshotWithoutInFlight(snapshot map[string]any) map[string]any {
	out := copyMap(snapshot)
	if out == nil {
		out = make(map[string]any)
	}
	delete(out, paymentOrderSnapshotRefundInFlight)
	return out
}

func refundSnapshotAfterExternalFinalization(snapshot map[string]any, fingerprint string) map[string]any {
	out := refundSnapshotWithoutInFlight(snapshot)
	delete(out, paymentOrderSnapshotRefundPending)
	return refundSnapshotWithNotificationFingerprint(out, fingerprint)
}

func refundSnapshotAwaitingWebhook(current *dbent.PaymentOrder, p *RefundPlan) map[string]any {
	out := copyMap(current.ProviderSnapshot)
	if out == nil {
		out = make(map[string]any)
	}
	detail, ok := refundInFlightDetailFromOrder(current)
	if !ok {
		pending, pendingOK := refundPendingDetailFromOrder(current)
		if pendingOK {
			deductBalance := true
			if pending.DeductBalance != nil {
				deductBalance = *pending.DeductBalance
			}
			detail = refundInFlightDetail{
				RefundAmount:                p.RefundAmount,
				GatewayAmount:               pending.GatewayAmount,
				ProviderRefundID:            pending.RefundID,
				PreviousRefundAmount:        pending.PreviousSettledRefundAmount,
				PreviousSettledRefundAmount: pending.PreviousSettledRefundAmount,
				DeductBalance:               deductBalance,
				DeductionType:               pending.DeductionType,
			}
			ok = true
		}
	}
	if ok {
		if detail.GatewayAmount <= 0 {
			detail.GatewayAmount = p.GatewayAmount
		}
		if strings.TrimSpace(p.ProviderRefundID) != "" {
			detail.ProviderRefundID = p.ProviderRefundID
		}
		detail.BalanceDeducted = p.BalanceToDeduct
		detail.SubDaysDeducted = p.SubDaysToDeduct
		out[paymentOrderSnapshotRefundInFlight] = detail
	}
	delete(out, paymentOrderSnapshotRefundPending)
	return out
}

func (s *PaymentService) restoreRefundClaim(ctx context.Context, current *dbent.PaymentOrder, p *RefundPlan) error {
	if current == nil || p == nil || p.Order == nil {
		return errors.New("missing refund claim snapshot")
	}
	detail, ok := refundInFlightDetailFromOrder(current)
	previousRefundAmount := p.Order.RefundAmount
	if ok {
		previousRefundAmount = detail.PreviousRefundAmount
	}
	restoreStatus := p.Order.Status
	if restoreStatus != OrderStatusCompleted && restoreStatus != OrderStatusRefundRequested && restoreStatus != OrderStatusRefundFailed {
		restoreStatus = OrderStatusCompleted
	}
	update := s.paymentOrderClient(ctx).PaymentOrder.Update().
		Where(paymentorder.IDEQ(p.OrderID), paymentorder.StatusEQ(OrderStatusRefunding)).
		SetStatus(restoreStatus).
		SetRefundAmount(previousRefundAmount).
		SetForceRefund(p.Order.ForceRefund).
		SetProviderSnapshot(refundSnapshotWithoutInFlight(current.ProviderSnapshot))
	if p.Order.RefundReason != nil {
		update.SetRefundReason(*p.Order.RefundReason)
	} else {
		update.ClearRefundReason()
	}
	if p.Order.RefundAt != nil {
		update.SetRefundAt(*p.Order.RefundAt)
	} else {
		update.ClearRefundAt()
	}
	if p.Order.FailedAt != nil {
		update.SetFailedAt(*p.Order.FailedAt)
	} else {
		update.ClearFailedAt()
	}
	if p.Order.FailedReason != nil {
		update.SetFailedReason(*p.Order.FailedReason)
	} else {
		update.ClearFailedReason()
	}
	updated, err := update.Save(ctx)
	if err != nil {
		return err
	}
	if updated != 1 {
		return infraerrors.Conflict("CONFLICT", "refund status changed")
	}
	return nil
}

func (s *PaymentService) persistRefundSuccess(ctx context.Context, current *dbent.PaymentOrder, p *RefundPlan, status string) error {
	now := time.Now()
	_, err := s.paymentOrderClient(ctx).PaymentOrder.UpdateOneID(p.OrderID).
		SetStatus(status).
		SetRefundAmount(p.RefundAmount).
		SetRefundReason(p.Reason).
		SetRefundAt(now).
		SetForceRefund(p.Force).
		SetProviderSnapshot(refundSnapshotAwaitingWebhook(current, p)).
		ClearFailedAt().
		ClearFailedReason().
		Save(ctx)
	if err != nil {
		return fmt.Errorf("mark refund: %w", err)
	}
	s.writeAuditLog(ctx, p.OrderID, "REFUND_SUCCESS", "admin", map[string]any{"refundAmount": p.RefundAmount, "reason": p.Reason, "balanceDeducted": p.BalanceToDeduct, "force": p.Force})
	return nil
}

func (s *PaymentService) syncReferralRefund(ctx context.Context, p *RefundPlan) error {
	if s.referralRefundSvc == nil || p == nil || p.Order == nil {
		return nil
	}
	if p.Order.OrderType != payment.OrderTypeBalance && p.Order.OrderType != payment.OrderTypeSubscription {
		return nil
	}

	rechargeOrder, err := s.referralRefundSvc.rechargeRepo.GetByProviderAndExternalOrderID(ctx, strings.TrimSpace(paymentReferralProviderKey(p.Order)), strings.TrimSpace(p.Order.OutTradeNo))
	if err != nil {
		if errors.Is(err, ErrRechargeOrderNotFound) {
			return nil
		}
		return err
	}

	settledGatewayAmount := referralSettlementReversalAmount(p.Order, rechargeOrder, p.GatewayAmount)
	refundedAmount := roundMoney(rechargeOrder.RefundedAmount + settledGatewayAmount)
	if refundedAmount <= 0 {
		return nil
	}
	paidAmount := roundMoney(rechargeOrder.PaidAmount)
	if paidAmount > 0 && refundedAmount > paidAmount {
		refundedAmount = paidAmount
	}

	_, _, err = s.referralRefundSvc.ApplyRefund(ctx, &RechargeRefundInput{
		RechargeOrderID:  rechargeOrder.ID,
		RefundedAmount:   refundedAmount,
		ChargebackAmount: roundMoney(rechargeOrder.ChargebackAmount),
	})
	return err
}

func (s *PaymentService) markRefundPending(ctx context.Context, p *RefundPlan, resp *payment.RefundResponse) (*RefundResult, error) {
	if s == nil || s.entClient == nil {
		return nil, errors.New("payment database is not configured")
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin pending refund transition: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)
	current, err := s.getPaymentOrderByIDForUpdate(txCtx, p.OrderID)
	if err != nil {
		return nil, err
	}
	if s.refundCompletedExternally(txCtx, current, p) {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return refundResultFromPlan(p), nil
	}
	if current.Status != OrderStatusRefunding {
		return nil, infraerrors.Conflict("REFUND_PENDING_CONFLICT", "refund status changed before pending state was saved")
	}

	balanceDeducted := p.BalanceToDeduct
	subDaysDeducted := p.SubDaysToDeduct
	inFlightDetail, _ := refundInFlightDetailFromOrder(current)
	deductRequested := p.DeductBalance || p.DeductionType != payment.DeductionTypeNone
	rollbackPreviouslyFailed := s.hasAuditLog(txCtx, p.OrderID, "REFUND_ROLLBACK_FAILED")
	rollbackOK := false
	if rollbackPreviouslyFailed {
		// A retry intentionally skips re-deduction after a prior rollback failure.
		// Preserve that unresolved state so the webhook/finalization path cannot
		// mistake the zeroed retry plan for a successful rollback.
		slog.Warn("preserving failed refund rollback state", "orderID", p.OrderID)
	} else {
		rollbackOK = s.RollbackRefund(txCtx, p, nil)
	}
	if rollbackOK {
		p.BalanceToDeduct = 0
		p.SubDaysToDeduct = 0
	}

	detail := map[string]any{
		"refundID":                    refundResponseID(resp),
		"refundAmount":                p.RefundAmount,
		"gatewayAmount":               p.GatewayAmount,
		"reason":                      p.Reason,
		"force":                       p.Force,
		"deductBalance":               deductRequested,
		"deductionType":               p.DeductionType,
		"previousSettledRefundAmount": inFlightDetail.PreviousSettledRefundAmount,
		"balanceDeducted":             p.BalanceToDeduct,
		"subDaysDeducted":             p.SubDaysToDeduct,
		"balanceRolledBack":           balanceDeducted,
		"subDaysRolledBack":           subDaysDeducted,
		"deductionRollbackOK":         rollbackOK,
	}
	providerSnapshot := refundSnapshotWithoutInFlight(current.ProviderSnapshot)
	if providerSnapshot == nil {
		providerSnapshot = make(map[string]any, 1)
	}
	providerSnapshot[paymentOrderSnapshotRefundPending] = detail
	updated, err := tx.PaymentOrder.Update().
		Where(paymentorder.IDEQ(p.OrderID), paymentorder.StatusEQ(OrderStatusRefunding)).
		SetStatus(OrderStatusRefundPending).
		SetRefundAmount(p.RefundAmount).
		SetRefundReason(p.Reason).
		SetProviderSnapshot(providerSnapshot).
		ClearRefundAt().
		SetForceRefund(p.Force).
		ClearFailedAt().
		ClearFailedReason().
		Save(txCtx)
	if err != nil {
		return nil, fmt.Errorf("mark refund pending: %w", err)
	}
	if updated != 1 {
		return nil, infraerrors.Conflict("REFUND_PENDING_CONFLICT", "refund status changed before pending state was saved")
	}
	s.writeAuditLog(txCtx, p.OrderID, "REFUND_PENDING", "admin", detail)
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit pending refund transition: %w", err)
	}
	s.invalidateFinalizedRefundSubscriptionCaches(p)

	warning := "gateway refund is pending confirmation"
	if !rollbackOK {
		warning += "; refund deduction rollback failed"
	}
	return &RefundResult{Success: false, Warning: warning}, nil
}

func refundResponseID(resp *payment.RefundResponse) string {
	if resp == nil {
		return ""
	}
	return strings.TrimSpace(resp.RefundID)
}

func (s *PaymentService) RollbackRefund(ctx context.Context, p *RefundPlan, gErr error) bool {
	if p.DeductionType == payment.DeductionTypeBalance && p.BalanceToDeduct > 0 {
		if err := s.userRepo.UpdateBalance(ctx, p.Order.UserID, p.BalanceToDeduct); err != nil {
			slog.Error("[CRITICAL] rollback failed", "orderID", p.OrderID, "amount", p.BalanceToDeduct, "error", err)
			s.writeAuditLog(ctx, p.OrderID, "REFUND_ROLLBACK_FAILED", "admin", map[string]any{"gatewayError": psErrMsg(gErr), "rollbackError": psErrMsg(err), "balanceDeducted": p.BalanceToDeduct})
			return false
		}
	}
	if p.DeductionType == payment.DeductionTypeSubscription && p.SubDaysToDeduct > 0 && p.SubscriptionID > 0 {
		if p.SubRevoked {
			if err := s.restoreRevokedSubscription(ctx, p); err != nil {
				slog.Error("[CRITICAL] subscription restore failed", "orderID", p.OrderID, "subID", p.SubscriptionID, "error", err)
				s.writeAuditLog(ctx, p.OrderID, "REFUND_ROLLBACK_FAILED", "admin", map[string]any{"gatewayError": psErrMsg(gErr), "rollbackError": psErrMsg(err), "subDaysDeducted": p.SubDaysToDeduct})
				return false
			}
			return true
		}
		if _, err := s.subscriptionSvc.ExtendSubscription(ctx, p.SubscriptionID, p.SubDaysToDeduct); err != nil {
			slog.Error("[CRITICAL] subscription rollback failed", "orderID", p.OrderID, "subID", p.SubscriptionID, "days", p.SubDaysToDeduct, "error", err)
			s.writeAuditLog(ctx, p.OrderID, "REFUND_ROLLBACK_FAILED", "admin", map[string]any{"gatewayError": psErrMsg(gErr), "rollbackError": psErrMsg(err), "subDaysDeducted": p.SubDaysToDeduct})
			return false
		}
	}
	if p.DeductionType == payment.DeductionTypeSubscription && p.SubDaysToDeduct > 0 && p.EntitlementID > 0 {
		if s.subscriptionEntitlementSvc == nil || p.EntitlementSnapshot == nil {
			slog.Error("[CRITICAL] subscription entitlement restore failed", "orderID", p.OrderID, "entitlementID", p.EntitlementID, "error", "missing entitlement snapshot")
			s.writeAuditLog(ctx, p.OrderID, "REFUND_ROLLBACK_FAILED", "admin", map[string]any{"gatewayError": psErrMsg(gErr), "rollbackError": "missing entitlement snapshot", "entitlementID": p.EntitlementID, "subDaysDeducted": p.SubDaysToDeduct})
			return false
		}
		if err := s.subscriptionEntitlementSvc.RestoreRefundSnapshot(ctx, p.EntitlementSnapshot, p.EntitlementUpdatedAt); err != nil {
			slog.Error("[CRITICAL] subscription entitlement rollback failed", "orderID", p.OrderID, "entitlementID", p.EntitlementID, "days", p.SubDaysToDeduct, "error", err)
			s.writeAuditLog(ctx, p.OrderID, "REFUND_ROLLBACK_FAILED", "admin", map[string]any{"gatewayError": psErrMsg(gErr), "rollbackError": psErrMsg(err), "entitlementID": p.EntitlementID, "subDaysDeducted": p.SubDaysToDeduct})
			return false
		}
	}
	return true
}

func (s *PaymentService) restoreRevokedSubscription(ctx context.Context, p *RefundPlan) error {
	if s == nil || s.subscriptionSvc == nil || s.subscriptionSvc.userSubRepo == nil || p == nil || p.SubscriptionSnapshot == nil {
		return fmt.Errorf("missing subscription snapshot")
	}

	snapshot := *p.SubscriptionSnapshot
	if err := s.subscriptionSvc.userSubRepo.Create(ctx, &snapshot); err != nil {
		existing, getErr := s.subscriptionSvc.userSubRepo.GetByUserIDAndGroupID(ctx, snapshot.UserID, snapshot.GroupID)
		if getErr != nil {
			return err
		}
		snapshot.ID = existing.ID
		if updateErr := s.subscriptionSvc.userSubRepo.Update(ctx, &snapshot); updateErr != nil {
			return updateErr
		}
	}

	s.subscriptionSvc.InvalidateSubCache(snapshot.UserID, snapshot.GroupID)
	if s.subscriptionSvc.billingCacheService != nil {
		userID, groupID := snapshot.UserID, snapshot.GroupID
		go func() {
			cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.subscriptionSvc.billingCacheService.InvalidateSubscription(cacheCtx, userID, groupID)
		}()
	}
	return nil
}
