package repository

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"time"
)

const paymentUsageProfitSQL = `SELECT to_char((created_at AT TIME ZONE $3)::date,'YYYY-MM-DD'),
 COALESCE(SUM(COALESCE(account_stats_cost,total_cost)*COALESCE(account_rate_multiplier,1)),0),
 COALESCE(SUM(actual_cost),0)
 FROM usage_logs WHERE created_at >= $1 AND created_at < $2
 GROUP BY (created_at AT TIME ZONE $3)::date ORDER BY 1`

// Global scope includes accounts not bound to upstream management and deleted accounts.
func (r *upstreamConnectionRepository) GetUsageProfitDays(ctx context.Context, start, end time.Time, zone string) ([]service.UsageProfitDay, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	rows, err := clientFromContext(ctx, r.client).QueryContext(ctx, paymentUsageProfitSQL, start, end, zone)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	points := []service.UsageProfitDay{}
	for rows.Next() {
		var point service.UsageProfitDay
		if err := rows.Scan(&point.Date, &point.AccountCost, &point.Revenue); err != nil {
			return nil, err
		}
		points = append(points, point)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return points, rows.Close()
}
