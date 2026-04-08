package gateway

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	TargetCookieName    = "sub2api_lobehub_target"
	BootstrapCookieName = "sub2api_lobehub_bootstrap"
	SyncCookieName      = "sub2api_lobehub_sync"

	defaultProviderID       = "generic-oidc"
	defaultBootstrapPath    = "/__lobehub_bootstrap"
	defaultSettingsCacheTTL = 30 * time.Second
	defaultUserStatePath    = "/trpc/lambda/aiProvider.getAiProviderRuntimeState"
	syncCookieMaxAge        = 7 * 24 * 60 * 60
)

var (
	errRefreshTargetRequired = errors.New("refresh target required")
	errLobeHubSessionMissing = errors.New("lobehub session missing")
)

type Config struct {
	ListenAddr             string
	UpstreamURL            string
	Sub2APIAPIBaseURL      string
	Sub2APIFrontendURL     string
	ProviderID             string
	BootstrapPath          string
	UserStatePath          string
	PublicSettingsCacheTTL time.Duration
	HTTPClient             *http.Client
}

type Server struct {
	upstreamURL        *url.URL
	sub2apiAPIBaseURL  *url.URL
	sub2apiFrontendURL *url.URL
	providerID         string
	bootstrapPath      string
	userStatePath      string
	settingsCacheTTL   time.Duration
	client             *http.Client
	probeClient        *http.Client
	proxy              *httputil.ReverseProxy

	settingsCache struct {
		mu        sync.RWMutex
		settings  publicSettings
		expiresAt time.Time
		ok        bool
	}
}

type publicSettings struct {
	LobeHubEnabled              bool   `json:"lobehub_enabled"`
	LobeHubRuntimeConfigVersion string `json:"lobehub_runtime_config_version"`
}

type apiResponseEnvelope[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type apiErrorEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type bootstrapExchangeRequest struct {
	ReturnURL string `json:"return_url"`
}

type signInOAuth2Request struct {
	AdditionalData map[string]any `json:"additionalData"`
	CallbackURL    string         `json:"callbackURL"`
	ProviderID     string         `json:"providerId"`
}

type signInOAuth2Response struct {
	URL      string `json:"url"`
	Redirect bool   `json:"redirect"`
}

type bootstrapExchangeResponse struct {
	BootstrapTicketID string `json:"bootstrap_ticket_id"`
}

type bootstrapConsumeResponse struct {
	RedirectURL   string            `json:"redirect_url"`
	ProviderID    string            `json:"provider_id"`
	FetchOnClient bool              `json:"fetch_on_client"`
	KeyVaults     map[string]string `json:"key_vaults"`
}

type configProbeCompareRequest struct {
	KeyVaults     map[string]configProbeKeyVault      `json:"key_vaults"`
	LanguageModel map[string]configProbeLanguageModel `json:"language_model"`
}

type configProbeKeyVault struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
}

type configProbeLanguageModel struct {
	Enabled       bool     `json:"enabled"`
	EnabledModels []string `json:"enabled_models"`
}

type configProbeCompareResponse struct {
	Matched                  bool   `json:"matched"`
	DesiredConfigFingerprint string `json:"desired_config_fingerprint"`
	CurrentConfigFingerprint string `json:"current_config_fingerprint"`
}

type targetTokenClaims struct {
	DesiredConfigFingerprint string `json:"desired_config_fingerprint"`
	RuntimeConfigVersion     string `json:"runtime_config_version"`
	ExpiresAt                int64  `json:"exp"`
}

type syncState struct {
	DesiredConfigFingerprint string
	RuntimeConfigVersion     string
}

