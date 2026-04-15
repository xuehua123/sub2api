package service

import (
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/net/publicsuffix"
)

const lobeHubSettingsSchemaVersion = "settings-url-share-v1"

type LobeHubUserPreferenceStore interface {
	GetByID(ctx context.Context, id int64) (*User, error)
	UpdateDefaultChatAPIKeyID(ctx context.Context, userID int64, apiKeyID *int64) error
}

type LobeHubSSOContinuationRequest struct {
	ResumeToken string
	ReturnURL   string
	APIKeyID    *int64
}

type LobeHubSSORefreshRequest struct {
	ReturnURL string
	APIKeyID  *int64
}

type LobeHubSSOContinuationResult struct {
	ContinueURL       string `json:"continue_url"`
	OIDCSessionID     string `json:"oidc_session_id,omitempty"`
	TargetToken       string `json:"target_token"`
	BootstrapTicketID string `json:"bootstrap_ticket_id"`
	CookieDomain      string `json:"cookie_domain,omitempty"`
}

type LobeHubBootstrapExchangeResult struct {
	BootstrapTicketID string `json:"bootstrap_ticket_id"`
}

type LobeHubBootstrapConsumeResult struct {
	RedirectURL   string            `json:"redirect_url"`
	ProviderID    string            `json:"provider_id"`
	FetchOnClient bool              `json:"fetch_on_client"`
	KeyVaults     map[string]string `json:"key_vaults"`
}

type LobeHubObservedSettings struct {
	KeyVaults     map[string]LobeHubObservedKeyVault      `json:"key_vaults"`
	LanguageModel map[string]LobeHubObservedLanguageModel `json:"language_model"`
}

type LobeHubObservedKeyVault struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
}

type LobeHubObservedLanguageModel struct {
	Enabled       bool     `json:"enabled"`
	EnabledModels []string `json:"enabled_models"`
}

type LobeHubConfigProbeResult struct {
	Matched                  bool   `json:"matched"`
	DesiredConfigFingerprint string `json:"desired_config_fingerprint,omitempty"`
	CurrentConfigFingerprint string `json:"current_config_fingerprint,omitempty"`
}

type lobeHubTargetTokenClaims struct {
	UserID                   int64  `json:"user_id"`
	APIKeyID                 int64  `json:"api_key_id"`
	DesiredConfigFingerprint string `json:"desired_config_fingerprint"`
	RuntimeConfigVersion     string `json:"runtime_config_version"`
	jwt.RegisteredClaims
}

type LobeHubSSOService struct {
	settingsReader     LobeHubSettingsReader
	userStore          LobeHubUserPreferenceStore
	apiKeyReader       LobeHubAPIKeyReader
	stateStore         LobeHubOIDCStateStore
	signingKeyProvider LobeHubOIDCSigningKeyProvider
	now                func() time.Time
}

func NewLobeHubSSOService(
	settingsReader LobeHubSettingsReader,
	userStore LobeHubUserPreferenceStore,
	apiKeyReader LobeHubAPIKeyReader,
	stateStore LobeHubOIDCStateStore,
	signingKeyProvider LobeHubOIDCSigningKeyProvider,
	now func() time.Time,
) *LobeHubSSOService {
	if now == nil {
		now = time.Now
	}
	return &LobeHubSSOService{
		settingsReader:     settingsReader,
		userStore:          userStore,
		apiKeyReader:       apiKeyReader,
		stateStore:         stateStore,
		signingKeyProvider: signingKeyProvider,
		now:                now,
	}
}

func (s *LobeHubSSOService) PrepareOIDCContinuation(ctx context.Context, userID int64, req *LobeHubSSOContinuationRequest) (*LobeHubSSOContinuationResult, error) {
	if req == nil {
		return nil, ErrLobeHubOIDCInvalidRequest
	}
	resume, err := s.stateStore.GetResumeToken(ctx, strings.TrimSpace(req.ResumeToken))
	if err != nil {
		return nil, ErrLobeHubOIDCResumeTokenNotFound
	}

	return s.prepare(ctx, userID, req.ReturnURL, req.APIKeyID, resume, true)
}

