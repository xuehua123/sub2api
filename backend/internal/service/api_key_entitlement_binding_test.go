//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type apiKeyEntitlementRuntimeStub struct {
	runtime SubscriptionEntitlementsRuntime
}

func (s apiKeyEntitlementRuntimeStub) GetSubscriptionEntitlementsRuntime(context.Context) SubscriptionEntitlementsRuntime {
	return s.runtime
}

type apiKeyBindingRepo struct {
	apiKeyRepoStub
	nextID  int64
	keys    map[int64]*APIKey
	created *APIKey
	updated *APIKey
}

func newAPIKeyBindingRepo(keys ...*APIKey) *apiKeyBindingRepo {
	repo := &apiKeyBindingRepo{nextID: 1, keys: make(map[int64]*APIKey)}
	for _, key := range keys {
		cp := cloneAPIKeyForBindingTest(key)
		if cp.ID == 0 {
			cp.ID = repo.nextID
		}
		if cp.ID >= repo.nextID {
			repo.nextID = cp.ID + 1
		}
		repo.keys[cp.ID] = cp
	}
	return repo
}

func (r *apiKeyBindingRepo) Create(_ context.Context, key *APIKey) error {
	if key.ID == 0 {
		key.ID = r.nextID
		r.nextID++
	}
	cp := cloneAPIKeyForBindingTest(key)
	r.keys[key.ID] = cp
	r.created = cloneAPIKeyForBindingTest(key)
	return nil
}

func (r *apiKeyBindingRepo) GetByID(_ context.Context, id int64) (*APIKey, error) {
	key, ok := r.keys[id]
	if !ok {
		return nil, ErrAPIKeyNotFound
	}
	return cloneAPIKeyForBindingTest(key), nil
}

func (r *apiKeyBindingRepo) Update(_ context.Context, key *APIKey) error {
	if _, ok := r.keys[key.ID]; !ok {
		return ErrAPIKeyNotFound
	}
	cp := cloneAPIKeyForBindingTest(key)
	r.keys[key.ID] = cp
	r.updated = cloneAPIKeyForBindingTest(key)
	return nil
}

func (r *apiKeyBindingRepo) ExistsByKey(context.Context, string) (bool, error) {
	return false, nil
}

type apiKeyBindingUserRepo struct {
	userRepoStubForGroupUpdate
	users map[int64]*User
}

func (r *apiKeyBindingUserRepo) GetByID(_ context.Context, id int64) (*User, error) {
	user, ok := r.users[id]
	if !ok {
		return nil, ErrUserNotFound
	}
	cp := *user
	cp.AllowedGroups = append([]int64(nil), user.AllowedGroups...)
	return &cp, nil
}

type apiKeyBindingGroupRepo struct {
	groupRepoStubForGroupUpdate
	groups map[int64]*Group
}

func (r *apiKeyBindingGroupRepo) GetByID(_ context.Context, id int64) (*Group, error) {
	group, ok := r.groups[id]
	if !ok {
		return nil, ErrGroupNotFound
	}
	cp := *group
	return &cp, nil
}

func (r *apiKeyBindingGroupRepo) ListActive(context.Context) ([]Group, error) {
	out := make([]Group, 0, len(r.groups))
	for _, group := range r.groups {
		if group.Status != StatusActive {
			continue
		}
		out = append(out, *group)
	}
	return out, nil
}

type apiKeyBindingUserSubRepo struct {
	userSubRepoNoop
	active map[int64]map[int64]*UserSubscription
	calls  int
}

func (r *apiKeyBindingUserSubRepo) GetActiveByUserIDAndGroupID(_ context.Context, userID, groupID int64) (*UserSubscription, error) {
	r.calls++
	if byGroup := r.active[userID]; byGroup != nil {
		if sub := byGroup[groupID]; sub != nil {
			cp := *sub
			return &cp, nil
		}
	}
	return nil, ErrSubscriptionNotFound
}

