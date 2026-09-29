package service

import (
	"context"
	"time"
)

// PaymentDashboardRepository returns already aggregated payment facts. Keeping
// aggregation in the repository prevents long date ranges from loading every
// order row into the application process.
type PaymentDashboardRepository interface {
	GetDashboardAggregates(ctx context.Context, start, end time.Time, paidStatuses []string) (*PaymentDashboardAggregates, error)
}

type PaymentDashboardAggregates struct {
	Totals       []PaymentDashboardAggregate
	Daily        []PaymentDashboardAggregate
	Methods      []PaymentDashboardAggregate
	Users        []PaymentDashboardAggregate
	PendingCount int
}

type PaymentDashboardAggregate struct {
	Currency    string
	Date        string
	PaymentType string
	UserID      int64
	Email       string
	Amount      float64
	Count       int
}
