package middleware

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// NewAPIKeyAuthMiddleware 创建 API Key 认证中间件
func NewAPIKeyAuthMiddleware(apiKeyService *service.APIKeyService, subscriptionService *service.SubscriptionService, cfg *config.Config) APIKeyAuthMiddleware {
	return APIKeyAuthMiddleware(apiKeyAuthWithSubscription(apiKeyService, subscriptionService, cfg))
}

// apiKeyAuthWithSubscription API Key认证中间件（支持订阅验证）
//
// 中间件职责分为两层：
//   - 鉴权（Authentication）：验证 Key 有效性、用户状态、IP 限制 —— 始终执行
//   - 计费执行（Billing Enforcement）：过期/配额/订阅/余额检查 —— skipBilling 时整块跳过
//
// /v1/usage 端点只需鉴权，不需要计费执行（允许过期/配额耗尽的 Key 查询自身用量）。
func apiKeyAuthWithSubscription(apiKeyService *service.APIKeyService, subscriptionService *service.SubscriptionService, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// ── 1. 提取 API Key ──────────────────────────────────────────

		queryKey := strings.TrimSpace(c.Query("key"))
		queryApiKey := strings.TrimSpace(c.Query("api_key"))
		if queryKey != "" || queryApiKey != "" {
			AbortWithError(c, 400, "api_key_in_query_deprecated", "API key in query parameter is deprecated. Please use Authorization header instead.")
			return
		}

		// 尝试从Authorization header中提取API key (Bearer scheme)
		authHeader := c.GetHeader("Authorization")
		var apiKeyString string

		if authHeader != "" {
			// 验证Bearer scheme
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
				apiKeyString = strings.TrimSpace(parts[1])
			}
		}

		// 如果Authorization header中没有，尝试从x-api-key header中提取
		if apiKeyString == "" {
			apiKeyString = c.GetHeader("x-api-key")
		}

		// 如果x-api-key header中没有，尝试从x-goog-api-key header中提取（Gemini CLI兼容）
		if apiKeyString == "" {
			apiKeyString = c.GetHeader("x-goog-api-key")
		}

		// 如果所有header都没有API key
		if apiKeyString == "" {
			AbortWithError(c, 401, "API_KEY_REQUIRED", "API key is required in Authorization header (Bearer scheme), x-api-key header, or x-goog-api-key header")
			return
		}

		// ── 2. 验证 Key 存在 ─────────────────────────────────────────

		apiKey, err := apiKeyService.GetByKey(c.Request.Context(), apiKeyString)
		if err != nil {
			if errors.Is(err, service.ErrAPIKeyNotFound) {
				AbortWithError(c, 401, "INVALID_API_KEY", "Invalid API key")
				return
			}
			AbortWithError(c, 500, "INTERNAL_ERROR", "Failed to validate API key")
			return
		}

		// apiKey 已加载（含 User/Group）。即便后续因分组停用/Key 停用/用户停用/
		// IP 限制等早退中断，也让 Ops 错误日志能回退取到 user/group/platform。
		SetOpsFallbackAPIKey(c, apiKey)

		// ── 3. 基础鉴权（始终执行） ─────────────────────────────────

		// disabled / 未知状态 → 无条件拦截（expired 和 quota_exhausted 留给计费阶段）
		if !apiKey.IsActive() &&
			apiKey.Status != service.StatusAPIKeyExpired &&
			apiKey.Status != service.StatusAPIKeyQuotaExhausted {
			AbortWithError(c, 401, "API_KEY_DISABLED", "API key is disabled")
			return
		}

		// 检查 IP 限制（白名单/黑名单）
		// 注意：错误信息故意模糊，避免暴露具体的 IP 限制机制
		if len(apiKey.IPWhitelist) > 0 || len(apiKey.IPBlacklist) > 0 {
			clientIP := ip.GetTrustedClientIP(c)
			if cfg.TrustForwardedIPForAPIKeyACL() {
				clientIP = ip.GetClientIP(c)
			}
			allowed, _ := ip.CheckIPRestrictionWithCompiledRules(clientIP, apiKey.CompiledIPWhitelist, apiKey.CompiledIPBlacklist)
			if !allowed {
				if clientIP == "" {
					clientIP = "unknown"
				}
				service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonIPRestriction)
				AbortWithError(c, 403, "ACCESS_DENIED", fmt.Sprintf("Access denied. Your IP is %s", clientIP))
				return
			}
		}

		// 检查关联的用户
		if apiKey.User == nil {
			AbortWithError(c, 401, "USER_NOT_FOUND", "User associated with API key not found")
			return
		}

		// 检查用户状态
		if !apiKey.User.IsActive() {
			AbortWithError(c, 401, "USER_INACTIVE", "User account is not active")
			return
		}
		isSubscriptionType := apiKey.Group != nil && apiKey.Group.IsSubscriptionType()
		v2EntitlementsEnabled := apiKeyService.IsSubscriptionEntitlementsV2Enabled(c.Request.Context())
		accessSource := apiKey.EffectiveAccessSource()
		useEntitlementAccess := v2EntitlementsEnabled && accessSource == service.APIKeyAccessSourceEntitlement
		if v2EntitlementsEnabled && accessSource == "" {
			AbortWithError(c, 403, "INVALID_ACCESS_SOURCE", "invalid API key access source")
			return
		}
		if useEntitlementAccess && apiKey.SubscriptionEntitlementID == nil {
			AbortWithError(c, 403, "SUBSCRIPTION_ENTITLEMENT_REQUIRED", "subscription entitlement is required for entitlement access source")
			return
		}
		groupUnavailableCode, groupUnavailableMessage, groupAvailable := validateAPIKeyGroupAvailable(apiKey)
		currentGroupUnavailable := !groupAvailable
		if currentGroupUnavailable && !useEntitlementAccess {
			service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonAPIKeyGroupUnavailable)
			AbortWithError(c, 403, groupUnavailableCode, groupUnavailableMessage)
			return
		}
		if abortIfAPIKeyGroupNotAllowed(c, apiKey, v2EntitlementsEnabled, accessSource) {
			return
		}

		// skipBilling: /v1/usage 只需鉴权，跳过所有计费执行
		skipBilling := c.Request.URL.Path == "/v1/usage"

		if !skipBilling {
			if status, code, message, blocked := apiKeyStatusBlock(apiKey); blocked {
				AbortWithError(c, status, code, message)
				return
			}
		}

		ctx := context.WithValue(c.Request.Context(), ctxkey.UserID, apiKey.User.ID)
		c.Request = c.Request.WithContext(ctx)

		// ── 4. SimpleMode → early return ─────────────────────────────

		if cfg.RunMode == config.RunModeSimple {
			c.Set(string(ContextKeyAPIKey), apiKey)
			c.Set(string(ContextKeyUser), AuthSubject{
				UserID:      apiKey.User.ID,
				Concurrency: apiKey.User.Concurrency,
			})
			c.Set(string(ContextKeyUserRole), apiKey.User.Role)
			setGroupContext(c, apiKey.Group)
			_ = apiKeyService.TouchLastUsed(c.Request.Context(), apiKey.ID)
			c.Next()
			return
		}

		// ── 5. 加载订阅（订阅模式时始终加载） ───────────────────────

		if !skipBilling {
			if status, code, message, blocked := apiKeyBillingBlock(apiKey); blocked {
				AbortWithError(c, status, code, message)
				return
			}
		}

		var subscription *service.UserSubscription
		var entitlement *service.SubscriptionEntitlement

		if useEntitlementAccess {
			if skipBilling {
				if resolved, entErr := apiKeyService.ResolveEntitlementForAPIKeyAuth(
					c.Request.Context(),
					apiKey,
					subscriptionSwitchRequestForContext(c),
					currentGroupUnavailable,
				); entErr == nil && resolved != nil {
					entitlement = resolved.Entitlement
					subscription = resolved.LegacySubscription
					if resolved.Group != nil {
						applyResolvedSubscriptionGroup(c, apiKey, resolved.Group, resolved.FromGroupID)
					}
				}
			} else {
				resolved, entErr := apiKeyService.ResolveEntitlementForAPIKeyAuth(
					c.Request.Context(),
					apiKey,
					subscriptionSwitchRequestForContext(c),
					currentGroupUnavailable,
				)
				if entErr != nil {
					AbortWithError(c, subscriptionErrorStatus(entErr), subscriptionErrorCode(entErr), subscriptionErrorMessage(entErr))
					return
				}
				if resolved != nil && resolved.Entitlement != nil {
					entitlement = resolved.Entitlement
					subscription = resolved.LegacySubscription
					if resolved.Switched {
						swapped, err := apiKeyService.CompareAndSwapGroupIDWithEntitlement(c.Request.Context(), apiKey, resolved.FromGroupID, resolved.ToGroupID, resolved.Entitlement.ID)
						if err != nil {
							AbortWithError(c, 500, "SUBSCRIPTION_SWITCH_FAILED", err.Error())
							return
						}
						if !swapped {
							apiKeyService.InvalidateAuthCacheByKey(c.Request.Context(), apiKey.Key)
							AbortWithError(c, 409, "SUBSCRIPTION_SWITCH_CONFLICT", "subscription group changed concurrently, please retry")
							return
						}
					}
					if resolved.Group != nil {
						applyResolvedSubscriptionGroup(c, apiKey, resolved.Group, resolved.FromGroupID)
					}
					if resolved.UseBalanceFallback {
						c.Set(string(ContextKeySubscriptionEntitlementBalanceFallback), true)
					}
				}
			}
		} else if !v2EntitlementsEnabled && isSubscriptionType && subscriptionService != nil {
			if skipBilling {
				sub, subErr := subscriptionService.GetActiveSubscription(
					c.Request.Context(),
					apiKey.User.ID,
					apiKey.Group.ID,
				)
				if subErr == nil {
					subscription = sub
				}
			} else {
				candidate, subErr := subscriptionService.ResolveUsableSubscriptionForAPIKeyWithRequest(
					c.Request.Context(),
					apiKey,
					subscriptionSwitchRequestForContext(c),
				)
				if subErr != nil {
					AbortWithError(c, subscriptionErrorStatus(subErr), subscriptionErrorCode(subErr), subscriptionErrorMessage(subErr))
					return
				}
				if candidate != nil && candidate.Subscription != nil {
					subscription = candidate.Subscription
					if candidate.Switched {
						swapped, err := apiKeyService.CompareAndSwapGroupID(c.Request.Context(), apiKey, candidate.FromGroupID, candidate.ToGroupID)
						if err != nil {
							AbortWithError(c, 500, "SUBSCRIPTION_SWITCH_FAILED", err.Error())
							return
						}
						if !swapped {
							apiKeyService.InvalidateAuthCacheByKey(c.Request.Context(), apiKey.Key)
							AbortWithError(c, 409, "SUBSCRIPTION_SWITCH_CONFLICT", "subscription group changed concurrently, please retry")
							return
						}
						subscriptionService.LogAutoSwitch(c.Request.Context(), apiKey, candidate)
					}
					if candidate.Group != nil {
						applyResolvedSubscriptionGroup(c, apiKey, candidate.Group, candidate.FromGroupID)
					}
				}
			}
		}

		// ── 6. 计费执行（skipBilling 时整块跳过） ────────────────────

		if !skipBilling {
			// 订阅模式：验证订阅限额
			if subscription != nil {
				needsMaintenance, validateErr := subscriptionService.ValidateAndCheckLimits(subscription, apiKey.Group)
				if needsMaintenance {
					refreshed, maintenanceErr := subscriptionService.EnsureWindowMaintenance(c.Request.Context(), subscription)
					if maintenanceErr != nil {
						AbortWithError(c, 500, "SUBSCRIPTION_MAINTENANCE_FAILED", "Failed to maintain subscription usage windows")
						return
					}
					subscription = refreshed
					_, validateErr = subscriptionService.ValidateAndCheckLimits(subscription, apiKey.Group)
				}
				if validateErr != nil {
					AbortWithError(c, subscriptionErrorStatus(validateErr), subscriptionErrorCode(validateErr), subscriptionErrorMessage(validateErr))
					return
				}
			} else if entitlement != nil {
				// Entitlement availability and quota were validated by APIKeyService.
				// Actual entitlement usage deduction is intentionally left to Task 10.
			} else {
				// 非订阅模式 或 订阅模式但 subscriptionService 未注入：回退到余额检查
				if apiKeyBalanceBelowAuthThreshold(apiKey.User.Balance, cfg) {
					AbortWithError(c, 429, "INSUFFICIENT_BALANCE", service.QuotaInsufficientMessage)
					return
				}
			}
		}

		// ── 7. 设置上下文 → Next ─────────────────────────────────────

		if subscription != nil {
			c.Set(string(ContextKeySubscription), subscription)
		}
		if entitlement != nil {
			c.Set(string(ContextKeySubscriptionEntitlement), entitlement)
		}
		c.Set(string(ContextKeyAPIKey), apiKey)
		c.Set(string(ContextKeyUser), AuthSubject{
			UserID:      apiKey.User.ID,
			Concurrency: apiKey.User.Concurrency,
		})
		c.Set(string(ContextKeyUserRole), apiKey.User.Role)
		setGroupContext(c, apiKey.Group)
		_ = apiKeyService.TouchLastUsed(c.Request.Context(), apiKey.ID)

		c.Next()
	}
}

