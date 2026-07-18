//go:build unit

package service

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type upstreamConnectionTestEncryptor struct{}

func (upstreamConnectionTestEncryptor) Encrypt(plaintext string) (string, error) {
	return base64.StdEncoding.EncodeToString([]byte(plaintext)), nil
}

func (upstreamConnectionTestEncryptor) Decrypt(ciphertext string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(ciphertext)
	return string(decoded), err
}

type upstreamConnectionTestRepo struct {
	connection         *UpstreamConnection
	createCalls        int
	items              []*UpstreamConnection
	deleteErr          error
	updateApplyResult  *bool
	credentialCAS      *UpstreamConnectionCredentialPersistence
	applyProbeResult   *bool
	probeUpdate        *UpstreamConnectionProbePersistence
	probeFailure       *UpstreamConnectionProbeFailure
	binding            *UpstreamAccountBinding
	bindingApplyResult *bool
	dueConnections     []*UpstreamConnection
	dueBindings        []UpstreamAccountBinding
}

type upstreamBindingAccountRepo struct {
	AccountRepository
	account     *Account
	updateCalls int
}

func (r *upstreamBindingAccountRepo) GetByID(_ context.Context, _ int64) (*Account, error) {
	copy := *r.account
	return &copy, nil
}

func (r *upstreamBindingAccountRepo) GetByIDs(_ context.Context, _ []int64) ([]*Account, error) {
	copy := *r.account
	return []*Account{&copy}, nil
}

func (r *upstreamBindingAccountRepo) Update(_ context.Context, _ *Account) error {
	r.updateCalls++
	return nil
}

func (r *upstreamConnectionTestRepo) Create(_ context.Context, connection *UpstreamConnection) error {
	r.createCalls++
	copy := *connection
	copy.ID = 41
	copy.CreatedAt = time.Unix(100, 0)
	copy.UpdatedAt = time.Unix(100, 0)
	r.connection = &copy
	connection.ID = copy.ID
	connection.CreatedAt = copy.CreatedAt
	connection.UpdatedAt = copy.UpdatedAt
	return nil
}

func (r *upstreamConnectionTestRepo) GetByID(_ context.Context, _ int64) (*UpstreamConnection, error) {
	if r.connection == nil {
		return nil, ErrUpstreamConnectionNotFound
	}
	copy := *r.connection
	return &copy, nil
}

func (r *upstreamConnectionTestRepo) GetByLegacyMigrationKey(_ context.Context, key string) (*UpstreamConnection, error) {
	if r.connection == nil || r.connection.LegacyMigrationKey != key {
		return nil, ErrUpstreamConnectionNotFound
	}
	copy := *r.connection
	return &copy, nil
}

func (r *upstreamConnectionTestRepo) List(_ context.Context, _ UpstreamConnectionListParams) ([]*UpstreamConnection, int64, error) {
	return r.items, int64(len(r.items)), nil
}

func (r *upstreamConnectionTestRepo) UpdateIfVersion(_ context.Context, connection *UpstreamConnection, expectedVersion int64, resetBindings bool) (bool, error) {
	if r.updateApplyResult != nil && !*r.updateApplyResult {
		return false, nil
	}
	if r.connection == nil || r.connection.Version != expectedVersion {
		return false, nil
	}
	copy := *connection
	if resetBindings {
		copy.Groups = []UpstreamGroup{}
		copy.GroupCount = 0
		if r.binding != nil {
			r.binding.RemoteTokenID = ""
			r.binding.RemoteTokenName = ""
			r.binding.ResolutionKind = UpstreamBindingResolutionUnresolved
			r.binding.RemoteGroupID = ""
			r.binding.RemoteGroupName = ""
			r.binding.ObservedMultiplier = nil
			r.binding.Status = UpstreamBindingStatusPending
			r.binding.LastError = ""
		}
	}
	r.connection = &copy
	return true, nil
}

func (r *upstreamConnectionTestRepo) DeleteIfUnbound(_ context.Context, _ int64) error {
	return r.deleteErr
}

func (r *upstreamConnectionTestRepo) UpdateCredentialIfVersion(_ context.Context, _ int64, expectedVersion int64, update UpstreamConnectionCredentialPersistence) (bool, error) {
	if r.connection == nil || r.connection.Version != expectedVersion {
		return false, nil
	}
	r.credentialCAS = &update
	r.connection.CredentialEncrypted = update.CredentialEncrypted
	r.connection.CredentialFingerprint = update.CredentialFingerprint
	r.connection.CredentialHint = update.CredentialHint
	r.connection.Version = update.Version
	return true, nil
}

func (r *upstreamConnectionTestRepo) FinalizeCredentialRefresh(
	_ context.Context,
	_ int64,
	expectedCiphertext, expectedProvider, expectedAuthMode, expectedManagementBaseURL string,
	update UpstreamConnectionCredentialPersistence,
) (bool, error) {
	if r.connection == nil ||
		r.connection.CredentialEncrypted != expectedCiphertext ||
		r.connection.Provider != expectedProvider ||
		r.connection.AuthMode != expectedAuthMode ||
		r.connection.ManagementBaseURL != expectedManagementBaseURL {
		return false, nil
	}
	r.credentialCAS = &update
	r.connection.CredentialEncrypted = update.CredentialEncrypted
	r.connection.CredentialFingerprint = update.CredentialFingerprint
	r.connection.CredentialHint = update.CredentialHint
	r.connection.Version++
	return true, nil
}

