package domain

import (
	"sort"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	AnnouncementStatusDraft    = "draft"
	AnnouncementStatusActive   = "active"
	AnnouncementStatusArchived = "archived"
)

const (
	AnnouncementNotifyModeSilent = "silent"
	AnnouncementNotifyModePopup  = "popup"
)

const (
	AnnouncementConditionTypeSubscription = "subscription"
	AnnouncementConditionTypeBalance      = "balance"
)

const (
	AnnouncementOperatorIn  = "in"
	AnnouncementOperatorGT  = "gt"
	AnnouncementOperatorGTE = "gte"
	AnnouncementOperatorLT  = "lt"
	AnnouncementOperatorLTE = "lte"
	AnnouncementOperatorEQ  = "eq"
)

var (
	ErrAnnouncementNotFound      = infraerrors.NotFound("ANNOUNCEMENT_NOT_FOUND", "announcement not found")
	ErrAnnouncementInvalidTarget = infraerrors.BadRequest("ANNOUNCEMENT_INVALID_TARGET", "invalid announcement targeting rules")
)

type AnnouncementTargeting struct {
	// UserIDs 是硬性白名单；非空时，只有列表内用户才可能命中。
	UserIDs []int64 `json:"user_ids,omitempty"`

	// AnyOf 表示 OR：任意一个条件组满足即可展示。
	AnyOf []AnnouncementConditionGroup `json:"any_of,omitempty"`
}

type AnnouncementConditionGroup struct {
	// AllOf 表示 AND：组内所有条件都满足才算命中该组。
	AllOf []AnnouncementCondition `json:"all_of,omitempty"`
}

type AnnouncementCondition struct {
	// Type: subscription | balance
	Type string `json:"type"`

	// Operator:
	// - subscription: in
	// - balance: gt/gte/lt/lte/eq
	Operator string `json:"operator"`

	// subscription 条件：匹配的订阅套餐（group_id）
	GroupIDs []int64 `json:"group_ids,omitempty"`

	// balance 条件：比较阈值
	Value float64 `json:"value,omitempty"`
}

func (t AnnouncementTargeting) Matches(balance float64, activeSubscriptionGroupIDs map[int64]struct{}) bool {
	return t.MatchesUser(0, balance, activeSubscriptionGroupIDs)
}

