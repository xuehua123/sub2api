package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
)

var errUpstreamConnectionRemoteUserIDRequired = errors.New("remote user id is required for this provider's access-token authentication")

type upstreamConnectionWalletObservation struct {
	Amount      *float64
	Currency    string
	USD         *float64
	Unlimited   bool
	Source      string
	Reliability string
	Raw         map[string]any
}

type upstreamConnectionProbeSnapshot struct {
	DetectedProvider string
	RemoteUserID     string
	Wallet           *upstreamConnectionWalletObservation
	WalletObserved   bool
	Groups           []UpstreamGroup
	GroupsObserved   bool
	Capabilities     map[string]any
	Warnings         []string
}

type sub2APIManagementLoginRequest struct {
	Email            string `json:"email"`
	Password         string `json:"password"`
	NotInCNConfirmed bool   `json:"not_in_cn_confirmed,omitempty"`
}

func sub2APIManagementLoginBody(credential upstreamConnectionCredential) sub2APIManagementLoginRequest {
	return sub2APIManagementLoginRequest{
		Email: credential.Username, Password: credential.Password,
		NotInCNConfirmed: credential.NotInCNConfirmed,
	}
}

type upstreamConnectionInspector struct {
	cfg       *config.Config
	proxyRepo ProxyRepository
	client    *http.Client
	now       func() time.Time
}

type upstreamConnectionKeyResolver func(context.Context, string) (UpstreamAccountBinding, error)

func newUpstreamConnectionInspector(cfg *config.Config, proxyRepo ProxyRepository, client *http.Client) *upstreamConnectionInspector {
	if client == nil {
		client = &http.Client{Timeout: upstreamManagementRequestTimeout}
	}
	return &upstreamConnectionInspector{cfg: cfg, proxyRepo: proxyRepo, client: client, now: time.Now}
}

func (i *upstreamConnectionInspector) Inspect(ctx context.Context, connection *UpstreamConnection, credential upstreamConnectionCredential) (*upstreamConnectionProbeSnapshot, error) {
	if connection == nil {
		return nil, errors.New("upstream connection is required")
	}
	client, err := i.clientForConnection(ctx, connection)
	if err != nil {
		return nil, err
	}
	providers := upstreamConnectionProbeProviders(connection.Provider)
	if len(providers) == 0 {
		return nil, fmt.Errorf("unsupported upstream provider %q", connection.Provider)
	}
	isAuto := strings.EqualFold(strings.TrimSpace(connection.Provider), UpstreamConnectionProviderAuto)
	providerHint := ""
	if isAuto {
		providerHint = i.detectUpstreamConnectionProvider(ctx, client, connection.ManagementBaseURL)
		providers = prioritizeUpstreamConnectionProvider(providers, providerHint)
	}

	probeErrors := make([]string, 0, len(providers))
	remoteUserIDRequired := false
	authenticationRejected := false
	var degradedSnapshot *upstreamConnectionProbeSnapshot
	degradedScore := -1
	for _, provider := range providers {
		var snapshot *upstreamConnectionProbeSnapshot
		var probeErr error
		if provider == UpstreamConnectionProviderSub2API {
			snapshot, probeErr = i.inspectSub2API(ctx, client, connection, credential)
		} else {
			snapshot, probeErr = i.inspectNewAPI(ctx, client, connection, credential, provider)
		}
		if probeErr == nil {
			if !isAuto || provider == providerHint || len(snapshot.Warnings) == 0 {
				return snapshot, nil
			}
			if score := upstreamConnectionProbeSnapshotScore(snapshot); degradedSnapshot == nil || score > degradedScore {
				degradedSnapshot = snapshot
				degradedScore = score
			}
			probeErrors = append(probeErrors, provider+": partial inspection: "+strings.Join(snapshot.Warnings, "; "))
			continue
		}
		if errors.Is(probeErr, errUpstreamConnectionRemoteUserIDRequired) {
			remoteUserIDRequired = true
			if connection.Provider != UpstreamConnectionProviderAuto {
				return nil, probeErr
			}
		}
		if errors.Is(probeErr, ErrUpstreamConnectionAuthentication) {
			authenticationRejected = true
		}
		if errors.Is(probeErr, ErrUpstreamManagementLocationConfirmationRequired) {
			return nil, probeErr
		}
		probeErrors = append(probeErrors, provider+": "+probeErr.Error())
	}
	if degradedSnapshot != nil {
		return degradedSnapshot, nil
	}
	if remoteUserIDRequired {
		return nil, errUpstreamConnectionRemoteUserIDRequired
	}
	if authenticationRejected {
		return nil, fmt.Errorf("%w: %s", ErrUpstreamConnectionAuthentication, strings.Join(probeErrors, "; "))
	}
	return nil, fmt.Errorf("unable to authenticate or inspect upstream (%s)", strings.Join(probeErrors, "; "))
}

func upstreamConnectionProbeProviders(provider string) []string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case UpstreamConnectionProviderAuto:
		return []string{
			UpstreamConnectionProviderSub2API,
			UpstreamConnectionProviderNewAPI,
			UpstreamConnectionProviderOneHub,
			UpstreamConnectionProviderDoneHub,
			UpstreamConnectionProviderOneAPI,
			UpstreamConnectionProviderVeloera,
			UpstreamConnectionProviderRixAPI,
			UpstreamConnectionProviderShellAPI,
		}
	case UpstreamConnectionProviderSub2API:
		return []string{UpstreamConnectionProviderSub2API}
	case UpstreamConnectionProviderNewAPI, UpstreamConnectionProviderRixAPI, UpstreamConnectionProviderShellAPI,
		UpstreamConnectionProviderOneAPI, UpstreamConnectionProviderVeloera,
		UpstreamConnectionProviderOneHub, UpstreamConnectionProviderDoneHub:
		return []string{provider}
	default:
		return nil
	}
}

func upstreamConnectionEffectiveProvider(connection *UpstreamConnection) string {
	if connection == nil {
		return ""
	}
	provider := strings.ToLower(strings.TrimSpace(connection.Provider))
	if provider != UpstreamConnectionProviderAuto {
		return provider
	}
	detected, _ := connection.Capabilities["detected_provider"].(string)
	detected = strings.ToLower(strings.TrimSpace(detected))
	if detected != "" && detected != UpstreamConnectionProviderAuto && len(upstreamConnectionProbeProviders(detected)) == 1 {
		return detected
	}
	return provider
}

