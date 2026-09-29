//go:build unit

package service

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestPaymentDateRange(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, timezone.Location())
	r, err := ParsePaymentDateRange("2026-08-01", "2026-08-31", 30, now)
	require.NoError(t, err)
	require.Equal(t, 31, r.Days)
	require.Equal(t, "2026-09-01", r.End.Format(time.DateOnly))
	r, err = ParsePaymentDateRange("", "", 30, now)
	require.NoError(t, err)
	require.Equal(t, "2026-08-30", r.Start.Format(time.DateOnly))
	for _, dates := range [][2]string{{"2026-02-30", "2026-03-01"}, {"2026-08-01", ""}, {"2026-09-28", "2026-09-01"}, {"2026-09-28", "2026-09-29"}, {"2024-01-01", "2026-01-01"}} {
		_, err = ParsePaymentDateRange(dates[0], dates[1], 30, now)
		require.Error(t, err, dates)
	}
}
