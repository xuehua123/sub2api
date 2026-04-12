package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	RechargeOrderStatusPending           = "pending"
	RechargeOrderStatusPaid              = "paid"
	RechargeOrderStatusCredited          = "credited"
	RechargeOrderStatusRefundPending     = "refund_pending"
	RechargeOrderStatusPartiallyRefunded = "partially_refunded"
	RechargeOrderStatusRefunded          = "refunded"
	RechargeOrderStatusChargeback        = "chargeback"
	RechargeOrderStatusClosed            = "closed"

	CommissionRewardStatusPending           = "pending"
	CommissionRewardStatusAvailable         = "available"
	CommissionRewardStatusFrozen            = "frozen"
	CommissionRewardStatusPartiallyFrozen   = "partially_frozen"
	CommissionRewardStatusPaid              = "paid"
	CommissionRewardStatusPartiallyPaid     = "partially_paid"
	CommissionRewardStatusReversed          = "reversed"
	CommissionRewardStatusPartiallyReversed = "partially_reversed"

	CommissionLedgerEntryRewardPendingCredit      = "reward_pending_credit"
	CommissionLedgerEntryRewardPendingToAvailable = "reward_pending_to_available"
	CommissionLedgerEntryWithdrawFreeze           = "withdraw_freeze"
	CommissionLedgerEntryWithdrawRejectReturn     = "withdraw_reject_return"
	CommissionLedgerEntryWithdrawPaid             = "withdraw_paid"
	CommissionLedgerEntryRefundReverse            = "refund_reverse"
	CommissionLedgerEntryAdminAdd                 = "admin_add"
	CommissionLedgerEntryAdminSubtract            = "admin_subtract"
	CommissionLedgerEntryNegativeCarry            = "negative_carry"

	CommissionLedgerBucketPending   = "pending"
	CommissionLedgerBucketAvailable = "available"
	CommissionLedgerBucketFrozen    = "frozen"
	CommissionLedgerBucketSettled   = "settled"

	CommissionWithdrawalStatusPendingReview = "pending_review"
	CommissionWithdrawalStatusApproved      = "approved"
	CommissionWithdrawalStatusRejected      = "rejected"
	CommissionWithdrawalStatusPaid          = "paid"

	CommissionWithdrawalItemStatusFrozen   = "frozen"
	CommissionWithdrawalItemStatusReturned = "returned"
	CommissionWithdrawalItemStatusPaid     = "paid"

	CommissionPayoutMethodAlipay = "alipay"
	CommissionPayoutMethodWechat = "wechat"
	CommissionPayoutMethodBank   = "bank"
)

var (
	ErrRechargeOrderNotFound                = infraerrors.NotFound("RECHARGE_ORDER_NOT_FOUND", "recharge order not found")
	ErrRechargeOrderConflict                = infraerrors.Conflict("RECHARGE_ORDER_CONFLICT", "recharge order already exists for a different user")
	ErrRechargeOrderCurrencyInvalid         = infraerrors.BadRequest("RECHARGE_ORDER_CURRENCY_INVALID", "recharge order only supports CNY settlement")
	ErrRechargeOrderAmountInvalid           = infraerrors.BadRequest("RECHARGE_ORDER_AMOUNT_INVALID", "paid amount must be greater than zero")
	ErrCommissionWithdrawalNotFound         = infraerrors.NotFound("COMMISSION_WITHDRAWAL_NOT_FOUND", "commission withdrawal not found")
	ErrCommissionWithdrawAmountInvalid      = infraerrors.BadRequest("COMMISSION_WITHDRAW_AMOUNT_INVALID", "withdraw amount is invalid")
	ErrCommissionWithdrawInsufficient       = infraerrors.BadRequest("COMMISSION_WITHDRAW_INSUFFICIENT", "insufficient available commission")
	ErrCommissionWithdrawMethodInvalid      = infraerrors.BadRequest("COMMISSION_WITHDRAW_METHOD_INVALID", "withdraw method is invalid")
	ErrCommissionWithdrawDailyLimitExceeded = infraerrors.BadRequest("COMMISSION_WITHDRAW_DAILY_LIMIT_EXCEEDED", "daily withdrawal limit exceeded")
	ErrCommissionWithdrawalConflict         = infraerrors.Conflict("COMMISSION_WITHDRAWAL_CONFLICT", "withdrawal is not in a reviewable state")
	ErrCommissionPayoutAccountNotFound      = infraerrors.NotFound("COMMISSION_PAYOUT_ACCOUNT_NOT_FOUND", "commission payout account not found")
	ErrCommissionPayoutAccountUpdateTooFrequent = infraerrors.BadRequest("COMMISSION_PAYOUT_ACCOUNT_UPDATE_TOO_FREQUENT", "payout account can only be modified once every 7 days")
)