func (s *LobeHubSSOService) PrepareTargetRefresh(ctx context.Context, userID int64, req *LobeHubSSORefreshRequest) (*LobeHubSSOContinuationResult, error) {
	if req == nil {
		return nil, ErrLobeHubInvalidReturnURL
	}
	return s.prepare(ctx, userID, req.ReturnURL, req.APIKeyID, nil, false)
}

func (s *LobeHubSSOService) ExchangeBootstrap(ctx context.Context, targetToken string, returnURL string) (*LobeHubBootstrapExchangeResult, error) {
	cfg, err := s.getSettings(ctx)
	if err != nil {
		return nil, err
	}
	claims, err := s.parseTargetToken(ctx, strings.TrimSpace(targetToken))
	if err != nil {
		return nil, err
	}
	resolvedReturnURL, err := sanitizeLobeHubReturnURL(cfg.LobeHubChatURL, returnURL)
	if err != nil {
		return nil, err
	}
	bootstrapTicketID, err := s.stateStore.CreateBootstrapTicket(ctx, &LobeHubBootstrapTicket{
		UserID:                   claims.UserID,
		APIKeyID:                 claims.APIKeyID,
		DesiredConfigFingerprint: claims.DesiredConfigFingerprint,
		RuntimeConfigVersion:     claims.RuntimeConfigVersion,
		ReturnURL:                resolvedReturnURL,
		CreatedAt:                s.now().UTC(),
	}, LobeHubBootstrapTicketTTL)
	if err != nil {
		return nil, err
	}
	return &LobeHubBootstrapExchangeResult{BootstrapTicketID: bootstrapTicketID}, nil
}

func (s *LobeHubSSOService) ConsumeBootstrap(ctx context.Context, ticketID string) (*LobeHubBootstrapConsumeResult, error) {
	cfg, err := s.getSettings(ctx)
	if err != nil {
		return nil, err
	}
	ticket, err := s.stateStore.ConsumeBootstrapTicket(ctx, strings.TrimSpace(ticketID))
	if err != nil {
		return nil, ErrLobeHubBootstrapTicketNotFound
	}
	if strings.TrimSpace(ticket.RuntimeConfigVersion) != strings.TrimSpace(cfg.LobeHubRuntimeConfigVersion) {
		return nil, ErrLobeHubInvalidTargetToken
	}

	apiKey, err := s.apiKeyReader.GetByID(ctx, ticket.APIKeyID)
	if err != nil {
		return nil, err
	}
	if apiKey == nil || apiKey.UserID != ticket.UserID || !isLobeHubAPIKeyUsable(apiKey) {
		return nil, ErrLobeHubAPIKeyNotOwned
	}

	providerID := normalizeLobeHubProvider(settingsProvider(cfg))
	redirectURL, err := buildLobeHubRedirectURL(ticket.ReturnURL, buildLobeHubSettingsPayload(cfg))
	if err != nil {
		return nil, err
	}
	return &LobeHubBootstrapConsumeResult{
		RedirectURL:   redirectURL,
		ProviderID:    providerID,
		FetchOnClient: false,
		KeyVaults: map[string]string{
			"apiKey":  strings.TrimSpace(apiKey.Key),
			"baseURL": normalizeLobeHubBaseURL(cfg.APIBaseURL),
		},
	}, nil
}

func (s *LobeHubSSOService) CompareCurrentConfig(ctx context.Context, targetToken string, observed *LobeHubObservedSettings) (*LobeHubConfigProbeResult, error) {
	cfg, err := s.getSettings(ctx)
	if err != nil {
		return nil, err
	}
	claims, err := s.parseTargetToken(ctx, strings.TrimSpace(targetToken))
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(claims.RuntimeConfigVersion) != strings.TrimSpace(cfg.LobeHubRuntimeConfigVersion) {
		return nil, ErrLobeHubInvalidTargetToken
	}

	apiKey, err := s.apiKeyReader.GetByID(ctx, claims.APIKeyID)
	if err != nil {
		return nil, err
	}
	if apiKey == nil || apiKey.UserID != claims.UserID || !isLobeHubAPIKeyUsable(apiKey) {
		return nil, ErrLobeHubAPIKeyNotOwned
	}

	matched := lobeHubObservedSettingsMatch(cfg, apiKey, observed)
	result := &LobeHubConfigProbeResult{
		Matched:                  matched,
		DesiredConfigFingerprint: strings.TrimSpace(claims.DesiredConfigFingerprint),
	}
	if matched {
		result.CurrentConfigFingerprint = result.DesiredConfigFingerprint
	}
	return result, nil
}