func NewServer(cfg Config) (*Server, error) {
	upstreamURL, err := parseRequiredURL(cfg.UpstreamURL, "UpstreamURL")
	if err != nil {
		return nil, err
	}
	sub2apiAPIBaseURL, err := parseRequiredURL(cfg.Sub2APIAPIBaseURL, "Sub2APIAPIBaseURL")
	if err != nil {
		return nil, err
	}
	sub2apiFrontendURL, err := parseRequiredURL(cfg.Sub2APIFrontendURL, "Sub2APIFrontendURL")
	if err != nil {
		return nil, err
	}

	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	probeClient := cloneHTTPClient(client)
	probeClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}

	providerID := strings.TrimSpace(cfg.ProviderID)
	if providerID == "" {
		providerID = defaultProviderID
	}
	bootstrapPath := strings.TrimSpace(cfg.BootstrapPath)
	if bootstrapPath == "" {
		bootstrapPath = defaultBootstrapPath
	}
	userStatePath := strings.TrimSpace(cfg.UserStatePath)
	if userStatePath == "" {
		userStatePath = defaultUserStatePath
	}
	settingsCacheTTL := cfg.PublicSettingsCacheTTL
	if settingsCacheTTL <= 0 {
		settingsCacheTTL = defaultSettingsCacheTTL
	}

	proxy := httputil.NewSingleHostReverseProxy(upstreamURL)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = upstreamURL.Host
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, proxyErr error) {
		http.Error(w, "upstream proxy error: "+proxyErr.Error(), http.StatusBadGateway)
	}

	return &Server{
		upstreamURL:        upstreamURL,
		sub2apiAPIBaseURL:  sub2apiAPIBaseURL,
		sub2apiFrontendURL: sub2apiFrontendURL,
		providerID:         providerID,
		bootstrapPath:      bootstrapPath,
		userStatePath:      userStatePath,
		settingsCacheTTL:   settingsCacheTTL,
		client:             client,
		probeClient:        probeClient,
		proxy:              proxy,
	}, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/healthz" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
		return
	}

	if r.URL.Path == s.bootstrapPath {
		s.handleBootstrap(w, r)
		return
	}

	if shouldProxyDirectly(r) {
		s.proxy.ServeHTTP(w, r)
		return
	}

	authenticated, err := s.probeSession(r.Context(), r)
	if err != nil {
		http.Error(w, "failed to probe lobehub session: "+err.Error(), http.StatusBadGateway)
		return
	}
	if !authenticated {
		s.startSignIn(w, r)
		return
	}

	settings, err := s.getPublicSettings(r.Context())
	if err != nil {
		http.Error(w, "failed to load public settings: "+err.Error(), http.StatusBadGateway)
		return
	}
	if !settings.LobeHubEnabled {
		http.Error(w, "lobehub integration is disabled", http.StatusServiceUnavailable)
		return
	}

	targetToken := readCookieValue(r, TargetCookieName)
	if targetToken == "" {
		s.redirectRefreshTarget(w, r)
		return
	}

	targetClaims, ok := parseTargetTokenClaims(targetToken)
	if !ok || targetClaims.IsExpired(time.Now()) {
		s.redirectRefreshTarget(w, r)
		return
	}
	if strings.TrimSpace(targetClaims.RuntimeConfigVersion) != strings.TrimSpace(settings.LobeHubRuntimeConfigVersion) {
		s.redirectRefreshTarget(w, r)
		return
	}

	matched, err := s.probeCurrentConfig(r.Context(), r, targetToken)
	if err == nil && matched {
		s.proxy.ServeHTTP(w, r)
		return
	}
	if errors.Is(err, errRefreshTargetRequired) {
		s.redirectRefreshTarget(w, r)
		return
	}
	if errors.Is(err, errLobeHubSessionMissing) {
		s.startSignIn(w, r)
		return
	}

	bootstrapTicketID, err := s.exchangeBootstrap(r.Context(), targetToken, currentRequestURL(r))
	if err != nil {
		if errors.Is(err, errRefreshTargetRequired) {
			s.redirectRefreshTarget(w, r)
			return
		}
		http.Error(w, "failed to exchange bootstrap ticket: "+err.Error(), http.StatusBadGateway)
		return
	}

	redirectURL := s.buildGatewayURL(r, s.bootstrapPath, url.Values{
		"mode":   []string{"sync"},
		"ticket": []string{bootstrapTicketID},
	})
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

