package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	clientip "github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"golang.org/x/sync/singleflight"
)

var (
	ErrRegistrationDisabled   = infraerrors.Forbidden("REGISTRATION_DISABLED", "registration is currently disabled")
	ErrSettingNotFound        = infraerrors.NotFound("SETTING_NOT_FOUND", "setting not found")
	ErrDefaultSubGroupInvalid = infraerrors.BadRequest(
		"DEFAULT_SUBSCRIPTION_GROUP_INVALID",
		"default subscription group must exist and be subscription type",
	)
	ErrDefaultSubGroupDuplicate = infraerrors.BadRequest(
		"DEFAULT_SUBSCRIPTION_GROUP_DUPLICATE",
		"default subscription group cannot be duplicated",
	)
)

type SettingRepository interface {
	Get(ctx context.Context, key string) (*Setting, error)
	GetValue(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
	GetMultiple(ctx context.Context, keys []string) (map[string]string, error)
	SetMultiple(ctx context.Context, settings map[string]string) error
	GetAll(ctx context.Context) (map[string]string, error)
	Delete(ctx context.Context, key string) error
}

// DefaultSubscriptionGroupReader validates group references used by default subscriptions.
type DefaultSubscriptionGroupReader interface {
	GetByID(ctx context.Context, id int64) (*Group, error)
}

// WebSearchManagerBuilder creates a websearch.Manager from config (injected by infra layer).
// proxyURLs maps proxy ID to resolved URL for provider-level proxy support.
type WebSearchManagerBuilder func(cfg *WebSearchEmulationConfig, proxyURLs map[int64]string)

// SettingService 系统设置服务
type SettingService struct {
	settingRepo                 SettingRepository
	defaultSubGroupReader       DefaultSubscriptionGroupReader
	proxyRepo                   ProxyRepository // for resolving websearch provider proxy URLs
	cfg                         *config.Config
	onUpdate                    func() // Callback when settings are updated (for cache invalidation)
	version                     string // Application version
	webSearchManagerBuilder     WebSearchManagerBuilder
	antigravityUAVersionCache   atomic.Value // *cachedAntigravityUserAgentVersion
	antigravityUAVersionSF      singleflight.Group
	openAICodexUACache          atomic.Value // *cachedOpenAICodexUserAgent
	openAICodexUASF             singleflight.Group
	codexRestrictionPolicyCache atomic.Value // *cachedCodexRestrictionPolicy
	codexRestrictionPolicySF    singleflight.Group

	cyberSessionBlockRuntimeCache atomic.Value // *cachedCyberSessionBlockRuntime
	cyberSessionBlockRuntimeSF    singleflight.Group

	// panelRateLimitCache 面板 API 限流配置进程内缓存（*cachedPanelRateLimitSettings）。
	// 面板每个认证请求都会读取，禁止在热路径上直接访问 DB。
	panelRateLimitCache atomic.Value
	panelRateLimitSF    singleflight.Group

	// openAIQuotaAutoPauseSettingsCache holds the most recently observed quota auto-pause
	// settings. GetOpenAIQuotaAutoPauseSettings reads this atomic.Value on the request hot
	// path without ever blocking on the DB; when the cached entry expires, a background
	// goroutine refreshes it via openAIQuotaAutoPauseSettingsSF (stale-while-revalidate).
	// This per-service field also gives tests natural isolation — each SettingService
	// instance owns its own cache, no shared package-level state.
	openAIQuotaAutoPauseSettingsCache atomic.Value // *cachedOpenAIQuotaAutoPauseSettings
	openAIQuotaAutoPauseSettingsSF    singleflight.Group

	connectivitySnapshot atomic.Value // *ConnectivityProbeSnapshot
	connectivityResolver connectivityResolverFunc
	connectivityClientIP *clientip.VerifiedClientIPResolver
	connectivityWriteMu  sync.Mutex
	connectivityMu       sync.Mutex
	connectivityRefresh  sync.Once
}

// DefaultPlatformQuotaSetting 单 platform 三档限额（nil = 沿用上层；0 = 显式禁用；>0 = 上限）
type DefaultPlatformQuotaSetting struct {
	DailyLimitUSD   *float64 `json:"daily"`
	WeeklyLimitUSD  *float64 `json:"weekly"`
	MonthlyLimitUSD *float64 `json:"monthly"`
}

type ProviderDefaultGrantSettings struct {
	Balance          float64
	Concurrency      int
	Subscriptions    []DefaultSubscriptionSetting
	GrantOnSignup    bool
	GrantOnFirstBind bool
	PlatformQuotas   map[string]*DefaultPlatformQuotaSetting // key = platform name
}

type AuthSourceDefaultSettings struct {
	Email                        ProviderDefaultGrantSettings
	LinuxDo                      ProviderDefaultGrantSettings
	OIDC                         ProviderDefaultGrantSettings
	WeChat                       ProviderDefaultGrantSettings
	GitHub                       ProviderDefaultGrantSettings
	Google                       ProviderDefaultGrantSettings
	DingTalk                     ProviderDefaultGrantSettings
	ForceEmailOnThirdPartySignup bool
}

type authSourceDefaultKeySet struct {
	// source 是 auth source 标识（如 "email"、"github"），仅用于 parse 时
	// slog.Warn 诊断输出，不再参与 key 拼接（platformQuotas 字段已存完整 key）。
	source           string
	balance          string
	concurrency      string
	subscriptions    string
	grantOnSignup    string
	grantOnFirstBind string
	platformQuotas   string // SettingKeyAuthSourcePlatformQuotas(source)
}

var (
	emailAuthSourceDefaultKeys = authSourceDefaultKeySet{
		source:           "email",
		balance:          SettingKeyAuthSourceDefaultEmailBalance,
		concurrency:      SettingKeyAuthSourceDefaultEmailConcurrency,
		subscriptions:    SettingKeyAuthSourceDefaultEmailSubscriptions,
		grantOnSignup:    SettingKeyAuthSourceDefaultEmailGrantOnSignup,
		grantOnFirstBind: SettingKeyAuthSourceDefaultEmailGrantOnFirstBind,
		platformQuotas:   SettingKeyAuthSourcePlatformQuotas("email"),
	}
	linuxDoAuthSourceDefaultKeys = authSourceDefaultKeySet{
		source:           "linuxdo",
		balance:          SettingKeyAuthSourceDefaultLinuxDoBalance,
		concurrency:      SettingKeyAuthSourceDefaultLinuxDoConcurrency,
		subscriptions:    SettingKeyAuthSourceDefaultLinuxDoSubscriptions,
		grantOnSignup:    SettingKeyAuthSourceDefaultLinuxDoGrantOnSignup,
		grantOnFirstBind: SettingKeyAuthSourceDefaultLinuxDoGrantOnFirstBind,
		platformQuotas:   SettingKeyAuthSourcePlatformQuotas("linuxdo"),
	}
	oidcAuthSourceDefaultKeys = authSourceDefaultKeySet{
		source:           "oidc",
		balance:          SettingKeyAuthSourceDefaultOIDCBalance,
		concurrency:      SettingKeyAuthSourceDefaultOIDCConcurrency,
		subscriptions:    SettingKeyAuthSourceDefaultOIDCSubscriptions,
		grantOnSignup:    SettingKeyAuthSourceDefaultOIDCGrantOnSignup,
		grantOnFirstBind: SettingKeyAuthSourceDefaultOIDCGrantOnFirstBind,
		platformQuotas:   SettingKeyAuthSourcePlatformQuotas("oidc"),
	}
	weChatAuthSourceDefaultKeys = authSourceDefaultKeySet{
		source:           "wechat",
		balance:          SettingKeyAuthSourceDefaultWeChatBalance,
		concurrency:      SettingKeyAuthSourceDefaultWeChatConcurrency,
		subscriptions:    SettingKeyAuthSourceDefaultWeChatSubscriptions,
		grantOnSignup:    SettingKeyAuthSourceDefaultWeChatGrantOnSignup,
		grantOnFirstBind: SettingKeyAuthSourceDefaultWeChatGrantOnFirstBind,
		platformQuotas:   SettingKeyAuthSourcePlatformQuotas("wechat"),
	}
	gitHubAuthSourceDefaultKeys = authSourceDefaultKeySet{
		source:           "github",
		balance:          SettingKeyAuthSourceDefaultGitHubBalance,
		concurrency:      SettingKeyAuthSourceDefaultGitHubConcurrency,
		subscriptions:    SettingKeyAuthSourceDefaultGitHubSubscriptions,
		grantOnSignup:    SettingKeyAuthSourceDefaultGitHubGrantOnSignup,
		grantOnFirstBind: SettingKeyAuthSourceDefaultGitHubGrantOnFirstBind,
		platformQuotas:   SettingKeyAuthSourcePlatformQuotas("github"),
	}
	googleAuthSourceDefaultKeys = authSourceDefaultKeySet{
		source:           "google",
		balance:          SettingKeyAuthSourceDefaultGoogleBalance,
		concurrency:      SettingKeyAuthSourceDefaultGoogleConcurrency,
		subscriptions:    SettingKeyAuthSourceDefaultGoogleSubscriptions,
		grantOnSignup:    SettingKeyAuthSourceDefaultGoogleGrantOnSignup,
		grantOnFirstBind: SettingKeyAuthSourceDefaultGoogleGrantOnFirstBind,
		platformQuotas:   SettingKeyAuthSourcePlatformQuotas("google"),
	}
	dingTalkAuthSourceDefaultKeys = authSourceDefaultKeySet{
		source:           "dingtalk",
		balance:          SettingKeyAuthSourceDefaultDingTalkBalance,
		concurrency:      SettingKeyAuthSourceDefaultDingTalkConcurrency,
		subscriptions:    SettingKeyAuthSourceDefaultDingTalkSubscriptions,
		grantOnSignup:    SettingKeyAuthSourceDefaultDingTalkGrantOnSignup,
		grantOnFirstBind: SettingKeyAuthSourceDefaultDingTalkGrantOnFirstBind,
		platformQuotas:   SettingKeyAuthSourcePlatformQuotas("dingtalk"),
	}
)

const (
	defaultAuthSourceBalance        = 0
	defaultAuthSourceConcurrency    = 5
	defaultWeChatConnectMode        = "open"
	defaultWeChatConnectScopes      = "snsapi_login"
	defaultWeChatConnectFrontend    = "/auth/wechat/callback"
	defaultGitHubOAuthAuthorize     = "https://github.com/login/oauth/authorize"
	defaultGitHubOAuthToken         = "https://github.com/login/oauth/access_token"
	defaultGitHubOAuthUserInfo      = "https://api.github.com/user"
	defaultGitHubOAuthEmails        = "https://api.github.com/user/emails"
	defaultGitHubOAuthScopes        = "read:user user:email"
	defaultGitHubOAuthFrontend      = "/auth/oauth/callback"
	defaultGoogleOAuthAuthorize     = "https://accounts.google.com/o/oauth2/v2/auth"
	defaultGoogleOAuthToken         = "https://oauth2.googleapis.com/token"
	defaultGoogleOAuthUserInfo      = "https://openidconnect.googleapis.com/v1/userinfo"
	defaultGoogleOAuthScopes        = "openid email profile"
	defaultGoogleOAuthFrontend      = "/auth/oauth/callback"
	defaultLoginAgreementMode       = "modal"
	defaultLoginAgreementDate       = "2026-03-31"
	defaultModelPriceUSDCNYRate     = 7.0
	defaultModelPriceCNYPerQuotaUSD = 0.068
)

// NewSettingService 创建系统设置服务实例
func NewSettingService(settingRepo SettingRepository, cfg *config.Config) *SettingService {
	s := &SettingService{
		settingRepo: settingRepo,
		cfg:         cfg,
	}
	s.connectivityResolver = defaultConnectivityResolver
	if cfg != nil {
		s.connectivityClientIP, _ = clientip.NewVerifiedClientIPResolver(clientip.VerifiedClientIPConfig{
			TrustedProxiesConfigured: cfg.Server.TrustedProxiesConfigured,
			TrustedProxies:           cfg.Server.TrustedProxies,
			DeniedCIDRs:              cfg.Connectivity.ClientIPDeniedCIDRs,
			AllowDirect:              cfg.Connectivity.AllowDirectClientIP,
			MaxHops:                  cfg.Connectivity.ClientIPMaxHops,
		})
	}
	s.connectivitySnapshot.Store(defaultConnectivityProbeSnapshot())
	return s
}

// SetDefaultSubscriptionGroupReader injects an optional group reader for default subscription validation.
func (s *SettingService) SetDefaultSubscriptionGroupReader(reader DefaultSubscriptionGroupReader) {
	s.defaultSubGroupReader = reader
}

// SetProxyRepository injects a proxy repo for resolving websearch provider proxy URLs.
func (s *SettingService) SetProxyRepository(repo ProxyRepository) {
	s.proxyRepo = repo
}

func (s *SettingService) LoadForwardedClientIPSettings(ctx context.Context) error {
	if s == nil || s.cfg == nil || s.settingRepo == nil {
		return nil
	}

	values, err := s.settingRepo.GetMultiple(ctx, []string{
		SettingKeyAPIKeyACLTrustForwardedIP,
		SettingKeyForwardedClientIPHeaders,
		settingKeyForwardedClientIPModeV2,
	})
	if err != nil {
		s.cfg.SetForwardedClientIPSettings(false, nil)
		return fmt.Errorf("get forwarded client ip settings: %w", err)
	}

	enabled := s.cfg.Security.TrustForwardedIPForAPIKeyACL
	headers := s.cfg.ForwardedClientIPSettings().Headers
	storedValue, hasStoredValue := values[SettingKeyAPIKeyACLTrustForwardedIP]
	if hasStoredValue {
		enabled = storedValue == "true"
	}

	var headersErr error
	if storedHeaders, ok := values[SettingKeyForwardedClientIPHeaders]; ok {
		headers, headersErr = parseForwardedClientIPHeadersSetting(storedHeaders)
		if headersErr != nil {
			enabled = false
			headers = []string{}
			headersErr = fmt.Errorf("load forwarded client ip headers: %w", headersErr)
		}
	}

	updates := make(map[string]string)
	if _, hasStoredHeaders := values[SettingKeyForwardedClientIPHeaders]; !hasStoredHeaders {
		headersJSON, marshalErr := json.Marshal(headers)
		if marshalErr != nil {
			headers = []string{}
			headersErr = errors.Join(headersErr, fmt.Errorf("marshal forwarded client ip headers: %w", marshalErr))
			headersJSON = []byte("[]")
		}
		updates[SettingKeyForwardedClientIPHeaders] = string(headersJSON)
	}
	if values[settingKeyForwardedClientIPModeV2] != "true" {
		updates[settingKeyForwardedClientIPModeV2] = "true"
		// Before this migration, new installations persisted false by default.
		// Restore compatibility only when no trusted-proxy policy was configured.
		if headersErr == nil && hasStoredValue && !enabled && !s.cfg.Server.TrustedProxiesConfigured {
			enabled = true
			updates[SettingKeyAPIKeyACLTrustForwardedIP] = "true"
		}
	}
	if len(updates) > 0 {
		if err := s.settingRepo.SetMultiple(ctx, updates); err != nil {
			s.cfg.SetForwardedClientIPSettings(enabled, headers)
			return errors.Join(headersErr, fmt.Errorf("migrate forwarded client ip setting: %w", err))
		}
	}

	s.cfg.SetForwardedClientIPSettings(enabled, headers)
	return headersErr
}

// GetAllSettings 获取所有系统设置
func (s *SettingService) GetAllSettings(ctx context.Context) (*SystemSettings, error) {
	settings, err := s.settingRepo.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("get all settings: %w", err)
	}

	return s.parseSettings(settings), nil
}

