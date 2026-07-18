package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	// UpstreamManagementAuthCredentialKey stores the encrypted management JWT,
	// refresh token, and optional password credentials as one opaque value.
	UpstreamManagementAuthCredentialKey = "upstream_management_auth"
	upstreamManagementAuthCredentialKey = UpstreamManagementAuthCredentialKey
	// UpstreamManagementBaseURLCredentialKey stores the optional management-plane
	// address. It must never replace the forwarding base_url used for API traffic.
	UpstreamManagementBaseURLCredentialKey = "upstream_management_base_url"
)

const (
	AccountExtraUpstreamRateMultiplierSyncProvider     = "upstream_rate_multiplier_sync_provider"
	AccountExtraUpstreamRateMultiplierSyncAuthMode     = "upstream_rate_multiplier_sync_auth_mode"
	AccountExtraUpstreamRateMultiplierSyncRemoteUserID = "upstream_rate_multiplier_sync_remote_user_id"
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

// UpstreamManagementAuthInput is accepted only on account create/update. Its
// plaintext values are encrypted before persistence and never returned by DTOs.
type UpstreamManagementAuthInput struct {
	Clear        bool   `json:"clear,omitempty"`
	Username     string `json:"username,omitempty"`
	Password     string `json:"password,omitempty"`
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
}

func applyUpstreamManagementBaseURLInput(credentials map[string]any, baseURL *string) map[string]any {
	if baseURL == nil {
		return credentials
	}
	updated := make(map[string]any, len(credentials)+1)
	for key, value := range credentials {
		updated[key] = value
	}
	if value := strings.TrimSpace(*baseURL); value != "" {
		updated[UpstreamManagementBaseURLCredentialKey] = value
	} else {
		delete(updated, UpstreamManagementBaseURLCredentialKey)
	}
	return updated
}

type upstreamManagementAuthSecret struct {
	Username     string `json:"username,omitempty"`
	Password     string `json:"password,omitempty"`
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresAt    int64  `json:"expires_at,omitempty"`
}

// UpstreamRateMultiplierSyncConfig is the non-secret, account-local mapping
// between a channel account and exactly one upstream user group.
type UpstreamRateMultiplierSyncConfig struct {
	Provider     UpstreamManagementProvider
	AuthMode     UpstreamManagementAuthMode
	Group        string
	RemoteUserID int64
}

func supportsUpstreamRateMultiplierSyncAccountType(accountType string) bool {
	return accountType == AccountTypeAPIKey || accountType == AccountTypeUpstream
}

func (a *Account) UpstreamRateMultiplierSyncConfig() (UpstreamRateMultiplierSyncConfig, error) {
	if a == nil || !a.IsUpstreamRateMultiplierSyncEnabled() {
		return UpstreamRateMultiplierSyncConfig{}, nil
	}
	return upstreamRateMultiplierSyncConfigFromExtra(a.Extra)
}

func (a *Account) HasUpstreamManagementAuth() bool {
	return a != nil && strings.TrimSpace(a.GetCredential(upstreamManagementAuthCredentialKey)) != ""
}

func upstreamRateMultiplierSyncConfigFromExtra(extra map[string]any) (UpstreamRateMultiplierSyncConfig, error) {
	config := UpstreamRateMultiplierSyncConfig{
		Group: strings.TrimSpace(extraString(extra, AccountExtraUpstreamRateMultiplierSyncGroup)),
	}
	if config.Group == "" {
		return UpstreamRateMultiplierSyncConfig{}, errors.New("upstream rate multiplier sync group is required")
	}
	if len(config.Group) > 128 || strings.ContainsAny(config.Group, "\r\n\x00") {
		return UpstreamRateMultiplierSyncConfig{}, errors.New("upstream rate multiplier sync group must be 1-128 characters without line breaks")
	}

	config.Provider = UpstreamManagementProvider(strings.ToLower(strings.TrimSpace(extraString(extra, AccountExtraUpstreamRateMultiplierSyncProvider))))
	switch config.Provider {
	case UpstreamManagementProviderNewAPI, UpstreamManagementProviderRixAPI, UpstreamManagementProviderShellAPI,
		UpstreamManagementProviderVeloera, UpstreamManagementProviderSub2API:
	default:
		return UpstreamRateMultiplierSyncConfig{}, errors.New("upstream rate multiplier sync provider must be newapi, rixapi, shellapi, veloera, or sub2api")
	}

	config.AuthMode = UpstreamManagementAuthMode(strings.ToLower(strings.TrimSpace(extraString(extra, AccountExtraUpstreamRateMultiplierSyncAuthMode))))
	switch config.AuthMode {
	case UpstreamManagementAuthModePassword, UpstreamManagementAuthModeAccessToken:
	default:
		return UpstreamRateMultiplierSyncConfig{}, errors.New("upstream rate multiplier sync auth mode must be password or access_token")
	}

	remoteUserID, err := upstreamManagementRemoteUserID(extra[AccountExtraUpstreamRateMultiplierSyncRemoteUserID])
	if err != nil {
		return UpstreamRateMultiplierSyncConfig{}, err
	}
	config.RemoteUserID = remoteUserID
	if config.AuthMode == UpstreamManagementAuthModeAccessToken && config.Provider != UpstreamManagementProviderSub2API && config.RemoteUserID <= 0 {
		return UpstreamRateMultiplierSyncConfig{}, errors.New("remote user id is required for this upstream access token")
	}
	return config, nil
}

func upstreamRateMultiplierSyncConfigFromExtraIfEnabled(extra map[string]any) (UpstreamRateMultiplierSyncConfig, error) {
	if extra == nil {
		return UpstreamRateMultiplierSyncConfig{}, nil
	}
	enabled, _ := extra[AccountExtraUpstreamRateMultiplierSyncEnabled].(bool)
	if !enabled {
		return UpstreamRateMultiplierSyncConfig{}, nil
	}
	return upstreamRateMultiplierSyncConfigFromExtra(extra)
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

func extraString(extra map[string]any, key string) string {
	if extra == nil {
		return ""
	}
	value, ok := extra[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func EncryptUpstreamManagementAuth(encryptor SecretEncryptor, config UpstreamRateMultiplierSyncConfig, input *UpstreamManagementAuthInput) (string, error) {
	if encryptor == nil {
		return "", errors.New("upstream management credential encryption is unavailable")
	}
	if input == nil || input.Clear {
		return "", errors.New("upstream management credentials are required")
	}
	secret := upstreamManagementAuthSecret{
		Username:     strings.TrimSpace(input.Username),
		Password:     strings.TrimSpace(input.Password),
		AccessToken:  strings.TrimSpace(input.AccessToken),
		RefreshToken: strings.TrimSpace(input.RefreshToken),
	}
	if err := validateUpstreamManagementAuth(config, secret); err != nil {
		return "", err
	}
	return encryptUpstreamManagementAuthSecret(encryptor, secret)
}

// encryptUpstreamManagementAuthSecret is kept separate from the admin input
// path so a rotated management token can retain its refresh expiry metadata.
// The secret is only ever persisted as one encrypted credential value.
func encryptUpstreamManagementAuthSecret(encryptor SecretEncryptor, secret upstreamManagementAuthSecret) (string, error) {
	if encryptor == nil {
		return "", errors.New("upstream management credential encryption is unavailable")
	}
	payload, err := json.Marshal(secret)
	if err != nil {
		return "", fmt.Errorf("encode upstream management credentials: %w", err)
	}
	ciphertext, err := encryptor.Encrypt(string(payload))
	if err != nil {
		return "", fmt.Errorf("encrypt upstream management credentials: %w", err)
	}
	if strings.TrimSpace(ciphertext) == "" {
		return "", errors.New("encrypt upstream management credentials returned empty ciphertext")
	}
	return ciphertext, nil
}

func DecryptUpstreamManagementAuth(encryptor SecretEncryptor, ciphertext string) (upstreamManagementAuthSecret, error) {
	if encryptor == nil {
		return upstreamManagementAuthSecret{}, errors.New("upstream management credential encryption is unavailable")
	}
	plaintext, err := encryptor.Decrypt(strings.TrimSpace(ciphertext))
	if err != nil {
		return upstreamManagementAuthSecret{}, errors.New("decrypt upstream management credentials")
	}
	var secret upstreamManagementAuthSecret
	if err := json.Unmarshal([]byte(plaintext), &secret); err != nil {
		return upstreamManagementAuthSecret{}, errors.New("decode upstream management credentials")
	}
	secret.Username = strings.TrimSpace(secret.Username)
	secret.Password = strings.TrimSpace(secret.Password)
	secret.AccessToken = strings.TrimSpace(secret.AccessToken)
	secret.RefreshToken = strings.TrimSpace(secret.RefreshToken)
	return secret, nil
}

func validateUpstreamManagementAuth(config UpstreamRateMultiplierSyncConfig, secret upstreamManagementAuthSecret) error {
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

func applyUpstreamManagementAuthInput(credentials map[string]any, config UpstreamRateMultiplierSyncConfig, input *UpstreamManagementAuthInput, encryptor SecretEncryptor) (map[string]any, error) {
	cloned := make(map[string]any, len(credentials)+1)
	for key, value := range credentials {
		cloned[key] = value
	}
	if input == nil {
		return cloned, nil
	}
	if input.Clear {
		delete(cloned, upstreamManagementAuthCredentialKey)
		return cloned, nil
	}
	if config.Group == "" {
		return nil, errors.New("upstream management credentials require enabled rate multiplier sync")
	}
	// The edit DTO intentionally never returns the existing refresh token. When
	// an admin replaces only the short-lived JWT, preserve the encrypted RT so
	// scheduled renewal remains enabled. Clear=true is the explicit removal
	// action for the whole management identity.
	effectiveInput := *input
	if effectiveInput.AccessToken != "" && effectiveInput.RefreshToken == "" {
		if existing, decryptErr := DecryptUpstreamManagementAuth(encryptor, upstreamManagementAuthCiphertext(credentials)); decryptErr == nil {
			effectiveInput.RefreshToken = existing.RefreshToken
		}
	}
	ciphertext, err := EncryptUpstreamManagementAuth(encryptor, config, &effectiveInput)
	if err != nil {
		return nil, err
	}
	cloned[upstreamManagementAuthCredentialKey] = ciphertext
	return cloned, nil
}

func upstreamManagementAuthCiphertext(credentials map[string]any) string {
	if credentials == nil {
		return ""
	}
	value, _ := credentials[upstreamManagementAuthCredentialKey].(string)
	return strings.TrimSpace(value)
}

func validateConfiguredUpstreamManagementAuth(account *Account, encryptor SecretEncryptor) error {
	if account == nil || !account.IsUpstreamRateMultiplierSyncEnabled() {
		return nil
	}
	if !supportsUpstreamRateMultiplierSyncAccountType(account.Type) {
		return errors.New("upstream rate multiplier sync only supports apikey and upstream accounts")
	}
	config, err := account.UpstreamRateMultiplierSyncConfig()
	if err != nil {
		return err
	}
	ciphertext := account.GetCredential(upstreamManagementAuthCredentialKey)
	if ciphertext == "" {
		return errors.New("upstream rate multiplier sync requires management credentials")
	}
	secret, err := DecryptUpstreamManagementAuth(encryptor, ciphertext)
	if err != nil {
		return err
	}
	return validateUpstreamManagementAuth(config, secret)
}
