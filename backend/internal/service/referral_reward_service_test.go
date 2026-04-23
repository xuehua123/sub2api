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

type rechargeOrderRepoStub struct {
	orders          map[string]*RechargeOrder
	paidOrderCounts map[int64]int
	nextID          int64
}

func newRechargeOrderRepoStub() *rechargeOrderRepoStub {
	return &rechargeOrderRepoStub{
		orders:          make(map[string]*RechargeOrder),
		paidOrderCounts: make(map[int64]int),
		nextID:          1,
	}
}

func (s *rechargeOrderRepoStub) GetByProviderAndExternalOrderID(ctx context.Context, provider, externalOrderID string) (*RechargeOrder, error) {
	if order, ok := s.orders[provider+"::"+externalOrderID]; ok {
		cloned := *order
		return &cloned, nil
	}
	return nil, ErrRechargeOrderNotFound
}

func (s *rechargeOrderRepoStub) GetByID(ctx context.Context, id int64) (*RechargeOrder, error) {
	for _, order := range s.orders {
		if order.ID == id {
			cloned := *order
			return &cloned, nil
		}
	}
	return nil, ErrRechargeOrderNotFound
}

func (s *rechargeOrderRepoStub) Create(ctx context.Context, order *RechargeOrder) error {
	if order.ID == 0 {
		order.ID = s.nextID
		s.nextID++
	}
	now := time.Now()
	if order.CreatedAt.IsZero() {
		order.CreatedAt = now
	}
	if order.UpdatedAt.IsZero() {
		order.UpdatedAt = order.CreatedAt
	}
	cloned := *order
	s.orders[order.Provider+"::"+order.ExternalOrderID] = &cloned
	s.paidOrderCounts[order.UserID]++
	return nil
}

func (s *rechargeOrderRepoStub) Update(ctx context.Context, order *RechargeOrder) error {
	if _, ok := s.orders[order.Provider+"::"+order.ExternalOrderID]; ok {
		cloned := *order
		s.orders[order.Provider+"::"+order.ExternalOrderID] = &cloned
		return nil
	}
	return ErrRechargeOrderNotFound
}

func (s *rechargeOrderRepoStub) CountPaidOrdersByUser(ctx context.Context, userID int64) (int, error) {
	return s.paidOrderCounts[userID], nil
}

func (s *rechargeOrderRepoStub) HasRefundOrChargeback(ctx context.Context, rechargeOrderID int64) (bool, error) {
	order, err := s.GetByID(ctx, rechargeOrderID)
	if err != nil {
		return false, err
	}
	return order.RefundedAmount > 0 || order.ChargebackAmount > 0, nil
}

type commissionRepoStub struct {
	rewards []CommissionReward
	ledgers []CommissionLedger
}

func (s *commissionRepoStub) CreateReward(ctx context.Context, reward *CommissionReward) error {
	reward.ID = int64(len(s.rewards) + 1)
	cloned := *reward
	s.rewards = append(s.rewards, cloned)
	return nil
}

func (s *commissionRepoStub) GetRewardByID(ctx context.Context, rewardID int64) (*CommissionReward, error) {
	for i := range s.rewards {
		if s.rewards[i].ID == rewardID {
			cloned := s.rewards[i]
			return &cloned, nil
		}
	}
	return nil, ErrCommissionWithdrawalNotFound
}

func (s *commissionRepoStub) ListRewardsByRechargeOrder(ctx context.Context, rechargeOrderID int64) ([]CommissionReward, error) {
	var result []CommissionReward
	for _, reward := range s.rewards {
		if reward.RechargeOrderID == rechargeOrderID {
			result = append(result, reward)
		}
	}
	return result, nil
}

func (s *commissionRepoStub) ListPendingRewardsReady(ctx context.Context, readyAt time.Time) ([]CommissionReward, error) {
	var result []CommissionReward
	for _, reward := range s.rewards {
		if reward.AvailableAt == nil || reward.AvailableAt.After(readyAt) {
			continue
		}
		if reward.Status == CommissionRewardStatusPending || reward.Status == CommissionRewardStatusPartiallyReversed {
			result = append(result, reward)
		}
	}
	return result, nil
}

