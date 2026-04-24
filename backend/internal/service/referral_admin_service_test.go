//go:build unit

package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type adminSearchUserRepoStub struct {
	users []User
}

func (s *adminSearchUserRepoStub) Create(context.Context, *User) error { panic("unexpected") }
func (s *adminSearchUserRepoStub) GetByID(_ context.Context, id int64) (*User, error) {
	for i := range s.users {
		if s.users[i].ID == id {
			user := s.users[i]
			return &user, nil
		}
	}
	return nil, ErrUserNotFound
}
func (s *adminSearchUserRepoStub) GetByEmail(context.Context, string) (*User, error) {
	panic("unexpected")
}
func (s *adminSearchUserRepoStub) GetFirstAdmin(context.Context) (*User, error) { panic("unexpected") }
func (s *adminSearchUserRepoStub) Update(context.Context, *User) error          { panic("unexpected") }
func (s *adminSearchUserRepoStub) Delete(context.Context, int64) error          { panic("unexpected") }
func (s *adminSearchUserRepoStub) GetUserAvatar(context.Context, int64) (*UserAvatar, error) {
	panic("unexpected")
}
func (s *adminSearchUserRepoStub) UpsertUserAvatar(context.Context, int64, UpsertUserAvatarInput) (*UserAvatar, error) {
	panic("unexpected")
}
func (s *adminSearchUserRepoStub) DeleteUserAvatar(context.Context, int64) error { panic("unexpected") }
func (s *adminSearchUserRepoStub) List(context.Context, pagination.PaginationParams) ([]User, *pagination.PaginationResult, error) {
	panic("unexpected")
}
func (s *adminSearchUserRepoStub) ListWithFilters(_ context.Context, params pagination.PaginationParams, filters UserListFilters) ([]User, *pagination.PaginationResult, error) {
	var items []User
	for _, user := range s.users {
		if filters.Search == "" || strings.Contains(strings.ToLower(user.Email), strings.ToLower(filters.Search)) || strings.Contains(strings.ToLower(user.Username), strings.ToLower(filters.Search)) {
			items = append(items, user)
		}
	}
	if len(items) > params.PageSize && params.PageSize > 0 {
		items = items[:params.PageSize]
	}
	return items, &pagination.PaginationResult{Total: int64(len(items)), Page: params.Page, PageSize: params.PageSize, Pages: 1}, nil
}
func (s *adminSearchUserRepoStub) UpdateBalance(context.Context, int64, float64) error {
	panic("unexpected")
}
func (s *adminSearchUserRepoStub) DeductBalance(context.Context, int64, float64) error {
	panic("unexpected")
}
func (s *adminSearchUserRepoStub) UpdateConcurrency(context.Context, int64, int) error {
	panic("unexpected")
}
func (s *adminSearchUserRepoStub) ExistsByEmail(context.Context, string) (bool, error) {
	panic("unexpected")
}
func (s *adminSearchUserRepoStub) RemoveGroupFromAllowedGroups(context.Context, int64) (int64, error) {
	panic("unexpected")
}
func (s *adminSearchUserRepoStub) AddGroupToAllowedGroups(context.Context, int64, int64) error {
	panic("unexpected")
}
func (s *adminSearchUserRepoStub) RemoveGroupFromUserAllowedGroups(context.Context, int64, int64) error {
	panic("unexpected")
}
func (s *adminSearchUserRepoStub) GetLatestUsedAtByUserIDs(context.Context, []int64) (map[int64]*time.Time, error) {
	panic("unexpected")
}
func (s *adminSearchUserRepoStub) GetLatestUsedAtByUserID(context.Context, int64) (*time.Time, error) {
	panic("unexpected")
}
func (s *adminSearchUserRepoStub) UpdateUserLastActiveAt(context.Context, int64, time.Time) error {
	panic("unexpected")
}
func (s *adminSearchUserRepoStub) UpdateDefaultChatAPIKeyID(context.Context, int64, *int64) error {
	panic("unexpected")
}
func (s *adminSearchUserRepoStub) ListUserAuthIdentities(context.Context, int64) ([]UserAuthIdentityRecord, error) {
	panic("unexpected")
}
func (s *adminSearchUserRepoStub) UnbindUserAuthProvider(context.Context, int64, string) error {
	panic("unexpected")
}
func (s *adminSearchUserRepoStub) UpdateTotpSecret(context.Context, int64, *string) error {
	panic("unexpected")
}
func (s *adminSearchUserRepoStub) EnableTotp(context.Context, int64) error  { panic("unexpected") }
func (s *adminSearchUserRepoStub) DisableTotp(context.Context, int64) error { panic("unexpected") }

