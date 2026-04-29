//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type withdrawalCommissionRepoStub struct {
	rewards          []CommissionReward
	ledgers          []CommissionLedger
	withdrawals      []CommissionWithdrawal
	withdrawalItems  []CommissionWithdrawalItem
	payoutAccounts   []CommissionPayoutAccount
	nextWithdrawalID int64
	nextItemID       int64
	nextAccountID    int64

	afterUnlockedWithdrawalRead func(id int64)
	beforeLockedWithdrawalRead  func(id int64)
	countWithdrawalsResults     []int
	countWithdrawalsCalls       int
}

func newWithdrawalCommissionRepoStub() *withdrawalCommissionRepoStub {
	return &withdrawalCommissionRepoStub{
		nextWithdrawalID: 1,
		nextItemID:       1,
		nextAccountID:    1,
	}
}

func (s *withdrawalCommissionRepoStub) CreateReward(ctx context.Context, reward *CommissionReward) error {
	return nil
}

func (s *withdrawalCommissionRepoStub) GetRewardByID(ctx context.Context, rewardID int64) (*CommissionReward, error) {
	for i := range s.rewards {
		if s.rewards[i].ID == rewardID {
			cloned := s.rewards[i]
			return &cloned, nil
		}
	}
	return nil, ErrCommissionWithdrawalNotFound
}

func (s *withdrawalCommissionRepoStub) ListRewardsByRechargeOrder(ctx context.Context, rechargeOrderID int64) ([]CommissionReward, error) {
	return nil, nil
}

func (s *withdrawalCommissionRepoStub) ListPendingRewardsReady(ctx context.Context, readyAt time.Time) ([]CommissionReward, error) {
	var result []CommissionReward
	for _, reward := range s.rewards {
		if reward.Status == CommissionRewardStatusPending && reward.AvailableAt != nil && !reward.AvailableAt.After(readyAt) {
			result = append(result, reward)
		}
	}
	return result, nil
}

func (s *withdrawalCommissionRepoStub) ListRewardsByUser(ctx context.Context, userID int64, statuses []string) ([]CommissionReward, error) {
	allowed := make(map[string]struct{}, len(statuses))
	for _, status := range statuses {
		allowed[status] = struct{}{}
	}
	var result []CommissionReward
	for _, reward := range s.rewards {
		if reward.UserID != userID {
			continue
		}
		if len(allowed) > 0 {
			if _, ok := allowed[reward.Status]; !ok {
				continue
			}
		}
		result = append(result, reward)
	}
	return result, nil
}

func (s *withdrawalCommissionRepoStub) SumRewardBucketAmount(ctx context.Context, rewardID int64, bucket string) (float64, error) {
	total := 0.0
	for _, ledger := range s.ledgers {
		if ledger.RewardID != nil && *ledger.RewardID == rewardID && ledger.Bucket == bucket {
			total += ledger.Amount
		}
	}
	return total, nil
}

func (s *withdrawalCommissionRepoStub) SumRewardBucketAmountForUpdate(ctx context.Context, rewardID int64, bucket string, forUpdate bool) (float64, error) {
	return s.SumRewardBucketAmount(ctx, rewardID, bucket)
}

func (s *withdrawalCommissionRepoStub) CreateWithdrawal(ctx context.Context, withdrawal *CommissionWithdrawal) error {
	withdrawal.ID = s.nextWithdrawalID
	s.nextWithdrawalID++
	if withdrawal.CreatedAt.IsZero() {
		withdrawal.CreatedAt = time.Now()
	}
	withdrawal.UpdatedAt = withdrawal.CreatedAt
	s.withdrawals = append(s.withdrawals, *withdrawal)
	return nil
}

func (s *withdrawalCommissionRepoStub) GetWithdrawalByID(ctx context.Context, id int64) (*CommissionWithdrawal, error) {
	for _, withdrawal := range s.withdrawals {
		if withdrawal.ID == id {
			cloned := withdrawal
			if s.afterUnlockedWithdrawalRead != nil {
				s.afterUnlockedWithdrawalRead(id)
			}
			return &cloned, nil
		}
	}
	return nil, ErrCommissionWithdrawalNotFound
}

func (s *withdrawalCommissionRepoStub) GetWithdrawalByIDForUpdate(ctx context.Context, id int64) (*CommissionWithdrawal, error) {
	if s.beforeLockedWithdrawalRead != nil {
		s.beforeLockedWithdrawalRead(id)
	}
	for _, withdrawal := range s.withdrawals {
		if withdrawal.ID == id {
			cloned := withdrawal
			return &cloned, nil
		}
	}
	return nil, ErrCommissionWithdrawalNotFound
}

func (s *withdrawalCommissionRepoStub) UpdateWithdrawal(ctx context.Context, withdrawal *CommissionWithdrawal) error {
	for i := range s.withdrawals {
		if s.withdrawals[i].ID == withdrawal.ID {
			s.withdrawals[i] = *withdrawal
			return nil
		}
	}
	return ErrCommissionWithdrawalNotFound
}

func (s *withdrawalCommissionRepoStub) CreateWithdrawalItems(ctx context.Context, items []CommissionWithdrawalItem) error {
	for i := range items {
		items[i].ID = s.nextItemID
		s.nextItemID++
		if items[i].CreatedAt.IsZero() {
			items[i].CreatedAt = time.Now()
		}
		items[i].UpdatedAt = items[i].CreatedAt
		s.withdrawalItems = append(s.withdrawalItems, items[i])
	}
	return nil
}

func (s *withdrawalCommissionRepoStub) ListWithdrawalItemsByWithdrawal(ctx context.Context, withdrawalID int64) ([]CommissionWithdrawalItem, error) {
	var result []CommissionWithdrawalItem
	for _, item := range s.withdrawalItems {
		if item.WithdrawalID == withdrawalID {
			result = append(result, item)
		}
	}
	return result, nil
}

func (s *withdrawalCommissionRepoStub) UpdateWithdrawalItem(ctx context.Context, item *CommissionWithdrawalItem) error {
	for i := range s.withdrawalItems {
		if s.withdrawalItems[i].ID == item.ID {
			s.withdrawalItems[i] = *item
			return nil
		}
	}
	return nil
}

func (s *withdrawalCommissionRepoStub) CreateLedgerEntries(ctx context.Context, entries []CommissionLedger) error {
	for i := range entries {
		entries[i].ID = int64(len(s.ledgers) + 1)
		entries[i].CreatedAt = time.Now()
		s.ledgers = append(s.ledgers, entries[i])
	}
	return nil
}

func (s *withdrawalCommissionRepoStub) UpdateReward(ctx context.Context, reward *CommissionReward) error {
	for i := range s.rewards {
		if s.rewards[i].ID == reward.ID {
			s.rewards[i] = *reward
			return nil
		}
	}
	return nil
}

