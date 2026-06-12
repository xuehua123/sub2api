package service

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionplan"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionplangroup"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	PlanAccessScopeExplicit                   = "explicit"
	PlanAccessScopePlatformSubscriptionGroups = "platform_subscription_groups"
	PlanAccessScopeAllSubscriptionGroups      = "all_subscription_groups"
)

// validatePlanRequired checks that all required fields for a plan are provided.
func validatePlanRequired(req CreatePlanRequest) error {
	if strings.TrimSpace(req.Name) == "" {
		return infraerrors.BadRequest("PLAN_NAME_REQUIRED", "plan name is required")
	}
	if req.Price <= 0 {
		return infraerrors.BadRequest("PLAN_PRICE_INVALID", "price must be > 0")
	}
	if req.ValidityDays <= 0 {
		return infraerrors.BadRequest("PLAN_VALIDITY_REQUIRED", "validity days must be > 0")
	}
	if strings.TrimSpace(req.ValidityUnit) == "" {
		return infraerrors.BadRequest("PLAN_VALIDITY_UNIT_REQUIRED", "validity unit is required")
	}
	if !isSupportedValidityUnit(req.ValidityUnit) {
		return infraerrors.BadRequest("PLAN_VALIDITY_UNIT_INVALID", "valid validity unit is required")
	}
	if req.OriginalPrice != nil && *req.OriginalPrice < 0 {
		return infraerrors.BadRequest("PLAN_ORIGINAL_PRICE_INVALID", "original price must be >= 0")
	}
	if err := validatePlanLimit("daily", req.DailyLimitUSD); err != nil {
		return err
	}
	if err := validatePlanLimit("weekly", req.WeeklyLimitUSD); err != nil {
		return err
	}
	if err := validatePlanLimit("monthly", req.MonthlyLimitUSD); err != nil {
		return err
	}
	return nil
}

// validatePlanPatch validates only the non-nil fields in a patch update.
func validatePlanPatch(req UpdatePlanRequest) error {
	if req.Name != nil && strings.TrimSpace(*req.Name) == "" {
		return infraerrors.BadRequest("PLAN_NAME_REQUIRED", "plan name is required")
	}
	if req.GroupID != nil && *req.GroupID <= 0 {
		return infraerrors.BadRequest("PLAN_GROUP_REQUIRED", "group is required")
	}
	if req.Price != nil && *req.Price <= 0 {
		return infraerrors.BadRequest("PLAN_PRICE_INVALID", "price must be > 0")
	}
	if req.ValidityDays != nil && *req.ValidityDays <= 0 {
		return infraerrors.BadRequest("PLAN_VALIDITY_REQUIRED", "validity days must be > 0")
	}
	if req.ValidityUnit != nil && strings.TrimSpace(*req.ValidityUnit) == "" {
		return infraerrors.BadRequest("PLAN_VALIDITY_UNIT_REQUIRED", "validity unit is required")
	}
	if req.ValidityUnit != nil && !isSupportedValidityUnit(*req.ValidityUnit) {
		return infraerrors.BadRequest("PLAN_VALIDITY_UNIT_INVALID", "valid validity unit is required")
	}
	if req.OriginalPrice.Set && req.OriginalPrice.Value != nil && *req.OriginalPrice.Value < 0 {
		return infraerrors.BadRequest("PLAN_ORIGINAL_PRICE_INVALID", "original price must be >= 0")
	}
	if req.DailyLimitUSD.Set {
		if err := validatePlanLimit("daily", req.DailyLimitUSD.Value); err != nil {
			return err
		}
	}
	if req.WeeklyLimitUSD.Set {
		if err := validatePlanLimit("weekly", req.WeeklyLimitUSD.Value); err != nil {
			return err
		}
	}
	if req.MonthlyLimitUSD.Set {
		if err := validatePlanLimit("monthly", req.MonthlyLimitUSD.Value); err != nil {
			return err
		}
	}
	return nil
}

func validatePlanLimit(label string, value *float64) error {
	if value == nil {
		return nil
	}
	if math.IsNaN(*value) || math.IsInf(*value, 0) || *value <= 0 {
		return infraerrors.BadRequest("PLAN_LIMIT_INVALID", label+" limit must be greater than 0")
	}
	return nil
}