type adminReferralRepoStub struct {
	*referralRepoStub
	adminRelationsByUser  map[int64]*AdminReferralRelation
	listRelations         []AdminReferralRelation
	listRelationHistories []ReferralRelationHistory
	inviteeCounts         map[int64]*ReferralInviteeCounts
	inviteesByUser        map[int64][]ReferralInvitee
}

func newAdminReferralRepoStub() *adminReferralRepoStub {
	return &adminReferralRepoStub{
		referralRepoStub:     newReferralRepoStub(),
		adminRelationsByUser: make(map[int64]*AdminReferralRelation),
		inviteeCounts:        make(map[int64]*ReferralInviteeCounts),
		inviteesByUser:       make(map[int64][]ReferralInvitee),
	}
}

func (s *adminReferralRepoStub) UpsertRelation(ctx context.Context, relation *ReferralRelation) error {
	if existing, ok := s.relationsByUser[relation.UserID]; ok {
		relation.ID = existing.ID
		if relation.CreatedAt.IsZero() {
			relation.CreatedAt = existing.CreatedAt
		}
	} else if relation.ID == 0 {
		relation.ID = s.nextRelationID
		s.nextRelationID++
	}
	if relation.CreatedAt.IsZero() {
		relation.CreatedAt = time.Now()
	}
	relation.UpdatedAt = time.Now()
	cloned := *relation
	s.relationsByUser[relation.UserID] = &cloned
	return nil
}

func (s *adminReferralRepoStub) ListRelations(ctx context.Context, params pagination.PaginationParams, search string) ([]AdminReferralRelation, *pagination.PaginationResult, error) {
	return s.listRelations, &pagination.PaginationResult{Total: int64(len(s.listRelations)), Page: params.Page, PageSize: params.PageSize, Pages: 1}, nil
}

func (s *adminReferralRepoStub) GetAdminRelationByUserID(ctx context.Context, userID int64) (*AdminReferralRelation, error) {
	if relation, ok := s.adminRelationsByUser[userID]; ok {
		cloned := *relation
		return &cloned, nil
	}
	relation, err := s.GetRelationByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	item := &AdminReferralRelation{
		UserID:     relation.UserID,
		BindSource: relation.BindSource,
		BindCode:   relation.BindCode,
		LockedAt:   relation.LockedAt,
		CreatedAt:  relation.CreatedAt,
		UpdatedAt:  relation.UpdatedAt,
	}
	for _, listed := range s.listRelations {
		if listed.UserID == userID {
			cloned := listed
			return &cloned, nil
		}
	}
	return item, nil
}

func (s *adminReferralRepoStub) ListRelationHistories(ctx context.Context, params pagination.PaginationParams, userID int64) ([]ReferralRelationHistory, *pagination.PaginationResult, error) {
	return s.listRelationHistories, &pagination.PaginationResult{Total: int64(len(s.listRelationHistories)), Page: params.Page, PageSize: params.PageSize, Pages: 1}, nil
}

func (s *adminReferralRepoStub) CountInvitees(ctx context.Context, userID int64) (*ReferralInviteeCounts, error) {
	if counts, ok := s.inviteeCounts[userID]; ok {
		cloned := *counts
		return &cloned, nil
	}
	return &ReferralInviteeCounts{}, nil
}

func (s *adminReferralRepoStub) ListInvitees(ctx context.Context, userID int64, params pagination.PaginationParams) ([]ReferralInvitee, *pagination.PaginationResult, error) {
	items := s.inviteesByUser[userID]
	return items, &pagination.PaginationResult{Total: int64(len(items)), Page: params.Page, PageSize: params.PageSize, Pages: 1}, nil
}

