package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	opsAccountHealthDefaultRecentLimit = 60
	opsAccountHealthMaxRecentLimit     = 120
	accountHealthProbePersistTimeout   = 5 * time.Second
	opsAccountHealthDefaultProbeModel  = "gpt-5.4-mini"
	opsEnterpriseWeChatWebhookMask     = "__configured__"
	opsEnterpriseWeChatHost            = "qyapi.weixin.qq.com"
	opsEnterpriseWeChatWebhookPath     = "/cgi-bin/webhook/send"

	accountHealthProbeStatusExtraKey    = "ops_health_probe_status"
	accountHealthProbeCheckedAtExtraKey = "ops_health_probe_checked_at"
	accountHealthProbeLatencyMsExtraKey = "ops_health_probe_latency_ms"
	accountHealthProbeModelIDExtraKey   = "ops_health_probe_model_id"
	accountHealthProbeConfigModelIDKey  = "ops_health_probe_config_model_id"
	accountHealthProbeErrorExtraKey     = "ops_health_probe_error"
	accountHealthProbeHistoryExtraKey   = "ops_health_probe_history"
	accountHealthProbeAutoDisabledKey   = "ops_health_probe_auto_disabled"
)

func defaultOpsAccountHealthSettings() OpsAccountHealthSettings {
	return OpsAccountHealthSettings{
		Enabled: true,
		Mode:    "smart",
		Burst: OpsAccountHealthBurstSettings{
			Enabled:                  true,
			WindowMinutes:            1,
			MinRequests:              10,
			ErrorRatePercent:         50,
			UpstreamErrorRatePercent: 25,
			CooldownMinutes:          1,
			BypassDigest:             true,
		},
		Degrade: OpsAccountHealthDegradeSettings{
			Enabled:                  true,
			WindowMinutes:            10,
			MinRequests:              20,
			SuccessRateMinPercent:    90,
			ErrorRatePercent:         20,
			UpstreamErrorRatePercent: 10,
			CooldownMinutes:          10,
		},
		Recovery: OpsAccountHealthRecoverySettings{
			Enabled:               true,
			WindowMinutes:         30,
			MinRequests:           10,
			SuccessRateMinPercent: 98,
			NotifyOpenedAccounts:  true,
			NotifyClosedAccounts:  true,
			CooldownMinutes:       30,
		},
		Probe: OpsAccountHealthProbeSettings{
			Enabled:         true,
			IntervalMinutes: 30,
			MaxPerRun:       2,
			TimeoutSeconds:  20,
			ModelID:         opsAccountHealthDefaultProbeModel,
			Mode:            "default",
			Prompt:          "",
		},
		Notification: OpsAccountHealthNotificationSettings{
			EnterpriseWeChatEnabled:    false,
			EnterpriseWeChatWebhookURL: "",
			MentionAllOnImmediate:      false,
		},
		RateLimitPerHour: 12,
	}
}

func normalizeOpsAccountHealthSettings(s *OpsAccountHealthSettings) {
	if s == nil {
		return
	}
	defaults := defaultOpsAccountHealthSettings()
	if isZeroOpsAccountHealthSettings(*s) {
		*s = defaults
		return
	}
	s.Mode = strings.ToLower(strings.TrimSpace(s.Mode))
	switch s.Mode {
	case "smart", "opened_only", "all":
	default:
		s.Mode = defaults.Mode
	}
	if s.Burst.WindowMinutes <= 0 {
		s.Burst.WindowMinutes = defaults.Burst.WindowMinutes
	}
	if s.Burst.MinRequests <= 0 {
		s.Burst.MinRequests = defaults.Burst.MinRequests
	}
	if s.Burst.ErrorRatePercent <= 0 {
		s.Burst.ErrorRatePercent = defaults.Burst.ErrorRatePercent
	}
	if s.Burst.UpstreamErrorRatePercent <= 0 {
		s.Burst.UpstreamErrorRatePercent = defaults.Burst.UpstreamErrorRatePercent
	}
	if s.Burst.CooldownMinutes < 0 {
		s.Burst.CooldownMinutes = defaults.Burst.CooldownMinutes
	}
	if s.Degrade.WindowMinutes <= 0 {
		s.Degrade.WindowMinutes = defaults.Degrade.WindowMinutes
	}
	if s.Degrade.MinRequests <= 0 {
		s.Degrade.MinRequests = defaults.Degrade.MinRequests
	}
	if s.Degrade.SuccessRateMinPercent <= 0 {
		s.Degrade.SuccessRateMinPercent = defaults.Degrade.SuccessRateMinPercent
	}
	if s.Degrade.ErrorRatePercent <= 0 {
		s.Degrade.ErrorRatePercent = defaults.Degrade.ErrorRatePercent
	}
	if s.Degrade.UpstreamErrorRatePercent <= 0 {
		s.Degrade.UpstreamErrorRatePercent = defaults.Degrade.UpstreamErrorRatePercent
	}
	if s.Degrade.CooldownMinutes < 0 {
		s.Degrade.CooldownMinutes = defaults.Degrade.CooldownMinutes
	}
	if s.Recovery.WindowMinutes <= 0 {
		s.Recovery.WindowMinutes = defaults.Recovery.WindowMinutes
	}
	if s.Recovery.MinRequests <= 0 {
		s.Recovery.MinRequests = defaults.Recovery.MinRequests
	}
	if s.Recovery.SuccessRateMinPercent <= 0 {
		s.Recovery.SuccessRateMinPercent = defaults.Recovery.SuccessRateMinPercent
	}
	if s.Recovery.CooldownMinutes < 0 {
		s.Recovery.CooldownMinutes = defaults.Recovery.CooldownMinutes
	}
	if s.Probe.IntervalMinutes <= 0 {
		s.Probe.IntervalMinutes = defaults.Probe.IntervalMinutes
	}
	if s.Probe.MaxPerRun <= 0 {
		s.Probe.MaxPerRun = defaults.Probe.MaxPerRun
	}
	if s.Probe.MaxPerRun > 20 {
		s.Probe.MaxPerRun = 20
	}
	if s.Probe.TimeoutSeconds <= 0 {
		s.Probe.TimeoutSeconds = defaults.Probe.TimeoutSeconds
	}
	if s.Probe.TimeoutSeconds > 120 {
		s.Probe.TimeoutSeconds = 120
	}
	s.Probe.ModelID = strings.TrimSpace(s.Probe.ModelID)
	if s.Probe.ModelID == "" {
		s.Probe.ModelID = defaults.Probe.ModelID
	}
	s.Probe.Mode = strings.ToLower(strings.TrimSpace(s.Probe.Mode))
	switch s.Probe.Mode {
	case "", "default":
		s.Probe.Mode = "default"
	case "compact":
		s.Probe.Mode = "compact"
	default:
		s.Probe.Mode = defaults.Probe.Mode
	}
	s.Probe.Prompt = strings.TrimSpace(s.Probe.Prompt)
	s.Notification.EnterpriseWeChatWebhookURL = strings.TrimSpace(s.Notification.EnterpriseWeChatWebhookURL)
	if s.RateLimitPerHour < 0 {
		s.RateLimitPerHour = defaults.RateLimitPerHour
	}
}

