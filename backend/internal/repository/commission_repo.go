package repository

import (
	"context"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/commissionledger"
	"github.com/Wei-Shaw/sub2api/ent/commissionpayoutaccount"
	"github.com/Wei-Shaw/sub2api/ent/commissionreward"
	"github.com/Wei-Shaw/sub2api/ent/commissionwithdrawal"
	"github.com/Wei-Shaw/sub2api/ent/commissionwithdrawalitem"
	"github.com/Wei-Shaw/sub2api/ent/rechargeorder"
	"github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type commissionRepository struct {
	client *dbent.Client
}

func NewCommissionRepository(client *dbent.Client) service.CommissionRepository {
	return &commissionRepository{client: client}
}

func (r *commissionRepository) CreateReward(ctx context.Context, reward *service.CommissionReward) error {
	client := clientFromContext(ctx, r.client)
	builder := client.CommissionReward.Create().
		SetUserID(reward.UserID).
		SetSourceUserID(reward.SourceUserID).
		SetRechargeOrderID(reward.RechargeOrderID).
		SetLevel(reward.Level).
		SetRateSnapshot(reward.RateSnapshot).
		SetBaseAmountSnapshot(reward.BaseAmountSnapshot).
		SetRewardAmount(reward.RewardAmount).
		SetCurrency(reward.Currency).
		SetRewardModeSnapshot(reward.RewardModeSnapshot).
		SetStatus(reward.Status)

	if reward.AvailableAt != nil {
		builder.SetAvailableAt(*reward.AvailableAt)
	}
	if reward.FrozenAt != nil {
		builder.SetFrozenAt(*reward.FrozenAt)
	}
	if reward.PaidAt != nil {
		builder.SetPaidAt(*reward.PaidAt)
	}
	if reward.ReversedAt != nil {
		builder.SetReversedAt(*reward.ReversedAt)
	}
	if reward.RuleSnapshotJSON != nil {
		builder.SetRuleSnapshotJSON(*reward.RuleSnapshotJSON)
	}
	if reward.RelationSnapshotJSON != nil {
		builder.SetRelationSnapshotJSON(*reward.RelationSnapshotJSON)
	}
	if reward.Notes != nil {
		builder.SetNotes(*reward.Notes)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return err
	}
	reward.ID = created.ID
	reward.CreatedAt = created.CreatedAt
	reward.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *commissionRepository) ListRewardsByRechargeOrder(ctx context.Context, rechargeOrderID int64) ([]service.CommissionReward, error) {
	models, err := clientFromContext(ctx, r.client).CommissionReward.Query().
		Where(commissionreward.RechargeOrderIDEQ(rechargeOrderID)).
		Order(dbent.Asc(commissionreward.FieldLevel)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]service.CommissionReward, 0, len(models))
	for _, model := range models {
		if reward := commissionRewardEntityToService(model); reward != nil {
			result = append(result, *reward)
		}
	}
	return result, nil
}

func (r *commissionRepository) ListPendingRewardsReady(ctx context.Context, readyAt time.Time) ([]service.CommissionReward, error) {
	models, err := clientFromContext(ctx, r.client).CommissionReward.Query().
		Where(
			commissionreward.StatusEQ(service.CommissionRewardStatusPending),
			commissionreward.AvailableAtLTE(readyAt),
		).
		ForUpdate().
		Order(dbent.Asc(commissionreward.FieldAvailableAt)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]service.CommissionReward, 0, len(models))
	for _, model := range models {
		if reward := commissionRewardEntityToService(model); reward != nil {
			result = append(result, *reward)
		}
	}
	return result, nil
}

func (r *commissionRepository) ListRewardsByUser(ctx context.Context, userID int64, statuses []string) ([]service.CommissionReward, error) {
	query := clientFromContext(ctx, r.client).CommissionReward.Query().
		Where(commissionreward.UserIDEQ(userID))
	if len(statuses) > 0 {
		query = query.Where(commissionreward.StatusIn(statuses...))
	}
	models, err := query.Order(dbent.Asc(commissionreward.FieldCreatedAt)).All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]service.CommissionReward, 0, len(models))
	for _, model := range models {
		if reward := commissionRewardEntityToService(model); reward != nil {
			result = append(result, *reward)
		}
	}
	return result, nil
}

func (r *commissionRepository) UpdateReward(ctx context.Context, reward *service.CommissionReward) error {
	client := clientFromContext(ctx, r.client)
	builder := client.CommissionReward.UpdateOneID(reward.ID).
		SetStatus(reward.Status)
	if reward.AvailableAt != nil {
		builder.SetAvailableAt(*reward.AvailableAt)
	} else {
		builder.ClearAvailableAt()
	}
	if reward.FrozenAt != nil {
		builder.SetFrozenAt(*reward.FrozenAt)
	} else {
		builder.ClearFrozenAt()
	}
	if reward.PaidAt != nil {
		builder.SetPaidAt(*reward.PaidAt)
	} else {
		builder.ClearPaidAt()
	}
	if reward.ReversedAt != nil {
		builder.SetReversedAt(*reward.ReversedAt)
	} else {
		builder.ClearReversedAt()
	}
	if reward.RuleSnapshotJSON != nil {
		builder.SetRuleSnapshotJSON(*reward.RuleSnapshotJSON)
	}
	if reward.RelationSnapshotJSON != nil {
		builder.SetRelationSnapshotJSON(*reward.RelationSnapshotJSON)
	}
	if reward.Notes != nil {
		builder.SetNotes(*reward.Notes)
	}

	updated, err := builder.Save(ctx)
	if err != nil {
		return err
	}
	reward.UpdatedAt = updated.UpdatedAt
	return nil
}

