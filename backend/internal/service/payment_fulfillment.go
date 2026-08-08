package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	"entgo.io/ent/dialect"

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

var errAffiliateRefundReversalDisabled = errors.New("affiliate refund reversal is disabled until all pre-reversal application slots have stopped")

const (
	affiliateRefundReversalEnabledEnv = "AFFILIATE_REFUND_REVERSAL_ENABLED"

	paymentAuditActionReferralRewardSyncFailed     = "REFERRAL_REWARD_SYNC_FAILED"
	paymentAuditActionReferralRewardSyncRecovered  = "REFERRAL_REWARD_SYNC_RECOVERED"
	paymentAuditActionReferralRewardSkipped        = "REFERRAL_REWARD_SKIPPED"
	paymentAuditActionSubscriptionAssigned         = "SUBSCRIPTION_ASSIGNED"
	paymentAuditActionSubscriptionSuccess          = "SUBSCRIPTION_SUCCESS"
	paymentAuditActionReferralRefundSyncFailed     = "REFUND_REFERRAL_SYNC_FAILED"
	paymentAuditActionReferralRefundSyncRecovered  = "REFUND_REFERRAL_SYNC_RECOVERED"
	paymentAuditActionAffiliateRefundSyncFailed    = "REFUND_AFFILIATE_SYNC_FAILED"
	paymentAuditActionAffiliateRefundSyncRecovered = "REFUND_AFFILIATE_SYNC_RECOVERED"

	defaultFailedReferralRewardRecoveryLimit = 50
	maxFailedReferralRewardRecoveryScan      = 500
	defaultFailedRefundSyncRecoveryLimit     = 50
	maxFailedRefundSyncRecoveryScan          = 500
	failedRefundSyncRecoveryCursorSettingKey = "payment_refund_reward_sync_recovery_cursor"
	paymentFulfillmentLeaseDuration          = 5 * time.Minute
)

type paymentFulfillmentLease struct {
	version time.Time
}

type refundRewardSyncFailure struct {
	action string
	kind   string
	err    error
}

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
		currentOrder, err := s.getPaymentOrderByIDForUpdate(txCtx, o.ID)
		if err != nil {
			return err
		}
		fingerprint := refundNotificationFingerprint(n, pk, chargeback)
		eventAuditAction := refundNotificationAuditAction(fingerprint, chargeback)
		if refundNotificationAlreadyProcessed(currentOrder.ProviderSnapshot, fingerprint) ||
			(eventAuditAction != "" && s.hasAuditLog(txCtx, currentOrder.ID, eventAuditAction)) {
			return nil
		}
		settledBefore := paymentOrderReversalComponents(currentOrder)
		components := reconcilePaymentReversalComponents(currentOrder, reversalAmount, n.AmountSemantic, chargeback)
		inFlightDetail, hasInFlight, confirmsInFlight := externalRefundClaimState(currentOrder, n, chargeback, components)
		if confirmsInFlight &&
			components.ProviderRefundAmount+paymentAmountToleranceForCurrency(PaymentOrderCurrency(currentOrder)) < roundMoney(inFlightDetail.RefundAmount) {
			return fmt.Errorf(
				"refund notification provider-refund component %.8f is below in-flight refund target %.8f",
				components.ProviderRefundAmount,
				inFlightDetail.RefundAmount,
			)
		}
		refundAmountTotal := components.CombinedAmount
		if currentOrder.OrderType != payment.OrderTypeBalance && currentOrder.OrderType != payment.OrderTypeSubscription {
			return nil
		}

		var rewardSyncFailures []refundRewardSyncFailure
		referralErr, fatalErr := runRefundSyncSavepoint(txCtx, s.paymentOrderClient(txCtx), "external_refund_referral_sync", func() error {
			return s.syncReferralReversalToComponents(txCtx, currentOrder, components)
		})
		if fatalErr != nil {
			return fatalErr
		}
		if referralErr != nil {
			rewardSyncFailures = append(rewardSyncFailures, refundRewardSyncFailure{
				action: paymentAuditActionReferralRefundSyncFailed,
				kind:   "referral",
				err:    referralErr,
			})
		}

		affiliateErr, fatalErr := runRefundSyncSavepoint(txCtx, s.paymentOrderClient(txCtx), "external_refund_affiliate_sync", func() error {
			_, syncErr := s.syncAffiliateRebateReversal(txCtx, currentOrder, refundAmountTotal)
			return syncErr
		})
		if fatalErr != nil {
			return fatalErr
		}
		if affiliateErr != nil {
			rewardSyncFailures = append(rewardSyncFailures, refundRewardSyncFailure{
				action: paymentAuditActionAffiliateRefundSyncFailed,
				kind:   "affiliate",
				err:    affiliateErr,
			})
		}
		creditedDelta := 0.0
		deductionOrder := *currentOrder
		deductionOrder.RefundAmount = settledBefore.CombinedAmount

		switch currentOrder.OrderType {
		case payment.OrderTypeBalance:
			creditedDelta = components.CreditedDelta
			if confirmsInFlight {
				creditedDelta = roundMoney(creditedDelta - refundClaimBalanceDeduction(inFlightDetail))
				if creditedDelta < 0 {
					creditedDelta = 0
				}
			}
			if creditedDelta > 0 {
				if err := s.userRepo.DeductBalance(txCtx, currentOrder.UserID, creditedDelta); err != nil {
					return err
				}
			}
		case payment.OrderTypeSubscription:
			if confirmsInFlight {
				deductionOrder.RefundAmount = roundMoney(math.Min(
					refundAmountTotal,
					settledBefore.CombinedAmount+refundClaimCreditedDeduction(inFlightDetail),
				))
			}
			if err := s.syncExternalSubscriptionReversal(txCtx, &deductionOrder, refundAmountTotal); err != nil {
				return err
			}
		default:
			return nil
		}

		status := paymentReversalStatus(currentOrder, refundAmountTotal)
		now := time.Now()
		providerSnapshot := refundSnapshotAfterExternalFinalization(currentOrder.ProviderSnapshot, fingerprint)
		if hasInFlight && !confirmsInFlight {
			providerSnapshot = refundSnapshotWithNotificationFingerprint(currentOrder.ProviderSnapshot, fingerprint)
			status = currentOrder.Status
		}
		if _, err := s.paymentOrderClient(txCtx).PaymentOrder.UpdateOneID(currentOrder.ID).
			SetStatus(status).
			SetRefundAmount(refundAmountTotal).
			SetProviderRefundAmount(components.ProviderRefundAmount).
			SetChargebackAmount(components.ChargebackAmount).
			SetRefundAt(now).
			SetProviderSnapshot(providerSnapshot).
			Save(txCtx); err != nil {
			return err
		}
		for _, failure := range rewardSyncFailures {
			if err := s.writeAuditLogRequired(txCtx, currentOrder.ID, failure.action, pk, map[string]any{
				"detail":            psErrMsg(failure.err),
				"refundAmountTotal": refundAmountTotal,
				"source":            "external_notification",
			}); err != nil {
				return fmt.Errorf("persist external %s refund sync failure audit: %w", failure.kind, err)
			}
		}

		action := "EXTERNAL_REFUND_SYNCED"
		if chargeback {
			action = "EXTERNAL_CHARGEBACK_SYNCED"
		}
		detail := map[string]any{
			"gatewayAmount":           reversalAmount,
			"amountSemantic":          n.AmountSemantic,
			"creditedDelta":           creditedDelta,
			"providerRefundAmount":    components.ProviderRefundAmount,
			"chargebackAmount":        components.ChargebackAmount,
			"refundAmountTotal":       refundAmountTotal,
			"subscriptionDays":        subscriptionDaysRefundDelta(&deductionOrder, refundAmountTotal),
			"tradeNo":                 n.TradeNo,
			"providerEventID":         n.EventID,
			"providerRefundID":        n.RefundID,
			"status":                  n.Status,
			"confirmedInFlightRefund": confirmsInFlight,
		}
		s.writeAuditLog(txCtx, currentOrder.ID, action, pk, detail)
		if eventAuditAction != "" {
			detail["fingerprint"] = fingerprint
			s.writeAuditLog(txCtx, currentOrder.ID, eventAuditAction, pk, detail)
		}
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