func isZeroOpsAccountHealthSettings(s OpsAccountHealthSettings) bool {
	return !s.Enabled &&
		strings.TrimSpace(s.Mode) == "" &&
		!s.Burst.Enabled &&
		s.Burst.WindowMinutes == 0 &&
		s.Burst.MinRequests == 0 &&
		s.Burst.ErrorRatePercent == 0 &&
		s.Burst.UpstreamErrorRatePercent == 0 &&
		s.Burst.CooldownMinutes == 0 &&
		!s.Burst.BypassDigest &&
		!s.Degrade.Enabled &&
		s.Degrade.WindowMinutes == 0 &&
		s.Degrade.MinRequests == 0 &&
		s.Degrade.SuccessRateMinPercent == 0 &&
		s.Degrade.ErrorRatePercent == 0 &&
		s.Degrade.UpstreamErrorRatePercent == 0 &&
		s.Degrade.CooldownMinutes == 0 &&
		!s.Recovery.Enabled &&
		s.Recovery.WindowMinutes == 0 &&
		s.Recovery.MinRequests == 0 &&
		s.Recovery.SuccessRateMinPercent == 0 &&
		!s.Recovery.NotifyOpenedAccounts &&
		!s.Recovery.NotifyClosedAccounts &&
		s.Recovery.CooldownMinutes == 0 &&
		!s.Probe.Enabled &&
		s.Probe.IntervalMinutes == 0 &&
		s.Probe.MaxPerRun == 0 &&
		s.Probe.TimeoutSeconds == 0 &&
		strings.TrimSpace(s.Probe.ModelID) == "" &&
		strings.TrimSpace(s.Probe.Mode) == "" &&
		strings.TrimSpace(s.Probe.Prompt) == "" &&
		!s.Notification.EnterpriseWeChatEnabled &&
		strings.TrimSpace(s.Notification.EnterpriseWeChatWebhookURL) == "" &&
		!s.Notification.MentionAllOnImmediate &&
		s.RateLimitPerHour == 0
}

func validateOpsAccountHealthSettings(s OpsAccountHealthSettings) error {
	if s.Mode != "" {
		switch strings.ToLower(strings.TrimSpace(s.Mode)) {
		case "smart", "opened_only", "all":
		default:
			return fmt.Errorf("account_health.mode must be smart, opened_only, or all")
		}
	}
	if err := validateOpsHealthWindow("account_health.burst", s.Burst.WindowMinutes, s.Burst.MinRequests); err != nil {
		return err
	}
	if err := validateOpsPercent("account_health.burst.error_rate_percent", s.Burst.ErrorRatePercent); err != nil {
		return err
	}
	if err := validateOpsPercent("account_health.burst.upstream_error_rate_percent", s.Burst.UpstreamErrorRatePercent); err != nil {
		return err
	}
	if err := validateOpsHealthWindow("account_health.degrade", s.Degrade.WindowMinutes, s.Degrade.MinRequests); err != nil {
		return err
	}
	if err := validateOpsPercent("account_health.degrade.success_rate_min_percent", s.Degrade.SuccessRateMinPercent); err != nil {
		return err
	}
	if err := validateOpsPercent("account_health.degrade.error_rate_percent", s.Degrade.ErrorRatePercent); err != nil {
		return err
	}
	if err := validateOpsPercent("account_health.degrade.upstream_error_rate_percent", s.Degrade.UpstreamErrorRatePercent); err != nil {
		return err
	}
	if err := validateOpsHealthWindow("account_health.recovery", s.Recovery.WindowMinutes, s.Recovery.MinRequests); err != nil {
		return err
	}
	if err := validateOpsPercent("account_health.recovery.success_rate_min_percent", s.Recovery.SuccessRateMinPercent); err != nil {
		return err
	}
	if s.Burst.CooldownMinutes < 0 || s.Degrade.CooldownMinutes < 0 || s.Recovery.CooldownMinutes < 0 {
		return fmt.Errorf("account_health cooldown minutes must be >= 0")
	}
	if s.Probe.IntervalMinutes <= 0 || s.Probe.IntervalMinutes > 1440 {
		return fmt.Errorf("account_health.probe.interval_minutes must be between 1 and 1440")
	}
	if s.Probe.MaxPerRun <= 0 || s.Probe.MaxPerRun > 20 {
		return fmt.Errorf("account_health.probe.max_per_run must be between 1 and 20")
	}
	if s.Probe.TimeoutSeconds <= 0 || s.Probe.TimeoutSeconds > 120 {
		return fmt.Errorf("account_health.probe.timeout_seconds must be between 1 and 120")
	}
	switch strings.ToLower(strings.TrimSpace(s.Probe.Mode)) {
	case "", "default", "compact":
	default:
		return fmt.Errorf("account_health.probe.mode must be default or compact")
	}
	if s.Notification.EnterpriseWeChatEnabled {
		webhookURL := strings.TrimSpace(s.Notification.EnterpriseWeChatWebhookURL)
		if webhookURL == "" {
			return fmt.Errorf("account_health.notification.enterprise_wechat_webhook_url is required")
		}
		if isOpsWebhookMasked(webhookURL) {
			return fmt.Errorf("account_health.notification.enterprise_wechat_webhook_url is required")
		}
		if err := validateOpsEnterpriseWeChatWebhookURL(webhookURL); err != nil {
			return err
		}
	}
	if s.RateLimitPerHour < 0 || s.RateLimitPerHour > 1000 {
		return fmt.Errorf("account_health.rate_limit_per_hour must be between 0 and 1000")
	}
	return nil
}