func (s *Server) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	if isLoginBootstrapRequest(r) {
		http.Redirect(w, r, sanitizeBootstrapReturnURL(r), http.StatusFound)
		return
	}

	ticketID := strings.TrimSpace(r.URL.Query().Get("ticket"))
	if ticketID == "" {
		ticketID = readCookieValue(r, BootstrapCookieName)
	}
	if ticketID == "" {
		http.Error(w, "missing bootstrap ticket", http.StatusBadRequest)
		return
	}

	result, err := s.consumeBootstrap(r.Context(), ticketID)
	if err != nil {
		http.Error(w, "failed to consume bootstrap ticket: "+err.Error(), http.StatusBadGateway)
		return
	}
	if err := s.applyProviderConfig(r.Context(), r, result); err != nil {
		if errors.Is(err, errLobeHubSessionMissing) {
			s.startSignIn(w, r)
			return
		}
		http.Error(w, "failed to apply lobehub provider config: "+err.Error(), http.StatusBadGateway)
		return
	}

	targetToken := readCookieValue(r, TargetCookieName)
	if claims, ok := parseTargetTokenClaims(targetToken); ok {
		setCookie(w, &http.Cookie{
			Name:     SyncCookieName,
			Value:    hashSyncState(claims.SyncState()),
			Path:     "/",
			Domain:   sharedCookieDomain(r.Host),
			MaxAge:   syncCookieMaxAge,
			HttpOnly: true,
			Secure:   requestIsHTTPS(r),
			SameSite: http.SameSiteLaxMode,
		})
	}

	setCookie(w, &http.Cookie{
		Name:     BootstrapCookieName,
		Value:    "",
		Path:     "/",
		Domain:   sharedCookieDomain(r.Host),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   requestIsHTTPS(r),
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(1, 0),
	})

	http.Redirect(w, r, result.RedirectURL, http.StatusFound)
}

