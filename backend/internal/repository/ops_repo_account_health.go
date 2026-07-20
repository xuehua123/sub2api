package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

func (r *opsRepository) GetAccountHealthMetrics(ctx context.Context, filter *service.OpsAccountHealthFilter) (map[int64]*service.OpsAccountHealthMetrics, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}

	endTime := time.Now().UTC()
	if filter != nil && !filter.EndTime.IsZero() {
		endTime = filter.EndTime.UTC()
	}
	start1m := endTime.Add(-1 * time.Minute)
	startPrev1m := endTime.Add(-2 * time.Minute) // exclusive prior minute [now-2m, now-1m)
	start5m := endTime.Add(-5 * time.Minute)
	start10m := endTime.Add(-10 * time.Minute)
	start30m := endTime.Add(-30 * time.Minute)
	start1h := endTime.Add(-1 * time.Hour)

	platform := ""
	groupID := int64(0)
	limit := 60
	accountIDs := []int64{}
	if filter != nil {
		platform = strings.TrimSpace(strings.ToLower(filter.Platform))
		if filter.GroupID != nil && *filter.GroupID > 0 {
			groupID = *filter.GroupID
		}
		if filter.RecentLimit > 0 {
			limit = filter.RecentLimit
		}
		if len(filter.AccountIDs) > 0 {
			seen := make(map[int64]struct{}, len(filter.AccountIDs))
			for _, id := range filter.AccountIDs {
				if id <= 0 {
					continue
				}
				if _, ok := seen[id]; ok {
					continue
				}
				seen[id] = struct{}{}
				accountIDs = append(accountIDs, id)
			}
		}
	}
	if limit > 120 {
		limit = 120
	}

	out := map[int64]*service.OpsAccountHealthMetrics{}
	if err := r.loadAccountHealthWindowStats(ctx, out, endTime, start1m, startPrev1m, start5m, start10m, start30m, start1h, platform, groupID, accountIDs); err != nil {
		return nil, err
	}
	if err := r.loadAccountHealthFirstTokenStats(ctx, out, endTime, start1m, start5m, start10m, start30m, start1h, platform, groupID, accountIDs); err != nil {
		return nil, err
	}
	if err := r.loadAccountHealthRecentSamples(ctx, out, endTime, start1h, platform, groupID, limit, accountIDs); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *opsRepository) loadAccountHealthWindowStats(
	ctx context.Context,
	out map[int64]*service.OpsAccountHealthMetrics,
	endTime time.Time,
	start1m time.Time,
	startPrev1m time.Time,
	start5m time.Time,
	start10m time.Time,
	start30m time.Time,
	start1h time.Time,
	platform string,
	groupID int64,
	accountIDs []int64,
) error {
	// All window counts are full aggregates from usage_logs ∪ ops_error_logs (not sample-capped).
	// prev_1m is exclusive [now-2m, now-1m); other windows are [start, end).
	query := `
WITH combined AS (
  SELECT
    ul.account_id AS account_id,
    ul.created_at AS created_at,
    false AS is_error,
    false AS is_upstream_error,
    NULL::INT AS status_code,
    ul.duration_ms AS duration_ms
  FROM usage_logs ul
  LEFT JOIN groups g ON g.id = ul.group_id
  LEFT JOIN accounts a ON a.id = ul.account_id
  WHERE ul.created_at >= $6 AND ul.created_at < $1
    AND ul.account_id IS NOT NULL
    AND ($7 = '' OR LOWER(COALESCE(NULLIF(g.platform, ''), NULLIF(a.platform, ''), '')) = $7)
    AND ($8::BIGINT <= 0 OR ul.group_id = $8)
    AND (cardinality($9::BIGINT[]) = 0 OR ul.account_id = ANY($9))

  UNION ALL

  SELECT
    o.account_id AS account_id,
    o.created_at AS created_at,
    true AS is_error,
    (COALESCE(o.error_owner, '') = 'provider' AND NOT o.is_business_limited AND COALESCE(o.upstream_status_code, o.status_code, 0) NOT IN (429, 529)) AS is_upstream_error,
    COALESCE(o.upstream_status_code, o.status_code) AS status_code,
    o.duration_ms AS duration_ms
  FROM ops_error_logs o
  LEFT JOIN groups g ON g.id = o.group_id
  LEFT JOIN accounts a ON a.id = o.account_id
  WHERE o.created_at >= $6 AND o.created_at < $1
    AND o.account_id IS NOT NULL
    AND COALESCE(o.upstream_status_code, o.status_code, 0) >= 400
    AND ($7 = '' OR LOWER(COALESCE(NULLIF(o.platform, ''), NULLIF(g.platform, ''), NULLIF(a.platform, ''), '')) = $7)
    AND ($8::BIGINT <= 0 OR o.group_id = $8)
    AND (cardinality($9::BIGINT[]) = 0 OR o.account_id = ANY($9))
),
windows(label, start_at, end_at) AS (
  VALUES
    ('1m'::TEXT, $2::TIMESTAMPTZ, $1::TIMESTAMPTZ),
    ('5m'::TEXT, $3::TIMESTAMPTZ, $1::TIMESTAMPTZ),
    ('10m'::TEXT, $4::TIMESTAMPTZ, $1::TIMESTAMPTZ),
    ('30m'::TEXT, $5::TIMESTAMPTZ, $1::TIMESTAMPTZ),
    ('1h'::TEXT, $6::TIMESTAMPTZ, $1::TIMESTAMPTZ),
    ('prev_1m'::TEXT, $10::TIMESTAMPTZ, $2::TIMESTAMPTZ)
)
SELECT
  c.account_id,
  w.label,
  COUNT(*) AS request_count,
  COUNT(*) FILTER (WHERE NOT c.is_error) AS success_count,
  COUNT(*) FILTER (WHERE c.is_error) AS error_count,
  COUNT(*) FILTER (WHERE c.is_upstream_error) AS upstream_error_count,
  COUNT(*) FILTER (WHERE c.status_code = 429) AS status_429_count,
  COUNT(*) FILTER (WHERE c.status_code = 529) AS status_529_count,
  AVG(c.duration_ms)::DOUBLE PRECISION AS avg_duration_ms
FROM combined c
JOIN windows w ON c.created_at >= w.start_at AND c.created_at < w.end_at
GROUP BY c.account_id, w.label
ORDER BY c.account_id, w.label
`

	rows, err := r.db.QueryContext(ctx, query, endTime, start1m, start5m, start10m, start30m, start1h, platform, groupID, pq.Array(accountIDs), startPrev1m)
	if err != nil {
		return fmt.Errorf("query account health windows: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			accountID int64
			label     string
			stat      service.OpsAccountHealthWindowStats
			avg       sql.NullFloat64
		)
		if err := rows.Scan(
			&accountID,
			&label,
			&stat.RequestCount,
			&stat.SuccessCount,
			&stat.ErrorCount,
			&stat.UpstreamErrorCount,
			&stat.Status429Count,
			&stat.Status529Count,
			&avg,
		); err != nil {
			return fmt.Errorf("scan account health window: %w", err)
		}
		stat.Window = label
		if avg.Valid {
			v := avg.Float64
			stat.AvgDurationMs = &v
		}
		metrics := ensureAccountHealthMetrics(out, accountID)
		metrics.Windows[label] = &stat
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate account health windows: %w", err)
	}
	return nil
}