func (s *PaymentService) syncReferralReversalToComponents(ctx context.Context, o *dbent.PaymentOrder, components paymentReversalComponents) error {
	if s.referralRefundSvc == nil || o == nil {
		return nil
	}
	if o.OrderType != payment.OrderTypeBalance && o.OrderType != payment.OrderTypeSubscription {
		return nil
	}
	rechargeOrder, err := s.referralRefundSvc.rechargeRepo.GetByProviderAndExternalOrderID(
		ctx,
		strings.TrimSpace(paymentReferralProviderKey(o)),
		strings.TrimSpace(o.OutTradeNo),
	)
	if err != nil {
		if errors.Is(err, ErrRechargeOrderNotFound) {
			return nil
		}
		return err
	}

	creditedTotal := roundMoney(o.Amount)
	paidAmount := roundMoney(rechargeOrder.PaidAmount)
	if creditedTotal <= 0 || paidAmount <= 0 {
		return fmt.Errorf("invalid referral reversal base for payment order %d", o.ID)
	}
	targetRefundedAmount := roundMoney(paidAmount * components.ProviderRefundAmount / creditedTotal)
	targetChargebackAmount := roundMoney(paidAmount * components.ChargebackAmount / creditedTotal)
	if targetRefundedAmount+targetChargebackAmount > paidAmount {
		targetChargebackAmount = roundMoney(math.Max(paidAmount-targetRefundedAmount, 0))
	}
	existingRefundedAmount := roundMoney(rechargeOrder.RefundedAmount)
	existingChargebackAmount := roundMoney(rechargeOrder.ChargebackAmount)
	if existingRefundedAmount > targetRefundedAmount+amountToleranceCNY ||
		existingChargebackAmount > targetChargebackAmount+amountToleranceCNY {
		return fmt.Errorf(
			"referral reversal components exceed payment order target: order=%d existing_refund=%.8f target_refund=%.8f existing_chargeback=%.8f target_chargeback=%.8f",
			o.ID,
			existingRefundedAmount,
			targetRefundedAmount,
			existingChargebackAmount,
			targetChargebackAmount,
		)
	}
	if existingRefundedAmount == targetRefundedAmount && existingChargebackAmount == targetChargebackAmount {
		return nil
	}
	_, _, err = s.referralRefundSvc.ApplyRefund(ctx, &RechargeRefundInput{
		RechargeOrderID:  rechargeOrder.ID,
		RefundedAmount:   targetRefundedAmount,
		ChargebackAmount: targetChargebackAmount,
	})
	return err
}

func (s *PaymentService) getPaymentOrderByIDForUpdate(ctx context.Context, orderID int64) (*dbent.PaymentOrder, error) {
	client := s.paymentOrderClient(ctx)
	query := client.PaymentOrder.Query().Where(paymentorder.IDEQ(orderID))
	if client.Driver().Dialect() == dialect.Postgres {
		query = query.ForUpdate()
	}
	return query.Only(ctx)
}

func externalRefundClaimState(
	order *dbent.PaymentOrder,
	n *payment.PaymentNotification,
	chargeback bool,
	components paymentReversalComponents,
) (refundInFlightDetail, bool, bool) {
	if order == nil {
		return refundInFlightDetail{}, false, false
	}
	detail, ok := refundInFlightDetailFromOrder(order)
	if !ok {
		pending, pendingOK := refundPendingDetailFromOrder(order)
		if !pendingOK || pending.RefundAmount <= 0 {
			return refundInFlightDetail{}, false, false
		}
		rollbackOK := pending.DeductionRollbackOK
		deductBalance := true
		if pending.DeductBalance != nil {
			deductBalance = *pending.DeductBalance
		}
		detail = refundInFlightDetail{
			RefundAmount:                pending.RefundAmount,
			GatewayAmount:               pending.GatewayAmount,
			ProviderRefundID:            pending.RefundID,
			PreviousRefundAmount:        pending.PreviousSettledRefundAmount,
			PreviousSettledRefundAmount: pending.PreviousSettledRefundAmount,
			DeductBalance:               deductBalance,
			DeductionType:               pending.DeductionType,
			DeductionRollbackOK:         &rollbackOK,
			BalanceDeducted:             pending.BalanceDeducted,
			SubDaysDeducted:             pending.SubDaysDeducted,
		}
	}
	if chargeback {
		return detail, true, false
	}
	if refundNotificationMatchesInFlight(n, detail) {
		return detail, true, true
	}

	expectedID := strings.TrimSpace(detail.ProviderRefundID)
	providedID := ""
	if n != nil {
		providedID = strings.TrimSpace(n.RefundID)
		if providedID == "" {
			providedID = strings.TrimSpace(n.EventID)
		}
	}
	if expectedID != "" && providedID != "" {
		return detail, true, false
	}
	confirms := components.ProviderRefundAmount+paymentAmountToleranceForCurrency(PaymentOrderCurrency(order)) >= roundMoney(detail.RefundAmount)
	return detail, true, confirms
}

func refundClaimCreditedDeduction(detail refundInFlightDetail) float64 {
	targetDelta := roundMoney(detail.RefundAmount - detail.PreviousSettledRefundAmount)
	if targetDelta < 0 {
		targetDelta = 0
	}
	switch detail.DeductionType {
	case payment.DeductionTypeNone:
		// The administrator explicitly chose not to reverse the credited
		// benefit, so a confirming webhook must not apply it implicitly.
		return targetDelta
	case payment.DeductionTypeBalance:
		if !detail.DeductBalance {
			return targetDelta
		}
		if detail.DeductionRollbackOK != nil && *detail.DeductionRollbackOK {
			return 0
		}
		if detail.BalanceDeducted > 0 {
			return roundMoney(detail.BalanceDeducted)
		}
		if detail.DeductBalance {
			return targetDelta
		}
		return targetDelta
	case payment.DeductionTypeSubscription:
		if detail.DeductionRollbackOK != nil {
			if *detail.DeductionRollbackOK {
				return 0
			}
			return targetDelta
		}
		if detail.SubDaysDeducted > 0 {
			return targetDelta
		}
		return 0
	default:
		// Compatibility for snapshots written before deductionType was stored.
		return targetDelta
	}
}

func refundClaimBalanceDeduction(detail refundInFlightDetail) float64 {
	if detail.DeductionType == payment.DeductionTypeSubscription {
		return 0
	}
	return refundClaimCreditedDeduction(detail)
}

func paymentReversalStatus(order *dbent.PaymentOrder, combinedAmount float64) string {
	if order != nil &&
		roundMoney(combinedAmount)+paymentAmountToleranceForCurrency(PaymentOrderCurrency(order)) < roundMoney(order.Amount) {
		return OrderStatusPartiallyRefunded
	}
	return OrderStatusRefunded
}

func refundNotificationMatchesInFlight(n *payment.PaymentNotification, detail refundInFlightDetail) bool {
	if n == nil {
		return false
	}
	expectedID := strings.TrimSpace(detail.ProviderRefundID)
	if expectedID == "" {
		return false
	}
	if expectedID == strings.TrimSpace(n.RefundID) || expectedID == strings.TrimSpace(n.TradeNo) {
		return true
	}
	return strings.Contains(n.RawData, expectedID)
}

func refundInFlightDetailFromOrder(order *dbent.PaymentOrder) (refundInFlightDetail, bool) {
	if order == nil || order.ProviderSnapshot == nil {
		return refundInFlightDetail{}, false
	}
	raw, ok := order.ProviderSnapshot[paymentOrderSnapshotRefundInFlight]
	if !ok {
		return refundInFlightDetail{}, false
	}
	detail := refundInFlightDetail{}
	encoded, err := json.Marshal(raw)
	if err != nil || json.Unmarshal(encoded, &detail) != nil || detail.RefundAmount <= 0 {
		return refundInFlightDetail{}, false
	}
	return detail, true
}

func psIsSuccessfulRefundStatus(status string) bool {
	return status == OrderStatusPartiallyRefunded || status == OrderStatusRefunded
}

const paymentOrderSnapshotRefundNotificationFingerprints = "refund_notification_fingerprints"

