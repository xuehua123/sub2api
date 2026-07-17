package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
)

const (
	upstreamRateMultiplierSyncInterval       = 5 * time.Minute
	upstreamRateMultiplierSyncTimeout        = 90 * time.Second
	upstreamRateMultiplierRequestTimeout     = 8 * time.Second
	upstreamRateMultiplierSyncMaxConcurrency = 4
	upstreamRateMultiplierResponseLimit      = 1 << 20
)

// ErrUpstreamManagementBalanceUnavailable means this account does not have a
// usable management-plane identity. It is a configuration gap, not an
// upstream balance or billing failure.
var ErrUpstreamManagementBalanceUnavailable = errors.New("upstream management credentials are not configured for account balance")

// ErrUpstreamAPIKeyGroupUnmapped means the management identity is valid but
// cannot prove which upstream group owns this account's forwarding API key.
var ErrUpstreamAPIKeyGroupUnmapped = errors.New("upstream API key group could not be mapped")

// UpstreamRateMultiplierSyncRepository is intentionally narrow. Syncing a
// supplier's group multiplier must not gain access to local group pricing.
type UpstreamRateMultiplierSyncRepository interface {
	ListActiveSchedulableForRateMultiplierPriority(ctx context.Context) ([]Account, error)
	UpdateRateMultipliers(ctx context.Context, multipliers map[int64]float64) (int64, error)
}

// UpstreamRateMultiplierSyncService reads the configured upstream user's group
// multiplier into each active account's own rate multiplier. It never changes
// local groups, account-group bindings, subscriptions, or billing rules.
type UpstreamRateMultiplierSyncService struct {
	repo      UpstreamRateMultiplierSyncRepository
	cfg       *config.Config
	client    *http.Client
	encryptor SecretEncryptor
	priority  *RateMultiplierPriorityService
	interval  time.Duration

	stopCh    chan struct{}
	runCtx    context.Context
	runCancel context.CancelFunc
	startOnce sync.Once
	stopOnce  sync.Once
	wg        sync.WaitGroup
}

func NewUpstreamRateMultiplierSyncService(repo UpstreamRateMultiplierSyncRepository, cfg *config.Config, client *http.Client, encryptor SecretEncryptor, priority *RateMultiplierPriorityService, interval time.Duration) *UpstreamRateMultiplierSyncService {
	if interval <= 0 {
		interval = upstreamRateMultiplierSyncInterval
	}
	if client == nil {
		client = &http.Client{Timeout: upstreamRateMultiplierRequestTimeout}
	}
	runCtx, runCancel := context.WithCancel(context.Background())
	return &UpstreamRateMultiplierSyncService{
		repo: repo, cfg: cfg, client: client, encryptor: encryptor, priority: priority, interval: interval,
		stopCh: make(chan struct{}), runCtx: runCtx, runCancel: runCancel,
	}
}

func (s *UpstreamRateMultiplierSyncService) Start() {
	if s == nil || s.repo == nil || s.encryptor == nil {
		return
	}
	s.startOnce.Do(func() {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.reconcileAndLog()
			ticker := time.NewTicker(s.interval)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					s.reconcileAndLog()
				case <-s.stopCh:
					return
				}
			}
		}()
	})
}

func (s *UpstreamRateMultiplierSyncService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		if s.runCancel != nil {
			s.runCancel()
		}
		close(s.stopCh)
		s.wg.Wait()
	})
}

func (s *UpstreamRateMultiplierSyncService) reconcileAndLog() {
	ctx := s.runCtx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, upstreamRateMultiplierSyncTimeout)
	defer cancel()
	updated, err := s.Reconcile(ctx)
	if err != nil {
		slog.Error("upstream_rate_multiplier_sync_failed", "error", err)
		return
	}
	if updated > 0 {
		slog.Info("upstream_rate_multiplier_synced", "updated_accounts", updated)
	}
}

type upstreamRateMultiplierTarget struct {
	baseURL    string
	config     UpstreamRateMultiplierSyncConfig
	ciphertext string
	client     *http.Client
	accounts   []Account
}