func (s *adminReferralRepoStub) CountAllInvitees(ctx context.Context) (map[int64]*ReferralInviteeCounts, error) {
	result := make(map[int64]*ReferralInviteeCounts)
	for userID, counts := range s.inviteeCounts {
		cloned := *counts
		result[userID] = &cloned
	}
	return result, nil
}

type adminCommissionRepoStub struct {
	rewards         map[int64]*CommissionReward
	ledgers         []CommissionLedger
	withdrawals     []CommissionWithdrawal
	withdrawalItems []CommissionWithdrawalItem
	rewardRows      []AdminCommissionReward
	ledgerRows      []AdminCommissionLedger
	withdrawalRows  []AdminCommissionWithdrawal
}

func newAdminCommissionRepoStub() *adminCommissionRepoStub {
	return &adminCommissionRepoStub{
		rewards: make(map[int64]*CommissionReward),
	}
}

func (s *adminCommissionRepoStub) GetRewardByID(ctx context.Context, rewardID int64) (*CommissionReward, error) {
	if reward, ok := s.rewards[rewardID]; ok {
		cloned := *reward
		return &cloned, nil
	}
	return nil, ErrRechargeOrderNotFound
}

func (s *adminCommissionRepoStub) UpdateReward(ctx context.Context, reward *CommissionReward) error {
	cloned := *reward
	s.rewards[reward.ID] = &cloned
	return nil
}

func (s *adminCommissionRepoStub) CreateLedgerEntries(ctx context.Context, entries []CommissionLedger) error {
	for _, entry := range entries {
		cloned := entry
		cloned.ID = int64(len(s.ledgers) + 1)
		cloned.CreatedAt = time.Now()
		s.ledgers = append(s.ledgers, cloned)
	}
	return nil
}

func (s *adminCommissionRepoStub) SumRewardBucketAmount(ctx context.Context, rewardID int64, bucket string) (float64, error) {
	total := 0.0
	for _, ledger := range s.ledgers {
		if ledger.RewardID != nil && *ledger.RewardID == rewardID && ledger.Bucket == bucket {
			total += ledger.Amount
		}
	}
	return total, nil
}

func (s *adminCommissionRepoStub) SumRewardBucketAmountForUpdate(ctx context.Context, rewardID int64, bucket string, forUpdate bool) (float64, error) {
	return s.SumRewardBucketAmount(ctx, rewardID, bucket)
}

func (s *adminCommissionRepoStub) ListCommissionRewards(ctx context.Context, params pagination.PaginationParams, filter AdminCommissionRewardFilter) ([]AdminCommissionReward, *pagination.PaginationResult, error) {
	rows := s.rewardRows
	if filter.UserID > 0 {
		filtered := make([]AdminCommissionReward, 0)
		for _, row := range rows {
			if row.UserID == filter.UserID {
				filtered = append(filtered, row)
			}
		}
		rows = filtered
	}
	return rows, &pagination.PaginationResult{Total: int64(len(rows)), Page: params.Page, PageSize: params.PageSize, Pages: 1}, nil
}

func (s *adminCommissionRepoStub) ListCommissionLedgers(ctx context.Context, params pagination.PaginationParams, filter AdminCommissionLedgerFilter) ([]AdminCommissionLedger, *pagination.PaginationResult, error) {
	rows := s.ledgerRows
	if filter.UserID > 0 {
		filtered := make([]AdminCommissionLedger, 0)
		for _, row := range rows {
			if row.UserID == filter.UserID {
				filtered = append(filtered, row)
			}
		}
		rows = filtered
	}
	return rows, &pagination.PaginationResult{Total: int64(len(rows)), Page: params.Page, PageSize: params.PageSize, Pages: 1}, nil
}

