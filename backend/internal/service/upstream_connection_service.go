package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
	"github.com/google/uuid"
)

var (
	ErrUpstreamConnectionNotFound             = infraerrors.NotFound("UPSTREAM_CONNECTION_NOT_FOUND", "upstream connection not found")
	ErrUpstreamConnectionInUse                = infraerrors.Conflict("UPSTREAM_CONNECTION_IN_USE", "upstream connection is still bound to accounts")
	ErrUpstreamConnectionBindingsChanged      = infraerrors.Conflict("UPSTREAM_CONNECTION_BINDINGS_CHANGED", "upstream connection bindings changed; reload and confirm again")
	ErrUpstreamConnectionConfirmationRequired = infraerrors.Conflict("UPSTREAM_CONNECTION_BINDING_CONFIRMATION_REQUIRED", "bound accounts must be reloaded and confirmed before deletion")
	ErrUpstreamConnectionInvalidReference     = infraerrors.BadRequest("INVALID_UPSTREAM_CONNECTION_REFERENCE", "referenced proxy or account does not exist")
	ErrUpstreamConnectionChanged              = infraerrors.Conflict("UPSTREAM_CONNECTION_CHANGED", "upstream connection changed; reload and retry")
	ErrUpstreamCredentialRefreshBusy          = infraerrors.Conflict("UPSTREAM_CREDENTIAL_REFRESH_BUSY", "another upstream credential refresh is already in progress")
	ErrUpstreamAccountBindingNotFound         = infraerrors.NotFound("UPSTREAM_ACCOUNT_BINDING_NOT_FOUND", "upstream account binding not found")
	ErrUpstreamConnectionAuthentication       = errors.New("upstream management authentication failed")
)

type UpstreamConnectionRepository interface {
	Create(ctx context.Context, connection *UpstreamConnection) error
	GetByID(ctx context.Context, id int64) (*UpstreamConnection, error)
	List(ctx context.Context, params UpstreamConnectionListParams) ([]*UpstreamConnection, int64, error)
	UpdateIfVersion(ctx context.Context, connection *UpstreamConnection, expectedVersion int64, resetBindings, updateCredential, updateRuntimeState, updateRemoteUserID bool) (bool, error)
	Delete(ctx context.Context, id int64, params UpstreamConnectionDeleteParams) error
	FinalizeCredentialRefresh(ctx context.Context, id int64, expectedCiphertext, expectedProvider, expectedAuthMode, expectedManagementBaseURL string, update UpstreamConnectionCredentialPersistence) (bool, error)
	ApplyProbeSuccess(ctx context.Context, id, expectedVersion int64, update UpstreamConnectionProbePersistence) (bool, error)
	RecordProbeFailure(ctx context.Context, id, expectedVersion int64, failure UpstreamConnectionProbeFailure) (bool, error)
	ListDueConnections(ctx context.Context, now time.Time, limit int) ([]*UpstreamConnection, error)
	ListDueAccountBindings(ctx context.Context, connectionID int64, now time.Time, limit int) ([]UpstreamAccountBinding, error)
	UpsertAccountBindingIfCurrent(ctx context.Context, binding *UpstreamAccountBinding, expectedConnectionVersion int64, rateMultiplier *float64) (bool, error)
	UpdateAccountBindingIfCurrent(ctx context.Context, binding *UpstreamAccountBinding, expectedConnectionID, expectedConnectionVersion int64, rateMultiplier *float64) (bool, error)
	GetAccountBinding(ctx context.Context, accountID int64) (*UpstreamAccountBinding, error)
	DeleteAccountBinding(ctx context.Context, connectionID, accountID int64) error
}

type upstreamConnectionUsageReader interface {
	GetUpstreamAccountUsageBuckets(ctx context.Context, accountIDs []int64, startTime, endTime time.Time, timezoneName string) ([]UpstreamConnectionAccountUsageBucket, error)
}

// upstreamConnectionRuntimeReader is intentionally separate from the detail-page
// reader. The list page needs a compact, group-level snapshot rather than an
// hourly series for each bound account.
type upstreamConnectionRuntimeReader interface {
	GetUpstreamConnectionRuntimeGroups(ctx context.Context, accountIDs []int64, startTime, endTime, fiveMinuteStart time.Time) ([]UpstreamConnectionRuntimeGroupMetric, error)
}

type UpstreamConnectionService struct {
	repo               UpstreamConnectionRepository
	encryptor          SecretEncryptor
	cfg                *config.Config
	inspector          *upstreamConnectionInspector
	accountRepo        AccountRepository
	usageReader        upstreamConnectionUsageReader
	runtimeReader      upstreamConnectionRuntimeReader
	concurrencyService *ConcurrencyService
	lockCache          LeaderLockCache
	db                 *sql.DB
	instanceID         string
	now                func() time.Time
	refreshMu          sync.Mutex
}

func NewUpstreamConnectionService(repo UpstreamConnectionRepository, encryptor SecretEncryptor, cfg *config.Config) *UpstreamConnectionService {
	return &UpstreamConnectionService{
		repo: repo, encryptor: encryptor, cfg: cfg,
		inspector:  newUpstreamConnectionInspector(cfg, nil, nil),
		instanceID: uuid.NewString(),
		now:        time.Now,
	}
}

func ProvideUpstreamConnectionService(repo UpstreamConnectionRepository, encryptor SecretEncryptor, cfg *config.Config, proxyRepo ProxyRepository, accountRepo AccountRepository, usageLogRepo UsageLogRepository, concurrencyService *ConcurrencyService, lockCache LeaderLockCache, db *sql.DB) *UpstreamConnectionService {
	service := NewUpstreamConnectionService(repo, encryptor, cfg)
	service.inspector = newUpstreamConnectionInspector(cfg, proxyRepo, nil)
	service.accountRepo = accountRepo
	if reader, ok := usageLogRepo.(upstreamConnectionUsageReader); ok {
		service.usageReader = reader
	}
	if reader, ok := usageLogRepo.(upstreamConnectionRuntimeReader); ok {
		service.runtimeReader = reader
	}
	service.concurrencyService = concurrencyService
	service.lockCache = lockCache
	service.db = db
	return service
}

func (s *UpstreamConnectionService) List(ctx context.Context, params UpstreamConnectionListParams) ([]*UpstreamConnection, int64, error) {
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 1 || params.PageSize > 200 {
		params.PageSize = 20
	}
	params.Provider = strings.ToLower(strings.TrimSpace(params.Provider))
	params.Status = strings.ToLower(strings.TrimSpace(params.Status))
	params.Search = strings.TrimSpace(params.Search)
	items, total, err := s.repo.List(ctx, params)
	if err != nil {
		return nil, 0, fmt.Errorf("list upstream connections: %w", err)
	}
	for _, item := range items {
		s.populateCredentialMetadata(item)
	}
	redactUpstreamConnections(items)
	return items, total, nil
}