func (r *commissionRepository) CreateLedgerEntries(ctx context.Context, entries []service.CommissionLedger) error {
	client := clientFromContext(ctx, r.client)
	for i := range entries {
		entry := entries[i]
		builder := client.CommissionLedger.Create().
			SetUserID(entry.UserID).
			SetEntryType(entry.EntryType).
			SetBucket(entry.Bucket).
			SetAmount(entry.Amount).
			SetCurrency(entry.Currency)

		if entry.RewardID != nil {
			builder.SetRewardID(*entry.RewardID)
		}
		if entry.RechargeOrderID != nil {
			builder.SetRechargeOrderID(*entry.RechargeOrderID)
		}
		if entry.WithdrawalID != nil {
			builder.SetWithdrawalID(*entry.WithdrawalID)
		}
		if entry.WithdrawalItemID != nil {
			builder.SetWithdrawalItemID(*entry.WithdrawalItemID)
		}
		if entry.IdempotencyKey != nil {
			builder.SetIdempotencyKey(*entry.IdempotencyKey)
		}
		if entry.OperatorUserID != nil {
			builder.SetOperatorUserID(*entry.OperatorUserID)
		}
		if entry.Remark != nil {
			builder.SetRemark(*entry.Remark)
		}
		if entry.MetadataJSON != nil {
			builder.SetMetadataJSON(*entry.MetadataJSON)
		}

		created, err := builder.Save(ctx)
		if err != nil {
			return err
		}
		entries[i].ID = created.ID
		entries[i].CreatedAt = created.CreatedAt
	}
	return nil
}

func (r *commissionRepository) SumRewardBucketAmount(ctx context.Context, rewardID int64, bucket string) (float64, error) {
	return r.SumRewardBucketAmountForUpdate(ctx, rewardID, bucket, false)
}

func (r *commissionRepository) SumRewardBucketAmountForUpdate(ctx context.Context, rewardID int64, bucket string, forUpdate bool) (float64, error) {
	client := clientFromContext(ctx, r.client)
	if forUpdate {
		// PostgreSQL does not allow FOR UPDATE with aggregate functions.
		// Lock the matching rows first, then sum them in a separate query.
		if _, err := client.CommissionLedger.Query().
			Where(
				commissionledger.RewardIDEQ(rewardID),
				commissionledger.BucketEQ(bucket),
			).
			ForUpdate().
			All(ctx); err != nil {
			return 0, err
		}
	}
	var result []struct {
		Sum *float64 `json:"sum"`
	}
	err := client.CommissionLedger.Query().
		Where(
			commissionledger.RewardIDEQ(rewardID),
			commissionledger.BucketEQ(bucket),
		).
		Aggregate(dbent.Sum(commissionledger.FieldAmount)).Scan(ctx, &result)
	if err != nil {
		return 0, err
	}
	if len(result) == 0 || result[0].Sum == nil {
		return 0, nil
	}
	return *result[0].Sum, nil
}

func (r *commissionRepository) CreateWithdrawal(ctx context.Context, withdrawal *service.CommissionWithdrawal) error {
	client := clientFromContext(ctx, r.client)
	builder := client.CommissionWithdrawal.Create().
		SetUserID(withdrawal.UserID).
		SetWithdrawalNo(withdrawal.WithdrawalNo).
		SetAmount(withdrawal.Amount).
		SetFeeAmount(withdrawal.FeeAmount).
		SetNetAmount(withdrawal.NetAmount).
		SetCurrency(withdrawal.Currency).
		SetStatus(withdrawal.Status).
		SetPayoutMethod(withdrawal.PayoutMethod)

	if withdrawal.PayoutAccountSnapshotJSON != nil {
		builder.SetPayoutAccountSnapshotJSON(*withdrawal.PayoutAccountSnapshotJSON)
	}
	if withdrawal.Remark != nil {
		builder.SetRemark(*withdrawal.Remark)
	}
	if withdrawal.RejectReason != nil {
		builder.SetRejectReason(*withdrawal.RejectReason)
	}
	if withdrawal.ReviewedBy != nil {
		builder.SetReviewedBy(*withdrawal.ReviewedBy)
	}
	if withdrawal.ReviewedAt != nil {
		builder.SetReviewedAt(*withdrawal.ReviewedAt)
	}
	if withdrawal.PaidBy != nil {
		builder.SetPaidBy(*withdrawal.PaidBy)
	}
	if withdrawal.PaidAt != nil {
		builder.SetPaidAt(*withdrawal.PaidAt)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return err
	}
	withdrawal.ID = created.ID
	withdrawal.CreatedAt = created.CreatedAt
	withdrawal.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *commissionRepository) GetWithdrawalByID(ctx context.Context, id int64) (*service.CommissionWithdrawal, error) {
	model, err := clientFromContext(ctx, r.client).CommissionWithdrawal.Get(ctx, id)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrCommissionWithdrawalNotFound, nil)
	}
	return commissionWithdrawalEntityToService(model), nil
}

