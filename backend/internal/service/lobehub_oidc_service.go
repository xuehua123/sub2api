package service

import (
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/golang-jwt/jwt/v5"
)

const (
	LobeHubOIDCWebSessionTTL         = 5 * time.Minute
	LobeHubOIDCAuthorizationCodeTTL  = 2 * time.Minute
	LobeHubOIDCAccessTokenTTL        = 10 * time.Minute
	LobeHubOIDCAccessTokenTTLSeconds = int(LobeHubOIDCAccessTokenTTL / time.Second)
	LobeHubOIDCResumeTokenTTL        = 15 * time.Minute
	LobeHubTargetTokenTTL            = 7 * 24 * time.Hour
	LobeHubBootstrapTicketTTL        = 3 * time.Minute
)

var (
	ErrLobeHubOIDCSessionNotFound = &LobeHubOIDCProtocolError{
		Code:        "login_required",
		Description: "oidc web session not found or expired",
		StatusCode:  401,
	}
	ErrLobeHubOIDCInvalidRequest = &LobeHubOIDCProtocolError{
		Code:        "invalid_request",
		Description: "invalid oidc request",
		StatusCode:  400,
	}
	ErrLobeHubOIDCInvalidClient = &LobeHubOIDCProtocolError{
		Code:        "invalid_client",
		Description: "invalid oidc client credentials",
		StatusCode:  401,
	}
	ErrLobeHubOIDCInvalidGrant = &LobeHubOIDCProtocolError{
		Code:        "invalid_grant",
		Description: "authorization code is invalid or expired",
		StatusCode:  400,
	}
	ErrLobeHubOIDCInvalidToken = &LobeHubOIDCProtocolError{
		Code:        "invalid_token",
		Description: "access token is invalid or expired",
		StatusCode:  401,
	}
	ErrLobeHubOIDCInsufficientScope = &LobeHubOIDCProtocolError{
		Code:        "insufficient_scope",
		Description: "access token does not include the required scope",
		StatusCode:  403,
	}
	ErrLobeHubOIDCConfigInvalid         = infraerrors.InternalServer("LOBEHUB_OIDC_CONFIG_INVALID", "lobehub oidc configuration is invalid")
	ErrLobeHubOIDCResumeTokenNotFound   = infraerrors.NotFound("LOBEHUB_OIDC_RESUME_TOKEN_NOT_FOUND", "lobehub oidc resume token not found or expired")
	ErrLobeHubBootstrapTicketNotFound   = infraerrors.NotFound("LOBEHUB_BOOTSTRAP_TICKET_NOT_FOUND", "lobehub bootstrap ticket not found or expired")
	ErrLobeHubDefaultChatAPIKeyRequired = infraerrors.Conflict("LOBEHUB_DEFAULT_CHAT_API_KEY_REQUIRED", "default chat api key selection is required")
	ErrLobeHubInvalidReturnURL          = infraerrors.BadRequest("LOBEHUB_INVALID_RETURN_URL", "invalid lobehub return url")
	ErrLobeHubInvalidTargetToken        = infraerrors.Unauthorized("LOBEHUB_INVALID_TARGET_TOKEN", "lobehub target token is invalid or expired")
)

type LobeHubOIDCProtocolError struct {
	Code        string
	Description string
	StatusCode  int
}

func (e *LobeHubOIDCProtocolError) Error() string {
	if e == nil {
		return ""
	}
	if e.Description == "" {
		return e.Code
	}
	return e.Code + ": " + e.Description
}

type LobeHubUserReader interface {
	GetByID(ctx context.Context, id int64) (*User, error)
}

type LobeHubOIDCSigningKeyProvider interface {
	GetSigningKey(ctx context.Context) (*rsa.PrivateKey, string, error)
}

