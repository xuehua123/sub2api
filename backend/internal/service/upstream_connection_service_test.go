//go:build unit

package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
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
	connection            *UpstreamConnection
	createCalls           int
	items                 []*UpstreamConnection
	deleteErr             error
	updateApplyResult     *bool
	lastResetBindings     bool
	credentialCAS         *UpstreamConnectionCredentialPersistence
	applyProbeResult      *bool
	probeUpdate           *UpstreamConnectionProbePersistence
	probeFailure          *UpstreamConnectionProbeFailure
	binding               *UpstreamAccountBinding
	bindingApplyResult    *bool
	appliedRateMultiplier *float64
	dueConnections        []*UpstreamConnection
	dueBindings           []UpstreamAccountBinding
}

type upstreamBindingAccountRepo struct {
	AccountRepository
	account     *Account
	updateCalls int
}

type upstreamConnectionUsageReaderStub struct {
	buckets    []UpstreamConnectionAccountUsageBucket
	accountIDs []int64
}

func (r *upstreamConnectionUsageReaderStub) GetUpstreamAccountUsageBuckets(
	_ context.Context,
	accountIDs []int64,
	_, _ time.Time,
	_ string,
) ([]UpstreamConnectionAccountUsageBucket, error) {
	r.accountIDs = append([]int64(nil), accountIDs...)
	return append([]UpstreamConnectionAccountUsageBucket(nil), r.buckets...), nil
}

type upstreamConnectionUsageAccountRepo struct {
	AccountRepository
	accounts []*Account
}

func (r *upstreamConnectionUsageAccountRepo) GetByIDs(_ context.Context, _ []int64) ([]*Account, error) {
	return r.accounts, nil
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

func (r *upstreamConnectionTestRepo) List(_ context.Context, _ UpstreamConnectionListParams) ([]*UpstreamConnection, int64, error) {
	return r.items, int64(len(r.items)), nil
}

func (r *upstreamConnectionTestRepo) UpdateIfVersion(_ context.Context, connection *UpstreamConnection, expectedVersion int64, resetBindings bool) (bool, error) {
	r.lastResetBindings = resetBindings
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

func (r *upstreamConnectionTestRepo) UpsertAccountBindingIfCurrent(_ context.Context, binding *UpstreamAccountBinding, expectedConnectionVersion int64, rateMultiplier *float64) (bool, error) {
	if r.bindingApplyResult != nil && !*r.bindingApplyResult {
		return false, nil
	}
	if r.connection == nil || r.connection.Version != expectedConnectionVersion {
		return false, nil
	}
	r.appliedRateMultiplier = cloneFloat64Ptr(rateMultiplier)
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

func (r *upstreamConnectionTestRepo) UpdateAccountBindingIfCurrent(_ context.Context, binding *UpstreamAccountBinding, expectedConnectionID, expectedConnectionVersion int64, rateMultiplier *float64) (bool, error) {
	if r.binding == nil || r.binding.ConnectionID != expectedConnectionID || r.connection == nil || r.connection.Version != expectedConnectionVersion {
		return false, nil
	}
	r.appliedRateMultiplier = cloneFloat64Ptr(rateMultiplier)
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

func TestUpstreamConnectionServiceEncryptsCredentialUserAgent(t *testing.T) {
	repo := &upstreamConnectionTestRepo{}
	connectionService := NewUpstreamConnectionService(repo, upstreamConnectionTestEncryptor{}, nil)

	connection, err := connectionService.Create(context.Background(), UpstreamConnectionCreateParams{
		Name:              "UA-bound upstream",
		Provider:          UpstreamConnectionProviderSub2API,
		AuthMode:          string(UpstreamManagementAuthModeAccessToken),
		ManagementBaseURL: "https://sub2api.example.com",
		Credential: UpstreamConnectionCredentialInput{
			AccessToken: "management-token",
			UserAgent:   "Mozilla/5.0 exact-login-agent",
		},
		SyncEnabled: true,
	})
	require.NoError(t, err)
	require.Empty(t, connection.CredentialEncrypted)
	require.NotContains(t, repo.connection.CredentialEncrypted, "exact-login-agent")

	credential, err := connectionService.loadCredential(repo.connection)
	require.NoError(t, err)
	require.Equal(t, "Mozilla/5.0 exact-login-agent", credential.UserAgent)
}

func TestUpstreamConnectionCredentialRejectsUnsafeUserAgent(t *testing.T) {
	for _, userAgent := range []string{
		"Mozilla/5.0\r\nX-Injected: true",
		"Mozilla/5.0\x00suffix",
		strings.Repeat("a", 513),
		string([]byte{'M', 'o', 'z', 0xff}),
	} {
		_, _, _, err := upstreamConnectionCredentialIdentity(
			string(UpstreamManagementAuthModeAccessToken),
			"https://sub2api.example.com",
			UpstreamConnectionCredentialInput{AccessToken: "management-token", UserAgent: userAgent},
		)
		require.Error(t, err)
	}
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
		CredentialFingerprint: fingerprint, CredentialHint: hint,
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
		ID: 1, CredentialEncrypted: "ciphertext", CredentialFingerprint: "fingerprint",
		Bindings: []UpstreamAccountBinding{{ID: 2, KeyFingerprint: "key-fingerprint"}},
	}}}
	service := NewUpstreamConnectionService(repo, upstreamConnectionTestEncryptor{}, nil)

	items, _, err := service.List(context.Background(), UpstreamConnectionListParams{})
	require.NoError(t, err)
	require.Empty(t, items[0].CredentialEncrypted)
	require.Empty(t, items[0].CredentialFingerprint)
	require.Empty(t, items[0].Bindings[0].KeyFingerprint)
}

func TestUpstreamConnectionServiceDeletePreservesInUseConflict(t *testing.T) {
	repo := &upstreamConnectionTestRepo{deleteErr: ErrUpstreamConnectionInUse}
	service := NewUpstreamConnectionService(repo, upstreamConnectionTestEncryptor{}, nil)

	err := service.Delete(context.Background(), 5)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrUpstreamConnectionInUse))
}

