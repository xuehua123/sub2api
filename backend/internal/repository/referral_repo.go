package repository

import (
	"context"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/commissionreward"
	"github.com/Wei-Shaw/sub2api/ent/rechargeorder"
	"github.com/Wei-Shaw/sub2api/ent/referralcode"
	"github.com/Wei-Shaw/sub2api/ent/referralrelation"
	"github.com/Wei-Shaw/sub2api/ent/referralrelationhistory"
	"github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type referralRepository struct {
	client *dbent.Client
}

func NewReferralRepository(client *dbent.Client) service.ReferralRepository {
	return &referralRepository{client: client}
}

func (r *referralRepository) GetDefaultCodeByUserID(ctx context.Context, userID int64) (*service.ReferralCode, error) {
	model, err := clientFromContext(ctx, r.client).ReferralCode.Query().
		Where(
			referralcode.UserIDEQ(userID),
			referralcode.IsDefault(true),
		).
		Order(dbent.Asc(referralcode.FieldCreatedAt), dbent.Asc(referralcode.FieldID)).
		First(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrReferralCodeNotFound, nil)
	}
	return referralCodeEntityToService(model), nil
}

func (r *referralRepository) GetCodeByCode(ctx context.Context, code string) (*service.ReferralCode, error) {
	model, err := clientFromContext(ctx, r.client).ReferralCode.Query().
		Where(referralcode.CodeEqualFold(code)).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrReferralCodeNotFound, nil)
	}
	return referralCodeEntityToService(model), nil
}

func (r *referralRepository) CreateCode(ctx context.Context, code *service.ReferralCode) error {
	client := clientFromContext(ctx, r.client)
	created, err := client.ReferralCode.Create().
		SetUserID(code.UserID).
		SetCode(code.Code).
		SetStatus(code.Status).
		SetIsDefault(code.IsDefault).
		Save(ctx)
	if err != nil {
		return translatePersistenceError(err, nil, service.ErrReferralAlreadyBound)
	}
	code.ID = created.ID
	code.CreatedAt = created.CreatedAt
	code.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *referralRepository) GetRelationByUserID(ctx context.Context, userID int64) (*service.ReferralRelation, error) {
	model, err := clientFromContext(ctx, r.client).ReferralRelation.Query().
		Where(referralrelation.UserIDEQ(userID)).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrReferralRelationNotFound, nil)
	}
	return referralRelationEntityToService(model), nil
}

func (r *referralRepository) CreateRelation(ctx context.Context, relation *service.ReferralRelation) error {
	client := clientFromContext(ctx, r.client)
	builder := client.ReferralRelation.Create().
		SetUserID(relation.UserID).
		SetReferrerUserID(relation.ReferrerUserID).
		SetBindSource(relation.BindSource)

	if relation.BindCode != nil {
		builder.SetBindCode(*relation.BindCode)
	}
	if relation.LockedAt != nil {
		builder.SetLockedAt(*relation.LockedAt)
	}
	if relation.Notes != nil {
		builder.SetNotes(*relation.Notes)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return translatePersistenceError(err, nil, service.ErrReferralAlreadyBound)
	}
	relation.ID = created.ID
	relation.CreatedAt = created.CreatedAt
	relation.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *referralRepository) CreateRelationHistory(ctx context.Context, history *service.ReferralRelationHistory) error {
	client := clientFromContext(ctx, r.client)
	builder := client.ReferralRelationHistory.Create().
		SetUserID(history.UserID).
		SetChangeSource(history.ChangeSource)

	if history.OldReferrerUserID != nil {
		builder.SetOldReferrerUserID(*history.OldReferrerUserID)
	}
	if history.NewReferrerUserID != nil {
		builder.SetNewReferrerUserID(*history.NewReferrerUserID)
	}
	if history.OldBindCode != nil {
		builder.SetOldBindCode(*history.OldBindCode)
	}
	if history.NewBindCode != nil {
		builder.SetNewBindCode(*history.NewBindCode)
	}
	if history.ChangedBy != nil {
		builder.SetChangedBy(*history.ChangedBy)
	}
	if history.Reason != nil {
		builder.SetReason(*history.Reason)
	}
	if history.MetadataJSON != nil {
		builder.SetMetadataJSON(*history.MetadataJSON)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return err
	}
	history.ID = created.ID
	history.CreatedAt = created.CreatedAt
	return nil
}

