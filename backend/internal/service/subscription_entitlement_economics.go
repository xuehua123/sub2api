package service

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
)

func (s *SubscriptionEntitlementService) attachEntitlementEconomics(ctx context.Context, ent *SubscriptionEntitlement) {
	if ent == nil {
		return
	}

	purchasePrice, purchaseCurrency := entitlementSnapshotPurchase(ent.PlanSnapshot)
	var plan *SubscriptionEntitlementPlan
	if ent.PlanID != nil && *ent.PlanID > 0 && s != nil && s.planRepo != nil {
		if loaded, err := s.planRepo.GetEntitlementPlan(ctx, *ent.PlanID); err == nil {
			plan = loaded
		}
	}
	if purchasePrice == nil && plan != nil && plan.Price > 0 {
		purchasePrice = &plan.Price
		purchaseCurrency = plan.Currency
	}
	if purchasePrice != nil && strings.TrimSpace(purchaseCurrency) == "" {
		purchaseCurrency = "CNY"
	}

	quotaUSD, quotaPeriod := entitlementEconomicsQuota(ent, plan)
	ent.PurchasePrice = cloneFloat64Ptr(purchasePrice)
	ent.PurchaseCurrency = strings.ToUpper(strings.TrimSpace(purchaseCurrency))
	ent.QuotaUSD = cloneFloat64Ptr(quotaUSD)
	ent.QuotaPeriod = quotaPeriod
	if purchasePrice != nil && *purchasePrice > 0 && quotaUSD != nil && *quotaUSD > 0 {
		purchaseCNY, ok := entitlementPurchaseCNY(*purchasePrice, ent.PurchaseCurrency, ent.PlanSnapshot)
		if ok {
			unitCost := purchaseCNY / *quotaUSD
			ent.UnitCostPerUSD = &unitCost
		}
	}
}

func entitlementPurchaseCNY(purchasePrice float64, purchaseCurrency string, snapshot map[string]any) (float64, bool) {
	switch strings.ToUpper(strings.TrimSpace(purchaseCurrency)) {
	case "", "CNY", "RMB":
		return purchasePrice, true
	case "USD":
		rate, ok := snapshotPositiveFloat(snapshot["purchase_cny_per_usd_rate"])
		if !ok {
			return 0, false
		}
		return purchasePrice * rate, true
	default:
		return 0, false
	}
}

func entitlementSnapshotPurchase(snapshot map[string]any) (*float64, string) {
	if snapshot == nil {
		return nil, ""
	}
	currency := firstSnapshotString(snapshot, "purchase_currency", "currency")
	for _, key := range []string{"purchase_price", "purchase_amount", "paid_amount", "pay_amount", "price"} {
		if price, ok := snapshotPositiveFloat(snapshot[key]); ok {
			return &price, currency
		}
	}
	return nil, currency
}

func entitlementEconomicsQuota(ent *SubscriptionEntitlement, plan *SubscriptionEntitlementPlan) (*float64, string) {
	if ent != nil {
		if quota, period := entitlementSnapshotQuota(ent.PlanSnapshot); quota != nil {
			return quota, period
		}
	}
	if plan != nil {
		if quota := positiveFloatPtr(plan.MonthlyLimitUSD); quota != nil {
			return quota, "monthly"
		}
		if quota := positiveFloatPtr(plan.WeeklyLimitUSD); quota != nil {
			return quota, "weekly"
		}
		if quota := positiveFloatPtr(plan.DailyLimitUSD); quota != nil {
			return quota, "daily"
		}
	}
	if ent != nil {
		if quota := positiveFloatPtr(ent.MonthlyLimitUSD); quota != nil {
			return quota, "monthly"
		}
		if quota := positiveFloatPtr(ent.WeeklyLimitUSD); quota != nil {
			return quota, "weekly"
		}
		if quota := positiveFloatPtr(ent.DailyLimitUSD); quota != nil {
			return quota, "daily"
		}
	}
	return nil, ""
}

func entitlementSnapshotQuota(snapshot map[string]any) (*float64, string) {
	if snapshot == nil {
		return nil, ""
	}
	if quota, ok := snapshotPositiveFloat(snapshot["monthly_limit_usd"]); ok {
		return &quota, "monthly"
	}
	if quota, ok := snapshotPositiveFloat(snapshot["weekly_limit_usd"]); ok {
		return &quota, "weekly"
	}
	if quota, ok := snapshotPositiveFloat(snapshot["daily_limit_usd"]); ok {
		return &quota, "daily"
	}
	return nil, ""
}

func positiveFloatPtr(value *float64) *float64 {
	if value == nil || *value <= 0 {
		return nil
	}
	return value
}

func firstSnapshotString(snapshot map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := snapshot[key]; ok {
			text := strings.TrimSpace(snapshotString(value))
			if text != "" {
				return text
			}
		}
	}
	return ""
}

func snapshotString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	default:
		return ""
	}
}

func snapshotPositiveFloat(value any) (float64, bool) {
	var out float64
	var err error
	switch v := value.(type) {
	case float64:
		out = v
	case float32:
		out = float64(v)
	case int:
		out = float64(v)
	case int64:
		out = float64(v)
	case int32:
		out = float64(v)
	case json.Number:
		out, err = v.Float64()
	case string:
		out, err = strconv.ParseFloat(strings.TrimSpace(v), 64)
	default:
		return 0, false
	}
	return out, err == nil && out > 0
}