func (r *commissionRepository) UpdateWithdrawal(ctx context.Context, withdrawal *service.CommissionWithdrawal) error {
	client := clientFromContext(ctx, r.client)
	builder := client.CommissionWithdrawal.UpdateOneID(withdrawal.ID).
		SetStatus(withdrawal.Status).
		SetAmount(withdrawal.Amount).
		SetFeeAmount(withdrawal.FeeAmount).
		SetNetAmount(withdrawal.NetAmount).
		SetCurrency(withdrawal.Currency).
		SetPayoutMethod(withdrawal.PayoutMethod)

	if withdrawal.PayoutAccountSnapshotJSON != nil {
		builder.SetPayoutAccountSnapshotJSON(*withdrawal.PayoutAccountSnapshotJSON)
	}
	if withdrawal.ReviewedBy != nil {
		builder.SetReviewedBy(*withdrawal.ReviewedBy)
	} else {
		builder.ClearReviewedBy()
	}
	if withdrawal.ReviewedAt != nil {
		builder.SetReviewedAt(*withdrawal.ReviewedAt)
	} else {
		builder.ClearReviewedAt()
	}
	if withdrawal.PaidBy != nil {
		builder.SetPaidBy(*withdrawal.PaidBy)
	} else {
		builder.ClearPaidBy()
	}
	if withdrawal.PaidAt != nil {
		builder.SetPaidAt(*withdrawal.PaidAt)
	} else {
		builder.ClearPaidAt()
	}
	if withdrawal.RejectReason != nil {
		builder.SetRejectReason(*withdrawal.RejectReason)
	} else {
		builder.ClearRejectReason()
	}
	if withdrawal.Remark != nil {
		builder.SetRemark(*withdrawal.Remark)
	}

	updated, err := builder.Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrCommissionWithdrawalNotFound, nil)
	}
	withdrawal.UpdatedAt = updated.UpdatedAt
	return nil
}

func (r *commissionRepository) CreateWithdrawalItems(ctx context.Context, items []service.CommissionWithdrawalItem) error {
	client := clientFromContext(ctx, r.client)
	for i := range items {
		item := items[i]
		builder := client.CommissionWithdrawalItem.Create().
			SetWithdrawalID(item.WithdrawalID).
			SetUserID(item.UserID).
			SetRewardID(item.RewardID).
			SetRechargeOrderID(item.RechargeOrderID).
			SetAllocatedAmount(item.AllocatedAmount).
			SetFeeAllocatedAmount(item.FeeAllocatedAmount).
			SetNetAllocatedAmount(item.NetAllocatedAmount).
			SetCurrency(item.Currency).
			SetStatus(item.Status)

		if item.FreezeLedgerID != nil {
			builder.SetFreezeLedgerID(*item.FreezeLedgerID)
		}
		if item.ReturnLedgerID != nil {
			builder.SetReturnLedgerID(*item.ReturnLedgerID)
		}
		if item.PaidLedgerID != nil {
			builder.SetPaidLedgerID(*item.PaidLedgerID)
		}
		if item.ReverseLedgerID != nil {
			builder.SetReverseLedgerID(*item.ReverseLedgerID)
		}

		created, err := builder.Save(ctx)
		if err != nil {
			return err
		}
		items[i].ID = created.ID
		items[i].CreatedAt = created.CreatedAt
		items[i].UpdatedAt = created.UpdatedAt
	}
	return nil
}

func (r *commissionRepository) ListWithdrawalItemsByWithdrawal(ctx context.Context, withdrawalID int64) ([]service.CommissionWithdrawalItem, error) {
	models, err := clientFromContext(ctx, r.client).CommissionWithdrawalItem.Query().
		Where(commissionwithdrawalitem.WithdrawalIDEQ(withdrawalID)).
		Order(dbent.Asc(commissionwithdrawalitem.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]service.CommissionWithdrawalItem, 0, len(models))
	for _, model := range models {
		if item := commissionWithdrawalItemEntityToService(model); item != nil {
			result = append(result, *item)
		}
	}

	// Enrich with reward + order + user info
	rewardIDs := make([]int64, 0, len(result))
	for _, item := range result {
		if item.RewardID > 0 {
			rewardIDs = append(rewardIDs, item.RewardID)
		}
	}
	if len(rewardIDs) == 0 {
		return result, nil
	}

	rewards, err := clientFromContext(ctx, r.client).CommissionReward.Query().
		Where(commissionreward.IDIn(rewardIDs...)).
		All(ctx)
	if err != nil {
		return result, nil // non-fatal: return items without enrichment
	}

	orderIDs := make([]int64, 0, len(rewards))
	sourceUserIDs := make([]int64, 0, len(rewards))
	rewardByID := make(map[int64]*dbent.CommissionReward, len(rewards))
	for _, rw := range rewards {
		rewardByID[rw.ID] = rw
		orderIDs = append(orderIDs, rw.RechargeOrderID)
		sourceUserIDs = append(sourceUserIDs, rw.SourceUserID)
	}

	ordersByID, _ := r.loadOrdersByID(ctx, orderIDs)
	usersByID, _ := r.loadUsersByID(ctx, sourceUserIDs)

	for i := range result {
		rw := rewardByID[result[i].RewardID]
		if rw == nil {
			continue
		}
		result[i].RewardRateSnapshot = rw.RateSnapshot
		if u := usersByID[rw.SourceUserID]; u != nil {
			result[i].SourceUserEmail = u.Email
		}
		if o := ordersByID[rw.RechargeOrderID]; o != nil {
			result[i].ExternalOrderID = o.ExternalOrderID
			result[i].OrderPaidAmount = o.PaidAmount
			result[i].OrderPaidAt = o.PaidAt
		}
	}

	return result, nil
}