func (s *LobeHubSSOService) prepare(
	ctx context.Context,
	userID int64,
	rawReturnURL string,
	explicitAPIKeyID *int64,
	resume *LobeHubOIDCResumeToken,
	createWebSession bool,
) (*LobeHubSSOContinuationResult, error) {
	cfg, err := s.getSettings(ctx)
	if err != nil {
		return nil, err
	}
	user, err := s.userStore.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil || !user.IsActive() {
		return nil, ErrUserNotFound
	}

	returnURL, err := sanitizeLobeHubReturnURL(cfg.LobeHubChatURL, rawReturnURL)
	if err != nil {
		return nil, err
	}

	apiKey, err := s.resolveAPIKey(ctx, user, explicitAPIKeyID)
	if err != nil {
		return nil, err
	}
	if resume != nil {
		resume.ReturnURL = returnURL
	}

	fingerprint := buildLobeHubDesiredConfigFingerprint(userID, apiKey.ID, cfg)
	targetToken, err := s.signTargetToken(ctx, &lobeHubTargetTokenClaims{
		UserID:                   userID,
		APIKeyID:                 apiKey.ID,
		DesiredConfigFingerprint: fingerprint,
		RuntimeConfigVersion:     strings.TrimSpace(cfg.LobeHubRuntimeConfigVersion),
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(s.now().UTC()),
			ExpiresAt: jwt.NewNumericDate(s.now().UTC().Add(LobeHubTargetTokenTTL)),
		},
	})
	if err != nil {
		return nil, err
	}

	bootstrapTicketID, err := s.stateStore.CreateBootstrapTicket(ctx, &LobeHubBootstrapTicket{
		UserID:                   userID,
		APIKeyID:                 apiKey.ID,
		DesiredConfigFingerprint: fingerprint,
		RuntimeConfigVersion:     strings.TrimSpace(cfg.LobeHubRuntimeConfigVersion),
		ReturnURL:                returnURL,
		CreatedAt:                s.now().UTC(),
	}, LobeHubBootstrapTicketTTL)
	if err != nil {
		return nil, err
	}

	continueURL := returnURL
	webSessionID := ""
	if createWebSession {
		webSessionID, err = s.stateStore.CreateWebSession(ctx, &LobeHubOIDCWebSession{
			UserID:    userID,
			APIKeyID:  apiKey.ID,
			CreatedAt: s.now().UTC(),
		}, LobeHubOIDCWebSessionTTL)
		if err != nil {
			return nil, err
		}
		continueURL = buildResumeAuthorizeURL(cfg.LobeHubOIDCIssuer, resume)
	}

	return &LobeHubSSOContinuationResult{
		ContinueURL:       continueURL,
		OIDCSessionID:     webSessionID,
		TargetToken:       targetToken,
		BootstrapTicketID: bootstrapTicketID,
		CookieDomain:      resolveSharedCookieDomain(cfg.LobeHubChatURL),
	}, nil
}

func (s *LobeHubSSOService) resolveAPIKey(ctx context.Context, user *User, explicitAPIKeyID *int64) (*APIKey, error) {
	if explicitAPIKeyID != nil && *explicitAPIKeyID > 0 {
		apiKey, err := s.lookupUsableAPIKey(ctx, *explicitAPIKeyID, user.ID)
		if err != nil {
			return nil, err
		}
		if err := s.userStore.UpdateDefaultChatAPIKeyID(ctx, user.ID, explicitAPIKeyID); err != nil {
			return nil, err
		}
		return apiKey, nil
	}
	if user.DefaultChatAPIKeyID == nil || *user.DefaultChatAPIKeyID <= 0 {
		return nil, ErrLobeHubDefaultChatAPIKeyRequired
	}
	apiKey, err := s.lookupUsableAPIKey(ctx, *user.DefaultChatAPIKeyID, user.ID)
	if err == nil {
		return apiKey, nil
	}
	if errors.Is(err, ErrLobeHubDefaultChatAPIKeyRequired) {
		if clearErr := s.userStore.UpdateDefaultChatAPIKeyID(ctx, user.ID, nil); clearErr != nil {
			return nil, clearErr
		}
	}
	return nil, err
}