func TestUpstreamConnectionServiceBindAccountAppliesObservedBillingMultiplier(t *testing.T) {
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
		Groups: []UpstreamGroup{{Name: "vip", RateMultiplier: &multiplier, Confidence: upstreamGroupRateConfidenceDefault}},
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
	require.Equal(t, UpstreamBindingApplyAuto, binding.ApplyPolicy)
	require.Equal(t, UpstreamBindingResolutionFixed, binding.ResolutionKind)
	require.Equal(t, 0.25, *binding.ObservedMultiplier)
	require.Equal(t, upstreamGroupRateConfidenceDefault, binding.ResolutionDetails[upstreamBindingRateConfidenceDetailKey])
	require.Empty(t, binding.KeyFingerprint)
	require.NotEmpty(t, repo.binding.KeyFingerprint)
	require.Equal(t, 0.25, *repo.appliedRateMultiplier)
}

func TestUpstreamConnectionServiceBindAccountSkipsUnavailableGroupRate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
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
	multiplier := 1.0
	repo.connection = &UpstreamConnection{
		ID: 5, Name: "NewAPI", Provider: UpstreamConnectionProviderNewAPI,
		AuthMode: string(UpstreamManagementAuthModeAccessToken), ManagementBaseURL: server.URL,
		CredentialEncrypted: ciphertext, CredentialFingerprint: fingerprint, CredentialHint: hint,
		RemoteUserID: "9", SyncEnabled: true, SyncIntervalSeconds: 300, Version: 1,
		Groups: []UpstreamGroup{{Name: "vip", RateMultiplier: &multiplier, Confidence: upstreamGroupRateConfidenceUnavailable}},
	}
	existingBillingMultiplier := 0.5
	accountRepo := &upstreamBindingAccountRepo{account: &Account{
		ID: 19, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-forwarding-key"},
		RateMultiplier: &existingBillingMultiplier,
	}}
	connectionService.accountRepo = accountRepo
	connectionService.inspector = newUpstreamConnectionInspector(nil, nil, server.Client())

	binding, err := connectionService.BindAccount(context.Background(), 5, 19)
	require.NoError(t, err)
	require.Equal(t, 1.0, *binding.ObservedMultiplier)
	require.Equal(t, upstreamGroupRateConfidenceUnavailable, binding.ResolutionDetails[upstreamBindingRateConfidenceDetailKey])
	require.Nil(t, repo.appliedRateMultiplier)
	require.Equal(t, 0.5, *accountRepo.account.RateMultiplier)
}