// Reconcile probes each distinct management identity/group once. Any failed
// upstream request is isolated to that target and leaves the current multiplier
// untouched, which prevents a temporary login or provider error from changing
// billing or scheduling.
func (s *UpstreamRateMultiplierSyncService) Reconcile(ctx context.Context) (int64, error) {
	if s == nil || s.repo == nil || s.encryptor == nil {
		return 0, nil
	}
	accounts, err := s.repo.ListActiveSchedulableForRateMultiplierPriority(ctx)
	if err != nil {
		return 0, err
	}

	targets := make(map[string]*upstreamRateMultiplierTarget)
	for _, account := range accounts {
		if account.Status != StatusActive || !account.Schedulable || !account.IsUpstreamRateMultiplierSyncEnabled() {
			continue
		}
		if account.Type != AccountTypeAPIKey && account.Type != AccountTypeUpstream {
			slog.Warn("upstream_rate_multiplier_sync_skipped", "account_id", account.ID, "reason", "account_type_does_not_support_management_sync")
			continue
		}
		config, configErr := account.UpstreamRateMultiplierSyncConfig()
		if configErr != nil {
			slog.Warn("upstream_rate_multiplier_sync_skipped", "account_id", account.ID, "reason", "invalid_management_config")
			continue
		}
		ciphertext := account.GetCredential(upstreamManagementAuthCredentialKey)
		if ciphertext == "" {
			slog.Warn("upstream_rate_multiplier_sync_skipped", "account_id", account.ID, "reason", "management_credentials_not_configured")
			continue
		}
		baseURL, baseURLErr := s.managementBaseURL(&account)
		if baseURLErr != nil {
			slog.Warn("upstream_rate_multiplier_sync_skipped", "account_id", account.ID, "reason", "invalid_management_base_url")
			continue
		}
		client, proxyKey, clientErr := s.managementClientForAccount(&account)
		if clientErr != nil {
			slog.Warn("upstream_rate_multiplier_sync_skipped", "account_id", account.ID, "reason", "invalid_management_proxy")
			continue
		}
		key := strings.Join([]string{baseURL, proxyKey, string(config.Provider), string(config.AuthMode), strconv.FormatInt(config.RemoteUserID, 10), ciphertext, config.Group}, "\x00")
		target := targets[key]
		if target == nil {
			target = &upstreamRateMultiplierTarget{baseURL: baseURL, config: config, ciphertext: ciphertext, client: client}
			targets[key] = target
		}
		target.accounts = append(target.accounts, account)
	}
	if len(targets) == 0 {
		return 0, nil
	}

	ordered := make([]*upstreamRateMultiplierTarget, 0, len(targets))
	for _, target := range targets {
		ordered = append(ordered, target)
	}
	sort.Slice(ordered, func(i, j int) bool {
		left := ordered[i]
		right := ordered[j]
		if left.baseURL != right.baseURL {
			return left.baseURL < right.baseURL
		}
		if left.config.Provider != right.config.Provider {
			return left.config.Provider < right.config.Provider
		}
		return left.config.Group < right.config.Group
	})

	type result struct {
		target     *upstreamRateMultiplierTarget
		multiplier float64
		err        error
	}
	jobs := make(chan *upstreamRateMultiplierTarget)
	results := make(chan result, len(ordered))
	workers := upstreamRateMultiplierSyncMaxConcurrency
	if workers > len(ordered) {
		workers = len(ordered)
	}
	var workersWG sync.WaitGroup
	for range workers {
		workersWG.Add(1)
		go func() {
			defer workersWG.Done()
			for target := range jobs {
				multiplier, fetchErr := s.fetchGroupRateMultiplier(ctx, target)
				results <- result{target: target, multiplier: multiplier, err: fetchErr}
			}
		}()
	}
	go func() {
		defer close(results)
		defer workersWG.Wait()
		defer close(jobs)
		for _, target := range ordered {
			select {
			case jobs <- target:
			case <-ctx.Done():
				return
			}
		}
	}()

	updates := make(map[int64]float64)
	for item := range results {
		if item.err != nil {
			slog.Warn("upstream_rate_multiplier_sync_probe_failed", "account_id", item.target.accounts[0].ID, "provider", item.target.config.Provider, "upstream_group", item.target.config.Group, "error", item.err)
			continue
		}
		for _, account := range item.target.accounts {
			if math.Abs(account.BillingRateMultiplier()-item.multiplier) > rateMultiplierPriorityEpsilon {
				updates[account.ID] = item.multiplier
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if len(updates) == 0 {
		return 0, nil
	}

	updated, err := s.repo.UpdateRateMultipliers(ctx, updates)
	if err != nil || updated == 0 || s.priority == nil {
		return updated, err
	}
	priorityCtx, cancel := context.WithTimeout(ctx, rateMultiplierPriorityTimeout)
	defer cancel()
	if _, err := s.priority.ReconcileIfEnabled(priorityCtx); err != nil {
		return updated, fmt.Errorf("reconcile priorities after upstream multiplier sync: %w", err)
	}
	return updated, nil
}

func (s *UpstreamRateMultiplierSyncService) managementBaseURL(account *Account) (string, error) {
	baseURL := accountUpstreamBaseURL(account)
	if baseURL == "" {
		return "", errors.New("base url is not configured")
	}
	if s.cfg == nil || !s.cfg.Security.URLAllowlist.Enabled {
		allowHTTP := s.cfg == nil || s.cfg.Security.URLAllowlist.AllowInsecureHTTP
		return urlvalidator.ValidateURLFormat(baseURL, allowHTTP)
	}
	return urlvalidator.ValidateHTTPSURL(baseURL, urlvalidator.ValidationOptions{
		AllowedHosts:     s.cfg.Security.URLAllowlist.UpstreamHosts,
		RequireAllowlist: len(s.cfg.Security.URLAllowlist.UpstreamHosts) > 0,
		AllowPrivate:     s.cfg.Security.URLAllowlist.AllowPrivateHosts,
	})
}

func accountUpstreamBaseURL(account *Account) string {
	if account == nil {
		return ""
	}
	for _, key := range []string{"base_url", "url", "endpoint"} {
		if value := strings.TrimSpace(account.GetCredential(key)); value != "" {
			return value
		}
	}
	if value := strings.TrimSpace(account.GetOpenAIBaseURL()); value != "" {
		return value
	}
	return strings.TrimSpace(account.GetBaseURL())
}

// managementClientForAccount deliberately follows an account's active proxy.
// Some upstream management planes bind login sessions to egress IP; querying
// direct while live traffic uses a proxy can otherwise create false failures.
func (s *UpstreamRateMultiplierSyncService) managementClientForAccount(account *Account) (*http.Client, string, error) {
	if s == nil || s.client == nil || account == nil || account.ProxyID == nil || account.Proxy == nil || !account.Proxy.IsActive() {
		return s.client, "", nil
	}
	proxyURL := strings.TrimSpace(account.Proxy.URL())
	if proxyURL == "" {
		return s.client, "", nil
	}
	validateResolvedIP := s.cfg != nil && s.cfg.Security.URLAllowlist.Enabled
	client, err := httpclient.GetClient(httpclient.Options{
		ProxyURL:              proxyURL,
		Timeout:               upstreamRateMultiplierRequestTimeout,
		ResponseHeaderTimeout: upstreamRateMultiplierRequestTimeout,
		ValidateResolvedIP:    validateResolvedIP,
		AllowPrivateHosts:     s.cfg != nil && s.cfg.Security.URLAllowlist.AllowPrivateHosts,
	})
	if err != nil {
		return nil, "", err
	}
	return client, proxyURL, nil
}

type upstreamManagementHTTPSession struct {
	remoteUserID int64
	accessToken  string
	cookie       *http.Cookie
}

// UpstreamRateMultiplierGroup is a selectable upstream group returned by a
// read-only management-plane discovery request.
type UpstreamRateMultiplierGroup struct {
	ID             int64   `json:"id,omitempty"`
	Name           string  `json:"name"`
	RateMultiplier float64 `json:"rate_multiplier"`
}

// UpstreamRateMultiplierDiscovery contains the adapter selected after probing
// an upstream management API. It deliberately contains no credential material.
type UpstreamRateMultiplierDiscovery struct {
	Provider     UpstreamManagementProvider    `json:"provider"`
	AuthMode     UpstreamManagementAuthMode    `json:"auth_mode"`
	RemoteUserID int64                         `json:"remote_user_id,omitempty"`
	Groups       []UpstreamRateMultiplierGroup `json:"groups"`
	MatchedGroup *UpstreamRateMultiplierGroup  `json:"matched_group,omitempty"`
}

// DiscoverGroups detects a compatible upstream management API and returns the
// groups and multipliers visible to the supplied management credentials. It is
// intentionally read-only and never persists the plaintext input.
func (s *UpstreamRateMultiplierSyncService) DiscoverGroups(
	ctx context.Context,
	account *Account,
	authMode UpstreamManagementAuthMode,
	remoteUserID int64,
	input *UpstreamManagementAuthInput,
	upstreamAPIKey string,
) (*UpstreamRateMultiplierDiscovery, error) {
	if s == nil || account == nil {
		return nil, errors.New("upstream rate multiplier discovery is unavailable")
	}
	if authMode != UpstreamManagementAuthModePassword && authMode != UpstreamManagementAuthModeAccessToken {
		return nil, errors.New("upstream management auth mode must be password or access_token")
	}
	if remoteUserID < 0 {
		return nil, errors.New("upstream remote user id must be positive")
	}
	baseURL, err := s.managementBaseURL(account)
	if err != nil {
		return nil, err
	}
	client, _, err := s.managementClientForAccount(account)
	if err != nil {
		return nil, err
	}
	secret, err := s.discoverySecret(account, authMode, input)
	if err != nil {
		return nil, err
	}

	configFor := func(provider UpstreamManagementProvider) UpstreamRateMultiplierSyncConfig {
		return UpstreamRateMultiplierSyncConfig{
			Provider:     provider,
			AuthMode:     authMode,
			Group:        "__discovery__",
			RemoteUserID: remoteUserID,
		}
	}
	if err := validateUpstreamManagementAuth(configFor(UpstreamManagementProviderSub2API), secret); err != nil {
		return nil, err
	}

	upstreamAPIKey = strings.TrimSpace(upstreamAPIKey)
	if upstreamAPIKey == "" {
		upstreamAPIKey = accountBalanceAPIKey(account)
	}
	sub2APIConfig := configFor(UpstreamManagementProviderSub2API)
	if groups, discoveredUserID, matchedGroup, discoveryErr := s.discoverSub2APIGroups(ctx, client, baseURL, sub2APIConfig, secret, upstreamAPIKey); discoveryErr == nil && len(groups) > 0 {
		return &UpstreamRateMultiplierDiscovery{
			Provider:     sub2APIConfig.Provider,
			AuthMode:     authMode,
			RemoteUserID: discoveredUserID,
			Groups:       groups,
			MatchedGroup: matchedGroup,
		}, nil
	} else if errors.Is(discoveryErr, ErrUpstreamAPIKeyGroupUnmapped) {
		return nil, discoveryErr
	}

	// NewAPI, RixAPI, and ShellAPI share the same management login endpoint.
	// Authenticate once, then probe only the provider-specific request headers.
	session, err := s.authenticateNewAPIManagementSession(ctx, client, baseURL, configFor(UpstreamManagementProviderNewAPI), secret)
	if err != nil {
		return nil, errors.New("unable to detect an upstream management API or load a usable group list")
	}
	for _, provider := range []UpstreamManagementProvider{
		UpstreamManagementProviderNewAPI,
		UpstreamManagementProviderRixAPI,
		UpstreamManagementProviderShellAPI,
	} {
		config := configFor(provider)
		groups, discoveredUserID, discoveryErr := s.discoverNewAPIGroups(ctx, client, baseURL, config, session)
		if discoveryErr == nil && len(groups) > 0 {
			return &UpstreamRateMultiplierDiscovery{
				Provider:     provider,
				AuthMode:     authMode,
				RemoteUserID: discoveredUserID,
				Groups:       groups,
			}, nil
		}
	}

	return nil, errors.New("unable to detect an upstream management API or load a usable group list")
}

// FetchAccountBalance reads the upstream user wallet through the same
// encrypted management identity used for rate-multiplier synchronization. It
// intentionally never falls back to /api/usage/token because that endpoint
// reports a single API key's quota rather than the upstream account balance.
func (s *UpstreamRateMultiplierSyncService) FetchAccountBalance(ctx context.Context, account *Account) (opsAccountBalanceMethodResult, error) {
	if s == nil || account == nil || s.encryptor == nil || !account.IsUpstreamRateMultiplierSyncEnabled() || !account.HasUpstreamManagementAuth() {
		return opsAccountBalanceMethodResult{}, ErrUpstreamManagementBalanceUnavailable
	}
	config, err := account.UpstreamRateMultiplierSyncConfig()
	if err != nil {
		return opsAccountBalanceMethodResult{}, fmt.Errorf("%w: %v", ErrUpstreamManagementBalanceUnavailable, err)
	}
	baseURL, err := s.managementBaseURL(account)
	if err != nil {
		return opsAccountBalanceMethodResult{}, err
	}
	client, _, err := s.managementClientForAccount(account)
	if err != nil {
		return opsAccountBalanceMethodResult{}, err
	}
	secret, err := DecryptUpstreamManagementAuth(s.encryptor, account.GetCredential(upstreamManagementAuthCredentialKey))
	if err != nil {
		return opsAccountBalanceMethodResult{}, err
	}
	if err := validateUpstreamManagementAuth(config, secret); err != nil {
		return opsAccountBalanceMethodResult{}, err
	}

	switch config.Provider {
	case UpstreamManagementProviderNewAPI, UpstreamManagementProviderRixAPI, UpstreamManagementProviderShellAPI:
		return s.fetchNewAPIUserBalance(ctx, client, baseURL, config, secret)
	case UpstreamManagementProviderSub2API:
		return s.fetchSub2APIUserBalance(ctx, client, baseURL, config, secret)
	default:
		return opsAccountBalanceMethodResult{}, fmt.Errorf("unsupported upstream management provider %q", config.Provider)
	}
}

func (s *UpstreamRateMultiplierSyncService) fetchNewAPIUserBalance(ctx context.Context, client *http.Client, baseURL string, config UpstreamRateMultiplierSyncConfig, secret upstreamManagementAuthSecret) (opsAccountBalanceMethodResult, error) {
	session, err := s.authenticateNewAPIManagementSession(ctx, client, baseURL, config, secret)
	if err != nil {
		return opsAccountBalanceMethodResult{}, err
	}
	headers := newAPIManagementHeaders(config.Provider, session)
	endpoint := accountBalanceJoinEndpoint(baseURL, "/api/user/self", false)
	profile, err := s.managementJSON(ctx, client, http.MethodGet, endpoint, headers, nil)
	if err != nil {
		return opsAccountBalanceMethodResult{}, err
	}
	data := accountBalanceDataObject(profile.payload)
	quota := accountBalanceNumber(data, "quota", "balance", "available")
	if quota == nil {
		return opsAccountBalanceMethodResult{}, errors.New("upstream management profile has no account quota")
	}

	amount := *quota
	currency := "QUOTA"
	var usedAmount *float64
	if status, statusErr := s.managementJSON(ctx, client, http.MethodGet, accountBalanceJoinEndpoint(baseURL, "/api/status", false), headers, nil); statusErr == nil {
		statusData := accountBalanceDataObject(status.payload)
		if perUnit := accountBalanceNumber(statusData, "quota_per_unit"); perUnit != nil && *perUnit > 0 {
			amount = *quota / *perUnit
			if usedQuota := accountBalanceNumber(data, "used_quota", "used"); usedQuota != nil {
				used := *usedQuota / *perUnit
				usedAmount = &used
			}
			if displayType := strings.ToUpper(strings.TrimSpace(firstString(statusData, "quota_display_type", "currency"))); displayType != "" {
				currency = displayType
			}
		}
	}

	result := opsAccountBalanceMethodResult{
		Method:          AccountBalanceProbeMethodUpstreamManagement,
		Endpoint:        endpoint,
		BalanceAmount:   &amount,
		BalanceCurrency: currency,
	}
	if currency == "USD" {
		result.BalanceUSD = &amount
		result.TotalUsedUSD = usedAmount
	}
	return result, nil
}

func (s *UpstreamRateMultiplierSyncService) fetchSub2APIUserBalance(ctx context.Context, client *http.Client, baseURL string, config UpstreamRateMultiplierSyncConfig, secret upstreamManagementAuthSecret) (opsAccountBalanceMethodResult, error) {
	accessToken := secret.AccessToken
	if config.AuthMode == UpstreamManagementAuthModePassword {
		login, err := s.managementJSON(ctx, client, http.MethodPost, accountBalanceJoinEndpoint(baseURL, "/api/v1/auth/login", false), nil, map[string]string{"email": secret.Username, "password": secret.Password})
		if err != nil {
			return opsAccountBalanceMethodResult{}, err
		}
		accessToken = firstString(envelopeData(login.payload), "access_token", "token", "jwt")
	}
	if accessToken == "" {
		return opsAccountBalanceMethodResult{}, errors.New("sub2api management login did not return an access token")
	}
	headers := make(http.Header)
	headers.Set("Authorization", "Bearer "+accessToken)
	endpoint := accountBalanceJoinEndpoint(baseURL, "/api/v1/user/profile", false)
	profile, err := s.managementJSON(ctx, client, http.MethodGet, endpoint, headers, nil)
	if err != nil {
		return opsAccountBalanceMethodResult{}, err
	}
	balance := accountBalanceNumber(accountBalanceDataObject(profile.payload), "balance")
	if balance == nil {
		return opsAccountBalanceMethodResult{}, errors.New("sub2api management profile has no account balance")
	}
	return opsAccountBalanceMethodResult{
		Method:          AccountBalanceProbeMethodUpstreamManagement,
		Endpoint:        endpoint,
		BalanceUSD:      balance,
		BalanceAmount:   balance,
		BalanceCurrency: "USD",
	}, nil
}

func (s *UpstreamRateMultiplierSyncService) discoverySecret(account *Account, authMode UpstreamManagementAuthMode, input *UpstreamManagementAuthInput) (upstreamManagementAuthSecret, error) {
	if input != nil && (strings.TrimSpace(input.Username) != "" || input.Password != "" || strings.TrimSpace(input.AccessToken) != "") {
		return upstreamManagementAuthSecret{
			Username:    strings.TrimSpace(input.Username),
			Password:    input.Password,
			AccessToken: strings.TrimSpace(input.AccessToken),
		}, nil
	}
	if account == nil || !account.HasUpstreamManagementAuth() {
		return upstreamManagementAuthSecret{}, errors.New("enter upstream management credentials before detection")
	}
	if s.encryptor == nil {
		return upstreamManagementAuthSecret{}, errors.New("upstream management credential decryption is unavailable")
	}
	return DecryptUpstreamManagementAuth(s.encryptor, account.GetCredential(upstreamManagementAuthCredentialKey))
}

func (s *UpstreamRateMultiplierSyncService) authenticateNewAPIManagementSession(ctx context.Context, client *http.Client, baseURL string, config UpstreamRateMultiplierSyncConfig, secret upstreamManagementAuthSecret) (upstreamManagementHTTPSession, error) {
	session := upstreamManagementHTTPSession{remoteUserID: config.RemoteUserID, accessToken: secret.AccessToken}
	if config.AuthMode == UpstreamManagementAuthModePassword {
		login, err := s.managementJSON(ctx, client, http.MethodPost, accountBalanceJoinEndpoint(baseURL, "/api/user/login", false), nil, map[string]string{"username": secret.Username, "password": secret.Password})
		if err != nil {
			return upstreamManagementHTTPSession{}, err
		}
		session.cookie = login.cookieNamed("session")
		if session.cookie == nil {
			return upstreamManagementHTTPSession{}, errors.New("upstream management login did not return a session cookie")
		}
		data := envelopeData(login.payload)
		session.remoteUserID = int64FromMap(data, "id")
		session.accessToken = firstString(data, "access_token", "token")
	} else if session.remoteUserID <= 0 {
		selfHeaders := make(http.Header)
		selfHeaders.Set("Authorization", "Bearer "+session.accessToken)
		self, err := s.managementJSON(ctx, client, http.MethodGet, accountBalanceJoinEndpoint(baseURL, "/api/user/self", false), selfHeaders, nil)
		if err == nil {
			session.remoteUserID = int64FromMap(envelopeData(self.payload), "id")
		}
	}
	if session.remoteUserID <= 0 {
		return upstreamManagementHTTPSession{}, errors.New("upstream management API did not return a user id; provide a management token with a user id")
	}
	return session, nil
}

func (s *UpstreamRateMultiplierSyncService) discoverNewAPIGroups(ctx context.Context, client *http.Client, baseURL string, config UpstreamRateMultiplierSyncConfig, session upstreamManagementHTTPSession) ([]UpstreamRateMultiplierGroup, int64, error) {
	headers := newAPIManagementHeaders(config.Provider, session)
	groupsResponse, groupsErr := s.managementJSON(ctx, client, http.MethodGet, accountBalanceJoinEndpoint(baseURL, "/api/user/self/groups", false), headers, nil)
	if groupsErr == nil {
		if groups := extractUpstreamRateMultiplierGroups(envelopeData(groupsResponse.payload)); len(groups) > 0 {
			return groups, session.remoteUserID, nil
		}
	}
	pricingResponse, pricingErr := s.managementJSON(ctx, client, http.MethodGet, accountBalanceJoinEndpoint(baseURL, "/api/pricing", false), headers, nil)
	if pricingErr == nil {
		if groups := extractUpstreamRateMultiplierGroups(envelopeData(pricingResponse.payload)); len(groups) > 0 {
			return groups, session.remoteUserID, nil
		}
	}
	if groupsErr != nil {
		return nil, 0, groupsErr
	}
	if pricingErr != nil {
		return nil, 0, pricingErr
	}
	return nil, 0, errors.New("upstream management API returned no usable groups")
}

func (s *UpstreamRateMultiplierSyncService) discoverSub2APIGroups(ctx context.Context, client *http.Client, baseURL string, config UpstreamRateMultiplierSyncConfig, secret upstreamManagementAuthSecret, upstreamAPIKey string) ([]UpstreamRateMultiplierGroup, int64, *UpstreamRateMultiplierGroup, error) {
	accessToken := secret.AccessToken
	if config.AuthMode == UpstreamManagementAuthModePassword {
		login, err := s.managementJSON(ctx, client, http.MethodPost, accountBalanceJoinEndpoint(baseURL, "/api/v1/auth/login", false), nil, map[string]string{"email": secret.Username, "password": secret.Password})
		if err != nil {
			return nil, 0, nil, err
		}
		accessToken = firstString(envelopeData(login.payload), "access_token", "token", "jwt")
		if accessToken == "" {
			return nil, 0, nil, errors.New("sub2api management login did not return an access token")
		}
	}
	headers := make(http.Header)
	headers.Set("Authorization", "Bearer "+accessToken)
	availableResponse, availableErr := s.managementJSON(ctx, client, http.MethodGet, accountBalanceJoinEndpoint(baseURL, "/api/v1/groups/available", false), headers, nil)
	if availableErr != nil {
		return nil, 0, nil, availableErr
	}
	groups := extractSub2APIAvailableGroups(envelopeData(availableResponse.payload))
	if len(groups) == 0 {
		return nil, 0, nil, errors.New("sub2api management API returned no usable groups")
	}

	// The rate endpoint is keyed by numeric group ID. Applying it after the
	// available-groups response preserves the exact ID -> name relationship.
	if ratesResponse, ratesErr := s.managementJSON(ctx, client, http.MethodGet, accountBalanceJoinEndpoint(baseURL, "/api/v1/groups/rates", false), headers, nil); ratesErr == nil {
		applySub2APIGroupRates(groups, envelopeData(ratesResponse.payload))
	}
	groups = mergeUpstreamRateMultiplierGroups(groups)
	if strings.TrimSpace(upstreamAPIKey) == "" {
		return groups, 0, nil, nil
	}

	matchedGroupID, err := s.findSub2APIKeyGroupID(ctx, client, baseURL, headers, upstreamAPIKey)
	if err != nil {
		return nil, 0, nil, err
	}
	for index := range groups {
		if groups[index].ID == matchedGroupID {
			matched := groups[index]
			return groups, 0, &matched, nil
		}
	}
	return nil, 0, nil, fmt.Errorf("%w: upstream API key belongs to group id %d, but that group is not available to the management user", ErrUpstreamAPIKeyGroupUnmapped, matchedGroupID)
}

func (s *UpstreamRateMultiplierSyncService) fetchGroupRateMultiplier(ctx context.Context, target *upstreamRateMultiplierTarget) (float64, error) {
	secret, err := DecryptUpstreamManagementAuth(s.encryptor, target.ciphertext)
	if err != nil {
		return 0, err
	}
	if err := validateUpstreamManagementAuth(target.config, secret); err != nil {
		return 0, err
	}
	switch target.config.Provider {
	case UpstreamManagementProviderNewAPI, UpstreamManagementProviderRixAPI, UpstreamManagementProviderShellAPI:
		return s.fetchNewAPIGroupRateMultiplier(ctx, target.client, target.baseURL, target.config, secret)
	case UpstreamManagementProviderSub2API:
		return s.fetchSub2APIGroupRateMultiplier(ctx, target.client, target.baseURL, target.config, secret)
	default:
		return 0, errors.New("unsupported upstream management provider")
	}
}

func (s *UpstreamRateMultiplierSyncService) fetchNewAPIGroupRateMultiplier(ctx context.Context, client *http.Client, baseURL string, config UpstreamRateMultiplierSyncConfig, secret upstreamManagementAuthSecret) (float64, error) {
	session := upstreamManagementHTTPSession{remoteUserID: config.RemoteUserID, accessToken: secret.AccessToken}
	if config.AuthMode == UpstreamManagementAuthModePassword {
		login, err := s.managementJSON(ctx, client, http.MethodPost, accountBalanceJoinEndpoint(baseURL, "/api/user/login", false), nil, map[string]string{"username": secret.Username, "password": secret.Password})
		if err != nil {
			return 0, err
		}
		session.cookie = login.cookieNamed("session")
		if session.cookie == nil {
			return 0, errors.New("upstream management login did not return a session cookie")
		}
		data := envelopeData(login.payload)
		session.remoteUserID = int64FromMap(data, "id")
		session.accessToken = firstString(data, "access_token", "token")
		if session.remoteUserID <= 0 {
			return 0, errors.New("upstream management login did not return a user id")
		}
	}
	headers := newAPIManagementHeaders(config.Provider, session)
	groupsResponse, groupErr := s.managementJSON(ctx, client, http.MethodGet, accountBalanceJoinEndpoint(baseURL, "/api/user/self/groups", false), headers, nil)
	if groupErr == nil {
		if multiplier, found := findGroupMultiplier(envelopeData(groupsResponse.payload), config.Group); found {
			return multiplier, nil
		}
	}
	pricingResponse, pricingErr := s.managementJSON(ctx, client, http.MethodGet, accountBalanceJoinEndpoint(baseURL, "/api/pricing", false), headers, nil)
	if pricingErr != nil {
		if groupErr != nil {
			return 0, groupErr
		}
		return 0, pricingErr
	}
	if multiplier, found := findGroupMultiplier(envelopeData(pricingResponse.payload), config.Group); found {
		return multiplier, nil
	}
	return 0, fmt.Errorf("upstream group %q is absent from management pricing", config.Group)
}

func newAPIManagementHeaders(provider UpstreamManagementProvider, session upstreamManagementHTTPSession) http.Header {
	headers := make(http.Header)
	if session.remoteUserID > 0 {
		userID := strconv.FormatInt(session.remoteUserID, 10)
		headers.Set("New-API-User", userID)
		// RixAPI accepts the NewAPI header too and additionally requires its
		// vendor-specific identity header. Sending both matches its management UI.
		if provider == UpstreamManagementProviderRixAPI {
			headers.Set("Rix-Api-User", userID)
		}
	}
	if session.accessToken != "" {
		headers.Set("Authorization", "Bearer "+session.accessToken)
	}
	if session.cookie != nil {
		headers.Add("Cookie", session.cookie.Name+"="+session.cookie.Value)
	}
	return headers
}

func (s *UpstreamRateMultiplierSyncService) fetchSub2APIGroupRateMultiplier(ctx context.Context, client *http.Client, baseURL string, config UpstreamRateMultiplierSyncConfig, secret upstreamManagementAuthSecret) (float64, error) {
	accessToken := secret.AccessToken
	if config.AuthMode == UpstreamManagementAuthModePassword {
		login, err := s.managementJSON(ctx, client, http.MethodPost, accountBalanceJoinEndpoint(baseURL, "/api/v1/auth/login", false), nil, map[string]string{"email": secret.Username, "password": secret.Password})
		if err != nil {
			return 0, err
		}
		accessToken = firstString(envelopeData(login.payload), "access_token", "token", "jwt")
		if accessToken == "" {
			return 0, errors.New("sub2api management login did not return an access token")
		}
	}
	headers := make(http.Header)
	headers.Set("Authorization", "Bearer "+accessToken)
	availableResponse, availableErr := s.managementJSON(ctx, client, http.MethodGet, accountBalanceJoinEndpoint(baseURL, "/api/v1/groups/available", false), headers, nil)
	if availableErr != nil {
		return 0, availableErr
	}
	groups := extractSub2APIAvailableGroups(envelopeData(availableResponse.payload))
	matched := -1
	for index := range groups {
		if groups[index].Name == config.Group {
			matched = index
			break
		}
	}
	if matched < 0 || groups[matched].ID <= 0 {
		return 0, fmt.Errorf("upstream group %q is absent from sub2api available groups", config.Group)
	}

	// /groups/rates is keyed by numeric group ID, while the persisted account
	// configuration deliberately stores the stable human-readable group name.
	// Resolve name -> ID through /groups/available before applying this user's
	// rate table; never fall back to a global/default group ratio.
	ratesResponse, ratesErr := s.managementJSON(ctx, client, http.MethodGet, accountBalanceJoinEndpoint(baseURL, "/api/v1/groups/rates", false), headers, nil)
	if ratesErr != nil {
		return 0, ratesErr
	}
	applySub2APIGroupRates(groups, envelopeData(ratesResponse.payload))
	return groups[matched].RateMultiplier, nil
}

type managementJSONResponse struct {
	payload map[string]any
	cookies []*http.Cookie
}

func (r managementJSONResponse) cookieNamed(name string) *http.Cookie {
	for _, cookie := range r.cookies {
		if cookie != nil && cookie.Name == name {
			return cookie
		}
	}
	return nil
}

func (s *UpstreamRateMultiplierSyncService) managementJSON(ctx context.Context, client *http.Client, method, endpoint string, headers http.Header, body any) (managementJSONResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, upstreamRateMultiplierRequestTimeout)
	defer cancel()
	var bodyReader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return managementJSONResponse{}, err
		}
		bodyReader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(requestCtx, method, endpoint, bodyReader)
	if err != nil {
		return managementJSONResponse{}, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	if client == nil {
		client = s.client
	}
	if client == nil {
		return managementJSONResponse{}, errors.New("upstream management http client is unavailable")
	}
	response, err := client.Do(req)
	if err != nil {
		return managementJSONResponse{}, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return managementJSONResponse{}, fmt.Errorf("upstream management request returned HTTP %d", response.StatusCode)
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, upstreamRateMultiplierResponseLimit+1))
	if err != nil {
		return managementJSONResponse{}, err
	}
	if len(payload) > upstreamRateMultiplierResponseLimit {
		return managementJSONResponse{}, fmt.Errorf("upstream management response exceeds %d bytes", upstreamRateMultiplierResponseLimit)
	}
	decoded := make(map[string]any)
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return managementJSONResponse{}, errors.New("upstream management response is not valid JSON")
	}
	if success, ok := decoded["success"].(bool); ok && !success {
		return managementJSONResponse{}, errors.New("upstream management request was rejected")
	}
	if _, ok := decoded["error"]; ok {
		return managementJSONResponse{}, errors.New("upstream management request returned an error")
	}
	return managementJSONResponse{payload: decoded, cookies: response.Cookies()}, nil
}

