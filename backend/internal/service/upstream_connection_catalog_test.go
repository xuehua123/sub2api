//go:build unit

package service

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
	"math"
	"testing"
	"time"
)

type catalogTestRepo struct {
	UpstreamConnectionRepository
	params     UpstreamGroupCatalogParams
	update     UpstreamGroupAnnotationUpdate
	start, end time.Time
	buckets    []UpstreamConnectionDailyCost
	calls      int
}

func (r *catalogTestRepo) ListGroupCatalog(_ context.Context, p UpstreamGroupCatalogParams, _ time.Time) (*UpstreamGroupCatalogResult, error) {
	r.params = p
	return &UpstreamGroupCatalogResult{}, nil
}
func (r *catalogTestRepo) UpdateGroupAnnotations(_ context.Context, p UpstreamGroupAnnotationUpdate) error {
	r.update = p
	return nil
}
func (r *catalogTestRepo) GetConnectionDailyCosts(_ context.Context, _ []int64, start, end time.Time, _ string) ([]UpstreamConnectionDailyCost, error) {
	r.start = start
	r.end = end
	r.calls++
	if r.buckets != nil {
		return r.buckets, nil
	}
	return []UpstreamConnectionDailyCost{
		{UpstreamConnectionCostItem: UpstreamConnectionCostItem{ConnectionID: 1, AccountCost: 1.2, Requests: 3}},
		{UpstreamConnectionCostItem: UpstreamConnectionCostItem{ConnectionID: 2, AccountCost: 2, Requests: 4}},
		{Day: start.Format("2006-01-02"), UpstreamConnectionCostItem: UpstreamConnectionCostItem{AccountCost: 3.2, Requests: 7}},
	}, nil
}
func TestUpstreamConnectionCatalogValidation(t *testing.T) {
	r := &catalogTestRepo{}
	s := NewUpstreamConnectionService(r, nil, nil)
	ctx := context.Background()
	zero := 0.0
	nan := math.NaN()
	_, err := s.ListGroupCatalog(ctx, UpstreamGroupCatalogParams{MinRate: &zero, Search: " x "})
	require.NoError(t, err)
	require.Equal(t, "x", r.params.Search)
	require.Equal(t, 20, r.params.PageSize)
	for _, p := range []UpstreamGroupCatalogParams{{Sort: "DROP TABLE"}, {MinRate: &nan}, {Binding: "bad"}, {Freshness: "bad"}} {
		_, err = s.ListGroupCatalog(ctx, p)
		require.Error(t, err)
	}
	err = s.UpdateGroupAnnotations(ctx, UpstreamGroupAnnotationUpdate{Groups: []UpstreamGroupReference{{ConnectionID: 1, RemoteKey: "id:x"}}, AddTags: []string{" main "}})
	require.NoError(t, err)
	require.Equal(t, []string{"main"}, r.update.AddTags)
	require.Error(t, s.UpdateGroupAnnotations(ctx, UpstreamGroupAnnotationUpdate{}))
}
func TestUpstreamConnectionCostHistoryDateBounds(t *testing.T) {
	r := &catalogTestRepo{}
	s := NewUpstreamConnectionService(r, nil, nil)
	now := time.Date(2026, 9, 25, 12, 30, 0, 0, timezone.Location())
	s.now = func() time.Time { return now }
	ctx := context.Background()
	result, err := s.GetCostHistory(ctx, nil, "2026-09-24", "2026-09-24")
	require.NoError(t, err)
	require.InDelta(t, 3.2, result.TotalCost, 1e-9)
	require.Equal(t, int64(7), result.TotalRequests)
	require.Equal(t, timezone.StartOfDay(now), r.end)
	require.Equal(t, timezone.StartOfDay(now).AddDate(0, 0, -1), r.start)
	_, err = s.GetCostHistory(ctx, nil, "", "")
	require.NoError(t, err)
	require.Equal(t, now, r.end)
	for _, dates := range [][2]string{{"bad", "2026-09-24"}, {"2026-09-25", "2026-09-24"}, {"2026-09-25", "2026-09-26"}, {"2024-01-01", "2026-01-01"}} {
		_, err = s.GetCostHistory(ctx, nil, dates[0], dates[1])
		require.Error(t, err)
	}
}

func TestUpstreamConnectionCostHistoryDailyGapsAndTotals(t *testing.T) {
	r := &catalogTestRepo{buckets: []UpstreamConnectionDailyCost{
		{UpstreamConnectionCostItem: UpstreamConnectionCostItem{ConnectionID: 1, AccountCost: 4.5, Requests: 3}},
		{Day: "2026-09-20", UpstreamConnectionCostItem: UpstreamConnectionCostItem{AccountCost: 1.5, Requests: 1}},
		{Day: "2026-09-22", UpstreamConnectionCostItem: UpstreamConnectionCostItem{AccountCost: 3, Requests: 2}},
	}}
	s := NewUpstreamConnectionService(r, nil, nil)
	s.now = func() time.Time { return time.Date(2026, 9, 25, 12, 0, 0, 0, timezone.Location()) }
	result, err := s.GetCostHistory(context.Background(), nil, "2026-09-20", "2026-09-23")
	require.NoError(t, err)
	require.Equal(t, 1, r.calls)
	require.Len(t, result.Daily, 4)
	require.Equal(t, UpstreamCostDay{Date: "2026-09-21"}, result.Daily[1])
	require.Equal(t, UpstreamCostDay{Date: "2026-09-23"}, result.Daily[3])
	require.Equal(t, 4.5, result.TotalCost)
	require.Equal(t, int64(3), result.TotalRequests)
	require.Equal(t, result.TotalCost, result.Items[0].AccountCost)
	r.buckets = []UpstreamConnectionDailyCost{}
	result, err = s.GetCostHistory(context.Background(), nil, "2026-09-24", "2026-09-24")
	require.NoError(t, err)
	require.Len(t, result.Daily, 1)
	require.Zero(t, result.TotalCost)
}
