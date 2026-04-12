package service

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

type CreateReferralWithdrawalInput struct {
	UserID          int64
	Amount          float64
	PayoutMethod    string
	PayoutAccountID int64
	Remark          string
}

type ReviewReferralWithdrawalInput struct {
	WithdrawalID int64
	ReviewerID   int64
	Reason       string
}

type MarkReferralWithdrawalPaidInput struct {
	WithdrawalID int64
	PaidBy       int64
	Remark       string
}

type UpsertReferralPayoutAccountInput struct {
	Method      string
	AccountName string
	AccountNo   string
	BankName    string
	QRImageURL  string
	IsDefault   bool
	Status      string
}

type ReferralWithdrawalResult struct {
	Withdrawal *CommissionWithdrawal      `json:"withdrawal"`
	Items      []CommissionWithdrawalItem `json:"items"`
}

type ReferralWithdrawalService struct {
	commissionRepo CommissionRepository
	userRepo       UserRepository
	entClient      *dbent.Client
	settingService *SettingService
	settlementSvc  *ReferralSettlementService
	encryptor      SecretEncryptor
}

func NewReferralWithdrawalService(
	commissionRepo CommissionRepository,
	userRepo UserRepository,
	entClient *dbent.Client,
	settingService *SettingService,
	settlementSvc *ReferralSettlementService,
	encryptor SecretEncryptor,
) *ReferralWithdrawalService {
	return &ReferralWithdrawalService{
		commissionRepo: commissionRepo,
		userRepo:       userRepo,
		entClient:      entClient,
		settingService: settingService,
		settlementSvc:  settlementSvc,
		encryptor:      encryptor,
	}
}

func (s *ReferralWithdrawalService) UpsertPayoutAccount(ctx context.Context, userID, accountID int64, input *UpsertReferralPayoutAccountInput) (*CommissionPayoutAccount, error) {
	if input == nil || userID <= 0 {
		return nil, infraerrors.BadRequest("COMMISSION_PAYOUT_ACCOUNT_INVALID", "invalid payout account request")
	}

	settings, err := s.loadSettings(ctx)
	if err != nil {
		return nil, err
	}
	method := strings.TrimSpace(input.Method)
	if !containsString(settings.ReferralWithdrawMethodsEnabled, method) {
		return nil, ErrCommissionWithdrawMethodInvalid
	}

	accountName := strings.TrimSpace(input.AccountName)
	accountNo := strings.TrimSpace(input.AccountNo)

	accounts, err := s.commissionRepo.ListPayoutAccountsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	var existing *CommissionPayoutAccount
	for i := range accounts {
		if accounts[i].ID == accountID && accountID > 0 {
			account := accounts[i]
			existing = &account
			break
		}
	}
	if accountID > 0 && existing == nil {
		return nil, ErrCommissionPayoutAccountNotFound
	}
	if accountName == "" || (accountNo == "" && strings.TrimSpace(input.QRImageURL) == "" && existing == nil) {
		return nil, infraerrors.BadRequest("COMMISSION_PAYOUT_ACCOUNT_INVALID", "payout account details are required")
	}
	if existing != nil {
		nextAllowedAt := existing.UpdatedAt.Add(7 * 24 * time.Hour)
		if time.Now().Before(nextAllowedAt) {
			return nil, ErrCommissionPayoutAccountUpdateTooFrequent
		}
	}

	encryptedAccountNo := optionalTrimmedString(accountNo)
	if accountNo != "" && s.encryptor != nil {
		encrypted, encErr := s.encryptor.Encrypt(accountNo)
		if encErr != nil {
			return nil, encErr
		}
		encryptedAccountNo = stringValuePtr(encrypted)
	}

	account := &CommissionPayoutAccount{
		ID:                 accountID,
		UserID:             userID,
		Method:             method,
		AccountName:        accountName,
		AccountNoMasked:    stringValuePtr(maskAccountNo(accountNo)),
		AccountNoEncrypted: encryptedAccountNo,
		BankName:           optionalTrimmedString(input.BankName),
		QRImageURL:         optionalTrimmedString(input.QRImageURL),
		IsDefault:          input.IsDefault,
		Status:             StatusActive,
	}
	if existing != nil && account.CreatedAt.IsZero() {
		account.CreatedAt = existing.CreatedAt
		if accountNo == "" {
			account.AccountNoMasked = existing.AccountNoMasked
			account.AccountNoEncrypted = existing.AccountNoEncrypted
		}
		if strings.TrimSpace(input.BankName) == "" {
			account.BankName = existing.BankName
		}
		if strings.TrimSpace(input.QRImageURL) == "" {
			account.QRImageURL = existing.QRImageURL
		}
	}

	apply := func(txCtx context.Context) error {
		if account.IsDefault {
			for i := range accounts {
				if accounts[i].ID == account.ID || !accounts[i].IsDefault {
					continue
				}
				copyAccount := accounts[i]
				copyAccount.IsDefault = false
				if err := s.commissionRepo.UpsertPayoutAccount(txCtx, &copyAccount); err != nil {
					return err
				}
			}
		} else if account.ID == 0 && len(accounts) == 0 {
			account.IsDefault = true
		}
		return s.commissionRepo.UpsertPayoutAccount(txCtx, account)
	}

	if err := s.withOptionalTx(ctx, apply); err != nil {
		return nil, err
	}
	return sanitizePayoutAccount(account), nil
}

