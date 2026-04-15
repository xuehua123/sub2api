package service

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

type ReferralInviteeCounts struct {
	DirectInvitees      int `json:"direct_invitees"`
	SecondLevelInvitees int `json:"second_level_invitees"`
}

type ReferralInvitee struct {
	UserID          int64      `json:"user_id"`
	Email           string     `json:"email"`
	Username        string     `json:"username"`
	BoundAt         time.Time  `json:"bound_at"`
	ReferralCode    *string    `json:"referral_code,omitempty"`
	Source          *string    `json:"source,omitempty"`
	SecondLevelNum  int        `json:"second_level_num"`
	TotalRecharge   float64    `json:"total_recharge"`
	LatestPaidAt    *time.Time `json:"latest_paid_at,omitempty"`
	TotalCommission float64    `json:"total_commission"`
	OrderCount      int        `json:"order_count"`
}

type ReferralCenterOverview struct {
	ReferralOverview
	DirectInvitees      int     `json:"direct_invitees"`
	SecondLevelInvitees int     `json:"second_level_invitees"`
	PendingCommission   float64 `json:"pending_commission"`
	AvailableCommission float64 `json:"available_commission"`
	FrozenCommission    float64 `json:"frozen_commission"`
	WithdrawnCommission float64 `json:"withdrawn_commission"`
	TotalCommission     float64 `json:"total_commission"`
}

type ReferralCenterRelationRepository interface {
	CountInvitees(ctx context.Context, userID int64) (*ReferralInviteeCounts, error)
	ListInvitees(ctx context.Context, userID int64, params pagination.PaginationParams) ([]ReferralInvitee, *pagination.PaginationResult, error)
}

type ReferralCenterCommissionRepository interface {
	SumUserBucketAmount(ctx context.Context, userID int64, bucket string) (float64, error)
	ListLedgerEntriesByUser(ctx context.Context, userID int64, params pagination.PaginationParams) ([]CommissionLedger, *pagination.PaginationResult, error)
	ListWithdrawalsByUser(ctx context.Context, userID int64, params pagination.PaginationParams) ([]CommissionWithdrawal, *pagination.PaginationResult, error)
	ListPayoutAccountsByUser(ctx context.Context, userID int64) ([]CommissionPayoutAccount, error)
	ListRewardsByUserAndSource(ctx context.Context, userID int64, sourceUserID int64) ([]UserInviteeReward, error)
}

// UserInviteeReward is a user-facing view of a commission reward with enriched order info.
type UserInviteeReward struct {
	ID              int64     `json:"id"`
	RechargeOrderID int64     `json:"recharge_order_id"`
	ExternalOrderID string    `json:"external_order_id,omitempty"`
	OrderPaidAmount float64   `json:"order_paid_amount"`
	RateSnapshot    float64   `json:"rate_snapshot"`
	RewardAmount    float64   `json:"reward_amount"`
	Currency        string    `json:"currency"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
}

type ReferralCenterService struct {
	baseService    *ReferralService
	relationRepo   ReferralCenterRelationRepository
	commissionRepo ReferralCenterCommissionRepository
	settlementSvc  *ReferralSettlementService
}

func NewReferralCenterService(
	baseService *ReferralService,
	relationRepo ReferralCenterRelationRepository,
	commissionRepo ReferralCenterCommissionRepository,
	settlementSvc *ReferralSettlementService,
) *ReferralCenterService {
	return &ReferralCenterService{
		baseService:    baseService,
		relationRepo:   relationRepo,
		commissionRepo: commissionRepo,
		settlementSvc:  settlementSvc,
	}
}

func (s *ReferralCenterService) GetOverview(ctx context.Context, userID int64) (*ReferralCenterOverview, error) {
	if s.settlementSvc != nil {
		if _, err := s.settlementSvc.SettlePendingRewards(ctx, time.Now()); err != nil {
			return nil, err
		}
	}

	baseOverview, err := s.baseService.GetOverview(ctx, userID)
	if err != nil {
		return nil, err
	}

	counts, err := s.relationRepo.CountInvitees(ctx, userID)
	if err != nil {
		return nil, err
	}

	pendingAmount, err := s.commissionRepo.SumUserBucketAmount(ctx, userID, CommissionLedgerBucketPending)
	if err != nil {
		return nil, err
	}
	availableAmount, err := s.commissionRepo.SumUserBucketAmount(ctx, userID, CommissionLedgerBucketAvailable)
	if err != nil {
		return nil, err
	}
	frozenAmount, err := s.commissionRepo.SumUserBucketAmount(ctx, userID, CommissionLedgerBucketFrozen)
	if err != nil {
		return nil, err
	}
	settledAmount, err := s.commissionRepo.SumUserBucketAmount(ctx, userID, CommissionLedgerBucketSettled)
	if err != nil {
		return nil, err
	}

	return &ReferralCenterOverview{
		ReferralOverview:    *baseOverview,
		DirectInvitees:      counts.DirectInvitees,
		SecondLevelInvitees: counts.SecondLevelInvitees,
		PendingCommission:   pendingAmount,
		AvailableCommission: availableAmount,
		FrozenCommission:    frozenAmount,
		WithdrawnCommission: settledAmount,
		TotalCommission:     roundMoney(pendingAmount + availableAmount + frozenAmount + settledAmount),
	}, nil
}

func (s *ReferralCenterService) ListLedger(ctx context.Context, userID int64, params pagination.PaginationParams) ([]CommissionLedger, *pagination.PaginationResult, error) {
	if err := s.checkReferralEnabled(ctx, userID); err != nil {
		return nil, nil, err
	}
	return s.commissionRepo.ListLedgerEntriesByUser(ctx, userID, params)
}

func (s *ReferralCenterService) ListInvitees(ctx context.Context, userID int64, params pagination.PaginationParams) ([]ReferralInvitee, *pagination.PaginationResult, error) {
	if err := s.checkReferralEnabled(ctx, userID); err != nil {
		return nil, nil, err
	}
	return s.relationRepo.ListInvitees(ctx, userID, params)
}

func (s *ReferralCenterService) ListWithdrawals(ctx context.Context, userID int64, params pagination.PaginationParams) ([]CommissionWithdrawal, *pagination.PaginationResult, error) {
	if err := s.checkReferralEnabled(ctx, userID); err != nil {
		return nil, nil, err
	}
	return s.commissionRepo.ListWithdrawalsByUser(ctx, userID, params)
}

func (s *ReferralCenterService) ListPayoutAccounts(ctx context.Context, userID int64) ([]CommissionPayoutAccount, error) {
	if err := s.checkReferralEnabled(ctx, userID); err != nil {
		return nil, err
	}
	accounts, err := s.commissionRepo.ListPayoutAccountsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	result := make([]CommissionPayoutAccount, 0, len(accounts))
	for i := range accounts {
		if sanitized := sanitizePayoutAccount(&accounts[i]); sanitized != nil {
			result = append(result, *sanitized)
		}
	}
	return result, nil
}

func (s *ReferralCenterService) ListInviteeRewards(ctx context.Context, userID int64, sourceUserID int64) ([]UserInviteeReward, error) {
	if err := s.checkReferralEnabled(ctx, userID); err != nil {
		return nil, err
	}
	return s.commissionRepo.ListRewardsByUserAndSource(ctx, userID, sourceUserID)
}

func (s *ReferralCenterService) checkReferralEnabled(ctx context.Context, userID int64) error {
	enabled, err := s.baseService.isReferralEnabledForUser(ctx, userID)
	if err != nil {
		return err
	}
	if !enabled {
		return ErrReferralDisabled
	}
	return nil
}