func (s *adminCommissionRepoStub) ListAdminWithdrawals(ctx context.Context, params pagination.PaginationParams, filter AdminCommissionWithdrawalFilter) ([]AdminCommissionWithdrawal, *pagination.PaginationResult, error) {
	rows := s.withdrawalRows
	filtered := make([]AdminCommissionWithdrawal, 0, len(rows))
	for _, row := range rows {
		if filter.UserID > 0 && row.UserID != filter.UserID {
			continue
		}
		if filter.Status != "" && row.Status != filter.Status {
			continue
		}
		filtered = append(filtered, row)
	}
	return filtered, &pagination.PaginationResult{Total: int64(len(filtered)), Page: params.Page, PageSize: params.PageSize, Pages: 1}, nil
}

func (s *adminCommissionRepoStub) ListWithdrawalItemsByWithdrawal(ctx context.Context, withdrawalID int64) ([]CommissionWithdrawalItem, error) {
	return s.withdrawalItems, nil
}

func (s *adminCommissionRepoStub) SumUserBucketAmount(ctx context.Context, userID int64, bucket string) (float64, error) {
	total := 0.0
	for _, ledger := range s.ledgers {
		if ledger.UserID == userID && ledger.Bucket == bucket {
			total += ledger.Amount
		}
	}
	return total, nil
}

func (s *adminCommissionRepoStub) SumUserBucketAmountForUpdate(ctx context.Context, userID int64, bucket string, forUpdate bool) (float64, error) {
	return s.SumUserBucketAmount(ctx, userID, bucket)
}

func (s *adminCommissionRepoStub) SumAllUserBucketAmounts(ctx context.Context, bucket string) (map[int64]float64, error) {
	result := make(map[int64]float64)
	for _, ledger := range s.ledgers {
		if ledger.Bucket == bucket {
			result[ledger.UserID] += ledger.Amount
		}
	}
	return result, nil
}

func newReferralAdminServiceForTest(refRepo *adminReferralRepoStub, commissionRepo *adminCommissionRepoStub, userRepo UserRepository) *ReferralAdminService {
	cfg := &config.Config{
		Default: config.DefaultConfig{
			UserBalance:     0,
			UserConcurrency: 1,
		},
	}
	if userRepo == nil {
		userRepo = &userRepoStub{}
	}
	baseReferralService := NewReferralService(refRepo.referralRepoStub, userRepo, nil, NewSettingService(&settingRepoStub{values: map[string]string{
		SettingKeyReferralEnabled:          "true",
		SettingKeyReferralAllowManualInput: "true",
	}}, cfg))
	return NewReferralAdminService(baseReferralService, refRepo, commissionRepo, nil, nil)
}

type adminReferralWrappedNotFoundRepoStub struct {
	*adminReferralRepoStub
}

func (s *adminReferralWrappedNotFoundRepoStub) GetRelationByUserID(ctx context.Context, userID int64) (*ReferralRelation, error) {
	relation, err := s.adminReferralRepoStub.GetRelationByUserID(ctx, userID)
	if err == ErrReferralRelationNotFound {
		return nil, ErrReferralRelationNotFound.WithCause(err)
	}
	return relation, err
}

func TestReferralAdminService_UpdateRelation_CreatesRelationWhenUserHasNoExistingRelation(t *testing.T) {
	baseRepo := newAdminReferralRepoStub()
	refRepo := &adminReferralWrappedNotFoundRepoStub{adminReferralRepoStub: baseRepo}
	refRepo.codesByCode["NEWCODE"] = &ReferralCode{
		ID:        1,
		UserID:    88,
		Code:      "NEWCODE",
		Status:    ReferralCodeStatusActive,
		IsDefault: true,
	}
	commissionRepo := newAdminCommissionRepoStub()
	userRepo := &userRepoStub{}
	baseReferralService := NewReferralService(refRepo, userRepo, nil, NewSettingService(&settingRepoStub{values: map[string]string{
		SettingKeyReferralEnabled:          "true",
		SettingKeyReferralAllowManualInput: "true",
	}}, &config.Config{}))
	svc := NewReferralAdminService(baseReferralService, refRepo, commissionRepo, nil, nil)

	relation, err := svc.UpdateRelation(context.Background(), &AdminUpdateReferralRelationInput{
		UserID:    7,
		Code:      "NEWCODE",
		ChangedBy: 9,
		Reason:    "manual correction",
	})
	require.NoError(t, err)
	require.Equal(t, int64(88), relation.ReferrerUserID)
	require.Equal(t, ReferralBindSourceAdminOverride, relation.BindSource)
	require.Len(t, refRepo.relationHistory, 1)
	require.Nil(t, refRepo.relationHistory[0].OldReferrerUserID)
}

