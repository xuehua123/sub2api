// Package dto provides data transfer objects for HTTP handlers.
package dto

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func UserFromServiceShallow(u *service.User) *User {
	if u == nil {
		return nil
	}
	return &User{
		ID:                         u.ID,
		Email:                      u.Email,
		Username:                   u.Username,
		Role:                       u.Role,
		Balance:                    u.Balance,
		FrozenBalance:              u.FrozenBalance,
		Concurrency:                u.Concurrency,
		Status:                     u.Status,
		AllowedGroups:              u.AllowedGroups,
		ReferralEnabled:            u.ReferralEnabled,
		DefaultChatAPIKeyID:        u.DefaultChatAPIKeyID,
		LastActiveAt:               u.LastActiveAt,
		CreatedAt:                  u.CreatedAt,
		UpdatedAt:                  u.UpdatedAt,
		BalanceNotifyEnabled:       u.BalanceNotifyEnabled,
		BalanceNotifyThresholdType: u.BalanceNotifyThresholdType,
		BalanceNotifyThreshold:     u.BalanceNotifyThreshold,
		BalanceNotifyExtraEmails:   NotifyEmailEntriesFromService(u.BalanceNotifyExtraEmails),
		TotalRecharged:             u.TotalRecharged,
		RPMLimit:                   u.RPMLimit,
		DeletedAt:                  u.DeletedAt,
	}
}

func UserFromService(u *service.User) *User {
	if u == nil {
		return nil
	}
	out := UserFromServiceShallow(u)
	if len(u.APIKeys) > 0 {
		out.APIKeys = make([]APIKey, 0, len(u.APIKeys))
		for i := range u.APIKeys {
			k := u.APIKeys[i]
			out.APIKeys = append(out.APIKeys, *APIKeyFromService(&k))
		}
	}
	if len(u.Subscriptions) > 0 {
		out.Subscriptions = make([]UserSubscription, 0, len(u.Subscriptions))
		for i := range u.Subscriptions {
			s := u.Subscriptions[i]
			out.Subscriptions = append(out.Subscriptions, *UserSubscriptionFromService(&s))
		}
	}
	return out
}

// UserFromServiceAdmin converts a service User to DTO for admin users.
// It includes notes - user-facing endpoints must not use this.
func UserFromServiceAdmin(u *service.User) *AdminUser {
	if u == nil {
		return nil
	}
	base := UserFromService(u)
	if base == nil {
		return nil
	}
	base.Subscriptions = nil
	subscriptions := make([]AdminUserSubscription, 0, len(u.Subscriptions))
	for i := range u.Subscriptions {
		s := u.Subscriptions[i]
		subscriptions = append(subscriptions, *UserSubscriptionFromServiceAdmin(&s))
	}
	return &AdminUser{
		User:          *base,
		Notes:         u.Notes,
		LastUsedAt:    u.LastUsedAt,
		Subscriptions: subscriptions,
		GroupRates:    u.GroupRates,
	}
}

func APIKeyFromService(k *service.APIKey) *APIKey {
	if k == nil {
		return nil
	}
	accessSource := k.AccessSource
	if accessSource == "" {
		if k.SubscriptionEntitlementID != nil {
			accessSource = service.APIKeyAccessSourceEntitlement
		} else {
			accessSource = service.APIKeyAccessSourceBalance
		}
	}
	out := &APIKey{
		ID:                        k.ID,
		UserID:                    k.UserID,
		Key:                       k.Key,
		Name:                      k.Name,
		GroupID:                   k.GroupID,
		SubscriptionEntitlementID: k.SubscriptionEntitlementID,
		AccessSource:              accessSource,
		AutoSwitchGroupEnabled:    k.AutoSwitchGroupEnabled,
		Status:                    k.Status,
		IPWhitelist:               k.IPWhitelist,
		IPBlacklist:               k.IPBlacklist,
		LastUsedAt:                k.LastUsedAt,
		LastUsedIP:                k.LastUsedIP,
		Quota:                     k.Quota,
		QuotaUsed:                 k.QuotaUsed,
		ExpiresAt:                 k.ExpiresAt,
		CreatedAt:                 k.CreatedAt,
		UpdatedAt:                 k.UpdatedAt,
		CurrentConcurrency:        k.CurrentConcurrency,
		RateLimit5h:               k.RateLimit5h,
		RateLimit1d:               k.RateLimit1d,
		RateLimit7d:               k.RateLimit7d,
		Usage5h:                   k.EffectiveUsage5h(),
		Usage1d:                   k.EffectiveUsage1d(),
		Usage7d:                   k.EffectiveUsage7d(),
		Window5hStart:             k.Window5hStart,
		Window1dStart:             k.Window1dStart,
		Window7dStart:             k.Window7dStart,
		User:                      UserFromServiceShallow(k.User),
		Group:                     GroupFromServiceShallow(k.Group),
	}
	if k.Window5hStart != nil && !service.IsWindowExpired(k.Window5hStart, service.RateLimitWindow5h) {
		t := k.Window5hStart.Add(service.RateLimitWindow5h)
		out.Reset5hAt = &t
	}
	if k.Window1dStart != nil && !service.IsWindowExpired(k.Window1dStart, service.RateLimitWindow1d) {
		t := k.Window1dStart.Add(service.RateLimitWindow1d)
		out.Reset1dAt = &t
	}
	if k.Window7dStart != nil && !service.IsWindowExpired(k.Window7dStart, service.RateLimitWindow7d) {
		t := k.Window7dStart.Add(service.RateLimitWindow7d)
		out.Reset7dAt = &t
	}
	return out
}

func GroupFromServiceShallow(g *service.Group) *Group {
	if g == nil {
		return nil
	}
	out := groupFromServiceBase(g)
	return &out
}

func GroupFromService(g *service.Group) *Group {
	if g == nil {
		return nil
	}
	return GroupFromServiceShallow(g)
}