func (r *commissionRepository) UpdateWithdrawalItem(ctx context.Context, item *service.CommissionWithdrawalItem) error {
	client := clientFromContext(ctx, r.client)
	builder := client.CommissionWithdrawalItem.UpdateOneID(item.ID).
		SetStatus(item.Status).
		SetAllocatedAmount(item.AllocatedAmount).
		SetFeeAllocatedAmount(item.FeeAllocatedAmount).
		SetNetAllocatedAmount(item.NetAllocatedAmount)
	if item.FreezeLedgerID != nil {
		builder.SetFreezeLedgerID(*item.FreezeLedgerID)
	} else {
		builder.ClearFreezeLedgerID()
	}
	if item.ReturnLedgerID != nil {
		builder.SetReturnLedgerID(*item.ReturnLedgerID)
	} else {
		builder.ClearReturnLedgerID()
	}
	if item.PaidLedgerID != nil {
		builder.SetPaidLedgerID(*item.PaidLedgerID)
	} else {
		builder.ClearPaidLedgerID()
	}
	if item.ReverseLedgerID != nil {
		builder.SetReverseLedgerID(*item.ReverseLedgerID)
	} else {
		builder.ClearReverseLedgerID()
	}
	updated, err := builder.Save(ctx)
	if err != nil {
		return err
	}
	item.UpdatedAt = updated.UpdatedAt
	return nil
}

func (r *commissionRepository) CountWithdrawalsByUserSince(ctx context.Context, userID int64, since time.Time) (int, error) {
	return clientFromContext(ctx, r.client).CommissionWithdrawal.Query().
		Where(
			commissionwithdrawal.UserIDEQ(userID),
			commissionwithdrawal.CreatedAtGTE(since),
		).
		Count(ctx)
}

func (r *commissionRepository) ListPayoutAccountsByUser(ctx context.Context, userID int64) ([]service.CommissionPayoutAccount, error) {
	models, err := clientFromContext(ctx, r.client).CommissionPayoutAccount.Query().
		Where(commissionpayoutaccount.UserIDEQ(userID)).
		Order(dbent.Desc(commissionpayoutaccount.FieldIsDefault), dbent.Desc(commissionpayoutaccount.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]service.CommissionPayoutAccount, 0, len(models))
	for _, model := range models {
		if account := commissionPayoutAccountEntityToService(model); account != nil {
			result = append(result, *account)
		}
	}
	return result, nil
}

func (r *commissionRepository) UpsertPayoutAccount(ctx context.Context, account *service.CommissionPayoutAccount) error {
	client := clientFromContext(ctx, r.client)
	if account.ID > 0 {
		builder := client.CommissionPayoutAccount.UpdateOneID(account.ID).
			SetMethod(account.Method).
			SetAccountName(account.AccountName).
			SetIsDefault(account.IsDefault).
			SetStatus(account.Status)
		if account.AccountNoMasked != nil {
			builder.SetAccountNoMasked(*account.AccountNoMasked)
		}
		if account.AccountNoEncrypted != nil {
			builder.SetAccountNoEncrypted(*account.AccountNoEncrypted)
		}
		if account.BankName != nil {
			builder.SetBankName(*account.BankName)
		}
		if account.QRImageURL != nil {
			builder.SetQrImageURL(*account.QRImageURL)
		}
		updated, err := builder.Save(ctx)
		if err != nil {
			return err
		}
		account.UpdatedAt = updated.UpdatedAt
		return nil
	}

	builder := client.CommissionPayoutAccount.Create().
		SetUserID(account.UserID).
		SetMethod(account.Method).
		SetAccountName(account.AccountName).
		SetIsDefault(account.IsDefault).
		SetStatus(account.Status)
	if account.AccountNoMasked != nil {
		builder.SetAccountNoMasked(*account.AccountNoMasked)
	}
	if account.AccountNoEncrypted != nil {
		builder.SetAccountNoEncrypted(*account.AccountNoEncrypted)
	}
	if account.BankName != nil {
		builder.SetBankName(*account.BankName)
	}
	if account.QRImageURL != nil {
		builder.SetQrImageURL(*account.QRImageURL)
	}
	created, err := builder.Save(ctx)
	if err != nil {
		return err
	}
	account.ID = created.ID
	account.CreatedAt = created.CreatedAt
	account.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *commissionRepository) GetRewardByID(ctx context.Context, rewardID int64) (*service.CommissionReward, error) {
	model, err := clientFromContext(ctx, r.client).CommissionReward.Get(ctx, rewardID)
	if err != nil {
		return nil, err
	}
	return commissionRewardEntityToService(model), nil
}

func (r *commissionRepository) SumUserBucketAmount(ctx context.Context, userID int64, bucket string) (float64, error) {
	return r.SumUserBucketAmountForUpdate(ctx, userID, bucket, false)
}

func (r *commissionRepository) SumUserBucketAmountForUpdate(ctx context.Context, userID int64, bucket string, forUpdate bool) (float64, error) {
	client := clientFromContext(ctx, r.client)
	if forUpdate {
		if _, err := client.CommissionLedger.Query().
			Where(
				commissionledger.UserIDEQ(userID),
				commissionledger.BucketEQ(bucket),
			).
			ForUpdate().
			All(ctx); err != nil {
			return 0, err
		}
	}
	var result []struct {
		Sum *float64 `json:"sum"`
	}
	err := client.CommissionLedger.Query().
		Where(
			commissionledger.UserIDEQ(userID),
			commissionledger.BucketEQ(bucket),
		).
		Aggregate(dbent.Sum(commissionledger.FieldAmount)).Scan(ctx, &result)
	if err != nil {
		return 0, err
	}
	if len(result) == 0 || result[0].Sum == nil {
		return 0, nil
	}
	return *result[0].Sum, nil
}

