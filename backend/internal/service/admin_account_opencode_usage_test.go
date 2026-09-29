package service

import (
	"context"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
	"time"
)

type openCodeGoAdminAccountRepo struct {
	AccountRepository
	mu          sync.Mutex
	accounts    map[int64]*Account
	updates     map[int64][]map[string]any
	bulkUpdates []AccountBulkUpdate
}

func (r *openCodeGoAdminAccountRepo) Create(_ context.Context, account *Account) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.accounts == nil {
		r.accounts = make(map[int64]*Account)
	}
	if account.ID == 0 {
		account.ID = int64(len(r.accounts) + 1)
	}
	r.accounts[account.ID] = account
	return nil
}

func (r *openCodeGoAdminAccountRepo) Update(_ context.Context, account *Account) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.accounts[account.ID] = account
	return nil
}

func (r *openCodeGoAdminAccountRepo) BulkUpdate(_ context.Context, ids []int64, updates AccountBulkUpdate) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bulkUpdates = append(r.bulkUpdates, updates)
	return int64(len(ids)), nil
}

func (r *openCodeGoAdminAccountRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	account := r.accounts[id]
	if account == nil {
		return nil, ErrAccountNotFound
	}
	clone := *account
	clone.Credentials = mergeMap(nil, account.Credentials)
	clone.Extra = mergeMap(nil, account.Extra)
	return &clone, nil
}

func (r *openCodeGoAdminAccountRepo) GetByIDs(_ context.Context, ids []int64) ([]*Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]*Account, 0, len(ids))
	for _, id := range ids {
		if account := r.accounts[id]; account != nil {
			result = append(result, account)
		}
	}
	return result, nil
}

func (r *openCodeGoAdminAccountRepo) UpdateExtra(_ context.Context, id int64, updates map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	account := r.accounts[id]
	if account == nil {
		return ErrAccountNotFound
	}
	if account.Extra == nil {
		account.Extra = make(map[string]any)
	}
	for key, value := range updates {
		account.Extra[key] = value
	}
	if r.updates == nil {
		r.updates = make(map[int64][]map[string]any)
	}
	r.updates[id] = append(r.updates[id], updates)
	return nil
}

type openCodeGoUsageSettingRepo struct {
	SettingRepository
	mu     sync.Mutex
	values map[string]string
}

func (r *openCodeGoUsageSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	value, ok := r.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}

func (r *openCodeGoUsageSettingRepo) Set(_ context.Context, key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.values == nil {
		r.values = make(map[string]string)
	}
	r.values[key] = value
	return nil
}

func openCodeGoAdminUsageAccount(id int64, apiKey string) *Account {
	previousAttempt := time.Date(2026, time.August, 15, 8, 0, 0, 0, time.UTC)
	return &Account{
		ID:       id,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Status:   StatusActive,
		Credentials: map[string]any{
			"api_key":  apiKey,
			"base_url": "https://opencode.ai/zen/go/v1",
		},
		Extra: map[string]any{
			OpenCodeGoUsageAutoRefreshExtraKey: true,
			OpenCodeGoUsageSnapshotExtraKey: &OpenCodeGoUsageSnapshot{
				Status:        OpenCodeGoUsageStatusOK,
				LastAttemptAt: previousAttempt,
				NextRefreshAt: previousAttempt.Add(time.Hour),
			},
		},
	}
}

// 普通账号编辑（脱敏 extra 不带受管键）不得丢 OpenCode Go 受管状态。
func TestUpdateAccountPreservesOpenCodeGoManagedStateForUnrelatedEdit(t *testing.T) {
	accountID := int64(210)
	repo := &openCodeGoAdminAccountRepo{accounts: map[int64]*Account{
		accountID: openCodeGoAdminUsageAccount(accountID, "sk-1"),
	}}

	svc := &adminServiceImpl{accountRepo: repo}
	updated, err := svc.UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
		Extra: map[string]any{"custom": "value"},
	})

	require.NoError(t, err)
	require.Equal(t, true, updated.Extra[OpenCodeGoUsageAutoRefreshExtraKey])
	snapshot, ok := updated.Extra[OpenCodeGoUsageSnapshotExtraKey].(*OpenCodeGoUsageSnapshot)
	require.True(t, ok)
	require.Equal(t, OpenCodeGoUsageStatusOK, snapshot.Status)
	require.Equal(t, "value", updated.Extra["custom"])
}

// 客户端在 extra 里伪造受管键必须被丢弃，以数据库中的受管状态为准。
func TestUpdateAccountRejectsInjectedOpenCodeGoManagedKeys(t *testing.T) {
	accountID := int64(211)
	repo := &openCodeGoAdminAccountRepo{accounts: map[int64]*Account{
		accountID: openCodeGoAdminUsageAccount(accountID, "sk-1"),
	}}

	svc := &adminServiceImpl{accountRepo: repo}
	updated, err := svc.UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
		Extra: map[string]any{
			OpenCodeGoUsageAutoRefreshExtraKey: false,
			OpenCodeGoUsageSnapshotExtraKey:    map[string]any{"status": "forged"},
			"custom":                           "value",
		},
	})

	require.NoError(t, err)
	require.Equal(t, true, updated.Extra[OpenCodeGoUsageAutoRefreshExtraKey])
	snapshot, ok := updated.Extra[OpenCodeGoUsageSnapshotExtraKey].(*OpenCodeGoUsageSnapshot)
	require.True(t, ok)
	require.Equal(t, OpenCodeGoUsageStatusOK, snapshot.Status)
	require.Equal(t, "value", updated.Extra["custom"])
}