// SetOnUpdateCallback sets a callback function to be called when settings are updated
// This is used for cache invalidation (e.g., HTML cache in frontend server)
func (s *SettingService) SetOnUpdateCallback(callback func()) {
	s.onUpdate = callback
}

// SetVersion sets the application version for injection into public settings
func (s *SettingService) SetVersion(version string) {
	s.version = version
}

// getStringOrDefault 获取字符串值或默认值
func (s *SettingService) getStringOrDefault(settings map[string]string, key, defaultValue string) string {
	if value, ok := settings[key]; ok && value != "" {
		return value
	}
	return defaultValue
}

const ReferralCreditConversionMultiplierMax = 1000

func (s *SettingService) GetModelPriceCNYPerQuotaUSD(ctx context.Context) float64 {
	if s == nil || s.settingRepo == nil {
		return defaultModelPriceCNYPerQuotaUSD
	}
	value, err := s.settingRepo.GetValue(ctx, SettingKeyModelPriceCNYPerQuotaUSD)
	if err != nil {
		return defaultModelPriceCNYPerQuotaUSD
	}
	return parseModelPriceCNYPerQuotaUSD(value)
}

func (s *SettingService) GetModelPriceCustomPrices(ctx context.Context) map[string]ModelPriceCustomPrice {
	out := map[string]ModelPriceCustomPrice{}
	if s == nil || s.settingRepo == nil {
		return out
	}
	value, err := s.settingRepo.GetValue(ctx, SettingKeyModelPriceCustomPrices)
	if err != nil || strings.TrimSpace(value) == "" {
		return out
	}
	if err := json.Unmarshal([]byte(value), &out); err != nil {
		return map[string]ModelPriceCustomPrice{}
	}
	return out
}