func (r *commissionRepository) SumAllUserBucketAmounts(ctx context.Context, bucket string) (map[int64]float64, error) {
	client := clientFromContext(ctx, r.client)
	var rows []struct {
		UserID int64    `json:"user_id"`
		Sum    *float64 `json:"sum"`
	}
	err := client.CommissionLedger.Query().
		Where(commissionledger.BucketEQ(bucket)).
		GroupBy(commissionledger.FieldUserID).
		Aggregate(dbent.Sum(commissionledger.FieldAmount)).
		Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	result := make(map[int64]float64, len(rows))
	for _, row := range rows {
		if row.Sum != nil {
			result[row.UserID] = *row.Sum
		}
	}
	return result, nil
}

func (r *commissionRepository) ListLedgerEntriesByUser(ctx context.Context, userID int64, params pagination.PaginationParams) ([]service.CommissionLedger, *pagination.PaginationResult, error) {
	query := clientFromContext(ctx, r.client).CommissionLedger.Query().
		Where(commissionledger.UserIDEQ(userID))
	total, err := query.Count(ctx)
	if err != nil {
		return nil, nil, err
	}
	models, err := query.
		Order(dbent.Desc(commissionledger.FieldCreatedAt)).
		Offset(params.Offset()).
		Limit(params.Limit()).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}
	items := make([]service.CommissionLedger, 0, len(models))
	for _, model := range models {
		if item := commissionLedgerEntityToService(model); item != nil {
			items = append(items, *item)
		}
	}

	// Collect unique reward IDs from ledger entries
	rewardIDs := make([]int64, 0, len(models))
	for _, model := range models {
		if model.RewardID != nil {
			rewardIDs = append(rewardIDs, *model.RewardID)
		}
	}

	// Batch-load commission rewards by ID
	type rewardInfo struct {
		SourceUserID    int64
		RechargeOrderID int64
		RateSnapshot    float64
		Level           int
	}
	rewardsByID := map[int64]*rewardInfo{}
	sourceUserIDs := map[int64]struct{}{}
	orderIDs := map[int64]struct{}{}
	if len(rewardIDs) > 0 {
		rewardModels, rewardErr := clientFromContext(ctx, r.client).CommissionReward.Query().
			Where(commissionreward.IDIn(uniqueInt64(rewardIDs)...)).
			All(ctx)
		if rewardErr != nil {
			return nil, nil, rewardErr
		}
		for _, rw := range rewardModels {
			rewardsByID[rw.ID] = &rewardInfo{
				SourceUserID:    rw.SourceUserID,
				RechargeOrderID: rw.RechargeOrderID,
				RateSnapshot:    rw.RateSnapshot,
				Level:           rw.Level,
			}
			sourceUserIDs[rw.SourceUserID] = struct{}{}
			orderIDs[rw.RechargeOrderID] = struct{}{}
		}
	}

	// Batch-load source users
	srcUserIDSlice := make([]int64, 0, len(sourceUserIDs))
	for id := range sourceUserIDs {
		srcUserIDSlice = append(srcUserIDSlice, id)
	}
	usersByID, err := r.loadUsersByID(ctx, srcUserIDSlice)
	if err != nil {
		return nil, nil, err
	}

	// Batch-load recharge orders
	orderIDSlice := make([]int64, 0, len(orderIDs))
	for id := range orderIDs {
		orderIDSlice = append(orderIDSlice, id)
	}
	ordersByID, err := r.loadOrdersByID(ctx, orderIDSlice)
	if err != nil {
		return nil, nil, err
	}

	// Map display fields back to ledger items
	for i := range items {
		if items[i].RewardID == nil {
			continue
		}
		ri, ok := rewardsByID[*items[i].RewardID]
		if !ok {
			continue
		}
		items[i].RewardRateSnapshot = ri.RateSnapshot
		items[i].RewardLevel = ri.Level
		if srcUser := usersByID[ri.SourceUserID]; srcUser != nil {
			items[i].SourceUserEmail = srcUser.Email
			items[i].SourceUserUsername = srcUser.Username
		}
		if order := ordersByID[ri.RechargeOrderID]; order != nil {
			items[i].ExternalOrderID = order.ExternalOrderID
			items[i].OrderPaidAmount = order.PaidAmount
		}
	}

	return items, paginationResultFrom(total, params), nil
}

func (r *commissionRepository) ListWithdrawalsByUser(ctx context.Context, userID int64, params pagination.PaginationParams) ([]service.CommissionWithdrawal, *pagination.PaginationResult, error) {
	query := clientFromContext(ctx, r.client).CommissionWithdrawal.Query().
		Where(commissionwithdrawal.UserIDEQ(userID))
	total, err := query.Count(ctx)
	if err != nil {
		return nil, nil, err
	}
	models, err := query.
		Order(dbent.Desc(commissionwithdrawal.FieldCreatedAt)).
		Offset(params.Offset()).
		Limit(params.Limit()).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}
	items := make([]service.CommissionWithdrawal, 0, len(models))
	for _, model := range models {
		if item := commissionWithdrawalEntityToService(model); item != nil {
			items = append(items, *item)
		}
	}
	return items, paginationResultFrom(total, params), nil
}