// GroupFromServiceAdmin converts a service Group to DTO for admin users.
// It includes internal fields like model_routing and account_count.
func GroupFromServiceAdmin(g *service.Group) *AdminGroup {
	if g == nil {
		return nil
	}
	out := &AdminGroup{
		Group:                       groupFromServiceBase(g),
		ProfitControlEnabled:        g.ProfitControlEnabled,
		ProfitMinMargin:             g.ProfitMinMargin,
		ProfitSafetyBuffer:          g.ProfitSafetyBuffer,
		ModelRouting:                g.ModelRouting,
		ModelRoutingEnabled:         g.ModelRoutingEnabled,
		MCPXMLInject:                g.MCPXMLInject,
		DefaultMappedModel:          g.DefaultMappedModel,
		MessagesDispatchModelConfig: g.MessagesDispatchModelConfig,
		ModelsListConfig:            g.ModelsListConfig,
		SupportedModelScopes:        g.SupportedModelScopes,
		AccountCount:                g.AccountCount,
		ActiveAccountCount:          g.ActiveAccountCount,
		RateLimitedAccountCount:     g.RateLimitedAccountCount,
		SortOrder:                   g.SortOrder,
	}
	if len(g.AccountGroups) > 0 {
		out.AccountGroups = make([]AccountGroup, 0, len(g.AccountGroups))
		for i := range g.AccountGroups {
			ag := g.AccountGroups[i]
			out.AccountGroups = append(out.AccountGroups, *AccountGroupFromService(&ag))
		}
	}
	return out
}

func groupFromServiceBase(g *service.Group) Group {
	return Group{
		ID:                              g.ID,
		Name:                            g.Name,
		Description:                     g.Description,
		Platform:                        g.Platform,
		RateMultiplier:                  g.RateMultiplier,
		SortOrder:                       g.SortOrder,
		IsExclusive:                     g.IsExclusive,
		Status:                          g.Status,
		SubscriptionType:                g.SubscriptionType,
		BalanceEnabled:                  g.BalanceEnabled,
		SubscriptionEnabled:             g.SubscriptionEnabled,
		PlanAutoGrantEnabled:            g.PlanAutoGrantEnabled,
		DailyLimitUSD:                   g.DailyLimitUSD,
		WeeklyLimitUSD:                  g.WeeklyLimitUSD,
		MonthlyLimitUSD:                 g.MonthlyLimitUSD,
		AllowImageGeneration:            g.AllowImageGeneration,
		AllowBatchImageGeneration:       g.AllowBatchImageGeneration,
		ImageRateIndependent:            g.ImageRateIndependent,
		ImageRateMultiplier:             g.ImageRateMultiplier,
		BatchImageDiscountMultiplier:    g.BatchImageDiscountMultiplier,
		BatchImageHoldMultiplier:        g.BatchImageHoldMultiplier,
		VideoRateIndependent:            g.VideoRateIndependent,
		VideoRateMultiplier:             g.VideoRateMultiplier,
		PeakRateEnabled:                 g.PeakRateEnabled,
		PeakStart:                       g.PeakStart,
		PeakEnd:                         g.PeakEnd,
		PeakRateMultiplier:              g.PeakRateMultiplier,
		ImagePrice1K:                    g.ImagePrice1K,
		ImagePrice2K:                    g.ImagePrice2K,
		ImagePrice4K:                    g.ImagePrice4K,
		VideoPrice480P:                  g.VideoPrice480P,
		VideoPrice720P:                  g.VideoPrice720P,
		VideoPrice1080P:                 g.VideoPrice1080P,
		WebSearchPricePerCall:           g.WebSearchPricePerCall,
		ClaudeCodeOnly:                  g.ClaudeCodeOnly,
		FallbackGroupID:                 g.FallbackGroupID,
		FallbackGroupIDOnInvalidRequest: g.FallbackGroupIDOnInvalidRequest,
		AllowMessagesDispatch:           g.AllowMessagesDispatch,
		AllowLive:                       g.AllowLive,
		RequireOAuthOnly:                g.RequireOAuthOnly,
		RequirePrivacySet:               g.RequirePrivacySet,
		RPMLimit:                        g.RPMLimit,
		MaxReasoningEffort:              g.MaxReasoningEffort,
		ReasoningEffortMappings:         g.ReasoningEffortMappings,
		CreatedAt:                       g.CreatedAt,
		UpdatedAt:                       g.UpdatedAt,
	}
}