func (r *upstreamConnectionTestRepo) ApplyProbeSuccess(_ context.Context, _ int64, _ int64, update UpstreamConnectionProbePersistence) (bool, error) {
	r.probeUpdate = &update
	if r.applyProbeResult != nil && !*r.applyProbeResult {
		return false, nil
	}
	if r.connection != nil {
		r.connection.RemoteUserID = update.RemoteUserID
		r.connection.Capabilities = update.Capabilities
		r.connection.Status = update.Status
		r.connection.LastError = update.LastError
		r.connection.SyncFailures = update.SyncFailures
		r.connection.Version = update.Version
		r.connection.Groups = append([]UpstreamGroup{}, update.Groups...)
		if update.WalletObserved {
			r.connection.WalletAmount = update.WalletAmount
			r.connection.WalletCurrency = update.WalletCurrency
			r.connection.WalletUSD = update.WalletUSD
			r.connection.WalletObservedAt = update.WalletObservedAt
		}
	}
	return true, nil
}

func (r *upstreamConnectionTestRepo) RecordProbeFailure(_ context.Context, _ int64, _ int64, failure UpstreamConnectionProbeFailure) (bool, error) {
	r.probeFailure = &failure
	return true, nil
}

func (r *upstreamConnectionTestRepo) ListDueConnections(_ context.Context, _ time.Time, limit int) ([]*UpstreamConnection, error) {
	items := r.dueConnections
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (r *upstreamConnectionTestRepo) ListDueAccountBindings(_ context.Context, _ int64, _ time.Time, limit int) ([]UpstreamAccountBinding, error) {
	items := r.dueBindings
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (r *upstreamConnectionTestRepo) UpsertAccountBindingIfCurrent(_ context.Context, binding *UpstreamAccountBinding, expectedConnectionVersion int64) (bool, error) {
	if r.bindingApplyResult != nil && !*r.bindingApplyResult {
		return false, nil
	}
	if r.connection == nil || r.connection.Version != expectedConnectionVersion {
		return false, nil
	}
	copy := *binding
	copy.ID = 77
	r.binding = &copy
	if r.connection != nil {
		r.connection.Bindings = []UpstreamAccountBinding{copy}
		r.connection.BindingCount = 1
	}
	*binding = copy
	return true, nil
}

func (r *upstreamConnectionTestRepo) UpdateAccountBindingIfCurrent(_ context.Context, binding *UpstreamAccountBinding, expectedConnectionID, expectedConnectionVersion int64) (bool, error) {
	if r.binding == nil || r.binding.ConnectionID != expectedConnectionID || r.connection == nil || r.connection.Version != expectedConnectionVersion {
		return false, nil
	}
	copy := *binding
	r.binding = &copy
	return true, nil
}

func (r *upstreamConnectionTestRepo) GetAccountBinding(_ context.Context, _ int64) (*UpstreamAccountBinding, error) {
	if r.binding == nil {
		return nil, ErrUpstreamAccountBindingNotFound
	}
	copy := *r.binding
	return &copy, nil
}

func (r *upstreamConnectionTestRepo) DeleteAccountBinding(_ context.Context, _, _ int64) error {
	if r.binding == nil {
		return ErrUpstreamAccountBindingNotFound
	}
	r.binding = nil
	return nil
}

func TestUpstreamConnectionServiceCreateEncryptsAndRedactsCredential(t *testing.T) {
	repo := &upstreamConnectionTestRepo{}
	service := NewUpstreamConnectionService(repo, upstreamConnectionTestEncryptor{}, nil)

	connection, err := service.Create(context.Background(), UpstreamConnectionCreateParams{
		Name:                "Primary upstream",
		Provider:            UpstreamConnectionProviderNewAPI,
		AuthMode:            string(UpstreamManagementAuthModePassword),
		ManagementBaseURL:   "https://newapi.example.com/",
		ForwardingBaseURL:   "https://gateway.example.com/",
		Credential:          UpstreamConnectionCredentialInput{Username: "alice", Password: "do-not-return"},
		SyncEnabled:         true,
		SyncIntervalSeconds: 300,
	})
	require.NoError(t, err)
	require.Equal(t, int64(41), connection.ID)
	require.Empty(t, connection.CredentialEncrypted)
	require.Empty(t, connection.CredentialFingerprint)
	require.Equal(t, "alice", connection.CredentialHint)
	require.Equal(t, "https://newapi.example.com", connection.ManagementBaseURL)
	require.NotEmpty(t, repo.connection.CredentialEncrypted)
	require.NotContains(t, repo.connection.CredentialEncrypted, "do-not-return")
	require.Contains(t, repo.connection.CredentialFingerprint, "sha256:v1:")

	credential, err := service.loadCredential(repo.connection)
	require.NoError(t, err)
	require.Equal(t, "alice", credential.Username)
	require.Equal(t, "do-not-return", credential.Password)
}

func TestUpstreamConnectionCredentialIdentityBoundsDisplayHint(t *testing.T) {
	username := strings.Repeat("用", 120) + "@example.com"
	_, _, hint, err := upstreamConnectionCredentialIdentity(
		string(UpstreamManagementAuthModePassword),
		"https://console.example.com",
		UpstreamConnectionCredentialInput{Username: username, Password: "secret"},
	)

	require.NoError(t, err)
	require.LessOrEqual(t, utf8.RuneCountInString(hint), 100)
	require.True(t, utf8.ValidString(hint))
}

func TestUpstreamConnectionCredentialIdentityMasksUnicodeTokenAsValidUTF8(t *testing.T) {
	_, _, hint, err := upstreamConnectionCredentialIdentity(
		string(UpstreamManagementAuthModeAccessToken),
		"https://console.example.com",
		UpstreamConnectionCredentialInput{AccessToken: strings.Repeat("令牌", 5)},
	)

	require.NoError(t, err)
	require.True(t, utf8.ValidString(hint))
	require.Equal(t, "令牌令牌...令牌令牌", hint)
}

func TestUpstreamConnectionServiceCreateRejectsSecretsAndQueriesInBaseURL(t *testing.T) {
	service := NewUpstreamConnectionService(&upstreamConnectionTestRepo{}, upstreamConnectionTestEncryptor{}, nil)
	base := UpstreamConnectionCreateParams{
		Name: "NewAPI", Provider: UpstreamConnectionProviderNewAPI,
		AuthMode:    string(UpstreamManagementAuthModePassword),
		Credential:  UpstreamConnectionCredentialInput{Username: "alice", Password: "secret"},
		SyncEnabled: true, SyncIntervalSeconds: 300,
	}

	withCredentials := base
	withCredentials.ManagementBaseURL = "https://alice:secret@example.com"
	_, err := service.Create(context.Background(), withCredentials)
	require.Error(t, err)
	require.Equal(t, "INVALID_UPSTREAM_MANAGEMENT_URL", infraerrors.Reason(err))

	withQuery := base
	withQuery.ManagementBaseURL = "https://example.com/console?token=secret"
	_, err = service.Create(context.Background(), withQuery)
	require.Error(t, err)
	require.Equal(t, "INVALID_UPSTREAM_MANAGEMENT_URL", infraerrors.Reason(err))
}

func TestUpstreamConnectionServiceCreateRequiresRemoteUserIDForExplicitNewAPIAccessToken(t *testing.T) {
	service := NewUpstreamConnectionService(&upstreamConnectionTestRepo{}, upstreamConnectionTestEncryptor{}, nil)

	_, err := service.Create(context.Background(), UpstreamConnectionCreateParams{
		Name: "NewAPI", Provider: UpstreamConnectionProviderNewAPI,
		AuthMode: string(UpstreamManagementAuthModeAccessToken), ManagementBaseURL: "https://example.com",
		Credential:  UpstreamConnectionCredentialInput{AccessToken: "management-token"},
		SyncEnabled: true, SyncIntervalSeconds: 300,
	})

	require.Error(t, err)
	require.Equal(t, "UPSTREAM_REMOTE_USER_ID_REQUIRED", infraerrors.Reason(err))
}

func TestUpstreamConnectionServiceUpdateIdentityClearsStaleObservations(t *testing.T) {
	wallet := 99.5
	observedMultiplier := 0.45
	now := time.Now()
	repo := &upstreamConnectionTestRepo{}
	service := NewUpstreamConnectionService(repo, upstreamConnectionTestEncryptor{}, nil)
	ciphertext, fingerprint, hint, err := service.encryptCredential(
		string(UpstreamManagementAuthModePassword), "https://old.example.com",
		UpstreamConnectionCredentialInput{Username: "alice", Password: "old-password"},
	)
	require.NoError(t, err)
	repo.connection = &UpstreamConnection{
		ID: 8, Name: "Old", Provider: UpstreamConnectionProviderNewAPI,
		AuthMode:          string(UpstreamManagementAuthModePassword),
		ManagementBaseURL: "https://old.example.com", CredentialEncrypted: ciphertext,
		CredentialFingerprint: fingerprint, CredentialHint: hint, LegacyMigrationKey: "legacy-key",
		Status: UpstreamConnectionStatusReady, SyncEnabled: true, SyncIntervalSeconds: 300,
		Version: 4, WalletAmount: &wallet, WalletCurrency: "USD", WalletUSD: &wallet,
		WalletRaw: map[string]any{"balance": wallet}, WalletObservedAt: &now,
		Capabilities: map[string]any{"groups": true}, LastSyncedAt: &now,
		Groups:     []UpstreamGroup{{ID: 71, ConnectionID: 8, Name: "old-group", RateMultiplier: &observedMultiplier}},
		GroupCount: 1,
	}
	repo.binding = &UpstreamAccountBinding{
		ID: 91, AccountID: 12, ConnectionID: 8, RemoteTokenID: "old-token",
		ResolutionKind: UpstreamBindingResolutionFixed, RemoteGroupName: "old-group",
		ObservedMultiplier: &observedMultiplier, Status: UpstreamBindingStatusReady,
	}
	nextURL := "https://next.example.com/"

	updated, err := service.Update(context.Background(), 8, UpstreamConnectionUpdateParams{ExpectedVersion: 4, ManagementBaseURL: &nextURL})
	require.NoError(t, err)
	require.Equal(t, UpstreamConnectionStatusPending, updated.Status)
	require.Nil(t, updated.WalletAmount)
	require.Nil(t, updated.WalletObservedAt)
	require.Empty(t, updated.Capabilities)
	require.Empty(t, updated.Groups)
	require.Zero(t, updated.GroupCount)
	require.Equal(t, int64(5), updated.Version)
	require.Equal(t, ciphertext, repo.connection.CredentialEncrypted)
	require.NotEqual(t, fingerprint, repo.connection.CredentialFingerprint)
	require.Empty(t, repo.connection.LegacyMigrationKey)
	require.Equal(t, UpstreamBindingStatusPending, repo.binding.Status)
	require.Equal(t, UpstreamBindingResolutionUnresolved, repo.binding.ResolutionKind)
	require.Nil(t, repo.binding.ObservedMultiplier)
	require.Empty(t, repo.binding.RemoteGroupName)
}

func TestUpstreamConnectionServiceUpdateRejectsConcurrentProbeWrite(t *testing.T) {
	apply := false
	repo := &upstreamConnectionTestRepo{updateApplyResult: &apply, connection: &UpstreamConnection{
		ID: 10, Name: "before", Provider: UpstreamConnectionProviderNewAPI,
		AuthMode: string(UpstreamManagementAuthModePassword), ManagementBaseURL: "https://example.com",
		CredentialEncrypted: "stored", SyncEnabled: true, SyncIntervalSeconds: 300, Version: 4,
	}}
	connectionService := NewUpstreamConnectionService(repo, upstreamConnectionTestEncryptor{}, nil)
	name := "after"

	_, err := connectionService.Update(context.Background(), 10, UpstreamConnectionUpdateParams{ExpectedVersion: 4, Name: &name})
	require.ErrorIs(t, err, ErrUpstreamConnectionChanged)
	require.Equal(t, "before", repo.connection.Name)
	require.Equal(t, int64(4), repo.connection.Version)
}

func TestUpstreamConnectionServiceUpdateRejectsStaleClientVersionBeforeMutation(t *testing.T) {
	repo := &upstreamConnectionTestRepo{connection: &UpstreamConnection{
		ID: 10, Name: "current", Provider: UpstreamConnectionProviderNewAPI,
		AuthMode: string(UpstreamManagementAuthModePassword), ManagementBaseURL: "https://example.com",
		CredentialEncrypted: "stored", SyncEnabled: true, SyncIntervalSeconds: 300, Version: 5,
	}}
	connectionService := NewUpstreamConnectionService(repo, upstreamConnectionTestEncryptor{}, nil)
	name := "stale browser value"

	_, err := connectionService.Update(context.Background(), 10, UpstreamConnectionUpdateParams{
		ExpectedVersion: 4,
		Name:            &name,
	})

	require.ErrorIs(t, err, ErrUpstreamConnectionChanged)
	require.Equal(t, "current", repo.connection.Name)
	require.Equal(t, int64(5), repo.connection.Version)
}

func TestUpstreamConnectionServiceUpdateReschedulesWhenIntervalChanges(t *testing.T) {
	nextSync := time.Now().UTC().Add(time.Hour)
	repo := &upstreamConnectionTestRepo{connection: &UpstreamConnection{
		ID: 10, Name: "connection", Provider: UpstreamConnectionProviderNewAPI,
		AuthMode: string(UpstreamManagementAuthModePassword), ManagementBaseURL: "https://example.com",
		CredentialEncrypted: "stored", Status: UpstreamConnectionStatusReady,
		SyncEnabled: true, SyncIntervalSeconds: 3600, NextSyncAt: &nextSync, Version: 4,
	}}
	service := NewUpstreamConnectionService(repo, upstreamConnectionTestEncryptor{}, nil)
	interval := 60
	started := time.Now().UTC()

	updated, err := service.Update(context.Background(), 10, UpstreamConnectionUpdateParams{ExpectedVersion: 4, SyncIntervalSeconds: &interval})

	require.NoError(t, err)
	require.NotNil(t, updated.NextSyncAt)
	require.False(t, updated.NextSyncAt.Before(started))
	require.Less(t, updated.NextSyncAt.Sub(started), time.Second)
}

func TestUpstreamConnectionServicePasswordRotationPreservesLegacyMigrationIdentity(t *testing.T) {
	repo := &upstreamConnectionTestRepo{}
	service := NewUpstreamConnectionService(repo, upstreamConnectionTestEncryptor{}, nil)
	ciphertext, fingerprint, hint, err := service.encryptCredential(
		string(UpstreamManagementAuthModePassword), "https://upstream.example.com",
		UpstreamConnectionCredentialInput{Username: "alice", Password: "old-password"},
	)
	require.NoError(t, err)
	repo.connection = &UpstreamConnection{
		ID: 18, Name: "Migrated", Provider: UpstreamConnectionProviderNewAPI,
		AuthMode: string(UpstreamManagementAuthModePassword), ManagementBaseURL: "https://upstream.example.com",
		CredentialEncrypted: ciphertext, CredentialFingerprint: fingerprint, CredentialHint: hint,
		LegacyMigrationKey: "legacy-key", SyncEnabled: true, SyncIntervalSeconds: 300, Version: 1,
	}
	replacement := UpstreamConnectionCredentialInput{Username: "alice", Password: "new-password"}

	_, err = service.Update(context.Background(), 18, UpstreamConnectionUpdateParams{ExpectedVersion: 1, Credential: &replacement})
	require.NoError(t, err)
	require.Equal(t, "legacy-key", repo.connection.LegacyMigrationKey)
	require.Equal(t, fingerprint, repo.connection.CredentialFingerprint)
	credential, err := service.loadCredential(repo.connection)
	require.NoError(t, err)
	require.Equal(t, "new-password", credential.Password)
}

func TestUpstreamConnectionServiceUpdateAuthModeRequiresReplacementCredential(t *testing.T) {
	repo := &upstreamConnectionTestRepo{connection: &UpstreamConnection{
		ID: 9, Name: "Upstream", Provider: UpstreamConnectionProviderNewAPI,
		AuthMode: string(UpstreamManagementAuthModePassword), ManagementBaseURL: "https://example.com",
		CredentialEncrypted: "persisted", SyncEnabled: true, SyncIntervalSeconds: 300, Version: 1,
	}}
	service := NewUpstreamConnectionService(repo, upstreamConnectionTestEncryptor{}, nil)
	nextMode := string(UpstreamManagementAuthModeAccessToken)

	_, err := service.Update(context.Background(), 9, UpstreamConnectionUpdateParams{ExpectedVersion: 1, AuthMode: &nextMode})
	require.Error(t, err)
	require.Equal(t, "UPSTREAM_CONNECTION_CREDENTIAL_REQUIRED", infraerrors.Reason(err))
}

func TestTruncateUpstreamConnectionErrorPreservesUTF8(t *testing.T) {
	message := strings.Repeat("a", 1999) + "中" + strings.Repeat("b", 20)

	truncated := truncateUpstreamConnectionError(message)

	require.True(t, utf8.ValidString(truncated))
	require.LessOrEqual(t, len(truncated), 2000)
	require.Equal(t, strings.Repeat("a", 1999), truncated)
}

func TestUpstreamConnectionServiceFirstTransientProbeFailureIsDegraded(t *testing.T) {
	repo := &upstreamConnectionTestRepo{}
	connectionService := NewUpstreamConnectionService(repo, upstreamConnectionTestEncryptor{}, nil)
	connection := &UpstreamConnection{
		ID: 5, Status: UpstreamConnectionStatusPending, SyncEnabled: true, Version: 1,
	}

	connectionService.recordProbeFailure(context.Background(), connection, 1, errors.New("dial tcp: i/o timeout"))

	require.NotNil(t, repo.probeFailure)
	require.Equal(t, UpstreamConnectionStatusDegraded, repo.probeFailure.Status)
}

func TestUpstreamConnectionServiceAuthenticationProbeFailureIsAuthError(t *testing.T) {
	repo := &upstreamConnectionTestRepo{}
	connectionService := NewUpstreamConnectionService(repo, upstreamConnectionTestEncryptor{}, nil)
	connection := &UpstreamConnection{
		ID: 5, Status: UpstreamConnectionStatusPending, SyncEnabled: true, Version: 1,
	}

	connectionService.recordProbeFailure(context.Background(), connection, 1, ErrUpstreamConnectionAuthentication)

	require.NotNil(t, repo.probeFailure)
	require.Equal(t, UpstreamConnectionStatusAuthError, repo.probeFailure.Status)
}

func TestUpstreamConnectionServiceListNeverReturnsStoredSecrets(t *testing.T) {
	repo := &upstreamConnectionTestRepo{items: []*UpstreamConnection{{
		ID: 1, CredentialEncrypted: "ciphertext", CredentialFingerprint: "fingerprint", LegacyMigrationKey: "migration-key",
		Bindings: []UpstreamAccountBinding{{ID: 2, KeyFingerprint: "key-fingerprint"}},
	}}}
	service := NewUpstreamConnectionService(repo, upstreamConnectionTestEncryptor{}, nil)

	items, _, err := service.List(context.Background(), UpstreamConnectionListParams{})
	require.NoError(t, err)
	require.Empty(t, items[0].CredentialEncrypted)
	require.Empty(t, items[0].CredentialFingerprint)
	require.Empty(t, items[0].LegacyMigrationKey)
	require.Empty(t, items[0].Bindings[0].KeyFingerprint)
}

func TestUpstreamConnectionServiceDeletePreservesInUseConflict(t *testing.T) {
	repo := &upstreamConnectionTestRepo{deleteErr: ErrUpstreamConnectionInUse}
	service := NewUpstreamConnectionService(repo, upstreamConnectionTestEncryptor{}, nil)

	err := service.Delete(context.Background(), 5)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrUpstreamConnectionInUse))
}

