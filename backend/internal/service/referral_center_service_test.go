//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type referralCenterRepoStub struct {
	*referralRepoStub
	inviteeCounts  ReferralInviteeCounts
	invitees       []ReferralInvitee
	ledgerEntries  []CommissionLedger
	withdrawals    []CommissionWithdrawal
	payoutAccounts []CommissionPayoutAccount
	bucketTotals   map[string]float64
}

func newReferralCenterRepoStub() *referralCenterRepoStub {
	return &referralCenterRepoStub{
		referralRepoStub: newReferralRepoStub(),
		bucketTotals:     make(map[string]float64),
	}
}

func (s *referralCenterRepoStub) CountInvitees(ctx context.Context, userID int64) (*ReferralInviteeCounts, error) {
	result := s.inviteeCounts
	return &result, nil
}

func (s *referralCenterRepoStub) ListInvitees(ctx context.Context, userID int64, params pagination.PaginationParams) ([]ReferralInvitee, *pagination.PaginationResult, error) {
	return s.invitees, &pagination.PaginationResult{Total: int64(len(s.invitees)), Page: params.Page, PageSize: params.PageSize, Pages: 1}, nil
}

func (s *referralCenterRepoStub) SumUserBucketAmount(ctx context.Context, userID int64, bucket string) (float64, error) {
	return s.bucketTotals[bucket], nil
}

func (s *referralCenterRepoStub) ListLedgerEntriesByUser(ctx context.Context, userID int64, params pagination.PaginationParams) ([]CommissionLedger, *pagination.PaginationResult, error) {
	return s.ledgerEntries, &pagination.PaginationResult{Total: int64(len(s.ledgerEntries)), Page: params.Page, PageSize: params.PageSize, Pages: 1}, nil
}

func (s *referralCenterRepoStub) ListWithdrawalsByUser(ctx context.Context, userID int64, params pagination.PaginationParams) ([]CommissionWithdrawal, *pagination.PaginationResult, error) {
	return s.withdrawals, &pagination.PaginationResult{Total: int64(len(s.withdrawals)), Page: params.Page, PageSize: params.PageSize, Pages: 1}, nil
}

func (s *referralCenterRepoStub) ListPayoutAccountsByUser(ctx context.Context, userID int64) ([]CommissionPayoutAccount, error) {
	return s.payoutAccounts, nil
}

func (s *referralCenterRepoStub) ListRewardsByUserAndSource(ctx context.Context, userID int64, sourceUserID int64) ([]UserInviteeReward, error) {
	return nil, nil
}

func newReferralCenterServiceForTest(repo *referralCenterRepoStub, settings map[string]string) *ReferralCenterService {
	cfg := &config.Config{
		Default: config.DefaultConfig{
			UserBalance:     0,
			UserConcurrency: 1,
		},
	}
	settingService := NewSettingService(&settingRepoStub{values: settings}, cfg)
	baseReferralService := NewReferralService(repo.referralRepoStub, &userRepoStub{}, nil, settingService)
	return NewReferralCenterService(baseReferralService, repo, repo, nil)
}

func TestReferralCenterService_GetOverview_AggregatesSummary(t *testing.T) {
	repo := newReferralCenterRepoStub()
	repo.codesByUser[7] = &ReferralCode{
		ID:        1,
		UserID:    7,
		Code:      "REF-007",
		Status:    ReferralCodeStatusActive,
		IsDefault: true,
	}
	repo.relationsByUser[7] = &ReferralRelation{
		ID:             2,
		UserID:         7,
		ReferrerUserID: 99,
		BindSource:     ReferralBindSourceLink,
	}
	repo.inviteeCounts = ReferralInviteeCounts{DirectInvitees: 3, SecondLevelInvitees: 5}
	repo.bucketTotals = map[string]float64{
		CommissionLedgerBucketPending:   12,
		CommissionLedgerBucketAvailable: 34,
		CommissionLedgerBucketFrozen:    5,
		CommissionLedgerBucketSettled:   18,
	}

	svc := newReferralCenterServiceForTest(repo, map[string]string{
		SettingKeyReferralEnabled:          "true",
		SettingKeyReferralAllowManualInput: "true",
		SettingKeyReferralWithdrawEnabled:  "true",
	})

	overview, err := svc.GetOverview(context.Background(), 7)
	require.NoError(t, err)
	require.NotNil(t, overview)
	require.True(t, overview.ReferralWithdrawEnabled)
	require.Equal(t, "REF-007", overview.DefaultCode.Code)
	require.Equal(t, 3, overview.DirectInvitees)
	require.Equal(t, 5, overview.SecondLevelInvitees)
	require.Equal(t, 12.0, overview.PendingCommission)
	require.Equal(t, 34.0, overview.AvailableCommission)
	require.Equal(t, 5.0, overview.FrozenCommission)
	require.Equal(t, 18.0, overview.WithdrawnCommission)
	require.Equal(t, 69.0, overview.TotalCommission)
}

func TestReferralCenterService_ListEndpoints_RejectWhenReferralDisabledForUser(t *testing.T) {
	repo := newReferralCenterRepoStub()
	cfg := &config.Config{
		Default: config.DefaultConfig{
			UserBalance:     0,
			UserConcurrency: 1,
		},
	}
	settingService := NewSettingService(&settingRepoStub{values: map[string]string{
		SettingKeyReferralEnabled: "false",
	}}, cfg)
	baseReferralService := NewReferralService(repo.referralRepoStub, &userRepoStub{user: &User{ID: 7, ReferralEnabled: false}}, nil, settingService)
	svc := NewReferralCenterService(baseReferralService, repo, repo, nil)
	params := pagination.PaginationParams{Page: 1, PageSize: 20}

	_, _, err := svc.ListLedger(context.Background(), 7, params)
	require.ErrorIs(t, err, ErrReferralDisabled)

	_, _, err = svc.ListWithdrawals(context.Background(), 7, params)
	require.ErrorIs(t, err, ErrReferralDisabled)

	_, _, err = svc.ListInvitees(context.Background(), 7, params)
	require.ErrorIs(t, err, ErrReferralDisabled)

	_, err = svc.ListPayoutAccounts(context.Background(), 7)
	require.ErrorIs(t, err, ErrReferralDisabled)

	_, err = svc.ListInviteeRewards(context.Background(), 7, 10)
	require.ErrorIs(t, err, ErrReferralDisabled)
}
