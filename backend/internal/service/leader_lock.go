package service

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

// LeaderLockCache provides cross-instance mutual exclusion for periodic background
// jobs. It is implemented in the repository layer (Redis-backed) so the service
// layer never depends on Redis directly. Release is a compare-and-delete keyed by
// owner so a stale holder can never delete a peer's lock.
type LeaderLockCache interface {
	// TryAcquireLeaderLock sets key=owner with the given TTL iff key is absent.
	// It returns true when the caller becomes the owner.
	TryAcquireLeaderLock(ctx context.Context, key, owner string, ttl time.Duration) (bool, error)
	// ReleaseLeaderLock deletes key iff it is still owned by owner.
	ReleaseLeaderLock(ctx context.Context, key, owner string) error
}

// tryAcquireSingletonLeaderLock provides single-flight execution of a periodic
// background job across multiple instances. Exactly one lock domain is
// used per call:
//   - if a LeaderLockCache is configured, Redis is the only domain;
//   - otherwise a Postgres advisory lock is used when a DB is configured;
//   - otherwise the job runs ungated (unit tests / single-instance).
//
// Redis errors must not fall through to Postgres. The two backends cannot see
// each other's holders, so a peer that still holds the Redis lock would race
// a second instance that acquired the advisory lock — including concurrent
// backup/restore of the same database.
//
// Semantics:
//   - acquired      -> returns a non-nil release func, true and nil; callers should
//     defer the release once the job finishes.
//   - held by peer  -> returns (nil, false, nil); callers should skip or wait.
//   - backend error -> returns (nil, false, err); callers must surface/log it and
//     must not invent a second lock domain.
//   - no backend    -> when neither the cache nor a DB is configured it runs
//     without gating, returning a no-op release, true and nil.
//
// The TTL is purely a crash-safety bound: callers release the lock as soon as the
// job completes, so leadership is re-contested every cycle rather than pinned to
// one instance. The TTL must therefore be larger than the job's worst-case
// runtime so the lock does not expire mid-run.
func tryAcquireSingletonLeaderLock(ctx context.Context, cache LeaderLockCache, db *sql.DB, key, owner string, ttl time.Duration) (func(), bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	if cache != nil {
		ok, err := cache.TryAcquireLeaderLock(ctx, key, owner, ttl)
		if err != nil {
			return nil, false, fmt.Errorf("acquire redis leader lock %q: %w", key, err)
		}
		if !ok {
			return nil, false, nil
		}
		release := func() {
			ctx2, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := cache.ReleaseLeaderLock(ctx2, key, owner); err != nil {
				slog.Warn("failed to release distributed leader lock", "key", key, "error", err)
			}
		}
		return release, true, nil
	}

	if db != nil {
		return tryAcquireDBAdvisoryLockWithError(ctx, db, hashAdvisoryLockID(key))
	}

	// No coordination backend available: run without gating.
	return func() {}, true, nil
}
