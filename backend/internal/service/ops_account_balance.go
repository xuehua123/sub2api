package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
)

const (
	accountBalanceNewAPIQuotaUnitPerUSD        = 500000.0
	accountBalancePersistTimeout               = 5 * time.Second
	accountBalanceDefaultTimeout               = 8 * time.Second
	accountBalanceMaxResponseBytes             = 64 * 1024
	accountBalanceLegacyDefaultIntervalMinutes = 60
)

func defaultOpsAccountBalanceSettings() OpsAccountBalanceSettings {
	return OpsAccountBalanceSettings{
		Enabled: true,
		Probe: OpsAccountBalanceProbeSettings{
			IntervalMinutes: 5,
			MaxPerRun:       2,
			TimeoutSeconds:  int(accountBalanceDefaultTimeout.Seconds()),
			OnlySchedulable: true,
			MethodOrder: []string{
				AccountBalanceProbeMethodSub2APIUsage,
				AccountBalanceProbeMethodUpstreamManagement,
				AccountBalanceProbeMethodOpenAIBilling,
			},
		},
		Notification: OpsAccountBalanceNotificationSettings{
			EnterpriseWeChatEnabled:    false,
			EnterpriseWeChatWebhookURL: "",
			MentionAllOnLowBalance:     false,
		},
		DefaultThresholdUSD: 10,
		RateLimitPerHour:    12,
	}
}

func normalizeOpsAccountBalanceSettings(settings *OpsAccountBalanceSettings) {
	if settings == nil {
		return
	}
	defaults := defaultOpsAccountBalanceSettings()
	if isZeroOpsAccountBalanceSettings(*settings) {
		*settings = defaults
		return
	}
	if settings.Probe.IntervalMinutes <= 0 || settings.Probe.IntervalMinutes == accountBalanceLegacyDefaultIntervalMinutes {
		settings.Probe.IntervalMinutes = defaults.Probe.IntervalMinutes
	}
	if settings.Probe.IntervalMinutes > 1440 {
		settings.Probe.IntervalMinutes = 1440
	}
	if settings.Probe.MaxPerRun <= 0 {
		settings.Probe.MaxPerRun = defaults.Probe.MaxPerRun
	}
	if settings.Probe.MaxPerRun > 20 {
		settings.Probe.MaxPerRun = 20
	}
	if settings.Probe.TimeoutSeconds <= 0 {
		settings.Probe.TimeoutSeconds = defaults.Probe.TimeoutSeconds
	}
	if settings.Probe.TimeoutSeconds > 60 {
		settings.Probe.TimeoutSeconds = 60
	}
	hadRetiredNewAPITokenUsage := false
	for _, method := range settings.Probe.MethodOrder {
		if strings.EqualFold(strings.TrimSpace(method), AccountBalanceProbeMethodNewAPITokenUsage) {
			hadRetiredNewAPITokenUsage = true
			break
		}
	}
	settings.Probe.MethodOrder = accountBalanceAccountMethodsOnly(normalizeAccountBalanceMethodOrder(settings.Probe.MethodOrder))
	if hadRetiredNewAPITokenUsage {
		settings.Probe.MethodOrder = append(
			[]string{AccountBalanceProbeMethodSub2APIUsage, AccountBalanceProbeMethodUpstreamManagement},
			settings.Probe.MethodOrder...,
		)
		settings.Probe.MethodOrder = normalizeAccountBalanceMethodOrder(settings.Probe.MethodOrder)
	}
	if len(settings.Probe.MethodOrder) == 0 {
		settings.Probe.MethodOrder = defaults.Probe.MethodOrder
	}
	if settings.DefaultThresholdUSD < 0 {
		settings.DefaultThresholdUSD = defaults.DefaultThresholdUSD
	}
	if settings.RateLimitPerHour < 0 {
		settings.RateLimitPerHour = defaults.RateLimitPerHour
	}
	if settings.RateLimitPerHour > 1000 {
		settings.RateLimitPerHour = 1000
	}
	settings.Notification.EnterpriseWeChatWebhookURL = strings.TrimSpace(settings.Notification.EnterpriseWeChatWebhookURL)
}

func isZeroOpsAccountBalanceSettings(settings OpsAccountBalanceSettings) bool {
	return !settings.Enabled &&
		settings.Probe.IntervalMinutes == 0 &&
		settings.Probe.MaxPerRun == 0 &&
		settings.Probe.TimeoutSeconds == 0 &&
		!settings.Probe.OnlySchedulable &&
		len(settings.Probe.MethodOrder) == 0 &&
		!settings.Notification.EnterpriseWeChatEnabled &&
		strings.TrimSpace(settings.Notification.EnterpriseWeChatWebhookURL) == "" &&
		!settings.Notification.MentionAllOnLowBalance &&
		settings.DefaultThresholdUSD == 0 &&
		settings.RateLimitPerHour == 0
}

func validateOpsAccountBalanceSettings(settings OpsAccountBalanceSettings) error {
	if settings.Probe.IntervalMinutes <= 0 || settings.Probe.IntervalMinutes > 1440 {
		return fmt.Errorf("account_balance.probe.interval_minutes must be between 1 and 1440")
	}
	if settings.Probe.MaxPerRun <= 0 || settings.Probe.MaxPerRun > 20 {
		return fmt.Errorf("account_balance.probe.max_per_run must be between 1 and 20")
	}
	if settings.Probe.TimeoutSeconds <= 0 || settings.Probe.TimeoutSeconds > 60 {
		return fmt.Errorf("account_balance.probe.timeout_seconds must be between 1 and 60")
	}
	for _, method := range settings.Probe.MethodOrder {
		if !isSupportedAccountBalanceProbeMethod(method) || method == AccountBalanceProbeMethodAuto || method == AccountBalanceProbeMethodDisabled {
			return fmt.Errorf("account_balance.probe.method_order contains invalid method: %s", method)
		}
	}
	if settings.DefaultThresholdUSD < 0 {
		return fmt.Errorf("account_balance.default_threshold_usd must be >= 0")
	}
	if settings.RateLimitPerHour < 0 || settings.RateLimitPerHour > 1000 {
		return fmt.Errorf("account_balance.rate_limit_per_hour must be between 0 and 1000")
	}
	if settings.Notification.EnterpriseWeChatEnabled {
		webhookURL := strings.TrimSpace(settings.Notification.EnterpriseWeChatWebhookURL)
		if webhookURL == "" || isOpsWebhookMasked(webhookURL) {
			return fmt.Errorf("account_balance.notification.enterprise_wechat_webhook_url is required")
		}
		if err := validateOpsEnterpriseWeChatWebhookURL(webhookURL); err != nil {
			return err
		}
	}
	return nil
}

func maskOpsAccountBalanceSettings(settings OpsAccountBalanceSettings) OpsAccountBalanceSettings {
	if strings.TrimSpace(settings.Notification.EnterpriseWeChatWebhookURL) != "" {
		settings.Notification.EnterpriseWeChatWebhookURL = opsEnterpriseWeChatWebhookMask
	}
	return settings
}

