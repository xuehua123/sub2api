//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type legacyCreateGroupRepoStub struct {
	GroupRepository
	created *Group
}

func (s *legacyCreateGroupRepoStub) ExistsByName(context.Context, string) (bool, error) {
	return false, nil
}

func (s *legacyCreateGroupRepoStub) Create(_ context.Context, group *Group) error {
	s.created = group
	return nil
}

func TestGroupServiceCreateDefaultsLongContextPricingEnabled(t *testing.T) {
	repo := &legacyCreateGroupRepoStub{}
	svc := NewGroupService(repo, nil)

	created, err := svc.Create(context.Background(), CreateGroupRequest{
		Name:           "legacy-create",
		RateMultiplier: 1,
	})

	require.NoError(t, err)
	require.NotNil(t, created)
	require.Same(t, created, repo.created)
	require.True(t, created.LongContextPricingEnabled)
}