func envelopeData(payload map[string]any) any {
	if payload == nil {
		return nil
	}
	if data, ok := payload["data"]; ok {
		return data
	}
	return payload
}

func findGroupMultiplier(payload any, group string) (float64, bool) {
	group = strings.TrimSpace(group)
	if group == "" {
		return 0, false
	}
	if mapPayload, ok := payload.(map[string]any); ok {
		for _, key := range []string{"group_data", "group_ratio", "GroupRatio", "group_info", "rates"} {
			if nested, exists := mapPayload[key]; exists {
				if multiplier, found := findGroupMultiplier(nested, group); found {
					return multiplier, true
				}
			}
		}
		if value, ok := mapPayload[group]; ok {
			return parseGroupMultiplier(value)
		}
		for name, value := range mapPayload {
			row, ok := value.(map[string]any)
			if !ok || firstString(row, "name", "group", "group_name", "id") != group && name != group {
				continue
			}
			if multiplier, found := parseGroupMultiplier(row); found {
				return multiplier, true
			}
		}
	}
	if listPayload, ok := payload.([]any); ok {
		for _, value := range listPayload {
			row, ok := value.(map[string]any)
			if !ok || firstString(row, "name", "group", "group_name", "id") != group {
				continue
			}
			return parseGroupMultiplier(row)
		}
	}
	return 0, false
}