func preserveMaskedOpsAccountBalanceWebhook(next *OpsAccountBalanceSettings, existing OpsAccountBalanceSettings) {
	if next == nil {
		return
	}
	incoming := strings.TrimSpace(next.Notification.EnterpriseWeChatWebhookURL)
	if !isOpsWebhookMasked(incoming) {
		return
	}
	existingURL := strings.TrimSpace(existing.Notification.EnterpriseWeChatWebhookURL)
	if existingURL != "" {
		next.Notification.EnterpriseWeChatWebhookURL = existingURL
	}
}

func (s *OpsService) UpdateOpsAccountBalanceSettings(ctx context.Context, settings OpsAccountBalanceSettings) (OpsAccountBalanceSettings, error) {
	if s == nil || s.settingRepo == nil {
		return OpsAccountBalanceSettings{}, errors.New("setting repository not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	cfg, err := s.GetOpsAlertRuntimeSettings(ctx)
	if err != nil {
		return OpsAccountBalanceSettings{}, err
	}
	if cfg == nil {
		cfg = defaultOpsAlertRuntimeSettings()
	}

	preserveMaskedOpsAccountBalanceWebhook(&settings, cfg.AccountBalance)
	normalizeOpsAccountBalanceSettings(&settings)
	if err := validateOpsAccountBalanceSettings(settings); err != nil {
		return OpsAccountBalanceSettings{}, err
	}

	cfg.AccountBalance = settings
	if _, err := s.storeOpsAlertRuntimeSettings(ctx, cfg); err != nil {
		return OpsAccountBalanceSettings{}, err
	}
	return settings, nil
}

func (s *OpsService) TestOpsAccountBalanceEnterpriseWeChat(ctx context.Context, candidate *OpsAccountBalanceSettings) error {
	if s == nil || s.settingRepo == nil {
		return errors.New("setting repository not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	cfg, err := s.GetOpsAlertRuntimeSettings(ctx)
	if err != nil {
		return err
	}
	if cfg == nil {
		cfg = defaultOpsAlertRuntimeSettings()
	}

	settings := cfg.AccountBalance
	if candidate != nil {
		settings = *candidate
		preserveMaskedOpsAccountBalanceWebhook(&settings, cfg.AccountBalance)
	}
	normalizeOpsAccountBalanceSettings(&settings)

	if !settings.Notification.EnterpriseWeChatEnabled {
		return errors.New("account balance enterprise wechat notification is disabled")
	}
	webhookURL := strings.TrimSpace(settings.Notification.EnterpriseWeChatWebhookURL)
	if webhookURL == "" || isOpsWebhookMasked(webhookURL) {
		return errors.New("account balance enterprise wechat webhook is not configured")
	}
	if err := validateOpsEnterpriseWeChatWebhookURL(webhookURL); err != nil {
		return err
	}
	return sendOpsEnterpriseWeChatMarkdown(ctx, webhookURL, buildOpsAccountBalanceTestWeComMarkdown(time.Now().UTC()), false)
}

func (s *OpsService) ListAccountBalanceMonitor(ctx context.Context, filter OpsAccountBalanceMonitorFilter) (*OpsAccountBalanceListResponse, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	if filter.PageSize > 200 {
		filter.PageSize = 200
	}
	filter.Platform = strings.TrimSpace(filter.Platform)
	filter.Status = strings.TrimSpace(filter.Status)
	filter.ProbeStatus = strings.TrimSpace(filter.ProbeStatus)
	filter.Search = strings.ToLower(strings.TrimSpace(filter.Search))
	filter.Method = normalizeAccountBalanceProbeMethod(filter.Method)
	filter.SortBy = normalizeAccountBalanceSortBy(filter.SortBy)
	filter.SortOrder = normalizeAccountBalanceSortOrder(filter.SortOrder)

	settings := defaultOpsAccountBalanceSettings()
	if cfg, err := s.GetOpsAlertRuntimeSettings(ctx); err == nil && cfg != nil {
		settings = cfg.AccountBalance
	}
	normalizeOpsAccountBalanceSettings(&settings)

	accounts, err := s.listAllAccountsForOps(ctx, filter.Platform, nil)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	allItems := make([]OpsAccountBalanceAccountItem, 0, len(accounts))
	summary := OpsAccountBalanceSummary{}
	for _, account := range accounts {
		state := AccountBalanceStateFromAccount(&account)
		item := OpsAccountBalanceAccountItem{
			AccountID:    account.ID,
			AccountName:  account.Name,
			Platform:     account.Platform,
			Type:         account.Type,
			Status:       account.Status,
			Schedulable:  account.Schedulable,
			GroupIDs:     account.GroupIDs,
			BalanceProbe: state,
		}
		summary.TotalAccounts++
		if state.BalanceUSD != nil || state.BalanceAmount != nil || state.Unlimited {
			summary.KnownBalanceCount++
		}
		if state.Unlimited {
			summary.UnlimitedCount++
		}
		if !state.Enabled || state.Method == AccountBalanceProbeMethodDisabled {
			summary.DisabledCount++
		}
		if state.Status == AccountBalanceProbeStatusFailed {
			summary.FailedCount++
		}
		if state.Status == AccountBalanceProbeStatusUnsupported {
			summary.UnsupportedCount++
		}
		if accountBalanceStateIsDue(state, settings, now) {
			summary.DueCount++
		}
		if accountBalanceStateIsLow(state, settings) {
			summary.LowBalanceCount++
		}
		if !matchesAccountBalanceFilter(item, filter, settings, now) {
			continue
		}
		allItems = append(allItems, item)
	}
	sortAccountBalanceMonitorItems(allItems, filter.SortBy, filter.SortOrder)

	total := int64(len(allItems))
	start := (filter.Page - 1) * filter.PageSize
	if start > len(allItems) {
		start = len(allItems)
	}
	end := start + filter.PageSize
	if end > len(allItems) {
		end = len(allItems)
	}
	items := allItems[start:end]

	return &OpsAccountBalanceListResponse{
		GeneratedAt: now,
		Settings:    maskOpsAccountBalanceSettings(settings),
		Items:       items,
		Total:       total,
		Page:        filter.Page,
		PageSize:    filter.PageSize,
		Summary:     summary,
	}, nil
}

func matchesAccountBalanceFilter(item OpsAccountBalanceAccountItem, filter OpsAccountBalanceMonitorFilter, settings OpsAccountBalanceSettings, now time.Time) bool {
	if filter.Status != "" && !strings.EqualFold(item.Status, filter.Status) {
		return false
	}
	if filter.ProbeStatus != "" && !strings.EqualFold(item.BalanceProbe.Status, filter.ProbeStatus) {
		return false
	}
	if filter.Method != "" && filter.Method != item.BalanceProbe.Method && filter.Method != item.BalanceProbe.DetectedMethod {
		return false
	}
	if filter.Search != "" {
		haystack := strings.ToLower(strings.Join([]string{
			item.AccountName,
			item.Platform,
			item.Type,
			item.BalanceProbe.Status,
			item.BalanceProbe.DetectedMethod,
			item.BalanceProbe.Error,
		}, " "))
		if !strings.Contains(haystack, filter.Search) {
			return false
		}
	}
	if filter.OnlyDue && !accountBalanceStateIsDue(item.BalanceProbe, settings, now) {
		return false
	}
	if filter.OnlyLow && !accountBalanceStateIsLow(item.BalanceProbe, settings) {
		return false
	}
	if filter.OnlyFailed && item.BalanceProbe.Status != AccountBalanceProbeStatusFailed && item.BalanceProbe.Status != AccountBalanceProbeStatusUnsupported {
		return false
	}
	if filter.OnlySchedulable && !item.Schedulable {
		return false
	}
	return true
}

func normalizeAccountBalanceSortBy(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "id", "account_id":
		return "account_id"
	case "account", "name", "account_name":
		return "account_name"
	case "platform":
		return "platform"
	case "type":
		return "type"
	case "account_status":
		return "account_status"
	case "status", "probe_status":
		return "probe_status"
	case "method":
		return "method"
	case "detected_method":
		return "detected_method"
	case "balance", "balance_usd":
		return "balance_usd"
	case "threshold", "threshold_usd":
		return "threshold_usd"
	case "checked_at", "last_checked_at":
		return "checked_at"
	case "schedulable":
		return "schedulable"
	default:
		return "account_name"
	}
}

func normalizeAccountBalanceSortOrder(raw string) string {
	if strings.EqualFold(strings.TrimSpace(raw), "desc") {
		return "desc"
	}
	return "asc"
}

func sortAccountBalanceMonitorItems(items []OpsAccountBalanceAccountItem, sortBy string, sortOrder string) {
	sortBy = normalizeAccountBalanceSortBy(sortBy)
	desc := normalizeAccountBalanceSortOrder(sortOrder) == "desc"
	sort.SliceStable(items, func(i, j int) bool {
		cmp := compareAccountBalanceMonitorItems(items[i], items[j], sortBy, desc)
		if cmp == 0 && sortBy != "platform" {
			cmp = compareStrings(strings.ToLower(items[i].Platform), strings.ToLower(items[j].Platform), false)
		}
		if cmp == 0 && sortBy != "account_name" {
			cmp = compareStrings(strings.ToLower(items[i].AccountName), strings.ToLower(items[j].AccountName), false)
		}
		if cmp == 0 {
			cmp = compareInts64(items[i].AccountID, items[j].AccountID, false)
		}
		return cmp < 0
	})
}

func compareAccountBalanceMonitorItems(left OpsAccountBalanceAccountItem, right OpsAccountBalanceAccountItem, sortBy string, desc bool) int {
	switch sortBy {
	case "account_id":
		return compareInts64(left.AccountID, right.AccountID, desc)
	case "account_name":
		return compareStrings(strings.ToLower(left.AccountName), strings.ToLower(right.AccountName), desc)
	case "platform":
		return compareStrings(strings.ToLower(left.Platform), strings.ToLower(right.Platform), desc)
	case "type":
		return compareStrings(strings.ToLower(left.Type), strings.ToLower(right.Type), desc)
	case "account_status":
		return compareStrings(strings.ToLower(left.Status), strings.ToLower(right.Status), desc)
	case "probe_status":
		return compareStrings(accountBalanceStatusRank(left.BalanceProbe.Status), accountBalanceStatusRank(right.BalanceProbe.Status), desc)
	case "method":
		return compareStrings(left.BalanceProbe.Method, right.BalanceProbe.Method, desc)
	case "detected_method":
		return compareStrings(left.BalanceProbe.DetectedMethod, right.BalanceProbe.DetectedMethod, desc)
	case "balance_usd":
		return compareOptionalFloat(accountBalanceSortableBalance(left.BalanceProbe), accountBalanceSortableBalance(right.BalanceProbe), desc)
	case "threshold_usd":
		return compareOptionalFloat(left.BalanceProbe.ThresholdUSD, right.BalanceProbe.ThresholdUSD, desc)
	case "checked_at":
		return compareOptionalTime(left.BalanceProbe.CheckedAt, right.BalanceProbe.CheckedAt, desc)
	case "schedulable":
		return compareBools(left.Schedulable, right.Schedulable, desc)
	default:
		return compareStrings(strings.ToLower(left.AccountName), strings.ToLower(right.AccountName), desc)
	}
}

func accountBalanceStatusRank(status string) string {
	switch status {
	case AccountBalanceProbeStatusOK:
		return "1_ok"
	case AccountBalanceProbeStatusFailed:
		return "2_failed"
	case AccountBalanceProbeStatusUnsupported:
		return "3_unsupported"
	case AccountBalanceProbeStatusSkipped:
		return "4_skipped"
	default:
		return "5_unknown"
	}
}

func accountBalanceSortableBalance(state OpsAccountBalanceState) *float64 {
	if state.BalanceUSD != nil {
		return state.BalanceUSD
	}
	if state.Unlimited {
		value := math.Inf(1)
		return &value
	}
	return state.BalanceUSD
}

func compareStrings(left string, right string, desc bool) int {
	cmp := strings.Compare(left, right)
	if desc {
		return -cmp
	}
	return cmp
}

func compareInts64(left int64, right int64, desc bool) int {
	cmp := 0
	if left < right {
		cmp = -1
	} else if left > right {
		cmp = 1
	}
	if desc {
		return -cmp
	}
	return cmp
}

func compareBools(left bool, right bool, desc bool) int {
	cmp := 0
	if !left && right {
		cmp = -1
	} else if left && !right {
		cmp = 1
	}
	if desc {
		return -cmp
	}
	return cmp
}

func compareOptionalFloat(left *float64, right *float64, desc bool) int {
	if left == nil && right == nil {
		return 0
	}
	if left == nil {
		return 1
	}
	if right == nil {
		return -1
	}
	cmp := 0
	if *left < *right {
		cmp = -1
	} else if *left > *right {
		cmp = 1
	}
	if desc {
		return -cmp
	}
	return cmp
}

func compareOptionalTime(left *time.Time, right *time.Time, desc bool) int {
	if left == nil && right == nil {
		return 0
	}
	if left == nil {
		return 1
	}
	if right == nil {
		return -1
	}
	cmp := 0
	if left.Before(*right) {
		cmp = -1
	} else if left.After(*right) {
		cmp = 1
	}
	if desc {
		return -cmp
	}
	return cmp
}

func (s *OpsService) UpdateAccountBalanceProbeConfig(ctx context.Context, accountID int64, req OpsAccountBalanceProbeConfigUpdate) (OpsAccountBalanceState, error) {
	if s == nil || s.accountRepo == nil {
		return OpsAccountBalanceState{}, fmt.Errorf("account repository is not available")
	}
	if accountID <= 0 {
		return OpsAccountBalanceState{}, fmt.Errorf("account id must be positive")
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return OpsAccountBalanceState{}, err
	}
	if account == nil {
		return OpsAccountBalanceState{}, ErrAccountNotFound
	}

	updates := map[string]any{}
	if req.Method != nil {
		method := normalizeAccountBalanceProbeMethod(*req.Method)
		if !isSupportedAccountBalanceProbeMethod(method) {
			return OpsAccountBalanceState{}, fmt.Errorf("unsupported balance probe method: %s", *req.Method)
		}
		updates[accountBalanceProbeMethodExtraKey] = method
		if method != AccountBalanceProbeMethodAuto {
			updates[accountBalanceProbeDetectedMethodExtraKey] = ""
		}
	}
	if req.Enabled != nil {
		updates[accountBalanceProbeEnabledExtraKey] = *req.Enabled
	}
	if req.UseDefaultThreshold != nil && *req.UseDefaultThreshold {
		updates[accountBalanceProbeThresholdUSDExtraKey] = nil
	} else if req.ThresholdUSD != nil {
		if *req.ThresholdUSD < 0 {
			return OpsAccountBalanceState{}, fmt.Errorf("threshold_usd must be >= 0")
		}
		updates[accountBalanceProbeThresholdUSDExtraKey] = *req.ThresholdUSD
	}
	if len(updates) == 0 {
		return AccountBalanceStateFromAccount(account), nil
	}
	if err := s.accountRepo.UpdateExtra(ctx, accountID, updates); err != nil {
		return OpsAccountBalanceState{}, err
	}
	if account.Extra == nil {
		account.Extra = map[string]any{}
	}
	for key, value := range updates {
		account.Extra[key] = value
	}
	return AccountBalanceStateFromAccount(account), nil
}

func (s *OpsService) ProbeAccountBalance(ctx context.Context, accountID int64, force bool, methodOverride string) (*OpsAccountBalanceProbeResult, error) {
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

	settings := defaultOpsAccountBalanceSettings()
	if cfg, err := s.GetOpsAlertRuntimeSettings(ctx); err == nil && cfg != nil {
		settings = cfg.AccountBalance
	}
	normalizeOpsAccountBalanceSettings(&settings)
	state := AccountBalanceStateFromAccount(account)
	if !force && !accountBalanceStateIsDue(state, settings, time.Now().UTC()) {
		return &OpsAccountBalanceProbeResult{AccountID: accountID, State: state}, nil
	}

	methods := accountBalanceProbeMethodsForAccount(state, settings, methodOverride)
	if len(methods) == 0 {
		updatedState := s.persistAccountBalanceSkipped(ctx, account, "no probe method configured")
		return &OpsAccountBalanceProbeResult{AccountID: accountID, State: updatedState}, nil
	}

	timeout := time.Duration(settings.Probe.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = accountBalanceDefaultTimeout
	}
	if timeout > 60*time.Second {
		timeout = 60 * time.Second
	}
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	attempts := make([]OpsAccountBalanceProbeAttempt, 0, len(methods))
	var lastErr error
	for _, method := range methods {
		result, attempt, err := s.tryProbeAccountBalanceMethod(probeCtx, account, method, timeout)
		attempts = append(attempts, attempt)
		if err != nil {
			lastErr = err
			continue
		}
		updatedState := s.persistAccountBalanceSuccess(ctx, account, result)
		return &OpsAccountBalanceProbeResult{
			AccountID: accountID,
			State:     updatedState,
			Attempts:  attempts,
		}, nil
	}

	message := "balance probe failed"
	if lastErr != nil {
		message = lastErr.Error()
	}
	updatedState := s.persistAccountBalanceFailure(ctx, account, message, attempts)
	return &OpsAccountBalanceProbeResult{AccountID: accountID, State: updatedState, Attempts: attempts}, nil
}

func (s *OpsService) RunAccountBalanceMonitorCycle(ctx context.Context, settings OpsAccountBalanceSettings) int {
	if s == nil || s.accountRepo == nil {
		return 0
	}
	normalizeOpsAccountBalanceSettings(&settings)
	if !settings.Enabled {
		return 0
	}
	accounts, err := s.listAllAccountsForOps(ctx, "", nil)
	if err != nil {
		return 0
	}
	now := time.Now().UTC()
	probed := 0
	limit := settings.Probe.MaxPerRun
	for _, account := range accounts {
		if limit > 0 && probed >= limit {
			break
		}
		if settings.Probe.OnlySchedulable && !account.Schedulable {
			continue
		}
		if account.Status != StatusActive {
			continue
		}
		state := AccountBalanceStateFromAccount(&account)
		if !state.Enabled || state.Method == AccountBalanceProbeMethodDisabled {
			continue
		}
		if !accountBalanceStateIsDue(state, settings, now) {
			continue
		}
		result, err := s.ProbeAccountBalance(ctx, account.ID, true, "")
		if err != nil || result == nil {
			continue
		}
		probed++
		s.maybeSendAccountBalanceNotification(ctx, account, result.State, settings, now)
	}
	return probed
}

func (s *OpsService) maybeSendAccountBalanceNotification(ctx context.Context, account Account, state OpsAccountBalanceState, settings OpsAccountBalanceSettings, now time.Time) {
	if !settings.Notification.EnterpriseWeChatEnabled {
		return
	}
	webhookURL := strings.TrimSpace(settings.Notification.EnterpriseWeChatWebhookURL)
	if webhookURL == "" || isOpsWebhookMasked(webhookURL) {
		return
	}
	if !accountBalanceStateIsLow(state, settings) {
		return
	}
	if !accountBalanceNotificationDue(state, settings, now) {
		return
	}
	content := buildOpsAccountBalanceLowWeComMarkdown(account, state, settings, now)
	if err := sendOpsEnterpriseWeChatMarkdown(ctx, webhookURL, content, settings.Notification.MentionAllOnLowBalance); err != nil {
		return
	}
	persistCtx, cancel := accountBalancePersistContext(ctx)
	defer cancel()
	_ = s.accountRepo.UpdateExtra(persistCtx, account.ID, map[string]any{
		accountBalanceProbeNotifiedAtExtraKey: now.Format(time.RFC3339Nano),
	})
}

func AccountBalanceStateFromAccount(account *Account) OpsAccountBalanceState {
	if account == nil {
		return OpsAccountBalanceState{
			Method:  AccountBalanceProbeMethodAuto,
			Enabled: true,
			Status:  AccountBalanceProbeStatusUnknown,
		}
	}
	rawMethod := strings.TrimSpace(account.getExtraString(accountBalanceProbeMethodExtraKey))
	method := normalizeAccountBalanceProbeMethod(rawMethod)
	if method == "" {
		method = AccountBalanceProbeMethodAuto
	}
	enabled := true
	if account.Extra != nil {
		if _, ok := account.Extra[accountBalanceProbeEnabledExtraKey]; ok {
			enabled = account.getExtraBool(accountBalanceProbeEnabledExtraKey)
		}
	}
	status := strings.TrimSpace(account.getExtraString(accountBalanceProbeStatusExtraKey))
	if status == "" {
		status = AccountBalanceProbeStatusUnknown
	}
	state := OpsAccountBalanceState{
		AccountID: account.ID,
		Method:    method,
		Enabled:   enabled,
		// A configured method may safely default to auto, but a detected method is
		// historical probe output. Keep an empty value empty after a failed probe so
		// the UI never presents a stale or fabricated successful detection.
		DetectedMethod:  strings.TrimSpace(account.getExtraString(accountBalanceProbeDetectedMethodExtraKey)),
		Status:          status,
		Error:           strings.TrimSpace(account.getExtraString(accountBalanceProbeErrorExtraKey)),
		Unlimited:       account.getExtraBool(accountBalanceProbeUnlimitedExtraKey),
		Endpoint:        strings.TrimSpace(account.getExtraString(accountBalanceProbeEndpointExtraKey)),
		TotalUsedUSD:    accountBalanceExtraFloatPtr(account, accountBalanceProbeTotalUsedUSDExtraKey),
		TotalGrantedUSD: accountBalanceExtraFloatPtr(account, accountBalanceProbeGrantedUSDExtraKey),
		BalanceUSD:      accountBalanceExtraFloatPtr(account, accountBalanceProbeBalanceUSDExtraKey),
		BalanceAmount:   accountBalanceExtraFloatPtr(account, accountBalanceProbeBalanceAmountExtraKey),
		BalanceCurrency: strings.ToUpper(strings.TrimSpace(account.getExtraString(accountBalanceProbeBalanceCurrencyExtraKey))),
		ThresholdUSD:    accountBalanceExtraFloatPtr(account, accountBalanceProbeThresholdUSDExtraKey),
	}
	if state.BalanceAmount == nil && state.BalanceUSD != nil {
		state.BalanceAmount = state.BalanceUSD
		if state.BalanceCurrency == "" {
			state.BalanceCurrency = "USD"
		}
	}
	if checkedAt := account.getExtraTime(accountBalanceProbeCheckedAtExtraKey); !checkedAt.IsZero() {
		t := checkedAt.UTC()
		state.CheckedAt = &t
	}
	if notifiedAt := account.getExtraTime(accountBalanceProbeNotifiedAtExtraKey); !notifiedAt.IsZero() {
		t := notifiedAt.UTC()
		state.NotifiedAt = &t
	}
	// Historical snapshots from /api/usage/token described one API key, not the
	// upstream account wallet. Hide them immediately instead of showing a
	// negative cumulative key ledger or an "unlimited account" badge.
	if state.DetectedMethod == AccountBalanceProbeMethodNewAPITokenUsage || rawMethod == AccountBalanceProbeMethodNewAPITokenUsage {
		state.Status = AccountBalanceProbeStatusUnsupported
		state.Error = "NewAPI API key usage is not an upstream account balance; use a supported direct balance endpoint or upstream management credentials"
		state.BalanceUSD = nil
		state.BalanceAmount = nil
		state.BalanceCurrency = ""
		state.Unlimited = false
	}
	return state
}

func accountBalanceExtraFloatPtr(account *Account, key string) *float64 {
	if account == nil || account.Extra == nil {
		return nil
	}
	raw, ok := account.Extra[key]
	if !ok || raw == nil {
		return nil
	}
	v := account.getExtraFloat64(key)
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	return &v
}

func accountBalanceStateIsDue(state OpsAccountBalanceState, settings OpsAccountBalanceSettings, now time.Time) bool {
	if !state.Enabled || state.Method == AccountBalanceProbeMethodDisabled {
		return false
	}
	if state.CheckedAt == nil || state.CheckedAt.IsZero() {
		return true
	}
	interval := time.Duration(settings.Probe.IntervalMinutes) * time.Minute
	if interval <= 0 {
		interval = time.Hour
	}
	return !state.CheckedAt.Add(interval).After(now)
}

func accountBalanceStateIsLow(state OpsAccountBalanceState, settings OpsAccountBalanceSettings) bool {
	if !state.Enabled || state.Unlimited || state.BalanceUSD == nil {
		return false
	}
	threshold := settings.DefaultThresholdUSD
	if state.ThresholdUSD != nil {
		threshold = *state.ThresholdUSD
	}
	if threshold <= 0 {
		return false
	}
	return *state.BalanceUSD <= threshold
}

func accountBalanceNotificationDue(state OpsAccountBalanceState, settings OpsAccountBalanceSettings, now time.Time) bool {
	if state.NotifiedAt == nil || state.NotifiedAt.IsZero() {
		return true
	}
	if settings.RateLimitPerHour <= 0 {
		return false
	}
	cooldown := time.Hour / time.Duration(settings.RateLimitPerHour)
	if cooldown < time.Minute {
		cooldown = time.Minute
	}
	return !state.NotifiedAt.Add(cooldown).After(now)
}

func normalizeAccountBalanceProbeMethod(method string) string {
	method = strings.ToLower(strings.TrimSpace(method))
	switch method {
	case "", "smart":
		return AccountBalanceProbeMethodAuto
	case AccountBalanceProbeMethodNewAPITokenUsage:
		// This endpoint is intentionally retired from account balance probing:
		// it describes an API key, not the upstream user wallet.
		return AccountBalanceProbeMethodAuto
	case AccountBalanceProbeMethodAuto,
		AccountBalanceProbeMethodDisabled,
		AccountBalanceProbeMethodUpstreamManagement,
		AccountBalanceProbeMethodSub2APIUsage,
		AccountBalanceProbeMethodOpenAIBilling:
		return method
	default:
		return method
	}
}

func isSupportedAccountBalanceProbeMethod(method string) bool {
	switch normalizeAccountBalanceProbeMethod(method) {
	case AccountBalanceProbeMethodAuto,
		AccountBalanceProbeMethodDisabled,
		AccountBalanceProbeMethodUpstreamManagement,
		AccountBalanceProbeMethodSub2APIUsage,
		AccountBalanceProbeMethodOpenAIBilling:
		return true
	default:
		return false
	}
}

func normalizeAccountBalanceMethodOrder(methods []string) []string {
	out := make([]string, 0, len(methods))
	seen := map[string]struct{}{}
	for _, raw := range methods {
		method := normalizeAccountBalanceProbeMethod(raw)
		if method == "" || method == AccountBalanceProbeMethodAuto || method == AccountBalanceProbeMethodDisabled {
			continue
		}
		if !isSupportedAccountBalanceProbeMethod(method) {
			continue
		}
		if _, ok := seen[method]; ok {
			continue
		}
		seen[method] = struct{}{}
		out = append(out, method)
	}
	return out
}

// accountBalanceAccountMethodsOnly prevents historical token-level usage
// endpoints from being used by automatic account-wallet monitoring. A token's
// quota is not the upstream user's balance.
func accountBalanceAccountMethodsOnly(methods []string) []string {
	out := make([]string, 0, len(methods))
	for _, method := range methods {
		if method == AccountBalanceProbeMethodNewAPITokenUsage {
			continue
		}
		out = append(out, method)
	}
	return out
}

func accountBalanceProbeMethodsForAccount(state OpsAccountBalanceState, settings OpsAccountBalanceSettings, methodOverride string) []string {
	override := normalizeAccountBalanceProbeMethod(methodOverride)
	if override != "" && override != AccountBalanceProbeMethodAuto {
		if override == AccountBalanceProbeMethodDisabled {
			return nil
		}
		return []string{override}
	}
	method := normalizeAccountBalanceProbeMethod(state.Method)
	if method == AccountBalanceProbeMethodDisabled {
		return nil
	}
	if method != "" && method != AccountBalanceProbeMethodAuto {
		return []string{method}
	}
	order := make([]string, 0, len(settings.Probe.MethodOrder)+1)
	if detected := normalizeAccountBalanceProbeMethod(state.DetectedMethod); detected != "" && detected != AccountBalanceProbeMethodAuto && detected != AccountBalanceProbeMethodDisabled {
		order = append(order, detected)
	}
	order = append(order, settings.Probe.MethodOrder...)
	return accountBalanceAccountMethodsOnly(normalizeAccountBalanceMethodOrder(order))
}

func (s *OpsService) tryProbeAccountBalanceMethod(ctx context.Context, account *Account, method string, timeout time.Duration) (opsAccountBalanceMethodResult, OpsAccountBalanceProbeAttempt, error) {
	method = normalizeAccountBalanceProbeMethod(method)
	if method == AccountBalanceProbeMethodUpstreamManagement {
		attempt := OpsAccountBalanceProbeAttempt{Method: method}
		if s == nil || s.upstreamBalanceFetcher == nil {
			attempt.Status = AccountBalanceProbeStatusUnsupported
			attempt.Message = "upstream management balance probing is unavailable"
			return opsAccountBalanceMethodResult{}, attempt, errors.New(attempt.Message)
		}
		probeCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		result, err := s.upstreamBalanceFetcher.FetchAccountBalance(probeCtx, account)
		attempt.Endpoint = result.Endpoint
		if err != nil {
			if errors.Is(err, ErrUpstreamManagementBalanceUnavailable) {
				attempt.Status = AccountBalanceProbeStatusUnsupported
			} else {
				attempt.Status = AccountBalanceProbeStatusFailed
			}
			attempt.Message = err.Error()
			return opsAccountBalanceMethodResult{}, attempt, err
		}
		attempt.Status = AccountBalanceProbeStatusOK
		return result, attempt, nil
	}
	endpoint, err := s.accountBalanceProbeEndpoint(account, method)
	attempt := OpsAccountBalanceProbeAttempt{Method: method, Endpoint: endpoint}
	if err != nil {
		attempt.Status = AccountBalanceProbeStatusUnsupported
		attempt.Message = err.Error()
		return opsAccountBalanceMethodResult{}, attempt, err
	}
	payload, err := s.fetchAccountBalanceJSON(ctx, account, endpoint, timeout)
	if err != nil {
		attempt.Status = AccountBalanceProbeStatusFailed
		attempt.Message = err.Error()
		return opsAccountBalanceMethodResult{}, attempt, err
	}
	result, err := parseAccountBalanceResult(method, endpoint, payload)
	if err != nil {
		attempt.Status = AccountBalanceProbeStatusUnsupported
		attempt.Message = err.Error()
		return opsAccountBalanceMethodResult{}, attempt, err
	}
	attempt.Status = AccountBalanceProbeStatusOK
	return result, attempt, nil
}

func (s *OpsService) accountBalanceProbeEndpoint(account *Account, method string) (string, error) {
	if account == nil {
		return "", errors.New("account is nil")
	}
	baseURL := accountBalanceBaseURL(account)
	if strings.TrimSpace(baseURL) == "" {
		return "", errors.New("base_url is not configured")
	}
	normalized, err := s.validateAccountBalanceBaseURL(baseURL)
	if err != nil {
		return "", err
	}
	switch method {
	case AccountBalanceProbeMethodUpstreamManagement:
		return "", errors.New("upstream management balance endpoint is resolved from the configured provider")
	case AccountBalanceProbeMethodNewAPITokenUsage:
		return accountBalanceJoinEndpoint(normalized, "/api/usage/token", false), nil
	case AccountBalanceProbeMethodSub2APIUsage:
		return accountBalanceJoinEndpoint(normalized, "/v1/usage", true), nil
	case AccountBalanceProbeMethodOpenAIBilling:
		return accountBalanceJoinEndpoint(normalized, "/v1/dashboard/billing/subscription", true), nil
	default:
		return "", fmt.Errorf("unsupported balance probe method: %s", method)
	}
}

func accountBalanceBaseURL(account *Account) string {
	return accountUpstreamBaseURL(account)
}

func accountBalanceAPIKey(account *Account) string {
	if account == nil {
		return ""
	}
	for _, key := range []string{"api_key", "key", "token", "access_token"} {
		if v := strings.TrimSpace(account.GetCredential(key)); v != "" {
			return v
		}
	}
	return ""
}

func (s *OpsService) validateAccountBalanceBaseURL(raw string) (string, error) {
	if s == nil || s.cfg == nil {
		return urlvalidator.ValidateURLFormat(raw, true)
	}
	if !s.cfg.Security.URLAllowlist.Enabled {
		return urlvalidator.ValidateURLFormat(raw, s.cfg.Security.URLAllowlist.AllowInsecureHTTP)
	}
	return urlvalidator.ValidateHTTPSURL(raw, urlvalidator.ValidationOptions{
		AllowedHosts:     s.cfg.Security.URLAllowlist.UpstreamHosts,
		RequireAllowlist: len(s.cfg.Security.URLAllowlist.UpstreamHosts) > 0,
		AllowPrivate:     s.cfg.Security.URLAllowlist.AllowPrivateHosts,
	})
}

func accountBalanceJoinEndpoint(baseURL string, endpointPath string, keepV1 bool) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	endpointPath = "/" + strings.TrimLeft(endpointPath, "/")
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return baseURL + endpointPath
	}
	path := strings.TrimRight(parsed.Path, "/")
	if !keepV1 && strings.HasSuffix(path, "/v1") {
		path = strings.TrimSuffix(path, "/v1")
	}
	if keepV1 && strings.HasPrefix(endpointPath, "/v1/") && strings.HasSuffix(path, "/v1") {
		endpointPath = strings.TrimPrefix(endpointPath, "/v1")
	}
	parsed.Path = path + endpointPath
	return parsed.String()
}

func (s *OpsService) fetchAccountBalanceJSON(ctx context.Context, account *Account, endpoint string, timeout time.Duration) (map[string]any, error) {
	apiKey := accountBalanceAPIKey(account)
	if apiKey == "" {
		return nil, errors.New("api key is not configured")
	}
	client := &http.Client{Timeout: timeout}
	if account != nil && account.Proxy != nil && account.Proxy.IsActive() {
		if proxyURL, err := url.Parse(account.Proxy.URL()); err == nil {
			client.Transport = &http.Transport{Proxy: http.ProxyURL(proxyURL)}
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("X-API-Key", apiKey)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, accountBalanceMaxResponseBytes))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("upstream returned %d: %s", resp.StatusCode, truncateString(string(body), 240))
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("invalid json response: %w", err)
	}
	return payload, nil
}

