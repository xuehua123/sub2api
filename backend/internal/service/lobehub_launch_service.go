package service

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	LobeHubLaunchTicketTTL = 60 * time.Second
	lobeHubProviderID      = "generic-oidc"
)

var (
	ErrLobeHubDisabled                = infraerrors.Forbidden("LOBEHUB_DISABLED", "lobehub integration is disabled")
	ErrLobeHubAPIKeyNotOwned          = infraerrors.Forbidden("LOBEHUB_API_KEY_NOT_OWNED", "api key does not belong to the current user")
	ErrLobeHubAPIKeyUnavailable       = infraerrors.Conflict("LOBEHUB_API_KEY_UNAVAILABLE", "api key is inactive, expired, or quota exhausted")
	ErrLobeHubLaunchTicketNotFound    = infraerrors.NotFound("LOBEHUB_LAUNCH_TICKET_NOT_FOUND", "lobehub launch ticket not found or expired")
	ErrLobeHubChatURLNotConfigured    = infraerrors.InternalServer("LOBEHUB_CHAT_URL_NOT_CONFIGURED", "lobehub chat url is not configured")
	ErrLobeHubAPIBaseURLNotConfigured = infraerrors.InternalServer("LOBEHUB_API_BASE_URL_NOT_CONFIGURED", "api base url is not configured")
)

type LobeHubSettingsReader interface {
	GetAllSettings(ctx context.Context) (*SystemSettings, error)
}

type LobeHubAPIKeyReader interface {
	GetByID(ctx context.Context, id int64) (*APIKey, error)
}

type LobeHubLaunchStateStore interface {
	CreateLaunchTicket(ctx context.Context, ticket *LobeHubLaunchTicket, ttl time.Duration) (string, error)
	ConsumeLaunchTicket(ctx context.Context, ticketID string) (*LobeHubLaunchTicket, error)
}

type LobeHubOIDCWebSessionCreator interface {
	CreateWebSession(ctx context.Context, userID, apiKeyID int64) (string, error)
}

type LobeHubLaunchTicket struct {
	UserID    int64     `json:"user_id"`
	APIKeyID  int64     `json:"api_key_id"`
	CreatedAt time.Time `json:"created_at"`
}

type LobeHubLaunchTicketResult struct {
	TicketID  string `json:"ticket_id"`
	BridgeURL string `json:"bridge_url"`
}

type LobeHubBridgePayload struct {
	FormActionURL string
	ProviderID    string
	CallbackURL   string
	WebSessionID  string
}

type LobeHubLaunchService struct {
	settingsReader    LobeHubSettingsReader
	apiKeyReader      LobeHubAPIKeyReader
	stateStore        LobeHubLaunchStateStore
	webSessionCreator LobeHubOIDCWebSessionCreator
	now               func() time.Time
}

func NewLobeHubLaunchService(
	settingsReader LobeHubSettingsReader,
	apiKeyReader LobeHubAPIKeyReader,
	stateStore LobeHubLaunchStateStore,
	webSessionCreator LobeHubOIDCWebSessionCreator,
	now func() time.Time,
) *LobeHubLaunchService {
	if now == nil {
		now = time.Now
	}
	return &LobeHubLaunchService{
		settingsReader:    settingsReader,
		apiKeyReader:      apiKeyReader,
		stateStore:        stateStore,
		webSessionCreator: webSessionCreator,
		now:               now,
	}
}

func (s *LobeHubLaunchService) CreateLaunchTicket(ctx context.Context, userID, apiKeyID int64) (*LobeHubLaunchTicketResult, error) {
	settings, err := s.settingsReader.GetAllSettings(ctx)
	if err != nil {
		return nil, err
	}
	if settings == nil || !settings.LobeHubEnabled {
		return nil, ErrLobeHubDisabled
	}

	apiKey, err := s.apiKeyReader.GetByID(ctx, apiKeyID)
	if err != nil {
		return nil, err
	}
	if err := validateLobeHubLaunchAPIKey(apiKey, userID); err != nil {
		return nil, err
	}

	ticketID, err := s.stateStore.CreateLaunchTicket(ctx, &LobeHubLaunchTicket{
		UserID:    userID,
		APIKeyID:  apiKeyID,
		CreatedAt: s.now().UTC(),
	}, LobeHubLaunchTicketTTL)
	if err != nil {
		return nil, err
	}

	return &LobeHubLaunchTicketResult{
		TicketID:  ticketID,
		BridgeURL: "/api/v1/lobehub/bridge?ticket=" + url.QueryEscape(ticketID),
	}, nil
}

