package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// Management endpoints may perform password verification and session
	// initialization through a remote control plane. Keep this separate from
	// user-facing gateway timeouts and allow slower upstreams enough time to
	// complete without being classified as unreachable.
	upstreamManagementRequestTimeout = 20 * time.Second
	upstreamManagementResponseLimit  = 1 << 20
)

// ErrUpstreamAPIKeyGroupUnmapped means the management identity is valid but
// cannot prove which upstream group owns a forwarding API key.
var ErrUpstreamAPIKeyGroupUnmapped = errors.New("upstream API key group could not be mapped")

// ErrUpstreamManagementTurnstileRequired means the upstream deliberately
// requires an interactive browser challenge for password login.
var ErrUpstreamManagementTurnstileRequired = errors.New("upstream management login requires Turnstile verification; use a management access token")

// ErrUpstreamManagementLocationConfirmationRequired is returned only when the
// upstream explicitly asks the operator to make this declaration.
var ErrUpstreamManagementLocationConfirmationRequired = errors.New("upstream login requires explicit confirmation that the account user is outside mainland China")

// upstreamManagementClient implements management protocols for shared upstream
// connections. It has no account repository or billing mutation capability.
type upstreamManagementClient struct {
	client *http.Client
}

type upstreamManagementHTTPStatusError struct {
	statusCode int
}

func (e *upstreamManagementHTTPStatusError) Error() string {
	if e == nil {
		return "upstream management request returned an unknown HTTP status"
	}
	return fmt.Sprintf("upstream management request returned HTTP %d", e.statusCode)
}

func isUpstreamManagementHTTPStatus(err error, statusCode int) bool {
	var statusErr *upstreamManagementHTTPStatusError
	return errors.As(err, &statusErr) && statusErr.statusCode == statusCode
}

type upstreamManagementHTTPSession struct {
	remoteUserID int64
	accessToken  string
	cookie       *http.Cookie
	userAgent    string
}

func (s *upstreamManagementClient) authenticateNewAPIManagementSession(ctx context.Context, client *http.Client, baseURL string, config upstreamManagementConfig, secret upstreamManagementAuthSecret) (upstreamManagementHTTPSession, error) {
	session := upstreamManagementHTTPSession{remoteUserID: config.RemoteUserID, accessToken: secret.AccessToken, userAgent: secret.UserAgent}
	if config.AuthMode == UpstreamManagementAuthModePassword {
		login, err := s.managementJSON(ctx, client, http.MethodPost, upstreamConnectionJoinEndpoint(baseURL, "/api/user/login", false), upstreamManagementRequestHeaders(secret.UserAgent), map[string]string{"username": secret.Username, "password": secret.Password})
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
	}
	if session.remoteUserID <= 0 {
		selfHeaders := newAPIManagementHeaders(config.Provider, session)
		self, err := s.managementJSON(ctx, client, http.MethodGet, upstreamConnectionJoinEndpoint(baseURL, "/api/user/self", false), selfHeaders, nil)
		if err != nil {
			return upstreamManagementHTTPSession{}, err
		}
		session.remoteUserID = int64FromMap(envelopeData(self.payload), "id")
	}
	if session.remoteUserID <= 0 {
		return upstreamManagementHTTPSession{}, errors.New("upstream management API did not return a user id; provide credentials whose user profile exposes an id")
	}
	return session, nil
}

func shouldRefreshSub2APIManagementToken(secret upstreamManagementAuthSecret) bool {
	if secret.RefreshToken == "" {
		return false
	}
	// A token without expiry metadata is refreshed so later runs
	// can renew shortly before expiry instead of waiting for a 401 response.
	return secret.ExpiresAt == 0 || time.Now().Add(5*time.Minute).Unix() >= secret.ExpiresAt
}

func (s *upstreamManagementClient) refreshSub2APIManagementToken(ctx context.Context, client *http.Client, baseURL string, secret upstreamManagementAuthSecret) (upstreamManagementAuthSecret, error) {
	response, err := s.managementJSON(ctx, client, http.MethodPost, upstreamConnectionJoinEndpoint(baseURL, "/api/v1/auth/refresh", false), upstreamManagementRequestHeaders(secret.UserAgent), map[string]string{
		"refresh_token": secret.RefreshToken,
	})
	if err != nil {
		return upstreamManagementAuthSecret{}, fmt.Errorf("refresh upstream management access token: %w", err)
	}
	data := envelopeData(response.payload)
	nextAccessToken := firstString(data, "access_token", "token", "jwt")
	if nextAccessToken == "" {
		return upstreamManagementAuthSecret{}, errors.New("upstream management token refresh returned no access token")
	}
	secret.AccessToken = nextAccessToken
	if nextRefreshToken := firstString(data, "refresh_token"); nextRefreshToken != "" {
		secret.RefreshToken = nextRefreshToken
	}
	if expiresIn := int64FromMap(data, "expires_in"); expiresIn > 0 {
		secret.ExpiresAt = time.Now().Add(time.Duration(expiresIn) * time.Second).Unix()
	} else {
		// Some Sub2API variants omit expires_in but return a standard JWT. Use
		// its exp claim when available so a successful refresh is not immediately
		// repeated with a rotation-sensitive refresh token.
		secret.ExpiresAt = upstreamManagementJWTExpiry(nextAccessToken)
	}
	return secret, nil
}