func extractUpstreamRateMultiplierGroups(payload any) []UpstreamRateMultiplierGroup {
	groups := make([]UpstreamRateMultiplierGroup, 0)
	var collect func(any)
	collect = func(value any) {
		switch current := value.(type) {
		case []any:
			for _, item := range current {
				collect(item)
			}
		case map[string]any:
			if name := firstString(current, "name", "group", "group_name", "id"); name != "" {
				if multiplier, ok := parseGroupMultiplier(current); ok {
					groups = append(groups, UpstreamRateMultiplierGroup{ID: int64FromMap(current, "id"), Name: name, RateMultiplier: multiplier})
					return
				}
			}
			for _, key := range []string{"data", "group_data", "group_ratio", "GroupRatio", "group_info", "rates", "groups"} {
				if nested, exists := current[key]; exists {
					collect(nested)
				}
			}
			for name, nested := range current {
				if name == "data" || name == "group_data" || name == "group_ratio" || name == "GroupRatio" || name == "group_info" || name == "rates" || name == "groups" {
					continue
				}
				if multiplier, ok := parseGroupMultiplier(nested); ok {
					groups = append(groups, UpstreamRateMultiplierGroup{Name: name, RateMultiplier: multiplier})
					continue
				}
				collect(nested)
			}
		}
	}
	collect(payload)
	return mergeUpstreamRateMultiplierGroups(groups)
}