type LobeHubOIDCStateStore interface {
	CreateWebSession(ctx context.Context, session *LobeHubOIDCWebSession, ttl time.Duration) (string, error)
	GetWebSession(ctx context.Context, sessionID string) (*LobeHubOIDCWebSession, error)
	DeleteWebSession(ctx context.Context, sessionID string) error
	CreateAuthorizationCode(ctx context.Context, code *LobeHubOIDCAuthorizationCode, ttl time.Duration) (string, error)
	ConsumeAuthorizationCode(ctx context.Context, code string) (*LobeHubOIDCAuthorizationCode, error)
	CreateAccessToken(ctx context.Context, token *LobeHubOIDCAccessToken, ttl time.Duration) (string, error)
	GetAccessToken(ctx context.Context, token string) (*LobeHubOIDCAccessToken, error)
	CreateResumeToken(ctx context.Context, resume *LobeHubOIDCResumeToken, ttl time.Duration) (string, error)
	GetResumeToken(ctx context.Context, resumeID string) (*LobeHubOIDCResumeToken, error)
	DeleteResumeToken(ctx context.Context, resumeID string) error
	CreateBootstrapTicket(ctx context.Context, ticket *LobeHubBootstrapTicket, ttl time.Duration) (string, error)
	ConsumeBootstrapTicket(ctx context.Context, ticketID string) (*LobeHubBootstrapTicket, error)
}

type LobeHubOIDCWebSession struct {
	UserID    int64     `json:"user_id"`
	APIKeyID  int64     `json:"api_key_id"`
	CreatedAt time.Time `json:"created_at"`
}

type LobeHubOIDCAuthorizationCode struct {
	UserID              int64     `json:"user_id"`
	RedirectURI         string    `json:"redirect_uri"`
	Scope               string    `json:"scope"`
	Nonce               string    `json:"nonce,omitempty"`
	CodeChallenge       string    `json:"code_challenge,omitempty"`
	CodeChallengeMethod string    `json:"code_challenge_method,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
}

type LobeHubOIDCAccessToken struct {
	UserID    int64     `json:"user_id"`
	Scope     string    `json:"scope"`
	CreatedAt time.Time `json:"created_at"`
}

type LobeHubOIDCResumeToken struct {
	ClientID            string    `json:"client_id"`
	RedirectURI         string    `json:"redirect_uri"`
	ResponseType        string    `json:"response_type"`
	Scope               string    `json:"scope"`
	State               string    `json:"state,omitempty"`
	Nonce               string    `json:"nonce,omitempty"`
	ReturnURL           string    `json:"return_url,omitempty"`
	CodeChallenge       string    `json:"code_challenge,omitempty"`
	CodeChallengeMethod string    `json:"code_challenge_method,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
}

type LobeHubBootstrapTicket struct {
	UserID                   int64     `json:"user_id"`
	APIKeyID                 int64     `json:"api_key_id"`
	DesiredConfigFingerprint string    `json:"desired_config_fingerprint"`
	RuntimeConfigVersion     string    `json:"runtime_config_version"`
	ReturnURL                string    `json:"return_url"`
	CreatedAt                time.Time `json:"created_at"`
}

type LobeHubOIDCDiscoveryDocument struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	UserInfoEndpoint                  string   `json:"userinfo_endpoint"`
	JWKSURI                           string   `json:"jwks_uri"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	SubjectTypesSupported             []string `json:"subject_types_supported"`
	IDTokenSigningAlgValuesSupported  []string `json:"id_token_signing_alg_values_supported"`
	ScopesSupported                   []string `json:"scopes_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
	ClaimsSupported                   []string `json:"claims_supported"`
	GrantTypesSupported               []string `json:"grant_types_supported"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported"`
}

type LobeHubJWKSet struct {
	Keys []LobeHubJWK `json:"keys"`
}

type LobeHubJWK struct {
	KeyType string `json:"kty"`
	Use     string `json:"use"`
	Alg     string `json:"alg"`
	KeyID   string `json:"kid"`
	N       string `json:"n"`
	E       string `json:"e"`
}