func AccountFromServiceShallow(a *service.Account) *Account {
	if a == nil {
		return nil
	}
	redactedCreds, credsStatus := RedactCredentials(a.Credentials)
	delete(redactedCreds, "upstream_management_auth")
	delete(redactedCreds, "upstream_management_base_url")
	delete(credsStatus, "has_upstream_management_auth")
	if len(credsStatus) == 0 {
		credsStatus = nil
	}
	extra := redactAccountManagedExtra(a.Extra)
	var ollamaCloudUsage *service.OllamaCloudUsageState
	if state := service.OllamaCloudUsageStateFromAccount(a); state.Eligible {
		ollamaCloudUsage = state
	}
	out := &Account{
		ID:                      a.ID,
		Name:                    a.Name,
		Notes:                   a.Notes,
		Platform:                a.Platform,
		Type:                    a.Type,
		Credentials:             redactedCreds,
		CredentialsStatus:       credsStatus,
		Extra:                   extra,
		OllamaCloudUsage:        ollamaCloudUsage,
		ProxyID:                 a.ProxyID,
		ProxyFallbackOriginID:   a.ProxyFallbackOriginID,
		ProxyFallbackOriginName: a.ProxyFallbackOriginName,
		Concurrency:             a.Concurrency,
		LoadFactor:              a.LoadFactor,
		Priority:                a.Priority,
		RateMultiplier:          a.BillingRateMultiplier(),
		Status:                  a.Status,
		ErrorMessage:            a.ErrorMessage,
		LastUsedAt:              a.LastUsedAt,
		ExpiresAt:               timeToUnixSeconds(a.ExpiresAt),
		AutoPauseOnExpired:      a.AutoPauseOnExpired,
		CreatedAt:               a.CreatedAt,
		UpdatedAt:               a.UpdatedAt,
		Schedulable:             a.Schedulable,
		RateLimitedAt:           a.RateLimitedAt,
		RateLimitResetAt:        a.RateLimitResetAt,
		OverloadUntil:           a.OverloadUntil,
		TempUnschedulableUntil:  a.TempUnschedulableUntil,
		TempUnschedulableReason: a.TempUnschedulableReason,
		SessionWindowStart:      a.SessionWindowStart,
		SessionWindowEnd:        a.SessionWindowEnd,
		SessionWindowStatus:     a.SessionWindowStatus,
		GroupIDs:                a.GroupIDs,
		ParentAccountID:         a.ParentAccountID,
		QuotaDimension:          a.QuotaDimension,
	}

	// 提取 5h 窗口费用控制和会话数量控制配置（仅 Anthropic OAuth/SetupToken 账号有效）
	if a.IsAnthropicOAuthOrSetupToken() {
		if limit := a.GetWindowCostLimit(); limit > 0 {
			out.WindowCostLimit = &limit
		}
		if reserve := a.GetWindowCostStickyReserve(); reserve > 0 {
			out.WindowCostStickyReserve = &reserve
		}
		if maxSessions := a.GetMaxSessions(); maxSessions > 0 {
			out.MaxSessions = &maxSessions
		}
		if idleTimeout := a.GetSessionIdleTimeoutMinutes(); idleTimeout > 0 {
			out.SessionIdleTimeoutMin = &idleTimeout
		}
		if rpm := a.GetBaseRPM(); rpm > 0 {
			out.BaseRPM = &rpm
			strategy := a.GetRPMStrategy()
			out.RPMStrategy = &strategy
			buffer := a.GetRPMStickyBuffer()
			out.RPMStickyBuffer = &buffer
		}
		// 用户消息队列模式
		if mode := a.GetUserMsgQueueMode(); mode != "" {
			out.UserMsgQueueMode = &mode
		}
		// TLS指纹伪装开关
		if a.IsTLSFingerprintEnabled() {
			enabled := true
			out.EnableTLSFingerprint = &enabled
		}
		// TLS指纹模板ID
		if profileID := a.GetTLSFingerprintProfileID(); profileID > 0 {
			out.TLSFingerprintProfileID = &profileID
		}
		// 会话ID伪装开关
		if a.IsSessionIDMaskingEnabled() {
			enabled := true
			out.EnableSessionIDMasking = &enabled
		}
		// 缓存 TTL 强制替换
		if a.IsCacheTTLOverrideEnabled() {
			enabled := true
			out.CacheTTLOverrideEnabled = &enabled
			target := a.GetCacheTTLOverrideTarget()
			out.CacheTTLOverrideTarget = &target
		}
		// 自定义 Base URL 中继转发
		if a.IsCustomBaseURLEnabled() {
			enabled := true
			out.CustomBaseURLEnabled = &enabled
			if customURL := a.GetCustomBaseURL(); customURL != "" {
				out.CustomBaseURL = &customURL
			}
		}
	}

	// 提取账号配额限制（apikey / bedrock 类型有效）
	if a.IsAPIKeyOrBedrock() {
		if limit := a.GetQuotaLimit(); limit > 0 {
			out.QuotaLimit = &limit
			used := a.GetQuotaUsed()
			out.QuotaUsed = &used
		}
		if limit := a.GetQuotaDailyLimit(); limit > 0 {
			out.QuotaDailyLimit = &limit
			used := a.GetQuotaDailyUsed()
			if a.IsDailyQuotaPeriodExpired() {
				used = 0
			}
			out.QuotaDailyUsed = &used
		}
		if limit := a.GetQuotaWeeklyLimit(); limit > 0 {
			out.QuotaWeeklyLimit = &limit
			used := a.GetQuotaWeeklyUsed()
			if a.IsWeeklyQuotaPeriodExpired() {
				used = 0
			}
			out.QuotaWeeklyUsed = &used
		}
		// 固定时间重置配置
		if mode := a.GetQuotaDailyResetMode(); mode == "fixed" {
			out.QuotaDailyResetMode = &mode
			hour := a.GetQuotaDailyResetHour()
			out.QuotaDailyResetHour = &hour
		}
		if mode := a.GetQuotaWeeklyResetMode(); mode == "fixed" {
			out.QuotaWeeklyResetMode = &mode
			day := a.GetQuotaWeeklyResetDay()
			out.QuotaWeeklyResetDay = &day
			hour := a.GetQuotaWeeklyResetHour()
			out.QuotaWeeklyResetHour = &hour
		}
		if a.GetQuotaDailyResetMode() == "fixed" || a.GetQuotaWeeklyResetMode() == "fixed" {
			tz := a.GetQuotaResetTimezone()
			out.QuotaResetTimezone = &tz
		}
		if a.Extra != nil {
			if v, ok := a.Extra["quota_daily_reset_at"].(string); ok && v != "" {
				out.QuotaDailyResetAt = &v
			}
			if v, ok := a.Extra["quota_weekly_reset_at"].(string); ok && v != "" {
				out.QuotaWeeklyResetAt = &v
			}
		}

		// 配额通知配置
		if enabled := a.GetQuotaNotifyDailyEnabled(); enabled {
			out.QuotaNotifyDailyEnabled = &enabled
		}
		if threshold := a.GetQuotaNotifyDailyThreshold(); threshold > 0 {
			out.QuotaNotifyDailyThreshold = &threshold
		}
		if enabled := a.GetQuotaNotifyWeeklyEnabled(); enabled {
			out.QuotaNotifyWeeklyEnabled = &enabled
		}
		if threshold := a.GetQuotaNotifyWeeklyThreshold(); threshold > 0 {
			out.QuotaNotifyWeeklyThreshold = &threshold
		}
		if enabled := a.GetQuotaNotifyTotalEnabled(); enabled {
			out.QuotaNotifyTotalEnabled = &enabled
		}
		if threshold := a.GetQuotaNotifyTotalThreshold(); threshold > 0 {
			out.QuotaNotifyTotalThreshold = &threshold
		}
	}

	return out
}

func redactAccountManagedExtra(extra map[string]any) map[string]any {
	if len(extra) == 0 {
		return extra
	}
	redacted := make(map[string]any, len(extra))
	for key, value := range extra {
		if strings.HasPrefix(key, "balance_probe_") ||
			strings.HasPrefix(key, "upstream_billing_probe") ||
			strings.HasPrefix(key, "upstream_rate_multiplier_sync_") {
			continue
		}
		switch key {
		case service.OllamaCloudUsageSessionExtraKey,
			service.OllamaCloudUsageAutoRefreshExtraKey,
			service.OllamaCloudUsageSnapshotExtraKey:
			continue
		default:
			redacted[key] = value
		}
	}
	return redacted
}

func AccountFromService(a *service.Account) *Account {
	if a == nil {
		return nil
	}
	out := AccountFromServiceShallow(a)
	out.Proxy = ProxyFromService(a.Proxy)
	if len(a.AccountGroups) > 0 {
		out.AccountGroups = make([]AccountGroup, 0, len(a.AccountGroups))
		for i := range a.AccountGroups {
			ag := a.AccountGroups[i]
			out.AccountGroups = append(out.AccountGroups, *AccountGroupFromService(&ag))
		}
	}
	if len(a.Groups) > 0 {
		out.Groups = make([]*Group, 0, len(a.Groups))
		for _, g := range a.Groups {
			out.Groups = append(out.Groups, GroupFromServiceShallow(g))
		}
	}
	return out
}

func timeToUnixSeconds(value *time.Time) *int64 {
	if value == nil {
		return nil
	}
	ts := value.Unix()
	return &ts
}

