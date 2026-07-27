//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/ent/upstreamaccountbinding"
	"github.com/Wei-Shaw/sub2api/ent/upstreamconnection"
	"github.com/Wei-Shaw/sub2api/ent/upstreamgroup"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUpstreamConnectionRepositoryIdentityResetClearsGroups(t *testing.T) {
	ctx := context.Background()
	tx, err := integrationEntClient.Tx(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	txCtx := dbent.NewTxContext(ctx, tx)

	repo := NewUpstreamConnectionRepository(integrationEntClient)
	connection := &service.UpstreamConnection{
		Name: "identity reset", Provider: service.UpstreamConnectionProviderNewAPI,
		AuthMode: "password", ManagementBaseURL: "https://old.example.com",
		CredentialEncrypted: "encrypted", CredentialFingerprint: "sha256:v1:test",
		Capabilities: map[string]any{"groups": true}, Status: service.UpstreamConnectionStatusReady,
		SyncEnabled: true, SyncIntervalSeconds: 300, Version: 1,
		WalletReliability: "unknown", WalletRaw: map[string]any{},
	}
	require.NoError(t, repo.Create(txCtx, connection))
	_, err = tx.Client().UpstreamGroup.Create().
		SetConnectionID(connection.ID).
		SetName("stale-group").
		SetRateMultiplier(0.5).
		Save(txCtx)
	require.NoError(t, err)

	loaded, err := repo.GetByID(txCtx, connection.ID)
	require.NoError(t, err)
	require.Len(t, loaded.Groups, 1)
	loaded.ManagementBaseURL = "https://new.example.com"
	loaded.Version = 2

	applied, err := repo.UpdateIfVersion(txCtx, loaded, 1, true, true, true, false)
	require.NoError(t, err)
	require.True(t, applied)
	count, err := tx.Client().UpstreamGroup.Query().
		Where(upstreamgroup.ConnectionIDEQ(connection.ID)).
		Count(txCtx)
	require.NoError(t, err)
	require.Zero(t, count)
}

func TestUpstreamConnectionRepositoryListIncludesBoundAccountIDsWithoutBindingDetails(t *testing.T) {
	ctx := context.Background()
	tx, err := integrationEntClient.Tx(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	txCtx := dbent.NewTxContext(ctx, tx)

	account, err := tx.Client().Account.Create().
		SetName("bound account").
		SetPlatform("openai").
		SetType(service.AccountTypeAPIKey).
		Save(txCtx)
	require.NoError(t, err)

	repo := NewUpstreamConnectionRepository(integrationEntClient)
	connection := &service.UpstreamConnection{
		Name: "shared wallet", Provider: service.UpstreamConnectionProviderSub2API,
		AuthMode: "access_token", ManagementBaseURL: "https://upstream.example.com",
		CredentialEncrypted: "encrypted", CredentialFingerprint: "sha256:v1:test",
		Capabilities: map[string]any{}, Status: service.UpstreamConnectionStatusReady,
		SyncEnabled: true, SyncIntervalSeconds: 300, Version: 1,
		WalletReliability: "exact", WalletRaw: map[string]any{},
	}
	require.NoError(t, repo.Create(txCtx, connection))
	_, err = tx.Client().UpstreamAccountBinding.Create().
		SetAccountID(account.ID).
		SetConnectionID(connection.ID).
		Save(txCtx)
	require.NoError(t, err)

	items, total, err := repo.List(txCtx, service.UpstreamConnectionListParams{Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	require.Equal(t, []int64{account.ID}, items[0].BoundAccountIDs)
	require.Empty(t, items[0].Bindings)
}

func TestUpstreamConnectionRepositoryListIncludesBindingDetailsWhenRequested(t *testing.T) {
	ctx := context.Background()
	tx, err := integrationEntClient.Tx(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	txCtx := dbent.NewTxContext(ctx, tx)

	account, err := tx.Client().Account.Create().
		SetName("bound account details").
		SetPlatform("openai").
		SetType(service.AccountTypeAPIKey).
		Save(txCtx)
	require.NoError(t, err)

	repo := NewUpstreamConnectionRepository(integrationEntClient)
	connection := &service.UpstreamConnection{
		Name: "shared wallet details", Provider: service.UpstreamConnectionProviderSub2API,
		AuthMode: "access_token", ManagementBaseURL: "https://upstream.example.com",
		CredentialEncrypted: "encrypted", CredentialFingerprint: "sha256:v1:test",
		Capabilities: map[string]any{}, Status: service.UpstreamConnectionStatusReady,
		SyncEnabled: true, SyncIntervalSeconds: 300, Version: 1,
		WalletReliability: "exact", WalletRaw: map[string]any{},
	}
	require.NoError(t, repo.Create(txCtx, connection))
	_, err = tx.Client().UpstreamAccountBinding.Create().
		SetAccountID(account.ID).
		SetConnectionID(connection.ID).
		SetStatus(service.UpstreamBindingStatusReady).
		SetRemoteGroupName("pro").
		SetObservedMultiplier(0.08).
		Save(txCtx)
	require.NoError(t, err)

	items, total, err := repo.List(txCtx, service.UpstreamConnectionListParams{Page: 1, PageSize: 20, IncludeBindings: true})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	require.Len(t, items[0].Bindings, 1)
	require.Equal(t, account.ID, items[0].Bindings[0].AccountID)
	require.Equal(t, "pro", items[0].Bindings[0].RemoteGroupName)
	require.Equal(t, 0.08, *items[0].Bindings[0].ObservedMultiplier)
}

func TestUpstreamConnectionRepositoryBindingSyncsAccountRateMultiplier(t *testing.T) {
	ctx := context.Background()
	tx, err := integrationEntClient.Tx(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	txCtx := dbent.NewTxContext(ctx, tx)

	account, err := tx.Client().Account.Create().
		SetName("auto multiplier account").
		SetPlatform("openai").
		SetType(service.AccountTypeAPIKey).
		SetRateMultiplier(1.5).
		Save(txCtx)
	require.NoError(t, err)

	repo := NewUpstreamConnectionRepository(integrationEntClient)
	connection := &service.UpstreamConnection{
		Name: "auto multiplier upstream", Provider: service.UpstreamConnectionProviderSub2API,
		AuthMode: "access_token", ManagementBaseURL: "https://upstream.example.com",
		CredentialEncrypted: "encrypted", CredentialFingerprint: "sha256:v1:auto-rate",
		Capabilities: map[string]any{}, Status: service.UpstreamConnectionStatusReady,
		SyncEnabled: true, SyncIntervalSeconds: 300, Version: 1,
		WalletReliability: "exact", WalletRaw: map[string]any{},
	}
	require.NoError(t, repo.Create(txCtx, connection))

	rate := 0.25
	binding := &service.UpstreamAccountBinding{
		AccountID: account.ID, ConnectionID: connection.ID,
		ResolutionKind:  service.UpstreamBindingResolutionFixed,
		RemoteGroupName: "vip", ObservedMultiplier: &rate,
		Confidence: "exact", ApplyPolicy: service.UpstreamBindingApplyAuto,
		Status:         service.UpstreamBindingStatusReady,
		FallbackGroups: []string{}, ResolutionDetails: map[string]any{},
	}
	applied, err := repo.UpsertAccountBindingIfCurrent(txCtx, binding, connection.Version, &rate)
	require.NoError(t, err)
	require.True(t, applied)

	updatedAccount, err := tx.Client().Account.Get(txCtx, account.ID)
	require.NoError(t, err)
	require.Equal(t, 0.25, updatedAccount.RateMultiplier)

	staleRate := 0.05
	binding.ObservedMultiplier = &staleRate
	applied, err = repo.UpdateAccountBindingIfCurrent(txCtx, binding, connection.ID, connection.Version+1, &staleRate)
	require.NoError(t, err)
	require.False(t, applied)
	updatedAccount, err = tx.Client().Account.Get(txCtx, account.ID)
	require.NoError(t, err)
	require.Equal(t, 0.25, updatedAccount.RateMultiplier)
}

func TestUpstreamConnectionRepositoryDeleteCleansSoftDeletedAccountBindings(t *testing.T) {
	ctx := context.Background()
	tx, err := integrationEntClient.Tx(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	txCtx := dbent.NewTxContext(ctx, tx)

	deletedAt := time.Now().UTC()
	account, err := tx.Client().Account.Create().
		SetName("soft-deleted upstream binding").
		SetPlatform("openai").
		SetType(service.AccountTypeAPIKey).
		SetDeletedAt(deletedAt).
		Save(txCtx)
	require.NoError(t, err)

	repo := NewUpstreamConnectionRepository(integrationEntClient)
	connection := mustCreateUpstreamConnectionForDelete(t, txCtx, repo, "soft-delete cleanup")
	_, err = tx.Client().UpstreamAccountBinding.Create().
		SetAccountID(account.ID).
		SetConnectionID(connection.ID).
		Save(txCtx)
	require.NoError(t, err)

	require.NoError(t, repo.Delete(txCtx, connection.ID, service.UpstreamConnectionDeleteParams{}))
	connectionExists, err := tx.Client().UpstreamConnection.Query().
		Where(upstreamconnection.IDEQ(connection.ID)).
		Exist(txCtx)
	require.NoError(t, err)
	require.False(t, connectionExists)
	bindingCount, err := tx.Client().UpstreamAccountBinding.Query().
		Where(upstreamaccountbinding.AccountIDEQ(account.ID)).
		Count(txCtx)
	require.NoError(t, err)
	require.Zero(t, bindingCount)
	_, err = tx.Client().Account.Get(mixins.SkipSoftDelete(txCtx), account.ID)
	require.NoError(t, err)
}

func TestUpstreamConnectionRepositoryDeleteRequiresExplicitUnbindForActiveAccounts(t *testing.T) {
	ctx := context.Background()
	tx, err := integrationEntClient.Tx(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	txCtx := dbent.NewTxContext(ctx, tx)

	account, err := tx.Client().Account.Create().
		SetName("active upstream binding").
		SetPlatform("openai").
		SetType(service.AccountTypeAPIKey).
		Save(txCtx)
	require.NoError(t, err)

	repo := NewUpstreamConnectionRepository(integrationEntClient)
	connection := mustCreateUpstreamConnectionForDelete(t, txCtx, repo, "active-binding cleanup")
	_, err = tx.Client().UpstreamAccountBinding.Create().
		SetAccountID(account.ID).
		SetConnectionID(connection.ID).
		Save(txCtx)
	require.NoError(t, err)
	softDeletedAt := time.Now().UTC()
	softDeletedAccount, err := tx.Client().Account.Create().
		SetName("hidden binding beside active account").
		SetPlatform("openai").
		SetType(service.AccountTypeAPIKey).
		SetDeletedAt(softDeletedAt).
		Save(txCtx)
	require.NoError(t, err)
	_, err = tx.Client().UpstreamAccountBinding.Create().
		SetAccountID(softDeletedAccount.ID).
		SetConnectionID(connection.ID).
		Save(txCtx)
	require.NoError(t, err)

	err = repo.Delete(txCtx, connection.ID, service.UpstreamConnectionDeleteParams{})
	require.ErrorIs(t, err, service.ErrUpstreamConnectionInUse)
	err = repo.Delete(txCtx, connection.ID, service.UpstreamConnectionDeleteParams{UnbindAccounts: true})
	require.ErrorIs(t, err, service.ErrUpstreamConnectionConfirmationRequired)
	bindingCountBeforeConfirmation, err := tx.Client().UpstreamAccountBinding.Query().
		Where(upstreamaccountbinding.ConnectionIDEQ(connection.ID)).
		Count(txCtx)
	require.NoError(t, err)
	require.Equal(t, 2, bindingCountBeforeConfirmation)

	require.NoError(t, repo.Delete(txCtx, connection.ID, service.UpstreamConnectionDeleteParams{
		UnbindAccounts:             true,
		HasExpectedBoundAccountIDs: true,
		ExpectedBoundAccountIDs:    []int64{account.ID},
	}))
	connectionExists, err := tx.Client().UpstreamConnection.Query().
		Where(upstreamconnection.IDEQ(connection.ID)).
		Exist(txCtx)
	require.NoError(t, err)
	require.False(t, connectionExists)
	bindingCount, err := tx.Client().UpstreamAccountBinding.Query().
		Where(upstreamaccountbinding.AccountIDEQ(account.ID)).
		Count(txCtx)
	require.NoError(t, err)
	require.Zero(t, bindingCount)
	_, err = tx.Client().Account.Get(txCtx, account.ID)
	require.NoError(t, err)
}

func TestUpstreamConnectionRepositoryDeleteRejectsChangedActiveBindingSet(t *testing.T) {
	ctx := context.Background()
	tx, err := integrationEntClient.Tx(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	txCtx := dbent.NewTxContext(ctx, tx)

	firstAccount, err := tx.Client().Account.Create().
		SetName("first active upstream binding").
		SetPlatform("openai").
		SetType(service.AccountTypeAPIKey).
		Save(txCtx)
	require.NoError(t, err)
	secondAccount, err := tx.Client().Account.Create().
		SetName("second active upstream binding").
		SetPlatform("openai").
		SetType(service.AccountTypeAPIKey).
		Save(txCtx)
	require.NoError(t, err)

	repo := NewUpstreamConnectionRepository(integrationEntClient)
	connection := mustCreateUpstreamConnectionForDelete(t, txCtx, repo, "stale binding confirmation")
	for _, accountID := range []int64{firstAccount.ID, secondAccount.ID} {
		_, err = tx.Client().UpstreamAccountBinding.Create().
			SetAccountID(accountID).
			SetConnectionID(connection.ID).
			Save(txCtx)
		require.NoError(t, err)
	}

	err = repo.Delete(txCtx, connection.ID, service.UpstreamConnectionDeleteParams{
		UnbindAccounts:             true,
		HasExpectedBoundAccountIDs: true,
		ExpectedBoundAccountIDs:    []int64{firstAccount.ID},
	})
	require.ErrorIs(t, err, service.ErrUpstreamConnectionBindingsChanged)

	connectionExists, err := tx.Client().UpstreamConnection.Query().
		Where(upstreamconnection.IDEQ(connection.ID)).
		Exist(txCtx)
	require.NoError(t, err)
	require.True(t, connectionExists)
	bindingCount, err := tx.Client().UpstreamAccountBinding.Query().
		Where(upstreamaccountbinding.ConnectionIDEQ(connection.ID)).
		Count(txCtx)
	require.NoError(t, err)
	require.Equal(t, 2, bindingCount)
}

func TestUpstreamConnectionRepositoryExcludesSoftDeletedBindingsFromVisibleAndDueLists(t *testing.T) {
	ctx := context.Background()
	tx, err := integrationEntClient.Tx(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	txCtx := dbent.NewTxContext(ctx, tx)

	deletedAt := time.Now().UTC()
	account, err := tx.Client().Account.Create().
		SetName("hidden upstream binding").
		SetPlatform("openai").
		SetType(service.AccountTypeAPIKey).
		SetDeletedAt(deletedAt).
		Save(txCtx)
	require.NoError(t, err)

	repo := NewUpstreamConnectionRepository(integrationEntClient)
	connection := mustCreateUpstreamConnectionForDelete(t, txCtx, repo, "hidden binding list")
	_, err = tx.Client().UpstreamAccountBinding.Create().
		SetAccountID(account.ID).
		SetConnectionID(connection.ID).
		Save(txCtx)
	require.NoError(t, err)

	loaded, err := repo.GetByID(txCtx, connection.ID)
	require.NoError(t, err)
	require.Zero(t, loaded.BindingCount)
	require.Empty(t, loaded.BoundAccountIDs)
	require.Empty(t, loaded.Bindings)

	items, _, err := repo.List(txCtx, service.UpstreamConnectionListParams{Page: 1, PageSize: 100, IncludeBindings: true})
	require.NoError(t, err)
	var listed *service.UpstreamConnection
	for _, item := range items {
		if item.ID == connection.ID {
			listed = item
			break
		}
	}
	require.NotNil(t, listed)
	require.Zero(t, listed.BindingCount)
	require.Empty(t, listed.BoundAccountIDs)
	require.Empty(t, listed.Bindings)

	due, err := repo.ListDueAccountBindings(txCtx, connection.ID, time.Now().UTC(), 10)
	require.NoError(t, err)
	require.Empty(t, due)
}

func mustCreateUpstreamConnectionForDelete(
	t *testing.T,
	ctx context.Context,
	repo service.UpstreamConnectionRepository,
	name string,
) *service.UpstreamConnection {
	t.Helper()
	connection := &service.UpstreamConnection{
		Name: name, Provider: service.UpstreamConnectionProviderSub2API,
		AuthMode: "access_token", ManagementBaseURL: "https://upstream.example.com",
		CredentialEncrypted: "encrypted", CredentialFingerprint: "sha256:v1:delete-test",
		Capabilities: map[string]any{}, Status: service.UpstreamConnectionStatusReady,
		SyncEnabled: true, SyncIntervalSeconds: 300, Version: 1,
		WalletReliability: "unknown", WalletRaw: map[string]any{},
	}
	require.NoError(t, repo.Create(ctx, connection))
	return connection
}
