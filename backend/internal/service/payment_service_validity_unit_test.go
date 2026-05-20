package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeValidityUnit(t *testing.T) {
	require.Equal(t, "", normalizeValidityUnit(""))
	require.Equal(t, validityUnitDay, normalizeValidityUnit("days"))
	require.Equal(t, validityUnitWeek, normalizeValidityUnit("week"))
	require.Equal(t, validityUnitWeek, normalizeValidityUnit("weeks"))
	require.Equal(t, validityUnitMonth, normalizeValidityUnit("months"))
	require.Equal(t, validityUnitYear, normalizeValidityUnit("years"))
	require.Equal(t, "unknown", normalizeValidityUnit("unknown"))
}

func TestIsSupportedValidityUnit(t *testing.T) {
	require.True(t, isSupportedValidityUnit("day"))
	require.True(t, isSupportedValidityUnit("days"))
	require.True(t, isSupportedValidityUnit("week"))
	require.True(t, isSupportedValidityUnit("months"))
	require.True(t, isSupportedValidityUnit("year"))
	require.False(t, isSupportedValidityUnit(""))
	require.False(t, isSupportedValidityUnit("unknown"))
}

func TestPSComputeValidityDaysSupportsCanonicalAndLegacyUnits(t *testing.T) {
	require.Equal(t, 3, psComputeValidityDays(3, "day"))
	require.Equal(t, 3, psComputeValidityDays(3, "days"))
	require.Equal(t, 14, psComputeValidityDays(2, "week"))
	require.Equal(t, 14, psComputeValidityDays(2, "weeks"))
	require.Equal(t, 60, psComputeValidityDays(2, "month"))
	require.Equal(t, 60, psComputeValidityDays(2, "months"))
	require.Equal(t, 365, psComputeValidityDays(1, "year"))
	require.Equal(t, 365, psComputeValidityDays(1, "years"))
}