func parseAccountBalanceResult(method string, endpoint string, payload map[string]any) (opsAccountBalanceMethodResult, error) {
	switch method {
	case AccountBalanceProbeMethodUpstreamManagement:
		return opsAccountBalanceMethodResult{}, errors.New("upstream management balance requires management authentication")
	case AccountBalanceProbeMethodNewAPITokenUsage:
		return parseNewAPIAccountBalance(endpoint, payload)
	case AccountBalanceProbeMethodSub2APIUsage:
		return parseSub2APIAccountBalance(endpoint, payload)
	case AccountBalanceProbeMethodOpenAIBilling:
		return parseOpenAIBillingAccountBalance(endpoint, payload)
	default:
		return opsAccountBalanceMethodResult{}, fmt.Errorf("unsupported balance probe method: %s", method)
	}
}

func parseNewAPIAccountBalance(endpoint string, payload map[string]any) (opsAccountBalanceMethodResult, error) {
	data := accountBalanceDataObject(payload)
	unlimited := accountBalanceBool(data, "unlimited_quota", "unlimited")
	if unlimited {
		return opsAccountBalanceMethodResult{}, errors.New("NewAPI API key quota is unlimited and cannot represent the upstream account balance")
	}
	totalAvailable := accountBalanceNumber(data, "total_available", "available", "quota_available")
	totalGranted := accountBalanceNumber(data, "total_granted", "quota", "total_quota")
	totalUsed := accountBalanceNumber(data, "total_used", "used", "quota_used")
	var balanceUSD *float64
	if totalAvailable != nil {
		v := *totalAvailable / accountBalanceNewAPIQuotaUnitPerUSD
		balanceUSD = &v
	}
	var grantedUSD *float64
	if totalGranted != nil {
		v := *totalGranted / accountBalanceNewAPIQuotaUnitPerUSD
		grantedUSD = &v
	}
	var usedUSD *float64
	if totalUsed != nil {
		v := *totalUsed / accountBalanceNewAPIQuotaUnitPerUSD
		usedUSD = &v
	}
	if balanceUSD == nil && grantedUSD == nil && usedUSD == nil {
		return opsAccountBalanceMethodResult{}, errors.New("newapi usage response has no recognizable quota fields")
	}
	return opsAccountBalanceMethodResult{
		Method:          AccountBalanceProbeMethodNewAPITokenUsage,
		Endpoint:        endpoint,
		BalanceUSD:      balanceUSD,
		Unlimited:       unlimited,
		TotalUsedUSD:    usedUSD,
		TotalGrantedUSD: grantedUSD,
	}, nil
}

