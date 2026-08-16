//go:build unit

package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type accountRepoStubForBulkUpdate struct {
	accountRepoStub
	bulkUpdateErr       error
	bulkUpdateIDs       []int64
	bulkUpdatePayload   AccountBulkUpdate
	bindGroupErrByID    map[int64]error
	bindGroupsCalls     []int64
	bindGroupsByAccount map[int64][]int64
	createAccount       *Account
	createID            int64
	createErr           error
	updatedAccounts     []*Account
	updateErr           error
	getByIDsAccounts    []*Account
	getByIDsErr         error
	getByIDsCalled      bool
	getByIDsIDs         []int64
	getByIDAccounts     map[int64]*Account
	getByIDErrByID      map[int64]error
	getByIDCalled       []int64
	listByGroupData     map[int64][]Account
	listByGroupErr      map[int64]error
	listData            []Account
	listResult          *pagination.PaginationResult
	listErr             error
	listCalled          bool
	lastListParams      pagination.PaginationParams
	lastListFilters     struct {
		platform    string
		accountType string
		status      string
		search      string
		groupID     int64
		privacyMode string
	}
}

func (s *accountRepoStubForBulkUpdate) BulkUpdate(_ context.Context, ids []int64, updates AccountBulkUpdate) (int64, error) {
	s.bulkUpdateIDs = append([]int64{}, ids...)
	s.bulkUpdatePayload = updates
	if s.bulkUpdateErr != nil {
		return 0, s.bulkUpdateErr
	}
	return int64(len(ids)), nil
}

func (s *accountRepoStubForBulkUpdate) Create(_ context.Context, account *Account) error {
	s.createAccount = account
	if s.createID > 0 {
		account.ID = s.createID
	}
	return s.createErr
}

func (s *accountRepoStubForBulkUpdate) Update(_ context.Context, account *Account) error {
	s.updatedAccounts = append(s.updatedAccounts, account)
	return s.updateErr
}

func (s *accountRepoStubForBulkUpdate) BindGroups(_ context.Context, accountID int64, groupIDs []int64) error {
	s.bindGroupsCalls = append(s.bindGroupsCalls, accountID)
	if s.bindGroupsByAccount == nil {
		s.bindGroupsByAccount = make(map[int64][]int64)
	}
	s.bindGroupsByAccount[accountID] = append([]int64{}, groupIDs...)
	if err, ok := s.bindGroupErrByID[accountID]; ok {
		return err
	}
	return nil
}

func (s *accountRepoStubForBulkUpdate) GetByIDs(_ context.Context, ids []int64) ([]*Account, error) {
	s.getByIDsCalled = true
	s.getByIDsIDs = append([]int64{}, ids...)
	if s.getByIDsErr != nil {
		return nil, s.getByIDsErr
	}
	return s.getByIDsAccounts, nil
}

func (s *accountRepoStubForBulkUpdate) GetByID(_ context.Context, id int64) (*Account, error) {
	s.getByIDCalled = append(s.getByIDCalled, id)
	if err, ok := s.getByIDErrByID[id]; ok {
		return nil, err
	}
	if account, ok := s.getByIDAccounts[id]; ok {
		return account, nil
	}
	return nil, errors.New("account not found")
}

func (s *accountRepoStubForBulkUpdate) ListByGroup(_ context.Context, groupID int64) ([]Account, error) {
	if err, ok := s.listByGroupErr[groupID]; ok {
		return nil, err
	}
	if rows, ok := s.listByGroupData[groupID]; ok {
		return rows, nil
	}
	return nil, nil
}

func (s *accountRepoStubForBulkUpdate) ListAllWithFilters(context.Context, string, string, string, string, int64, string) ([]Account, error) {
	return nil, nil
}

func (s *accountRepoStubForBulkUpdate) ListWithFilters(_ context.Context, params pagination.PaginationParams, platform, accountType, status, search string, groupID int64, privacyMode string) ([]Account, *pagination.PaginationResult, error) {
	s.listCalled = true
	s.lastListParams = params
	s.lastListFilters.platform = platform
	s.lastListFilters.accountType = accountType
	s.lastListFilters.status = status
	s.lastListFilters.search = search
	s.lastListFilters.groupID = groupID
	s.lastListFilters.privacyMode = privacyMode
	if s.listErr != nil {
		return nil, nil, s.listErr
	}
	if s.listResult != nil {
		return s.listData, s.listResult, nil
	}
	return s.listData, &pagination.PaginationResult{Total: int64(len(s.listData))}, nil
}

