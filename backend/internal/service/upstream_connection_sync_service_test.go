//go:build unit

package service

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type upstreamConnectionSyncLock struct {
	mu         sync.Mutex
	held       bool
	releases   int
	acquireErr error
}

func (l *upstreamConnectionSyncLock) TryAcquireLeaderLock(_ context.Context, _, _ string, _ time.Duration) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.acquireErr != nil {
		return false, l.acquireErr
	}
	if l.held {
		return false, nil
	}
	l.held = true
	return true, nil
}

func (l *upstreamConnectionSyncLock) ReleaseLeaderLock(_ context.Context, _, _ string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.held = false
	l.releases++
	return nil
}

func TestUpstreamConnectionSyncServiceRunsBoundedDueBatchOnLeader(t *testing.T) {
	repo := &upstreamConnectionTestRepo{}
	for id := int64(1); id <= upstreamConnectionSyncMaxConnections+3; id++ {
		repo.dueConnections = append(repo.dueConnections, &UpstreamConnection{ID: id})
	}
	lock := &upstreamConnectionSyncLock{}
	service := NewUpstreamConnectionSyncService(repo, nil, lock, nil)
	var mu sync.Mutex
	var synced []int64
	service.syncConnection = func(_ context.Context, id int64, bindingLimit int) error {
		require.Equal(t, upstreamConnectionSyncMaxBindings, bindingLimit)
		mu.Lock()
		synced = append(synced, id)
		mu.Unlock()
		return nil
	}

	require.NoError(t, service.RunDue(context.Background()))
	sort.Slice(synced, func(left, right int) bool { return synced[left] < synced[right] })
	require.Equal(t, []int64{1, 2, 3, 4, 5}, synced)
	require.Equal(t, 1, lock.releases)
}

func TestUpstreamConnectionSyncServiceSkipsWhenPeerIsLeader(t *testing.T) {
	repo := &upstreamConnectionTestRepo{dueConnections: []*UpstreamConnection{{ID: 1}}}
	lock := &upstreamConnectionSyncLock{held: true}
	service := NewUpstreamConnectionSyncService(repo, nil, lock, nil)
	called := false
	service.syncConnection = func(context.Context, int64, int) error {
		called = true
		return nil
	}

	require.NoError(t, service.RunDue(context.Background()))
	require.False(t, called)
	require.Zero(t, lock.releases)
}

func TestUpstreamConnectionSyncServiceReportsLeaderLockBackendError(t *testing.T) {
	repo := &upstreamConnectionTestRepo{dueConnections: []*UpstreamConnection{{ID: 1}}}
	lockErr := errors.New("redis unavailable")
	lock := &upstreamConnectionSyncLock{acquireErr: lockErr}
	service := NewUpstreamConnectionSyncService(repo, nil, lock, nil)
	called := false
	service.syncConnection = func(context.Context, int64, int) error {
		called = true
		return nil
	}

	err := service.RunDue(context.Background())

	require.ErrorIs(t, err, lockErr)
	require.False(t, called)
	require.Zero(t, lock.releases)
}

func TestUpstreamConnectionSyncLeaderLeaseCoversWorstCaseBatch(t *testing.T) {
	waves := (upstreamConnectionSyncMaxConnections + upstreamConnectionSyncConcurrency - 1) / upstreamConnectionSyncConcurrency
	worstCase := time.Duration(waves) * upstreamConnectionSyncPerConnectionTTL
	require.Greater(t, upstreamConnectionSyncLeaderLockTTL, worstCase)
}
