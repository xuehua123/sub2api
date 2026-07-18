package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

const (
	upstreamLegacyMigrationActionCreateAndBind       = "create_and_bind"
	upstreamLegacyMigrationActionReuseAndBind        = "reuse_and_bind"
	upstreamLegacyMigrationActionMigrated            = "migrated"
	upstreamLegacyMigrationActionReusedAndBound      = "reused_and_bound"
	upstreamLegacyMigrationActionAlreadyMigrated     = "already_migrated"
	upstreamLegacyMigrationActionSkipDisabled        = "skip_disabled"
	upstreamLegacyMigrationActionSkipInvalid         = "skip_invalid"
	upstreamLegacyMigrationActionSkipExistingBinding = "skip_existing_binding"
	upstreamLegacyMigrationActionFailed              = "failed"
)

type upstreamLegacyMigrationAccountRepository interface {
	ListUpstreamManagementAuthRotationCandidates(ctx context.Context) ([]Account, error)
}

type upstreamLegacyMigrationCandidate struct {
	account      Account
	params       UpstreamConnectionCreateParams
	legacyGroup  string
	migrationKey string
	itemIndex    int
}

type upstreamLegacyMigrationGroup struct {
	key        string
	candidates []*upstreamLegacyMigrationCandidate
	source     *upstreamLegacyMigrationCandidate
	connection *UpstreamConnection
}

type upstreamLegacyMigrationPlan struct {
	result UpstreamLegacyMigrationResult
	groups []*upstreamLegacyMigrationGroup
}

// PreviewLegacyMigration reports exactly what the explicit compatibility
// import would do. It decrypts legacy credentials only in memory and never
// returns credential material or identity fingerprints.
func (s *UpstreamConnectionService) PreviewLegacyMigration(ctx context.Context) (*UpstreamLegacyMigrationResult, error) {
	plan, err := s.planLegacyMigration(ctx)
	if err != nil {
		return nil, err
	}
	plan.result.DryRun = true
	plan.result.Summary = summarizeLegacyMigration(plan.result, len(plan.groups))
	return &plan.result, nil
}

// MigrateLegacyConnections explicitly imports old account-local management
// identities into shared observe-only connections. Legacy credentials, legacy
// sync flags, account multipliers, and all billing fields remain untouched.
func (s *UpstreamConnectionService) MigrateLegacyConnections(ctx context.Context) (*UpstreamLegacyMigrationResult, error) {
	plan, err := s.planLegacyMigration(ctx)
	if err != nil {
		return nil, err
	}
	plan.result.DryRun = false

	for _, group := range plan.groups {
		planned := legacyMigrationPlannedCandidates(group, plan.result.Items)
		if len(planned) == 0 {
			continue
		}

		connection := group.connection
		created := false
		if connection == nil {
			connection, err = s.create(ctx, group.source.params, group.key)
			created = err == nil
			if err != nil {
				// The unique migration key makes concurrent admin requests
				// converge on one row. If another node won the race, reuse it.
				connection, _ = s.repo.GetByLegacyMigrationKey(ctx, group.key)
			}
			if connection == nil {
				for _, candidate := range planned {
					item := &plan.result.Items[candidate.itemIndex]
					item.Action = upstreamLegacyMigrationActionFailed
					item.Message = "failed to create shared upstream connection"
				}
				continue
			}
		}

		for _, candidate := range planned {
			item := &plan.result.Items[candidate.itemIndex]
			binding, bindingErr := s.repo.GetAccountBinding(ctx, candidate.account.ID)
			switch {
			case bindingErr == nil && binding.ConnectionID == connection.ID:
				item.Action = upstreamLegacyMigrationActionAlreadyMigrated
				item.ConnectionID = int64Pointer(connection.ID)
				item.Message = "account is already bound to this migrated connection"
				continue
			case bindingErr == nil:
				item.Action = upstreamLegacyMigrationActionSkipExistingBinding
				item.ConnectionID = int64Pointer(binding.ConnectionID)
				item.Message = "account already has a different V2 binding; it was not replaced"
				continue
			case !errors.Is(bindingErr, ErrUpstreamAccountBindingNotFound):
				item.Action = upstreamLegacyMigrationActionFailed
				item.Message = "failed to recheck the existing V2 binding"
				continue
			}

			binding, bindErr := s.BindAccount(ctx, connection.ID, candidate.account.ID)
			if bindErr != nil {
				item.Action = upstreamLegacyMigrationActionFailed
				item.ConnectionID = int64Pointer(connection.ID)
				item.Message = "shared connection was created, but the account binding failed"
				continue
			}
			if created {
				item.Action = upstreamLegacyMigrationActionMigrated
			} else {
				item.Action = upstreamLegacyMigrationActionReusedAndBound
			}
			item.ConnectionID = int64Pointer(connection.ID)
			item.Message = "legacy settings remain active; verify V2 observations before disabling them"
			if binding.Status == UpstreamBindingStatusError {
				item.Message = "binding saved for retry; initial key resolution did not succeed"
			}
		}

		// Bind first so the compatibility credential hand-off can see its
		// legacy source. Probing an expiring Sub2API token before the binding
		// exists could otherwise make V2 and the legacy worker rotate the same
		// refresh token concurrently.
		if _, probeErr := s.Probe(ctx, connection.ID); probeErr != nil {
			plan.result.Warnings = append(plan.result.Warnings,
				fmt.Sprintf("connection %d was migrated, but its initial wallet/group probe failed", connection.ID))
		} else {
			// The first bind intentionally precedes the probe to establish token
			// ownership. Resolve the newly bound keys once more now that the group
			// snapshot exists, so their observed multipliers are immediately useful.
			for _, candidate := range planned {
				item := &plan.result.Items[candidate.itemIndex]
				if item.ConnectionID == nil || *item.ConnectionID != connection.ID {
					continue
				}
				if _, resolveErr := s.BindAccount(ctx, connection.ID, candidate.account.ID); resolveErr != nil {
					plan.result.Warnings = append(plan.result.Warnings,
						fmt.Sprintf("account %d was migrated, but its key group could not be re-resolved", candidate.account.ID))
				}
			}
		}
	}

	plan.result.Summary = summarizeLegacyMigration(plan.result, len(plan.groups))
	return &plan.result, nil
}

