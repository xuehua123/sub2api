package service

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

func (s *ReferralAdminService) GetOverview(ctx context.Context) (*AdminReferralOverview, error) {
	if s.settlementSvc != nil {
		if _, err := s.settlementSvc.SettlePendingRewards(ctx, time.Now()); err != nil {
			return nil, err
		}
	}

	relations, err := s.collectAllRelations(ctx)
	if err != nil {
		return nil, err
	}
	rewards, err := s.collectAllRewards(ctx)
	if err != nil {
		return nil, err
	}
	withdrawals, err := s.collectAllWithdrawals(ctx, AdminCommissionWithdrawalFilter{})
	if err != nil {
		return nil, err
	}

	// Batch-load all bucket amounts in 4 queries instead of 4*N
	pendingByUser, err := s.commissionRepo.SumAllUserBucketAmounts(ctx, CommissionLedgerBucketPending)
	if err != nil {
		return nil, err
	}
	availableByUser, err := s.commissionRepo.SumAllUserBucketAmounts(ctx, CommissionLedgerBucketAvailable)
	if err != nil {
		return nil, err
	}
	frozenByUser, err := s.commissionRepo.SumAllUserBucketAmounts(ctx, CommissionLedgerBucketFrozen)
	if err != nil {
		return nil, err
	}
	settledByUser, err := s.commissionRepo.SumAllUserBucketAmounts(ctx, CommissionLedgerBucketSettled)
	if err != nil {
		return nil, err
	}

	// Batch-load invitee counts in 1 query instead of N
	inviteeCountsByUser, err := s.relationRepo.CountAllInvitees(ctx)
	if err != nil {
		return nil, err
	}

	// Collect unique user IDs from relations and rewards
	accountIndex := make(map[int64]AdminReferralRankingItem)
	for _, relation := range relations {
		accountIndex[relation.UserID] = AdminReferralRankingItem{
			UserID:   relation.UserID,
			Email:    relation.UserEmail,
			Username: relation.Username,
		}
		if relation.ReferrerUserID != nil {
			item := accountIndex[*relation.ReferrerUserID]
			item.UserID = *relation.ReferrerUserID
			if relation.ReferrerEmail != nil {
				item.Email = *relation.ReferrerEmail
			}
			if relation.ReferrerUsername != nil {
				item.Username = *relation.ReferrerUsername
			}
			accountIndex[*relation.ReferrerUserID] = item
		}
	}
	for _, reward := range rewards {
		item := accountIndex[reward.UserID]
		item.UserID = reward.UserID
		item.Email = reward.UserEmail
		item.Username = reward.Username
		accountIndex[reward.UserID] = item
	}

	ranking := make([]AdminReferralRankingItem, 0, len(accountIndex))
	overview := &AdminReferralOverview{
		TotalBoundUsers: len(relations),
	}
	for userID, item := range accountIndex {
		if userID <= 0 {
			continue
		}
		referralCode, err := s.lookupDefaultReferralCode(ctx, userID)
		if err != nil {
			return nil, err
		}

		pending := pendingByUser[userID]
		available := availableByUser[userID]
		frozen := frozenByUser[userID]
		settled := settledByUser[userID]

		item.ReferralCode = referralCode
		if counts := inviteeCountsByUser[userID]; counts != nil {
			item.DirectInvitees = counts.DirectInvitees
			item.SecondLevelInvitees = counts.SecondLevelInvitees
		}
		item.AvailableCommission = available
		item.WithdrawnCommission = settled
		item.TotalCommission = roundMoney(pending + available + frozen + settled)
		ranking = append(ranking, item)

		overview.PendingCommission = roundMoney(overview.PendingCommission + pending)
		overview.AvailableCommission = roundMoney(overview.AvailableCommission + available)
		overview.FrozenCommission = roundMoney(overview.FrozenCommission + frozen)
		overview.WithdrawnCommission = roundMoney(overview.WithdrawnCommission + settled)
	}
	overview.TotalAccounts = len(ranking)

	sort.Slice(ranking, func(i, j int) bool {
		if ranking[i].TotalCommission == ranking[j].TotalCommission {
			return ranking[i].AvailableCommission > ranking[j].AvailableCommission
		}
		return ranking[i].TotalCommission > ranking[j].TotalCommission
	})
	if len(ranking) > 10 {
		overview.Ranking = ranking[:10]
	} else {
		overview.Ranking = ranking
	}
	overview.RecentTrend = buildAdminReferralTrend(rewards, withdrawals, time.Now())

	for _, withdrawal := range withdrawals {
		if withdrawal.Status != CommissionWithdrawalStatusPendingReview {
			continue
		}
		overview.PendingWithdrawalCount++
		overview.PendingWithdrawalAmount = roundMoney(overview.PendingWithdrawalAmount + withdrawal.NetAmount)
	}
	return overview, nil
}

func (s *ReferralAdminService) GetRelationTree(ctx context.Context, userID int64) (*AdminReferralTreeNode, error) {
	return s.buildTreeNode(ctx, userID, 0)
}