func applyResolvedSubscriptionGroup(c *gin.Context, apiKey *service.APIKey, group *service.Group, previousGroupID int64) {
	if apiKey == nil || group == nil {
		return
	}
	if previousGroupID <= 0 && apiKey.GroupID != nil {
		previousGroupID = *apiKey.GroupID
	} else if previousGroupID <= 0 && apiKey.Group != nil {
		previousGroupID = apiKey.Group.ID
	}
	apiKey.Group = group
	groupID := group.ID
	apiKey.GroupID = &groupID
	if apiKey.User != nil && previousGroupID > 0 && previousGroupID != group.ID {
		apiKey.User.UserGroupRPMOverride = nil
	}
	setGroupContext(c, group)
}

func apiKeyStatusBlock(apiKey *service.APIKey) (status int, code string, message string, blocked bool) {
	switch apiKey.Status {
	case service.StatusAPIKeyQuotaExhausted:
		return 429, "API_KEY_QUOTA_EXHAUSTED", service.QuotaInsufficientMessage, true
	case service.StatusAPIKeyExpired:
		return 403, "API_KEY_EXPIRED", "API key 已过期", true
	}
	return 0, "", "", false
}

func apiKeyBillingBlock(apiKey *service.APIKey) (status int, code string, message string, blocked bool) {
	if status, code, message, blocked := apiKeyStatusBlock(apiKey); blocked {
		return status, code, message, true
	}
	if apiKey.IsExpired() {
		return 403, "API_KEY_EXPIRED", "API key 已过期", true
	}
	if apiKey.IsQuotaExhausted() {
		return 429, "API_KEY_QUOTA_EXHAUSTED", service.QuotaInsufficientMessage, true
	}
	return 0, "", "", false
}