func AccountGroupFromService(ag *service.AccountGroup) *AccountGroup {
	if ag == nil {
		return nil
	}
	return &AccountGroup{
		AccountID: ag.AccountID,
		GroupID:   ag.GroupID,
		Priority:  ag.Priority,
		CreatedAt: ag.CreatedAt,
		Account:   AccountFromServiceShallow(ag.Account),
		Group:     GroupFromServiceShallow(ag.Group),
	}
}

func ProxyFromService(p *service.Proxy) *Proxy {
	if p == nil {
		return nil
	}
	return &Proxy{
		ID:             p.ID,
		Name:           p.Name,
		Protocol:       p.Protocol,
		Host:           p.Host,
		Port:           p.Port,
		Username:       p.Username,
		Status:         p.Status,
		CreatedAt:      p.CreatedAt,
		UpdatedAt:      p.UpdatedAt,
		ExpiresAt:      p.ExpiresAt,
		FallbackMode:   p.FallbackMode,
		BackupProxyID:  p.BackupProxyID,
		ExpiryWarnDays: p.ExpiryWarnDays,
	}
}

func ProxyWithAccountCountFromService(p *service.ProxyWithAccountCount) *ProxyWithAccountCount {
	if p == nil {
		return nil
	}
	return &ProxyWithAccountCount{
		Proxy:          *ProxyFromService(&p.Proxy),
		AccountCount:   p.AccountCount,
		LatencyMs:      p.LatencyMs,
		LatencyStatus:  p.LatencyStatus,
		LatencyMessage: p.LatencyMessage,
		IPAddress:      p.IPAddress,
		Country:        p.Country,
		CountryCode:    p.CountryCode,
		Region:         p.Region,
		City:           p.City,
		QualityStatus:  p.QualityStatus,
		QualityScore:   p.QualityScore,
		QualityGrade:   p.QualityGrade,
		QualitySummary: p.QualitySummary,
		QualityChecked: p.QualityChecked,
	}
}

// ProxyFromServiceAdmin converts a service Proxy to AdminProxy DTO for admin users.
// It includes the password field - user-facing endpoints must not use this.
func ProxyFromServiceAdmin(p *service.Proxy) *AdminProxy {
	if p == nil {
		return nil
	}
	base := ProxyFromService(p)
	if base == nil {
		return nil
	}
	return &AdminProxy{
		Proxy:    *base,
		Password: p.Password,
	}
}

// ProxyWithAccountCountFromServiceAdmin converts a service ProxyWithAccountCount to AdminProxyWithAccountCount DTO.
// It includes the password field - user-facing endpoints must not use this.
func ProxyWithAccountCountFromServiceAdmin(p *service.ProxyWithAccountCount) *AdminProxyWithAccountCount {
	if p == nil {
		return nil
	}
	admin := ProxyFromServiceAdmin(&p.Proxy)
	if admin == nil {
		return nil
	}
	return &AdminProxyWithAccountCount{
		AdminProxy:     *admin,
		AccountCount:   p.AccountCount,
		LatencyMs:      p.LatencyMs,
		LatencyStatus:  p.LatencyStatus,
		LatencyMessage: p.LatencyMessage,
		IPAddress:      p.IPAddress,
		Country:        p.Country,
		CountryCode:    p.CountryCode,
		Region:         p.Region,
		City:           p.City,
		QualityStatus:  p.QualityStatus,
		QualityScore:   p.QualityScore,
		QualityGrade:   p.QualityGrade,
		QualitySummary: p.QualitySummary,
		QualityChecked: p.QualityChecked,
	}
}

func ProxyAccountSummaryFromService(a *service.ProxyAccountSummary) *ProxyAccountSummary {
	if a == nil {
		return nil
	}
	return &ProxyAccountSummary{
		ID:       a.ID,
		Name:     a.Name,
		Platform: a.Platform,
		Type:     a.Type,
		Notes:    a.Notes,
	}
}

func RedeemCodeFromService(rc *service.RedeemCode) *RedeemCode {
	if rc == nil {
		return nil
	}
	out := redeemCodeFromServiceBase(rc)
	return &out
}

// RedeemCodeFromServiceAdmin converts a service RedeemCode to DTO for admin users.
// It includes notes - user-facing endpoints must not use this.
func RedeemCodeFromServiceAdmin(rc *service.RedeemCode) *AdminRedeemCode {
	if rc == nil {
		return nil
	}
	return &AdminRedeemCode{
		RedeemCode: redeemCodeFromServiceBase(rc),
		Notes:      rc.Notes,
	}
}

func redeemCodeFromServiceBase(rc *service.RedeemCode) RedeemCode {
	out := RedeemCode{
		ID:                        rc.ID,
		Code:                      rc.Code,
		Type:                      rc.Type,
		Value:                     rc.Value,
		Status:                    rc.Status,
		UsedBy:                    rc.UsedBy,
		UsedAt:                    rc.UsedAt,
		CreatedAt:                 rc.CreatedAt,
		ExpiresAt:                 rc.ExpiresAt,
		GroupID:                   rc.GroupID,
		PlanID:                    rc.PlanID,
		SubscriptionEntitlementID: rc.SubscriptionEntitlementID,
		ValidityDays:              rc.ValidityDays,
		User:                      UserFromServiceShallow(rc.User),
		Group:                     GroupFromServiceShallow(rc.Group),
	}
	if rc.IsExpired() {
		out.Status = service.StatusExpired
	}

	// For admin_balance/admin_concurrency types, include notes so users can see
	// why they were charged or credited by admin
	if (rc.Type == "admin_balance" || rc.Type == "admin_concurrency") && rc.Notes != "" {
		out.Notes = &rc.Notes
	}

	return out
}

// AccountSummaryFromService returns a minimal AccountSummary for usage log display.
// Only includes ID and Name - no sensitive fields like Credentials, Proxy, etc.
func AccountSummaryFromService(a *service.Account) *AccountSummary {
	if a == nil {
		return nil
	}
	return &AccountSummary{
		ID:   a.ID,
		Name: a.Name,
	}
}

