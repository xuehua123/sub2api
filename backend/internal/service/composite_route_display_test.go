package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfiguredModelsForDisplayUsesGatewayMatcherAndPreservesEndpointDifferences(t *testing.T) {
	routes := []CompositeModelRoute{
		{ID: 1, GroupID: 80, PublicModel: "alias", MatchType: CompositeRouteMatchExact, TargetPlatform: PlatformOpenAI, UpstreamModel: "gpt-5.5", Endpoint: CompositeRouteEndpointAny, Enabled: true},
		{ID: 2, GroupID: 80, PublicModel: "alias", MatchType: CompositeRouteMatchExact, TargetPlatform: PlatformGrok, UpstreamModel: "grok-4.7", Endpoint: CompositeRouteEndpointResponses, Enabled: true},
		{ID: 3, GroupID: 80, PublicModel: "deepseek-", MatchType: CompositeRouteMatchPrefix, TargetPlatform: PlatformDeepseek, Endpoint: CompositeRouteEndpointAny, Enabled: true},
		{ID: 4, GroupID: 80, PublicModel: "disabled", MatchType: CompositeRouteMatchExact, TargetPlatform: PlatformOpenAI, Endpoint: CompositeRouteEndpointAny},
	}
	r := NewCompositeRouteResolver(compositeRouteRepoStub{routes: routes})
	decisions, err := r.ConfiguredModelsForDisplay(context.Background(), 80, []string{"deepseek-v4.1-flash"})
	require.NoError(t, err)
	require.Len(t, decisions, 3)
	for _, decision := range decisions {
		actual, err := r.Resolve(context.Background(), 80, decision.PublicModel, decision.Endpoint)
		require.NoError(t, err)
		require.Equal(t, actual.TargetPlatform, decision.TargetPlatform)
		require.Equal(t, actual.UpstreamModel, decision.UpstreamModel)
	}
}
