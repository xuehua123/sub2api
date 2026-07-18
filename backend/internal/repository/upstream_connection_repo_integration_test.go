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