func (s *LobeHubLaunchService) BuildBridgePayload(ctx context.Context, ticketID string) (*LobeHubBridgePayload, error) {
	ticket, err := s.stateStore.ConsumeLaunchTicket(ctx, strings.TrimSpace(ticketID))
	if err != nil {
		return nil, err
	}

	settings, err := s.settingsReader.GetAllSettings(ctx)
	if err != nil {
		return nil, err
	}
	if settings == nil || !settings.LobeHubEnabled {
		return nil, ErrLobeHubDisabled
	}

	chatURL := strings.TrimSpace(settings.LobeHubChatURL)
	if chatURL == "" {
		return nil, ErrLobeHubChatURLNotConfigured
	}

	apiBaseURL := normalizeLobeHubBaseURL(settings.APIBaseURL)
	if apiBaseURL == "" {
		return nil, ErrLobeHubAPIBaseURLNotConfigured
	}

	apiKey, err := s.apiKeyReader.GetByID(ctx, ticket.APIKeyID)
	if err != nil {
		return nil, err
	}
	if err := validateLobeHubLaunchAPIKey(apiKey, ticket.UserID); err != nil {
		return nil, err
	}

	settingsJSON, err := json.Marshal(map[string]any{
		"keyVaults": map[string]any{
			settings.LobeHubDefaultProvider: map[string]any{
				"apiKey":  apiKey.Key,
				"baseURL": apiBaseURL,
			},
		},
		"languageModel": map[string]any{
			settings.LobeHubDefaultProvider: map[string]any{
				"enabled":             true,
				"fetchOnClient":       true,
				"autoFetchModelLists": true,
				"enabledModels":       []string{strings.TrimSpace(settings.LobeHubDefaultModel)},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	callbackBase, err := resolveURL(chatURL, "/")
	if err != nil {
		return nil, err
	}
	callbackParsed, err := url.Parse(callbackBase)
	if err != nil {
		return nil, err
	}
	query := callbackParsed.Query()
	query.Set("settings", string(settingsJSON))
	callbackParsed.RawQuery = query.Encode()

	formActionURL, err := resolveURL(chatURL, "/api/auth/sign-in/oauth2")
	if err != nil {
		return nil, err
	}

	webSessionID := ""
	if s.webSessionCreator != nil {
		webSessionID, err = s.webSessionCreator.CreateWebSession(ctx, ticket.UserID, ticket.APIKeyID)
		if err != nil {
			return nil, err
		}
	}

	return &LobeHubBridgePayload{
		FormActionURL: formActionURL,
		ProviderID:    lobeHubProviderID,
		CallbackURL:   callbackParsed.String(),
		WebSessionID:  webSessionID,
	}, nil
}

func resolveURL(base, path string) (string, error) {
	parsedBase, err := url.Parse(strings.TrimSpace(base))
	if err != nil {
		return "", err
	}
	return parsedBase.ResolveReference(&url.URL{Path: path}).String(), nil
}

func normalizeLobeHubBaseURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""

	path := strings.TrimRight(parsed.Path, "/")
	if path == "" {
		parsed.Path = "/v1"
	} else if !strings.HasSuffix(path, "/v1") {
		parsed.Path = path + "/v1"
	} else {
		parsed.Path = path
	}

	return parsed.String()
}

func validateLobeHubLaunchAPIKey(apiKey *APIKey, userID int64) error {
	if apiKey == nil || apiKey.UserID != userID {
		return ErrLobeHubAPIKeyNotOwned
	}
	if !isLobeHubAPIKeyUsable(apiKey) {
		return ErrLobeHubAPIKeyUnavailable
	}
	return nil
}