func (s *SettingService) GetModelPriceHiddenGroupIDs(ctx context.Context) map[int64]struct{} {
	out := map[int64]struct{}{}
	if s == nil || s.settingRepo == nil {
		return out
	}
	value, err := s.settingRepo.GetValue(ctx, SettingKeyModelPriceHiddenGroupIDs)
	if err != nil || strings.TrimSpace(value) == "" {
		return out
	}
	var ids []int64
	if err := json.Unmarshal([]byte(value), &ids); err != nil {
		return out
	}
	for _, id := range ids {
		if id > 0 {
			out[id] = struct{}{}
		}
	}
	return out
}

func (s *SettingService) GetModelPriceHiddenModelKeys(ctx context.Context) map[string]struct{} {
	out := map[string]struct{}{}
	if s == nil || s.settingRepo == nil {
		return out
	}
	value, err := s.settingRepo.GetValue(ctx, SettingKeyModelPriceHiddenModelKeys)
	if err != nil || strings.TrimSpace(value) == "" {
		return out
	}
	var keys []string
	if err := json.Unmarshal([]byte(value), &keys); err != nil {
		return out
	}
	for _, key := range keys {
		if normalized := normalizeModelPriceHiddenModelKey(key); normalized != "" {
			out[normalized] = struct{}{}
		}
	}
	return out
}