func (r *commissionRepository) ListCommissionRewards(ctx context.Context, params pagination.PaginationParams, filter service.AdminCommissionRewardFilter) ([]service.AdminCommissionReward, *pagination.PaginationResult, error) {
	query := clientFromContext(ctx, r.client).CommissionReward.Query()
	if filter.UserID > 0 {
		query = query.Where(commissionreward.UserIDEQ(filter.UserID))
	}
	if filter.SourceUserID > 0 {
		query = query.Where(commissionreward.SourceUserIDEQ(filter.SourceUserID))
	}
	if strings.TrimSpace(filter.Status) != "" {
		query = query.Where(commissionreward.StatusEQ(strings.TrimSpace(filter.Status)))
	}
	if ids, ok, err := r.searchUserIDs(ctx, filter.Search); err != nil {
		return nil, nil, err
	} else if ok {
		query = query.Where(commissionreward.Or(
			commissionreward.UserIDIn(ids...),
			commissionreward.SourceUserIDIn(ids...),
		))
	} else if strings.TrimSpace(filter.Search) != "" {
		return []service.AdminCommissionReward{}, paginationResultFrom(0, params), nil
	}

	total, err := query.Count(ctx)
	if err != nil {
		return nil, nil, err
	}
	models, err := query.
		Order(dbent.Desc(commissionreward.FieldCreatedAt)).
		Offset(params.Offset()).
		Limit(params.Limit()).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}

	userIDs := make([]int64, 0, len(models)*2)
	orderIDs := make([]int64, 0, len(models))
	for _, model := range models {
		userIDs = append(userIDs, model.UserID, model.SourceUserID)
		orderIDs = append(orderIDs, model.RechargeOrderID)
	}
	usersByID, err := r.loadUsersByID(ctx, userIDs)
	if err != nil {
		return nil, nil, err
	}
	ordersByID, err := r.loadOrdersByID(ctx, orderIDs)
	if err != nil {
		return nil, nil, err
	}

	items := make([]service.AdminCommissionReward, 0, len(models))
	for _, model := range models {
		reward := commissionRewardEntityToService(model)
		if reward == nil {
			continue
		}
		item := service.AdminCommissionReward{CommissionReward: *reward}
		if userModel := usersByID[model.UserID]; userModel != nil {
			item.UserEmail = userModel.Email
			item.Username = userModel.Username
		}
		if sourceModel := usersByID[model.SourceUserID]; sourceModel != nil {
			item.SourceUserEmail = sourceModel.Email
			item.SourceUsername = sourceModel.Username
		}
		if orderModel := ordersByID[model.RechargeOrderID]; orderModel != nil {
			item.ExternalOrderID = &orderModel.ExternalOrderID
		}
		items = append(items, item)
	}
	return items, paginationResultFrom(total, params), nil
}

func (r *commissionRepository) ListCommissionLedgers(ctx context.Context, params pagination.PaginationParams, filter service.AdminCommissionLedgerFilter) ([]service.AdminCommissionLedger, *pagination.PaginationResult, error) {
	query := clientFromContext(ctx, r.client).CommissionLedger.Query()
	if filter.UserID > 0 {
		query = query.Where(commissionledger.UserIDEQ(filter.UserID))
	}
	if strings.TrimSpace(filter.EntryType) != "" {
		query = query.Where(commissionledger.EntryTypeEQ(strings.TrimSpace(filter.EntryType)))
	}
	if strings.TrimSpace(filter.Bucket) != "" {
		query = query.Where(commissionledger.BucketEQ(strings.TrimSpace(filter.Bucket)))
	}
	if ids, ok, err := r.searchUserIDs(ctx, filter.Search); err != nil {
		return nil, nil, err
	} else if ok {
		query = query.Where(commissionledger.UserIDIn(ids...))
	} else if strings.TrimSpace(filter.Search) != "" {
		return []service.AdminCommissionLedger{}, paginationResultFrom(0, params), nil
	}

	total, err := query.Count(ctx)
	if err != nil {
		return nil, nil, err
	}
	models, err := query.
		Order(dbent.Desc(commissionledger.FieldCreatedAt)).
		Offset(params.Offset()).
		Limit(params.Limit()).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}
	userIDs := make([]int64, 0, len(models))
	withdrawalIDs := make([]int64, 0, len(models))
	for _, model := range models {
		userIDs = append(userIDs, model.UserID)
		if model.WithdrawalID != nil {
			withdrawalIDs = append(withdrawalIDs, *model.WithdrawalID)
		}
	}
	usersByID, err := r.loadUsersByID(ctx, userIDs)
	if err != nil {
		return nil, nil, err
	}
	withdrawalsByID, err := r.loadWithdrawalsByID(ctx, withdrawalIDs)
	if err != nil {
		return nil, nil, err
	}

	items := make([]service.AdminCommissionLedger, 0, len(models))
	for _, model := range models {
		entry := commissionLedgerEntityToService(model)
		if entry == nil {
			continue
		}
		item := service.AdminCommissionLedger{CommissionLedger: *entry}
		if userModel := usersByID[model.UserID]; userModel != nil {
			item.UserEmail = userModel.Email
			item.Username = userModel.Username
		}
		if model.WithdrawalID != nil {
			if withdrawalModel := withdrawalsByID[*model.WithdrawalID]; withdrawalModel != nil {
				item.WithdrawalNo = &withdrawalModel.WithdrawalNo
			}
		}
		items = append(items, item)
	}
	return items, paginationResultFrom(total, params), nil
}