func (r *referralRepository) UpsertRelation(ctx context.Context, relation *service.ReferralRelation) error {
	client := clientFromContext(ctx, r.client)
	existing, err := client.ReferralRelation.Query().
		Where(referralrelation.UserIDEQ(relation.UserID)).
		Only(ctx)
	if err != nil && !dbent.IsNotFound(err) {
		return err
	}
	if err == nil {
		builder := client.ReferralRelation.UpdateOneID(existing.ID).
			SetReferrerUserID(relation.ReferrerUserID).
			SetBindSource(relation.BindSource)
		if relation.BindCode != nil {
			builder.SetBindCode(*relation.BindCode)
		} else {
			builder.ClearBindCode()
		}
		if relation.LockedAt != nil {
			builder.SetLockedAt(*relation.LockedAt)
		} else {
			builder.ClearLockedAt()
		}
		if relation.Notes != nil {
			builder.SetNotes(*relation.Notes)
		} else {
			builder.ClearNotes()
		}
		updated, saveErr := builder.Save(ctx)
		if saveErr != nil {
			return saveErr
		}
		relation.ID = updated.ID
		relation.CreatedAt = updated.CreatedAt
		relation.UpdatedAt = updated.UpdatedAt
		return nil
	}
	return r.CreateRelation(ctx, relation)
}