func TestUpstreamConnectionServiceBindAccountIsObserveOnlyAndDoesNotChangeBillingMultiplier(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		require.Equal(t, "/api/token/search", request.URL.Path)
		require.Equal(t, "sk-forwarding-key", request.URL.Query().Get("token"))
		writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{"items": []any{
			map[string]any{"id": 51, "name": "bound key", "group": "vip"},
		}}})
	}))
	defer server.Close()

	repo := &upstreamConnectionTestRepo{}
	connectionService := NewUpstreamConnectionService(repo, upstreamConnectionTestEncryptor{}, nil)
	ciphertext, fingerprint, hint, err := connectionService.encryptCredential(
		string(UpstreamManagementAuthModeAccessToken), server.URL,
		UpstreamConnectionCredentialInput{AccessToken: "management-token"},
	)
	require.NoError(t, err)
	multiplier := 0.25
	repo.connection = &UpstreamConnection{
		ID: 5, Name: "NewAPI", Provider: UpstreamConnectionProviderNewAPI,
		AuthMode: string(UpstreamManagementAuthModeAccessToken), ManagementBaseURL: server.URL,
		CredentialEncrypted: ciphertext, CredentialFingerprint: fingerprint, CredentialHint: hint,
		RemoteUserID: "9", SyncEnabled: true, SyncIntervalSeconds: 300, Version: 1,
		Groups: []UpstreamGroup{{Name: "vip", RateMultiplier: &multiplier}},
	}
	existingBillingMultiplier := 1.7
	accountRepo := &upstreamBindingAccountRepo{account: &Account{
		ID: 19, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-forwarding-key"},
		RateMultiplier: &existingBillingMultiplier,
	}}
	connectionService.accountRepo = accountRepo
	connectionService.inspector = newUpstreamConnectionInspector(nil, nil, server.Client())

	binding, err := connectionService.BindAccount(context.Background(), 5, 19)
	require.NoError(t, err)
	require.Equal(t, UpstreamBindingApplyObserveOnly, binding.ApplyPolicy)
	require.Equal(t, UpstreamBindingResolutionFixed, binding.ResolutionKind)
	require.Equal(t, 0.25, *binding.ObservedMultiplier)
	require.Empty(t, binding.KeyFingerprint)
	require.NotEmpty(t, repo.binding.KeyFingerprint)
	require.Equal(t, 0, accountRepo.updateCalls)
	require.Equal(t, 1.7, *accountRepo.account.RateMultiplier)
}