func validateOpsHealthWindow(name string, windowMinutes int, minRequests int) error {
	if windowMinutes <= 0 || windowMinutes > 1440 {
		return fmt.Errorf("%s.window_minutes must be between 1 and 1440", name)
	}
	if minRequests < 0 || minRequests > 100000 {
		return fmt.Errorf("%s.min_requests must be between 0 and 100000", name)
	}
	return nil
}

func validateOpsPercent(name string, value float64) error {
	if value < 0 || value > 100 {
		return fmt.Errorf("%s must be between 0 and 100", name)
	}
	return nil
}

func (s *OpsService) GetAccountHealth(ctx context.Context, filter *OpsAccountHealthFilter) (*OpsAccountHealthResponse, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	if filter == nil {
		filter = &OpsAccountHealthFilter{}
	}
	filter.Platform = strings.TrimSpace(filter.Platform)
	if filter.RecentLimit <= 0 {
		filter.RecentLimit = opsAccountHealthDefaultRecentLimit
	}
	if filter.RecentLimit > opsAccountHealthMaxRecentLimit {
		filter.RecentLimit = opsAccountHealthMaxRecentLimit
	}
	if filter.EndTime.IsZero() {
		filter.EndTime = now
	}
	if filter.StartTime.IsZero() {
		filter.StartTime = filter.EndTime.Add(-1 * time.Hour)
	}

	runtimeCfg := defaultOpsAlertRuntimeSettings()
	if loaded, err := s.GetOpsAlertRuntimeSettings(ctx); err == nil && loaded != nil {
		runtimeCfg = loaded
	}
	settings := runtimeCfg.AccountHealth
	normalizeOpsAccountHealthSettings(&settings)

	_, _, availabilityByAccount, _, err := s.GetAccountAvailabilityStats(ctx, filter.Platform, filter.GroupID)
	if err != nil {
		return nil, err
	}
	if availabilityByAccount == nil {
		availabilityByAccount = map[int64]*AccountAvailability{}
	}

	metricsByAccount := map[int64]*OpsAccountHealthMetrics{}
	if s.opsRepo != nil {
		loaded, err := s.opsRepo.GetAccountHealthMetrics(ctx, filter)
		if err != nil {
			return nil, err
		}
		if loaded != nil {
			metricsByAccount = loaded
		}
	}

	ids := make([]int64, 0, len(availabilityByAccount))
	for id := range availabilityByAccount {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		left := availabilityByAccount[ids[i]]
		right := availabilityByAccount[ids[j]]
		if left == nil || right == nil {
			return ids[i] < ids[j]
		}
		if left.Platform != right.Platform {
			return left.Platform < right.Platform
		}
		if left.GroupName != right.GroupName {
			return left.GroupName < right.GroupName
		}
		return strings.ToLower(left.AccountName) < strings.ToLower(right.AccountName)
	})

	items := make([]*OpsAccountHealthItem, 0, len(ids))
	for _, id := range ids {
		availability := availabilityByAccount[id]
		if availability == nil {
			continue
		}
		metrics := metricsByAccount[id]
		if metrics == nil {
			metrics = &OpsAccountHealthMetrics{
				AccountID: id,
				Windows:   defaultOpsAccountHealthWindows(),
				Recent:    []*OpsAccountHealthSample{},
			}
		}
		normalizeAccountHealthMetrics(metrics)

		item := &OpsAccountHealthItem{
			AccountID:              availability.AccountID,
			AccountName:            availability.AccountName,
			Platform:               availability.Platform,
			GroupID:                availability.GroupID,
			GroupName:              availability.GroupName,
			Tags:                   availability.Tags,
			Status:                 availability.Status,
			IsOpened:               availability.IsOpened,
			IsSchedulable:          availability.IsSchedulable,
			IsAvailable:            availability.IsAvailable,
			IsRateLimited:          availability.IsRateLimited,
			IsOverloaded:           availability.IsOverloaded,
			IsTempUnschedulable:    availability.IsTempUnschedulable,
			HasError:               availability.HasError,
			ProbeAutoDisabled:      availability.ProbeAutoDisabled,
			RateLimitResetAt:       availability.RateLimitResetAt,
			RateLimitRemainingSec:  availability.RateLimitRemainingSec,
			OverloadUntil:          availability.OverloadUntil,
			OverloadRemainingSec:   availability.OverloadRemainingSec,
			TempUnschedulableUntil: availability.TempUnschedulableUntil,
			ErrorMessage:           availability.ErrorMessage,
			Windows:                metrics.Windows,
			Recent:                 metrics.Recent,
			FirstToken5m:           metrics.FirstToken5m,
			FirstTokenWindows:      metrics.FirstTokenWindows,
			Probe:                  availability.HealthProbe,
		}
		item.ProbeModelID = accountHealthProbeConfiguredModelIDFromAvailability(availability)
		item.ProbeModelEffective = resolveOpsAccountHealthProbeModelID(item.ProbeModelID, settings.Probe.ModelID)
		item.Recommendation = decideOpsAccountHealth(item, settings)
		items = append(items, item)
	}

	responseSettings := maskOpsAccountHealthSettings(settings)
	return &OpsAccountHealthResponse{
		Enabled:     true,
		GeneratedAt: now,
		Items:       items,
		Settings:    responseSettings,
	}, nil
}