func (i *upstreamConnectionInspector) detectUpstreamConnectionProvider(
	ctx context.Context,
	client *http.Client,
	baseURL string,
) string {
	management := &upstreamManagementClient{client: client}
	status, err := management.managementJSON(ctx, client, http.MethodGet,
		upstreamConnectionJoinEndpoint(baseURL, "/api/status", false), nil, nil)
	if err != nil {
		return ""
	}
	return upstreamConnectionProviderHintFromStatus(envelopeData(status.payload))
}

func upstreamConnectionProviderHintFromStatus(payload any) string {
	status, ok := payload.(map[string]any)
	if !ok {
		return ""
	}
	normalizedName := strings.ToLower(strings.TrimSpace(firstString(status, "system_name", "systemName")))
	normalizedName = strings.NewReplacer(" ", "", "-", "", "_", "").Replace(normalizedName)
	for _, candidate := range []struct {
		marker   string
		provider string
	}{
		{marker: "sub2api", provider: UpstreamConnectionProviderSub2API},
		{marker: "donehub", provider: UpstreamConnectionProviderDoneHub},
		{marker: "onehub", provider: UpstreamConnectionProviderOneHub},
		{marker: "oneapi", provider: UpstreamConnectionProviderOneAPI},
		{marker: "newapi", provider: UpstreamConnectionProviderNewAPI},
		{marker: "veloera", provider: UpstreamConnectionProviderVeloera},
		{marker: "rixapi", provider: UpstreamConnectionProviderRixAPI},
		{marker: "shellapi", provider: UpstreamConnectionProviderShellAPI},
	} {
		if strings.Contains(normalizedName, candidate.marker) {
			return candidate.provider
		}
	}

	// These public status fields are product-specific in the maintained
	// upstream implementations and remain useful when operators rename a site.
	if upstreamConnectionStatusHasAny(status, "ClaudeAPIEnabled", "GeminiAPIEnabled", "builtin_chat_enabled") {
		return UpstreamConnectionProviderDoneHub
	}
	if upstreamConnectionStatusHasAny(status, "EnableSafe", "PaymentUSDRate", "SafeToolName") {
		return UpstreamConnectionProviderOneHub
	}
	if upstreamConnectionStatusHasAny(status, "aff_enabled", "idcflare_client_id", "idcflare_oauth") {
		return UpstreamConnectionProviderVeloera
	}
	if upstreamConnectionStatusHasAny(status, "HeaderNavModules", "SidebarModulesAdmin", "announcements_enabled") {
		return UpstreamConnectionProviderNewAPI
	}
	if upstreamConnectionStatusHasAny(status, "oidc", "oidc_well_known", "oidc_token_endpoint") {
		return UpstreamConnectionProviderOneAPI
	}
	return ""
}

func upstreamConnectionStatusHasAny(status map[string]any, keys ...string) bool {
	for _, key := range keys {
		if _, exists := status[key]; exists {
			return true
		}
	}
	return false
}

func prioritizeUpstreamConnectionProvider(providers []string, providerHint string) []string {
	if providerHint == "" {
		return providers
	}
	prioritized := make([]string, 0, len(providers))
	prioritized = append(prioritized, providerHint)
	for _, provider := range providers {
		if provider != providerHint {
			prioritized = append(prioritized, provider)
		}
	}
	return prioritized
}

func upstreamConnectionProbeSnapshotScore(snapshot *upstreamConnectionProbeSnapshot) int {
	if snapshot == nil {
		return -1
	}
	score := 0
	if snapshot.WalletObserved {
		score += 2
	}
	if snapshot.GroupsObserved && len(snapshot.Groups) > 0 {
		score += 2
	}
	return score - len(snapshot.Warnings)
}

func (i *upstreamConnectionInspector) inspectNewAPI(
	ctx context.Context,
	client *http.Client,
	connection *UpstreamConnection,
	credential upstreamConnectionCredential,
	provider string,
) (*upstreamConnectionProbeSnapshot, error) {
	remoteUserID, err := parseConnectionRemoteUserID(connection.RemoteUserID)
	if err != nil {
		return nil, err
	}
	authMode := UpstreamManagementAuthMode(connection.AuthMode)
	if authMode == UpstreamManagementAuthModeAccessToken && remoteUserID <= 0 &&
		upstreamConnectionProviderRequiresRemoteUserID(provider) {
		return nil, errUpstreamConnectionRemoteUserIDRequired
	}
	legacyProvider := upstreamConnectionLegacyNewAPIProvider(provider)
	config := upstreamManagementConfig{
		Provider: legacyProvider, AuthMode: authMode, Group: "__connection_probe__", RemoteUserID: remoteUserID,
	}
	secret := upstreamManagementAuthSecret{
		Username: credential.Username, Password: credential.Password,
		AccessToken: credential.AccessToken, RefreshToken: credential.RefreshToken,
		UserAgent: credential.UserAgent,
	}
	if err := validateUpstreamManagementAuth(config, secret); err != nil {
		return nil, err
	}
	management := &upstreamManagementClient{client: client}
	session, err := management.authenticateNewAPIManagementSession(ctx, client, connection.ManagementBaseURL, config, secret)
	if err != nil {
		return nil, err
	}
	headers := newAPIManagementHeaders(legacyProvider, session)

	now := i.now().UTC()
	snapshot := &upstreamConnectionProbeSnapshot{
		DetectedProvider: provider,
		RemoteUserID:     strconv.FormatInt(session.remoteUserID, 10),
		Capabilities: map[string]any{
			"dialect": provider, "wallet": false, "groups": false, "key_lookup": true,
		},
		Groups:   []UpstreamGroup{},
		Warnings: []string{},
	}
	successfulRequests := 0
	authenticatedSession := authMode == UpstreamManagementAuthModePassword
	profileEndpoint := upstreamConnectionJoinEndpoint(connection.ManagementBaseURL, "/api/user/self", false)
	profile, profileErr := management.managementJSON(ctx, client, http.MethodGet, profileEndpoint, headers, nil)
	var profileData map[string]any
	if profileErr == nil {
		profileData = upstreamConnectionDataObject(profile.payload)
		discoveredID := int64FromMap(profileData, "id")
		profileQuota := upstreamConnectionNumber(profileData, "quota", "balance", "available")
		if discoveredID > 0 || profileQuota != nil || strings.TrimSpace(firstString(profileData, "group")) != "" {
			successfulRequests++
			authenticatedSession = true
		}
		if discoveredID > 0 {
			snapshot.RemoteUserID = strconv.FormatInt(discoveredID, 10)
		}
		snapshot.Wallet = i.newAPIWallet(ctx, management, client, connection.ManagementBaseURL, headers, profileData)
		if snapshot.Wallet != nil {
			snapshot.WalletObserved = true
			snapshot.Capabilities["wallet"] = true
		} else {
			snapshot.Warnings = append(snapshot.Warnings, "wallet: profile did not expose a usable quota")
		}
	} else {
		snapshot.Warnings = append(snapshot.Warnings, "wallet: "+profileErr.Error())
	}

	groups, groupSource, groupsErr := inspectNewAPIGroups(ctx, management, client, connection.ManagementBaseURL, headers, provider, profileData)
	if groupsErr == nil {
		successfulRequests++
		if groupSource == "newapi:self_groups" {
			authenticatedSession = true
		}
		snapshot.GroupsObserved = true
		for index := range groups {
			groups[index].Source = groupSource
			groups[index].Metadata = map[string]any{}
			groups[index].ObservedAt = &now
			if groups[index].RateMultiplier != nil {
				groups[index].Confidence = "reported"
			} else {
				groups[index].Confidence = "unknown"
			}
		}
		snapshot.Groups = groups
		snapshot.Capabilities["groups"] = len(snapshot.Groups) > 0
		if len(snapshot.Groups) == 0 {
			snapshot.Warnings = append(snapshot.Warnings, "groups: upstream returned no usable groups")
		}
	} else {
		snapshot.Warnings = append(snapshot.Warnings, "groups: "+groupsErr.Error())
	}
	if successfulRequests == 0 {
		if errors.Is(profileErr, ErrUpstreamConnectionAuthentication) || errors.Is(groupsErr, ErrUpstreamConnectionAuthentication) {
			return nil, fmt.Errorf("%w: NewAPI management endpoints rejected the authenticated session", ErrUpstreamConnectionAuthentication)
		}
		return nil, errors.New("NewAPI management endpoints rejected the authenticated session")
	}
	if !authenticatedSession {
		if errors.Is(profileErr, ErrUpstreamConnectionAuthentication) {
			return nil, fmt.Errorf("%w: NewAPI management endpoints rejected the authenticated session", ErrUpstreamConnectionAuthentication)
		}
		return nil, errors.New("NewAPI management endpoints did not verify the authenticated session")
	}
	return snapshot, nil
}

