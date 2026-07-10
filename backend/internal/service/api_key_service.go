package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"html"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/dgraph-io/ristretto"
	"golang.org/x/sync/singleflight"
)

var (
	ErrAPIKeyNotFound      = infraerrors.NotFound("API_KEY_NOT_FOUND", "api key not found")
	ErrGroupNotAllowed     = infraerrors.Forbidden("GROUP_NOT_ALLOWED", "user is not allowed to bind this group")
	ErrAPIKeyExists        = infraerrors.Conflict("API_KEY_EXISTS", "api key already exists")
	ErrAPIKeyTooShort      = infraerrors.BadRequest("API_KEY_TOO_SHORT", "api key must be at least 16 characters")
	ErrAPIKeyInvalidChars  = infraerrors.BadRequest("API_KEY_INVALID_CHARS", "api key can only contain letters, numbers, underscores, and hyphens")
	ErrAPIKeyRateLimited   = infraerrors.TooManyRequests("API_KEY_RATE_LIMITED", "too many failed attempts, please try again later")
	ErrInvalidIPPattern    = infraerrors.BadRequest("INVALID_IP_PATTERN", "invalid IP or CIDR pattern")
	ErrInvalidAccessSource = infraerrors.BadRequest("INVALID_ACCESS_SOURCE", "access_source must be balance or entitlement")
	// ErrAPIKeyExpired        = infraerrors.Forbidden("API_KEY_EXPIRED", "api key has expired")
	ErrAPIKeyExpired = infraerrors.Forbidden("API_KEY_EXPIRED", "api key 已过期")
	// ErrAPIKeyQuotaExhausted = infraerrors.TooManyRequests("API_KEY_QUOTA_EXHAUSTED", "api key quota exhausted")
	ErrAPIKeyQuotaExhausted = infraerrors.TooManyRequests("API_KEY_QUOTA_EXHAUSTED", "api key 额度已用完")

	// Rate limit errors
	ErrAPIKeyRateLimit5hExceeded = infraerrors.TooManyRequests("API_KEY_RATE_5H_EXCEEDED", "api key 5小时限额已用完")
	ErrAPIKeyRateLimit1dExceeded = infraerrors.TooManyRequests("API_KEY_RATE_1D_EXCEEDED", "api key 日限额已用完")
	ErrAPIKeyRateLimit7dExceeded = infraerrors.TooManyRequests("API_KEY_RATE_7D_EXCEEDED", "api key 7天限额已用完")
)

const (
	apiKeyMaxErrorsPerHour       = 20
	apiKeyLastUsedMinTouch       = 30 * time.Second
	apiKeySortCurrentConcurrency = "current_concurrency"
	// DB 写失败后的短退避，避免请求路径持续同步重试造成写风暴与高延迟。
	apiKeyLastUsedFailBackoff = 5 * time.Second
)

type APIKeyRepository interface {
	Create(ctx context.Context, key *APIKey) error
	GetByID(ctx context.Context, id int64) (*APIKey, error)
	// GetKeyAndOwnerID 仅获取 API Key 的 key 与所有者 ID，用于删除等轻量场景
	GetKeyAndOwnerID(ctx context.Context, id int64) (string, int64, error)
	GetByKey(ctx context.Context, key string) (*APIKey, error)
	// GetByKeyForAuth 认证专用查询，返回最小字段集
	GetByKeyForAuth(ctx context.Context, key string) (*APIKey, error)
	Update(ctx context.Context, key *APIKey) error
	Delete(ctx context.Context, id int64) error
	// DeleteWithAudit 在同一事务内先写 deleted_api_key_audits 审计、再软删除该 key。
	DeleteWithAudit(ctx context.Context, id int64) error

	ListByUserID(ctx context.Context, userID int64, params pagination.PaginationParams, filters APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error)
	VerifyOwnership(ctx context.Context, userID int64, apiKeyIDs []int64) ([]int64, error)
	CountByUserID(ctx context.Context, userID int64) (int64, error)
	ExistsByKey(ctx context.Context, key string) (bool, error)
	ListByGroupID(ctx context.Context, groupID int64, params pagination.PaginationParams) ([]APIKey, *pagination.PaginationResult, error)
	SearchAPIKeys(ctx context.Context, userID int64, keyword string, limit int) ([]APIKey, error)
	ClearGroupIDByGroupID(ctx context.Context, groupID int64) (int64, error)
	// UpdateGroupIDByUserAndGroup 将用户下绑定 oldGroupID 的所有 Key 迁移到 newGroupID
	UpdateGroupIDByUserAndGroup(ctx context.Context, userID, oldGroupID, newGroupID int64) (int64, error)
	CompareAndSwapGroupID(ctx context.Context, id int64, oldGroupID, newGroupID int64) (bool, error)
	CompareAndSwapGroupIDWithEntitlement(ctx context.Context, id int64, oldGroupID, newGroupID int64, expectedEntitlementID, newEntitlementID *int64) (bool, error)
	CountByGroupID(ctx context.Context, groupID int64) (int64, error)
	ListKeysByUserID(ctx context.Context, userID int64) ([]string, error)
	ListKeysByGroupID(ctx context.Context, groupID int64) ([]string, error)

	// Quota methods
	IncrementQuotaUsed(ctx context.Context, id int64, amount float64) (float64, error)
	UpdateLastUsed(ctx context.Context, id int64, usedAt time.Time) error

	// Rate limit methods
	IncrementRateLimitUsage(ctx context.Context, id int64, cost float64) error
	ResetRateLimitWindows(ctx context.Context, id int64) error
	GetRateLimitData(ctx context.Context, id int64) (*APIKeyRateLimitData, error)
}

type apiKeyAllByUserIDLister interface {
	ListAllByUserID(ctx context.Context, userID int64, filters APIKeyListFilters) ([]APIKey, error)
}

// APIKeyRateLimitData holds rate limit usage and window state for an API key.
type APIKeyRateLimitData struct {
	Usage5h       float64
	Usage1d       float64
	Usage7d       float64
	Window5hStart *time.Time
	Window1dStart *time.Time
	Window7dStart *time.Time
}

// EffectiveUsage5h returns the 5h window usage, or 0 if the window has expired.
func (d *APIKeyRateLimitData) EffectiveUsage5h() float64 {
	if IsWindowExpired(d.Window5hStart, RateLimitWindow5h) {
		return 0
	}
	return d.Usage5h
}

// EffectiveUsage1d returns the 1d window usage, or 0 if the window has expired.
func (d *APIKeyRateLimitData) EffectiveUsage1d() float64 {
	if IsWindowExpired(d.Window1dStart, RateLimitWindow1d) {
		return 0
	}
	return d.Usage1d
}

// EffectiveUsage7d returns the 7d window usage, or 0 if the window has expired.
func (d *APIKeyRateLimitData) EffectiveUsage7d() float64 {
	if IsWindowExpired(d.Window7dStart, RateLimitWindow7d) {
		return 0
	}
	return d.Usage7d
}

// APIKeyQuotaUsageState captures the latest quota fields after an atomic quota update.
// It is intentionally small so repositories can return it from a single SQL statement.
type APIKeyQuotaUsageState struct {
	QuotaUsed float64
	Quota     float64
	Key       string
	Status    string
}

// APIKeyCache defines cache operations for API key service
type APIKeyCache interface {
	GetCreateAttemptCount(ctx context.Context, userID int64) (int, error)
	IncrementCreateAttemptCount(ctx context.Context, userID int64) error
	DeleteCreateAttemptCount(ctx context.Context, userID int64) error

	IncrementDailyUsage(ctx context.Context, apiKey string) error
	SetDailyUsageExpiry(ctx context.Context, apiKey string, ttl time.Duration) error

	GetAuthCache(ctx context.Context, key string) (*APIKeyAuthCacheEntry, error)
	SetAuthCache(ctx context.Context, key string, entry *APIKeyAuthCacheEntry, ttl time.Duration) error
	DeleteAuthCache(ctx context.Context, key string) error

	// Pub/Sub for L1 cache invalidation across instances
	PublishAuthCacheInvalidation(ctx context.Context, cacheKey string) error
	SubscribeAuthCacheInvalidation(ctx context.Context, handler func(cacheKey string)) error
}

// APIKeyAuthCacheInvalidator 提供认证缓存失效能力
type APIKeyAuthCacheInvalidator interface {
	InvalidateAuthCacheByKey(ctx context.Context, key string)
	InvalidateAuthCacheByUserID(ctx context.Context, userID int64)
	InvalidateAuthCacheByGroupID(ctx context.Context, groupID int64)
}