func TestUpstreamConnectionServiceBindAccountSkipsRatesArrayPayload(t *testing.T) {
	// /groups/rates returned success with data:[]; available default must not write billing.
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/auth/me":
			writeProbeJSON(t, writer, map[string]any{"data": map[string]any{"id": 11, "balance": 10}})
		case "/api/v1/groups/available":
			writeProbeJSON(t, writer, map[string]any{"data": []any{
				map[string]any{"id": 2, "name": "default", "rate_multiplier": 1.0},
			}})
		case "/api/v1/groups/rates":
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": []any{}})
		case "/api/v1/keys":
			writeProbeJSON(t, writer, map[string]any{"data": map[string]any{"items": []any{
				map[string]any{"id": 17, "name": "codex", "key": "sub2-secret-key", "group_id": 2},
			}}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	repo := &upstreamConnectionTestRepo{}
	connectionService := NewUpstreamConnectionService(repo, upstreamConnectionTestEncryptor{}, nil)
	ciphertext, fingerprint, hint, err := connectionService.encryptCredential(
		string(UpstreamManagementAuthModeAccessToken), server.URL,
		UpstreamConnectionCredentialInput{AccessToken: "management-token"},
	)
	require.NoError(t, err)
	repo.connection = &UpstreamConnection{
		ID: 8, Name: "Sub2API", Provider: UpstreamConnectionProviderSub2API,
		AuthMode: string(UpstreamManagementAuthModeAccessToken), ManagementBaseURL: server.URL,
		CredentialEncrypted: ciphertext, CredentialFingerprint: fingerprint, CredentialHint: hint,
		SyncEnabled: true, SyncIntervalSeconds: 300, Version: 1,
	}
	existingBillingMultiplier := 0.4
	accountRepo := &upstreamBindingAccountRepo{account: &Account{
		ID: 33, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sub2-secret-key"},
		RateMultiplier: &existingBillingMultiplier,
	}}
	connectionService.accountRepo = accountRepo
	connectionService.inspector = newUpstreamConnectionInspector(nil, nil, server.Client())

	// Probe first so connection groups reflect the invalid rates payload classification.
	probed, err := connectionService.inspector.Inspect(context.Background(), repo.connection, upstreamConnectionCredential{
		Version: 1, AccessToken: "management-token",
	})
	require.NoError(t, err)
	require.Len(t, probed.Groups, 1)
	require.Equal(t, upstreamGroupRateConfidenceUnavailable, probed.Groups[0].Confidence)
	repo.connection.Groups = probed.Groups

	binding, err := connectionService.BindAccount(context.Background(), 8, 33)
	require.NoError(t, err)
	require.Equal(t, 1.0, *binding.ObservedMultiplier)
	require.Equal(t, upstreamGroupRateConfidenceUnavailable, binding.ResolutionDetails[upstreamBindingRateConfidenceDetailKey])
	require.Nil(t, repo.appliedRateMultiplier)
	require.Equal(t, 0.4, *accountRepo.account.RateMultiplier)
}

func TestUpstreamConnectionServiceBindAccountSkipsInvalidKnownGroupRateValue(t *testing.T) {
	// rates map includes the bound group with a null/illegal multiplier.
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/auth/me":
			writeProbeJSON(t, writer, map[string]any{"data": map[string]any{"id": 11, "balance": 10}})
		case "/api/v1/groups/available":
			writeProbeJSON(t, writer, map[string]any{"data": []any{
				map[string]any{"id": 2, "name": "default", "rate_multiplier": 1.0},
			}})
		case "/api/v1/groups/rates":
			writeProbeJSON(t, writer, map[string]any{"data": map[string]any{"2": "not-a-rate"}})
		case "/api/v1/keys":
			writeProbeJSON(t, writer, map[string]any{"data": map[string]any{"items": []any{
				map[string]any{"id": 17, "name": "codex", "key": "sub2-secret-key", "group_id": 2},
			}}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	repo := &upstreamConnectionTestRepo{}
	connectionService := NewUpstreamConnectionService(repo, upstreamConnectionTestEncryptor{}, nil)
	ciphertext, fingerprint, hint, err := connectionService.encryptCredential(
		string(UpstreamManagementAuthModeAccessToken), server.URL,
		UpstreamConnectionCredentialInput{AccessToken: "management-token"},
	)
	require.NoError(t, err)
	repo.connection = &UpstreamConnection{
		ID: 9, Name: "Sub2API", Provider: UpstreamConnectionProviderSub2API,
		AuthMode: string(UpstreamManagementAuthModeAccessToken), ManagementBaseURL: server.URL,
		CredentialEncrypted: ciphertext, CredentialFingerprint: fingerprint, CredentialHint: hint,
		SyncEnabled: true, SyncIntervalSeconds: 300, Version: 1,
	}
	existingBillingMultiplier := 0.4
	accountRepo := &upstreamBindingAccountRepo{account: &Account{
		ID: 34, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sub2-secret-key"},
		RateMultiplier: &existingBillingMultiplier,
	}}
	connectionService.accountRepo = accountRepo
	connectionService.inspector = newUpstreamConnectionInspector(nil, nil, server.Client())

	probed, err := connectionService.inspector.Inspect(context.Background(), repo.connection, upstreamConnectionCredential{
		Version: 1, AccessToken: "management-token",
	})
	require.NoError(t, err)
	require.Len(t, probed.Groups, 1)
	require.Equal(t, upstreamGroupRateConfidenceUnavailable, probed.Groups[0].Confidence)
	require.Equal(t, 1.0, *probed.Groups[0].RateMultiplier)
	require.Equal(t, "sub2api:available_groups", probed.Groups[0].Source)
	repo.connection.Groups = probed.Groups

	binding, err := connectionService.BindAccount(context.Background(), 9, 34)
	require.NoError(t, err)
	require.Equal(t, 1.0, *binding.ObservedMultiplier)
	require.Equal(t, upstreamGroupRateConfidenceUnavailable, binding.ResolutionDetails[upstreamBindingRateConfidenceDetailKey])
	require.Nil(t, repo.appliedRateMultiplier)
	require.Equal(t, 0.4, *accountRepo.account.RateMultiplier)
}

func TestUpstreamConnectionServiceBindAccountAppliesPartialSub2APIDefaultRate(t *testing.T) {
	// available has the bound key's group at 1.0x, and /groups/rates only covers a
	// different group. The leftover available-group rate is "default" and may sync.
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/keys":
			writeProbeJSON(t, writer, map[string]any{"data": map[string]any{"items": []any{
				map[string]any{"id": 17, "name": "codex", "key": "sub2-secret-key", "group_id": 2},
			}}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	availableRate := 1.0
	overrideOtherRate := 0.25
	repo := &upstreamConnectionTestRepo{}
	connectionService := NewUpstreamConnectionService(repo, upstreamConnectionTestEncryptor{}, nil)
	ciphertext, fingerprint, hint, err := connectionService.encryptCredential(
		string(UpstreamManagementAuthModeAccessToken), server.URL,
		UpstreamConnectionCredentialInput{AccessToken: "management-token"},
	)
	require.NoError(t, err)
	repo.connection = &UpstreamConnection{
		ID: 8, Name: "Sub2API", Provider: UpstreamConnectionProviderSub2API,
		AuthMode: string(UpstreamManagementAuthModeAccessToken), ManagementBaseURL: server.URL,
		CredentialEncrypted: ciphertext, CredentialFingerprint: fingerprint, CredentialHint: hint,
		SyncEnabled: true, SyncIntervalSeconds: 300, Version: 1,
		Groups: []UpstreamGroup{
			{
				RemoteID: "1", Name: "covered", RateMultiplier: &overrideOtherRate,
				Source: "sub2api:group_rates", Confidence: upstreamGroupRateConfidenceOverride,
			},
			{
				RemoteID: "2", Name: "missing", RateMultiplier: &availableRate,
				Source: "sub2api:available_groups", Confidence: upstreamGroupRateConfidenceDefault,
			},
		},
	}
	existingBillingMultiplier := 0.4
	accountRepo := &upstreamBindingAccountRepo{account: &Account{
		ID: 33, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sub2-secret-key"},
		RateMultiplier: &existingBillingMultiplier,
	}}
	connectionService.accountRepo = accountRepo
	connectionService.inspector = newUpstreamConnectionInspector(nil, nil, server.Client())

	binding, err := connectionService.BindAccount(context.Background(), 8, 33)
	require.NoError(t, err)
	require.Equal(t, "2", binding.RemoteGroupID)
	require.Equal(t, 1.0, *binding.ObservedMultiplier)
	require.Equal(t, upstreamGroupRateConfidenceDefault, binding.ResolutionDetails[upstreamBindingRateConfidenceDetailKey])
	require.Equal(t, 1.0, *repo.appliedRateMultiplier)
	require.Equal(t, 0.4, *accountRepo.account.RateMultiplier)
}

func TestUpstreamConnectionServiceBindAccountAppliesOverrideRate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/keys":
			writeProbeJSON(t, writer, map[string]any{"data": map[string]any{"items": []any{
				map[string]any{"id": 17, "name": "codex", "key": "sub2-secret-key", "group_id": 2},
			}}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	overrideRate := 0.18
	repo := &upstreamConnectionTestRepo{}
	connectionService := NewUpstreamConnectionService(repo, upstreamConnectionTestEncryptor{}, nil)
	ciphertext, fingerprint, hint, err := connectionService.encryptCredential(
		string(UpstreamManagementAuthModeAccessToken), server.URL,
		UpstreamConnectionCredentialInput{AccessToken: "management-token"},
	)
	require.NoError(t, err)
	repo.connection = &UpstreamConnection{
		ID: 9, Name: "Sub2API", Provider: UpstreamConnectionProviderSub2API,
		AuthMode: string(UpstreamManagementAuthModeAccessToken), ManagementBaseURL: server.URL,
		CredentialEncrypted: ciphertext, CredentialFingerprint: fingerprint, CredentialHint: hint,
		SyncEnabled: true, SyncIntervalSeconds: 300, Version: 1,
		Groups: []UpstreamGroup{{
			RemoteID: "2", Name: "grok", RateMultiplier: &overrideRate,
			Source: "sub2api:group_rates", Confidence: upstreamGroupRateConfidenceOverride,
		}},
	}
	existingBillingMultiplier := 1.0
	accountRepo := &upstreamBindingAccountRepo{account: &Account{
		ID: 34, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sub2-secret-key"},
		RateMultiplier: &existingBillingMultiplier,
	}}
	connectionService.accountRepo = accountRepo
	connectionService.inspector = newUpstreamConnectionInspector(nil, nil, server.Client())

	binding, err := connectionService.BindAccount(context.Background(), 9, 34)
	require.NoError(t, err)
	require.Equal(t, 0.18, *binding.ObservedMultiplier)
	require.Equal(t, upstreamGroupRateConfidenceOverride, binding.ResolutionDetails[upstreamBindingRateConfidenceDetailKey])
	require.Equal(t, 0.18, *repo.appliedRateMultiplier)
}

func TestUpstreamConnectionServiceBindAccountSkipsNewAPIPricingRate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
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
	pricingRate := 0.35
	repo.connection = &UpstreamConnection{
		ID: 5, Name: "NewAPI", Provider: UpstreamConnectionProviderNewAPI,
		AuthMode: string(UpstreamManagementAuthModeAccessToken), ManagementBaseURL: server.URL,
		CredentialEncrypted: ciphertext, CredentialFingerprint: fingerprint, CredentialHint: hint,
		RemoteUserID: "9", SyncEnabled: true, SyncIntervalSeconds: 300, Version: 1,
		Groups: []UpstreamGroup{{
			Name: "vip", RateMultiplier: &pricingRate, Source: "newapi:pricing",
			Confidence: upstreamGroupRateConfidenceUnavailable,
		}},
	}
	existingBillingMultiplier := 0.5
	accountRepo := &upstreamBindingAccountRepo{account: &Account{
		ID: 19, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-forwarding-key"},
		RateMultiplier: &existingBillingMultiplier,
	}}
	connectionService.accountRepo = accountRepo
	connectionService.inspector = newUpstreamConnectionInspector(nil, nil, server.Client())

	binding, err := connectionService.BindAccount(context.Background(), 5, 19)
	require.NoError(t, err)
	require.Equal(t, 0.35, *binding.ObservedMultiplier)
	require.Equal(t, upstreamGroupRateConfidenceUnavailable, binding.ResolutionDetails[upstreamBindingRateConfidenceDetailKey])
	require.Nil(t, repo.appliedRateMultiplier)
	require.Equal(t, 0.5, *accountRepo.account.RateMultiplier)
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
		Groups: []UpstreamGroup{{Name: "vip", RateMultiplier: &multiplier, Confidence: upstreamGroupRateConfidenceDefault}},
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
	require.Nil(t, repo.appliedRateMultiplier)
	require.Equal(t, 0, accountRepo.updateCalls)
	require.Equal(t, 1.7, *accountRepo.account.RateMultiplier)
}

func TestUpstreamConnectionServiceRefreshBindingAppliesObservedBillingMultiplier(t *testing.T) {
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
		Groups: []UpstreamGroup{{Name: "vip", RateMultiplier: &multiplier, Confidence: upstreamGroupRateConfidenceDefault}},
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
	require.Equal(t, UpstreamBindingApplyAuto, repo.binding.ApplyPolicy)
	require.Equal(t, 0, repo.binding.SyncFailures)
	require.Equal(t, upstreamGroupRateConfidenceDefault, repo.binding.ResolutionDetails[upstreamBindingRateConfidenceDetailKey])
	require.Equal(t, 0.4, *repo.appliedRateMultiplier)
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
	require.Nil(t, repo.appliedRateMultiplier)
}

func TestObservedAccountRateMultiplierRequiresReliableReadyBinding(t *testing.T) {
	valid := 0.25
	negative := -0.1
	nan := math.NaN()
	overrideDetails := map[string]any{upstreamBindingRateConfidenceDetailKey: upstreamGroupRateConfidenceOverride}
	defaultDetails := map[string]any{upstreamBindingRateConfidenceDetailKey: upstreamGroupRateConfidenceDefault}
	legacyReported := map[string]any{upstreamBindingRateConfidenceDetailKey: "reported"}
	legacyFallback := map[string]any{upstreamBindingRateConfidenceDetailKey: "fallback"}
	unavailableDetails := map[string]any{upstreamBindingRateConfidenceDetailKey: upstreamGroupRateConfidenceUnavailable}

	require.Equal(t, 0.25, *observedAccountRateMultiplier(&UpstreamAccountBinding{
		Status: UpstreamBindingStatusReady, Confidence: "exact", ObservedMultiplier: &valid,
		ResolutionDetails: overrideDetails,
	}))
	require.Equal(t, 0.25, *observedAccountRateMultiplier(&UpstreamAccountBinding{
		Status: UpstreamBindingStatusReady, Confidence: "exact", ObservedMultiplier: &valid,
		ResolutionDetails: defaultDetails,
	}))
	require.Equal(t, 0.25, *observedAccountRateMultiplier(&UpstreamAccountBinding{
		Status: UpstreamBindingStatusReady, Confidence: "exact", ObservedMultiplier: &valid,
		ResolutionDetails: legacyReported,
	}))
	require.Equal(t, upstreamGroupRateConfidenceOverride, bindingRateConfidence(&UpstreamAccountBinding{
		ResolutionDetails: legacyReported,
	}))
	require.Nil(t, observedAccountRateMultiplier(&UpstreamAccountBinding{
		Status: UpstreamBindingStatusReady, Confidence: "exact", ObservedMultiplier: &valid,
		ResolutionDetails: legacyFallback,
	}))
	require.Nil(t, observedAccountRateMultiplier(&UpstreamAccountBinding{
		Status: UpstreamBindingStatusPending, Confidence: "exact", ObservedMultiplier: &valid,
		ResolutionDetails: overrideDetails,
	}))
	require.Nil(t, observedAccountRateMultiplier(&UpstreamAccountBinding{
		Status: UpstreamBindingStatusReady, Confidence: "unknown", ObservedMultiplier: &valid,
		ResolutionDetails: overrideDetails,
	}))
	require.Nil(t, observedAccountRateMultiplier(&UpstreamAccountBinding{
		Status: UpstreamBindingStatusReady, Confidence: "exact", ObservedMultiplier: &valid,
		ResolutionDetails: unavailableDetails,
	}))
	require.Nil(t, observedAccountRateMultiplier(&UpstreamAccountBinding{
		Status: UpstreamBindingStatusReady, Confidence: "exact", ObservedMultiplier: &valid,
	}))
	require.Nil(t, observedAccountRateMultiplier(&UpstreamAccountBinding{
		Status: UpstreamBindingStatusReady, Confidence: "exact", ObservedMultiplier: &negative,
		ResolutionDetails: overrideDetails,
	}))
	require.Nil(t, observedAccountRateMultiplier(&UpstreamAccountBinding{
		Status: UpstreamBindingStatusReady, Confidence: "exact", ObservedMultiplier: &nan,
		ResolutionDetails: overrideDetails,
	}))
}

func TestUpstreamConnectionCredentialRefreshesAutoDetectedSub2API(t *testing.T) {
	var refreshCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, "/api/v1/auth/refresh", request.URL.Path)
		refreshCalls.Add(1)
		writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{
			"access_token": "next-access", "refresh_token": "next-refresh", "expires_in": 3600,
		}})
	}))
	defer server.Close()

	repo := &upstreamConnectionTestRepo{}
	connectionService := NewUpstreamConnectionService(repo, upstreamConnectionTestEncryptor{}, nil)
	connectionService.inspector = newUpstreamConnectionInspector(nil, nil, server.Client())
	ciphertext, fingerprint, hint, err := connectionService.encryptCredential(
		string(UpstreamManagementAuthModeAccessToken), server.URL,
		UpstreamConnectionCredentialInput{
			AccessToken: "old-access", RefreshToken: "old-refresh", ExpiresAt: time.Now().Add(-time.Minute).Unix(),
		},
	)
	require.NoError(t, err)
	repo.connection = &UpstreamConnection{
		ID: 13, Provider: UpstreamConnectionProviderAuto,
		AuthMode: string(UpstreamManagementAuthModeAccessToken), ManagementBaseURL: server.URL,
		CredentialEncrypted: ciphertext, CredentialFingerprint: fingerprint, CredentialHint: hint, Version: 4,
		Capabilities: map[string]any{"detected_provider": UpstreamConnectionProviderSub2API},
	}
	credential, err := connectionService.loadCredential(repo.connection)
	require.NoError(t, err)

	updated, refreshed, err := connectionService.prepareConnectionCredential(context.Background(), repo.connection, credential)

	require.NoError(t, err)
	require.Equal(t, int32(1), refreshCalls.Load())
	require.Equal(t, "next-access", refreshed.AccessToken)
	require.Equal(t, "next-refresh", refreshed.RefreshToken)
	require.Greater(t, refreshed.ExpiresAt, time.Now().Unix())
	require.Equal(t, int64(6), updated.Version)
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

func TestSub2APIManagementLoginBodyOnlySendsExplicitLocationConfirmation(t *testing.T) {
	withoutConfirmation, err := json.Marshal(sub2APIManagementLoginBody(upstreamConnectionCredential{
		Username: "admin@example.com", Password: "secret",
	}))
	require.NoError(t, err)
	require.NotContains(t, string(withoutConfirmation), "not_in_cn_confirmed")

	withConfirmation, err := json.Marshal(sub2APIManagementLoginBody(upstreamConnectionCredential{
		Username: "admin@example.com", Password: "secret", NotInCNConfirmed: true,
	}))
	require.NoError(t, err)
	require.JSONEq(t, `{"email":"admin@example.com","password":"secret","not_in_cn_confirmed":true}`, string(withConfirmation))
}

func TestUpstreamManagementClientClassifiesLocationConfirmationRequirement(t *testing.T) {
	for _, statusCode := range []int{http.StatusBadRequest, http.StatusOK} {
		t.Run(http.StatusText(statusCode), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				writer.WriteHeader(statusCode)
				writeProbeJSON(t, writer, map[string]any{
					"success": false,
					"code":    400, "message": "must confirm you are not located in mainland China",
					"reason": "NOT_IN_CN_CONFIRMATION_REQUIRED",
				})
			}))
			defer server.Close()

			client := &upstreamManagementClient{client: server.Client()}
			_, err := client.managementJSON(context.Background(), server.Client(), http.MethodPost, server.URL, http.Header{}, map[string]string{"email": "admin@example.com"})

			require.ErrorIs(t, err, ErrUpstreamManagementLocationConfirmationRequired)
		})
	}
}