func usageLogFromServiceUser(l *service.UsageLog) UsageLog {
	// 普通用户 DTO：严禁包含管理员字段（例如 account_rate_multiplier、account、upstream_model）。
	requestType := l.EffectiveRequestType()
	stream, openAIWSMode := service.ApplyLegacyRequestFields(requestType, l.Stream, l.OpenAIWSMode)
	requestedModel := l.RequestedModel
	if requestedModel == "" {
		requestedModel = l.Model
	}
	return UsageLog{
		ID:                        l.ID,
		UserID:                    l.UserID,
		APIKeyID:                  l.APIKeyID,
		AccountID:                 l.AccountID,
		RequestID:                 l.RequestID,
		Model:                     requestedModel,
		ServiceTier:               l.ServiceTier,
		ReasoningEffort:           l.ReasoningEffort,
		InboundEndpoint:           l.InboundEndpoint,
		GroupID:                   l.GroupID,
		SubscriptionID:            l.SubscriptionID,
		EntitlementID:             l.EntitlementID,
		InputTokens:               l.InputTokens,
		OutputTokens:              l.OutputTokens,
		CacheCreationTokens:       l.CacheCreationTokens,
		CacheReadTokens:           l.CacheReadTokens,
		CacheCreation5mTokens:     l.CacheCreation5mTokens,
		CacheCreation1hTokens:     l.CacheCreation1hTokens,
		InputCost:                 l.InputCost,
		OutputCost:                l.OutputCost,
		CacheCreationCost:         l.CacheCreationCost,
		CacheReadCost:             l.CacheReadCost,
		TotalCost:                 l.TotalCost,
		ActualCost:                l.ActualCost,
		RateMultiplier:            l.RateMultiplier,
		LongContextBillingApplied: l.LongContextBillingApplied,
		BillingType:               l.BillingType,
		RequestType:               requestType.String(),
		Stream:                    stream,
		OpenAIWSMode:              openAIWSMode,
		DurationMs:                l.DurationMs,
		FirstTokenMs:              l.FirstTokenMs,
		FirstSSEEventMs:           service.FirstSSEEventMsOrFallback(l.FirstSSEEventMs, l.FirstTokenMs),
		FirstClientFlushMs:        l.FirstClientFlushMs,
		ImageCount:                l.ImageCount,
		ImageSize:                 l.ImageSize,
		ImageInputSize:            l.ImageInputSize,
		ImageOutputSize:           l.ImageOutputSize,
		ImageInputTokens:          l.ImageInputTokens,
		ImageInputCost:            l.ImageInputCost,
		ImageOutputTokens:         l.ImageOutputTokens,
		ImageOutputCost:           l.ImageOutputCost,
		ImageSizeSource:           l.ImageSizeSource,
		ImageSizeBreakdown:        l.ImageSizeBreakdown,
		MediaType:                 l.MediaType,
		UserAgent:                 l.UserAgent,
		IPAddress:                 l.IPAddress,
		SessionID:                 l.SessionID,
		CacheTTLOverridden:        l.CacheTTLOverridden,
		BillingMode:               l.BillingMode,
		CreatedAt:                 l.CreatedAt,
		User:                      UserFromServiceShallow(l.User),
		APIKey:                    APIKeyFromService(l.APIKey),
		Group:                     GroupFromServiceShallow(l.Group),
		Subscription:              UserSubscriptionFromService(l.Subscription),
	}
}

// UsageLogFromService converts a service UsageLog to DTO for regular users.
// It excludes admin-only account/upstream internals while keeping user billing and request metadata.
func UsageLogFromService(l *service.UsageLog) *UsageLog {
	if l == nil {
		return nil
	}
	u := usageLogFromServiceUser(l)
	return &u
}

// UsageLogFromServiceAdmin converts a service UsageLog to DTO for admin users.
// It includes minimal Account info (ID, Name only) and IP address.
func UsageLogFromServiceAdmin(l *service.UsageLog) *AdminUsageLog {
	if l == nil {
		return nil
	}
	var billingSource *string
	if source := l.EffectiveBillingSource(); source != "" {
		billingSource = &source
	}
	usageLog := usageLogFromServiceUser(l)
	usageLog.UpstreamEndpoint = l.UpstreamEndpoint
	return &AdminUsageLog{
		UsageLog:              usageLog,
		UpstreamModel:         l.UpstreamModel,
		UpstreamResponseModel: l.UpstreamResponseModel,
		UpstreamModelMismatch: l.UpstreamModelMismatch,
		ChannelID:             l.ChannelID,
		BillingSource:         billingSource,
		ModelMappingChain:     l.ModelMappingChain,
		BillingTier:           l.BillingTier,
		AccountRateMultiplier: l.AccountRateMultiplier,
		AccountStatsCost:      l.AccountStatsCost,
		IPAddress:             l.IPAddress,
		Account:               AccountSummaryFromService(l.Account),
	}
}

func UsageCleanupTaskFromService(task *service.UsageCleanupTask) *UsageCleanupTask {
	if task == nil {
		return nil
	}
	return &UsageCleanupTask{
		ID:     task.ID,
		Status: task.Status,
		Filters: UsageCleanupFilters{
			StartTime:   task.Filters.StartTime,
			EndTime:     task.Filters.EndTime,
			UserID:      task.Filters.UserID,
			APIKeyID:    task.Filters.APIKeyID,
			AccountID:   task.Filters.AccountID,
			GroupID:     task.Filters.GroupID,
			Model:       task.Filters.Model,
			RequestType: requestTypeStringPtr(task.Filters.RequestType),
			Stream:      task.Filters.Stream,
			BillingType: task.Filters.BillingType,
		},
		CreatedBy:    task.CreatedBy,
		DeletedRows:  task.DeletedRows,
		ErrorMessage: task.ErrorMsg,
		CanceledBy:   task.CanceledBy,
		CanceledAt:   task.CanceledAt,
		StartedAt:    task.StartedAt,
		FinishedAt:   task.FinishedAt,
		CreatedAt:    task.CreatedAt,
		UpdatedAt:    task.UpdatedAt,
	}
}

func requestTypeStringPtr(requestType *int16) *string {
	if requestType == nil {
		return nil
	}
	value := service.RequestTypeFromInt16(*requestType).String()
	return &value
}

func SettingFromService(s *service.Setting) *Setting {
	if s == nil {
		return nil
	}
	return &Setting{
		ID:        s.ID,
		Key:       s.Key,
		Value:     s.Value,
		UpdatedAt: s.UpdatedAt,
	}
}

func UserSubscriptionFromService(sub *service.UserSubscription) *UserSubscription {
	if sub == nil {
		return nil
	}
	out := userSubscriptionFromServiceBase(sub)
	return &out
}