func mergeUpstreamRateMultiplierGroups(groups []UpstreamRateMultiplierGroup) []UpstreamRateMultiplierGroup {
	byName := make(map[string]UpstreamRateMultiplierGroup, len(groups))
	for _, group := range groups {
		group.Name = strings.TrimSpace(group.Name)
		if group.Name == "" || math.IsNaN(group.RateMultiplier) || math.IsInf(group.RateMultiplier, 0) || group.RateMultiplier < 0 {
			continue
		}
		if existing, ok := byName[group.Name]; !ok || (existing.ID == 0 && group.ID > 0) {
			byName[group.Name] = group
		}
	}
	result := make([]UpstreamRateMultiplierGroup, 0, len(byName))
	for _, group := range byName {
		result = append(result, group)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].RateMultiplier != result[j].RateMultiplier {
			return result[i].RateMultiplier < result[j].RateMultiplier
		}
		return result[i].Name < result[j].Name
	})
	return result
}

func extractSub2APIAvailableGroups(payload any) []UpstreamRateMultiplierGroup {
	items := upstreamManagementItems(payload)
	groups := make([]UpstreamRateMultiplierGroup, 0, len(items))
	for _, item := range items {
		row, ok := item.(map[string]any)
		if !ok {
			continue
		}
		groupID := int64FromMap(row, "id")
		name := firstString(row, "name", "group", "group_name")
		if name == "" {
			continue
		}
		multiplier, ok := parseGroupMultiplier(row)
		if !ok {
			// A missing multiplier is not a reason to discard the group: the
			// user-specific rate table can still supply it below.
			multiplier = 1
		}
		groups = append(groups, UpstreamRateMultiplierGroup{ID: groupID, Name: name, RateMultiplier: multiplier})
	}
	return groups
}