func newAPIManagementHeaders(provider UpstreamManagementProvider, session upstreamManagementHTTPSession) http.Header {
	headers := upstreamManagementRequestHeaders(session.userAgent)
	if session.remoteUserID > 0 {
		userID := strconv.FormatInt(session.remoteUserID, 10)
		headers.Set("New-API-User", userID)
		// RixAPI accepts the NewAPI header too and additionally requires its
		// vendor-specific identity header. Sending both matches its management UI.
		if provider == UpstreamManagementProviderRixAPI {
			headers.Set("Rix-Api-User", userID)
		}
		if provider == UpstreamManagementProviderVeloera {
			headers.Set("Veloera-User", userID)
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

func (s *upstreamManagementClient) managementJSON(ctx context.Context, client *http.Client, method, endpoint string, headers http.Header, body any) (managementJSONResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, upstreamManagementRequestTimeout)
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
	payload, err := io.ReadAll(io.LimitReader(response.Body, upstreamManagementResponseLimit+1))
	if err != nil {
		return managementJSONResponse{}, err
	}
	if len(payload) > upstreamManagementResponseLimit {
		return managementJSONResponse{}, fmt.Errorf("upstream management response exceeds %d bytes", upstreamManagementResponseLimit)
	}
	var decodedValue any
	decodeErr := json.Unmarshal(payload, &decodedValue)
	decoded, decodedObject := decodedValue.(map[string]any)
	if !decodedObject {
		// A few compatible management APIs return resource collections as a
		// top-level JSON array. Preserve the established envelope contract for
		// callers while accepting that valid response shape.
		if list, ok := decodedValue.([]any); ok {
			decoded = map[string]any{"data": list}
		} else if decodeErr == nil {
			decodeErr = errors.New("upstream management JSON response must be an object or array")
		}
	}
	if decodeErr == nil && isUpstreamLocationConfirmationRejection(decoded) {
		return managementJSONResponse{}, ErrUpstreamManagementLocationConfirmationRequired
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		statusErr := &upstreamManagementHTTPStatusError{statusCode: response.StatusCode}
		if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
			return managementJSONResponse{}, fmt.Errorf("%w: %w", ErrUpstreamConnectionAuthentication, statusErr)
		}
		return managementJSONResponse{}, statusErr
	}
	if decodeErr != nil {
		return managementJSONResponse{}, errors.New("upstream management response is not valid JSON")
	}
	if success, ok := decoded["success"].(bool); ok && !success {
		message := firstString(decoded, "message")
		if strings.Contains(strings.ToLower(message), "turnstile") {
			return managementJSONResponse{}, ErrUpstreamManagementTurnstileRequired
		}
		if isUpstreamAuthenticationRejectionMessage(message) {
			return managementJSONResponse{}, fmt.Errorf("%w: upstream management request was rejected", ErrUpstreamConnectionAuthentication)
		}
		return managementJSONResponse{}, errors.New("upstream management request was rejected")
	}
	if _, ok := decoded["error"]; ok {
		return managementJSONResponse{}, errors.New("upstream management request returned an error")
	}
	return managementJSONResponse{payload: decoded, cookies: response.Cookies()}, nil
}

func isUpstreamLocationConfirmationRejection(payload map[string]any) bool {
	if success, ok := payload["success"].(bool); ok && success {
		return false
	}
	reason := strings.ToUpper(strings.TrimSpace(firstString(payload, "reason", "code")))
	if reason == "NOT_IN_CN_CONFIRMATION_REQUIRED" {
		return true
	}
	message := strings.ToLower(strings.TrimSpace(firstString(payload, "message")))
	return strings.Contains(message, "not located in mainland china") ||
		strings.Contains(message, "outside mainland china")
}

func isUpstreamAuthenticationRejectionMessage(message string) bool {
	message = strings.ToLower(strings.TrimSpace(message))
	if message == "" {
		return false
	}
	for _, marker := range []string{
		"unauthorized", "forbidden", "invalid access token", "access token invalid",
		"authentication failed", "not logged in", "login required", "username or password",
		"无权", "未登录", "登录失效", "用户名或密码", "认证失败", "鉴权失败", "access token 无效",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
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

func upstreamManagementItems(payload any) []any {
	if list, ok := payload.([]any); ok {
		return list
	}
	row, ok := payload.(map[string]any)
	if !ok {
		return nil
	}
	for _, key := range []string{"items", "list", "results", "data"} {
		switch nested := row[key].(type) {
		case []any:
			return nested
		case map[string]any:
			if list := upstreamManagementItems(nested); list != nil {
				return list
			}
		}
	}
	return nil
}

func upstreamManagementItemsRecognized(payload any) bool {
	if _, ok := payload.([]any); ok {
		return true
	}
	row, ok := payload.(map[string]any)
	if !ok {
		return false
	}
	for _, key := range []string{"items", "list", "results", "data"} {
		switch nested := row[key].(type) {
		case []any:
			return true
		case map[string]any:
			if upstreamManagementItemsRecognized(nested) {
				return true
			}
		}
	}
	return false
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