type LobeHubOIDCAuthorizeRequest struct {
	ClientID            string
	RedirectURI         string
	ResponseType        string
	Scope               string
	State               string
	Nonce               string
	ReturnURL           string
	CodeChallenge       string
	CodeChallengeMethod string
}

type LobeHubOIDCAuthorizeResult struct {
	RedirectURL string
}

type LobeHubOIDCTokenRequest struct {
	GrantType    string
	Code         string
	RedirectURI  string
	ClientID     string
	ClientSecret string
	CodeVerifier string
}

type LobeHubOIDCTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope,omitempty"`
	IDToken     string `json:"id_token"`
}

type LobeHubOIDCUserInfo struct {
	Subject           string `json:"sub"`
	Email             string `json:"email,omitempty"`
	EmailVerified     bool   `json:"email_verified"`
	Name              string `json:"name,omitempty"`
	PreferredUsername string `json:"preferred_username,omitempty"`
}

type lobeHubOIDCConfig struct {
	issuer       string
	chatURL      string
	frontendURL  string
	clientID     string
	clientSecret string
}

type LobeHubOIDCService struct {
	settingsReader     LobeHubSettingsReader
	userReader         LobeHubUserReader
	stateStore         LobeHubOIDCStateStore
	signingKeyProvider LobeHubOIDCSigningKeyProvider
	now                func() time.Time
}

func NewLobeHubOIDCService(
	settingsReader LobeHubSettingsReader,
	userReader LobeHubUserReader,
	stateStore LobeHubOIDCStateStore,
	signingKeyProvider LobeHubOIDCSigningKeyProvider,
	now func() time.Time,
) *LobeHubOIDCService {
	if now == nil {
		now = time.Now
	}
	return &LobeHubOIDCService{
		settingsReader:     settingsReader,
		userReader:         userReader,
		stateStore:         stateStore,
		signingKeyProvider: signingKeyProvider,
		now:                now,
	}
}

func (s *LobeHubOIDCService) CreateWebSession(ctx context.Context, userID, apiKeyID int64) (string, error) {
	return s.stateStore.CreateWebSession(ctx, &LobeHubOIDCWebSession{
		UserID:    userID,
		APIKeyID:  apiKeyID,
		CreatedAt: s.now().UTC(),
	}, LobeHubOIDCWebSessionTTL)
}

func (s *LobeHubOIDCService) GetDiscoveryDocument(ctx context.Context) (*LobeHubOIDCDiscoveryDocument, error) {
	cfg, err := s.getConfig(ctx)
	if err != nil {
		return nil, err
	}
	return &LobeHubOIDCDiscoveryDocument{
		Issuer:                            cfg.issuer,
		AuthorizationEndpoint:             buildOIDCIssuerURL(cfg.issuer, "authorize"),
		TokenEndpoint:                     buildOIDCIssuerURL(cfg.issuer, "token"),
		UserInfoEndpoint:                  buildOIDCIssuerURL(cfg.issuer, "userinfo"),
		JWKSURI:                           buildOIDCIssuerURL(cfg.issuer, "jwks"),
		ResponseTypesSupported:            []string{"code"},
		SubjectTypesSupported:             []string{"public"},
		IDTokenSigningAlgValuesSupported:  []string{jwt.SigningMethodRS256.Name},
		ScopesSupported:                   []string{"openid", "profile", "email"},
		TokenEndpointAuthMethodsSupported: []string{"client_secret_post", "client_secret_basic"},
		ClaimsSupported:                   []string{"sub", "email", "email_verified", "name", "preferred_username"},
		GrantTypesSupported:               []string{"authorization_code"},
		CodeChallengeMethodsSupported:     []string{"S256"},
	}, nil
}