func TestReferralAdminService_UpdateRelation_RebindsAndWritesHistory(t *testing.T) {
	refRepo := newAdminReferralRepoStub()
	refRepo.codesByCode["NEWCODE"] = &ReferralCode{
		ID:        1,
		UserID:    88,
		Code:      "NEWCODE",
		Status:    ReferralCodeStatusActive,
		IsDefault: true,
	}
	refRepo.relationsByUser[7] = &ReferralRelation{
		ID:             1,
		UserID:         7,
		ReferrerUserID: 66,
		BindSource:     ReferralBindSourceLink,
		CreatedAt:      time.Now().Add(-time.Hour),
		UpdatedAt:      time.Now().Add(-time.Hour),
	}
	commissionRepo := newAdminCommissionRepoStub()
	svc := newReferralAdminServiceForTest(refRepo, commissionRepo, nil)

	relation, err := svc.UpdateRelation(context.Background(), &AdminUpdateReferralRelationInput{
		UserID:    7,
		Code:      "NEWCODE",
		ChangedBy: 9,
		Reason:    "manual correction",
		Notes:     "admin override",
	})
	require.NoError(t, err)
	require.Equal(t, int64(88), relation.ReferrerUserID)
	require.Equal(t, ReferralBindSourceAdminOverride, relation.BindSource)
	require.Len(t, refRepo.relationHistory, 1)
	require.NotNil(t, refRepo.relationHistory[0].OldReferrerUserID)
	require.Equal(t, int64(66), *refRepo.relationHistory[0].OldReferrerUserID)
}

func TestReferralAdminService_UpdateRelation_UsesReferrerUserIDAndCreatesMissingDefaultCode(t *testing.T) {
	refRepo := newAdminReferralRepoStub()
	commissionRepo := newAdminCommissionRepoStub()
	svc := newReferralAdminServiceForTest(refRepo, commissionRepo, nil)

	relation, err := svc.UpdateRelation(context.Background(), &AdminUpdateReferralRelationInput{
		UserID:         7,
		ReferrerUserID: 88,
		ChangedBy:      9,
		Reason:         "manual correction",
	})
	require.NoError(t, err)
	require.Equal(t, int64(88), relation.ReferrerUserID)
	require.NotNil(t, relation.BindCode)
	require.NotEmpty(t, *relation.BindCode)
	code := refRepo.codesByUser[88]
	require.NotNil(t, code)
	require.Equal(t, *relation.BindCode, code.Code)
}

func TestReferralAdminService_GetRelation_FetchesCurrentRelationByUserID(t *testing.T) {
	refRepo := newAdminReferralRepoStub()
	refRepo.listRelations = []AdminReferralRelation{
		{UserID: 101, UserEmail: "other@example.com", Username: "other", ReferrerUserID: int64ValuePtr(88), ReferrerEmail: stringValuePtr("parent@example.com")},
	}
	refRepo.adminRelationsByUser[7] = &AdminReferralRelation{
		UserID:           7,
		UserEmail:        "target@example.com",
		Username:         "target",
		ReferrerUserID:   int64ValuePtr(99),
		ReferrerEmail:    stringValuePtr("real-parent@example.com"),
		ReferrerUsername: stringValuePtr("real-parent"),
		BindSource:       ReferralBindSourceLink,
		BindCode:         stringValuePtr("PARENT99"),
	}
	userRepo := &adminSearchUserRepoStub{
		users: []User{
			{ID: 7, Email: "target@example.com", Username: "target"},
			{ID: 99, Email: "real-parent@example.com", Username: "real-parent"},
		},
	}
	svc := newReferralAdminServiceForTest(refRepo, newAdminCommissionRepoStub(), userRepo)

	relation, err := svc.GetRelation(context.Background(), 7)
	require.NoError(t, err)
	require.NotNil(t, relation)
	require.Equal(t, int64(7), relation.UserID)
	require.NotNil(t, relation.ReferrerUserID)
	require.Equal(t, int64(99), *relation.ReferrerUserID)
	require.NotNil(t, relation.ReferrerEmail)
	require.Equal(t, "real-parent@example.com", *relation.ReferrerEmail)
}