// --- Plan CRUD ---

// PlanGroupInfo holds the group details needed for subscription plan display.
type PlanGroupInfo struct {
	ID              int64    `json:"id"`
	Platform        string   `json:"platform"`
	Name            string   `json:"name"`
	RateMultiplier  float64  `json:"rate_multiplier"`
	DailyLimitUSD   *float64 `json:"daily_limit_usd"`
	WeeklyLimitUSD  *float64 `json:"weekly_limit_usd"`
	MonthlyLimitUSD *float64 `json:"monthly_limit_usd"`
	ModelScopes     []string `json:"supported_model_scopes"`
	SortOrder       int      `json:"sort_order"`
}

// SubscriptionPlanResponse is the admin-facing plan shape with v2 entitlement configuration.
type SubscriptionPlanResponse struct {
	ID               int64           `json:"id"`
	GroupID          int64           `json:"group_id"`
	GroupIDs         []int64         `json:"group_ids"`
	Groups           []PlanGroupInfo `json:"groups"`
	GroupPlatform    string          `json:"group_platform,omitempty"`
	GroupName        string          `json:"group_name,omitempty"`
	RateMultiplier   float64         `json:"rate_multiplier,omitempty"`
	ModelScopes      []string        `json:"supported_model_scopes,omitempty"`
	Name             string          `json:"name"`
	Description      string          `json:"description"`
	Price            float64         `json:"price"`
	OriginalPrice    *float64        `json:"original_price,omitempty"`
	ValidityDays     int             `json:"validity_days"`
	ValidityUnit     string          `json:"validity_unit"`
	AccessScope      string          `json:"access_scope"`
	AllowedPlatforms []string        `json:"allowed_platforms"`
	DailyLimitUSD    *float64        `json:"daily_limit_usd"`
	WeeklyLimitUSD   *float64        `json:"weekly_limit_usd"`
	MonthlyLimitUSD  *float64        `json:"monthly_limit_usd"`
	OveragePolicy    string          `json:"overage_policy"`
	Features         string          `json:"features"`
	ProductName      string          `json:"product_name"`
	ForSale          bool            `json:"for_sale"`
	SortOrder        int             `json:"sort_order"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

// GetGroupPlatformMap returns a map of group_id → platform for the given plans.
func (s *PaymentConfigService) GetGroupPlatformMap(ctx context.Context, plans []*dbent.SubscriptionPlan) map[int64]string {
	info := s.GetGroupInfoMap(ctx, plans)
	m := make(map[int64]string, len(info))
	for id, gi := range info {
		m[id] = gi.Platform
	}
	return m
}

// GetGroupInfoMap returns a map of group_id → PlanGroupInfo for the given plans.
func (s *PaymentConfigService) GetGroupInfoMap(ctx context.Context, plans []*dbent.SubscriptionPlan) map[int64]PlanGroupInfo {
	ids := make([]int64, 0, len(plans))
	seen := make(map[int64]bool)
	for _, p := range plans {
		if p != nil && p.GroupID > 0 && !seen[p.GroupID] {
			seen[p.GroupID] = true
			ids = append(ids, p.GroupID)
		}
	}
	groupsByID, err := s.loadGroupsByID(ctx, ids, false)
	if err != nil {
		return nil
	}
	m := make(map[int64]PlanGroupInfo, len(groupsByID))
	for _, id := range ids {
		if g := groupsByID[id]; g != nil {
			m[id] = planGroupInfoFromEnt(g, g.SortOrder)
		}
	}
	return m
}

func (s *PaymentConfigService) ListPlans(ctx context.Context) ([]SubscriptionPlanResponse, error) {
	plans, err := s.entClient.SubscriptionPlan.Query().Order(subscriptionplan.BySortOrder(), subscriptionplan.ByID()).All(ctx)
	if err != nil {
		return nil, err
	}
	return s.buildPlanResponses(ctx, plans)
}

func (s *PaymentConfigService) ListPlansForSale(ctx context.Context) ([]*dbent.SubscriptionPlan, error) {
	plans, err := s.entClient.SubscriptionPlan.Query().Where(subscriptionplan.ForSaleEQ(true)).Order(subscriptionplan.BySortOrder(), subscriptionplan.ByID()).All(ctx)
	if err != nil {
		return nil, err
	}
	filtered := make([]*dbent.SubscriptionPlan, 0, len(plans))
	for _, plan := range plans {
		if plan != nil && isSupportedValidityUnit(plan.ValidityUnit) {
			filtered = append(filtered, plan)
		}
	}
	return filtered, nil
}

func (s *PaymentConfigService) CreatePlan(ctx context.Context, req CreatePlanRequest) (*SubscriptionPlanResponse, error) {
	if err := validatePlanRequired(req); err != nil {
		return nil, err
	}
	req.ValidityUnit = normalizeValidityUnit(req.ValidityUnit)
	scope, err := normalizePlanAccessScopeWithDefault(req.AccessScope)
	if err != nil {
		return nil, err
	}
	overagePolicy, err := normalizePlanOveragePolicyWithDefault(req.OveragePolicy)
	if err != nil {
		return nil, err
	}
	groupIDs, err := normalizePlanGroupIDs(planCreateGroupIDs(req))
	if err != nil {
		return nil, err
	}
	allowedPlatforms, err := normalizePlanPlatforms(req.AllowedPlatforms)
	if err != nil {
		return nil, err
	}
	access, err := s.resolvePlanAccess(ctx, scope, groupIDs, true, allowedPlatforms)
	if err != nil {
		return nil, err
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("start plan create transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	b := tx.SubscriptionPlan.Create().
		SetGroupID(access.PrimaryGroupID).
		SetName(req.Name).
		SetDescription(req.Description).
		SetPrice(req.Price).
		SetValidityDays(req.ValidityDays).
		SetValidityUnit(req.ValidityUnit).
		SetAccessScope(scope).
		SetAllowedPlatforms(access.AllowedPlatforms).
		SetOveragePolicy(overagePolicy).
		SetFeatures(req.Features).
		SetProductName(req.ProductName).
		SetForSale(req.ForSale).
		SetSortOrder(req.SortOrder)
	if req.OriginalPrice != nil {
		b.SetOriginalPrice(*req.OriginalPrice)
	}
	b.SetNillableDailyLimitUsd(req.DailyLimitUSD)
	b.SetNillableWeeklyLimitUsd(req.WeeklyLimitUSD)
	b.SetNillableMonthlyLimitUsd(req.MonthlyLimitUSD)
	plan, err := b.Save(ctx)
	if err != nil {
		return nil, err
	}
	if err := replacePlanGroupsTx(ctx, tx, plan.ID, access.PersistGroupIDs); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	committed = true
	return s.GetPlanResponse(ctx, plan.ID)
}

// UpdatePlan updates a subscription plan by ID (patch semantics).
func (s *PaymentConfigService) UpdatePlan(ctx context.Context, id int64, req UpdatePlanRequest) (*SubscriptionPlanResponse, error) {
	if err := validatePlanPatch(req); err != nil {
		return nil, err
	}
	existing, err := s.entClient.SubscriptionPlan.Get(ctx, id)
	if err != nil {
		return nil, infraerrors.NotFound("PLAN_NOT_FOUND", "subscription plan not found")
	}
	scope, err := effectivePlanAccessScope(existing.AccessScope, req.AccessScope)
	if err != nil {
		return nil, err
	}
	groupIDs, groupIDsExplicit, err := s.effectivePlanGroupIDs(ctx, id, req.GroupIDs, req.GroupID)
	if err != nil {
		return nil, err
	}
	allowedPlatforms, err := effectivePlanAllowedPlatforms(existing.AllowedPlatforms, req.AccessScope, req.AllowedPlatforms)
	if err != nil {
		return nil, err
	}
	overagePolicy, err := effectivePlanOveragePolicy(existing.OveragePolicy, req.OveragePolicy)
	if err != nil {
		return nil, err
	}
	access, err := s.resolvePlanAccess(ctx, scope, groupIDs, groupIDsExplicit, allowedPlatforms)
	if err != nil {
		return nil, err
	}

	if req.ValidityUnit != nil {
		normalized := normalizeValidityUnit(*req.ValidityUnit)
		req.ValidityUnit = &normalized
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("start plan update transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	u := tx.SubscriptionPlan.UpdateOneID(id).
		SetGroupID(access.PrimaryGroupID).
		SetAccessScope(scope).
		SetAllowedPlatforms(access.AllowedPlatforms).
		SetOveragePolicy(overagePolicy)
	if req.Name != nil {
		u.SetName(*req.Name)
	}
	if req.Description != nil {
		u.SetDescription(*req.Description)
	}
	if req.Price != nil {
		u.SetPrice(*req.Price)
	}
	if req.OriginalPrice.Set {
		if req.OriginalPrice.Value == nil {
			u.ClearOriginalPrice()
		} else {
			u.SetOriginalPrice(*req.OriginalPrice.Value)
		}
	}
	if req.ValidityDays != nil {
		u.SetValidityDays(*req.ValidityDays)
	}
	if req.ValidityUnit != nil {
		u.SetValidityUnit(*req.ValidityUnit)
	}
	applyPlanLimitPatch(u, req.DailyLimitUSD, req.WeeklyLimitUSD, req.MonthlyLimitUSD)
	if req.Features != nil {
		u.SetFeatures(*req.Features)
	}
	if req.ProductName != nil {
		u.SetProductName(*req.ProductName)
	}
	if req.ForSale != nil {
		u.SetForSale(*req.ForSale)
	}
	if req.SortOrder != nil {
		u.SetSortOrder(*req.SortOrder)
	}
	if _, err := u.Save(ctx); err != nil {
		return nil, err
	}
	if groupIDsExplicit {
		if err := replacePlanGroupsTx(ctx, tx, id, access.PersistGroupIDs); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	committed = true
	return s.GetPlanResponse(ctx, id)
}

func (s *PaymentConfigService) DeletePlan(ctx context.Context, id int64) error {
	count, err := s.countPendingOrdersByPlan(ctx, id)
	if err != nil {
		return fmt.Errorf("check pending orders: %w", err)
	}
	if count > 0 {
		return infraerrors.Conflict("PENDING_ORDERS",
			fmt.Sprintf("this plan has %d in-progress orders and cannot be deleted — wait for orders to complete first", count))
	}
	return s.entClient.SubscriptionPlan.DeleteOneID(id).Exec(ctx)
}

// GetPlan returns a subscription plan by ID.
func (s *PaymentConfigService) GetPlan(ctx context.Context, id int64) (*dbent.SubscriptionPlan, error) {
	plan, err := s.entClient.SubscriptionPlan.Get(ctx, id)
	if err != nil {
		return nil, infraerrors.NotFound("PLAN_NOT_FOUND", "subscription plan not found")
	}
	return plan, nil
}

// GetPlanResponse returns an admin-facing plan response by ID.
func (s *PaymentConfigService) GetPlanResponse(ctx context.Context, id int64) (*SubscriptionPlanResponse, error) {
	plan, err := s.GetPlan(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.buildPlanResponse(ctx, plan)
}

type resolvedPlanAccess struct {
	PrimaryGroupID   int64
	AllowedPlatforms []string
	PersistGroupIDs  []int64
	EffectiveGroups  []*dbent.Group
}

func (s *PaymentConfigService) resolvePlanAccess(ctx context.Context, scope string, groupIDs []int64, groupIDsExplicit bool, allowedPlatforms []string) (*resolvedPlanAccess, error) {
	switch scope {
	case PlanAccessScopeExplicit:
		if len(groupIDs) == 0 {
			return nil, infraerrors.BadRequest("PLAN_GROUPS_REQUIRED", "explicit access scope requires group_ids")
		}
		groups, err := s.loadSubscriptionGroupsInOrder(ctx, groupIDs)
		if err != nil {
			return nil, err
		}
		return &resolvedPlanAccess{
			PrimaryGroupID:   groupIDs[0],
			AllowedPlatforms: []string{},
			PersistGroupIDs:  groupIDs,
			EffectiveGroups:  groups,
		}, nil
	case PlanAccessScopePlatformSubscriptionGroups:
		if groupIDsExplicit && len(groupIDs) > 0 {
			return nil, infraerrors.BadRequest("PLAN_GROUPS_SCOPE_MISMATCH", "platform access scope does not accept explicit group_ids")
		}
		if len(allowedPlatforms) == 0 {
			return nil, infraerrors.BadRequest("PLAN_ALLOWED_PLATFORMS_REQUIRED", "platform access scope requires allowed_platforms")
		}
		groups, err := s.loadSubscriptionGroupsByPlatforms(ctx, allowedPlatforms)
		if err != nil {
			return nil, err
		}
		if len(groups) == 0 {
			return nil, infraerrors.BadRequest("PLAN_ALLOWED_PLATFORMS_EMPTY", "allowed_platforms must match at least one active subscription group")
		}
		return &resolvedPlanAccess{
			PrimaryGroupID:   groups[0].ID,
			AllowedPlatforms: allowedPlatforms,
			PersistGroupIDs:  []int64{},
			EffectiveGroups:  groups,
		}, nil
	case PlanAccessScopeAllSubscriptionGroups:
		if groupIDsExplicit && len(groupIDs) > 0 {
			return nil, infraerrors.BadRequest("PLAN_GROUPS_SCOPE_MISMATCH", "all access scope does not accept explicit group_ids")
		}
		if len(allowedPlatforms) > 0 {
			return nil, infraerrors.BadRequest("PLAN_ALLOWED_PLATFORMS_SCOPE_MISMATCH", "all access scope does not accept allowed_platforms")
		}
		groups, err := s.loadAllSubscriptionGroups(ctx)
		if err != nil {
			return nil, err
		}
		if len(groups) == 0 {
			return nil, infraerrors.BadRequest("PLAN_GROUPS_EMPTY", "all access scope requires at least one active subscription group")
		}
		return &resolvedPlanAccess{
			PrimaryGroupID:   groups[0].ID,
			AllowedPlatforms: []string{},
			PersistGroupIDs:  []int64{},
			EffectiveGroups:  groups,
		}, nil
	default:
		return nil, infraerrors.BadRequest("PLAN_ACCESS_SCOPE_INVALID", "invalid access scope")
	}
}

func normalizePlanAccessScopeWithDefault(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = PlanAccessScopeExplicit
	}
	return normalizePlanAccessScope(value)
}

func normalizePlanAccessScope(value string) (string, error) {
	switch strings.TrimSpace(value) {
	case PlanAccessScopeExplicit:
		return PlanAccessScopeExplicit, nil
	case PlanAccessScopePlatformSubscriptionGroups:
		return PlanAccessScopePlatformSubscriptionGroups, nil
	case PlanAccessScopeAllSubscriptionGroups:
		return PlanAccessScopeAllSubscriptionGroups, nil
	default:
		return "", infraerrors.BadRequest("PLAN_ACCESS_SCOPE_INVALID", "invalid access scope")
	}
}

func normalizePlanOveragePolicyWithDefault(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = SubscriptionEntitlementOverageBlock
	}
	return normalizePlanOveragePolicy(value)
}

func normalizePlanOveragePolicy(value string) (string, error) {
	switch strings.TrimSpace(value) {
	case SubscriptionEntitlementOverageBlock:
		return SubscriptionEntitlementOverageBlock, nil
	case SubscriptionEntitlementOverageBalanceFallback:
		return SubscriptionEntitlementOverageBalanceFallback, nil
	default:
		return "", infraerrors.BadRequest("PLAN_OVERAGE_POLICY_INVALID", "invalid overage policy")
	}
}

func effectivePlanAccessScope(existing string, patch *string) (string, error) {
	if patch == nil {
		return normalizePlanAccessScopeWithDefault(existing)
	}
	return normalizePlanAccessScopeWithDefault(*patch)
}

func effectivePlanOveragePolicy(existing string, patch *string) (string, error) {
	if patch == nil {
		return normalizePlanOveragePolicyWithDefault(existing)
	}
	return normalizePlanOveragePolicyWithDefault(*patch)
}

func effectivePlanAllowedPlatforms(existing []string, accessScopePatch *string, patch []string) ([]string, error) {
	if patch != nil {
		return normalizePlanPlatforms(patch)
	}
	if accessScopePatch != nil {
		scope, err := normalizePlanAccessScopeWithDefault(*accessScopePatch)
		if err != nil {
			return nil, err
		}
		if scope != PlanAccessScopePlatformSubscriptionGroups {
			return []string{}, nil
		}
	}
	return normalizePlanPlatforms(existing)
}

func planCreateGroupIDs(req CreatePlanRequest) []int64 {
	if req.GroupIDs != nil {
		return req.GroupIDs
	}
	if req.GroupID > 0 {
		return []int64{req.GroupID}
	}
	return nil
}

func (s *PaymentConfigService) effectivePlanGroupIDs(ctx context.Context, planID int64, groupIDs []int64, groupID *int64) ([]int64, bool, error) {
	if groupIDs != nil {
		ids, err := normalizePlanGroupIDs(groupIDs)
		return ids, true, err
	}
	if groupID != nil {
		ids, err := normalizePlanGroupIDs([]int64{*groupID})
		return ids, true, err
	}
	ids, err := s.listPersistedPlanGroupIDs(ctx, planID)
	return ids, false, err
}

func normalizePlanGroupIDs(ids []int64) ([]int64, error) {
	if ids == nil {
		return nil, nil
	}
	out := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, infraerrors.BadRequest("PLAN_GROUP_REQUIRED", "group is required")
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out, nil
}

func normalizePlanPlatforms(platforms []string) ([]string, error) {
	if platforms == nil {
		return nil, nil
	}
	out := make([]string, 0, len(platforms))
	seen := make(map[string]struct{}, len(platforms))
	for _, platform := range platforms {
		platform = strings.ToLower(strings.TrimSpace(platform))
		if platform == "" {
			return nil, infraerrors.BadRequest("PLAN_ALLOWED_PLATFORM_INVALID", "allowed platform cannot be empty")
		}
		if _, ok := seen[platform]; ok {
			continue
		}
		seen[platform] = struct{}{}
		out = append(out, platform)
	}
	return out, nil
}

func (s *PaymentConfigService) listPersistedPlanGroupIDs(ctx context.Context, planID int64) ([]int64, error) {
	rows, err := s.entClient.SubscriptionPlanGroup.Query().
		Where(subscriptionplangroup.PlanIDEQ(planID), subscriptionplangroup.EnabledEQ(true)).
		Order(subscriptionplangroup.BySortOrder(), subscriptionplangroup.ByGroupID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.GroupID)
	}
	return ids, nil
}

func (s *PaymentConfigService) loadSubscriptionGroupsInOrder(ctx context.Context, ids []int64) ([]*dbent.Group, error) {
	groupsByID, err := s.loadGroupsByID(ctx, ids, true)
	if err != nil {
		return nil, err
	}
	groups := make([]*dbent.Group, 0, len(ids))
	for _, id := range ids {
		g := groupsByID[id]
		if g == nil {
			return nil, infraerrors.BadRequest("PLAN_GROUP_INVALID", "group must be an active subscription group")
		}
		groups = append(groups, g)
	}
	return groups, nil
}

func (s *PaymentConfigService) loadGroupsByID(ctx context.Context, ids []int64, requireActiveSubscription bool) (map[int64]*dbent.Group, error) {
	out := make(map[int64]*dbent.Group, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	groups, err := s.entClient.Group.Query().Where(group.IDIn(ids...)).All(ctx)
	if err != nil {
		return nil, err
	}
	for _, g := range groups {
		if requireActiveSubscription && (g.Status != StatusActive || g.SubscriptionType != SubscriptionTypeSubscription) {
			continue
		}
		out[g.ID] = g
	}
	return out, nil
}

func (s *PaymentConfigService) loadSubscriptionGroupsByPlatforms(ctx context.Context, platforms []string) ([]*dbent.Group, error) {
	return s.entClient.Group.Query().
		Where(
			group.StatusEQ(StatusActive),
			group.SubscriptionTypeEQ(SubscriptionTypeSubscription),
			group.PlatformIn(platforms...),
		).
		Order(group.BySortOrder(), group.ByID()).
		All(ctx)
}

func (s *PaymentConfigService) loadAllSubscriptionGroups(ctx context.Context) ([]*dbent.Group, error) {
	return s.entClient.Group.Query().
		Where(
			group.StatusEQ(StatusActive),
			group.SubscriptionTypeEQ(SubscriptionTypeSubscription),
		).
		Order(group.BySortOrder(), group.ByID()).
		All(ctx)
}

func replacePlanGroupsTx(ctx context.Context, tx *dbent.Tx, planID int64, groupIDs []int64) error {
	if _, err := tx.SubscriptionPlanGroup.Delete().
		Where(subscriptionplangroup.PlanIDEQ(planID)).
		Exec(ctx); err != nil {
		return err
	}
	if len(groupIDs) == 0 {
		return nil
	}
	creates := make([]*dbent.SubscriptionPlanGroupCreate, 0, len(groupIDs))
	for i, groupID := range groupIDs {
		creates = append(creates, tx.SubscriptionPlanGroup.Create().
			SetPlanID(planID).
			SetGroupID(groupID).
			SetSortOrder(i).
			SetEnabled(true))
	}
	return tx.SubscriptionPlanGroup.CreateBulk(creates...).Exec(ctx)
}

func applyPlanLimitPatch(u *dbent.SubscriptionPlanUpdateOne, daily, weekly, monthly OptionalFloat64) {
	if daily.Set {
		if daily.Value == nil {
			u.ClearDailyLimitUsd()
		} else {
			u.SetDailyLimitUsd(*daily.Value)
		}
	}
	if weekly.Set {
		if weekly.Value == nil {
			u.ClearWeeklyLimitUsd()
		} else {
			u.SetWeeklyLimitUsd(*weekly.Value)
		}
	}
	if monthly.Set {
		if monthly.Value == nil {
			u.ClearMonthlyLimitUsd()
		} else {
			u.SetMonthlyLimitUsd(*monthly.Value)
		}
	}
}

func (s *PaymentConfigService) buildPlanResponses(ctx context.Context, plans []*dbent.SubscriptionPlan) ([]SubscriptionPlanResponse, error) {
	out := make([]SubscriptionPlanResponse, 0, len(plans))
	for _, plan := range plans {
		resp, err := s.buildPlanResponse(ctx, plan)
		if err != nil {
			return nil, err
		}
		out = append(out, *resp)
	}
	return out, nil
}

func (s *PaymentConfigService) buildPlanResponse(ctx context.Context, plan *dbent.SubscriptionPlan) (*SubscriptionPlanResponse, error) {
	if plan == nil {
		return nil, infraerrors.NotFound("PLAN_NOT_FOUND", "subscription plan not found")
	}
	groups, groupIDs, err := s.resolvePlanResponseGroups(ctx, plan)
	if err != nil {
		return nil, err
	}
	resp := &SubscriptionPlanResponse{
		ID:               plan.ID,
		GroupID:          plan.GroupID,
		GroupIDs:         groupIDs,
		Groups:           groups,
		Name:             plan.Name,
		Description:      plan.Description,
		Price:            plan.Price,
		OriginalPrice:    plan.OriginalPrice,
		ValidityDays:     plan.ValidityDays,
		ValidityUnit:     plan.ValidityUnit,
		AccessScope:      normalizePlanAccessScopeForResponse(plan.AccessScope),
		AllowedPlatforms: nonNilStrings(plan.AllowedPlatforms),
		DailyLimitUSD:    plan.DailyLimitUsd,
		WeeklyLimitUSD:   plan.WeeklyLimitUsd,
		MonthlyLimitUSD:  plan.MonthlyLimitUsd,
		OveragePolicy:    normalizePlanOveragePolicyForResponse(plan.OveragePolicy),
		Features:         plan.Features,
		ProductName:      plan.ProductName,
		ForSale:          plan.ForSale,
		SortOrder:        plan.SortOrder,
		CreatedAt:        plan.CreatedAt,
		UpdatedAt:        plan.UpdatedAt,
	}
	if len(groups) > 0 {
		first := groups[0]
		resp.GroupPlatform = first.Platform
		resp.GroupName = first.Name
		resp.RateMultiplier = first.RateMultiplier
		resp.ModelScopes = first.ModelScopes
	}
	return resp, nil
}

func (s *PaymentConfigService) resolvePlanResponseGroups(ctx context.Context, plan *dbent.SubscriptionPlan) ([]PlanGroupInfo, []int64, error) {
	scope := normalizePlanAccessScopeForResponse(plan.AccessScope)
	switch scope {
	case PlanAccessScopePlatformSubscriptionGroups:
		groups, err := s.loadSubscriptionGroupsByPlatforms(ctx, plan.AllowedPlatforms)
		if err != nil {
			return nil, nil, err
		}
		return planGroupInfosFromEnt(groups), groupIDsFromEntGroups(groups), nil
	case PlanAccessScopeAllSubscriptionGroups:
		groups, err := s.loadAllSubscriptionGroups(ctx)
		if err != nil {
			return nil, nil, err
		}
		return planGroupInfosFromEnt(groups), groupIDsFromEntGroups(groups), nil
	default:
		rows, err := s.entClient.SubscriptionPlanGroup.Query().
			Where(subscriptionplangroup.PlanIDEQ(plan.ID), subscriptionplangroup.EnabledEQ(true)).
			Order(subscriptionplangroup.BySortOrder(), subscriptionplangroup.ByGroupID()).
			All(ctx)
		if err != nil {
			return nil, nil, err
		}
		ids := make([]int64, 0, len(rows))
		sortOrders := make(map[int64]int, len(rows))
		for _, row := range rows {
			ids = append(ids, row.GroupID)
			sortOrders[row.GroupID] = row.SortOrder
		}
		groupsByID, err := s.loadGroupsByID(ctx, ids, false)
		if err != nil {
			return nil, nil, err
		}
		infos := make([]PlanGroupInfo, 0, len(ids))
		for _, id := range ids {
			if g := groupsByID[id]; g != nil {
				infos = append(infos, planGroupInfoFromEnt(g, sortOrders[id]))
			}
		}
		return infos, ids, nil
	}
}

func planGroupInfosFromEnt(groups []*dbent.Group) []PlanGroupInfo {
	infos := make([]PlanGroupInfo, 0, len(groups))
	for _, g := range groups {
		infos = append(infos, planGroupInfoFromEnt(g, g.SortOrder))
	}
	return infos
}

func planGroupInfoFromEnt(g *dbent.Group, sortOrder int) PlanGroupInfo {
	if g == nil {
		return PlanGroupInfo{}
	}
	return PlanGroupInfo{
		ID:              g.ID,
		Platform:        g.Platform,
		Name:            g.Name,
		RateMultiplier:  g.RateMultiplier,
		DailyLimitUSD:   g.DailyLimitUsd,
		WeeklyLimitUSD:  g.WeeklyLimitUsd,
		MonthlyLimitUSD: g.MonthlyLimitUsd,
		ModelScopes:     g.SupportedModelScopes,
		SortOrder:       sortOrder,
	}
}

func groupIDsFromEntGroups(groups []*dbent.Group) []int64 {
	ids := make([]int64, 0, len(groups))
	for _, g := range groups {
		ids = append(ids, g.ID)
	}
	return ids
}

func normalizePlanAccessScopeForResponse(value string) string {
	scope, err := normalizePlanAccessScopeWithDefault(value)
	if err != nil {
		return PlanAccessScopeExplicit
	}
	return scope
}

func normalizePlanOveragePolicyForResponse(value string) string {
	policy, err := normalizePlanOveragePolicyWithDefault(value)
	if err != nil {
		return SubscriptionEntitlementOverageBlock
	}
	return policy
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
