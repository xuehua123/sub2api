package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func newUserRepoSQLite(t *testing.T) (*userRepository, *dbent.Client) {
	t.Helper()

	db, err := sql.Open("sqlite", "file:user_repo_default_chat_api_key?mode=memory&cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	return &userRepository{client: client, sql: db}, client
}

func mustCreateDefaultChatRepoUser(t *testing.T, ctx context.Context, repo *userRepository, email string, defaultChatAPIKeyID *int64) *service.User {
	t.Helper()

	user := &service.User{
		Email:               email,
		PasswordHash:        "test-password-hash",
		Role:                service.RoleUser,
		Status:              service.StatusActive,
		Concurrency:         5,
		DefaultChatAPIKeyID: defaultChatAPIKeyID,
	}
	require.NoError(t, repo.Create(ctx, user))
	return user
}

func mustCreateDefaultChatRepoAPIKey(t *testing.T, ctx context.Context, client *dbent.Client, userID int64) int64 {
	t.Helper()

	keyModel, err := client.APIKey.Create().
		SetUserID(userID).
		SetKey("sk-" + time.Now().Format(time.RFC3339Nano)).
		SetName("default-chat").
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	return keyModel.ID
}

func TestUserRepositoryCreatePersistsDefaultChatAPIKeyID(t *testing.T) {
	repo, client := newUserRepoSQLite(t)
	ctx := context.Background()
	apiKeyHolder := mustCreateDefaultChatRepoUser(t, ctx, repo, "api-owner@test.com", nil)
	defaultKeyID := mustCreateDefaultChatRepoAPIKey(t, ctx, client, apiKeyHolder.ID)

	user := mustCreateDefaultChatRepoUser(t, ctx, repo, "create-default-chat@test.com", &defaultKeyID)

	got, err := repo.GetByID(ctx, user.ID)
	require.NoError(t, err)
	require.NotNil(t, got.DefaultChatAPIKeyID)
	require.Equal(t, defaultKeyID, *got.DefaultChatAPIKeyID)
}

func TestUserRepositoryUpdatePersistsAndClearsDefaultChatAPIKeyID(t *testing.T) {
	repo, client := newUserRepoSQLite(t)
	ctx := context.Background()
	apiKeyHolder := mustCreateDefaultChatRepoUser(t, ctx, repo, "api-owner-update@test.com", nil)
	defaultKeyID := mustCreateDefaultChatRepoAPIKey(t, ctx, client, apiKeyHolder.ID)
	user := mustCreateDefaultChatRepoUser(t, ctx, repo, "update-default-chat@test.com", nil)

	got, err := repo.GetByID(ctx, user.ID)
	require.NoError(t, err)
	got.DefaultChatAPIKeyID = &defaultKeyID
	require.NoError(t, repo.Update(ctx, got))

	updated, err := repo.GetByID(ctx, user.ID)
	require.NoError(t, err)
	require.NotNil(t, updated.DefaultChatAPIKeyID)
	require.Equal(t, defaultKeyID, *updated.DefaultChatAPIKeyID)

	updated.DefaultChatAPIKeyID = nil
	require.NoError(t, repo.Update(ctx, updated))

	cleared, err := repo.GetByID(ctx, user.ID)
	require.NoError(t, err)
	require.Nil(t, cleared.DefaultChatAPIKeyID)
}

func TestUserRepositoryUpdateDefaultChatAPIKeyID(t *testing.T) {
	repo, client := newUserRepoSQLite(t)
	ctx := context.Background()
	apiKeyHolder := mustCreateDefaultChatRepoUser(t, ctx, repo, "api-owner-store@test.com", nil)
	defaultKeyID := mustCreateDefaultChatRepoAPIKey(t, ctx, client, apiKeyHolder.ID)
	user := mustCreateDefaultChatRepoUser(t, ctx, repo, "store-default-chat@test.com", nil)

	require.NoError(t, repo.UpdateDefaultChatAPIKeyID(ctx, user.ID, &defaultKeyID))

	stored, err := repo.GetByID(ctx, user.ID)
	require.NoError(t, err)
	require.NotNil(t, stored.DefaultChatAPIKeyID)
	require.Equal(t, defaultKeyID, *stored.DefaultChatAPIKeyID)

	require.NoError(t, repo.UpdateDefaultChatAPIKeyID(ctx, user.ID, nil))

	cleared, err := repo.GetByID(ctx, user.ID)
	require.NoError(t, err)
	require.Nil(t, cleared.DefaultChatAPIKeyID)
}

func TestUserRepositoryUpdateDefaultChatAPIKeyIDTreatsNonPositiveAsClear(t *testing.T) {
	repo, client := newUserRepoSQLite(t)
	ctx := context.Background()
	apiKeyHolder := mustCreateDefaultChatRepoUser(t, ctx, repo, "api-owner-clear@test.com", nil)
	defaultKeyID := mustCreateDefaultChatRepoAPIKey(t, ctx, client, apiKeyHolder.ID)
	user := mustCreateDefaultChatRepoUser(t, ctx, repo, "clear-default-chat@test.com", &defaultKeyID)

	zero := int64(0)
	require.NoError(t, repo.UpdateDefaultChatAPIKeyID(ctx, user.ID, &zero))

	cleared, err := repo.GetByID(ctx, user.ID)
	require.NoError(t, err)
	require.Nil(t, cleared.DefaultChatAPIKeyID)
}