func (r *referralRepository) HasPaidRecharge(ctx context.Context, userID int64) (bool, error) {
	count, err := clientFromContext(ctx, r.client).RechargeOrder.Query().
		Where(
			rechargeorder.UserIDEQ(userID),
			rechargeorder.StatusIn("paid", "credited", "refund_pending", "partially_refunded", "refunded", "chargeback"),
		).
		Count(ctx)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *referralRepository) CountInvitees(ctx context.Context, userID int64) (*service.ReferralInviteeCounts, error) {
	client := clientFromContext(ctx, r.client)
	directRelations, err := client.ReferralRelation.Query().
		Where(referralrelation.ReferrerUserIDEQ(userID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	result := &service.ReferralInviteeCounts{DirectInvitees: len(directRelations)}
	if len(directRelations) == 0 {
		return result, nil
	}
	directIDs := make([]int64, 0, len(directRelations))
	for _, relation := range directRelations {
		directIDs = append(directIDs, relation.UserID)
	}
	secondLevelCount, err := client.ReferralRelation.Query().
		Where(referralrelation.ReferrerUserIDIn(directIDs...)).
		Count(ctx)
	if err != nil {
		return nil, err
	}
	result.SecondLevelInvitees = secondLevelCount
	return result, nil
}

func (r *referralRepository) CountAllInvitees(ctx context.Context) (map[int64]*service.ReferralInviteeCounts, error) {
	client := clientFromContext(ctx, r.client)
	allRelations, err := client.ReferralRelation.Query().All(ctx)
	if err != nil {
		return nil, err
	}
	result := make(map[int64]*service.ReferralInviteeCounts)
	directUserIDs := make(map[int64]int64) // userID -> referrerUserID
	for _, relation := range allRelations {
		if _, ok := result[relation.ReferrerUserID]; !ok {
			result[relation.ReferrerUserID] = &service.ReferralInviteeCounts{}
		}
		result[relation.ReferrerUserID].DirectInvitees++
		directUserIDs[relation.UserID] = relation.ReferrerUserID
	}
	// Count second-level: for each relation, if the user is also a referrer, their referrer gets second-level count
	for _, relation := range allRelations {
		if grandparentID, ok := directUserIDs[relation.ReferrerUserID]; ok {
			if _, exists := result[grandparentID]; !exists {
				result[grandparentID] = &service.ReferralInviteeCounts{}
			}
			result[grandparentID].SecondLevelInvitees++
		}
	}
	return result, nil
}

func (r *referralRepository) ListInvitees(ctx context.Context, userID int64, params pagination.PaginationParams) ([]service.ReferralInvitee, *pagination.PaginationResult, error) {
	client := clientFromContext(ctx, r.client)
	query := client.ReferralRelation.Query().
		Where(referralrelation.ReferrerUserIDEQ(userID))
	total, err := query.Count(ctx)
	if err != nil {
		return nil, nil, err
	}
	models, err := query.
		Order(dbent.Desc(referralrelation.FieldCreatedAt)).
		Offset(params.Offset()).
		Limit(params.Limit()).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}
	userIDs := make([]int64, 0, len(models))
	for _, model := range models {
		userIDs = append(userIDs, model.UserID)
	}
	usersByID, err := r.loadUsersByID(ctx, userIDs)
	if err != nil {
		return nil, nil, err
	}
	secondLevelCounts := map[int64]int{}
	if len(userIDs) > 0 {
		secondLevelRelations, countErr := client.ReferralRelation.Query().
			Where(referralrelation.ReferrerUserIDIn(userIDs...)).
			All(ctx)
		if countErr != nil {
			return nil, nil, countErr
		}
		for _, relation := range secondLevelRelations {
			secondLevelCounts[relation.ReferrerUserID]++
		}
	}

	// Aggregate recharge data per invitee using grouped queries
	rechargeTotal := map[int64]float64{}
	rechargeCount := map[int64]int{}
	rechargeLatest := map[int64]*time.Time{}
	if len(userIDs) > 0 {
		paidStatuses := []string{"paid", "credited", "refund_pending", "partially_refunded", "refunded", "chargeback"}
		var rechargeAgg []struct {
			UserID    int64      `json:"user_id"`
			Sum       *float64   `json:"sum"`
			Count     int        `json:"count"`
			MaxPaidAt *time.Time `json:"max"`
		}
		err := client.RechargeOrder.Query().
			Where(
				rechargeorder.UserIDIn(userIDs...),
				rechargeorder.StatusIn(paidStatuses...),
			).
			GroupBy(rechargeorder.FieldUserID).
			Aggregate(
				dbent.Sum(rechargeorder.FieldPaidAmount),
				dbent.Count(),
				dbent.Max(rechargeorder.FieldPaidAt),
			).
			Scan(ctx, &rechargeAgg)
		if err != nil {
			return nil, nil, err
		}
		for _, row := range rechargeAgg {
			if row.Sum != nil {
				rechargeTotal[row.UserID] = *row.Sum
			}
			rechargeCount[row.UserID] = row.Count
			rechargeLatest[row.UserID] = row.MaxPaidAt
		}
	}

	// Aggregate commission rewards per source user using grouped query
	commissionTotal := map[int64]float64{}
	if len(userIDs) > 0 {
		var commissionAgg []struct {
			SourceUserID int64    `json:"source_user_id"`
			Sum          *float64 `json:"sum"`
		}
		err := client.CommissionReward.Query().
			Where(commissionreward.SourceUserIDIn(userIDs...)).
			GroupBy(commissionreward.FieldSourceUserID).
			Aggregate(dbent.Sum(commissionreward.FieldRewardAmount)).
			Scan(ctx, &commissionAgg)
		if err != nil {
			return nil, nil, err
		}
		for _, row := range commissionAgg {
			if row.Sum != nil {
				commissionTotal[row.SourceUserID] = *row.Sum
			}
		}
	}

	items := make([]service.ReferralInvitee, 0, len(models))
	for _, model := range models {
		userModel := usersByID[model.UserID]
		item := service.ReferralInvitee{
			UserID:          model.UserID,
			BoundAt:         model.CreatedAt,
			ReferralCode:    model.BindCode,
			Source:          &model.BindSource,
			SecondLevelNum:  secondLevelCounts[model.UserID],
			TotalRecharge:   rechargeTotal[model.UserID],
			LatestPaidAt:    rechargeLatest[model.UserID],
			TotalCommission: commissionTotal[model.UserID],
			OrderCount:      rechargeCount[model.UserID],
		}
		if userModel != nil {
			item.Email = userModel.Email
			item.Username = userModel.Username
		}
		items = append(items, item)
	}
	return items, paginationResultFrom(total, params), nil
}

func (r *referralRepository) ListRelations(ctx context.Context, params pagination.PaginationParams, search string) ([]service.AdminReferralRelation, *pagination.PaginationResult, error) {
	client := clientFromContext(ctx, r.client)
	query := client.ReferralRelation.Query()
	if ids, ok, err := r.searchUserIDs(ctx, search); err != nil {
		return nil, nil, err
	} else if ok {
		query = query.Where(referralrelation.Or(
			referralrelation.UserIDIn(ids...),
			referralrelation.ReferrerUserIDIn(ids...),
		))
	} else if search != "" {
		return []service.AdminReferralRelation{}, paginationResultFrom(0, params), nil
	}

	total, err := query.Count(ctx)
	if err != nil {
		return nil, nil, err
	}
	models, err := query.
		Order(dbent.Desc(referralrelation.FieldUpdatedAt)).
		Offset(params.Offset()).
		Limit(params.Limit()).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}

	userIDs := make([]int64, 0, len(models)*2)
	for _, model := range models {
		userIDs = append(userIDs, model.UserID, model.ReferrerUserID)
	}
	usersByID, err := r.loadUsersByID(ctx, userIDs)
	if err != nil {
		return nil, nil, err
	}

	items := make([]service.AdminReferralRelation, 0, len(models))
	for _, model := range models {
		item := service.AdminReferralRelation{
			UserID:     model.UserID,
			BindSource: model.BindSource,
			BindCode:   model.BindCode,
			LockedAt:   model.LockedAt,
			CreatedAt:  model.CreatedAt,
			UpdatedAt:  model.UpdatedAt,
		}
		if userModel := usersByID[model.UserID]; userModel != nil {
			item.UserEmail = userModel.Email
			item.Username = userModel.Username
		}
		if referrerModel := usersByID[model.ReferrerUserID]; referrerModel != nil {
			item.ReferrerUserID = &referrerModel.ID
			item.ReferrerEmail = &referrerModel.Email
			item.ReferrerUsername = &referrerModel.Username
		}
		items = append(items, item)
	}
	return items, paginationResultFrom(total, params), nil
}