func subscriptionErrorStatus(err error) int {
	if errors.Is(err, service.ErrBillingServiceUnavailable) {
		return 503
	}
	if isQuotaInsufficientError(err) {
		return 429
	}
	if errors.Is(err, service.ErrSubscriptionMaintenance) {
		return 503
	}
	return 403
}

func subscriptionErrorMessage(err error) string {
	if isQuotaInsufficientError(err) {
		return service.QuotaInsufficientMessage
	}
	return err.Error()
}

func isQuotaInsufficientError(err error) bool {
	return errors.Is(err, service.ErrDailyLimitExceeded) ||
		errors.Is(err, service.ErrWeeklyLimitExceeded) ||
		errors.Is(err, service.ErrMonthlyLimitExceeded) ||
		errors.Is(err, service.ErrSubscriptionEntitlementQuotaExceeded) ||
		errors.Is(err, service.ErrInsufficientBalance) ||
		errors.Is(err, service.ErrUserPlatformDailyQuotaExhausted) ||
		errors.Is(err, service.ErrUserPlatformWeeklyQuotaExhausted) ||
		errors.Is(err, service.ErrUserPlatformMonthlyQuotaExhausted)
}

func subscriptionErrorCode(err error) string {
	if errors.Is(err, service.ErrBillingServiceUnavailable) {
		return "billing_service_error"
	}
	if errors.Is(err, service.ErrInsufficientBalance) {
		return "INSUFFICIENT_BALANCE"
	}
	if errors.Is(err, service.ErrSubscriptionEntitlementQuotaExceeded) {
		return "SUBSCRIPTION_ENTITLEMENT_QUOTA_EXCEEDED"
	}
	if isQuotaInsufficientError(err) {
		return "USAGE_LIMIT_EXCEEDED"
	}
	if errors.Is(err, service.ErrSubscriptionNotFound) {
		return "SUBSCRIPTION_NOT_FOUND"
	}
	if errors.Is(err, service.ErrSubscriptionEntitlementNotFound) {
		return "SUBSCRIPTION_ENTITLEMENT_NOT_FOUND"
	}
	if errors.Is(err, service.ErrSubscriptionEntitlementExpired) {
		return "SUBSCRIPTION_ENTITLEMENT_EXPIRED"
	}
	if errors.Is(err, service.ErrSubscriptionEntitlementInactive) {
		return "SUBSCRIPTION_ENTITLEMENT_INACTIVE"
	}
	if errors.Is(err, service.ErrSubscriptionMaintenance) {
		return "SUBSCRIPTION_MAINTENANCE_FAILED"
	}
	if errors.Is(err, service.ErrSubscriptionEndpointUnsupported) {
		return "SUBSCRIPTION_ENDPOINT_UNSUPPORTED"
	}
	return "SUBSCRIPTION_INVALID"
}