func (s *LobeHubOIDCService) GetJWKS(ctx context.Context) (*LobeHubJWKSet, error) {
	if _, err := s.getConfig(ctx); err != nil {
		return nil, err
	}
	privateKey, keyID, err := s.signingKeyProvider.GetSigningKey(ctx)
	if err != nil {
		return nil, err
	}
	publicKey := privateKey.PublicKey
	return &LobeHubJWKSet{
		Keys: []LobeHubJWK{{
			KeyType: "RSA",
			Use:     "sig",
			Alg:     jwt.SigningMethodRS256.Name,
			KeyID:   keyID,
			N:       base64.RawURLEncoding.EncodeToString(publicKey.N.Bytes()),
			E:       base64.RawURLEncoding.EncodeToString(bigEndianBytes(publicKey.E)),
		}},
	}, nil
}

func (s *LobeHubOIDCService) Authorize(ctx context.Context, webSessionID string, req *LobeHubOIDCAuthorizeRequest) (*LobeHubOIDCAuthorizeResult, error) {
	cfg, err := s.getConfig(ctx)
	if err != nil {
		return nil, err
	}
	if req == nil {
		return nil, ErrLobeHubOIDCInvalidRequest
	}
	if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(req.ClientID)), []byte(cfg.clientID)) != 1 {
		return nil, &LobeHubOIDCProtocolError{Code: ErrLobeHubOIDCInvalidClient.Code, Description: "client_id does not match configured LobeHub client", StatusCode: ErrLobeHubOIDCInvalidClient.StatusCode}
	}
	if strings.TrimSpace(req.ResponseType) != "code" {
		return nil, &LobeHubOIDCProtocolError{Code: ErrLobeHubOIDCInvalidRequest.Code, Description: "response_type must be code", StatusCode: ErrLobeHubOIDCInvalidRequest.StatusCode}
	}
	if !scopeContains(strings.TrimSpace(req.Scope), "openid") {
		return nil, &LobeHubOIDCProtocolError{Code: ErrLobeHubOIDCInvalidRequest.Code, Description: "scope must include openid", StatusCode: ErrLobeHubOIDCInvalidRequest.StatusCode}
	}
	if !isAllowedLobeHubRedirectURI(cfg.chatURL, strings.TrimSpace(req.RedirectURI)) {
		return nil, &LobeHubOIDCProtocolError{Code: ErrLobeHubOIDCInvalidRequest.Code, Description: "redirect_uri is not allowed", StatusCode: ErrLobeHubOIDCInvalidRequest.StatusCode}
	}
	if strings.TrimSpace(req.CodeChallenge) == "" {
		return nil, &LobeHubOIDCProtocolError{Code: ErrLobeHubOIDCInvalidRequest.Code, Description: "code_challenge is required", StatusCode: ErrLobeHubOIDCInvalidRequest.StatusCode}
	}
	if strings.TrimSpace(req.CodeChallengeMethod) != "S256" {
		return nil, &LobeHubOIDCProtocolError{Code: ErrLobeHubOIDCInvalidRequest.Code, Description: "code_challenge_method must be S256", StatusCode: ErrLobeHubOIDCInvalidRequest.StatusCode}
	}

	webSession, err := s.stateStore.GetWebSession(ctx, strings.TrimSpace(webSessionID))
	if err != nil {
		returnURL, sanitizeErr := sanitizeLobeHubReturnURL(cfg.chatURL, req.ReturnURL)
		if sanitizeErr != nil {
			return nil, sanitizeErr
		}
		resumeID, createErr := s.stateStore.CreateResumeToken(ctx, &LobeHubOIDCResumeToken{
			ClientID:            strings.TrimSpace(req.ClientID),
			RedirectURI:         strings.TrimSpace(req.RedirectURI),
			ResponseType:        strings.TrimSpace(req.ResponseType),
			Scope:               normalizeScope(req.Scope),
			State:               strings.TrimSpace(req.State),
			Nonce:               strings.TrimSpace(req.Nonce),
			ReturnURL:           returnURL,
			CodeChallenge:       strings.TrimSpace(req.CodeChallenge),
			CodeChallengeMethod: strings.TrimSpace(req.CodeChallengeMethod),
			CreatedAt:           s.now().UTC(),
		}, LobeHubOIDCResumeTokenTTL)
		if createErr != nil {
			return nil, createErr
		}
		redirectURL, redirectErr := buildLobeHubResumeRedirectURL(cfg.frontendURL, resumeID, returnURL)
		if redirectErr != nil {
			return nil, ErrLobeHubOIDCConfigInvalid
		}
		return &LobeHubOIDCAuthorizeResult{RedirectURL: redirectURL}, nil
	}

	code, err := s.stateStore.CreateAuthorizationCode(ctx, &LobeHubOIDCAuthorizationCode{
		UserID:              webSession.UserID,
		RedirectURI:         strings.TrimSpace(req.RedirectURI),
		Scope:               normalizeScope(req.Scope),
		Nonce:               strings.TrimSpace(req.Nonce),
		CodeChallenge:       strings.TrimSpace(req.CodeChallenge),
		CodeChallengeMethod: strings.TrimSpace(req.CodeChallengeMethod),
		CreatedAt:           s.now().UTC(),
	}, LobeHubOIDCAuthorizationCodeTTL)
	if err != nil {
		return nil, err
	}

	redirectTarget, err := url.Parse(strings.TrimSpace(req.RedirectURI))
	if err != nil {
		return nil, &LobeHubOIDCProtocolError{Code: ErrLobeHubOIDCInvalidRequest.Code, Description: "redirect_uri is malformed", StatusCode: ErrLobeHubOIDCInvalidRequest.StatusCode}
	}
	query := redirectTarget.Query()
	query.Set("code", code)
	if strings.TrimSpace(req.State) != "" {
		query.Set("state", strings.TrimSpace(req.State))
	}
	redirectTarget.RawQuery = query.Encode()

	return &LobeHubOIDCAuthorizeResult{RedirectURL: redirectTarget.String()}, nil
}