// CreateAPIKeyRequest 创建API Key请求
type CreateAPIKeyRequest struct {
	Name                      string   `json:"name"`
	GroupID                   *int64   `json:"group_id"`
	SubscriptionEntitlementID *int64   `json:"subscription_entitlement_id"`
	AccessSource              *string  `json:"access_source"`
	CustomKey                 *string  `json:"custom_key"`   // 可选的自定义key
	IPWhitelist               []string `json:"ip_whitelist"` // IP 白名单
	IPBlacklist               []string `json:"ip_blacklist"` // IP 黑名单

	// Quota fields
	Quota         float64 `json:"quota"`           // Quota limit in USD (0 = unlimited)
	ExpiresInDays *int    `json:"expires_in_days"` // Days until expiry (nil = never expires)

	// Rate limit fields (0 = unlimited)
	RateLimit5h float64 `json:"rate_limit_5h"`
	RateLimit1d float64 `json:"rate_limit_1d"`
	RateLimit7d float64 `json:"rate_limit_7d"`

	AutoSwitchGroupEnabled *bool `json:"auto_switch_group_enabled"`
}

// UpdateAPIKeyRequest 更新API Key请求
type UpdateAPIKeyRequest struct {
	Name                         *string  `json:"name"`
	GroupID                      *int64   `json:"group_id"`
	ClearGroup                   bool     `json:"-"`
	SubscriptionEntitlementID    *int64   `json:"subscription_entitlement_id"`
	SubscriptionEntitlementIDSet bool     `json:"-"`
	AccessSource                 *string  `json:"access_source"`
	Status                       *string  `json:"status"`
	IPWhitelist                  []string `json:"ip_whitelist"` // IP 白名单（空数组清空）
	IPBlacklist                  []string `json:"ip_blacklist"` // IP 黑名单（空数组清空）

	// Quota fields
	Quota           *float64   `json:"quota"`       // Quota limit in USD (nil = no change, 0 = unlimited)
	ExpiresAt       *time.Time `json:"expires_at"`  // Expiration time (nil = no change)
	ClearExpiration bool       `json:"-"`           // Clear expiration (internal use)
	ResetQuota      *bool      `json:"reset_quota"` // Reset quota_used to 0

	// Rate limit fields (nil = no change, 0 = unlimited)
	RateLimit5h         *float64 `json:"rate_limit_5h"`
	RateLimit1d         *float64 `json:"rate_limit_1d"`
	RateLimit7d         *float64 `json:"rate_limit_7d"`
	ResetRateLimitUsage *bool    `json:"reset_rate_limit_usage"` // Reset all usage counters to 0

	AutoSwitchGroupEnabled *bool `json:"auto_switch_group_enabled"`
}

// APIKeyService API Key服务
// RateLimitCacheInvalidator invalidates rate limit cache entries on manual reset.
type RateLimitCacheInvalidator interface {
	InvalidateAPIKeyRateLimit(ctx context.Context, keyID int64) error
}

type APIKeyService struct {
	apiKeyRepo                 APIKeyRepository
	userRepo                   UserRepository
	groupRepo                  GroupRepository
	userSubRepo                UserSubscriptionRepository
	userGroupRateRepo          UserGroupRateRepository
	settingSvc                 SubscriptionEntitlementsRuntimeProvider
	subscriptionEntitlementSvc *SubscriptionEntitlementService
	cache                      APIKeyCache
	rateLimitCacheInvalid      RateLimitCacheInvalidator // optional: invalidate Redis rate limit cache
	concurrencyService         *ConcurrencyService
	cfg                        *config.Config
	authCacheL1                *ristretto.Cache
	authCfg                    apiKeyAuthCacheConfig
	authGroup                  singleflight.Group
	lastUsedTouchL1            sync.Map // keyID -> nextAllowedAt(time.Time)
	lastUsedTouchSF            singleflight.Group
}

// NewAPIKeyService 创建API Key服务实例
func NewAPIKeyService(
	apiKeyRepo APIKeyRepository,
	userRepo UserRepository,
	groupRepo GroupRepository,
	userSubRepo UserSubscriptionRepository,
	userGroupRateRepo UserGroupRateRepository,
	cache APIKeyCache,
	cfg *config.Config,
) *APIKeyService {
	svc := &APIKeyService{
		apiKeyRepo:        apiKeyRepo,
		userRepo:          userRepo,
		groupRepo:         groupRepo,
		userSubRepo:       userSubRepo,
		userGroupRateRepo: userGroupRateRepo,
		cache:             cache,
		cfg:               cfg,
	}
	svc.initAuthCache(cfg)
	return svc
}

// SetRateLimitCacheInvalidator sets the optional rate limit cache invalidator.
// Called after construction (e.g. in wire) to avoid circular dependencies.
func (s *APIKeyService) SetRateLimitCacheInvalidator(inv RateLimitCacheInvalidator) {
	s.rateLimitCacheInvalid = inv
}

func (s *APIKeyService) SetSubscriptionEntitlementDependencies(settingSvc SubscriptionEntitlementsRuntimeProvider, entitlementSvc *SubscriptionEntitlementService) {
	s.settingSvc = settingSvc
	s.subscriptionEntitlementSvc = entitlementSvc
}

func (s *APIKeyService) subscriptionEntitlementsRuntime(ctx context.Context) SubscriptionEntitlementsRuntime {
	if s == nil || s.settingSvc == nil {
		return SubscriptionEntitlementsRuntime{}
	}
	return s.settingSvc.GetSubscriptionEntitlementsRuntime(ctx)
}

func (s *APIKeyService) SetConcurrencyService(concurrencyService *ConcurrencyService) {
	s.concurrencyService = concurrencyService
}

func (s *APIKeyService) compileAPIKeyIPRules(apiKey *APIKey) {
	if apiKey == nil {
		return
	}
	apiKey.CompiledIPWhitelist = ip.CompileIPRules(apiKey.IPWhitelist)
	apiKey.CompiledIPBlacklist = ip.CompileIPRules(apiKey.IPBlacklist)
}

// GenerateKey 生成随机API Key
func (s *APIKeyService) GenerateKey() (string, error) {
	// 生成32字节随机数据
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}

	// 转换为十六进制字符串并添加前缀
	prefix := s.cfg.Default.APIKeyPrefix
	if prefix == "" {
		prefix = "sk-"
	}

	key := prefix + hex.EncodeToString(bytes)
	return key, nil
}

