//go:build unit

package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func TestExtractOutTradeNo_ParsesAlipayFormPayload(t *testing.T) {
	got := extractOutTradeNo("out_trade_no=sub2_123&trade_no=2026abc", payment.TypeAlipay)
	require.Equal(t, "sub2_123", got)
}

func TestExtractOutTradeNo_ParsesStripeMetadataOrderID(t *testing.T) {
	raw := `{"type":"payment_intent.succeeded","data":{"object":{"metadata":{"orderId":"sub2_456"}}}}`
	got := extractOutTradeNo(raw, payment.TypeStripe)
	require.Equal(t, "sub2_456", got)
}

func TestExtractOutTradeNo_ParsesStripeNestedPaymentIntentMetadata(t *testing.T) {
	raw := `{"type":"charge.refunded","data":{"object":{"payment_intent":{"metadata":{"orderId":"sub2_nested"}}}}}`
	got := extractOutTradeNo(raw, payment.TypeStripe)
	require.Equal(t, "sub2_nested", got)
}

func TestExtractWebhookInstanceHint_PrefersExplicitQueryParam(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/v1/payment/webhook/wxpay?instance_id=42&provider_instance_id=84", nil)

	got := extractWebhookInstanceHint(req)
	require.Equal(t, "42", got)
}

func TestExtractWebhookInstanceHint_FallsBackToProviderInstanceID(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/v1/payment/webhook/wxpay?provider_instance_id=84", nil)

	got := extractWebhookInstanceHint(req)
	require.Equal(t, "84", got)
}