func TestUpstreamConnectionServiceBindAccountRejectsStaleConnectionVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{"items": []any{
			map[string]any{"id": 51, "name": "bound key", "group": "vip"},
		}}})
	}))
	defer server.Close()

	apply := false
	repo := &upstreamConnectionTestRepo{bindingApplyResult: &apply}
	connectionService := NewUpstreamConnectionService(repo, upstreamConnectionTestEncryptor{}, nil)
	ciphertext, fingerprint, hint, err := connectionService.encryptCredential(
		string(UpstreamManagementAuthModeAccessToken), server.URL,
		UpstreamConnectionCredentialInput{AccessToken: "management-token"},
	)
	require.NoError(t, err)
	multiplier := 0.25
	repo.connection = &UpstreamConnection{
		ID: 5, Name: "NewAPI", Provider: UpstreamConnectionProviderNewAPI,
		AuthMode: string(UpstreamManagementAuthModeAccessToken), ManagementBaseURL: server.URL,
		CredentialEncrypted: ciphertext, CredentialFingerprint: fingerprint, CredentialHint: hint,
		RemoteUserID: "9", SyncEnabled: true, SyncIntervalSeconds: 300, Version: 1,
		Groups: []UpstreamGroup{{Name: "vip", RateMultiplier: &multiplier}},
	}
	existingBillingMultiplier := 1.7
	accountRepo := &upstreamBindingAccountRepo{account: &Account{
		ID: 19, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-forwarding-key"},
		RateMultiplier: &existingBillingMultiplier,
	}}
	connectionService.accountRepo = accountRepo
	connectionService.inspector = newUpstreamConnectionInspector(nil, nil, server.Client())

	_, err = connectionService.BindAccount(context.Background(), 5, 19)

	require.ErrorIs(t, err, ErrUpstreamConnectionChanged)
	require.Nil(t, repo.binding)
	require.Equal(t, 0, accountRepo.updateCalls)
	require.Equal(t, 1.7, *accountRepo.account.RateMultiplier)
}

