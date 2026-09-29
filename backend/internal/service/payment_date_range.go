package service

import (
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

// PaymentDateRange uses inclusive calendar dates and an exclusive query end.
type PaymentDateRange struct {
	Start, End time.Time
	Days       int
}

func ParsePaymentDateRange(startDate, endDate string, days int, now time.Time) (PaymentDateRange, error) {
	invalid := func() (PaymentDateRange, error) {
		return PaymentDateRange{}, infraerrors.BadRequest("INVALID_DATE_RANGE", "Select a valid date range of up to 366 days, ending no later than today")
	}
	today := timezone.StartOfDay(now.In(timezone.Location()))
	start, end := today, today
	if startDate != "" || endDate != "" {
		var err error
		start, err = time.ParseInLocation(time.DateOnly, startDate, timezone.Location())
		if err != nil {
			return invalid()
		}
		end, err = time.ParseInLocation(time.DateOnly, endDate, timezone.Location())
		if err != nil {
			return invalid()
		}
	} else {
		if days < 1 || days > 366 {
			return invalid()
		}
		start = today.AddDate(0, 0, 1-days)
	}
	if start.After(end) || end.After(today) || end.After(start.AddDate(0, 0, 365)) {
		return invalid()
	}
	days = 1
	for d := start; d.Before(end); d = d.AddDate(0, 0, 1) {
		days++
	}
	return PaymentDateRange{Start: start, End: end.AddDate(0, 0, 1), Days: days}, nil
}