func (s *UpstreamConnectionService) Get(ctx context.Context, id int64) (*UpstreamConnection, error) {
	connection, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get upstream connection: %w", err)
	}
	s.populateCredentialMetadata(connection)
	redactUpstreamConnection(connection)
	return connection, nil
}

func (s *UpstreamConnectionService) Create(ctx context.Context, params UpstreamConnectionCreateParams) (*UpstreamConnection, error) {
	normalized, credential, err := s.normalizeCreate(params)
	if err != nil {
		return nil, err
	}
	credential.NotInCNConfirmed = normalized.NotInCNConfirmed
	ciphertext, fingerprint, hint, err := s.encryptCredential(normalized.AuthMode, normalized.ManagementBaseURL, credential)
	if err != nil {
		return nil, err
	}
	status := UpstreamConnectionStatusPending
	if !normalized.SyncEnabled {
		status = UpstreamConnectionStatusDisabled
	}
	connection := &UpstreamConnection{
		Name:                  normalized.Name,
		Provider:              normalized.Provider,
		AuthMode:              normalized.AuthMode,
		ManagementBaseURL:     normalized.ManagementBaseURL,
		ForwardingBaseURL:     normalized.ForwardingBaseURL,
		CredentialEncrypted:   ciphertext,
		CredentialFingerprint: fingerprint,
		CredentialHint:        hint,
		NotInCNConfirmed:      credential.NotInCNConfirmed,
		RemoteUserID:          normalized.RemoteUserID,
		ProxyID:               cloneInt64Pointer(normalized.ProxyID),
		Capabilities:          map[string]any{},
		Status:                status,
		SyncEnabled:           normalized.SyncEnabled,
		SyncIntervalSeconds:   normalized.SyncIntervalSeconds,
		Version:               1,
		WalletReliability:     "unknown",
		WalletRaw:             map[string]any{},
		Groups:                []UpstreamGroup{},
		Bindings:              []UpstreamAccountBinding{},
	}
	if err := s.repo.Create(ctx, connection); err != nil {
		return nil, fmt.Errorf("create upstream connection: %w", err)
	}
	redactUpstreamConnection(connection)
	return connection, nil
}

func (s *UpstreamConnectionService) Update(ctx context.Context, id int64, params UpstreamConnectionUpdateParams) (*UpstreamConnection, error) {
	connection, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get upstream connection: %w", err)
	}
	if params.ExpectedVersion <= 0 || params.ExpectedVersion != connection.Version {
		return nil, ErrUpstreamConnectionChanged
	}

	expectedVersion := params.ExpectedVersion
	previousAuthMode := connection.AuthMode
	previousSyncEnabled := connection.SyncEnabled
	previousSyncInterval := connection.SyncIntervalSeconds
	identityChanged := false
	managementURLChanged := false
	credentialChanged := false
	remoteUserIDChanged := false
	if params.Name != nil {
		connection.Name = strings.TrimSpace(*params.Name)
	}
	if params.Provider != nil {
		next := strings.ToLower(strings.TrimSpace(*params.Provider))
		identityChanged = identityChanged || next != connection.Provider
		connection.Provider = next
	}
	if params.AuthMode != nil {
		next := strings.ToLower(strings.TrimSpace(*params.AuthMode))
		if next != connection.AuthMode && params.Credential == nil {
			return nil, infraerrors.BadRequest("UPSTREAM_CONNECTION_CREDENTIAL_REQUIRED", "credentials are required when auth mode changes")
		}
		identityChanged = identityChanged || next != connection.AuthMode
		connection.AuthMode = next
	}
	if params.ManagementBaseURL != nil {
		next, normalizeErr := s.normalizeURL(*params.ManagementBaseURL, true)
		if normalizeErr != nil {
			return nil, infraerrors.BadRequest("INVALID_UPSTREAM_MANAGEMENT_URL", normalizeErr.Error())
		}
		managementURLChanged = next != connection.ManagementBaseURL
		identityChanged = identityChanged || managementURLChanged
		connection.ManagementBaseURL = next
	}
	if params.ForwardingBaseURL != nil {
		next, normalizeErr := s.normalizeOptionalURL(*params.ForwardingBaseURL)
		if normalizeErr != nil {
			return nil, infraerrors.BadRequest("INVALID_UPSTREAM_FORWARDING_URL", normalizeErr.Error())
		}
		identityChanged = identityChanged || next != connection.ForwardingBaseURL
		connection.ForwardingBaseURL = next
	}
	if params.RemoteUserID != nil {
		next, normalizeErr := normalizeRemoteUserID(*params.RemoteUserID)
		if normalizeErr != nil {
			return nil, infraerrors.BadRequest("INVALID_UPSTREAM_REMOTE_USER_ID", normalizeErr.Error())
		}
		remoteUserIDChanged = next != connection.RemoteUserID
		identityChanged = identityChanged || remoteUserIDChanged
		connection.RemoteUserID = next
	}
	if params.ClearProxy {
		identityChanged = identityChanged || connection.ProxyID != nil
		connection.ProxyID = nil
	} else if params.ProxyID != nil {
		proxyChanged := connection.ProxyID == nil || *connection.ProxyID != *params.ProxyID
		identityChanged = identityChanged || proxyChanged
		connection.ProxyID = cloneInt64Pointer(params.ProxyID)
	}
	if params.SyncEnabled != nil {
		connection.SyncEnabled = *params.SyncEnabled
	}
	if params.SyncIntervalSeconds != nil {
		connection.SyncIntervalSeconds = *params.SyncIntervalSeconds
	}
	credentialInput := params.Credential
	if params.Credential != nil {
		input := *params.Credential
		if params.NotInCNConfirmed != nil {
			input.NotInCNConfirmed = *params.NotInCNConfirmed
		} else if connection.AuthMode == previousAuthMode {
			stored, credentialErr := s.loadCredential(connection)
			if credentialErr != nil {
				return nil, credentialErr
			}
			input.NotInCNConfirmed = stored.NotInCNConfirmed
		}
		credentialInput = &input
	} else if params.NotInCNConfirmed != nil {
		stored, credentialErr := s.loadCredential(connection)
		if credentialErr != nil {
			return nil, credentialErr
		}
		if stored.NotInCNConfirmed != *params.NotInCNConfirmed {
			input := upstreamConnectionCredentialInput(stored)
			input.NotInCNConfirmed = *params.NotInCNConfirmed
			credentialInput = &input
		}
	}
	if credentialInput != nil {
		ciphertext, fingerprint, hint, encryptErr := s.encryptCredential(connection.AuthMode, connection.ManagementBaseURL, *credentialInput)
		if encryptErr != nil {
			return nil, encryptErr
		}
		connection.CredentialEncrypted = ciphertext
		connection.CredentialFingerprint = fingerprint
		connection.CredentialHint = hint
		connection.NotInCNConfirmed = credentialInput.NotInCNConfirmed
		identityChanged = true
		credentialChanged = true
	} else if managementURLChanged {
		credential, credentialErr := s.loadCredential(connection)
		if credentialErr != nil {
			return nil, credentialErr
		}
		_, fingerprint, hint, identityErr := upstreamConnectionCredentialIdentity(
			connection.AuthMode, connection.ManagementBaseURL,
			upstreamConnectionCredentialInput(credential),
		)
		if identityErr != nil {
			return nil, identityErr
		}
		connection.CredentialFingerprint = fingerprint
		connection.CredentialHint = hint
		credentialChanged = connection.AuthMode == string(UpstreamManagementAuthModePassword)
	}
	if err := validateUpstreamConnection(connection); err != nil {
		return nil, err
	}

	connection.Version = expectedVersion + 1
	if identityChanged {
		resetUpstreamConnectionObservations(connection)
	}
	if !connection.SyncEnabled {
		connection.Status = UpstreamConnectionStatusDisabled
		connection.NextSyncAt = nil
	} else if connection.Status == UpstreamConnectionStatusDisabled {
		connection.Status = UpstreamConnectionStatusPending
	}
	runtimeStateChanged := identityChanged || !previousSyncEnabled || connection.SyncIntervalSeconds != previousSyncInterval
	if connection.SyncEnabled && runtimeStateChanged {
		now := time.Now().UTC()
		connection.NextSyncAt = &now
	}
	applied, err := s.repo.UpdateIfVersion(ctx, connection, expectedVersion, identityChanged, credentialChanged, runtimeStateChanged, remoteUserIDChanged)
	if err != nil {
		return nil, fmt.Errorf("update upstream connection: %w", err)
	}
	if !applied {
		return nil, ErrUpstreamConnectionChanged
	}
	s.populateCredentialMetadata(connection)
	redactUpstreamConnection(connection)
	return connection, nil
}