func TestUpstreamConnectionServiceRefreshBindingRemainsObserveOnly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		require.Equal(t, "/api/token/search", request.URL.Path)
		writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{"items": []any{
			map[string]any{"id": 12, "name": "refreshed key", "group": "vip"},
		}}})
	}))
	defer server.Close()

	multiplier := 0.4
	connection := &UpstreamConnection{
		ID: 7, Provider: UpstreamConnectionProviderNewAPI,
		AuthMode: string(UpstreamManagementAuthModeAccessToken), ManagementBaseURL: server.URL,
		RemoteUserID: "3", SyncIntervalSeconds: 300,
		Groups: []UpstreamGroup{{Name: "vip", RateMultiplier: &multiplier}},
	}
	existingBillingMultiplier := 1.8
	accountRepo := &upstreamBindingAccountRepo{account: &Account{
		ID: 21, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-refresh-key"},
		RateMultiplier: &existingBillingMultiplier,
	}}
	binding := &UpstreamAccountBinding{
		ID: 91, AccountID: 21, ConnectionID: 7, KeyFingerprint: "old",
		ResolutionKind: UpstreamBindingResolutionUnresolved, Status: UpstreamBindingStatusPending,
		ApplyPolicy: UpstreamBindingApplyObserveOnly,
	}
	repo := &upstreamConnectionTestRepo{connection: connection, binding: binding}
	connectionService := NewUpstreamConnectionService(repo, upstreamConnectionTestEncryptor{}, nil)
	connectionService.accountRepo = accountRepo
	connectionService.inspector = newUpstreamConnectionInspector(nil, nil, server.Client())
	resolver, err := connectionService.inspector.PrepareKeyResolver(context.Background(), connection, upstreamConnectionCredential{
		Version: 1, AccessToken: "management-token",
	})
	require.NoError(t, err)

	err = connectionService.refreshAccountBinding(context.Background(), connection, resolver, binding)
	require.NoError(t, err)
	require.Equal(t, UpstreamBindingStatusReady, repo.binding.Status)
	require.Equal(t, 0.4, *repo.binding.ObservedMultiplier)
	require.Equal(t, 0, repo.binding.SyncFailures)
	require.Equal(t, 0, accountRepo.updateCalls)
	require.Equal(t, 1.8, *accountRepo.account.RateMultiplier)
}

