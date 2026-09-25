package repository

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// Share the same payment deduplication and settled-refund evidence as the user report.
// The parameter positions intentionally match reportArgs; this query has global scope.
const paymentUsageProfitSQL = `WITH daily_cost AS MATERIALIZED (
 SELECT (created_at AT TIME ZONE $7)::date AS day,
 SUM(COALESCE(account_stats_cost,total_cost)*COALESCE(account_rate_multiplier,1)) AS cost
 FROM usage_logs WHERE created_at >= $1 AND created_at < $2
 GROUP BY 1
) ` + userBusinessMoneyCTE + `,
 daily_cash AS (
 SELECT (at AT TIME ZONE $7)::date AS day,SUM(paid) AS revenue,SUM(refund) AS refund,
 COUNT(*) FILTER(WHERE uncertain) AS uncertain_count
 FROM cash_events GROUP BY 1
 )
 SELECT to_char(COALESCE(c.day,p.day),'YYYY-MM-DD') AS date,COALESCE(c.cost,0) AS account_cost,COALESCE(p.revenue,0) AS revenue,COALESCE(p.refund,0) AS refund,COALESCE(p.uncertain_count,0) AS uncertain_count
 FROM daily_cost c FULL OUTER JOIN daily_cash p ON p.day=c.day
 WHERE $3::text='' AND $4::text='' AND $5::int>0 AND $6::int>=0 ORDER BY 1`

// Include payment-only days, users without usage, and costs of unbound/deleted accounts.
func (r *upstreamConnectionRepository) GetUsageProfitDays(ctx context.Context, start, end time.Time, zone string) ([]service.UsageProfitDay, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	rows, err := clientFromContext(ctx, r.client).QueryContext(ctx, paymentUsageProfitSQL, start, end, "", "", 1, 0, zone, end, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	points := []service.UsageProfitDay{}
	for rows.Next() {
		var point service.UsageProfitDay
		if err := rows.Scan(&point.Date, &point.AccountCost, &point.Revenue, &point.Refund, &point.UncertainCount); err != nil {
			return nil, err
		}
		points = append(points, point)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return points, rows.Close()
}
