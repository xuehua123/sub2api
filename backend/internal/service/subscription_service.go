package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"sort"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/dgraph-io/ristretto"
	"golang.org/x/sync/singleflight"
)

// MaxExpiresAt is the maximum allowed expiration date (year 2099)
// This prevents time.Time JSON serialization errors (RFC 3339 requires year <= 9999)
var MaxExpiresAt = time.Date(2099, 12, 31, 23, 59, 59, 0, time.UTC)

// MaxValidityDays is the maximum allowed validity days for subscriptions (100 years)
const MaxValidityDays = 36500
const (
	monthlyCycleDuration              = 30 * 24 * time.Hour
	monthlyCycleAdvanceUsageThreshold = 0.9
	monthlyCycleAdvanceValidityGrace  = time.Minute
)

var (
	ErrSubscriptionNotFound              = infraerrors.NotFound("SUBSCRIPTION_NOT_FOUND", "subscription not found")
	ErrSubscriptionExpired               = infraerrors.Forbidden("SUBSCRIPTION_EXPIRED", "subscription has expired")
	ErrSubscriptionSuspended             = infraerrors.Forbidden("SUBSCRIPTION_SUSPENDED", "subscription is suspended")
	ErrSubscriptionAlreadyExists         = infraerrors.Conflict("SUBSCRIPTION_ALREADY_EXISTS", "subscription already exists for this user and group")
	ErrSubscriptionAssignConflict        = infraerrors.Conflict("SUBSCRIPTION_ASSIGN_CONFLICT", "subscription exists but request conflicts with existing assignment semantics")
	ErrSubscriptionNotRevoked            = infraerrors.Conflict("SUBSCRIPTION_NOT_REVOKED", "subscription is not revoked")
	ErrSubscriptionRestoreConflict       = infraerrors.Conflict("SUBSCRIPTION_RESTORE_CONFLICT", "subscription already exists for this user and group")
	ErrGroupNotSubscriptionType          = infraerrors.BadRequest("GROUP_NOT_SUBSCRIPTION_TYPE", "group is not a subscription type")
	ErrInvalidInput                      = infraerrors.BadRequest("INVALID_INPUT", "at least one of resetDaily, resetWeekly, or resetMonthly must be true")
	ErrMonthlyCycleNotExhausted          = infraerrors.BadRequest("MONTHLY_CYCLE_NOT_EXHAUSTED", "remaining monthly quota must be 10% or less before advancing the next cycle")
	ErrMonthlyCycleNoFutureTime          = infraerrors.BadRequest("MONTHLY_CYCLE_NO_FUTURE_TIME", "subscription does not include a full next monthly cycle to advance")
	ErrMonthlyCycleAdjustmentInvalid     = infraerrors.BadRequest("MONTHLY_CYCLE_ADJUSTMENT_INVALID", "invalid monthly cycle adjustment request")
	ErrMonthlyCycleAdjustmentUnavailable = infraerrors.BadRequest("MONTHLY_CYCLE_ADJUSTMENT_UNAVAILABLE", "monthly cycle adjustment is not available")
	ErrDailyLimitExceeded                = infraerrors.TooManyRequests("DAILY_LIMIT_EXCEEDED", "daily usage limit exceeded")
	ErrWeeklyLimitExceeded               = infraerrors.TooManyRequests("WEEKLY_LIMIT_EXCEEDED", "weekly usage limit exceeded")
	ErrMonthlyLimitExceeded              = infraerrors.TooManyRequests("MONTHLY_LIMIT_EXCEEDED", "monthly usage limit exceeded")
	ErrSubscriptionNilInput              = infraerrors.BadRequest("SUBSCRIPTION_NIL_INPUT", "subscription input cannot be nil")
	ErrAdjustWouldExpire                 = infraerrors.BadRequest("ADJUST_WOULD_EXPIRE", "adjustment would result in expired subscription (remaining days must be > 0)")
	ErrSubscriptionMaintenance           = infraerrors.ServiceUnavailable("SUBSCRIPTION_MAINTENANCE_FAILED", "subscription maintenance failed")
	ErrSubscriptionEndpointUnsupported   = infraerrors.Forbidden("SUBSCRIPTION_ENDPOINT_UNSUPPORTED", "subscription group does not support this endpoint")
)

// SubscriptionService 订阅服务
type SubscriptionService struct {
	groupRepo           GroupRepository
	userSubRepo         UserSubscriptionRepository
	billingCacheService *BillingCacheService
	entClient           *dbent.Client
	settingSvc          SubscriptionEntitlementsRuntimeProvider
	entitlementSvc      *SubscriptionEntitlementService

	// L1 缓存：加速中间件热路径的订阅查询
	subCacheL1     *ristretto.Cache
	subCacheGroup  singleflight.Group
	subCacheTTL    time.Duration
	subCacheJitter int // 抖动百分比

	maintenanceQueue *SubscriptionMaintenanceQueue
	now              func() time.Time
}

type SubscriptionSwitchCandidate struct {
	Subscription       *UserSubscription
	Group              *Group
	Switched           bool
	FromGroupID        int64
	ToGroupID          int64
	FromSubscriptionID *int64
	Reason             string
}

type SubscriptionSwitchRequest struct {
	InboundEndpoint string
}

type SubscriptionGroupPreference struct {
	GroupID   int64 `json:"group_id"`
	SortOrder int   `json:"sort_order"`
	Enabled   bool  `json:"enabled"`
}

type AdvanceMonthlyCycleResult struct {
	Subscription          *UserSubscription `json:"subscription"`
	PreviousExpiresAt     time.Time         `json:"previous_expires_at"`
	NewExpiresAt          time.Time         `json:"new_expires_at"`
	DeductedDays          int               `json:"deducted_days"`
	DeductedSeconds       int64             `json:"deducted_seconds"`
	PreviousMonthlyUsage  float64           `json:"previous_monthly_usage_usd"`
	NewMonthlyWindowStart time.Time         `json:"new_monthly_window_start"`
}

type subscriptionGroupPreferenceRank struct {
	SortOrder int
	Enabled   bool
}

// NewSubscriptionService 创建订阅服务
func NewSubscriptionService(groupRepo GroupRepository, userSubRepo UserSubscriptionRepository, billingCacheService *BillingCacheService, entClient *dbent.Client, cfg *config.Config) *SubscriptionService {
	svc := &SubscriptionService{
		groupRepo:           groupRepo,
		userSubRepo:         userSubRepo,
		billingCacheService: billingCacheService,
		entClient:           entClient,
		now:                 time.Now,
	}
	svc.initSubCache(cfg)
	svc.initMaintenanceQueue(cfg)
	svc.StartSubCacheInvalidationSubscriber(context.Background())
	return svc
}

func (s *SubscriptionService) SetSubscriptionEntitlementAliasDependencies(settingSvc SubscriptionEntitlementsRuntimeProvider, entitlementSvc *SubscriptionEntitlementService) {
	if s == nil {
		return
	}
	s.settingSvc = settingSvc
	s.entitlementSvc = entitlementSvc
	if entitlementSvc != nil {
		entitlementSvc.SetLegacySubscriptionRepository(s.userSubRepo)
		entitlementSvc.SetLegacyAliasInvalidator(s.invalidateEntitlementLegacyAlias)
	}
}

func (s *SubscriptionService) ShouldUseSubscriptionEntitlementAliases(ctx context.Context) bool {
	if s == nil || s.settingSvc == nil || s.entitlementSvc == nil {
		return false
	}
	return s.settingSvc.GetSubscriptionEntitlementsRuntime(ctx).Enabled
}

func (s *SubscriptionService) ListUserSubscriptionEntitlementAliases(ctx context.Context, userID int64) ([]SubscriptionEntitlement, error) {
	if s == nil || s.entitlementSvc == nil {
		return nil, ErrSubscriptionEntitlementNotFound
	}
	entitlements, err := s.entitlementSvc.ListUserEntitlements(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.mergeLegacySubscriptionAliasUsage(ctx, userID, filterSubscriptionAliasEntitlements(entitlements), time.Now())
}

func (s *SubscriptionService) ListActiveUserSubscriptionEntitlementAliases(ctx context.Context, userID int64, now time.Time) ([]SubscriptionEntitlement, error) {
	if s == nil || s.entitlementSvc == nil {
		return nil, ErrSubscriptionEntitlementNotFound
	}
	entitlements, err := s.entitlementSvc.ListActiveUserEntitlements(ctx, userID, now)
	if err != nil {
		return nil, err
	}
	return s.mergeLegacySubscriptionAliasUsage(ctx, userID, filterSubscriptionAliasEntitlements(entitlements), now)
}

func filterSubscriptionAliasEntitlements(entitlements []SubscriptionEntitlement) []SubscriptionEntitlement {
	out := make([]SubscriptionEntitlement, 0, len(entitlements))
	for i := range entitlements {
		if entitlements[i].LegacySubscriptionID == nil {
			continue
		}
		out = append(out, entitlements[i])
	}
	return out
}

func (s *SubscriptionService) mergeLegacySubscriptionAliasUsage(ctx context.Context, userID int64, aliases []SubscriptionEntitlement, now time.Time) ([]SubscriptionEntitlement, error) {
	if len(aliases) == 0 || s == nil || s.userSubRepo == nil || !hasLegacySubscriptionEntitlements(aliases) {
		return aliases, nil
	}
	activeSubscriptions, err := s.userSubRepo.ListActiveByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	legacySubscriptionsByID := mapLegacySubscriptionsByID(activeSubscriptions)
	for i := range aliases {
		mergeLegacySubscriptionUsageIntoEntitlement(&aliases[i], legacySubscriptionsByID, now)
	}
	return aliases, nil
}

func mergeLegacySubscriptionUsageIntoEntitlement(ent *SubscriptionEntitlement, legacySubscriptionsByID map[int64]*UserSubscription, now time.Time) {
	if ent == nil || ent.LegacySubscriptionID == nil || *ent.LegacySubscriptionID <= 0 || len(legacySubscriptionsByID) == 0 {
		return
	}
	legacySub := legacySubscriptionsByID[*ent.LegacySubscriptionID]
	if legacySub == nil {
		return
	}
	ent.DailyUsageUSD = maxFloat64(ent.DailyUsageUSD, eligibleLegacySubscriptionUsageUSD(ent, legacySub, "daily", now))
	ent.WeeklyUsageUSD = maxFloat64(ent.WeeklyUsageUSD, eligibleLegacySubscriptionUsageUSD(ent, legacySub, "weekly", now))
	ent.MonthlyUsageUSD = maxFloat64(ent.MonthlyUsageUSD, eligibleLegacySubscriptionUsageUSD(ent, legacySub, "monthly", now))
}

func maxFloat64(left, right float64) float64 {
	if right > left {
		return right
	}
	return left
}

func (s *SubscriptionService) initMaintenanceQueue(cfg *config.Config) {
	if cfg == nil {
		return
	}
	mc := cfg.SubscriptionMaintenance
	if mc.WorkerCount <= 0 || mc.QueueSize <= 0 {
		return
	}
	s.maintenanceQueue = NewSubscriptionMaintenanceQueue(mc.WorkerCount, mc.QueueSize)
}

// Stop stops the maintenance worker pool.
func (s *SubscriptionService) Stop() {
	if s == nil {
		return
	}
	if s.maintenanceQueue != nil {
		s.maintenanceQueue.Stop()
	}
}

// initSubCache 初始化订阅 L1 缓存
func (s *SubscriptionService) initSubCache(cfg *config.Config) {
	if cfg == nil {
		return
	}
	sc := cfg.SubscriptionCache
	if sc.L1Size <= 0 || sc.L1TTLSeconds <= 0 {
		return
	}
	cache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: int64(sc.L1Size) * 10,
		MaxCost:     int64(sc.L1Size),
		BufferItems: 64,
	})
	if err != nil {
		log.Printf("Warning: failed to init subscription L1 cache: %v", err)
		return
	}
	s.subCacheL1 = cache
	s.subCacheTTL = time.Duration(sc.L1TTLSeconds) * time.Second
	s.subCacheJitter = sc.JitterPercent
}

// subCacheKey 生成订阅缓存 key（热路径，避免 fmt.Sprintf 开销）
func subCacheKey(userID, groupID int64) string {
	return "sub:" + strconv.FormatInt(userID, 10) + ":" + strconv.FormatInt(groupID, 10)
}

func activeSubscriptionsCacheKey(userID int64) string {
	return "sub:list:" + strconv.FormatInt(userID, 10)
}

func subscriptionGroupPreferenceCacheKey(userID int64) string {
	return "sub:prefs:" + strconv.FormatInt(userID, 10)
}

// jitteredTTL 为 TTL 添加抖动，避免集中过期
func (s *SubscriptionService) jitteredTTL(ttl time.Duration) time.Duration {
	if ttl <= 0 || s.subCacheJitter <= 0 {
		return ttl
	}
	pct := s.subCacheJitter
	if pct > 100 {
		pct = 100
	}
	delta := float64(pct) / 100
	factor := 1 - delta + rand.Float64()*(2*delta)
	if factor <= 0 {
		return ttl
	}
	return time.Duration(float64(ttl) * factor)
}

