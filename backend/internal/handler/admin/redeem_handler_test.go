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
	req := CreateAndRedeemCodeRequest{
		Code:         "auto_cmogqzd8p00r701p8wz3l9tgc",
		Type:         service.RedeemTypeSubscription,
		Value:        68,
		UserID:       486,
		GroupID:      &groupID,
		ValidityDays: 31,
		Notes:        "sub2apipay subscription order:untrusted-note-value",
	}
	sourceCtx := service.DetectRedeemSourceContext(service.RedeemSourceDetectionInput{
		IdempotencyKey: "s2p_cmogqzd8p00r701p8wz3l9tgc",
		Code:           req.Code,
		Type:           req.Type,
		GroupID:        req.GroupID,
		ValidityDays:   req.ValidityDays,
		Value:          req.Value,
	})

	input := buildSub2ApiPayReferralCreditInput(req, sourceCtx, paidAt)

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
	}, service.RedeemSourceContext{}, time.Now())

	assert.Nil(t, input)
}

func TestBuildSub2ApiPayReferralCreditInput_IgnoresNotesOnly(t *testing.T) {
	groupID := int64(12)
	req := CreateAndRedeemCodeRequest{
		Code:         "manual-code",
		Type:         service.RedeemTypeSubscription,
		Value:        68,
		UserID:       486,
		GroupID:      &groupID,
		ValidityDays: 31,
		Notes:        "sub2apipay subscription order:cmogqzd8p00r701p8wz3l9tgc",
	}
	sourceCtx := service.DetectRedeemSourceContext(service.RedeemSourceDetectionInput{
		IdempotencyKey: "",
		Code:           req.Code,
		Type:           req.Type,
		GroupID:        req.GroupID,
		ValidityDays:   req.ValidityDays,
		Value:          req.Value,
	})

	input := buildSub2ApiPayReferralCreditInput(req, sourceCtx, time.Now())

	assert.Nil(t, input)
}

func TestValidateCreateAndRedeemReplayRequiresExactFields(t *testing.T) {
	groupID := int64(12)
	existing := &service.RedeemCode{
		Code:         "auto_order_replay",
		Type:         service.RedeemTypeSubscription,
		Value:        68,
		Status:       service.StatusUsed,
		GroupID:      &groupID,
		ValidityDays: 31,
	}
	req := CreateAndRedeemCodeRequest{
		Code:         existing.Code,
		Type:         service.RedeemTypeSubscription,
		Value:        68,
		UserID:       486,
		GroupID:      &groupID,
		ValidityDays: 31,
	}
	sourceCtx := service.DetectRedeemSourceContext(service.RedeemSourceDetectionInput{
		IdempotencyKey: "s2p_order_replay",
		Code:           req.Code,
		Type:           req.Type,
		GroupID:        req.GroupID,
		ValidityDays:   req.ValidityDays,
		Value:          req.Value,
	})

	require.NoError(t, validateCreateAndRedeemReplay(existing, req, sourceCtx))

	req.Value = 69
	require.ErrorIs(t, validateCreateAndRedeemReplay(existing, req, sourceCtx), service.ErrRedeemCodeConflict)
	req.Value = 68
	req.ValidityDays = 30
	require.ErrorIs(t, validateCreateAndRedeemReplay(existing, req, sourceCtx), service.ErrRedeemCodeConflict)
	req.ValidityDays = 31
	otherGroupID := int64(13)
	req.GroupID = &otherGroupID
	require.ErrorIs(t, validateCreateAndRedeemReplay(existing, req, sourceCtx), service.ErrRedeemCodeConflict)
	req.GroupID = &groupID
	req.Type = service.RedeemTypeBalance
	require.ErrorIs(t, validateCreateAndRedeemReplay(existing, req, sourceCtx), service.ErrRedeemCodeConflict)
}