func (s *ReferralWithdrawalService) CreateWithdrawal(ctx context.Context, input *CreateReferralWithdrawalInput) (*ReferralWithdrawalResult, error) {
	if input == nil || input.UserID <= 0 || input.Amount <= 0 {
		return nil, ErrCommissionWithdrawAmountInvalid
	}
	settings, err := s.loadSettings(ctx)
	if err != nil {
		return nil, err
	}
	if !settings.ReferralWithdrawEnabled {
		return nil, ErrReferralDisabled
	}
	if !settings.ReferralEnabled {
		if s.userRepo == nil {
			return nil, ErrReferralDisabled
		}
		user, userErr := s.userRepo.GetByID(ctx, input.UserID)
		if userErr != nil {
			return nil, userErr
		}
		if !user.ReferralEnabled {
			return nil, ErrReferralDisabled
		}
	}

	method := strings.TrimSpace(input.PayoutMethod)
	if !containsString(settings.ReferralWithdrawMethodsEnabled, method) {
		return nil, ErrCommissionWithdrawMethodInvalid
	}
	if input.Amount < settings.ReferralWithdrawMinAmount {
		return nil, ErrCommissionWithdrawAmountInvalid
	}
	if settings.ReferralWithdrawMaxAmount > 0 && input.Amount > settings.ReferralWithdrawMaxAmount {
		return nil, ErrCommissionWithdrawAmountInvalid
	}
	if settings.ReferralWithdrawDailyLimit > 0 {
		now := time.Now()
		startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		count, err := s.commissionRepo.CountWithdrawalsByUserSince(ctx, input.UserID, startOfDay)
		if err != nil {
			return nil, err
		}
		if count >= settings.ReferralWithdrawDailyLimit {
			return nil, ErrCommissionWithdrawDailyLimitExceeded
		}
	}

	payoutAccount, err := s.findPayoutAccount(ctx, input.UserID, input.PayoutAccountID, method)
	if err != nil {
		return nil, err
	}

	feeAmount := roundMoney(input.Amount*settings.ReferralWithdrawFeeRate + settings.ReferralWithdrawFixedFee)
	if feeAmount < 0 {
		feeAmount = 0
	}
	if feeAmount > input.Amount {
		feeAmount = input.Amount
	}
	netAmount := roundMoney(input.Amount - feeAmount)
	snapshot, err := json.Marshal(payoutAccount)
	if err != nil {
		return nil, err
	}

	result := &ReferralWithdrawalResult{}
	apply := func(txCtx context.Context) error {
		if s.settlementSvc != nil {
			if _, err := s.settlementSvc.SettlePendingRewards(txCtx, time.Now()); err != nil {
				return err
			}
		}

		// Fetch rewards and allocate INSIDE the transaction so FOR UPDATE locks are held
		rewards, err := s.commissionRepo.ListRewardsByUser(txCtx, input.UserID, nil)
		if err != nil {
			return err
		}
		allocations, totalAvailable, err := s.allocateRewards(txCtx, rewards, input.Amount)
		if err != nil {
			return err
		}
		if totalAvailable < input.Amount {
			return ErrCommissionWithdrawInsufficient
		}

		now := time.Now()
		withdrawal := &CommissionWithdrawal{
			UserID:                    input.UserID,
			WithdrawalNo:              generateWithdrawalNo(now),
			Amount:                    roundMoney(input.Amount),
			FeeAmount:                 feeAmount,
			NetAmount:                 netAmount,
			Currency:                  ReferralSettlementCurrencyCNY,
			Status:                    CommissionWithdrawalStatusPendingReview,
			PayoutMethod:              method,
			PayoutAccountSnapshotJSON: stringValuePtr(string(snapshot)),
			Remark:                    optionalTrimmedString(input.Remark),
		}
		if !settings.ReferralWithdrawManualReviewRequired {
			withdrawal.Status = CommissionWithdrawalStatusApproved
		}

		if err := s.commissionRepo.CreateWithdrawal(txCtx, withdrawal); err != nil {
			return err
		}

		items := make([]CommissionWithdrawalItem, 0, len(allocations))
		freezeLedgers := make([]CommissionLedger, 0, len(allocations)*2)
		remainingFee := feeAmount
		remainingNet := netAmount
		for i, allocation := range allocations {
			itemFee := 0.0
			itemNet := allocation.amount
			if feeAmount > 0 {
				if i == len(allocations)-1 {
					itemFee = remainingFee
				} else {
					itemFee = roundMoney(feeAmount * allocation.amount / input.Amount)
				}
				remainingFee = roundMoney(remainingFee - itemFee)
				itemNet = roundMoney(allocation.amount - itemFee)
			}
			if i == len(allocations)-1 {
				itemNet = remainingNet
			}
			remainingNet = roundMoney(remainingNet - itemNet)

			freezeLedgers = append(freezeLedgers,
				CommissionLedger{
					UserID:          input.UserID,
					RewardID:        int64ValuePtr(allocation.reward.ID),
					RechargeOrderID: int64ValuePtr(allocation.reward.RechargeOrderID),
					WithdrawalID:    int64ValuePtr(withdrawal.ID),
					EntryType:       CommissionLedgerEntryWithdrawFreeze,
					Bucket:          CommissionLedgerBucketAvailable,
					Amount:          -allocation.amount,
					Currency:        ReferralSettlementCurrencyCNY,
				},
				CommissionLedger{
					UserID:          input.UserID,
					RewardID:        int64ValuePtr(allocation.reward.ID),
					RechargeOrderID: int64ValuePtr(allocation.reward.RechargeOrderID),
					WithdrawalID:    int64ValuePtr(withdrawal.ID),
					EntryType:       CommissionLedgerEntryWithdrawFreeze,
					Bucket:          CommissionLedgerBucketFrozen,
					Amount:          allocation.amount,
					Currency:        ReferralSettlementCurrencyCNY,
				},
			)

			items = append(items, CommissionWithdrawalItem{
				WithdrawalID:       withdrawal.ID,
				UserID:             input.UserID,
				RewardID:           allocation.reward.ID,
				RechargeOrderID:    allocation.reward.RechargeOrderID,
				AllocatedAmount:    allocation.amount,
				FeeAllocatedAmount: itemFee,
				NetAllocatedAmount: itemNet,
				Currency:           ReferralSettlementCurrencyCNY,
				Status:             CommissionWithdrawalItemStatusFrozen,
			})
		}
		if err := s.commissionRepo.CreateLedgerEntries(txCtx, freezeLedgers); err != nil {
			return err
		}
		for i := range items {
			if ledgerIndex := i*2 + 1; ledgerIndex < len(freezeLedgers) {
				items[i].FreezeLedgerID = int64ValuePtr(freezeLedgers[ledgerIndex].ID)
			}
		}
		if err := s.commissionRepo.CreateWithdrawalItems(txCtx, items); err != nil {
			return err
		}
		for i := range allocations {
			if err := s.refreshRewardStatus(txCtx, allocations[i].reward.ID, now); err != nil {
				return err
			}
		}
		result.Withdrawal = withdrawal
		result.Items = items
		return nil
	}
	if err := s.withOptionalTx(ctx, apply); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *ReferralWithdrawalService) ApproveWithdrawal(ctx context.Context, withdrawalID, reviewerID int64, remark string) (*CommissionWithdrawal, error) {
	withdrawal, err := s.commissionRepo.GetWithdrawalByID(ctx, withdrawalID)
	if err != nil {
		return nil, err
	}
	if withdrawal.Status != CommissionWithdrawalStatusPendingReview {
		return nil, ErrCommissionWithdrawalConflict
	}
	now := time.Now()
	withdrawal.Status = CommissionWithdrawalStatusApproved
	withdrawal.ReviewedBy = int64ValuePtr(reviewerID)
	withdrawal.ReviewedAt = timeValuePtr(now)
	withdrawal.Remark = optionalTrimmedString(remark)
	if err := s.commissionRepo.UpdateWithdrawal(ctx, withdrawal); err != nil {
		return nil, err
	}
	return withdrawal, nil
}