// InvalidateSubCache 失效指定用户+分组的订阅 L1 缓存
func (s *SubscriptionService) InvalidateSubCache(userID, groupID int64) {
	if s.subCacheL1 == nil {
		return
	}
	s.subCacheL1.Del(subCacheKey(userID, groupID))
	s.subCacheL1.Del(activeSubscriptionsCacheKey(userID))
}

func (s *SubscriptionService) waitSubCacheInvalidation() {
	if s != nil && s.subCacheL1 != nil {
		s.subCacheL1.Wait()
	}
}

// InvalidateSubCacheSync 失效订阅 L1 缓存并等待 Ristretto 删除操作生效。
func (s *SubscriptionService) InvalidateSubCacheSync(userID, groupID int64) {
	s.invalidateSubCacheKeySync(subCacheKey(userID, groupID))
	s.invalidateSubCacheKeySync(activeSubscriptionsCacheKey(userID))
}

func (s *SubscriptionService) invalidateSubCacheKeySync(key string) {
	if s.subCacheL1 == nil {
		return
	}
	s.subCacheL1.Del(key)
	s.subCacheL1.Wait()
}

// StartSubCacheInvalidationSubscriber 启动跨实例订阅 L1 缓存失效订阅。
func (s *SubscriptionService) StartSubCacheInvalidationSubscriber(ctx context.Context) {
	if s.billingCacheService == nil || s.subCacheL1 == nil {
		return
	}
	if err := s.billingCacheService.SubscribeSubscriptionCacheInvalidation(ctx, func(cacheKey string) {
		s.invalidateSubCacheKeySync(cacheKey)
	}); err != nil {
		log.Printf("Warning: failed to start subscription cache invalidation subscriber: %v", err)
	}
}

func (s *SubscriptionService) invalidateSubscriptionCachesBefore(userID, groupID, staleBeforeVersion int64) error {
	s.InvalidateSubCacheSync(userID, groupID)
	if s.billingCacheService == nil {
		return nil
	}

	cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.billingCacheService.InvalidateSubscriptionBefore(cacheCtx, userID, groupID, staleBeforeVersion); err != nil {
		return fmt.Errorf("invalidate billing subscription cache: %w", err)
	}
	if err := s.billingCacheService.PublishSubscriptionCacheInvalidation(cacheCtx, subCacheKey(userID, groupID)); err != nil {
		return fmt.Errorf("publish subscription cache invalidation: %w", err)
	}
	if err := s.billingCacheService.PublishSubscriptionCacheInvalidation(cacheCtx, activeSubscriptionsCacheKey(userID)); err != nil {
		return fmt.Errorf("publish active subscriptions cache invalidation: %w", err)
	}
	return nil
}

// AssignSubscriptionInput 分配订阅输入
func (s *SubscriptionService) bestEffortInvalidateSubscriptionCachesBefore(action string, userID, groupID, staleBeforeVersion int64) {
	if err := s.invalidateSubscriptionCachesBefore(userID, groupID, staleBeforeVersion); err != nil {
		log.Printf("Warning: subscription cache invalidation failed after %s for user=%d group=%d: %v", action, userID, groupID, err)
	}
}

type AssignSubscriptionInput struct {
	UserID       int64
	GroupID      int64
	PlanID       int64
	ValidityDays int
	AssignedBy   int64
	Notes        string
}

type subscriptionAssignmentOutcome uint8

const (
	subscriptionAssignmentCreated subscriptionAssignmentOutcome = iota
	subscriptionAssignmentReused
	subscriptionAssignmentRenewed
)

func (o subscriptionAssignmentOutcome) changed() bool {
	return o == subscriptionAssignmentCreated || o == subscriptionAssignmentRenewed
}

func (o subscriptionAssignmentOutcome) reused() bool {
	return o != subscriptionAssignmentCreated
}

// AssignSubscription 分配订阅给用户（不允许重复分配）
func (s *SubscriptionService) AssignSubscription(ctx context.Context, input *AssignSubscriptionInput) (*UserSubscription, error) {
	if input == nil {
		return nil, ErrInvalidInput
	}
	resolvedInput := *input
	resolvedInput.ValidityDays = normalizeAssignValidityDays(input.ValidityDays)
	input = &resolvedInput

	if s.shouldAssignPlanEntitlementAlias(ctx, input) {
		var (
			sub            *UserSubscription
			outcome        subscriptionAssignmentOutcome
			aliasMayChange bool
		)
		entitlementSvc, err := s.adminEntitlementService()
		if err != nil {
			return nil, err
		}
		err = entitlementSvc.entitlementRepo.WithUserEntitlementMutationTx(ctx, input.UserID, func(txCtx context.Context) error {
			var innerErr error
			sub, outcome, innerErr = s.assignSubscriptionWithReuse(txCtx, input, true)
			if innerErr != nil {
				return innerErr
			}
			aliasMayChange = planEntitlementAliasMayChange(sub, outcome)
			return s.assignPlanEntitlementAlias(txCtx, input, sub, outcome)
		})
		if err != nil {
			if shouldFallbackToEntitlementOnlyAssign(err) {
				return s.assignPlanEntitlementOnly(ctx, input)
			}
			return nil, err
		}
		if outcome.changed() || aliasMayChange {
			s.maybeInvalidateAssignmentCaches(input.UserID, input.GroupID, false)
		}
		return sub, nil
	}

	sub, outcome, err := s.assignSubscriptionWithReuse(ctx, input, false)
	if err != nil {
		return nil, err
	}
	if err := s.assignPlanEntitlementAlias(ctx, input, sub, outcome); err != nil {
		return nil, err
	}
	return sub, nil
}

func (s *SubscriptionService) shouldAssignPlanEntitlementAlias(ctx context.Context, input *AssignSubscriptionInput) bool {
	return s != nil && input != nil && input.PlanID > 0 && s.ShouldUseSubscriptionEntitlementAliases(ctx)
}

func shouldFallbackToEntitlementOnlyAssign(err error) bool {
	if !errors.Is(err, ErrSubscriptionAssignConflict) {
		return false
	}
	appErr := infraerrors.FromError(err)
	if appErr == nil || appErr.Metadata == nil {
		return false
	}
	return appErr.Metadata["conflict_reason"] == "plan_id_mismatch"
}

// AssignOrExtendSubscription 分配或续期订阅（用于兑换码等场景）
// 如果用户已有同分组的订阅：
//   - 未过期：从当前过期时间累加天数
//   - 已过期：从当前时间开始计算新的过期时间，并激活订阅
//
// 如果没有订阅：创建新订阅
func (s *SubscriptionService) AssignOrExtendSubscription(ctx context.Context, input *AssignSubscriptionInput) (*UserSubscription, bool, error) {
	return s.assignOrExtendSubscription(ctx, input, false)
}

func (s *SubscriptionService) assignOrExtendSubscription(ctx context.Context, input *AssignSubscriptionInput, deferCacheInvalidation bool) (*UserSubscription, bool, error) {
	// 检查分组是否存在且为订阅类型
	group, err := s.groupRepo.GetByID(ctx, input.GroupID)
	if err != nil {
		return nil, false, fmt.Errorf("group not found: %w", err)
	}
	if !group.IsSubscriptionType() {
		return nil, false, ErrGroupNotSubscriptionType
	}

	// 查询是否已有订阅
	existingSub, err := s.userSubRepo.GetByUserIDAndGroupID(ctx, input.UserID, input.GroupID)
	if err != nil {
		// 不存在记录是正常情况，其他错误需要返回
		existingSub = nil
	}

	validityDays := input.ValidityDays
	if validityDays <= 0 {
		validityDays = 30
	}
	if validityDays > MaxValidityDays {
		validityDays = MaxValidityDays
	}

	// 已有订阅，执行续期（在事务中完成所有更新）
	if existingSub != nil {
		if _, err := s.updateExistingSubscriptionTerm(ctx, existingSub.ID, validityDays, input.Notes, false); err != nil {
			return nil, false, err
		}

		// 失效订阅缓存
		s.maybeInvalidateAssignmentCaches(input.UserID, input.GroupID, deferCacheInvalidation)

		// 返回更新后的订阅
		sub, err := s.userSubRepo.GetByID(ctx, existingSub.ID)
		return sub, true, err // true 表示是续期
	}

	// 没有订阅，创建新订阅
	sub, err := s.createSubscription(ctx, input)
	if err != nil {
		return nil, false, err
	}

	// 失效订阅缓存
	s.maybeInvalidateAssignmentCaches(input.UserID, input.GroupID, deferCacheInvalidation)

	return sub, false, nil // false 表示是新建
}

func (s *SubscriptionService) maybeInvalidateAssignmentCaches(userID, groupID int64, deferred bool) {
	// Payment fulfillment owns an outer transaction and performs a synchronous
	// invalidation after commit. Invalidating inside that transaction can reload
	// the pre-commit subscription into cache.
	if deferred {
		return
	}

	s.bestEffortInvalidateSubscriptionCachesBefore("assign subscription", userID, groupID, 0)
}

func (s *SubscriptionService) assignPlanEntitlementAlias(ctx context.Context, input *AssignSubscriptionInput, sub *UserSubscription, outcome subscriptionAssignmentOutcome) error {
	if s == nil || input == nil || sub == nil || input.PlanID <= 0 || !s.ShouldUseSubscriptionEntitlementAliases(ctx) {
		return nil
	}
	if !planEntitlementAliasMayChange(sub, outcome) {
		return nil
	}
	sourceExternalID := adminAssignEntitlementSourceExternalID(sub.ID, input.PlanID)
	assignNow := time.Time{}
	if shouldRenewPlanEntitlementAlias(sub, outcome) {
		sourceExternalID = adminAssignEntitlementRenewalSourceExternalID(sub.ID, input.PlanID, sub.StartsAt)
		assignNow = sub.StartsAt
	} else if outcome == subscriptionAssignmentReused && sub.EntitlementLink == nil {
		assignNow = sub.StartsAt
	}
	legacySubscriptionID := sub.ID
	_, _, err := s.entitlementSvc.AssignOrExtendFromPlan(ctx, AssignEntitlementFromPlanInput{
		UserID:               input.UserID,
		PlanID:               input.PlanID,
		LegacySubscriptionID: &legacySubscriptionID,
		SourceType:           SubscriptionEntitlementSourceAdminAssign,
		SourceExternalID:     &sourceExternalID,
		ValidityDaysOverride: input.ValidityDays,
		AssignedBy:           input.AssignedBy,
		Notes:                input.Notes,
		Now:                  assignNow,
	})
	if err != nil {
		return fmt.Errorf("assign subscription entitlement alias: %w", err)
	}
	return nil
}

func (s *SubscriptionService) assignPlanEntitlementOnly(ctx context.Context, input *AssignSubscriptionInput) (*UserSubscription, error) {
	if s == nil || input == nil || input.PlanID <= 0 || !s.ShouldUseSubscriptionEntitlementAliases(ctx) {
		return nil, ErrSubscriptionEntitlementPlanRequired
	}
	ent, _, err := s.entitlementSvc.AssignOrExtendFromPlan(ctx, AssignEntitlementFromPlanInput{
		UserID:               input.UserID,
		PlanID:               input.PlanID,
		SourceType:           SubscriptionEntitlementSourceAdminAssign,
		ValidityDaysOverride: input.ValidityDays,
		AssignedBy:           input.AssignedBy,
		Notes:                input.Notes,
	})
	if err != nil {
		return nil, fmt.Errorf("assign subscription entitlement only: %w", err)
	}
	if input.GroupID > 0 {
		s.InvalidateSubCache(input.UserID, input.GroupID)
	}
	return adminSubscriptionFromEntitlement(ent), nil
}

func adminAssignEntitlementSourceExternalID(legacySubscriptionID, planID int64) string {
	return fmt.Sprintf("%s:%d:plan:%d", SubscriptionEntitlementSourceAdminAssign, legacySubscriptionID, planID)
}

func adminAssignEntitlementRenewalSourceExternalID(legacySubscriptionID, planID int64, startsAt time.Time) string {
	return fmt.Sprintf("%s:term:%d", adminAssignEntitlementSourceExternalID(legacySubscriptionID, planID), startsAt.UTC().UnixNano())
}

func planEntitlementAliasMayChange(sub *UserSubscription, outcome subscriptionAssignmentOutcome) bool {
	if sub == nil {
		return false
	}
	switch outcome {
	case subscriptionAssignmentCreated:
		return true
	case subscriptionAssignmentRenewed:
		return shouldRenewPlanEntitlementAlias(sub, outcome)
	case subscriptionAssignmentReused:
		return sub.Status == SubscriptionStatusActive && sub.EntitlementLink == nil
	default:
		return false
	}
}