func (s *OpsService) UpdateAccountHealthProbeAuto(ctx context.Context, accountID int64, enabled bool) (*OpsAccountHealthProbeAutoState, error) {
	if s == nil || s.accountRepo == nil {
		return nil, fmt.Errorf("account repository is not available")
	}
	if accountID <= 0 {
		return nil, fmt.Errorf("account id must be positive")
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, ErrAccountNotFound
	}
	disabled := !enabled
	if err := s.accountRepo.UpdateExtra(ctx, accountID, map[string]any{
		accountHealthProbeAutoDisabledKey: disabled,
	}); err != nil {
		return nil, err
	}
	return &OpsAccountHealthProbeAutoState{
		AccountID:         accountID,
		ProbeAutoDisabled: disabled,
	}, nil
}

func (s *OpsService) UpdateAccountHealthProbeModel(ctx context.Context, accountID int64, modelID string) (*OpsAccountHealthProbeModelState, error) {
	if s == nil || s.accountRepo == nil {
		return nil, fmt.Errorf("account repository is not available")
	}
	if accountID <= 0 {
		return nil, fmt.Errorf("account id must be positive")
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, ErrAccountNotFound
	}

	modelID = strings.TrimSpace(modelID)
	if err := s.accountRepo.UpdateExtra(ctx, accountID, map[string]any{
		accountHealthProbeConfigModelIDKey: modelID,
	}); err != nil {
		return nil, err
	}

	globalModelID := ""
	if cfg, err := s.GetOpsAlertRuntimeSettings(ctx); err == nil && cfg != nil {
		globalModelID = cfg.AccountHealth.Probe.ModelID
	}
	return &OpsAccountHealthProbeModelState{
		AccountID:           accountID,
		ProbeModelID:        modelID,
		ProbeModelEffective: resolveOpsAccountHealthProbeModelID(modelID, globalModelID),
		GlobalProbeModelID:  resolveOpsAccountHealthProbeModelID("", globalModelID),
		DefaultProbeModelID: opsAccountHealthDefaultProbeModel,
		InheritsGlobalModel: modelID == "",
		HasAccountOverride:  modelID != "",
	}, nil
}

func (s *OpsService) ResolveAccountHealthProbeModelID(ctx context.Context, accountID int64, requestedModelID string) string {
	requestedModelID = strings.TrimSpace(requestedModelID)
	if requestedModelID != "" {
		return requestedModelID
	}

	accountModelID := ""
	if s != nil && s.accountRepo != nil && accountID > 0 {
		if account, err := s.accountRepo.GetByID(ctx, accountID); err == nil && account != nil {
			accountModelID = accountHealthProbeConfiguredModelIDFromAccount(account)
		}
	}

	globalModelID := ""
	if s != nil {
		if cfg, err := s.GetOpsAlertRuntimeSettings(ctx); err == nil && cfg != nil {
			globalModelID = cfg.AccountHealth.Probe.ModelID
		}
	}
	return resolveOpsAccountHealthProbeModelID(accountModelID, globalModelID)
}

func defaultOpsAccountHealthWindows() map[string]*OpsAccountHealthWindowStats {
	return map[string]*OpsAccountHealthWindowStats{
		OpsAccountHealthWindow1m:  {Window: OpsAccountHealthWindow1m},
		OpsAccountHealthWindow5m:  {Window: OpsAccountHealthWindow5m},
		OpsAccountHealthWindow10m: {Window: OpsAccountHealthWindow10m},
		OpsAccountHealthWindow30m: {Window: OpsAccountHealthWindow30m},
		OpsAccountHealthWindow1h:  {Window: OpsAccountHealthWindow1h},
	}
}

func normalizeAccountHealthMetrics(metrics *OpsAccountHealthMetrics) {
	if metrics.Windows == nil {
		metrics.Windows = defaultOpsAccountHealthWindows()
	}
	for key, value := range defaultOpsAccountHealthWindows() {
		if metrics.Windows[key] == nil {
			metrics.Windows[key] = value
		}
	}
	for _, stat := range metrics.Windows {
		if stat == nil {
			continue
		}
		total := stat.RequestCount
		if total <= 0 {
			stat.SuccessRatePercent = 0
			stat.ErrorRatePercent = 0
			stat.UpstreamErrorRatePercent = 0
			continue
		}
		stat.SuccessRatePercent = ratioPercent(stat.SuccessCount, total)
		stat.ErrorRatePercent = ratioPercent(stat.ErrorCount, total)
		stat.UpstreamErrorRatePercent = ratioPercent(stat.UpstreamErrorCount, total)
	}
	if metrics.Recent == nil {
		metrics.Recent = []*OpsAccountHealthSample{}
	}
	if metrics.FirstTokenWindows == nil {
		metrics.FirstTokenWindows = map[string]*OpsAccountHealthFirstTokenStats{}
	}
}