func inspectNewAPIGroups(
	ctx context.Context,
	management *upstreamManagementClient,
	client *http.Client,
	baseURL string,
	headers http.Header,
	provider string,
	profileData map[string]any,
) ([]UpstreamGroup, string, error) {
	if provider == UpstreamConnectionProviderOneAPI {
		groupName := strings.TrimSpace(firstString(profileData, "group"))
		if groupName == "" {
			return nil, "", errors.New("OneAPI user profile did not expose a group")
		}
		return []UpstreamGroup{{Name: groupName}}, "oneapi:user_self", nil
	}
	if provider == UpstreamConnectionProviderOneHub || provider == UpstreamConnectionProviderDoneHub {
		endpoint := upstreamConnectionJoinEndpoint(baseURL, "/api/user_group_map", false)
		response, err := management.managementJSON(ctx, client, http.MethodGet, endpoint, headers, nil)
		if err != nil {
			return nil, "", err
		}
		groups := extractNewAPIConnectionGroups(envelopeData(response.payload))
		if len(groups) == 0 {
			return nil, "", errors.New("upstream returned no usable groups")
		}
		return groups, provider + ":user_group_map", nil
	}

	groupsEndpoint := upstreamConnectionJoinEndpoint(baseURL, "/api/user/self/groups", false)
	groupsResponse, groupsErr := management.managementJSON(ctx, client, http.MethodGet, groupsEndpoint, headers, nil)
	if groupsErr == nil {
		if groups := extractNewAPIConnectionGroups(envelopeData(groupsResponse.payload)); len(groups) > 0 {
			return groups, "newapi:self_groups", nil
		}
	}
	pricingEndpoint := upstreamConnectionJoinEndpoint(baseURL, "/api/pricing", false)
	pricingResponse, pricingErr := management.managementJSON(ctx, client, http.MethodGet, pricingEndpoint, headers, nil)
	if pricingErr == nil {
		if groups := extractNewAPIConnectionGroups(newAPIPricingGroupRatios(pricingResponse.payload)); len(groups) > 0 {
			return groups, "newapi:pricing", nil
		}
	}
	if groupsErr != nil {
		return nil, "", groupsErr
	}
	if pricingErr != nil {
		return nil, "", pricingErr
	}
	return nil, "", errors.New("upstream returned no usable groups")
}

func newAPIPricingGroupRatios(payload map[string]any) any {
	if payload == nil {
		return nil
	}
	for _, key := range []string{"group_ratio", "GroupRatio"} {
		if value, ok := payload[key]; ok {
			return value
		}
	}
	if data, ok := payload["data"].(map[string]any); ok {
		for _, key := range []string{"group_ratio", "GroupRatio"} {
			if value, exists := data[key]; exists {
				return value
			}
		}
	}
	return nil
}