func TestReferralAdminService_CreateCommissionAdjustment_UpdatesRewardBalance(t *testing.T) {
	refRepo := newAdminReferralRepoStub()
	commissionRepo := newAdminCommissionRepoStub()
	commissionRepo.rewards[1] = &CommissionReward{
		ID:              1,
		UserID:          200,
		RechargeOrderID: 11,
		RewardAmount:    10,
		Currency:        ReferralSettlementCurrencyCNY,
		Status:          CommissionRewardStatusAvailable,
	}
	commissionRepo.ledgers = []CommissionLedger{
		{ID: 1, UserID: 200, RewardID: int64ValuePtr(1), RechargeOrderID: int64ValuePtr(11), Bucket: CommissionLedgerBucketAvailable, Amount: 10, Currency: ReferralSettlementCurrencyCNY},
	}
	svc := newReferralAdminServiceForTest(refRepo, commissionRepo, nil)

	ledger, err := svc.CreateCommissionAdjustment(context.Background(), &AdminCommissionAdjustmentInput{
		RewardID:       1,
		OperatorUserID: 9,
		Amount:         -3,
		Remark:         "manual deduct",
	})
	require.NoError(t, err)
	require.Equal(t, CommissionLedgerEntryAdminSubtract, ledger.EntryType)
	require.Len(t, commissionRepo.ledgers, 2)
	available, err := commissionRepo.SumRewardBucketAmount(context.Background(), 1, CommissionLedgerBucketAvailable)
	require.NoError(t, err)
	require.Equal(t, 7.0, available)
	require.Equal(t, CommissionRewardStatusAvailable, commissionRepo.rewards[1].Status)
}

func TestReferralAdminService_SearchAccounts_ReturnsExistingCodesWithoutCreating(t *testing.T) {
	refRepo := newAdminReferralRepoStub()
	refRepo.codesByUser[7] = &ReferralCode{
		ID:        1,
		UserID:    7,
		Code:      "ALPHA01",
		Status:    ReferralCodeStatusActive,
		IsDefault: true,
	}
	commissionRepo := newAdminCommissionRepoStub()
	userRepo := &adminSearchUserRepoStub{
		users: []User{
			{ID: 7, Email: "alpha@example.com", Username: "alpha"},
			{ID: 8, Email: "bravo@example.com", Username: "bravo"},
		},
	}

	svc := newReferralAdminServiceForTest(refRepo, commissionRepo, userRepo)
	options, err := svc.SearchAccounts(context.Background(), "alp", 10)
	require.NoError(t, err)
	require.Len(t, options, 1)
	require.Equal(t, int64(7), options[0].UserID)
	require.Equal(t, "alpha@example.com", options[0].Email)
	require.Equal(t, "ALPHA01", options[0].ReferralCode)
	require.Nil(t, refRepo.codesByUser[8])
	require.Len(t, refRepo.codesByUser, 1)
}

func TestReferralAdminService_SearchAccounts_DoesNotCreateMissingCode(t *testing.T) {
	refRepo := newAdminReferralRepoStub()
	commissionRepo := newAdminCommissionRepoStub()
	userRepo := &adminSearchUserRepoStub{
		users: []User{
			{ID: 8, Email: "bravo@example.com", Username: "bravo"},
		},
	}

	svc := newReferralAdminServiceForTest(refRepo, commissionRepo, userRepo)
	options, err := svc.SearchAccounts(context.Background(), "bravo", 10)
	require.NoError(t, err)
	require.Len(t, options, 1)
	require.Equal(t, int64(8), options[0].UserID)
	require.Empty(t, options[0].ReferralCode)
	require.Empty(t, refRepo.codesByUser)
	require.Empty(t, refRepo.codesByCode)
}