func decideOpsAccountHealth(item *OpsAccountHealthItem, settings OpsAccountHealthSettings) OpsAccountHealthRecommendation {
	if item == nil {
		return OpsAccountHealthRecommendation{
			Action:     OpsAccountHealthActionUnavailable,
			Severity:   "P3",
			Title:      "No account data",
			NotifyMode: OpsAccountHealthNotifyNone,
		}
	}

	if !item.IsOpened && settings.Recovery.Enabled && probeRecoveryReady(item.Probe, settings.Recovery.WindowMinutes) && !hasAnyRequests(item) {
		return OpsAccountHealthRecommendation{
			Action:        OpsAccountHealthActionCanOpen,
			Severity:      "P2",
			Title:         "账号探测已恢复，可尝试打开",
			Reason:        fmt.Sprintf("active probe succeeded within %dm", settings.Recovery.WindowMinutes),
			NotifyMode:    recoveryNotifyMode(item, settings),
			RecoveryReady: true,
		}
	}

	if item.HasError {
		return OpsAccountHealthRecommendation{
			Action:     pickOpenedAction(item, OpsAccountHealthActionCloseNow, OpsAccountHealthActionKeepClosed),
			Severity:   "P1",
			Title:      "账号处于错误状态",
			Reason:     strings.TrimSpace(item.ErrorMessage),
			NotifyMode: notifyModeForAccount(item, settings, true),
			Immediate:  item.IsOpened,
		}
	}

	if item.IsRateLimited {
		return OpsAccountHealthRecommendation{
			Action:     pickOpenedAction(item, OpsAccountHealthActionWatch, OpsAccountHealthActionKeepClosed),
			Severity:   "P2",
			Title:      "账号正在限流",
			Reason:     "rate limit cooldown is active",
			NotifyMode: notifyModeForAccount(item, settings, false),
		}
	}

	if item.IsOverloaded || item.IsTempUnschedulable {
		return OpsAccountHealthRecommendation{
			Action:     pickOpenedAction(item, OpsAccountHealthActionWatch, OpsAccountHealthActionKeepClosed),
			Severity:   "P2",
			Title:      "账号临时不可调度",
			Reason:     "overload or temporary unschedulable state is active",
			NotifyMode: notifyModeForAccount(item, settings, false),
		}
	}

	if settings.Burst.Enabled {
		if stat := windowStatForMinutes(item, settings.Burst.WindowMinutes); stat != nil && stat.RequestCount >= int64(settings.Burst.MinRequests) {
			if stat.ErrorRatePercent >= settings.Burst.ErrorRatePercent || stat.UpstreamErrorRatePercent >= settings.Burst.UpstreamErrorRatePercent {
				return OpsAccountHealthRecommendation{
					Action:     pickOpenedAction(item, OpsAccountHealthActionCloseNow, OpsAccountHealthActionKeepClosed),
					Severity:   "P1",
					Title:      "1 分钟内异常波动过大",
					Reason:     fmt.Sprintf("1m requests=%d error_rate=%.1f%% upstream_error_rate=%.1f%%", stat.RequestCount, stat.ErrorRatePercent, stat.UpstreamErrorRatePercent),
					NotifyMode: notifyModeForAccount(item, settings, true),
					Immediate:  settings.Burst.BypassDigest,
				}
			}
		}
	}

	if settings.Degrade.Enabled {
		if stat := windowStatForMinutes(item, settings.Degrade.WindowMinutes); stat != nil && stat.RequestCount >= int64(settings.Degrade.MinRequests) {
			if accountHealthWindowIsDegraded(stat, settings.Degrade) {
				sustainedStats := sustainedAccountHealthDegradeStats(item, settings.Degrade)
				if len(sustainedStats) >= 3 {
					return OpsAccountHealthRecommendation{
						Action:     pickOpenedAction(item, OpsAccountHealthActionCloseNow, OpsAccountHealthActionKeepClosed),
						Severity:   "P2",
						Title:      "账号持续变差，建议处理",
						Reason:     formatSustainedAccountHealthDegradeReason(sustainedStats),
						NotifyMode: notifyModeForAccount(item, settings, false),
					}
				}
				return OpsAccountHealthRecommendation{
					Action:     pickOpenedAction(item, OpsAccountHealthActionWatch, OpsAccountHealthActionKeepClosed),
					Severity:   "P2",
					Title:      "账号短期变差，继续观察",
					Reason:     fmt.Sprintf("%dm success_rate=%.1f%% error_rate=%.1f%% upstream_error_rate=%.1f%%", settings.Degrade.WindowMinutes, stat.SuccessRatePercent, stat.ErrorRatePercent, stat.UpstreamErrorRatePercent),
					NotifyMode: OpsAccountHealthNotifyNone,
				}
			}
		}
	}

	if settings.Recovery.Enabled {
		if stat := windowStatForMinutes(item, settings.Recovery.WindowMinutes); stat != nil && stat.RequestCount >= int64(settings.Recovery.MinRequests) {
			if stat.SuccessRatePercent >= settings.Recovery.SuccessRateMinPercent && stat.ErrorCount == 0 && stat.UpstreamErrorCount == 0 {
				if item.IsOpened {
					return OpsAccountHealthRecommendation{
						Action:        OpsAccountHealthActionKeepOpen,
						Severity:      "P3",
						Title:         "账号稳定运行",
						Reason:        fmt.Sprintf("%dm success_rate=%.1f%% and no errors", settings.Recovery.WindowMinutes, stat.SuccessRatePercent),
						NotifyMode:    recoveryNotifyMode(item, settings),
						RecoveryReady: true,
					}
				}
				return OpsAccountHealthRecommendation{
					Action:        OpsAccountHealthActionCanOpen,
					Severity:      "P2",
					Title:         "账号已恢复，可尝试打开",
					Reason:        fmt.Sprintf("%dm success_rate=%.1f%% and no errors", settings.Recovery.WindowMinutes, stat.SuccessRatePercent),
					NotifyMode:    recoveryNotifyMode(item, settings),
					RecoveryReady: true,
				}
			}
		}
	}

	if !item.IsOpened && settings.Recovery.Enabled {
		if probeRecentlyFailed(item.Probe, settings.Recovery.WindowMinutes) {
			return OpsAccountHealthRecommendation{
				Action:     OpsAccountHealthActionKeepClosed,
				Severity:   "P3",
				Title:      "主动探测仍失败",
				Reason:     probeFailureReason(item.Probe),
				NotifyMode: OpsAccountHealthNotifyNone,
			}
		}
	}

	if item.IsOpened {
		if !item.IsAvailable {
			return OpsAccountHealthRecommendation{
				Action:     OpsAccountHealthActionWatch,
				Severity:   "P2",
				Title:      "账号已打开但当前不可调度",
				Reason:     "status is active, but scheduler availability is false",
				NotifyMode: OpsAccountHealthNotifyDigest,
			}
		}
		return OpsAccountHealthRecommendation{
			Action:     OpsAccountHealthActionKeepOpen,
			Severity:   "P3",
			Title:      "保持开启",
			Reason:     "no anomaly above configured thresholds",
			NotifyMode: OpsAccountHealthNotifyNone,
		}
	}

	if hasAnyRequests(item) {
		return OpsAccountHealthRecommendation{
			Action:     OpsAccountHealthActionKeepClosed,
			Severity:   "P3",
			Title:      "继续观察",
			Reason:     "recent data does not meet recovery threshold",
			NotifyMode: OpsAccountHealthNotifyNone,
		}
	}

	return OpsAccountHealthRecommendation{
		Action:     OpsAccountHealthActionNeedsProbe,
		Severity:   "P3",
		Title:      "缺少恢复判断数据",
		Reason:     "closed accounts need traffic or active probes before a reopen recommendation",
		NotifyMode: OpsAccountHealthNotifyNone,
	}
}