func extractNewAPIConnectionGroups(payload any) []UpstreamGroup {
	if row, ok := payload.(map[string]any); ok {
		for _, key := range []string{"group_data", "group_ratio", "GroupRatio", "group_info", "rates", "groups"} {
			if nested, exists := row[key]; exists {
				return extractNewAPIConnectionGroups(nested)
			}
		}
	}

	groups := make([]UpstreamGroup, 0)
	appendGroup := func(name string, value any) {
		name = strings.TrimSpace(name)
		remoteID := ""
		if row, ok := value.(map[string]any); ok {
			if reportedName := strings.TrimSpace(firstString(row, "symbol", "group", "group_name", "name")); reportedName != "" {
				name = reportedName
			}
			if id := int64FromMap(row, "id", "group_id"); id > 0 {
				remoteID = strconv.FormatInt(id, 10)
			}
		}
		if name == "" {
			return
		}
		var multiplier *float64
		if parsed, valid := parseGroupMultiplier(value); valid {
			copy := parsed
			multiplier = &copy
		}
		groups = append(groups, UpstreamGroup{RemoteID: remoteID, Name: name, RateMultiplier: multiplier})
	}

	switch value := payload.(type) {
	case map[string]any:
		if name := strings.TrimSpace(firstString(value, "symbol", "group", "group_name", "name")); name != "" {
			appendGroup(name, value)
		} else {
			for name, item := range value {
				appendGroup(name, item)
			}
		}
	case []any:
		for _, item := range value {
			row, ok := item.(map[string]any)
			if !ok {
				continue
			}
			appendGroup(firstString(row, "symbol", "group", "group_name", "name"), row)
		}
	}

	byName := make(map[string]UpstreamGroup, len(groups))
	for _, group := range groups {
		existing, exists := byName[group.Name]
		if !exists {
			byName[group.Name] = group
			continue
		}
		if existing.RateMultiplier == nil && group.RateMultiplier != nil {
			existing.RateMultiplier = group.RateMultiplier
		}
		if existing.RemoteID == "" && group.RemoteID != "" {
			existing.RemoteID = group.RemoteID
		}
		byName[group.Name] = existing
	}
	groups = groups[:0]
	for _, group := range byName {
		groups = append(groups, group)
	}
	sort.Slice(groups, func(left, right int) bool {
		if groups[left].RateMultiplier == nil || groups[right].RateMultiplier == nil {
			if groups[left].RateMultiplier == nil && groups[right].RateMultiplier != nil {
				return false
			}
			if groups[left].RateMultiplier != nil && groups[right].RateMultiplier == nil {
				return true
			}
		} else if *groups[left].RateMultiplier != *groups[right].RateMultiplier {
			return *groups[left].RateMultiplier < *groups[right].RateMultiplier
		}
		return groups[left].Name < groups[right].Name
	})
	return groups
}

func (i *upstreamConnectionInspector) newAPIWallet(
	ctx context.Context,
	management *upstreamManagementClient,
	client *http.Client,
	baseURL string,
	headers http.Header,
	profileData map[string]any,
) *upstreamConnectionWalletObservation {
	quota := upstreamConnectionNumber(profileData, "quota", "balance", "available")
	if quota == nil {
		return nil
	}
	amount := *quota
	currency := "QUOTA"
	reliability := "raw_quota"
	raw := map[string]any{"quota": *quota}
	if used := upstreamConnectionNumber(profileData, "used_quota", "used"); used != nil {
		raw["used_quota"] = *used
	}
	statusEndpoint := upstreamConnectionJoinEndpoint(baseURL, "/api/status", false)
	if status, statusErr := management.managementJSON(ctx, client, http.MethodGet, statusEndpoint, headers, nil); statusErr == nil {
		statusData := upstreamConnectionDataObject(status.payload)
		if perUnit := upstreamConnectionNumber(statusData, "quota_per_unit"); perUnit != nil && *perUnit > 0 {
			usdAmount := *quota / *perUnit
			amount = usdAmount
			currency = "USD"
			usd := usdAmount
			raw["quota_per_unit"] = *perUnit
			reliability = "exact"

			displayType := strings.ToUpper(strings.TrimSpace(firstString(statusData, "quota_display_type", "currency")))
			if displayType == "" {
				if displayInCurrency, exists := statusData["display_in_currency"].(bool); exists {
					if displayInCurrency {
						displayType = "USD"
					} else {
						displayType = "TOKENS"
					}
				}
			}
			if displayType != "" {
				raw["quota_display_type"] = displayType
			}
			switch displayType {
			case "CNY":
				if exchangeRate := upstreamConnectionNumber(statusData, "usd_exchange_rate"); exchangeRate != nil && *exchangeRate > 0 {
					amount = usdAmount * *exchangeRate
					currency = "CNY"
					reliability = "converted"
					raw["usd_exchange_rate"] = *exchangeRate
				}
			case "CUSTOM":
				if exchangeRate := upstreamConnectionNumber(statusData, "custom_currency_exchange_rate"); exchangeRate != nil && *exchangeRate > 0 {
					amount = usdAmount * *exchangeRate
					currency = safeUpstreamWalletCurrency(firstString(statusData, "custom_currency_symbol"), "CUSTOM")
					reliability = "converted"
					raw["custom_currency_exchange_rate"] = *exchangeRate
					raw["custom_currency_symbol"] = currency
				}
			case "TOKENS":
				amount = *quota
				currency = "TOKENS"
				reliability = "raw_quota"
			}
			observation := &upstreamConnectionWalletObservation{
				Amount: &amount, Currency: currency, USD: &usd, Source: "newapi:user_self",
				Reliability: reliability, Raw: raw,
			}
			return observation
		}
	}
	return &upstreamConnectionWalletObservation{
		Amount: &amount, Currency: currency, Source: "newapi:user_self", Reliability: reliability, Raw: raw,
	}
}

func safeUpstreamWalletCurrency(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" || utf8.RuneCountInString(value) > 16 || strings.ContainsFunc(value, unicode.IsControl) {
		return fallback
	}
	return value
}

