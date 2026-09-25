//go:build unit

package service

import (
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
)

type businessFXStub struct {
	businessRepoStub
	saved []UserBusinessFX
}

func (r *businessFXStub) ListFX(context.Context, time.Time, time.Time) (json.RawMessage, error) {
	return json.RawMessage("[]"), nil
}
func (r *businessFXStub) InsertFX(_ context.Context, p UserBusinessFX) error {
	r.saved = append(r.saved, p)
	return nil
}
func TestUserBusinessFXRequiresEvidence(t *testing.T) {
	r := &businessFXStub{}
	s := NewUserBusinessService(r, nil)
	s.now = func() time.Time { return time.Date(2026, 9, 25, 12, 0, 0, 0, timezone.Location()) }
	ctx := context.Background()
	for _, p := range []UserBusinessFX{{Date: "bad", Rate: 7, Source: "x"}, {Date: "2026-09-26", Rate: 7, Source: "x"}, {Date: "2026-09-24", Rate: 7}, {Date: "2026-09-24", Rate: math.NaN(), Source: "x"}, {Date: "2026-09-24", Rate: -1, Source: "x"}} {
		require.Error(t, s.InsertFX(ctx, p))
	}
	require.Empty(t, r.saved)
	require.NoError(t, s.InsertFX(ctx, UserBusinessFX{Date: "2026-09-24", Rate: 6.5, Source: " evidence "}))
	require.Equal(t, "evidence", r.saved[0].Source)
}