func (s *withdrawalCommissionRepoStub) ListPayoutAccountsByUser(ctx context.Context, userID int64) ([]CommissionPayoutAccount, error) {
	var result []CommissionPayoutAccount
	for _, account := range s.payoutAccounts {
		if account.UserID == userID {
			result = append(result, account)
		}
	}
	return result, nil
}

func (s *withdrawalCommissionRepoStub) CountWithdrawalsByUserSince(ctx context.Context, userID int64, since time.Time) (int, error) {
	if s.countWithdrawalsCalls < len(s.countWithdrawalsResults) {
		result := s.countWithdrawalsResults[s.countWithdrawalsCalls]
		s.countWithdrawalsCalls++
		return result, nil
	}
	s.countWithdrawalsCalls++
	count := 0
	for _, withdrawal := range s.withdrawals {
		if withdrawal.UserID == userID && !withdrawal.CreatedAt.Before(since) {
			count++
		}
	}
	return count, nil
}

func (s *withdrawalCommissionRepoStub) UpsertPayoutAccount(ctx context.Context, account *CommissionPayoutAccount) error {
	if account.ID == 0 {
		account.ID = s.nextAccountID
		s.nextAccountID++
		if account.CreatedAt.IsZero() {
			account.CreatedAt = time.Now()
		}
		account.UpdatedAt = account.CreatedAt
		s.payoutAccounts = append(s.payoutAccounts, *account)
		return nil
	}
	for i := range s.payoutAccounts {
		if s.payoutAccounts[i].ID == account.ID {
			account.CreatedAt = s.payoutAccounts[i].CreatedAt
			account.UpdatedAt = time.Now()
			s.payoutAccounts[i] = *account
			return nil
		}
	}
	return nil
}

type withdrawalUserRepoStub struct {
	user           *User
	balanceUpdates map[int64]float64
}

func (s *withdrawalUserRepoStub) Create(ctx context.Context, user *User) error {
	return nil
}

func (s *withdrawalUserRepoStub) GetByID(ctx context.Context, id int64) (*User, error) {
	if s.user != nil {
		return s.user, nil
	}
	return &User{ID: id, ReferralEnabled: true}, nil
}

func (s *withdrawalUserRepoStub) GetByEmail(ctx context.Context, email string) (*User, error) {
	return nil, ErrUserNotFound
}

func (s *withdrawalUserRepoStub) GetFirstAdmin(ctx context.Context) (*User, error) {
	return nil, ErrUserNotFound
}

func (s *withdrawalUserRepoStub) Update(ctx context.Context, user *User) error {
	return nil
}

func (s *withdrawalUserRepoStub) Delete(ctx context.Context, id int64) error {
	return nil
}

func (s *withdrawalUserRepoStub) GetUserAvatar(context.Context, int64) (*UserAvatar, error) {
	panic("unexpected")
}

func (s *withdrawalUserRepoStub) UpsertUserAvatar(context.Context, int64, UpsertUserAvatarInput) (*UserAvatar, error) {
	panic("unexpected")
}

func (s *withdrawalUserRepoStub) DeleteUserAvatar(context.Context, int64) error {
	panic("unexpected")
}

func (s *withdrawalUserRepoStub) List(ctx context.Context, params pagination.PaginationParams) ([]User, *pagination.PaginationResult, error) {
	return nil, &pagination.PaginationResult{}, nil
}

func (s *withdrawalUserRepoStub) ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters UserListFilters) ([]User, *pagination.PaginationResult, error) {
	return nil, &pagination.PaginationResult{}, nil
}

func (s *withdrawalUserRepoStub) UpdateBalance(ctx context.Context, id int64, amount float64) error {
	if s.balanceUpdates == nil {
		s.balanceUpdates = map[int64]float64{}
	}
	s.balanceUpdates[id] += amount
	return nil
}

func (s *withdrawalUserRepoStub) DeductBalance(ctx context.Context, id int64, amount float64) error {
	return nil
}

func (s *withdrawalUserRepoStub) UpdateConcurrency(ctx context.Context, id int64, concurrency int) error {
	return nil
}

func (s *withdrawalUserRepoStub) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	return false, nil
}

func (s *withdrawalUserRepoStub) RemoveGroupFromAllowedGroups(ctx context.Context, groupID int64) (int64, error) {
	return 0, nil
}

func (s *withdrawalUserRepoStub) AddGroupToAllowedGroups(ctx context.Context, userID int64, groupID int64) error {
	return nil
}

func (s *withdrawalUserRepoStub) RemoveGroupFromUserAllowedGroups(ctx context.Context, userID int64, groupID int64) error {
	return nil
}

func (s *withdrawalUserRepoStub) GetLatestUsedAtByUserIDs(context.Context, []int64) (map[int64]*time.Time, error) {
	return map[int64]*time.Time{}, nil
}

func (s *withdrawalUserRepoStub) GetLatestUsedAtByUserID(context.Context, int64) (*time.Time, error) {
	return nil, nil
}

func (s *withdrawalUserRepoStub) UpdateUserLastActiveAt(context.Context, int64, time.Time) error {
	return nil
}

func (s *withdrawalUserRepoStub) UpdateTotpSecret(ctx context.Context, userID int64, encryptedSecret *string) error {
	return nil
}

func (s *withdrawalUserRepoStub) EnableTotp(ctx context.Context, userID int64) error {
	return nil
}

func (s *withdrawalUserRepoStub) DisableTotp(ctx context.Context, userID int64) error {
	return nil
}

func (s *withdrawalUserRepoStub) UpdateDefaultChatAPIKeyID(ctx context.Context, userID int64, apiKeyID *int64) error {
	return nil
}

func (s *withdrawalUserRepoStub) ListUserAuthIdentities(context.Context, int64) ([]UserAuthIdentityRecord, error) {
	panic("unexpected")
}

func (s *withdrawalUserRepoStub) UnbindUserAuthProvider(context.Context, int64, string) error {
	panic("unexpected")
}

func newReferralWithdrawalServiceForTest(repo *withdrawalCommissionRepoStub, settings map[string]string, rechargeRepo RechargeOrderRepository) *ReferralWithdrawalService {
	cfg := &config.Config{}
	if rechargeRepo == nil {
		rechargeRepo = newRechargeOrderRepoStub()
	}
	settingService := NewSettingService(&settingRepoStub{values: settings}, cfg)
	settlementService := NewReferralSettlementService(repo, rechargeRepo, nil)
	return NewReferralWithdrawalService(
		repo,
		&withdrawalUserRepoStub{},
		nil,
		settingService,
		settlementService,
		nil,
	)
}