// TestAdminService_BulkUpdateAccounts_AllSuccessIDs 验证批量更新成功时返回 success_ids/failed_ids。
func TestAdminService_BulkUpdateAccounts_AllSuccessIDs(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{}
	svc := &adminServiceImpl{accountRepo: repo}

	schedulable := true
	input := &BulkUpdateAccountsInput{
		AccountIDs:  []int64{1, 2, 3},
		Schedulable: &schedulable,
	}

	result, err := svc.BulkUpdateAccounts(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, 3, result.Success)
	require.Equal(t, 0, result.Failed)
	require.ElementsMatch(t, []int64{1, 2, 3}, result.SuccessIDs)
	require.Empty(t, result.FailedIDs)
	require.Len(t, result.Results, 3)
}

func TestValidateCodexFingerprintModeExtraForAccount(t *testing.T) {
	tests := []struct {
		name        string
		platform    string
		accountType string
		extra       map[string]any
		wantReason  string
	}{
		{
			name:        "openai oauth accepts explicit off",
			platform:    PlatformOpenAI,
			accountType: AccountTypeOAuth,
			extra:       map[string]any{codexFingerprintModeExtraKey: "off"},
		},
		{
			name:        "openai oauth rejects malformed value",
			platform:    PlatformOpenAI,
			accountType: AccountTypeOAuth,
			extra:       map[string]any{codexFingerprintModeExtraKey: true},
			wantReason:  "CODEX_FINGERPRINT_MODE_INVALID",
		},
		{
			name:        "non oauth rejects setting",
			platform:    PlatformOpenAI,
			accountType: AccountTypeAPIKey,
			extra:       map[string]any{codexFingerprintModeExtraKey: "off"},
			wantReason:  "CODEX_FINGERPRINT_MODE_TARGET_INVALID",
		},
		{
			name:        "other platform rejects setting",
			platform:    PlatformAnthropic,
			accountType: AccountTypeOAuth,
			extra:       map[string]any{codexFingerprintModeExtraKey: "off"},
			wantReason:  "CODEX_FINGERPRINT_MODE_TARGET_INVALID",
		},
		{
			name:        "unrelated extra remains allowed",
			platform:    PlatformAnthropic,
			accountType: AccountTypeOAuth,
			extra:       map[string]any{"provider_owned": true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCodexFingerprintModeExtraForAccount(tt.platform, tt.accountType, tt.extra)
			if tt.wantReason == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Equal(t, tt.wantReason, infraerrors.Reason(err))
		})
	}
}

func TestAdminServiceUpdateAccountRejectsFingerprintModeOnTypeOnlyTransition(t *testing.T) {
	tests := []struct {
		name        string
		accountType string
		extra       map[string]any
		targetType  string
		wantReason  string
	}{
		{
			name:        "oauth to setup token cannot retain oauth-only setting",
			accountType: AccountTypeOAuth,
			extra:       map[string]any{codexFingerprintModeExtraKey: "session"},
			targetType:  AccountTypeSetupToken,
			wantReason:  "CODEX_FINGERPRINT_MODE_TARGET_INVALID",
		},
		{
			name:        "legacy api key malformed setting cannot become oauth",
			accountType: AccountTypeAPIKey,
			extra:       map[string]any{codexFingerprintModeExtraKey: true},
			targetType:  AccountTypeOAuth,
			wantReason:  "CODEX_FINGERPRINT_MODE_INVALID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{
				ID:       71,
				Platform: PlatformOpenAI,
				Type:     tt.accountType,
				Extra:    tt.extra,
			}
			repo := &accountRepoStubForBulkUpdate{getByIDAccounts: map[int64]*Account{account.ID: account}}
			svc := &adminServiceImpl{accountRepo: repo}

			_, err := svc.UpdateAccount(context.Background(), account.ID, &UpdateAccountInput{Type: tt.targetType})

			require.Error(t, err)
			require.Equal(t, tt.wantReason, infraerrors.Reason(err))
			require.Empty(t, repo.updatedAccounts, "validation must run before persistence")
		})
	}
}