func (s *Server) startSignIn(w http.ResponseWriter, r *http.Request) {
	callbackURL := s.buildGatewayURL(r, s.bootstrapPath, url.Values{
		"mode":       []string{"login"},
		"return_url": []string{currentRequestURL(r)},
	})
	body, err := json.Marshal(signInOAuth2Request{
		AdditionalData: map[string]any{},
		CallbackURL:    callbackURL,
		ProviderID:     s.providerID,
	})
	if err != nil {
		http.Error(w, "failed to build sign-in payload: "+err.Error(), http.StatusInternalServerError)
		return
	}

	endpoint := *s.upstreamURL
	endpoint.Path = "/api/auth/sign-in/oauth2"
	endpoint.RawPath = ""
	endpoint.RawQuery = ""
	endpoint.Fragment = ""

	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		http.Error(w, "failed to create sign-in request: "+err.Error(), http.StatusInternalServerError)
		return
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", requestOrigin(r))
	req.Header.Set("Referer", currentRequestURL(r))
	if cookieHeader := r.Header.Get("Cookie"); cookieHeader != "" {
		req.Header.Set("Cookie", cookieHeader)
	}

	resp, err := s.probeClient.Do(req)
	if err != nil {
		http.Error(w, "failed to start sign-in: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "failed to read sign-in response: "+err.Error(), http.StatusBadGateway)
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		http.Error(w, "failed to start sign-in: "+string(responseBody), http.StatusBadGateway)
		return
	}

	var result signInOAuth2Response
	if err := json.Unmarshal(responseBody, &result); err != nil {
		http.Error(w, "failed to parse sign-in response: "+err.Error(), http.StatusBadGateway)
		return
	}
	if strings.TrimSpace(result.URL) == "" {
		http.Error(w, "sign-in response did not include redirect url", http.StatusBadGateway)
		return
	}

	for _, cookieValue := range resp.Header.Values("Set-Cookie") {
		w.Header().Add("Set-Cookie", cookieValue)
	}
	http.Redirect(w, r, strings.TrimSpace(result.URL), http.StatusFound)
}

func (s *Server) redirectRefreshTarget(w http.ResponseWriter, r *http.Request) {
	redirectURL, err := buildRefreshTargetURL(s.sub2apiFrontendURL.String(), currentRequestURL(r))
	if err != nil {
		http.Error(w, "failed to build refresh-target URL: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

func (s *Server) probeSession(ctx context.Context, r *http.Request) (bool, error) {
	probeURL := *s.upstreamURL
	probeURL.Path = r.URL.Path
	probeURL.RawPath = r.URL.RawPath
	probeURL.RawQuery = r.URL.RawQuery

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, probeURL.String(), nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Accept", "text/html")
	if cookieHeader := r.Header.Get("Cookie"); cookieHeader != "" {
		req.Header.Set("Cookie", cookieHeader)
	}

	resp, err := s.probeClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if isSignInRedirect(resp) {
		return false, nil
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return false, nil
	}
	return true, nil
}

func (s *Server) getPublicSettings(ctx context.Context) (*publicSettings, error) {
	s.settingsCache.mu.RLock()
	if s.settingsCache.ok && time.Now().Before(s.settingsCache.expiresAt) {
		cached := s.settingsCache.settings
		s.settingsCache.mu.RUnlock()
		return &cached, nil
	}
	stale := s.settingsCache.settings
	staleOK := s.settingsCache.ok
	s.settingsCache.mu.RUnlock()

	endpoint := s.apiURL("settings/public")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}

	var envelope apiResponseEnvelope[publicSettings]
	if err := s.doJSON(req, &envelope); err != nil {
		if staleOK {
			return &stale, nil
		}
		return nil, err
	}

	s.settingsCache.mu.Lock()
	s.settingsCache.settings = envelope.Data
	s.settingsCache.expiresAt = time.Now().Add(s.settingsCacheTTL)
	s.settingsCache.ok = true
	s.settingsCache.mu.Unlock()

	return &envelope.Data, nil
}

func (s *Server) exchangeBootstrap(ctx context.Context, targetToken string, returnURL string) (string, error) {
	body, err := json.Marshal(bootstrapExchangeRequest{ReturnURL: returnURL})
	if err != nil {
		return "", err
	}

	endpoint := s.apiURL("lobehub/bootstrap-exchange")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", (&http.Cookie{Name: TargetCookieName, Value: targetToken}).String())

	var envelope apiResponseEnvelope[bootstrapExchangeResponse]
	if err := s.doJSON(req, &envelope); err != nil {
		var statusErr *statusError
		if errors.As(err, &statusErr) && statusErr.StatusCode == http.StatusUnauthorized {
			return "", errRefreshTargetRequired
		}
		return "", err
	}
	if strings.TrimSpace(envelope.Data.BootstrapTicketID) == "" {
		return "", fmt.Errorf("bootstrap ticket id missing in response")
	}
	return strings.TrimSpace(envelope.Data.BootstrapTicketID), nil
}

func (s *Server) probeCurrentConfig(ctx context.Context, r *http.Request, targetToken string) (bool, error) {
	observed, err := s.fetchObservedSettings(ctx, r)
	if err != nil {
		return false, err
	}

	body, err := json.Marshal(observed)
	if err != nil {
		return false, err
	}

	endpoint := s.apiURL("lobehub/config-probe/compare")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", (&http.Cookie{Name: TargetCookieName, Value: targetToken}).String())

	var envelope apiResponseEnvelope[configProbeCompareResponse]
	if err := s.doJSON(req, &envelope); err != nil {
		var statusErr *statusError
		if errors.As(err, &statusErr) && statusErr.StatusCode == http.StatusUnauthorized {
			return false, errRefreshTargetRequired
		}
		return false, err
	}
	return envelope.Data.Matched, nil
}

func (s *Server) fetchObservedSettings(ctx context.Context, r *http.Request) (*configProbeCompareRequest, error) {
	endpoint := *s.upstreamURL
	endpoint.Path = s.userStatePath
	endpoint.RawPath = ""
	endpoint.RawQuery = buildObservedSettingsQuery(s.userStatePath)
	endpoint.Fragment = ""

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if cookieHeader := r.Header.Get("Cookie"); cookieHeader != "" {
		req.Header.Set("Cookie", cookieHeader)
	}

	resp, err := s.probeClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if isSignInRedirect(resp) || resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, errLobeHubSessionMissing
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, &statusError{StatusCode: resp.StatusCode, Message: string(body)}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return parseObservedSettings(body)
}

func (s *Server) applyProviderConfig(ctx context.Context, r *http.Request, config *bootstrapConsumeResponse) error {
	if config == nil || strings.TrimSpace(config.ProviderID) == "" || len(config.KeyVaults) == 0 {
		return nil
	}

	body, err := json.Marshal(map[string]any{
		"json": map[string]any{
			"id": strings.TrimSpace(config.ProviderID),
			"value": map[string]any{
				"fetchOnClient": config.FetchOnClient,
				"keyVaults":     config.KeyVaults,
			},
		},
	})
	if err != nil {
		return err
	}

	endpoint := *s.upstreamURL
	endpoint.Path = "/trpc/lambda/aiProvider.updateAiProviderConfig"
	endpoint.RawPath = ""
	endpoint.RawQuery = ""
	endpoint.Fragment = ""

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	if cookieHeader := r.Header.Get("Cookie"); cookieHeader != "" {
		req.Header.Set("Cookie", cookieHeader)
	}

	resp, err := s.probeClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if isSignInRedirect(resp) || resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return errLobeHubSessionMissing
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return &statusError{StatusCode: resp.StatusCode, Message: string(body)}
	}

	return nil
}

func (s *Server) consumeBootstrap(ctx context.Context, ticketID string) (*bootstrapConsumeResponse, error) {
	endpoint := s.apiURL("lobehub/bootstrap/consume")
	query := endpoint.Query()
	query.Set("ticket", strings.TrimSpace(ticketID))
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}

	var envelope apiResponseEnvelope[bootstrapConsumeResponse]
	if err := s.doJSON(req, &envelope); err != nil {
		return nil, err
	}
	return &envelope.Data, nil
}

func (s *Server) doJSON(req *http.Request, out any) error {
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr apiErrorEnvelope
		if json.Unmarshal(body, &apiErr) == nil && apiErr.Message != "" {
			return &statusError{StatusCode: resp.StatusCode, Message: apiErr.Message}
		}
		return &statusError{StatusCode: resp.StatusCode, Message: string(body)}
	}
	if err := json.Unmarshal(body, out); err != nil {
		return err
	}
	return nil
}