func UserSubscriptionAliasFromEntitlement(ent *service.SubscriptionEntitlement) *UserSubscriptionAlias {
	if ent == nil || ent.LegacySubscriptionID == nil {
		return nil
	}
	groupID, group := entitlementAliasPrimaryGroup(ent)
	if groupID <= 0 {
		return nil
	}
	groupDTO := GroupFromServiceShallow(group)
	if groupDTO != nil {
		groupDTO.DailyLimitUSD = cloneFloat64(ent.DailyLimitUSD)
		groupDTO.WeeklyLimitUSD = cloneFloat64(ent.WeeklyLimitUSD)
		groupDTO.MonthlyLimitUSD = cloneFloat64(ent.MonthlyLimitUSD)
	}
	return &UserSubscriptionAlias{
		ID:                 *ent.LegacySubscriptionID,
		UserID:             ent.UserID,
		GroupID:            groupID,
		StartsAt:           ent.StartsAt,
		ExpiresAt:          ent.ExpiresAt,
		Status:             ent.Status,
		DailyWindowStart:   cloneTime(ent.DailyWindowStart),
		WeeklyWindowStart:  cloneTime(ent.WeeklyWindowStart),
		MonthlyWindowStart: cloneTime(ent.MonthlyWindowStart),
		DailyUsageUSD:      ent.DailyUsageUSD,
		WeeklyUsageUSD:     ent.WeeklyUsageUSD,
		MonthlyUsageUSD:    ent.MonthlyUsageUSD,
		DailyLimitUSD:      cloneFloat64(ent.DailyLimitUSD),
		WeeklyLimitUSD:     cloneFloat64(ent.WeeklyLimitUSD),
		MonthlyLimitUSD:    cloneFloat64(ent.MonthlyLimitUSD),
		CreatedAt:          ent.CreatedAt,
		UpdatedAt:          ent.UpdatedAt,
		Group:              groupDTO,
		EntitlementID:      ent.ID,
		PlanID:             cloneInt64(ent.PlanID),
		PlanName:           ent.Name,
		Groups:             userEntitlementGroups(ent),
		OveragePolicy:      ent.OveragePolicy,
	}
}

func UserSubscriptionAliasesFromEntitlements(in []service.SubscriptionEntitlement) []UserSubscriptionAlias {
	out := make([]UserSubscriptionAlias, 0, len(in))
	for i := range in {
		if alias := UserSubscriptionAliasFromEntitlement(&in[i]); alias != nil {
			out = append(out, *alias)
		}
	}
	return out
}