func pickOpenedAction(item *OpsAccountHealthItem, openedAction string, closedAction string) string {
	if item != nil && item.IsOpened {
		return openedAction
	}
	return closedAction
}

func notifyModeForAccount(item *OpsAccountHealthItem, settings OpsAccountHealthSettings, immediate bool) string {
	if item == nil || !settings.Enabled {
		return OpsAccountHealthNotifyNone
	}
	mode := strings.ToLower(strings.TrimSpace(settings.Mode))
	if mode == "" {
		mode = "smart"
	}
	switch mode {
	case "all":
		if immediate {
			return OpsAccountHealthNotifyImmediate
		}
		return OpsAccountHealthNotifyDigest
	case "opened_only":
		if !item.IsOpened {
			return OpsAccountHealthNotifyNone
		}
	case "smart":
		if !item.IsOpened {
			return OpsAccountHealthNotifyNone
		}
	}
	if immediate {
		return OpsAccountHealthNotifyImmediate
	}
	return OpsAccountHealthNotifyDigest
}

func recoveryNotifyMode(item *OpsAccountHealthItem, settings OpsAccountHealthSettings) string {
	if item == nil || !settings.Enabled || !settings.Recovery.Enabled {
		return OpsAccountHealthNotifyNone
	}
	mode := strings.ToLower(strings.TrimSpace(settings.Mode))
	if mode == "" {
		mode = "smart"
	}
	if mode == "opened_only" && !item.IsOpened {
		return OpsAccountHealthNotifyNone
	}
	if item.IsOpened && settings.Recovery.NotifyOpenedAccounts {
		return OpsAccountHealthNotifyDigest
	}
	if !item.IsOpened && settings.Recovery.NotifyClosedAccounts {
		return OpsAccountHealthNotifyDigest
	}
	return OpsAccountHealthNotifyNone
}

func windowStatForMinutes(item *OpsAccountHealthItem, minutes int) *OpsAccountHealthWindowStats {
	if item == nil || item.Windows == nil {
		return nil
	}
	switch {
	case minutes <= 1:
		return item.Windows[OpsAccountHealthWindow1m]
	case minutes <= 5:
		return item.Windows[OpsAccountHealthWindow5m]
	case minutes <= 10:
		return item.Windows[OpsAccountHealthWindow10m]
	case minutes <= 30:
		return item.Windows[OpsAccountHealthWindow30m]
	default:
		return item.Windows[OpsAccountHealthWindow1h]
	}
}

func accountHealthWindowIsDegraded(stat *OpsAccountHealthWindowStats, settings OpsAccountHealthDegradeSettings) bool {
	if stat == nil || stat.RequestCount < int64(settings.MinRequests) {
		return false
	}
	return stat.SuccessRatePercent < settings.SuccessRateMinPercent ||
		stat.ErrorRatePercent >= settings.ErrorRatePercent ||
		stat.UpstreamErrorRatePercent >= settings.UpstreamErrorRatePercent
}

func sustainedAccountHealthDegradeStats(item *OpsAccountHealthItem, settings OpsAccountHealthDegradeSettings) []*OpsAccountHealthWindowStats {
	if item == nil || item.Windows == nil {
		return nil
	}
	out := make([]*OpsAccountHealthWindowStats, 0, 3)
	for _, window := range []string{OpsAccountHealthWindow5m, OpsAccountHealthWindow10m, OpsAccountHealthWindow30m} {
		stat := item.Windows[window]
		if !accountHealthWindowIsDegraded(stat, settings) {
			return nil
		}
		out = append(out, stat)
	}
	return out
}