func (s *UpstreamConnectionService) planLegacyMigration(ctx context.Context) (*upstreamLegacyMigrationPlan, error) {
	source, ok := s.accountRepo.(upstreamLegacyMigrationAccountRepository)
	if !ok {
		return nil, errors.New("legacy upstream migration account source is unavailable")
	}
	accounts, err := source.ListUpstreamManagementAuthRotationCandidates(ctx)
	if err != nil {
		return nil, fmt.Errorf("list legacy upstream management accounts: %w", err)
	}

	plan := &upstreamLegacyMigrationPlan{
		result: UpstreamLegacyMigrationResult{
			Items: make([]UpstreamLegacyMigrationItem, 0, len(accounts)), Warnings: []string{},
		},
	}
	plan.result.Summary.ScannedAccounts = len(accounts)
	groupsByKey := make(map[string]*upstreamLegacyMigrationGroup)

	for index := range accounts {
		account := accounts[index]
		item := UpstreamLegacyMigrationItem{
			AccountID: account.ID, AccountName: account.Name, ProxyID: cloneInt64Pointer(account.ProxyID),
			Provider:          strings.ToLower(extraString(account.Extra, AccountExtraUpstreamRateMultiplierSyncProvider)),
			AuthMode:          strings.ToLower(extraString(account.Extra, AccountExtraUpstreamRateMultiplierSyncAuthMode)),
			ManagementBaseURL: accountUpstreamManagementBaseURL(&account),
			ForwardingBaseURL: accountUpstreamBaseURL(&account),
			LegacyGroup:       extraString(account.Extra, AccountExtraUpstreamRateMultiplierSyncGroup),
		}
		if !account.IsUpstreamRateMultiplierSyncEnabled() {
			item.Action = upstreamLegacyMigrationActionSkipDisabled
			item.Message = "legacy upstream monitoring is disabled"
			plan.result.Items = append(plan.result.Items, item)
			continue
		}

		candidate, buildErr := s.buildLegacyMigrationCandidate(account)
		if buildErr != nil {
			item.Action = upstreamLegacyMigrationActionSkipInvalid
			item.Message = buildErr.Error()
			plan.result.Items = append(plan.result.Items, item)
			continue
		}
		item.Provider = candidate.params.Provider
		item.AuthMode = candidate.params.AuthMode
		item.ManagementBaseURL = candidate.params.ManagementBaseURL
		item.ForwardingBaseURL = candidate.params.ForwardingBaseURL
		item.LegacyGroup = candidate.legacyGroup
		candidate.itemIndex = len(plan.result.Items)
		plan.result.Items = append(plan.result.Items, item)
		plan.result.Summary.EligibleAccounts++

		group := groupsByKey[candidate.migrationKey]
		if group == nil {
			group = &upstreamLegacyMigrationGroup{key: candidate.migrationKey}
			groupsByKey[candidate.migrationKey] = group
			plan.groups = append(plan.groups, group)
		}
		group.candidates = append(group.candidates, candidate)
		if group.source == nil || candidate.account.UpdatedAt.After(group.source.account.UpdatedAt) {
			group.source = candidate
		}
	}

	sort.Slice(plan.groups, func(left, right int) bool {
		return plan.groups[left].source.account.ID < plan.groups[right].source.account.ID
	})
	for _, group := range plan.groups {
		connection, lookupErr := s.repo.GetByLegacyMigrationKey(ctx, group.key)
		if lookupErr != nil && !errors.Is(lookupErr, ErrUpstreamConnectionNotFound) {
			return nil, fmt.Errorf("find existing migrated upstream connection: %w", lookupErr)
		}
		group.connection = connection
		for _, candidate := range group.candidates {
			item := &plan.result.Items[candidate.itemIndex]
			binding, bindingErr := s.repo.GetAccountBinding(ctx, candidate.account.ID)
			switch {
			case bindingErr == nil && connection != nil && binding.ConnectionID == connection.ID:
				item.Action = upstreamLegacyMigrationActionAlreadyMigrated
				item.ConnectionID = int64Pointer(connection.ID)
				item.Message = "account is already bound to this migrated connection"
			case bindingErr == nil:
				item.Action = upstreamLegacyMigrationActionSkipExistingBinding
				item.ConnectionID = int64Pointer(binding.ConnectionID)
				item.Message = "account already has a different V2 binding; it will not be replaced"
			case errors.Is(bindingErr, ErrUpstreamAccountBindingNotFound):
				if connection == nil {
					item.Action = upstreamLegacyMigrationActionCreateAndBind
				} else {
					item.Action = upstreamLegacyMigrationActionReuseAndBind
					item.ConnectionID = int64Pointer(connection.ID)
				}
				item.Message = "legacy settings will be preserved"
			default:
				return nil, fmt.Errorf("check account %d upstream binding: %w", candidate.account.ID, bindingErr)
			}
		}
	}

	sort.Slice(plan.result.Items, func(left, right int) bool {
		return plan.result.Items[left].AccountID < plan.result.Items[right].AccountID
	})
	// Sorting response items changes their indexes; rebuild candidate pointers.
	itemIndexByAccount := make(map[int64]int, len(plan.result.Items))
	for index := range plan.result.Items {
		itemIndexByAccount[plan.result.Items[index].AccountID] = index
	}
	for _, group := range plan.groups {
		for _, candidate := range group.candidates {
			candidate.itemIndex = itemIndexByAccount[candidate.account.ID]
		}
	}
	return plan, nil
}

