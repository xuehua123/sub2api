package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

// --- Dashboard & Analytics ---

func (s *PaymentService) GetDashboardStats(ctx context.Context, days int, dates ...string) (*DashboardStats, error) {
	if days <= 0 {
		days = 30
	}
	now := timezone.Now()
	startDate, endDate := "", ""
	if len(dates) == 2 {
		startDate, endDate = dates[0], dates[1]
	}
	rangeValue, err := ParsePaymentDateRange(startDate, endDate, days, now)
	if err != nil {
		return nil, err
	}
	since, days := rangeValue.Start, rangeValue.Days
	todayStart := timezone.StartOfDay(now)
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	paidStatuses := []string{OrderStatusCompleted, OrderStatusPaid, OrderStatusRecharging}
	if s.dashboardRepository != nil {
		aggregates, err := s.dashboardRepository.GetDashboardAggregates(ctx, rangeValue.Start, rangeValue.End, paidStatuses)
		if err != nil {
			return nil, err
		}
		return buildDashboardStatsFromAggregates(aggregates, rangeValue.Start, rangeValue.Days, todayStart), nil
	}

	orders, err := s.entClient.PaymentOrder.Query().
		Where(
			paymentorder.StatusIn(paidStatuses...),
			paymentorder.PaidAtGTE(since),
			paymentorder.PaidAtLT(rangeValue.End),
		).
		Select(paymentorder.FieldUserID, paymentorder.FieldUserEmail, paymentorder.FieldPayAmount,
			paymentorder.FieldPaidAt, paymentorder.FieldPaymentType, paymentorder.FieldProviderSnapshot).
		All(ctx)
	if err != nil {
		return nil, err
	}

	st := &DashboardStats{}
	computeBasicStats(st, orders, todayStart)

	st.PendingOrders, err = s.entClient.PaymentOrder.Query().
		Where(paymentorder.StatusEQ(OrderStatusPending), paymentorder.CreatedAtGTE(since), paymentorder.CreatedAtLT(rangeValue.End)).
		Count(ctx)
	if err != nil {
		return nil, err
	}

	st.DailySeries = buildDailySeries(orders, since, days)
	st.PaymentMethods = buildMethodDistribution(orders)
	st.TopUsers = buildTopUsers(orders)

	return st, nil
}

func buildDashboardStatsFromAggregates(aggregates *PaymentDashboardAggregates, since time.Time, days int, todayStart time.Time) *DashboardStats {
	st := &DashboardStats{TotalAmount: make(CurrencyAmounts), TodayAmount: make(CurrencyAmounts), AvgAmount: make(CurrencyAmounts), PendingOrders: aggregates.PendingCount}
	counts := make(map[string]int)
	for _, a := range aggregates.Totals {
		st.TotalAmount[a.Currency] = roundAmount(a.Amount)
		counts[a.Currency] = a.Count
		st.TotalCount += a.Count
	}
	for currency, amount := range st.TotalAmount {
		if counts[currency] > 0 {
			st.AvgAmount[currency] = roundAmount(amount / float64(counts[currency]))
		}
	}
	for _, a := range aggregates.Daily {
		if a.Date == todayStart.Format("2006-01-02") {
			st.TodayAmount[a.Currency] += a.Amount
			st.TodayCount += a.Count
		}
	}
	roundCurrencyAmounts(st.TodayAmount)
	st.DailySeries = make([]DailyStats, 0, days)
	daily := make(map[string]*DailyStats)
	for _, a := range aggregates.Daily {
		d := daily[a.Date]
		if d == nil {
			d = &DailyStats{Date: a.Date, Amount: make(CurrencyAmounts)}
			daily[a.Date] = d
		}
		d.Amount[a.Currency] += a.Amount
		d.Count += a.Count
	}
	for i := 0; i < days; i++ {
		date := since.AddDate(0, 0, i).Format("2006-01-02")
		if d := daily[date]; d != nil {
			roundCurrencyAmounts(d.Amount)
			st.DailySeries = append(st.DailySeries, *d)
		} else {
			st.DailySeries = append(st.DailySeries, DailyStats{Date: date, Amount: make(CurrencyAmounts)})
		}
	}
	methods := make(map[string]*PaymentMethodStat)
	for _, a := range aggregates.Methods {
		m := methods[a.PaymentType]
		if m == nil {
			m = &PaymentMethodStat{Type: a.PaymentType, Amount: make(CurrencyAmounts)}
			methods[a.PaymentType] = m
		}
		m.Amount[a.Currency] += a.Amount
		m.Count += a.Count
	}
	for _, m := range methods {
		roundCurrencyAmounts(m.Amount)
		st.PaymentMethods = append(st.PaymentMethods, *m)
	}
	sort.Slice(st.PaymentMethods, func(i, j int) bool { return st.PaymentMethods[i].Type < st.PaymentMethods[j].Type })
	users := make(map[string][]TopUserStat)
	for _, a := range aggregates.Users {
		users[a.Currency] = append(users[a.Currency], TopUserStat{UserID: a.UserID, Email: a.Email, Amount: roundAmount(a.Amount)})
	}
	for currency, list := range users {
		sort.Slice(list, func(i, j int) bool { return list[i].Amount > list[j].Amount })
		if len(list) > topUsersLimit {
			list = list[:topUsersLimit]
		}
		users[currency] = list
	}
	st.TopUsers = users
	return st
}