func (s *SettingService) GetModelPriceUSDCNYRate(ctx context.Context) float64 {
	if s == nil || s.settingRepo == nil {
		return defaultModelPriceUSDCNYRate
	}
	value, err := s.settingRepo.GetValue(ctx, SettingKeyModelPriceUSDCNYRate)
	if err != nil {
		return defaultModelPriceUSDCNYRate
	}
	return parseModelPriceUSDCNYRate(value)
}

func (s *SettingService) GetSubscriptionEntitlementsRuntime(ctx context.Context) SubscriptionEntitlementsRuntime {
	vals, err := s.settingRepo.GetMultiple(ctx, []string{
		SettingKeySubscriptionEntitlementsV2Enabled,
		SettingKeySub2PaymentPageLegacyMappingEnabled,
	})
	if err != nil {
		return SubscriptionEntitlementsRuntime{}
	}
	return SubscriptionEntitlementsRuntime{
		Enabled:                     vals[SettingKeySubscriptionEntitlementsV2Enabled] == "true",
		LegacyCashierMappingEnabled: vals[SettingKeySub2PaymentPageLegacyMappingEnabled] == "true",
	}
}

func (s *SettingService) IsModelPricesUserVisible(ctx context.Context) bool {
	if s == nil || s.settingRepo == nil {
		return true
	}
	value, err := s.settingRepo.GetValue(ctx, SettingKeyModelPricesUserVisible)
	if err != nil {
		return true
	}
	return !isFalseSettingValue(value)
}

