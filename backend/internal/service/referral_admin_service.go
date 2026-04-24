package service

import (
	"context"
	"errors"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

type AdminReferralRelation struct {
	UserID           int64      `json:"user_id"`
	UserEmail        string     `json:"user_email"`
	Username         string     `json:"username"`
	ReferrerUserID   *int64     `json:"referrer_user_id,omitempty"`
	ReferrerEmail    *string    `json:"referrer_email,omitempty"`
	ReferrerUsername *string    `json:"referrer_username,omitempty"`
	BindSource       string     `json:"bind_source"`
	BindCode         *string    `json:"bind_code,omitempty"`
	LockedAt         *time.Time `json:"locked_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type AdminReferralAccountOption struct {
	UserID       int64  `json:"user_id"`
	Email        string `json:"email"`
	Username     string `json:"username"`
	ReferralCode string `json:"referral_code"`
}

type AdminReferralOverview struct {
	TotalAccounts           int                        `json:"total_accounts"`
	TotalBoundUsers         int                        `json:"total_bound_users"`
	PendingCommission       float64                    `json:"pending_commission"`
	AvailableCommission     float64                    `json:"available_commission"`
	FrozenCommission        float64                    `json:"frozen_commission"`
	WithdrawnCommission     float64                    `json:"withdrawn_commission"`
	PendingWithdrawalCount  int                        `json:"pending_withdrawal_count"`
	PendingWithdrawalAmount float64                    `json:"pending_withdrawal_amount"`
	RecentTrend             []AdminReferralTrendPoint  `json:"recent_trend"`
	Ranking                 []AdminReferralRankingItem `json:"ranking"`
}

type AdminReferralTrendPoint struct {
	Date             string  `json:"date"`
	RewardAmount     float64 `json:"reward_amount"`
	WithdrawalAmount float64 `json:"withdrawal_amount"`
}

type AdminReferralRankingItem struct {
	UserID              int64   `json:"user_id"`
	Email               string  `json:"email"`
	Username            string  `json:"username"`
	ReferralCode        string  `json:"referral_code"`
	DirectInvitees      int     `json:"direct_invitees"`
	SecondLevelInvitees int     `json:"second_level_invitees"`
	TotalCommission     float64 `json:"total_commission"`
	AvailableCommission float64 `json:"available_commission"`
	WithdrawnCommission float64 `json:"withdrawn_commission"`
}

type AdminReferralTreeNode struct {
	UserID              int64                   `json:"user_id"`
	Email               string                  `json:"email"`
	Username            string                  `json:"username"`
	ReferralCode        string                  `json:"referral_code"`
	Level               int                     `json:"level"`
	DirectInvitees      int                     `json:"direct_invitees"`
	SecondLevelInvitees int                     `json:"second_level_invitees"`
	TotalCommission     float64                 `json:"total_commission"`
	AvailableCommission float64                 `json:"available_commission"`
	Children            []AdminReferralTreeNode `json:"children"`
}

type AdminCommissionReward struct {
	CommissionReward
	UserEmail       string  `json:"user_email"`
	Username        string  `json:"username"`
	SourceUserEmail string  `json:"source_user_email"`
	SourceUsername  string  `json:"source_username"`
	ExternalOrderID *string `json:"external_order_id,omitempty"`
}

type AdminCommissionLedger struct {
	CommissionLedger
	UserEmail    string  `json:"user_email"`
	Username     string  `json:"username"`
	WithdrawalNo *string `json:"withdrawal_no,omitempty"`
}

type AdminCommissionWithdrawal struct {
	CommissionWithdrawal
	UserEmail string `json:"user_email"`
	Username  string `json:"username"`
	ItemCount int    `json:"item_count"`
}

type AdminCommissionRewardFilter struct {
	UserID       int64
	SourceUserID int64
	Status       string
	Search       string
}

type AdminCommissionLedgerFilter struct {
	UserID    int64
	EntryType string
	Bucket    string
	Search    string
}

type AdminCommissionWithdrawalFilter struct {
	UserID int64
	Status string
	Search string
}

type AdminUpdateReferralRelationInput struct {
	UserID    int64
	Code      string
	ChangedBy int64
	Reason    string
	Notes     string
}

type AdminCommissionAdjustmentInput struct {
	RewardID       int64
	OperatorUserID int64
	Amount         float64
	Remark         string
}

type ReferralAdminRelationRepository interface {
	GetCodeByCode(ctx context.Context, code string) (*ReferralCode, error)
	GetRelationByUserID(ctx context.Context, userID int64) (*ReferralRelation, error)
	CreateRelationHistory(ctx context.Context, history *ReferralRelationHistory) error
	UpsertRelation(ctx context.Context, relation *ReferralRelation) error
	CountInvitees(ctx context.Context, userID int64) (*ReferralInviteeCounts, error)
	CountAllInvitees(ctx context.Context) (map[int64]*ReferralInviteeCounts, error)
	ListInvitees(ctx context.Context, userID int64, params pagination.PaginationParams) ([]ReferralInvitee, *pagination.PaginationResult, error)
	ListRelations(ctx context.Context, params pagination.PaginationParams, search string) ([]AdminReferralRelation, *pagination.PaginationResult, error)
	ListRelationHistories(ctx context.Context, params pagination.PaginationParams, userID int64) ([]ReferralRelationHistory, *pagination.PaginationResult, error)
}

type ReferralAdminCommissionRepository interface {
	GetRewardByID(ctx context.Context, rewardID int64) (*CommissionReward, error)
	UpdateReward(ctx context.Context, reward *CommissionReward) error
	CreateLedgerEntries(ctx context.Context, entries []CommissionLedger) error
	SumRewardBucketAmount(ctx context.Context, rewardID int64, bucket string) (float64, error)
	SumRewardBucketAmountForUpdate(ctx context.Context, rewardID int64, bucket string, forUpdate bool) (float64, error)
	SumUserBucketAmount(ctx context.Context, userID int64, bucket string) (float64, error)
	SumUserBucketAmountForUpdate(ctx context.Context, userID int64, bucket string, forUpdate bool) (float64, error)
	SumAllUserBucketAmounts(ctx context.Context, bucket string) (map[int64]float64, error)
	ListCommissionRewards(ctx context.Context, params pagination.PaginationParams, filter AdminCommissionRewardFilter) ([]AdminCommissionReward, *pagination.PaginationResult, error)
	ListCommissionLedgers(ctx context.Context, params pagination.PaginationParams, filter AdminCommissionLedgerFilter) ([]AdminCommissionLedger, *pagination.PaginationResult, error)
	ListAdminWithdrawals(ctx context.Context, params pagination.PaginationParams, filter AdminCommissionWithdrawalFilter) ([]AdminCommissionWithdrawal, *pagination.PaginationResult, error)
	ListWithdrawalItemsByWithdrawal(ctx context.Context, withdrawalID int64) ([]CommissionWithdrawalItem, error)
}

type ReferralAdminService struct {
	baseService    *ReferralService
	relationRepo   ReferralAdminRelationRepository
	commissionRepo ReferralAdminCommissionRepository
	entClient      *dbent.Client
	settlementSvc  *ReferralSettlementService
}

func NewReferralAdminService(
	baseService *ReferralService,
	relationRepo ReferralAdminRelationRepository,
	commissionRepo ReferralAdminCommissionRepository,
	entClient *dbent.Client,
	settlementSvc *ReferralSettlementService,
) *ReferralAdminService {
	return &ReferralAdminService{
		baseService:    baseService,
		relationRepo:   relationRepo,
		commissionRepo: commissionRepo,
		entClient:      entClient,
		settlementSvc:  settlementSvc,
	}
}

func (s *ReferralAdminService) UpdateRelation(ctx context.Context, input *AdminUpdateReferralRelationInput) (*ReferralRelation, error) {
	if input == nil || input.UserID <= 0 {
		return nil, ErrReferralRelationNotFound
	}
	code, err := s.baseService.ValidateReferralCode(ctx, input.Code)
	if err != nil {
		return nil, err
	}
	if code.UserID == input.UserID {
		return nil, ErrReferralSelfBind
	}
	if err := s.baseService.ensureNoCycle(ctx, input.UserID, code.UserID); err != nil {
		return nil, err
	}

	existing, err := s.relationRepo.GetRelationByUserID(ctx, input.UserID)
	if err != nil && !errors.Is(err, ErrReferralRelationNotFound) {
		return nil, err
	}

	relation := &ReferralRelation{
		UserID:         input.UserID,
		ReferrerUserID: code.UserID,
		BindSource:     ReferralBindSourceAdminOverride,
		BindCode:       stringValuePtr(strings.TrimSpace(input.Code)),
		Notes:          optionalTrimmedString(input.Notes),
	}
	if existing != nil {
		relation.ID = existing.ID
		relation.CreatedAt = existing.CreatedAt
	}

	history := &ReferralRelationHistory{
		UserID:            input.UserID,
		NewReferrerUserID: int64ValuePtr(code.UserID),
		NewBindCode:       stringValuePtr(strings.TrimSpace(input.Code)),
		ChangeSource:      ReferralBindSourceAdminOverride,
		ChangedBy:         int64ValuePtr(input.ChangedBy),
		Reason:            optionalTrimmedString(input.Reason),
	}
	if existing != nil {
		history.OldReferrerUserID = int64ValuePtr(existing.ReferrerUserID)
		history.OldBindCode = existing.BindCode
	}

	apply := func(txCtx context.Context) error {
		if err := s.relationRepo.UpsertRelation(txCtx, relation); err != nil {
			return err
		}
		return s.relationRepo.CreateRelationHistory(txCtx, history)
	}

	if err := s.withOptionalTx(ctx, apply); err != nil {
		return nil, err
	}
	return relation, nil
}

func (s *ReferralAdminService) CreateCommissionAdjustment(ctx context.Context, input *AdminCommissionAdjustmentInput) (*CommissionLedger, error) {
	if input == nil || input.RewardID <= 0 || input.Amount == 0 {
		return nil, ErrCommissionWithdrawAmountInvalid
	}

	var resultLedger *CommissionLedger

	apply := func(txCtx context.Context) error {
		reward, err := s.commissionRepo.GetRewardByID(txCtx, input.RewardID)
		if err != nil {
			return err
		}
		if input.Amount < 0 {
			available, err := s.commissionRepo.SumRewardBucketAmountForUpdate(txCtx, reward.ID, CommissionLedgerBucketAvailable, true)
			if err != nil {
				return err
			}
			if roundMoney(available+input.Amount) < 0 {
				return ErrCommissionWithdrawInsufficient
			}
		}

		entryType := CommissionLedgerEntryAdminAdd
		if input.Amount < 0 {
			entryType = CommissionLedgerEntryAdminSubtract
		}
		ledgers := []CommissionLedger{{
			UserID:          reward.UserID,
			RewardID:        int64ValuePtr(reward.ID),
			RechargeOrderID: int64ValuePtr(reward.RechargeOrderID),
			EntryType:       entryType,
			Bucket:          CommissionLedgerBucketAvailable,
			Amount:          roundMoney(input.Amount),
			Currency:        reward.Currency,
			OperatorUserID:  int64ValuePtr(input.OperatorUserID),
			Remark:          optionalTrimmedString(input.Remark),
		}}
		if err := s.commissionRepo.CreateLedgerEntries(txCtx, ledgers); err != nil {
			return err
		}

		now := time.Now()
		available, err := s.commissionRepo.SumRewardBucketAmount(txCtx, reward.ID, CommissionLedgerBucketAvailable)
		if err != nil {
			return err
		}
		frozen, err := s.commissionRepo.SumRewardBucketAmount(txCtx, reward.ID, CommissionLedgerBucketFrozen)
		if err != nil {
			return err
		}
		settled, err := s.commissionRepo.SumRewardBucketAmount(txCtx, reward.ID, CommissionLedgerBucketSettled)
		if err != nil {
			return err
		}
		switch {
		case settled > 0 && available <= 0 && frozen <= 0:
			reward.Status = CommissionRewardStatusPaid
			reward.PaidAt = timeValuePtr(now)
		case settled > 0:
			reward.Status = CommissionRewardStatusPartiallyPaid
			reward.PaidAt = timeValuePtr(now)
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
		if err := s.commissionRepo.UpdateReward(txCtx, reward); err != nil {
			return err
		}
		resultLedger = &ledgers[0]
		return nil
	}

	if err := s.withOptionalTx(ctx, apply); err != nil {
		return nil, err
	}
	return resultLedger, nil
}

func (s *ReferralAdminService) ListRelations(ctx context.Context, params pagination.PaginationParams, search string) ([]AdminReferralRelation, *pagination.PaginationResult, error) {
	return s.relationRepo.ListRelations(ctx, params, search)
}

func (s *ReferralAdminService) ListRelationHistories(ctx context.Context, params pagination.PaginationParams, userID int64) ([]ReferralRelationHistory, *pagination.PaginationResult, error) {
	return s.relationRepo.ListRelationHistories(ctx, params, userID)
}

func (s *ReferralAdminService) ListCommissionRewards(ctx context.Context, params pagination.PaginationParams, filter AdminCommissionRewardFilter) ([]AdminCommissionReward, *pagination.PaginationResult, error) {
	return s.commissionRepo.ListCommissionRewards(ctx, params, filter)
}

func (s *ReferralAdminService) ListCommissionLedgers(ctx context.Context, params pagination.PaginationParams, filter AdminCommissionLedgerFilter) ([]AdminCommissionLedger, *pagination.PaginationResult, error) {
	return s.commissionRepo.ListCommissionLedgers(ctx, params, filter)
}

func (s *ReferralAdminService) ListWithdrawals(ctx context.Context, params pagination.PaginationParams, filter AdminCommissionWithdrawalFilter) ([]AdminCommissionWithdrawal, *pagination.PaginationResult, error) {
	return s.commissionRepo.ListAdminWithdrawals(ctx, params, filter)
}

func (s *ReferralAdminService) ListWithdrawalItems(ctx context.Context, withdrawalID int64) ([]CommissionWithdrawalItem, error) {
	return s.commissionRepo.ListWithdrawalItemsByWithdrawal(ctx, withdrawalID)
}

func (s *ReferralAdminService) SearchAccounts(ctx context.Context, query string, limit int) ([]AdminReferralAccountOption, error) {
	if s.baseService == nil || s.baseService.userRepo == nil {
		return []AdminReferralAccountOption{}, nil
	}
	search := strings.TrimSpace(query)
	if search == "" {
		return []AdminReferralAccountOption{}, nil
	}
	if limit <= 0 {
		limit = 10
	}
	users, _, err := s.baseService.userRepo.ListWithFilters(ctx, pagination.PaginationParams{
		Page:     1,
		PageSize: limit,
	}, UserListFilters{
		Search: search,
	})
	if err != nil {
		return nil, err
	}
	options := make([]AdminReferralAccountOption, 0, len(users))
	for i := range users {
		code, codeErr := s.baseService.EnsureDefaultCode(ctx, users[i].ID)
		if codeErr != nil {
			return nil, codeErr
		}
		options = append(options, AdminReferralAccountOption{
			UserID:       users[i].ID,
			Email:        users[i].Email,
			Username:     users[i].Username,
			ReferralCode: code.Code,
		})
	}
	return options, nil
}

func (s *ReferralAdminService) withOptionalTx(ctx context.Context, apply func(context.Context) error) error {
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
