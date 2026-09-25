//go:build unit

package service

import (
	"context"
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
	"math"
	"testing"
	"time"
)

type businessRepoStub struct{}

func (businessRepoStub) Report(context.Context, UserBusinessQuery) (json.RawMessage, error) {
	return json.RawMessage("{}"), nil
}
func (businessRepoStub) Detail(context.Context, UserBusinessQuery) (json.RawMessage, error) {
	return json.RawMessage("{}"), nil
}
func TestUserBusinessValidation(t *testing.T) {
	s := NewUserBusinessService(businessRepoStub{}, nil)
	s.now = func() time.Time { return time.Date(2026, 9, 25, 12, 0, 0, 0, timezone.Location()) }
	q, err := s.Normalize(context.Background(), UserBusinessParams{})
	require.NoError(t, err)
	require.Equal(t, "2026-09-25", q.StartDate)
	require.Equal(t, 7.0, q.USDCNY)
	require.Equal(t, "consumption", q.Sort)
 require.Equal(t,"historical",q.CostMode)
	require.Equal(t, 20, q.PageSize)
	for _, p := range []UserBusinessParams{{StartDate: "bad", EndDate: "bad"}, {StartDate: "2026-09-26", EndDate: "2026-09-26"}, {StartDate: "2024-01-01", EndDate: "2026-01-01"}, {CostMode:"bad"}, {Sort: "DROP"}, {Order: "sideways"}, {Filter: "other"}, {USDCNY: math.NaN()}, {USDCNY: math.Inf(1)}, {USDCNY: -7}, {Page: 1000001}} {
		_, err = s.Normalize(context.Background(), p)
		require.Error(t, err)
	}
	p := q.UserBusinessParams
	p.USDCNY = 6.5
	q2, err := s.Normalize(context.Background(), p)
	require.NoError(t, err)
	require.NotEqual(t, q.CacheKey(), q2.CacheKey())
	_, err = s.Detail(context.Background(), q, 0)
	require.Error(t, err)
}
