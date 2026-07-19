//go:build integration

package repository

import (
	"context"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
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

	applied, err := repo.UpdateIfVersion(txCtx, loaded, 1, true)
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