func (i *upstreamConnectionInspector) inspectSub2API(
	ctx context.Context,
	client *http.Client,
	connection *UpstreamConnection,
	credential upstreamConnectionCredential,
) (*upstreamConnectionProbeSnapshot, error) {
	management := &upstreamManagementClient{client: client}
	accessToken := strings.TrimSpace(credential.AccessToken)
	headers := upstreamManagementRequestHeaders(credential.UserAgent)
	if connection.AuthMode == string(UpstreamManagementAuthModePassword) {
		login, err := management.managementJSON(ctx, client, http.MethodPost,
			upstreamConnectionJoinEndpoint(connection.ManagementBaseURL, "/api/v1/auth/login", false), headers,
			sub2APIManagementLoginBody(credential))
		if err != nil {
			return nil, err
		}
		accessToken = firstString(envelopeData(login.payload), "access_token", "token", "jwt")
	}
	if accessToken == "" {
		return nil, errors.New("Sub2API management login did not return an access token")
	}
	headers.Set("Authorization", "Bearer "+accessToken)
	now := i.now().UTC()
	snapshot := &upstreamConnectionProbeSnapshot{
		DetectedProvider: UpstreamConnectionProviderSub2API,
		Capabilities: map[string]any{
			"dialect": UpstreamConnectionProviderSub2API, "wallet": false, "groups": false, "key_lookup": true,
		},
		Groups: []UpstreamGroup{}, Warnings: []string{},
	}
	successfulRequests := 0
	// Current Sub2API deployments expose the authenticated account through
	// auth/me. The historical user/profile route is not implemented by them.
	profileEndpoint := upstreamConnectionJoinEndpoint(connection.ManagementBaseURL, "/api/v1/auth/me", false)
	profile, profileErr := management.managementJSON(ctx, client, http.MethodGet, profileEndpoint, headers, nil)
	if profileErr == nil {
		profileData := upstreamConnectionDataObject(profile.payload)
		discoveredID := int64FromMap(profileData, "id", "user_id")
		balance := upstreamConnectionNumber(profileData, "balance")
		if discoveredID > 0 || balance != nil {
			successfulRequests++
		}
		if discoveredID > 0 {
			snapshot.RemoteUserID = strconv.FormatInt(discoveredID, 10)
		}
		if balance != nil {
			amount := *balance
			snapshot.Wallet = &upstreamConnectionWalletObservation{
				Amount: &amount, Currency: "USD", USD: &amount, Source: "sub2api:auth_me",
				Reliability: "exact", Raw: map[string]any{"balance": amount},
			}
			snapshot.Capabilities["wallet"] = true
			snapshot.WalletObserved = true
		} else {
			snapshot.Warnings = append(snapshot.Warnings, "wallet: profile did not expose a balance")
		}
	} else {
		snapshot.Warnings = append(snapshot.Warnings, "wallet: "+profileErr.Error())
	}

	groups, groupsWarning, groupsErr := inspectSub2APIGroups(ctx, management, client, connection.ManagementBaseURL, headers, now)
	if groupsErr == nil {
		successfulRequests++
		snapshot.GroupsObserved = true
		snapshot.Groups = groups
		if groupsWarning != "" {
			snapshot.Warnings = append(snapshot.Warnings, groupsWarning)
		}
		snapshot.Capabilities["groups"] = len(groups) > 0
		if len(groups) == 0 {
			snapshot.Warnings = append(snapshot.Warnings, "groups: upstream returned no usable groups")
		}
	} else {
		snapshot.Warnings = append(snapshot.Warnings, "groups: "+groupsErr.Error())
	}
	if successfulRequests == 0 {
		if errors.Is(profileErr, ErrUpstreamConnectionAuthentication) || errors.Is(groupsErr, ErrUpstreamConnectionAuthentication) {
			return nil, fmt.Errorf("%w: Sub2API management endpoints rejected the access token", ErrUpstreamConnectionAuthentication)
		}
		return nil, errors.New("Sub2API management endpoints rejected the access token")
	}
	return snapshot, nil
}

func inspectSub2APIGroups(
	ctx context.Context,
	management *upstreamManagementClient,
	client *http.Client,
	baseURL string,
	headers http.Header,
	now time.Time,
) ([]UpstreamGroup, string, error) {
	availableEndpoint := upstreamConnectionJoinEndpoint(baseURL, "/api/v1/groups/available", false)
	available, err := management.managementJSON(ctx, client, http.MethodGet, availableEndpoint, headers, nil)
	if err != nil {
		return nil, "", err
	}
	availableData := envelopeData(available.payload)
	if !upstreamManagementItemsRecognized(availableData) {
		return nil, "", errors.New("Sub2API groups response has no recognizable item collection")
	}
	groups := extractSub2APIConnectionGroups(availableData, now)
	if len(groups) == 0 {
		return []UpstreamGroup{}, "", nil
	}
	ratesConfirmed := false
	warning := ""
	ratesEndpoint := upstreamConnectionJoinEndpoint(baseURL, "/api/v1/groups/rates", false)
	if rates, ratesErr := management.managementJSON(ctx, client, http.MethodGet, ratesEndpoint, headers, nil); ratesErr == nil {
		applySub2APIConnectionGroupRates(groups, envelopeData(rates.payload))
		ratesConfirmed = true
	} else {
		warning = "groups: user-specific rates unavailable; showing available-group fallback rates"
	}
	for index := range groups {
		if groups[index].RateMultiplier != nil && ratesConfirmed {
			groups[index].Confidence = "reported"
		} else if groups[index].RateMultiplier != nil {
			groups[index].Confidence = "fallback"
		} else {
			groups[index].Confidence = "unknown"
		}
	}
	sort.Slice(groups, func(left, right int) bool {
		return groups[left].Name < groups[right].Name
	})
	return groups, warning, nil
}

func extractSub2APIConnectionGroups(payload any, now time.Time) []UpstreamGroup {
	items := upstreamManagementItems(payload)
	groups := make([]UpstreamGroup, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		row, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name := strings.TrimSpace(firstString(row, "name", "group", "group_name"))
		if name == "" {
			continue
		}
		remoteID := strconv.FormatInt(int64FromMap(row, "id", "group_id"), 10)
		key := remoteID + "\x00" + name
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		var multiplier *float64
		if parsed, valid := parseGroupMultiplier(row); valid {
			value := parsed
			multiplier = &value
		}
		observed := now
		groups = append(groups, UpstreamGroup{
			RemoteID: remoteID, Name: name, RateMultiplier: multiplier, Source: "sub2api:available_groups",
			Metadata: map[string]any{}, ObservedAt: &observed,
		})
	}
	return groups
}

func applySub2APIConnectionGroupRates(groups []UpstreamGroup, payload any) {
	rates, ok := payload.(map[string]any)
	if !ok {
		return
	}
	for index := range groups {
		raw, exists := rates[groups[index].RemoteID]
		if !exists {
			continue
		}
		if parsed, valid := parseGroupMultiplier(raw); valid {
			value := parsed
			groups[index].RateMultiplier = &value
			groups[index].Source = "sub2api:group_rates"
		}
	}
}

func upstreamConnectionLegacyNewAPIProvider(provider string) UpstreamManagementProvider {
	switch provider {
	case UpstreamConnectionProviderRixAPI:
		return UpstreamManagementProviderRixAPI
	case UpstreamConnectionProviderShellAPI:
		return UpstreamManagementProviderShellAPI
	case UpstreamConnectionProviderVeloera:
		return UpstreamManagementProviderVeloera
	default:
		return UpstreamManagementProviderNewAPI
	}
}