func (s *UpstreamConnectionService) buildLegacyMigrationCandidate(account Account) (*upstreamLegacyMigrationCandidate, error) {
	if !supportsUpstreamRateMultiplierSyncAccountType(account.Type) {
		return nil, errors.New("legacy monitoring account type is not supported by V2")
	}
	config, err := account.UpstreamRateMultiplierSyncConfig()
	if err != nil {
		return nil, fmt.Errorf("invalid legacy monitoring config: %w", err)
	}
	secret, err := DecryptUpstreamManagementAuth(s.encryptor, account.GetCredential(upstreamManagementAuthCredentialKey))
	if err != nil {
		return nil, errors.New("legacy management credentials cannot be decrypted")
	}
	if err := validateUpstreamManagementAuth(config, secret); err != nil {
		return nil, fmt.Errorf("invalid legacy management credentials: %w", err)
	}
	if strings.TrimSpace(accountBalanceAPIKey(&account)) == "" {
		return nil, errors.New("account does not contain a forwarding API key")
	}

	credential := UpstreamConnectionCredentialInput{
		Username: secret.Username, Password: secret.Password,
		AccessToken: secret.AccessToken, RefreshToken: secret.RefreshToken,
		ExpiresAt: secret.ExpiresAt, LegacyManaged: true,
	}
	remoteUserID := ""
	if config.RemoteUserID > 0 {
		remoteUserID = strconv.FormatInt(config.RemoteUserID, 10)
	}
	params := UpstreamConnectionCreateParams{
		Name:     legacyUpstreamConnectionName(string(config.Provider), accountUpstreamManagementBaseURL(&account), account.Name),
		Provider: string(config.Provider), AuthMode: string(config.AuthMode),
		ManagementBaseURL: accountUpstreamManagementBaseURL(&account),
		ForwardingBaseURL: accountUpstreamBaseURL(&account), Credential: credential,
		RemoteUserID: remoteUserID, ProxyID: cloneInt64Pointer(account.ProxyID),
		SyncEnabled: true, SyncIntervalSeconds: 300,
	}
	normalized, normalizedCredential, err := s.normalizeCreate(params)
	if err != nil {
		return nil, err
	}
	_, credentialFingerprint, _, err := upstreamConnectionCredentialIdentity(
		normalized.AuthMode, normalized.ManagementBaseURL, normalizedCredential,
	)
	if err != nil {
		return nil, err
	}
	identityCredential := credentialFingerprint
	identityRemoteUserID := normalized.RemoteUserID
	if normalized.AuthMode == string(UpstreamManagementAuthModePassword) {
		identityRemoteUserID = ""
	}
	if normalized.AuthMode == string(UpstreamManagementAuthModeAccessToken) &&
		normalized.Provider != UpstreamConnectionProviderSub2API && normalized.RemoteUserID != "" {
		identityCredential = "remote-user:" + normalized.RemoteUserID
	}
	identity := strings.Join([]string{
		"legacy-upstream-connection-v2", normalized.Provider, normalized.AuthMode,
		normalized.ManagementBaseURL, identityRemoteUserID, pointerInt64String(normalized.ProxyID), identityCredential,
	}, "\x00")
	digest := sha256.Sum256([]byte(identity))
	return &upstreamLegacyMigrationCandidate{
		account: account, params: normalized, legacyGroup: config.Group,
		migrationKey: "sha256:v1:" + hex.EncodeToString(digest[:]),
	}, nil
}