func (t AnnouncementTargeting) MatchesUser(userID int64, balance float64, activeSubscriptionGroupIDs map[int64]struct{}) bool {
	if len(t.UserIDs) > 0 {
		matched := false
		for _, targetUserID := range t.UserIDs {
			if targetUserID == userID {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// 空规则：展示给所有用户
	if len(t.AnyOf) == 0 {
		return true
	}

	for _, group := range t.AnyOf {
		if len(group.AllOf) == 0 {
			// 空条件组不命中（避免 OR 中出现无条件 “全命中”）
			continue
		}
		allMatched := true
		for _, cond := range group.AllOf {
			if !cond.Matches(balance, activeSubscriptionGroupIDs) {
				allMatched = false
				break
			}
		}
		if allMatched {
			return true
		}
	}

	return false
}

func (c AnnouncementCondition) Matches(balance float64, activeSubscriptionGroupIDs map[int64]struct{}) bool {
	switch c.Type {
	case AnnouncementConditionTypeSubscription:
		if c.Operator != AnnouncementOperatorIn {
			return false
		}
		if len(c.GroupIDs) == 0 {
			return false
		}
		if len(activeSubscriptionGroupIDs) == 0 {
			return false
		}
		for _, gid := range c.GroupIDs {
			if _, ok := activeSubscriptionGroupIDs[gid]; ok {
				return true
			}
		}
		return false

	case AnnouncementConditionTypeBalance:
		switch c.Operator {
		case AnnouncementOperatorGT:
			return balance > c.Value
		case AnnouncementOperatorGTE:
			return balance >= c.Value
		case AnnouncementOperatorLT:
			return balance < c.Value
		case AnnouncementOperatorLTE:
			return balance <= c.Value
		case AnnouncementOperatorEQ:
			return balance == c.Value
		default:
			return false
		}

	default:
		return false
	}
}

func (t AnnouncementTargeting) NormalizeAndValidate() (AnnouncementTargeting, error) {
	if t.UserIDs != nil && len(t.UserIDs) == 0 {
		return AnnouncementTargeting{}, ErrAnnouncementInvalidTarget
	}
	normalized := AnnouncementTargeting{
		UserIDs: make([]int64, 0, len(t.UserIDs)),
		AnyOf:   make([]AnnouncementConditionGroup, 0, len(t.AnyOf)),
	}
	if len(t.UserIDs) > 1000 {
		return AnnouncementTargeting{}, ErrAnnouncementInvalidTarget
	}
	seenUserIDs := make(map[int64]struct{}, len(t.UserIDs))
	for _, userID := range t.UserIDs {
		if userID <= 0 {
			return AnnouncementTargeting{}, ErrAnnouncementInvalidTarget
		}
		if _, exists := seenUserIDs[userID]; exists {
			continue
		}
		seenUserIDs[userID] = struct{}{}
		normalized.UserIDs = append(normalized.UserIDs, userID)
	}
	sort.Slice(normalized.UserIDs, func(i, j int) bool { return normalized.UserIDs[i] < normalized.UserIDs[j] })

	// 允许空 targeting（展示给所有用户）
	if len(t.AnyOf) == 0 {
		return normalized, nil
	}

	if len(t.AnyOf) > 50 {
		return AnnouncementTargeting{}, ErrAnnouncementInvalidTarget
	}

	for _, g := range t.AnyOf {
		if len(g.AllOf) == 0 {
			return AnnouncementTargeting{}, ErrAnnouncementInvalidTarget
		}
		if len(g.AllOf) > 50 {
			return AnnouncementTargeting{}, ErrAnnouncementInvalidTarget
		}

		group := AnnouncementConditionGroup{AllOf: make([]AnnouncementCondition, 0, len(g.AllOf))}
		for _, c := range g.AllOf {
			cond := AnnouncementCondition{
				Type:     strings.TrimSpace(c.Type),
				Operator: strings.TrimSpace(c.Operator),
				Value:    c.Value,
			}
			for _, gid := range c.GroupIDs {
				if gid <= 0 {
					return AnnouncementTargeting{}, ErrAnnouncementInvalidTarget
				}
				cond.GroupIDs = append(cond.GroupIDs, gid)
			}

			if err := cond.validate(); err != nil {
				return AnnouncementTargeting{}, err
			}
			group.AllOf = append(group.AllOf, cond)
		}

		normalized.AnyOf = append(normalized.AnyOf, group)
	}

	return normalized, nil
}

func (c AnnouncementCondition) validate() error {
	switch c.Type {
	case AnnouncementConditionTypeSubscription:
		if c.Operator != AnnouncementOperatorIn {
			return ErrAnnouncementInvalidTarget
		}
		if len(c.GroupIDs) == 0 {
			return ErrAnnouncementInvalidTarget
		}
		return nil

	case AnnouncementConditionTypeBalance:
		switch c.Operator {
		case AnnouncementOperatorGT, AnnouncementOperatorGTE, AnnouncementOperatorLT, AnnouncementOperatorLTE, AnnouncementOperatorEQ:
			return nil
		default:
			return ErrAnnouncementInvalidTarget
		}

	default:
		return ErrAnnouncementInvalidTarget
	}
}

type Announcement struct {
	ID         int64
	Title      string
	Content    string
	Status     string
	NotifyMode string
	Targeting  AnnouncementTargeting
	StartsAt   *time.Time
	EndsAt     *time.Time
	CreatedBy  *int64
	UpdatedBy  *int64
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (a *Announcement) IsActiveAt(now time.Time) bool {
	if a == nil {
		return false
	}
	if a.Status != AnnouncementStatusActive {
		return false
	}
	if a.StartsAt != nil && now.Before(*a.StartsAt) {
		return false
	}
	if a.EndsAt != nil && !now.Before(*a.EndsAt) {
		// ends_at 语义：到点即下线
		return false
	}
	return true
}
