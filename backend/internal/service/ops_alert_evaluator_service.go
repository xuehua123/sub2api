package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	opsAlertEvaluatorJobName = "ops_alert_evaluator"

	opsAlertEvaluatorTimeout         = 45 * time.Second
	opsAlertEvaluatorLeaderLockKey   = "ops:alert:evaluator:leader"
	opsAlertEvaluatorLeaderLockTTL   = 90 * time.Second
	opsAlertEvaluatorSkipLogInterval = 1 * time.Minute
	opsEnterpriseWeChatSendTimeout   = 5 * time.Second
)

var opsAlertEvaluatorReleaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`)

var opsNotifyTimeLocation = loadOpsNotifyTimeLocation()

func loadOpsNotifyTimeLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("Asia/Shanghai", 8*60*60)
	}
	return loc
}

type OpsAlertEvaluatorService struct {
	opsService         *OpsService
	opsRepo            OpsRepository
	emailService       *EmailService
	accountTestService *AccountTestService
	proxyRepo          ProxyRepository

	redisClient *redis.Client
	cfg         *config.Config
	instanceID  string

	stopCh    chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
	wg        sync.WaitGroup

	mu         sync.Mutex
	ruleStates map[int64]*opsAlertRuleState

	emailLimiter          *slidingWindowLimiter
	accountHealthLimiter  *slidingWindowLimiter
	accountHealthNotifyAt map[string]time.Time
	accountHealthProbeAt  map[int64]time.Time

	skipLogMu sync.Mutex
	skipLogAt time.Time

	warnNoRedisOnce sync.Once
}

type opsAlertRuleState struct {
	LastEvaluatedAt     time.Time
	ConsecutiveBreaches int
}

func NewOpsAlertEvaluatorService(
	opsService *OpsService,
	opsRepo OpsRepository,
	emailService *EmailService,
	accountTestService *AccountTestService,
	redisClient *redis.Client,
	cfg *config.Config,
	proxyRepo ProxyRepository,
) *OpsAlertEvaluatorService {
	return &OpsAlertEvaluatorService{
		opsService:            opsService,
		opsRepo:               opsRepo,
		emailService:          emailService,
		accountTestService:    accountTestService,
		proxyRepo:             proxyRepo,
		redisClient:           redisClient,
		cfg:                   cfg,
		instanceID:            uuid.NewString(),
		ruleStates:            map[int64]*opsAlertRuleState{},
		emailLimiter:          newSlidingWindowLimiter(0, time.Hour),
		accountHealthLimiter:  newSlidingWindowLimiter(0, time.Hour),
		accountHealthNotifyAt: map[string]time.Time{},
		accountHealthProbeAt:  map[int64]time.Time{},
	}
}

func (s *OpsAlertEvaluatorService) Start() {
	if s == nil {
		return
	}
	s.startOnce.Do(func() {
		if s.stopCh == nil {
			s.stopCh = make(chan struct{})
		}
		s.wg.Add(1)
		go s.run()
	})
}

func (s *OpsAlertEvaluatorService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		if s.stopCh != nil {
			close(s.stopCh)
		}
	})
	s.wg.Wait()
}

func (s *OpsAlertEvaluatorService) run() {
	defer s.wg.Done()

	// Start immediately to produce early feedback in ops dashboard.
	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			interval := s.getInterval()
			s.evaluateOnce(interval)
			timer.Reset(interval)
		case <-s.stopCh:
			return
		}
	}
}

func (s *OpsAlertEvaluatorService) getInterval() time.Duration {
	// Default.
	interval := 60 * time.Second

	if s == nil || s.opsService == nil {
		return interval
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cfg, err := s.opsService.GetOpsAlertRuntimeSettings(ctx)
	if err != nil || cfg == nil {
		return interval
	}
	if cfg.EvaluationIntervalSeconds <= 0 {
		return interval
	}
	if cfg.EvaluationIntervalSeconds < 1 {
		return interval
	}
	if cfg.EvaluationIntervalSeconds > int((24 * time.Hour).Seconds()) {
		return interval
	}
	return time.Duration(cfg.EvaluationIntervalSeconds) * time.Second
}

func (s *OpsAlertEvaluatorService) evaluateOnce(interval time.Duration) {
	if s == nil || s.opsRepo == nil {
		return
	}
	if s.cfg != nil && !s.cfg.Ops.Enabled {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), opsAlertEvaluatorTimeout)
	defer cancel()

	if s.opsService != nil && !s.opsService.IsMonitoringEnabled(ctx) {
		return
	}

	runtimeCfg := defaultOpsAlertRuntimeSettings()
	if s.opsService != nil {
		if loaded, err := s.opsService.GetOpsAlertRuntimeSettings(ctx); err == nil && loaded != nil {
			runtimeCfg = loaded
		}
	}

	release, ok := s.tryAcquireLeaderLock(ctx, runtimeCfg.DistributedLock)
	if !ok {
		return
	}
	if release != nil {
		defer release()
	}

	startedAt := time.Now().UTC()
	runAt := startedAt

	rules, err := s.opsRepo.ListAlertRules(ctx)
	if err != nil {
		s.recordHeartbeatError(runAt, time.Since(startedAt), err)
		logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] list rules failed: %v", err)
		return
	}

	rulesTotal := len(rules)
	rulesEnabled := 0
	rulesEvaluated := 0
	eventsCreated := 0
	eventsResolved := 0
	emailsSent := 0
	enterpriseWeChatSent := 0

	now := time.Now().UTC()
	safeEnd := now.Truncate(time.Minute)
	if safeEnd.IsZero() {
		safeEnd = now
	}

	systemMetrics, _ := s.opsRepo.GetLatestSystemMetrics(ctx, 1)

	// Cleanup stale state for removed rules.
	s.pruneRuleStates(rules)

	for _, rule := range rules {
		if rule == nil || !rule.Enabled || rule.ID <= 0 {
			continue
		}
		rulesEnabled++

		scopePlatform, scopeGroupID, scopeRegion := parseOpsAlertRuleScope(rule.Filters)

		windowMinutes := rule.WindowMinutes
		if windowMinutes <= 0 {
			windowMinutes = 1
		}
		windowStart := safeEnd.Add(-time.Duration(windowMinutes) * time.Minute)
		windowEnd := safeEnd

		metricValue, ok := s.computeRuleMetric(ctx, rule, systemMetrics, windowStart, windowEnd, scopePlatform, scopeGroupID)
		if !ok {
			s.resetRuleState(rule.ID, now)
			continue
		}
		rulesEvaluated++

		breachedNow := compareMetric(metricValue, rule.Operator, rule.Threshold)
		required := requiredSustainedBreaches(rule.SustainedMinutes, interval)
		consecutive := s.updateRuleBreaches(rule.ID, now, interval, breachedNow)

		activeEvent, err := s.opsRepo.GetActiveAlertEvent(ctx, rule.ID)
		if err != nil {
			logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] get active event failed (rule=%d): %v", rule.ID, err)
			continue
		}

		if breachedNow && consecutive >= required {
			if activeEvent != nil {
				continue
			}

			// Scoped silencing: if a matching silence exists, skip creating a firing event.
			if s.opsService != nil {
				platform := strings.TrimSpace(scopePlatform)
				region := scopeRegion
				if platform != "" {
					if ok, err := s.opsService.IsAlertSilenced(ctx, rule.ID, platform, scopeGroupID, region, now); err == nil && ok {
						continue
					}
				}
			}

			latestEvent, err := s.opsRepo.GetLatestAlertEvent(ctx, rule.ID)
			if err != nil {
				logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] get latest event failed (rule=%d): %v", rule.ID, err)
				continue
			}
			if latestEvent != nil && rule.CooldownMinutes > 0 {
				cooldown := time.Duration(rule.CooldownMinutes) * time.Minute
				if now.Sub(latestEvent.FiredAt) < cooldown {
					continue
				}
			}

			firedEvent := &OpsAlertEvent{
				RuleID:         rule.ID,
				Severity:       strings.TrimSpace(rule.Severity),
				Status:         OpsAlertStatusFiring,
				Title:          fmt.Sprintf("%s: %s", strings.TrimSpace(rule.Severity), strings.TrimSpace(rule.Name)),
				Description:    buildOpsAlertDescription(rule, metricValue, windowMinutes, scopePlatform, scopeGroupID),
				MetricValue:    float64Ptr(metricValue),
				ThresholdValue: float64Ptr(rule.Threshold),
				Dimensions:     buildOpsAlertDimensions(scopePlatform, scopeGroupID),
				FiredAt:        now,
				CreatedAt:      now,
			}

			created, err := s.opsRepo.CreateAlertEvent(ctx, firedEvent)
			if err != nil {
				logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] create event failed (rule=%d): %v", rule.ID, err)
				continue
			}

			eventsCreated++
			if created != nil && created.ID > 0 {
				if s.maybeSendAlertEmail(ctx, runtimeCfg, rule, created) {
					emailsSent++
				}
				if s.maybeSendAlertEnterpriseWeChat(ctx, runtimeCfg, rule, created) {
					enterpriseWeChatSent++
				}
			}
			continue
		}

		// Not breached: resolve active event if present.
		if activeEvent != nil {
			resolvedAt := now
			if err := s.opsRepo.UpdateAlertEventStatus(ctx, activeEvent.ID, OpsAlertStatusResolved, &resolvedAt); err != nil {
				logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] resolve event failed (event=%d): %v", activeEvent.ID, err)
			} else {
				eventsResolved++
			}
		}
	}

	accountHealthSent := s.evaluateAccountHealthNotifications(ctx, runtimeCfg)
	upstreamConnectionBalancesEvaluated := 0
	if s.opsService != nil && runtimeCfg.UpstreamConnectionBalance != nil {
		upstreamConnectionBalancesEvaluated = s.opsService.RunUpstreamConnectionBalanceMonitorCycle(ctx, *runtimeCfg.UpstreamConnectionBalance)
	}

	result := truncateString(fmt.Sprintf("rules=%d enabled=%d evaluated=%d created=%d resolved=%d emails_sent=%d enterprise_wechat_sent=%d account_health_sent=%d upstream_connection_balances_evaluated=%d", rulesTotal, rulesEnabled, rulesEvaluated, eventsCreated, eventsResolved, emailsSent, enterpriseWeChatSent, accountHealthSent, upstreamConnectionBalancesEvaluated), 2048)
	s.recordHeartbeatSuccess(runAt, time.Since(startedAt), result)
}

func (s *OpsAlertEvaluatorService) pruneRuleStates(rules []*OpsAlertRule) {
	s.mu.Lock()
	defer s.mu.Unlock()

	live := map[int64]struct{}{}
	for _, r := range rules {
		if r != nil && r.ID > 0 {
			live[r.ID] = struct{}{}
		}
	}
	for id := range s.ruleStates {
		if _, ok := live[id]; !ok {
			delete(s.ruleStates, id)
		}
	}
}

func (s *OpsAlertEvaluatorService) resetRuleState(ruleID int64, now time.Time) {
	if ruleID <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.ruleStates[ruleID]
	if !ok {
		state = &opsAlertRuleState{}
		s.ruleStates[ruleID] = state
	}
	state.LastEvaluatedAt = now
	state.ConsecutiveBreaches = 0
}

func (s *OpsAlertEvaluatorService) updateRuleBreaches(ruleID int64, now time.Time, interval time.Duration, breached bool) int {
	if ruleID <= 0 {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.ruleStates[ruleID]
	if !ok {
		state = &opsAlertRuleState{}
		s.ruleStates[ruleID] = state
	}

	if !state.LastEvaluatedAt.IsZero() && interval > 0 {
		if now.Sub(state.LastEvaluatedAt) > interval*2 {
			state.ConsecutiveBreaches = 0
		}
	}

	state.LastEvaluatedAt = now
	if breached {
		state.ConsecutiveBreaches++
	} else {
		state.ConsecutiveBreaches = 0
	}
	return state.ConsecutiveBreaches
}

func requiredSustainedBreaches(sustainedMinutes int, interval time.Duration) int {
	if sustainedMinutes <= 0 {
		return 1
	}
	if interval <= 0 {
		return sustainedMinutes
	}
	required := int(math.Ceil(float64(sustainedMinutes*60) / interval.Seconds()))
	if required < 1 {
		return 1
	}
	return required
}

func parseOpsAlertRuleScope(filters map[string]any) (platform string, groupID *int64, region *string) {
	if filters == nil {
		return "", nil, nil
	}
	if v, ok := filters["platform"]; ok {
		if s, ok := v.(string); ok {
			platform = strings.TrimSpace(s)
		}
	}
	if v, ok := filters["group_id"]; ok {
		switch t := v.(type) {
		case float64:
			if t > 0 {
				id := int64(t)
				groupID = &id
			}
		case int64:
			if t > 0 {
				id := t
				groupID = &id
			}
		case int:
			if t > 0 {
				id := int64(t)
				groupID = &id
			}
		case string:
			n, err := strconv.ParseInt(strings.TrimSpace(t), 10, 64)
			if err == nil && n > 0 {
				groupID = &n
			}
		}
	}
	if v, ok := filters["region"]; ok {
		if s, ok := v.(string); ok {
			vv := strings.TrimSpace(s)
			if vv != "" {
				region = &vv
			}
		}
	}
	return platform, groupID, region
}

func (s *OpsAlertEvaluatorService) computeRuleMetric(
	ctx context.Context,
	rule *OpsAlertRule,
	systemMetrics *OpsSystemMetricsSnapshot,
	start time.Time,
	end time.Time,
	platform string,
	groupID *int64,
) (float64, bool) {
	if rule == nil {
		return 0, false
	}
	switch strings.TrimSpace(rule.MetricType) {
	case "cpu_usage_percent":
		if systemMetrics != nil && systemMetrics.CPUUsagePercent != nil {
			return *systemMetrics.CPUUsagePercent, true
		}
		return 0, false
	case "memory_usage_percent":
		if systemMetrics != nil && systemMetrics.MemoryUsagePercent != nil {
			return *systemMetrics.MemoryUsagePercent, true
		}
		return 0, false
	case "concurrency_queue_depth":
		if systemMetrics != nil && systemMetrics.ConcurrencyQueueDepth != nil {
			return float64(*systemMetrics.ConcurrencyQueueDepth), true
		}
		return 0, false
	case "group_available_accounts":
		if groupID == nil || *groupID <= 0 {
			return 0, false
		}
		if s == nil || s.opsService == nil {
			return 0, false
		}
		availability, err := s.opsService.GetAccountAvailability(ctx, platform, groupID)
		if err != nil || availability == nil {
			return 0, false
		}
		if availability.Group == nil {
			return 0, true
		}
		return float64(availability.Group.AvailableCount), true
	case "group_available_ratio":
		if groupID == nil || *groupID <= 0 {
			return 0, false
		}
		if s == nil || s.opsService == nil {
			return 0, false
		}
		availability, err := s.opsService.GetAccountAvailability(ctx, platform, groupID)
		if err != nil || availability == nil {
			return 0, false
		}
		return computeGroupAvailableRatio(availability.Group), true
	case "account_rate_limited_count":
		if s == nil || s.opsService == nil {
			return 0, false
		}
		availability, err := s.opsService.GetAccountAvailability(ctx, platform, groupID)
		if err != nil || availability == nil {
			return 0, false
		}
		return float64(countAccountsByCondition(availability.Accounts, func(acc *AccountAvailability) bool {
			return acc.IsRateLimited
		})), true
	case "account_error_count":
		if s == nil || s.opsService == nil {
			return 0, false
		}
		availability, err := s.opsService.GetAccountAvailability(ctx, platform, groupID)
		if err != nil || availability == nil {
			return 0, false
		}
		return float64(countAccountsByCondition(availability.Accounts, func(acc *AccountAvailability) bool {
			return acc.HasError && acc.TempUnschedulableUntil == nil
		})), true
	case "account_temp_unscheduled_count":
		if s == nil || s.opsService == nil {
			return 0, false
		}
		availability, err := s.opsService.GetAccountAvailability(ctx, platform, groupID)
		if err != nil || availability == nil {
			return 0, false
		}
		now := time.Now().UTC()
		return float64(countAccountsByCondition(availability.Accounts, func(acc *AccountAvailability) bool {
			return acc.TempUnschedulableUntil != nil && now.Before(*acc.TempUnschedulableUntil)
		})), true
	case "group_rate_limit_ratio":
		if groupID == nil || *groupID <= 0 {
			return 0, false
		}
		if s == nil || s.opsService == nil {
			return 0, false
		}
		availability, err := s.opsService.GetAccountAvailability(ctx, platform, groupID)
		if err != nil || availability == nil {
			return 0, false
		}
		if availability.Group == nil || availability.Group.TotalAccounts <= 0 {
			return 0, true
		}
		return (float64(availability.Group.RateLimitCount) / float64(availability.Group.TotalAccounts)) * 100, true
	case "account_error_ratio":
		if s == nil || s.opsService == nil {
			return 0, false
		}
		availability, err := s.opsService.GetAccountAvailability(ctx, platform, groupID)
		if err != nil || availability == nil {
			return 0, false
		}
		total := int64(len(availability.Accounts))
		if total <= 0 {
			return 0, true
		}
		errorCount := countAccountsByCondition(availability.Accounts, func(acc *AccountAvailability) bool {
			return acc.HasError && acc.TempUnschedulableUntil == nil
		})
		return (float64(errorCount) / float64(total)) * 100, true
	case "overload_account_count":
		if s == nil || s.opsService == nil {
			return 0, false
		}
		availability, err := s.opsService.GetAccountAvailability(ctx, platform, groupID)
		if err != nil || availability == nil {
			return 0, false
		}
		return float64(countAccountsByCondition(availability.Accounts, func(acc *AccountAvailability) bool {
			return acc.IsOverloaded
		})), true
	case "proxy_expired_count":
		if s == nil || s.proxyRepo == nil {
			return 0, false
		}
		n, err := s.proxyRepo.CountExpired(ctx)
		if err != nil {
			return 0, false
		}
		return float64(n), true
	case "proxy_expiring_soon_count":
		if s == nil || s.proxyRepo == nil {
			return 0, false
		}
		n, err := s.proxyRepo.CountExpiringSoon(ctx, time.Now())
		if err != nil {
			return 0, false
		}
		return float64(n), true
	}

	overview, err := s.opsRepo.GetDashboardOverview(ctx, &OpsDashboardFilter{
		StartTime: start,
		EndTime:   end,
		Platform:  platform,
		GroupID:   groupID,
		QueryMode: OpsQueryModeRaw,
	})
	if err != nil {
		return 0, false
	}
	if overview == nil {
		return 0, false
	}

	switch strings.TrimSpace(rule.MetricType) {
	case "success_rate":
		if overview.RequestCountSLA <= 0 {
			return 0, false
		}
		return overview.SLA * 100, true
	case "error_rate":
		if overview.RequestCountSLA <= 0 {
			return 0, false
		}
		return overview.ErrorRate * 100, true
	case "upstream_error_rate":
		if overview.RequestCountSLA <= 0 {
			return 0, false
		}
		return overview.UpstreamErrorRate * 100, true
	default:
		return 0, false
	}
}

func compareMetric(value float64, operator string, threshold float64) bool {
	switch strings.TrimSpace(operator) {
	case ">":
		return value > threshold
	case ">=":
		return value >= threshold
	case "<":
		return value < threshold
	case "<=":
		return value <= threshold
	case "==":
		return value == threshold
	case "!=":
		return value != threshold
	default:
		return false
	}
}

func buildOpsAlertDimensions(platform string, groupID *int64) map[string]any {
	dims := map[string]any{}
	if strings.TrimSpace(platform) != "" {
		dims["platform"] = strings.TrimSpace(platform)
	}
	if groupID != nil && *groupID > 0 {
		dims["group_id"] = *groupID
	}
	if len(dims) == 0 {
		return nil
	}
	return dims
}

func buildOpsAlertDescription(rule *OpsAlertRule, value float64, windowMinutes int, platform string, groupID *int64) string {
	if rule == nil {
		return ""
	}
	scope := "overall"
	if strings.TrimSpace(platform) != "" {
		scope = fmt.Sprintf("platform=%s", strings.TrimSpace(platform))
	}
	if groupID != nil && *groupID > 0 {
		scope = fmt.Sprintf("%s group_id=%d", scope, *groupID)
	}
	if windowMinutes <= 0 {
		windowMinutes = 1
	}
	return fmt.Sprintf("%s %s %.2f (current %.2f) over last %dm (%s)",
		strings.TrimSpace(rule.MetricType),
		strings.TrimSpace(rule.Operator),
		rule.Threshold,
		value,
		windowMinutes,
		strings.TrimSpace(scope),
	)
}

func (s *OpsAlertEvaluatorService) evaluateAccountHealthNotifications(ctx context.Context, runtimeCfg *OpsAlertRuntimeSettings) int {
	if s == nil || s.opsService == nil || runtimeCfg == nil {
		return 0
	}

	settings := runtimeCfg.AccountHealth
	normalizeOpsAccountHealthSettings(&settings)
	if !settings.Enabled {
		return 0
	}

	health, err := s.opsService.GetAccountHealth(ctx, &OpsAccountHealthFilter{
		RecentLimit: opsAccountHealthDefaultRecentLimit,
	})
	if err != nil || health == nil {
		if err != nil {
			logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] account health load failed: %v", err)
		}
		return 0
	}

	webhookURL := strings.TrimSpace(settings.Notification.EnterpriseWeChatWebhookURL)
	if !settings.Notification.EnterpriseWeChatEnabled || webhookURL == "" || isOpsWebhookMasked(webhookURL) {
		_ = s.scheduleAccountHealthRecoveryProbes(ctx, health.Items, settings)
		return 0
	}
	if err := validateOpsEnterpriseWeChatWebhookURL(webhookURL); err != nil {
		logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] enterprise wechat webhook invalid: %v", err)
		_ = s.scheduleAccountHealthRecoveryProbes(ctx, health.Items, settings)
		return 0
	}

	now := time.Now().UTC()
	sent := 0
	digestItems := make([]*OpsAccountHealthItem, 0)
	for _, item := range health.Items {
		if item == nil {
			continue
		}
		mode := strings.TrimSpace(item.Recommendation.NotifyMode)
		if mode == "" || mode == OpsAccountHealthNotifyNone {
			continue
		}
		immediate := mode == OpsAccountHealthNotifyImmediate || item.Recommendation.Immediate
		if !s.shouldSendAccountHealthNotification(ctx, item, settings, now) {
			continue
		}
		if !immediate {
			if shouldDigestOpsAccountHealthNotification(item) {
				digestItems = append(digestItems, item)
				continue
			}
			if s.accountHealthLimiter == nil {
				s.accountHealthLimiter = newSlidingWindowLimiter(settings.RateLimitPerHour, time.Hour)
			}
			s.accountHealthLimiter.SetLimit(settings.RateLimitPerHour)
			if !s.accountHealthLimiter.Allow(now) {
				continue
			}
			content := buildOpsAccountHealthWeComText(item, now)
			if err := sendOpsEnterpriseWeChatMarkdown(ctx, webhookURL, content, false); err != nil {
				logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] enterprise wechat account health send failed (account=%d): %v", item.AccountID, err)
				continue
			}
			s.markAccountHealthNotificationSent(ctx, item, settings, now)
			sent++
			continue
		}

		mentionAll := immediate && settings.Notification.MentionAllOnImmediate
		content := buildOpsAccountHealthWeComText(item, now)
		if err := sendOpsEnterpriseWeChatMarkdown(ctx, webhookURL, content, mentionAll); err != nil {
			logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] enterprise wechat account health send failed (account=%d): %v", item.AccountID, err)
			continue
		}
		s.markAccountHealthNotificationSent(ctx, item, settings, now)
		sent++
	}
	if len(digestItems) > 0 {
		if s.accountHealthLimiter == nil {
			s.accountHealthLimiter = newSlidingWindowLimiter(settings.RateLimitPerHour, time.Hour)
		}
		s.accountHealthLimiter.SetLimit(settings.RateLimitPerHour)
		if s.accountHealthLimiter.Allow(now) {
			content := buildOpsAccountHealthDigestWeComText(digestItems, now)
			if err := sendOpsEnterpriseWeChatMarkdown(ctx, webhookURL, content, false); err != nil {
				logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] enterprise wechat account health digest send failed (items=%d): %v", len(digestItems), err)
			} else {
				for _, item := range digestItems {
					s.markAccountHealthNotificationSent(ctx, item, settings, now)
				}
				sent++
			}
		}
	}
	_ = s.scheduleAccountHealthRecoveryProbes(ctx, health.Items, settings)
	return sent
}

func shouldDigestOpsAccountHealthNotification(item *OpsAccountHealthItem) bool {
	if item == nil {
		return false
	}
	rec := item.Recommendation
	return rec.NotifyMode == OpsAccountHealthNotifyDigest &&
		strings.TrimSpace(rec.Severity) == "P2" &&
		strings.TrimSpace(rec.Title) == "账号持续变差，建议处理"
}

func (s *OpsAlertEvaluatorService) scheduleAccountHealthRecoveryProbes(ctx context.Context, items []*OpsAccountHealthItem, settings OpsAccountHealthSettings) int {
	// Auto-probe is controlled solely by Probe settings (interval / max_per_run / enabled).
	// Recovery.Enabled only governs reopen recommendations & notifications — not probing itself.
	if s == nil || s.accountTestService == nil || !settings.Probe.Enabled {
		return 0
	}
	maxPerRun := settings.Probe.MaxPerRun
	if maxPerRun <= 0 {
		maxPerRun = 1
	}
	if maxPerRun > 20 {
		maxPerRun = 20
	}
	timeout := time.Duration(settings.Probe.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 20 * time.Second
	}

	candidates := make([]*OpsAccountHealthItem, 0, len(items))
	for _, item := range items {
		if shouldProbeAccountHealthRecovery(item, settings) {
			candidates = append(candidates, item)
		}
	}
	// Prefer accounts that never probed / probed longest ago so rotation is fair under max_per_run.
	sort.SliceStable(candidates, func(i, j int) bool {
		return accountHealthProbePriority(candidates[i]) < accountHealthProbePriority(candidates[j])
	})

	runs := 0
	for _, item := range candidates {
		if runs >= maxPerRun {
			break
		}
		if !s.tryAcquireAccountHealthProbeSlot(ctx, item.AccountID, settings) {
			continue
		}
		accountID := item.AccountID
		modelID := resolveOpsAccountHealthProbeModelID(item.ProbeModelEffective, settings.Probe.ModelID)
		mode := settings.Probe.Mode
		prompt := settings.Probe.Prompt
		runs++
		go s.runAccountHealthRecoveryProbe(accountID, modelID, prompt, mode, timeout)
	}
	return runs
}

// accountHealthProbePriority lower = probe sooner. Never-probed accounts rank first.
func accountHealthProbePriority(item *OpsAccountHealthItem) int64 {
	if item == nil {
		return time.Now().UnixNano()
	}
	if item.Probe == nil || item.Probe.CheckedAt == nil || item.Probe.CheckedAt.IsZero() {
		return 0
	}
	return item.Probe.CheckedAt.UTC().UnixNano()
}

func (s *OpsAlertEvaluatorService) runAccountHealthRecoveryProbe(accountID int64, modelID string, prompt string, mode string, timeout time.Duration) {
	if s == nil || s.accountTestService == nil || accountID <= 0 {
		return
	}
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	probeCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if _, err := s.accountTestService.RunAccountHealthProbeWithOptions(probeCtx, accountID, modelID, prompt, mode); err != nil {
		logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] account health probe failed (account=%d): %v", accountID, err)
	}
}

func shouldProbeAccountHealthRecovery(item *OpsAccountHealthItem, settings OpsAccountHealthSettings) bool {
	if item == nil || item.AccountID <= 0 || item.ProbeAutoDisabled {
		return false
	}
	if !settings.Probe.Enabled {
		return false
	}
	// Closed accounts always need probes when auto-probe is on — they have little/no live
	// traffic for quality judgment. Opened-but-unavailable accounts also get connectivity checks.
	if !item.IsOpened {
		return true
	}
	if !item.IsAvailable {
		return true
	}
	return false
}

func (s *OpsAlertEvaluatorService) tryAcquireAccountHealthProbeSlot(ctx context.Context, accountID int64, settings OpsAccountHealthSettings) bool {
	if s == nil || accountID <= 0 {
		return false
	}
	ttl := time.Duration(settings.Probe.IntervalMinutes) * time.Minute
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	key := fmt.Sprintf("ops:account-health:probe:%d", accountID)
	if s.redisClient != nil {
		ok, err := s.redisClient.SetNX(ctx, key, time.Now().UTC().Format(time.RFC3339Nano), ttl).Result()
		if err == nil {
			return ok
		}
		logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] account health probe redis cooldown failed: %v", err)
	}

	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.accountHealthProbeAt == nil {
		s.accountHealthProbeAt = map[int64]time.Time{}
	}
	last := s.accountHealthProbeAt[accountID]
	if !last.IsZero() && now.Sub(last) < ttl {
		return false
	}
	s.accountHealthProbeAt[accountID] = now
	return true
}

func (s *OpsAlertEvaluatorService) shouldSendAccountHealthNotification(ctx context.Context, item *OpsAccountHealthItem, settings OpsAccountHealthSettings, now time.Time) bool {
	if s == nil || item == nil {
		return false
	}
	key := accountHealthNotifyKey(item)
	if key == "" {
		return false
	}
	cooldownMinutes := accountHealthNotifyCooldownMinutes(item, settings)
	if cooldownMinutes <= 0 {
		return true
	}
	if s.redisClient != nil {
		redisKey := "ops:account-health:notify:" + key
		exists, err := s.redisClient.Exists(ctx, redisKey).Result()
		if err == nil {
			return exists == 0
		}
		logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] account health notify redis cooldown failed: %v", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.accountHealthNotifyAt == nil {
		s.accountHealthNotifyAt = map[string]time.Time{}
	}
	last := s.accountHealthNotifyAt[key]
	if !last.IsZero() && now.Sub(last) < time.Duration(cooldownMinutes)*time.Minute {
		return false
	}
	return true
}

func (s *OpsAlertEvaluatorService) markAccountHealthNotificationSent(ctx context.Context, item *OpsAccountHealthItem, settings OpsAccountHealthSettings, now time.Time) {
	if s == nil || item == nil {
		return
	}
	key := accountHealthNotifyKey(item)
	if key == "" {
		return
	}
	cooldownMinutes := accountHealthNotifyCooldownMinutes(item, settings)
	if cooldownMinutes > 0 && s.redisClient != nil {
		redisKey := "ops:account-health:notify:" + key
		if err := s.redisClient.Set(ctx, redisKey, now.Format(time.RFC3339Nano), time.Duration(cooldownMinutes)*time.Minute).Err(); err != nil {
			logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] account health notify redis mark failed: %v", err)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.accountHealthNotifyAt == nil {
		s.accountHealthNotifyAt = map[string]time.Time{}
	}
	s.accountHealthNotifyAt[key] = now
}

func accountHealthNotifyKey(item *OpsAccountHealthItem) string {
	if item == nil || item.AccountID <= 0 {
		return ""
	}
	rec := item.Recommendation
	return fmt.Sprintf("%d:%s:%s:%s", item.AccountID, strings.TrimSpace(rec.Action), strings.TrimSpace(rec.NotifyMode), strings.TrimSpace(rec.Severity))
}

func accountHealthNotifyCooldownMinutes(item *OpsAccountHealthItem, settings OpsAccountHealthSettings) int {
	if item == nil {
		return 30
	}
	if item.Recommendation.NotifyMode == OpsAccountHealthNotifyImmediate || item.Recommendation.Immediate {
		return settings.Burst.CooldownMinutes
	}
	if item.Recommendation.RecoveryReady || item.Recommendation.Action == OpsAccountHealthActionCanOpen {
		return settings.Recovery.CooldownMinutes
	}
	if item.Recommendation.Action == OpsAccountHealthActionCloseNow {
		return settings.Degrade.CooldownMinutes
	}
	return 30
}

func buildOpsAccountHealthWeComText(item *OpsAccountHealthItem, now time.Time) string {
	if item == nil {
		return ""
	}
	rec := item.Recommendation
	lines := []string{
		fmt.Sprintf("## <font color=\"%s\">[%s] %s</font>",
			opsAccountHealthSeverityColor(rec.Severity),
			escapeOpsWeComMarkdown(strings.TrimSpace(rec.Severity)),
			escapeOpsWeComMarkdown(strings.TrimSpace(rec.Title)),
		),
		fmt.Sprintf("> **账号**：%s (#%d)", escapeOpsWeComMarkdown(accountHealthNotifyAccountName(item)), item.AccountID),
		fmt.Sprintf("> **平台/分组**：%s / %s (#%d)",
			escapeOpsWeComMarkdown(strings.TrimSpace(item.Platform)),
			escapeOpsWeComMarkdown(strings.TrimSpace(item.GroupName)),
			item.GroupID,
		),
		fmt.Sprintf("> **时间**：%s", formatOpsNotifyTime(now)),
		"",
		"**建议动作**",
		fmt.Sprintf("> <font color=\"%s\">%s</font>",
			opsAccountHealthSeverityColor(rec.Severity),
			escapeOpsWeComMarkdown(opsAccountHealthNotifyActionText(rec.Action)),
		),
		fmt.Sprintf("> **原因**：%s", escapeOpsWeComMarkdown(strings.TrimSpace(rec.Reason))),
		"",
		"**状态概览**",
		fmt.Sprintf("> 调度状态：%s / %s",
			opsAccountHealthBoolText(item.IsOpened, "已打开", "已关闭"),
			opsAccountHealthBoolText(item.IsAvailable, "可调度", "不可调度"),
		),
		fmt.Sprintf("> 异常标记：%s", escapeOpsWeComMarkdown(opsAccountHealthFlagSummary(item))),
		"",
		"**窗口指标**",
		formatOpsAccountHealthWindowForMarkdown("1m", item.Windows[OpsAccountHealthWindow1m]),
		formatOpsAccountHealthWindowForMarkdown("5m", item.Windows[OpsAccountHealthWindow5m]),
		formatOpsAccountHealthWindowForMarkdown("10m", item.Windows[OpsAccountHealthWindow10m]),
		formatOpsAccountHealthWindowForMarkdown("30m", item.Windows[OpsAccountHealthWindow30m]),
	}
	return truncateString(strings.Join(lines, "\n"), 1900)
}

func buildOpsAccountHealthDigestWeComText(items []*OpsAccountHealthItem, now time.Time) string {
	lines := []string{
		fmt.Sprintf("## <font color=\"warning\">Sub2API 账号健康汇总 · %d 个账号</font>", len(items)),
		fmt.Sprintf("> **时间**：%s", formatOpsNotifyTime(now)),
		"> **说明**：汇总包含持续变差或需要人工关注的账号，请按建议动作处理。",
		"",
		"**重点账号**",
	}
	limit := 12
	shown := 0
	for _, item := range items {
		if item == nil {
			continue
		}
		if shown >= limit {
			lines = append(lines, fmt.Sprintf("> 还有 **%d** 个账号未列出，请打开账号健康看板查看。", len(items)-limit))
			break
		}
		rec := item.Recommendation
		shown++
		lines = append(lines, fmt.Sprintf("%d. <font color=\"%s\">[%s] %s</font> %s (#%d)",
			shown,
			opsAccountHealthSeverityColor(rec.Severity),
			escapeOpsWeComMarkdown(strings.TrimSpace(rec.Severity)),
			opsAccountHealthNotifyActionText(rec.Action),
			escapeOpsWeComMarkdown(accountHealthNotifyAccountName(item)),
			item.AccountID,
		))
		lines = append(lines, fmt.Sprintf("> %s / %s (#%d)",
			escapeOpsWeComMarkdown(strings.TrimSpace(item.Platform)),
			escapeOpsWeComMarkdown(strings.TrimSpace(item.GroupName)),
			item.GroupID,
		))
		lines = append(lines, formatOpsAccountHealthWindowForMarkdown("5m", item.Windows[OpsAccountHealthWindow5m]))
		lines = append(lines, formatOpsAccountHealthWindowForMarkdown("10m", item.Windows[OpsAccountHealthWindow10m]))
		lines = append(lines, formatOpsAccountHealthWindowForMarkdown("30m", item.Windows[OpsAccountHealthWindow30m]))
		if reason := strings.TrimSpace(rec.Reason); reason != "" {
			lines = append(lines, fmt.Sprintf("> 原因：%s", escapeOpsWeComMarkdown(reason)))
		}
		lines = append(lines, "")
	}
	return truncateString(strings.Join(lines, "\n"), 1900)
}

func buildOpsAccountHealthTestWeComMarkdown(now time.Time) string {
	lines := []string{
		"## <font color=\"info\">Sub2API 账号健康测试通知</font>",
		fmt.Sprintf("> **时间**：%s", formatOpsNotifyTime(now)),
		"> **通道**：企业微信机器人 Markdown",
		"> **结果**：Webhook 已连通，账号健康通知将按此样式展示。",
		"",
		"**示例重点**",
		"> <font color=\"warning\">[P1] 账号处于错误状态</font>",
		"> 建议动作：建议关闭；原因、窗口指标和分组信息会分段展示。",
		"",
		"**窗口指标示例**",
		"> 1m：请求 **24** · 成功 <font color=\"warning\">58.3%</font> · 错误 41.7% · 上游 29.2%",
	}
	return truncateString(strings.Join(lines, "\n"), 1900)
}

func opsAccountHealthNotifyActionText(action string) string {
	switch strings.TrimSpace(action) {
	case OpsAccountHealthActionCloseNow:
		return "建议关闭"
	case OpsAccountHealthActionWatch:
		return "继续观察"
	case OpsAccountHealthActionCanOpen:
		return "可尝试打开"
	case OpsAccountHealthActionKeepOpen:
		return "保持开启"
	case OpsAccountHealthActionKeepClosed:
		return "保持关闭"
	case OpsAccountHealthActionNeedsProbe:
		return "需要探测"
	default:
		return "查看看板"
	}
}

func formatOpsAccountHealthWindowForMarkdown(label string, stat *OpsAccountHealthWindowStats) string {
	label = escapeOpsWeComMarkdown(strings.TrimSpace(label))
	if stat == nil || stat.RequestCount <= 0 {
		return fmt.Sprintf("> %s：<font color=\"comment\">无数据</font>", label)
	}
	successColor := "info"
	if stat.SuccessRatePercent < 90 || stat.ErrorRatePercent >= 10 || stat.UpstreamErrorRatePercent >= 5 {
		successColor = "warning"
	}
	return fmt.Sprintf(
		"> %s：请求 **%d** · 成功 <font color=\"%s\">%.1f%%</font> · 错误 %.1f%% · 上游 %.1f%%",
		label,
		stat.RequestCount,
		successColor,
		stat.SuccessRatePercent,
		stat.ErrorRatePercent,
		stat.UpstreamErrorRatePercent,
	)
}

func opsAccountHealthSeverityColor(severity string) string {
	switch strings.ToUpper(strings.TrimSpace(severity)) {
	case "P0", "P1", "CRITICAL":
		return "warning"
	case "P2", "WARNING":
		return "info"
	default:
		return "comment"
	}
}

func accountHealthNotifyAccountName(item *OpsAccountHealthItem) string {
	if item == nil {
		return "未知账号"
	}
	name := strings.TrimSpace(item.AccountName)
	if name == "" {
		return fmt.Sprintf("#%d", item.AccountID)
	}
	return name
}

func opsAccountHealthBoolText(ok bool, yes string, no string) string {
	if ok {
		return yes
	}
	return no
}

func opsAccountHealthFlagSummary(item *OpsAccountHealthItem) string {
	if item == nil {
		return "无数据"
	}
	flags := make([]string, 0, 5)
	if item.HasError {
		flags = append(flags, "错误状态")
	}
	if item.IsRateLimited {
		flags = append(flags, "限流")
	}
	if item.IsOverloaded {
		flags = append(flags, "过载")
	}
	if item.IsTempUnschedulable {
		flags = append(flags, "临时不可调度")
	}
	if !item.IsSchedulable {
		flags = append(flags, "调度关闭")
	}
	if len(flags) == 0 {
		return "无明显异常"
	}
	return strings.Join(flags, " / ")
}

func formatOpsNotifyTime(t time.Time) string {
	if t.IsZero() {
		t = time.Now()
	}
	return t.In(opsNotifyTimeLocation).Format("2006-01-02 15:04:05 Asia/Shanghai")
}

func escapeOpsWeComMarkdown(value string) string {
	value = strings.TrimSpace(value)
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
	)
	return replacer.Replace(value)
}

func sendOpsEnterpriseWeChatText(ctx context.Context, webhookURL string, content string, mentionAll bool) error {
	webhookURL = strings.TrimSpace(webhookURL)
	if webhookURL == "" {
		return fmt.Errorf("enterprise wechat webhook url is empty")
	}
	if err := validateOpsEnterpriseWeChatWebhookURL(webhookURL); err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	requestCtx, cancel := context.WithTimeout(ctx, opsEnterpriseWeChatSendTimeout)
	defer cancel()

	payload := struct {
		MsgType string `json:"msgtype"`
		Text    struct {
			Content       string   `json:"content"`
			MentionedList []string `json:"mentioned_list,omitempty"`
		} `json:"text"`
	}{
		MsgType: "text",
	}
	payload.Text.Content = strings.TrimSpace(content)
	if mentionAll {
		payload.Text.MentionedList = []string{"@all"}
	}

	return postOpsEnterpriseWeChatPayload(requestCtx, webhookURL, payload)
}

func sendOpsEnterpriseWeChatMarkdown(ctx context.Context, webhookURL string, content string, mentionAll bool) error {
	webhookURL = strings.TrimSpace(webhookURL)
	if webhookURL == "" {
		return fmt.Errorf("enterprise wechat webhook url is empty")
	}
	if err := validateOpsEnterpriseWeChatWebhookURL(webhookURL); err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	requestCtx, cancel := context.WithTimeout(ctx, opsEnterpriseWeChatSendTimeout)
	defer cancel()

	payload := struct {
		MsgType  string `json:"msgtype"`
		Markdown struct {
			Content string `json:"content"`
		} `json:"markdown"`
	}{
		MsgType: "markdown",
	}
	payload.Markdown.Content = strings.TrimSpace(content)
	if mentionAll {
		payload.Markdown.Content = strings.TrimSpace(payload.Markdown.Content + "\n\n<@all>")
	}
	return postOpsEnterpriseWeChatPayload(requestCtx, webhookURL, payload)
}

func postOpsEnterpriseWeChatPayload(ctx context.Context, webhookURL string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("enterprise wechat status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(respBody, &result); err == nil && result.ErrCode != 0 {
		return fmt.Errorf("enterprise wechat errcode %d: %s", result.ErrCode, strings.TrimSpace(result.ErrMsg))
	}
	return nil
}

func (s *OpsAlertEvaluatorService) maybeSendAlertEnterpriseWeChat(ctx context.Context, runtimeCfg *OpsAlertRuntimeSettings, rule *OpsAlertRule, event *OpsAlertEvent) bool {
	if s == nil || runtimeCfg == nil || event == nil {
		return false
	}
	settings := runtimeCfg.AccountHealth
	normalizeOpsAccountHealthSettings(&settings)
	notification := settings.Notification
	webhookURL := strings.TrimSpace(notification.EnterpriseWeChatWebhookURL)
	if !notification.EnterpriseWeChatEnabled || webhookURL == "" || isOpsWebhookMasked(webhookURL) {
		return false
	}
	content := buildOpsAlertEventWeComText(rule, event, time.Now().UTC())
	mentionAll := shouldMentionAllForOpsAlertEnterpriseWeChat(notification, event)
	if err := sendOpsEnterpriseWeChatText(ctx, webhookURL, content, mentionAll); err != nil {
		logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] enterprise wechat alert event send failed (event=%d): %v", event.ID, err)
		return false
	}
	return true
}

func shouldMentionAllForOpsAlertEnterpriseWeChat(notification OpsAccountHealthNotificationSettings, event *OpsAlertEvent) bool {
	if !notification.MentionAllOnImmediate || event == nil {
		return false
	}
	return isCriticalOpsAlertSeverity(event.Severity)
}

func isCriticalOpsAlertSeverity(severity string) bool {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "p0", "critical":
		return true
	default:
		return false
	}
}

func buildOpsAlertEventWeComText(rule *OpsAlertRule, event *OpsAlertEvent, now time.Time) string {
	if event == nil {
		return ""
	}
	lines := []string{
		"Sub2API 运维告警",
		fmt.Sprintf("时间: %s", formatOpsNotifyTime(now)),
		fmt.Sprintf("级别: %s", strings.TrimSpace(event.Severity)),
		fmt.Sprintf("标题: %s", strings.TrimSpace(event.Title)),
	}
	if rule != nil {
		lines = append(lines,
			fmt.Sprintf("规则: %s (#%d)", strings.TrimSpace(rule.Name), rule.ID),
			fmt.Sprintf("指标: %s %s %.2f", strings.TrimSpace(rule.MetricType), strings.TrimSpace(rule.Operator), rule.Threshold),
		)
	}
	if event.MetricValue != nil {
		lines = append(lines, fmt.Sprintf("当前值: %.2f", *event.MetricValue))
	}
	if desc := strings.TrimSpace(event.Description); desc != "" {
		lines = append(lines, fmt.Sprintf("说明: %s", desc))
	}
	return truncateString(strings.Join(lines, "\n"), 1900)
}

func (s *OpsAlertEvaluatorService) maybeSendAlertEmail(ctx context.Context, runtimeCfg *OpsAlertRuntimeSettings, rule *OpsAlertRule, event *OpsAlertEvent) bool {
	if s == nil || s.emailService == nil || s.opsService == nil || event == nil || rule == nil {
		return false
	}
	if event.EmailSent {
		return false
	}
	if !rule.NotifyEmail {
		return false
	}

	emailCfg, err := s.opsService.GetEmailNotificationConfig(ctx)
	if err != nil || emailCfg == nil || !emailCfg.Alert.Enabled {
		return false
	}

	if len(emailCfg.Alert.Recipients) == 0 {
		return false
	}
	if !shouldSendOpsAlertEmailByMinSeverity(strings.TrimSpace(emailCfg.Alert.MinSeverity), strings.TrimSpace(rule.Severity)) {
		return false
	}

	if runtimeCfg != nil && runtimeCfg.Silencing.Enabled {
		if isOpsAlertSilenced(time.Now().UTC(), rule, event, runtimeCfg.Silencing) {
			return false
		}
	}

	// Apply/update rate limiter.
	s.emailLimiter.SetLimit(emailCfg.Alert.RateLimitPerHour)

	subject := fmt.Sprintf("[Ops Alert][%s] %s", strings.TrimSpace(rule.Severity), strings.TrimSpace(rule.Name))
	body := buildOpsAlertEmailBody(rule, event)

	anySent := false
	for _, to := range emailCfg.Alert.Recipients {
		addr := strings.TrimSpace(to)
		if addr == "" {
			continue
		}
		if !s.emailLimiter.Allow(time.Now().UTC()) {
			continue
		}
		if s.emailService.notificationEmailService != nil {
			if err := s.emailService.notificationEmailService.Send(ctx, NotificationEmailSendInput{
				Event:          NotificationEmailEventOpsAlert,
				RecipientEmail: addr,
				RecipientName:  emailRecipientName(addr),
				SourceType:     "ops_alert",
				SourceID:       fmt.Sprintf("%d", event.ID),
				Variables:      opsAlertEmailVariables(rule, event),
			}); err == nil {
				anySent = true
				continue
			} else if !shouldFallbackNotificationEmail(err) {
				continue
			}
		}
		if err := s.emailService.SendEmail(ctx, addr, subject, body); err != nil {
			// Ignore per-recipient failures; continue best-effort.
			continue
		}
		anySent = true
	}

	if anySent {
		_ = s.opsRepo.UpdateAlertEventEmailSent(context.Background(), event.ID, true)
	}
	return anySent
}

func opsAlertEmailVariables(rule *OpsAlertRule, event *OpsAlertEvent) map[string]string {
	variables := map[string]string{
		"rule_name":         "-",
		"severity":          "-",
		"alert_status":      "-",
		"metric_type":       "-",
		"operator":          "-",
		"metric_value":      "-",
		"threshold_value":   "-",
		"triggered_at":      time.Now().UTC().Format(time.RFC3339),
		"alert_description": "-",
	}
	if rule != nil {
		variables["rule_name"] = strings.TrimSpace(rule.Name)
		variables["severity"] = strings.TrimSpace(rule.Severity)
		variables["metric_type"] = strings.TrimSpace(rule.MetricType)
		variables["operator"] = strings.TrimSpace(rule.Operator)
		variables["threshold_value"] = fmt.Sprintf("%.2f", rule.Threshold)
		if strings.TrimSpace(rule.Description) != "" {
			variables["alert_description"] = strings.TrimSpace(rule.Description)
		}
	}
	if event != nil {
		variables["alert_status"] = strings.TrimSpace(event.Status)
		if event.MetricValue != nil {
			variables["metric_value"] = fmt.Sprintf("%.2f", *event.MetricValue)
		}
		if event.ThresholdValue != nil {
			variables["threshold_value"] = fmt.Sprintf("%.2f", *event.ThresholdValue)
		}
		if !event.FiredAt.IsZero() {
			variables["triggered_at"] = event.FiredAt.UTC().Format(time.RFC3339)
		}
		if strings.TrimSpace(event.Description) != "" {
			variables["alert_description"] = strings.TrimSpace(event.Description)
		}
	}
	return variables
}

func buildOpsAlertEmailBody(rule *OpsAlertRule, event *OpsAlertEvent) string {
	if rule == nil || event == nil {
		return ""
	}
	metric := strings.TrimSpace(rule.MetricType)
	value := "-"
	threshold := fmt.Sprintf("%.2f", rule.Threshold)
	if event.MetricValue != nil {
		value = fmt.Sprintf("%.2f", *event.MetricValue)
	}
	if event.ThresholdValue != nil {
		threshold = fmt.Sprintf("%.2f", *event.ThresholdValue)
	}
	return fmt.Sprintf(`
