//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"strconv"
	"testing"
	"time"
)

func TestUserBusinessMoneyAndPagination(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := &userBusinessRepository{db: tx}
	user := mustCreateUser(t, client, &service.User{Email: "business@test.invalid", Username: "business", Balance: 50})
	key := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-business", Name: "k"})
	account := mustCreateAccount(t, client, &service.Account{Name: "business"})
	start := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	usage := newUsageLogRepositoryWithSQL(client, tx)
	rate := 0.5
	for i, at := range []time.Time{start.Add(-time.Second), start, start.Add(time.Hour), end} {
		_, err := usage.Create(ctx, &service.UsageLog{UserID: user.ID, APIKeyID: key.ID, AccountID: account.ID, RequestID: "business-" + strconv.Itoa(i), Model: "test", ActualCost: 3, TotalCost: 10, AccountRateMultiplier: &rate, CreatedAt: at})
		require.NoError(t, err)
	}
	p, err := client.PaymentOrder.Create().SetUserID(user.ID).SetUserEmail(user.Email).SetUserName("business").SetAmount(200).SetPayAmount(100).SetRechargeCode("business").SetOutTradeNo("business").SetPaymentType("alipay").SetPaymentTradeNo("business").SetStatus("COMPLETED").SetOrderType("subscription").SetPaidAt(start).SetExpiresAt(end).SetClientIP("127.0.0.1").SetSrcHost("test").Save(ctx)
	require.NoError(t, err)
	_, err = client.RechargeOrder.Create().SetUserID(user.ID).SetExternalOrderID("business").SetProvider("payment").SetCurrency("CNY").SetPaidAmount(100).SetStatus("credited").SetPaidAt(start).SetMetadataJSON("{\"payment_order_id\":" + strconv.FormatInt(p.ID, 10) + "}").Save(ctx)
	require.NoError(t, err)
	_, err = client.PaymentAuditLog.Create().SetOrderID(strconv.FormatInt(p.ID, 10)).SetAction("REFUND_EVENT_test").SetDetail(`{"refundAmountTotal":40}`).SetCreatedAt(start.Add(2 * time.Hour)).Save(ctx)
	require.NoError(t, err)
	_, err = client.PaymentOrder.UpdateOneID(p.ID).SetRefundAmount(40).SetProviderRefundAmount(40).SetRefundAt(start.Add(2 * time.Hour)).Save(ctx)
	require.NoError(t, err)
	q := service.UserBusinessQuery{UserBusinessParams: service.UserBusinessParams{Search: "business@test.invalid", StartDate: "2026-09-24", EndDate: "2026-09-24", Page: 1, PageSize: 20, Sort: "consumption", Order: "desc"}, Start: start, End: end, Now: end.Add(12 * time.Hour)}
	raw, err := repo.Report(ctx, q)
	require.NoError(t, err)
	var report struct {
		Items []struct {
			ID                              int64
			Paid, Refund, Cost, Consumption float64
			Profit                          *float64
		}
		Total   int
		Summary map[string]any
	}
	require.NoError(t, json.Unmarshal(raw, &report))
	require.Equal(t, 1, report.Total)
	require.Len(t, report.Items, 1)
	row := report.Items[0]
	require.Equal(t, 100.0, row.Paid)
	require.Equal(t, 20.0, row.Refund)
	require.Equal(t, 10.0, row.Cost)
	require.Equal(t, 6.0, row.Consumption)
	require.NotNil(t, row.Profit)
	require.Equal(t, 70.0, *row.Profit)
	q.Page = 2
	raw, err = repo.Report(ctx, q)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &report))
	require.Empty(t, report.Items)
	require.Equal(t, 100.0, report.Summary["paid"])
	q.Page = 1
	q.UserID = user.ID
	raw, err = repo.Detail(ctx, q)
	require.NoError(t, err)
	var detail struct {
		Daily []struct {
			Paid, Cost, Consumption float64
			Profit                  *float64
		}
		Cards         []any
		PaymentsTotal int `json:"payments_total"`
	}
	require.NoError(t, json.Unmarshal(raw, &detail))
	require.Len(t, detail.Daily, 1)
	require.Equal(t, 100.0, detail.Daily[0].Paid)
	require.Equal(t, 70.0, *detail.Daily[0].Profit)

	// Current assets must include cards purchased after the queried period and deduplicate legacy aliases.
	group := mustCreateGroup(t, client, &service.Group{Name: "Card group"})
	legacy, err := client.UserSubscription.Create().SetUserID(user.ID).SetGroupID(group.ID).SetStartsAt(start).SetExpiresAt(end.AddDate(0, 0, 60)).Save(ctx)
	require.NoError(t, err)
	ent, err := client.SubscriptionEntitlement.Create().SetUserID(user.ID).SetName("Current card").SetLegacySubscriptionID(legacy.ID).SetStartsAt(start).SetExpiresAt(end.AddDate(0, 0, 60)).SetMonthlyLimitUsd(100).SetMonthlyUsageUsd(25).SetMonthlyWindowStart(start).Save(ctx)
	require.NoError(t, err)
	_, err = client.PaymentOrder.UpdateOneID(p.ID).SetSubscriptionEntitlementID(ent.ID).Save(ctx)
	require.NoError(t, err)
	q.UserID = user.ID
	raw, err = repo.Detail(ctx, q)
	require.NoError(t, err)
	var assets struct {
		Cards []struct {
			Paid      float64 `json:"paid_cny"`
			Remaining float64 `json:"monthly_remaining"`
		}
		Balance float64
	}
	require.NoError(t, json.Unmarshal(raw, &assets))
	require.Len(t, assets.Cards, 1)
	require.Equal(t, 100.0, assets.Cards[0].Paid)
	require.Equal(t, 75.0, assets.Cards[0].Remaining)
	// Successful refund checkpoints outside this window must form the baseline, not be charged again.
	_, err = client.PaymentAuditLog.Create().SetOrderID(strconv.FormatInt(p.ID, 10)).SetAction("REFUND_EVENT_before").SetDetail(`{"refundAmountTotal":20}`).SetCreatedAt(start.Add(-time.Hour)).Save(ctx)
	require.NoError(t, err)
	q.UserID = 0
	raw, err = repo.Report(ctx, q)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &report))
	require.Equal(t, 10.0, report.Items[0].Refund)
	require.Equal(t, 80.0, *report.Items[0].Profit)

	// Refund pending still represents a paid order; only settled checkpoints count as refund.
	_, err = client.PaymentOrder.UpdateOneID(p.ID).SetStatus("REFUND_PENDING").Save(ctx)
	require.NoError(t, err)
	raw, err = repo.Report(ctx, q)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &report))
	require.Equal(t, 100.0, report.Items[0].Paid)
	_, err = client.PaymentOrder.UpdateOneID(p.ID).SetStatus("COMPLETED").Save(ctx)
	require.NoError(t, err)
	// Current asset payment history must not be restricted to the historical report end date.
	_, err = client.PaymentOrder.Create().SetUserID(user.ID).SetUserEmail(user.Email).SetUserName("business").SetAmount(50).SetPayAmount(50).SetRechargeCode("business-later").SetOutTradeNo("business-later").SetPaymentType("alipay").SetPaymentTradeNo("business-later").SetStatus("COMPLETED").SetOrderType("subscription").SetSubscriptionEntitlementID(ent.ID).SetPaidAt(end.Add(time.Hour)).SetExpiresAt(end.AddDate(0, 0, 1)).SetClientIP("127.0.0.1").SetSrcHost("test").Save(ctx)
	require.NoError(t, err)
	q.UserID = user.ID
	raw, err = repo.Detail(ctx, q)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &assets))
	require.Equal(t, 150.0, assets.Cards[0].Paid)
	q.UserID = 0
	raw, err = repo.Report(ctx, q)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &report))
	require.Equal(t, 100.0, report.Items[0].Paid)
	// Missing successful refund audit cannot silently be promoted to an exact period profit.
	_, err = client.PaymentOrder.UpdateOneID(p.ID).SetRefundAmount(60).SetProviderRefundAmount(60).SetRefundAt(start.Add(3 * time.Hour)).Save(ctx)
	require.NoError(t, err)
	q.UserID = 0
	raw, err = repo.Report(ctx, q)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &report))
	require.Nil(t, report.Items[0].Profit)
	// A successful late audit closes the uncertainty gap; duplicate cumulative events are not added twice.
	for _, action := range []string{"REFUND_EVENT_final", "EXTERNAL_REFUND_SYNCED"} {
		_, err = client.PaymentAuditLog.Create().SetOrderID(strconv.FormatInt(p.ID, 10)).SetAction(action).SetDetail(`{"refundAmountTotal":60}`).SetCreatedAt(start.Add(3 * time.Hour)).Save(ctx)
		require.NoError(t, err)
	}
	foreign, err := client.PaymentOrder.Create().SetUserID(user.ID).SetUserEmail(user.Email).SetUserName("business").SetAmount(50).SetPayAmount(50).SetRechargeCode("business-usd").SetOutTradeNo("business-usd").SetPaymentType("stripe").SetPaymentTradeNo("business-usd").SetProviderSnapshot(map[string]any{"currency": "USD"}).SetStatus("COMPLETED").SetOrderType("balance").SetPaidAt(start).SetExpiresAt(end).SetClientIP("127.0.0.1").SetSrcHost("test").Save(ctx)
	require.NoError(t, err)
	raw, err = repo.Report(ctx, q)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &report))
	require.Equal(t, 100.0, report.Items[0].Paid)
	require.Nil(t, report.Items[0].Profit)
	_, err = client.RechargeOrder.Create().SetUserID(user.ID).SetExternalOrderID("business-usd").SetProvider("stripe").SetCurrency("CNY").SetPaidAmount(350).SetStatus("credited").SetPaidAt(start).SetMetadataJSON("{\"payment_order_id\":" + strconv.FormatInt(foreign.ID, 10) + "}").Save(ctx)
	require.NoError(t, err)
	raw, err = repo.Report(ctx, q)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &report))
	require.Equal(t, 450.0, report.Items[0].Paid)
	require.Equal(t, 20.0, report.Items[0].Refund)
	require.NotNil(t, report.Items[0].Profit)
	require.Equal(t, 420.0, *report.Items[0].Profit)

}
