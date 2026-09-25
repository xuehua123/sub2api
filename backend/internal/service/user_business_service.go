package service

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

type UserBusinessParams struct {
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	Search    string `json:"search"`
	Filter    string `json:"filter"`
	Sort      string `json:"sort"`
	Order     string `json:"order"`
	Page      int    `json:"page"`
	PageSize  int    `json:"page_size"`
}
type UserBusinessQuery struct {
	UserBusinessParams
	Start, End, Now time.Time
	UserID          int64
}
type UserBusinessRepository interface {
	Report(context.Context, UserBusinessQuery) (json.RawMessage, error)
	Detail(context.Context, UserBusinessQuery) (json.RawMessage, error)
}
type UserBusinessService struct {
	repo UserBusinessRepository
	now  func() time.Time
}

func NewUserBusinessService(repo UserBusinessRepository) *UserBusinessService {
	return &UserBusinessService{repo: repo, now: time.Now}
}
func (s *UserBusinessService) Normalize(ctx context.Context, p UserBusinessParams) (UserBusinessQuery, error) {
	now := s.now().In(timezone.Location())
	if p.StartDate == "" && p.EndDate == "" {
		p.StartDate = now.Format("2006-01-02")
		p.EndDate = p.StartDate
	}
	start, e := time.ParseInLocation("2006-01-02", p.StartDate, timezone.Location())
	if e != nil {
		return UserBusinessQuery{}, infraerrors.BadRequest("INVALID_RANGE", "Invalid start date")
	}
	last, e := time.ParseInLocation("2006-01-02", p.EndDate, timezone.Location())
	if e != nil || last.Before(start) || last.After(timezone.StartOfDay(now)) || last.AddDate(0, 0, 1).After(start.AddDate(1, 0, 0)) {
		return UserBusinessQuery{}, infraerrors.BadRequest("INVALID_RANGE", "Select up to one year, ending no later than today")
	}
	if p.Page < 1 {
		p.Page = 1
	}
	if p.Page > 1000000 {
		return UserBusinessQuery{}, infraerrors.BadRequest("INVALID_PAGE", "Page too large")
	}
	if p.PageSize < 1 {
		p.PageSize = 20
	}
	if p.PageSize > 100 {
		p.PageSize = 100
	}
	p.Search = strings.TrimSpace(p.Search)
	if len(p.Search) > 200 {
		return UserBusinessQuery{}, infraerrors.BadRequest("INVALID_SEARCH", "Search too long")
	}
	switch p.Filter {
	case "", "used", "paid", "loss", "active":
	default:
		return UserBusinessQuery{}, infraerrors.BadRequest("INVALID_FILTER", "Unknown filter")
	}
	if p.Sort == "" {
		p.Sort = "consumption"
	}
	switch p.Sort {
	case "consumption", "paid", "cost", "profit", "balance":
	default:
		return UserBusinessQuery{}, infraerrors.BadRequest("INVALID_SORT", "Unknown sort")
	}
	if p.Order == "" {
		p.Order = "desc"
	}
	if p.Order != "asc" && p.Order != "desc" {
		return UserBusinessQuery{}, infraerrors.BadRequest("INVALID_SORT", "Unknown direction")
	}
	end := last.AddDate(0, 0, 1)
	if end.After(now) {
		end = now
	}
	return UserBusinessQuery{UserBusinessParams: p, Start: start, End: end, Now: now}, nil
}
func (s *UserBusinessService) Report(ctx context.Context, q UserBusinessQuery) (json.RawMessage, error) {
	return s.repo.Report(ctx, q)
}
func (s *UserBusinessService) Detail(ctx context.Context, q UserBusinessQuery, id int64) (json.RawMessage, error) {
	if id <= 0 {
		return nil, infraerrors.BadRequest("INVALID_USER", "Invalid user")
	}
	q.UserID = id
	return s.repo.Detail(ctx, q)
}
func (q UserBusinessQuery) CacheKey() string {
	b, _ := json.Marshal(q.UserBusinessParams)
	return string(b) + "|" + strconv.FormatInt(q.UserID, 10)
}
