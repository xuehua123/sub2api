//go:build integration

package repository

import (
	"context"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"strconv"
	"testing"
	"time"
)

func TestPaymentProfitUsesPaidOrdersNotConsumption(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	r := &upstreamConnectionRepository{client: client}
	start := time.Date(2036, 4, 10, 0, 0, 0, 0, time.FixedZone("CST", 8*3600))
	end := start.Add(48 * time.Hour)
	u := mustCreateUser(t, client, &service.User{Email: "profit-cash@example.invalid"})
	unused := mustCreateUser(t, client, &service.User{Email: "profit-no-usage@example.invalid"})
	key := mustCreateApiKey(t, client, &service.APIKey{UserID: u.ID, Key: "sk-profit-cash", Name: "k"})
	account := mustCreateAccount(t, client, &service.Account{Name: "profit-cost"})
	usage := newUsageLogRepositoryWithSQL(client, tx)
	rate := 0.5
	_, err := usage.Create(ctx, &service.UsageLog{UserID: u.ID, APIKeyID: key.ID, AccountID: account.ID, Model: "test", RequestID: "profit-cash", TotalCost: 10, ActualCost: 999, AccountRateMultiplier: &rate, CreatedAt: start})
	require.NoError(t, err)
	create := func(uid int64, kind, status string, at time.Time, paid float64) *dbent.PaymentOrder {
		p, e := client.PaymentOrder.Create().SetUserID(uid).SetUserEmail("profit@example.invalid").SetUserName("profit").SetAmount(1000).SetPayAmount(paid).SetRechargeCode("test").SetPaymentType("alipay").SetPaymentTradeNo("trade").SetStatus(status).SetOrderType(kind).SetPaidAt(at).SetExpiresAt(end).SetClientIP("127.0.0.1").SetSrcHost("test").Save(ctx)
		require.NoError(t, e)
		return p
	}
	balance := create(u.ID, "balance", "COMPLETED", start, 100)
	card := create(unused.ID, "subscription", "REFUNDED", start.Add(time.Hour), 150)
	// Mirror ledger for the same order must not add receipts twice.
	_, err = client.RechargeOrder.Create().SetUserID(unused.ID).SetExternalOrderID("mirror").SetProvider("alipay").SetCurrency("CNY").SetPaidAmount(150).SetStatus("credited").SetPaidAt(start).SetMetadataJSON("{\"payment_order_id\":" + strconv.FormatInt(card.ID, 10) + "}").Save(ctx)
	require.NoError(t, err)
	_, err = client.PaymentAuditLog.Create().SetOrderID(strconv.FormatInt(card.ID, 10)).SetAction("REFUND_SUCCESS").SetDetail(`{"refundAmount":100}`).SetCreatedAt(start.Add(2 * time.Hour)).Save(ctx)
	require.NoError(t, err)
	create(u.ID, "balance", "PENDING", start, 500)
	create(u.ID, "balance", "FAILED", start, 500)
	create(u.ID, "balance", "COMPLETED", start.Add(-time.Second), 500)
	create(u.ID, "balance", "COMPLETED", end, 500)
	// Include payment-only second day, even though there are no usage log rows.
	create(unused.ID, "subscription", "COMPLETED", start.Add(25*time.Hour), 80)
	rows, err := r.GetUsageProfitDays(ctx, start, end, "Asia/Shanghai")
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, "2036-04-10", rows[0].Date)
	require.Equal(t, 250.0, rows[0].Revenue)
	require.Equal(t, 15.0, rows[0].Refund)
	require.Equal(t, 5.0, rows[0].AccountCost)
	require.Zero(t, rows[0].UncertainCount)
	require.Equal(t, 80.0, rows[1].Revenue)
	require.Zero(t, rows[1].AccountCost)
	// Receipt remains after status transitions; preserve unknown refund evidence, never invent profit.
	_, err = client.PaymentAuditLog.Create().SetOrderID(strconv.FormatInt(balance.ID, 10)).SetAction("REFUND_EVENT_missing").SetDetail("{}").SetCreatedAt(start.Add(time.Hour)).Save(ctx)
	require.NoError(t, err)
	rows, err = r.GetUsageProfitDays(ctx, start, end, "Asia/Shanghai")
	require.NoError(t, err)
	require.Positive(t, rows[0].UncertainCount)
}
