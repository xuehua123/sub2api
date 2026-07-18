//go:build unit

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type upstreamLegacyMigrationAccountRepoStub struct {
	AccountRepository
	accounts    []Account
	updateCalls int
}

func (r *upstreamLegacyMigrationAccountRepoStub) ListUpstreamManagementAuthRotationCandidates(context.Context) ([]Account, error) {
	return append([]Account{}, r.accounts...), nil
}

func (r *upstreamLegacyMigrationAccountRepoStub) GetByID(_ context.Context, id int64) (*Account, error) {
	for index := range r.accounts {
		if r.accounts[index].ID == id {
			copy := r.accounts[index]
			return &copy, nil
		}
	}
	return nil, ErrAccountNotFound
}

func (r *upstreamLegacyMigrationAccountRepoStub) GetByIDs(_ context.Context, ids []int64) ([]*Account, error) {
	accounts := make([]*Account, 0, len(ids))
	for _, id := range ids {
		account, err := r.GetByID(context.Background(), id)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, account)
	}
	return accounts, nil
}

func (r *upstreamLegacyMigrationAccountRepoStub) Update(context.Context, *Account) error {
	r.updateCalls++
	return nil
}

func TestUpstreamConnectionLegacyMigrationPreviewDeduplicatesIdentityAndPreservesDisabledAccounts(t *testing.T) {
	accountOne := newLegacyMigrationAccount(t, 10, "first", "sk-one", "vip", true, "https://manage.example.com")
	accountTwo := newLegacyMigrationAccount(t, 11, "second", "sk-two", "standard", true, "https://manage.example.com")
	disabled := newLegacyMigrationAccount(t, 12, "disabled", "sk-disabled", "vip", false, "https://manage.example.com")
	accountRepo := &upstreamLegacyMigrationAccountRepoStub{accounts: []Account{accountOne, accountTwo, disabled}}
	repo := &upstreamConnectionTestRepo{}
	connectionService := NewUpstreamConnectionService(repo, upstreamConnectionTestEncryptor{}, nil)
	connectionService.accountRepo = accountRepo

	result, err := connectionService.PreviewLegacyMigration(context.Background())
	require.NoError(t, err)
	require.True(t, result.DryRun)
	require.Equal(t, 3, result.Summary.ScannedAccounts)
	require.Equal(t, 2, result.Summary.EligibleAccounts)
	require.Equal(t, 1, result.Summary.UniqueConnections)
	require.Equal(t, 2, result.Summary.PlannedAccounts)
	require.Equal(t, 1, result.Summary.SkippedAccounts)
	require.Equal(t, upstreamLegacyMigrationActionCreateAndBind, result.Items[0].Action)
	require.Equal(t, upstreamLegacyMigrationActionCreateAndBind, result.Items[1].Action)
	require.Equal(t, upstreamLegacyMigrationActionSkipDisabled, result.Items[2].Action)
	require.Empty(t, repo.connection)
	require.Zero(t, accountRepo.updateCalls)
}

func TestUpstreamConnectionLegacyMigrationPasswordIdentityIgnoresObservedRemoteUserID(t *testing.T) {
	withoutObservedID := newLegacyPasswordMigrationAccount(t, 13, 0)
	withObservedID := newLegacyPasswordMigrationAccount(t, 14, 7)
	accountRepo := &upstreamLegacyMigrationAccountRepoStub{accounts: []Account{withoutObservedID, withObservedID}}
	connectionService := NewUpstreamConnectionService(&upstreamConnectionTestRepo{}, upstreamConnectionTestEncryptor{}, nil)
	connectionService.accountRepo = accountRepo

	result, err := connectionService.PreviewLegacyMigration(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, result.Summary.EligibleAccounts)
	require.Equal(t, 1, result.Summary.UniqueConnections)
}

func TestUpstreamConnectionLegacyMigrationIsObserveOnlyAndIdempotent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		require.Equal(t, "Bearer management-token", request.Header.Get("Authorization"))
		require.Equal(t, "7", request.Header.Get("New-API-User"))
		switch request.URL.Path {
		case "/api/user/self":
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{
				"id": 7, "group": "vip", "quota": 1_000_000,
			}})
		case "/api/status":
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{
				"quota_per_unit": 500_000, "quota_display_type": "USD",
			}})
		case "/api/user/self/groups":
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{
				"group_ratio": map[string]any{"vip": 0.5},
			}})
		case "/api/token/search":
			require.Equal(t, "sk-forwarding", request.URL.Query().Get("token"))
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{"items": []any{
				map[string]any{"id": 99, "name": "production", "group": "vip"},
			}}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	account := newLegacyMigrationAccount(t, 21, "production", "sk-forwarding", "vip", true, server.URL)
	billingMultiplier := 1.7
	account.RateMultiplier = &billingMultiplier
	accountRepo := &upstreamLegacyMigrationAccountRepoStub{accounts: []Account{account}}
	repo := &upstreamConnectionTestRepo{}
	connectionService := NewUpstreamConnectionService(repo, upstreamConnectionTestEncryptor{}, nil)
	connectionService.accountRepo = accountRepo
	connectionService.inspector = newUpstreamConnectionInspector(nil, nil, server.Client())

	result, err := connectionService.MigrateLegacyConnections(context.Background())
	require.NoError(t, err)
	require.False(t, result.DryRun)
	require.Equal(t, 1, result.Summary.MigratedAccounts)
	require.Zero(t, result.Summary.FailedAccounts)
	require.Equal(t, upstreamLegacyMigrationActionMigrated, result.Items[0].Action)
	require.Equal(t, 1, repo.createCalls)
	require.NotEmpty(t, repo.connection.LegacyMigrationKey)
	require.Equal(t, UpstreamConnectionStatusReady, repo.connection.Status)
	require.Equal(t, 2.0, *repo.connection.WalletAmount)
	require.Len(t, repo.connection.Groups, 1)
	require.Equal(t, UpstreamBindingApplyObserveOnly, repo.binding.ApplyPolicy)
	require.Equal(t, 0.5, *repo.binding.ObservedMultiplier)
	require.Zero(t, accountRepo.updateCalls)
	require.Equal(t, 1.7, *accountRepo.accounts[0].RateMultiplier)
	require.True(t, accountRepo.accounts[0].IsUpstreamRateMultiplierSyncEnabled())
	require.NotEmpty(t, accountRepo.accounts[0].GetCredential(UpstreamManagementAuthCredentialKey))

	second, err := connectionService.MigrateLegacyConnections(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, second.Summary.AlreadyMigrated)
	require.Zero(t, second.Summary.MigratedAccounts)
	require.Equal(t, upstreamLegacyMigrationActionAlreadyMigrated, second.Items[0].Action)
	require.Equal(t, 1, repo.createCalls)
	require.Zero(t, accountRepo.updateCalls)
}

