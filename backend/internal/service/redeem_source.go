package service

import "strings"

const (
	sub2PaymentPageIdempotencyPrefix = "s2p_"
	sub2PaymentPageAutoCodePrefix    = "auto_"
)

type RedeemSourceDetectionInput struct {
	IdempotencyKey string
	Code           string
	Type           string
	GroupID        *int64
	ValidityDays   int
	Value          float64
}

type RedeemSourceContext struct {
	IdempotencyKey string

	Source          string
	ExternalOrderID string
	Trusted         bool

	LegacyGroupID      int64
	LegacyValidityDays int
	LegacyValue        float64

	IdempotencyKeyHasS2PPrefix bool
	CodeHasAutoPrefix          bool
	IdempotencySuffix          string
	CodeSuffix                 string
}

func DetectRedeemSourceContext(input RedeemSourceDetectionInput) RedeemSourceContext {
	idempotencyKey := strings.TrimSpace(input.IdempotencyKey)
	code := strings.TrimSpace(input.Code)
	sourceType := strings.TrimSpace(input.Type)

	ctx := RedeemSourceContext{
		IdempotencyKey: idempotencyKey,
	}
	if strings.HasPrefix(idempotencyKey, sub2PaymentPageIdempotencyPrefix) {
		ctx.IdempotencyKeyHasS2PPrefix = true
		ctx.IdempotencySuffix = strings.TrimPrefix(idempotencyKey, sub2PaymentPageIdempotencyPrefix)
	}
	if strings.HasPrefix(code, sub2PaymentPageAutoCodePrefix) {
		ctx.CodeHasAutoPrefix = true
		ctx.CodeSuffix = strings.TrimPrefix(code, sub2PaymentPageAutoCodePrefix)
	}

	if !ctx.IdempotencyKeyHasS2PPrefix ||
		!ctx.CodeHasAutoPrefix ||
		ctx.IdempotencySuffix == "" ||
		ctx.IdempotencySuffix != ctx.CodeSuffix ||
		sourceType != RedeemTypeSubscription ||
		input.GroupID == nil ||
		*input.GroupID <= 0 ||
		input.ValidityDays <= 0 ||
		input.Value <= 0 {
		return ctx
	}

	ctx.Source = SubscriptionPlanExternalMappingSourceSub2PaymentPage
	ctx.ExternalOrderID = ctx.IdempotencySuffix
	ctx.Trusted = true
	ctx.LegacyGroupID = *input.GroupID
	ctx.LegacyValidityDays = input.ValidityDays
	ctx.LegacyValue = input.Value
	return ctx
}

func (c RedeemSourceContext) IsSub2PaymentPageLegacy() bool {
	return c.Trusted && c.Source == SubscriptionPlanExternalMappingSourceSub2PaymentPage && c.ExternalOrderID != ""
}

func (c RedeemSourceContext) HasSub2PaymentPageSuffixMismatch() bool {
	return c.IdempotencyKeyHasS2PPrefix &&
		c.CodeHasAutoPrefix &&
		c.IdempotencySuffix != "" &&
		c.CodeSuffix != "" &&
		c.IdempotencySuffix != c.CodeSuffix
}