type RechargeOrder struct {
	ID                    int64      `json:"id"`
	UserID                int64      `json:"user_id"`
	ExternalOrderID       string     `json:"external_order_id"`
	Provider              string     `json:"provider"`
	Channel               *string    `json:"channel,omitempty"`
	Currency              string     `json:"currency"`
	GrossAmount           float64    `json:"gross_amount"`
	DiscountAmount        float64    `json:"discount_amount"`
	PaidAmount            float64    `json:"paid_amount"`
	GiftBalanceAmount     float64    `json:"gift_balance_amount"`
	CreditedBalanceAmount float64    `json:"credited_balance_amount"`
	RefundedAmount        float64    `json:"refunded_amount"`
	ChargebackAmount      float64    `json:"chargeback_amount"`
	Status                string     `json:"status"`
	PaidAt                *time.Time `json:"paid_at,omitempty"`
	CreditedAt            *time.Time `json:"credited_at,omitempty"`
	RefundedAt            *time.Time `json:"refunded_at,omitempty"`
	ChargebackAt          *time.Time `json:"chargeback_at,omitempty"`
	IdempotencyKey        *string    `json:"idempotency_key,omitempty"`
	MetadataJSON          *string    `json:"metadata_json,omitempty"`
	Notes                 *string    `json:"notes,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

type CommissionReward struct {
	ID                   int64      `json:"id"`
	UserID               int64      `json:"user_id"`
	SourceUserID         int64      `json:"source_user_id"`
	RechargeOrderID      int64      `json:"recharge_order_id"`
	Level                int        `json:"level"`
	RateSnapshot         float64    `json:"rate_snapshot"`
	BaseAmountSnapshot   float64    `json:"base_amount_snapshot"`
	RewardAmount         float64    `json:"reward_amount"`
	Currency             string     `json:"currency"`
	RewardModeSnapshot   string     `json:"reward_mode_snapshot"`
	Status               string     `json:"status"`
	AvailableAt          *time.Time `json:"available_at,omitempty"`
	FrozenAt             *time.Time `json:"frozen_at,omitempty"`
	PaidAt               *time.Time `json:"paid_at,omitempty"`
	ReversedAt           *time.Time `json:"reversed_at,omitempty"`
	RuleSnapshotJSON     *string    `json:"rule_snapshot_json,omitempty"`
	RelationSnapshotJSON *string    `json:"relation_snapshot_json,omitempty"`
	Notes                *string    `json:"notes,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

type CommissionLedger struct {
	ID               int64     `json:"id"`
	UserID           int64     `json:"user_id"`
	RewardID         *int64    `json:"reward_id,omitempty"`
	RechargeOrderID  *int64    `json:"recharge_order_id,omitempty"`
	WithdrawalID     *int64    `json:"withdrawal_id,omitempty"`
	WithdrawalItemID *int64    `json:"withdrawal_item_id,omitempty"`
	EntryType        string    `json:"entry_type"`
	Bucket           string    `json:"bucket"`
	Amount           float64   `json:"amount"`
	Currency         string    `json:"currency"`
	IdempotencyKey   *string   `json:"idempotency_key,omitempty"`
	OperatorUserID   *int64    `json:"operator_user_id,omitempty"`
	Remark           *string   `json:"remark,omitempty"`
	MetadataJSON     *string   `json:"metadata_json,omitempty"`
	CreatedAt        time.Time `json:"created_at"`

	SourceUserEmail    string  `json:"source_user_email,omitempty"`
	SourceUserUsername string  `json:"source_user_username,omitempty"`
	ExternalOrderID    string  `json:"external_order_id,omitempty"`
	OrderPaidAmount    float64 `json:"order_paid_amount,omitempty"`
	RewardRateSnapshot float64 `json:"reward_rate_snapshot,omitempty"`
	RewardLevel        int     `json:"reward_level,omitempty"`
}

type CommissionWithdrawal struct {
	ID                        int64      `json:"id"`
	UserID                    int64      `json:"user_id"`
	WithdrawalNo              string     `json:"withdrawal_no"`
	Amount                    float64    `json:"amount"`
	FeeAmount                 float64    `json:"fee_amount"`
	NetAmount                 float64    `json:"net_amount"`
	Currency                  string     `json:"currency"`
	Status                    string     `json:"status"`
	PayoutMethod              string     `json:"payout_method"`
	PayoutAccountSnapshotJSON *string    `json:"payout_account_snapshot_json,omitempty"`
	ReviewedBy                *int64     `json:"reviewed_by,omitempty"`
	ReviewedAt                *time.Time `json:"reviewed_at,omitempty"`
	PaidBy                    *int64     `json:"paid_by,omitempty"`
	PaidAt                    *time.Time `json:"paid_at,omitempty"`
	RejectReason              *string    `json:"reject_reason,omitempty"`
	Remark                    *string    `json:"remark,omitempty"`
	CreatedAt                 time.Time  `json:"created_at"`
	UpdatedAt                 time.Time  `json:"updated_at"`
}

type CommissionWithdrawalItem struct {
	ID                 int64     `json:"id"`
	WithdrawalID       int64     `json:"withdrawal_id"`
	UserID             int64     `json:"user_id"`
	RewardID           int64     `json:"reward_id"`
	RechargeOrderID    int64     `json:"recharge_order_id"`
	AllocatedAmount    float64   `json:"allocated_amount"`
	FeeAllocatedAmount float64   `json:"fee_allocated_amount"`
	NetAllocatedAmount float64   `json:"net_allocated_amount"`
	Currency           string    `json:"currency"`
	Status             string    `json:"status"`
	FreezeLedgerID     *int64    `json:"freeze_ledger_id,omitempty"`
	ReturnLedgerID     *int64    `json:"return_ledger_id,omitempty"`
	PaidLedgerID       *int64    `json:"paid_ledger_id,omitempty"`
	ReverseLedgerID    *int64    `json:"reverse_ledger_id,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	// Display fields (populated by enriched queries)
	SourceUserEmail    string    `json:"source_user_email,omitempty"`
	ExternalOrderID    string    `json:"external_order_id,omitempty"`
	OrderPaidAmount    float64   `json:"order_paid_amount,omitempty"`
	RewardRateSnapshot float64   `json:"reward_rate_snapshot,omitempty"`
	OrderPaidAt        *time.Time `json:"order_paid_at,omitempty"`
}

type CommissionPayoutAccount struct {
	ID                 int64     `json:"id"`
	UserID             int64     `json:"user_id"`
	Method             string    `json:"method"`
	AccountName        string    `json:"account_name"`
	AccountNoMasked    *string   `json:"account_no_masked,omitempty"`
	AccountNoEncrypted *string   `json:"account_no_encrypted,omitempty"`
	BankName           *string   `json:"bank_name,omitempty"`
	QRImageURL         *string   `json:"qr_image_url,omitempty"`
	IsDefault          bool      `json:"is_default"`
	Status             string    `json:"status"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type RechargeCreditInput struct {
	UserID                int64
	ExternalOrderID       string
	Provider              string
	Channel               string
	Currency              string
	GrossAmount           float64
	DiscountAmount        float64
	PaidAmount            float64
	GiftBalanceAmount     float64
	CreditedBalanceAmount float64
	IdempotencyKey        string
	MetadataJSON          string
	Notes                 string
	PaidAt                *time.Time
}

type RechargeCreditResult struct {
	RechargeOrder     *RechargeOrder     `json:"recharge_order"`
	CommissionRewards []CommissionReward `json:"commission_rewards"`
}

type RechargeOrderRepository interface {
	GetByProviderAndExternalOrderID(ctx context.Context, provider, externalOrderID string) (*RechargeOrder, error)
	GetByID(ctx context.Context, id int64) (*RechargeOrder, error)
	Create(ctx context.Context, order *RechargeOrder) error
	Update(ctx context.Context, order *RechargeOrder) error
	CountPaidOrdersByUser(ctx context.Context, userID int64) (int, error)
	HasRefundOrChargeback(ctx context.Context, rechargeOrderID int64) (bool, error)
}

type CommissionRepository interface {
	CreateReward(ctx context.Context, reward *CommissionReward) error
	ListRewardsByRechargeOrder(ctx context.Context, rechargeOrderID int64) ([]CommissionReward, error)
	ListPendingRewardsReady(ctx context.Context, readyAt time.Time) ([]CommissionReward, error)
	ListRewardsByUser(ctx context.Context, userID int64, statuses []string) ([]CommissionReward, error)
	UpdateReward(ctx context.Context, reward *CommissionReward) error
	CreateLedgerEntries(ctx context.Context, entries []CommissionLedger) error
	SumRewardBucketAmount(ctx context.Context, rewardID int64, bucket string) (float64, error)
	GetRewardByID(ctx context.Context, rewardID int64) (*CommissionReward, error)
	SumRewardBucketAmountForUpdate(ctx context.Context, rewardID int64, bucket string, forUpdate bool) (float64, error)
	CreateWithdrawal(ctx context.Context, withdrawal *CommissionWithdrawal) error
	GetWithdrawalByID(ctx context.Context, id int64) (*CommissionWithdrawal, error)
	UpdateWithdrawal(ctx context.Context, withdrawal *CommissionWithdrawal) error
	CreateWithdrawalItems(ctx context.Context, items []CommissionWithdrawalItem) error
	ListWithdrawalItemsByWithdrawal(ctx context.Context, withdrawalID int64) ([]CommissionWithdrawalItem, error)
	UpdateWithdrawalItem(ctx context.Context, item *CommissionWithdrawalItem) error
	CountWithdrawalsByUserSince(ctx context.Context, userID int64, since time.Time) (int, error)
	ListPayoutAccountsByUser(ctx context.Context, userID int64) ([]CommissionPayoutAccount, error)
	UpsertPayoutAccount(ctx context.Context, account *CommissionPayoutAccount) error
}

type ReferralRewardService struct {
	rechargeOrders RechargeOrderRepository
	commissionRepo CommissionRepository
	userRepo       UserRepository
	referralRepo   ReferralRepository
	entClient      *dbent.Client
	settingService *SettingService
	settlementSvc  *ReferralSettlementService
}

func NewReferralRewardService(
	rechargeOrders RechargeOrderRepository,
	commissionRepo CommissionRepository,
	userRepo UserRepository,
	referralRepo ReferralRepository,
	entClient *dbent.Client,
	settingService *SettingService,
	settlementSvc *ReferralSettlementService,
) *ReferralRewardService {
	return &ReferralRewardService{
		rechargeOrders: rechargeOrders,
		commissionRepo: commissionRepo,
		userRepo:       userRepo,
		referralRepo:   referralRepo,
		entClient:      entClient,
		settingService: settingService,
		settlementSvc:  settlementSvc,
	}
}

func (s *ReferralRewardService) CreditRechargeOrder(ctx context.Context, input *RechargeCreditInput) (*RechargeCreditResult, error) {
	if input == nil {
		return nil, infraerrors.BadRequest("RECHARGE_ORDER_INVALID", "invalid recharge order request")
	}

	currency := strings.ToUpper(strings.TrimSpace(input.Currency))
	if currency == "" {
		currency = ReferralSettlementCurrencyCNY
	}
	if currency != ReferralSettlementCurrencyCNY {
		return nil, ErrRechargeOrderCurrencyInvalid
	}
	if input.PaidAmount <= 0 {
		return nil, ErrRechargeOrderAmountInvalid
	}

	existing, err := s.rechargeOrders.GetByProviderAndExternalOrderID(ctx, strings.TrimSpace(input.Provider), strings.TrimSpace(input.ExternalOrderID))
	if err == nil && existing != nil {
		if existing.UserID != input.UserID {
			return nil, ErrRechargeOrderConflict
		}
		rewards, listErr := s.commissionRepo.ListRewardsByRechargeOrder(ctx, existing.ID)
		if listErr != nil {
			return nil, listErr
		}
		return &RechargeCreditResult{
			RechargeOrder:     existing,
			CommissionRewards: rewards,
		}, nil
	}
	if err != nil && !errors.Is(err, ErrRechargeOrderNotFound) {
		return nil, err
	}

	settings, settingsErr := s.loadSettings(ctx)
	if settingsErr != nil {
		return nil, settingsErr
	}
	isFirstPaidOrder := true
	if settings.ReferralRewardMode == ReferralRewardModeFirstPaidOrder {
		count, countErr := s.rechargeOrders.CountPaidOrdersByUser(ctx, input.UserID)
		if countErr != nil {
			return nil, countErr
		}
		isFirstPaidOrder = count == 0
	}

	now := time.Now()
	paidAt := input.PaidAt
	if paidAt == nil {
		paidAt = &now
	}
	order := &RechargeOrder{
		UserID:                input.UserID,
		ExternalOrderID:       strings.TrimSpace(input.ExternalOrderID),
		Provider:              strings.TrimSpace(input.Provider),
		Channel:               optionalTrimmedString(input.Channel),
		Currency:              currency,
		GrossAmount:           input.GrossAmount,
		DiscountAmount:        input.DiscountAmount,
		PaidAmount:            input.PaidAmount,
		GiftBalanceAmount:     input.GiftBalanceAmount,
		CreditedBalanceAmount: input.CreditedBalanceAmount,
		Status:                RechargeOrderStatusCredited,
		PaidAt:                paidAt,
		CreditedAt:            &now,
		IdempotencyKey:        optionalTrimmedString(input.IdempotencyKey),
		MetadataJSON:          optionalTrimmedString(input.MetadataJSON),
		Notes:                 optionalTrimmedString(input.Notes),
	}

	apply := func(txCtx context.Context) (*RechargeCreditResult, error) {
		if err := s.rechargeOrders.Create(txCtx, order); err != nil {
			return nil, err
		}
		if order.CreditedBalanceAmount != 0 {
			if err := s.userRepo.UpdateBalance(txCtx, input.UserID, order.CreditedBalanceAmount); err != nil {
				return nil, err
			}
		}

		rewards, ledgers, rewardErr := s.buildRewardsAndLedgers(txCtx, settings, order, isFirstPaidOrder)
		if rewardErr != nil {
			return nil, rewardErr
		}
		// buildRewardsAndLedgers produces rewards and ledgers in lock-step (1:1).
		// Create each reward to get the DB-generated ID, then patch the
		// corresponding ledger entry before the ledger batch-insert.
		for i := range rewards {
			if err := s.commissionRepo.CreateReward(txCtx, &rewards[i]); err != nil {
				return nil, err
			}
			// Patch the ledger entry at the same index with the now-known reward ID.
			if i < len(ledgers) {
				ledgers[i].RewardID = int64ValuePtr(rewards[i].ID)
			}
		}
		if len(ledgers) > 0 {
			if err := s.commissionRepo.CreateLedgerEntries(txCtx, ledgers); err != nil {
				return nil, err
			}
		}
		if s.settlementSvc != nil {
			if _, err := s.settlementSvc.SettlePendingRewards(txCtx, now); err != nil {
				return nil, err
			}
		}
		return &RechargeCreditResult{
			RechargeOrder:     order,
			CommissionRewards: rewards,
		}, nil
	}

	if s.entClient == nil || dbent.TxFromContext(ctx) != nil {
		return apply(ctx)
	}

	tx, txErr := s.entClient.Tx(ctx)
	if txErr != nil {
		return nil, txErr
	}
	defer func() { _ = tx.Rollback() }()

	result, applyErr := apply(dbent.NewTxContext(ctx, tx))
	if applyErr != nil {
		return nil, applyErr
	}
	if commitErr := tx.Commit(); commitErr != nil {
		return nil, commitErr
	}
	return result, nil
}

func (s *ReferralRewardService) buildRewardsAndLedgers(
	ctx context.Context,
	settings *SystemSettings,
	order *RechargeOrder,
	isFirstPaidOrder bool,
) ([]CommissionReward, []CommissionLedger, error) {
	if settings == nil || order.PaidAmount <= 0 {
		return nil, nil, nil
	}
	if settings.ReferralRewardMode == ReferralRewardModeFirstPaidOrder && !isFirstPaidOrder {
		return nil, nil, nil
	}

	directRelation, err := s.referralRepo.GetRelationByUserID(ctx, order.UserID)
	if err != nil && !errors.Is(err, ErrReferralRelationNotFound) {
		return nil, nil, err
	}
	if errors.Is(err, ErrReferralRelationNotFound) || directRelation == nil {
		return nil, nil, nil
	}

	rewards := make([]CommissionReward, 0, 1)
	ledgers := make([]CommissionLedger, 0, 1)

	appendReward := func(userID int64, level int, rate float64, relationSnapshot map[string]any) error {
		if rate <= 0 {
			return nil
		}
		ruleSnapshot := map[string]any{
			"level":                 level,
			"rate":                  rate,
			"reward_mode":           settings.ReferralRewardMode,
			"settlement_delay_days": settings.ReferralSettlementDelayDays,
			"settlement_currency":   settings.ReferralSettlementCurrency,
			"level_1_enabled":       settings.ReferralLevel1Enabled,
		}
		ruleSnapshotJSON, err := json.Marshal(ruleSnapshot)
		if err != nil {
			return err
		}
		relationSnapshotJSON, err := json.Marshal(relationSnapshot)
		if err != nil {
			return err
		}
		reward := CommissionReward{
			UserID:               userID,
			SourceUserID:         order.UserID,
			RechargeOrderID:      order.ID,
			Level:                level,
			RateSnapshot:         rate,
			BaseAmountSnapshot:   order.PaidAmount,
			RewardAmount:         roundMoney(order.PaidAmount * rate),
			Currency:             order.Currency,
			RewardModeSnapshot:   settings.ReferralRewardMode,
			Status:               CommissionRewardStatusPending,
			AvailableAt:          timeValuePtr(order.PaidAt.Add(time.Duration(settings.ReferralSettlementDelayDays) * 24 * time.Hour)),
			RuleSnapshotJSON:     stringValuePtr(string(ruleSnapshotJSON)),
			RelationSnapshotJSON: stringValuePtr(string(relationSnapshotJSON)),
		}
		rewards = append(rewards, reward)
		ledgers = append(ledgers, CommissionLedger{
			UserID:          userID,
			RechargeOrderID: int64ValuePtr(order.ID),
			EntryType:       CommissionLedgerEntryRewardPendingCredit,
			Bucket:          CommissionLedgerBucketPending,
			Amount:          reward.RewardAmount,
			Currency:        order.Currency,
			IdempotencyKey:  optionalTrimmedString(order.ExternalOrderID + fmt.Sprintf(":reward:%d", level)),
		})
		return nil
	}

	if settings.ReferralLevel1Enabled {
		if err := appendReward(directRelation.ReferrerUserID, 1, settings.ReferralLevel1Rate, map[string]any{
			"source_user_id":     order.UserID,
			"direct_referrer_id": directRelation.ReferrerUserID,
			"bind_source":        directRelation.BindSource,
			"bind_code":          nullableStringValue(directRelation.BindCode),
		}); err != nil {
			return nil, nil, err
		}
	}

	return rewards, ledgers, nil
}

func (s *ReferralRewardService) loadSettings(ctx context.Context) (*SystemSettings, error) {
	if s.settingService == nil {
		return &SystemSettings{
			ReferralEnabled:             false,
			ReferralRewardMode:          ReferralRewardModeFirstPaidOrder,
			ReferralSettlementCurrency:  ReferralSettlementCurrencyCNY,
			ReferralSettlementDelayDays: 7,
		}, nil
	}
	return s.settingService.GetAllSettings(ctx)
}

func optionalTrimmedString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func stringValuePtr(value string) *string {
	return &value
}

func int64ValuePtr(value int64) *int64 {
	return &value
}

func timeValuePtr(value time.Time) *time.Time {
	return &value
}

func nullableStringValue(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func roundMoney(value float64) float64 {
	return math.Round(value*100000000) / 100000000
}
