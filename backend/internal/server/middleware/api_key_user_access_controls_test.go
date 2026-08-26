//go:build unit

package middleware

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestValidateAPIKeyGroupAllowedRejectsExistingUnauthorizedKey(t *testing.T) {
	t.Parallel()

	groupID := int64(9)
	key := &service.APIKey{
		GroupID: &groupID,
		User: &service.User{
			RestrictToAllowedGroups: true,
			AllowedGroups:           []int64{7},
		},
		Group: &service.Group{
			ID:             groupID,
			IsExclusive:    true,
			Status:         service.StatusActive,
			BalanceEnabled: true,
		},
	}

	require.False(t, validateAPIKeyGroupAllowed(key, false, service.APIKeyAccessSourceBalance))
	require.True(t, validateAPIKeyGroupAllowed(key, true, service.APIKeyAccessSourceEntitlement), "a valid entitlement authorizes an exclusive group without granting balance access")

	key.User.AllowedGroups = append(key.User.AllowedGroups, groupID)
	require.True(t, validateAPIKeyGroupAllowed(key, false, service.APIKeyAccessSourceBalance))

	key.Group.IsExclusive = false
	require.False(t, validateAPIKeyGroupAllowed(key, true, service.APIKeyAccessSourceEntitlement), "public groups remain unusable in exclusive-only mode")
}

func TestValidateAPIKeyGroupAllowedRejectsRestrictedKeyWithoutGroup(t *testing.T) {
	t.Parallel()

	key := &service.APIKey{User: &service.User{RestrictToAllowedGroups: true}}
	require.False(t, validateAPIKeyGroupAllowed(key, false, service.APIKeyAccessSourceBalance))
	require.False(t, validateAPIKeyGroupAllowed(key, true, service.APIKeyAccessSourceBalance))

	key.User.RestrictToAllowedGroups = false
	require.True(t, validateAPIKeyGroupAllowed(key, false, service.APIKeyAccessSourceBalance), "legacy users may keep global no-group keys")
}