func parseConnectionRemoteUserID(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return 0, errors.New("remote user id must be a positive integer for this provider")
	}
	return parsed, nil
}

func (i *upstreamConnectionInspector) clientForConnection(ctx context.Context, connection *UpstreamConnection) (*http.Client, error) {
	if connection.ProxyID == nil {
		if i.cfg != nil && i.cfg.Security.URLAllowlist.Enabled {
			return httpclient.GetClient(httpclient.Options{
				Timeout:               upstreamManagementRequestTimeout,
				ResponseHeaderTimeout: upstreamManagementRequestTimeout,
				ValidateResolvedIP:    true,
				AllowPrivateHosts:     i.cfg.Security.URLAllowlist.AllowPrivateHosts,
			})
		}
		return i.client, nil
	}
	if i.proxyRepo == nil {
		return nil, errors.New("proxy repository is unavailable")
	}
	proxy, err := i.proxyRepo.GetByID(ctx, *connection.ProxyID)
	if err != nil {
		return nil, err
	}
	if proxy == nil || !proxy.IsActive() || strings.TrimSpace(proxy.URL()) == "" {
		return nil, errors.New("configured upstream proxy is inactive")
	}
	validateResolvedIP := i.cfg != nil && i.cfg.Security.URLAllowlist.Enabled
	return httpclient.GetClient(httpclient.Options{
		ProxyURL:              proxy.URL(),
		Timeout:               upstreamManagementRequestTimeout,
		ResponseHeaderTimeout: upstreamManagementRequestTimeout,
		ValidateResolvedIP:    validateResolvedIP,
		AllowPrivateHosts:     i.cfg != nil && i.cfg.Security.URLAllowlist.AllowPrivateHosts,
	})
}

func sanitizeUpstreamKeyLookupError(err error, apiKey string) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if urlErr.Err != nil {
			message = fmt.Sprintf("upstream API key lookup request failed: %v", urlErr.Err)
		} else {
			message = "upstream API key lookup request failed"
		}
	}
	if secret := strings.TrimSpace(apiKey); secret != "" {
		message = strings.ReplaceAll(message, secret, "[redacted]")
	}
	return errors.New(message)
}

func (i *upstreamConnectionInspector) ResolveKey(
	ctx context.Context,
	connection *UpstreamConnection,
	credential upstreamConnectionCredential,
	apiKey string,
) (UpstreamAccountBinding, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return UpstreamAccountBinding{}, errors.New("upstream API key is empty")
	}
	resolver, err := i.PrepareKeyResolver(ctx, connection, credential)
	if err != nil {
		return UpstreamAccountBinding{}, err
	}
	return resolver(ctx, apiKey)
}

func (i *upstreamConnectionInspector) PrepareKeyResolver(
	ctx context.Context,
	connection *UpstreamConnection,
	credential upstreamConnectionCredential,
) (upstreamConnectionKeyResolver, error) {
	client, err := i.clientForConnection(ctx, connection)
	if err != nil {
		return nil, err
	}
	detectedProvider := upstreamConnectionEffectiveProvider(connection)
	if detectedProvider == "" || detectedProvider == UpstreamConnectionProviderAuto {
		return nil, errors.New("probe the upstream connection before binding an account")
	}
	if detectedProvider == UpstreamConnectionProviderSub2API {
		return i.prepareSub2APIKeyResolver(ctx, client, connection, credential)
	}
	return i.prepareNewAPIKeyResolver(ctx, client, connection, credential, detectedProvider)
}

func (i *upstreamConnectionInspector) prepareNewAPIKeyResolver(
	ctx context.Context,
	client *http.Client,
	connection *UpstreamConnection,
	credential upstreamConnectionCredential,
	provider string,
) (upstreamConnectionKeyResolver, error) {
	remoteUserID, err := parseConnectionRemoteUserID(connection.RemoteUserID)
	if err != nil {
		return nil, err
	}
	if connection.AuthMode == string(UpstreamManagementAuthModeAccessToken) && remoteUserID <= 0 &&
		upstreamConnectionProviderRequiresRemoteUserID(provider) {
		return nil, errUpstreamConnectionRemoteUserIDRequired
	}
	legacyProvider := upstreamConnectionLegacyNewAPIProvider(provider)
	config := upstreamManagementConfig{
		Provider: legacyProvider, AuthMode: UpstreamManagementAuthMode(connection.AuthMode),
		Group: "__key_resolution__", RemoteUserID: remoteUserID,
	}
	secret := upstreamManagementAuthSecret{
		Username: credential.Username, Password: credential.Password,
		AccessToken: credential.AccessToken, RefreshToken: credential.RefreshToken,
		UserAgent: credential.UserAgent,
	}
	management := &upstreamManagementClient{client: client}
	session, err := management.authenticateNewAPIManagementSession(ctx, client, connection.ManagementBaseURL, config, secret)
	if err != nil {
		return nil, err
	}
	headers := newAPIManagementHeaders(legacyProvider, session)
	if providerUsesPaginatedTokenList(provider) {
		rows, listErr := listNewAPILikeTokenRows(ctx, management, client, connection.ManagementBaseURL, headers, provider)
		if listErr != nil {
			return nil, listErr
		}
		return func(resolveCtx context.Context, apiKey string) (UpstreamAccountBinding, error) {
			row, ok := rows[normalizeUpstreamForwardingKey(apiKey)]
			if !ok {
				return UpstreamAccountBinding{}, fmt.Errorf("%w: upstream API key was not found under the configured management user", ErrUpstreamAPIKeyGroupUnmapped)
			}
			return i.resolveNewAPITokenRow(resolveCtx, management, client, connection, headers, row, provider+":token_list")
		}, nil
	}
	return func(resolveCtx context.Context, apiKey string) (UpstreamAccountBinding, error) {
		return i.resolveNewAPIKeyWithSession(resolveCtx, management, client, connection, headers, apiKey, provider)
	}, nil
}

func providerUsesPaginatedTokenList(provider string) bool {
	switch provider {
	case UpstreamConnectionProviderOneAPI, UpstreamConnectionProviderOneHub, UpstreamConnectionProviderDoneHub:
		return true
	default:
		return false
	}
}