func TestReferralAdminService_GetOverview_ReturnsRankingAndStats(t *testing.T) {
	now := time.Now()
	refRepo := newAdminReferralRepoStub()
	refRepo.listRelations = []AdminReferralRelation{
		{UserID: 7, UserEmail: "alpha@example.com", Username: "alpha", ReferrerUserID: int64ValuePtr(8), ReferrerEmail: stringValuePtr("bravo@example.com"), ReferrerUsername: stringValuePtr("bravo")},
	}
	refRepo.inviteeCounts[7] = &ReferralInviteeCounts{DirectInvitees: 2, SecondLevelInvitees: 3}
	refRepo.inviteeCounts[8] = &ReferralInviteeCounts{DirectInvitees: 1, SecondLevelInvitees: 0}

	commissionRepo := newAdminCommissionRepoStub()
	commissionRepo.rewardRows = []AdminCommissionReward{
		{CommissionReward: CommissionReward{ID: 1, UserID: 7, RewardAmount: 18, CreatedAt: now.AddDate(0, 0, -1)}, UserEmail: "alpha@example.com", Username: "alpha"},
		{CommissionReward: CommissionReward{ID: 2, UserID: 8, RewardAmount: 22, CreatedAt: now.AddDate(0, 0, -3)}, UserEmail: "bravo@example.com", Username: "bravo"},
	}
	commissionRepo.ledgers = []CommissionLedger{
		{UserID: 7, Bucket: CommissionLedgerBucketPending, Amount: 10},
		{UserID: 7, Bucket: CommissionLedgerBucketAvailable, Amount: 20},
		{UserID: 8, Bucket: CommissionLedgerBucketSettled, Amount: 15},
	}
	commissionRepo.withdrawalRows = []AdminCommissionWithdrawal{
		{CommissionWithdrawal: CommissionWithdrawal{ID: 1, UserID: 7, NetAmount: 12, Status: CommissionWithdrawalStatusPendingReview, CreatedAt: now.AddDate(0, 0, -1)}},
		{CommissionWithdrawal: CommissionWithdrawal{ID: 2, UserID: 8, NetAmount: 6, Status: CommissionWithdrawalStatusApproved, CreatedAt: now.AddDate(0, 0, -3)}},
	}
	userRepo := &adminSearchUserRepoStub{
		users: []User{
			{ID: 7, Email: "alpha@example.com", Username: "alpha"},
			{ID: 8, Email: "bravo@example.com", Username: "bravo"},
		},
	}

	svc := newReferralAdminServiceForTest(refRepo, commissionRepo, userRepo)
	overview, err := svc.GetOverview(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, overview.TotalAccounts)
	require.Equal(t, 1, overview.TotalBoundUsers)
	require.Equal(t, 10.0, overview.PendingCommission)
	require.Equal(t, 20.0, overview.AvailableCommission)
	require.Equal(t, 15.0, overview.WithdrawnCommission)
	require.Equal(t, 1, overview.PendingWithdrawalCount)
	require.Equal(t, 12.0, overview.PendingWithdrawalAmount)
	require.Len(t, overview.Ranking, 2)
	require.Equal(t, int64(7), overview.Ranking[0].UserID)
	require.Len(t, overview.RecentTrend, 7)
	require.Equal(t, now.AddDate(0, 0, -1).Format("2006-01-02"), overview.RecentTrend[5].Date)
	require.Equal(t, 18.0, overview.RecentTrend[5].RewardAmount)
	require.Equal(t, 12.0, overview.RecentTrend[5].WithdrawalAmount)
	require.Equal(t, now.AddDate(0, 0, -3).Format("2006-01-02"), overview.RecentTrend[3].Date)
	require.Equal(t, 22.0, overview.RecentTrend[3].RewardAmount)
	require.Equal(t, 6.0, overview.RecentTrend[3].WithdrawalAmount)
}