// Delete removes a shared upstream connection. Bindings for already soft-deleted
// accounts are always cleaned up. Bindings for existing accounts require the
// caller to explicitly acknowledge unbinding them.
func (s *UpstreamConnectionService) Delete(ctx context.Context, id int64, params UpstreamConnectionDeleteParams) error {
	if err := s.repo.Delete(ctx, id, params); err != nil {
		return fmt.Errorf("delete upstream connection: %w", err)
	}
	return nil
}

func (s *UpstreamConnectionService) BindAccount(ctx context.Context, connectionID, accountID int64) (*UpstreamAccountBinding, error) {
	if s.accountRepo == nil {
		return nil, errors.New("account repository is unavailable")
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("get account for upstream binding: %w", err)
	}
	if !supportsUpstreamConnectionAccountType(account.Type) {
		return nil, infraerrors.BadRequest("UNSUPPORTED_UPSTREAM_BINDING_ACCOUNT", "only API-key and upstream accounts can bind to an upstream connection")
	}
	apiKey := strings.TrimSpace(upstreamConnectionAPIKey(account))
	if apiKey == "" {
		return nil, infraerrors.BadRequest("UPSTREAM_BINDING_API_KEY_REQUIRED", "account does not contain a forwarding API key")
	}
	connection, err := s.repo.GetByID(ctx, connectionID)
	if err != nil {
		return nil, fmt.Errorf("get upstream connection: %w", err)
	}
	credential, err := s.loadCredential(connection)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	nextSync := now.Add(time.Minute)
	binding := UpstreamAccountBinding{
		AccountID: accountID, ConnectionID: connectionID, KeyFingerprint: upstreamAPIKeyFingerprint(apiKey),
		ResolutionKind: UpstreamBindingResolutionUnresolved, FallbackGroups: []string{},
		Confidence: "unknown", Source: "", ApplyPolicy: UpstreamBindingApplyAuto,
		Status: UpstreamBindingStatusPending, ResolutionDetails: map[string]any{}, NextSyncAt: &nextSync,
	}
	resolved, resolveErr := s.inspector.ResolveKey(ctx, connection, credential, apiKey)
	if resolveErr != nil {
		binding.Status = UpstreamBindingStatusError
		binding.SyncFailures = 1
		binding.LastError = truncateUpstreamConnectionError(resolveErr.Error())
		next := now.Add(upstreamConnectionFailureBackoff(binding.SyncFailures))
		binding.NextSyncAt = &next
	} else {
		resolved.AccountID = accountID
		resolved.ConnectionID = connectionID
		resolved.KeyFingerprint = binding.KeyFingerprint
		resolved.ApplyPolicy = UpstreamBindingApplyAuto
		if resolved.FallbackGroups == nil {
			resolved.FallbackGroups = []string{}
		}
		if resolved.ResolutionDetails == nil {
			resolved.ResolutionDetails = map[string]any{}
		}
		if resolved.Status == "" {
			resolved.Status = UpstreamBindingStatusReady
		}
		resolved.ObservedAt = &now
		freshUntil := now.Add(2 * time.Duration(connection.SyncIntervalSeconds) * time.Second)
		resolved.FreshUntil = &freshUntil
		next := now.Add(time.Duration(connection.SyncIntervalSeconds) * time.Second)
		resolved.NextSyncAt = &next
		binding = resolved
	}
	applied, err := s.repo.UpsertAccountBindingIfCurrent(ctx, &binding, connection.Version, observedAccountRateMultiplier(&binding))
	if err != nil {
		return nil, fmt.Errorf("save upstream account binding: %w", err)
	}
	if !applied {
		return nil, ErrUpstreamConnectionChanged
	}
	binding.KeyFingerprint = ""
	return &binding, nil
}

func (s *UpstreamConnectionService) GetAccountBinding(ctx context.Context, accountID int64) (*UpstreamAccountBinding, error) {
	binding, err := s.repo.GetAccountBinding(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("get upstream account binding: %w", err)
	}
	binding.KeyFingerprint = ""
	return binding, nil
}

func (s *UpstreamConnectionService) UnbindAccount(ctx context.Context, connectionID, accountID int64) error {
	if err := s.repo.DeleteAccountBinding(ctx, connectionID, accountID); err != nil {
		return fmt.Errorf("delete upstream account binding: %w", err)
	}
	return nil
}