<h2>Ops Alert</h2>
<p><b>Rule</b>: %s</p>
<p><b>Severity</b>: %s</p>
<p><b>Status</b>: %s</p>
<p><b>Metric</b>: %s %s %s</p>
<p><b>Fired at</b>: %s</p>
<p><b>Description</b>: %s</p>
`,
		htmlEscape(rule.Name),
		htmlEscape(rule.Severity),
		htmlEscape(event.Status),
		htmlEscape(metric),
		htmlEscape(rule.Operator),
		htmlEscape(fmt.Sprintf("%s (threshold %s)", value, threshold)),
		event.FiredAt.Format(time.RFC3339),
		htmlEscape(event.Description),
	)
}

func shouldSendOpsAlertEmailByMinSeverity(minSeverity string, ruleSeverity string) bool {
	minSeverity = strings.ToLower(strings.TrimSpace(minSeverity))
	if minSeverity == "" {
		return true
	}

	eventLevel := opsEmailSeverityForOps(ruleSeverity)
	minLevel := strings.ToLower(minSeverity)

	rank := func(level string) int {
		switch level {
		case "critical":
			return 3
		case "warning":
			return 2
		case "info":
			return 1
		default:
			return 0
		}
	}
	return rank(eventLevel) >= rank(minLevel)
}

func opsEmailSeverityForOps(severity string) string {
	switch strings.ToUpper(strings.TrimSpace(severity)) {
	case "P0":
		return "critical"
	case "P1":
		return "warning"
	default:
		return "info"
	}
}

func isOpsAlertSilenced(now time.Time, rule *OpsAlertRule, event *OpsAlertEvent, silencing OpsAlertSilencingSettings) bool {
	if !silencing.Enabled {
		return false
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if strings.TrimSpace(silencing.GlobalUntilRFC3339) != "" {
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(silencing.GlobalUntilRFC3339)); err == nil {
			if now.Before(t) {
				return true
			}
		}
	}

	for _, entry := range silencing.Entries {
		untilRaw := strings.TrimSpace(entry.UntilRFC3339)
		if untilRaw == "" {
			continue
		}
		until, err := time.Parse(time.RFC3339, untilRaw)
		if err != nil {
			continue
		}
		if now.After(until) {
			continue
		}
		if entry.RuleID != nil && rule != nil && rule.ID > 0 && *entry.RuleID != rule.ID {
			continue
		}
		if len(entry.Severities) > 0 {
			match := false
			for _, s := range entry.Severities {
				if strings.EqualFold(strings.TrimSpace(s), strings.TrimSpace(event.Severity)) || strings.EqualFold(strings.TrimSpace(s), strings.TrimSpace(rule.Severity)) {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		return true
	}

	return false
}

func (s *OpsAlertEvaluatorService) tryAcquireLeaderLock(ctx context.Context, lock OpsDistributedLockSettings) (func(), bool) {
	if !lock.Enabled {
		return nil, true
	}
	if s.redisClient == nil {
		s.warnNoRedisOnce.Do(func() {
			logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] redis not configured; running without distributed lock")
		})
		return nil, true
	}
	key := strings.TrimSpace(lock.Key)
	if key == "" {
		key = opsAlertEvaluatorLeaderLockKey
	}
	ttl := time.Duration(lock.TTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = opsAlertEvaluatorLeaderLockTTL
	}

	ok, err := s.redisClient.SetNX(ctx, key, s.instanceID, ttl).Result()
	if err != nil {
		// Prefer fail-closed to avoid duplicate evaluators stampeding the DB when Redis is flaky.
		// Single-node deployments can disable the distributed lock via runtime settings.
		s.warnNoRedisOnce.Do(func() {
			logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] leader lock SetNX failed; skipping this cycle: %v", err)
		})
		return nil, false
	}
	if !ok {
		s.maybeLogSkip(key)
		return nil, false
	}
	return func() {
		releaseCtx, releaseCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer releaseCancel()
		_, _ = opsAlertEvaluatorReleaseScript.Run(releaseCtx, s.redisClient, []string{key}, s.instanceID).Result()
	}, true
}

func (s *OpsAlertEvaluatorService) maybeLogSkip(key string) {
	s.skipLogMu.Lock()
	defer s.skipLogMu.Unlock()

	now := time.Now()
	if !s.skipLogAt.IsZero() && now.Sub(s.skipLogAt) < opsAlertEvaluatorSkipLogInterval {
		return
	}
	s.skipLogAt = now
	logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] leader lock held by another instance; skipping (key=%q)", key)
}

func (s *OpsAlertEvaluatorService) recordHeartbeatSuccess(runAt time.Time, duration time.Duration, result string) {
	if s == nil || s.opsRepo == nil {
		return
	}
	now := time.Now().UTC()
	durMs := duration.Milliseconds()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	msg := strings.TrimSpace(result)
	if msg == "" {
		msg = "ok"
	}
	msg = truncateString(msg, 2048)
	_ = s.opsRepo.UpsertJobHeartbeat(ctx, &OpsUpsertJobHeartbeatInput{
		JobName:        opsAlertEvaluatorJobName,
		LastRunAt:      &runAt,
		LastSuccessAt:  &now,
		LastDurationMs: &durMs,
		LastResult:     &msg,
	})
}

func (s *OpsAlertEvaluatorService) recordHeartbeatError(runAt time.Time, duration time.Duration, err error) {
	if s == nil || s.opsRepo == nil || err == nil {
		return
	}
	now := time.Now().UTC()
	durMs := duration.Milliseconds()
	msg := truncateString(err.Error(), 2048)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = s.opsRepo.UpsertJobHeartbeat(ctx, &OpsUpsertJobHeartbeatInput{
		JobName:        opsAlertEvaluatorJobName,
		LastRunAt:      &runAt,
		LastErrorAt:    &now,
		LastError:      &msg,
		LastDurationMs: &durMs,
	})
}

func htmlEscape(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&#39;",
	)
	return replacer.Replace(s)
}

type slidingWindowLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	sent   []time.Time
}

func newSlidingWindowLimiter(limit int, window time.Duration) *slidingWindowLimiter {
	if window <= 0 {
		window = time.Hour
	}
	return &slidingWindowLimiter{
		limit:  limit,
		window: window,
		sent:   []time.Time{},
	}
}

func (l *slidingWindowLimiter) SetLimit(limit int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.limit = limit
}

func (l *slidingWindowLimiter) Allow(now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.limit <= 0 {
		return true
	}
	cutoff := now.Add(-l.window)
	keep := l.sent[:0]
	for _, t := range l.sent {
		if t.After(cutoff) {
			keep = append(keep, t)
		}
	}
	l.sent = keep
	if len(l.sent) >= l.limit {
		return false
	}
	l.sent = append(l.sent, now)
	return true
}

// computeGroupAvailableRatio returns the available percentage for a group.
// Formula: (AvailableCount / TotalAccounts) * 100.
// Returns 0 when TotalAccounts is 0.
func computeGroupAvailableRatio(group *GroupAvailability) float64 {
	if group == nil || group.TotalAccounts <= 0 {
		return 0
	}
	return (float64(group.AvailableCount) / float64(group.TotalAccounts)) * 100
}

// countAccountsByCondition counts accounts that satisfy the given condition.
func countAccountsByCondition(accounts map[int64]*AccountAvailability, condition func(*AccountAvailability) bool) int64 {
	if len(accounts) == 0 || condition == nil {
		return 0
	}
	var count int64
	for _, account := range accounts {
		if account != nil && condition(account) {
			count++
		}
	}
	return count
}
