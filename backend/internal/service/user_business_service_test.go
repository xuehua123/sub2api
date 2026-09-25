//go:build unit

package service

import (
	"context"
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
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
	s := NewUserBusinessService(businessRepoStub{})
	s.now = func() time.Time { return time.Date(2026, 9, 25, 12, 0, 0, 0, timezone.Location()) }
	q, err := s.Normalize(context.Background(), UserBusinessParams{})
	require.NoError(t, err)
	require.Equal(t, "2026-09-25", q.StartDate)
	require.Equal(t, "consumption", q.Sort)
	require.Equal(t, 20, q.PageSize)
	for _, p := range []UserBusinessParams{{StartDate: "bad", EndDate: "bad"}, {StartDate: "2026-09-26", EndDate: "2026-09-26"}, {StartDate: "2024-01-01", EndDate: "2026-01-01"}, {Sort: "DROP"}, {Order: "sideways"}, {Filter: "other"}, {Page: 1000001}} {
		_, err = s.Normalize(context.Background(), p)
		require.Error(t, err)
	}
	p := q.UserBusinessParams
	p.Sort = "paid"
	q2, err := s.Normalize(context.Background(), p)
	require.NoError(t, err)
	require.NotEqual(t, q.CacheKey(), q2.CacheKey())
	_, err = s.Detail(context.Background(), q, 0)
	require.Error(t, err)
}