func (s *LobeHubOIDCService) ExchangeAuthorizationCode(ctx context.Context, req *LobeHubOIDCTokenRequest) (*LobeHubOIDCTokenResponse, error) {
	cfg, err := s.getConfig(ctx)
	if err != nil {
		return nil, err
	}
	if req == nil {
		return nil, ErrLobeHubOIDCInvalidRequest
	}
	if strings.TrimSpace(req.GrantType) != "authorization_code" {
		return nil, &LobeHubOIDCProtocolError{Code: ErrLobeHubOIDCInvalidRequest.Code, Description: "grant_type must be authorization_code", StatusCode: ErrLobeHubOIDCInvalidRequest.StatusCode}
	}
	if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(req.ClientID)), []byte(cfg.clientID)) != 1 ||
		subtle.ConstantTimeCompare([]byte(req.ClientSecret), []byte(cfg.clientSecret)) != 1 {
		return nil, ErrLobeHubOIDCInvalidClient
	}

	codePayload, err := s.stateStore.ConsumeAuthorizationCode(ctx, strings.TrimSpace(req.Code))
	if err != nil {
		return nil, ErrLobeHubOIDCInvalidGrant
	}
	if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(req.RedirectURI)), []byte(codePayload.RedirectURI)) != 1 {
		return nil, &LobeHubOIDCProtocolError{Code: ErrLobeHubOIDCInvalidGrant.Code, Description: "redirect_uri does not match the authorization request", StatusCode: ErrLobeHubOIDCInvalidGrant.StatusCode}
	}
	if codePayload.CodeChallenge != "" {
		if !verifyPKCE(codePayload.CodeChallenge, strings.TrimSpace(req.CodeVerifier)) {
			return nil, &LobeHubOIDCProtocolError{Code: ErrLobeHubOIDCInvalidGrant.Code, Description: "code_verifier is invalid", StatusCode: ErrLobeHubOIDCInvalidGrant.StatusCode}
		}
	}

	user, err := s.userReader.GetByID(ctx, codePayload.UserID)
	if err != nil {
		return nil, err
	}
	if user == nil || !user.IsActive() {
		return nil, ErrLobeHubOIDCInvalidGrant
	}

	accessToken, err := s.stateStore.CreateAccessToken(ctx, &LobeHubOIDCAccessToken{
		UserID:    user.ID,
		Scope:     codePayload.Scope,
		CreatedAt: s.now().UTC(),
	}, LobeHubOIDCAccessTokenTTL)
	if err != nil {
		return nil, err
	}

	idToken, err := s.signIDToken(ctx, cfg, user, codePayload)
	if err != nil {
		return nil, err
	}

	return &LobeHubOIDCTokenResponse{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   LobeHubOIDCAccessTokenTTLSeconds,
		Scope:       codePayload.Scope,
		IDToken:     idToken,
	}, nil
}