func (s *SettingService) SetModelPriceCustomPrice(ctx context.Context, groupID int64, model string, price *ModelPriceCustomPrice) (map[string]ModelPriceCustomPrice, error) {
	if s == nil || s.settingRepo == nil {
		return map[string]ModelPriceCustomPrice{}, nil
	}
	if groupID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_GROUP_ID", "group_id must be greater than 0")
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, infraerrors.BadRequest("INVALID_MODEL", "model is required")
	}

	prices := s.GetModelPriceCustomPrices(ctx)
	key := ModelPriceCustomPriceKey(groupID, model)
	if price != nil {
		normalizeModelPriceCustomPrice(price)
	}
	if price == nil || !price.HasPrice() {
		delete(prices, key)
	} else {
		normalized := *price
		normalized.BillingMode = strings.TrimSpace(normalized.BillingMode)
		normalized.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		prices[key] = normalized
	}
	payload, err := json.Marshal(prices)
	if err != nil {
		return nil, fmt.Errorf("marshal model price custom prices: %w", err)
	}
	if err := s.settingRepo.Set(ctx, SettingKeyModelPriceCustomPrices, string(payload)); err != nil {
		return nil, fmt.Errorf("set model price custom prices: %w", err)
	}
	return prices, nil
}

func (s *SettingService) SetModelPriceHiddenGroupIDs(ctx context.Context, ids []int64) ([]int64, error) {
	if s == nil || s.settingRepo == nil {
		return nil, nil
	}
	normalized := normalizePositiveInt64s(ids)
	payload, err := json.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("marshal model price hidden group ids: %w", err)
	}
	if err := s.settingRepo.Set(ctx, SettingKeyModelPriceHiddenGroupIDs, string(payload)); err != nil {
		return nil, fmt.Errorf("set model price hidden group ids: %w", err)
	}
	return normalized, nil
}

