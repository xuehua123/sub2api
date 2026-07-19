package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	upstreamConnectionBalancePageSize          = 200
	upstreamConnectionBalanceMinFreshWindow    = 15 * time.Minute
	upstreamConnectionBalanceNotifiedKeyPrefix = "ops_upstream_connection_balance_notified_"
	upstreamConnectionBalanceOverrideKeyPrefix = "ops_upstream_connection_balance_config_"
)

type upstreamConnectionBalanceSource interface {
	List(ctx context.Context, params UpstreamConnectionListParams) ([]*UpstreamConnection, int64, error)
	Get(ctx context.Context, id int64) (*UpstreamConnection, error)
	Probe(ctx context.Context, id int64) (*UpstreamConnection, error)
}

func defaultOpsUpstreamConnectionBalanceSettings() OpsUpstreamConnectionBalanceSettings {
	return OpsUpstreamConnectionBalanceSettings{
		Enabled:             true,
		DefaultThresholdUSD: 10,
		RateLimitPerHour:    12,
		Notification: OpsUpstreamConnectionBalanceNotificationSettings{
			EnterpriseWeChatEnabled:    false,
			EnterpriseWeChatWebhookURL: "",
			MentionAllOnLowBalance:     false,
		},
	}
}

func normalizeOpsUpstreamConnectionBalanceSettings(settings *OpsUpstreamConnectionBalanceSettings) {
	if settings == nil {
		return
	}
	defaults := defaultOpsUpstreamConnectionBalanceSettings()
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

func validateOpsUpstreamConnectionBalanceSettings(settings OpsUpstreamConnectionBalanceSettings) error {
	if settings.DefaultThresholdUSD < 0 {
		return errors.New("upstream_connection_balance.default_threshold_usd must be >= 0")
	}
	if settings.RateLimitPerHour < 0 || settings.RateLimitPerHour > 1000 {
		return errors.New("upstream_connection_balance.rate_limit_per_hour must be between 0 and 1000")
	}
	if settings.Notification.EnterpriseWeChatEnabled {
		webhookURL := strings.TrimSpace(settings.Notification.EnterpriseWeChatWebhookURL)
		if webhookURL == "" || isOpsWebhookMasked(webhookURL) {
			return errors.New("upstream_connection_balance.notification.enterprise_wechat_webhook_url is required")
		}
		if err := validateOpsEnterpriseWeChatWebhookURL(webhookURL); err != nil {
			return err
		}
	}
	return nil
}

func maskOpsUpstreamConnectionBalanceSettings(settings OpsUpstreamConnectionBalanceSettings) OpsUpstreamConnectionBalanceSettings {
	if strings.TrimSpace(settings.Notification.EnterpriseWeChatWebhookURL) != "" {
		settings.Notification.EnterpriseWeChatWebhookURL = opsEnterpriseWeChatWebhookMask
	}
	return settings
}

func preserveMaskedOpsUpstreamConnectionBalanceWebhook(next *OpsUpstreamConnectionBalanceSettings, existing OpsUpstreamConnectionBalanceSettings) {
	if next == nil || !isOpsWebhookMasked(strings.TrimSpace(next.Notification.EnterpriseWeChatWebhookURL)) {
		return
	}
	if existingURL := strings.TrimSpace(existing.Notification.EnterpriseWeChatWebhookURL); existingURL != "" {
		next.Notification.EnterpriseWeChatWebhookURL = existingURL
	}
}

func (s *OpsService) UpdateOpsUpstreamConnectionBalanceSettings(ctx context.Context, settings OpsUpstreamConnectionBalanceSettings) (OpsUpstreamConnectionBalanceSettings, error) {
	if s == nil || s.settingRepo == nil {
		return OpsUpstreamConnectionBalanceSettings{}, errors.New("setting repository not initialized")
	}
	existing, err := s.opsUpstreamConnectionBalanceSettings(ctx)
	if err != nil {
		return OpsUpstreamConnectionBalanceSettings{}, err
	}
	preserveMaskedOpsUpstreamConnectionBalanceWebhook(&settings, existing)
	normalizeOpsUpstreamConnectionBalanceSettings(&settings)
	if err := validateOpsUpstreamConnectionBalanceSettings(settings); err != nil {
		return OpsUpstreamConnectionBalanceSettings{}, err
	}
	raw, err := json.Marshal(settings)
	if err != nil {
		return OpsUpstreamConnectionBalanceSettings{}, err
	}
	if err := s.settingRepo.Set(ctx, SettingKeyOpsUpstreamConnectionBalance, string(raw)); err != nil {
		return OpsUpstreamConnectionBalanceSettings{}, err
	}
	return settings, nil
}

func (s *OpsService) TestOpsUpstreamConnectionBalanceEnterpriseWeChat(ctx context.Context, candidate *OpsUpstreamConnectionBalanceSettings) error {
	if s == nil || s.settingRepo == nil {
		return errors.New("setting repository not initialized")
	}
	settings, err := s.opsUpstreamConnectionBalanceSettings(ctx)
	if err != nil {
		return err
	}
	if candidate != nil {
		next := *candidate
		preserveMaskedOpsUpstreamConnectionBalanceWebhook(&next, settings)
		settings = next
	}
	normalizeOpsUpstreamConnectionBalanceSettings(&settings)
	if !settings.Notification.EnterpriseWeChatEnabled {
		return errors.New("upstream connection balance enterprise wechat notification is disabled")
	}
	webhookURL := strings.TrimSpace(settings.Notification.EnterpriseWeChatWebhookURL)
	if webhookURL == "" || isOpsWebhookMasked(webhookURL) {
		return errors.New("upstream connection balance enterprise wechat webhook is not configured")
	}
	if err := validateOpsEnterpriseWeChatWebhookURL(webhookURL); err != nil {
		return err
	}
	return sendOpsEnterpriseWeChatMarkdown(ctx, webhookURL, buildOpsUpstreamConnectionBalanceTestWeComMarkdown(time.Now().UTC()), false)
}

func (s *OpsService) ListUpstreamConnectionBalanceMonitor(ctx context.Context, filter OpsUpstreamConnectionBalanceMonitorFilter) (*OpsUpstreamConnectionBalanceListResponse, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s == nil || s.upstreamConnectionBalance == nil {
		return nil, errors.New("upstream connection service is not available")
	}
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	if filter.PageSize > upstreamConnectionBalancePageSize {
		filter.PageSize = upstreamConnectionBalancePageSize
	}
	filter.Search = strings.ToLower(strings.TrimSpace(filter.Search))
	filter.Status = strings.ToLower(strings.TrimSpace(filter.Status))

	settings, err := s.opsUpstreamConnectionBalanceSettings(ctx)
	if err != nil {
		return nil, err
	}
	connections, err := s.listAllUpstreamConnectionsForBalance(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	notifiedAt, err := s.loadUpstreamConnectionBalanceNotifiedAt(ctx, connections)
	if err != nil {
		return nil, err
	}
	overrides, err := s.loadUpstreamConnectionBalanceOverrides(ctx, connections)
	if err != nil {
		return nil, err
	}
	items := make([]OpsUpstreamConnectionBalanceItem, 0, len(connections))
	summary := OpsUpstreamConnectionBalanceSummary{}
	for _, connection := range connections {
		if connection == nil {
			continue
		}
		item := buildOpsUpstreamConnectionBalanceItem(connection, settings, upstreamConnectionBalanceOverride(overrides, connection.ID), notifiedAt[connection.ID], now)
		summary.TotalConnections++
		if settings.Enabled && item.Alert.Enabled && item.Alert.Eligible {
			summary.MonitoredConnections++
		}
		if item.Alert.Low {
			summary.LowBalanceConnections++
		}
		if upstreamConnectionBalanceFailed(connection) {
			summary.FailedConnections++
		}
		if item.Alert.Eligible && !item.Alert.SnapshotFresh {
			summary.StaleConnections++
		}
		if connection.WalletUnlimited {
			summary.UnlimitedConnections++
		}
		if matchesUpstreamConnectionBalanceFilter(item, filter) {
			items = append(items, item)
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Alert.Low != items[j].Alert.Low {
			return items[i].Alert.Low
		}
		if upstreamConnectionBalanceFailedItem(items[i]) != upstreamConnectionBalanceFailedItem(items[j]) {
			return upstreamConnectionBalanceFailedItem(items[i])
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	total := int64(len(items))
	start := (filter.Page - 1) * filter.PageSize
	if start > len(items) {
		start = len(items)
	}
	end := start + filter.PageSize
	if end > len(items) {
		end = len(items)
	}
	return &OpsUpstreamConnectionBalanceListResponse{
		GeneratedAt: now,
		Settings:    maskOpsUpstreamConnectionBalanceSettings(settings),
		Items:       items[start:end],
		Total:       total,
		Page:        filter.Page,
		PageSize:    filter.PageSize,
		Summary:     summary,
	}, nil
}

func (s *OpsService) UpdateUpstreamConnectionBalanceAlert(ctx context.Context, connectionID int64, request OpsUpstreamConnectionBalanceAlertUpdate) (OpsUpstreamConnectionBalanceAlertState, error) {
	if s == nil || s.upstreamConnectionBalance == nil {
		return OpsUpstreamConnectionBalanceAlertState{}, errors.New("upstream connection service is not available")
	}
	if s.settingRepo == nil {
		return OpsUpstreamConnectionBalanceAlertState{}, errors.New("setting repository not initialized")
	}
	if connectionID <= 0 {
		return OpsUpstreamConnectionBalanceAlertState{}, errors.New("connection id must be positive")
	}
	connection, err := s.upstreamConnectionBalance.Get(ctx, connectionID)
	if err != nil {
		return OpsUpstreamConnectionBalanceAlertState{}, err
	}
	notifiedAt, err := s.loadUpstreamConnectionBalanceNotifiedAt(ctx, []*UpstreamConnection{connection})
	if err != nil {
		return OpsUpstreamConnectionBalanceAlertState{}, err
	}
	settings, err := s.opsUpstreamConnectionBalanceSettings(ctx)
	if err != nil {
		return OpsUpstreamConnectionBalanceAlertState{}, err
	}
	overrides, err := s.loadUpstreamConnectionBalanceOverrides(ctx, []*UpstreamConnection{connection})
	if err != nil {
		return OpsUpstreamConnectionBalanceAlertState{}, err
	}
	override, exists := overrides[connectionID]
	if !exists {
		override.Enabled = true
	}
	if request.Enabled != nil {
		override.Enabled = *request.Enabled
	}
	if request.UseDefaultThreshold != nil && *request.UseDefaultThreshold {
		override.ThresholdUSD = nil
	} else if request.ThresholdUSD != nil {
		if *request.ThresholdUSD < 0 {
			return OpsUpstreamConnectionBalanceAlertState{}, errors.New("threshold_usd must be >= 0")
		}
		value := *request.ThresholdUSD
		override.ThresholdUSD = &value
	}
	if override.Enabled && override.ThresholdUSD == nil {
		if err := s.settingRepo.Delete(ctx, upstreamConnectionBalanceOverrideKey(connectionID)); err != nil && !errors.Is(err, ErrSettingNotFound) {
			return OpsUpstreamConnectionBalanceAlertState{}, err
		}
		override = OpsUpstreamConnectionBalanceOverride{Enabled: true}
	} else {
		raw, err := json.Marshal(override)
		if err != nil {
			return OpsUpstreamConnectionBalanceAlertState{}, err
		}
		if err := s.settingRepo.Set(ctx, upstreamConnectionBalanceOverrideKey(connectionID), string(raw)); err != nil {
			return OpsUpstreamConnectionBalanceAlertState{}, err
		}
	}
	return buildOpsUpstreamConnectionBalanceItem(connection, settings, &override, notifiedAt[connectionID], time.Now().UTC()).Alert, nil
}

func (s *OpsService) ProbeUpstreamConnectionBalance(ctx context.Context, connectionID int64) (*OpsUpstreamConnectionBalanceItem, error) {
	if s == nil || s.upstreamConnectionBalance == nil {
		return nil, errors.New("upstream connection service is not available")
	}
	if connectionID <= 0 {
		return nil, errors.New("connection id must be positive")
	}
	notifiedAt, err := s.loadUpstreamConnectionBalanceNotifiedAt(ctx, []*UpstreamConnection{{ID: connectionID}})
	if err != nil {
		return nil, err
	}
	connection, err := s.upstreamConnectionBalance.Probe(ctx, connectionID)
	if err != nil {
		return nil, err
	}
	settings, err := s.opsUpstreamConnectionBalanceSettings(ctx)
	if err != nil {
		return nil, err
	}
	overrides, err := s.loadUpstreamConnectionBalanceOverrides(ctx, []*UpstreamConnection{connection})
	if err != nil {
		return nil, err
	}
	item := buildOpsUpstreamConnectionBalanceItem(connection, settings, upstreamConnectionBalanceOverride(overrides, connectionID), notifiedAt[connectionID], time.Now().UTC())
	return &item, nil
}

func (s *OpsService) RunUpstreamConnectionBalanceMonitorCycle(ctx context.Context, settings OpsUpstreamConnectionBalanceSettings) int {
	if s == nil || s.upstreamConnectionBalance == nil || s.settingRepo == nil {
		return 0
	}
	normalizeOpsUpstreamConnectionBalanceSettings(&settings)
	if !settings.Enabled {
		return 0
	}
	connections, err := s.listAllUpstreamConnectionsForBalance(ctx)
	if err != nil {
		return 0
	}
	now := time.Now().UTC()
	notifiedAt, err := s.loadUpstreamConnectionBalanceNotifiedAt(ctx, connections)
	if err != nil {
		return 0
	}
	overrides, err := s.loadUpstreamConnectionBalanceOverrides(ctx, connections)
	if err != nil {
		return 0
	}
	evaluated := 0
	for _, connection := range connections {
		if connection == nil {
			continue
		}
		item := buildOpsUpstreamConnectionBalanceItem(connection, settings, upstreamConnectionBalanceOverride(overrides, connection.ID), notifiedAt[connection.ID], now)
		if !item.Alert.Enabled || !item.Alert.Eligible || !item.Alert.SnapshotFresh {
			continue
		}
		evaluated++
		if !item.Alert.Low || !settings.Notification.EnterpriseWeChatEnabled || !upstreamConnectionBalanceNotificationDue(item.Alert.NotifiedAt, settings.RateLimitPerHour, now) {
			continue
		}
		webhookURL := strings.TrimSpace(settings.Notification.EnterpriseWeChatWebhookURL)
		if webhookURL == "" || isOpsWebhookMasked(webhookURL) {
			continue
		}
		content := buildOpsUpstreamConnectionBalanceLowWeComMarkdown(item, now)
		if err := sendOpsEnterpriseWeChatMarkdown(ctx, webhookURL, content, settings.Notification.MentionAllOnLowBalance); err != nil {
			continue
		}
		persistCtx, cancel := upstreamConnectionBalancePersistContext(ctx)
		_ = s.settingRepo.Set(persistCtx, upstreamConnectionBalanceNotifiedKey(connection.ID), now.Format(time.RFC3339Nano))
		cancel()
	}
	return evaluated
}

func (s *OpsService) opsUpstreamConnectionBalanceSettings(ctx context.Context) (OpsUpstreamConnectionBalanceSettings, error) {
	cfg, err := s.GetOpsAlertRuntimeSettings(ctx)
	if err != nil {
		return OpsUpstreamConnectionBalanceSettings{}, err
	}
	if cfg == nil || cfg.UpstreamConnectionBalance == nil {
		return defaultOpsUpstreamConnectionBalanceSettings(), nil
	}
	settings := *cfg.UpstreamConnectionBalance
	normalizeOpsUpstreamConnectionBalanceSettings(&settings)
	return settings, nil
}

func (s *OpsService) resolveOpsUpstreamConnectionBalanceSettings(ctx context.Context, embedded *OpsUpstreamConnectionBalanceSettings) (OpsUpstreamConnectionBalanceSettings, error) {
	settings := defaultOpsUpstreamConnectionBalanceSettings()
	if embedded != nil {
		settings = *embedded
		normalizeOpsUpstreamConnectionBalanceSettings(&settings)
	}
	if s == nil || s.settingRepo == nil {
		return settings, nil
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyOpsUpstreamConnectionBalance)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return settings, nil
		}
		return OpsUpstreamConnectionBalanceSettings{}, err
	}
	if strings.TrimSpace(raw) == "" {
		return settings, nil
	}
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return OpsUpstreamConnectionBalanceSettings{}, fmt.Errorf("invalid upstream connection balance settings: %w", err)
	}
	normalizeOpsUpstreamConnectionBalanceSettings(&settings)
	return settings, nil
}

func (s *OpsService) listAllUpstreamConnectionsForBalance(ctx context.Context) ([]*UpstreamConnection, error) {
	connections := make([]*UpstreamConnection, 0)
	for page := 1; ; page++ {
		items, total, err := s.upstreamConnectionBalance.List(ctx, UpstreamConnectionListParams{Page: page, PageSize: upstreamConnectionBalancePageSize})
		if err != nil {
			return nil, err
		}
		connections = append(connections, items...)
		if len(items) == 0 || int64(len(connections)) >= total {
			break
		}
	}
	return connections, nil
}

func (s *OpsService) loadUpstreamConnectionBalanceNotifiedAt(ctx context.Context, connections []*UpstreamConnection) (map[int64]*time.Time, error) {
	result := make(map[int64]*time.Time, len(connections))
	if s == nil || s.settingRepo == nil || len(connections) == 0 {
		return result, nil
	}
	keys := make([]string, 0, len(connections))
	for _, connection := range connections {
		if connection != nil && connection.ID > 0 {
			keys = append(keys, upstreamConnectionBalanceNotifiedKey(connection.ID))
		}
	}
	values, err := s.settingRepo.GetMultiple(ctx, keys)
	if err != nil {
		return nil, err
	}
	for _, connection := range connections {
		if connection == nil {
			continue
		}
		value := strings.TrimSpace(values[upstreamConnectionBalanceNotifiedKey(connection.ID)])
		if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
			parsed = parsed.UTC()
			result[connection.ID] = &parsed
		}
	}
	return result, nil
}

func (s *OpsService) loadUpstreamConnectionBalanceOverrides(ctx context.Context, connections []*UpstreamConnection) (map[int64]OpsUpstreamConnectionBalanceOverride, error) {
	result := make(map[int64]OpsUpstreamConnectionBalanceOverride, len(connections))
	if s == nil || s.settingRepo == nil || len(connections) == 0 {
		return result, nil
	}
	keys := make([]string, 0, len(connections))
	for _, connection := range connections {
		if connection != nil && connection.ID > 0 {
			keys = append(keys, upstreamConnectionBalanceOverrideKey(connection.ID))
		}
	}
	values, err := s.settingRepo.GetMultiple(ctx, keys)
	if err != nil {
		return nil, err
	}
	for _, connection := range connections {
		if connection == nil || connection.ID <= 0 {
			continue
		}
		raw := strings.TrimSpace(values[upstreamConnectionBalanceOverrideKey(connection.ID)])
		if raw == "" {
			continue
		}
		var override OpsUpstreamConnectionBalanceOverride
		if err := json.Unmarshal([]byte(raw), &override); err != nil {
			return nil, fmt.Errorf("invalid upstream connection balance override for connection %d: %w", connection.ID, err)
		}
		if override.ThresholdUSD != nil && *override.ThresholdUSD < 0 {
			return nil, fmt.Errorf("invalid upstream connection balance threshold for connection %d", connection.ID)
		}
		result[connection.ID] = override
	}
	return result, nil
}

func upstreamConnectionBalanceOverride(overrides map[int64]OpsUpstreamConnectionBalanceOverride, connectionID int64) *OpsUpstreamConnectionBalanceOverride {
	override, ok := overrides[connectionID]
	if !ok {
		return nil
	}
	return &override
}

func buildOpsUpstreamConnectionBalanceItem(connection *UpstreamConnection, settings OpsUpstreamConnectionBalanceSettings, override *OpsUpstreamConnectionBalanceOverride, notifiedAt *time.Time, now time.Time) OpsUpstreamConnectionBalanceItem {
	boundIDs := append([]int64(nil), connection.BoundAccountIDs...)
	bindingCount := connection.BindingCount
	if len(boundIDs) > bindingCount {
		bindingCount = len(boundIDs)
	}
	enabled := true
	threshold := settings.DefaultThresholdUSD
	usesDefault := true
	if override != nil {
		enabled = override.Enabled
		if override.ThresholdUSD != nil {
			threshold = *override.ThresholdUSD
			usesDefault = false
		}
	}
	eligible := connection.SyncEnabled && bindingCount > 0 && connection.Status != UpstreamConnectionStatusDisabled
	fresh := upstreamConnectionBalanceSnapshotFresh(connection, now)
	low := enabled && eligible && fresh && !connection.WalletUnlimited && connection.WalletUSD != nil && *connection.WalletUSD <= threshold
	return OpsUpstreamConnectionBalanceItem{
		ConnectionID: connection.ID, Name: connection.Name, Provider: connection.Provider,
		Status: connection.Status, LastError: connection.LastError, SyncEnabled: connection.SyncEnabled,
		SyncIntervalSeconds: connection.SyncIntervalSeconds, BindingCount: bindingCount, BoundAccountIDs: boundIDs,
		WalletAmount: connection.WalletAmount, WalletCurrency: connection.WalletCurrency, WalletUSD: connection.WalletUSD,
		WalletUnlimited: connection.WalletUnlimited, WalletSource: connection.WalletSource,
		WalletReliability: connection.WalletReliability, WalletObservedAt: connection.WalletObservedAt,
		Alert: OpsUpstreamConnectionBalanceAlertState{
			Enabled: enabled, ThresholdUSD: threshold, UsesDefault: usesDefault,
			Eligible: eligible, SnapshotFresh: fresh, Low: low, NotifiedAt: notifiedAt,
		},
	}
}

func upstreamConnectionBalanceSnapshotFresh(connection *UpstreamConnection, now time.Time) bool {
	if connection == nil || connection.WalletObservedAt == nil || connection.WalletObservedAt.IsZero() {
		return false
	}
	window := time.Duration(connection.SyncIntervalSeconds*3) * time.Second
	if window < upstreamConnectionBalanceMinFreshWindow {
		window = upstreamConnectionBalanceMinFreshWindow
	}
	return !connection.WalletObservedAt.Add(window).Before(now)
}

func upstreamConnectionBalanceNotificationDue(notifiedAt *time.Time, rateLimitPerHour int, now time.Time) bool {
	if notifiedAt == nil || notifiedAt.IsZero() {
		return true
	}
	if rateLimitPerHour <= 0 {
		return false
	}
	cooldown := time.Hour / time.Duration(rateLimitPerHour)
	if cooldown < time.Minute {
		cooldown = time.Minute
	}
	return !notifiedAt.Add(cooldown).After(now)
}

func upstreamConnectionBalanceFailed(connection *UpstreamConnection) bool {
	if connection == nil {
		return false
	}
	return connection.Status == UpstreamConnectionStatusAuthError || connection.Status == UpstreamConnectionStatusNeedsInput || strings.TrimSpace(connection.LastError) != ""
}

func upstreamConnectionBalanceFailedItem(item OpsUpstreamConnectionBalanceItem) bool {
	return item.Status == UpstreamConnectionStatusAuthError || item.Status == UpstreamConnectionStatusNeedsInput || strings.TrimSpace(item.LastError) != ""
}

func matchesUpstreamConnectionBalanceFilter(item OpsUpstreamConnectionBalanceItem, filter OpsUpstreamConnectionBalanceMonitorFilter) bool {
	if filter.Status != "" && !strings.EqualFold(item.Status, filter.Status) {
		return false
	}
	if filter.OnlyLow && !item.Alert.Low {
		return false
	}
	if filter.OnlyFailed && !upstreamConnectionBalanceFailedItem(item) {
		return false
	}
	if filter.Search != "" {
		haystack := strings.ToLower(strings.Join([]string{item.Name, item.Provider, item.Status, item.LastError}, " "))
		if !strings.Contains(haystack, filter.Search) {
			return false
		}
	}
	return true
}

func upstreamConnectionBalanceNotifiedKey(connectionID int64) string {
	return upstreamConnectionBalanceNotifiedKeyPrefix + strconv.FormatInt(connectionID, 10)
}

func upstreamConnectionBalanceOverrideKey(connectionID int64) string {
	return upstreamConnectionBalanceOverrideKeyPrefix + strconv.FormatInt(connectionID, 10)
}

func buildOpsUpstreamConnectionBalanceLowWeComMarkdown(item OpsUpstreamConnectionBalanceItem, now time.Time) string {
	balance := "unknown"
	if item.WalletUSD != nil {
		balance = fmt.Sprintf("$%.2f", *item.WalletUSD)
	}
	observed := "unknown"
	if item.WalletObservedAt != nil {
		observed = formatOpsNotifyTime(*item.WalletObservedAt)
	}
	return fmt.Sprintf(
		"## Shared upstream balance alert\n> Connection: %s (#%d)\n> Provider: %s\n> Balance: <font color=\"warning\">%s</font>\n> Threshold: $%.2f\n> Bound accounts: %d\n> Observed: %s\n> Alerted: %s",
		escapeOpsWeComMarkdown(item.Name), item.ConnectionID, escapeOpsWeComMarkdown(item.Provider), balance,
		item.Alert.ThresholdUSD, item.BindingCount, observed, formatOpsNotifyTime(now),
	)
}

func buildOpsUpstreamConnectionBalanceTestWeComMarkdown(now time.Time) string {
	return fmt.Sprintf("## Shared upstream balance test\n> The connection-level balance alert channel is working.\n> Sent: %s", formatOpsNotifyTime(now))
}