func (s *LobeHubSSOService) getSettings(ctx context.Context) (*SystemSettings, error) {
	settings, err := s.settingsReader.GetAllSettings(ctx)
	if err != nil {
		return nil, err
	}
	if settings == nil || !settings.LobeHubEnabled {
		return nil, ErrLobeHubDisabled
	}
	if strings.TrimSpace(settings.LobeHubChatURL) == "" || strings.TrimSpace(settings.LobeHubOIDCIssuer) == "" {
		return nil, ErrLobeHubOIDCConfigInvalid
	}
	if normalizeLobeHubBaseURL(settings.APIBaseURL) == "" {
		return nil, ErrLobeHubAPIBaseURLNotConfigured
	}
	if strings.TrimSpace(settings.LobeHubDefaultProvider) == "" {
		settings.LobeHubDefaultProvider = "openai"
	}
	return settings, nil
}

func (s *LobeHubSSOService) signTargetToken(ctx context.Context, claims *lobeHubTargetTokenClaims) (string, error) {
	privateKey, _, err := s.signingKeyProvider.GetSigningKey(ctx)
	if err != nil {
		return "", err
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(lobeHubTargetSigningKey(privateKey))
}

func (s *LobeHubSSOService) parseTargetToken(ctx context.Context, raw string) (*lobeHubTargetTokenClaims, error) {
	privateKey, _, err := s.signingKeyProvider.GetSigningKey(ctx)
	if err != nil {
		return nil, err
	}
	claims := &lobeHubTargetTokenClaims{}
	parsed, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) {
		return lobeHubTargetSigningKey(privateKey), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithTimeFunc(s.now))
	if err != nil || !parsed.Valid {
		return nil, ErrLobeHubInvalidTargetToken
	}
	return claims, nil
}

func buildResumeAuthorizeURL(issuer string, resume *LobeHubOIDCResumeToken) string {
	base := strings.TrimRight(strings.TrimSpace(issuer), "/") + "/authorize"
	if resume == nil {
		return base
	}
	values := url.Values{}
	values.Set("client_id", strings.TrimSpace(resume.ClientID))
	values.Set("redirect_uri", strings.TrimSpace(resume.RedirectURI))
	values.Set("response_type", strings.TrimSpace(resume.ResponseType))
	values.Set("scope", normalizeScope(resume.Scope))
	if strings.TrimSpace(resume.State) != "" {
		values.Set("state", strings.TrimSpace(resume.State))
	}
	if strings.TrimSpace(resume.Nonce) != "" {
		values.Set("nonce", strings.TrimSpace(resume.Nonce))
	}
	if strings.TrimSpace(resume.ReturnURL) != "" {
		values.Set("return_url", strings.TrimSpace(resume.ReturnURL))
	}
	if strings.TrimSpace(resume.CodeChallenge) != "" {
		values.Set("code_challenge", strings.TrimSpace(resume.CodeChallenge))
	}
	if strings.TrimSpace(resume.CodeChallengeMethod) != "" {
		values.Set("code_challenge_method", strings.TrimSpace(resume.CodeChallengeMethod))
	}
	return base + "?" + values.Encode()
}

func (s *LobeHubSSOService) lookupUsableAPIKey(ctx context.Context, apiKeyID int64, userID int64) (*APIKey, error) {
	apiKey, err := s.apiKeyReader.GetByID(ctx, apiKeyID)
	if err != nil {
		if errors.Is(err, ErrAPIKeyNotFound) {
			return nil, ErrLobeHubDefaultChatAPIKeyRequired
		}
		return nil, err
	}
	if apiKey == nil || apiKey.UserID != userID || !isLobeHubAPIKeyUsable(apiKey) {
		return nil, ErrLobeHubDefaultChatAPIKeyRequired
	}
	return apiKey, nil
}

