package service

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

type UserBusinessFX struct {
	Date   string  `json:"date"`
	Rate   float64 `json:"usd_cny"`
	Source string  `json:"source"`
}
type UserBusinessFXRepository interface {
	ListFX(context.Context, time.Time, time.Time) (json.RawMessage, error)
	InsertFX(context.Context, UserBusinessFX) error
}

func (s *UserBusinessService) ListFX(ctx context.Context, p UserBusinessParams) (json.RawMessage, error) {
	q, err := s.Normalize(ctx, p)
	if err != nil {
		return nil, err
	}
	r, ok := s.repo.(UserBusinessFXRepository)
	if !ok {
		return nil, infraerrors.ServiceUnavailable("FX_UNAVAILABLE", "FX repository unavailable")
	}
	last, _ := time.ParseInLocation("2006-01-02", p.EndDate, timezone.Location())
	if p.EndDate == "" {
		last = timezone.StartOfDay(q.Now)
	}
	return r.ListFX(ctx, q.Start, last)
}
func (s *UserBusinessService) InsertFX(ctx context.Context, p UserBusinessFX) error {
	date, err := time.ParseInLocation("2006-01-02", p.Date, timezone.Location())
	if err != nil || date.After(timezone.StartOfDay(s.now().In(timezone.Location()))) {
		return infraerrors.BadRequest("INVALID_DATE", "Invalid FX date")
	}
	p.Source = strings.TrimSpace(p.Source)
	if p.Rate <= 0 || p.Rate > 100 || math.IsNaN(p.Rate) || math.IsInf(p.Rate, 0) || p.Source == "" || len(p.Source) > 200 {
		return infraerrors.BadRequest("INVALID_FX", "Provide a valid daily rate and source")
	}
	r, ok := s.repo.(UserBusinessFXRepository)
	if !ok {
		return infraerrors.ServiceUnavailable("FX_UNAVAILABLE", "FX repository unavailable")
	}
	return r.InsertFX(ctx, p)
}
