package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type userBusinessRepository struct{ db sqlExecutor }

func NewUserBusinessRepository(db *sql.DB) service.UserBusinessRepository {
	return &userBusinessRepository{db: db}
}

const userBusinessUsageCTE = `WITH usage AS MATERIALIZED (
 SELECT ul.user_id,SUM(actual_cost) AS consumption,
 COALESCE(SUM(actual_cost) FILTER(WHERE CASE WHEN COALESCE(billing_source,'')<>'' THEN billing_source IN ('entitlement_quota','legacy_subscription') ELSE billing_type=1 END),0) AS subscription_consumption,
 COALESCE(SUM(actual_cost) FILTER(WHERE NOT(CASE WHEN COALESCE(billing_source,'')<>'' THEN billing_source IN ('entitlement_quota','legacy_subscription') ELSE billing_type=1 END)),0) AS balance_consumption,
 CASE WHEN COUNT(*) FILTER(WHERE $11='historical' AND fx.usd_cny IS NULL AND COALESCE(account_stats_cost,total_cost)*COALESCE(account_rate_multiplier,1)<>0)>0 THEN NULL
 ELSE COALESCE(SUM(COALESCE(account_stats_cost,total_cost)*COALESCE(account_rate_multiplier,1)*CASE WHEN $11='estimate' THEN $3::numeric ELSE fx.usd_cny END),0) END AS cost,
 COUNT(*) FILTER(WHERE $11='historical' AND fx.usd_cny IS NULL AND COALESCE(account_stats_cost,total_cost)*COALESCE(account_rate_multiplier,1)<>0) AS missing_fx_count,COUNT(*) AS requests
 FROM usage_logs ul LEFT JOIN user_business_daily_fx fx ON fx.rate_date=(ul.created_at AT TIME ZONE $8)::date
 WHERE ul.created_at >= $1 AND ul.created_at < $2 AND ($10::bigint=0 OR ul.user_id=$10)
 GROUP BY ul.user_id
) `
const userBusinessRowsCTE = `,
active_card_counts AS MATERIALIZED (
 SELECT e.user_id,COUNT(*) AS count FROM subscription_entitlements e
 WHERE e.deleted_at IS NULL AND e.status='active' AND e.starts_at <= $9 AND e.expires_at > $9 GROUP BY e.user_id
 UNION ALL
 SELECT s.user_id,COUNT(*) FROM user_subscriptions s WHERE s.deleted_at IS NULL AND s.status='active' AND s.starts_at <= $9 AND s.expires_at > $9
 AND NOT EXISTS(SELECT 1 FROM subscription_entitlements e WHERE e.legacy_subscription_id=s.id AND e.deleted_at IS NULL)
 GROUP BY s.user_id
), active_cards AS MATERIALIZED (SELECT user_id,SUM(count) AS count FROM active_card_counts GROUP BY user_id),
ranked_users AS MATERIALIZED (
 SELECT u.id,u.username,u.email,u.deleted_at IS NOT NULL AS deleted,u.balance,
 COALESCE(g.consumption,0) AS consumption,COALESCE(g.subscription_consumption,0) AS subscription_consumption,
 COALESCE(g.balance_consumption,0) AS balance_consumption,CASE WHEN g.user_id IS NULL THEN 0 ELSE g.cost END AS cost,COALESCE(g.requests,0) AS requests,
 COALESCE(c.paid,0) AS paid,COALESCE(c.balance_paid,0) AS balance_paid,COALESCE(c.subscription_paid,0) AS subscription_paid,
 COALESCE(c.refund,0) AS refund,COALESCE(c.uncertain_count,0)+COALESCE(g.missing_fx_count,0) AS uncertain_count,
 CASE WHEN COALESCE(c.uncertain_count,0)=0 AND COALESCE(g.missing_fx_count,0)=0 THEN COALESCE(c.paid,0)-COALESCE(c.refund,0)-COALESCE(g.cost,0) END AS profit,
 COALESCE(a.count,0) AS active_count
 FROM users u LEFT JOIN usage g ON g.user_id=u.id LEFT JOIN cash c ON c.user_id=u.id LEFT JOIN active_cards a ON a.user_id=u.id
 WHERE ($10::bigint=0 OR u.id=$10) AND ($4='' OR strpos(lower(COALESCE(u.username,'') || ' ' || u.email || ' ' || u.id::text),lower($4))>0)
 AND (g.user_id IS NOT NULL OR EXISTS(SELECT 1 FROM usage_logs old WHERE old.user_id=u.id LIMIT 1))
), filtered AS MATERIALIZED (
 SELECT * FROM ranked_users WHERE $5='' OR ($5='used' AND requests>0) OR ($5='paid' AND paid>0) OR ($5='loss' AND profit<0) OR ($5='active' AND active_count>0)
) `