func TestUpstreamConnectionServiceRefreshBindingRejectsStaleConnectionVersion(t *testing.T) {
	connection := &UpstreamConnection{ID: 7, Version: 2, SyncIntervalSeconds: 300}
	binding := &UpstreamAccountBinding{
		ID: 92, AccountID: 22, ConnectionID: 7, ResolutionKind: UpstreamBindingResolutionUnresolved,
		Status: UpstreamBindingStatusPending, ApplyPolicy: UpstreamBindingApplyObserveOnly,
	}
	repo := &upstreamConnectionTestRepo{
		connection: &UpstreamConnection{ID: 7, Version: 3, SyncIntervalSeconds: 300}, binding: binding,
	}
	accountRepo := &upstreamBindingAccountRepo{account: &Account{
		ID: 22, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-current"},
	}}
	connectionService := NewUpstreamConnectionService(repo, upstreamConnectionTestEncryptor{}, nil)
	connectionService.accountRepo = accountRepo
	resolver := func(context.Context, string) (UpstreamAccountBinding, error) {
		multiplier := 0.25
		return UpstreamAccountBinding{
			ResolutionKind: UpstreamBindingResolutionFixed, RemoteGroupName: "new-group",
			ObservedMultiplier: &multiplier, Status: UpstreamBindingStatusReady,
		}, nil
	}

	err := connectionService.refreshAccountBinding(context.Background(), connection, resolver, binding)
	require.ErrorIs(t, err, ErrUpstreamConnectionChanged)
	require.Equal(t, UpstreamBindingStatusPending, repo.binding.Status)
	require.Nil(t, repo.binding.ObservedMultiplier)
}