func shouldRenewPlanEntitlementAlias(sub *UserSubscription, outcome subscriptionAssignmentOutcome) bool {
	if sub == nil || outcome != subscriptionAssignmentRenewed {
		return false
	}
	if sub.EntitlementLink == nil {
		return true
	}
	return sub.EntitlementLink.Status == SubscriptionStatusExpired || !sub.EntitlementLink.ExpiresAt.After(sub.StartsAt)
}

func (s *SubscriptionService) updateExistingSubscriptionTerm(
	ctx context.Context,
	subscriptionID int64,
	validityDays int,
	notes string,
	assignmentSemantics bool,
) (updated bool, err error) {
	err = s.withSubscriptionUpdateTx(ctx, func(txCtx context.Context) error {
		existingSub, err := s.userSubRepo.GetByIDForUpdate(txCtx, subscriptionID)
		if err != nil {
			return fmt.Errorf("lock subscription for renewal: %w", err)
		}

		now := time.Now()
		if s.now != nil {
			now = s.now()
		}
		if assignmentSemantics {
			if existingSub.Status == SubscriptionStatusSuspended {
				return nil
			}
			// AssignSubscription is idempotent. A stale pre-lock read may have seen
			// an expired row that another concurrent assignment has already renewed.
			// In that case, reuse the locked current term instead of extending it a
			// second time and letting the legacy row drift past its V2 entitlement.
			if existingSub.Status == SubscriptionStatusActive && existingSub.ExpiresAt.After(now) {
				return nil
			}
		}
		isExpired := !existingSub.ExpiresAt.After(now)
		if assignmentSemantics {
			isExpired = existingSub.Status == SubscriptionStatusExpired ||
				(existingSub.Status != SubscriptionStatusSuspended && !existingSub.ExpiresAt.After(now))
		}
		newExpiresAt := existingSub.ExpiresAt.AddDate(0, 0, validityDays)
		if isExpired {
			newExpiresAt = now.AddDate(0, 0, validityDays)
		}
		if newExpiresAt.After(MaxExpiresAt) {
			newExpiresAt = MaxExpiresAt
		}
		if assignmentSemantics && strings.TrimSpace(existingSub.Notes) == strings.TrimSpace(notes) {
			notes = ""
		}

		if isExpired {
			renewed := renewedSubscriptionTerm(existingSub, notes, now, newExpiresAt)
			if err := s.userSubRepo.Update(txCtx, renewed); err != nil {
				return fmt.Errorf("renew expired subscription: %w", err)
			}
			updated = true
			return nil
		}

		// 更新过期时间
		if err := s.userSubRepo.ExtendExpiry(txCtx, existingSub.ID, newExpiresAt); err != nil {
			return fmt.Errorf("extend subscription: %w", err)
		}

		// 如果订阅被暂停，恢复为 active 状态
		if existingSub.Status != SubscriptionStatusActive {
			if err := s.userSubRepo.UpdateStatus(txCtx, existingSub.ID, SubscriptionStatusActive); err != nil {
				return fmt.Errorf("update subscription status: %w", err)
			}
		}

		// 追加备注
		if notes != "" {
			if err := s.userSubRepo.UpdateNotes(txCtx, existingSub.ID, appendSubscriptionNotes(existingSub.Notes, notes)); err != nil {
				return fmt.Errorf("update subscription notes: %w", err)
			}
		}

		updated = true
		return nil
	})
	return updated, err
}