func (r *commissionRepository) ListAdminWithdrawals(ctx context.Context, params pagination.PaginationParams, filter service.AdminCommissionWithdrawalFilter) ([]service.AdminCommissionWithdrawal, *pagination.PaginationResult, error) {
	query := clientFromContext(ctx, r.client).CommissionWithdrawal.Query()
	if filter.UserID > 0 {
		query = query.Where(commissionwithdrawal.UserIDEQ(filter.UserID))
	}
	if strings.TrimSpace(filter.Status) != "" {
		query = query.Where(commissionwithdrawal.StatusEQ(strings.TrimSpace(filter.Status)))
	}
	if ids, ok, err := r.searchUserIDs(ctx, filter.Search); err != nil {
		return nil, nil, err
	} else if ok {
		query = query.Where(commissionwithdrawal.UserIDIn(ids...))
	} else if strings.TrimSpace(filter.Search) != "" {
		return []service.AdminCommissionWithdrawal{}, paginationResultFrom(0, params), nil
	}

	total, err := query.Count(ctx)
	if err != nil {
		return nil, nil, err
	}
	models, err := query.
		Order(dbent.Desc(commissionwithdrawal.FieldCreatedAt)).
		Offset(params.Offset()).
		Limit(params.Limit()).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}
	userIDs := make([]int64, 0, len(models))
	withdrawalIDs := make([]int64, 0, len(models))
	for _, model := range models {
		userIDs = append(userIDs, model.UserID)
		withdrawalIDs = append(withdrawalIDs, model.ID)
	}
	usersByID, err := r.loadUsersByID(ctx, userIDs)
	if err != nil {
		return nil, nil, err
	}
	itemCounts, err := r.loadWithdrawalItemCounts(ctx, withdrawalIDs)
	if err != nil {
		return nil, nil, err
	}
	items := make([]service.AdminCommissionWithdrawal, 0, len(models))
	for _, model := range models {
		withdrawal := commissionWithdrawalEntityToService(model)
		if withdrawal == nil {
			continue
		}
		item := service.AdminCommissionWithdrawal{
			CommissionWithdrawal: *withdrawal,
			ItemCount:            itemCounts[model.ID],
		}
		if userModel := usersByID[model.UserID]; userModel != nil {
			item.UserEmail = userModel.Email
			item.Username = userModel.Username
		}
		items = append(items, item)
	}
	return items, paginationResultFrom(total, params), nil
}