func UserEntitlementFromService(ent *service.SubscriptionEntitlement, now time.Time) *UserEntitlement {
	if ent == nil {
		return nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	oneTimeDaily := ent.HasOneTimeDailyQuota()
	isActive := ent.IsActiveAt(now)
	dailyWindowStart := entitlementDailyQuotaWindowStart(ent.DailyWindowStart, ent.StartsAt, ent.DailyLimitUSD, now, oneTimeDaily, isActive)
	dailyUsageUSD := ent.DailyUsageUSD
	if isActive && ent.NeedsDailyResetAt(now) {
		dailyUsageUSD = 0
	}
	weeklyWindowStart := entitlementQuotaWindowStart(ent.WeeklyWindowStart, ent.StartsAt, ent.WeeklyLimitUSD)
	monthlyWindowStart := entitlementQuotaWindowStart(ent.MonthlyWindowStart, ent.StartsAt, ent.MonthlyLimitUSD)
	dailyResetsAt, dailyResetsInSeconds := entitlementDailyWindowReset(dailyWindowStart, ent.ExpiresAt, now, oneTimeDaily)
	weeklyResetsAt, weeklyResetsInSeconds := entitlementWindowReset(weeklyWindowStart, ent.ExpiresAt, 7*24*time.Hour, now, false)
	monthlyResetsAt, monthlyResetsInSeconds := entitlementWindowReset(monthlyWindowStart, ent.ExpiresAt, 30*24*time.Hour, now, false)
	return &UserEntitlement{
		ID:                     ent.ID,
		PlanID:                 cloneInt64(ent.PlanID),
		PlanName:               ent.Name,
		Name:                   ent.Name,
		Status:                 ent.Status,
		StartsAt:               ent.StartsAt,
		ExpiresAt:              ent.ExpiresAt,
		Groups:                 userEntitlementGroups(ent),
		DailyLimitUSD:          cloneFloat64(ent.DailyLimitUSD),
		DailyUsageUSD:          dailyUsageUSD,
		DailyWindowStart:       cloneTime(dailyWindowStart),
		DailyResetsAt:          dailyResetsAt,
		DailyResetsInSeconds:   dailyResetsInSeconds,
		WeeklyLimitUSD:         cloneFloat64(ent.WeeklyLimitUSD),
		WeeklyUsageUSD:         ent.WeeklyUsageUSD,
		WeeklyWindowStart:      cloneTime(weeklyWindowStart),
		WeeklyResetsAt:         weeklyResetsAt,
		WeeklyResetsInSeconds:  weeklyResetsInSeconds,
		MonthlyLimitUSD:        cloneFloat64(ent.MonthlyLimitUSD),
		MonthlyUsageUSD:        ent.MonthlyUsageUSD,
		MonthlyWindowStart:     cloneTime(monthlyWindowStart),
		MonthlyResetsAt:        monthlyResetsAt,
		MonthlyResetsInSeconds: monthlyResetsInSeconds,
		OveragePolicy:          ent.OveragePolicy,
		LegacySubscriptionID:   cloneInt64(ent.LegacySubscriptionID),
		PurchasePrice:          cloneFloat64(ent.PurchasePrice),
		PurchaseCurrency:       ent.PurchaseCurrency,
		QuotaUSD:               cloneFloat64(ent.QuotaUSD),
		QuotaPeriod:            ent.QuotaPeriod,
		UnitCostPerUSD:         cloneFloat64(ent.UnitCostPerUSD),
	}
}

func UserEntitlementsFromService(in []service.SubscriptionEntitlement, now time.Time) []UserEntitlement {
	out := make([]UserEntitlement, 0, len(in))
	for i := range in {
		if ent := UserEntitlementFromService(&in[i], now); ent != nil {
			out = append(out, *ent)
		}
	}
	return out
}

// UserSubscriptionFromServiceAdmin converts a service UserSubscription to DTO for admin users.
// It includes safe assignment metadata and entitlement linkage.
func UserSubscriptionFromServiceAdmin(sub *service.UserSubscription) *AdminUserSubscription {
	if sub == nil {
		return nil
	}
	base := userSubscriptionFromServiceBase(sub)
	if sub.EntitlementLink != nil {
		applyAdminEntitlementQuota(&base, sub.EntitlementLink)
	}
	return &AdminUserSubscription{
		UserSubscription:          base,
		AssignedBy:                sub.AssignedBy,
		AssignedAt:                sub.AssignedAt,
		AssignedByUser:            UserFromServiceShallow(sub.AssignedByUser),
		DailyLimitUSD:             cloneFloat64(adminSubscriptionEntitlementDailyLimitUSD(sub.EntitlementLink)),
		WeeklyLimitUSD:            cloneFloat64(adminSubscriptionEntitlementWeeklyLimitUSD(sub.EntitlementLink)),
		MonthlyLimitUSD:           cloneFloat64(adminSubscriptionEntitlementMonthlyLimitUSD(sub.EntitlementLink)),
		EntitlementOnly:           sub.EntitlementOnly,
		EntitlementID:             adminSubscriptionEntitlementID(sub.EntitlementLink),
		PlanID:                    cloneInt64(adminSubscriptionPlanID(sub.EntitlementLink)),
		PlanName:                  cloneString(adminSubscriptionPlanName(sub.EntitlementLink)),
		EntitlementStatus:         cloneString(adminSubscriptionEntitlementStatus(sub.EntitlementLink)),
		EntitlementExpiresAt:      cloneTime(adminSubscriptionEntitlementExpiresAt(sub.EntitlementLink)),
		EntitlementPrimaryGroupID: cloneInt64(adminSubscriptionEntitlementPrimaryGroupID(sub.EntitlementLink)),
		EntitlementOveragePolicy:  cloneString(adminSubscriptionEntitlementOveragePolicy(sub.EntitlementLink)),
	}
}

func applyAdminEntitlementQuota(out *UserSubscription, link *service.UserSubscriptionEntitlementLink) {
	if out == nil || link == nil {
		return
	}
	if !link.ExpiresAt.IsZero() {
		out.ExpiresAt = link.ExpiresAt
	}
	if link.Status != "" {
		out.Status = link.Status
	}
	out.DailyWindowStart = cloneTime(link.DailyWindowStart)
	out.WeeklyWindowStart = cloneTime(link.WeeklyWindowStart)
	out.MonthlyWindowStart = cloneTime(link.MonthlyWindowStart)
	out.DailyUsageUSD = link.DailyUsageUSD
	out.WeeklyUsageUSD = link.WeeklyUsageUSD
	out.MonthlyUsageUSD = link.MonthlyUsageUSD
}

func adminSubscriptionEntitlementID(link *service.UserSubscriptionEntitlementLink) *int64 {
	if link == nil || link.EntitlementID <= 0 {
		return nil
	}
	id := link.EntitlementID
	return &id
}

func adminSubscriptionPlanID(link *service.UserSubscriptionEntitlementLink) *int64 {
	if link == nil {
		return nil
	}
	return link.PlanID
}

func adminSubscriptionPlanName(link *service.UserSubscriptionEntitlementLink) *string {
	if link == nil {
		return nil
	}
	return link.PlanName
}

func adminSubscriptionEntitlementStatus(link *service.UserSubscriptionEntitlementLink) *string {
	if link == nil || link.Status == "" {
		return nil
	}
	return &link.Status
}

func adminSubscriptionEntitlementExpiresAt(link *service.UserSubscriptionEntitlementLink) *time.Time {
	if link == nil || link.ExpiresAt.IsZero() {
		return nil
	}
	return &link.ExpiresAt
}

func adminSubscriptionEntitlementPrimaryGroupID(link *service.UserSubscriptionEntitlementLink) *int64 {
	if link == nil {
		return nil
	}
	return link.PrimaryGroupID
}

func adminSubscriptionEntitlementDailyLimitUSD(link *service.UserSubscriptionEntitlementLink) *float64 {
	if link == nil {
		return nil
	}
	return link.DailyLimitUSD
}

func adminSubscriptionEntitlementWeeklyLimitUSD(link *service.UserSubscriptionEntitlementLink) *float64 {
	if link == nil {
		return nil
	}
	return link.WeeklyLimitUSD
}

func adminSubscriptionEntitlementMonthlyLimitUSD(link *service.UserSubscriptionEntitlementLink) *float64 {
	if link == nil {
		return nil
	}
	return link.MonthlyLimitUSD
}

func adminSubscriptionEntitlementOveragePolicy(link *service.UserSubscriptionEntitlementLink) *string {
	if link == nil || link.OveragePolicy == "" {
		return nil
	}
	return &link.OveragePolicy
}

func userEntitlementGroups(ent *service.SubscriptionEntitlement) []UserEntitlementGroup {
	if ent == nil {
		return []UserEntitlementGroup{}
	}
	if len(ent.GroupGrants) > 0 {
		grants := append([]service.SubscriptionEntitlementGroupGrant(nil), ent.GroupGrants...)
		sort.SliceStable(grants, func(i, j int) bool {
			if grants[i].SortOrder != grants[j].SortOrder {
				return grants[i].SortOrder < grants[j].SortOrder
			}
			return grants[i].GroupID < grants[j].GroupID
		})
		out := make([]UserEntitlementGroup, 0, len(grants))
		for _, grant := range grants {
			if !grant.Enabled {
				continue
			}
			item := UserEntitlementGroup{ID: grant.GroupID, SortOrder: grant.SortOrder}
			if grant.Group != nil {
				item.Name = grant.Group.Name
				item.Platform = grant.Group.Platform
				item.RateMultiplier = grant.Group.RateMultiplier
			}
			if item.RateMultiplier <= 0 {
				item.RateMultiplier = 1
			}
			out = append(out, item)
		}
		return out
	}
	groups := append([]service.Group(nil), ent.Groups...)
	sort.SliceStable(groups, func(i, j int) bool {
		return groups[i].ID < groups[j].ID
	})
	out := make([]UserEntitlementGroup, 0, len(groups))
	for _, group := range groups {
		out = append(out, UserEntitlementGroup{
			ID:             group.ID,
			Name:           group.Name,
			Platform:       group.Platform,
			RateMultiplier: normalizedGroupRate(group.RateMultiplier),
			SortOrder:      group.SortOrder,
		})
	}
	return out
}

func normalizedGroupRate(rate float64) float64 {
	if rate <= 0 {
		return 1
	}
	return rate
}

func entitlementAliasPrimaryGroup(ent *service.SubscriptionEntitlement) (int64, *service.Group) {
	if ent == nil {
		return 0, nil
	}
	candidates := entitlementAliasGroupCandidates(ent)
	if ent.PrimaryGroupID != nil {
		for i := range candidates {
			if candidates[i].id == *ent.PrimaryGroupID {
				return candidates[i].id, candidates[i].group
			}
		}
	}
	if len(candidates) == 0 {
		return 0, nil
	}
	return candidates[0].id, candidates[0].group
}

type entitlementAliasGroupCandidate struct {
	id    int64
	group *service.Group
}

func entitlementAliasGroupCandidates(ent *service.SubscriptionEntitlement) []entitlementAliasGroupCandidate {
	if ent == nil {
		return nil
	}
	if len(ent.GroupGrants) > 0 {
		grants := append([]service.SubscriptionEntitlementGroupGrant(nil), ent.GroupGrants...)
		sort.SliceStable(grants, func(i, j int) bool {
			if grants[i].SortOrder != grants[j].SortOrder {
				return grants[i].SortOrder < grants[j].SortOrder
			}
			return grants[i].GroupID < grants[j].GroupID
		})
		out := make([]entitlementAliasGroupCandidate, 0, len(grants))
		for i := range grants {
			if !grants[i].Enabled {
				continue
			}
			out = append(out, entitlementAliasGroupCandidate{
				id:    grants[i].GroupID,
				group: grants[i].Group,
			})
		}
		return out
	}
	groups := append([]service.Group(nil), ent.Groups...)
	sort.SliceStable(groups, func(i, j int) bool {
		return groups[i].ID < groups[j].ID
	})
	out := make([]entitlementAliasGroupCandidate, 0, len(groups))
	for i := range groups {
		out = append(out, entitlementAliasGroupCandidate{
			id:    groups[i].ID,
			group: &groups[i],
		})
	}
	return out
}

func entitlementWindowReset(windowStart *time.Time, expiresAt time.Time, cycle time.Duration, now time.Time, oneTimeDaily bool) (*time.Time, *int64) {
	if windowStart == nil || cycle <= 0 {
		return nil, nil
	}
	resetsAt := windowStart.Add(cycle)
	if oneTimeDaily && !expiresAt.IsZero() && expiresAt.Before(resetsAt) {
		resetsAt = expiresAt
	}
	resetsInSeconds := int64(resetsAt.Sub(now).Seconds())
	if resetsInSeconds < 0 {
		resetsInSeconds = 0
	}
	return cloneTime(&resetsAt), &resetsInSeconds
}

func entitlementDailyWindowReset(windowStart *time.Time, expiresAt, now time.Time, oneTimeDaily bool) (*time.Time, *int64) {
	if windowStart == nil {
		return nil, nil
	}
	resetsAt := timezone.StartOfDay(*windowStart).AddDate(0, 0, 1)
	if oneTimeDaily && !expiresAt.IsZero() {
		resetsAt = expiresAt
	} else if !expiresAt.IsZero() && expiresAt.Before(resetsAt) {
		resetsAt = expiresAt
	}
	resetsInSeconds := int64(resetsAt.Sub(now).Seconds())
	if resetsInSeconds < 0 {
		resetsInSeconds = 0
	}
	return cloneTime(&resetsAt), &resetsInSeconds
}

func entitlementDailyQuotaWindowStart(windowStart *time.Time, startsAt time.Time, limit *float64, now time.Time, oneTimeDaily, projectCurrent bool) *time.Time {
	if limit == nil || *limit <= 0 {
		return nil
	}
	if windowStart != nil {
		start := timezone.StartOfDay(*windowStart)
		if projectCurrent && !oneTimeDaily {
			today := timezone.StartOfDay(now)
			if today.After(start) {
				start = today
			}
		}
		return &start
	}
	if startsAt.IsZero() {
		return nil
	}
	start := timezone.StartOfDay(startsAt)
	if projectCurrent && !oneTimeDaily {
		today := timezone.StartOfDay(now)
		if today.After(start) {
			start = today
		}
	}
	return &start
}

func entitlementQuotaWindowStart(windowStart *time.Time, startsAt time.Time, limit *float64) *time.Time {
	if limit == nil || *limit <= 0 {
		return nil
	}
	if windowStart != nil {
		return windowStart
	}
	if startsAt.IsZero() {
		return nil
	}
	return cloneTime(&startsAt)
}

func cloneInt64(v *int64) *int64 {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

func cloneFloat64(v *float64) *float64 {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

func cloneString(v *string) *string {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

func cloneTime(v *time.Time) *time.Time {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

func userSubscriptionFromServiceBase(sub *service.UserSubscription) UserSubscription {
	return UserSubscription{
		ID:                 sub.ID,
		UserID:             sub.UserID,
		GroupID:            sub.GroupID,
		StartsAt:           sub.StartsAt,
		ExpiresAt:          sub.ExpiresAt,
		Status:             sub.Status,
		DailyWindowStart:   sub.DailyWindowStart,
		WeeklyWindowStart:  sub.WeeklyWindowStart,
		MonthlyWindowStart: sub.MonthlyWindowStart,
		DailyUsageUSD:      sub.DailyUsageUSD,
		WeeklyUsageUSD:     sub.WeeklyUsageUSD,
		MonthlyUsageUSD:    sub.MonthlyUsageUSD,
		CreatedAt:          sub.CreatedAt,
		UpdatedAt:          sub.UpdatedAt,
		RevokedAt:          sub.DeletedAt,
		User:               UserFromServiceShallow(sub.User),
		Group:              GroupFromServiceShallow(sub.Group),
	}
}

func BulkAssignResultFromService(r *service.BulkAssignResult) *BulkAssignResult {
	if r == nil {
		return nil
	}
	subs := make([]AdminUserSubscription, 0, len(r.Subscriptions))
	for i := range r.Subscriptions {
		subs = append(subs, *UserSubscriptionFromServiceAdmin(&r.Subscriptions[i]))
	}
	statuses := make(map[string]string, len(r.Statuses))
	for userID, status := range r.Statuses {
		statuses[strconv.FormatInt(userID, 10)] = status
	}
	return &BulkAssignResult{
		SuccessCount:  r.SuccessCount,
		CreatedCount:  r.CreatedCount,
		ReusedCount:   r.ReusedCount,
		FailedCount:   r.FailedCount,
		Subscriptions: subs,
		Errors:        r.Errors,
		Statuses:      statuses,
	}
}

func PromoCodeFromService(pc *service.PromoCode) *PromoCode {
	if pc == nil {
		return nil
	}
	return &PromoCode{
		ID:          pc.ID,
		Code:        pc.Code,
		BonusAmount: pc.BonusAmount,
		MaxUses:     pc.MaxUses,
		UsedCount:   pc.UsedCount,
		Status:      pc.Status,
		ExpiresAt:   pc.ExpiresAt,
		Notes:       pc.Notes,
		CreatedAt:   pc.CreatedAt,
		UpdatedAt:   pc.UpdatedAt,
	}
}

func PromoCodeUsageFromService(u *service.PromoCodeUsage) *PromoCodeUsage {
	if u == nil {
		return nil
	}
	return &PromoCodeUsage{
		ID:          u.ID,
		PromoCodeID: u.PromoCodeID,
		UserID:      u.UserID,
		BonusAmount: u.BonusAmount,
		UsedAt:      u.UsedAt,
		User:        UserFromServiceShallow(u.User),
	}
}