func TestUpstreamConnectionLocationConfirmationFailureNeedsInput(t *testing.T) {
	repo := &upstreamConnectionTestRepo{connection: &UpstreamConnection{ID: 91, SyncEnabled: true, Version: 4}}
	svc := NewUpstreamConnectionService(repo, upstreamConnectionTestEncryptor{}, nil)

	svc.recordProbeFailure(context.Background(), repo.connection, 4, ErrUpstreamManagementLocationConfirmationRequired)

	require.NotNil(t, repo.probeFailure)
	require.Equal(t, UpstreamConnectionStatusNeedsInput, repo.probeFailure.Status)
}

func TestUpstreamConnectionUpdateCanChangeLocationConfirmationWithoutPassword(t *testing.T) {
	repo := &upstreamConnectionTestRepo{}
	svc := NewUpstreamConnectionService(repo, upstreamConnectionTestEncryptor{}, nil)
	ciphertext, fingerprint, hint, err := svc.encryptCredential(
		string(UpstreamManagementAuthModePassword), "https://console.example.com",
		UpstreamConnectionCredentialInput{Username: "admin@example.com", Password: "secret"},
	)
	require.NoError(t, err)
	repo.connection = &UpstreamConnection{
		ID: 71, Name: "console", Provider: UpstreamConnectionProviderSub2API,
		AuthMode: string(UpstreamManagementAuthModePassword), ManagementBaseURL: "https://console.example.com",
		CredentialEncrypted: ciphertext, CredentialFingerprint: fingerprint, CredentialHint: hint,
		SyncEnabled: true, SyncIntervalSeconds: 300, Status: UpstreamConnectionStatusReady, Version: 3,
	}
	confirmed := true

	updated, err := svc.Update(context.Background(), 71, UpstreamConnectionUpdateParams{
		ExpectedVersion: 3, NotInCNConfirmed: &confirmed,
	})

	require.NoError(t, err)
	require.True(t, updated.NotInCNConfirmed)
	require.Empty(t, updated.CredentialEncrypted)
	stored, err := svc.loadCredential(repo.connection)
	require.NoError(t, err)
	require.Equal(t, "admin@example.com", stored.Username)
	require.Equal(t, "secret", stored.Password)
	require.True(t, stored.NotInCNConfirmed)
	require.True(t, repo.lastResetBindings)
}

