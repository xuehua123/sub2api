package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type upstreamPublishedCatalogCache struct {
	payload any
	err     error
	expires time.Time
}

func (s *UpstreamConnectionService) cachedNewAPIModelCatalog(ctx context.Context, c *UpstreamConnection, force bool, fetch func() (any, error)) (any, error) {
	key := fmt.Sprintf("%d:%d", c.ID, c.Version)
	read := func() (upstreamPublishedCatalogCache, bool) {
		s.modelCatalogCacheMu.Lock()
		defer s.modelCatalogCacheMu.Unlock()
		item, ok := s.modelCatalogCache[key]
		return item, ok && item.expires.After(s.now())
	}
	if !force {
		if item, ok := read(); ok {
			return item.payload, item.err
		}
	}
	result := s.modelCatalogFlight.DoChan(key, func() (any, error) {
		if !force {
			if item, ok := read(); ok {
				return item.payload, item.err
			}
		}
		payload, err := fetch()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		ttl := 5 * time.Minute
		if err != nil {
			ttl = time.Minute
		}
		s.modelCatalogCacheMu.Lock()
		defer s.modelCatalogCacheMu.Unlock()
		for k, item := range s.modelCatalogCache {
			if !item.expires.After(s.now()) {
				delete(s.modelCatalogCache, k)
			}
		}
		if len(s.modelCatalogCache) >= 128 {
			for k := range s.modelCatalogCache {
				delete(s.modelCatalogCache, k)
				break
			}
		}
		s.modelCatalogCache[key] = upstreamPublishedCatalogCache{payload: payload, err: err, expires: s.now().Add(ttl)}
		return payload, err
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case value := <-result:
		return value.Val, value.Err
	}
}

// Management catalogs expose groups without requiring us to create keys or
// change an existing key's group. Published catalogs are not live probes.
func (s *UpstreamConnectionService) fetchManagedGroupModels(ctx context.Context, c *UpstreamConnection, group UpstreamGroup, force bool) ([]string, string, error) {
	if s.inspector == nil {
		return nil, "", errors.New("inspector unavailable")
	}
	provider := upstreamConnectionEffectiveProvider(c)
	if provider == "" || provider == UpstreamConnectionProviderAuto {
		return nil, "", errors.New("provider unknown")
	}
	credential, err := s.loadCredential(c)
	if err != nil {
		return nil, "", err
	}
	c, credential, err = s.prepareConnectionCredential(ctx, c, credential)
	if err != nil {
		return nil, "", err
	}
	client, err := s.inspector.clientForConnection(ctx, c)
	if err != nil {
		return nil, "", err
	}
	// Do not forward management credentials through redirects to another host.
	safeClient := *client
	safeClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	client = &safeClient
	management := &upstreamManagementClient{client: client}
	if provider == UpstreamConnectionProviderSub2API {
		headers := upstreamManagementRequestHeaders(credential.UserAgent)
		accessToken := credential.AccessToken
		if c.AuthMode == string(UpstreamManagementAuthModePassword) {
			login, _, e := sub2APIManagementJSON(ctx, management, client, http.MethodPost, c.ManagementBaseURL, "/auth/login", sub2APIManagementV1Prefix, headers, sub2APIManagementLoginBody(credential))
			if e != nil {
				return nil, "", e
			}
			accessToken = firstString(envelopeData(login.payload), "access_token", "token", "jwt")
		}
		if accessToken == "" {
			return nil, "", errors.New("management token unavailable")
		}
		headers.Set("Authorization", "Bearer "+accessToken)
		result, _, e := sub2APIManagementJSON(ctx, management, client, http.MethodGet, c.ManagementBaseURL, "/model-prices?group_id="+url.QueryEscape(group.RemoteID), sub2APIManagementV1Prefix, headers, nil)
		if e != nil {
			return nil, "", e
		}
		models, e := parseSub2APIGroupModelCatalog(envelopeData(result.payload), group.RemoteID)
		return models, "sub2api:published_models", e
	}
	payload, err := s.cachedNewAPIModelCatalog(ctx, c, force, func() (any, error) {
		remoteID, err := parseConnectionRemoteUserID(c.RemoteUserID)
		if err != nil {
			return nil, err
		}
		legacy := upstreamConnectionLegacyNewAPIProvider(provider)
		session, err := management.authenticateNewAPIManagementSession(ctx, client, c.ManagementBaseURL,
			upstreamManagementConfig{Provider: legacy, AuthMode: UpstreamManagementAuthMode(c.AuthMode), RemoteUserID: remoteID},
			upstreamManagementAuthSecret{Username: credential.Username, Password: credential.Password, AccessToken: credential.AccessToken, UserAgent: credential.UserAgent})
		if err != nil {
			return nil, err
		}
		result, err := management.managementJSON(ctx, client, http.MethodGet, upstreamConnectionJoinEndpoint(c.ManagementBaseURL, "/api/pricing", false), newAPIManagementHeaders(legacy, session), nil)
		if err != nil {
			return nil, err
		}
		return envelopeData(result.payload), nil
	})
	if err != nil {
		return nil, "", err
	}
	models, err := parseNewAPIGroupModelCatalog(payload, group.Name)
	return models, "newapi:published_models", err
}

func parseSub2APIGroupModelCatalog(payload any, remoteID string) ([]string, error) {
	data, ok := payload.(map[string]any)
	expected, err := strconv.ParseInt(remoteID, 10, 64)
	if !ok || err != nil || expected <= 0 || int64FromMap(data, "selected_group_id") != expected {
		return nil, errors.New("catalog did not confirm requested group")
	}
	entries, ok := data["models"].([]any)
	if !ok {
		return nil, errors.New("catalog model list missing")
	}
	models := []string{}
	for _, entry := range entries {
		name := firstString(entry, "name")
		if name == "" {
			return nil, errors.New("invalid model catalog entry")
		}
		models = append(models, name)
	}
	return dedupeAndSortModelIDs(models), nil
}

func parseNewAPIGroupModelCatalog(payload any, group string) ([]string, error) {
	entries, ok := payload.([]any)
	if !ok {
		return nil, errors.New("catalog model list missing")
	}
	models := []string{}
	recognized := false
	for _, entry := range entries {
		row, ok := entry.(map[string]any)
		if !ok {
			return nil, errors.New("invalid model entry")
		}
		groups, ok := row["enable_groups"].([]any)
		name := firstString(row, "model_name")
		if !ok || name == "" {
			return nil, errors.New("catalog does not expose group membership")
		}
		recognized = true
		for _, g := range groups {
			if g == group {
				models = append(models, name)
				break
			}
		}
	}
	if !recognized {
		return nil, errors.New("no group-scoped catalog evidence")
	}
	return dedupeAndSortModelIDs(models), nil
}