// ValidateCustomKey 验证自定义API Key格式
func (s *APIKeyService) ValidateCustomKey(key string) error {
	// 检查长度
	if len(key) < 16 {
		return ErrAPIKeyTooShort
	}

	// 检查字符：只允许字母、数字、下划线、连字符
	for _, c := range key {
		if (c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') ||
			c == '_' || c == '-' {
			continue
		}
		return ErrAPIKeyInvalidChars
	}

	return nil
}

// checkAPIKeyRateLimit 检查用户创建自定义Key的错误次数是否超限
func (s *APIKeyService) checkAPIKeyRateLimit(ctx context.Context, userID int64) error {
	if s.cache == nil {
		return nil
	}

	count, err := s.cache.GetCreateAttemptCount(ctx, userID)
	if err != nil {
		// Redis 出错时不阻止用户操作
		return nil
	}

	if count >= apiKeyMaxErrorsPerHour {
		return ErrAPIKeyRateLimited
	}

	return nil
}

// incrementAPIKeyErrorCount 增加用户创建自定义Key的错误计数
func (s *APIKeyService) incrementAPIKeyErrorCount(ctx context.Context, userID int64) {
	if s.cache == nil {
		return
	}

	_ = s.cache.IncrementCreateAttemptCount(ctx, userID)
}

// canUserBindGroup 检查用户是否可以绑定指定分组
// 对于订阅类型分组：检查用户是否有有效订阅
// 对于标准类型分组：使用原有的 AllowedGroups 和 IsExclusive 逻辑
func (s *APIKeyService) canUserBindGroup(ctx context.Context, user *User, group *Group) bool {
	// 订阅类型分组：需要有效订阅
	if group.SupportsSubscriptionAccess() {
		if s.subscriptionEntitlementsRuntime(ctx).Enabled {
			return s.subscriptionEntitlementSvc != nil
		}
		if s.userSubRepo == nil {
			return false
		}
		_, err := s.userSubRepo.GetActiveByUserIDAndGroupID(ctx, user.ID, group.ID)
		return err == nil // 有有效订阅则允许
	}
	// 标准类型分组：使用原有逻辑
	return user.CanBindGroup(group.ID, group.IsExclusive)
}

// Create 创建API Key
func (s *APIKeyService) resolveCreateAccessSourceBinding(ctx context.Context, user *User, group *Group, accessSource string, explicitEntitlementID *int64, runtime SubscriptionEntitlementsRuntime) (*int64, error) {
	if !runtime.Enabled {
		return s.resolveCreateSubscriptionEntitlementID(ctx, user, group, explicitEntitlementID, runtime)
	}
	switch accessSource {
	case APIKeyAccessSourceBalance:
		if err := validateAPIKeyAccessSourceBinding(accessSource, explicitEntitlementID); err != nil {
			return nil, err
		}
		if group != nil {
			if !canUserUseBalanceAccessSource(user, group) {
				return nil, ErrGroupNotAllowed
			}
		}
		return nil, nil
	case APIKeyAccessSourceEntitlement:
		if err := validateAPIKeyAccessSourceBinding(accessSource, explicitEntitlementID); err != nil {
			return nil, err
		}
		if group == nil || !runtime.Enabled {
			return nil, ErrGroupNotAllowed
		}
		return s.resolveSubscriptionEntitlementForGroup(ctx, user.ID, group.ID, explicitEntitlementID)
	default:
		return nil, ErrInvalidAccessSource
	}
}

func (s *APIKeyService) resolveUpdateAccessSourceBinding(ctx context.Context, user *User, group *Group, accessSource string, currentEntitlementID *int64, explicitEntitlementID *int64, explicitEntitlementSet bool, groupChanged bool, runtime SubscriptionEntitlementsRuntime) (*int64, error) {
	if !runtime.Enabled {
		return s.resolveUpdateSubscriptionEntitlementID(ctx, user, group, currentEntitlementID, explicitEntitlementID, explicitEntitlementSet, groupChanged, runtime)
	}
	switch accessSource {
	case APIKeyAccessSourceBalance:
		if explicitEntitlementSet && explicitEntitlementID != nil {
			return nil, ErrInvalidAccessSource
		}
		if group != nil {
			if !canUserUseBalanceAccessSource(user, group) {
				return nil, ErrGroupNotAllowed
			}
		}
		return nil, nil
	case APIKeyAccessSourceEntitlement:
		if group == nil || !runtime.Enabled {
			return nil, ErrGroupNotAllowed
		}
		if explicitEntitlementSet {
			if explicitEntitlementID == nil {
				return nil, ErrInvalidAccessSource
			}
			return s.resolveSubscriptionEntitlementForGroup(ctx, user.ID, group.ID, explicitEntitlementID)
		}
		if currentEntitlementID != nil {
			if entID, err := s.resolveSubscriptionEntitlementForGroup(ctx, user.ID, group.ID, currentEntitlementID); err == nil {
				return entID, nil
			}
		}
		if groupChanged {
			return s.resolveSubscriptionEntitlementForGroup(ctx, user.ID, group.ID, nil)
		}
		return nil, ErrInvalidAccessSource
	default:
		return nil, ErrInvalidAccessSource
	}
}

func (s *APIKeyService) resolveCreateSubscriptionEntitlementID(ctx context.Context, user *User, group *Group, explicitEntitlementID *int64, runtime SubscriptionEntitlementsRuntime) (*int64, error) {
	if group == nil {
		if runtime.Enabled && explicitEntitlementID != nil {
			return nil, ErrGroupNotAllowed
		}
		return nil, nil
	}
	if !group.SupportsSubscriptionAccess() {
		if !user.CanBindGroup(group.ID, group.IsExclusive) {
			return nil, ErrGroupNotAllowed
		}
		return nil, nil
	}
	if !runtime.Enabled {
		if !s.canUserBindGroup(ctx, user, group) {
			return nil, ErrGroupNotAllowed
		}
		return nil, nil
	}
	return s.resolveSubscriptionEntitlementForGroup(ctx, user.ID, group.ID, explicitEntitlementID)
}

func (s *APIKeyService) resolveUpdateSubscriptionEntitlementID(ctx context.Context, user *User, group *Group, currentEntitlementID *int64, explicitEntitlementID *int64, explicitEntitlementSet bool, groupChanged bool, runtime SubscriptionEntitlementsRuntime) (*int64, error) {
	if group == nil {
		if runtime.Enabled && explicitEntitlementSet && explicitEntitlementID != nil {
			return nil, ErrGroupNotAllowed
		}
		return nil, nil
	}
	if !group.SupportsSubscriptionAccess() {
		if !user.CanBindGroup(group.ID, group.IsExclusive) {
			return nil, ErrGroupNotAllowed
		}
		return nil, nil
	}
	if !runtime.Enabled {
		if !s.canUserBindGroup(ctx, user, group) {
			return nil, ErrGroupNotAllowed
		}
		if groupChanged {
			return nil, nil
		}
		return currentEntitlementID, nil
	}
	if explicitEntitlementSet {
		if explicitEntitlementID == nil {
			return s.resolveSubscriptionEntitlementForGroup(ctx, user.ID, group.ID, nil)
		}
		return s.resolveSubscriptionEntitlementForGroup(ctx, user.ID, group.ID, explicitEntitlementID)
	}
	if currentEntitlementID != nil {
		if entID, err := s.resolveSubscriptionEntitlementForGroup(ctx, user.ID, group.ID, currentEntitlementID); err == nil {
			return entID, nil
		}
	}
	return s.resolveSubscriptionEntitlementForGroup(ctx, user.ID, group.ID, nil)
}

func (s *APIKeyService) resolveSubscriptionEntitlementForGroup(ctx context.Context, userID, groupID int64, explicitEntitlementID *int64) (*int64, error) {
	if s.subscriptionEntitlementSvc == nil {
		return nil, ErrGroupNotAllowed
	}
	var resolution *EntitlementResolution
	var err error
	now := s.subscriptionEntitlementSvc.inputNow(time.Time{})
	if explicitEntitlementID != nil {
		resolution, err = s.subscriptionEntitlementSvc.ValidateBindingForGroup(ctx, userID, groupID, *explicitEntitlementID, now)
	} else {
		resolution, err = s.subscriptionEntitlementSvc.ResolveBindingForGroup(ctx, userID, groupID, now)
	}
	if err != nil {
		return nil, err
	}
	if resolution == nil || resolution.Entitlement == nil {
		return nil, ErrGroupNotAllowed
	}
	return cloneInt64PtrValue(resolution.Entitlement.ID), nil
}

func cloneInt64PtrValue(v int64) *int64 {
	out := v
	return &out
}

func sameInt64PtrValue(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func normalizeAPIKeyAccessSource(input *string, entitlementID *int64) (string, error) {
	if input == nil {
		if entitlementID != nil {
			return APIKeyAccessSourceEntitlement, nil
		}
		return APIKeyAccessSourceBalance, nil
	}
	source := strings.ToLower(strings.TrimSpace(*input))
	switch source {
	case APIKeyAccessSourceBalance, APIKeyAccessSourceEntitlement:
		return source, nil
	default:
		return "", ErrInvalidAccessSource
	}
}

func validateAPIKeyAccessSourceBinding(accessSource string, entitlementID *int64) error {
	switch accessSource {
	case APIKeyAccessSourceBalance:
		if entitlementID != nil {
			return ErrInvalidAccessSource
		}
	case APIKeyAccessSourceEntitlement:
		if entitlementID == nil {
			return ErrInvalidAccessSource
		}
	default:
		return ErrInvalidAccessSource
	}
	return nil
}

func (s *APIKeyService) Create(ctx context.Context, userID int64, req CreateAPIKeyRequest) (*APIKey, error) {
	// 验证用户存在
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	// 验证 IP 白名单格式
	if len(req.IPWhitelist) > 0 {
		if invalid := ip.ValidateIPPatterns(req.IPWhitelist); len(invalid) > 0 {
			return nil, fmt.Errorf("%w: %v", ErrInvalidIPPattern, invalid)
		}
	}

	// 验证 IP 黑名单格式
	if len(req.IPBlacklist) > 0 {
		if invalid := ip.ValidateIPPatterns(req.IPBlacklist); len(invalid) > 0 {
			return nil, fmt.Errorf("%w: %v", ErrInvalidIPPattern, invalid)
		}
	}

	accessSource, err := normalizeAPIKeyAccessSource(req.AccessSource, req.SubscriptionEntitlementID)
	if err != nil {
		return nil, err
	}
	if req.AccessSource != nil {
		if err := validateAPIKeyAccessSourceBinding(accessSource, req.SubscriptionEntitlementID); err != nil {
			return nil, err
		}
	}

	runtime := s.subscriptionEntitlementsRuntime(ctx)
	var group *Group

	// 验证分组权限（如果指定了分组）
	if req.GroupID != nil {
		group, err = s.groupRepo.GetByID(ctx, *req.GroupID)
		if err != nil {
			return nil, fmt.Errorf("get group: %w", err)
		}

		// 检查用户是否可以绑定该分组
		if runtime.Enabled {
			switch accessSource {
			case APIKeyAccessSourceBalance:
				if !canUserUseBalanceAccessSource(user, group) {
					return nil, ErrGroupNotAllowed
				}
			case APIKeyAccessSourceEntitlement:
				// Entitlement coverage is validated by resolveCreateAccessSourceBinding below.
			default:
				return nil, ErrInvalidAccessSource
			}
		} else if !s.canUserBindGroup(ctx, user, group) {
			return nil, ErrGroupNotAllowed
		}
	}

	if req.GroupID == nil && runtime.Enabled && req.SubscriptionEntitlementID != nil {
		return nil, ErrGroupNotAllowed
	}
	accessSource, err = normalizeAPIKeyAccessSource(req.AccessSource, req.SubscriptionEntitlementID)
	if err != nil {
		return nil, err
	}
	if req.AccessSource != nil {
		if err := validateAPIKeyAccessSourceBinding(accessSource, req.SubscriptionEntitlementID); err != nil {
			return nil, err
		}
	}
	subscriptionEntitlementID, err := s.resolveCreateAccessSourceBinding(ctx, user, group, accessSource, req.SubscriptionEntitlementID, runtime)
	if err != nil {
		return nil, err
	}

	var key string

	// 判断是否使用自定义Key
	if req.CustomKey != nil && *req.CustomKey != "" {
		// 检查限流（仅对自定义key进行限流）
		if err := s.checkAPIKeyRateLimit(ctx, userID); err != nil {
			return nil, err
		}

		// 验证自定义Key格式
		if err := s.ValidateCustomKey(*req.CustomKey); err != nil {
			return nil, err
		}

		// 检查Key是否已存在
		exists, err := s.apiKeyRepo.ExistsByKey(ctx, *req.CustomKey)
		if err != nil {
			return nil, fmt.Errorf("check key exists: %w", err)
		}
		if exists {
			// Key已存在，增加错误计数
			s.incrementAPIKeyErrorCount(ctx, userID)
			return nil, ErrAPIKeyExists
		}

		key = *req.CustomKey
	} else {
		// 生成随机API Key
		var err error
		key, err = s.GenerateKey()
		if err != nil {
			return nil, fmt.Errorf("generate key: %w", err)
		}
	}

	// 创建API Key记录
	autoSwitchGroupEnabled := true
	if req.AutoSwitchGroupEnabled != nil {
		autoSwitchGroupEnabled = *req.AutoSwitchGroupEnabled
	}
	apiKey := &APIKey{
		UserID:                    userID,
		Key:                       key,
		Name:                      html.EscapeString(req.Name),
		GroupID:                   req.GroupID,
		SubscriptionEntitlementID: subscriptionEntitlementID,
		AccessSource:              accessSource,
		AutoSwitchGroupEnabled:    autoSwitchGroupEnabled,
		Status:                    StatusActive,
		IPWhitelist:               req.IPWhitelist,
		IPBlacklist:               req.IPBlacklist,
		Quota:                     req.Quota,
		QuotaUsed:                 0,
		RateLimit5h:               req.RateLimit5h,
		RateLimit1d:               req.RateLimit1d,
		RateLimit7d:               req.RateLimit7d,
	}

	// Set expiration time if specified
	if req.ExpiresInDays != nil && *req.ExpiresInDays > 0 {
		expiresAt := time.Now().AddDate(0, 0, *req.ExpiresInDays)
		apiKey.ExpiresAt = &expiresAt
	}

	if err := s.apiKeyRepo.Create(ctx, apiKey); err != nil {
		return nil, fmt.Errorf("create api key: %w", err)
	}

	s.InvalidateAuthCacheByKey(ctx, apiKey.Key)
	s.compileAPIKeyIPRules(apiKey)

	return apiKey, nil
}

// List 获取用户的API Key列表
func (s *APIKeyService) List(ctx context.Context, userID int64, params pagination.PaginationParams, filters APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error) {
	if normalizedAPIKeySortBy(params.SortBy) == apiKeySortCurrentConcurrency {
		return s.listByCurrentConcurrency(ctx, userID, params, filters)
	}

	keys, pagination, err := s.apiKeyRepo.ListByUserID(ctx, userID, params, filters)
	if err != nil {
		return nil, nil, fmt.Errorf("list api keys: %w", err)
	}
	s.fillCurrentConcurrency(ctx, keys)
	return keys, pagination, nil
}

func (s *APIKeyService) listByCurrentConcurrency(ctx context.Context, userID int64, params pagination.PaginationParams, filters APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error) {
	repo, ok := s.apiKeyRepo.(apiKeyAllByUserIDLister)
	if !ok {
		return nil, nil, fmt.Errorf("list api keys by current concurrency: repository does not support unpaginated API key listing")
	}

	keys, err := repo.ListAllByUserID(ctx, userID, filters)
	if err != nil {
		return nil, nil, fmt.Errorf("list api keys: %w", err)
	}
	s.fillCurrentConcurrency(ctx, keys)
	sortAPIKeysByCurrentConcurrency(keys, params.NormalizedSortOrder(pagination.SortOrderDesc))
	return paginateAPIKeys(keys, params), apiKeyPaginationResult(int64(len(keys)), params), nil
}

func normalizedAPIKeySortBy(sortBy string) string {
	return strings.ToLower(strings.TrimSpace(sortBy))
}

func sortAPIKeysByCurrentConcurrency(keys []APIKey, sortOrder string) {
	desc := sortOrder != pagination.SortOrderAsc
	sort.SliceStable(keys, func(i, j int) bool {
		if keys[i].CurrentConcurrency == keys[j].CurrentConcurrency {
			if desc {
				return keys[i].ID > keys[j].ID
			}
			return keys[i].ID < keys[j].ID
		}
		if desc {
			return keys[i].CurrentConcurrency > keys[j].CurrentConcurrency
		}
		return keys[i].CurrentConcurrency < keys[j].CurrentConcurrency
	})
}

func paginateAPIKeys(keys []APIKey, params pagination.PaginationParams) []APIKey {
	if len(keys) == 0 {
		return []APIKey{}
	}
	limit := params.Limit()
	page := params.Page
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * limit
	if offset >= len(keys) {
		return []APIKey{}
	}
	end := offset + limit
	if end > len(keys) {
		end = len(keys)
	}
	return keys[offset:end]
}

func apiKeyPaginationResult(total int64, params pagination.PaginationParams) *pagination.PaginationResult {
	limit := params.Limit()
	pages := int(total) / limit
	if int(total)%limit > 0 {
		pages++
	}
	return &pagination.PaginationResult{
		Total:    total,
		Page:     params.Page,
		PageSize: limit,
		Pages:    pages,
	}
}

func (s *APIKeyService) fillCurrentConcurrency(ctx context.Context, keys []APIKey) {
	if s == nil || s.concurrencyService == nil || len(keys) == 0 {
		return
	}
	ids := make([]int64, 0, len(keys))
	for i := range keys {
		if keys[i].ID > 0 {
			ids = append(ids, keys[i].ID)
		}
	}
	counts, err := s.concurrencyService.GetAPIKeyConcurrencyBatch(ctx, ids)
	if err != nil {
		return
	}
	for i := range keys {
		keys[i].CurrentConcurrency = counts[keys[i].ID]
	}
}

func (s *APIKeyService) currentConcurrencyForAPIKey(ctx context.Context, apiKeyID int64) int {
	if s == nil || s.concurrencyService == nil || apiKeyID <= 0 {
		return 0
	}
	counts, err := s.concurrencyService.GetAPIKeyConcurrencyBatch(ctx, []int64{apiKeyID})
	if err != nil {
		return 0
	}
	return counts[apiKeyID]
}

func (s *APIKeyService) VerifyOwnership(ctx context.Context, userID int64, apiKeyIDs []int64) ([]int64, error) {
	if len(apiKeyIDs) == 0 {
		return []int64{}, nil
	}

	validIDs, err := s.apiKeyRepo.VerifyOwnership(ctx, userID, apiKeyIDs)
	if err != nil {
		return nil, fmt.Errorf("verify api key ownership: %w", err)
	}
	return validIDs, nil
}

// GetByID 根据ID获取API Key
func (s *APIKeyService) GetByID(ctx context.Context, id int64) (*APIKey, error) {
	apiKey, err := s.apiKeyRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get api key: %w", err)
	}
	s.compileAPIKeyIPRules(apiKey)
	if apiKey != nil {
		apiKey.CurrentConcurrency = s.currentConcurrencyForAPIKey(ctx, apiKey.ID)
	}
	return apiKey, nil
}

// GetByKey 根据Key字符串获取API Key（用于认证）
func (s *APIKeyService) GetByKey(ctx context.Context, key string) (*APIKey, error) {
	cacheKey := s.authCacheKey(key)

	if entry, ok := s.getAuthCacheEntry(ctx, cacheKey); ok {
		if apiKey, used, err := s.applyAuthCacheEntry(key, entry); used {
			if err != nil {
				return nil, fmt.Errorf("get api key: %w", err)
			}
			s.compileAPIKeyIPRules(apiKey)
			return apiKey, nil
		}
	}

	if s.authCfg.singleflight {
		value, err, _ := s.authGroup.Do(cacheKey, func() (any, error) {
			return s.loadAuthCacheEntry(ctx, key, cacheKey)
		})
		if err != nil {
			return nil, err
		}
		entry, _ := value.(*APIKeyAuthCacheEntry)
		if apiKey, used, err := s.applyAuthCacheEntry(key, entry); used {
			if err != nil {
				return nil, fmt.Errorf("get api key: %w", err)
			}
			s.compileAPIKeyIPRules(apiKey)
			return apiKey, nil
		}
	} else {
		entry, err := s.loadAuthCacheEntry(ctx, key, cacheKey)
		if err != nil {
			return nil, err
		}
		if apiKey, used, err := s.applyAuthCacheEntry(key, entry); used {
			if err != nil {
				return nil, fmt.Errorf("get api key: %w", err)
			}
			s.compileAPIKeyIPRules(apiKey)
			return apiKey, nil
		}
	}

	apiKey, err := s.apiKeyRepo.GetByKeyForAuth(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("get api key: %w", err)
	}
	apiKey.Key = key
	s.compileAPIKeyIPRules(apiKey)
	return apiKey, nil
}

func (s *APIKeyService) CompareAndSwapGroupID(ctx context.Context, apiKey *APIKey, oldGroupID, newGroupID int64) (bool, error) {
	if apiKey == nil {
		return false, ErrAPIKeyNotFound
	}
	if oldGroupID <= 0 || newGroupID <= 0 || oldGroupID == newGroupID {
		return false, nil
	}
	swapped, err := s.apiKeyRepo.CompareAndSwapGroupID(ctx, apiKey.ID, oldGroupID, newGroupID)
	if err != nil {
		return false, fmt.Errorf("switch api key group: %w", err)
	}
	if swapped {
		apiKey.GroupID = &newGroupID
		s.InvalidateAuthCacheByKey(ctx, apiKey.Key)
	}
	return swapped, nil
}

// Update 更新API Key
func (s *APIKeyService) Update(ctx context.Context, id int64, userID int64, req UpdateAPIKeyRequest) (*APIKey, error) {
	apiKey, err := s.apiKeyRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get api key: %w", err)
	}

	// 验证所有权
	if apiKey.UserID != userID {
		return nil, ErrInsufficientPerms
	}
	previousGroupID := cloneInt64Ptr(apiKey.GroupID)
	previousEntitlementID := cloneInt64Ptr(apiKey.SubscriptionEntitlementID)
	accessSource := apiKey.AccessSource
	if accessSource == "" {
		accessSource, err = normalizeAPIKeyAccessSource(nil, apiKey.SubscriptionEntitlementID)
		if err != nil {
			return nil, err
		}
	}
	if req.AccessSource != nil {
		accessSource, err = normalizeAPIKeyAccessSource(req.AccessSource, nil)
		if err != nil {
			return nil, err
		}
		if accessSource == APIKeyAccessSourceBalance && req.SubscriptionEntitlementIDSet && req.SubscriptionEntitlementID != nil {
			return nil, ErrInvalidAccessSource
		}
		if accessSource == APIKeyAccessSourceEntitlement && (!req.SubscriptionEntitlementIDSet || req.SubscriptionEntitlementID == nil) {
			return nil, ErrInvalidAccessSource
		}
	}
	if accessSource == APIKeyAccessSourceBalance {
		req.SubscriptionEntitlementIDSet = true
		req.SubscriptionEntitlementID = nil
	}

	// 验证 IP 白名单格式
	if len(req.IPWhitelist) > 0 {
		if invalid := ip.ValidateIPPatterns(req.IPWhitelist); len(invalid) > 0 {
			return nil, fmt.Errorf("%w: %v", ErrInvalidIPPattern, invalid)
		}
	}

	// 验证 IP 黑名单格式
	if len(req.IPBlacklist) > 0 {
		if invalid := ip.ValidateIPPatterns(req.IPBlacklist); len(invalid) > 0 {
			return nil, fmt.Errorf("%w: %v", ErrInvalidIPPattern, invalid)
		}
	}

	// 更新字段
	if req.Name != nil {
		apiKey.Name = html.EscapeString(*req.Name)
	}

	runtime := s.subscriptionEntitlementsRuntime(ctx)

	if req.GroupID != nil {
		// 验证分组权限
		user, err := s.userRepo.GetByID(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("get user: %w", err)
		}

		group, err := s.groupRepo.GetByID(ctx, *req.GroupID)
		if err != nil {
			return nil, fmt.Errorf("get group: %w", err)
		}

		if runtime.Enabled && accessSource == APIKeyAccessSourceBalance {
			if !canUserUseBalanceAccessSource(user, group) {
				return nil, ErrGroupNotAllowed
			}
		} else if runtime.Enabled && accessSource == APIKeyAccessSourceEntitlement {
			// Entitlement coverage is validated by resolveUpdateAccessSourceBinding below.
		} else if !s.canUserBindGroup(ctx, user, group) {
			return nil, ErrGroupNotAllowed
		}

		apiKey.GroupID = req.GroupID
	}

	if req.ClearGroup {
		apiKey.GroupID = nil
		apiKey.Group = nil
	}

	shouldResolveBinding := req.ClearGroup || req.GroupID != nil || req.AccessSource != nil || (runtime.Enabled && req.SubscriptionEntitlementIDSet)
	if shouldResolveBinding {
		user, err := s.userRepo.GetByID(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("get user: %w", err)
		}
		var group *Group
		if apiKey.GroupID != nil {
			group, err = s.groupRepo.GetByID(ctx, *apiKey.GroupID)
			if err != nil {
				return nil, fmt.Errorf("get group: %w", err)
			}
			apiKey.Group = group
		}
		if req.AccessSource == nil {
			if group == nil || (group.SupportsBalanceAccess() && !group.SupportsSubscriptionAccess()) {
				accessSource = APIKeyAccessSourceBalance
				req.SubscriptionEntitlementIDSet = true
				req.SubscriptionEntitlementID = nil
			}
		}
		groupChanged := req.ClearGroup || !sameInt64PtrValue(previousGroupID, apiKey.GroupID)
		entitlementID, err := s.resolveUpdateAccessSourceBinding(ctx, user, group, accessSource, previousEntitlementID, req.SubscriptionEntitlementID, req.SubscriptionEntitlementIDSet, groupChanged, runtime)
		if err != nil {
			return nil, err
		}
		apiKey.SubscriptionEntitlementID = entitlementID
	}
	apiKey.AccessSource = accessSource

	if req.Status != nil {
		apiKey.Status = *req.Status
		// 如果状态改变，清除Redis缓存
		if s.cache != nil {
			_ = s.cache.DeleteCreateAttemptCount(ctx, apiKey.UserID)
		}
	}
	if req.AutoSwitchGroupEnabled != nil {
		apiKey.AutoSwitchGroupEnabled = *req.AutoSwitchGroupEnabled
	}

	// Update quota fields
	if req.Quota != nil {
		apiKey.Quota = *req.Quota
		// If quota now has room, or is changed to unlimited, reactivate exhausted keys.
		if apiKey.Status == StatusAPIKeyQuotaExhausted && (*req.Quota <= 0 || *req.Quota > apiKey.QuotaUsed) {
			apiKey.Status = StatusActive
		}
	}
	if req.ResetQuota != nil && *req.ResetQuota {
		apiKey.QuotaUsed = 0
		// If resetting quota and status was quota_exhausted, reactivate
		if apiKey.Status == StatusAPIKeyQuotaExhausted {
			apiKey.Status = StatusActive
		}
	}
	if req.ClearExpiration {
		apiKey.ExpiresAt = nil
		// If clearing expiry and status was expired, reactivate
		if apiKey.Status == StatusAPIKeyExpired {
			apiKey.Status = StatusActive
		}
	} else if req.ExpiresAt != nil {
		apiKey.ExpiresAt = req.ExpiresAt
		// If extending expiry and status was expired, reactivate
		if apiKey.Status == StatusAPIKeyExpired && time.Now().Before(*req.ExpiresAt) {
			apiKey.Status = StatusActive
		}
	}

	// 更新 IP 限制（空数组会清空设置）
	apiKey.IPWhitelist = req.IPWhitelist
	apiKey.IPBlacklist = req.IPBlacklist

	// Update rate limit configuration
	if req.RateLimit5h != nil {
		apiKey.RateLimit5h = *req.RateLimit5h
	}
	if req.RateLimit1d != nil {
		apiKey.RateLimit1d = *req.RateLimit1d
	}
	if req.RateLimit7d != nil {
		apiKey.RateLimit7d = *req.RateLimit7d
	}
	resetRateLimit := req.ResetRateLimitUsage != nil && *req.ResetRateLimitUsage
	if resetRateLimit {
		apiKey.Usage5h = 0
		apiKey.Usage1d = 0
		apiKey.Usage7d = 0
		apiKey.Window5hStart = nil
		apiKey.Window1dStart = nil
		apiKey.Window7dStart = nil
	}

	if err := s.apiKeyRepo.Update(ctx, apiKey); err != nil {
		return nil, fmt.Errorf("update api key: %w", err)
	}

	s.InvalidateAuthCacheByKey(ctx, apiKey.Key)
	s.compileAPIKeyIPRules(apiKey)

	// Invalidate Redis rate limit cache so reset takes effect immediately
	if resetRateLimit && s.rateLimitCacheInvalid != nil {
		_ = s.rateLimitCacheInvalid.InvalidateAPIKeyRateLimit(ctx, apiKey.ID)
	}

	return apiKey, nil
}