// GetAPIKeyFromContext 从上下文中获取API key
func GetAPIKeyFromContext(c *gin.Context) (*service.APIKey, bool) {
	value, exists := c.Get(string(ContextKeyAPIKey))
	if !exists {
		return nil, false
	}
	apiKey, ok := value.(*service.APIKey)
	return apiKey, ok
}

// SetOpsFallbackAPIKey 记录已加载的 API Key，供 Ops 错误日志在鉴权早退时回退使用。
// 与 ContextKeyAPIKey 区分：写入它不代表请求已通过鉴权，因此不影响 handler、
// 审计日志等对“已鉴权”的判断。
func SetOpsFallbackAPIKey(c *gin.Context, apiKey *service.APIKey) {
	if c == nil || apiKey == nil {
		return
	}
	c.Set(string(ContextKeyOpsFallbackAPIKey), apiKey)
}

// GetOpsFallbackAPIKey 读取 Ops 错误日志专用的回退 API Key。
func GetOpsFallbackAPIKey(c *gin.Context) (*service.APIKey, bool) {
	value, exists := c.Get(string(ContextKeyOpsFallbackAPIKey))
	if !exists {
		return nil, false
	}
	apiKey, ok := value.(*service.APIKey)
	return apiKey, ok
}

// GetSubscriptionFromContext 从上下文中获取订阅信息
func GetSubscriptionFromContext(c *gin.Context) (*service.UserSubscription, bool) {
	value, exists := c.Get(string(ContextKeySubscription))
	if !exists {
		return nil, false
	}
	subscription, ok := value.(*service.UserSubscription)
	return subscription, ok
}

