package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type paymentDashboardRepository struct{ db *sql.DB }

func NewPaymentDashboardRepository(db *sql.DB) service.PaymentDashboardRepository {
	return &paymentDashboardRepository{db: db}
}

func (r *paymentDashboardRepository) GetDashboardAggregates(ctx context.Context, start, end time.Time, statuses []string) (*service.PaymentDashboardAggregates, error) {
	if len(statuses) != 3 {
		return nil, sql.ErrNoRows
	}
	const query = `
WITH base AS (
  SELECT user_id, user_email, pay_amount::double precision AS amount, paid_at,
         payment_type,
         COALESCE(NULLIF(UPPER(TRIM(provider_snapshot->>'currency')), ''), 'CNY') AS currency
  FROM payment_orders
  WHERE status IN ($1,$2,$3) AND paid_at >= $4 AND paid_at < $5
),
totals AS (
  SELECT 'total' AS kind, currency, '' AS day, '' AS method, 0::bigint AS user_id, '' AS email, SUM(amount) amount, COUNT(*) count FROM base GROUP BY currency
), daily AS (
  SELECT 'daily', currency, (paid_at AT TIME ZONE current_setting('TIMEZONE'))::date::text, '', 0, '', SUM(amount), COUNT(*) FROM base GROUP BY currency, (paid_at AT TIME ZONE current_setting('TIMEZONE'))::date
), methods AS (
  SELECT 'method', currency, '', payment_type, 0, '', SUM(amount), COUNT(*) FROM base GROUP BY currency, payment_type
), users AS (
  SELECT 'user', currency, '', '', user_id, MAX(user_email), SUM(amount), COUNT(*) FROM base GROUP BY currency, user_id
)
SELECT kind, currency, day, method, user_id, email, amount, count FROM totals
UNION ALL SELECT * FROM daily
UNION ALL SELECT * FROM methods
UNION ALL SELECT * FROM users`
	rows, err := r.db.QueryContext(ctx, query, statuses[0], statuses[1], statuses[2], start, end)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := &service.PaymentDashboardAggregates{}
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM payment_orders WHERE status = 'PENDING' AND created_at >= $1 AND created_at < $2`, start, end).Scan(&out.PendingCount); err != nil {
		return nil, err
	}
	for rows.Next() {
		var kind, currency, day, method, email string
		var userID int64
		var amount float64
		var count int
		if err := rows.Scan(&kind, &currency, &day, &method, &userID, &email, &amount, &count); err != nil {
			return nil, err
		}
		a := service.PaymentDashboardAggregate{Currency: currency, Date: day, PaymentType: method, UserID: userID, Email: email, Amount: amount, Count: count}
		switch kind {
		case "total":
			out.Totals = append(out.Totals, a)
		case "daily":
			out.Daily = append(out.Daily, a)
		case "method":
			out.Methods = append(out.Methods, a)
		case "user":
			out.Users = append(out.Users, a)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