// Delete 删除API Key
func (s *APIKeyService) Delete(ctx context.Context, id int64, userID int64) error {
	key, ownerID, err := s.apiKeyRepo.GetKeyAndOwnerID(ctx, id)
	if err != nil {
		return fmt.Errorf("get api key: %w", err)
	}

	// 验证当前用户是否为该 API Key 的所有者
	if ownerID != userID {
		return ErrInsufficientPerms
	}

	// 事务内:写审计 + 软删除(tombstone)。
	if err := s.apiKeyRepo.DeleteWithAudit(ctx, id); err != nil {
		return fmt.Errorf("delete api key: %w", err)
	}

	// 删除成功后再清理缓存,避免"缓存已清但删除失败"的竞态。
	if s.cache != nil {
		_ = s.cache.DeleteCreateAttemptCount(ctx, userID)
	}
	s.InvalidateAuthCacheByKey(ctx, key)
	s.lastUsedTouchL1.Delete(id)

	return nil
}

// ValidateKey 验证API Key是否有效（用于认证中间件）
func (s *APIKeyService) ValidateKey(ctx context.Context, key string) (*APIKey, *User, error) {
	// 获取API Key
	apiKey, err := s.GetByKey(ctx, key)
	if err != nil {
		return nil, nil, err
	}

	// 检查API Key状态
	if !apiKey.IsActive() {
		return nil, nil, infraerrors.Unauthorized("API_KEY_INACTIVE", "api key is not active")
	}

	// 获取用户信息
	user, err := s.userRepo.GetByID(ctx, apiKey.UserID)
	if err != nil {
		return nil, nil, fmt.Errorf("get user: %w", err)
	}

	// 检查用户状态
	if !user.IsActive() {
		return nil, nil, ErrUserNotActive
	}

	return apiKey, user, nil
}

