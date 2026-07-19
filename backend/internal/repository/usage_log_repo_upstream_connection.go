package repository

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

// GetUpstreamAccountUsageBuckets aggregates all bound accounts in one query so
// the upstream-connection detail page never performs one database query per key.
func (r *usageLogRepository) GetUpstreamAccountUsageBuckets(
	ctx context.Context,
	accountIDs []int64,
	startTime, endTime time.Time,
	timezoneName string,
) (results []service.UpstreamConnectionAccountUsageBucket, err error) {
	accountIDs = normalizePositiveInt64IDs(accountIDs)
	if len(accountIDs) == 0 {
		return []service.UpstreamConnectionAccountUsageBucket{}, nil
	}

	query := `
		SELECT
			account_id,
			date_trunc('hour', created_at AT TIME ZONE $4) AT TIME ZONE $4 AS bucket_start,
			COUNT(*) AS requests,
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) AS tokens,
			COALESCE(SUM(COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1)), 0) AS account_cost,
			COALESCE(SUM(total_cost), 0) AS standard_cost,
			COALESCE(SUM(actual_cost), 0) AS user_cost
		FROM usage_logs
		WHERE account_id = ANY($1)
		  AND created_at >= $2
		  AND created_at < $3
		GROUP BY account_id, bucket_start
		ORDER BY bucket_start ASC, account_id ASC
	`
	rows, err := r.sql.QueryContext(ctx, query, pq.Array(accountIDs), startTime, endTime, timezoneName)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
		}
	}()

	results = make([]service.UpstreamConnectionAccountUsageBucket, 0)
	for rows.Next() {
		var bucket service.UpstreamConnectionAccountUsageBucket
		if err = rows.Scan(
			&bucket.AccountID,
			&bucket.Bucket,
			&bucket.Requests,
			&bucket.Tokens,
			&bucket.AccountCost,
			&bucket.StandardCost,
			&bucket.UserCost,
		); err != nil {
			return nil, err
		}
		results = append(results, bucket)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}