func (r *referralRepository) GetAdminRelationByUserID(ctx context.Context, userID int64) (*service.AdminReferralRelation, error) {
	model, err := clientFromContext(ctx, r.client).ReferralRelation.Query().
		Where(referralrelation.UserIDEQ(userID)).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrReferralRelationNotFound, nil)
	}
	usersByID, err := r.loadUsersByID(ctx, []int64{model.UserID, model.ReferrerUserID})
	if err != nil {
		return nil, err
	}

	item := &service.AdminReferralRelation{
		UserID:     model.UserID,
		BindSource: model.BindSource,
		BindCode:   model.BindCode,
		LockedAt:   model.LockedAt,
		CreatedAt:  model.CreatedAt,
		UpdatedAt:  model.UpdatedAt,
	}
	if userModel := usersByID[model.UserID]; userModel != nil {
		item.UserEmail = userModel.Email
		item.Username = userModel.Username
	}
	if referrerModel := usersByID[model.ReferrerUserID]; referrerModel != nil {
		item.ReferrerUserID = &referrerModel.ID
		item.ReferrerEmail = &referrerModel.Email
		item.ReferrerUsername = &referrerModel.Username
	}
	return item, nil
}

func (r *referralRepository) ListRelationHistories(ctx context.Context, params pagination.PaginationParams, userID int64) ([]service.ReferralRelationHistory, *pagination.PaginationResult, error) {
	client := clientFromContext(ctx, r.client)
	query := client.ReferralRelationHistory.Query()
	if userID > 0 {
		query = query.Where(referralrelationhistory.UserIDEQ(userID))
	}
	total, err := query.Count(ctx)
	if err != nil {
		return nil, nil, err
	}
	models, err := query.
		Order(dbent.Desc(referralrelationhistory.FieldCreatedAt)).
		Offset(params.Offset()).
		Limit(params.Limit()).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}
	items := make([]service.ReferralRelationHistory, 0, len(models))
	for _, model := range models {
		if item := referralRelationHistoryEntityToService(model); item != nil {
			items = append(items, *item)
		}
	}
	return items, paginationResultFrom(total, params), nil
}

