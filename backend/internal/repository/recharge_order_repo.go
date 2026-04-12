package repository

import (
	"context"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/rechargeorder"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type rechargeOrderRepository struct {
	client *dbent.Client
}

func NewRechargeOrderRepository(client *dbent.Client) service.RechargeOrderRepository {
	return &rechargeOrderRepository{client: client}
}

func (r *rechargeOrderRepository) GetByProviderAndExternalOrderID(ctx context.Context, provider, externalOrderID string) (*service.RechargeOrder, error) {
	model, err := clientFromContext(ctx, r.client).RechargeOrder.Query().
		Where(
			rechargeorder.ProviderEQ(provider),
			rechargeorder.ExternalOrderIDEQ(externalOrderID),
		).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrRechargeOrderNotFound, nil)
	}
	return rechargeOrderEntityToService(model), nil
}

func (r *rechargeOrderRepository) GetByID(ctx context.Context, id int64) (*service.RechargeOrder, error) {
	model, err := clientFromContext(ctx, r.client).RechargeOrder.Get(ctx, id)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrRechargeOrderNotFound, nil)
	}
	return rechargeOrderEntityToService(model), nil
}

func (r *rechargeOrderRepository) Create(ctx context.Context, order *service.RechargeOrder) error {
	client := clientFromContext(ctx, r.client)
	builder := client.RechargeOrder.Create().
		SetUserID(order.UserID).
		SetExternalOrderID(order.ExternalOrderID).
		SetProvider(order.Provider).
		SetCurrency(order.Currency).
		SetGrossAmount(order.GrossAmount).
		SetDiscountAmount(order.DiscountAmount).
		SetPaidAmount(order.PaidAmount).
		SetGiftBalanceAmount(order.GiftBalanceAmount).
		SetCreditedBalanceAmount(order.CreditedBalanceAmount).
		SetRefundedAmount(order.RefundedAmount).
		SetChargebackAmount(order.ChargebackAmount).
		SetStatus(order.Status)

	if order.Channel != nil {
		builder.SetChannel(*order.Channel)
	}
	if order.PaidAt != nil {
		builder.SetPaidAt(*order.PaidAt)
	}
	if order.CreditedAt != nil {
		builder.SetCreditedAt(*order.CreditedAt)
	}
	if order.RefundedAt != nil {
		builder.SetRefundedAt(*order.RefundedAt)
	}
	if order.ChargebackAt != nil {
		builder.SetChargebackAt(*order.ChargebackAt)
	}
	if order.IdempotencyKey != nil {
		builder.SetIdempotencyKey(*order.IdempotencyKey)
	}
	if order.MetadataJSON != nil {
		builder.SetMetadataJSON(*order.MetadataJSON)
	}
	if order.Notes != nil {
		builder.SetNotes(*order.Notes)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return translatePersistenceError(err, nil, service.ErrRechargeOrderConflict)
	}
	order.ID = created.ID
	order.CreatedAt = created.CreatedAt
	order.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *rechargeOrderRepository) Update(ctx context.Context, order *service.RechargeOrder) error {
	client := clientFromContext(ctx, r.client)
	builder := client.RechargeOrder.UpdateOneID(order.ID).
		SetStatus(order.Status).
		SetRefundedAmount(order.RefundedAmount).
		SetChargebackAmount(order.ChargebackAmount)

	if order.Channel != nil {
		builder.SetChannel(*order.Channel)
	}
	if order.PaidAt != nil {
		builder.SetPaidAt(*order.PaidAt)
	} else {
		builder.ClearPaidAt()
	}
	if order.CreditedAt != nil {
		builder.SetCreditedAt(*order.CreditedAt)
	} else {
		builder.ClearCreditedAt()
	}
	if order.RefundedAt != nil {
		builder.SetRefundedAt(*order.RefundedAt)
	} else {
		builder.ClearRefundedAt()
	}
	if order.ChargebackAt != nil {
		builder.SetChargebackAt(*order.ChargebackAt)
	} else {
		builder.ClearChargebackAt()
	}
	if order.MetadataJSON != nil {
		builder.SetMetadataJSON(*order.MetadataJSON)
	}
	if order.Notes != nil {
		builder.SetNotes(*order.Notes)
	}

	updated, err := builder.Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrRechargeOrderNotFound, nil)
	}
	order.UpdatedAt = updated.UpdatedAt
	return nil
}

func (r *rechargeOrderRepository) CountPaidOrdersByUser(ctx context.Context, userID int64) (int, error) {
	return clientFromContext(ctx, r.client).RechargeOrder.Query().
		Where(
			rechargeorder.UserIDEQ(userID),
			rechargeorder.StatusIn(
				service.RechargeOrderStatusPaid,
				service.RechargeOrderStatusCredited,
				service.RechargeOrderStatusRefundPending,
				service.RechargeOrderStatusPartiallyRefunded,
				service.RechargeOrderStatusRefunded,
				service.RechargeOrderStatusChargeback,
			),
		).
		Count(ctx)
}

func (r *rechargeOrderRepository) HasRefundOrChargeback(ctx context.Context, rechargeOrderID int64) (bool, error) {
	count, err := clientFromContext(ctx, r.client).RechargeOrder.Query().
		Where(
			rechargeorder.IDEQ(rechargeOrderID),
			rechargeorder.Or(
				rechargeorder.RefundedAmountGT(0),
				rechargeorder.ChargebackAmountGT(0),
				rechargeorder.StatusIn(
					service.RechargeOrderStatusRefundPending,
					service.RechargeOrderStatusPartiallyRefunded,
					service.RechargeOrderStatusRefunded,
					service.RechargeOrderStatusChargeback,
				),
			),
		).
		Count(ctx)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func rechargeOrderEntityToService(model *dbent.RechargeOrder) *service.RechargeOrder {
	if model == nil {
		return nil
	}
	return &service.RechargeOrder{
		ID:                    model.ID,
		UserID:                model.UserID,
		ExternalOrderID:       model.ExternalOrderID,
		Provider:              model.Provider,
		Channel:               model.Channel,
		Currency:              model.Currency,
		GrossAmount:           model.GrossAmount,
		DiscountAmount:        model.DiscountAmount,
		PaidAmount:            model.PaidAmount,
		GiftBalanceAmount:     model.GiftBalanceAmount,
		CreditedBalanceAmount: model.CreditedBalanceAmount,
		RefundedAmount:        model.RefundedAmount,
		ChargebackAmount:      model.ChargebackAmount,
		Status:                model.Status,
		PaidAt:                model.PaidAt,
		CreditedAt:            model.CreditedAt,
		RefundedAt:            model.RefundedAt,
		ChargebackAt:          model.ChargebackAt,
		IdempotencyKey:        model.IdempotencyKey,
		MetadataJSON:          model.MetadataJSON,
		Notes:                 model.Notes,
		CreatedAt:             model.CreatedAt,
		UpdatedAt:             model.UpdatedAt,
	}
}
