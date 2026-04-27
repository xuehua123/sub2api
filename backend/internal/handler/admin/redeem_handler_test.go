package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newCreateAndRedeemHandler creates a RedeemHandler with a non-nil (but minimal)
// RedeemService so that CreateAndRedeem's nil guard passes and we can test the
// parameter-validation layer that runs before any service call.
func newCreateAndRedeemHandler() *RedeemHandler {
	return &RedeemHandler{
		adminService:          newStubAdminService(),
		redeemService:         &service.RedeemService{}, // non-nil to pass nil guard
		referralRewardService: nil,
	}
}

// postCreateAndRedeemValidation calls CreateAndRedeem and returns the response
// status code. For cases that pass validation and proceed into the service layer,
// a panic may occur (because RedeemService internals are nil); this is expected
// and treated as "validation passed" (returns 0 to indicate panic).
func postCreateAndRedeemValidation(t *testing.T, handler *RedeemHandler, body any) (code int) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	jsonBytes, err := json.Marshal(body)
	require.NoError(t, err)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/admin/redeem-codes/create-and-redeem", bytes.NewReader(jsonBytes))
	c.Request.Header.Set("Content-Type", "application/json")

	defer func() {
		if r := recover(); r != nil {
			// Panic means we passed validation and entered service layer (expected for minimal stub).
			code = 0
		}
	}()
	handler.CreateAndRedeem(c)
	return w.Code
}

func TestCreateAndRedeem_TypeDefaultsToBalance(t *testing.T) {
	// 不传 type 字段时应默认 balance，不触发 subscription 校验。
	// 验证通过后进入 service 层会 panic（返回 0），说明默认值生效。
	h := newCreateAndRedeemHandler()
	code := postCreateAndRedeemValidation(t, h, map[string]any{
		"code":    "test-balance-default",
		"value":   10.0,
		"user_id": 1,
	})

	assert.NotEqual(t, http.StatusBadRequest, code,
		"omitting type should default to balance and pass validation")
}

func TestCreateAndRedeem_SubscriptionRequiresGroupID(t *testing.T) {
	h := newCreateAndRedeemHandler()
	code := postCreateAndRedeemValidation(t, h, map[string]any{
		"code":          "test-sub-no-group",
		"type":          "subscription",
		"value":         29.9,
		"user_id":       1,
		"validity_days": 30,
		// group_id 缺失
	})

	assert.Equal(t, http.StatusBadRequest, code)
}

func TestCreateAndRedeem_SubscriptionRequiresNonZeroValidityDays(t *testing.T) {
	groupID := int64(5)
	h := newCreateAndRedeemHandler()

	// zero should be rejected
	t.Run("zero", func(t *testing.T) {
		code := postCreateAndRedeemValidation(t, h, map[string]any{
			"code":          "test-sub-bad-days-zero",
			"type":          "subscription",
			"value":         29.9,
			"user_id":       1,
			"group_id":      groupID,
			"validity_days": 0,
		})

		assert.Equal(t, http.StatusBadRequest, code)
	})

	// negative should pass validation (used for refund/reduction)
	t.Run("negative_passes_validation", func(t *testing.T) {
		code := postCreateAndRedeemValidation(t, h, map[string]any{
			"code":          "test-sub-negative-days",
			"type":          "subscription",
			"value":         29.9,
			"user_id":       1,
			"group_id":      groupID,
			"validity_days": -7,
		})

		assert.NotEqual(t, http.StatusBadRequest, code,
			"negative validity_days should pass validation for refund")
	})
}

func TestCreateAndRedeem_SubscriptionValidParamsPassValidation(t *testing.T) {
	groupID := int64(5)
	h := newCreateAndRedeemHandler()
	code := postCreateAndRedeemValidation(t, h, map[string]any{
		"code":          "test-sub-valid",
		"type":          "subscription",
		"value":         29.9,
		"user_id":       1,
		"group_id":      groupID,
		"validity_days": 31,
	})

	assert.NotEqual(t, http.StatusBadRequest, code,
		"valid subscription params should pass validation")
}

func TestCreateAndRedeem_BalanceIgnoresSubscriptionFields(t *testing.T) {
	h := newCreateAndRedeemHandler()
	// balance 类型不传 group_id 和 validity_days，不应报 400
	code := postCreateAndRedeemValidation(t, h, map[string]any{
		"code":    "test-balance-no-extras",
		"type":    "balance",
		"value":   50.0,
		"user_id": 1,
	})

	assert.NotEqual(t, http.StatusBadRequest, code,
		"balance type should not require group_id or validity_days")
}

func TestBuildSub2ApiPayReferralCreditInput_Subscription(t *testing.T) {
	groupID := int64(12)
	paidAt := time.Date(2026, 4, 27, 5, 19, 40, 0, time.UTC)

	input := buildSub2ApiPayReferralCreditInput(CreateAndRedeemCodeRequest{
		Code:         "s2p_cmogqzd8p00r701p8wz32c113df6",
		Type:         service.RedeemTypeSubscription,
		Value:        68,
		UserID:       486,
		GroupID:      &groupID,
		ValidityDays: 31,
		Notes:        "sub2apipay subscription order:cmogqzd8p00r701p8wz3l9tgc",
	}, paidAt)

	require.NotNil(t, input)
	assert.Equal(t, int64(486), input.UserID)
	assert.Equal(t, "cmogqzd8p00r701p8wz3l9tgc", input.ExternalOrderID)
	assert.Equal(t, "sub2apipay", input.Provider)
	assert.Equal(t, service.RedeemTypeSubscription, input.Channel)
	assert.Equal(t, 68.0, input.PaidAmount)
	assert.Equal(t, 0.0, input.CreditedBalanceAmount)
	assert.True(t, input.SkipBalanceCredit)
	assert.Equal(t, "sub2apipay:cmogqzd8p00r701p8wz3l9tgc:referral", input.IdempotencyKey)
	assert.Equal(t, paidAt, *input.PaidAt)
	assert.Contains(t, input.MetadataJSON, `"source":"sub2apipay_create_and_redeem"`)
	assert.Contains(t, input.MetadataJSON, `"validity_days":31`)
}

func TestBuildSub2ApiPayReferralCreditInput_IgnoresManualRedeem(t *testing.T) {
	input := buildSub2ApiPayReferralCreditInput(CreateAndRedeemCodeRequest{
		Code:   "manual-code",
		Type:   service.RedeemTypeSubscription,
		Value:  68,
		UserID: 486,
		Notes:  "manual gift",
	}, time.Now())

	assert.Nil(t, input)
}
