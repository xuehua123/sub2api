//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"fmt"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"strconv"
	"testing"
	"time"
)

type businessFixRow struct {
	Paid, Refund, Consumption float64
	Subscription              float64 `json:"subscription_consumption"`
	BalanceUsage              float64 `json:"balance_consumption"`
	Cost, Profit              *float64
	Uncertain                 int `json:"uncertain_count"`
}
type businessFixReport struct {
	Items   []businessFixRow
	Total   int
	Summary struct{ Cost, Profit *float64 }
}
type businessFixFixture struct {
	ctx      context.Context
	client   *dbent.Client
	repo     *userBusinessRepository
	user     *service.User
	key      *service.APIKey
	account  *service.Account
	usage    *usageLogRepository
	q        service.UserBusinessQuery
	sequence int
}

func newBusinessFixFixture(t *testing.T) *businessFixFixture {
	tx := testEntTx(t)
	c := tx.Client()
	u := mustCreateUser(t, c, &service.User{Email: "fix@test.invalid"})
	k := mustCreateApiKey(t, c, &service.APIKey{UserID: u.ID, Key: "sk-business-fix", Name: "fix"})
	a := mustCreateAccount(t, c, &service.Account{Name: "fix"})
	start := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	return &businessFixFixture{ctx: context.Background(), client: c, repo: &userBusinessRepository{db: tx}, user: u, key: k, account: a, usage: newUsageLogRepositoryWithSQL(c, tx), q: service.UserBusinessQuery{UserBusinessParams: service.UserBusinessParams{Search: "fix@test.invalid", Page: 1, PageSize: 20, Sort: "consumption", Order: "desc", USDCNY: 7, CostMode: "estimate"}, Start: start, End: start.AddDate(0, 0, 1), Now: start.AddDate(0, 0, 2)}}
}
func (f *businessFixFixture) use(t *testing.T, at time.Time, source string, kind int8, amount float64) {
	f.sequence++
	_, e := f.usage.Create(f.ctx, &service.UsageLog{UserID: f.user.ID, APIKeyID: f.key.ID, AccountID: f.account.ID, RequestID: fmt.Sprintf("fix-%d", f.sequence), Model: "test", TotalCost: 1, ActualCost: amount, BillingType: kind, BillingSource: &source, CreatedAt: at})
	require.NoError(t, e)
}
func (f *businessFixFixture) order(t *testing.T, status string) *dbent.PaymentOrder {
	f.sequence++
	n := fmt.Sprintf("fix-%d", f.sequence)
	p, e := f.client.PaymentOrder.Create().SetUserID(f.user.ID).SetUserEmail(f.user.Email).SetUserName("fix").SetAmount(200).SetPayAmount(100).SetRechargeCode(n).SetOutTradeNo(n).SetPaymentType("alipay").SetPaymentTradeNo(n).SetStatus(status).SetOrderType("subscription").SetPaidAt(f.q.Start).SetExpiresAt(f.q.End).SetClientIP("127.0.0.1").SetSrcHost("test").Save(f.ctx)
	require.NoError(t, e)
	return p
}
func (f *businessFixFixture) report(t *testing.T) businessFixReport {
	raw, e := f.repo.Report(f.ctx, f.q)
	require.NoError(t, e)
	var r businessFixReport
	require.NoError(t, json.Unmarshal(raw, &r))
	return r
}
func TestUserBusinessFixConsumptionSourceAndPeriod(t *testing.T) {
	f := newBusinessFixFixture(t)
	for _, x := range []struct {
		s string
		k int8
		n float64
	}{{"entitlement_balance_fallback", 1, 7}, {"", 1, 3}, {"entitlement_quota", 1, 5}, {"balance", 0, 2}} {
		f.use(t, f.q.Start, x.s, x.k, x.n)
	}
	r := f.report(t)
	require.Equal(t, 8.0, r.Items[0].Subscription)
	require.Equal(t, 9.0, r.Items[0].BalanceUsage)
	require.Equal(t, 17.0, r.Items[0].Consumption)
	f.q.Start = f.q.Start.AddDate(0, 0, 1)
	f.q.End = f.q.End.AddDate(0, 0, 1)
	f.q.Filter = "used"
	require.Zero(t, f.report(t).Total)
	f.q.Filter = ""
	require.Equal(t, 1, f.report(t).Total)
}
func TestUserBusinessFixFailedPaymentNeedsEvidence(t *testing.T) {
	f := newBusinessFixFixture(t)
	f.use(t, f.q.Start, "balance", 0, 1)
	p := f.order(t, "FAILED")
	require.Zero(t, f.report(t).Items[0].Paid)
	_, e := f.client.PaymentAuditLog.Create().SetOrderID(strconv.FormatInt(p.ID, 10)).SetAction("ORDER_PAID").SetDetail(`{"paidAmount":100}`).Save(f.ctx)
	require.NoError(t, e)
	require.Equal(t, 100.0, f.report(t).Items[0].Paid)
}
func TestUserBusinessFixLegacyCardRealPayments(t *testing.T) {
	f := newBusinessFixFixture(t)
	g := mustCreateGroup(t, f.client, &service.Group{Name: "Legacy monthly"})
	p := f.order(t, "COMPLETED")
	_, e := f.client.PaymentOrder.UpdateOneID(p.ID).SetSubscriptionGroupID(g.ID).Save(f.ctx)
	require.NoError(t, e)
	sub, e := f.client.UserSubscription.Create().SetUserID(f.user.ID).SetGroupID(g.ID).SetStartsAt(f.q.Start).SetExpiresAt(f.q.Now.AddDate(0, 0, 30)).SetNotes("prefix payment order " + strconv.FormatInt(p.ID, 10) + " suffix").Save(f.ctx)
	require.NoError(t, e)
	f.q.UserID = f.user.ID
	read := func() *float64 {
		raw, e := f.repo.Detail(f.ctx, f.q)
		require.NoError(t, e)
		var d struct {
			Cards []struct {
				Paid *float64 `json:"paid_cny"`
			}
		}
		require.NoError(t, json.Unmarshal(raw, &d))
		require.Len(t, d.Cards, 1)
		return d.Cards[0].Paid
	}
	require.Nil(t, read())
	_, e = f.client.UserSubscription.UpdateOneID(sub.ID).SetNotes("before\r\npayment_order_id=" + strconv.FormatInt(p.ID, 10) + "\r\nafter").Save(f.ctx)
	require.NoError(t, e)
	paid := read()
	require.NotNil(t, paid)
	require.Equal(t, 100.0, *paid)
}
func TestUserBusinessFixRefundEvidenceAndTimestamp(t *testing.T) {
	f := newBusinessFixFixture(t)
	f.use(t, f.q.Start, "balance", 0, 1)
	p := f.order(t, "COMPLETED")
	at := f.q.Start.Add(time.Hour)
	_, e := f.client.PaymentOrder.UpdateOneID(p.ID).SetRefundAmount(40).SetProviderRefundAmount(40).SetRefundAt(at).Save(f.ctx)
	require.NoError(t, e)
	_, e = f.client.PaymentAuditLog.Create().SetOrderID(strconv.FormatInt(p.ID, 10)).SetAction("REFUND_SUCCESS").SetDetail(`{"refundAmount":40}`).SetCreatedAt(at.Add(time.Millisecond)).Save(f.ctx)
	require.NoError(t, e)
	r := f.report(t)
	require.NotNil(t, r.Items[0].Profit)
	require.Equal(t, 20.0, r.Items[0].Refund)
	require.Equal(t, 73.0, *r.Items[0].Profit)
	_, e = f.client.PaymentAuditLog.Create().SetOrderID(strconv.FormatInt(p.ID, 10)).SetAction("REFUND_EVENT_missing").SetDetail("{}").SetCreatedAt(at.Add(time.Hour)).Save(f.ctx)
	require.NoError(t, e)
	require.Nil(t, f.report(t).Items[0].Profit)
}
func TestUserBusinessFixStandaloneCNYRefund(t *testing.T) {
	f := newBusinessFixFixture(t)
	f.use(t, f.q.Start, "balance", 0, 1)
	_, e := f.client.RechargeOrder.Create().SetUserID(f.user.ID).SetExternalOrderID("standalone").SetProvider("manual").SetCurrency("CNY").SetPaidAmount(100).SetStatus("partially_refunded").SetPaidAt(f.q.Start).SetRefundedAmount(20).SetRefundedAt(f.q.Start.Add(time.Hour)).Save(f.ctx)
	require.NoError(t, e)
	r := f.report(t)
	require.Equal(t, 20.0, r.Items[0].Refund)
	require.Zero(t, r.Items[0].Uncertain)
	require.NotNil(t, r.Items[0].Profit)
	require.Equal(t, 73.0, *r.Items[0].Profit)
}
func TestUserBusinessFixHistoricalFX(t *testing.T) {
	f := newBusinessFixFixture(t)
	f.use(t, f.q.Start, "balance", 0, 1)
	f.use(t, f.q.Start.AddDate(0, 0, 1), "balance", 0, 1)
	f.q.End = f.q.End.AddDate(0, 0, 1)
	f.q.CostMode = "historical"
	r := f.report(t)
	require.Nil(t, r.Items[0].Cost)
	require.Nil(t, r.Summary.Cost)
	require.Nil(t, r.Items[0].Profit)
	f.q.UserID = f.user.ID
	raw, err := f.repo.Detail(f.ctx, f.q)
	require.NoError(t, err)
	var detail struct {
		Daily []struct{ Cost, Profit *float64 }
	}
	require.NoError(t, json.Unmarshal(raw, &detail))
	require.Nil(t, detail.Daily[0].Cost)
	require.Nil(t, detail.Daily[0].Profit)
	f.q.UserID = 0
	entries, err := f.repo.ListFX(f.ctx, f.q.Start, f.q.End.AddDate(0, 0, -1))
	require.NoError(t, err)
	var days []struct {
		Date string
		Rate *float64 `json:"usd_cny"`
	}
	require.NoError(t, json.Unmarshal(entries, &days))
	require.Len(t, days, 2)
	require.Nil(t, days[0].Rate)
	entry := service.UserBusinessFX{Date: "2026-09-24", Rate: 6, Source: "settlement proof"}
	require.NoError(t, f.repo.InsertFX(f.ctx, entry))
	require.NoError(t, f.repo.InsertFX(f.ctx, entry))
	require.Nil(t, f.report(t).Items[0].Cost)
	require.NoError(t, f.repo.InsertFX(f.ctx, service.UserBusinessFX{Date: "2026-09-25", Rate: 7, Source: "settlement proof"}))
	r = f.report(t)
	require.Equal(t, 13.0, *r.Items[0].Cost)
	require.Equal(t, -13.0, *r.Items[0].Profit)
	entry.Rate = 8
	require.Error(t, f.repo.InsertFX(f.ctx, entry))
	f.q.CostMode = "estimate"
	f.q.USDCNY = 8
	require.Equal(t, 16.0, *f.report(t).Items[0].Cost)
}

func TestUserBusinessFixStandaloneRefundSnapshotCannotInventDailyHistory(t *testing.T) {
	f := newBusinessFixFixture(t)
	f.use(t, f.q.Start, "balance", 0, 1)
	_, err := f.client.RechargeOrder.Create().SetUserID(f.user.ID).SetExternalOrderID("old-refund").SetProvider("manual").SetCurrency("CNY").SetPaidAmount(100).SetStatus("partially_refunded").SetPaidAt(f.q.Start.AddDate(0, 0, -7)).SetRefundedAmount(30).SetRefundedAt(f.q.Start.Add(time.Hour)).Save(f.ctx)
	require.NoError(t, err)
	r := f.report(t)
	require.Nil(t, r.Items[0].Profit)
	require.Greater(t, r.Items[0].Uncertain, 0)
}