func (s *SettingService) SetModelPriceHiddenModel(ctx context.Context, groupID int64, model string, hidden bool) ([]string, error) {
	return s.SetModelPriceHiddenModels(ctx, groupID, []string{model}, hidden)
}

func (s *SettingService) SetModelPriceHiddenModels(ctx context.Context, groupID int64, models []string, hidden bool) ([]string, error) {
	if s == nil || s.settingRepo == nil {
		return []string{}, nil
	}
	if groupID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_GROUP_ID", "group_id must be greater than 0")
	}
	normalizedModels := normalizeModelPriceModelNames(models)
	if len(normalizedModels) == 0 {
		return nil, infraerrors.BadRequest("INVALID_MODEL", "models are required")
	}

	keys := s.GetModelPriceHiddenModelKeys(ctx)
	for _, model := range normalizedModels {
		key := ModelPriceHiddenModelKey(groupID, model)
		if hidden {
			keys[key] = struct{}{}
		} else {
			delete(keys, key)
		}
	}
	normalized := sortedModelPriceHiddenModelKeys(keys)
	payload, err := json.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("marshal model price hidden model keys: %w", err)
	}
	if err := s.settingRepo.Set(ctx, SettingKeyModelPriceHiddenModelKeys, string(payload)); err != nil {
		return nil, fmt.Errorf("set model price hidden model keys: %w", err)
	}
	return normalized, nil
}