func (s *commissionRepoStub) ListRewardsByUser(ctx context.Context, userID int64, statuses []string) ([]CommissionReward, error) {
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

func (s *commissionRepoStub) UpdateReward(ctx context.Context, reward *CommissionReward) error {
	for i := range s.rewards {
		if s.rewards[i].ID == reward.ID {
			s.rewards[i] = *reward
			return nil
		}
	}
	return nil
}

func (s *commissionRepoStub) CreateLedgerEntries(ctx context.Context, entries []CommissionLedger) error {
	for _, entry := range entries {
		cloned := entry
		cloned.ID = int64(len(s.ledgers) + 1)
		s.ledgers = append(s.ledgers, cloned)
	}
	return nil
}

func (s *commissionRepoStub) SumRewardBucketAmount(ctx context.Context, rewardID int64, bucket string) (float64, error) {
	total := 0.0
	for _, ledger := range s.ledgers {
		if ledger.RewardID != nil && *ledger.RewardID == rewardID && ledger.Bucket == bucket {
			total += ledger.Amount
		}
	}
	return total, nil
}

func (s *commissionRepoStub) SumRewardBucketAmountForUpdate(ctx context.Context, rewardID int64, bucket string, forUpdate bool) (float64, error) {
	return s.SumRewardBucketAmount(ctx, rewardID, bucket)
}

func (s *commissionRepoStub) CreateWithdrawal(ctx context.Context, withdrawal *CommissionWithdrawal) error {
	withdrawal.ID = 1
	return nil
}

func (s *commissionRepoStub) GetWithdrawalByID(ctx context.Context, id int64) (*CommissionWithdrawal, error) {
	return nil, ErrCommissionWithdrawalNotFound
}

func (s *commissionRepoStub) UpdateWithdrawal(ctx context.Context, withdrawal *CommissionWithdrawal) error {
	return nil
}

func (s *commissionRepoStub) CreateWithdrawalItems(ctx context.Context, items []CommissionWithdrawalItem) error {
	return nil
}

func (s *commissionRepoStub) ListWithdrawalItemsByWithdrawal(ctx context.Context, withdrawalID int64) ([]CommissionWithdrawalItem, error) {
	return nil, nil
}

func (s *commissionRepoStub) UpdateWithdrawalItem(ctx context.Context, item *CommissionWithdrawalItem) error {
	return nil
}

func (s *commissionRepoStub) CountWithdrawalsByUserSince(ctx context.Context, userID int64, since time.Time) (int, error) {
	return 0, nil
}

func (s *commissionRepoStub) ListPayoutAccountsByUser(ctx context.Context, userID int64) ([]CommissionPayoutAccount, error) {
	return nil, nil
}

func (s *commissionRepoStub) UpsertPayoutAccount(ctx context.Context, account *CommissionPayoutAccount) error {
	return nil
}

type rewardUserRepoStub struct {
	users              map[int64]*User
	updateBalanceCalls []float64
}

func newRewardUserRepoStub() *rewardUserRepoStub {
	return &rewardUserRepoStub{
		users: make(map[int64]*User),
	}
}

func (s *rewardUserRepoStub) Create(ctx context.Context, user *User) error {
	panic("unexpected Create")
}
func (s *rewardUserRepoStub) GetByID(ctx context.Context, id int64) (*User, error) {
	return s.users[id], nil
}
func (s *rewardUserRepoStub) GetByEmail(ctx context.Context, email string) (*User, error) {
	panic("unexpected GetByEmail")
}
func (s *rewardUserRepoStub) GetFirstAdmin(ctx context.Context) (*User, error) {
	panic("unexpected GetFirstAdmin")
}
func (s *rewardUserRepoStub) Update(ctx context.Context, user *User) error {
	panic("unexpected Update")
}
func (s *rewardUserRepoStub) Delete(ctx context.Context, id int64) error { panic("unexpected Delete") }
func (s *rewardUserRepoStub) GetUserAvatar(context.Context, int64) (*UserAvatar, error) {
	panic("unexpected GetUserAvatar")
}
func (s *rewardUserRepoStub) UpsertUserAvatar(context.Context, int64, UpsertUserAvatarInput) (*UserAvatar, error) {
	panic("unexpected UpsertUserAvatar")
}
func (s *rewardUserRepoStub) DeleteUserAvatar(context.Context, int64) error {
	panic("unexpected DeleteUserAvatar")
}
func (s *rewardUserRepoStub) List(ctx context.Context, params pagination.PaginationParams) ([]User, *pagination.PaginationResult, error) {
	panic("unexpected List")
}
func (s *rewardUserRepoStub) ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters UserListFilters) ([]User, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters")
}
func (s *rewardUserRepoStub) UpdateBalance(ctx context.Context, id int64, amount float64) error {
	s.updateBalanceCalls = append(s.updateBalanceCalls, amount)
	if user, ok := s.users[id]; ok {
		user.Balance += amount
	}
	return nil
}
func (s *rewardUserRepoStub) DeductBalance(ctx context.Context, id int64, amount float64) error {
	panic("unexpected DeductBalance")
}
func (s *rewardUserRepoStub) UpdateConcurrency(ctx context.Context, id int64, amount int) error {
	panic("unexpected UpdateConcurrency")
}
func (s *rewardUserRepoStub) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	panic("unexpected ExistsByEmail")
}
func (s *rewardUserRepoStub) RemoveGroupFromAllowedGroups(ctx context.Context, groupID int64) (int64, error) {
	panic("unexpected RemoveGroupFromAllowedGroups")
}
func (s *rewardUserRepoStub) AddGroupToAllowedGroups(ctx context.Context, userID int64, groupID int64) error {
	panic("unexpected AddGroupToAllowedGroups")
}
func (s *rewardUserRepoStub) RemoveGroupFromUserAllowedGroups(ctx context.Context, userID int64, groupID int64) error {
	panic("unexpected RemoveGroupFromUserAllowedGroups")
}
func (s *rewardUserRepoStub) GetLatestUsedAtByUserIDs(context.Context, []int64) (map[int64]*time.Time, error) {
	panic("unexpected GetLatestUsedAtByUserIDs")
}
func (s *rewardUserRepoStub) GetLatestUsedAtByUserID(context.Context, int64) (*time.Time, error) {
	panic("unexpected GetLatestUsedAtByUserID")
}
func (s *rewardUserRepoStub) UpdateUserLastActiveAt(context.Context, int64, time.Time) error {
	panic("unexpected UpdateUserLastActiveAt")
}
func (s *rewardUserRepoStub) UpdateDefaultChatAPIKeyID(ctx context.Context, userID int64, apiKeyID *int64) error {
	panic("unexpected UpdateDefaultChatAPIKeyID")
}
func (s *rewardUserRepoStub) ListUserAuthIdentities(context.Context, int64) ([]UserAuthIdentityRecord, error) {
	panic("unexpected ListUserAuthIdentities")
}
func (s *rewardUserRepoStub) UnbindUserAuthProvider(context.Context, int64, string) error {
	panic("unexpected UnbindUserAuthProvider")
}
func (s *rewardUserRepoStub) UpdateTotpSecret(ctx context.Context, userID int64, encryptedSecret *string) error {
	panic("unexpected UpdateTotpSecret")
}
func (s *rewardUserRepoStub) EnableTotp(ctx context.Context, userID int64) error {
	panic("unexpected EnableTotp")
}
func (s *rewardUserRepoStub) DisableTotp(ctx context.Context, userID int64) error {
	panic("unexpected DisableTotp")
}

