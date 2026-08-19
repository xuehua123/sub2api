package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

// fakeLeaderLockCache is an in-memory LeaderLockCache for unit tests. It models the
// compare-and-delete release semantics of the real Redis-backed implementation.
type fakeLeaderLockCache struct {
	mu         sync.Mutex
	owners     map[string]string
	acquireErr error
}

func (f *fakeLeaderLockCache) TryAcquireLeaderLock(_ context.Context, key, owner string, _ time.Duration) (bool, error) {
	if f.acquireErr != nil {
		return false, f.acquireErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.owners == nil {
		f.owners = map[string]string{}
	}
	if _, held := f.owners[key]; held {
		return false, nil
	}
	f.owners[key] = owner
	return true, nil
}

func (f *fakeLeaderLockCache) ReleaseLeaderLock(_ context.Context, key, owner string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.owners[key] == owner {
		delete(f.owners, key)
	}
	return nil
}

func (f *fakeLeaderLockCache) heldBy(key string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.owners[key]
}

func TestTryAcquireSingletonLeaderLock_NoBackendRunsUngated(t *testing.T) {
	release, ok, err := tryAcquireSingletonLeaderLock(context.Background(), nil, nil, "k", "inst", time.Minute)
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, release)
	require.NotPanics(t, release)
}

func TestTryAcquireSingletonLeaderLock_ContendedThenReleased(t *testing.T) {
	cache := &fakeLeaderLockCache{}
	ctx := context.Background()
	const key = "leader:test:contended"

	releaseA, ok, err := tryAcquireSingletonLeaderLock(ctx, cache, nil, key, "A", time.Minute)
	require.NoError(t, err)
	require.True(t, ok, "first instance should acquire")
	require.Equal(t, "A", cache.heldBy(key))

	_, okB, err := tryAcquireSingletonLeaderLock(ctx, cache, nil, key, "B", time.Minute)
	require.NoError(t, err)
	require.False(t, okB, "peer must be locked out while the lock is held")

	releaseA()
	require.Empty(t, cache.heldBy(key), "release must free the lock")

	releaseB, okB, err := tryAcquireSingletonLeaderLock(ctx, cache, nil, key, "B", time.Minute)
	require.NoError(t, err)
	require.True(t, okB, "peer should acquire after the holder releases")
	releaseB()
}

// When a cache is configured, it is the only lock domain. An acquire error must
// skip the cycle instead of falling through to Postgres or running ungated;
// otherwise a peer still holding the Redis lock can race this instance.
func TestTryAcquireSingletonLeaderLock_CacheErrorDoesNotFallBack(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	redisErr := errors.New("redis unavailable")
	cache := &fakeLeaderLockCache{acquireErr: redisErr}
	release, ok, err := tryAcquireSingletonLeaderLock(context.Background(), cache, db, "k", "inst", time.Minute)
	require.False(t, ok, "configured cache error must skip, not invent another lock domain")
	require.Nil(t, release)
	require.ErrorIs(t, err, redisErr)
	require.NoError(t, mock.ExpectationsWereMet(), "Redis errors must not issue a Postgres advisory-lock query")
}

func TestSubscriptionExpiryService_ReminderSkipsScanWhenNotLeader(t *testing.T) {
	cache := &fakeLeaderLockCache{}
	// A peer already holds the reminder leader lock.
	_, _ = cache.TryAcquireLeaderLock(context.Background(), subscriptionExpiryReminderLeaderLockKey, "peer", time.Minute)

	repo := &subscriptionExpiryRepoStub{}
	settingRepo := &subscriptionExpirySettingRepoStub{values: map[string]string{SettingKeySMTPHost: "smtp.example.com"}}
	svc := NewSubscriptionExpiryService(repo, time.Minute)
	svc.SetSettingRepository(settingRepo)
	svc.SetNotificationEmailService(NewNotificationEmailService(settingRepo, NewEmailService(settingRepo, nil)))
	svc.SetLeaderLock(cache, nil)

	svc.sendExpiryReminders(context.Background())

	require.Zero(t, repo.listCalls, "non-leader must not scan active subscriptions")
}

func TestSubscriptionExpiryService_ReminderScansWhenLeader(t *testing.T) {
	repo := &subscriptionExpiryRepoStub{}
	settingRepo := &subscriptionExpirySettingRepoStub{values: map[string]string{SettingKeySMTPHost: "smtp.example.com"}}
	svc := NewSubscriptionExpiryService(repo, time.Minute)
	svc.SetSettingRepository(settingRepo)
	svc.SetNotificationEmailService(NewNotificationEmailService(settingRepo, NewEmailService(settingRepo, nil)))
	svc.SetLeaderLock(&fakeLeaderLockCache{}, nil)

	svc.sendExpiryReminders(context.Background())

	require.Equal(t, 1, repo.listCalls, "leader should scan active subscriptions once")
}

// Single-instance correctness: the lock is released at the end of each cycle, so
// the same instance must re-acquire it and run on every subsequent cycle (no
// self-lockout). Covers both the cache-backed path and the no-backend path.
func TestSubscriptionExpiryService_ReminderRunsEveryCycleSingleInstance(t *testing.T) {
	cases := map[string]LeaderLockCache{
		"with_cache": &fakeLeaderLockCache{},
		"no_backend": nil,
	}
	for name, cache := range cases {
		t.Run(name, func(t *testing.T) {
			repo := &subscriptionExpiryRepoStub{}
			settingRepo := &subscriptionExpirySettingRepoStub{values: map[string]string{SettingKeySMTPHost: "smtp.example.com"}}
			svc := NewSubscriptionExpiryService(repo, time.Minute)
			svc.SetSettingRepository(settingRepo)
			svc.SetNotificationEmailService(NewNotificationEmailService(settingRepo, NewEmailService(settingRepo, nil)))
			svc.SetLeaderLock(cache, nil)

			// Three consecutive cycles, mimicking the ticker loop.
			svc.sendExpiryReminders(context.Background())
			svc.sendExpiryReminders(context.Background())
			svc.sendExpiryReminders(context.Background())

			require.Equal(t, 3, repo.listCalls, "single instance must run every cycle")
		})
	}
}