func GetSubscriptionEntitlementFromContext(c *gin.Context) (*service.SubscriptionEntitlement, bool) {
	value, exists := c.Get(string(ContextKeySubscriptionEntitlement))
	if !exists {
		return nil, false
	}
	entitlement, ok := value.(*service.SubscriptionEntitlement)
	return entitlement, ok
}

func GetSubscriptionEntitlementBalanceFallbackFromContext(c *gin.Context) bool {
	value, exists := c.Get(string(ContextKeySubscriptionEntitlementBalanceFallback))
	if !exists {
		return false
	}
	enabled, _ := value.(bool)
	return enabled
}

func setGroupContext(c *gin.Context, group *service.Group) {
	if !service.IsGroupContextValid(group) {
		return
	}
	if existing, ok := c.Request.Context().Value(ctxkey.Group).(*service.Group); ok && existing != nil && existing.ID == group.ID && service.IsGroupContextValid(existing) {
		return
	}
	ctx := context.WithValue(c.Request.Context(), ctxkey.Group, group)
	c.Request = c.Request.WithContext(ctx)
}

// apiKeyBalanceBelowAuthThreshold 保持鉴权层的历史语义：仅在余额耗尽（<=0）时拒绝。
// MinimumBalanceReserve 只作为 billing-cache 预检的保守下限，不得复用为鉴权硬门槛，
// 否则已配置该值的存量部署升级后，0 < balance < reserve 的用户会在所有端点被静默 403。
func apiKeyBalanceBelowAuthThreshold(balance float64, _ *config.Config) bool {
	return balance <= 0
}