// OpenCode 身份改变（api_key / base_url / type）必须清除旧受管状态，防止跨组污染。
func TestUpdateAccountClearsOpenCodeGoManagedStateWhenIdentityChanges(t *testing.T) {
	tests := []struct {
		name  string
		input *UpdateAccountInput
	}{
		{name: "api key", input: &UpdateAccountInput{Credentials: map[string]any{"api_key": "sk-new"}}},
		{name: "base url", input: &UpdateAccountInput{Credentials: map[string]any{"base_url": "https://api.openai.com/v1"}}},
		{name: "type", input: &UpdateAccountInput{Type: AccountTypeOAuth}},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accountID := int64(220 + i)
			repo := &openCodeGoAdminAccountRepo{accounts: map[int64]*Account{
				accountID: openCodeGoAdminUsageAccount(accountID, "sk-1"),
			}}

			updated, err := (&adminServiceImpl{accountRepo: repo}).UpdateAccount(context.Background(), accountID, tt.input)

			require.NoError(t, err)
			require.NotContains(t, updated.Extra, OpenCodeGoUsageAutoRefreshExtraKey)
			require.NotContains(t, updated.Extra, OpenCodeGoUsageSnapshotExtraKey)
		})
	}
}

// UpdateAccountExtra 是 key 级合并入口，伪造的受管键必须被剥离。
func TestUpdateAccountExtraDropsOpenCodeGoManagedKeys(t *testing.T) {
	accountID := int64(230)
	repo := &openCodeGoAdminAccountRepo{accounts: map[int64]*Account{
		accountID: openCodeGoAdminUsageAccount(accountID, "sk-1"),
	}}

	svc := &adminServiceImpl{accountRepo: repo}
	err := svc.UpdateAccountExtra(context.Background(), accountID, map[string]any{
		OpenCodeGoUsageAutoRefreshExtraKey: false,
		OpenCodeGoUsageSnapshotExtraKey:    map[string]any{"status": "forged"},
		"custom":                           "value",
	})

	require.NoError(t, err)
	require.Len(t, repo.updates[accountID], 1)
	persisted := repo.updates[accountID][0]
	require.NotContains(t, persisted, OpenCodeGoUsageAutoRefreshExtraKey)
	require.NotContains(t, persisted, OpenCodeGoUsageSnapshotExtraKey)
	require.Equal(t, "value", persisted["custom"])
}

// 批量更新同样不得接受伪造的受管键。
func TestBulkUpdateAccountsDropsOpenCodeGoManagedKeys(t *testing.T) {
	accountID := int64(240)
	repo := &openCodeGoAdminAccountRepo{accounts: map[int64]*Account{
		accountID: openCodeGoAdminUsageAccount(accountID, "sk-1"),
	}}

	svc := &adminServiceImpl{accountRepo: repo}
	_, err := svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
		AccountIDs: []int64{accountID},
		Extra: map[string]any{
			OpenCodeGoUsageAutoRefreshExtraKey: false,
			OpenCodeGoUsageSnapshotExtraKey:    map[string]any{"status": "forged"},
			"custom":                           "value",
		},
	})

	require.NoError(t, err)
	require.Len(t, repo.bulkUpdates, 1)
	require.NotContains(t, repo.bulkUpdates[0].Extra, OpenCodeGoUsageAutoRefreshExtraKey)
	require.NotContains(t, repo.bulkUpdates[0].Extra, OpenCodeGoUsageSnapshotExtraKey)
	require.Equal(t, "value", repo.bulkUpdates[0].Extra["custom"])
}

// 创建入口不接受伪造的受管键。
func TestCreateAccountDropsOpenCodeGoManagedKeys(t *testing.T) {
	repo := &openCodeGoAdminAccountRepo{}
	svc := &adminServiceImpl{accountRepo: repo}

	created, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Name:                 "opencode",
		Platform:             PlatformOpenAI,
		Type:                 AccountTypeAPIKey,
		Credentials:          map[string]any{"api_key": "sk-1", "base_url": "https://opencode.ai/zen/go/v1"},
		SkipDefaultGroupBind: true,
		Extra: map[string]any{
			OpenCodeGoUsageAutoRefreshExtraKey: true,
			OpenCodeGoUsageSnapshotExtraKey:    map[string]any{"status": "forged"},
			"custom":                           "value",
		},
	})

	require.NoError(t, err)
	require.NotContains(t, created.Extra, OpenCodeGoUsageAutoRefreshExtraKey)
	require.NotContains(t, created.Extra, OpenCodeGoUsageSnapshotExtraKey)
	require.Equal(t, "value", created.Extra["custom"])
}