func TestUpstreamConnectionUpdateDoesNotResetBindingsWhenLocationConfirmationIsUnchanged(t *testing.T) {
	repo := &upstreamConnectionTestRepo{}
	svc := NewUpstreamConnectionService(repo, upstreamConnectionTestEncryptor{}, nil)
	ciphertext, fingerprint, hint, err := svc.encryptCredential(
		string(UpstreamManagementAuthModePassword), "https://console.example.com",
		UpstreamConnectionCredentialInput{
			Username: "admin@example.com", Password: "secret", NotInCNConfirmed: true,
		},
	)
	require.NoError(t, err)
	repo.connection = &UpstreamConnection{
		ID: 72, Name: "console", Provider: UpstreamConnectionProviderSub2API,
		AuthMode: string(UpstreamManagementAuthModePassword), ManagementBaseURL: "https://console.example.com",
		CredentialEncrypted: ciphertext, CredentialFingerprint: fingerprint, CredentialHint: hint,
		SyncEnabled: true, SyncIntervalSeconds: 300, Status: UpstreamConnectionStatusReady, Version: 3,
		Groups: []UpstreamGroup{{ID: 1, Name: "vip"}}, GroupCount: 1,
	}
	confirmed := true

	updated, err := svc.Update(context.Background(), 72, UpstreamConnectionUpdateParams{
		ExpectedVersion: 3, NotInCNConfirmed: &confirmed,
	})

	require.NoError(t, err)
	require.False(t, repo.lastResetBindings)
	require.Len(t, updated.Groups, 1)
	require.Equal(t, ciphertext, repo.connection.CredentialEncrypted)
}