// TouchLastUsed 通过防抖更新 api_keys.last_used_at，减少高频写放大。
// 该操作为尽力而为，不应阻塞主请求链路。
func (s *APIKeyService) TouchLastUsed(ctx context.Context, keyID int64) error {
	if keyID <= 0 {
		return nil
	}

	now := time.Now()
	if v, ok := s.lastUsedTouchL1.Load(keyID); ok {
		if nextAllowedAt, ok := v.(time.Time); ok && now.Before(nextAllowedAt) {
			return nil
		}
	}

	_, err, _ := s.lastUsedTouchSF.Do(strconv.FormatInt(keyID, 10), func() (any, error) {
		latest := time.Now()
		if v, ok := s.lastUsedTouchL1.Load(keyID); ok {
			if nextAllowedAt, ok := v.(time.Time); ok && latest.Before(nextAllowedAt) {
				return nil, nil
			}
		}

		if err := s.apiKeyRepo.UpdateLastUsed(ctx, keyID, latest); err != nil {
			s.lastUsedTouchL1.Store(keyID, latest.Add(apiKeyLastUsedFailBackoff))
			return nil, fmt.Errorf("touch api key last used: %w", err)
		}
		s.lastUsedTouchL1.Store(keyID, latest.Add(apiKeyLastUsedMinTouch))
		return nil, nil
	})
	return err
}