func formatSustainedAccountHealthDegradeReason(stats []*OpsAccountHealthWindowStats) string {
	parts := make([]string, 0, len(stats))
	for _, stat := range stats {
		if stat == nil {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s req=%d success=%.1f%% err=%.1f%% upstream=%.1f%%",
			stat.Window,
			stat.RequestCount,
			stat.SuccessRatePercent,
			stat.ErrorRatePercent,
			stat.UpstreamErrorRatePercent,
		))
	}
	return strings.Join(parts, "; ")
}

func hasAnyRequests(item *OpsAccountHealthItem) bool {
	if item == nil || item.Windows == nil {
		return false
	}
	for _, stat := range item.Windows {
		if stat != nil && stat.RequestCount > 0 {
			return true
		}
	}
	return false
}

func ratioPercent(numerator int64, denominator int64) float64 {
	if denominator <= 0 {
		return 0
	}
	return (float64(numerator) / float64(denominator)) * 100
}

func isOpsWebhookMasked(raw string) bool {
	switch strings.TrimSpace(raw) {
	case "", opsEnterpriseWeChatWebhookMask, "********":
		return strings.TrimSpace(raw) != ""
	default:
		return false
	}
}

func maskOpsAccountHealthSettings(settings OpsAccountHealthSettings) OpsAccountHealthSettings {
	if strings.TrimSpace(settings.Notification.EnterpriseWeChatWebhookURL) != "" {
		settings.Notification.EnterpriseWeChatWebhookURL = opsEnterpriseWeChatWebhookMask
	}
	return settings
}

func validateOpsEnterpriseWeChatWebhookURL(raw string) error {
	webhookURL := strings.TrimSpace(raw)
	if len(webhookURL) > 2048 {
		return fmt.Errorf("account_health.notification.enterprise_wechat_webhook_url is too long")
	}
	parsed, err := url.Parse(webhookURL)
	if err != nil {
		return fmt.Errorf("account_health.notification.enterprise_wechat_webhook_url is invalid")
	}
	if strings.ToLower(parsed.Scheme) != "https" {
		return fmt.Errorf("account_health.notification.enterprise_wechat_webhook_url must be https")
	}
	if strings.ToLower(parsed.Hostname()) != opsEnterpriseWeChatHost {
		return fmt.Errorf("account_health.notification.enterprise_wechat_webhook_url must use %s", opsEnterpriseWeChatHost)
	}
	if parsed.Path != opsEnterpriseWeChatWebhookPath {
		return fmt.Errorf("account_health.notification.enterprise_wechat_webhook_url path must be %s", opsEnterpriseWeChatWebhookPath)
	}
	if strings.TrimSpace(parsed.RawQuery) == "" {
		return fmt.Errorf("account_health.notification.enterprise_wechat_webhook_url missing key")
	}
	return nil
}

func probeRecoveryReady(probe *OpsAccountHealthProbe, windowMinutes int) bool {
	if probe == nil || probe.CheckedAt == nil {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(probe.Status), "success") {
		return probeWithinWindow(probe.CheckedAt, windowMinutes)
	}
	return false
}

func probeRecentlyFailed(probe *OpsAccountHealthProbe, windowMinutes int) bool {
	if probe == nil || probe.CheckedAt == nil {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(probe.Status), "failed") {
		return probeWithinWindow(probe.CheckedAt, windowMinutes)
	}
	return false
}

func probeWithinWindow(checkedAt *time.Time, windowMinutes int) bool {
	if checkedAt == nil || checkedAt.IsZero() {
		return false
	}
	if windowMinutes <= 0 {
		windowMinutes = 30
	}
	return time.Since(*checkedAt) <= time.Duration(windowMinutes)*time.Minute
}

func probeFailureReason(probe *OpsAccountHealthProbe) string {
	if probe == nil {
		return "active probe has not run"
	}
	if msg := strings.TrimSpace(probe.ErrorMessage); msg != "" {
		return msg
	}
	return "active probe failed"
}

func accountHealthProbeAutoDisabledFromAccount(account *Account) bool {
	if account == nil {
		return false
	}
	return account.getExtraBool(accountHealthProbeAutoDisabledKey)
}

func accountHealthProbeConfiguredModelIDFromAvailability(availability *AccountAvailability) string {
	if availability == nil {
		return ""
	}
	return strings.TrimSpace(availability.ProbeModelID)
}

func accountHealthProbeConfiguredModelIDFromAccount(account *Account) string {
	if account == nil {
		return ""
	}
	return strings.TrimSpace(account.getExtraString(accountHealthProbeConfigModelIDKey))
}

func resolveOpsAccountHealthProbeModelID(accountModelID string, globalModelID string) string {
	if modelID := strings.TrimSpace(accountModelID); modelID != "" {
		return modelID
	}
	if modelID := strings.TrimSpace(globalModelID); modelID != "" {
		return modelID
	}
	return opsAccountHealthDefaultProbeModel
}