func TestAdminService_BulkUpdateAccounts_AllowsManualMultiplierAfterRetiredSyncRemoval(t *testing.T) {
	multiplier := 0.5
	repo := &accountRepoStubForBulkUpdate{
		getByIDsAccounts: []*Account{{
			ID: 1,
			Extra: map[string]any{
				"upstream_rate_multiplier_sync_enabled": true,
			},
		}},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	result, err := svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
		AccountIDs:     []int64{1},
		RateMultiplier: &multiplier,
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, result.Success)
	require.True(t, repo.getByIDsCalled)
	require.Equal(t, []int64{1}, repo.bulkUpdateIDs)
}

func TestAdminService_BulkUpdateAccounts_DropsRetiredUpstreamSyncConfiguration(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{}
	svc := &adminServiceImpl{accountRepo: repo}

	input := &BulkUpdateAccountsInput{
		AccountIDs: []int64{1},
		Extra: map[string]any{
			"upstream_rate_multiplier_sync_enabled": true,
		},
	}
	result, err := svc.BulkUpdateAccounts(context.Background(), input)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, result.Success)
	require.Equal(t, []int64{1}, repo.bulkUpdateIDs)
	require.NotContains(t, input.Extra, "upstream_rate_multiplier_sync_enabled")
}

func TestAdminService_BulkUpdateAccounts_PartialFailureIDs(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{
		bindGroupErrByID: map[int64]error{
			2: errors.New("bind failed"),
		},
	}
	svc := &adminServiceImpl{
		accountRepo: repo,
		groupRepo:   &groupRepoStubForAdmin{getByID: &Group{ID: 10, Name: "g10"}},
	}

	groupIDs := []int64{10}
	schedulable := false
	input := &BulkUpdateAccountsInput{
		AccountIDs:            []int64{1, 2, 3},
		GroupIDs:              &groupIDs,
		Schedulable:           &schedulable,
		SkipMixedChannelCheck: true,
	}

	result, err := svc.BulkUpdateAccounts(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, 2, result.Success)
	require.Equal(t, 1, result.Failed)
	require.ElementsMatch(t, []int64{1, 3}, result.SuccessIDs)
	require.ElementsMatch(t, []int64{2}, result.FailedIDs)
	require.Len(t, result.Results, 3)
}

func TestAdminService_BulkUpdateAccounts_NilGroupRepoReturnsError(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{}
	svc := &adminServiceImpl{accountRepo: repo}

	groupIDs := []int64{10}
	input := &BulkUpdateAccountsInput{
		AccountIDs: []int64{1},
		GroupIDs:   &groupIDs,
	}

	result, err := svc.BulkUpdateAccounts(context.Background(), input)
	require.Nil(t, result)
	require.Error(t, err)
	require.Contains(t, err.Error(), "group repository not configured")
}

// TestAdminService_BulkUpdateAccounts_MixedChannelPreCheckBlocksOnExistingConflict verifies
// that the global pre-check detects a conflict with existing group members and returns an
// error before any DB write is performed.
func TestAdminService_BulkUpdateAccounts_MixedChannelPreCheckBlocksOnExistingConflict(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{
		getByIDsAccounts: []*Account{
			{ID: 1, Platform: PlatformAntigravity},
		},
		// Group 10 already contains an Anthropic account.
		listByGroupData: map[int64][]Account{
			10: {{ID: 99, Platform: PlatformAnthropic}},
		},
	}
	svc := &adminServiceImpl{
		accountRepo: repo,
		groupRepo:   &groupRepoStubForAdmin{getByID: &Group{ID: 10, Name: "target-group"}},
	}

	groupIDs := []int64{10}
	input := &BulkUpdateAccountsInput{
		AccountIDs: []int64{1},
		GroupIDs:   &groupIDs,
	}

	result, err := svc.BulkUpdateAccounts(context.Background(), input)
	require.Nil(t, result)
	require.Error(t, err)
	require.Contains(t, err.Error(), "mixed channel")
	// No BindGroups should have been called since the check runs before any write.
	require.Empty(t, repo.bindGroupsCalls)
}