// IncrementUsage 增加API Key使用次数（可选：用于统计）
func (s *APIKeyService) IncrementUsage(ctx context.Context, keyID int64) error {
	// 使用Redis计数器
	if s.cache != nil {
		cacheKey := fmt.Sprintf("apikey:usage:%d:%s", keyID, timezone.Now().Format("2006-01-02"))
		if err := s.cache.IncrementDailyUsage(ctx, cacheKey); err != nil {
			return fmt.Errorf("increment usage: %w", err)
		}
		// 设置24小时过期
		_ = s.cache.SetDailyUsageExpiry(ctx, cacheKey, 24*time.Hour)
	}
	return nil
}

// GetAvailableGroups 获取用户有权限绑定的分组列表
// 返回用户可以选择的分组：
// - 标准类型分组：公开的（非专属）或用户被明确允许的
// - 订阅类型分组：用户有有效订阅的
func (s *APIKeyService) GetAvailableGroups(ctx context.Context, userID int64) ([]Group, error) {
	// 获取用户信息
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	// 获取所有活跃分组
	allGroups, err := s.groupRepo.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active groups: %w", err)
	}

	// 获取用户的所有有效订阅
	activeSubscriptions, err := s.userSubRepo.ListActiveByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list active subscriptions: %w", err)
	}

	// 构建订阅分组 ID 集合
	subscribedGroupIDs := make(map[int64]bool)
	for _, sub := range activeSubscriptions {
		subscribedGroupIDs[sub.GroupID] = true
	}

	// 过滤出用户有权限的分组
	availableGroups := make([]Group, 0)
	for _, group := range allGroups {
		if s.canUserBindGroupInternal(user, &group, subscribedGroupIDs) {
			availableGroups = append(availableGroups, group)
		}
	}

	return availableGroups, nil
}

