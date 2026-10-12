package service

import (
	"context"
	"sort"
	"strings"
)

// ConfiguredModelsForDisplay reads routes once and uses the gateway's matcher.
// Endpoint-specific routes can give one public ID different billing targets;
// return every distinct projection so presentation can decline a single price.
func (r *CompositeRouteResolver) ConfiguredModelsForDisplay(ctx context.Context, groupID int64, existingModels []string) ([]CompositeRouteDecision, error) {
	if r == nil || r.repo == nil || groupID <= 0 {
		return nil, nil
	}
	routes, err := r.repo.ListByGroup(ctx, groupID, false)
	if err != nil {
		return nil, err
	}
	names := map[string]struct{}{}
	for _, name := range existingModels {
		names[name] = struct{}{}
	}
	for _, route := range routes {
		if route.Enabled && route.MatchType == CompositeRouteMatchExact {
			if name := strings.TrimSpace(route.PublicModel); name != "" {
				names[name] = struct{}{}
			}
		}
	}
	sorted := make([]string, 0, len(names))
	for name := range names {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)
	decisions := []CompositeRouteDecision{}
	endpoints := []string{CompositeRouteEndpointAny, CompositeRouteEndpointMessages, CompositeRouteEndpointCountTokens, CompositeRouteEndpointResponses, CompositeRouteEndpointChatCompletions, CompositeRouteEndpointEmbeddings, CompositeRouteEndpointImages, CompositeRouteEndpointGemini}
	for _, name := range sorted {
		seen := map[string]struct{}{}
		for _, endpoint := range endpoints {
			route, matched := matchCompositeRoute(routes, name, endpoint)
			if !matched {
				continue
			}
			upstream := strings.TrimSpace(route.UpstreamModel)
			if upstream == "" {
				upstream = name
			}
			key := route.TargetPlatform + "\x00" + upstream
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			decisions = append(decisions, CompositeRouteDecision{Matched: true, Source: CompositeRouteSourceExplicit, GroupID: groupID, PublicModel: name, TargetPlatform: route.TargetPlatform, UpstreamModel: upstream, Endpoint: endpoint})
		}
	}
	return decisions, nil
}