func TestUpstreamConnectionCredentialHandoffFollowsEnabledLegacySourceWithoutRefreshing(t *testing.T) {
	managementURL := "https://sub2.example.com"
	legacySecret := upstreamManagementAuthSecret{
		AccessToken: "legacy-current-access", RefreshToken: "legacy-current-refresh",
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}
	legacyCiphertext, err := encryptUpstreamManagementAuthSecret(upstreamConnectionTestEncryptor{}, legacySecret)
	require.NoError(t, err)

	account := upstreamConnectionLegacyHandoffAccount(55, managementURL, legacyCiphertext, true)
	billingMultiplier := 1.75
	account.RateMultiplier = &billingMultiplier
	accountRepo := &upstreamBindingAccountRepo{account: &account}
	repo := &upstreamConnectionTestRepo{}
	connectionService := NewUpstreamConnectionService(repo, upstreamConnectionTestEncryptor{}, nil)
	connectionService.accountRepo = accountRepo
	ciphertext, fingerprint, hint, err := connectionService.encryptCredential(
		string(UpstreamManagementAuthModeAccessToken), managementURL,
		UpstreamConnectionCredentialInput{
			AccessToken: "stale-access", RefreshToken: "stale-refresh",
			ExpiresAt: time.Now().Add(-time.Hour).Unix(), LegacyManaged: true,
		},
	)
	require.NoError(t, err)
	repo.connection = &UpstreamConnection{
		ID: 9, Provider: UpstreamConnectionProviderSub2API,
		AuthMode: string(UpstreamManagementAuthModeAccessToken), ManagementBaseURL: managementURL,
		CredentialEncrypted: ciphertext, CredentialFingerprint: fingerprint, CredentialHint: hint,
		LegacyMigrationKey: "legacy", Version: 3,
		Bindings: []UpstreamAccountBinding{{ID: 1, AccountID: account.ID, ConnectionID: 9}},
	}
	credential, err := connectionService.loadCredential(repo.connection)
	require.NoError(t, err)

	_, prepared, err := connectionService.prepareConnectionCredential(context.Background(), repo.connection, credential)
	require.NoError(t, err)
	require.Equal(t, "legacy-current-access", prepared.AccessToken)
	require.Equal(t, "legacy-current-refresh", prepared.RefreshToken)
	require.True(t, prepared.LegacyManaged)
	require.Equal(t, int64(4), repo.connection.Version)
	require.Zero(t, accountRepo.updateCalls)
	require.Equal(t, 1.75, *accountRepo.account.RateMultiplier)
}