func (s *SettingService) getReferralCurrencyPublic(settings map[string]string) string {
	v := settings[SettingKeyReferralSettlementCurrency]
	if v == "" {
		return ReferralSettlementCurrencyCNY
	}
	return v
}

func (s *SettingService) getReferralWithdrawMethodsPublic(settings map[string]string) []string {
	raw := settings[SettingKeyReferralWithdrawMethodsEnabled]
	if raw == "" {
		return nil
	}
	var methods []string
	if err := json.Unmarshal([]byte(raw), &methods); err == nil {
		return methods
	}
	return nil
}

func (p ModelPriceCustomPrice) HasPrice() bool {
	return modelPricePositiveFloatPtr(p.InputUSDPerM) ||
		modelPricePositiveFloatPtr(p.OutputUSDPerM) ||
		modelPricePositiveFloatPtr(p.CacheWriteUSDPerM) ||
		modelPricePositiveFloatPtr(p.CacheReadUSDPerM) ||
		modelPricePositiveFloatPtr(p.ImageOutputUSDPerM) ||
		modelPricePositiveFloatPtr(p.PerRequestUSD)
}

func ModelPriceCustomPriceKey(groupID int64, model string) string {
	return strconv.FormatInt(groupID, 10) + ":" + strings.ToLower(strings.TrimSpace(model))
}

func ModelPriceHiddenModelKey(groupID int64, model string) string {
	return strconv.FormatInt(groupID, 10) + ":" + strings.ToLower(strings.TrimSpace(model))
}

func ValidateReferralCreditConversionMultiplier(multiplier float64) error {
	if math.IsNaN(multiplier) || math.IsInf(multiplier, 0) || multiplier <= 0 || multiplier > ReferralCreditConversionMultiplierMax {
		return infraerrors.BadRequest(
			"INVALID_REFERRAL_CREDIT_CONVERSION_RATE",
			fmt.Sprintf("referral credit conversion multiplier must be greater than 0 and no more than %d", ReferralCreditConversionMultiplierMax),
		)
	}
	return nil
}

func modelPricePositiveFloatPtr(v *float64) bool {
	return v != nil && !math.IsNaN(*v) && !math.IsInf(*v, 0) && *v > 0
}

func normalizeModelPriceCustomPrice(price *ModelPriceCustomPrice) {
	if price == nil {
		return
	}
	price.InputUSDPerM = positiveModelPriceValue(price.InputUSDPerM)
	price.OutputUSDPerM = positiveModelPriceValue(price.OutputUSDPerM)
	price.CacheWriteUSDPerM = positiveModelPriceValue(price.CacheWriteUSDPerM)
	price.CacheReadUSDPerM = positiveModelPriceValue(price.CacheReadUSDPerM)
	price.ImageOutputUSDPerM = positiveModelPriceValue(price.ImageOutputUSDPerM)
	price.PerRequestUSD = positiveModelPriceValue(price.PerRequestUSD)
}

func normalizeModelPriceHiddenModelKey(key string) string {
	parts := strings.SplitN(strings.TrimSpace(key), ":", 2)
	if len(parts) != 2 {
		return ""
	}
	groupID, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil || groupID <= 0 {
		return ""
	}
	model := strings.ToLower(strings.TrimSpace(parts[1]))
	if model == "" {
		return ""
	}
	return strconv.FormatInt(groupID, 10) + ":" + model
}

func normalizeModelPriceModelNames(models []string) []string {
	if len(models) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(models))
	out := make([]string, 0, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		key := strings.ToLower(model)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, model)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToLower(out[i]) < strings.ToLower(out[j])
	})
	return out
}

func normalizePositiveInt64s(ids []int64) []int64 {
	if len(ids) == 0 {
		return []int64{}
	}
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func parseModelPriceCNYPerQuotaUSD(raw string) float64 {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return defaultModelPriceCNYPerQuotaUSD
	}
	return value
}

