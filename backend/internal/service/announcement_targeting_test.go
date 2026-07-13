package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAnnouncementTargeting_Matches_EmptyMatchesAll(t *testing.T) {
	var targeting AnnouncementTargeting
	require.True(t, targeting.Matches(0, nil))
	require.True(t, targeting.Matches(123.45, map[int64]struct{}{1: {}}))
}

func TestAnnouncementTargeting_MatchesUser_OnlyAllowsExplicitUsers(t *testing.T) {
	targeting := AnnouncementTargeting{UserIDs: []int64{1012, 2048}}

	require.True(t, targeting.MatchesUser(1012, 0, nil))
	require.True(t, targeting.MatchesUser(2048, 0, nil))
	require.False(t, targeting.MatchesUser(999, 0, nil))
	require.False(t, targeting.Matches(0, nil), "缺少用户身份时不得命中定向公告")
}

func TestAnnouncementTargeting_NormalizeAndValidate_UserIDs(t *testing.T) {
	targeting := AnnouncementTargeting{UserIDs: []int64{2048, 1012, 2048}}

	normalized, err := targeting.NormalizeAndValidate()
	require.NoError(t, err)
	require.Equal(t, []int64{1012, 2048}, normalized.UserIDs)

	_, err = (AnnouncementTargeting{UserIDs: []int64{0}}).NormalizeAndValidate()
	require.ErrorIs(t, err, ErrAnnouncementInvalidTarget)

	_, err = (AnnouncementTargeting{UserIDs: []int64{}}).NormalizeAndValidate()
	require.ErrorIs(t, err, ErrAnnouncementInvalidTarget)
}

func TestAnnouncementTargeting_NormalizeAndValidate_RejectsEmptyGroup(t *testing.T) {
	targeting := AnnouncementTargeting{
		AnyOf: []AnnouncementConditionGroup{
			{AllOf: nil},
		},
	}
	_, err := targeting.NormalizeAndValidate()
	require.Error(t, err)
	require.ErrorIs(t, err, ErrAnnouncementInvalidTarget)
}

func TestAnnouncementTargeting_NormalizeAndValidate_RejectsInvalidCondition(t *testing.T) {
	targeting := AnnouncementTargeting{
		AnyOf: []AnnouncementConditionGroup{
			{
				AllOf: []AnnouncementCondition{
					{Type: "balance", Operator: "between", Value: 10},
				},
			},
		},
	}
	_, err := targeting.NormalizeAndValidate()
	require.Error(t, err)
	require.ErrorIs(t, err, ErrAnnouncementInvalidTarget)
}

func TestAnnouncementTargeting_Matches_AndOrSemantics(t *testing.T) {
	targeting := AnnouncementTargeting{
		AnyOf: []AnnouncementConditionGroup{
			{
				AllOf: []AnnouncementCondition{
					{Type: AnnouncementConditionTypeBalance, Operator: AnnouncementOperatorGTE, Value: 100},
					{Type: AnnouncementConditionTypeSubscription, Operator: AnnouncementOperatorIn, GroupIDs: []int64{10}},
				},
			},
			{
				AllOf: []AnnouncementCondition{
					{Type: AnnouncementConditionTypeBalance, Operator: AnnouncementOperatorLT, Value: 5},
				},
			},
		},
	}

	// 命中第 2 组（balance < 5）
	require.True(t, targeting.Matches(4.99, nil))
	require.False(t, targeting.Matches(5, nil))

	// 命中第 1 组（balance >= 100 AND 订阅 in [10]）
	require.False(t, targeting.Matches(100, map[int64]struct{}{}))
	require.False(t, targeting.Matches(99.9, map[int64]struct{}{10: {}}))
	require.True(t, targeting.Matches(100, map[int64]struct{}{10: {}}))
}