func TestUpstreamConnectionCredentialHandoffRefreshesAfterLegacySourceIsDisabled(t *testing.T) {
	var refreshCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, "/api/v1/auth/refresh", request.URL.Path)
		refreshCalls.Add(1)
		writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{
			"access_token": "v2-access", "refresh_token": "v2-refresh", "expires_in": 3600,
		}})
	}))
	defer server.Close()

	legacySecret := upstreamManagementAuthSecret{
		AccessToken: "handoff-access", RefreshToken: "handoff-refresh", ExpiresAt: time.Now().Add(-time.Minute).Unix(),
	}
	legacyCiphertext, err := encryptUpstreamManagementAuthSecret(upstreamConnectionTestEncryptor{}, legacySecret)
	require.NoError(t, err)
	account := upstreamConnectionLegacyHandoffAccount(77, server.URL, legacyCiphertext, false)
	delete(account.Extra, AccountExtraUpstreamRateMultiplierSyncEnabled)
	delete(account.Extra, AccountExtraUpstreamRateMultiplierSyncGroup)
	delete(account.Extra, AccountExtraUpstreamRateMultiplierSyncProvider)
	delete(account.Extra, AccountExtraUpstreamRateMultiplierSyncAuthMode)
	billingMultiplier := 1.25
	account.RateMultiplier = &billingMultiplier
	accountRepo := &upstreamBindingAccountRepo{account: &account}
	repo := &upstreamConnectionTestRepo{}
	connectionService := NewUpstreamConnectionService(repo, upstreamConnectionTestEncryptor{}, nil)
	connectionService.accountRepo = accountRepo
	connectionService.inspector = newUpstreamConnectionInspector(nil, nil, server.Client())
	ciphertext, fingerprint, hint, err := connectionService.encryptCredential(
		string(UpstreamManagementAuthModeAccessToken), server.URL,
		UpstreamConnectionCredentialInput{
			AccessToken: "older-access", RefreshToken: "older-refresh",
			ExpiresAt: time.Now().Add(-time.Hour).Unix(), LegacyManaged: true,
		},
	)
	require.NoError(t, err)
	repo.connection = &UpstreamConnection{
		ID: 12, Provider: UpstreamConnectionProviderSub2API,
		AuthMode: string(UpstreamManagementAuthModeAccessToken), ManagementBaseURL: server.URL,
		CredentialEncrypted: ciphertext, CredentialFingerprint: fingerprint, CredentialHint: hint,
		LegacyMigrationKey: "legacy", Version: 5,
		Bindings: []UpstreamAccountBinding{{ID: 2, AccountID: account.ID, ConnectionID: 12}},
	}
	credential, err := connectionService.loadCredential(repo.connection)
	require.NoError(t, err)

	_, prepared, err := connectionService.prepareConnectionCredential(context.Background(), repo.connection, credential)
	require.NoError(t, err)
	require.Equal(t, int32(1), refreshCalls.Load())
	require.Equal(t, "v2-access", prepared.AccessToken)
	require.Equal(t, "v2-refresh", prepared.RefreshToken)
	require.Greater(t, prepared.ExpiresAt, time.Now().Unix())
	require.False(t, prepared.LegacyManaged)
	// One write hands off ownership, one claims the rotating token, and one
	// persists the refreshed pair.
	require.Equal(t, int64(8), repo.connection.Version)
	require.Zero(t, accountRepo.updateCalls)
	require.Equal(t, 1.25, *accountRepo.account.RateMultiplier)

	// Re-enabling the old owner after V2 has rotated would create a dual-token
	// race. The connection must stop before either side consumes another token.
	accountRepo.account.Extra[AccountExtraUpstreamRateMultiplierSyncEnabled] = true
	accountRepo.account.Extra[AccountExtraUpstreamRateMultiplierSyncGroup] = "Grok"
	accountRepo.account.Extra[AccountExtraUpstreamRateMultiplierSyncProvider] = string(UpstreamManagementProviderSub2API)
	accountRepo.account.Extra[AccountExtraUpstreamRateMultiplierSyncAuthMode] = string(UpstreamManagementAuthModeAccessToken)
	_, _, err = connectionService.prepareConnectionCredential(context.Background(), repo.connection, prepared)
	require.ErrorIs(t, err, errUpstreamLegacyOwnershipConflict)
	require.Equal(t, int32(1), refreshCalls.Load())
	require.Zero(t, accountRepo.updateCalls)
}

func TestUpstreamConnectionCredentialRefreshStopsWhenDistributedLockIsHeld(t *testing.T) {
	var refreshCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		refreshCalls.Add(1)
		writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{
			"access_token": "next-access", "refresh_token": "next-refresh", "expires_in": 3600,
		}})
	}))
	defer server.Close()

	repo := &upstreamConnectionTestRepo{}
	connectionService := NewUpstreamConnectionService(repo, upstreamConnectionTestEncryptor{}, nil)
	connectionService.inspector = newUpstreamConnectionInspector(nil, nil, server.Client())
	lockCache := &fakeLeaderLockCache{}
	lockKey := "upstream:connections:v2:credential-refresh:13"
	acquired, err := lockCache.TryAcquireLeaderLock(context.Background(), lockKey, "peer", time.Minute)
	require.NoError(t, err)
	require.True(t, acquired)
	connectionService.lockCache = lockCache

	ciphertext, fingerprint, hint, err := connectionService.encryptCredential(
		string(UpstreamManagementAuthModeAccessToken), server.URL,
		UpstreamConnectionCredentialInput{
			AccessToken: "old-access", RefreshToken: "old-refresh", ExpiresAt: time.Now().Add(-time.Minute).Unix(),
		},
	)
	require.NoError(t, err)
	repo.connection = &UpstreamConnection{
		ID: 13, Provider: UpstreamConnectionProviderSub2API,
		AuthMode: string(UpstreamManagementAuthModeAccessToken), ManagementBaseURL: server.URL,
		CredentialEncrypted: ciphertext, CredentialFingerprint: fingerprint, CredentialHint: hint, Version: 1,
	}
	credential, err := connectionService.loadCredential(repo.connection)
	require.NoError(t, err)

	_, _, err = connectionService.prepareConnectionCredential(context.Background(), repo.connection, credential)

	require.ErrorIs(t, err, ErrUpstreamCredentialRefreshBusy)
	require.Zero(t, refreshCalls.Load())
	require.Equal(t, int64(1), repo.connection.Version)
}

func upstreamConnectionLegacyHandoffAccount(id int64, managementURL, ciphertext string, enabled bool) Account {
	return Account{
		ID: id, Name: "legacy source", Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true,
		UpdatedAt: time.Now().UTC(),
		Credentials: map[string]any{
			"api_key": "sk-forwarding", "base_url": managementURL,
			UpstreamManagementBaseURLCredentialKey: managementURL,
			UpstreamManagementAuthCredentialKey:    ciphertext,
		},
		Extra: map[string]any{
			AccountExtraUpstreamRateMultiplierSyncEnabled:  enabled,
			AccountExtraUpstreamRateMultiplierSyncGroup:    "Grok",
			AccountExtraUpstreamRateMultiplierSyncProvider: string(UpstreamManagementProviderSub2API),
			AccountExtraUpstreamRateMultiplierSyncAuthMode: string(UpstreamManagementAuthModeAccessToken),
		},
	}
}