func applySub2APIGroupRates(groups []UpstreamRateMultiplierGroup, payload any) {
	rates, ok := payload.(map[string]any)
	if !ok {
		return
	}
	for index := range groups {
		raw, exists := rates[strconv.FormatInt(groups[index].ID, 10)]
		if !exists {
			continue
		}
		if multiplier, valid := parseGroupMultiplier(raw); valid {
			groups[index].RateMultiplier = multiplier
		}
	}
}

func (s *UpstreamRateMultiplierSyncService) findSub2APIKeyGroupID(ctx context.Context, client *http.Client, baseURL string, headers http.Header, upstreamAPIKey string) (int64, error) {
	const pageSize = 100
	for page := 1; page <= 100; page++ {
		endpoint := accountBalanceJoinEndpoint(baseURL, "/api/v1/api-keys", false)
		parsedEndpoint, err := url.Parse(endpoint)
		if err != nil {
			return 0, err
		}
		query := parsedEndpoint.Query()
		query.Set("page", strconv.Itoa(page))
		query.Set("page_size", strconv.Itoa(pageSize))
		parsedEndpoint.RawQuery = query.Encode()
		endpoint = parsedEndpoint.String()
		response, err := s.managementJSON(ctx, client, http.MethodGet, endpoint, headers, nil)
		if err != nil {
			return 0, err
		}
		data := envelopeData(response.payload)
		items := upstreamManagementItems(data)
		for _, item := range items {
			row, ok := item.(map[string]any)
			if !ok || firstString(row, "key", "api_key", "token") != upstreamAPIKey {
				continue
			}
			groupID := int64FromMap(row, "group_id", "groupId")
			if groupID <= 0 {
				return 0, fmt.Errorf("%w: upstream API key has no assigned group", ErrUpstreamAPIKeyGroupUnmapped)
			}
			return groupID, nil
		}
		pages := upstreamManagementPageCount(data)
		if len(items) < pageSize || (pages > 0 && page >= pages) {
			break
		}
	}
	return 0, fmt.Errorf("%w: upstream API key was not found under the configured management user", ErrUpstreamAPIKeyGroupUnmapped)
}

