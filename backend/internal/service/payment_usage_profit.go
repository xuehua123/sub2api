package service

import (
	"context"
	"errors"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"time"
)

// Usage revenue is actual_cost, not cash receipts or recognized subscription revenue.
type UsageProfitDay struct {
	Date        string  `json:"date"`
	AccountCost float64 `json:"account_cost"`
	Revenue     float64 `json:"revenue"`
	GrossProfit float64 `json:"gross_profit"`
}
type UsageProfitTrend struct {
	Daily        []UsageProfitDay `json:"daily"`
	TotalCost    float64          `json:"total_cost"`
	TotalRevenue float64          `json:"total_revenue"`
	GrossProfit  float64          `json:"gross_profit"`
	Timezone     string           `json:"timezone"`
	Currency     string           `json:"currency"`
}
type usageProfitReader interface {
	GetUsageProfitDays(context.Context, time.Time, time.Time, string) ([]UsageProfitDay, error)
}

func (s *UpstreamConnectionService) GetPaymentUsageProfit(ctx context.Context, days int) (*UsageProfitTrend, error) {
	if days != 7 && days != 30 && days != 90 {
		return nil, infraerrors.BadRequest("INVALID_DAYS", "days must be 7, 30 or 90")
	}
	now := s.now().In(timezone.Location())
	start := timezone.StartOfDay(now).AddDate(0, 0, 1-days)
	reader, ok := s.repo.(usageProfitReader)
	if !ok {
		return nil, errors.New("usage profit reader unavailable")
	}
	points, err := reader.GetUsageProfitDays(ctx, start, now, timezone.Location().String())
	if err != nil {
		return nil, err
	}
	byDate := make(map[string]UsageProfitDay, len(points))
	for _, point := range points {
		byDate[point.Date] = point
	}
	result := &UsageProfitTrend{Daily: make([]UsageProfitDay, 0, days), Timezone: timezone.Location().String(), Currency: "USD"}
	for i := 0; i < days; i++ {
		date := start.AddDate(0, 0, i).Format("2006-01-02")
		point := byDate[date]
		point.Date = date
		point.GrossProfit = point.Revenue - point.AccountCost
		result.Daily = append(result.Daily, point)
		result.TotalCost += point.AccountCost
		result.TotalRevenue += point.Revenue
	}
	result.GrossProfit = result.TotalRevenue - result.TotalCost
	return result, nil
}