// SyncConnection refreshes one shared connection and then re-resolves a bounded
// batch of its due API-key bindings.
func (s *UpstreamConnectionService) SyncConnection(ctx context.Context, connectionID int64, bindingLimit int) error {
	if _, err := s.Probe(ctx, connectionID); err != nil {
		return err
	}
	connection, err := s.repo.GetByID(ctx, connectionID)
	if err != nil {
		return fmt.Errorf("reload probed upstream connection: %w", err)
	}
	if bindingLimit < 1 {
		return nil
	}
	bindings, err := s.repo.ListDueAccountBindings(ctx, connectionID, time.Now().UTC(), bindingLimit)
	if err != nil {
		return err
	}
	if len(bindings) == 0 {
		return nil
	}
	credential, err := s.loadCredential(connection)
	if err != nil {
		return err
	}
	resolver, err := s.inspector.PrepareKeyResolver(ctx, connection, credential)
	if err != nil {
		return fmt.Errorf("prepare upstream key resolver: %w", err)
	}
	var syncErrors []error
	for index := range bindings {
		if ctx.Err() != nil {
			syncErrors = append(syncErrors, ctx.Err())
			break
		}
		if err := s.refreshAccountBinding(ctx, connection, resolver, &bindings[index]); err != nil {
			syncErrors = append(syncErrors, fmt.Errorf("account %d: %w", bindings[index].AccountID, err))
		}
	}
	return errors.Join(syncErrors...)
}

func (s *UpstreamConnectionService) refreshAccountBinding(
	ctx context.Context,
	connection *UpstreamConnection,
	resolver upstreamConnectionKeyResolver,
	binding *UpstreamAccountBinding,
) error {
	if s.accountRepo == nil {
		return errors.New("account repository is unavailable")
	}
	expectedConnectionID := binding.ConnectionID
	account, err := s.accountRepo.GetByID(ctx, binding.AccountID)
	if err != nil {
		return s.persistBindingRefreshFailure(ctx, binding, expectedConnectionID, connection.Version, err)
	}
	if !supportsUpstreamConnectionAccountType(account.Type) {
		return s.persistBindingRefreshFailure(ctx, binding, expectedConnectionID, connection.Version, errors.New("bound account type no longer supports upstream key resolution"))
	}
	apiKey := strings.TrimSpace(upstreamConnectionAPIKey(account))
	if apiKey == "" {
		return s.persistBindingRefreshFailure(ctx, binding, expectedConnectionID, connection.Version, errors.New("bound account no longer contains a forwarding API key"))
	}
	resolved, err := resolver(ctx, apiKey)
	if err != nil {
		return s.persistBindingRefreshFailure(ctx, binding, expectedConnectionID, connection.Version, err)
	}
	now := time.Now().UTC()
	resolved.ID = binding.ID
	resolved.AccountID = binding.AccountID
	resolved.ConnectionID = connection.ID
	resolved.KeyFingerprint = upstreamAPIKeyFingerprint(apiKey)
	resolved.ApplyPolicy = UpstreamBindingApplyAuto
	resolved.SyncFailures = 0
	resolved.LastError = strings.TrimSpace(resolved.LastError)
	resolved.ObservedAt = &now
	if resolved.FallbackGroups == nil {
		resolved.FallbackGroups = []string{}
	}
	if resolved.ResolutionDetails == nil {
		resolved.ResolutionDetails = map[string]any{}
	}
	if resolved.Status == "" {
		resolved.Status = UpstreamBindingStatusReady
	}
	freshUntil := now.Add(2 * time.Duration(connection.SyncIntervalSeconds) * time.Second)
	resolved.FreshUntil = &freshUntil
	next := now.Add(time.Duration(connection.SyncIntervalSeconds) * time.Second)
	resolved.NextSyncAt = &next
	applied, err := s.repo.UpdateAccountBindingIfCurrent(ctx, &resolved, expectedConnectionID, connection.Version, observedAccountRateMultiplier(&resolved))
	if err != nil {
		return fmt.Errorf("persist refreshed upstream account binding: %w", err)
	}
	if !applied {
		return ErrUpstreamConnectionChanged
	}
	return nil
}

func (s *UpstreamConnectionService) persistBindingRefreshFailure(
	ctx context.Context,
	binding *UpstreamAccountBinding,
	expectedConnectionID, expectedConnectionVersion int64,
	refreshErr error,
) error {
	binding.Status = UpstreamBindingStatusError
	binding.SyncFailures++
	binding.LastError = truncateUpstreamConnectionError(refreshErr.Error())
	next := time.Now().UTC().Add(upstreamConnectionFailureBackoff(binding.SyncFailures))
	binding.NextSyncAt = &next
	applied, err := s.repo.UpdateAccountBindingIfCurrent(ctx, binding, expectedConnectionID, expectedConnectionVersion, nil)
	if err != nil {
		return fmt.Errorf("persist upstream binding refresh failure: %w", err)
	}
	if !applied {
		return ErrUpstreamConnectionChanged
	}
	return refreshErr
}

func observedAccountRateMultiplier(binding *UpstreamAccountBinding) *float64 {
	if binding == nil || binding.Status != UpstreamBindingStatusReady || binding.Confidence != "exact" ||
		binding.ObservedMultiplier == nil || strings.TrimSpace(binding.LastError) != "" {
		return nil
	}
	// Key→group mapping confidence is separate from group-rate confidence.
	// Only override (user-specific) and default (authenticated upstream default)
	// rates may rewrite account billing. unavailable/unknown are display-only.
	if !isAutoSyncableGroupRateConfidence(bindingRateConfidence(binding)) {
		return nil
	}
	value := *binding.ObservedMultiplier
	if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return nil
	}
	return cloneFloat64Ptr(&value)
}

func isAutoSyncableGroupRateConfidence(confidence string) bool {
	switch normalizeGroupRateConfidence(confidence) {
	case upstreamGroupRateConfidenceOverride, upstreamGroupRateConfidenceDefault:
		return true
	default:
		return false
	}
}

// normalizeGroupRateConfidence maps persisted/legacy labels onto the current
// override/default/unavailable/unknown model used by auto-sync and UI.
func normalizeGroupRateConfidence(confidence string) string {
	switch strings.TrimSpace(confidence) {
	case upstreamGroupRateConfidenceOverride, "reported":
		// Pre-rename "reported" meant a trusted observed rate. Map to override
		// so UI shows "upstream-specific" rather than generic default.
		return upstreamGroupRateConfidenceOverride
	case upstreamGroupRateConfidenceDefault:
		return upstreamGroupRateConfidenceDefault
	case upstreamGroupRateConfidenceUnavailable, "fallback":
		// Pre-rename "fallback" meant rates were not reliable enough to write.
		return upstreamGroupRateConfidenceUnavailable
	case upstreamGroupRateConfidenceUnknown, "":
		return upstreamGroupRateConfidenceUnknown
	default:
		return upstreamGroupRateConfidenceUnknown
	}
}