func (i *upstreamConnectionInspector) resolveNewAPIKeyWithSession(
	ctx context.Context,
	management *upstreamManagementClient,
	client *http.Client,
	connection *UpstreamConnection,
	headers http.Header,
	apiKey string,
	provider string,
) (UpstreamAccountBinding, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return UpstreamAccountBinding{}, errors.New("upstream API key is empty")
	}
	endpoint := upstreamConnectionJoinEndpoint(connection.ManagementBaseURL, "/api/token/search", false)
	parsedEndpoint, err := url.Parse(endpoint)
	if err != nil {
		return UpstreamAccountBinding{}, err
	}
	query := parsedEndpoint.Query()
	query.Set("token", apiKey)
	query.Set("p", "0")
	query.Set("page_size", "10")
	parsedEndpoint.RawQuery = query.Encode()
	response, err := management.managementJSON(ctx, client, http.MethodGet, parsedEndpoint.String(), headers, nil)
	if err != nil {
		return UpstreamAccountBinding{}, sanitizeUpstreamKeyLookupError(err, apiKey)
	}
	items := upstreamManagementItems(envelopeData(response.payload))
	if len(items) != 1 {
		return UpstreamAccountBinding{}, fmt.Errorf("%w: exact NewAPI token search returned %d matches", ErrUpstreamAPIKeyGroupUnmapped, len(items))
	}
	row, ok := items[0].(map[string]any)
	if !ok {
		return UpstreamAccountBinding{}, fmt.Errorf("%w: NewAPI token search returned an invalid item", ErrUpstreamAPIKeyGroupUnmapped)
	}
	return i.resolveNewAPITokenRow(ctx, management, client, connection, headers, row, provider+":token_search")
}

func (i *upstreamConnectionInspector) resolveNewAPITokenRow(
	ctx context.Context,
	management *upstreamManagementClient,
	client *http.Client,
	connection *UpstreamConnection,
	headers http.Header,
	row map[string]any,
	source string,
) (UpstreamAccountBinding, error) {
	groupName := strings.TrimSpace(firstString(row, "group", "group_name"))
	resolutionKind := UpstreamBindingResolutionFixed
	fallbackGroups := []string{}
	if backupGroup := strings.TrimSpace(firstString(row, "backup_group", "backupGroup")); backupGroup != "" {
		fallbackGroups = append(fallbackGroups, backupGroup)
	}
	if groupName == "" {
		profile, profileErr := management.managementJSON(ctx, client, http.MethodGet,
			upstreamConnectionJoinEndpoint(connection.ManagementBaseURL, "/api/user/self", false), headers, nil)
		if profileErr != nil {
			return UpstreamAccountBinding{}, fmt.Errorf("resolve inherited NewAPI group: %w", profileErr)
		}
		groupName = strings.TrimSpace(firstString(upstreamConnectionDataObject(profile.payload), "group"))
		resolutionKind = UpstreamBindingResolutionInherited
	}
	if strings.EqualFold(groupName, "auto") {
		resolutionKind = UpstreamBindingResolutionDynamic
		if firstBool(row, "cross_group_retry") || len(fallbackGroups) > 0 {
			resolutionKind = UpstreamBindingResolutionFallbackChain
		}
	} else if len(fallbackGroups) > 0 {
		resolutionKind = UpstreamBindingResolutionFallbackChain
	}
	now := i.now().UTC()
	binding := UpstreamAccountBinding{
		RemoteTokenID:   strconv.FormatInt(int64FromMap(row, "id", "token_id"), 10),
		RemoteTokenName: strings.TrimSpace(firstString(row, "name")),
		ResolutionKind:  resolutionKind, RemoteGroupName: groupName, FallbackGroups: fallbackGroups,
		Confidence: "exact", Source: source, ApplyPolicy: UpstreamBindingApplyObserveOnly,
		Status: UpstreamBindingStatusReady, ResolutionDetails: map[string]any{
			"token_group": groupName, "cross_group_retry": firstBool(row, "cross_group_retry"),
		},
		ObservedAt: &now,
	}
	if groupName == "" {
		binding.Status = UpstreamBindingStatusUnresolved
		binding.LastError = "inherited upstream user profile did not expose a group"
		return binding, nil
	}
	if resolutionKind == UpstreamBindingResolutionFixed || resolutionKind == UpstreamBindingResolutionInherited ||
		(resolutionKind == UpstreamBindingResolutionFallbackChain && !strings.EqualFold(groupName, "auto")) {
		if group, found := findObservedUpstreamGroup(connection.Groups, "", groupName); found {
			binding.RemoteGroupID = group.RemoteID
			binding.ObservedMultiplier = cloneFloat64Ptr(group.RateMultiplier)
		} else {
			binding.Status = UpstreamBindingStatusUnresolved
			binding.LastError = fmt.Sprintf("resolved group %q is absent from the latest group snapshot", groupName)
		}
	}
	return binding, nil
}

func listNewAPILikeTokenRows(
	ctx context.Context,
	management *upstreamManagementClient,
	client *http.Client,
	baseURL string,
	headers http.Header,
	provider string,
) (map[string]map[string]any, error) {
	const (
		pageSize = 100
		maxPages = 1000
	)
	rows := make(map[string]map[string]any)
	startPage := 1
	if provider == UpstreamConnectionProviderOneAPI {
		startPage = 0
	}
	for offset := 0; offset < maxPages; offset++ {
		page := startPage + offset
		endpoint, err := url.Parse(upstreamConnectionJoinEndpoint(baseURL, "/api/token/", false))
		if err != nil {
			return nil, err
		}
		query := endpoint.Query()
		if provider == UpstreamConnectionProviderOneAPI {
			query.Set("p", strconv.Itoa(page))
		} else {
			query.Set("page", strconv.Itoa(page))
			query.Set("size", strconv.Itoa(pageSize))
		}
		endpoint.RawQuery = query.Encode()
		response, requestErr := management.managementJSON(ctx, client, http.MethodGet, endpoint.String(), headers, nil)
		if requestErr != nil {
			return nil, requestErr
		}
		data := envelopeData(response.payload)
		items := upstreamManagementItems(data)
		for _, item := range items {
			row, ok := item.(map[string]any)
			if !ok {
				continue
			}
			key := normalizeUpstreamForwardingKey(firstString(row, "key", "api_key", "token"))
			if key == "" {
				continue
			}
			if _, duplicate := rows[key]; duplicate {
				return nil, errors.New("upstream management API returned duplicate API keys")
			}
			rows[key] = row
		}
		if provider == UpstreamConnectionProviderOneAPI {
			if len(items) == 0 {
				return rows, nil
			}
			continue
		}
		totalCount := int64FromMap(data, "total_count", "total")
		returnedPage := int64FromMap(data, "page")
		returnedSize := int64FromMap(data, "size", "page_size")
		if totalCount > 0 && returnedPage > 0 && returnedSize > 0 && returnedPage*returnedSize >= totalCount {
			return rows, nil
		}
		if len(items) < pageSize {
			return rows, nil
		}
	}
	return nil, fmt.Errorf("upstream token listing exceeded %d pages", maxPages)
}