func (s *APIKeyService) GetAvailableGroupsWithEntitlements(ctx context.Context, userID int64) ([]AvailableAPIKeyGroup, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	allGroups, err := s.groupRepo.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active groups: %w", err)
	}

	runtime := s.subscriptionEntitlementsRuntime(ctx)
	subscribedGroupIDs := make(map[int64]bool)
	entitlementsByGroupID := make(map[int64][]AvailableAPIKeyGroupEntitlement)
	if runtime.Enabled {
		if s.subscriptionEntitlementSvc != nil {
			activeEntitlements, err := s.subscriptionEntitlementSvc.ListActiveBindingsByUser(ctx, userID, time.Now())
			if err != nil {
				return nil, fmt.Errorf("list active entitlement bindings: %w", err)
			}
			var legacySubscriptionsByID map[int64]*UserSubscription
			if hasLegacySubscriptionEntitlements(activeEntitlements) && s.userSubRepo != nil {
				activeSubscriptions, err := s.userSubRepo.ListActiveByUserID(ctx, userID)
				if err != nil {
					return nil, fmt.Errorf("list active legacy subscriptions: %w", err)
				}
				legacySubscriptionsByID = mapLegacySubscriptionsByID(activeSubscriptions)
			}
			entitlementsByGroupID = buildAvailableEntitlementGroups(activeEntitlements, legacySubscriptionsByID)
		}
	} else {
		activeSubscriptions, err := s.userSubRepo.ListActiveByUserID(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("list active subscriptions: %w", err)
		}
		for _, sub := range activeSubscriptions {
			subscribedGroupIDs[sub.GroupID] = true
		}
	}

	availableGroups := make([]AvailableAPIKeyGroup, 0)
	for _, group := range allGroups {
		if runtime.Enabled {
			entitlements := entitlementsByGroupID[group.ID]
			accessSources := buildAvailableGroupAccessSources(user, &group, entitlements)
			if len(accessSources) > 0 {
				availableGroups = append(availableGroups, AvailableAPIKeyGroup{
					Group:         group,
					Entitlements:  entitlements,
					AccessSources: accessSources,
				})
			}
			continue
		}
		if group.IsSubscriptionType() {
			if subscribedGroupIDs[group.ID] {
				availableGroups = append(availableGroups, AvailableAPIKeyGroup{Group: group})
			}
			continue
		}
		if user.CanBindGroup(group.ID, group.IsExclusive) {
			availableGroups = append(availableGroups, AvailableAPIKeyGroup{Group: group})
		}
	}

	return availableGroups, nil
}

func canUserUseBalanceAccessSource(user *User, group *Group) bool {
	if user == nil || group == nil {
		return false
	}
	return group.IsActive() && group.BalanceEnabled && user.CanBindGroup(group.ID, group.IsExclusive)
}

func buildAvailableGroupAccessSources(user *User, group *Group, entitlements []AvailableAPIKeyGroupEntitlement) []AvailableAPIKeyGroupAccessSource {
	if group == nil {
		return nil
	}
	sources := make([]AvailableAPIKeyGroupAccessSource, 0, 1+len(entitlements))
	if canUserUseBalanceAccessSource(user, group) {
		sources = append(sources, AvailableAPIKeyGroupAccessSource{
			Type:  APIKeyAccessSourceBalance,
			Label: "Balance access",
			Name:  "Balance access",
		})
	}
	for _, entitlement := range entitlements {
		entitlementID := entitlement.ID
		expiresAt := entitlement.ExpiresAt
		sources = append(sources, AvailableAPIKeyGroupAccessSource{
			Type:             APIKeyAccessSourceEntitlement,
			Label:            entitlement.Name,
			Name:             entitlement.Name,
			EntitlementID:    &entitlementID,
			PlanID:           cloneInt64Ptr(entitlement.PlanID),
			PurchasePrice:    cloneFloat64Ptr(entitlement.PurchasePrice),
			PurchaseCurrency: entitlement.PurchaseCurrency,
			QuotaUSD:         cloneFloat64Ptr(entitlement.QuotaUSD),
			QuotaUsedUSD:     entitlement.QuotaUsedUSD,
			QuotaPeriod:      entitlement.QuotaPeriod,
			UnitCostPerUSD:   cloneFloat64Ptr(entitlement.UnitCostPerUSD),
			OveragePolicy:    entitlement.OveragePolicy,
			ExpiresAt:        &expiresAt,
		})
	}
	return sources
}

func buildAvailableEntitlementGroups(entitlements []SubscriptionEntitlement, legacySubscriptionsByID map[int64]*UserSubscription) map[int64][]AvailableAPIKeyGroupEntitlement {
	out := make(map[int64][]AvailableAPIKeyGroupEntitlement)
	now := time.Now()
	for i := range entitlements {
		ent := entitlements[i]
		groupIDs := entitlementCoveredGroupIDs(&ent)
		for _, groupID := range groupIDs {
			out[groupID] = append(out[groupID], AvailableAPIKeyGroupEntitlement{
				ID:               ent.ID,
				Name:             ent.Name,
				PlanID:           cloneInt64Ptr(ent.PlanID),
				PrimaryGroupID:   cloneInt64Ptr(ent.PrimaryGroupID),
				StartsAt:         ent.StartsAt,
				ExpiresAt:        ent.ExpiresAt,
				PurchasePrice:    cloneFloat64Ptr(ent.PurchasePrice),
				PurchaseCurrency: ent.PurchaseCurrency,
				QuotaUSD:         cloneFloat64Ptr(ent.QuotaUSD),
				QuotaUsedUSD:     entitlementDisplayPeriodUsageUSD(&ent, legacySubscriptionsByID, now),
				QuotaPeriod:      ent.QuotaPeriod,
				UnitCostPerUSD:   cloneFloat64Ptr(ent.UnitCostPerUSD),
				OveragePolicy:    ent.OveragePolicy,
			})
		}
	}
	for groupID := range out {
		sort.SliceStable(out[groupID], func(i, j int) bool {
			return out[groupID][i].ID < out[groupID][j].ID
		})
	}
	return out
}

func hasLegacySubscriptionEntitlements(entitlements []SubscriptionEntitlement) bool {
	for i := range entitlements {
		if entitlements[i].LegacySubscriptionID != nil && *entitlements[i].LegacySubscriptionID > 0 {
			return true
		}
	}
	return false
}

func mapLegacySubscriptionsByID(subscriptions []UserSubscription) map[int64]*UserSubscription {
	out := make(map[int64]*UserSubscription, len(subscriptions))
	for i := range subscriptions {
		if subscriptions[i].ID <= 0 {
			continue
		}
		out[subscriptions[i].ID] = &subscriptions[i]
	}
	return out
}

func entitlementDisplayPeriodUsageUSD(ent *SubscriptionEntitlement, legacySubscriptionsByID map[int64]*UserSubscription, now time.Time) float64 {
	used := entitlementCurrentPeriodUsageUSD(ent, now)
	if ent == nil || ent.LegacySubscriptionID == nil || *ent.LegacySubscriptionID <= 0 || len(legacySubscriptionsByID) == 0 {
		return used
	}
	if legacySub := legacySubscriptionsByID[*ent.LegacySubscriptionID]; legacySub != nil {
		legacyUsed := eligibleLegacySubscriptionUsageUSD(ent, legacySub, ent.QuotaPeriod, now)
		if legacyUsed > used {
			return legacyUsed
		}
	}
	return used
}

func eligibleLegacySubscriptionUsageUSD(ent *SubscriptionEntitlement, legacySub *UserSubscription, quotaPeriod string, now time.Time) float64 {
	if !shouldMergeLegacySubscriptionUsage(ent, legacySub, quotaPeriod, now) {
		return 0
	}
	return legacySubscriptionCurrentPeriodUsageUSD(legacySub, quotaPeriod, now)
}

func shouldMergeLegacySubscriptionUsage(ent *SubscriptionEntitlement, legacySub *UserSubscription, quotaPeriod string, now time.Time) bool {
	if ent == nil || legacySub == nil {
		return false
	}
	if legacySub.GroupID <= 0 || !entitlementCoversGroupID(ent, legacySub.GroupID) {
		return false
	}
	return entitlementAndLegacyUsageWindowsAligned(ent, legacySub, quotaPeriod, now)
}

func entitlementCoversGroupID(ent *SubscriptionEntitlement, groupID int64) bool {
	if ent == nil || groupID <= 0 {
		return false
	}
	if ent.PrimaryGroupID != nil && *ent.PrimaryGroupID == groupID {
		return true
	}
	for _, coveredGroupID := range entitlementCoveredGroupIDs(ent) {
		if coveredGroupID == groupID {
			return true
		}
	}
	return false
}

func entitlementAndLegacyUsageWindowsAligned(ent *SubscriptionEntitlement, legacySub *UserSubscription, quotaPeriod string, now time.Time) bool {
	entWindowStart := entitlementCurrentUsageWindowStart(ent, quotaPeriod, now)
	legacyWindowStart := legacySubscriptionCurrentUsageWindowStart(legacySub, quotaPeriod, now)
	if entWindowStart == nil || legacyWindowStart == nil {
		return true
	}
	return entWindowStart.Equal(*legacyWindowStart)
}

