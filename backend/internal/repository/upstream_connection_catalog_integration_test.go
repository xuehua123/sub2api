//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"testing"
	"time"

	"github.com/lib/pq"
)

func TestUpstreamCatalogQueriesAndAnnotations(t *testing.T) {
	ctx := context.Background()
	repo := &upstreamConnectionRepository{client: integrationEntClient}
	c := &service.UpstreamConnection{Name: "catalog-test", Provider: "sub2api", AuthMode: "access_token", ManagementBaseURL: "https://catalog.invalid", CredentialEncrypted: "test", CredentialFingerprint: fmt.Sprint(time.Now().UnixNano()), Capabilities: map[string]any{}, Status: "ready", SyncEnabled: true, SyncIntervalSeconds: 60, Version: 1, WalletRaw: map[string]any{}}
	require.NoError(t, repo.Create(ctx, c))
	t.Cleanup(func() { _ = integrationEntClient.UpstreamConnection.DeleteOneID(c.ID).Exec(ctx) })
	now := time.Now().UTC()
	for i, name := range []string{"Alpha", "Beta", "Gamma"} {
		b := integrationEntClient.UpstreamGroup.Create().SetConnectionID(c.ID).SetRemoteID(fmt.Sprint(i)).SetName(name).SetObservedAt(now).SetFreshUntil(now.Add(time.Hour))
		if i < 2 {
			b.SetRateMultiplier(float64(i))
		}
		_, err := b.Save(ctx)
		require.NoError(t, err)
	}
	account, err := integrationEntClient.Account.Create().SetName("catalog-account").SetPlatform("openai").SetType(service.AccountTypeAPIKey).Save(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = integrationEntClient.Account.DeleteOneID(account.ID).Exec(ctx) })
	_, err = integrationEntClient.UpstreamAccountBinding.Create().SetAccountID(account.ID).SetConnectionID(c.ID).SetRemoteGroupID("0").SetRemoteGroupName("wrong-name").Save(ctx)
	require.NoError(t, err)
	p := service.UpstreamGroupCatalogParams{Page: 1, PageSize: 1, ConnectionIDs: []int64{c.ID}, Sort: "rate_asc"}
	result, err := repo.ListGroupCatalog(ctx, p, now)
	require.NoError(t, err)
	require.Equal(t, int64(3), result.Total)
	require.Len(t, result.Items, 1)
	require.Equal(t, "Alpha", result.Items[0].Name)
	require.Equal(t, 0.0, *result.Items[0].RateMultiplier)
	require.Equal(t, 1, result.Items[0].BindingCount)
	p.Page = 3
	result, err = repo.ListGroupCatalog(ctx, p, now)
	require.NoError(t, err)
	require.Nil(t, result.Items[0].RateMultiplier)
	p.Page = 1
	p.Binding = "unbound"
	p.PageSize = 20
	result, err = repo.ListGroupCatalog(ctx, p, now)
	require.NoError(t, err)
	require.Equal(t, int64(2), result.Total)
	yes := true
	ref := []service.UpstreamGroupReference{{ConnectionID: c.ID, RemoteKey: "id:0"}}
	require.NoError(t, repo.UpdateGroupAnnotations(ctx, service.UpstreamGroupAnnotationUpdate{Groups: ref, AddTags: []string{"main", "main"}, Favorite: &yes}))
	require.NoError(t, repo.UpdateGroupAnnotations(ctx, service.UpstreamGroupAnnotationUpdate{Groups: ref, AddTags: []string{"backup"}, RemoveTags: []string{"main"}}))
	p.Binding = ""
	p.Tag = "backup"
	p.Favorites = true
	result, err = repo.ListGroupCatalog(ctx, p, now)
	require.NoError(t, err)
	require.Equal(t, int64(1), result.Total)
	require.Equal(t, []string{"backup"}, result.Items[0].Tags)
	// Probe snapshots replace group IDs, while annotations follow the stable remote identity.
	applied, err := repo.ApplyProbeSuccess(ctx, c.ID, c.Version, service.UpstreamConnectionProbePersistence{GroupsObserved: true, Groups: []service.UpstreamGroup{{RemoteID: "0", Name: "Renamed", Source: "test", Metadata: map[string]any{}}}})
	require.NoError(t, err)
	require.True(t, applied)
	result, err = repo.ListGroupCatalog(ctx, p, now)
	require.NoError(t, err)
	require.Equal(t, int64(1), result.Total)
	require.Equal(t, "Renamed", result.Items[0].Name)
	p.Tag = ""
	p.Search = "does-not-exist"
	result, err = repo.ListGroupCatalog(ctx, p, now)
	require.NoError(t, err)
	require.Zero(t, result.Total)
}
func TestUpstreamCatalogHistoricalCostBoundaries(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := &upstreamConnectionRepository{client: client}
	c := &service.UpstreamConnection{Name: "history", Provider: "sub2api", AuthMode: "access_token", ManagementBaseURL: "https://history.invalid", CredentialEncrypted: "test", CredentialFingerprint: "history", Capabilities: map[string]any{}, Status: "ready", SyncIntervalSeconds: 60, Version: 1, WalletRaw: map[string]any{}}
	require.NoError(t, repo.Create(ctx, c))
	account := mustCreateAccount(t, client, &service.Account{Name: "history-account"})
	user := mustCreateUser(t, client, &service.User{Email: "history@catalog.invalid"})
	key := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-history-catalog", Name: "history"})
	_, err := client.UpstreamAccountBinding.Create().SetAccountID(account.ID).SetConnectionID(c.ID).Save(ctx)
	require.NoError(t, err)
	start := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	usage := newUsageLogRepositoryWithSQL(client, tx)
	for i, at := range []time.Time{start.Add(-time.Second), start, start.Add(time.Hour), end} {
		rate := 0.5
		_, err = usage.Create(ctx, &service.UsageLog{UserID: user.ID, APIKeyID: key.ID, AccountID: account.ID, RequestID: fmt.Sprintf("history-%d", i), Model: "test", TotalCost: 10, ActualCost: 20, AccountRateMultiplier: &rate, CreatedAt: at})
		require.NoError(t, err)
	}
	items, err := repo.GetConnectionDailyCosts(ctx, []int64{c.ID}, start, end, "UTC")
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, "2026-09-24", items[0].Day)
	require.Equal(t, int64(0), items[0].ConnectionID)
	require.Equal(t, int64(2), items[0].Requests)
	require.InDelta(t, 10.0, items[0].AccountCost, 1e-8)
	require.Equal(t, c.ID, items[1].ConnectionID)
	require.Equal(t, "", items[1].Day)
	require.Equal(t, items[0].AccountCost, items[1].AccountCost)
	profit, err := repo.GetUsageProfitDays(ctx, start, end, "UTC")
	require.NoError(t, err)
	require.Len(t, profit, 1)
	require.Equal(t, 10.0, profit[0].AccountCost)
	require.Zero(t, profit[0].Revenue, "usage actual_cost is not a paid order")
	// Payment overview is global; it must retain usage after management unbinding.
	_, err = client.ExecContext(ctx, "DELETE FROM upstream_account_bindings WHERE account_id=$1", account.ID)
	require.NoError(t, err)
	globalProfit, err := repo.GetUsageProfitDays(ctx, start, end, "UTC")
	require.NoError(t, err)
	require.Equal(t, profit, globalProfit)
	_, err = client.UpstreamAccountBinding.Create().SetAccountID(account.ID).SetConnectionID(c.ID).Save(ctx)
	require.NoError(t, err)
	// A timezone behind UTC must put both included requests on the previous local date.
	shifted, err := repo.GetConnectionDailyCosts(ctx, []int64{c.ID}, start, end, "America/New_York")
	require.NoError(t, err)
	require.Len(t, shifted, 2)
	require.Equal(t, "2026-09-23", shifted[0].Day)
	require.Equal(t, items[0].AccountCost, shifted[0].AccountCost)
	// With no matching usage, retain the selected connection with a zero total.
	empty, err := repo.GetConnectionDailyCosts(ctx, []int64{c.ID}, end.Add(time.Hour), end.Add(2*time.Hour), "UTC")
	require.NoError(t, err)
	require.Len(t, empty, 1)
	require.Zero(t, empty[0].AccountCost)
	require.Empty(t, empty[0].Day)
	rows, err := client.QueryContext(ctx, "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) "+upstreamDailyCostsSQL, pq.Array([]int64{c.ID}), start, end, "UTC")
	require.NoError(t, err)
	require.True(t, rows.Next())
	var raw []byte
	require.NoError(t, rows.Scan(&raw))
	require.NoError(t, rows.Close())
	var plan []map[string]any
	require.NoError(t, json.Unmarshal(raw, &plan))
	scans := 0
	var visit func(any)
	visit = func(value any) {
		switch node := value.(type) {
		case map[string]any:
			if node["Relation Name"] == "usage_logs" {
				scans++
			}
			for _, child := range node {
				visit(child)
			}
		case []any:
			for _, child := range node {
				visit(child)
			}
		}
	}
	visit(plan[0]["Plan"])
	require.Equal(t, 1, scans, "daily and connection totals must share the same usage scan")
	t.Logf("Historical cost query: one usage scan, execution %.3f ms (small integration fixture)", plan[0]["Execution Time"])
}
