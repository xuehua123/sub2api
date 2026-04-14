//go:build unit

package provider

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	stripe "github.com/stripe/stripe-go/v85"
	"github.com/stretchr/testify/require"
)

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