func TestValidateCreateAndRedeemReplayAllowsMappedPlanIDForPaymentPageReplay(t *testing.T) {
	groupID := int64(12)
	planID := int64(99)
	existing := &service.RedeemCode{
		Code:         "auto_order_replay",
		Type:         service.RedeemTypeSubscription,
		Value:        68,
		Status:       service.StatusUsed,
		GroupID:      &groupID,
		PlanID:       &planID,
		ValidityDays: 31,
	}
	req := CreateAndRedeemCodeRequest{
		Code:         existing.Code,
		Type:         service.RedeemTypeSubscription,
		Value:        68,
		UserID:       486,
		GroupID:      &groupID,
		ValidityDays: 31,
	}
	sourceCtx := service.DetectRedeemSourceContext(service.RedeemSourceDetectionInput{
		IdempotencyKey: "s2p_order_replay",
		Code:           req.Code,
		Type:           req.Type,
		GroupID:        req.GroupID,
		ValidityDays:   req.ValidityDays,
		Value:          req.Value,
	})

	require.NoError(t, validateCreateAndRedeemReplay(existing, req, sourceCtx))

	untrustedSource := service.RedeemSourceContext{}
	require.ErrorIs(t, validateCreateAndRedeemReplay(existing, req, untrustedSource), service.ErrRedeemCodeConflict)

	otherPlanID := int64(100)
	req.PlanID = &otherPlanID
	require.ErrorIs(t, validateCreateAndRedeemReplay(existing, req, sourceCtx), service.ErrRedeemCodeConflict)
}

func TestValidateCreateAndRedeemReplayRejectsSourceSuffixMismatch(t *testing.T) {
	groupID := int64(12)
	req := CreateAndRedeemCodeRequest{
		Code:         "auto_order_code",
		Type:         service.RedeemTypeSubscription,
		Value:        68,
		UserID:       486,
		GroupID:      &groupID,
		ValidityDays: 31,
	}
	sourceCtx := service.DetectRedeemSourceContext(service.RedeemSourceDetectionInput{
		IdempotencyKey: "s2p_other_order",
		Code:           req.Code,
		Type:           req.Type,
		GroupID:        req.GroupID,
		ValidityDays:   req.ValidityDays,
		Value:          req.Value,
	})

	err := validateCreateAndRedeemReplay(&service.RedeemCode{
		Code:         req.Code,
		Type:         req.Type,
		Value:        req.Value,
		GroupID:      req.GroupID,
		ValidityDays: req.ValidityDays,
	}, req, sourceCtx)

	require.ErrorIs(t, err, service.ErrRedeemCodeConflict)
}

func TestValidateCreateAndRedeemSubscriptionRequestRejectsInitialSourceSuffixMismatch(t *testing.T) {
	groupID := int64(12)
	req := CreateAndRedeemCodeRequest{
		Code:         "auto_order_code",
		Type:         service.RedeemTypeSubscription,
		Value:        68,
		UserID:       486,
		GroupID:      &groupID,
		ValidityDays: 31,
	}
	sourceCtx := service.DetectRedeemSourceContext(service.RedeemSourceDetectionInput{
		IdempotencyKey: "s2p_other_order",
		Code:           req.Code,
		Type:           req.Type,
		GroupID:        req.GroupID,
		ValidityDays:   req.ValidityDays,
		Value:          req.Value,
	})

	require.ErrorIs(t, validateCreateAndRedeemSubscriptionRequest(req, sourceCtx), service.ErrRedeemCodeConflict)
}

func TestResolveRedeemCodeExpiresAt_FromDays(t *testing.T) {
	days := 3
	expiresAt, err := resolveRedeemCodeExpiresAt(nil, &days)
	require.NoError(t, err)
	require.NotNil(t, expiresAt)
	require.WithinDuration(t, time.Now().UTC().AddDate(0, 0, days), *expiresAt, 2*time.Second)
}

func TestResolveRedeemCodeExpiresAt_RejectsPastAbsoluteTime(t *testing.T) {
	past := time.Now().UTC().Add(-time.Minute)
	expiresAt, err := resolveRedeemCodeExpiresAt(&past, nil)
	require.Error(t, err)
	require.Nil(t, expiresAt)
}

func TestResolveRedeemCodeExpiresAt_RejectsNonPositiveDays(t *testing.T) {
	days := 0
	expiresAt, err := resolveRedeemCodeExpiresAt(nil, &days)
	require.Error(t, err)
	require.Nil(t, expiresAt)
}

func TestResolveRedeemCodeExpiresAt_RejectsConflictingInputs(t *testing.T) {
	future := time.Now().UTC().Add(time.Hour)
	days := 3
	expiresAt, err := resolveRedeemCodeExpiresAt(&future, &days)
	require.Error(t, err)
	require.Nil(t, expiresAt)
}