func (s *ReferralAdminService) buildTreeNode(ctx context.Context, userID int64, level int) (*AdminReferralTreeNode, error) {
	user, err := s.baseService.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	referralCode, err := s.lookupDefaultReferralCode(ctx, userID)
	if err != nil {
		return nil, err
	}
	counts, err := s.relationRepo.CountInvitees(ctx, userID)
	if err != nil {
		return nil, err
	}
	pending, err := s.commissionRepo.SumUserBucketAmount(ctx, userID, CommissionLedgerBucketPending)
	if err != nil {
		return nil, err
	}
	available, err := s.commissionRepo.SumUserBucketAmount(ctx, userID, CommissionLedgerBucketAvailable)
	if err != nil {
		return nil, err
	}
	frozen, err := s.commissionRepo.SumUserBucketAmount(ctx, userID, CommissionLedgerBucketFrozen)
	if err != nil {
		return nil, err
	}
	settled, err := s.commissionRepo.SumUserBucketAmount(ctx, userID, CommissionLedgerBucketSettled)
	if err != nil {
		return nil, err
	}

	node := &AdminReferralTreeNode{
		UserID:              userID,
		Email:               user.Email,
		Username:            user.Username,
		ReferralCode:        referralCode,
		Level:               level,
		DirectInvitees:      counts.DirectInvitees,
		SecondLevelInvitees: counts.SecondLevelInvitees,
		AvailableCommission: available,
		TotalCommission:     roundMoney(pending + available + frozen + settled),
	}
	if level >= 2 {
		return node, nil
	}

	invitees, err := s.collectAllInvitees(ctx, userID)
	if err != nil {
		return nil, err
	}
	for _, invitee := range invitees {
		child, err := s.buildTreeNode(ctx, invitee.UserID, level+1)
		if err != nil {
			return nil, err
		}
		node.Children = append(node.Children, *child)
	}
	return node, nil
}

func (s *ReferralAdminService) lookupDefaultReferralCode(ctx context.Context, userID int64) (string, error) {
	if s.baseService == nil || s.baseService.repo == nil {
		return "", nil
	}
	code, err := s.baseService.repo.GetDefaultCodeByUserID(ctx, userID)
	if errors.Is(err, ErrReferralCodeNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if code == nil {
		return "", nil
	}
	return code.Code, nil
}

func (s *ReferralAdminService) collectAllRelations(ctx context.Context) ([]AdminReferralRelation, error) {
	page := 1
	result := make([]AdminReferralRelation, 0)
	for {
		items, paginationResult, err := s.relationRepo.ListRelations(ctx, pagination.PaginationParams{
			Page:     page,
			PageSize: 100,
		}, "")
		if err != nil {
			return nil, err
		}
		result = append(result, items...)
		if paginationResult == nil || page >= paginationResult.Pages {
			break
		}
		page++
	}
	return result, nil
}

func (s *ReferralAdminService) collectAllRewards(ctx context.Context) ([]AdminCommissionReward, error) {
	page := 1
	result := make([]AdminCommissionReward, 0)
	for {
		items, paginationResult, err := s.commissionRepo.ListCommissionRewards(ctx, pagination.PaginationParams{
			Page:     page,
			PageSize: 100,
		}, AdminCommissionRewardFilter{})
		if err != nil {
			return nil, err
		}
		result = append(result, items...)
		if paginationResult == nil || page >= paginationResult.Pages {
			break
		}
		page++
	}
	return result, nil
}

func (s *ReferralAdminService) collectAllWithdrawals(ctx context.Context, filter AdminCommissionWithdrawalFilter) ([]AdminCommissionWithdrawal, error) {
	page := 1
	result := make([]AdminCommissionWithdrawal, 0)
	for {
		items, paginationResult, err := s.commissionRepo.ListAdminWithdrawals(ctx, pagination.PaginationParams{
			Page:     page,
			PageSize: 100,
		}, filter)
		if err != nil {
			return nil, err
		}
		result = append(result, items...)
		if paginationResult == nil || page >= paginationResult.Pages {
			break
		}
		page++
	}
	return result, nil
}

func (s *ReferralAdminService) collectAllInvitees(ctx context.Context, userID int64) ([]ReferralInvitee, error) {
	page := 1
	result := make([]ReferralInvitee, 0)
	for {
		items, paginationResult, err := s.relationRepo.ListInvitees(ctx, userID, pagination.PaginationParams{
			Page:     page,
			PageSize: 100,
		})
		if err != nil {
			return nil, err
		}
		result = append(result, items...)
		if paginationResult == nil || page >= paginationResult.Pages {
			break
		}
		page++
	}
	return result, nil
}

func buildAdminReferralTrend(
	rewards []AdminCommissionReward,
	withdrawals []AdminCommissionWithdrawal,
	now time.Time,
) []AdminReferralTrendPoint {
	location := now.Location()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location).AddDate(0, 0, -6)
	points := make([]AdminReferralTrendPoint, 0, 7)
	indexByDate := make(map[string]int, 7)

	for day := 0; day < 7; day++ {
		date := start.AddDate(0, 0, day).Format("2006-01-02")
		indexByDate[date] = len(points)
		points = append(points, AdminReferralTrendPoint{Date: date})
	}

	for _, reward := range rewards {
		date := reward.CreatedAt.In(location).Format("2006-01-02")
		index, ok := indexByDate[date]
		if !ok {
			continue
		}
		points[index].RewardAmount = roundMoney(points[index].RewardAmount + reward.RewardAmount)
	}

	for _, withdrawal := range withdrawals {
		date := withdrawal.CreatedAt.In(location).Format("2006-01-02")
		index, ok := indexByDate[date]
		if !ok {
			continue
		}
		points[index].WithdrawalAmount = roundMoney(points[index].WithdrawalAmount + withdrawal.NetAmount)
	}

	return points
}