func TestAdminServiceBulkUpdateAccounts_ResolvesIDsFromFilters(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{
		listData: []Account{
			{ID: 7},
			{ID: 11},
		},
		listResult: &pagination.PaginationResult{Total: 2},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	schedulable := true
	input := &BulkUpdateAccountsInput{
		Schedulable: &schedulable,
	}

	filtersField := reflect.ValueOf(input).Elem().FieldByName("Filters")
	require.True(t, filtersField.IsValid(), "BulkUpdateAccountsInput should expose Filters for filter-target bulk update")
	require.Equal(t, reflect.Ptr, filtersField.Kind(), "BulkUpdateAccountsInput.Filters should be a pointer field")

	filtersValue := reflect.New(filtersField.Type().Elem())
	filtersValue.Elem().FieldByName("Platform").SetString(PlatformOpenAI)
	filtersValue.Elem().FieldByName("Type").SetString(AccountTypeOAuth)
	filtersValue.Elem().FieldByName("Status").SetString(StatusActive)
	filtersValue.Elem().FieldByName("Group").SetString("12")
	filtersValue.Elem().FieldByName("PrivacyMode").SetString(PrivacyModeCFBlocked)
	filtersValue.Elem().FieldByName("Search").SetString("bulk-target")
	filtersField.Set(filtersValue)

	result, err := svc.BulkUpdateAccounts(context.Background(), input)
	require.NoError(t, err)
	require.True(t, repo.listCalled, "expected filter-target bulk update to resolve matching IDs via account list filters")
	require.Equal(t, PlatformOpenAI, repo.lastListFilters.platform)
	require.Equal(t, AccountTypeOAuth, repo.lastListFilters.accountType)
	require.Equal(t, StatusActive, repo.lastListFilters.status)
	require.Equal(t, "bulk-target", repo.lastListFilters.search)
	require.Equal(t, int64(12), repo.lastListFilters.groupID)
	require.Equal(t, PrivacyModeCFBlocked, repo.lastListFilters.privacyMode)
	require.Equal(t, []int64{7, 11}, repo.bulkUpdateIDs)
	require.Equal(t, 2, result.Success)
	require.Equal(t, 0, result.Failed)
	require.Equal(t, []int64{7, 11}, result.SuccessIDs)
}

func TestAdminServiceBulkUpdateAccounts_CodexFingerprintModeRequiresEveryTargetToBeOpenAIOAuth(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{
		getByIDsAccounts: []*Account{
			{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth},
			{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
		},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	result, err := svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
		AccountIDs: []int64{1, 2},
		Extra: map[string]any{
			codexFingerprintModeExtraKey: "session",
		},
	})

	require.Nil(t, result)
	require.ErrorContains(t, err, "must be an OpenAI OAuth account")
	require.Empty(t, repo.bulkUpdateIDs, "validation must finish before any target is updated")
}

func TestAdminServiceBulkUpdateAccounts_CodexFingerprintModeValidatesAllFilterTargets(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{
		listData: []Account{
			{ID: 7},
			{ID: 11},
		},
		listResult: &pagination.PaginationResult{Total: 2},
		getByIDsAccounts: []*Account{
			{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeOAuth},
			{ID: 11, Platform: PlatformAnthropic, Type: AccountTypeOAuth},
		},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	result, err := svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
		Filters: &BulkUpdateAccountFilters{Platform: PlatformOpenAI},
		Extra: map[string]any{
			codexFingerprintModeExtraKey: "full",
		},
	})

	require.Nil(t, result)
	require.ErrorContains(t, err, "must be an OpenAI OAuth account")
	require.True(t, repo.listCalled)
	require.Equal(t, []int64{7, 11}, repo.getByIDsIDs)
	require.Empty(t, repo.bulkUpdateIDs, "a later incompatible filter target must block the whole write")
}

func TestAdminServiceBulkUpdateAccounts_CodexFingerprintModeAcceptsOnlyCanonicalModes(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{}
	svc := &adminServiceImpl{accountRepo: repo}

	result, err := svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
		AccountIDs: []int64{1},
		Extra: map[string]any{
			codexFingerprintModeExtraKey: " session ",
		},
	})

	require.Nil(t, result)
	require.ErrorContains(t, err, "codex_fingerprint_mode must be one of")
	require.False(t, repo.getByIDsCalled)
	require.Empty(t, repo.bulkUpdateIDs)
}

func TestAdminServiceBulkUpdateAccounts_CodexFingerprintModeUpdatesEligibleTargets(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{
		getByIDsAccounts: []*Account{
			{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth},
			{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth},
		},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	result, err := svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
		AccountIDs: []int64{1, 2},
		Extra: map[string]any{
			codexFingerprintModeExtraKey: "off",
		},
	})

	require.NoError(t, err)
	require.Equal(t, 2, result.Success)
	require.Equal(t, []int64{1, 2}, repo.bulkUpdateIDs)
	require.Equal(t, "off", repo.bulkUpdatePayload.Extra[codexFingerprintModeExtraKey])
}
