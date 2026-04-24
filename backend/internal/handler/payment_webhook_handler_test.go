//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteSuccessResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name            string
		providerKey     string
		wantCode        int
		wantContentType string
		wantBody        string
		checkJSON       bool
		wantJSONCode    string
		wantJSONMessage string
	}{
		{
			name:            "wxpay returns JSON with code SUCCESS",
			providerKey:     payment.TypeWxpay,
			wantCode:        http.StatusOK,
			wantContentType: "application/json",
			checkJSON:       true,
			wantJSONCode:    "SUCCESS",
			wantJSONMessage: "成功",
		},
		{
			name:            "stripe returns empty 200",
			providerKey:     payment.TypeStripe,
			wantCode:        http.StatusOK,
			wantContentType: "text/plain",
			wantBody:        "",
		},
		{
			name:            "easypay returns plain text success",
			providerKey:     payment.TypeEasyPay,
			wantCode:        http.StatusOK,
			wantContentType: "text/plain",
			wantBody:        "success",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			writeSuccessResponse(c, tt.providerKey)

			assert.Equal(t, tt.wantCode, w.Code)
			assert.Contains(t, w.Header().Get("Content-Type"), tt.wantContentType)

			if tt.checkJSON {
				var resp wxpaySuccessResponse
				err := json.Unmarshal(w.Body.Bytes(), &resp)
				require.NoError(t, err)
				assert.Equal(t, tt.wantJSONCode, resp.Code)
				assert.Equal(t, tt.wantJSONMessage, resp.Message)
			} else {
				assert.Equal(t, tt.wantBody, w.Body.String())
			}
		})
	}
}

func TestUnknownOrderWebhookAcksWithSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	wrapped := fmt.Errorf("%w: out_trade_no=sub2_missing_42", service.ErrOrderNotFound)
	require.True(t, errors.Is(wrapped, service.ErrOrderNotFound))
	require.False(t, errors.Is(errors.New("lookup order failed: connection refused"), service.ErrOrderNotFound))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	writeSuccessResponse(c, payment.TypeStripe)
	require.Equal(t, http.StatusOK, w.Code)
	require.Empty(t, w.Body.String())
}

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

func TestExtractOutTradeNo(t *testing.T) {
	tests := []struct {
		name        string
		providerKey string
		rawBody     string
		want        string
	}{
		{
			name:        "easypay query payload",
			providerKey: "easypay",
			rawBody:     "out_trade_no=sub2_123&trade_status=TRADE_SUCCESS",
			want:        "sub2_123",
		},
		{
			name:        "alipay query payload",
			providerKey: "alipay",
			rawBody:     "notify_time=2026-04-20+12%3A00%3A00&out_trade_no=sub2_456",
			want:        "sub2_456",
		},
		{
			name:        "unknown provider",
			providerKey: "wxpay",
			rawBody:     "{}",
			want:        "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, extractOutTradeNo(tt.rawBody, tt.providerKey))
		})
	}
}

func TestVerifyNotificationWithProvidersReturnsMatchedProvider(t *testing.T) {
	firstErr := errors.New("wrong provider")
	providers := []payment.Provider{
		webhookHandlerProviderStub{
			key:       payment.TypeWxpay,
			verifyErr: firstErr,
		},
		webhookHandlerProviderStub{
			key: payment.TypeWxpay,
			notification: &payment.PaymentNotification{
				OrderID: "sub2_42",
				TradeNo: "trade-42",
				Status:  payment.NotificationStatusSuccess,
			},
		},
	}

	providerKey, notification, err := verifyNotificationWithProviders(context.Background(), providers, "{}", map[string]string{"wechatpay-signature": "sig"})
	require.NoError(t, err)
	require.Equal(t, payment.TypeWxpay, providerKey)
	require.NotNil(t, notification)
	require.Equal(t, "sub2_42", notification.OrderID)
}

func TestVerifyNotificationWithProvidersFailsWhenAllProvidersReject(t *testing.T) {
	providers := []payment.Provider{
		webhookHandlerProviderStub{
			key:       payment.TypeWxpay,
			verifyErr: errors.New("verify failed a"),
		},
		webhookHandlerProviderStub{
			key:       payment.TypeWxpay,
			verifyErr: errors.New("verify failed b"),
		},
	}

	_, _, err := verifyNotificationWithProviders(context.Background(), providers, "{}", nil)
	require.Error(t, err)
}

type webhookHandlerProviderStub struct {
	key          string
	notification *payment.PaymentNotification
	verifyErr    error
}

func (p webhookHandlerProviderStub) Name() string        { return p.key }
func (p webhookHandlerProviderStub) ProviderKey() string { return p.key }
func (p webhookHandlerProviderStub) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.PaymentType(p.key)}
}
func (p webhookHandlerProviderStub) CreatePayment(context.Context, payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	panic("unexpected call")
}
func (p webhookHandlerProviderStub) QueryOrder(context.Context, string) (*payment.QueryOrderResponse, error) {
	panic("unexpected call")
}
func (p webhookHandlerProviderStub) VerifyNotification(context.Context, string, map[string]string) (*payment.PaymentNotification, error) {
	if p.verifyErr != nil {
		return nil, p.verifyErr
	}
	return p.notification, nil
}
func (p webhookHandlerProviderStub) Refund(context.Context, payment.RefundRequest) (*payment.RefundResponse, error) {
	panic("unexpected call")
}