func parseModelPriceUSDCNYRate(raw string) float64 {
	rate, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || rate <= 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
		return defaultModelPriceUSDCNYRate
	}
	return rate
}

func parseNormalizedStringSliceSetting(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}

	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return []string{}
	}

	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	return normalized
}

func parseReferralCreditConversionRate(raw string) float64 {
	rate, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || ValidateReferralCreditConversionMultiplier(rate) != nil {
		return 1
	}
	return rate
}

// parseReferralLevel1RatePublic parses the stored level-1 commission rate as a
// raw fraction (typically 0–1), matching settlement (GetAllSettings). Do NOT
// invent percent→fraction conversion here — that would make marketing UI lie
// relative to reward booking (PaidAmount * raw rate).
func parseReferralLevel1RatePublic(raw string) float64 {
	rate, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || rate < 0 {
		return 0
	}
	return rate
}

func parseReferralLevel1EnabledPublic(settings map[string]string) bool {
	if v, ok := settings[SettingKeyReferralLevel1Enabled]; ok {
		return v == "true"
	}
	// Match GetAllSettings default: level-1 on when unset.
	return true
}

// parseReferralLevel1RateForDisplay zeros the rate when level-1 rewards are
// disabled so public marketing never promises commission that will not book.
func parseReferralLevel1RateForDisplay(settings map[string]string) float64 {
	if !parseReferralLevel1EnabledPublic(settings) {
		return 0
	}
	return parseReferralLevel1RatePublic(settings[SettingKeyReferralLevel1Rate])
}

// parseReferralRewardModePublic normalizes reward mode the same way as
// GetAllSettings / settlement, so marketing copy never claims every-order
// while booking only honors first-paid (or vice versa).
func parseReferralRewardModePublic(raw string) string {
	mode := strings.TrimSpace(raw)
	switch mode {
	case ReferralRewardModeEveryPaidOrder, ReferralRewardModeFirstPaidOrder:
		return mode
	default:
		return ReferralRewardModeFirstPaidOrder
	}
}

// parseReferralSettlementDelayDaysPublic matches settlement clamping:
// invalid or negative → default 7 (never advertise a negative settle window).
func parseReferralSettlementDelayDaysPublic(raw string) int {
	days, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || days < 0 {
		return 7
	}
	return days
}

func positiveModelPriceValue(value *float64) *float64 {
	if !modelPricePositiveFloatPtr(value) {
		return nil
	}
	return value
}

func referralCreditConversionEnabledPublic(settings map[string]string) bool {
	if v, ok := settings[SettingKeyReferralCreditConversionEnabled]; ok {
		return v == "true"
	}
	return settings[SettingKeyReferralWithdrawEnabled] == "true"
}

func sortedModelPriceHiddenModelKeys(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for v := range values {
		if normalized := normalizeModelPriceHiddenModelKey(v); normalized != "" {
			out = append(out, normalized)
		}
	}
	sort.Strings(out)
	return out
}

type ModelPriceCustomPrice struct {
	BillingMode        string   `json:"billing_mode,omitempty"`
	InputUSDPerM       *float64 `json:"input_usd_per_m,omitempty"`
	OutputUSDPerM      *float64 `json:"output_usd_per_m,omitempty"`
	CacheWriteUSDPerM  *float64 `json:"cache_write_usd_per_m,omitempty"`
	CacheReadUSDPerM   *float64 `json:"cache_read_usd_per_m,omitempty"`
	ImageOutputUSDPerM *float64 `json:"image_output_usd_per_m,omitempty"`
	PerRequestUSD      *float64 `json:"per_request_usd,omitempty"`
	UpdatedAt          string   `json:"updated_at,omitempty"`
}

type SubscriptionEntitlementsRuntime struct {
	Enabled                     bool
	LegacyCashierMappingEnabled bool
}