func TestUpstreamConnectionLegacyMigrationDoesNotReplaceExistingV2Binding(t *testing.T) {
	account := newLegacyMigrationAccount(t, 31, "manual", "sk-manual", "vip", true, "https://manage.example.com")
	accountRepo := &upstreamLegacyMigrationAccountRepoStub{accounts: []Account{account}}
	repo := &upstreamConnectionTestRepo{binding: &UpstreamAccountBinding{ID: 5, AccountID: 31, ConnectionID: 900}}
	connectionService := NewUpstreamConnectionService(repo, upstreamConnectionTestEncryptor{}, nil)
	connectionService.accountRepo = accountRepo

	result, err := connectionService.MigrateLegacyConnections(context.Background())
	require.NoError(t, err)
	require.Equal(t, upstreamLegacyMigrationActionSkipExistingBinding, result.Items[0].Action)
	require.Equal(t, int64(900), *result.Items[0].ConnectionID)
	require.Equal(t, 1, result.Summary.SkippedAccounts)
	require.Zero(t, repo.createCalls)
	require.Equal(t, int64(900), repo.binding.ConnectionID)
	require.Zero(t, accountRepo.updateCalls)
}

func newLegacyMigrationAccount(t *testing.T, id int64, name, apiKey, group string, enabled bool, managementURL string) Account {
	t.Helper()
	config := UpstreamRateMultiplierSyncConfig{
		Provider: UpstreamManagementProviderNewAPI, AuthMode: UpstreamManagementAuthModeAccessToken,
		Group: group, RemoteUserID: 7,
	}
	ciphertext, err := EncryptUpstreamManagementAuth(upstreamConnectionTestEncryptor{}, config, &UpstreamManagementAuthInput{
		AccessToken: "management-token", RefreshToken: "refresh-token",
	})
	require.NoError(t, err)
	return Account{
		ID: id, Name: name, Type: AccountTypeAPIKey, UpdatedAt: time.Unix(id, 0),
		Credentials: map[string]any{
			"api_key": apiKey, "base_url": managementURL,
			UpstreamManagementBaseURLCredentialKey: managementURL,
			UpstreamManagementAuthCredentialKey:    ciphertext,
		},
		Extra: map[string]any{
			AccountExtraUpstreamRateMultiplierSyncEnabled:      enabled,
			AccountExtraUpstreamRateMultiplierSyncGroup:        group,
			AccountExtraUpstreamRateMultiplierSyncProvider:     string(config.Provider),
			AccountExtraUpstreamRateMultiplierSyncAuthMode:     string(config.AuthMode),
			AccountExtraUpstreamRateMultiplierSyncRemoteUserID: config.RemoteUserID,
		},
	}
}

func newLegacyPasswordMigrationAccount(t *testing.T, id, remoteUserID int64) Account {
	t.Helper()
	config := UpstreamRateMultiplierSyncConfig{
		Provider: UpstreamManagementProviderNewAPI, AuthMode: UpstreamManagementAuthModePassword,
		Group: "vip", RemoteUserID: remoteUserID,
	}
	ciphertext, err := EncryptUpstreamManagementAuth(upstreamConnectionTestEncryptor{}, config, &UpstreamManagementAuthInput{
		Username: "same-user@example.com", Password: "same-password",
	})
	require.NoError(t, err)
	return Account{
		ID: id, Name: "password", Type: AccountTypeAPIKey, UpdatedAt: time.Unix(id, 0),
		Credentials: map[string]any{
			"api_key": "sk-password", "base_url": "https://manage.example.com",
			UpstreamManagementBaseURLCredentialKey: "https://manage.example.com",
			UpstreamManagementAuthCredentialKey:    ciphertext,
		},
		Extra: map[string]any{
			AccountExtraUpstreamRateMultiplierSyncEnabled:      true,
			AccountExtraUpstreamRateMultiplierSyncGroup:        config.Group,
			AccountExtraUpstreamRateMultiplierSyncProvider:     string(config.Provider),
			AccountExtraUpstreamRateMultiplierSyncAuthMode:     string(config.AuthMode),
			AccountExtraUpstreamRateMultiplierSyncRemoteUserID: config.RemoteUserID,
		},
	}
}