func (s *LobeHubOIDCService) GetUserInfo(ctx context.Context, accessToken string) (*LobeHubOIDCUserInfo, error) {
	if _, err := s.getConfig(ctx); err != nil {
		return nil, err
	}
	tokenPayload, err := s.stateStore.GetAccessToken(ctx, strings.TrimSpace(accessToken))
	if err != nil {
		return nil, ErrLobeHubOIDCInvalidToken
	}
	if !scopeContains(tokenPayload.Scope, "openid") {
		return nil, ErrLobeHubOIDCInsufficientScope
	}

	user, err := s.userReader.GetByID(ctx, tokenPayload.UserID)
	if err != nil {
		return nil, err
	}
	if user == nil || !user.IsActive() {
		return nil, ErrLobeHubOIDCInvalidToken
	}

	return buildLobeHubOIDCUserInfo(user), nil
}

func (s *LobeHubOIDCService) getConfig(ctx context.Context) (*lobeHubOIDCConfig, error) {
	settings, err := s.settingsReader.GetAllSettings(ctx)
	if err != nil {
		return nil, err
	}
	if settings == nil || !settings.LobeHubEnabled {
		return nil, ErrLobeHubDisabled
	}
	issuer := strings.TrimSpace(settings.LobeHubOIDCIssuer)
	chatURL := strings.TrimSpace(settings.LobeHubChatURL)
	clientID := strings.TrimSpace(settings.LobeHubOIDCClientID)
	clientSecret := settings.LobeHubOIDCClientSecret
	if issuer == "" || chatURL == "" || clientID == "" || clientSecret == "" {
		return nil, ErrLobeHubOIDCConfigInvalid
	}
	frontendURL := strings.TrimSpace(settings.FrontendURL)
	if resolver, ok := s.settingsReader.(interface{ GetFrontendURL(context.Context) string }); ok {
		frontendURL = strings.TrimSpace(resolver.GetFrontendURL(ctx))
	}
	if err := validateAbsoluteHTTPURL(issuer); err != nil {
		return nil, ErrLobeHubOIDCConfigInvalid
	}
	if err := validateAbsoluteHTTPURL(chatURL); err != nil {
		return nil, ErrLobeHubOIDCConfigInvalid
	}
	return &lobeHubOIDCConfig{
		issuer:       strings.TrimRight(issuer, "/"),
		chatURL:      chatURL,
		frontendURL:  frontendURL,
		clientID:     clientID,
		clientSecret: clientSecret,
	}, nil
}