func (s *Server) buildGatewayURL(r *http.Request, path string, query url.Values) string {
	base := url.URL{
		Scheme: requestScheme(r),
		Host:   effectiveHost(r),
		Path:   path,
	}
	if len(query) > 0 {
		base.RawQuery = query.Encode()
	}
	return base.String()
}

type statusError struct {
	StatusCode int
	Message    string
}

func (e *statusError) Error() string {
	return fmt.Sprintf("status %d: %s", e.StatusCode, e.Message)
}

func parseRequiredURL(raw string, field string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", field, err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("%s must be an absolute URL", field)
	}
	return parsed, nil
}

func cloneHTTPClient(client *http.Client) *http.Client {
	clone := *client
	return &clone
}

func (s *Server) apiURL(relativePath string) *url.URL {
	base := *s.sub2apiAPIBaseURL
	base.Path = strings.TrimRight(base.Path, "/") + "/" + strings.TrimLeft(relativePath, "/")
	base.RawPath = ""
	base.RawQuery = ""
	base.Fragment = ""
	return &base
}

func shouldProxyDirectly(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return true
	}
	if strings.TrimSpace(r.URL.Query().Get("settings")) != "" {
		return true
	}
	if isWebSocketRequest(r) {
		return true
	}

	path := r.URL.Path
	for _, prefix := range []string{"/_next/", "/api/", "/trpc/", "/webapi/", "/images/"} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	if strings.HasPrefix(path, "/favicon") || strings.HasPrefix(path, "/sitemap") {
		return true
	}
	return path == "/robots.txt"
}

func isWebSocketRequest(r *http.Request) bool {
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("Upgrade")), "websocket")
}

func isSignInRedirect(resp *http.Response) bool {
	location := strings.TrimSpace(resp.Header.Get("Location"))
	if location == "" {
		return false
	}
	parsed, err := url.Parse(location)
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(parsed.Path), "/signin")
}

func readCookieValue(r *http.Request, name string) string {
	cookie, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cookie.Value)
}

func requestScheme(r *http.Request) string {
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		return strings.TrimSpace(parts[0])
	}
	if r.TLS != nil {
		return "https"
	}
	if r.URL.Scheme != "" {
		return r.URL.Scheme
	}
	return "http"
}