func TestReferralAdminService_GetOverview_DoesNotCreateMissingDefaultCodes(t *testing.T) {
	refRepo := newAdminReferralRepoStub()
	refRepo.listRelations = []AdminReferralRelation{
		{UserID: 7, UserEmail: "alpha@example.com", Username: "alpha", ReferrerUserID: int64ValuePtr(8), ReferrerEmail: stringValuePtr("bravo@example.com"), ReferrerUsername: stringValuePtr("bravo")},
	}
	commissionRepo := newAdminCommissionRepoStub()
	userRepo := &adminSearchUserRepoStub{
		users: []User{
			{ID: 7, Email: "alpha@example.com", Username: "alpha"},
			{ID: 8, Email: "bravo@example.com", Username: "bravo"},
		},
	}

	svc := newReferralAdminServiceForTest(refRepo, commissionRepo, userRepo)
	overview, err := svc.GetOverview(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, overview.TotalAccounts)
	require.Empty(t, refRepo.codesByUser)
	require.Empty(t, refRepo.codesByCode)
}

func TestReferralAdminService_GetRelationTree_BuildsTwoLevels(t *testing.T) {
	refRepo := newAdminReferralRepoStub()
	refRepo.inviteeCounts[7] = &ReferralInviteeCounts{DirectInvitees: 2, SecondLevelInvitees: 2}
	refRepo.inviteeCounts[10] = &ReferralInviteeCounts{DirectInvitees: 1, SecondLevelInvitees: 0}
	refRepo.inviteeCounts[11] = &ReferralInviteeCounts{DirectInvitees: 1, SecondLevelInvitees: 0}
	refRepo.inviteeCounts[20] = &ReferralInviteeCounts{}
	refRepo.inviteeCounts[21] = &ReferralInviteeCounts{}
	refRepo.inviteesByUser[7] = []ReferralInvitee{
		{UserID: 10, Email: "child-a@example.com", Username: "child-a"},
		{UserID: 11, Email: "child-b@example.com", Username: "child-b"},
	}
	refRepo.inviteesByUser[10] = []ReferralInvitee{
		{UserID: 20, Email: "grand-a@example.com", Username: "grand-a"},
	}
	refRepo.inviteesByUser[11] = []ReferralInvitee{
		{UserID: 21, Email: "grand-b@example.com", Username: "grand-b"},
	}

	commissionRepo := newAdminCommissionRepoStub()
	commissionRepo.ledgers = []CommissionLedger{
		{UserID: 7, Bucket: CommissionLedgerBucketAvailable, Amount: 20},
		{UserID: 10, Bucket: CommissionLedgerBucketAvailable, Amount: 5},
		{UserID: 11, Bucket: CommissionLedgerBucketPending, Amount: 6},
	}
	userRepo := &adminSearchUserRepoStub{
		users: []User{
			{ID: 7, Email: "root@example.com", Username: "root"},
			{ID: 10, Email: "child-a@example.com", Username: "child-a"},
			{ID: 11, Email: "child-b@example.com", Username: "child-b"},
			{ID: 20, Email: "grand-a@example.com", Username: "grand-a"},
			{ID: 21, Email: "grand-b@example.com", Username: "grand-b"},
		},
	}

	svc := newReferralAdminServiceForTest(refRepo, commissionRepo, userRepo)
	tree, err := svc.GetRelationTree(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, int64(7), tree.UserID)
	require.Len(t, tree.Children, 2)
	require.Equal(t, 1, tree.Children[0].Level)
	require.Len(t, tree.Children[0].Children, 1)
	require.Equal(t, 2, tree.Children[0].Children[0].Level)
}

func TestReferralAdminService_GetRelationTree_DoesNotCreateMissingDefaultCodes(t *testing.T) {
	refRepo := newAdminReferralRepoStub()
	userRepo := &adminSearchUserRepoStub{
		users: []User{
			{ID: 7, Email: "root@example.com", Username: "root"},
		},
	}
	svc := newReferralAdminServiceForTest(refRepo, newAdminCommissionRepoStub(), userRepo)

	tree, err := svc.GetRelationTree(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, int64(7), tree.UserID)
	require.Empty(t, tree.ReferralCode)
	require.Empty(t, refRepo.codesByUser)
	require.Empty(t, refRepo.codesByCode)
}