func (r *apiKeyBindingUserSubRepo) ListActiveByUserID(_ context.Context, userID int64) ([]UserSubscription, error) {
	out := make([]UserSubscription, 0)
	if byGroup := r.active[userID]; byGroup != nil {
		for _, sub := range byGroup {
			out = append(out, *sub)
		}
	}
	return out, nil
}

func TestAPIKeyService_CreateSubscriptionEntitlementExplicit(t *testing.T) {
	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	svc, apiRepo, _ := newAPIKeyEntitlementBindingFixture(t, now, true,
		testBindingEntitlement(1, 1, now.Add(-time.Hour), now.Add(24*time.Hour), SubscriptionStatusActive, []int64{20}),
	)

	key, err := svc.Create(context.Background(), 1, CreateAPIKeyRequest{
		Name:                      "explicit",
		GroupID:                   cloneInt64PtrValue(20),
		SubscriptionEntitlementID: cloneInt64PtrValue(1),
		CustomKey:                 stringPtrForAPIKeyBindingTest("sk-api-key-binding-explicit"),
	})

	require.NoError(t, err)
	require.NotNil(t, key.SubscriptionEntitlementID)
	require.Equal(t, int64(1), *key.SubscriptionEntitlementID)
	require.NotNil(t, apiRepo.created.SubscriptionEntitlementID)
	require.Equal(t, int64(1), *apiRepo.created.SubscriptionEntitlementID)
}

func TestAPIKeyService_CreateSubscriptionEntitlementDefaultResolve(t *testing.T) {
	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	svc, _, _ := newAPIKeyEntitlementBindingFixture(t, now, true,
		testBindingEntitlement(1, 1, now.Add(-time.Hour), now.Add(72*time.Hour), SubscriptionStatusActive, []int64{20}),
		testBindingEntitlement(2, 1, now.Add(-time.Hour), now.Add(24*time.Hour), SubscriptionStatusActive, []int64{20}),
	)

	key, err := svc.Create(context.Background(), 1, CreateAPIKeyRequest{
		Name:      "default",
		GroupID:   cloneInt64PtrValue(20),
		CustomKey: stringPtrForAPIKeyBindingTest("sk-api-key-binding-default"),
	})

	require.NoError(t, err)
	require.NotNil(t, key.SubscriptionEntitlementID)
	require.Equal(t, int64(2), *key.SubscriptionEntitlementID)
}

func TestAPIKeyService_CreateSubscriptionEntitlementRejectsInvalidExplicitBinding(t *testing.T) {
	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name        string
		entitlement *SubscriptionEntitlement
	}{
		{
			name:        "expired",
			entitlement: testBindingEntitlement(1, 1, now.Add(-48*time.Hour), now.Add(-time.Hour), SubscriptionStatusActive, []int64{20}),
		},
		{
			name:        "future",
			entitlement: testBindingEntitlement(1, 1, now.Add(time.Hour), now.Add(48*time.Hour), SubscriptionStatusActive, []int64{20}),
		},
		{
			name:        "not-owned",
			entitlement: testBindingEntitlement(1, 2, now.Add(-time.Hour), now.Add(48*time.Hour), SubscriptionStatusActive, []int64{20}),
		},
		{
			name:        "not-covering",
			entitlement: testBindingEntitlement(1, 1, now.Add(-time.Hour), now.Add(48*time.Hour), SubscriptionStatusActive, []int64{30}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, apiRepo, _ := newAPIKeyEntitlementBindingFixture(t, now, true, tt.entitlement)

			_, err := svc.Create(context.Background(), 1, CreateAPIKeyRequest{
				Name:                      tt.name,
				GroupID:                   cloneInt64PtrValue(20),
				SubscriptionEntitlementID: cloneInt64PtrValue(1),
				CustomKey:                 stringPtrForAPIKeyBindingTest("sk-api-key-binding-invalid-" + tt.name),
			})

			require.Error(t, err)
			require.Nil(t, apiRepo.created)
		})
	}
}