func effectiveHost(r *http.Request) string {
	if host := strings.TrimSpace(r.Host); host != "" {
		return host
	}
	return r.URL.Host
}

func currentRequestURL(r *http.Request) string {
	requestURL := url.URL{
		Scheme:   requestScheme(r),
		Host:     effectiveHost(r),
		Path:     r.URL.Path,
		RawPath:  r.URL.RawPath,
		RawQuery: r.URL.RawQuery,
	}
	return requestURL.String()
}

func isLoginBootstrapRequest(r *http.Request) bool {
	return strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("mode")), "login")
}

func sanitizeBootstrapReturnURL(r *http.Request) string {
	raw := strings.TrimSpace(r.URL.Query().Get("return_url"))
	fallback := samesiteFallbackURL(r)
	if raw == "" {
		return fallback
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return fallback
	}
	if !parsed.IsAbs() {
		if !strings.HasPrefix(parsed.Path, "/") {
			return fallback
		}
		return (&url.URL{
			Scheme:   requestScheme(r),
			Host:     effectiveHost(r),
			Path:     parsed.Path,
			RawPath:  parsed.RawPath,
			RawQuery: parsed.RawQuery,
			Fragment: parsed.Fragment,
		}).String()
	}
	if !sameOriginURL(parsed, requestScheme(r), effectiveHost(r)) {
		return fallback
	}
	return parsed.String()
}

func sameOriginURL(target *url.URL, scheme string, host string) bool {
	if target == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(target.Scheme), strings.TrimSpace(scheme)) &&
		strings.EqualFold(strings.TrimSpace(target.Host), strings.TrimSpace(host))
}

func samesiteFallbackURL(r *http.Request) string {
	return (&url.URL{
		Scheme: requestScheme(r),
		Host:   effectiveHost(r),
		Path:   "/",
	}).String()
}

func requestOrigin(r *http.Request) string {
	return (&url.URL{
		Scheme: requestScheme(r),
		Host:   effectiveHost(r),
	}).String()
}

func buildRefreshTargetURL(frontendBaseURL string, returnURL string) (string, error) {
	base, err := url.Parse(strings.TrimSpace(frontendBaseURL))
	if err != nil {
		return "", err
	}
	base = base.ResolveReference(&url.URL{Path: "/auth/lobehub-sso"})
	query := base.Query()
	query.Set("mode", "refresh-target")
	query.Set("return_url", strings.TrimSpace(returnURL))
	base.RawQuery = query.Encode()
	return base.String(), nil
}

func parseTargetTokenClaims(raw string) (*targetTokenClaims, bool) {
	parts := strings.Split(strings.TrimSpace(raw), ".")
	if len(parts) < 2 {
		return nil, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, false
	}
	var claims targetTokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, false
	}
	if strings.TrimSpace(claims.DesiredConfigFingerprint) == "" || strings.TrimSpace(claims.RuntimeConfigVersion) == "" {
		return nil, false
	}
	return &claims, true
}

func (c *targetTokenClaims) IsExpired(now time.Time) bool {
	return c == nil || c.ExpiresAt <= 0 || now.Unix() >= c.ExpiresAt
}

func (c *targetTokenClaims) SyncState() syncState {
	if c == nil {
		return syncState{}
	}
	return syncState{
		DesiredConfigFingerprint: strings.TrimSpace(c.DesiredConfigFingerprint),
		RuntimeConfigVersion:     strings.TrimSpace(c.RuntimeConfigVersion),
	}
}

func hashSyncState(state syncState) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		strings.TrimSpace(state.DesiredConfigFingerprint),
		strings.TrimSpace(state.RuntimeConfigVersion),
	}, "|")))
	return hex.EncodeToString(sum[:])
}

func sharedCookieDomain(host string) string {
	hostname := host
	if strings.Contains(hostname, ":") {
		var err error
		hostname, _, err = net.SplitHostPort(hostname)
		if err != nil {
			hostname = host
		}
	}
	if hostname == "" || strings.EqualFold(hostname, "localhost") || net.ParseIP(hostname) != nil {
		return ""
	}
	parts := strings.Split(hostname, ".")
	if len(parts) < 2 {
		return ""
	}
	return "." + strings.Join(parts[len(parts)-2:], ".")
}

