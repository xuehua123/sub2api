package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

// GetUpstreamConnectionRuntimeGroups loads the connection-list snapshot in one
// query. Successful usage comes from usage_logs; failed requests are added from
// ops_error_logs so the short-window success rate has the same semantics as the
// account health view.
func (r *usageLogRepository) GetUpstreamConnectionRuntimeGroups(
	ctx context.Context,
	accountIDs []int64,
	startTime, endTime, fiveMinuteStart time.Time,
) (results []service.UpstreamConnectionRuntimeGroupMetric, err error) {
	accountIDs = normalizePositiveInt64IDs(accountIDs)
	if len(accountIDs) == 0 {
		return []service.UpstreamConnectionRuntimeGroupMetric{}, nil
	}
	query := `
WITH events AS (
  SELECT
    ul.account_id,
    COALESCE(ul.group_id, 0) AS group_id,
    ul.created_at,
    false AS is_error,
    ul.input_tokens + ul.output_tokens + ul.cache_creation_tokens + ul.cache_read_tokens AS tokens,
    COALESCE(ul.account_stats_cost, ul.total_cost) * COALESCE(ul.account_rate_multiplier, 1) AS account_cost,
    ul.total_cost AS standard_cost,
    ul.actual_cost AS user_cost
  FROM usage_logs ul
  WHERE ul.account_id = ANY($1)
    AND ul.created_at >= $2
    AND ul.created_at < $3

  UNION ALL

  SELECT
    o.account_id,
    COALESCE(o.group_id, 0) AS group_id,
    o.created_at,
    true AS is_error,
    0::BIGINT AS tokens,
    0::NUMERIC AS account_cost,
    0::NUMERIC AS standard_cost,
    0::NUMERIC AS user_cost
  FROM ops_error_logs o
  WHERE o.account_id = ANY($1)
    AND o.created_at >= $2
    AND o.created_at < $3
    AND COALESCE(o.upstream_status_code, o.status_code, 0) >= 400
)
SELECT
  e.account_id,
  e.group_id,
  -- Empty name is localized on the client (ungrouped vs deleted group).
  COALESCE(NULLIF(g.name, ''), '') AS group_name,
  COUNT(*) FILTER (WHERE NOT e.is_error) AS today_requests,
  COALESCE(SUM(e.tokens) FILTER (WHERE NOT e.is_error), 0) AS today_tokens,
  COALESCE(SUM(e.account_cost) FILTER (WHERE NOT e.is_error), 0) AS today_account_cost,
  COALESCE(SUM(e.standard_cost) FILTER (WHERE NOT e.is_error), 0) AS today_standard_cost,
  COALESCE(SUM(e.user_cost) FILTER (WHERE NOT e.is_error), 0) AS today_user_cost,
  COUNT(*) FILTER (WHERE e.created_at >= $4) AS five_minute_requests,
  COUNT(*) FILTER (WHERE e.created_at >= $4 AND NOT e.is_error) AS five_minute_success_count,
  COUNT(*) FILTER (WHERE e.created_at >= $4 AND e.is_error) AS five_minute_error_count
FROM events e
LEFT JOIN groups g ON g.id = e.group_id
GROUP BY e.account_id, e.group_id, g.name
ORDER BY e.account_id ASC, today_account_cost DESC, today_requests DESC, group_name ASC
`
	rows, err := r.sql.QueryContext(ctx, query, pq.Array(accountIDs), startTime, endTime, fiveMinuteStart)
	if err != nil {
		return nil, fmt.Errorf("query upstream connection runtime groups: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
		}
	}()

	results = make([]service.UpstreamConnectionRuntimeGroupMetric, 0)
	for rows.Next() {
		var metric service.UpstreamConnectionRuntimeGroupMetric
		if err = rows.Scan(
			&metric.AccountID,
			&metric.GroupID,
			&metric.GroupName,
			&metric.Today.Requests,
			&metric.Today.Tokens,
			&metric.Today.AccountCost,
			&metric.Today.StandardCost,
			&metric.Today.UserCost,
			&metric.FiveMinuteRequests,
			&metric.FiveMinuteSuccessCount,
			&metric.FiveMinuteErrorCount,
		); err != nil {
			return nil, fmt.Errorf("scan upstream connection runtime group: %w", err)
		}
		results = append(results, metric)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate upstream connection runtime groups: %w", err)
	}
	return results, nil
}