func (r *opsRepository) loadAccountHealthFirstTokenStats(
	ctx context.Context,
	out map[int64]*service.OpsAccountHealthMetrics,
	endTime time.Time,
	start1m time.Time,
	start5m time.Time,
	start10m time.Time,
	start30m time.Time,
	start1h time.Time,
	platform string,
	groupID int64,
	accountIDs []int64,
) error {
	query := `
WITH windows(label, start_at) AS (
  VALUES
    ('1m'::TEXT, $2::TIMESTAMPTZ),
    ('5m'::TEXT, $3::TIMESTAMPTZ),
    ('10m'::TEXT, $4::TIMESTAMPTZ),
    ('30m'::TEXT, $5::TIMESTAMPTZ),
    ('1h'::TEXT, $6::TIMESTAMPTZ)
)
SELECT
  ul.account_id,
  w.label,
  COUNT(*)::BIGINT AS sample_count,
  AVG(ul.first_token_ms)::DOUBLE PRECISION AS avg_first_token_ms
FROM usage_logs ul
JOIN windows w ON ul.created_at >= w.start_at
LEFT JOIN groups g ON g.id = ul.group_id
LEFT JOIN accounts a ON a.id = ul.account_id
WHERE ul.created_at >= $6 AND ul.created_at < $1
  AND ul.account_id IS NOT NULL
  AND ul.first_token_ms IS NOT NULL
  AND ($7 = '' OR LOWER(COALESCE(NULLIF(g.platform, ''), NULLIF(a.platform, ''), '')) = $7)
  AND ($8::BIGINT <= 0 OR ul.group_id = $8)
  AND (cardinality($9::BIGINT[]) = 0 OR ul.account_id = ANY($9))
GROUP BY ul.account_id, w.label
ORDER BY ul.account_id, w.label
`

	rows, err := r.db.QueryContext(ctx, query, endTime, start1m, start5m, start10m, start30m, start1h, platform, groupID, pq.Array(accountIDs))
	if err != nil {
		return fmt.Errorf("query account health first token stats: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			accountID int64
			label     string
			stat      service.OpsAccountHealthFirstTokenStats
			avg       sql.NullFloat64
		)
		if err := rows.Scan(&accountID, &label, &stat.SampleCount, &avg); err != nil {
			return fmt.Errorf("scan account health first token stats: %w", err)
		}
		stat.Window = label
		if avg.Valid {
			v := avg.Float64
			stat.AvgMs = &v
		}
		metrics := ensureAccountHealthMetrics(out, accountID)
		if metrics.FirstTokenWindows == nil {
			metrics.FirstTokenWindows = map[string]*service.OpsAccountHealthFirstTokenStats{}
		}
		metrics.FirstTokenWindows[label] = &stat
		if label == "5m" {
			metrics.FirstToken5m = &stat
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate account health first token stats: %w", err)
	}
	return nil
}

func (r *opsRepository) loadAccountHealthRecentSamples(
	ctx context.Context,
	out map[int64]*service.OpsAccountHealthMetrics,
	endTime time.Time,
	startTime time.Time,
	platform string,
	groupID int64,
	limit int,
	accountIDs []int64,
) error {
	query := `
WITH combined AS (
  SELECT
    ul.account_id AS account_id,
    'success'::TEXT AS kind,
    ul.created_at AS created_at,
    ul.request_id AS request_id,
    ul.model AS model,
    ul.duration_ms AS duration_ms,
    NULL::INT AS status_code,
    NULL::TEXT AS message
  FROM usage_logs ul
  LEFT JOIN groups g ON g.id = ul.group_id
  LEFT JOIN accounts a ON a.id = ul.account_id
  WHERE ul.created_at >= $2 AND ul.created_at < $1
    AND ul.account_id IS NOT NULL
    AND ($3 = '' OR LOWER(COALESCE(NULLIF(g.platform, ''), NULLIF(a.platform, ''), '')) = $3)
    AND ($4::BIGINT <= 0 OR ul.group_id = $4)
    AND (cardinality($6::BIGINT[]) = 0 OR ul.account_id = ANY($6))

  UNION ALL

  SELECT
    o.account_id AS account_id,
    'error'::TEXT AS kind,
    o.created_at AS created_at,
    COALESCE(NULLIF(o.request_id, ''), NULLIF(o.client_request_id, ''), '') AS request_id,
    o.model AS model,
    o.duration_ms AS duration_ms,
    COALESCE(o.upstream_status_code, o.status_code) AS status_code,
    COALESCE(NULLIF(o.upstream_error_message, ''), NULLIF(o.error_message, ''), '') AS message
  FROM ops_error_logs o
  LEFT JOIN groups g ON g.id = o.group_id
  LEFT JOIN accounts a ON a.id = o.account_id
  WHERE o.created_at >= $2 AND o.created_at < $1
    AND o.account_id IS NOT NULL
    AND COALESCE(o.upstream_status_code, o.status_code, 0) >= 400
    AND ($3 = '' OR LOWER(COALESCE(NULLIF(o.platform, ''), NULLIF(g.platform, ''), NULLIF(a.platform, ''), '')) = $3)
    AND ($4::BIGINT <= 0 OR o.group_id = $4)
    AND (cardinality($6::BIGINT[]) = 0 OR o.account_id = ANY($6))
),
ranked AS (
  SELECT
    *,
    ROW_NUMBER() OVER (PARTITION BY account_id ORDER BY created_at DESC) AS rn
  FROM combined
)
SELECT
  account_id,
  kind,
  created_at,
  request_id,
  model,
  duration_ms,
  status_code,
  message
FROM ranked
WHERE rn <= $5
ORDER BY account_id, created_at DESC
`

	rows, err := r.db.QueryContext(ctx, query, endTime, startTime, platform, groupID, limit, pq.Array(accountIDs))
	if err != nil {
		return fmt.Errorf("query account health recent samples: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			accountID int64
			sample    service.OpsAccountHealthSample
			requestID sql.NullString
			model     sql.NullString
			duration  sql.NullInt64
			status    sql.NullInt64
			message   sql.NullString
		)
		if err := rows.Scan(
			&accountID,
			&sample.Kind,
			&sample.CreatedAt,
			&requestID,
			&model,
			&duration,
			&status,
			&message,
		); err != nil {
			return fmt.Errorf("scan account health recent sample: %w", err)
		}
		sample.RequestID = strings.TrimSpace(requestID.String)
		sample.Model = strings.TrimSpace(model.String)
		sample.Message = strings.TrimSpace(message.String)
		if duration.Valid {
			v := int(duration.Int64)
			sample.DurationMs = &v
		}
		if status.Valid {
			v := int(status.Int64)
			sample.StatusCode = &v
		}
		metrics := ensureAccountHealthMetrics(out, accountID)
		metrics.Recent = append(metrics.Recent, &sample)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate account health recent samples: %w", err)
	}
	return nil
}

func ensureAccountHealthMetrics(out map[int64]*service.OpsAccountHealthMetrics, accountID int64) *service.OpsAccountHealthMetrics {
	metrics := out[accountID]
	if metrics != nil {
		return metrics
	}
	metrics = &service.OpsAccountHealthMetrics{
		AccountID:         accountID,
		Windows:           map[string]*service.OpsAccountHealthWindowStats{},
		Recent:            []*service.OpsAccountHealthSample{},
		FirstTokenWindows: map[string]*service.OpsAccountHealthFirstTokenStats{},
	}
	out[accountID] = metrics
	return metrics
}