func refundNotificationFingerprint(n *payment.PaymentNotification, providerKey string, chargeback bool) string {
	if n == nil {
		return ""
	}
	eventKind := strconv.FormatBool(chargeback)
	providerKey = strings.TrimSpace(providerKey)
	orderID := strings.TrimSpace(n.OrderID)
	tradeNo := strings.TrimSpace(n.TradeNo)
	if eventID := strings.TrimSpace(n.EventID); eventID != "" {
		return hashRefundNotificationFingerprint(strings.Join([]string{
			providerKey,
			"event",
			eventID,
		}, "|"))
	}
	if refundID := strings.TrimSpace(n.RefundID); refundID != "" {
		return hashRefundNotificationFingerprint(strings.Join([]string{
			providerKey,
			"refund",
			refundID,
		}, "|"))
	}
	payload := strings.Join([]string{
		providerKey,
		eventKind,
		"fallback",
		strings.TrimSpace(n.Status),
		strings.TrimSpace(n.AmountSemantic),
		strconv.FormatFloat(n.Amount, 'g', -1, 64),
		orderID,
		tradeNo,
		strings.TrimSpace(n.RawData),
	}, "|")
	return hashRefundNotificationFingerprint(payload)
}

func hashRefundNotificationFingerprint(payload string) string {
	sum := sha256.Sum256([]byte(payload))
	return fmt.Sprintf("%x", sum[:])
}

func refundNotificationAuditAction(fingerprint string, chargeback bool) string {
	fingerprint = strings.TrimSpace(fingerprint)
	if fingerprint == "" {
		return ""
	}
	prefix := "REFUND_EVENT_"
	if chargeback {
		prefix = "CHARGEBACK_EVENT_"
	}
	maxFingerprintLength := 50 - len(prefix)
	if len(fingerprint) > maxFingerprintLength {
		fingerprint = fingerprint[:maxFingerprintLength]
	}
	return prefix + fingerprint
}

func refundNotificationAlreadyProcessed(snapshot map[string]any, fingerprint string) bool {
	if snapshot == nil || fingerprint == "" {
		return false
	}
	raw, ok := snapshot[paymentOrderSnapshotRefundNotificationFingerprints]
	if !ok {
		return false
	}
	var fingerprints []string
	encoded, err := json.Marshal(raw)
	if err != nil || json.Unmarshal(encoded, &fingerprints) != nil {
		return false
	}
	for _, existing := range fingerprints {
		if existing == fingerprint {
			return true
		}
	}
	return false
}