func normalizeUpstreamForwardingKey(value string) string {
	return strings.TrimPrefix(strings.TrimSpace(value), "sk-")
}

func (i *upstreamConnectionInspector) prepareSub2APIKeyResolver(
	ctx context.Context,
	client *http.Client,
	connection *UpstreamConnection,
	credential upstreamConnectionCredential,
) (upstreamConnectionKeyResolver, error) {
	management := &upstreamManagementClient{client: client}
	accessToken := strings.TrimSpace(credential.AccessToken)
	headers := upstreamManagementRequestHeaders(credential.UserAgent)
	if connection.AuthMode == string(UpstreamManagementAuthModePassword) {
		login, err := management.managementJSON(ctx, client, http.MethodPost,
			upstreamConnectionJoinEndpoint(connection.ManagementBaseURL, "/api/v1/auth/login", false), headers,
			sub2APIManagementLoginBody(credential))
		if err != nil {
			return nil, err
		}
		accessToken = firstString(envelopeData(login.payload), "access_token", "token", "jwt")
	}
	if accessToken == "" {
		return nil, errors.New("Sub2API management login did not return an access token")
	}
	headers.Set("Authorization", "Bearer "+accessToken)
	rows, err := listSub2APIKeyRows(ctx, management, client, connection.ManagementBaseURL, headers)
	if err != nil {
		return nil, err
	}
	return func(_ context.Context, apiKey string) (UpstreamAccountBinding, error) {
		row, ok := rows[strings.TrimSpace(apiKey)]
		if !ok {
			return UpstreamAccountBinding{}, fmt.Errorf("%w: upstream API key was not found under the configured management user", ErrUpstreamAPIKeyGroupUnmapped)
		}
		return i.resolveSub2APIKeyRow(connection, row)
	}, nil
}

func (i *upstreamConnectionInspector) resolveSub2APIKeyRow(
	connection *UpstreamConnection,
	row map[string]any,
) (UpstreamAccountBinding, error) {
	groupID := int64FromMap(row, "group_id", "groupId")
	groupIDString := ""
	if groupID > 0 {
		groupIDString = strconv.FormatInt(groupID, 10)
	}
	now := i.now().UTC()
	binding := UpstreamAccountBinding{
		RemoteTokenID:   strconv.FormatInt(int64FromMap(row, "id", "key_id"), 10),
		RemoteTokenName: strings.TrimSpace(firstString(row, "name")),
		ResolutionKind:  UpstreamBindingResolutionFixed, RemoteGroupID: groupIDString,
		FallbackGroups: []string{}, Confidence: "exact", Source: "sub2api:keys",
		ApplyPolicy: UpstreamBindingApplyObserveOnly, Status: UpstreamBindingStatusReady,
		ResolutionDetails: map[string]any{"group_id": groupID}, ObservedAt: &now,
	}
	if groupID <= 0 {
		binding.ResolutionKind = UpstreamBindingResolutionUnresolved
		binding.Status = UpstreamBindingStatusUnresolved
		binding.LastError = "upstream API key has no assigned group"
		return binding, nil
	}
	if group, found := findObservedUpstreamGroup(connection.Groups, groupIDString, ""); found {
		binding.RemoteGroupName = group.Name
		binding.ObservedMultiplier = cloneFloat64Ptr(group.RateMultiplier)
	} else {
		binding.Status = UpstreamBindingStatusUnresolved
		binding.LastError = fmt.Sprintf("resolved group id %d is absent from the latest group snapshot", groupID)
	}
	return binding, nil
}

func listSub2APIKeyRows(
	ctx context.Context,
	management *upstreamManagementClient,
	client *http.Client,
	baseURL string,
	headers http.Header,
) (map[string]map[string]any, error) {
	const (
		pageSize = 100
		maxPages = 100
	)
	rows := make(map[string]map[string]any)
	complete := false
	for page := 1; page <= maxPages; page++ {
		endpoint, err := url.Parse(upstreamConnectionJoinEndpoint(baseURL, "/api/v1/keys", false))
		if err != nil {
			return nil, err
		}
		query := endpoint.Query()
		query.Set("page", strconv.Itoa(page))
		query.Set("page_size", strconv.Itoa(pageSize))
		endpoint.RawQuery = query.Encode()
		response, err := management.managementJSON(ctx, client, http.MethodGet, endpoint.String(), headers, nil)
		if err != nil {
			return nil, err
		}
		data := envelopeData(response.payload)
		items := upstreamManagementItems(data)
		for _, item := range items {
			row, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if apiKey := strings.TrimSpace(firstString(row, "key", "api_key", "token")); apiKey != "" {
				rows[apiKey] = row
			}
		}
		pages := upstreamManagementPageCount(data)
		if pages > maxPages {
			return nil, fmt.Errorf("Sub2API key listing exceeds the %d-page safety limit", maxPages)
		}
		if len(items) < pageSize || (pages > 0 && page >= pages) {
			complete = true
			break
		}
	}
	if !complete {
		return nil, fmt.Errorf("Sub2API key listing exceeds the %d-page safety limit", maxPages)
	}
	return rows, nil
}

func findObservedUpstreamGroup(groups []UpstreamGroup, remoteID, name string) (UpstreamGroup, bool) {
	for _, group := range groups {
		if remoteID != "" && group.RemoteID == remoteID {
			return group, true
		}
		if name != "" && group.Name == name {
			return group, true
		}
	}
	return UpstreamGroup{}, false
}

func firstBool(value any, keys ...string) bool {
	row, ok := value.(map[string]any)
	if !ok {
		return false
	}
	for _, key := range keys {
		switch raw := row[key].(type) {
		case bool:
			return raw
		case string:
			parsed, err := strconv.ParseBool(strings.TrimSpace(raw))
			if err == nil {
				return parsed
			}
		}
	}
	return false
}