func bindingRateConfidence(binding *UpstreamAccountBinding) string {
	if binding == nil || binding.ResolutionDetails == nil {
		return upstreamGroupRateConfidenceUnknown
	}
	raw, ok := binding.ResolutionDetails[upstreamBindingRateConfidenceDetailKey]
	if !ok || raw == nil {
		return upstreamGroupRateConfidenceUnknown
	}
	switch value := raw.(type) {
	case string:
		return normalizeGroupRateConfidence(value)
	}
	return upstreamGroupRateConfidenceUnknown
}

func upstreamAPIKeyFingerprint(apiKey string) string {
	digest := sha256.Sum256([]byte("upstream-api-key\x00" + strings.TrimSpace(apiKey)))
	return "sha256:v1:" + hex.EncodeToString(digest[:])
}

func (s *UpstreamConnectionService) Probe(ctx context.Context, id int64) (*UpstreamConnection, error) {
	connection, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get upstream connection: %w", err)
	}
	credential, err := s.loadCredential(connection)
	if err != nil {
		return nil, err
	}
	connection, credential, err = s.prepareConnectionCredential(ctx, connection, credential)
	if err != nil {
		if errors.Is(err, ErrUpstreamConnectionChanged) || errors.Is(err, ErrUpstreamCredentialRefreshBusy) {
			return nil, err
		}
		s.recordProbeFailure(ctx, connection, connection.Version, err)
		return nil, upstreamCredentialRefreshProbeError(err)
	}
	if s.inspector == nil {
		return nil, errors.New("upstream connection inspector is unavailable")
	}
	expectedVersion := connection.Version
	snapshot, probeErr := s.inspector.Inspect(ctx, connection, credential)
	if probeErr != nil {
		s.recordProbeFailure(ctx, connection, expectedVersion, probeErr)
		if errors.Is(probeErr, errUpstreamConnectionRemoteUserIDRequired) {
			return nil, infraerrors.BadRequest("UPSTREAM_REMOTE_USER_ID_REQUIRED", probeErr.Error())
		}
		if errors.Is(probeErr, ErrUpstreamManagementLocationConfirmationRequired) {
			return nil, infraerrors.BadRequest("UPSTREAM_LOCATION_CONFIRMATION_REQUIRED", probeErr.Error())
		}
		return nil, infraerrors.New(502, "UPSTREAM_CONNECTION_PROBE_FAILED", "upstream connection probe failed").WithCause(probeErr)
	}

	now := time.Now().UTC()
	status := UpstreamConnectionStatusReady
	lastError := ""
	if len(snapshot.Warnings) > 0 {
		status = UpstreamConnectionStatusDegraded
		lastError = truncateUpstreamConnectionError(strings.Join(snapshot.Warnings, "; "))
	}
	if !connection.SyncEnabled {
		status = UpstreamConnectionStatusDisabled
	}
	capabilities := snapshot.Capabilities
	if capabilities == nil {
		capabilities = map[string]any{}
	}
	capabilities["detected_provider"] = snapshot.DetectedProvider
	capabilities["warnings"] = append([]string{}, snapshot.Warnings...)
	remoteUserID := connection.RemoteUserID
	if strings.TrimSpace(snapshot.RemoteUserID) != "" {
		remoteUserID = snapshot.RemoteUserID
	}
	update := UpstreamConnectionProbePersistence{
		RemoteUserID: remoteUserID, Capabilities: capabilities, Status: status, LastError: lastError,
		SyncFailures:   0,
		WalletObserved: snapshot.WalletObserved, GroupsObserved: snapshot.GroupsObserved,
		Groups: snapshot.Groups, LastDiscoveredAt: &now, LastSyncedAt: &now,
	}
	if connection.SyncEnabled {
		next := now.Add(time.Duration(connection.SyncIntervalSeconds) * time.Second)
		update.NextSyncAt = &next
	}
	if snapshot.Wallet != nil {
		update.WalletAmount = cloneFloat64Ptr(snapshot.Wallet.Amount)
		update.WalletCurrency = snapshot.Wallet.Currency
		update.WalletUSD = cloneFloat64Ptr(snapshot.Wallet.USD)
		update.WalletUnlimited = snapshot.Wallet.Unlimited
		update.WalletSource = snapshot.Wallet.Source
		update.WalletReliability = snapshot.Wallet.Reliability
		update.WalletRaw = snapshot.Wallet.Raw
		update.WalletObservedAt = &now
	}
	for index := range update.Groups {
		observedAt := now
		freshUntil := now.Add(2 * time.Duration(connection.SyncIntervalSeconds) * time.Second)
		update.Groups[index].ConnectionID = connection.ID
		update.Groups[index].ObservedAt = &observedAt
		update.Groups[index].FreshUntil = &freshUntil
	}
	applied, err := s.repo.ApplyProbeSuccess(ctx, id, expectedVersion, update)
	if err != nil {
		return nil, fmt.Errorf("persist upstream connection probe: %w", err)
	}
	if !applied {
		return nil, ErrUpstreamConnectionChanged
	}
	return s.Get(ctx, id)
}