func legacyMigrationPlannedCandidates(group *upstreamLegacyMigrationGroup, items []UpstreamLegacyMigrationItem) []*upstreamLegacyMigrationCandidate {
	planned := make([]*upstreamLegacyMigrationCandidate, 0, len(group.candidates))
	for _, candidate := range group.candidates {
		action := items[candidate.itemIndex].Action
		if action == upstreamLegacyMigrationActionCreateAndBind || action == upstreamLegacyMigrationActionReuseAndBind {
			planned = append(planned, candidate)
		}
	}
	return planned
}

func summarizeLegacyMigration(result UpstreamLegacyMigrationResult, uniqueConnections int) UpstreamLegacyMigrationSummary {
	summary := result.Summary
	summary.UniqueConnections = uniqueConnections
	summary.PlannedAccounts = 0
	summary.MigratedAccounts = 0
	summary.AlreadyMigrated = 0
	summary.SkippedAccounts = 0
	summary.FailedAccounts = 0
	for _, item := range result.Items {
		switch item.Action {
		case upstreamLegacyMigrationActionCreateAndBind, upstreamLegacyMigrationActionReuseAndBind:
			summary.PlannedAccounts++
		case upstreamLegacyMigrationActionMigrated, upstreamLegacyMigrationActionReusedAndBound:
			summary.MigratedAccounts++
		case upstreamLegacyMigrationActionAlreadyMigrated:
			summary.AlreadyMigrated++
		case upstreamLegacyMigrationActionFailed:
			summary.FailedAccounts++
		default:
			summary.SkippedAccounts++
		}
	}
	return summary
}

func legacyUpstreamConnectionName(provider, rawURL, accountName string) string {
	host := strings.TrimSpace(rawURL)
	if parsed, err := url.Parse(host); err == nil && parsed.Hostname() != "" {
		host = parsed.Hostname()
	}
	name := fmt.Sprintf("Legacy %s %s", strings.ToUpper(strings.TrimSpace(provider)), host)
	if accountName = strings.TrimSpace(accountName); accountName != "" {
		name += " (" + accountName + ")"
	}
	runes := []rune(name)
	if len(runes) > 100 {
		name = string(runes[:100])
	}
	return name
}

func pointerInt64String(value *int64) string {
	if value == nil {
		return "direct"
	}
	return strconv.FormatInt(*value, 10)
}

func int64Pointer(value int64) *int64 {
	copy := value
	return &copy
}