func newReferralRewardServiceForTest(
	rechargeRepo RechargeOrderRepository,
	commissionRepo CommissionRepository,
	userRepo UserRepository,
	referralRepo ReferralRepository,
	settings map[string]string,
) *ReferralRewardService {
	cfg := &config.Config{
		Default: config.DefaultConfig{
			UserBalance:     0,
			UserConcurrency: 1,
		},
	}
	settingService := NewSettingService(&settingRepoStub{values: settings}, cfg)
	settlementService := NewReferralSettlementService(commissionRepo, rechargeRepo, nil)
	return NewReferralRewardService(
		rechargeRepo,
		commissionRepo,
		userRepo,
		referralRepo,
		nil,
		settingService,
		settlementService,
	)
}

func TestReferralRewardService_CreditRechargeOrder_RejectsNonCNY(t *testing.T) {
	svc := newReferralRewardServiceForTest(newRechargeOrderRepoStub(), &commissionRepoStub{}, newRewardUserRepoStub(), newReferralRepoStub(), map[string]string{
		SettingKeyReferralEnabled: "true",
	})

	_, err := svc.CreditRechargeOrder(context.Background(), &RechargeCreditInput{
		UserID:          1,
		ExternalOrderID: "order-1",
		Provider:        "sub2apipay",
		Currency:        "USD",
		PaidAmount:      100,
	})
	require.ErrorIs(t, err, ErrRechargeOrderCurrencyInvalid)
}