func entitlementCurrentUsageWindowStart(ent *SubscriptionEntitlement, quotaPeriod string, now time.Time) *time.Time {
	if ent == nil {
		return nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	switch quotaPeriod {
	case "daily":
		if ent.DailyWindowStart == nil {
			return nil
		}
		if ent.HasOneTimeDailyQuota() {
			t := *ent.DailyWindowStart
			return &t
		}
		return effectiveWindowStartAt(ent.DailyWindowStart, ent.StartsAt, 24*time.Hour, now)
	case "weekly":
		if ent.WeeklyWindowStart == nil {
			return nil
		}
		return effectiveWindowStartAt(ent.WeeklyWindowStart, ent.StartsAt, 7*24*time.Hour, now)
	default:
		if ent.MonthlyWindowStart == nil {
			return nil
		}
		return effectiveWindowStartAt(ent.MonthlyWindowStart, ent.StartsAt, monthlyCycleDuration, now)
	}
}

func legacySubscriptionCurrentUsageWindowStart(sub *UserSubscription, quotaPeriod string, now time.Time) *time.Time {
	if sub == nil {
		return nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	switch quotaPeriod {
	case "daily":
		if sub.DailyWindowStart == nil {
			return nil
		}
		if sub.HasOneTimeDailyQuota() {
			t := *sub.DailyWindowStart
			return &t
		}
		return effectiveWindowStartAt(sub.DailyWindowStart, sub.StartsAt, 24*time.Hour, now)
	case "weekly":
		if sub.WeeklyWindowStart == nil {
			return nil
		}
		return effectiveWindowStartAt(sub.WeeklyWindowStart, sub.StartsAt, 7*24*time.Hour, now)
	default:
		if sub.MonthlyWindowStart == nil {
			return nil
		}
		return effectiveWindowStartAt(sub.MonthlyWindowStart, sub.StartsAt, monthlyCycleDuration, now)
	}
}

func entitlementCurrentPeriodUsageUSD(ent *SubscriptionEntitlement, now time.Time) float64 {
	if ent == nil {
		return 0
	}
	if now.IsZero() {
		now = time.Now()
	}

	var used float64
	switch ent.QuotaPeriod {
	case "daily":
		if ent.NeedsDailyResetAt(now) {
			return 0
		}
		used = ent.DailyUsageUSD
	case "weekly":
		if ent.NeedsWeeklyResetAt(now) {
			return 0
		}
		used = ent.WeeklyUsageUSD
	default:
		if ent.NeedsMonthlyResetAt(now) {
			return 0
		}
		used = ent.MonthlyUsageUSD
	}
	if used < 0 {
		return 0
	}
	return used
}

func legacySubscriptionCurrentPeriodUsageUSD(sub *UserSubscription, quotaPeriod string, now time.Time) float64 {
	if sub == nil {
		return 0
	}
	if now.IsZero() {
		now = time.Now()
	}

	var used float64
	switch quotaPeriod {
	case "daily":
		if sub.DailyWindowStart != nil && !sub.HasOneTimeDailyQuota() && needsWindowResetAt(sub.DailyWindowStart, sub.StartsAt, 24*time.Hour, now) {
			return 0
		}
		used = sub.DailyUsageUSD
	case "weekly":
		if needsWindowResetAt(sub.WeeklyWindowStart, sub.StartsAt, 7*24*time.Hour, now) {
			return 0
		}
		used = sub.WeeklyUsageUSD
	default:
		if needsWindowResetAt(sub.MonthlyWindowStart, sub.StartsAt, monthlyCycleDuration, now) {
			return 0
		}
		used = sub.MonthlyUsageUSD
	}
	if used < 0 {
		return 0
	}
	return used
}

func entitlementCoveredGroupIDs(ent *SubscriptionEntitlement) []int64 {
	if ent == nil {
		return nil
	}
	seen := make(map[int64]struct{})
	out := make([]int64, 0, len(ent.GroupGrants)+len(ent.Groups))
	for _, grant := range ent.GroupGrants {
		if !grant.Enabled || grant.GroupID <= 0 {
			continue
		}
		if _, ok := seen[grant.GroupID]; ok {
			continue
		}
		seen[grant.GroupID] = struct{}{}
		out = append(out, grant.GroupID)
	}
	for _, group := range ent.Groups {
		if group.ID <= 0 {
			continue
		}
		if _, ok := seen[group.ID]; ok {
			continue
		}
		seen[group.ID] = struct{}{}
		out = append(out, group.ID)
	}
	return out
}

// canUserBindGroupInternal 内部方法，检查用户是否可以绑定分组（使用预加载的订阅数据）
func (s *APIKeyService) canUserBindGroupInternal(user *User, group *Group, subscribedGroupIDs map[int64]bool) bool {
	// 订阅类型分组：需要有效订阅
	if group.IsSubscriptionType() {
		return subscribedGroupIDs[group.ID]
	}
	// 标准类型分组：使用原有逻辑
	return user.CanBindGroup(group.ID, group.IsExclusive)
}

func (s *APIKeyService) SearchAPIKeys(ctx context.Context, userID int64, keyword string, limit int) ([]APIKey, error) {
	keys, err := s.apiKeyRepo.SearchAPIKeys(ctx, userID, keyword, limit)
	if err != nil {
		return nil, fmt.Errorf("search api keys: %w", err)
	}
	return keys, nil
}

// GetUserGroupRates 获取用户的专属分组倍率配置
// 返回 map[groupID]rateMultiplier
func (s *APIKeyService) GetUserGroupRates(ctx context.Context, userID int64) (map[int64]float64, error) {
	if s.userGroupRateRepo == nil {
		return nil, nil
	}
	rates, err := s.userGroupRateRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user group rates: %w", err)
	}
	return rates, nil
}

// CheckAPIKeyQuotaAndExpiry checks if the API key is valid for use (not expired, quota not exhausted)
// Returns nil if valid, error if invalid
func (s *APIKeyService) CheckAPIKeyQuotaAndExpiry(apiKey *APIKey) error {
	// Check expiration
	if apiKey.IsExpired() {
		return ErrAPIKeyExpired
	}

	// Check quota
	if apiKey.IsQuotaExhausted() {
		return ErrAPIKeyQuotaExhausted
	}

	return nil
}

// UpdateQuotaUsed updates the quota_used field after a request
// Also checks if quota is exhausted and updates status accordingly
func (s *APIKeyService) UpdateQuotaUsed(ctx context.Context, apiKeyID int64, cost float64) error {
	if cost <= 0 {
		return nil
	}

	type quotaStateReader interface {
		IncrementQuotaUsedAndGetState(ctx context.Context, id int64, amount float64) (*APIKeyQuotaUsageState, error)
	}

	if repo, ok := s.apiKeyRepo.(quotaStateReader); ok {
		state, err := repo.IncrementQuotaUsedAndGetState(ctx, apiKeyID, cost)
		if err != nil {
			return fmt.Errorf("increment quota used: %w", err)
		}
		if state != nil && state.Status == StatusAPIKeyQuotaExhausted && strings.TrimSpace(state.Key) != "" {
			s.InvalidateAuthCacheByKey(ctx, state.Key)
		}
		return nil
	}

	// Use repository to atomically increment quota_used
	newQuotaUsed, err := s.apiKeyRepo.IncrementQuotaUsed(ctx, apiKeyID, cost)
	if err != nil {
		return fmt.Errorf("increment quota used: %w", err)
	}

	// Check if quota is now exhausted and update status if needed
	apiKey, err := s.apiKeyRepo.GetByID(ctx, apiKeyID)
	if err != nil {
		return nil // Don't fail the request, just log
	}

	// If quota is set and now exhausted, update status
	if apiKey.Quota > 0 && newQuotaUsed >= apiKey.Quota {
		apiKey.Status = StatusAPIKeyQuotaExhausted
		if err := s.apiKeyRepo.Update(ctx, apiKey); err != nil {
			return nil // Don't fail the request
		}
		// Invalidate cache so next request sees the new status
		s.InvalidateAuthCacheByKey(ctx, apiKey.Key)
	}

	return nil
}

// GetRateLimitData returns rate limit usage and window state for an API key.
func (s *APIKeyService) GetRateLimitData(ctx context.Context, id int64) (*APIKeyRateLimitData, error) {
	return s.apiKeyRepo.GetRateLimitData(ctx, id)
}

// UpdateRateLimitUsage atomically increments rate limit usage counters in the DB.
func (s *APIKeyService) UpdateRateLimitUsage(ctx context.Context, apiKeyID int64, cost float64) error {
	if cost <= 0 {
		return nil
	}
	return s.apiKeyRepo.IncrementRateLimitUsage(ctx, apiKeyID, cost)
}