func (s *ReferralWithdrawalService) RejectWithdrawal(ctx context.Context, input *ReviewReferralWithdrawalInput) (*ReferralWithdrawalResult, error) {
	if input == nil || input.WithdrawalID <= 0 {
		return nil, ErrCommissionWithdrawalConflict
	}
	withdrawal, err := s.commissionRepo.GetWithdrawalByID(ctx, input.WithdrawalID)
	if err != nil {
		return nil, err
	}
	if withdrawal.Status != CommissionWithdrawalStatusPendingReview && withdrawal.Status != CommissionWithdrawalStatusApproved {
		return nil, ErrCommissionWithdrawalConflict
	}

	items, err := s.commissionRepo.ListWithdrawalItemsByWithdrawal(ctx, input.WithdrawalID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	result := &ReferralWithdrawalResult{}
	apply := func(txCtx context.Context) error {
		for i := range items {
			item := &items[i]
			ledgers := []CommissionLedger{
				{
					UserID:           withdrawal.UserID,
					RewardID:         int64ValuePtr(item.RewardID),
					RechargeOrderID:  int64ValuePtr(item.RechargeOrderID),
					WithdrawalID:     int64ValuePtr(withdrawal.ID),
					WithdrawalItemID: int64ValuePtr(item.ID),
					EntryType:        CommissionLedgerEntryWithdrawRejectReturn,
					Bucket:           CommissionLedgerBucketFrozen,
					Amount:           -item.AllocatedAmount,
					Currency:         item.Currency,
				},
				{
					UserID:           withdrawal.UserID,
					RewardID:         int64ValuePtr(item.RewardID),
					RechargeOrderID:  int64ValuePtr(item.RechargeOrderID),
					WithdrawalID:     int64ValuePtr(withdrawal.ID),
					WithdrawalItemID: int64ValuePtr(item.ID),
					EntryType:        CommissionLedgerEntryWithdrawRejectReturn,
					Bucket:           CommissionLedgerBucketAvailable,
					Amount:           item.AllocatedAmount,
					Currency:         item.Currency,
				},
			}
			if err := s.commissionRepo.CreateLedgerEntries(txCtx, ledgers); err != nil {
				return err
			}
			item.Status = CommissionWithdrawalItemStatusReturned
			item.ReturnLedgerID = int64ValuePtr(ledgers[1].ID)
			if err := s.commissionRepo.UpdateWithdrawalItem(txCtx, item); err != nil {
				return err
			}
			if err := s.refreshRewardStatus(txCtx, item.RewardID, now); err != nil {
				return err
			}
		}

		withdrawal.Status = CommissionWithdrawalStatusRejected
		withdrawal.ReviewedBy = int64ValuePtr(input.ReviewerID)
		withdrawal.ReviewedAt = timeValuePtr(now)
		withdrawal.RejectReason = optionalTrimmedString(input.Reason)
		if err := s.commissionRepo.UpdateWithdrawal(txCtx, withdrawal); err != nil {
			return err
		}
		result.Withdrawal = withdrawal
		result.Items = items
		return nil
	}
	if err := s.withOptionalTx(ctx, apply); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *ReferralWithdrawalService) MarkWithdrawalPaid(ctx context.Context, input *MarkReferralWithdrawalPaidInput) (*ReferralWithdrawalResult, error) {
	if input == nil || input.WithdrawalID <= 0 {
		return nil, ErrCommissionWithdrawalConflict
	}
	withdrawal, err := s.commissionRepo.GetWithdrawalByID(ctx, input.WithdrawalID)
	if err != nil {
		return nil, err
	}
	if withdrawal.Status != CommissionWithdrawalStatusApproved {
		return nil, ErrCommissionWithdrawalConflict
	}

	items, err := s.commissionRepo.ListWithdrawalItemsByWithdrawal(ctx, input.WithdrawalID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	result := &ReferralWithdrawalResult{}
	apply := func(txCtx context.Context) error {
		for i := range items {
			item := &items[i]
			ledgers := []CommissionLedger{
				{
					UserID:           withdrawal.UserID,
					RewardID:         int64ValuePtr(item.RewardID),
					RechargeOrderID:  int64ValuePtr(item.RechargeOrderID),
					WithdrawalID:     int64ValuePtr(withdrawal.ID),
					WithdrawalItemID: int64ValuePtr(item.ID),
					EntryType:        CommissionLedgerEntryWithdrawPaid,
					Bucket:           CommissionLedgerBucketFrozen,
					Amount:           -item.AllocatedAmount,
					Currency:         item.Currency,
				},
				{
					UserID:           withdrawal.UserID,
					RewardID:         int64ValuePtr(item.RewardID),
					RechargeOrderID:  int64ValuePtr(item.RechargeOrderID),
					WithdrawalID:     int64ValuePtr(withdrawal.ID),
					WithdrawalItemID: int64ValuePtr(item.ID),
					EntryType:        CommissionLedgerEntryWithdrawPaid,
					Bucket:           CommissionLedgerBucketSettled,
					Amount:           item.AllocatedAmount,
					Currency:         item.Currency,
				},
			}
			if err := s.commissionRepo.CreateLedgerEntries(txCtx, ledgers); err != nil {
				return err
			}
			item.Status = CommissionWithdrawalItemStatusPaid
			item.PaidLedgerID = int64ValuePtr(ledgers[1].ID)
			if err := s.commissionRepo.UpdateWithdrawalItem(txCtx, item); err != nil {
				return err
			}
			if err := s.refreshRewardStatus(txCtx, item.RewardID, now); err != nil {
				return err
			}
		}

		withdrawal.Status = CommissionWithdrawalStatusPaid
		withdrawal.PaidBy = int64ValuePtr(input.PaidBy)
		withdrawal.PaidAt = timeValuePtr(now)
		withdrawal.Remark = optionalTrimmedString(input.Remark)
		if err := s.commissionRepo.UpdateWithdrawal(txCtx, withdrawal); err != nil {
			return err
		}
		result.Withdrawal = withdrawal
		result.Items = items
		return nil
	}
	if err := s.withOptionalTx(ctx, apply); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *ReferralWithdrawalService) findPayoutAccount(ctx context.Context, userID, payoutAccountID int64, method string) (*CommissionPayoutAccount, error) {
	accounts, err := s.commissionRepo.ListPayoutAccountsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	for i := range accounts {
		account := accounts[i]
		if payoutAccountID > 0 && account.ID == payoutAccountID {
			if account.Method != method {
				return nil, ErrCommissionWithdrawMethodInvalid
			}
			return &account, nil
		}
	}
	if payoutAccountID > 0 {
		return nil, ErrCommissionPayoutAccountNotFound
	}
	for i := range accounts {
		if accounts[i].Method == method && accounts[i].IsDefault {
			account := accounts[i]
			return &account, nil
		}
	}
	return nil, ErrCommissionPayoutAccountNotFound
}

type rewardAllocation struct {
	reward CommissionReward
	amount float64
}

func (s *ReferralWithdrawalService) allocateRewards(ctx context.Context, rewards []CommissionReward, target float64) ([]rewardAllocation, float64, error) {
	totalAvailable := 0.0
	allocations := make([]rewardAllocation, 0)
	remaining := roundMoney(target)
	for _, reward := range rewards {
		available, err := s.commissionRepo.SumRewardBucketAmountForUpdate(ctx, reward.ID, CommissionLedgerBucketAvailable, true)
		if err != nil {
			return nil, 0, err
		}
		if available <= 0 {
			continue
		}
		totalAvailable = roundMoney(totalAvailable + available)
		if remaining <= 0 {
			continue
		}
		amount := available
		if amount > remaining {
			amount = remaining
		}
		amount = roundMoney(amount)
		if amount <= 0 {
			continue
		}
		allocations = append(allocations, rewardAllocation{reward: reward, amount: amount})
		remaining = roundMoney(remaining - amount)
	}
	return allocations, totalAvailable, nil
}

func (s *ReferralWithdrawalService) refreshRewardStatus(ctx context.Context, rewardID int64, now time.Time) error {
	reward, err := s.commissionRepo.GetRewardByID(ctx, rewardID)
	if err != nil {
		return err
	}

	available, err := s.commissionRepo.SumRewardBucketAmount(ctx, reward.ID, CommissionLedgerBucketAvailable)
	if err != nil {
		return err
	}
	frozen, err := s.commissionRepo.SumRewardBucketAmount(ctx, reward.ID, CommissionLedgerBucketFrozen)
	if err != nil {
		return err
	}
	settled, err := s.commissionRepo.SumRewardBucketAmount(ctx, reward.ID, CommissionLedgerBucketSettled)
	if err != nil {
		return err
	}

	switch {
	case settled > 0 && available <= 0 && frozen <= 0:
		reward.Status = CommissionRewardStatusPaid
		reward.PaidAt = timeValuePtr(now)
		reward.FrozenAt = nil
	case settled > 0:
		reward.Status = CommissionRewardStatusPartiallyPaid
		reward.PaidAt = timeValuePtr(now)
		if frozen > 0 {
			reward.FrozenAt = timeValuePtr(now)
		} else {
			reward.FrozenAt = nil
		}
	case frozen > 0 && available <= 0:
		reward.Status = CommissionRewardStatusFrozen
		reward.FrozenAt = timeValuePtr(now)
	case frozen > 0:
		reward.Status = CommissionRewardStatusPartiallyFrozen
		reward.FrozenAt = timeValuePtr(now)
	default:
		reward.Status = CommissionRewardStatusAvailable
		reward.FrozenAt = nil
	}

	return s.commissionRepo.UpdateReward(ctx, reward)
}

func (s *ReferralWithdrawalService) loadSettings(ctx context.Context) (*SystemSettings, error) {
	if s.settingService == nil {
		return &SystemSettings{
			ReferralEnabled:                      false,
			ReferralWithdrawEnabled:              false,
			ReferralWithdrawMinAmount:            100,
			ReferralWithdrawMaxAmount:            5000,
			ReferralWithdrawFeeRate:              0,
			ReferralWithdrawFixedFee:             0,
			ReferralWithdrawManualReviewRequired: true,
			ReferralWithdrawMethodsEnabled:       []string{"alipay", "wechat"},
		}, nil
	}
	return s.settingService.GetAllSettings(ctx)
}

func (s *ReferralWithdrawalService) withOptionalTx(ctx context.Context, apply func(context.Context) error) error {
	if s.entClient == nil || dbent.TxFromContext(ctx) != nil {
		return apply(ctx)
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := apply(dbent.NewTxContext(ctx, tx)); err != nil {
		return err
	}
	return tx.Commit()
}

func maskAccountNo(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) <= 4 {
		return trimmed
	}
	if len(trimmed) <= 8 {
		return trimmed[:2] + strings.Repeat("*", len(trimmed)-4) + trimmed[len(trimmed)-2:]
	}
	return trimmed[:4] + strings.Repeat("*", len(trimmed)-8) + trimmed[len(trimmed)-4:]
}

func generateWithdrawalNo(now time.Time) string {
	var buf [4]byte
	_, _ = rand.Read(buf[:])
	suffix := binary.BigEndian.Uint32(buf[:]) % 1000000
	return fmt.Sprintf("WD%s%06d", now.UTC().Format("20060102150405"), suffix)
}

func containsString(items []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, item := range items {
		if strings.TrimSpace(item) == target {
			return true
		}
	}
	return false
}

func sanitizePayoutAccount(account *CommissionPayoutAccount) *CommissionPayoutAccount {
	if account == nil {
		return nil
	}
	cloned := *account
	cloned.AccountNoEncrypted = nil
	return &cloned
}

func (s *ReferralWithdrawalService) ConvertCommissionToCredit(ctx context.Context, userID int64, amount float64) error {
	if userID <= 0 || amount <= 0 {
		return ErrCommissionWithdrawAmountInvalid
	}

	settings, err := s.loadSettings(ctx)
	if err != nil {
		return err
	}
	if !settings.ReferralWithdrawEnabled {
		return ErrReferralDisabled
	}
	if !settings.ReferralEnabled {
		if s.userRepo == nil {
			return ErrReferralDisabled
		}
		user, userErr := s.userRepo.GetByID(ctx, userID)
		if userErr != nil {
			return userErr
		}
		if !user.ReferralEnabled {
			return ErrReferralDisabled
		}
	}
	if amount < settings.ReferralWithdrawMinAmount {
		return ErrCommissionWithdrawAmountInvalid
	}
	if settings.ReferralWithdrawMaxAmount > 0 && amount > settings.ReferralWithdrawMaxAmount {
		return ErrCommissionWithdrawAmountInvalid
	}

	if settings.ReferralWithdrawDailyLimit > 0 {
		now := time.Now()
		startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		count, err := s.commissionRepo.CountWithdrawalsByUserSince(ctx, userID, startOfDay)
		if err != nil {
			return err
		}
		if count >= settings.ReferralWithdrawDailyLimit {
			return ErrCommissionWithdrawDailyLimitExceeded
		}
	}

	apply := func(txCtx context.Context) error {
		if s.settlementSvc != nil {
			if _, err := s.settlementSvc.SettlePendingRewards(txCtx, time.Now()); err != nil {
				return err
			}
		}

		rewards, err := s.commissionRepo.ListRewardsByUser(txCtx, userID, nil)
		if err != nil {
			return err
		}
		allocations, totalAvailable, err := s.allocateRewards(txCtx, rewards, amount)
		if err != nil {
			return err
		}
		if totalAvailable < amount {
			return ErrCommissionWithdrawInsufficient
		}

		now := time.Now()
		withdrawal := &CommissionWithdrawal{
			UserID:       userID,
			WithdrawalNo: generateWithdrawalNo(now),
			Amount:       roundMoney(amount),
			FeeAmount:    0,
			NetAmount:    roundMoney(amount),
			Currency:     ReferralSettlementCurrencyCNY,
			Status:       CommissionWithdrawalStatusPaid,
			PayoutMethod: "credit_conversion",
			PaidBy:       int64ValuePtr(userID),
			PaidAt:       timeValuePtr(now),
		}
		if err := s.commissionRepo.CreateWithdrawal(txCtx, withdrawal); err != nil {
			return err
		}

		items := make([]CommissionWithdrawalItem, 0, len(allocations))
		ledgers := make([]CommissionLedger, 0, len(allocations)*4)
		for _, allocation := range allocations {
			ledgers = append(ledgers,
				CommissionLedger{
					UserID:          userID,
					RewardID:        int64ValuePtr(allocation.reward.ID),
					RechargeOrderID: int64ValuePtr(allocation.reward.RechargeOrderID),
					WithdrawalID:    int64ValuePtr(withdrawal.ID),
					EntryType:       CommissionLedgerEntryWithdrawFreeze,
					Bucket:          CommissionLedgerBucketAvailable,
					Amount:          -allocation.amount,
					Currency:        ReferralSettlementCurrencyCNY,
				},
				CommissionLedger{
					UserID:          userID,
					RewardID:        int64ValuePtr(allocation.reward.ID),
					RechargeOrderID: int64ValuePtr(allocation.reward.RechargeOrderID),
					WithdrawalID:    int64ValuePtr(withdrawal.ID),
					EntryType:       CommissionLedgerEntryWithdrawFreeze,
					Bucket:          CommissionLedgerBucketFrozen,
					Amount:          allocation.amount,
					Currency:        ReferralSettlementCurrencyCNY,
				},
				CommissionLedger{
					UserID:          userID,
					RewardID:        int64ValuePtr(allocation.reward.ID),
					RechargeOrderID: int64ValuePtr(allocation.reward.RechargeOrderID),
					WithdrawalID:    int64ValuePtr(withdrawal.ID),
					EntryType:       CommissionLedgerEntryWithdrawPaid,
					Bucket:          CommissionLedgerBucketFrozen,
					Amount:          -allocation.amount,
					Currency:        ReferralSettlementCurrencyCNY,
				},
				CommissionLedger{
					UserID:          userID,
					RewardID:        int64ValuePtr(allocation.reward.ID),
					RechargeOrderID: int64ValuePtr(allocation.reward.RechargeOrderID),
					WithdrawalID:    int64ValuePtr(withdrawal.ID),
					EntryType:       CommissionLedgerEntryWithdrawPaid,
					Bucket:          CommissionLedgerBucketSettled,
					Amount:          allocation.amount,
					Currency:        ReferralSettlementCurrencyCNY,
				},
			)

			items = append(items, CommissionWithdrawalItem{
				WithdrawalID:       withdrawal.ID,
				UserID:             userID,
				RewardID:           allocation.reward.ID,
				RechargeOrderID:    allocation.reward.RechargeOrderID,
				AllocatedAmount:    allocation.amount,
				FeeAllocatedAmount: 0,
				NetAllocatedAmount: allocation.amount,
				Currency:           ReferralSettlementCurrencyCNY,
				Status:             CommissionWithdrawalItemStatusPaid,
			})
		}

		if err := s.commissionRepo.CreateLedgerEntries(txCtx, ledgers); err != nil {
			return err
		}
		if err := s.commissionRepo.CreateWithdrawalItems(txCtx, items); err != nil {
			return err
		}
		for i := range allocations {
			if err := s.refreshRewardStatus(txCtx, allocations[i].reward.ID, now); err != nil {
				return err
			}
		}

		return s.userRepo.UpdateBalance(txCtx, userID, amount)
	}

	return s.withOptionalTx(ctx, apply)
}