func TestReferralRewardService_CreditRechargeOrder_GeneratesLevel1Rewards(t *testing.T) {
	rechargeRepo := newRechargeOrderRepoStub()
	commissionRepo := &commissionRepoStub{}
	userRepo := newRewardUserRepoStub()
	userRepo.users[100] = &User{ID: 100, Balance: 0}

	referralRepo := newReferralRepoStub()
	referralRepo.relationsByUser[100] = &ReferralRelation{
		UserID:         100,
		ReferrerUserID: 200,
		BindSource:     ReferralBindSourceLink,
	}

	svc := newReferralRewardServiceForTest(rechargeRepo, commissionRepo, userRepo, referralRepo, map[string]string{
		SettingKeyReferralEnabled:             "true",
		SettingKeyReferralLevel1Enabled:       "true",
		SettingKeyReferralLevel1Rate:          "0.10",
		SettingKeyReferralRewardMode:          ReferralRewardModeFirstPaidOrder,
		SettingKeyReferralSettlementDelayDays: "7",
	})

	result, err := svc.CreditRechargeOrder(context.Background(), &RechargeCreditInput{
		UserID:                100,
		ExternalOrderID:       "order-1",
		Provider:              "sub2apipay",
		Currency:              "CNY",
		PaidAmount:            100,
		CreditedBalanceAmount: 100,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.CommissionRewards, 1)
	require.Equal(t, float64(100), userRepo.users[100].Balance)
	require.Equal(t, []float64{100}, userRepo.updateBalanceCalls)
	require.Equal(t, int64(200), result.CommissionRewards[0].UserID)
	require.Equal(t, 10.0, result.CommissionRewards[0].RewardAmount)
	require.Len(t, commissionRepo.ledgers, 1)
	require.Equal(t, CommissionLedgerEntryRewardPendingCredit, commissionRepo.ledgers[0].EntryType)
}

func TestReferralRewardService_CreditRechargeOrder_UsesFirstPaidOrderMode(t *testing.T) {
	rechargeRepo := newRechargeOrderRepoStub()
	rechargeRepo.paidOrderCounts[100] = 1
	commissionRepo := &commissionRepoStub{}
	userRepo := newRewardUserRepoStub()
	userRepo.users[100] = &User{ID: 100}
	referralRepo := newReferralRepoStub()
	referralRepo.relationsByUser[100] = &ReferralRelation{
		UserID:         100,
		ReferrerUserID: 200,
		BindSource:     ReferralBindSourceLink,
	}

	svc := newReferralRewardServiceForTest(rechargeRepo, commissionRepo, userRepo, referralRepo, map[string]string{
		SettingKeyReferralEnabled:       "true",
		SettingKeyReferralLevel1Enabled: "true",
		SettingKeyReferralLevel1Rate:    "0.10",
		SettingKeyReferralRewardMode:    ReferralRewardModeFirstPaidOrder,
	})

	result, err := svc.CreditRechargeOrder(context.Background(), &RechargeCreditInput{
		UserID:                100,
		ExternalOrderID:       "order-2",
		Provider:              "sub2apipay",
		Currency:              "CNY",
		PaidAmount:            50,
		CreditedBalanceAmount: 50,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Empty(t, result.CommissionRewards)
}

func TestReferralRewardService_CreditRechargeOrder_IdempotentForSameOrderAndUser(t *testing.T) {
	rechargeRepo := newRechargeOrderRepoStub()
	commissionRepo := &commissionRepoStub{}
	userRepo := newRewardUserRepoStub()
	userRepo.users[100] = &User{ID: 100}
	referralRepo := newReferralRepoStub()
	referralRepo.relationsByUser[100] = &ReferralRelation{
		UserID:         100,
		ReferrerUserID: 200,
		BindSource:     ReferralBindSourceLink,
	}

	svc := newReferralRewardServiceForTest(rechargeRepo, commissionRepo, userRepo, referralRepo, map[string]string{
		SettingKeyReferralEnabled:       "true",
		SettingKeyReferralLevel1Enabled: "true",
		SettingKeyReferralLevel1Rate:    "0.10",
		SettingKeyReferralRewardMode:    ReferralRewardModeEveryPaidOrder,
	})

	first, err := svc.CreditRechargeOrder(context.Background(), &RechargeCreditInput{
		UserID:                100,
		ExternalOrderID:       "order-3",
		Provider:              "sub2apipay",
		Currency:              "CNY",
		PaidAmount:            80,
		CreditedBalanceAmount: 80,
	})
	require.NoError(t, err)
	require.Len(t, first.CommissionRewards, 1)

	second, err := svc.CreditRechargeOrder(context.Background(), &RechargeCreditInput{
		UserID:                100,
		ExternalOrderID:       "order-3",
		Provider:              "sub2apipay",
		Currency:              "CNY",
		PaidAmount:            80,
		CreditedBalanceAmount: 80,
	})
	require.NoError(t, err)
	require.Equal(t, first.RechargeOrder.ID, second.RechargeOrder.ID)
	require.Len(t, second.CommissionRewards, 1)
	require.Len(t, userRepo.updateBalanceCalls, 1)
	require.Len(t, commissionRepo.rewards, 1)
}

func TestReferralRewardService_CreditRechargeOrder_RejectsDifferentUserForSameOrder(t *testing.T) {
	rechargeRepo := newRechargeOrderRepoStub()
	commissionRepo := &commissionRepoStub{}
	userRepo := newRewardUserRepoStub()
	userRepo.users[100] = &User{ID: 100}
	userRepo.users[101] = &User{ID: 101}
	svc := newReferralRewardServiceForTest(rechargeRepo, commissionRepo, userRepo, newReferralRepoStub(), map[string]string{
		SettingKeyReferralEnabled: "true",
	})

	_, err := svc.CreditRechargeOrder(context.Background(), &RechargeCreditInput{
		UserID:                100,
		ExternalOrderID:       "order-4",
		Provider:              "sub2apipay",
		Currency:              "CNY",
		PaidAmount:            20,
		CreditedBalanceAmount: 20,
	})
	require.NoError(t, err)

	_, err = svc.CreditRechargeOrder(context.Background(), &RechargeCreditInput{
		UserID:                101,
		ExternalOrderID:       "order-4",
		Provider:              "sub2apipay",
		Currency:              "CNY",
		PaidAmount:            20,
		CreditedBalanceAmount: 20,
	})
	require.ErrorIs(t, err, ErrRechargeOrderConflict)
}

func TestReferralRewardService_CreditRechargeOrder_SettlesPreviouslyReadyPendingRewards(t *testing.T) {
	rechargeRepo := newRechargeOrderRepoStub()
	readyAt := time.Now().Add(-time.Hour)
	rechargeRepo.orders["provider::old-order"] = &RechargeOrder{
		ID:              1,
		UserID:          50,
		Provider:        "provider",
		ExternalOrderID: "old-order",
		PaidAmount:      20,
		Currency:        ReferralSettlementCurrencyCNY,
		Status:          RechargeOrderStatusCredited,
	}
	commissionRepo := &commissionRepoStub{
		rewards: []CommissionReward{
			{
				ID:              1,
				UserID:          200,
				SourceUserID:    50,
				RechargeOrderID: 1,
				RewardAmount:    8,
				Currency:        ReferralSettlementCurrencyCNY,
				Status:          CommissionRewardStatusPending,
				AvailableAt:     &readyAt,
			},
		},
		ledgers: []CommissionLedger{
			{
				ID:              1,
				UserID:          200,
				RewardID:        int64ValuePtr(1),
				RechargeOrderID: int64ValuePtr(1),
				EntryType:       CommissionLedgerEntryRewardPendingCredit,
				Bucket:          CommissionLedgerBucketPending,
				Amount:          8,
				Currency:        ReferralSettlementCurrencyCNY,
			},
		},
	}
	userRepo := newRewardUserRepoStub()
	userRepo.users[100] = &User{ID: 100}
	referralRepo := newReferralRepoStub()

	svc := newReferralRewardServiceForTest(rechargeRepo, commissionRepo, userRepo, referralRepo, map[string]string{
		SettingKeyReferralEnabled:       "true",
		SettingKeyReferralLevel1Enabled: "false",
		SettingKeyReferralRewardMode:    ReferralRewardModeEveryPaidOrder,
	})

	_, err := svc.CreditRechargeOrder(context.Background(), &RechargeCreditInput{
		UserID:                100,
		ExternalOrderID:       "order-5",
		Provider:              "sub2apipay",
		Currency:              "CNY",
		PaidAmount:            10,
		CreditedBalanceAmount: 10,
	})
	require.NoError(t, err)
	require.Equal(t, CommissionRewardStatusAvailable, commissionRepo.rewards[0].Status)
	require.Len(t, commissionRepo.ledgers, 3)
	require.Equal(t, CommissionLedgerEntryRewardPendingToAvailable, commissionRepo.ledgers[1].EntryType)
	require.Equal(t, CommissionLedgerBucketAvailable, commissionRepo.ledgers[2].Bucket)
}
