package service

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

type UpstreamGroupCatalogParams struct {
	Page, PageSize                                  int
	Search, Provider, Tag, Binding, Freshness, Sort string
	ConnectionIDs                                   []int64
	MinRate, MaxRate                                *float64
	Favorites, GroupByConnection                    bool
}
type UpstreamGroupCatalogItem struct {
	ConnectionID      int64      `json:"connection_id"`
	ConnectionName    string     `json:"connection_name"`
	ManagementBaseURL string     `json:"management_base_url"`
	Provider          string     `json:"provider"`
	RemoteKey         string     `json:"remote_key"`
	RemoteID          string     `json:"remote_id"`
	Name              string     `json:"name"`
	RateMultiplier    *float64   `json:"rate_multiplier"`
	Source            string     `json:"source"`
	Confidence        string     `json:"confidence"`
	ObservedAt        *time.Time `json:"observed_at"`
	FreshUntil        *time.Time `json:"fresh_until"`
	Tags              []string   `json:"tags"`
	Favorite          bool       `json:"favorite"`
	AccountIDs        []int64    `json:"account_ids"`
	BindingCount      int        `json:"binding_count"`
	Freshness         string     `json:"freshness"`
	AutoTags          []string   `json:"auto_tags"`
	ExcludedAutoTags  []string   `json:"excluded_auto_tags"`
	ModelCount        int        `json:"model_count"`
	ModelPreview      []string   `json:"model_preview"`
	ModelStatus       string     `json:"model_status"`
	ModelCoverage     string     `json:"model_coverage"`
	ModelsObservedAt  *time.Time `json:"models_observed_at"`
}
type UpstreamGroupCatalogResult struct {
	Items    []UpstreamGroupCatalogItem `json:"items"`
	Total    int64                      `json:"total"`
	Page     int                        `json:"page"`
	PageSize int                        `json:"page_size"`
	Tags     []string                   `json:"tags"`
}
type UpstreamGroupReference struct {
	ConnectionID int64  `json:"connection_id"`
	RemoteKey    string `json:"remote_key"`
}
type UpstreamGroupAnnotationUpdate struct {
	Groups        []UpstreamGroupReference `json:"groups"`
	AddTags       []string                 `json:"add_tags"`
	RemoveTags    []string                 `json:"remove_tags"`
	Favorite      *bool                    `json:"favorite"`
	ResetAutoTags bool                     `json:"reset_auto_tags"`
}
type UpstreamConnectionCostItem struct {
	ConnectionID int64   `json:"connection_id"`
	Name         string  `json:"name"`
	Requests     int64   `json:"requests"`
	AccountCost  float64 `json:"account_cost"`
}

// A nonempty Day is a cross-connection daily total (ConnectionID=0).
// An empty Day is one connection's interval total, including zero-usage connections.
type UpstreamConnectionDailyCost struct {
	UpstreamConnectionCostItem
	Day string
}
type UpstreamCostDay struct {
	Date        string  `json:"date"`
	AccountCost float64 `json:"account_cost"`
	Requests    int64   `json:"requests"`
}
type UpstreamConnectionCostHistory struct {
	Daily         []UpstreamCostDay            `json:"daily"`
	Items         []UpstreamConnectionCostItem `json:"items"`
	StartAt       time.Time                    `json:"start_at"`
	EndAt         time.Time                    `json:"end_at"`
	Timezone      string                       `json:"timezone"`
	TotalCost     float64                      `json:"total_cost"`
	TotalRequests int64                        `json:"total_requests"`
}

// Separate read-model capability keeps management and synchronization contracts unchanged.
type UpstreamConnectionCatalogRepository interface {
	ListGroupCatalog(context.Context, UpstreamGroupCatalogParams, time.Time) (*UpstreamGroupCatalogResult, error)
	UpdateGroupAnnotations(context.Context, UpstreamGroupAnnotationUpdate) error
	GetConnectionDailyCosts(context.Context, []int64, time.Time, time.Time, string) ([]UpstreamConnectionDailyCost, error)
}