func (s *SubscriptionService) withSubscriptionUpdateTx(ctx context.Context, fn func(context.Context) error) error {
	if dbent.TxFromContext(ctx) != nil {
		return fn(ctx)
	}
	if s.entClient == nil {
		return fn(ctx)
	}
	if dbent.TxFromContext(ctx) != nil {
		return fn(ctx)
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	txCtx := dbent.NewTxContext(ctx, tx)

	if err := fn(txCtx); err != nil {
		_ = tx.Rollback()
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func renewedSubscriptionTerm(existingSub *UserSubscription, notes string, startsAt, expiresAt time.Time) *UserSubscription {
	renewed := *existingSub
	// 日窗口按日历日对齐（0 点刷新）；周/月窗口按订阅期限对齐（锚点为新周期起点）。
	dailyWindowStart := timezone.StartOfDay(startsAt)
	periodicWindowStart := startsAt
	renewed.StartsAt = startsAt
	renewed.ExpiresAt = expiresAt
	renewed.Status = SubscriptionStatusActive
	renewed.DailyWindowStart = &dailyWindowStart
	renewed.WeeklyWindowStart = &periodicWindowStart
	renewed.MonthlyWindowStart = &periodicWindowStart
	renewed.DailyUsageUSD = 0
	renewed.WeeklyUsageUSD = 0
	renewed.MonthlyUsageUSD = 0
	renewed.Notes = appendSubscriptionNotes(existingSub.Notes, notes)
	return &renewed
}

func appendSubscriptionNotes(existingNotes, newNotes string) string {
	if newNotes == "" {
		return existingNotes
	}
	if existingNotes == "" {
		return newNotes
	}
	return existingNotes + "\n" + newNotes
}

// createSubscription 创建新订阅（内部方法）
func (s *SubscriptionService) createSubscription(ctx context.Context, input *AssignSubscriptionInput) (*UserSubscription, error) {
	validityDays := input.ValidityDays
	if validityDays <= 0 {
		validityDays = 30
	}
	if validityDays > MaxValidityDays {
		validityDays = MaxValidityDays
	}

	now := time.Now()
	expiresAt := now.AddDate(0, 0, validityDays)
	if expiresAt.After(MaxExpiresAt) {
		expiresAt = MaxExpiresAt
	}
	dailyWindowStart := timezone.StartOfDay(now)
	periodicWindowStart := now

	sub := &UserSubscription{
		UserID:             input.UserID,
		GroupID:            input.GroupID,
		StartsAt:           now,
		ExpiresAt:          expiresAt,
		Status:             SubscriptionStatusActive,
		DailyWindowStart:   &dailyWindowStart,
		WeeklyWindowStart:  &periodicWindowStart,
		MonthlyWindowStart: &periodicWindowStart,
		AssignedAt:         now,
		Notes:              input.Notes,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	// 只有当 AssignedBy > 0 时才设置（0 表示系统分配，如兑换码）
	if input.AssignedBy > 0 {
		sub.AssignedBy = &input.AssignedBy
	}

	if err := s.userSubRepo.Create(ctx, sub); err != nil {
		return nil, err
	}

	// 重新获取完整订阅信息（包含关联）
	return s.userSubRepo.GetByID(ctx, sub.ID)
}

// BulkAssignSubscriptionInput 批量分配订阅输入
type BulkAssignSubscriptionInput struct {
	UserIDs      []int64
	GroupID      int64
	ValidityDays int
	AssignedBy   int64
	Notes        string
}

// BulkAssignResult 批量分配结果
type BulkAssignResult struct {
	SuccessCount  int
	CreatedCount  int
	ReusedCount   int
	FailedCount   int
	Subscriptions []UserSubscription
	Errors        []string
	Statuses      map[int64]string
}

// BulkAssignSubscription 批量分配订阅
func (s *SubscriptionService) BulkAssignSubscription(ctx context.Context, input *BulkAssignSubscriptionInput) (*BulkAssignResult, error) {
	result := &BulkAssignResult{
		Subscriptions: make([]UserSubscription, 0),
		Errors:        make([]string, 0),
		Statuses:      make(map[int64]string),
	}

	for _, userID := range input.UserIDs {
		sub, outcome, err := s.assignSubscriptionWithReuse(ctx, &AssignSubscriptionInput{
			UserID:       userID,
			GroupID:      input.GroupID,
			ValidityDays: input.ValidityDays,
			AssignedBy:   input.AssignedBy,
			Notes:        input.Notes,
		}, false)
		if err != nil {
			result.FailedCount++
			result.Errors = append(result.Errors, fmt.Sprintf("user %d: %v", userID, err))
			result.Statuses[userID] = "failed"
		} else {
			result.SuccessCount++
			result.Subscriptions = append(result.Subscriptions, *sub)
			if outcome.reused() {
				result.ReusedCount++
				result.Statuses[userID] = "reused"
			} else {
				result.CreatedCount++
				result.Statuses[userID] = "created"
			}
		}
	}

	return result, nil
}

func (s *SubscriptionService) assignSubscriptionWithReuse(ctx context.Context, input *AssignSubscriptionInput, deferCacheInvalidation bool) (*UserSubscription, subscriptionAssignmentOutcome, error) {
	// 检查分组是否存在且为订阅类型
	group, err := s.groupRepo.GetByID(ctx, input.GroupID)
	if err != nil {
		return nil, subscriptionAssignmentCreated, fmt.Errorf("group not found: %w", err)
	}
	if !group.IsSubscriptionType() {
		return nil, subscriptionAssignmentCreated, ErrGroupNotSubscriptionType
	}

	// 检查是否已存在订阅；若已存在，则按幂等成功返回现有订阅
	exists, err := s.userSubRepo.ExistsByUserIDAndGroupID(ctx, input.UserID, input.GroupID)
	if err != nil {
		return nil, subscriptionAssignmentCreated, err
	}
	if exists {
		sub, getErr := s.userSubRepo.GetByUserIDAndGroupID(ctx, input.UserID, input.GroupID)
		if getErr != nil {
			return nil, subscriptionAssignmentCreated, getErr
		}
		if conflictReason, conflict := detectAssignPlanConflict(sub, input); conflict {
			return nil, subscriptionAssignmentCreated, ErrSubscriptionAssignConflict.WithMetadata(map[string]string{
				"conflict_reason": conflictReason,
			})
		}
		now := time.Now()
		if sub.Status == SubscriptionStatusExpired ||
			(sub.Status != SubscriptionStatusSuspended && !sub.ExpiresAt.After(now)) {
			validityDays := normalizeAssignValidityDays(input.ValidityDays)
			updated, err := s.updateExistingSubscriptionTerm(ctx, sub.ID, validityDays, input.Notes, true)
			if err != nil {
				return nil, subscriptionAssignmentCreated, err
			}
			if !updated {
				current, getErr := s.userSubRepo.GetByID(ctx, sub.ID)
				return current, subscriptionAssignmentReused, getErr
			}
			s.maybeInvalidateAssignmentCaches(input.UserID, input.GroupID, deferCacheInvalidation)
			renewed, getErr := s.userSubRepo.GetByID(ctx, sub.ID)
			return renewed, subscriptionAssignmentRenewed, getErr
		}
		if conflictReason, conflict := detectAssignSemanticConflict(sub, input); conflict {
			return nil, subscriptionAssignmentCreated, ErrSubscriptionAssignConflict.WithMetadata(map[string]string{
				"conflict_reason": conflictReason,
			})
		}
		return sub, subscriptionAssignmentReused, nil
	}

	sub, err := s.createSubscription(ctx, input)
	if err != nil {
		return nil, subscriptionAssignmentCreated, err
	}

	// 失效订阅缓存
	s.maybeInvalidateAssignmentCaches(input.UserID, input.GroupID, deferCacheInvalidation)

	return sub, subscriptionAssignmentCreated, nil
}

func detectAssignPlanConflict(existing *UserSubscription, input *AssignSubscriptionInput) (string, bool) {
	if existing == nil || input == nil || input.PlanID <= 0 || existing.EntitlementLink == nil {
		return "", false
	}
	if existing.EntitlementLink.PlanID == nil || *existing.EntitlementLink.PlanID != input.PlanID {
		return "plan_id_mismatch", true
	}
	return "", false
}

func detectAssignSemanticConflict(existing *UserSubscription, input *AssignSubscriptionInput) (string, bool) {
	if existing == nil || input == nil {
		return "", false
	}

	if conflictReason, conflict := detectAssignPlanConflict(existing, input); conflict {
		return conflictReason, true
	}

	normalizedDays := normalizeAssignValidityDays(input.ValidityDays)
	if !existing.StartsAt.IsZero() {
		expectedExpiresAt := existing.StartsAt.AddDate(0, 0, normalizedDays)
		if expectedExpiresAt.After(MaxExpiresAt) {
			expectedExpiresAt = MaxExpiresAt
		}
		if !existing.ExpiresAt.Equal(expectedExpiresAt) {
			return "validity_days_mismatch", true
		}
	}

	existingNotes := strings.TrimSpace(existing.Notes)
	inputNotes := strings.TrimSpace(input.Notes)
	if existingNotes != inputNotes {
		return "notes_mismatch", true
	}

	return "", false
}

func normalizeAssignValidityDays(days int) int {
	if days <= 0 {
		days = 30
	}
	if days > MaxValidityDays {
		days = MaxValidityDays
	}
	return days
}

// RevokeSubscription 撤销订阅
func (s *SubscriptionService) RevokeSubscription(ctx context.Context, subscriptionID int64) error {
	if entitlementID, ok := syntheticEntitlementIDFromSubscriptionID(subscriptionID); ok {
		return s.adminRevokeEntitlement(ctx, entitlementID)
	}

	// 先获取订阅信息用于失效缓存
	sub, err := s.userSubRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return err
	}
	if sub.EntitlementLink != nil && sub.EntitlementLink.EntitlementID > 0 {
		if err := s.revokeLinkedEntitlementSubscription(ctx, sub); err != nil {
			return err
		}
		return nil
	}

	if err := s.userSubRepo.Delete(ctx, subscriptionID); err != nil {
		return err
	}

	s.bestEffortInvalidateSubscriptionCachesBefore("revoke subscription", sub.UserID, sub.GroupID, subscriptionCacheVersion(sub))
	return nil
}

func (s *SubscriptionService) revokeLinkedEntitlementSubscription(ctx context.Context, sub *UserSubscription) error {
	if sub == nil || sub.EntitlementLink == nil || sub.EntitlementLink.EntitlementID <= 0 {
		return ErrSubscriptionNotFound
	}
	entitlementSvc, err := s.adminEntitlementService()
	if err != nil {
		return err
	}

	if err := s.withSubscriptionUpdateTx(ctx, func(txCtx context.Context) error {
		return entitlementSvc.withLockedEntitlement(txCtx, sub.EntitlementLink.EntitlementID, sub.UserID, func(lockedCtx context.Context, ent *SubscriptionEntitlement) error {
			if ent.LegacySubscriptionID != nil && *ent.LegacySubscriptionID != sub.ID {
				return ErrSubscriptionNotFound
			}
			// Older linked rows may only carry the alias -> entitlement edge. Use the
			// exact alias selected by the caller while repairing that legacy shape.
			linked := *ent
			if linked.LegacySubscriptionID == nil {
				legacyID := sub.ID
				linked.LegacySubscriptionID = &legacyID
			}
			now := entitlementSvc.inputNow(time.Time{})
			return entitlementSvc.revokeEntitlementAndLinkedAliasLocked(lockedCtx, &linked, now, "revoked by admin")
		})
	}); err != nil {
		return err
	}

	s.bestEffortInvalidateSubscriptionCachesBefore("revoke linked entitlement subscription", sub.UserID, sub.GroupID, subscriptionCacheVersion(sub))

	return nil
}

func (s *SubscriptionService) RevokeUserSubscription(ctx context.Context, userID, subscriptionID int64) error {
	if userID <= 0 || subscriptionID <= 0 {
		return ErrSubscriptionNotFound
	}
	sub, err := s.userSubRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return err
	}
	if sub.UserID != userID {
		return ErrSubscriptionNotFound
	}
	return s.RevokeSubscription(ctx, subscriptionID)
}

// RestoreSubscription 恢复已撤销订阅
func (s *SubscriptionService) RestoreSubscription(ctx context.Context, subscriptionID int64) (*UserSubscription, error) {
	if entitlementID, ok := syntheticEntitlementIDFromSubscriptionID(subscriptionID); ok {
		return s.adminRestoreEntitlement(ctx, entitlementID)
	}

	sub, err := s.userSubRepo.GetByIDIncludeDeleted(ctx, subscriptionID)
	if err != nil {
		return nil, err
	}
	if sub.DeletedAt == nil {
		return nil, ErrSubscriptionNotRevoked
	}

	restoredStatus := sub.Status
	now := time.Now()
	if restoredStatus == SubscriptionStatusActive && !sub.ExpiresAt.After(now) {
		restoredStatus = SubscriptionStatusExpired
	}

	var restored *UserSubscription
	if sub.EntitlementLink != nil && sub.EntitlementLink.EntitlementID > 0 {
		restored, err = s.restoreLinkedEntitlementSubscription(ctx, sub)
	} else {
		exists, existsErr := s.userSubRepo.ExistsActiveByUserIDAndGroupID(ctx, sub.UserID, sub.GroupID)
		if existsErr != nil {
			return nil, existsErr
		}
		if exists {
			return nil, ErrSubscriptionRestoreConflict
		}
		restored, err = s.userSubRepo.Restore(ctx, subscriptionID, restoredStatus)
	}
	if err != nil {
		return nil, err
	}

	s.bestEffortInvalidateSubscriptionCachesBefore("restore subscription", restored.UserID, restored.GroupID, 0)
	return restored, nil
}

// ExtendSubscription 调整订阅时长（正数延长，负数缩短）
func (s *SubscriptionService) restoreLinkedEntitlementSubscription(ctx context.Context, sub *UserSubscription) (*UserSubscription, error) {
	if sub == nil || sub.EntitlementLink == nil || sub.EntitlementLink.EntitlementID <= 0 {
		return nil, ErrSubscriptionNotFound
	}
	entitlementSvc, err := s.adminEntitlementService()
	if err != nil {
		return nil, err
	}

	var restored *UserSubscription
	if err := s.withSubscriptionUpdateTx(ctx, func(txCtx context.Context) error {
		return entitlementSvc.withLockedEntitlement(txCtx, sub.EntitlementLink.EntitlementID, sub.UserID, func(lockedCtx context.Context, ent *SubscriptionEntitlement) error {
			currentSub, err := s.userSubRepo.GetByIDIncludeDeletedForUpdate(lockedCtx, sub.ID)
			if err != nil {
				return err
			}
			if currentSub.EntitlementLink == nil || currentSub.EntitlementLink.EntitlementID != ent.ID {
				return ErrSubscriptionNotFound
			}
			// A concurrent purchase may already have restored this exact linked
			// alias after the initial deleted-row read. Treat that outcome as the
			// same successful restore, while retaining conflicts for a different
			// active subscription below.
			if currentSub.DeletedAt == nil {
				restored = currentSub
				return nil
			}
			exists, existsErr := s.userSubRepo.ExistsActiveByUserIDAndGroupID(lockedCtx, currentSub.UserID, currentSub.GroupID)
			if existsErr != nil {
				return existsErr
			}
			if exists {
				return ErrSubscriptionRestoreConflict
			}

			restoredStatus := currentSub.Status
			now := entitlementSvc.inputNow(time.Time{})
			if restoredStatus == SubscriptionStatusActive && !currentSub.ExpiresAt.After(now) {
				restoredStatus = SubscriptionStatusExpired
			}
			lifecycle := UserSubscriptionLifecycleState{
				StartsAt:           currentSub.StartsAt,
				ExpiresAt:          currentSub.ExpiresAt,
				Status:             restoredStatus,
				DailyWindowStart:   currentSub.DailyWindowStart,
				WeeklyWindowStart:  currentSub.WeeklyWindowStart,
				MonthlyWindowStart: currentSub.MonthlyWindowStart,
				DailyUsageUSD:      currentSub.DailyUsageUSD,
				WeeklyUsageUSD:     currentSub.WeeklyUsageUSD,
				MonthlyUsageUSD:    currentSub.MonthlyUsageUSD,
			}
			if ent.Status == SubscriptionStatusRevoked {
				startsAt := lifecycle.StartsAt
				if startsAt.IsZero() {
					startsAt = ent.StartsAt
				}
				expiresAt := lifecycle.ExpiresAt
				if expiresAt.IsZero() {
					expiresAt = ent.ExpiresAt
				}
				notes := appendSubscriptionNotes(ent.Notes, "restored by admin")
				if err := entitlementSvc.entitlementRepo.UpdateTerm(lockedCtx, ent.ID, startsAt, expiresAt, restoredStatus, notes); err != nil {
					return err
				}
				// The deleted alias is the pre-revoke lifecycle snapshot. Keep its
				// windows and usage so restoring cannot reduce already accrued usage.
				lifecycle.StartsAt = startsAt
				lifecycle.ExpiresAt = expiresAt
			} else {
				// A concurrent purchase may have reactivated and extended the entitlement
				// after the initial alias read. The locked entitlement is the source of truth.
				lifecycle = UserSubscriptionLifecycleState{
					StartsAt:           ent.StartsAt,
					ExpiresAt:          ent.ExpiresAt,
					Status:             ent.Status,
					DailyWindowStart:   ent.DailyWindowStart,
					WeeklyWindowStart:  ent.WeeklyWindowStart,
					MonthlyWindowStart: ent.MonthlyWindowStart,
					DailyUsageUSD:      ent.DailyUsageUSD,
					WeeklyUsageUSD:     ent.WeeklyUsageUSD,
					MonthlyUsageUSD:    ent.MonthlyUsageUSD,
				}
			}

			restored, err = s.userSubRepo.RestoreWithLifecycle(lockedCtx, currentSub.ID, lifecycle)
			return err
		})
	}); err != nil {
		return nil, err
	}
	return restored, nil
}

func (s *SubscriptionService) ExtendSubscription(ctx context.Context, subscriptionID int64, days int) (*UserSubscription, error) {
	if entitlementID, ok := syntheticEntitlementIDFromSubscriptionID(subscriptionID); ok {
		return s.adminAdjustEntitlement(ctx, entitlementID, days)
	}

	sub, err := s.userSubRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return nil, ErrSubscriptionNotFound
	}
	if sub.EntitlementLink != nil && sub.EntitlementLink.EntitlementID > 0 {
		return s.adminAdjustEntitlement(ctx, sub.EntitlementLink.EntitlementID, days)
	}

	// 限制调整天数范围
	if days > MaxValidityDays {
		days = MaxValidityDays
	}
	if days < -MaxValidityDays {
		days = -MaxValidityDays
	}

	now := time.Now()
	isExpired := !sub.ExpiresAt.After(now)

	// 如果订阅已过期，不允许负向调整
	if isExpired && days < 0 {
		return nil, infraerrors.BadRequest("CANNOT_SHORTEN_EXPIRED", "cannot shorten an expired subscription")
	}

	// 计算新的过期时间
	var newExpiresAt time.Time
	if isExpired {
		// 已过期：从当前时间开始增加天数
		newExpiresAt = now.AddDate(0, 0, days)
	} else {
		// 未过期：从原过期时间增加/减少天数
		newExpiresAt = sub.ExpiresAt.AddDate(0, 0, days)
	}

	if newExpiresAt.After(MaxExpiresAt) {
		newExpiresAt = MaxExpiresAt
	}

	// 检查新的过期时间必须大于当前时间
	if !newExpiresAt.After(now) {
		return nil, ErrAdjustWouldExpire
	}

	if err := s.userSubRepo.ExtendExpiry(ctx, subscriptionID, newExpiresAt); err != nil {
		return nil, err
	}

	// 如果订阅已过期，恢复为active状态
	if sub.Status == SubscriptionStatusExpired {
		if err := s.userSubRepo.UpdateStatus(ctx, subscriptionID, SubscriptionStatusActive); err != nil {
			return nil, err
		}
	}

	// 失效订阅缓存
	s.InvalidateSubCache(sub.UserID, sub.GroupID)
	if s.billingCacheService != nil {
		userID, groupID, staleVersion := sub.UserID, sub.GroupID, subscriptionCacheVersion(sub)
		go func() {
			cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.billingCacheService.InvalidateSubscriptionBefore(cacheCtx, userID, groupID, staleVersion)
		}()
	}

	return s.userSubRepo.GetByID(ctx, subscriptionID)
}

// GetByID 根据ID获取订阅
func (s *SubscriptionService) GetByID(ctx context.Context, id int64) (*UserSubscription, error) {
	return s.userSubRepo.GetByID(ctx, id)
}

// GetActiveSubscription 获取用户对特定分组的有效订阅
// 使用 L1 缓存 + singleflight 加速中间件热路径。
// 返回缓存对象的浅拷贝，调用方可安全修改字段而不会污染缓存或触发 data race。
func (s *SubscriptionService) GetActiveSubscription(ctx context.Context, userID, groupID int64) (*UserSubscription, error) {
	key := subCacheKey(userID, groupID)

	// L1 缓存命中：返回浅拷贝
	if s.subCacheL1 != nil {
		if v, ok := s.subCacheL1.Get(key); ok {
			if sub, ok := v.(*UserSubscription); ok {
				cp := *sub
				return &cp, nil
			}
		}
	}

	// singleflight 防止并发击穿
	value, err, _ := s.subCacheGroup.Do(key, func() (any, error) {
		sub, err := s.userSubRepo.GetActiveByUserIDAndGroupID(ctx, userID, groupID)
		if err != nil {
			return nil, err // 直接透传 repo 已翻译的错误（NotFound → ErrSubscriptionNotFound，其他错误原样返回）
		}
		// 写入 L1 缓存
		if s.subCacheL1 != nil {
			_ = s.subCacheL1.SetWithTTL(key, sub, 1, s.jitteredTTL(s.subCacheTTL))
		}
		return sub, nil
	})
	if err != nil {
		return nil, err
	}
	// singleflight 返回的也是缓存指针，需要浅拷贝
	sub, ok := value.(*UserSubscription)
	if !ok || sub == nil {
		return nil, ErrSubscriptionNotFound
	}
	cp := *sub
	return &cp, nil
}

func (s *SubscriptionService) ResolveUsableSubscriptionForAPIKey(ctx context.Context, apiKey *APIKey) (*SubscriptionSwitchCandidate, error) {
	return s.ResolveUsableSubscriptionForAPIKeyWithRequest(ctx, apiKey, SubscriptionSwitchRequest{})
}

func (s *SubscriptionService) ResolveUsableSubscriptionForAPIKeyWithRequest(ctx context.Context, apiKey *APIKey, req SubscriptionSwitchRequest) (*SubscriptionSwitchCandidate, error) {
	if apiKey == nil || apiKey.User == nil || apiKey.Group == nil || !apiKey.Group.IsSubscriptionType() {
		return nil, nil
	}

	fromGroupID := apiKey.Group.ID
	currentSub, currentErr := s.GetActiveSubscription(ctx, apiKey.User.ID, fromGroupID)
	if currentErr == nil {
		validateErr := subscriptionSwitchRequestEligibilityError(apiKey.Group, req)
		if validateErr == nil {
			validateErr = s.validateSwitchCandidate(ctx, apiKey.User.ID, currentSub, apiKey.Group)
		}
		if validateErr == nil {
			return &SubscriptionSwitchCandidate{
				Subscription: currentSub,
				Group:        apiKey.Group,
				FromGroupID:  fromGroupID,
				ToGroupID:    fromGroupID,
			}, nil
		}
		currentErr = validateErr
	}

	if !apiKey.AutoSwitchGroupEnabled || !isAutoSwitchableSubscriptionError(currentErr) {
		return nil, currentErr
	}

	preferences := s.loadSubscriptionGroupPreferencesCached(ctx, apiKey.User.ID)
	candidates, err := s.listAutoSwitchCandidates(ctx, apiKey.User.ID, fromGroupID, apiKey.Group, req, preferences, false)
	if err != nil {
		return nil, err
	}
	var fromSubscriptionID *int64
	if currentSub != nil {
		id := currentSub.ID
		fromSubscriptionID = &id
	}
	reason := subscriptionSwitchReason(currentErr)
	var maintenanceErr error
	for i := range candidates {
		sub := &candidates[i]
		if sub.GroupID == fromGroupID || sub.Group == nil || !sub.Group.IsSubscriptionType() || !sub.Group.IsActive() {
			continue
		}
		if !apiKey.User.AllowsGroupByPolicy(sub.Group.ID, sub.Group.IsExclusive) {
			continue
		}
		validateErr := s.validateSwitchCandidate(ctx, apiKey.User.ID, sub, sub.Group)
		if validateErr != nil {
			if errors.Is(validateErr, ErrSubscriptionMaintenance) {
				maintenanceErr = validateErr
				continue
			}
			if !isAutoSwitchableSubscriptionError(validateErr) {
				continue
			}
			continue
		}
		return &SubscriptionSwitchCandidate{
			Subscription:       sub,
			Group:              sub.Group,
			Switched:           true,
			FromGroupID:        fromGroupID,
			ToGroupID:          sub.GroupID,
			FromSubscriptionID: fromSubscriptionID,
			Reason:             reason,
		}, nil
	}

	if maintenanceErr != nil {
		return nil, maintenanceErr
	}
	return nil, currentErr
}

func (s *SubscriptionService) validateSwitchCandidate(ctx context.Context, userID int64, sub *UserSubscription, group *Group) error {
	if sub == nil || group == nil {
		return ErrSubscriptionNotFound
	}

	needsMaintenance := sub.NeedsDailyReset() ||
		sub.NeedsWeeklyReset() ||
		sub.NeedsMonthlyReset() ||
		!sub.IsWindowActivated()
	if needsMaintenance {
		refreshed, err := s.EnsureWindowMaintenance(ctx, sub)
		if err != nil {
			return err
		}
		*sub = *refreshed
	}

	if s.billingCacheService == nil {
		_, err := s.ValidateAndCheckLimits(sub, group)
		return err
	}

	subData, err := s.billingCacheService.GetSubscriptionStatus(ctx, userID, group.ID)
	if err != nil {
		if s.billingCacheService.circuitBreaker != nil {
			s.billingCacheService.circuitBreaker.OnFailure(err)
		}
		log.Printf("ALERT: billing subscription switch check failed for user %d group %d: %v", userID, group.ID, err)
		return ErrBillingServiceUnavailable.WithCause(err)
	}
	if s.billingCacheService.circuitBreaker != nil {
		s.billingCacheService.circuitBreaker.OnSuccess()
	}

	applySubscriptionCacheSnapshot(sub, subData)
	return subscriptionSnapshotEligibilityError(group, subData)
}

func applySubscriptionCacheSnapshot(sub *UserSubscription, data *subscriptionCacheData) {
	if sub == nil || data == nil {
		return
	}
	sub.Status = data.Status
	sub.ExpiresAt = data.ExpiresAt
	sub.DailyUsageUSD = data.DailyUsage
	sub.WeeklyUsageUSD = data.WeeklyUsage
	sub.MonthlyUsageUSD = data.MonthlyUsage
	if data.Version > 0 {
		sub.UpdatedAt = time.UnixMicro(data.Version)
	}
}

func subscriptionSnapshotEligibilityError(group *Group, data *subscriptionCacheData) error {
	if group == nil || data == nil {
		return ErrSubscriptionNotFound
	}
	switch data.Status {
	case SubscriptionStatusActive:
	case SubscriptionStatusExpired:
		return ErrSubscriptionExpired
	case SubscriptionStatusSuspended:
		return ErrSubscriptionSuspended
	default:
		return ErrSubscriptionInvalid
	}
	if time.Now().After(data.ExpiresAt) {
		return ErrSubscriptionExpired
	}
	if group.HasDailyLimit() && data.DailyUsage >= *group.DailyLimitUSD {
		return ErrDailyLimitExceeded
	}
	if group.HasWeeklyLimit() && data.WeeklyUsage >= *group.WeeklyLimitUSD {
		return ErrWeeklyLimitExceeded
	}
	if group.HasMonthlyLimit() && data.MonthlyUsage >= *group.MonthlyLimitUSD {
		return ErrMonthlyLimitExceeded
	}
	return nil
}

func isAutoSwitchableSubscriptionError(err error) bool {
	return err == nil ||
		errors.Is(err, ErrSubscriptionNotFound) ||
		errors.Is(err, ErrSubscriptionExpired) ||
		errors.Is(err, ErrDailyLimitExceeded) ||
		errors.Is(err, ErrWeeklyLimitExceeded) ||
		errors.Is(err, ErrMonthlyLimitExceeded) ||
		errors.Is(err, ErrSubscriptionEndpointUnsupported) ||
		strings.Contains(err.Error(), "SUBSCRIPTION_NOT_FOUND") ||
		strings.Contains(err.Error(), "SUBSCRIPTION_EXPIRED") ||
		strings.Contains(err.Error(), "DAILY_LIMIT_EXCEEDED") ||
		strings.Contains(err.Error(), "WEEKLY_LIMIT_EXCEEDED") ||
		strings.Contains(err.Error(), "MONTHLY_LIMIT_EXCEEDED") ||
		strings.Contains(err.Error(), "SUBSCRIPTION_ENDPOINT_UNSUPPORTED")
}

func subscriptionSwitchReason(err error) string {
	if err == nil {
		return "unknown"
	}
	switch {
	case errors.Is(err, ErrSubscriptionNotFound) || strings.Contains(err.Error(), "SUBSCRIPTION_NOT_FOUND"):
		return "subscription_not_found"
	case errors.Is(err, ErrSubscriptionExpired) || strings.Contains(err.Error(), "SUBSCRIPTION_EXPIRED"):
		return "subscription_expired"
	case errors.Is(err, ErrDailyLimitExceeded) || strings.Contains(err.Error(), "DAILY_LIMIT_EXCEEDED"):
		return "daily_limit_exceeded"
	case errors.Is(err, ErrWeeklyLimitExceeded) || strings.Contains(err.Error(), "WEEKLY_LIMIT_EXCEEDED"):
		return "weekly_limit_exceeded"
	case errors.Is(err, ErrMonthlyLimitExceeded) || strings.Contains(err.Error(), "MONTHLY_LIMIT_EXCEEDED"):
		return "monthly_limit_exceeded"
	case errors.Is(err, ErrSubscriptionEndpointUnsupported) || strings.Contains(err.Error(), "SUBSCRIPTION_ENDPOINT_UNSUPPORTED"):
		return "endpoint_unsupported"
	default:
		return "subscription_unavailable"
	}
}

func (s *SubscriptionService) listAutoSwitchCandidates(ctx context.Context, userID, currentGroupID int64, currentGroup *Group, req SubscriptionSwitchRequest, preferences map[int64]subscriptionGroupPreferenceRank, includeCurrent bool) ([]UserSubscription, error) {
	subs, err := s.listActiveSubscriptionsForSwitch(ctx, userID)
	if err != nil {
		return nil, err
	}
	if preferences == nil {
		preferences = map[int64]subscriptionGroupPreferenceRank{}
	}
	const unranked = int(^uint(0) >> 1)
	sort.SliceStable(subs, func(i, j int) bool {
		pi, okI := preferences[subs[i].GroupID]
		ri := unranked
		if !okI {
			ri = unranked
		} else if pi.Enabled {
			ri = pi.SortOrder
		}
		pj, okJ := preferences[subs[j].GroupID]
		rj := unranked
		if !okJ {
			rj = unranked
		} else if pj.Enabled {
			rj = pj.SortOrder
		}
		if ri != rj {
			return ri < rj
		}
		if !subs[i].ExpiresAt.Equal(subs[j].ExpiresAt) {
			return subs[i].ExpiresAt.Before(subs[j].ExpiresAt)
		}
		var si, sj int
		if subs[i].Group != nil {
			si = subs[i].Group.SortOrder
		}
		if subs[j].Group != nil {
			sj = subs[j].Group.SortOrder
		}
		if si != sj {
			return si < sj
		}
		return subs[i].ID < subs[j].ID
	})
	filtered := subs[:0]
	for i := range subs {
		group := subs[i].Group
		if group == nil {
			continue
		}
		if !includeCurrent && group.ID == currentGroupID {
			continue
		}
		if pref, ok := preferences[group.ID]; ok && !pref.Enabled {
			continue
		}
		if err := subscriptionSwitchRequestEligibilityError(group, req); err != nil {
			continue
		}
		if currentGroup != nil && currentGroup.Platform != "" && group.Platform != currentGroup.Platform {
			continue
		}
		if !subscriptionSwitchGroupsCompatible(currentGroup, group, req) {
			continue
		}
		filtered = append(filtered, subs[i])
	}
	return filtered, nil
}

func (s *SubscriptionService) listActiveSubscriptionsForSwitch(ctx context.Context, userID int64) ([]UserSubscription, error) {
	if s == nil || userID <= 0 {
		return nil, ErrSubscriptionNotFound
	}
	key := activeSubscriptionsCacheKey(userID)
	if s.subCacheL1 != nil {
		if v, ok := s.subCacheL1.Get(key); ok {
			if cached, ok := v.([]UserSubscription); ok {
				return cloneUserSubscriptions(cached), nil
			}
		}
	}
	subs, err := s.userSubRepo.ListActiveByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	subs = cloneUserSubscriptions(subs)
	if s.subCacheL1 != nil {
		_ = s.subCacheL1.SetWithTTL(key, cloneUserSubscriptions(subs), 1, s.jitteredTTL(s.subCacheTTL))
	}
	return subs, nil
}

func cloneUserSubscriptions(in []UserSubscription) []UserSubscription {
	out := make([]UserSubscription, len(in))
	for i := range in {
		out[i] = in[i]
		if in[i].Group != nil {
			group := *in[i].Group
			out[i].Group = &group
		}
		if in[i].User != nil {
			user := *in[i].User
			out[i].User = &user
		}
	}
	return out
}

func subscriptionSwitchGroupsCompatible(currentGroup, candidateGroup *Group, req SubscriptionSwitchRequest) bool {
	if currentGroup == nil || candidateGroup == nil {
		return false
	}
	if currentGroup.Platform != "" && candidateGroup.Platform != currentGroup.Platform {
		return false
	}

	switch currentGroup.Platform {
	case PlatformOpenAI:
		if normalizeSubscriptionSwitchEndpoint(req.InboundEndpoint) != "" {
			return true
		}
		return currentGroup.AllowMessagesDispatch == candidateGroup.AllowMessagesDispatch
	case PlatformAntigravity:
		return true
	default:
		return true
	}
}

func subscriptionSwitchRequestEligibilityError(group *Group, req SubscriptionSwitchRequest) error {
	if group == nil {
		return ErrSubscriptionNotFound
	}
	endpoint := normalizeSubscriptionSwitchEndpoint(req.InboundEndpoint)
	if endpoint == "" {
		return nil
	}
	switch group.Platform {
	case PlatformOpenAI:
		if endpoint == "/v1/messages" && !group.AllowMessagesDispatch {
			return ErrSubscriptionEndpointUnsupported
		}
		if endpoint != "/v1/messages" && group.ClaudeCodeOnly {
			return ErrSubscriptionEndpointUnsupported
		}
	case PlatformAntigravity:
		requiredScope := subscriptionSwitchAntigravityScope(endpoint)
		if requiredScope != "" && !subscriptionSwitchGroupSupportsScope(group, requiredScope) {
			return ErrSubscriptionEndpointUnsupported
		}
	}
	return nil
}

func subscriptionSwitchAntigravityScope(endpoint string) string {
	switch normalizeSubscriptionSwitchEndpoint(endpoint) {
	case "/v1/messages":
		return "claude"
	case "/v1beta/models":
		return "gemini_text"
	default:
		return ""
	}
}

func subscriptionSwitchGroupSupportsScope(group *Group, requiredScope string) bool {
	if group == nil || requiredScope == "" || len(group.SupportedModelScopes) == 0 {
		return true
	}
	for _, scope := range group.SupportedModelScopes {
		if strings.EqualFold(strings.TrimSpace(scope), requiredScope) {
			return true
		}
	}
	return false
}

func (s *SubscriptionService) loadSubscriptionGroupPreferencesCached(ctx context.Context, userID int64) map[int64]subscriptionGroupPreferenceRank {
	if s == nil || userID <= 0 {
		return map[int64]subscriptionGroupPreferenceRank{}
	}
	key := subscriptionGroupPreferenceCacheKey(userID)
	if s.subCacheL1 != nil {
		if v, ok := s.subCacheL1.Get(key); ok {
			if cached, ok := v.(map[int64]subscriptionGroupPreferenceRank); ok {
				return cloneSubscriptionGroupPreferences(cached)
			}
		}
	}
	preferences := s.loadSubscriptionGroupPreferences(ctx, userID)
	if s.subCacheL1 != nil {
		_ = s.subCacheL1.SetWithTTL(key, cloneSubscriptionGroupPreferences(preferences), 1, s.jitteredTTL(s.subCacheTTL))
	}
	return preferences
}

func cloneSubscriptionGroupPreferences(in map[int64]subscriptionGroupPreferenceRank) map[int64]subscriptionGroupPreferenceRank {
	out := make(map[int64]subscriptionGroupPreferenceRank, len(in))
	for groupID, pref := range in {
		out[groupID] = pref
	}
	return out
}

func (s *SubscriptionService) invalidateSubscriptionGroupPreferencesCache(userID int64) {
	if s == nil || s.subCacheL1 == nil || userID <= 0 {
		return
	}
	s.subCacheL1.Del(subscriptionGroupPreferenceCacheKey(userID))
	s.subCacheL1.Wait()
}

func NewSubscriptionSwitchRequestFromPath(path string) SubscriptionSwitchRequest {
	return SubscriptionSwitchRequest{InboundEndpoint: normalizeSubscriptionSwitchEndpoint(path)}
}

func normalizeSubscriptionSwitchEndpoint(path string) string {
	path = strings.TrimSpace(strings.ToLower(path))
	switch {
	case strings.Contains(path, "/v1/chat/completions"):
		return "/v1/chat/completions"
	case strings.Contains(path, "/v1/messages"):
		return "/v1/messages"
	case strings.Contains(path, "/images/generations"):
		return "/v1/images/generations"
	case strings.Contains(path, "/images/edits"):
		return "/v1/images/edits"
	case strings.Contains(path, "/responses"):
		return "/v1/responses"
	case strings.Contains(path, "/v1beta/models"):
		return "/v1beta/models"
	default:
		return path
	}
}

func (s *SubscriptionService) loadSubscriptionGroupPreferences(ctx context.Context, userID int64) map[int64]subscriptionGroupPreferenceRank {
	out := make(map[int64]subscriptionGroupPreferenceRank)
	if s == nil || s.entClient == nil || userID <= 0 {
		return out
	}
	rows, err := s.entClient.QueryContext(ctx, `
		SELECT group_id, sort_order, enabled
		FROM user_subscription_group_preferences
		WHERE user_id = $1
		ORDER BY sort_order ASC, group_id ASC
	`, userID)
	if err != nil {
		log.Printf("Warning: failed to load subscription group preferences for user %d: %v", userID, err)
		return out
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var groupID int64
		var sortOrder int
		var enabled bool
		if err := rows.Scan(&groupID, &sortOrder, &enabled); err == nil {
			out[groupID] = subscriptionGroupPreferenceRank{SortOrder: sortOrder, Enabled: enabled}
		}
	}
	if err := rows.Err(); err != nil {
		log.Printf("Warning: failed to read subscription group preferences for user %d: %v", userID, err)
	}
	return out
}

func (s *SubscriptionService) LogAutoSwitch(ctx context.Context, apiKey *APIKey, candidate *SubscriptionSwitchCandidate) {
	if s == nil || s.entClient == nil || apiKey == nil || candidate == nil || candidate.Subscription == nil {
		return
	}
	_, _ = s.entClient.ExecContext(ctx, `
		INSERT INTO api_key_auto_switch_logs (
			api_key_id, user_id, from_group_id, to_group_id, from_subscription_id, to_subscription_id, reason, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
	`, apiKey.ID, apiKey.UserID, candidate.FromGroupID, candidate.ToGroupID, candidate.FromSubscriptionID, candidate.Subscription.ID, candidate.Reason)
}

func (s *SubscriptionService) ListGroupPreferences(ctx context.Context, userID int64) ([]SubscriptionGroupPreference, error) {
	if s == nil || s.entClient == nil {
		return nil, nil
	}
	rows, err := s.entClient.QueryContext(ctx, `
		SELECT group_id, sort_order, enabled
		FROM user_subscription_group_preferences
		WHERE user_id = $1
		ORDER BY sort_order ASC, group_id ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]SubscriptionGroupPreference, 0)
	for rows.Next() {
		var pref SubscriptionGroupPreference
		if err := rows.Scan(&pref.GroupID, &pref.SortOrder, &pref.Enabled); err != nil {
			return nil, err
		}
		out = append(out, pref)
	}
	return out, rows.Err()
}

func (s *SubscriptionService) SaveGroupPreferences(ctx context.Context, userID int64, prefs []SubscriptionGroupPreference) ([]SubscriptionGroupPreference, error) {
	if s == nil || s.entClient == nil {
		return nil, nil
	}
	activeSubs, err := s.ListActiveUserSubscriptions(ctx, userID)
	if err != nil {
		return nil, err
	}
	activeGroupIDs := make(map[int64]struct{}, len(activeSubs))
	for i := range activeSubs {
		activeGroupIDs[activeSubs[i].GroupID] = struct{}{}
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, err
	}
	txClient := tx.Client()
	_, err = txClient.ExecContext(ctx, `DELETE FROM user_subscription_group_preferences WHERE user_id = $1`, userID)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	saved := make([]SubscriptionGroupPreference, 0, len(prefs))
	seenGroupIDs := make(map[int64]struct{}, len(prefs))
	for _, pref := range prefs {
		if pref.GroupID <= 0 {
			continue
		}
		if _, seen := seenGroupIDs[pref.GroupID]; seen {
			continue
		}
		seenGroupIDs[pref.GroupID] = struct{}{}
		if _, ok := activeGroupIDs[pref.GroupID]; !ok {
			continue
		}
		pref.SortOrder = len(saved)
		if !pref.Enabled {
			pref.Enabled = false
		} else {
			pref.Enabled = true
		}
		if _, err := txClient.ExecContext(ctx, `
			INSERT INTO user_subscription_group_preferences (user_id, group_id, sort_order, enabled, created_at, updated_at)
			VALUES ($1, $2, $3, $4, NOW(), NOW())
		`, userID, pref.GroupID, pref.SortOrder, pref.Enabled); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
		saved = append(saved, pref)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	s.invalidateSubscriptionGroupPreferencesCache(userID)
	return saved, nil
}

func (s *SubscriptionService) AdvanceMonthlyCycle(ctx context.Context, userID, subscriptionID int64) (*AdvanceMonthlyCycleResult, error) {
	sub, err := s.userSubRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return nil, err
	}
	if sub.UserID != userID {
		return nil, ErrSubscriptionNotFound
	}
	group := sub.Group
	if group == nil {
		group, err = s.groupRepo.GetByID(ctx, sub.GroupID)
		if err != nil {
			return nil, err
		}
	}
	if group == nil || !group.HasMonthlyLimit() {
		return nil, ErrMonthlyCycleNotExhausted
	}
	now := time.Now().Truncate(time.Second)

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, err
	}
	txClient := tx.Client()

	var previousUsage float64
	var previousWindowStart sql.NullTime
	var previousStartsAt time.Time
	var previousExpiresAt time.Time
	var previousStatus string
	var previousUpdatedAt sql.NullTime
	rows, err := txClient.QueryContext(ctx, `
		SELECT monthly_usage_usd, monthly_window_start, starts_at, expires_at, status, updated_at
		FROM user_subscriptions
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
		FOR UPDATE
	`, subscriptionID, userID)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if !rows.Next() {
		_ = rows.Close()
		_ = tx.Rollback()
		return nil, ErrSubscriptionNotFound
	}
	if err := rows.Scan(&previousUsage, &previousWindowStart, &previousStartsAt, &previousExpiresAt, &previousStatus, &previousUpdatedAt); err != nil {
		_ = rows.Close()
		_ = tx.Rollback()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	switch previousStatus {
	case SubscriptionStatusActive:
	case SubscriptionStatusSuspended:
		_ = tx.Rollback()
		return nil, ErrSubscriptionSuspended
	default:
		_ = tx.Rollback()
		return nil, ErrSubscriptionExpired
	}
	if !canAdvanceMonthlyCycleByUsage(previousUsage, *group.MonthlyLimitUSD) {
		_ = tx.Rollback()
		return nil, ErrMonthlyCycleNotExhausted
	}
	if !previousExpiresAt.After(now) {
		_ = tx.Rollback()
		return nil, ErrSubscriptionExpired
	}
	var previousWindowStartPtr *time.Time
	if previousWindowStart.Valid {
		previousWindowStartPtr = &previousWindowStart.Time
	}
	resetAt := monthlyCycleResetAt(previousWindowStartPtr, previousStartsAt, now)
	if !resetAt.After(now) {
		_ = tx.Rollback()
		return nil, ErrMonthlyCycleNotExhausted
	}
	if !canAdvanceMonthlyCycleByValidity(previousStartsAt, previousExpiresAt, resetAt) {
		_ = tx.Rollback()
		return nil, ErrMonthlyCycleNoFutureTime
	}
	remaining := resetAt.Sub(now)
	deductedSeconds := ceilDurationSeconds(remaining)
	deductedDays := int((time.Duration(deductedSeconds)*time.Second + 24*time.Hour - 1) / (24 * time.Hour))
	if deductedDays <= 0 {
		deductedDays = 1
	}
	newExpiresAt := previousExpiresAt.Add(-time.Duration(deductedSeconds) * time.Second)
	if !newExpiresAt.After(now) {
		_ = tx.Rollback()
		return nil, ErrMonthlyCycleNoFutureTime
	}
	if s.billingCacheService != nil {
		if err := s.billingCacheService.InvalidateSubscriptionBefore(ctx, sub.UserID, sub.GroupID, subscriptionCacheVersionFromNullTime(previousUpdatedAt)); err != nil {
			_ = tx.Rollback()
			return nil, ErrSubscriptionMaintenance.WithCause(fmt.Errorf("invalidate subscription billing cache: %w", err))
		}
	}
	newWindowStart := now
	newUpdatedAt := now.Add(time.Millisecond)

	result, err := txClient.ExecContext(ctx, `
		UPDATE user_subscriptions
		SET monthly_usage_usd = 0,
			monthly_window_start = $1,
			expires_at = $2,
			updated_at = $3
		WHERE id = $4 AND user_id = $5 AND deleted_at IS NULL
	`, newWindowStart, newExpiresAt, newUpdatedAt, subscriptionID, userID)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if affected == 0 {
		_ = tx.Rollback()
		return nil, ErrSubscriptionNotFound
	}
	if _, err := txClient.ExecContext(ctx, `
		INSERT INTO subscription_cycle_reset_logs (
			user_id, subscription_id, group_id, previous_expires_at, new_expires_at,
			previous_monthly_usage_usd, previous_monthly_window_start, new_monthly_window_start,
			deducted_days, deducted_seconds, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
	`, userID, subscriptionID, sub.GroupID, previousExpiresAt, newExpiresAt, previousUsage, nullableTimeArg(previousWindowStart), newWindowStart, deductedDays, deductedSeconds); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	s.InvalidateSubCache(sub.UserID, sub.GroupID)
	s.waitSubCacheInvalidation()
	updated, err := s.userSubRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return nil, err
	}
	return &AdvanceMonthlyCycleResult{
		Subscription:          updated,
		PreviousExpiresAt:     previousExpiresAt,
		NewExpiresAt:          newExpiresAt,
		DeductedDays:          deductedDays,
		DeductedSeconds:       deductedSeconds,
		PreviousMonthlyUsage:  previousUsage,
		NewMonthlyWindowStart: newWindowStart,
	}, nil
}

func canAdvanceMonthlyCycleByUsage(usage, limit float64) bool {
	return limit > 0 && usage >= limit*monthlyCycleAdvanceUsageThreshold
}

func canAdvanceMonthlyCycleByValidity(startsAt, expiresAt, resetAt time.Time) bool {
	if startsAt.IsZero() || expiresAt.IsZero() || resetAt.IsZero() {
		return false
	}
	if !expiresAt.After(startsAt.Add(monthlyCycleDuration)) {
		return false
	}
	return !expiresAt.Add(monthlyCycleAdvanceValidityGrace).Before(resetAt.Add(monthlyCycleDuration))
}

func monthlyCycleResetAt(windowStart *time.Time, startsAt, now time.Time) time.Time {
	var resetAt time.Time
	if windowStart != nil {
		effectiveStart := effectiveWindowStartAt(windowStart, startsAt, monthlyCycleDuration, now)
		if effectiveStart != nil {
			resetAt = effectiveStart.Add(monthlyCycleDuration)
		}
	}
	if resetAt.IsZero() {
		if alignedStart, ok := alignedCycleStart(startsAt, monthlyCycleDuration, now); ok {
			resetAt = alignedStart.Add(monthlyCycleDuration)
		} else {
			resetAt = now.Add(monthlyCycleDuration)
		}
	} else if !resetAt.After(now) {
		resetAt = advanceWindowStart(resetAt, monthlyCycleDuration, now)
	}
	if resetAt.IsZero() {
		resetAt = now.Add(monthlyCycleDuration)
	}
	if !resetAt.After(now) {
		if windowStart != nil {
			resetAt = advanceWindowStart(*windowStart, monthlyCycleDuration, now).Add(monthlyCycleDuration)
		} else {
			resetAt = now.Add(monthlyCycleDuration)
		}
	}
	return resetAt
}

func ceilDurationSeconds(d time.Duration) int64 {
	if d <= 0 {
		return 0
	}
	return int64((d + time.Second - 1) / time.Second)
}

func nullableTimeArg(t sql.NullTime) any {
	if !t.Valid {
		return nil
	}
	return t.Time
}

func subscriptionCacheVersion(sub *UserSubscription) int64 {
	if sub == nil {
		return 0
	}
	return subscriptionCacheVersionFromTime(sub.UpdatedAt)
}

func subscriptionCacheVersionFromTime(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMicro()
}

func subscriptionCacheVersionFromNullTime(t sql.NullTime) int64 {
	if !t.Valid {
		return 0
	}
	return subscriptionCacheVersionFromTime(t.Time)
}

// ListUserSubscriptions 获取用户的所有订阅
func (s *SubscriptionService) ListUserSubscriptions(ctx context.Context, userID int64) ([]UserSubscription, error) {
	subs, err := s.userSubRepo.ListByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	normalizeExpiredWindows(subs)
	normalizeSubscriptionStatus(subs)
	return subs, nil
}

// ListActiveUserSubscriptions 获取用户的所有有效订阅
func (s *SubscriptionService) ListActiveUserSubscriptions(ctx context.Context, userID int64) ([]UserSubscription, error) {
	subs, err := s.userSubRepo.ListActiveByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	normalizeExpiredWindows(subs)
	return subs, nil
}

// ListGroupSubscriptions 获取分组的所有订阅
func (s *SubscriptionService) ListGroupSubscriptions(ctx context.Context, groupID int64, page, pageSize int) ([]UserSubscription, *pagination.PaginationResult, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSize}
	subs, pag, err := s.userSubRepo.ListByGroupID(ctx, groupID, params)
	if err != nil {
		return nil, nil, err
	}
	normalizeExpiredWindows(subs)
	normalizeSubscriptionStatus(subs)
	return subs, pag, nil
}

// List 获取所有订阅（分页，支持筛选和排序）
func (s *SubscriptionService) List(ctx context.Context, page, pageSize int, userID, groupID *int64, status, platform, sortBy, sortOrder string) ([]UserSubscription, *pagination.PaginationResult, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSize}
	subs, pag, err := s.userSubRepo.List(ctx, params, userID, groupID, status, platform, sortBy, sortOrder)
	if err != nil {
		return nil, nil, err
	}
	normalizeExpiredWindows(subs)
	normalizeSubscriptionStatus(subs)
	return subs, pag, nil
}

// normalizeExpiredWindows 将已过期窗口的数据清零（仅影响返回数据，不影响数据库）
// 这确保前端显示正确的当前窗口状态，而不是过期窗口的历史数据
func normalizeExpiredWindows(subs []UserSubscription) {
	normalizeExpiredWindowsAt(subs, time.Now())
}

func normalizeExpiredWindowsAt(subs []UserSubscription, now time.Time) {
	for i := range subs {
		sub := &subs[i]
		// 日窗口过期：清零展示数据
		if sub.canAutomaticallyResetDailyAt(now) {
			sub.DailyWindowStart = nil
			sub.DailyUsageUSD = 0
		}
		// 周窗口过期：清零展示数据
		if sub.canAutomaticallyResetWeeklyAt(now) {
			sub.WeeklyWindowStart = nil
			sub.WeeklyUsageUSD = 0
		}
		// 月窗口过期：清零展示数据
		if sub.canAutomaticallyResetMonthlyAt(now) {
			sub.MonthlyWindowStart = nil
			sub.MonthlyUsageUSD = 0
		}
	}
}

// normalizeSubscriptionStatus 根据实际过期时间修正状态（仅影响返回数据，不影响数据库）
// 这确保前端显示正确的状态，即使定时任务尚未更新数据库
func normalizeSubscriptionStatus(subs []UserSubscription) {
	now := time.Now()
	for i := range subs {
		sub := &subs[i]
		if sub.Status == SubscriptionStatusActive && !sub.ExpiresAt.After(now) {
			sub.Status = SubscriptionStatusExpired
		}
	}
}

// startOfDay 返回给定时间所在日期的零点（保持原时区）
func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// CheckAndActivateWindow 检查并激活窗口（为历史未初始化订阅补齐窗口）
func (s *SubscriptionService) CheckAndActivateWindow(ctx context.Context, sub *UserSubscription) error {
	return s.checkAndActivateWindowAt(ctx, sub, s.now())
}

func (s *SubscriptionService) checkAndActivateWindowAt(ctx context.Context, sub *UserSubscription, now time.Time) error {
	if sub.IsWindowActivated() {
		return nil
	}

	periodicStart := now
	if !sub.StartsAt.IsZero() {
		// Fork subscriptions align quota windows with the paid term. This path
		// only repairs legacy rows whose windows were never initialized.
		periodicStart = sub.StartsAt
	}
	dailyStart := timezone.StartOfDay(now)
	if err := s.userSubRepo.ActivateWindows(ctx, sub.ID, dailyStart, periodicStart); err != nil {
		return err
	}
	sub.DailyWindowStart = &dailyStart
	sub.WeeklyWindowStart = &periodicStart
	sub.MonthlyWindowStart = &periodicStart
	return nil
}

func (s *SubscriptionService) ensureWindowMaintenance(ctx context.Context, sub *UserSubscription) error {
	if s == nil || sub == nil {
		return nil
	}
	if s.billingCacheService != nil {
		if err := s.billingCacheService.InvalidateSubscriptionBefore(ctx, sub.UserID, sub.GroupID, subscriptionCacheVersion(sub)); err != nil {
			return ErrSubscriptionMaintenance.WithCause(fmt.Errorf("invalidate subscription billing cache: %w", err))
		}
	}
	if err := s.CheckAndActivateWindow(ctx, sub); err != nil {
		return ErrSubscriptionMaintenance.WithCause(fmt.Errorf("activate windows: %w", err))
	}
	if err := s.CheckAndResetWindows(ctx, sub); err != nil {
		return ErrSubscriptionMaintenance.WithCause(fmt.Errorf("reset windows: %w", err))
	}
	s.InvalidateSubCache(sub.UserID, sub.GroupID)
	s.waitSubCacheInvalidation()
	return nil
}

// AdminResetQuota manually resets the daily, weekly, and/or monthly usage windows.
func (s *SubscriptionService) AdminResetQuota(ctx context.Context, subscriptionID int64, resetDaily, resetWeekly, resetMonthly bool) (*UserSubscription, error) {
	if entitlementID, ok := syntheticEntitlementIDFromSubscriptionID(subscriptionID); ok {
		return s.adminResetEntitlementQuota(ctx, entitlementID, resetDaily, resetWeekly, resetMonthly)
	}

	if !resetDaily && !resetWeekly && !resetMonthly {
		return nil, ErrInvalidInput
	}
	sub, err := s.userSubRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return nil, err
	}
	if sub.EntitlementLink != nil && sub.EntitlementLink.EntitlementID > 0 {
		if _, err := s.resetEntitlementQuotaUsage(ctx, sub.EntitlementLink.EntitlementID, resetDaily, resetWeekly, resetMonthly); err != nil {
			return nil, err
		}
		return s.userSubRepo.GetByID(ctx, subscriptionID)
	}
	// Invalidate L1 ristretto cache. Ristretto's Del() is asynchronous by design,
	// so call Wait() immediately after to flush pending operations and guarantee
	// the deleted key is not returned on the very next Get() call.
	s.InvalidateSubCacheSync(sub.UserID, sub.GroupID)
	if s.billingCacheService != nil {
		if err := s.billingCacheService.InvalidateSubscriptionBefore(ctx, sub.UserID, sub.GroupID, subscriptionCacheVersion(sub)); err != nil {
			return nil, ErrSubscriptionMaintenance.WithCause(fmt.Errorf("invalidate subscription billing cache: %w", err))
		}
	}
	now := s.now()
	// 日窗口锚点取当天 0 点：手动重置只清空用量，不改变“每天 0 点刷新”的节奏。
	// 周/月窗口保持锚定重置时刻（期限对齐滚动窗口语义）。
	if err := s.userSubRepo.ResetUsageWindows(ctx, sub.ID, resetDaily, resetWeekly, resetMonthly, timezone.StartOfDay(now), now); err != nil {
		return nil, err
	}
	// Invalidate L1 ristretto cache. Ristretto's Del() is asynchronous by design,
	// so call Wait() immediately after to flush pending operations and guarantee
	// the deleted key is not returned on the very next Get() call.
	s.InvalidateSubCache(sub.UserID, sub.GroupID)
	s.waitSubCacheInvalidation()
	// Return the refreshed subscription from DB
	return s.userSubRepo.GetByID(ctx, subscriptionID)
}

// CheckAndResetWindows 检查并重置过期的窗口
func (s *SubscriptionService) CheckAndResetWindows(ctx context.Context, sub *UserSubscription) error {
	now := s.now()
	needsInvalidateCache := false
	dailyWindowStart, dailyReset := sub.automaticDailyWindowStartAt(now)
	weeklyWindowStart, weeklyReset := sub.automaticWindowStartAt(sub.WeeklyWindowStart, 7*24*time.Hour, now)
	monthlyWindowStart, monthlyReset := sub.automaticWindowStartAt(sub.MonthlyWindowStart, monthlyCycleDuration, now)
	needsReset := dailyReset || weeklyReset || monthlyReset
	if needsReset && s.billingCacheService != nil {
		if err := s.billingCacheService.InvalidateSubscriptionBefore(ctx, sub.UserID, sub.GroupID, subscriptionCacheVersion(sub)); err != nil {
			return err
		}
	}

	// 日窗口重置（每天 0 点）
	if dailyReset {
		expectedWindowStart := sub.DailyWindowStart
		if err := s.userSubRepo.ResetDailyUsage(ctx, sub.ID, expectedWindowStart, dailyWindowStart); err != nil {
			return err
		}
		sub.DailyWindowStart = &dailyWindowStart
		sub.DailyUsageUSD = 0
		needsInvalidateCache = true
	}

	// 周窗口重置（7天）
	if weeklyReset {
		expectedWindowStart := sub.WeeklyWindowStart
		if err := s.userSubRepo.ResetWeeklyUsage(ctx, sub.ID, expectedWindowStart, weeklyWindowStart); err != nil {
			return err
		}
		sub.WeeklyWindowStart = &weeklyWindowStart
		sub.WeeklyUsageUSD = 0
		needsInvalidateCache = true
	}

	// 月窗口重置（30天）
	if monthlyReset {
		expectedWindowStart := sub.MonthlyWindowStart
		if err := s.userSubRepo.ResetMonthlyUsage(ctx, sub.ID, expectedWindowStart, monthlyWindowStart); err != nil {
			return err
		}
		sub.MonthlyWindowStart = &monthlyWindowStart
		sub.MonthlyUsageUSD = 0
		needsInvalidateCache = true
	}

	// 如果有窗口被重置，失效缓存以保持一致性
	if needsInvalidateCache {
		s.InvalidateSubCache(sub.UserID, sub.GroupID)
		s.waitSubCacheInvalidation()
	}

	return nil
}

func advanceWindowStart(windowStart time.Time, cycle time.Duration, now time.Time) time.Time {
	if cycle <= 0 || windowStart.IsZero() {
		return windowStart
	}
	if now.Before(windowStart.Add(cycle)) {
		return windowStart
	}

	elapsed := now.Sub(windowStart)
	steps := elapsed / cycle
	if steps < 1 {
		steps = 1
	}
	return windowStart.Add(steps * cycle)
}

func resolvedWindowResetStart(windowStart *time.Time, startsAt time.Time, cycle time.Duration, now time.Time) time.Time {
	effectiveStart := effectiveWindowStartAt(windowStart, startsAt, cycle, now)
	if effectiveStart == nil {
		return now
	}
	return advanceWindowStart(*effectiveStart, cycle, now)
}

// EnsureWindowMaintenance advances expired usage windows before a request is
// allowed to proceed. It returns a fresh database snapshot because a competing
// request may have won one of the conditional resets.
func (s *SubscriptionService) EnsureWindowMaintenance(ctx context.Context, sub *UserSubscription) (*UserSubscription, error) {
	if sub == nil {
		return nil, ErrSubscriptionNilInput
	}
	if !sub.IsWindowActivated() {
		if err := s.CheckAndActivateWindow(ctx, sub); err != nil {
			return nil, err
		}
	}
	if err := s.CheckAndResetWindows(ctx, sub); err != nil {
		return nil, err
	}

	// GetByID bypasses the service caches. This prevents a stale loser of the
	// CAS from validating limits against zeroed in-memory usage.
	refreshed, err := s.userSubRepo.GetByID(ctx, sub.ID)
	if err != nil {
		return nil, err
	}
	s.InvalidateSubCacheSync(sub.UserID, sub.GroupID)
	return refreshed, nil
}

// CheckUsageLimits 检查使用限额（返回错误如果超限）
// 用于中间件的快速预检查，additionalCost 通常为 0
func (s *SubscriptionService) CheckUsageLimits(ctx context.Context, sub *UserSubscription, group *Group, additionalCost float64) error {
	if !sub.CheckDailyLimit(group, additionalCost) {
		return ErrDailyLimitExceeded
	}
	if !sub.CheckWeeklyLimit(group, additionalCost) {
		return ErrWeeklyLimitExceeded
	}
	if !sub.CheckMonthlyLimit(group, additionalCost) {
		return ErrMonthlyLimitExceeded
	}
	return nil
}

// ValidateAndCheckLimits 合并验证+限额检查（中间件热路径专用）
// 仅做内存检查，不触发 DB 写入。调用方必须在放行请求前同步完成窗口维护。
// 返回 needsMaintenance 表示是否需要执行窗口维护并回读数据库快照。
func (s *SubscriptionService) ValidateAndCheckLimits(sub *UserSubscription, group *Group) (needsMaintenance bool, err error) {
	now := s.now()
	// 1. 验证订阅状态
	if sub.Status == SubscriptionStatusExpired {
		return false, ErrSubscriptionExpired
	}
	if sub.Status == SubscriptionStatusSuspended {
		return false, ErrSubscriptionSuspended
	}
	if !sub.ExpiresAt.After(now) {
		return false, ErrSubscriptionExpired
	}

	// 2. 内存中修正过期窗口的用量，确保预检查不会误拒绝用户。
	//    调用方随后同步推进 DB 窗口，并用回读快照重新校验。
	if sub.canAutomaticallyResetDailyAt(now) {
		sub.DailyUsageUSD = 0
		needsMaintenance = true
	}
	if sub.canAutomaticallyResetWeeklyAt(now) {
		sub.WeeklyUsageUSD = 0
		needsMaintenance = true
	}
	if sub.canAutomaticallyResetMonthlyAt(now) {
		sub.MonthlyUsageUSD = 0
		needsMaintenance = true
	}
	if !sub.IsWindowActivated() {
		needsMaintenance = true
	}

	// 3. 检查用量限额
	if !sub.CheckDailyLimit(group, 0) {
		return needsMaintenance, ErrDailyLimitExceeded
	}
	if !sub.CheckWeeklyLimit(group, 0) {
		return needsMaintenance, ErrWeeklyLimitExceeded
	}
	if !sub.CheckMonthlyLimit(group, 0) {
		return needsMaintenance, ErrMonthlyLimitExceeded
	}

	return needsMaintenance, nil
}

// DoWindowMaintenance 异步执行窗口维护（激活+重置）
// 使用独立 context，不受请求取消影响。
// 注意：此方法仅在 ValidateAndCheckLimits 返回 needsMaintenance=true 时调用，
// 而 IsExpired()=true 的订阅在 ValidateAndCheckLimits 中已被拦截返回错误，
// 因此进入此方法的订阅一定未过期，无需处理过期状态同步。
func (s *SubscriptionService) DoWindowMaintenance(sub *UserSubscription) {
	if s == nil {
		return
	}
	if s.maintenanceQueue != nil {
		err := s.maintenanceQueue.TryEnqueue(func() {
			s.doWindowMaintenance(sub)
		})
		if err != nil {
			log.Printf("Subscription maintenance enqueue failed: %v", err)
		}
		return
	}

	s.doWindowMaintenance(sub)
}

func (s *SubscriptionService) doWindowMaintenance(sub *UserSubscription) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.ensureWindowMaintenance(ctx, sub); err != nil {
		log.Printf("Failed to maintain subscription windows: %v", err)
	}
}

// RecordUsage 记录使用量到订阅
func (s *SubscriptionService) RecordUsage(ctx context.Context, subscriptionID int64, costUSD float64) error {
	return s.userSubRepo.IncrementUsage(ctx, subscriptionID, costUSD)
}

// SubscriptionProgress 订阅进度
type SubscriptionProgress struct {
	ID            int64                `json:"id"`
	GroupName     string               `json:"group_name"`
	ExpiresAt     time.Time            `json:"expires_at"`
	ExpiresInDays int                  `json:"expires_in_days"`
	Daily         *UsageWindowProgress `json:"daily,omitempty"`
	Weekly        *UsageWindowProgress `json:"weekly,omitempty"`
	Monthly       *UsageWindowProgress `json:"monthly,omitempty"`
}

// UsageWindowProgress 使用窗口进度
type UsageWindowProgress struct {
	LimitUSD        float64   `json:"limit_usd"`
	UsedUSD         float64   `json:"used_usd"`
	RemainingUSD    float64   `json:"remaining_usd"`
	Percentage      float64   `json:"percentage"`
	WindowStart     time.Time `json:"window_start"`
	ResetsAt        time.Time `json:"resets_at"`
	ResetsInSeconds int64     `json:"resets_in_seconds"`
}

// GetSubscriptionProgress 获取订阅使用进度
func (s *SubscriptionService) GetSubscriptionProgress(ctx context.Context, subscriptionID int64) (*SubscriptionProgress, error) {
	sub, err := s.userSubRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return nil, ErrSubscriptionNotFound
	}

	group := sub.Group
	if group == nil {
		group, err = s.groupRepo.GetByID(ctx, sub.GroupID)
		if err != nil {
			return nil, err
		}
	}

	return s.calculateProgress(sub, group), nil
}

// calculateProgress 根据已加载的订阅和分组数据计算使用进度（纯内存计算，无 DB 查询）
func (s *SubscriptionService) calculateProgress(sub *UserSubscription, group *Group) *SubscriptionProgress {
	progress := &SubscriptionProgress{
		ID:            sub.ID,
		GroupName:     group.Name,
		ExpiresAt:     sub.ExpiresAt,
		ExpiresInDays: sub.DaysRemaining(),
	}

	// 日进度
	if group.HasDailyLimit() && sub.DailyWindowStart != nil {
		limit := *group.DailyLimitUSD
		resetsAt := sub.DailyWindowStart.Add(24 * time.Hour)
		if dailyResetTime := sub.DailyResetTime(); dailyResetTime != nil {
			resetsAt = *dailyResetTime
		}
		progress.Daily = &UsageWindowProgress{
			LimitUSD:        limit,
			UsedUSD:         sub.DailyUsageUSD,
			RemainingUSD:    limit - sub.DailyUsageUSD,
			Percentage:      (sub.DailyUsageUSD / limit) * 100,
			WindowStart:     *sub.DailyWindowStart,
			ResetsAt:        resetsAt,
			ResetsInSeconds: int64(time.Until(resetsAt).Seconds()),
		}
		if progress.Daily.RemainingUSD < 0 {
			progress.Daily.RemainingUSD = 0
		}
		if progress.Daily.Percentage > 100 {
			progress.Daily.Percentage = 100
		}
		if progress.Daily.ResetsInSeconds < 0 {
			progress.Daily.ResetsInSeconds = 0
		}
	}

	// 周进度
	if group.HasWeeklyLimit() && sub.WeeklyWindowStart != nil {
		limit := *group.WeeklyLimitUSD
		resetsAt := sub.WeeklyWindowStart.Add(7 * 24 * time.Hour)
		if weeklyResetTime := sub.WeeklyResetTime(); weeklyResetTime != nil {
			resetsAt = *weeklyResetTime
		}
		progress.Weekly = &UsageWindowProgress{
			LimitUSD:        limit,
			UsedUSD:         sub.WeeklyUsageUSD,
			RemainingUSD:    limit - sub.WeeklyUsageUSD,
			Percentage:      (sub.WeeklyUsageUSD / limit) * 100,
			WindowStart:     *sub.WeeklyWindowStart,
			ResetsAt:        resetsAt,
			ResetsInSeconds: int64(time.Until(resetsAt).Seconds()),
		}
		if progress.Weekly.RemainingUSD < 0 {
			progress.Weekly.RemainingUSD = 0
		}
		if progress.Weekly.Percentage > 100 {
			progress.Weekly.Percentage = 100
		}
		if progress.Weekly.ResetsInSeconds < 0 {
			progress.Weekly.ResetsInSeconds = 0
		}
	}

	// 月进度
	if group.HasMonthlyLimit() && sub.MonthlyWindowStart != nil {
		limit := *group.MonthlyLimitUSD
		resetsAt := sub.MonthlyWindowStart.Add(30 * 24 * time.Hour)
		if monthlyResetTime := sub.MonthlyResetTime(); monthlyResetTime != nil {
			resetsAt = *monthlyResetTime
		}
		progress.Monthly = &UsageWindowProgress{
			LimitUSD:        limit,
			UsedUSD:         sub.MonthlyUsageUSD,
			RemainingUSD:    limit - sub.MonthlyUsageUSD,
			Percentage:      (sub.MonthlyUsageUSD / limit) * 100,
			WindowStart:     *sub.MonthlyWindowStart,
			ResetsAt:        resetsAt,
			ResetsInSeconds: int64(time.Until(resetsAt).Seconds()),
		}
		if progress.Monthly.RemainingUSD < 0 {
			progress.Monthly.RemainingUSD = 0
		}
		if progress.Monthly.Percentage > 100 {
			progress.Monthly.Percentage = 100
		}
		if progress.Monthly.ResetsInSeconds < 0 {
			progress.Monthly.ResetsInSeconds = 0
		}
	}

	return progress
}

// GetUserSubscriptionsWithProgress 获取用户所有订阅及进度
func (s *SubscriptionService) GetUserSubscriptionsWithProgress(ctx context.Context, userID int64) ([]SubscriptionProgress, error) {
	// ListActiveByUserID 已使用 .WithGroup() eager-load Group 关联，1 次查询获取所有数据
	subs, err := s.userSubRepo.ListActiveByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	progresses := make([]SubscriptionProgress, 0, len(subs))
	for i := range subs {
		sub := &subs[i]
		group := sub.Group
		if group == nil {
			continue
		}
		progresses = append(progresses, *s.calculateProgress(sub, group))
	}

	return progresses, nil
}

// ValidateSubscription 验证订阅是否有效
func (s *SubscriptionService) ValidateSubscription(ctx context.Context, sub *UserSubscription) error {
	if sub.Status == SubscriptionStatusExpired {
		return ErrSubscriptionExpired
	}
	if sub.Status == SubscriptionStatusSuspended {
		return ErrSubscriptionSuspended
	}
	if sub.IsExpired() {
		// 更新状态
		_ = s.userSubRepo.UpdateStatus(ctx, sub.ID, SubscriptionStatusExpired)
		return ErrSubscriptionExpired
	}
	return nil
}