func computeBasicStats(st *DashboardStats, orders []*dbent.PaymentOrder, todayStart time.Time) {
	st.TotalAmount = make(CurrencyAmounts)
	st.TodayAmount = make(CurrencyAmounts)
	st.AvgAmount = make(CurrencyAmounts)
	currencyCounts := make(map[string]int)
	var todayCount int
	for _, o := range orders {
		currency := PaymentOrderCurrency(o)
		st.TotalAmount[currency] += o.PayAmount
		currencyCounts[currency]++
		if o.PaidAt != nil && !o.PaidAt.Before(todayStart) {
			st.TodayAmount[currency] += o.PayAmount
			todayCount++
		}
	}
	st.TotalCount = len(orders)
	st.TodayCount = todayCount
	for currency, totalAmount := range st.TotalAmount {
		st.AvgAmount[currency] = roundAmount(totalAmount / float64(currencyCounts[currency]))
	}
	roundCurrencyAmounts(st.TotalAmount)
	roundCurrencyAmounts(st.TodayAmount)
}

func buildDailySeries(orders []*dbent.PaymentOrder, since time.Time, days int) []DailyStats {
	dailyMap := make(map[string]*DailyStats)
	for _, o := range orders {
		if o.PaidAt == nil {
			continue
		}
		date := o.PaidAt.In(since.Location()).Format("2006-01-02")
		ds, ok := dailyMap[date]
		if !ok {
			ds = &DailyStats{Date: date, Amount: make(CurrencyAmounts)}
			dailyMap[date] = ds
		}
		ds.Amount[PaymentOrderCurrency(o)] += o.PayAmount
		ds.Count++
	}
	series := make([]DailyStats, 0, days)
	for i := 0; i < days; i++ {
		date := since.AddDate(0, 0, i).Format("2006-01-02")
		if ds, ok := dailyMap[date]; ok {
			roundCurrencyAmounts(ds.Amount)
			series = append(series, *ds)
		} else {
			series = append(series, DailyStats{Date: date, Amount: make(CurrencyAmounts)})
		}
	}
	return series
}

func buildMethodDistribution(orders []*dbent.PaymentOrder) []PaymentMethodStat {
	methodMap := make(map[string]*PaymentMethodStat)
	for _, o := range orders {
		ms, ok := methodMap[o.PaymentType]
		if !ok {
			ms = &PaymentMethodStat{Type: o.PaymentType, Amount: make(CurrencyAmounts)}
			methodMap[o.PaymentType] = ms
		}
		ms.Amount[PaymentOrderCurrency(o)] += o.PayAmount
		ms.Count++
	}
	methods := make([]PaymentMethodStat, 0, len(methodMap))
	for _, ms := range methodMap {
		roundCurrencyAmounts(ms.Amount)
		methods = append(methods, *ms)
	}
	sort.Slice(methods, func(i, j int) bool {
		return methods[i].Type < methods[j].Type
	})
	return methods
}

func buildTopUsers(orders []*dbent.PaymentOrder) TopUsersByCurrency {
	userMap := make(map[string]map[int64]*TopUserStat)
	for _, o := range orders {
		currency := PaymentOrderCurrency(o)
		users, ok := userMap[currency]
		if !ok {
			users = make(map[int64]*TopUserStat)
			userMap[currency] = users
		}
		us, ok := users[o.UserID]
		if !ok {
			us = &TopUserStat{UserID: o.UserID, Email: o.UserEmail}
			users[o.UserID] = us
		}
		us.Amount += o.PayAmount
	}
	result := make(TopUsersByCurrency, len(userMap))
	for currency, users := range userMap {
		userList := make([]*TopUserStat, 0, len(users))
		for _, us := range users {
			us.Amount = roundAmount(us.Amount)
			userList = append(userList, us)
		}
		sort.Slice(userList, func(i, j int) bool {
			return userList[i].Amount > userList[j].Amount
		})
		limit := topUsersLimit
		if len(userList) < limit {
			limit = len(userList)
		}
		result[currency] = make([]TopUserStat, 0, limit)
		for i := 0; i < limit; i++ {
			result[currency] = append(result[currency], *userList[i])
		}
	}
	return result
}

func roundCurrencyAmounts(amounts CurrencyAmounts) {
	for currency, amount := range amounts {
		amounts[currency] = roundAmount(amount)
	}
}

func roundAmount(amount float64) float64 {
	return math.Round(amount*100) / 100
}

// --- Audit Logs ---

func (s *PaymentService) currentClient(ctx context.Context) *dbent.Client {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		return tx.Client()
	}
	return s.entClient
}

func (s *PaymentService) writeAuditLog(ctx context.Context, oid int64, action, op string, detail map[string]any) {
	if err := s.writeAuditLogRequired(ctx, oid, action, op, detail); err != nil {
		slog.Error("audit log failed", "orderID", oid, "action", action, "error", err)
	}
}

func (s *PaymentService) writeAuditLogRequired(ctx context.Context, oid int64, action, op string, detail map[string]any) error {
	dj, _ := json.Marshal(detail)
	return s.currentClient(ctx).PaymentAuditLog.Create().
		SetOrderID(strconv.FormatInt(oid, 10)).
		SetAction(action).
		SetDetail(string(dj)).
		SetOperator(op).
		OnConflictColumns(paymentauditlog.FieldOrderID, paymentauditlog.FieldAction).
		Ignore().
		Exec(ctx)
}

func (s *PaymentService) GetOrderAuditLogs(ctx context.Context, oid int64) ([]*dbent.PaymentAuditLog, error) {
	return s.currentClient(ctx).PaymentAuditLog.Query().Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(oid, 10))).Order(paymentauditlog.ByCreatedAt()).All(ctx)
}