func requestIsHTTPS(r *http.Request) bool {
	return strings.EqualFold(requestScheme(r), "https")
}

func setCookie(w http.ResponseWriter, cookie *http.Cookie) {
	if cookie == nil {
		return
	}
	http.SetCookie(w, cookie)
}

func parseObservedSettings(body []byte) (*configProbeCompareRequest, error) {
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}

	observed := &configProbeCompareRequest{
		KeyVaults:     map[string]configProbeKeyVault{},
		LanguageModel: map[string]configProbeLanguageModel{},
	}

	if runtimeConfig, ok := findRuntimeConfigObject(payload); ok {
		for provider, rawConfig := range runtimeConfig {
			providerConfig, ok := asStringMap(rawConfig)
			if !ok {
				continue
			}
			vault, ok := asStringMap(providerConfig["keyVaults"])
			if !ok {
				continue
			}
			observed.KeyVaults[provider] = configProbeKeyVault{
				APIKey:  stringValue(vault["apiKey"]),
				BaseURL: stringValue(vault["baseURL"]),
			}
		}
		return observed, nil
	}

	settings, ok := findSettingsObject(payload)
	if !ok {
		return nil, fmt.Errorf("runtime config missing in ai provider response")
	}

	if keyVaults, ok := asStringMap(settings["keyVaults"]); ok {
		for provider, rawVault := range keyVaults {
			vault, ok := asStringMap(rawVault)
			if !ok {
				continue
			}
			observed.KeyVaults[provider] = configProbeKeyVault{
				APIKey:  stringValue(vault["apiKey"]),
				BaseURL: stringValue(vault["baseURL"]),
			}
		}
	}

	if languageModel, ok := asStringMap(settings["languageModel"]); ok {
		for provider, rawConfig := range languageModel {
			modelConfig, ok := asStringMap(rawConfig)
			if !ok {
				continue
			}
			observed.LanguageModel[provider] = configProbeLanguageModel{
				Enabled:       boolValue(modelConfig["enabled"]),
				EnabledModels: stringSliceValue(modelConfig["enabledModels"]),
			}
		}
	}

	return observed, nil
}

func findSettingsObject(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		if settings, ok := asStringMap(typed["settings"]); ok {
			return settings, true
		}
		for _, key := range []string{"result", "data", "json"} {
			if nested, ok := findSettingsObject(typed[key]); ok {
				return nested, true
			}
		}
	case []any:
		for _, item := range typed {
			if nested, ok := findSettingsObject(item); ok {
				return nested, true
			}
		}
	}
	return nil, false
}

func findRuntimeConfigObject(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		if runtimeConfig, ok := asStringMap(typed["runtimeConfig"]); ok {
			return runtimeConfig, true
		}
		if jsonData, ok := asStringMap(typed["json"]); ok {
			if runtimeConfig, ok := asStringMap(jsonData["runtimeConfig"]); ok {
				return runtimeConfig, true
			}
		}
		for _, key := range []string{"result", "data"} {
			if nested, ok := findRuntimeConfigObject(typed[key]); ok {
				return nested, true
			}
		}
	case []any:
		for _, item := range typed {
			if nested, ok := findRuntimeConfigObject(item); ok {
				return nested, true
			}
		}
	}
	return nil, false
}

func asStringMap(value any) (map[string]any, bool) {
	result, ok := value.(map[string]any)
	return result, ok
}

func stringValue(value any) string {
	raw, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(raw)
}

func boolValue(value any) bool {
	raw, ok := value.(bool)
	return ok && raw
}

func stringSliceValue(value any) []string {
	values, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, item := range values {
		str, ok := item.(string)
		if !ok {
			continue
		}
		str = strings.TrimSpace(str)
		if str == "" {
			continue
		}
		result = append(result, str)
	}
	return result
}

func buildObservedSettingsQuery(path string) string {
	if strings.TrimSpace(path) != defaultUserStatePath {
		return ""
	}
	values := url.Values{}
	values.Set("input", `{"json":{"isLogin":true}}`)
	return values.Encode()
}