func (s *UpstreamConnectionService) catalogRepository() (UpstreamConnectionCatalogRepository, error) {
	r, ok := s.repo.(UpstreamConnectionCatalogRepository)
	if !ok {
		return nil, errors.New("upstream catalog repository unavailable")
	}
	return r, nil
}
func (s *UpstreamConnectionService) ListGroupCatalog(ctx context.Context, p UpstreamGroupCatalogParams) (*UpstreamGroupCatalogResult, error) {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PageSize < 1 || p.PageSize > 200 {
		p.PageSize = 20
	}
	p.Search = strings.TrimSpace(p.Search)
	p.Tag = strings.TrimSpace(p.Tag)
	if len(p.Search) > 300 || len(p.ConnectionIDs) > 200 {
		return nil, infraerrors.BadRequest("INVALID_FILTER", "Too many filters")
	}
	switch p.Binding {
	case "", "bound", "unbound":
	default:
		return nil, infraerrors.BadRequest("INVALID_FILTER", "Invalid binding filter")
	}
	switch p.Freshness {
	case "", "fresh", "stale", "unknown", "error":
	default:
		return nil, infraerrors.BadRequest("INVALID_FILTER", "Invalid freshness filter")
	}
	switch p.Sort {
	case "", "name_asc", "name_desc", "rate_asc", "rate_desc", "bindings_desc", "observed_desc", "connection_asc":
	default:
		return nil, infraerrors.BadRequest("INVALID_FILTER", "Invalid sort")
	}
	for _, v := range []*float64{p.MinRate, p.MaxRate} {
		if v != nil && (math.IsNaN(*v) || math.IsInf(*v, 0) || *v < 0) {
			return nil, infraerrors.BadRequest("INVALID_FILTER", "Invalid rate")
		}
	}
	if p.MinRate != nil && p.MaxRate != nil && *p.MinRate > *p.MaxRate {
		return nil, infraerrors.BadRequest("INVALID_FILTER", "Minimum exceeds maximum")
	}
	r, err := s.catalogRepository()
	if err != nil {
		return nil, err
	}
	return r.ListGroupCatalog(ctx, p, s.now())
}
func (s *UpstreamConnectionService) UpdateGroupAnnotations(ctx context.Context, p UpstreamGroupAnnotationUpdate) error {
	if len(p.Groups) == 0 || len(p.Groups) > 200 {
		return infraerrors.BadRequest("INVALID_GROUPS", "Select 1 to 200 groups")
	}
	for _, g := range p.Groups {
		if g.ConnectionID <= 0 || g.RemoteKey == "" || len(g.RemoteKey) > 1024 {
			return infraerrors.BadRequest("INVALID_GROUPS", "Invalid group reference")
		}
	}
	for _, tags := range [][]string{p.AddTags, p.RemoveTags} {
		if len(tags) > 20 {
			return infraerrors.BadRequest("INVALID_TAGS", "Too many tags")
		}
		for i, tag := range tags {
			tags[i] = strings.TrimSpace(tag)
			if tags[i] == "" || utf8.RuneCountInString(tags[i]) > 32 {
				return infraerrors.BadRequest("INVALID_TAGS", "Tags must contain 1 to 32 characters")
			}
		}
	}
	r, err := s.catalogRepository()
	if err != nil {
		return err
	}
	return r.UpdateGroupAnnotations(ctx, p)
}
func (s *UpstreamConnectionService) GetCostHistory(ctx context.Context, ids []int64, startDate, endDate string) (*UpstreamConnectionCostHistory, error) {
	if startDate == "" && endDate == "" {
		startDate = s.now().In(timezone.Location()).Format("2006-01-02")
		endDate = startDate
	}
	start, err := time.ParseInLocation("2006-01-02", startDate, timezone.Location())
	if err != nil {
		return nil, infraerrors.BadRequest("INVALID_DATE", "Invalid start date")
	}
	last, err := time.ParseInLocation("2006-01-02", endDate, timezone.Location())
	if err != nil {
		return nil, infraerrors.BadRequest("INVALID_DATE", "Invalid end date")
	}
	now := s.now().In(timezone.Location())
	end := last.AddDate(0, 0, 1)
	if start.After(last) || last.After(timezone.StartOfDay(now)) || end.After(start.AddDate(1, 0, 0)) || len(ids) > 200 {
		return nil, infraerrors.BadRequest("INVALID_DATE_RANGE", "Select a past or current range of at most one year")
	}
	if end.After(now) {
		end = now
	}
	r, err := s.catalogRepository()
	if err != nil {
		return nil, err
	}
	buckets, err := r.GetConnectionDailyCosts(ctx, ids, start, end, timezone.Location().String())
	if err != nil {
		return nil, err
	}
	result := &UpstreamConnectionCostHistory{Items: []UpstreamConnectionCostItem{}, Daily: []UpstreamCostDay{}, StartAt: start, EndAt: end, Timezone: timezone.Location().String()}
	dayIndexes := make(map[string]int)
	// Calendar increments preserve 23/25-hour days across daylight-saving changes.
	for day := start; !day.After(last); day = day.AddDate(0, 0, 1) {
		key := day.Format("2006-01-02")
		dayIndexes[key] = len(result.Daily)
		result.Daily = append(result.Daily, UpstreamCostDay{Date: key})
	}
	connectionIndexes := make(map[int64]int)
	for _, bucket := range buckets {
		if bucket.Day != "" {
			if dayIndex, ok := dayIndexes[bucket.Day]; ok {
				result.Daily[dayIndex].AccountCost += bucket.AccountCost
				result.Daily[dayIndex].Requests += bucket.Requests
			}
			continue
		}
		index, exists := connectionIndexes[bucket.ConnectionID]
		if !exists {
			index = len(result.Items)
			connectionIndexes[bucket.ConnectionID] = index
			result.Items = append(result.Items, UpstreamConnectionCostItem{ConnectionID: bucket.ConnectionID, Name: bucket.Name})
		}
		result.Items[index].AccountCost += bucket.AccountCost
		result.Items[index].Requests += bucket.Requests
	}
	for _, day := range result.Daily {
		result.TotalCost += day.AccountCost
		result.TotalRequests += day.Requests
	}
	sort.Slice(result.Items, func(i, j int) bool {
		if result.Items[i].AccountCost != result.Items[j].AccountCost {
			return result.Items[i].AccountCost > result.Items[j].AccountCost
		}
		return result.Items[i].ConnectionID < result.Items[j].ConnectionID
	})
	return result, nil
}