// prepareConnectionCredential refreshes an expiring shared Sub2API management
// token. Shared connections are the sole credential owner.
func (s *UpstreamConnectionService) prepareConnectionCredential(
	ctx context.Context,
	connection *UpstreamConnection,
	credential upstreamConnectionCredential,
) (*UpstreamConnection, upstreamConnectionCredential, error) {
	if upstreamConnectionEffectiveProvider(connection) != UpstreamConnectionProviderSub2API ||
		connection.AuthMode != string(UpstreamManagementAuthModeAccessToken) ||
		!shouldRefreshSub2APIManagementToken(upstreamManagementAuthSecret{
			AccessToken: credential.AccessToken, RefreshToken: credential.RefreshToken, ExpiresAt: credential.ExpiresAt,
		}) {
		return connection, credential, nil
	}
	if s.inspector == nil {
		return connection, credential, errors.New("upstream connection inspector is unavailable")
	}

	// Token rotation must be single-flight both within this process and across
	// application instances. A second caller reloads after the first finishes,
	// observes the new expiry, and skips another refresh.
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	release, acquired := tryAcquireSingletonLeaderLock(
		ctx, s.lockCache, s.db,
		fmt.Sprintf("upstream:connections:v2:credential-refresh:%d", connection.ID),
		s.instanceID, 2*time.Minute,
	)
	if !acquired {
		return connection, credential, ErrUpstreamCredentialRefreshBusy
	}
	defer release()

	latest, err := s.repo.GetByID(ctx, connection.ID)
	if err != nil {
		return connection, credential, fmt.Errorf("reload upstream connection before credential refresh: %w", err)
	}
	credential, err = s.loadCredential(latest)
	if err != nil {
		return latest, credential, err
	}
	connection = latest
	if upstreamConnectionEffectiveProvider(connection) != UpstreamConnectionProviderSub2API ||
		connection.AuthMode != string(UpstreamManagementAuthModeAccessToken) ||
		!shouldRefreshSub2APIManagementToken(upstreamManagementAuthSecret{
			AccessToken: credential.AccessToken, RefreshToken: credential.RefreshToken, ExpiresAt: credential.ExpiresAt,
		}) {
		return connection, credential, nil
	}

	// The singleton lock prevents concurrent refreshes. Do not persist a
	// pre-refresh "claim": a rotating refresh token may be consumed even when
	// the upstream response is lost, and that extra write only creates version
	// conflicts without making the ambiguous network outcome recoverable.
	claimedCiphertext := connection.CredentialEncrypted

	client, err := s.inspector.clientForConnection(ctx, connection)
	if err != nil {
		return connection, credential, err
	}
	management := &upstreamManagementClient{client: client}
	refreshed, err := management.refreshSub2APIManagementToken(ctx, client, connection.ManagementBaseURL, upstreamManagementAuthSecret{
		AccessToken: credential.AccessToken, RefreshToken: credential.RefreshToken,
		UserAgent: credential.UserAgent, ExpiresAt: credential.ExpiresAt,
	})
	if err != nil {
		return connection, credential, err
	}
	refreshedInput := UpstreamConnectionCredentialInput{
		AccessToken: refreshed.AccessToken, RefreshToken: refreshed.RefreshToken,
		NotInCNConfirmed: credential.NotInCNConfirmed,
		UserAgent:        refreshed.UserAgent, ExpiresAt: refreshed.ExpiresAt,
	}
	ciphertext, fingerprint, hint, err := s.encryptCredential(connection.AuthMode, connection.ManagementBaseURL, refreshedInput)
	if err != nil {
		return connection, credential, err
	}
	applied, err := s.repo.FinalizeCredentialRefresh(
		ctx, connection.ID, claimedCiphertext, connection.Provider, connection.AuthMode, connection.ManagementBaseURL,
		UpstreamConnectionCredentialPersistence{
			CredentialEncrypted: ciphertext, CredentialFingerprint: fingerprint, CredentialHint: hint,
		},
	)
	if err != nil {
		return connection, credential, fmt.Errorf("persist refreshed upstream connection credential: %w", err)
	}
	if !applied {
		return connection, credential, ErrUpstreamConnectionChanged
	}
	connection, err = s.repo.GetByID(ctx, connection.ID)
	if err != nil {
		return connection, credential, fmt.Errorf("reload refreshed upstream connection credential: %w", err)
	}
	credential, err = s.loadCredential(connection)
	return connection, credential, err
}

func upstreamConnectionCredentialInput(credential upstreamConnectionCredential) UpstreamConnectionCredentialInput {
	return UpstreamConnectionCredentialInput{
		Username: credential.Username, Password: credential.Password,
		AccessToken: credential.AccessToken, RefreshToken: credential.RefreshToken,
		NotInCNConfirmed: credential.NotInCNConfirmed,
		UserAgent:        credential.UserAgent, ExpiresAt: credential.ExpiresAt,
	}
}

func (s *UpstreamConnectionService) recordProbeFailure(ctx context.Context, connection *UpstreamConnection, expectedVersion int64, probeErr error) {
	status := UpstreamConnectionStatusDegraded
	if !connection.SyncEnabled {
		status = UpstreamConnectionStatusDisabled
	} else if errors.Is(probeErr, ErrUpstreamConnectionAuthentication) {
		status = UpstreamConnectionStatusAuthError
	}
	if connection.SyncEnabled && errors.Is(probeErr, errUpstreamConnectionRemoteUserIDRequired) {
		status = UpstreamConnectionStatusNeedsInput
	}
	if connection.SyncEnabled && errors.Is(probeErr, ErrUpstreamManagementLocationConfirmationRequired) {
		status = UpstreamConnectionStatusNeedsInput
	}
	failures := connection.SyncFailures + 1
	backoff := upstreamConnectionFailureBackoff(failures)
	var next *time.Time
	if connection.SyncEnabled {
		value := time.Now().UTC().Add(backoff)
		next = &value
	}
	_, _ = s.repo.RecordProbeFailure(ctx, connection.ID, expectedVersion, UpstreamConnectionProbeFailure{
		Status: status, LastError: truncateUpstreamConnectionError(probeErr.Error()), SyncFailures: failures,
		NextSyncAt: next,
	})
}

func upstreamCredentialRefreshProbeError(err error) error {
	if errors.Is(err, ErrUpstreamConnectionAuthentication) {
		return infraerrors.New(401, "UPSTREAM_CONNECTION_CREDENTIAL_REAUTH_REQUIRED", "upstream connection credentials were rejected; sign in again").WithCause(err)
	}
	return infraerrors.New(502, "UPSTREAM_CONNECTION_CREDENTIAL_REFRESH_FAILED", "upstream connection credential refresh failed; retry the probe").WithCause(err)
}

func upstreamConnectionFailureBackoff(failures int) time.Duration {
	if failures < 1 {
		failures = 1
	}
	delay := time.Minute << min(failures-1, 5)
	if delay > 30*time.Minute {
		return 30 * time.Minute
	}
	return delay
}

func truncateUpstreamConnectionError(message string) string {
	const limit = 2000
	message = strings.TrimSpace(strings.ToValidUTF8(message, "�"))
	if len(message) <= limit {
		return message
	}
	cut := limit
	for cut > 0 && !utf8.ValidString(message[:cut]) {
		cut--
	}
	return message[:cut]
}

func (s *UpstreamConnectionService) loadCredential(connection *UpstreamConnection) (upstreamConnectionCredential, error) {
	if s == nil || s.encryptor == nil || connection == nil || strings.TrimSpace(connection.CredentialEncrypted) == "" {
		return upstreamConnectionCredential{}, errors.New("upstream connection credentials are unavailable")
	}
	plaintext, err := s.encryptor.Decrypt(connection.CredentialEncrypted)
	if err != nil {
		return upstreamConnectionCredential{}, errors.New("decrypt upstream connection credentials")
	}
	var credential upstreamConnectionCredential
	if err := json.Unmarshal([]byte(plaintext), &credential); err != nil {
		return upstreamConnectionCredential{}, errors.New("decode upstream connection credentials")
	}
	if credential.Version != 1 {
		return upstreamConnectionCredential{}, errors.New("unsupported upstream connection credential version")
	}
	userAgent, err := normalizeUpstreamManagementUserAgent(credential.UserAgent)
	if err != nil {
		return upstreamConnectionCredential{}, errors.New("invalid upstream connection user agent")
	}
	credential.UserAgent = userAgent
	return credential, nil
}

