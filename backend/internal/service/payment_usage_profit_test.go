//go:build unit

package service

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type usageProfitTestReader struct {
	UpstreamConnectionRepository
	start, end time.Time
	calls      int
}

func (r *usageProfitTestReader) GetUsageProfitDays(_ context.Context, start, end time.Time, _ string) ([]UsageProfitDay, error) {
	r.calls++
	r.start = start
	r.end = end
	return []UsageProfitDay{{Date: "2026-09-24", AccountCost: 10, Revenue: 4}, {Date: "2026-09-25", AccountCost: 2, Revenue: 8}}, nil
}
func TestPaymentUsageProfitRangeAndNegativeProfit(t *testing.T) {
	r := &usageProfitTestReader{}
	s := NewUpstreamConnectionService(r, nil, nil)
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, timezone.Location())
	s.now = func() time.Time { return now }
	result, err := s.GetPaymentUsageProfit(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, 1, r.calls)
	require.Len(t, result.Daily, 7)
	require.Equal(t, "2026-09-19", result.Daily[0].Date)
	require.Zero(t, result.Daily[0].AccountCost)
	require.Equal(t, -6.0, result.Daily[5].GrossProfit)
	require.Equal(t, 6.0, result.Daily[6].GrossProfit)
	require.Equal(t, 12.0, result.TotalRevenue)
	require.Equal(t, 12.0, result.TotalCost)
	require.Zero(t, result.GrossProfit)
	require.Equal(t, now, r.end)
	_, err = s.GetPaymentUsageProfit(context.Background(), 365)
	require.Error(t, err)
	require.Equal(t, 1, r.calls)
}
