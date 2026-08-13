package service

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

const (
	upstreamConnectionSyncCycleInterval = 30 * time.Second
	upstreamConnectionSyncLeaderLockKey = "upstream:connections:v2:sync:leader"
	// Five connections at concurrency two can occupy three two-minute waves.
	// Keep the singleton lease beyond that bound so another node cannot start
	// the same due batch while the first leader is still finishing it.
	upstreamConnectionSyncLeaderLockTTL    = 7 * time.Minute
	upstreamConnectionSyncMaxConnections   = 5
	upstreamConnectionSyncMaxBindings      = 20
	upstreamConnectionSyncConcurrency      = 2
	upstreamConnectionSyncPerConnectionTTL = 2 * time.Minute
)

// UpstreamConnectionSyncService schedules refreshes for the V2 shared
// connection tables. Legacy account-local probes retain their own runner during
// the compatibility window.
type UpstreamConnectionSyncService struct {
	repo              UpstreamConnectionRepository
	connectionService *UpstreamConnectionService
	lockCache         LeaderLockCache
	db                *sql.DB
	instanceID        string
	now               func() time.Time
	syncConnection    func(context.Context, int64, int) error

	parentCtx    context.Context
	parentCancel context.CancelFunc
	wg           sync.WaitGroup
	mu           sync.Mutex
	cycleMu      sync.Mutex
	started      bool
	stopped      bool
}

func NewUpstreamConnectionSyncService(
	repo UpstreamConnectionRepository,
	connectionService *UpstreamConnectionService,
	lockCache LeaderLockCache,
	db *sql.DB,
) *UpstreamConnectionSyncService {
	ctx, cancel := context.WithCancel(context.Background())
	service := &UpstreamConnectionSyncService{
		repo: repo, connectionService: connectionService, lockCache: lockCache, db: db,
		instanceID: uuid.NewString(), now: time.Now, parentCtx: ctx, parentCancel: cancel,
	}
	if connectionService != nil {
		service.syncConnection = connectionService.SyncConnection
	}
	return service
}

func ProvideUpstreamConnectionSyncService(
	repo UpstreamConnectionRepository,
	connectionService *UpstreamConnectionService,
	lockCache LeaderLockCache,
	db *sql.DB,
) *UpstreamConnectionSyncService {
	service := NewUpstreamConnectionSyncService(repo, connectionService, lockCache, db)
	service.Start()
	return service
}

func (s *UpstreamConnectionSyncService) Start() {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.started || s.stopped {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.wg.Add(1)
	s.mu.Unlock()
	go s.runLoop()
}

func (s *UpstreamConnectionSyncService) Stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	s.parentCancel()
	s.mu.Unlock()
	s.wg.Wait()
}

func (s *UpstreamConnectionSyncService) runLoop() {
	defer s.wg.Done()
	if err := s.RunDue(s.parentCtx); err != nil {
		logger.LegacyPrintf("service.upstream_connection_sync", "initial_sync_failed: err=%v", err)
	}
	ticker := time.NewTicker(upstreamConnectionSyncCycleInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.parentCtx.Done():
			return
		case <-ticker.C:
			if err := s.RunDue(s.parentCtx); err != nil {
				logger.LegacyPrintf("service.upstream_connection_sync", "sync_due_failed: err=%v", err)
			}
		}
	}
}

func (s *UpstreamConnectionSyncService) RunDue(ctx context.Context) error {
	if s == nil || s.repo == nil || s.syncConnection == nil {
		return nil
	}
	s.cycleMu.Lock()
	defer s.cycleMu.Unlock()

	release, acquired, err := tryAcquireSingletonLeaderLock(
		ctx, s.lockCache, s.db, upstreamConnectionSyncLeaderLockKey,
		s.instanceID, upstreamConnectionSyncLeaderLockTTL,
	)
	if err != nil {
		return fmt.Errorf("acquire upstream connection sync leader lock: %w", err)
	}
	if !acquired {
		return nil
	}
	defer release()

	connections, err := s.repo.ListDueConnections(ctx, s.now().UTC(), upstreamConnectionSyncMaxConnections)
	if err != nil {
		return err
	}
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(upstreamConnectionSyncConcurrency)
	for _, connection := range connections {
		if connection == nil {
			continue
		}
		connectionID := connection.ID
		group.Go(func() error {
			connectionCtx, cancel := context.WithTimeout(groupCtx, upstreamConnectionSyncPerConnectionTTL)
			defer cancel()
			if err := s.syncConnection(connectionCtx, connectionID, upstreamConnectionSyncMaxBindings); err != nil {
				logger.LegacyPrintf("service.upstream_connection_sync", "connection_sync_failed: connection_id=%d err=%v", connectionID, err)
			}
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return fmt.Errorf("wait for upstream connection sync: %w", err)
	}
	return nil
}