func parseSub2APIAccountBalance(endpoint string, payload map[string]any) (opsAccountBalanceMethodResult, error) {
	data := accountBalanceDataObject(payload)
	balanceUSD := accountBalanceNumber(data, "balance", "remaining", "available", "credit")
	totalUsedUSD := accountBalanceNumber(data, "total_used", "used", "total_usage", "usage")
	totalGrantedUSD := accountBalanceNumber(data, "total_granted", "granted", "total", "quota")
	if balanceUSD == nil {
		if usage, ok := data["usage"].(map[string]any); ok {
			totalUsedUSD = accountBalanceNumber(usage, "cost", "total_cost", "total")
		}
	}
	if balanceUSD == nil && totalUsedUSD == nil && totalGrantedUSD == nil {
		return opsAccountBalanceMethodResult{}, errors.New("sub2api usage response has no recognizable balance fields")
	}
	return opsAccountBalanceMethodResult{
		Method:          AccountBalanceProbeMethodSub2APIUsage,
		Endpoint:        endpoint,
		BalanceUSD:      balanceUSD,
		Unlimited:       accountBalanceBool(data, "unlimited", "unlimited_quota"),
		TotalUsedUSD:    totalUsedUSD,
		TotalGrantedUSD: totalGrantedUSD,
	}, nil
}