func (s *LobeHubOIDCService) signIDToken(ctx context.Context, cfg *lobeHubOIDCConfig, user *User, codePayload *LobeHubOIDCAuthorizationCode) (string, error) {
	privateKey, keyID, err := s.signingKeyProvider.GetSigningKey(ctx)
	if err != nil {
		return "", err
	}

	now := s.now().UTC()
	claims := jwt.MapClaims{
		"iss":                cfg.issuer,
		"sub":                strconv.FormatInt(user.ID, 10),
		"aud":                cfg.clientID,
		"exp":                now.Add(LobeHubOIDCAccessTokenTTL).Unix(),
		"iat":                now.Unix(),
		"auth_time":          codePayload.CreatedAt.Unix(),
		"email":              user.Email,
		"email_verified":     true,
		"name":               preferredLobeHubUserName(user),
		"preferred_username": preferredLobeHubUserName(user),
	}
	if codePayload.Nonce != "" {
		claims["nonce"] = codePayload.Nonce
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = keyID
	return token.SignedString(privateKey)
}

func buildLobeHubOIDCUserInfo(user *User) *LobeHubOIDCUserInfo {
	name := preferredLobeHubUserName(user)
	return &LobeHubOIDCUserInfo{
		Subject:           strconv.FormatInt(user.ID, 10),
		Email:             user.Email,
		EmailVerified:     true,
		Name:              name,
		PreferredUsername: name,
	}
}

func preferredLobeHubUserName(user *User) string {
	if user == nil {
		return ""
	}
	if name := strings.TrimSpace(user.Username); name != "" {
		return name
	}
	if email := strings.TrimSpace(user.Email); email != "" {
		return email
	}
	return "user-" + strconv.FormatInt(user.ID, 10)
}

func buildOIDCIssuerURL(issuer, path string) string {
	return strings.TrimRight(issuer, "/") + "/" + strings.TrimLeft(path, "/")
}

func normalizeScope(scope string) string {
	parts := strings.Fields(strings.TrimSpace(scope))
	if len(parts) == 0 {
		return "openid"
	}
	return strings.Join(parts, " ")
}

func scopeContains(scope string, required string) bool {
	for _, part := range strings.Fields(scope) {
		if part == required {
			return true
		}
	}
	return false
}

func verifyPKCE(expectedChallenge string, verifier string) bool {
	if verifier == "" || expectedChallenge == "" {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	computed := base64.RawURLEncoding.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(expectedChallenge), []byte(computed)) == 1
}

func isAllowedLobeHubRedirectURI(chatURL string, redirectURI string) bool {
	redirectURI = strings.TrimSpace(redirectURI)
	if redirectURI == "" {
		return false
	}
	for _, candidate := range lobeHubRedirectURICandidates(chatURL) {
		if subtle.ConstantTimeCompare([]byte(candidate), []byte(redirectURI)) == 1 {
			return true
		}
	}
	return false
}

func lobeHubRedirectURICandidates(chatURL string) []string {
	candidates := make([]string, 0, 4)
	seen := map[string]struct{}{}
	appendCandidate := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		candidates = append(candidates, value)
	}

	absolutePaths := []string{
		"/api/auth/oauth2/callback/" + lobeHubProviderID,
		"/api/auth/callback/" + lobeHubProviderID,
	}
	relativePaths := []string{
		"api/auth/oauth2/callback/" + lobeHubProviderID,
		"api/auth/callback/" + lobeHubProviderID,
	}

	for _, path := range absolutePaths {
		if value, err := resolveURL(chatURL, path); err == nil {
			appendCandidate(value)
		}
	}
	baseWithSlash := strings.TrimRight(strings.TrimSpace(chatURL), "/") + "/"
	for _, path := range relativePaths {
		if value, err := resolveURL(baseWithSlash, path); err == nil {
			appendCandidate(value)
		}
	}

	return candidates
}

func validateAbsoluteHTTPURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("parse url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("unsupported scheme")
	}
	if parsed.Host == "" {
		return fmt.Errorf("missing host")
	}
	return nil
}

func buildLobeHubResumeRedirectURL(frontendURL string, resumeID string, returnURL string) (string, error) {
	base, err := url.Parse(strings.TrimSpace(frontendURL))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return "", fmt.Errorf("invalid frontend url")
	}
	base = base.ResolveReference(&url.URL{Path: "/auth/lobehub-sso"})
	query := base.Query()
	query.Set("resume", strings.TrimSpace(resumeID))
	query.Set("return_url", strings.TrimSpace(returnURL))
	base.RawQuery = query.Encode()
	return base.String(), nil
}

func bigEndianBytes(value int) []byte {
	if value == 0 {
		return []byte{0}
	}
	out := make([]byte, 0, 8)
	for value > 0 {
		out = append([]byte{byte(value & 0xff)}, out...)
		value >>= 8
	}
	return out
}