func abortIfAPIKeyGroupNotAllowed(c *gin.Context, apiKey *service.APIKey, v2EntitlementsEnabled bool, accessSource string) bool {
	if validateAPIKeyGroupAllowed(apiKey, v2EntitlementsEnabled, accessSource) {
		return false
	}
	service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonAPIKeyGroupUnavailable)
	AbortWithError(c, 403, "GROUP_NOT_ALLOWED", "API Key 所属专属分组不再允许当前用户使用")
	return true
}

func validateAPIKeyGroupAllowed(apiKey *service.APIKey, v2EntitlementsEnabled bool, accessSource string) bool {
	if apiKey == nil || apiKey.GroupID == nil || apiKey.User == nil || apiKey.Group == nil {
		return true
	}
	group := apiKey.Group
	if v2EntitlementsEnabled {
		switch accessSource {
		case service.APIKeyAccessSourceBalance:
			return group.IsActive() && group.BalanceEnabled && apiKey.User.CanBindGroup(group.ID, group.IsExclusive)
		case service.APIKeyAccessSourceEntitlement:
			return true
		default:
			return false
		}
	}
	if group.IsSubscriptionType() {
		return true
	}
	return apiKey.User.CanBindGroup(group.ID, group.IsExclusive)
}

func validateAPIKeyGroupAvailable(apiKey *service.APIKey) (string, string, bool) {
	if apiKey == nil || apiKey.GroupID == nil {
		return "", "", true
	}
	group := apiKey.Group
	if group == nil || strings.EqualFold(group.Status, "deleted") {
		return "GROUP_DELETED", "API Key 所属分组已删除", false
	}
	if !group.IsActive() {
		return "GROUP_DISABLED", "API Key 所属分组已停用", false
	}
	return "", "", true
}

func subscriptionSwitchRequestForContext(c *gin.Context) service.SubscriptionSwitchRequest {
	if c == nil {
		return service.SubscriptionSwitchRequest{}
	}
	path := c.FullPath()
	if path == "" && c.Request != nil && c.Request.URL != nil {
		path = c.Request.URL.Path
	}
	return service.NewSubscriptionSwitchRequestFromPath(path)
}