func reportArgs(q service.UserBusinessQuery) []any {
	return []any{q.Start, q.End, q.USDCNY, q.Search, q.Filter, q.PageSize, (q.Page - 1) * q.PageSize, q.Now.In(q.Start.Location()).Location().String(), q.Now, q.UserID, q.CostMode}
}
func (r *userBusinessRepository) Report(ctx context.Context, q service.UserBusinessQuery) (json.RawMessage, error) {
	sortKey := map[string]string{"consumption": "consumption", "paid": "paid", "cost": "cost", "profit": "profit", "balance": "balance"}[q.Sort]
	if sortKey == "" {
		sortKey = "consumption"
	}
	direction := "DESC"
	if q.Order == "asc" {
		direction = "ASC"
	}
	query := userBusinessUsageCTE + userBusinessMoneyCTE + userBusinessRowsCTE + `,
 numbered AS (SELECT *,ROW_NUMBER() OVER(ORDER BY ` + sortKey + " " + direction + ` NULLS LAST,id ASC) AS rank FROM filtered),
 page AS (SELECT * FROM numbered ORDER BY rank LIMIT $6 OFFSET $7)
 SELECT jsonb_build_object('items',COALESCE((SELECT jsonb_agg(to_jsonb(p) ORDER BY rank) FROM page p),'[]'::jsonb),
 'total',(SELECT COUNT(*) FROM filtered),'summary',(SELECT jsonb_build_object('paid',COALESCE(SUM(paid),0),'refund',COALESCE(SUM(refund),0),'consumption',COALESCE(SUM(consumption),0),'cost',CASE WHEN COUNT(*) FILTER(WHERE cost IS NULL)=0 THEN COALESCE(SUM(cost),0) END,'profit',CASE WHEN COALESCE(SUM(uncertain_count),0)=0 THEN COALESCE(SUM(profit),0) END,'uncertain_count',COALESCE(SUM(uncertain_count),0)) FROM filtered),
 'start_at',$1::timestamptz,'end_at',$2::timestamptz,'cost_mode',$11::text,'usd_cny',$3::numeric,'timezone',$8::text,'as_of',$9::timestamptz)
 `
	return r.query(ctx, query, reportArgs(q)...)
}
func (r *userBusinessRepository) query(ctx context.Context, query string, args ...any) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("user business query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var data json.RawMessage
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, sql.ErrNoRows
	}
	if err := rows.Scan(&data); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return data, rows.Close()
}
func (r *userBusinessRepository) Detail(ctx context.Context, q service.UserBusinessQuery) (json.RawMessage, error) {
	query := `WITH usage AS MATERIALIZED (
 SELECT (ul.created_at AT TIME ZONE $8)::date AS day,SUM(actual_cost) AS consumption,
 CASE WHEN COUNT(*) FILTER(WHERE $11='historical' AND fx.usd_cny IS NULL AND COALESCE(account_stats_cost,total_cost)*COALESCE(account_rate_multiplier,1)<>0)>0 THEN NULL
 ELSE COALESCE(SUM(COALESCE(account_stats_cost,total_cost)*COALESCE(account_rate_multiplier,1)*CASE WHEN $11='estimate' THEN $3::numeric ELSE fx.usd_cny END),0) END AS cost
 FROM usage_logs ul LEFT JOIN user_business_daily_fx fx ON fx.rate_date=(ul.created_at AT TIME ZONE $8)::date
 WHERE ul.user_id=$10 AND ul.created_at >= $1 AND ul.created_at < $2 GROUP BY 1
) ` + userBusinessMoneyCTE + `,
 day_cash AS (SELECT (at AT TIME ZONE $8)::date AS day,SUM(paid) AS paid,SUM(refund) AS refund,COUNT(*) FILTER(WHERE uncertain) AS uncertain_count FROM cash_events GROUP BY 1),
 days AS (SELECT generate_series(($1 AT TIME ZONE $8)::date,(($2-interval '1 microsecond') AT TIME ZONE $8)::date,interval '1 day')::date AS day),
 daily AS (SELECT to_char(d.day,'YYYY-MM-DD') AS date,COALESCE(u.consumption,0) AS consumption,CASE WHEN u.day IS NULL THEN 0 ELSE u.cost END AS cost,
 COALESCE(c.paid,0) AS paid,COALESCE(c.refund,0) AS refund,CASE WHEN COALESCE(c.uncertain_count,0)=0 AND (u.day IS NULL OR u.cost IS NOT NULL) THEN COALESCE(c.paid,0)-COALESCE(c.refund,0)-COALESCE(u.cost,0) END AS profit
 FROM days d LEFT JOIN usage u USING(day) LEFT JOIN day_cash c USING(day))
 SELECT jsonb_build_object('daily',COALESCE((SELECT jsonb_agg(to_jsonb(d) ORDER BY date) FROM daily d),'[]'::jsonb),
 'cards',` + userBusinessCardsSQL + `,'balance',(SELECT balance FROM users WHERE id=$10),'as_of',$9::timestamptz,
 'start_at',$1::timestamptz,'end_at',$2::timestamptz,'timezone',$8::text,'usd_cny',$3::numeric,'cost_mode',$11::text,
 'payments',COALESCE((SELECT jsonb_agg(to_jsonb(e) ORDER BY at DESC) FROM (SELECT order_id,at,order_type,paid,refund,uncertain FROM cash_events ORDER BY at DESC LIMIT 200) e),'[]'::jsonb),
 'payments_total',(SELECT COUNT(*) FROM cash_events))
 WHERE $4::text IS NOT NULL AND $5::text IS NOT NULL AND $6::int>0 AND $7::int>=0`
	return r.query(ctx, query, reportArgs(q)...)
}