func TestAPIKeyService_CreateSubscriptionEntitlementFlagOffKeepsLegacy(t *testing.T) {
	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	svc, _, userSubRepo := newAPIKeyEntitlementBindingFixture(t, now, false)
	userSubRepo.active[1] = map[int64]*UserSubscription{
		20: {ID: 100, UserID: 1, GroupID: 20, Status: SubscriptionStatusActive, ExpiresAt: now.Add(24 * time.Hour)},
	}

	key, err := svc.Create(context.Background(), 1, CreateAPIKeyRequest{
		Name:                      "legacy",
		GroupID:                   cloneInt64PtrValue(20),
		SubscriptionEntitlementID: cloneInt64PtrValue(999),
		CustomKey:                 stringPtrForAPIKeyBindingTest("sk-api-key-binding-legacy"),
	})

	require.NoError(t, err)
	require.Nil(t, key.SubscriptionEntitlementID)
	require.GreaterOrEqual(t, userSubRepo.calls, 1)
}

func TestAPIKeyService_UpdateSubscriptionEntitlementBindings(t *testing.T) {
	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)

	t.Run("same entitlement covers new group", func(t *testing.T) {
		existing := &APIKey{ID: 10, UserID: 1, Key: "sk-existing", Name: "existing", GroupID: cloneInt64PtrValue(20), SubscriptionEntitlementID: cloneInt64PtrValue(1), Status: StatusActive}
		svc, apiRepo, _ := newAPIKeyEntitlementBindingFixtureWithKeys(t, now, true, []*APIKey{existing},
			testBindingEntitlement(1, 1, now.Add(-time.Hour), now.Add(24*time.Hour), SubscriptionStatusActive, []int64{20, 30}),
		)

		updated, err := svc.Update(context.Background(), 10, 1, UpdateAPIKeyRequest{GroupID: cloneInt64PtrValue(30)})

		require.NoError(t, err)
		require.NotNil(t, updated.SubscriptionEntitlementID)
		require.Equal(t, int64(1), *updated.SubscriptionEntitlementID)
		require.Equal(t, int64(1), *apiRepo.updated.SubscriptionEntitlementID)
	})

	t.Run("re-resolves when current entitlement does not cover new group", func(t *testing.T) {
		existing := &APIKey{ID: 11, UserID: 1, Key: "sk-existing-2", Name: "existing", GroupID: cloneInt64PtrValue(20), SubscriptionEntitlementID: cloneInt64PtrValue(1), Status: StatusActive}
		svc, _, _ := newAPIKeyEntitlementBindingFixtureWithKeys(t, now, true, []*APIKey{existing},
			testBindingEntitlement(1, 1, now.Add(-time.Hour), now.Add(24*time.Hour), SubscriptionStatusActive, []int64{20}),
			testBindingEntitlement(2, 1, now.Add(-time.Hour), now.Add(24*time.Hour), SubscriptionStatusActive, []int64{30}),
		)

		updated, err := svc.Update(context.Background(), 11, 1, UpdateAPIKeyRequest{GroupID: cloneInt64PtrValue(30)})

		require.NoError(t, err)
		require.NotNil(t, updated.SubscriptionEntitlementID)
		require.Equal(t, int64(2), *updated.SubscriptionEntitlementID)
	})

	t.Run("standard group clears entitlement", func(t *testing.T) {
		existing := &APIKey{ID: 12, UserID: 1, Key: "sk-existing-3", Name: "existing", GroupID: cloneInt64PtrValue(20), SubscriptionEntitlementID: cloneInt64PtrValue(1), Status: StatusActive}
		svc, _, _ := newAPIKeyEntitlementBindingFixtureWithKeys(t, now, true, []*APIKey{existing},
			testBindingEntitlement(1, 1, now.Add(-time.Hour), now.Add(24*time.Hour), SubscriptionStatusActive, []int64{20}),
		)

		updated, err := svc.Update(context.Background(), 12, 1, UpdateAPIKeyRequest{GroupID: cloneInt64PtrValue(10)})

		require.NoError(t, err)
		require.Nil(t, updated.SubscriptionEntitlementID)
	})

	t.Run("explicit mismatch rejects", func(t *testing.T) {
		existing := &APIKey{ID: 13, UserID: 1, Key: "sk-existing-4", Name: "existing", GroupID: cloneInt64PtrValue(20), SubscriptionEntitlementID: cloneInt64PtrValue(1), Status: StatusActive}
		svc, apiRepo, _ := newAPIKeyEntitlementBindingFixtureWithKeys(t, now, true, []*APIKey{existing},
			testBindingEntitlement(1, 1, now.Add(-time.Hour), now.Add(24*time.Hour), SubscriptionStatusActive, []int64{20}),
			testBindingEntitlement(2, 1, now.Add(-time.Hour), now.Add(24*time.Hour), SubscriptionStatusActive, []int64{30}),
		)

		_, err := svc.Update(context.Background(), 13, 1, UpdateAPIKeyRequest{
			GroupID:                      cloneInt64PtrValue(30),
			SubscriptionEntitlementID:    cloneInt64PtrValue(1),
			SubscriptionEntitlementIDSet: true,
		})

		require.Error(t, err)
		require.Nil(t, apiRepo.updated)
	})
}