func parseOpenAIBillingAccountBalance(endpoint string, payload map[string]any) (opsAccountBalanceMethodResult, error) {
	data := accountBalanceDataObject(payload)
	balanceUSD := accountBalanceNumber(data, "balance", "available")
	hardLimitUSD := accountBalanceNumber(data, "hard_limit_usd", "system_hard_limit_usd")
	usedUSD := accountBalanceNumber(data, "used_usd", "total_used")
	if usedUSD == nil {
		if totalUsage := accountBalanceNumber(data, "total_usage"); totalUsage != nil {
			v := *totalUsage / 100
			usedUSD = &v
		}
	}
	var unlimited bool
	if hardLimitUSD != nil && *hardLimitUSD >= 999999 {
		unlimited = true
	}
	if balanceUSD == nil && hardLimitUSD != nil && usedUSD != nil && !unlimited {
		v := *hardLimitUSD - *usedUSD
		if v < 0 {
			v = 0
		}
		balanceUSD = &v
	}
	if !unlimited && balanceUSD == nil && hardLimitUSD == nil && usedUSD == nil {
		return opsAccountBalanceMethodResult{}, errors.New("openai billing response has no recognizable balance fields")
	}
	return opsAccountBalanceMethodResult{
		Method:          AccountBalanceProbeMethodOpenAIBilling,
		Endpoint:        endpoint,
		BalanceUSD:      balanceUSD,
		Unlimited:       unlimited,
		TotalUsedUSD:    usedUSD,
		TotalGrantedUSD: hardLimitUSD,
	}, nil
}

