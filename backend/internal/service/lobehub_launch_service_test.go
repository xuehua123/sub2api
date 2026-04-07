//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type lobehubSettingsReaderStub struct {
	settings *SystemSettings
	err      error
}

func (s *lobehubSettingsReaderStub) GetAllSettings(context.Context) (*SystemSettings, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.settings, nil
}

type lobehubAPIKeyRepoStub struct {
	key *APIKey
	err error
}

func (s *lobehubAPIKeyRepoStub) GetByID(context.Context, int64) (*APIKey, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.key, nil
}

type lobehubStateStoreStub struct {
	createID   string
	created    *LobeHubLaunchTicket
	createdTTL time.Duration
	consume    map[string]*LobeHubLaunchTicket
}

func (s *lobehubStateStoreStub) CreateLaunchTicket(_ context.Context, ticket *LobeHubLaunchTicket, ttl time.Duration) (string, error) {
	s.created = ticket
	s.createdTTL = ttl
	return s.createID, nil
}

func (s *lobehubStateStoreStub) ConsumeLaunchTicket(_ context.Context, ticketID string) (*LobeHubLaunchTicket, error) {
	if ticket, ok := s.consume[ticketID]; ok {
		delete(s.consume, ticketID)
		return ticket, nil
	}
	return nil, ErrLobeHubLaunchTicketNotFound
}

type lobehubWebSessionWriterStub struct {
	sessionID string
	userID    int64
	apiKeyID  int64
}

func (s *lobehubWebSessionWriterStub) CreateWebSession(_ context.Context, userID, apiKeyID int64) (string, error) {
	s.userID = userID
	s.apiKeyID = apiKeyID
	return s.sessionID, nil
}

func TestLobeHubLaunchService_CreateLaunchTicket(t *testing.T) {
	store := &lobehubStateStoreStub{createID: "ticket-1"}
	svc := NewLobeHubLaunchService(
		&lobehubSettingsReaderStub{settings: &SystemSettings{LobeHubEnabled: true}},
		&lobehubAPIKeyRepoStub{key: &APIKey{ID: 9, UserID: 42, Key: "sk-user-1", Status: StatusActive}},
		store,
		nil,
		func() time.Time { return time.Unix(1_712_345_678, 0).UTC() },
	)

	result, err := svc.CreateLaunchTicket(context.Background(), 42, 9)
	require.NoError(t, err)
	require.Equal(t, "ticket-1", result.TicketID)
	require.Equal(t, "/api/v1/lobehub/bridge?ticket=ticket-1", result.BridgeURL)
	require.NotNil(t, store.created)
	require.Equal(t, int64(42), store.created.UserID)
	require.Equal(t, int64(9), store.created.APIKeyID)
	require.Equal(t, LobeHubLaunchTicketTTL, store.createdTTL)
}

func TestLobeHubLaunchService_CreateLaunchTicket_RejectsForeignKey(t *testing.T) {
	store := &lobehubStateStoreStub{createID: "ticket-1"}
	svc := NewLobeHubLaunchService(
		&lobehubSettingsReaderStub{settings: &SystemSettings{LobeHubEnabled: true}},
		&lobehubAPIKeyRepoStub{key: &APIKey{ID: 9, UserID: 99, Key: "sk-user-1", Status: StatusActive}},
		store,
		nil,
		time.Now,
	)

	_, err := svc.CreateLaunchTicket(context.Background(), 42, 9)
	require.ErrorIs(t, err, ErrLobeHubAPIKeyNotOwned)
	require.Nil(t, store.created)
}

func TestLobeHubLaunchService_CreateLaunchTicketRejectsUnusableKey(t *testing.T) {
	expiredAt := time.Now().Add(-time.Hour)

	testCases := []struct {
		name string
		key  *APIKey
	}{
		{
			name: "disabled",
			key:  &APIKey{ID: 9, UserID: 42, Key: "sk-user-1", Status: StatusDisabled},
		},
		{
			name: "expired",
			key:  &APIKey{ID: 9, UserID: 42, Key: "sk-user-1", Status: StatusActive, ExpiresAt: &expiredAt},
		},
		{
			name: "quota exhausted",
			key:  &APIKey{ID: 9, UserID: 42, Key: "sk-user-1", Status: StatusActive, Quota: 10, QuotaUsed: 10},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			store := &lobehubStateStoreStub{createID: "ticket-1"}
			svc := NewLobeHubLaunchService(
				&lobehubSettingsReaderStub{settings: &SystemSettings{LobeHubEnabled: true}},
				&lobehubAPIKeyRepoStub{key: tc.key},
				store,
				nil,
				time.Now,
			)

			_, err := svc.CreateLaunchTicket(context.Background(), 42, 9)
			require.ErrorIs(t, err, ErrLobeHubAPIKeyUnavailable)
			require.Nil(t, store.created)
		})
	}
}

