package middleware

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/googleapi"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// APIKeyAuthGoogle is a Google-style error wrapper for API key auth.
func APIKeyAuthGoogle(apiKeyService *service.APIKeyService, cfg *config.Config) gin.HandlerFunc {
	return APIKeyAuthWithSubscriptionGoogle(apiKeyService, nil, cfg)
}

// APIKeyAuthWithSubscriptionGoogle behaves like ApiKeyAuthWithSubscription but returns Google-style errors:
// {"error":{"code":401,"message":"...","status":"UNAUTHENTICATED"}}
//
// It is intended for Gemini native endpoints (/v1beta) to match Gemini SDK expectations.
func APIKeyAuthWithSubscriptionGoogle(apiKeyService *service.APIKeyService, subscriptionService *service.SubscriptionService, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		if v := strings.TrimSpace(c.Query("api_key")); v != "" {
			abortWithGoogleError(c, 400, "Query parameter api_key is deprecated. Use Authorization header or key instead.")
			return
		}
		apiKeyString := extractAPIKeyForGoogle(c)
		if apiKeyString == "" {
			abortWithGoogleError(c, 401, "API key is required")
			return
		}

		apiKey, err := apiKeyService.GetByKey(c.Request.Context(), apiKeyString)
		if err != nil {
			if errors.Is(err, service.ErrAPIKeyNotFound) {
				abortWithGoogleError(c, 401, "Invalid API key")
				return
			}
			abortWithGoogleError(c, 500, "Failed to validate API key")
			return
		}

		// 同 api_key_auth.go：早退中断前也写入 Ops 回退 key，便于错误日志展示
		// user/group/platform。
		SetOpsFallbackAPIKey(c, apiKey)

		// disabled / 未知状态 → 无条件拦截（expired 和 quota_exhausted 留给计费阶段，
		// 与主中间件 api_key_auth.go 保持一致）。

		if !apiKey.IsActive() &&
			apiKey.Status != service.StatusAPIKeyExpired &&
			apiKey.Status != service.StatusAPIKeyQuotaExhausted {
			abortWithGoogleError(c, 401, "API key is disabled")
			return
		}

		// 检查 IP 限制（白名单/黑名单）。与主中间件保持一致，避免 Gemini 端点绕过 Key 的 IP ACL。
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
				abortWithGoogleError(c, 403, fmt.Sprintf("Access denied. Your IP is %s", clientIP))
				return
			}
		}

		if apiKey.User == nil {
			abortWithGoogleError(c, 401, "User associated with API key not found")
			return
		}
		if !apiKey.User.IsActive() {
			abortWithGoogleError(c, 401, "User account is not active")
			return
		}
		isSubscriptionType := apiKey.Group != nil && apiKey.Group.IsSubscriptionType()
		v2EntitlementsEnabled := apiKeyService.IsSubscriptionEntitlementsV2Enabled(c.Request.Context())
		accessSource := apiKey.EffectiveAccessSource()
		useEntitlementAccess := v2EntitlementsEnabled && accessSource == service.APIKeyAccessSourceEntitlement
		if v2EntitlementsEnabled && accessSource == "" {
			abortWithGoogleError(c, 403, "Invalid API key access source")
			return
		}
		if useEntitlementAccess && apiKey.SubscriptionEntitlementID == nil {
			abortWithGoogleError(c, 403, "Subscription entitlement is required for entitlement access source")
			return
		}
		_, groupUnavailableMessage, groupAvailable := validateAPIKeyGroupAvailable(apiKey)
		currentGroupUnavailable := !groupAvailable
		if currentGroupUnavailable && !useEntitlementAccess {
			service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonAPIKeyGroupUnavailable)
			abortWithGoogleError(c, 403, groupUnavailableMessage)
			return
		}
		if v2EntitlementsEnabled && !validateAPIKeyGroupAllowed(apiKey, v2EntitlementsEnabled, accessSource) {
			service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonAPIKeyGroupUnavailable)
			abortWithGoogleError(c, 403, "API key group is not allowed")
			return
		}

		if status, _, message, blocked := apiKeyStatusBlock(apiKey); blocked {
			abortWithGoogleError(c, status, message)
			return
		}

		// 简易模式：跳过余额和订阅检查
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

		if status, _, message, blocked := apiKeyBillingBlock(apiKey); blocked {
			abortWithGoogleError(c, status, message)
			return
		}

		var entitlement *service.SubscriptionEntitlement
		if useEntitlementAccess {
			resolved, err := apiKeyService.ResolveEntitlementForAPIKeyAuth(
				c.Request.Context(),
				apiKey,
				subscriptionSwitchRequestForContext(c),
				currentGroupUnavailable,
			)
			if err != nil {
				abortWithGoogleError(c, subscriptionErrorStatus(err), subscriptionErrorMessage(err))
				return
			}
			if resolved != nil && resolved.Entitlement != nil {
				entitlement = resolved.Entitlement
				if resolved.Switched {
					swapped, err := apiKeyService.CompareAndSwapGroupIDWithEntitlement(c.Request.Context(), apiKey, resolved.FromGroupID, resolved.ToGroupID, resolved.Entitlement.ID)
					if err != nil {
						abortWithGoogleError(c, 500, err.Error())
						return
					}
					if !swapped {
						apiKeyService.InvalidateAuthCacheByKey(c.Request.Context(), apiKey.Key)
						abortWithGoogleError(c, 409, "Subscription group changed concurrently, please retry")
						return
					}
				}
				if resolved.Group != nil {
					applyResolvedSubscriptionGroup(c, apiKey, resolved.Group, resolved.FromGroupID)
				}
				if resolved.LegacySubscription != nil {
					c.Set(string(ContextKeySubscription), resolved.LegacySubscription)
				}
				if resolved.UseBalanceFallback {
					c.Set(string(ContextKeySubscriptionEntitlementBalanceFallback), true)
				}
			}
		} else if !v2EntitlementsEnabled && isSubscriptionType && subscriptionService != nil {
			candidate, err := subscriptionService.ResolveUsableSubscriptionForAPIKeyWithRequest(
				c.Request.Context(),
				apiKey,
				subscriptionSwitchRequestForContext(c),
			)
			if err != nil {
				abortWithGoogleError(c, subscriptionErrorStatus(err), subscriptionErrorMessage(err))
				return
			}
			if candidate != nil && candidate.Subscription != nil {
				subscription := candidate.Subscription
				if candidate.Switched {
					swapped, err := apiKeyService.CompareAndSwapGroupID(c.Request.Context(), apiKey, candidate.FromGroupID, candidate.ToGroupID)
					if err != nil {
						abortWithGoogleError(c, 500, err.Error())
						return
					}
					if !swapped {
						apiKeyService.InvalidateAuthCacheByKey(c.Request.Context(), apiKey.Key)
						abortWithGoogleError(c, 409, "Subscription group changed concurrently, please retry")
						return
					}
					subscriptionService.LogAutoSwitch(c.Request.Context(), apiKey, candidate)
				}
				if candidate.Group != nil {
					applyResolvedSubscriptionGroup(c, apiKey, candidate.Group, candidate.FromGroupID)
				}
				needsMaintenance, validateErr := subscriptionService.ValidateAndCheckLimits(subscription, apiKey.Group)
				if needsMaintenance {
					refreshed, maintenanceErr := subscriptionService.EnsureWindowMaintenance(c.Request.Context(), subscription)
					if maintenanceErr != nil {
						abortWithGoogleError(c, 500, "Failed to maintain subscription usage windows")
						return
					}
					subscription = refreshed
					_, validateErr = subscriptionService.ValidateAndCheckLimits(subscription, apiKey.Group)
				}
				if validateErr != nil {
					abortWithGoogleError(c, subscriptionErrorStatus(validateErr), subscriptionErrorMessage(validateErr))
					return
				}
				c.Set(string(ContextKeySubscription), subscription)
			}
		} else {
			if apiKeyBalanceBelowAuthThreshold(apiKey.User.Balance, cfg) {
				abortWithGoogleError(c, 429, service.QuotaInsufficientMessage)
				return
			}
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

// extractAPIKeyForGoogle extracts API key for Google/Gemini endpoints.
// Priority: x-goog-api-key > Authorization: Bearer > x-api-key > query key
// This allows OpenClaw and other clients using Bearer auth to work with Gemini endpoints.
func extractAPIKeyForGoogle(c *gin.Context) string {
	// 1) preferred: Gemini native header
	if k := strings.TrimSpace(c.GetHeader("x-goog-api-key")); k != "" {
		return k
	}

	// 2) fallback: Authorization: Bearer <key>
	auth := strings.TrimSpace(c.GetHeader("Authorization"))
	if auth != "" {
		parts := strings.SplitN(auth, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			if k := strings.TrimSpace(parts[1]); k != "" {
				return k
			}
		}
	}

	// 3) x-api-key header (backward compatibility)
	if k := strings.TrimSpace(c.GetHeader("x-api-key")); k != "" {
		return k
	}

	// 4) query parameter key (for specific paths)
	if allowGoogleQueryKey(c.Request.URL.Path) {
		if v := strings.TrimSpace(c.Query("key")); v != "" {
			return v
		}
	}

	return ""
}

func allowGoogleQueryKey(path string) bool {
	return strings.HasPrefix(path, "/v1beta") || strings.HasPrefix(path, "/antigravity/v1beta")
}

func abortWithGoogleError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"code":    status,
			"message": message,
			"status":  googleapi.HTTPStatusToGoogleStatus(status),
		},
	})
	c.Abort()
}