func referralCodeEntityToService(model *dbent.ReferralCode) *service.ReferralCode {
	if model == nil {
		return nil
	}
	return &service.ReferralCode{
		ID:        model.ID,
		UserID:    model.UserID,
		Code:      model.Code,
		Status:    model.Status,
		IsDefault: model.IsDefault,
		CreatedAt: model.CreatedAt,
		UpdatedAt: model.UpdatedAt,
	}
}

func referralRelationEntityToService(model *dbent.ReferralRelation) *service.ReferralRelation {
	if model == nil {
		return nil
	}
	return &service.ReferralRelation{
		ID:             model.ID,
		UserID:         model.UserID,
		ReferrerUserID: model.ReferrerUserID,
		BindSource:     model.BindSource,
		BindCode:       model.BindCode,
		LockedAt:       model.LockedAt,
		Notes:          model.Notes,
		CreatedAt:      model.CreatedAt,
		UpdatedAt:      model.UpdatedAt,
	}
}

func referralRelationHistoryEntityToService(model *dbent.ReferralRelationHistory) *service.ReferralRelationHistory {
	if model == nil {
		return nil
	}
	return &service.ReferralRelationHistory{
		ID:                model.ID,
		UserID:            model.UserID,
		OldReferrerUserID: model.OldReferrerUserID,
		NewReferrerUserID: model.NewReferrerUserID,
		OldBindCode:       model.OldBindCode,
		NewBindCode:       model.NewBindCode,
		ChangeSource:      model.ChangeSource,
		ChangedBy:         model.ChangedBy,
		Reason:            model.Reason,
		MetadataJSON:      model.MetadataJSON,
		CreatedAt:         model.CreatedAt,
	}
}

func (r *referralRepository) loadUsersByID(ctx context.Context, ids []int64) (map[int64]*dbent.User, error) {
	result := make(map[int64]*dbent.User, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	unique := uniqueInt64(ids)
	models, err := clientFromContext(ctx, r.client).User.Query().
		Where(user.IDIn(unique...)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	for _, model := range models {
		result[model.ID] = model
	}
	return result, nil
}

func (r *referralRepository) searchUserIDs(ctx context.Context, raw string) ([]int64, bool, error) {
	search := strings.TrimSpace(raw)
	if search == "" {
		return nil, false, nil
	}
	users, err := clientFromContext(ctx, r.client).User.Query().
		Where(user.Or(
			user.EmailContainsFold(search),
			user.UsernameContainsFold(search),
		)).
		All(ctx)
	if err != nil {
		return nil, false, err
	}
	if len(users) == 0 {
		return nil, false, nil
	}
	ids := make([]int64, 0, len(users))
	for _, userModel := range users {
		ids = append(ids, userModel.ID)
	}
	return ids, true, nil
}