func accountHealthProbeFromAccount(account *Account) *OpsAccountHealthProbe {
	if account == nil {
		return nil
	}
	status := strings.TrimSpace(account.getExtraString(accountHealthProbeStatusExtraKey))
	checkedAt := account.getExtraTime(accountHealthProbeCheckedAtExtraKey)
	history := accountHealthProbeHistoryFromAccount(account)
	if status == "" && checkedAt.IsZero() && len(history) == 0 {
		return nil
	}
	probe := &OpsAccountHealthProbe{
		Status:       status,
		ModelID:      strings.TrimSpace(account.getExtraString(accountHealthProbeModelIDExtraKey)),
		ErrorMessage: strings.TrimSpace(account.getExtraString(accountHealthProbeErrorExtraKey)),
	}
	if !checkedAt.IsZero() {
		probe.CheckedAt = &checkedAt
	}
	if latency := account.getExtraInt(accountHealthProbeLatencyMsExtraKey); latency > 0 {
		v := int64(latency)
		probe.LatencyMs = &v
	}
	if probe.Status == "" && len(history) > 0 {
		last := history[len(history)-1]
		probe.Status = "failed"
		if strings.EqualFold(last.Kind, "success") {
			probe.Status = "success"
		}
		if !last.CreatedAt.IsZero() {
			t := last.CreatedAt
			probe.CheckedAt = &t
		}
		probe.ModelID = strings.TrimSpace(last.Model)
		probe.ErrorMessage = strings.TrimSpace(last.Message)
		if last.DurationMs != nil {
			v := int64(*last.DurationMs)
			probe.LatencyMs = &v
		}
	}
	if len(history) == 0 && probe.CheckedAt != nil {
		history = []*OpsAccountHealthSample{accountHealthProbeSampleFromProbe(probe)}
	}
	applyAccountHealthProbeHistoryStats(probe, history)
	return probe
}

func accountHealthProbePersistContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(context.WithoutCancel(ctx), accountHealthProbePersistTimeout)
}

func accountHealthProbeHistoryFromAccount(account *Account) []*OpsAccountHealthSample {
	if account == nil || account.Extra == nil {
		return []*OpsAccountHealthSample{}
	}
	raw, ok := account.Extra[accountHealthProbeHistoryExtraKey]
	if !ok || raw == nil {
		return []*OpsAccountHealthSample{}
	}
	if text, ok := raw.(string); ok {
		text = strings.TrimSpace(text)
		if text == "" {
			return []*OpsAccountHealthSample{}
		}
		var samples []*OpsAccountHealthSample
		if err := json.Unmarshal([]byte(text), &samples); err == nil {
			return normalizeAccountHealthProbeHistory(samples)
		}
		return []*OpsAccountHealthSample{}
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return []*OpsAccountHealthSample{}
	}
	var samples []*OpsAccountHealthSample
	if err := json.Unmarshal(payload, &samples); err != nil {
		return []*OpsAccountHealthSample{}
	}
	return normalizeAccountHealthProbeHistory(samples)
}

func appendAccountHealthProbeHistory(history []*OpsAccountHealthSample, sample *OpsAccountHealthSample) []*OpsAccountHealthSample {
	history = normalizeAccountHealthProbeHistory(history)
	if sample != nil {
		history = append(history, sample)
	}
	return normalizeAccountHealthProbeHistory(history)
}

func normalizeAccountHealthProbeHistory(samples []*OpsAccountHealthSample) []*OpsAccountHealthSample {
	if len(samples) == 0 {
		return []*OpsAccountHealthSample{}
	}
	normalized := make([]*OpsAccountHealthSample, 0, len(samples))
	for _, sample := range samples {
		if sample == nil || sample.CreatedAt.IsZero() {
			continue
		}
		kind := strings.ToLower(strings.TrimSpace(sample.Kind))
		switch kind {
		case "success":
		default:
			kind = "error"
		}
		normalized = append(normalized, &OpsAccountHealthSample{
			Kind:       kind,
			CreatedAt:  sample.CreatedAt.UTC(),
			RequestID:  strings.TrimSpace(sample.RequestID),
			Model:      strings.TrimSpace(sample.Model),
			DurationMs: sample.DurationMs,
			StatusCode: sample.StatusCode,
			Message:    strings.TrimSpace(sample.Message),
		})
	}
	if len(normalized) > opsAccountHealthDefaultRecentLimit {
		normalized = normalized[len(normalized)-opsAccountHealthDefaultRecentLimit:]
	}
	return normalized
}

func accountHealthProbeSampleFromProbe(probe *OpsAccountHealthProbe) *OpsAccountHealthSample {
	if probe == nil || probe.CheckedAt == nil || probe.CheckedAt.IsZero() {
		return nil
	}
	kind := "error"
	if strings.EqualFold(strings.TrimSpace(probe.Status), "success") {
		kind = "success"
	}
	var durationMs *int
	if probe.LatencyMs != nil {
		v := int(*probe.LatencyMs)
		durationMs = &v
	}
	message := strings.TrimSpace(probe.ErrorMessage)
	if message == "" {
		message = "主动探测"
	}
	return &OpsAccountHealthSample{
		Kind:       kind,
		CreatedAt:  probe.CheckedAt.UTC(),
		Model:      strings.TrimSpace(probe.ModelID),
		DurationMs: durationMs,
		Message:    message,
	}
}

func applyAccountHealthProbeHistoryStats(probe *OpsAccountHealthProbe, history []*OpsAccountHealthSample) {
	if probe == nil {
		return
	}
	history = normalizeAccountHealthProbeHistory(history)
	probe.Recent = history
	probe.RequestCount = int64(len(history))
	probe.SuccessCount = 0
	probe.ErrorCount = 0
	probe.SuccessRatePercent = 0
	probe.ErrorRatePercent = 0
	probe.AvgLatencyMs = nil
	if probe.RequestCount == 0 {
		return
	}
	var latencySum int64
	var latencyCount int64
	for _, sample := range history {
		if strings.EqualFold(sample.Kind, "success") {
			probe.SuccessCount++
		} else {
			probe.ErrorCount++
		}
		if sample.DurationMs != nil {
			latencySum += int64(*sample.DurationMs)
			latencyCount++
		}
	}
	probe.SuccessRatePercent = ratioPercent(probe.SuccessCount, probe.RequestCount)
	probe.ErrorRatePercent = ratioPercent(probe.ErrorCount, probe.RequestCount)
	if latencyCount > 0 {
		avg := float64(latencySum) / float64(latencyCount)
		probe.AvgLatencyMs = &avg
	}
}