func (s *UpstreamConnectionService) normalizeCreate(params UpstreamConnectionCreateParams) (UpstreamConnectionCreateParams, UpstreamConnectionCredentialInput, error) {
	params.Name = strings.TrimSpace(params.Name)
	params.Provider = strings.ToLower(strings.TrimSpace(params.Provider))
	params.AuthMode = strings.ToLower(strings.TrimSpace(params.AuthMode))
	managementURL, err := s.normalizeURL(params.ManagementBaseURL, true)
	if err != nil {
		return params, params.Credential, infraerrors.BadRequest("INVALID_UPSTREAM_MANAGEMENT_URL", err.Error())
	}
	params.ManagementBaseURL = managementURL
	forwardingURL, err := s.normalizeOptionalURL(params.ForwardingBaseURL)
	if err != nil {
		return params, params.Credential, infraerrors.BadRequest("INVALID_UPSTREAM_FORWARDING_URL", err.Error())
	}
	params.ForwardingBaseURL = forwardingURL
	params.RemoteUserID, err = normalizeRemoteUserID(params.RemoteUserID)
	if err != nil {
		return params, params.Credential, infraerrors.BadRequest("INVALID_UPSTREAM_REMOTE_USER_ID", err.Error())
	}
	if params.SyncIntervalSeconds == 0 {
		params.SyncIntervalSeconds = 300
	}
	temporary := &UpstreamConnection{
		Name: params.Name, Provider: params.Provider, AuthMode: params.AuthMode,
		ManagementBaseURL: params.ManagementBaseURL, ForwardingBaseURL: params.ForwardingBaseURL,
		RemoteUserID: params.RemoteUserID, ProxyID: params.ProxyID, SyncEnabled: params.SyncEnabled,
		SyncIntervalSeconds: params.SyncIntervalSeconds,
	}
	if err := validateUpstreamConnection(temporary); err != nil {
		return params, params.Credential, err
	}
	return params, params.Credential, nil
}

func (s *UpstreamConnectionService) encryptCredential(authMode, managementBaseURL string, input UpstreamConnectionCredentialInput) (string, string, string, error) {
	if s == nil || s.encryptor == nil {
		return "", "", "", infraerrors.New(500, "UPSTREAM_CREDENTIAL_ENCRYPTION_UNAVAILABLE", "upstream credential encryption is unavailable")
	}
	credential, fingerprint, hint, err := upstreamConnectionCredentialIdentity(authMode, managementBaseURL, input)
	if err != nil {
		return "", "", "", err
	}
	payload, err := json.Marshal(credential)
	if err != nil {
		return "", "", "", fmt.Errorf("encode upstream connection credentials: %w", err)
	}
	ciphertext, err := s.encryptor.Encrypt(string(payload))
	if err != nil {
		return "", "", "", fmt.Errorf("encrypt upstream connection credentials: %w", err)
	}
	return ciphertext, fingerprint, hint, nil
}

func upstreamConnectionCredentialIdentity(authMode, managementBaseURL string, input UpstreamConnectionCredentialInput) (upstreamConnectionCredential, string, string, error) {
	userAgent, err := normalizeUpstreamManagementUserAgent(input.UserAgent)
	if err != nil {
		return upstreamConnectionCredential{}, "", "", infraerrors.BadRequest("INVALID_UPSTREAM_USER_AGENT", err.Error())
	}
	credential := upstreamConnectionCredential{
		Version: 1, Username: strings.TrimSpace(input.Username), Password: strings.TrimSpace(input.Password),
		AccessToken: strings.TrimSpace(input.AccessToken), RefreshToken: strings.TrimSpace(input.RefreshToken),
		NotInCNConfirmed: input.NotInCNConfirmed, UserAgent: userAgent, ExpiresAt: input.ExpiresAt,
	}
	if credential.ExpiresAt == 0 && credential.RefreshToken != "" {
		credential.ExpiresAt = upstreamManagementJWTExpiry(credential.AccessToken)
	}
	var fingerprintSource, hint string
	switch authMode {
	case string(UpstreamManagementAuthModePassword):
		if credential.Username == "" || credential.Password == "" {
			return upstreamConnectionCredential{}, "", "", infraerrors.BadRequest("INVALID_UPSTREAM_CREDENTIAL", "username and password are required")
		}
		fingerprintSource = "password-identity\x00" + strings.ToLower(credential.Username) + "\x00" + managementBaseURL
		hint = truncateUpstreamCredentialHint(credential.Username)
	case string(UpstreamManagementAuthModeAccessToken):
		if credential.AccessToken == "" {
			return upstreamConnectionCredential{}, "", "", infraerrors.BadRequest("INVALID_UPSTREAM_CREDENTIAL", "access token is required")
		}
		fingerprintSource = "access-token\x00" + credential.AccessToken
		hint = maskUpstreamCredential(credential.AccessToken)
	default:
		return upstreamConnectionCredential{}, "", "", infraerrors.BadRequest("INVALID_UPSTREAM_AUTH_MODE", "auth mode must be password or access_token")
	}
	digest := sha256.Sum256([]byte(fingerprintSource))
	return credential, "sha256:v1:" + hex.EncodeToString(digest[:]), hint, nil
}

// upstreamManagementJWTExpiry extracts the standard exp claim only to schedule
// a later refresh. The upstream still validates the token on every request.
func upstreamManagementJWTExpiry(accessToken string) int64 {
	parts := strings.Split(strings.TrimSpace(accessToken), ".")
	if len(parts) != 3 || parts[1] == "" {
		return 0
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		payload, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return 0
		}
	}
	var claims struct {
		ExpiresAt json.Number `json:"exp"`
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&claims); err != nil || claims.ExpiresAt == "" {
		return 0
	}
	expiresAt, err := strconv.ParseFloat(claims.ExpiresAt.String(), 64)
	const maxUnixSeconds = int64(1<<63 - 1)
	if err != nil || math.IsNaN(expiresAt) || math.IsInf(expiresAt, 0) || expiresAt <= 0 || expiresAt > float64(maxUnixSeconds) {
		return 0
	}
	return int64(expiresAt)
}

