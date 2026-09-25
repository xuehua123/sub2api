package repository

import (
	"context"
	"encoding/json"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *userBusinessRepository) ListFX(ctx context.Context, start, last time.Time) (json.RawMessage, error) {
	return r.query(ctx, `SELECT COALESCE(jsonb_agg(jsonb_build_object('date',to_char(d.day,'YYYY-MM-DD'),'usd_cny',f.usd_cny,'source',f.source) ORDER BY d.day),'[]'::jsonb) FROM generate_series($1::date,$2::date,interval '1 day') d(day) LEFT JOIN user_business_daily_fx f ON f.rate_date=d.day::date`, start.Format("2006-01-02"), last.Format("2006-01-02"))
}
func (r *userBusinessRepository) InsertFX(ctx context.Context, p service.UserBusinessFX) error {
	result, err := r.db.ExecContext(ctx, `INSERT INTO user_business_daily_fx(rate_date,usd_cny,source) VALUES($1::date,$2,$3) ON CONFLICT(rate_date) DO UPDATE SET source=user_business_daily_fx.source WHERE user_business_daily_fx.usd_cny=EXCLUDED.usd_cny AND user_business_daily_fx.source=EXCLUDED.source`, p.Date, p.Rate, p.Source)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return infraerrors.Conflict("FX_ALREADY_RECORDED", "A different rate is already recorded for this date; historical rates cannot be silently overwritten")
	}
	return nil
}