func accountBalanceDataObject(payload map[string]any) map[string]any {
	if payload == nil {
		return map[string]any{}
	}
	if data, ok := payload["data"].(map[string]any); ok {
		return data
	}
	if data, ok := payload["result"].(map[string]any); ok {
		return data
	}
	return payload
}

func accountBalanceBool(data map[string]any, keys ...string) bool {
	for _, key := range keys {
		raw, ok := data[key]
		if !ok || raw == nil {
			continue
		}
		switch v := raw.(type) {
		case bool:
			return v
		case string:
			switch strings.ToLower(strings.TrimSpace(v)) {
			case "true", "1", "yes", "on":
				return true
			}
		case float64:
			return v != 0
		case json.Number:
			i, _ := v.Int64()
			return i != 0
		}
	}
	return false
}

func accountBalanceNumber(data map[string]any, keys ...string) *float64 {
	for _, key := range keys {
		raw, ok := data[key]
		if !ok || raw == nil {
			continue
		}
		if v, ok := accountBalanceParseNumber(raw); ok {
			return &v
		}
	}
	return nil
}

func accountBalanceParseNumber(raw any) (float64, bool) {
	switch v := raw.(type) {
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return 0, false
		}
		return v, true
	case float32:
		n := float64(v)
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return 0, false
		}
		return n, true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		n, err := v.Float64()
		return n, err == nil
	case string:
		s := strings.TrimSpace(strings.TrimPrefix(v, "$"))
		if s == "" {
			return 0, false
		}
		s = strings.ReplaceAll(s, ",", "")
		n, err := strconv.ParseFloat(s, 64)
		return n, err == nil
	default:
		return 0, false
	}
}