func refundSnapshotWithNotificationFingerprint(snapshot map[string]any, fingerprint string) map[string]any {
	out := copyMap(snapshot)
	if out == nil {
		out = make(map[string]any)
	}
	var fingerprints []string
	if raw, ok := out[paymentOrderSnapshotRefundNotificationFingerprints]; ok {
		encoded, err := json.Marshal(raw)
		if err == nil {
			_ = json.Unmarshal(encoded, &fingerprints)
		}
	}
	if fingerprint != "" {
		fingerprints = append(fingerprints, fingerprint)
	}
	const maxRefundNotificationFingerprints = 32
	if len(fingerprints) > maxRefundNotificationFingerprints {
		fingerprints = fingerprints[len(fingerprints)-maxRefundNotificationFingerprints:]
	}
	out[paymentOrderSnapshotRefundNotificationFingerprints] = fingerprints
	return out
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

type paymentReversalComponents struct {
	ProviderRefundAmount float64
	ChargebackAmount     float64
	CombinedAmount       float64
	CreditedDelta        float64
}

func paymentOrderReversalComponents(o *dbent.PaymentOrder) paymentReversalComponents {
	if o == nil {
		return paymentReversalComponents{}
	}
	creditedTotal := roundMoney(o.Amount)
	if creditedTotal <= 0 {
		return paymentReversalComponents{}
	}
	providerRefundAmount := roundMoney(o.ProviderRefundAmount)
	if providerRefundAmount < 0 {
		providerRefundAmount = 0
	}
	if providerRefundAmount > creditedTotal {
		providerRefundAmount = creditedTotal
	}
	chargebackAmount := roundMoney(o.ChargebackAmount)
	if chargebackAmount < 0 {
		chargebackAmount = 0
	}
	if chargebackAmount > creditedTotal-providerRefundAmount {
		chargebackAmount = roundMoney(creditedTotal - providerRefundAmount)
	}

	// Preserve pre-migration snapshots (and lightweight test fixtures that do
	// not apply SQL migrations) by treating refund_amount as the canonical
	// combined projection. During migration 197's expand phase, the database
	// bridge reconciles legacy projection-only writes at transaction commit.
	legacyProjection := settledRefundAmount(o)
	if legacyProjection > creditedTotal {
		legacyProjection = creditedTotal
	}
	componentTotal := roundMoney(providerRefundAmount + chargebackAmount)
	if legacyProjection > componentTotal {
		providerRefundAmount = roundMoney(providerRefundAmount + legacyProjection - componentTotal)
		if providerRefundAmount > creditedTotal-chargebackAmount {
			providerRefundAmount = roundMoney(creditedTotal - chargebackAmount)
		}
	}

	return paymentReversalComponents{
		ProviderRefundAmount: providerRefundAmount,
		ChargebackAmount:     chargebackAmount,
		CombinedAmount:       roundMoney(providerRefundAmount + chargebackAmount),
	}
}

func externalGatewayAmountToCredited(o *dbent.PaymentOrder, gatewayAmount float64) float64 {
	if o == nil || gatewayAmount <= 0 || math.IsNaN(gatewayAmount) || math.IsInf(gatewayAmount, 0) {
		return 0
	}
	creditedTotal := roundMoney(o.Amount)
	if creditedTotal <= 0 {
		return 0
	}
	paidTotal := roundMoney(o.PayAmount)
	if paidTotal <= 0 {
		paidTotal = creditedTotal
	}
	creditedAmount := gatewayAmount
	if math.Abs(paidTotal-creditedTotal) > amountToleranceCNY {
		creditedAmount = gatewayAmount * creditedTotal / paidTotal
	}
	creditedAmount = roundMoney(creditedAmount)
	if creditedAmount < 0 {
		return 0
	}
	if creditedAmount > creditedTotal {
		return creditedTotal
	}
	return creditedAmount
}

func reconcilePaymentReversalComponents(o *dbent.PaymentOrder, gatewayAmount float64, amountSemantic string, chargeback bool) paymentReversalComponents {
	components := paymentOrderReversalComponents(o)
	if o == nil {
		return components
	}
	previousCombined := components.CombinedAmount
	incomingCredited := externalGatewayAmountToCredited(o, gatewayAmount)
	if incomingCredited <= 0 {
		return components
	}

	existingComponent := components.ProviderRefundAmount
	otherComponent := components.ChargebackAmount
	if chargeback {
		existingComponent = components.ChargebackAmount
		otherComponent = components.ProviderRefundAmount
	}
	targetComponent := incomingCredited
	if amountSemantic != payment.NotificationAmountTotal {
		targetComponent = roundMoney(existingComponent + incomingCredited)
	} else if targetComponent < existingComponent {
		targetComponent = existingComponent
	}
	maxComponent := roundMoney(math.Max(roundMoney(o.Amount)-otherComponent, 0))
	if targetComponent > maxComponent {
		targetComponent = maxComponent
	}

	if chargeback {
		components.ChargebackAmount = targetComponent
	} else {
		components.ProviderRefundAmount = targetComponent
	}
	components.CombinedAmount = roundMoney(components.ProviderRefundAmount + components.ChargebackAmount)
	components.CreditedDelta = roundMoney(components.CombinedAmount - previousCombined)
	if components.CreditedDelta < 0 {
		components.CreditedDelta = 0
	}
	return components
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
	existingDays := proportionalSubscriptionDays(*o.SubscriptionDays, baseAmount, paymentOrderReversalComponents(o).CombinedAmount)
	if targetDays <= existingDays {
		return 0
	}
	return targetDays - existingDays
}

func (s *PaymentService) syncExternalSubscriptionReversal(ctx context.Context, o *dbent.PaymentOrder, refundAmountTotal float64) error {
	if s == nil || o == nil || o.OrderType != payment.OrderTypeSubscription {
		return nil
	}
	if o.SubscriptionDays == nil || *o.SubscriptionDays <= 0 {
		return nil
	}
	deltaDays := subscriptionDaysRefundDelta(o, refundAmountTotal)
	if deltaDays <= 0 {
		return nil
	}

	if o.SubscriptionEntitlementID != nil && *o.SubscriptionEntitlementID > 0 && s.subscriptionEntitlementSvc != nil {
		if _, err := s.subscriptionEntitlementSvc.ShortenForRefund(ctx, *o.SubscriptionEntitlementID, deltaDays, time.Now()); err != nil {
			if !errors.Is(err, ErrSubscriptionEntitlementNotFound) && !errors.Is(err, ErrSubscriptionEntitlementExpired) {
				return err
			}
		} else {
			return nil
		}
	}

	// An explicitly V2 order without a usable entitlement has not granted a
	// legacy subscription. Falling back here could shorten an unrelated one.
	if enabled, snapshotted := paymentOrderEntitlementV2Mode(o); snapshotted && enabled {
		return nil
	}

	if s.subscriptionSvc == nil || o.SubscriptionGroupID == nil {
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
	if math.Abs(paid-o.PayAmount) > paymentAmountToleranceForCurrency(PaymentOrderCurrency(o)) {
		s.writeAuditLog(ctx, o.ID, "PAYMENT_AMOUNT_MISMATCH", pk, map[string]any{"expected": o.PayAmount, "paid": paid, "tradeNo": tradeNo})
		return fmt.Errorf("amount mismatch: expected %s, got %s", strconv.FormatFloat(o.PayAmount, 'f', -1, 64), strconv.FormatFloat(paid, 'f', -1, 64))
	}
	return s.toPaid(ctx, o, tradeNo, paid, pk)
}

func paymentAmountToleranceForCurrency(currency string) float64 {
	minorUnit := payment.CurrencyMinorUnit(currency)
	if minorUnit <= 2 {
		return amountToleranceCNY
	}
	return math.Pow10(-minorUnit) / 2
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
	case OrderStatusFailed, OrderStatusPaid, OrderStatusRecharging:
		return s.executeFulfillment(ctx, o.ID)
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
	if o.Status != OrderStatusPaid && o.Status != OrderStatusFailed && o.Status != OrderStatusRecharging {
		return infraerrors.BadRequest("INVALID_STATUS", "order cannot fulfill in status "+o.Status)
	}
	lease, err := s.acquirePaymentFulfillmentLease(ctx, o)
	if err != nil {
		return err
	}
	if lease == nil {
		return nil
	}
	if err := s.doBalance(ctx, o, lease); err != nil {
		s.markFailed(ctx, oid, lease, err)
		return err
	}
	return nil
}

func (s *PaymentService) acquirePaymentFulfillmentLease(ctx context.Context, o *dbent.PaymentOrder) (*paymentFulfillmentLease, error) {
	if o == nil {
		return nil, infraerrors.BadRequest("INVALID_STATUS", "nil payment order")
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	staleBefore := now.Add(-paymentFulfillmentLeaseDuration)
	updated, err := s.entClient.PaymentOrder.Update().
		Where(
			paymentorder.IDEQ(o.ID),
			paymentorder.Or(
				paymentorder.StatusIn(OrderStatusPaid, OrderStatusFailed),
				paymentorder.And(
					paymentorder.StatusEQ(OrderStatusRecharging),
					paymentorder.UpdatedAtLTE(staleBefore),
				),
			),
		).
		SetStatus(OrderStatusRecharging).
		SetUpdatedAt(now).
		ClearFailedAt().
		ClearFailedReason().
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire fulfillment lease: %w", err)
	}
	if updated == 0 {
		current, getErr := s.entClient.PaymentOrder.Get(ctx, o.ID)
		if getErr != nil {
			return nil, fmt.Errorf("reload fulfillment lease: %w", getErr)
		}
		if current.Status == OrderStatusCompleted {
			return nil, nil
		}
		if current.Status == OrderStatusRecharging {
			return nil, infraerrors.Conflict("CONFLICT", "order is being processed")
		}
		return nil, infraerrors.Conflict("CONFLICT", "order status changed while acquiring fulfillment lease")
	}

	// Reload the persisted timestamp instead of trusting application clock precision.
	claimed, err := s.entClient.PaymentOrder.Get(ctx, o.ID)
	if err != nil {
		return nil, fmt.Errorf("reload acquired fulfillment lease: %w", err)
	}
	if claimed.Status != OrderStatusRecharging {
		return nil, infraerrors.Conflict("CONFLICT", "fulfillment lease was lost")
	}
	return &paymentFulfillmentLease{version: claimed.UpdatedAt}, nil
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

func (s *PaymentService) doBalance(ctx context.Context, o *dbent.PaymentOrder, lease *paymentFulfillmentLease) error {
	// Idempotency: check if redeem code already exists (from a previous partial run)
	existing, lookupErr := s.redeemService.GetByCode(ctx, o.RechargeCode)
	action := resolveRedeemAction(existing, lookupErr)

	switch action {
	case redeemActionSkipCompleted:
		// Code already created and redeemed — just mark completed
		return s.markCompleted(ctx, o, lease, "RECHARGE_SUCCESS")
	case redeemActionCreate:
		rc := &RedeemCode{Code: o.RechargeCode, Type: RedeemTypeBalance, Value: o.Amount, Status: StatusUnused}
		if err := s.redeemService.CreateCode(ctx, rc); err != nil {
			return fmt.Errorf("create redeem code: %w", err)
		}
	case redeemActionRedeem:
		// Code exists but unused — skip creation, proceed to redeem
	}
	if _, err := s.redeemService.Redeem(ContextSkipRedeemAffiliate(ctx), o.UserID, o.RechargeCode); err != nil {
		return fmt.Errorf("redeem balance: %w", err)
	}
	return s.markCompleted(ctx, o, lease, "RECHARGE_SUCCESS")
}

func (s *PaymentService) markCompleted(ctx context.Context, o *dbent.PaymentOrder, lease *paymentFulfillmentLease, auditAction string) error {
	if lease == nil {
		return errors.New("missing payment fulfillment lease")
	}
	if o == nil {
		return errors.New("missing payment order")
	}

	var (
		completedOrder *dbent.PaymentOrder
		shouldDispatch bool
		affiliateErr   error
	)
	apply := func(txCtx context.Context) error {
		currentOrder, err := s.getPaymentOrderByIDForUpdate(txCtx, o.ID)
		if err != nil {
			return fmt.Errorf("lock payment order before completion: %w", err)
		}
		if currentOrder.Status == OrderStatusCompleted {
			return nil
		}
		if currentOrder.Status != OrderStatusRecharging || !currentOrder.UpdatedAt.Equal(lease.version) {
			return infraerrors.Conflict("CONFLICT", "fulfillment lease was lost before completion")
		}

		referralErr := s.syncReferralReward(txCtx, currentOrder)
		if referralErr != nil && currentOrder.OrderType != payment.OrderTypeSubscription {
			return fmt.Errorf("sync referral reward: %w", referralErr)
		}
		if err := s.applyAffiliateRebateForOrder(txCtx, currentOrder); err != nil {
			affiliateErr = err
			return err
		}

		now := time.Now()
		updated, err := s.paymentOrderClient(txCtx).PaymentOrder.Update().Where(
			paymentorder.IDEQ(currentOrder.ID),
			paymentorder.StatusEQ(OrderStatusRecharging),
			paymentorder.UpdatedAtEQ(lease.version),
		).SetStatus(OrderStatusCompleted).SetCompletedAt(now).Save(txCtx)
		if err != nil {
			return fmt.Errorf("mark completed: %w", err)
		}
		if updated != 1 {
			return infraerrors.Conflict("CONFLICT", "fulfillment lease was lost before completion")
		}
		if referralErr != nil {
			s.writeAuditLog(txCtx, currentOrder.ID, paymentAuditActionReferralRewardSyncFailed, "system", map[string]any{
				"detail": referralErr.Error(),
			})
		}
		if !s.hasAuditLog(txCtx, currentOrder.ID, auditAction) {
			s.writeAuditLog(txCtx, currentOrder.ID, auditAction, "system", map[string]any{
				"rechargeCode":   currentOrder.RechargeCode,
				"creditedAmount": currentOrder.Amount,
				"payAmount":      currentOrder.PayAmount,
			})
			shouldDispatch = true
		}
		completedOrder = currentOrder
		return nil
	}

	if tx := dbent.TxFromContext(ctx); tx != nil {
		if err := apply(ctx); err != nil {
			return err
		}
	} else {
		tx, err := s.entClient.Tx(ctx)
		if err != nil {
			return fmt.Errorf("begin payment completion transaction: %w", err)
		}
		defer func() { _ = tx.Rollback() }()
		if err := apply(dbent.NewTxContext(ctx, tx)); err != nil {
			if affiliateErr != nil {
				s.writeAuditLog(ctx, o.ID, "AFFILIATE_REBATE_FAILED", "system", map[string]any{"error": affiliateErr.Error()})
			}
			return err
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit payment completion: %w", err)
		}
	}

	if shouldDispatch && completedOrder != nil {
		s.dispatchPaymentFulfillmentNotification(completedOrder, auditAction)
	}
	return nil
}

func (s *PaymentService) syncReferralReward(ctx context.Context, o *dbent.PaymentOrder) error {
	if s.referralRewardSvc == nil || o == nil {
		return nil
	}
	settlement, skipReason, err := s.resolveReferralSettlement(ctx, o)
	if err != nil {
		return err
	}
	if skipReason != "" {
		s.recordReferralRewardSkip(ctx, o, skipReason)
		return nil
	}

	switch o.OrderType {
	case payment.OrderTypeBalance:
		return s.syncBalanceReferralReward(ctx, o, settlement)
	case payment.OrderTypeSubscription:
		return s.syncSubscriptionReferralReward(ctx, o, settlement)
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

type referralSettlement struct {
	sourceCurrency string
	rate           float64
}

func (s referralSettlement) amount(value float64) float64 {
	return roundMoney(value * s.rate)
}

// resolveReferralSettlement translates a payment into the CNY-only referral
// ledger. USD subscription payments use the explicitly configured subscription
// rate; other foreign-currency orders are skipped rather than being recorded as
// a numerically identical CNY amount.
func (s *PaymentService) resolveReferralSettlement(ctx context.Context, o *dbent.PaymentOrder) (referralSettlement, string, error) {
	currency := PaymentOrderCurrency(o)
	if currency == ReferralSettlementCurrencyCNY {
		return referralSettlement{sourceCurrency: currency, rate: 1}, "", nil
	}
	if currency != "USD" {
		return referralSettlement{}, "referral settlement supports CNY and configured USD subscription payments only (payment currency " + currency + ")", nil
	}
	if o == nil || o.OrderType != payment.OrderTypeSubscription {
		return referralSettlement{}, "USD balance orders have no configured CNY referral settlement rate", nil
	}
	if rate, snapshotted := subscriptionReferralSettlementRateFromOrder(o); snapshotted {
		if rate <= 0 {
			return referralSettlement{}, "USD subscription payment had referral CNY settlement disabled when the order was created", nil
		}
		return referralSettlement{sourceCurrency: currency, rate: rate}, "", nil
	}
	if s == nil || s.configService == nil {
		return referralSettlement{}, "USD subscription payment has no payment configuration for CNY referral settlement", nil
	}
	cfg, err := s.configService.GetPaymentConfig(ctx)
	if err != nil {
		return referralSettlement{}, "", fmt.Errorf("load referral settlement payment config: %w", err)
	}
	rate := normalizeSubscriptionUSDToCNYRate(cfg.SubscriptionUSDToCNYRate)
	if rate <= 0 {
		return referralSettlement{}, "USD subscription payment requires subscription_usd_to_cny_rate before CNY referral settlement", nil
	}
	return referralSettlement{sourceCurrency: currency, rate: rate}, "", nil
}

func subscriptionReferralSettlementRateFromOrder(o *dbent.PaymentOrder) (float64, bool) {
	if o == nil || o.ProviderSnapshot == nil {
		return 0, false
	}
	raw, ok := o.ProviderSnapshot[paymentOrderSnapshotSubscriptionUSDToCNYRate]
	if !ok {
		return 0, false
	}
	rate, ok := snapshotPositiveFloat(raw)
	if !ok {
		return 0, true
	}
	return normalizeSubscriptionUSDToCNYRate(rate), true
}

func (s *PaymentService) recordReferralRewardSkip(ctx context.Context, o *dbent.PaymentOrder, reason string) {
	if s == nil || s.entClient == nil || o == nil {
		return
	}
	if s.hasAuditLog(ctx, o.ID, paymentAuditActionReferralRewardSkipped) {
		return
	}
	s.writeAuditLog(ctx, o.ID, paymentAuditActionReferralRewardSkipped, "system", map[string]any{
		"reason":              reason,
		"payment_currency":    PaymentOrderCurrency(o),
		"settlement_currency": ReferralSettlementCurrencyCNY,
	})
}

func referralSettlementMetadata(base map[string]any, settlement referralSettlement) (string, error) {
	base["payment_currency"] = settlement.sourceCurrency
	base["settlement_currency"] = ReferralSettlementCurrencyCNY
	base["settlement_rate"] = settlement.rate
	encoded, err := json.Marshal(base)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func (s *PaymentService) syncBalanceReferralReward(ctx context.Context, o *dbent.PaymentOrder, settlement referralSettlement) error {
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
	metadataJSON, err := referralSettlementMetadata(map[string]any{
		"order_type":       payment.OrderTypeBalance,
		"payment_order_id": o.ID,
	}, settlement)
	if err != nil {
		return err
	}

	_, err = s.referralRewardSvc.CreditRechargeOrder(ctx, &RechargeCreditInput{
		UserID:                o.UserID,
		ExternalOrderID:       o.OutTradeNo,
		Provider:              paymentReferralProviderKey(o),
		Channel:               o.PaymentType,
		Currency:              ReferralSettlementCurrencyCNY,
		GrossAmount:           settlement.amount(grossAmount),
		DiscountAmount:        settlement.amount(discountAmount),
		PaidAmount:            settlement.amount(paidAmount),
		GiftBalanceAmount:     settlement.amount(giftBalanceAmount),
		CreditedBalanceAmount: settlement.amount(creditedBalanceAmount),
		SkipBalanceCredit:     true,
		PaidAt:                o.PaidAt,
		MetadataJSON:          metadataJSON,
		Notes:                 fmt.Sprintf("payment order %d", o.ID),
	})
	return err
}

func (s *PaymentService) syncSubscriptionReferralReward(ctx context.Context, o *dbent.PaymentOrder, settlement referralSettlement) error {
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
	metadataJSON, err := referralSettlementMetadata(metadata, settlement)
	if err != nil {
		return err
	}

	_, err = s.referralRewardSvc.CreditRechargeOrder(ctx, &RechargeCreditInput{
		UserID:                o.UserID,
		ExternalOrderID:       o.OutTradeNo,
		Provider:              paymentReferralProviderKey(o),
		Channel:               o.PaymentType,
		Currency:              ReferralSettlementCurrencyCNY,
		GrossAmount:           settlement.amount(grossAmount),
		DiscountAmount:        0,
		PaidAmount:            settlement.amount(paidAmount),
		GiftBalanceAmount:     0,
		CreditedBalanceAmount: 0,
		SkipBalanceCredit:     true,
		PaidAt:                o.PaidAt,
		MetadataJSON:          metadataJSON,
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

		if err := s.syncReferralReward(ctx, order); err != nil {
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

func (s *PaymentService) dispatchPaymentFulfillmentNotification(o *dbent.PaymentOrder, auditAction string) {
	if s == nil || s.notificationEmailService == nil || o == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), emailSendTimeout)
		defer cancel()
		var err error
		switch auditAction {
		case "RECHARGE_SUCCESS":
			err = s.sendBalanceRechargeSuccessNotification(ctx, o)
		case "SUBSCRIPTION_SUCCESS":
			err = s.sendSubscriptionPurchaseSuccessNotification(ctx, o)
		default:
			return
		}
		if err != nil {
			slog.Warn("payment fulfillment notification email failed", "order_id", o.ID, "action", auditAction, "err", err.Error())
		}
	}()
}

func (s *PaymentService) sendBalanceRechargeSuccessNotification(ctx context.Context, o *dbent.PaymentOrder) error {
	currentBalance := ""
	if s.userRepo != nil {
		if user, err := s.userRepo.GetByID(ctx, o.UserID); err == nil && user != nil {
			currentBalance = fmt.Sprintf("%.2f", user.Balance)
		}
	}
	return s.notificationEmailService.Send(ctx, NotificationEmailSendInput{
		Event:          NotificationEmailEventBalanceRechargeSuccess,
		RecipientEmail: o.UserEmail,
		RecipientName:  firstNonEmpty(o.UserName, o.UserEmail),
		UserID:         o.UserID,
		SourceType:     "payment_order",
		SourceID:       strconv.FormatInt(o.ID, 10),
		Variables: map[string]string{
			"recharge_amount": fmt.Sprintf("%.2f", o.Amount),
			"current_balance": currentBalance,
			"order_id":        strconv.FormatInt(o.ID, 10),
		},
	})
}

func (s *PaymentService) sendSubscriptionPurchaseSuccessNotification(ctx context.Context, o *dbent.PaymentOrder) error {
	variables := map[string]string{
		"subscription_group": "Subscription",
		"subscription_days":  "",
		"expiry_time":        "",
		"order_id":           strconv.FormatInt(o.ID, 10),
	}
	if o.SubscriptionDays != nil {
		variables["subscription_days"] = strconv.Itoa(*o.SubscriptionDays)
	}
	if o.SubscriptionGroupID != nil {
		if s.groupRepo != nil {
			if group, err := s.groupRepo.GetByID(ctx, *o.SubscriptionGroupID); err == nil && group != nil && strings.TrimSpace(group.Name) != "" {
				variables["subscription_group"] = group.Name
			}
		}
		if s.subscriptionSvc != nil {
			if sub, err := s.subscriptionSvc.GetActiveSubscription(ctx, o.UserID, *o.SubscriptionGroupID); err == nil && sub != nil {
				variables["expiry_time"] = sub.ExpiresAt.Format("2006-01-02 15:04")
			}
		}
	}
	return s.notificationEmailService.Send(ctx, NotificationEmailSendInput{
		Event:          NotificationEmailEventSubscriptionPurchaseSuccess,
		RecipientEmail: o.UserEmail,
		RecipientName:  firstNonEmpty(o.UserName, o.UserEmail),
		UserID:         o.UserID,
		SourceType:     "payment_order",
		SourceID:       strconv.FormatInt(o.ID, 10),
		Variables:      variables,
	})
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
	if o.Status != OrderStatusPaid && o.Status != OrderStatusFailed && o.Status != OrderStatusRecharging {
		return infraerrors.BadRequest("INVALID_STATUS", "order cannot fulfill in status "+o.Status)
	}
	if !s.shouldUseSubscriptionEntitlementV2(ctx, o) && (o.SubscriptionGroupID == nil || o.SubscriptionDays == nil) {
		return infraerrors.BadRequest("INVALID_STATUS", "missing subscription info")
	}
	lease, err := s.acquirePaymentFulfillmentLease(ctx, o)
	if err != nil {
		return err
	}
	if lease == nil {
		return nil
	}
	if err := s.doSub(ctx, o, lease); err != nil {
		s.markFailed(ctx, oid, lease, err)
		return err
	}
	return nil
}

func (s *PaymentService) doSub(ctx context.Context, o *dbent.PaymentOrder, lease *paymentFulfillmentLease) error {
	if s.shouldUseSubscriptionEntitlementV2(ctx, o) {
		return s.doSubV2(ctx, o, lease)
	}
	return s.doSubLegacy(ctx, o, lease)
}

func (s *PaymentService) shouldUseSubscriptionEntitlementV2(ctx context.Context, o *dbent.PaymentOrder) bool {
	if s == nil || o == nil || o.PlanID == nil {
		return false
	}
	if enabled, snapshotted := paymentOrderEntitlementV2Mode(o); snapshotted {
		return enabled
	}
	if s.settingSvc == nil {
		return false
	}
	return s.settingSvc.GetSubscriptionEntitlementsRuntime(ctx).Enabled
}

func paymentOrderEntitlementV2Mode(o *dbent.PaymentOrder) (bool, bool) {
	if o == nil || o.ProviderSnapshot == nil {
		return false, false
	}
	raw, ok := o.ProviderSnapshot[paymentOrderSnapshotEntitlementV2Enabled]
	if !ok {
		return false, false
	}
	switch value := raw.(type) {
	case bool:
		return value, true
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(value))
		if err == nil {
			return parsed, true
		}
	}
	return false, false
}

func paymentOrderEntitlementPlanSnapshot(o *dbent.PaymentOrder) (*SubscriptionEntitlementPlan, bool, error) {
	if o == nil || o.ProviderSnapshot == nil {
		return nil, false, nil
	}
	raw, ok := o.ProviderSnapshot[paymentOrderSnapshotEntitlementPlan]
	if !ok {
		return nil, false, nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, true, fmt.Errorf("encode subscription entitlement plan snapshot: %w", err)
	}
	var snapshot paymentOrderEntitlementPlanSnapshotPayload
	if err := json.Unmarshal(encoded, &snapshot); err != nil {
		return nil, true, fmt.Errorf("decode subscription entitlement plan snapshot: %w", err)
	}
	if snapshot.PlanID <= 0 || o.PlanID == nil || snapshot.PlanID != *o.PlanID {
		return nil, true, errors.New("subscription entitlement plan snapshot does not match order")
	}
	groupIDs := uniquePositiveInt64s(snapshot.GroupIDs)
	if len(groupIDs) == 0 {
		return nil, true, errors.New("subscription entitlement plan snapshot has no groups")
	}
	groups := make([]SubscriptionEntitlementPlanGroup, 0, len(groupIDs))
	for index, groupID := range groupIDs {
		groups = append(groups, SubscriptionEntitlementPlanGroup{
			GroupID:   groupID,
			SortOrder: index,
			Enabled:   true,
		})
	}
	return &SubscriptionEntitlementPlan{
		ID:               snapshot.PlanID,
		GroupID:          snapshot.PrimaryGroupID,
		Name:             snapshot.Name,
		Description:      snapshot.Description,
		Price:            snapshot.Price,
		Currency:         snapshot.Currency,
		ValidityDays:     snapshot.ValidityDays,
		ValidityUnit:     snapshot.ValidityUnit,
		AccessScope:      snapshot.AccessScope,
		AllowedPlatforms: append([]string(nil), snapshot.AllowedPlatforms...),
		DailyLimitUSD:    cloneFloat64Ptr(snapshot.DailyLimitUSD),
		WeeklyLimitUSD:   cloneFloat64Ptr(snapshot.WeeklyLimitUSD),
		MonthlyLimitUSD:  cloneFloat64Ptr(snapshot.MonthlyLimitUSD),
		OveragePolicy:    snapshot.OveragePolicy,
		Features:         snapshot.Features,
		ProductName:      snapshot.ProductName,
		ForSale:          true,
		Groups:           groups,
	}, true, nil
}

func uniquePositiveInt64s(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	result := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func (s *PaymentService) validatePaymentEntitlementPlanSnapshotGroups(ctx context.Context, plan *SubscriptionEntitlementPlan) error {
	if plan == nil || len(plan.Groups) == 0 {
		return ErrSubscriptionEntitlementPlanInvalid
	}
	if s == nil || s.groupRepo == nil {
		return errors.New("subscription entitlement group repository is not configured")
	}
	for index := range plan.Groups {
		group, err := s.groupRepo.GetByID(ctx, plan.Groups[index].GroupID)
		if err != nil || group == nil || !group.IsActive() || !group.SupportsSubscriptionAccess() {
			return fmt.Errorf("subscription entitlement group %d is no longer available", plan.Groups[index].GroupID)
		}
		plan.Groups[index].Group = group
	}
	return nil
}

func (s *PaymentService) doSubV2(ctx context.Context, o *dbent.PaymentOrder, lease *paymentFulfillmentLease) error {
	if s.subscriptionEntitlementSvc == nil {
		return infraerrors.InternalServer("SUBSCRIPTION_ENTITLEMENT_SERVICE_MISSING", "subscription entitlement service is not configured")
	}
	if o == nil || o.PlanID == nil {
		return infraerrors.BadRequest("INVALID_STATUS", "missing subscription plan")
	}
	planSnapshot, snapshotted, err := paymentOrderEntitlementPlanSnapshot(o)
	if err != nil {
		return err
	}
	if snapshotted {
		if err := s.validatePaymentEntitlementPlanSnapshotGroups(ctx, planSnapshot); err != nil {
			return err
		}
	}

	input := AssignEntitlementFromPlanInput{
		UserID:                o.UserID,
		PlanID:                *o.PlanID,
		RequireEligibleGroups: true,
		PlanSnapshot:          planSnapshot,
		OrderID:               o.ID,
		SourceType:            SubscriptionEntitlementSourcePaymentOrder,
		Notes:                 subscriptionAssignmentMarker(o.ID),
		PurchasePrice:         &o.PayAmount,
		PurchaseCurrency:      PaymentOrderCurrency(o),
	}
	if input.PurchaseCurrency == "USD" {
		if rate, snapshotted := subscriptionReferralSettlementRateFromOrder(o); snapshotted && rate > 0 {
			input.PurchaseCNYPerUSDRate = &rate
		}
	}
	if o.SubscriptionDays != nil && *o.SubscriptionDays > 0 {
		input.ValidityDaysOverride = *o.SubscriptionDays
	}
	if o.PaidAt != nil {
		input.AssignedAt = *o.PaidAt
	}
	if outTradeNo := strings.TrimSpace(o.OutTradeNo); outTradeNo != "" {
		input.SourceExternalID = &outTradeNo
	}

	var (
		ent            *SubscriptionEntitlement
		reused         bool
		persistedOrder *dbent.PaymentOrder
	)
	apply := func(txCtx context.Context) error {
		currentOrder, err := s.getPaymentOrderByIDForUpdate(txCtx, o.ID)
		if err != nil {
			return fmt.Errorf("lock payment order before subscription entitlement assignment: %w", err)
		}
		if currentOrder.Status != OrderStatusRecharging || !currentOrder.UpdatedAt.Equal(lease.version) {
			return infraerrors.Conflict("CONFLICT", "fulfillment lease was lost before subscription entitlement assignment")
		}

		ent, reused, err = s.subscriptionEntitlementSvc.AssignOrExtendFromPlanTx(txCtx, input)
		if err != nil {
			return fmt.Errorf("assign subscription entitlement: %w", err)
		}
		if ent == nil || ent.ID <= 0 {
			return fmt.Errorf("assign subscription entitlement: missing entitlement")
		}

		update := s.currentClient(txCtx).PaymentOrder.Update().Where(
			paymentorder.IDEQ(o.ID),
			paymentorder.StatusEQ(OrderStatusRecharging),
			paymentorder.UpdatedAtEQ(lease.version),
		).
			SetSubscriptionEntitlementID(ent.ID).
			SetUpdatedAt(lease.version)
		if currentOrder.SubscriptionGroupID == nil && ent.PrimaryGroupID != nil {
			update.SetSubscriptionGroupID(*ent.PrimaryGroupID)
		}
		updated, err := update.Save(txCtx)
		if err != nil {
			return fmt.Errorf("set subscription entitlement id: %w", err)
		}
		if updated != 1 {
			return infraerrors.Conflict("CONFLICT", "fulfillment lease was lost while assigning subscription entitlement")
		}
		persistedOrder, err = s.currentClient(txCtx).PaymentOrder.Get(txCtx, o.ID)
		if err != nil {
			return fmt.Errorf("reload payment order after subscription entitlement assignment: %w", err)
		}
		s.writeAuditLog(txCtx, o.ID, paymentAuditActionSubscriptionAssigned, "system", map[string]any{
			"entitlementID": ent.ID,
			"planID":        *o.PlanID,
			"sourceType":    SubscriptionEntitlementSourcePaymentOrder,
			"sourceID":      o.ID,
			"reused":        reused,
		})
		return nil
	}

	if tx := dbent.TxFromContext(ctx); tx != nil {
		if err := apply(ctx); err != nil {
			return err
		}
	} else {
		tx, err := s.entClient.Tx(ctx)
		if err != nil {
			return fmt.Errorf("begin subscription entitlement transaction: %w", err)
		}
		defer func() { _ = tx.Rollback() }()
		if err := apply(dbent.NewTxContext(ctx, tx)); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit subscription entitlement transaction: %w", err)
		}
	}

	if persistedOrder == nil {
		return errors.New("subscription entitlement assignment did not persist the payment order")
	}
	o.UpdatedAt = lease.version
	o.SubscriptionEntitlementID = &ent.ID
	if o.SubscriptionGroupID == nil && ent.PrimaryGroupID != nil {
		o.SubscriptionGroupID = ent.PrimaryGroupID
	}
	return s.markCompleted(ctx, o, lease, paymentAuditActionSubscriptionSuccess)
}

func (s *PaymentService) doSubLegacy(ctx context.Context, o *dbent.PaymentOrder, lease *paymentFulfillmentLease) error {
	gid := *o.SubscriptionGroupID
	days := *o.SubscriptionDays
	g, err := s.groupRepo.GetByID(ctx, gid)
	if err != nil || g.Status != payment.EntityStatusActive {
		return fmt.Errorf("group %d no longer exists or inactive", gid)
	}
	if err := s.ensurePaymentSubscriptionAssigned(ctx, o, gid, days); err != nil {
		return err
	}
	return s.markCompleted(ctx, o, lease, paymentAuditActionSubscriptionSuccess)
}

func (s *PaymentService) ensurePaymentSubscriptionAssigned(ctx context.Context, o *dbent.PaymentOrder, groupID int64, days int) error {
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin subscription fulfillment tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	txCtx := dbent.NewTxContext(ctx, tx)
	txClient := tx.Client()
	alreadyAssigned, err := hasPaymentSubscriptionAssignmentAudit(txCtx, txClient, o.ID)
	if err != nil {
		return fmt.Errorf("check subscription assignment audit: %w", err)
	}

	recoveredFromNote := false
	assignmentChanged := false
	if !alreadyAssigned {
		orderNote := subscriptionAssignmentMarker(o.ID)
		existing, lookupErr := txClient.UserSubscription.Query().
			Where(usersubscription.UserIDEQ(o.UserID), usersubscription.GroupIDEQ(groupID)).
			Only(txCtx)
		if lookupErr == nil && existing != nil && existing.Notes != nil && subscriptionNotesContainAssignmentMarker(*existing.Notes, o.ID) {
			recoveredFromNote = true
		} else if lookupErr != nil && !dbent.IsNotFound(lookupErr) {
			return fmt.Errorf("check existing subscription assignment: %w", lookupErr)
		}

		// Local repository implementations and tests may own the subscription
		// record outside this Ent client. Fall back only when Ent has no row.
		if !recoveredFromNote && dbent.IsNotFound(lookupErr) && s.subscriptionSvc != nil {
			repoSub, repoErr := s.subscriptionSvc.userSubRepo.GetByUserIDAndGroupID(txCtx, o.UserID, groupID)
			switch {
			case repoErr == nil && repoSub != nil && subscriptionNotesContainAssignmentMarker(repoSub.Notes, o.ID):
				recoveredFromNote = true
			case repoErr != nil && !errors.Is(repoErr, ErrSubscriptionNotFound):
				return fmt.Errorf("check repository subscription assignment: %w", repoErr)
			}
		}

		if !recoveredFromNote {
			if s.subscriptionSvc == nil {
				return errors.New("subscription service is unavailable")
			}
			if _, _, err := s.subscriptionSvc.assignOrExtendSubscription(txCtx, &AssignSubscriptionInput{
				UserID:       o.UserID,
				GroupID:      groupID,
				ValidityDays: days,
				AssignedBy:   0,
				Notes:        orderNote,
			}, true); err != nil {
				return fmt.Errorf("assign subscription: %w", err)
			}
			assignmentChanged = true
		}

		detail, _ := json.Marshal(map[string]any{
			"groupID":           groupID,
			"validityDays":      days,
			"recoveredFromNote": recoveredFromNote,
		})
		if _, err := txClient.PaymentAuditLog.Create().
			SetOrderID(strconv.FormatInt(o.ID, 10)).
			SetAction("SUBSCRIPTION_ASSIGNED").
			SetDetail(string(detail)).
			SetOperator("system").
			Save(txCtx); err != nil {
			if dbent.IsConstraintError(err) {
				_ = tx.Rollback()
				claimed, checkErr := hasPaymentSubscriptionAssignmentAudit(ctx, s.entClient, o.ID)
				if checkErr == nil && claimed {
					if s.subscriptionSvc != nil {
						return s.subscriptionSvc.invalidateSubscriptionCachesBefore(o.UserID, groupID, 0)
					}
					return nil
				}
			}
			return fmt.Errorf("record subscription assignment audit: %w", err)
		}
	} else {
		slog.Info("subscription already assigned for order, skipping", "orderID", o.ID, "groupID", groupID)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit subscription fulfillment tx: %w", err)
	}
	if !assignmentChanged {
		return nil
	}
	// Assignment cache invalidation is deferred while this transaction is open,
	// then performed synchronously against the committed subscription.
	if err := s.subscriptionSvc.invalidateSubscriptionCachesBefore(o.UserID, groupID, 0); err != nil {
		return fmt.Errorf("invalidate subscription cache after fulfillment: %w", err)
	}
	return nil
}

func hasPaymentSubscriptionAssignmentAudit(ctx context.Context, client *dbent.Client, orderID int64) (bool, error) {
	count, err := client.PaymentAuditLog.Query().
		Where(
			paymentauditlog.OrderIDEQ(strconv.FormatInt(orderID, 10)),
			paymentauditlog.ActionIn("SUBSCRIPTION_ASSIGNED", "SUBSCRIPTION_SUCCESS"),
		).
		Limit(1).
		Count(ctx)
	return count > 0, err
}

func (s *PaymentService) hasAuditLog(ctx context.Context, orderID int64, action string) bool {
	found, err := s.hasAuditLogRequired(ctx, orderID, action)
	if err != nil {
		slog.Error("query payment audit log failed", "orderID", orderID, "action", action, "error", err)
		return false
	}
	return found
}

func (s *PaymentService) hasAuditLogRequired(ctx context.Context, orderID int64, action string) (bool, error) {
	oid := strconv.FormatInt(orderID, 10)
	c, err := s.paymentOrderClient(ctx).PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(oid), paymentauditlog.ActionEQ(action)).
		Limit(1).Count(ctx)
	return c > 0, err
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

func (s *PaymentService) applyAffiliateRebateForOrder(ctx context.Context, o *dbent.PaymentOrder) error {
	baseAmount := affiliateRebateBaseAmount(o)
	if o == nil || baseAmount <= 0 {
		return nil
	}
	if s.affiliateService == nil {
		return nil
	}

	apply := func(txCtx context.Context, client *dbent.Client) error {
		claimed, err := s.tryClaimAffiliateRebateAudit(txCtx, client, o.ID, baseAmount)
		if err != nil {
			return fmt.Errorf("claim affiliate rebate audit: %w", err)
		}
		if !claimed {
			return nil
		}

		sourceOrderID := o.ID
		rebateAmount, err := s.affiliateService.AccrueInviteRebateForOrder(txCtx, o.UserID, baseAmount, &sourceOrderID)
		if err != nil {
			return fmt.Errorf("accrue affiliate rebate: %w", err)
		}
		if rebateAmount <= 0 {
			if err := s.updateClaimedAffiliateRebateAudit(txCtx, client, o.ID, "AFFILIATE_REBATE_SKIPPED", map[string]any{
				"baseAmount": baseAmount,
				"reason":     "no inviter bound or rebate amount <= 0",
			}); err != nil {
				return fmt.Errorf("update affiliate rebate skipped audit: %w", err)
			}
			return nil
		}

		if err := s.updateClaimedAffiliateRebateAudit(txCtx, client, o.ID, "AFFILIATE_REBATE_APPLIED", map[string]any{
			"baseAmount":   baseAmount,
			"rebateAmount": rebateAmount,
		}); err != nil {
			return fmt.Errorf("update affiliate rebate applied audit: %w", err)
		}
		return nil
	}

	if tx := dbent.TxFromContext(ctx); tx != nil {
		return apply(ctx, tx.Client())
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		s.writeAuditLog(ctx, o.ID, "AFFILIATE_REBATE_FAILED", "system", map[string]any{"error": err.Error()})
		return fmt.Errorf("begin affiliate rebate tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := apply(dbent.NewTxContext(ctx, tx), tx.Client()); err != nil {
		s.writeAuditLog(ctx, o.ID, "AFFILIATE_REBATE_FAILED", "system", map[string]any{"error": err.Error()})
		return err
	}
	if err := tx.Commit(); err != nil {
		s.writeAuditLog(ctx, o.ID, "AFFILIATE_REBATE_FAILED", "system", map[string]any{"error": err.Error()})
		return fmt.Errorf("commit affiliate rebate tx: %w", err)
	}
	return nil
}

func affiliateRebateBaseAmount(o *dbent.PaymentOrder) float64 {
	if o == nil {
		return 0
	}
	switch o.OrderType {
	case payment.OrderTypeBalance, payment.OrderTypeSubscription:
		return o.Amount
	default:
		return 0
	}
}

func (s *PaymentService) syncAffiliateRebateReversal(ctx context.Context, o *dbent.PaymentOrder, refundAmountTotal float64) (*AffiliateRebateReversal, error) {
	baseAmount := affiliateRebateBaseAmount(o)
	if s == nil || o == nil || baseAmount <= 0 || refundAmountTotal <= 0 {
		return nil, nil
	}
	if math.IsNaN(refundAmountTotal) || math.IsInf(refundAmountTotal, 0) {
		return nil, errors.New("invalid cumulative affiliate refund amount")
	}
	applied, err := s.hasAuditLogRequired(ctx, o.ID, "AFFILIATE_REBATE_APPLIED")
	if err != nil {
		return nil, fmt.Errorf("query affiliate rebate audit for payment order %d: %w", o.ID, err)
	}
	if !applied {
		return nil, nil
	}
	if !s.affiliateReversalEnabled {
		return nil, errAffiliateRefundReversalDisabled
	}
	if s.affiliateService == nil {
		return nil, errors.New("affiliate service unavailable for rebate reversal")
	}

	refundRatio := refundAmountTotal / baseAmount
	if refundRatio > 1 {
		refundRatio = 1
	}
	reversal, err := s.affiliateService.ReverseInviteRebateForOrder(ctx, o.ID, refundRatio)
	if err != nil {
		return nil, fmt.Errorf("reverse affiliate rebate for payment order %d: %w", o.ID, err)
	}
	return reversal, nil
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
	query, args := buildAffiliateRebateAuditClaimQuery(client, oid, string(detail))
	rows, err := client.QueryContext(ctx, query, args...)
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

func buildAffiliateRebateAuditClaimQuery(client *dbent.Client, orderID, detail string) (string, []any) {
	nowExpr := paymentAuditCurrentTimestampExpr(client)
	if paymentAuditDialect(client) == dialect.Postgres {
		return fmt.Sprintf(`
INSERT INTO payment_audit_logs (order_id, action, detail, operator, created_at)
SELECT $1::text, 'AFFILIATE_REBATE_APPLIED', $2::text, 'system', %s
WHERE NOT EXISTS (
	SELECT 1
	FROM payment_audit_logs
	WHERE order_id = $1::text
	  AND action IN ('AFFILIATE_REBATE_APPLIED', 'AFFILIATE_REBATE_SKIPPED')
)
ON CONFLICT (order_id, action) DO NOTHING
RETURNING id`, nowExpr), []any{orderID, detail}
	}
	return fmt.Sprintf(`
INSERT INTO payment_audit_logs (order_id, action, detail, operator, created_at)
SELECT ?, 'AFFILIATE_REBATE_APPLIED', ?, 'system', %s
WHERE NOT EXISTS (
	SELECT 1
	FROM payment_audit_logs
	WHERE order_id = ?
	  AND action IN ('AFFILIATE_REBATE_APPLIED', 'AFFILIATE_REBATE_SKIPPED')
)
ON CONFLICT (order_id, action) DO NOTHING
RETURNING id`, nowExpr), []any{orderID, detail, orderID}
}

func paymentAuditCurrentTimestampExpr(client *dbent.Client) string {
	if paymentAuditDialect(client) == dialect.Postgres {
		return "NOW()"
	}
	return "CURRENT_TIMESTAMP"
}

func paymentAuditDialect(client *dbent.Client) string {
	if client == nil || client.Driver() == nil {
		return ""
	}
	return client.Driver().Dialect()
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

func (s *PaymentService) markFailed(ctx context.Context, oid int64, lease *paymentFulfillmentLease, cause error) {
	if lease == nil {
		slog.Error("mark FAILED without fulfillment lease", "orderID", oid)
		return
	}
	now := time.Now()
	r := psErrMsg(cause)
	// The lease version prevents a stale worker from overwriting a newer owner.
	c, e := s.entClient.PaymentOrder.Update().
		Where(
			paymentorder.IDEQ(oid),
			paymentorder.StatusEQ(OrderStatusRecharging),
			paymentorder.UpdatedAtEQ(lease.version),
		).
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
	if o.Status == OrderStatusCompleted {
		return infraerrors.BadRequest("INVALID_STATUS", "order already completed")
	}
	if o.Status != OrderStatusFailed && o.Status != OrderStatusPaid && o.Status != OrderStatusRecharging {
		return infraerrors.BadRequest("INVALID_STATUS", "only paid, failed, and recoverable recharging orders can retry")
	}
	s.writeAuditLog(ctx, oid, "RECHARGE_RETRY", "admin", map[string]any{"detail": "admin manual retry"})
	return s.executeFulfillment(ctx, oid)
}
