//go:build unit

package provider

import (
	"bytes"
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	stripe "github.com/stripe/stripe-go/v85"
	"github.com/stretchr/testify/require"
)

type stripeRefundBackend struct {
	params []*stripe.RefundCreateParams
}

func (b *stripeRefundBackend) Call(_ string, _ string, _ string, params stripe.ParamsContainer, v stripe.LastResponseSetter) error {
	b.params = append(b.params, params.(*stripe.RefundCreateParams))
	refund := v.(*stripe.Refund)
	refund.ID = "re_123"
	refund.Status = stripe.RefundStatusSucceeded
	return nil
}

func (*stripeRefundBackend) CallStreaming(string, string, string, stripe.ParamsContainer, stripe.StreamingLastResponseSetter) error {
	return nil
}

func (*stripeRefundBackend) CallRaw(string, string, string, []byte, *stripe.Params, stripe.LastResponseSetter) error {
	return nil
}

func (*stripeRefundBackend) CallMultipart(string, string, string, string, *bytes.Buffer, *stripe.Params, stripe.LastResponseSetter) error {
	return nil
}

func (*stripeRefundBackend) SetMaxNetworkRetries(int64) {}

func TestStripeRefundUsesStableAmountSpecificIdempotencyKey(t *testing.T) {
	backend := &stripeRefundBackend{}
	client := stripe.NewClient("sk_test", stripe.WithBackends(&stripe.Backends{API: backend}))
	provider := &Stripe{
		config:      map[string]string{"currency": "CNY"},
		initialized: true,
		sc:          client,
	}

	refund := func(amount string) {
		_, err := provider.Refund(context.Background(), payment.RefundRequest{
			TradeNo: "pi_123",
			OrderID: "sub2_order_456",
			Amount:  amount,
		})
		require.NoError(t, err)
	}

	refund("12.34")
	refund("12.34")
	refund("12.35")

	require.Len(t, backend.params, 3)
	require.Equal(t, int64(1234), *backend.params[0].Amount)
	require.Equal(t, "re-sub2_order_456-1234", *backend.params[0].IdempotencyKey)
	require.Equal(t, backend.params[0].IdempotencyKey, backend.params[1].IdempotencyKey)
	require.Equal(t, int64(1235), *backend.params[2].Amount)
	require.Equal(t, "re-sub2_order_456-1235", *backend.params[2].IdempotencyKey)
	require.NotEqual(t, *backend.params[0].IdempotencyKey, *backend.params[2].IdempotencyKey)
}

func TestParseStripeChargeRefunded_PreservesTradeNoWithoutOrderMetadata(t *testing.T) {
	t.Parallel()

	notification, err := parseStripeChargeRefunded(&stripe.Event{
		Data: &stripe.EventData{Raw: []byte(`{
			"amount_refunded": 5000,
			"metadata": {},
			"payment_intent": {
				"id": "pi_refund_lookup",
				"metadata": {}
			}
		}`)},
	}, "raw")
	require.NoError(t, err)
	require.NotNil(t, notification)
	require.Equal(t, "pi_refund_lookup", notification.TradeNo)
	require.Empty(t, notification.OrderID)
	require.Equal(t, 50.0, notification.Amount)
	require.Equal(t, payment.NotificationAmountTotal, notification.AmountSemantic)
	require.Equal(t, payment.NotificationStatusRefunded, notification.Status)
}

func TestParseStripeChargeDispute_FallsBackAcrossMetadataLocations(t *testing.T) {
	t.Parallel()

	notification, err := parseStripeChargeDispute(&stripe.Event{
		Data: &stripe.EventData{Raw: []byte(`{
			"amount": 3600,
			"payment_intent": {
				"id": "pi_dispute_lookup",
				"metadata": {
					"orderId": "order_from_payment_intent"
				}
			},
			"charge": {
				"metadata": {},
				"payment_intent": {
					"id": "pi_nested_dispute_lookup",
					"metadata": {}
				}
			}
		}`)},
	}, "raw")
	require.NoError(t, err)
	require.NotNil(t, notification)
	require.Equal(t, "pi_dispute_lookup", notification.TradeNo)
	require.Equal(t, "order_from_payment_intent", notification.OrderID)
	require.Equal(t, 36.0, notification.Amount)
	require.Equal(t, payment.NotificationAmountTotal, notification.AmountSemantic)
	require.Equal(t, payment.NotificationStatusChargeback, notification.Status)
}