func (s *OpsService) persistAccountBalanceSuccess(ctx context.Context, account *Account, result opsAccountBalanceMethodResult) OpsAccountBalanceState {
	now := time.Now().UTC()
	updates := map[string]any{
		accountBalanceProbeDetectedMethodExtraKey:  result.Method,
		accountBalanceProbeStatusExtraKey:          AccountBalanceProbeStatusOK,
		accountBalanceProbeErrorExtraKey:           "",
		accountBalanceProbeCheckedAtExtraKey:       now.Format(time.RFC3339Nano),
		accountBalanceProbeUnlimitedExtraKey:       result.Unlimited,
		accountBalanceProbeEndpointExtraKey:        result.Endpoint,
		accountBalanceProbeBalanceUSDExtraKey:      nil,
		accountBalanceProbeBalanceAmountExtraKey:   nil,
		accountBalanceProbeBalanceCurrencyExtraKey: nil,
		accountBalanceProbeTotalUsedUSDExtraKey:    nil,
		accountBalanceProbeGrantedUSDExtraKey:      nil,
	}
	if result.BalanceUSD != nil {
		updates[accountBalanceProbeBalanceUSDExtraKey] = *result.BalanceUSD
	}
	if result.BalanceAmount != nil {
		updates[accountBalanceProbeBalanceAmountExtraKey] = *result.BalanceAmount
		updates[accountBalanceProbeBalanceCurrencyExtraKey] = strings.ToUpper(strings.TrimSpace(result.BalanceCurrency))
	}
	if result.TotalUsedUSD != nil {
		updates[accountBalanceProbeTotalUsedUSDExtraKey] = *result.TotalUsedUSD
	}
	if result.TotalGrantedUSD != nil {
		updates[accountBalanceProbeGrantedUSDExtraKey] = *result.TotalGrantedUSD
	}
	return s.persistAccountBalanceState(ctx, account, updates)
}