func (r *commissionRepository) loadUsersByID(ctx context.Context, ids []int64) (map[int64]*dbent.User, error) {
	result := make(map[int64]*dbent.User, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	models, err := clientFromContext(ctx, r.client).User.Query().
		Where(user.IDIn(uniqueInt64(ids)...)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	for _, model := range models {
		result[model.ID] = model
	}
	return result, nil
}

func (r *commissionRepository) searchUserIDs(ctx context.Context, raw string) ([]int64, bool, error) {
	search := strings.TrimSpace(raw)
	if search == "" {
		return nil, false, nil
	}
	models, err := clientFromContext(ctx, r.client).User.Query().
		Where(user.Or(
			user.EmailContainsFold(search),
			user.UsernameContainsFold(search),
		)).
		All(ctx)
	if err != nil {
		return nil, false, err
	}
	if len(models) == 0 {
		return nil, false, nil
	}
	ids := make([]int64, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ID)
	}
	return ids, true, nil
}

func (r *commissionRepository) loadOrdersByID(ctx context.Context, ids []int64) (map[int64]*dbent.RechargeOrder, error) {
	result := make(map[int64]*dbent.RechargeOrder, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	models, err := clientFromContext(ctx, r.client).RechargeOrder.Query().
		Where(rechargeorder.IDIn(uniqueInt64(ids)...)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	for _, model := range models {
		result[model.ID] = model
	}
	return result, nil
}

func (r *commissionRepository) loadWithdrawalsByID(ctx context.Context, ids []int64) (map[int64]*dbent.CommissionWithdrawal, error) {
	result := make(map[int64]*dbent.CommissionWithdrawal, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	models, err := clientFromContext(ctx, r.client).CommissionWithdrawal.Query().
		Where(commissionwithdrawal.IDIn(uniqueInt64(ids)...)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	for _, model := range models {
		result[model.ID] = model
	}
	return result, nil
}

func (r *commissionRepository) loadWithdrawalItemCounts(ctx context.Context, withdrawalIDs []int64) (map[int64]int, error) {
	result := make(map[int64]int, len(withdrawalIDs))
	if len(withdrawalIDs) == 0 {
		return result, nil
	}
	models, err := clientFromContext(ctx, r.client).CommissionWithdrawalItem.Query().
		Where(commissionwithdrawalitem.WithdrawalIDIn(uniqueInt64(withdrawalIDs)...)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	for _, model := range models {
		result[model.WithdrawalID]++
	}
	return result, nil
}

func (r *commissionRepository) ListRewardsByUserAndSource(ctx context.Context, userID int64, sourceUserID int64) ([]service.UserInviteeReward, error) {
	models, err := clientFromContext(ctx, r.client).CommissionReward.Query().
		Where(
			commissionreward.UserIDEQ(userID),
			commissionreward.SourceUserIDEQ(sourceUserID),
		).
		Order(dbent.Desc(commissionreward.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	// Collect order IDs for batch loading
	orderIDs := make([]int64, 0, len(models))
	for _, model := range models {
		orderIDs = append(orderIDs, model.RechargeOrderID)
	}
	ordersByID, err := r.loadOrdersByID(ctx, orderIDs)
	if err != nil {
		return nil, err
	}

	result := make([]service.UserInviteeReward, 0, len(models))
	for _, model := range models {
		item := service.UserInviteeReward{
			ID:              model.ID,
			RechargeOrderID: model.RechargeOrderID,
			OrderPaidAmount: model.BaseAmountSnapshot,
			RateSnapshot:    model.RateSnapshot,
			RewardAmount:    model.RewardAmount,
			Currency:        model.Currency,
			Status:          model.Status,
			CreatedAt:       model.CreatedAt,
		}
		if order := ordersByID[model.RechargeOrderID]; order != nil {
			item.ExternalOrderID = order.ExternalOrderID
			item.OrderPaidAmount = order.PaidAmount
		}
		result = append(result, item)
	}
	return result, nil
}

func commissionLedgerEntityToService(model *dbent.CommissionLedger) *service.CommissionLedger {
	if model == nil {
		return nil
	}
	return &service.CommissionLedger{
		ID:               model.ID,
		UserID:           model.UserID,
		RewardID:         model.RewardID,
		RechargeOrderID:  model.RechargeOrderID,
		WithdrawalID:     model.WithdrawalID,
		WithdrawalItemID: model.WithdrawalItemID,
		EntryType:        model.EntryType,
		Bucket:           model.Bucket,
		Amount:           model.Amount,
		Currency:         model.Currency,
		IdempotencyKey:   model.IdempotencyKey,
		OperatorUserID:   model.OperatorUserID,
		Remark:           model.Remark,
		MetadataJSON:     model.MetadataJSON,
		CreatedAt:        model.CreatedAt,
	}
}

func commissionRewardEntityToService(model *dbent.CommissionReward) *service.CommissionReward {
	if model == nil {
		return nil
	}
	return &service.CommissionReward{
		ID:                   model.ID,
		UserID:               model.UserID,
		SourceUserID:         model.SourceUserID,
		RechargeOrderID:      model.RechargeOrderID,
		Level:                model.Level,
		RateSnapshot:         model.RateSnapshot,
		BaseAmountSnapshot:   model.BaseAmountSnapshot,
		RewardAmount:         model.RewardAmount,
		Currency:             model.Currency,
		RewardModeSnapshot:   model.RewardModeSnapshot,
		Status:               model.Status,
		AvailableAt:          model.AvailableAt,
		FrozenAt:             model.FrozenAt,
		PaidAt:               model.PaidAt,
		ReversedAt:           model.ReversedAt,
		RuleSnapshotJSON:     model.RuleSnapshotJSON,
		RelationSnapshotJSON: model.RelationSnapshotJSON,
		Notes:                model.Notes,
		CreatedAt:            model.CreatedAt,
		UpdatedAt:            model.UpdatedAt,
	}
}

func commissionWithdrawalEntityToService(model *dbent.CommissionWithdrawal) *service.CommissionWithdrawal {
	if model == nil {
		return nil
	}
	return &service.CommissionWithdrawal{
		ID:                        model.ID,
		UserID:                    model.UserID,
		WithdrawalNo:              model.WithdrawalNo,
		Amount:                    model.Amount,
		FeeAmount:                 model.FeeAmount,
		NetAmount:                 model.NetAmount,
		Currency:                  model.Currency,
		Status:                    model.Status,
		PayoutMethod:              model.PayoutMethod,
		PayoutAccountSnapshotJSON: model.PayoutAccountSnapshotJSON,
		ReviewedBy:                model.ReviewedBy,
		ReviewedAt:                model.ReviewedAt,
		PaidBy:                    model.PaidBy,
		PaidAt:                    model.PaidAt,
		RejectReason:              model.RejectReason,
		Remark:                    model.Remark,
		CreatedAt:                 model.CreatedAt,
		UpdatedAt:                 model.UpdatedAt,
	}
}

func commissionWithdrawalItemEntityToService(model *dbent.CommissionWithdrawalItem) *service.CommissionWithdrawalItem {
	if model == nil {
		return nil
	}
	return &service.CommissionWithdrawalItem{
		ID:                 model.ID,
		WithdrawalID:       model.WithdrawalID,
		UserID:             model.UserID,
		RewardID:           model.RewardID,
		RechargeOrderID:    model.RechargeOrderID,
		AllocatedAmount:    model.AllocatedAmount,
		FeeAllocatedAmount: model.FeeAllocatedAmount,
		NetAllocatedAmount: model.NetAllocatedAmount,
		Currency:           model.Currency,
		Status:             model.Status,
		FreezeLedgerID:     model.FreezeLedgerID,
		ReturnLedgerID:     model.ReturnLedgerID,
		PaidLedgerID:       model.PaidLedgerID,
		ReverseLedgerID:    model.ReverseLedgerID,
		CreatedAt:          model.CreatedAt,
		UpdatedAt:          model.UpdatedAt,
	}
}

func commissionPayoutAccountEntityToService(model *dbent.CommissionPayoutAccount) *service.CommissionPayoutAccount {
	if model == nil {
		return nil
	}
	return &service.CommissionPayoutAccount{
		ID:                 model.ID,
		UserID:             model.UserID,
		Method:             model.Method,
		AccountName:        model.AccountName,
		AccountNoMasked:    model.AccountNoMasked,
		AccountNoEncrypted: model.AccountNoEncrypted,
		BankName:           model.BankName,
		QRImageURL:         model.QrImageURL,
		IsDefault:          model.IsDefault,
		Status:             model.Status,
		CreatedAt:          model.CreatedAt,
		UpdatedAt:          model.UpdatedAt,
	}
}