func truncateUpstreamCredentialHint(value string) string {
	const limit = 100
	runes := []rune(strings.TrimSpace(strings.ToValidUTF8(value, "")))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit-3]) + "..."
}

func (s *UpstreamConnectionService) normalizeOptionalURL(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	return s.normalizeURL(raw, false)
}

func (s *UpstreamConnectionService) normalizeURL(raw string, _ bool) (string, error) {
	var normalized string
	var err error
	if s.cfg == nil || !s.cfg.Security.URLAllowlist.Enabled {
		allowHTTP := s.cfg == nil || s.cfg.Security.URLAllowlist.AllowInsecureHTTP
		normalized, err = urlvalidator.ValidateURLFormat(raw, allowHTTP)
	} else {
		normalized, err = urlvalidator.ValidateHTTPSURL(raw, urlvalidator.ValidationOptions{
			AllowedHosts:     s.cfg.Security.URLAllowlist.UpstreamHosts,
			RequireAllowlist: len(s.cfg.Security.URLAllowlist.UpstreamHosts) > 0,
			AllowPrivate:     s.cfg.Security.URLAllowlist.AllowPrivateHosts,
		})
	}
	if err != nil {
		return "", err
	}
	parsed, err := url.Parse(normalized)
	if err != nil {
		return "", errors.New("invalid upstream base url")
	}
	if parsed.User != nil {
		return "", errors.New("upstream base url must not contain embedded credentials")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("upstream base url must not contain a query string or fragment")
	}
	return normalized, nil
}

func validateUpstreamConnection(connection *UpstreamConnection) error {
	if connection == nil {
		return infraerrors.BadRequest("INVALID_UPSTREAM_CONNECTION", "upstream connection is required")
	}
	if connection.Name == "" || utf8.RuneCountInString(connection.Name) > 100 {
		return infraerrors.BadRequest("INVALID_UPSTREAM_CONNECTION_NAME", "name must be 1-100 characters")
	}
	if !validUpstreamConnectionProvider(connection.Provider) {
		return infraerrors.BadRequest("INVALID_UPSTREAM_PROVIDER", "unsupported upstream provider")
	}
	if connection.AuthMode != string(UpstreamManagementAuthModePassword) && connection.AuthMode != string(UpstreamManagementAuthModeAccessToken) {
		return infraerrors.BadRequest("INVALID_UPSTREAM_AUTH_MODE", "auth mode must be password or access_token")
	}
	if connection.ManagementBaseURL == "" {
		return infraerrors.BadRequest("INVALID_UPSTREAM_MANAGEMENT_URL", "management base url is required")
	}
	if connection.SyncIntervalSeconds < 30 || connection.SyncIntervalSeconds > 86400 {
		return infraerrors.BadRequest("INVALID_UPSTREAM_SYNC_INTERVAL", "sync interval must be between 30 and 86400 seconds")
	}
	if connection.ProxyID != nil && *connection.ProxyID <= 0 {
		return infraerrors.BadRequest("INVALID_PROXY_ID", "proxy id must be positive")
	}
	if strings.TrimSpace(connection.RemoteUserID) != "" {
		if _, err := parseConnectionRemoteUserID(connection.RemoteUserID); err != nil {
			return infraerrors.BadRequest("INVALID_UPSTREAM_REMOTE_USER_ID", err.Error())
		}
	}
	if connection.AuthMode == string(UpstreamManagementAuthModeAccessToken) &&
		upstreamConnectionProviderRequiresRemoteUserID(connection.Provider) &&
		strings.TrimSpace(connection.RemoteUserID) == "" {
		return infraerrors.BadRequest("UPSTREAM_REMOTE_USER_ID_REQUIRED", "remote user id is required for this provider in access-token mode")
	}
	return nil
}

func upstreamConnectionProviderRequiresRemoteUserID(provider string) bool {
	switch provider {
	case UpstreamConnectionProviderNewAPI, UpstreamConnectionProviderRixAPI, UpstreamConnectionProviderShellAPI,
		UpstreamConnectionProviderVeloera:
		return true
	default:
		return false
	}
}

func validUpstreamConnectionProvider(provider string) bool {
	switch provider {
	case UpstreamConnectionProviderAuto, UpstreamConnectionProviderNewAPI, UpstreamConnectionProviderSub2API,
		UpstreamConnectionProviderRixAPI, UpstreamConnectionProviderShellAPI, UpstreamConnectionProviderOneAPI,
		UpstreamConnectionProviderVeloera, UpstreamConnectionProviderOneHub, UpstreamConnectionProviderDoneHub:
		return true
	default:
		return false
	}
}

func normalizeRemoteUserID(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if len(value) > 128 || strings.ContainsAny(value, "\r\n\x00") {
		return "", errors.New("remote user id must be at most 128 characters without line breaks")
	}
	return value, nil
}

func maskUpstreamCredential(value string) string {
	runes := []rune(strings.TrimSpace(strings.ToValidUTF8(value, "")))
	if len(runes) <= 8 {
		return "***"
	}
	return string(runes[:4]) + "..." + string(runes[len(runes)-4:])
}

func resetUpstreamConnectionObservations(connection *UpstreamConnection) {
	connection.Capabilities = map[string]any{}
	connection.Groups = []UpstreamGroup{}
	connection.GroupCount = 0
	connection.Status = UpstreamConnectionStatusPending
	connection.LastError = ""
	connection.SyncFailures = 0
	connection.WalletAmount = nil
	connection.WalletCurrency = ""
	connection.WalletUSD = nil
	connection.WalletUnlimited = false
	connection.WalletSource = ""
	connection.WalletReliability = "unknown"
	connection.WalletRaw = map[string]any{}
	connection.WalletObservedAt = nil
	connection.LastDiscoveredAt = nil
	connection.LastSyncedAt = nil
	connection.NextSyncAt = nil
}

func redactUpstreamConnections(connections []*UpstreamConnection) {
	for _, connection := range connections {
		redactUpstreamConnection(connection)
	}
}

func redactUpstreamConnection(connection *UpstreamConnection) {
	if connection == nil {
		return
	}
	connection.CredentialEncrypted = ""
	connection.CredentialFingerprint = ""
	for i := range connection.Bindings {
		connection.Bindings[i].KeyFingerprint = ""
	}
}

func (s *UpstreamConnectionService) populateCredentialMetadata(connection *UpstreamConnection) {
	if connection == nil || connection.AuthMode != string(UpstreamManagementAuthModePassword) {
		return
	}
	credential, err := s.loadCredential(connection)
	if err == nil {
		connection.NotInCNConfirmed = credential.NotInCNConfirmed
	}
}