func TestLobeHubLaunchService_BuildBridgePayload(t *testing.T) {
	store := &lobehubStateStoreStub{
		consume: map[string]*LobeHubLaunchTicket{
			"ticket-1": {
				UserID:    42,
				APIKeyID:  9,
				CreatedAt: time.Unix(1_712_345_678, 0).UTC(),
			},
		},
	}
	webSessionWriter := &lobehubWebSessionWriterStub{sessionID: "session-1"}
	svc := NewLobeHubLaunchService(
		&lobehubSettingsReaderStub{settings: &SystemSettings{
			LobeHubEnabled:         true,
			LobeHubChatURL:         "https://chat.example.com",
			LobeHubDefaultProvider: "openai",
			LobeHubDefaultModel:    "gpt-4.1",
			APIBaseURL:             "https://api.example.com",
		}},
		&lobehubAPIKeyRepoStub{key: &APIKey{ID: 9, UserID: 42, Key: "sk-user-1", Status: StatusActive}},
		store,
		webSessionWriter,
		time.Now,
	)

	payload, err := svc.BuildBridgePayload(context.Background(), "ticket-1")
	require.NoError(t, err)
	require.Equal(t, "https://chat.example.com/api/auth/sign-in/oauth2", payload.FormActionURL)
	require.Equal(t, "generic-oidc", payload.ProviderID)
	require.Equal(t, "session-1", payload.WebSessionID)
	require.Equal(t, int64(42), webSessionWriter.userID)
	require.Equal(t, int64(9), webSessionWriter.apiKeyID)

	callbackURL, err := url.Parse(payload.CallbackURL)
	require.NoError(t, err)
	require.Equal(t, "https://chat.example.com/", callbackURL.Scheme+"://"+callbackURL.Host+callbackURL.Path)

	var settings map[string]any
	require.NoError(t, json.Unmarshal([]byte(callbackURL.Query().Get("settings")), &settings))

	keyVaults := settings["keyVaults"].(map[string]any)
	openAIKeyVault := keyVaults["openai"].(map[string]any)
	require.Equal(t, "sk-user-1", openAIKeyVault["apiKey"])
	require.Equal(t, "https://api.example.com/v1", openAIKeyVault["baseURL"])

	languageModel := settings["languageModel"].(map[string]any)
	openAIModel := languageModel["openai"].(map[string]any)
	require.Equal(t, true, openAIModel["enabled"])
	require.Equal(t, []any{"gpt-4.1"}, openAIModel["enabledModels"])
}

func TestLobeHubLaunchService_BuildBridgePayloadRejectsUnusableKey(t *testing.T) {
	store := &lobehubStateStoreStub{
		consume: map[string]*LobeHubLaunchTicket{
			"ticket-1": {
				UserID:    42,
				APIKeyID:  9,
				CreatedAt: time.Unix(1_712_345_678, 0).UTC(),
			},
		},
	}
	svc := NewLobeHubLaunchService(
		&lobehubSettingsReaderStub{settings: &SystemSettings{
			LobeHubEnabled:         true,
			LobeHubChatURL:         "https://chat.example.com",
			LobeHubDefaultProvider: "openai",
			LobeHubDefaultModel:    "gpt-4.1",
			APIBaseURL:             "https://api.example.com",
		}},
		&lobehubAPIKeyRepoStub{key: &APIKey{ID: 9, UserID: 42, Key: "sk-user-1", Status: StatusDisabled}},
		store,
		nil,
		time.Now,
	)

	_, err := svc.BuildBridgePayload(context.Background(), "ticket-1")
	require.ErrorIs(t, err, ErrLobeHubAPIKeyUnavailable)
}