func TestUpstreamConnectionTodayUsageAggregatesBoundAccountsAndFillsHourlyGaps(t *testing.T) {
	location := time.Local
	now := time.Date(2026, time.July, 19, 10, 30, 0, 0, location)
	start := time.Date(2026, time.July, 19, 0, 0, 0, 0, location)
	repo := &upstreamConnectionTestRepo{connection: &UpstreamConnection{
		ID: 81,
		Bindings: []UpstreamAccountBinding{
			{ID: 1, AccountID: 11, RemoteTokenID: "101", RemoteTokenName: "primary", RemoteGroupName: "vip", Status: UpstreamBindingStatusReady},
			{ID: 2, AccountID: 12, RemoteTokenID: "102", RemoteTokenName: "backup", RemoteGroupName: "default", Status: UpstreamBindingStatusReady},
		},
	}}
	reader := &upstreamConnectionUsageReaderStub{buckets: []UpstreamConnectionAccountUsageBucket{
		{AccountID: 11, Bucket: start.Add(9 * time.Hour), UpstreamConnectionUsageStats: UpstreamConnectionUsageStats{Requests: 2, Tokens: 100, AccountCost: 1.25, StandardCost: 1, UserCost: 1.5}},
		{AccountID: 11, Bucket: start.Add(10 * time.Hour), UpstreamConnectionUsageStats: UpstreamConnectionUsageStats{Requests: 3, Tokens: 200, AccountCost: 2.5, StandardCost: 2, UserCost: 3}},
	}}
	svc := NewUpstreamConnectionService(repo, upstreamConnectionTestEncryptor{}, nil)
	svc.accountRepo = &upstreamConnectionUsageAccountRepo{accounts: []*Account{
		{ID: 11, Name: "Primary account"}, {ID: 12, Name: "Backup account"},
	}}
	svc.usageReader = reader
	svc.now = func() time.Time { return now }

	usage, err := svc.GetTodayUsage(context.Background(), 81)

	require.NoError(t, err)
	require.Equal(t, []int64{11, 12}, reader.accountIDs)
	require.Equal(t, int64(5), usage.Summary.Requests)
	require.Equal(t, int64(300), usage.Summary.Tokens)
	require.InDelta(t, 3.75, usage.Summary.AccountCost, 0.000001)
	require.Len(t, usage.Trend, 11)
	require.Len(t, usage.Accounts, 2)
	require.Equal(t, "Primary account", usage.Accounts[0].AccountName)
	require.Equal(t, int64(5), usage.Accounts[0].Stats.Requests)
	require.Zero(t, usage.Accounts[0].Trend[8].Requests)
	require.Equal(t, int64(2), usage.Accounts[0].Trend[9].Requests)
	require.Equal(t, "Backup account", usage.Accounts[1].AccountName)
	require.Zero(t, usage.Accounts[1].Stats.Requests)
}
