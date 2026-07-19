package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type UpstreamManagementProvider string

const (
	UpstreamManagementProviderNewAPI   UpstreamManagementProvider = "newapi"
	UpstreamManagementProviderRixAPI   UpstreamManagementProvider = "rixapi"
	UpstreamManagementProviderShellAPI UpstreamManagementProvider = "shellapi"
	UpstreamManagementProviderVeloera  UpstreamManagementProvider = "veloera"
	UpstreamManagementProviderSub2API  UpstreamManagementProvider = "sub2api"
)

type UpstreamManagementAuthMode string

const (
	UpstreamManagementAuthModePassword    UpstreamManagementAuthMode = "password"
	UpstreamManagementAuthModeAccessToken UpstreamManagementAuthMode = "access_token"
)

type upstreamManagementAuthSecret struct {
	Username     string `json:"username,omitempty"`
	Password     string `json:"password,omitempty"`
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	UserAgent    string `json:"user_agent,omitempty"`
	ExpiresAt    int64  `json:"expires_at,omitempty"`
}

func normalizeUpstreamManagementUserAgent(value string) (string, error) {
	if !utf8.ValidString(value) {
		return "", errors.New("upstream management user agent must be valid UTF-8")
	}
	value = strings.TrimSpace(value)
	if len(value) > 512 {
		return "", errors.New("upstream management user agent must not exceed 512 bytes")
	}
	if strings.ContainsFunc(value, unicode.IsControl) {
		return "", errors.New("upstream management user agent must not contain control characters")
	}
	return value, nil
}

func upstreamManagementRequestHeaders(userAgent string) http.Header {
	headers := make(http.Header)
	if userAgent != "" {
		headers.Set("User-Agent", userAgent)
	}
	return headers
}

type upstreamManagementConfig struct {
	Provider     UpstreamManagementProvider
	AuthMode     UpstreamManagementAuthMode
	Group        string
	RemoteUserID int64
}

func supportsUpstreamConnectionAccountType(accountType string) bool {
	return accountType == AccountTypeAPIKey || accountType == AccountTypeUpstream
}

func upstreamManagementRemoteUserID(raw any) (int64, error) {
	if raw == nil || strings.TrimSpace(fmt.Sprint(raw)) == "" {
		return 0, nil
	}
	switch value := raw.(type) {
	case int64:
		if value < 0 {
			return 0, errors.New("upstream remote user id must be positive")
		}
		return value, nil
	case int:
		if value < 0 {
			return 0, errors.New("upstream remote user id must be positive")
		}
		return int64(value), nil
	case float64:
		if value < 0 || value != float64(int64(value)) {
			return 0, errors.New("upstream remote user id must be a positive integer")
		}
		return int64(value), nil
	case json.Number:
		parsed, err := value.Int64()
		if err != nil || parsed < 0 {
			return 0, errors.New("upstream remote user id must be a positive integer")
		}
		return parsed, nil
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err != nil || parsed < 0 {
			return 0, errors.New("upstream remote user id must be a positive integer")
		}
		return parsed, nil
	default:
		return 0, errors.New("upstream remote user id must be a positive integer")
	}
}

func validateUpstreamManagementAuth(config upstreamManagementConfig, secret upstreamManagementAuthSecret) error {
	if _, err := normalizeUpstreamManagementUserAgent(secret.UserAgent); err != nil {
		return err
	}
	switch config.AuthMode {
	case UpstreamManagementAuthModePassword:
		if secret.Username == "" || secret.Password == "" {
			return errors.New("upstream management username and password are required")
		}
	case UpstreamManagementAuthModeAccessToken:
		if secret.AccessToken == "" {
			return errors.New("upstream management access token is required")
		}
	default:
		return errors.New("unsupported upstream management auth mode")
	}
	return nil
}