func newAPIKeyEntitlementBindingFixture(t *testing.T, now time.Time, v2Enabled bool, entitlements ...*SubscriptionEntitlement) (*APIKeyService, *apiKeyBindingRepo, *apiKeyBindingUserSubRepo) {
	t.Helper()
	return newAPIKeyEntitlementBindingFixtureWithKeys(t, now, v2Enabled, nil, entitlements...)
}

func newAPIKeyEntitlementBindingFixtureWithKeys(t *testing.T, now time.Time, v2Enabled bool, keys []*APIKey, entitlements ...*SubscriptionEntitlement) (*APIKeyService, *apiKeyBindingRepo, *apiKeyBindingUserSubRepo) {
	t.Helper()
	apiRepo := newAPIKeyBindingRepo(keys...)
	userSubRepo := &apiKeyBindingUserSubRepo{active: make(map[int64]map[int64]*UserSubscription)}
	userRepo := &apiKeyBindingUserRepo{users: map[int64]*User{
		1: {ID: 1, Email: "user@test.local", Status: StatusActive},
	}}
	groupRepo := &apiKeyBindingGroupRepo{groups: map[int64]*Group{
		10: {ID: 10, Name: "standard", Status: StatusActive, SubscriptionType: SubscriptionTypeStandard},
		20: {ID: 20, Name: "subscription-a", Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription},
		30: {ID: 30, Name: "subscription-b", Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription},
	}}
	entRepo := newFakeSubscriptionEntitlementRepo(now)
	for _, ent := range entitlements {
		entRepo.entitlements[ent.ID] = cloneTestEntitlement(ent)
		if ent.ID >= entRepo.nextID {
			entRepo.nextID = ent.ID + 1
		}
	}
	entSvc := NewSubscriptionEntitlementService(entRepo, nil)
	entSvc.SetNowFunc(func() time.Time { return now })

	svc := NewAPIKeyService(apiRepo, userRepo, groupRepo, userSubRepo, nil, nil, &config.Config{})
	svc.SetSubscriptionEntitlementDependencies(apiKeyEntitlementRuntimeStub{runtime: SubscriptionEntitlementsRuntime{Enabled: v2Enabled}}, entSvc)
	return svc, apiRepo, userSubRepo
}

func testBindingEntitlement(id, userID int64, startsAt, expiresAt time.Time, status string, groupIDs []int64) *SubscriptionEntitlement {
	return &SubscriptionEntitlement{
		ID:          id,
		UserID:      userID,
		Name:        "entitlement",
		Status:      status,
		StartsAt:    startsAt,
		ExpiresAt:   expiresAt,
		GroupGrants: testGroupGrants(groupIDs),
		Groups:      testGroups(groupIDs),
	}
}

func cloneAPIKeyForBindingTest(key *APIKey) *APIKey {
	if key == nil {
		return nil
	}
	cp := *key
	cp.GroupID = cloneInt64Ptr(key.GroupID)
	cp.SubscriptionEntitlementID = cloneInt64Ptr(key.SubscriptionEntitlementID)
	return &cp
}

func stringPtrForAPIKeyBindingTest(v string) *string {
	return &v
}