func buildLobeHubDesiredConfigFingerprint(userID, apiKeyID int64, settings *SystemSettings) string {
	baseURL := normalizeLobeHubBaseURL(settings.APIBaseURL)
	payload := strings.Join([]string{
		strconv.FormatInt(userID, 10),
		strconv.FormatInt(apiKeyID, 10),
		baseURL,
		normalizeLobeHubProvider(settingsProvider(settings)),
		lobeHubSettingsSchemaVersion,
		strings.TrimSpace(settings.LobeHubRuntimeConfigVersion),
	}, "|")
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

func buildLobeHubSettingsPayload(settings *SystemSettings) map[string]any {
	provider := normalizeLobeHubProvider(settingsProvider(settings))
	model := strings.TrimSpace(settings.LobeHubDefaultModel)

	payload := map[string]any{}
	if model != "" {
		payload["defaultAgent"] = map[string]any{
			"config": map[string]any{
				"model":    model,
				"provider": provider,
			},
		}
		payload["languageModel"] = map[string]any{
			provider: map[string]any{
				"enabled":       true,
				"enabledModels": []string{model},
			},
		}
	}
	return payload
}

func buildLobeHubRedirectURL(returnURL string, settingsPayload map[string]any) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(returnURL))
	if err != nil {
		return "", err
	}
	settingsJSON, err := json.Marshal(settingsPayload)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set("settings", string(settingsJSON))
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func sanitizeLobeHubReturnURL(chatURL string, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return resolveURL(chatURL, "/")
	}
	if strings.HasPrefix(raw, "/") {
		return resolveURL(chatURL, raw)
	}
	chatParsed, err := url.Parse(strings.TrimSpace(chatURL))
	if err != nil {
		return "", ErrLobeHubInvalidReturnURL
	}
	returnParsed, err := url.Parse(raw)
	if err != nil || (returnParsed.Scheme != "http" && returnParsed.Scheme != "https") {
		return "", ErrLobeHubInvalidReturnURL
	}
	if !strings.EqualFold(returnParsed.Scheme, chatParsed.Scheme) || !strings.EqualFold(returnParsed.Host, chatParsed.Host) {
		return "", ErrLobeHubInvalidReturnURL
	}
	return returnParsed.String(), nil
}

func resolveSharedCookieDomain(chatURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(chatURL))
	if err != nil {
		return ""
	}
	host := parsed.Hostname()
	if host == "" || strings.EqualFold(host, "localhost") || net.ParseIP(host) != nil {
		return ""
	}
	registrableDomain, err := publicsuffix.EffectiveTLDPlusOne(host)
	if err != nil || registrableDomain == "" {
		return ""
	}
	if strings.EqualFold(host, registrableDomain) {
		return ""
	}
	return "." + registrableDomain
}

func lobeHubTargetSigningKey(privateKey *rsa.PrivateKey) []byte {
	if privateKey == nil || privateKey.D == nil {
		return []byte("lobehub-target")
	}
	sum := sha256.Sum256(privateKey.D.Bytes())
	return sum[:]
}

func isLobeHubAPIKeyUsable(apiKey *APIKey) bool {
	return apiKey != nil && apiKey.IsActive() && !apiKey.IsExpired() && !apiKey.IsQuotaExhausted()
}

func lobeHubObservedSettingsMatch(settings *SystemSettings, apiKey *APIKey, observed *LobeHubObservedSettings) bool {
	if settings == nil || apiKey == nil || observed == nil {
		return false
	}

	provider := normalizeLobeHubProvider(settingsProvider(settings))

	vault, ok := observed.KeyVaults[provider]
	if !ok {
		return false
	}
	if strings.TrimSpace(vault.APIKey) != strings.TrimSpace(apiKey.Key) {
		return false
	}
	if normalizeLobeHubBaseURL(vault.BaseURL) != normalizeLobeHubBaseURL(settings.APIBaseURL) {
		return false
	}
	return true
}

func settingsProvider(settings *SystemSettings) string {
	if settings == nil {
		return "openai"
	}
	return strings.TrimSpace(settings.LobeHubDefaultProvider)
}

func normalizeLobeHubProvider(provider string) string {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return "openai"
	}
	return provider
}