func (s *OpsService) persistAccountBalanceSkipped(ctx context.Context, account *Account, message string) OpsAccountBalanceState {
	now := time.Now().UTC()
	return s.persistAccountBalanceState(ctx, account, map[string]any{
		accountBalanceProbeStatusExtraKey:    AccountBalanceProbeStatusSkipped,
		accountBalanceProbeErrorExtraKey:     strings.TrimSpace(message),
		accountBalanceProbeCheckedAtExtraKey: now.Format(time.RFC3339Nano),
	})
}

func (s *OpsService) persistAccountBalanceFailure(ctx context.Context, account *Account, message string, attempts []OpsAccountBalanceProbeAttempt) OpsAccountBalanceState {
	now := time.Now().UTC()
	status := AccountBalanceProbeStatusFailed
	if len(attempts) > 0 {
		allUnsupported := true
		for _, attempt := range attempts {
			if attempt.Status != AccountBalanceProbeStatusUnsupported {
				allUnsupported = false
				break
			}
		}
		if allUnsupported {
			status = AccountBalanceProbeStatusUnsupported
		}
	}
	return s.persistAccountBalanceState(ctx, account, map[string]any{
		accountBalanceProbeDetectedMethodExtraKey:  "",
		accountBalanceProbeStatusExtraKey:          status,
		accountBalanceProbeErrorExtraKey:           truncateString(strings.TrimSpace(message), 500),
		accountBalanceProbeCheckedAtExtraKey:       now.Format(time.RFC3339Nano),
		accountBalanceProbeUnlimitedExtraKey:       false,
		accountBalanceProbeEndpointExtraKey:        "",
		accountBalanceProbeBalanceUSDExtraKey:      nil,
		accountBalanceProbeBalanceAmountExtraKey:   nil,
		accountBalanceProbeBalanceCurrencyExtraKey: nil,
		accountBalanceProbeTotalUsedUSDExtraKey:    nil,
		accountBalanceProbeGrantedUSDExtraKey:      nil,
	})
}

func (s *OpsService) persistAccountBalanceState(ctx context.Context, account *Account, updates map[string]any) OpsAccountBalanceState {
	if account == nil {
		return AccountBalanceStateFromAccount(nil)
	}
	persistCtx, cancel := accountBalancePersistContext(ctx)
	defer cancel()
	if s != nil && s.accountRepo != nil {
		_ = s.accountRepo.UpdateExtra(persistCtx, account.ID, updates)
	}
	if account.Extra == nil {
		account.Extra = map[string]any{}
	}
	for key, value := range updates {
		account.Extra[key] = value
	}
	return AccountBalanceStateFromAccount(account)
}

func accountBalancePersistContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(context.WithoutCancel(ctx), accountBalancePersistTimeout)
}

func buildOpsAccountBalanceLowWeComMarkdown(account Account, state OpsAccountBalanceState, settings OpsAccountBalanceSettings, now time.Time) string {
	balance := "未知"
	if state.Unlimited {
		balance = "无限额度"
	} else if state.BalanceUSD != nil {
		balance = fmt.Sprintf("$%.2f", *state.BalanceUSD)
	}
	threshold := settings.DefaultThresholdUSD
	if state.ThresholdUSD != nil {
		threshold = *state.ThresholdUSD
	}
	lines := []string{
		"### 上游账号额度提醒",
		fmt.Sprintf("> 账号：%s", escapeOpsWeComMarkdown(account.Name)),
		fmt.Sprintf("> 平台：%s / %s", escapeOpsWeComMarkdown(account.Platform), escapeOpsWeComMarkdown(account.Type)),
		fmt.Sprintf("> 当前余额：<font color=\"warning\">%s</font>", escapeOpsWeComMarkdown(balance)),
		fmt.Sprintf("> 告警阈值：$%.2f", threshold),
		fmt.Sprintf("> 查询方式：%s", escapeOpsWeComMarkdown(accountBalanceMethodLabel(state.DetectedMethod, state.Method))),
		fmt.Sprintf("> 检查时间：%s", formatOpsNotifyTime(now)),
		"",
		"请及时补充上游余额，避免调度账号突然不可用。",
	}
	return strings.Join(lines, "\n")
}

func buildOpsAccountBalanceTestWeComMarkdown(now time.Time) string {
	return strings.Join([]string{
		"### 上游余额通知测试",
		"> 企业微信机器人已连通。",
		fmt.Sprintf("> 测试时间：%s", formatOpsNotifyTime(now)),
		"",
		"之后账号余额低于阈值时，会按配置发送提醒。",
	}, "\n")
}

func accountBalanceMethodLabel(detected string, method string) string {
	detected = normalizeAccountBalanceProbeMethod(detected)
	method = normalizeAccountBalanceProbeMethod(method)
	if detected != "" && detected != AccountBalanceProbeMethodAuto && detected != AccountBalanceProbeMethodDisabled {
		return detected
	}
	if method == "" {
		return AccountBalanceProbeMethodAuto
	}
	return method
}

var _ = pagination.PaginationParams{}