func upstreamManagementItems(payload any) []any {
	if list, ok := payload.([]any); ok {
		return list
	}
	row, ok := payload.(map[string]any)
	if !ok {
		return nil
	}
	for _, key := range []string{"items", "list", "results"} {
		if list, ok := row[key].([]any); ok {
			return list
		}
	}
	return nil
}

func upstreamManagementPageCount(payload any) int {
	row, ok := payload.(map[string]any)
	if !ok {
		return 0
	}
	pages := int64FromMap(row, "pages", "page_count")
	if pages > 0 {
		return int(pages)
	}
	return 0
}

func parseGroupMultiplier(value any) (float64, bool) {
	if row, ok := value.(map[string]any); ok {
		for _, key := range []string{"ratio", "GroupRatio", "rate", "rate_multiplier", "multiplier"} {
			if raw, exists := row[key]; exists {
				return parseGroupMultiplier(raw)
			}
		}
		return 0, false
	}
	var multiplier float64
	switch raw := value.(type) {
	case float64:
		multiplier = raw
	case float32:
		multiplier = float64(raw)
	case int:
		multiplier = float64(raw)
	case int64:
		multiplier = float64(raw)
	case json.Number:
		parsed, err := raw.Float64()
		if err != nil {
			return 0, false
		}
		multiplier = parsed
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
		if err != nil {
			return 0, false
		}
		multiplier = parsed
	default:
		return 0, false
	}
	if math.IsNaN(multiplier) || math.IsInf(multiplier, 0) || multiplier < 0 {
		return 0, false
	}
	return multiplier, true
}

func int64FromMap(value any, keys ...string) int64 {
	row, ok := value.(map[string]any)
	if !ok {
		return 0
	}
	for _, key := range keys {
		parsed, err := upstreamManagementRemoteUserID(row[key])
		if err == nil && parsed > 0 {
			return parsed
		}
	}
	return 0
}

func firstString(value any, keys ...string) string {
	row, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	for _, key := range keys {
		if raw, ok := row[key]; ok {
			if text, ok := raw.(string); ok && strings.TrimSpace(text) != "" {
				return strings.TrimSpace(text)
			}
		}
	}
	return ""
}
