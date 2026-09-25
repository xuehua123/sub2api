//go:build integration

package repository

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestUserBusinessExternalCashierUsesActualMetadataSchema(t *testing.T) {
	f := newBusinessFixFixture(t)
	f.use(t, f.q.Start, "balance", 0, 1)
	// These keys match buildSub2ApiPayReferralCreditInput, not the native payment-order fixture.
	fixtures := []struct {
		id, channel, metadata string
		amount                float64
	}{
		{"cashier-card", "subscription", `{"source":"sub2apipay_create_and_redeem","redeem_type":"subscription","external_order_id":"cashier-card"}`, 199},
		{"cashier-wallet", "balance", `{"source":"sub2apipay_create_and_redeem","redeem_type":"balance","external_order_id":"cashier-wallet"}`, 50},
		{"cashier-channel", "subscription", `{"source":"sub2apipay_create_and_redeem"}`, 99},
		{"native-priority", "balance", `{"order_type":"subscription","redeem_type":"balance"}`, 20},
	}
	for _, x := range fixtures {
		_, err := f.client.RechargeOrder.Create().SetUserID(f.user.ID).SetExternalOrderID(x.id).SetProvider("sub2apipay").SetChannel(x.channel).SetCurrency("CNY").SetPaidAmount(x.amount).SetStatus("credited").SetPaidAt(f.q.Start).SetMetadataJSON(x.metadata).Save(f.ctx)
		require.NoError(t, err)
	}
	raw, err := f.repo.Report(f.ctx, f.q)
	require.NoError(t, err)
	var r struct {
		Items []struct {
			Paid         float64
			Balance      float64 `json:"balance_paid"`
			Subscription float64 `json:"subscription_paid"`
		}
	}
	require.NoError(t, json.Unmarshal(raw, &r))
	require.Len(t, r.Items, 1)
	require.Equal(t, 368.0, r.Items[0].Paid)
	require.Equal(t, 50.0, r.Items[0].Balance)
	require.Equal(t, 318.0, r.Items[0].Subscription)
	f.q.UserID = f.user.ID
	raw, err = f.repo.Detail(f.ctx, f.q)
	require.NoError(t, err)
	var d struct {
		Payments []struct {
			Type string `json:"order_type"`
			Paid float64
		}
	}
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Len(t, d.Payments, 4)
	for _, p := range d.Payments {
		if p.Paid == 199 || p.Paid == 99 || p.Paid == 20 {
			require.Equal(t, "subscription", p.Type)
		}
	}
}
func TestUserBusinessExternalCashierUnknownTypeIsNotBalance(t *testing.T) {
	f := newBusinessFixFixture(t)
	f.use(t, f.q.Start, "balance", 0, 1)
	_, err := f.client.RechargeOrder.Create().SetUserID(f.user.ID).SetExternalOrderID("unknown-type").SetProvider("sub2apipay").SetCurrency("CNY").SetPaidAmount(12).SetStatus("credited").SetPaidAt(f.q.Start).SetMetadataJSON(`{"redeem_type":"not-supported"}`).Save(f.ctx)
	require.NoError(t, err)
	raw, err := f.repo.Report(f.ctx, f.q)
	require.NoError(t, err)
	var r struct {
		Items []struct {
			Balance   float64 `json:"balance_paid"`
			Profit    *float64
			Uncertain int `json:"uncertain_count"`
		}
	}
	require.NoError(t, json.Unmarshal(raw, &r))
	require.Zero(t, r.Items[0].Balance)
	require.Nil(t, r.Items[0].Profit)
	require.Positive(t, r.Items[0].Uncertain)
}