func TestReferralWithdrawalService_CreateWithdrawal_FreezesAvailableRewards(t *testing.T) {
	repo := newWithdrawalCommissionRepoStub()
	repo.rewards = []CommissionReward{
		{ID: 1, UserID: 200, RechargeOrderID: 11, RewardAmount: 15, Currency: ReferralSettlementCurrencyCNY, Status: CommissionRewardStatusAvailable},
		{ID: 2, UserID: 200, RechargeOrderID: 12, RewardAmount: 10, Currency: ReferralSettlementCurrencyCNY, Status: CommissionRewardStatusAvailable},
	}
	repo.ledgers = []CommissionLedger{
		{ID: 1, UserID: 200, RewardID: int64ValuePtr(1), RechargeOrderID: int64ValuePtr(11), Bucket: CommissionLedgerBucketAvailable, Amount: 15, Currency: ReferralSettlementCurrencyCNY},
		{ID: 2, UserID: 200, RewardID: int64ValuePtr(2), RechargeOrderID: int64ValuePtr(12), Bucket: CommissionLedgerBucketAvailable, Amount: 10, Currency: ReferralSettlementCurrencyCNY},
	}
	repo.payoutAccounts = []CommissionPayoutAccount{
		{ID: 1, UserID: 200, Method: CommissionPayoutMethodAlipay, AccountName: "Alice", AccountNoMasked: stringValuePtr("alipay@example.com"), AccountNoEncrypted: stringValuePtr("alipay@example.com"), IsDefault: true, Status: StatusActive},
	}

	svc := newReferralWithdrawalServiceForTest(repo, map[string]string{
		SettingKeyReferralEnabled:                      "true",
		SettingKeyReferralWithdrawEnabled:              "true",
		SettingKeyReferralWithdrawMinAmount:            "10",
		SettingKeyReferralWithdrawFeeRate:              "0.10",
		SettingKeyReferralWithdrawManualReviewRequired: "true",
		SettingKeyReferralWithdrawMethodsEnabled:       `["alipay","bank"]`,
	}, nil)

	result, err := svc.CreateWithdrawal(context.Background(), &CreateReferralWithdrawalInput{
		UserID:          200,
		Amount:          20,
		PayoutMethod:    CommissionPayoutMethodAlipay,
		PayoutAccountID: 1,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, CommissionWithdrawalStatusPendingReview, result.Withdrawal.Status)
	require.Equal(t, 20.0, result.Withdrawal.Amount)
	require.Equal(t, 2.0, result.Withdrawal.FeeAmount)
	require.Equal(t, 18.0, result.Withdrawal.NetAmount)
	require.Len(t, result.Items, 2)
	require.Equal(t, 15.0, result.Items[0].AllocatedAmount)
	require.Equal(t, 5.0, result.Items[1].AllocatedAmount)
	require.Len(t, repo.ledgers, 6)
	require.NotNil(t, result.Items[0].FreezeLedgerID)
	require.NotNil(t, result.Items[1].FreezeLedgerID)
	require.NotNil(t, result.Withdrawal.PayoutAccountSnapshotJSON)
	require.Contains(t, *result.Withdrawal.PayoutAccountSnapshotJSON, `"account_no_encrypted":"alipay@example.com"`)
	require.Equal(t, CommissionRewardStatusFrozen, repo.rewards[0].Status)
	require.Equal(t, CommissionRewardStatusPartiallyFrozen, repo.rewards[1].Status)
}

func TestReferralWithdrawalService_RejectWithdrawal_ReturnsFrozenAmount(t *testing.T) {
	repo := newWithdrawalCommissionRepoStub()
	repo.rewards = []CommissionReward{
		{ID: 1, UserID: 200, RechargeOrderID: 11, RewardAmount: 15, Currency: ReferralSettlementCurrencyCNY, Status: CommissionRewardStatusAvailable},
		{ID: 2, UserID: 200, RechargeOrderID: 12, RewardAmount: 10, Currency: ReferralSettlementCurrencyCNY, Status: CommissionRewardStatusAvailable},
	}
	repo.ledgers = []CommissionLedger{
		{ID: 1, UserID: 200, RewardID: int64ValuePtr(1), RechargeOrderID: int64ValuePtr(11), Bucket: CommissionLedgerBucketAvailable, Amount: 15, Currency: ReferralSettlementCurrencyCNY},
		{ID: 2, UserID: 200, RewardID: int64ValuePtr(2), RechargeOrderID: int64ValuePtr(12), Bucket: CommissionLedgerBucketAvailable, Amount: 10, Currency: ReferralSettlementCurrencyCNY},
	}
	repo.payoutAccounts = []CommissionPayoutAccount{
		{ID: 1, UserID: 200, Method: CommissionPayoutMethodAlipay, AccountName: "Alice", AccountNoMasked: stringValuePtr("alipay@example.com"), IsDefault: true, Status: StatusActive},
	}

	svc := newReferralWithdrawalServiceForTest(repo, map[string]string{
		SettingKeyReferralEnabled:                      "true",
		SettingKeyReferralWithdrawEnabled:              "true",
		SettingKeyReferralWithdrawMinAmount:            "10",
		SettingKeyReferralWithdrawManualReviewRequired: "true",
		SettingKeyReferralWithdrawMethodsEnabled:       `["alipay"]`,
	}, nil)

	created, err := svc.CreateWithdrawal(context.Background(), &CreateReferralWithdrawalInput{
		UserID:          200,
		Amount:          20,
		PayoutMethod:    CommissionPayoutMethodAlipay,
		PayoutAccountID: 1,
	})
	require.NoError(t, err)

	rejected, err := svc.RejectWithdrawal(context.Background(), &ReviewReferralWithdrawalInput{
		WithdrawalID: created.Withdrawal.ID,
		ReviewerID:   9,
		Reason:       "risk review failed",
	})
	require.NoError(t, err)
	require.Equal(t, CommissionWithdrawalStatusRejected, rejected.Withdrawal.Status)
	require.Len(t, rejected.Items, 2)
	require.Equal(t, CommissionWithdrawalItemStatusReturned, rejected.Items[0].Status)
	require.Equal(t, CommissionWithdrawalItemStatusReturned, rejected.Items[1].Status)
	require.NotNil(t, rejected.Items[0].ReturnLedgerID)
	require.NotNil(t, rejected.Items[1].ReturnLedgerID)
	require.Len(t, repo.ledgers, 10)
	require.Equal(t, CommissionRewardStatusAvailable, repo.rewards[0].Status)
	require.Equal(t, CommissionRewardStatusAvailable, repo.rewards[1].Status)
}

func TestReferralWithdrawalService_MarkWithdrawalPaid_RequiresApproval(t *testing.T) {
	repo := newWithdrawalCommissionRepoStub()
	repo.rewards = []CommissionReward{
		{ID: 1, UserID: 200, RechargeOrderID: 11, RewardAmount: 20, Currency: ReferralSettlementCurrencyCNY, Status: CommissionRewardStatusAvailable},
	}
	repo.ledgers = []CommissionLedger{
		{ID: 1, UserID: 200, RewardID: int64ValuePtr(1), RechargeOrderID: int64ValuePtr(11), Bucket: CommissionLedgerBucketAvailable, Amount: 20, Currency: ReferralSettlementCurrencyCNY},
	}
	repo.payoutAccounts = []CommissionPayoutAccount{
		{ID: 1, UserID: 200, Method: CommissionPayoutMethodAlipay, AccountName: "Alice", AccountNoMasked: stringValuePtr("alipay@example.com"), IsDefault: true, Status: StatusActive},
	}

	svc := newReferralWithdrawalServiceForTest(repo, map[string]string{
		SettingKeyReferralEnabled:                      "true",
		SettingKeyReferralWithdrawEnabled:              "true",
		SettingKeyReferralWithdrawMinAmount:            "10",
		SettingKeyReferralWithdrawManualReviewRequired: "true",
		SettingKeyReferralWithdrawMethodsEnabled:       `["alipay"]`,
	}, nil)

	created, err := svc.CreateWithdrawal(context.Background(), &CreateReferralWithdrawalInput{
		UserID:          200,
		Amount:          10,
		PayoutMethod:    CommissionPayoutMethodAlipay,
		PayoutAccountID: 1,
	})
	require.NoError(t, err)

	_, err = svc.MarkWithdrawalPaid(context.Background(), &MarkReferralWithdrawalPaidInput{
		WithdrawalID: created.Withdrawal.ID,
		PaidBy:       9,
	})
	require.ErrorIs(t, err, ErrCommissionWithdrawalConflict)
}

func TestReferralWithdrawalService_ApproveAndMarkPaid_SettlesFrozenAmount(t *testing.T) {
	repo := newWithdrawalCommissionRepoStub()
	repo.rewards = []CommissionReward{
		{ID: 1, UserID: 200, RechargeOrderID: 11, RewardAmount: 15, Currency: ReferralSettlementCurrencyCNY, Status: CommissionRewardStatusAvailable},
		{ID: 2, UserID: 200, RechargeOrderID: 12, RewardAmount: 10, Currency: ReferralSettlementCurrencyCNY, Status: CommissionRewardStatusAvailable},
	}
	repo.ledgers = []CommissionLedger{
		{ID: 1, UserID: 200, RewardID: int64ValuePtr(1), RechargeOrderID: int64ValuePtr(11), Bucket: CommissionLedgerBucketAvailable, Amount: 15, Currency: ReferralSettlementCurrencyCNY},
		{ID: 2, UserID: 200, RewardID: int64ValuePtr(2), RechargeOrderID: int64ValuePtr(12), Bucket: CommissionLedgerBucketAvailable, Amount: 10, Currency: ReferralSettlementCurrencyCNY},
	}
	repo.payoutAccounts = []CommissionPayoutAccount{
		{ID: 1, UserID: 200, Method: CommissionPayoutMethodAlipay, AccountName: "Alice", AccountNoMasked: stringValuePtr("alipay@example.com"), IsDefault: true, Status: StatusActive},
	}

	svc := newReferralWithdrawalServiceForTest(repo, map[string]string{
		SettingKeyReferralEnabled:                      "true",
		SettingKeyReferralWithdrawEnabled:              "true",
		SettingKeyReferralWithdrawMinAmount:            "10",
		SettingKeyReferralWithdrawManualReviewRequired: "true",
		SettingKeyReferralWithdrawMethodsEnabled:       `["alipay"]`,
	}, nil)

	created, err := svc.CreateWithdrawal(context.Background(), &CreateReferralWithdrawalInput{
		UserID:          200,
		Amount:          20,
		PayoutMethod:    CommissionPayoutMethodAlipay,
		PayoutAccountID: 1,
	})
	require.NoError(t, err)

	approved, err := svc.ApproveWithdrawal(context.Background(), created.Withdrawal.ID, 9, "looks good")
	require.NoError(t, err)
	require.Equal(t, CommissionWithdrawalStatusApproved, approved.Status)

	paid, err := svc.MarkWithdrawalPaid(context.Background(), &MarkReferralWithdrawalPaidInput{
		WithdrawalID: created.Withdrawal.ID,
		PaidBy:       9,
		Remark:       "paid via manual transfer",
	})
	require.NoError(t, err)
	require.Equal(t, CommissionWithdrawalStatusPaid, paid.Withdrawal.Status)
	require.Len(t, paid.Items, 2)
	require.Equal(t, CommissionWithdrawalItemStatusPaid, paid.Items[0].Status)
	require.Equal(t, CommissionWithdrawalItemStatusPaid, paid.Items[1].Status)
	require.NotNil(t, paid.Items[0].PaidLedgerID)
	require.NotNil(t, paid.Items[1].PaidLedgerID)
	require.Len(t, repo.ledgers, 10)
	require.Equal(t, CommissionRewardStatusPaid, repo.rewards[0].Status)
	require.Equal(t, CommissionRewardStatusPartiallyPaid, repo.rewards[1].Status)
}

func TestReferralWithdrawalService_ApproveWithdrawal_DetectsStatusChangeBeforeCommit(t *testing.T) {
	repo := newWithdrawalCommissionRepoStub()
	repo.withdrawals = []CommissionWithdrawal{
		{ID: 1, UserID: 200, WithdrawalNo: "WD-APPROVE", Amount: 10, Currency: ReferralSettlementCurrencyCNY, Status: CommissionWithdrawalStatusPendingReview},
	}
	repo.afterUnlockedWithdrawalRead = func(id int64) {
		for i := range repo.withdrawals {
			if repo.withdrawals[i].ID == id {
				repo.withdrawals[i].Status = CommissionWithdrawalStatusRejected
			}
		}
	}
	repo.beforeLockedWithdrawalRead = func(id int64) {
		for i := range repo.withdrawals {
			if repo.withdrawals[i].ID == id {
				repo.withdrawals[i].Status = CommissionWithdrawalStatusRejected
			}
		}
	}

	svc := newReferralWithdrawalServiceForTest(repo, map[string]string{
		SettingKeyReferralEnabled: "true",
	}, nil)

	_, err := svc.ApproveWithdrawal(context.Background(), 1, 9, "looks good")
	require.ErrorIs(t, err, ErrCommissionWithdrawalConflict)
	require.Equal(t, CommissionWithdrawalStatusRejected, repo.withdrawals[0].Status)
}

func TestReferralWithdrawalService_RejectWithdrawal_DetectsStatusChangeBeforeMutation(t *testing.T) {
	repo := newWithdrawalCommissionRepoStub()
	repo.withdrawals = []CommissionWithdrawal{
		{ID: 1, UserID: 200, WithdrawalNo: "WD-REJECT", Amount: 10, Currency: ReferralSettlementCurrencyCNY, Status: CommissionWithdrawalStatusPendingReview},
	}
	repo.withdrawalItems = []CommissionWithdrawalItem{
		{ID: 1, WithdrawalID: 1, UserID: 200, RewardID: 11, RechargeOrderID: 21, AllocatedAmount: 10, NetAllocatedAmount: 10, Currency: ReferralSettlementCurrencyCNY, Status: CommissionWithdrawalItemStatusFrozen},
	}
	repo.rewards = []CommissionReward{
		{ID: 11, UserID: 200, RechargeOrderID: 21, RewardAmount: 10, Currency: ReferralSettlementCurrencyCNY, Status: CommissionRewardStatusFrozen},
	}
	repo.ledgers = []CommissionLedger{
		{ID: 1, UserID: 200, RewardID: int64ValuePtr(11), RechargeOrderID: int64ValuePtr(21), Bucket: CommissionLedgerBucketFrozen, Amount: 10, Currency: ReferralSettlementCurrencyCNY},
	}
	repo.afterUnlockedWithdrawalRead = func(id int64) {
		for i := range repo.withdrawals {
			if repo.withdrawals[i].ID == id {
				repo.withdrawals[i].Status = CommissionWithdrawalStatusPaid
			}
		}
	}
	repo.beforeLockedWithdrawalRead = func(id int64) {
		for i := range repo.withdrawals {
			if repo.withdrawals[i].ID == id {
				repo.withdrawals[i].Status = CommissionWithdrawalStatusPaid
			}
		}
	}

	svc := newReferralWithdrawalServiceForTest(repo, map[string]string{
		SettingKeyReferralEnabled: "true",
	}, nil)

	_, err := svc.RejectWithdrawal(context.Background(), &ReviewReferralWithdrawalInput{
		WithdrawalID: 1,
		ReviewerID:   9,
		Reason:       "risk review failed",
	})
	require.ErrorIs(t, err, ErrCommissionWithdrawalConflict)
	require.Equal(t, CommissionWithdrawalStatusPaid, repo.withdrawals[0].Status)
	require.Len(t, repo.ledgers, 1)
	require.Equal(t, CommissionWithdrawalItemStatusFrozen, repo.withdrawalItems[0].Status)
}

func TestReferralWithdrawalService_MarkWithdrawalPaid_DetectsStatusChangeBeforeMutation(t *testing.T) {
	repo := newWithdrawalCommissionRepoStub()
	repo.withdrawals = []CommissionWithdrawal{
		{ID: 1, UserID: 200, WithdrawalNo: "WD-PAID", Amount: 10, Currency: ReferralSettlementCurrencyCNY, Status: CommissionWithdrawalStatusApproved},
	}
	repo.withdrawalItems = []CommissionWithdrawalItem{
		{ID: 1, WithdrawalID: 1, UserID: 200, RewardID: 11, RechargeOrderID: 21, AllocatedAmount: 10, NetAllocatedAmount: 10, Currency: ReferralSettlementCurrencyCNY, Status: CommissionWithdrawalItemStatusFrozen},
	}
	repo.rewards = []CommissionReward{
		{ID: 11, UserID: 200, RechargeOrderID: 21, RewardAmount: 10, Currency: ReferralSettlementCurrencyCNY, Status: CommissionRewardStatusFrozen},
	}
	repo.ledgers = []CommissionLedger{
		{ID: 1, UserID: 200, RewardID: int64ValuePtr(11), RechargeOrderID: int64ValuePtr(21), Bucket: CommissionLedgerBucketFrozen, Amount: 10, Currency: ReferralSettlementCurrencyCNY},
	}
	repo.afterUnlockedWithdrawalRead = func(id int64) {
		for i := range repo.withdrawals {
			if repo.withdrawals[i].ID == id {
				repo.withdrawals[i].Status = CommissionWithdrawalStatusRejected
			}
		}
	}
	repo.beforeLockedWithdrawalRead = func(id int64) {
		for i := range repo.withdrawals {
			if repo.withdrawals[i].ID == id {
				repo.withdrawals[i].Status = CommissionWithdrawalStatusRejected
			}
		}
	}

	svc := newReferralWithdrawalServiceForTest(repo, map[string]string{
		SettingKeyReferralEnabled: "true",
	}, nil)

	_, err := svc.MarkWithdrawalPaid(context.Background(), &MarkReferralWithdrawalPaidInput{
		WithdrawalID: 1,
		PaidBy:       9,
		Remark:       "paid via manual transfer",
	})
	require.ErrorIs(t, err, ErrCommissionWithdrawalConflict)
	require.Equal(t, CommissionWithdrawalStatusRejected, repo.withdrawals[0].Status)
	require.Len(t, repo.ledgers, 1)
	require.Equal(t, CommissionWithdrawalItemStatusFrozen, repo.withdrawalItems[0].Status)
}

func TestReferralWithdrawalService_CreateWithdrawal_EnforcesDailyLimit(t *testing.T) {
	repo := newWithdrawalCommissionRepoStub()
	repo.rewards = []CommissionReward{
		{ID: 1, UserID: 200, RechargeOrderID: 11, RewardAmount: 20, Currency: ReferralSettlementCurrencyCNY, Status: CommissionRewardStatusAvailable},
	}
	repo.ledgers = []CommissionLedger{
		{ID: 1, UserID: 200, RewardID: int64ValuePtr(1), RechargeOrderID: int64ValuePtr(11), Bucket: CommissionLedgerBucketAvailable, Amount: 20, Currency: ReferralSettlementCurrencyCNY},
	}
	repo.withdrawals = []CommissionWithdrawal{
		{ID: 1, UserID: 200, WithdrawalNo: "WD-OLD", Amount: 10, Currency: ReferralSettlementCurrencyCNY, Status: CommissionWithdrawalStatusPendingReview, CreatedAt: time.Now()},
	}
	repo.payoutAccounts = []CommissionPayoutAccount{
		{ID: 1, UserID: 200, Method: CommissionPayoutMethodAlipay, AccountName: "Alice", AccountNoMasked: stringValuePtr("alipay@example.com"), IsDefault: true, Status: StatusActive},
	}

	svc := newReferralWithdrawalServiceForTest(repo, map[string]string{
		SettingKeyReferralEnabled:                      "true",
		SettingKeyReferralWithdrawEnabled:              "true",
		SettingKeyReferralWithdrawMinAmount:            "10",
		SettingKeyReferralWithdrawDailyLimit:           "1",
		SettingKeyReferralWithdrawManualReviewRequired: "true",
		SettingKeyReferralWithdrawMethodsEnabled:       `["alipay"]`,
	}, nil)

	_, err := svc.CreateWithdrawal(context.Background(), &CreateReferralWithdrawalInput{
		UserID:          200,
		Amount:          10,
		PayoutMethod:    CommissionPayoutMethodAlipay,
		PayoutAccountID: 1,
	})
	require.ErrorIs(t, err, ErrCommissionWithdrawDailyLimitExceeded)
}

func TestReferralWithdrawalService_CreateWithdrawal_RechecksDailyLimitInsideTransaction(t *testing.T) {
	repo := newWithdrawalCommissionRepoStub()
	repo.rewards = []CommissionReward{
		{ID: 1, UserID: 200, RechargeOrderID: 11, RewardAmount: 20, Currency: ReferralSettlementCurrencyCNY, Status: CommissionRewardStatusAvailable},
	}
	repo.ledgers = []CommissionLedger{
		{ID: 1, UserID: 200, RewardID: int64ValuePtr(1), RechargeOrderID: int64ValuePtr(11), Bucket: CommissionLedgerBucketAvailable, Amount: 20, Currency: ReferralSettlementCurrencyCNY},
	}
	repo.payoutAccounts = []CommissionPayoutAccount{
		{ID: 1, UserID: 200, Method: CommissionPayoutMethodAlipay, AccountName: "Alice", AccountNoMasked: stringValuePtr("alipay@example.com"), IsDefault: true, Status: StatusActive},
	}
	repo.countWithdrawalsResults = []int{0, 1}

	svc := newReferralWithdrawalServiceForTest(repo, map[string]string{
		SettingKeyReferralEnabled:                      "true",
		SettingKeyReferralWithdrawEnabled:              "true",
		SettingKeyReferralWithdrawMinAmount:            "10",
		SettingKeyReferralWithdrawDailyLimit:           "1",
		SettingKeyReferralWithdrawManualReviewRequired: "true",
		SettingKeyReferralWithdrawMethodsEnabled:       `["alipay"]`,
	}, nil)

	_, err := svc.CreateWithdrawal(context.Background(), &CreateReferralWithdrawalInput{
		UserID:          200,
		Amount:          10,
		PayoutMethod:    CommissionPayoutMethodAlipay,
		PayoutAccountID: 1,
	})
	require.ErrorIs(t, err, ErrCommissionWithdrawDailyLimitExceeded)
	require.Len(t, repo.withdrawals, 0)
	require.Equal(t, 2, repo.countWithdrawalsCalls)
}

func TestReferralWithdrawalService_UpsertPayoutAccount_RejectsDisabledMethod(t *testing.T) {
	repo := newWithdrawalCommissionRepoStub()
	svc := newReferralWithdrawalServiceForTest(repo, map[string]string{
		SettingKeyReferralEnabled:                "true",
		SettingKeyReferralWithdrawMethodsEnabled: `["alipay"]`,
	}, nil)

	_, err := svc.UpsertPayoutAccount(context.Background(), 200, 0, &UpsertReferralPayoutAccountInput{
		Method:      CommissionPayoutMethodBank,
		AccountName: "Alice",
		AccountNo:   "6222021234567890",
		IsDefault:   true,
	})
	require.ErrorIs(t, err, ErrCommissionWithdrawMethodInvalid)
}

func TestReferralWithdrawalService_UpsertPayoutAccount_RejectsWhenReferralDisabledForUser(t *testing.T) {
	repo := newWithdrawalCommissionRepoStub()
	svc := newReferralWithdrawalServiceForTest(repo, map[string]string{
		SettingKeyReferralEnabled:                "false",
		SettingKeyReferralWithdrawMethodsEnabled: `["alipay"]`,
	}, nil)
	svc.userRepo = &withdrawalUserRepoStub{user: &User{ID: 200, ReferralEnabled: false}}

	_, err := svc.UpsertPayoutAccount(context.Background(), 200, 0, &UpsertReferralPayoutAccountInput{
		Method:      CommissionPayoutMethodAlipay,
		AccountName: "Alice",
		AccountNo:   "alice@example.com",
		IsDefault:   true,
	})
	require.ErrorIs(t, err, ErrReferralDisabled)
}

func TestReferralWithdrawalService_UpsertPayoutAccount_RejectsWeeklyModification(t *testing.T) {
	repo := newWithdrawalCommissionRepoStub()
	now := time.Now()
	repo.payoutAccounts = []CommissionPayoutAccount{
		{
			ID:              1,
			UserID:          200,
			Method:          CommissionPayoutMethodAlipay,
			AccountName:     "Alice",
			AccountNoMasked: stringValuePtr("alice@example.com"),
			IsDefault:       true,
			Status:          StatusActive,
			CreatedAt:       now.Add(-24 * time.Hour),
			UpdatedAt:       now.Add(-24 * time.Hour),
		},
	}
	svc := newReferralWithdrawalServiceForTest(repo, map[string]string{
		SettingKeyReferralEnabled:                "true",
		SettingKeyReferralWithdrawMethodsEnabled: `["alipay"]`,
	}, nil)

	_, err := svc.UpsertPayoutAccount(context.Background(), 200, 1, &UpsertReferralPayoutAccountInput{
		Method:      CommissionPayoutMethodAlipay,
		AccountName: "Alice Updated",
		AccountNo:   "alice-new@example.com",
		IsDefault:   true,
	})
	require.ErrorIs(t, err, ErrCommissionPayoutAccountUpdateTooFrequent)
}

func TestReferralWithdrawalService_UpsertPayoutAccount_PreservesAccountWhenNoNewNumberProvided(t *testing.T) {
	repo := newWithdrawalCommissionRepoStub()
	repo.payoutAccounts = []CommissionPayoutAccount{
		{
			ID:                 1,
			UserID:             200,
			Method:             CommissionPayoutMethodAlipay,
			AccountName:        "Alice",
			AccountNoMasked:    stringValuePtr("alice@example.com"),
			AccountNoEncrypted: stringValuePtr("alice@example.com"),
			IsDefault:          true,
			Status:             StatusActive,
			CreatedAt:          time.Now().Add(-10 * 24 * time.Hour),
			UpdatedAt:          time.Now().Add(-8 * 24 * time.Hour),
		},
	}
	svc := newReferralWithdrawalServiceForTest(repo, map[string]string{
		SettingKeyReferralEnabled:                "true",
		SettingKeyReferralWithdrawMethodsEnabled: `["alipay"]`,
	}, nil)

	account, err := svc.UpsertPayoutAccount(context.Background(), 200, 1, &UpsertReferralPayoutAccountInput{
		Method:      CommissionPayoutMethodAlipay,
		AccountName: "Alice Updated",
		AccountNo:   "",
		IsDefault:   true,
	})
	require.NoError(t, err)
	require.Equal(t, "Alice Updated", account.AccountName)
	require.NotNil(t, account.AccountNoMasked)
	require.Equal(t, "alice@example.com", *account.AccountNoMasked)
	require.NotNil(t, repo.payoutAccounts[0].AccountNoEncrypted)
	require.Equal(t, "alice@example.com", *repo.payoutAccounts[0].AccountNoEncrypted)
}

func TestReferralWithdrawalService_UpsertPayoutAccount_StoresPlaintextAccountNumber(t *testing.T) {
	repo := newWithdrawalCommissionRepoStub()
	svc := newReferralWithdrawalServiceForTest(repo, map[string]string{
		SettingKeyReferralEnabled:                "true",
		SettingKeyReferralWithdrawMethodsEnabled: `["bank"]`,
	}, nil)

	account, err := svc.UpsertPayoutAccount(context.Background(), 200, 0, &UpsertReferralPayoutAccountInput{
		Method:      CommissionPayoutMethodBank,
		AccountName: "Alice",
		AccountNo:   "6222021234567890",
		BankName:    "ICBC",
		IsDefault:   true,
	})
	require.NoError(t, err)
	require.NotNil(t, account)
	require.NotNil(t, repo.payoutAccounts[0].AccountNoEncrypted)
	require.Equal(t, "6222021234567890", *repo.payoutAccounts[0].AccountNoEncrypted)
}

func TestReferralWithdrawalService_CreateWithdrawal_SettlesDuePendingRewardsBeforeAllocation(t *testing.T) {
	repo := newWithdrawalCommissionRepoStub()
	readyAt := time.Now().Add(-time.Hour)
	repo.rewards = []CommissionReward{
		{
			ID:              1,
			UserID:          200,
			RechargeOrderID: 11,
			RewardAmount:    20,
			Currency:        ReferralSettlementCurrencyCNY,
			Status:          CommissionRewardStatusPending,
			AvailableAt:     &readyAt,
		},
	}
	repo.ledgers = []CommissionLedger{
		{ID: 1, UserID: 200, RewardID: int64ValuePtr(1), RechargeOrderID: int64ValuePtr(11), Bucket: CommissionLedgerBucketPending, Amount: 20, Currency: ReferralSettlementCurrencyCNY},
	}
	repo.payoutAccounts = []CommissionPayoutAccount{
		{ID: 1, UserID: 200, Method: CommissionPayoutMethodAlipay, AccountName: "Alice", AccountNoMasked: stringValuePtr("alipay@example.com"), IsDefault: true, Status: StatusActive},
	}
	rechargeRepo := newRechargeOrderRepoStub()
	rechargeRepo.orders["provider::order-11"] = &RechargeOrder{
		ID:              11,
		UserID:          200,
		Provider:        "provider",
		ExternalOrderID: "order-11",
		PaidAmount:      20,
		Currency:        ReferralSettlementCurrencyCNY,
		Status:          RechargeOrderStatusCredited,
	}

	svc := newReferralWithdrawalServiceForTest(repo, map[string]string{
		SettingKeyReferralEnabled:                      "true",
		SettingKeyReferralWithdrawEnabled:              "true",
		SettingKeyReferralWithdrawMinAmount:            "10",
		SettingKeyReferralWithdrawManualReviewRequired: "true",
		SettingKeyReferralWithdrawMethodsEnabled:       `["alipay"]`,
	}, rechargeRepo)

	result, err := svc.CreateWithdrawal(context.Background(), &CreateReferralWithdrawalInput{
		UserID:          200,
		Amount:          10,
		PayoutMethod:    CommissionPayoutMethodAlipay,
		PayoutAccountID: 1,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Items, 1)
	require.Equal(t, CommissionRewardStatusPartiallyFrozen, repo.rewards[0].Status)
	require.Len(t, repo.ledgers, 5)
	require.Equal(t, CommissionLedgerEntryRewardPendingToAvailable, repo.ledgers[1].EntryType)
	require.Equal(t, CommissionLedgerBucketAvailable, repo.ledgers[2].Bucket)
}

func TestReferralWithdrawalService_ConvertCommissionToCredit_DoesNotRequireWithdrawEnabled(t *testing.T) {
	repo := newWithdrawalCommissionRepoStub()
	repo.rewards = []CommissionReward{
		{ID: 1, UserID: 200, RechargeOrderID: 11, RewardAmount: 20, Currency: ReferralSettlementCurrencyCNY, Status: CommissionRewardStatusAvailable},
	}
	repo.ledgers = []CommissionLedger{
		{ID: 1, UserID: 200, RewardID: int64ValuePtr(1), RechargeOrderID: int64ValuePtr(11), Bucket: CommissionLedgerBucketAvailable, Amount: 20, Currency: ReferralSettlementCurrencyCNY},
	}

	svc := newReferralWithdrawalServiceForTest(repo, map[string]string{
		SettingKeyReferralEnabled:                 "true",
		SettingKeyReferralWithdrawEnabled:         "false",
		SettingKeyReferralCreditConversionEnabled: "true",
		SettingKeyReferralWithdrawMinAmount:       "10",
	}, nil)
	userRepo := svc.userRepo.(*withdrawalUserRepoStub)

	err := svc.ConvertCommissionToCredit(context.Background(), 200, 10)
	require.NoError(t, err)
	require.Len(t, repo.withdrawals, 1)
	require.Equal(t, "credit_conversion", repo.withdrawals[0].PayoutMethod)
	require.Equal(t, 10.0, userRepo.balanceUpdates[200])
}

func TestReferralWithdrawalService_ConvertCommissionToCredit_AppliesConversionRate(t *testing.T) {
	repo := newWithdrawalCommissionRepoStub()
	repo.rewards = []CommissionReward{
		{ID: 1, UserID: 200, RechargeOrderID: 11, RewardAmount: 20, Currency: ReferralSettlementCurrencyCNY, Status: CommissionRewardStatusAvailable},
	}
	repo.ledgers = []CommissionLedger{
		{ID: 1, UserID: 200, RewardID: int64ValuePtr(1), RechargeOrderID: int64ValuePtr(11), Bucket: CommissionLedgerBucketAvailable, Amount: 20, Currency: ReferralSettlementCurrencyCNY},
	}

	svc := newReferralWithdrawalServiceForTest(repo, map[string]string{
		SettingKeyReferralEnabled:                 "true",
		SettingKeyReferralCreditConversionEnabled: "true",
		SettingKeyReferralCreditConversionRate:    "0.8",
		SettingKeyReferralWithdrawMinAmount:       "10",
	}, nil)
	userRepo := svc.userRepo.(*withdrawalUserRepoStub)

	err := svc.ConvertCommissionToCredit(context.Background(), 200, 10)
	require.NoError(t, err)
	require.Len(t, repo.withdrawals, 1)
	require.Equal(t, 10.0, repo.withdrawals[0].Amount)
	require.Equal(t, 8.0, repo.withdrawals[0].NetAmount)
	require.Equal(t, 8.0, userRepo.balanceUpdates[200])
}

func TestReferralWithdrawalService_ConvertCommissionToCredit_AllowsConversionMultiplierAboveOne(t *testing.T) {
	repo := newWithdrawalCommissionRepoStub()
	repo.rewards = []CommissionReward{
		{ID: 1, UserID: 200, RechargeOrderID: 11, RewardAmount: 20, Currency: ReferralSettlementCurrencyCNY, Status: CommissionRewardStatusAvailable},
	}
	repo.ledgers = []CommissionLedger{
		{ID: 1, UserID: 200, RewardID: int64ValuePtr(1), RechargeOrderID: int64ValuePtr(11), Bucket: CommissionLedgerBucketAvailable, Amount: 20, Currency: ReferralSettlementCurrencyCNY},
	}

	svc := newReferralWithdrawalServiceForTest(repo, map[string]string{
		SettingKeyReferralEnabled:                 "true",
		SettingKeyReferralCreditConversionEnabled: "true",
		SettingKeyReferralCreditConversionRate:    "10",
		SettingKeyReferralWithdrawMinAmount:       "10",
	}, nil)
	userRepo := svc.userRepo.(*withdrawalUserRepoStub)

	err := svc.ConvertCommissionToCredit(context.Background(), 200, 10)
	require.NoError(t, err)
	require.Equal(t, 100.0, repo.withdrawals[0].NetAmount)
	require.Equal(t, 100.0, userRepo.balanceUpdates[200])
}

func TestReferralWithdrawalService_ConvertCommissionToCredit_DistributesMultiplierAcrossRewards(t *testing.T) {
	repo := newWithdrawalCommissionRepoStub()
	repo.rewards = []CommissionReward{
		{ID: 1, UserID: 200, RechargeOrderID: 11, RewardAmount: 3.33, Currency: ReferralSettlementCurrencyCNY, Status: CommissionRewardStatusAvailable},
		{ID: 2, UserID: 200, RechargeOrderID: 12, RewardAmount: 3.33, Currency: ReferralSettlementCurrencyCNY, Status: CommissionRewardStatusAvailable},
		{ID: 3, UserID: 200, RechargeOrderID: 13, RewardAmount: 3.34, Currency: ReferralSettlementCurrencyCNY, Status: CommissionRewardStatusAvailable},
	}
	repo.ledgers = []CommissionLedger{
		{ID: 1, UserID: 200, RewardID: int64ValuePtr(1), RechargeOrderID: int64ValuePtr(11), Bucket: CommissionLedgerBucketAvailable, Amount: 3.33, Currency: ReferralSettlementCurrencyCNY},
		{ID: 2, UserID: 200, RewardID: int64ValuePtr(2), RechargeOrderID: int64ValuePtr(12), Bucket: CommissionLedgerBucketAvailable, Amount: 3.33, Currency: ReferralSettlementCurrencyCNY},
		{ID: 3, UserID: 200, RewardID: int64ValuePtr(3), RechargeOrderID: int64ValuePtr(13), Bucket: CommissionLedgerBucketAvailable, Amount: 3.34, Currency: ReferralSettlementCurrencyCNY},
	}

	svc := newReferralWithdrawalServiceForTest(repo, map[string]string{
		SettingKeyReferralEnabled:                 "true",
		SettingKeyReferralCreditConversionEnabled: "true",
		SettingKeyReferralCreditConversionRate:    "0.123456789",
		SettingKeyReferralWithdrawMinAmount:       "10",
	}, nil)
	userRepo := svc.userRepo.(*withdrawalUserRepoStub)

	err := svc.ConvertCommissionToCredit(context.Background(), 200, 10)
	require.NoError(t, err)
	require.Len(t, repo.withdrawals, 1)
	require.Len(t, repo.withdrawalItems, 3)
	require.Equal(t, 10.0, repo.withdrawals[0].Amount)
	require.InDelta(t, 1.23456789, repo.withdrawals[0].NetAmount, 0.000000001)

	netTotal := repo.withdrawalItems[0].NetAllocatedAmount + repo.withdrawalItems[1].NetAllocatedAmount + repo.withdrawalItems[2].NetAllocatedAmount
	require.InDelta(t, repo.withdrawals[0].NetAmount, netTotal, 0.000000001)
	require.InDelta(t, 0.41111111, repo.withdrawalItems[0].NetAllocatedAmount, 0.000000001)
	require.InDelta(t, 0.41111111, repo.withdrawalItems[1].NetAllocatedAmount, 0.000000001)
	require.InDelta(t, 0.41234567, repo.withdrawalItems[2].NetAllocatedAmount, 0.000000001)
	require.Equal(t, CommissionRewardStatusPaid, repo.rewards[0].Status)
	require.Equal(t, CommissionRewardStatusPaid, repo.rewards[1].Status)
	require.Equal(t, CommissionRewardStatusPaid, repo.rewards[2].Status)
	require.InDelta(t, 1.23456789, userRepo.balanceUpdates[200], 0.000000001)
}

func TestReferralWithdrawalService_ConvertCommissionToCredit_RechecksDailyLimitInsideTransaction(t *testing.T) {
	repo := newWithdrawalCommissionRepoStub()
	repo.rewards = []CommissionReward{
		{ID: 1, UserID: 200, RechargeOrderID: 11, RewardAmount: 20, Currency: ReferralSettlementCurrencyCNY, Status: CommissionRewardStatusAvailable},
	}
	repo.ledgers = []CommissionLedger{
		{ID: 1, UserID: 200, RewardID: int64ValuePtr(1), RechargeOrderID: int64ValuePtr(11), Bucket: CommissionLedgerBucketAvailable, Amount: 20, Currency: ReferralSettlementCurrencyCNY},
	}
	repo.countWithdrawalsResults = []int{0, 1}

	svc := newReferralWithdrawalServiceForTest(repo, map[string]string{
		SettingKeyReferralEnabled:                 "true",
		SettingKeyReferralWithdrawEnabled:         "false",
		SettingKeyReferralCreditConversionEnabled: "true",
		SettingKeyReferralWithdrawMinAmount:       "10",
		SettingKeyReferralWithdrawDailyLimit:      "1",
	}, nil)
	userRepo := svc.userRepo.(*withdrawalUserRepoStub)

	err := svc.ConvertCommissionToCredit(context.Background(), 200, 10)
	require.ErrorIs(t, err, ErrCommissionWithdrawDailyLimitExceeded)
	require.Len(t, repo.withdrawals, 0)
	require.Empty(t, userRepo.balanceUpdates)
	require.Equal(t, 2, repo.countWithdrawalsCalls)
}
