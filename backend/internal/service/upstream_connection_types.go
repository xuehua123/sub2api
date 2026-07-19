package service

import "time"

const (
	UpstreamConnectionProviderAuto     = "auto"
	UpstreamConnectionProviderNewAPI   = "newapi"
	UpstreamConnectionProviderSub2API  = "sub2api"
	UpstreamConnectionProviderRixAPI   = "rixapi"
	UpstreamConnectionProviderShellAPI = "shellapi"
	UpstreamConnectionProviderOneAPI   = "oneapi"
	UpstreamConnectionProviderVeloera  = "veloera"
	UpstreamConnectionProviderOneHub   = "onehub"
	UpstreamConnectionProviderDoneHub  = "donehub"
)

const (
	UpstreamConnectionStatusPending    = "pending"
	UpstreamConnectionStatusReady      = "ready"
	UpstreamConnectionStatusDegraded   = "degraded"
	UpstreamConnectionStatusAuthError  = "auth_error"
	UpstreamConnectionStatusNeedsInput = "needs_input"
	UpstreamConnectionStatusDisabled   = "disabled"
)

const (
	UpstreamBindingResolutionFixed            = "fixed"
	UpstreamBindingResolutionInherited        = "inherited"
	UpstreamBindingResolutionDynamic          = "dynamic"
	UpstreamBindingResolutionFallbackChain    = "fallback_chain"
	UpstreamBindingResolutionUnresolved       = "unresolved"
	UpstreamBindingResolutionLegacyUnverified = "legacy_unverified"
)

const (
	UpstreamBindingApplyObserveOnly = "observe_only"
	UpstreamBindingApplyAuto        = "auto"
)

const (
	UpstreamBindingStatusPending    = "pending"
	UpstreamBindingStatusReady      = "ready"
	UpstreamBindingStatusUnresolved = "unresolved"
	UpstreamBindingStatusError      = "error"
)

// ResolutionDetails keys that control whether an observed upstream multiplier
// is reliable enough to write into the local account billing rate.
const (
	// upstreamBindingRateConfidenceDetailKey stores the confidence of the
	// observed group rate itself (not the key→group mapping confidence).
	upstreamBindingRateConfidenceDetailKey = "rate_confidence"

	// upstreamGroupRateConfidenceOverride is a user-specific rate from
	// Sub2API /groups/rates (or equivalent). Auto-sync is allowed.
	upstreamGroupRateConfidenceOverride = "override"
	// upstreamGroupRateConfidenceDefault is an authenticated upstream default
	// group rate (Sub2API available when rates succeeded without that group,
	// or NewAPI self_groups / authenticated group maps). Auto-sync is allowed.
	upstreamGroupRateConfidenceDefault = "default"
	// upstreamGroupRateConfidenceUnavailable means the user-specific rates
	// probe failed or only public pricing was available. Display only.
	upstreamGroupRateConfidenceUnavailable = "unavailable"
	// upstreamGroupRateConfidenceUnknown means no usable multiplier was found.
	upstreamGroupRateConfidenceUnknown = "unknown"
)

// UpstreamConnection is the service-layer view of one reusable upstream
// management identity. CredentialEncrypted never leaves the service/repository
// boundary and must not be serialized by handlers.
type UpstreamConnection struct {
	ID                    int64
	Name                  string
	Provider              string
	AuthMode              string
	ManagementBaseURL     string
	ForwardingBaseURL     string
	CredentialEncrypted   string
	CredentialFingerprint string
	CredentialHint        string
	NotInCNConfirmed      bool
	RemoteUserID          string
	ProxyID               *int64
	Capabilities          map[string]any
	Status                string
	LastError             string
	SyncEnabled           bool
	SyncIntervalSeconds   int
	SyncFailures          int
	Version               int64
	WalletAmount          *float64
	WalletCurrency        string
	WalletUSD             *float64
	WalletUnlimited       bool
	WalletSource          string
	WalletReliability     string
	WalletRaw             map[string]any
	WalletObservedAt      *time.Time
	LastDiscoveredAt      *time.Time
	LastSyncedAt          *time.Time
	NextSyncAt            *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
	GroupCount            int
	BindingCount          int
	BoundAccountIDs       []int64
	Groups                []UpstreamGroup
	Bindings              []UpstreamAccountBinding
}

type UpstreamGroup struct {
	ID             int64
	ConnectionID   int64
	RemoteID       string
	Name           string
	RateMultiplier *float64
	Source         string
	Confidence     string
	Metadata       map[string]any
	ObservedAt     *time.Time
	FreshUntil     *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type UpstreamAccountBinding struct {
	ID                 int64
	AccountID          int64
	ConnectionID       int64
	KeyFingerprint     string
	RemoteTokenID      string
	RemoteTokenName    string
	ResolutionKind     string
	RemoteGroupID      string
	RemoteGroupName    string
	FallbackGroups     []string
	ObservedMultiplier *float64
	Confidence         string
	Source             string
	ApplyPolicy        string
	Status             string
	SyncFailures       int
	LastError          string
	ResolutionDetails  map[string]any
	ObservedAt         *time.Time
	FreshUntil         *time.Time
	NextSyncAt         *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type UpstreamConnectionListParams struct {
	Page     int
	PageSize int
	Provider string
	Status   string
	Search   string
	// IncludeBindings is opt-in because connection list views normally only need
	// bound account IDs. The account table requests the observation details.
	IncludeBindings bool
}

type UpstreamConnectionCredentialInput struct {
	Username         string `json:"username,omitempty"`
	Password         string `json:"password,omitempty"`
	AccessToken      string `json:"access_token,omitempty"`
	RefreshToken     string `json:"refresh_token,omitempty"`
	NotInCNConfirmed bool   `json:"not_in_cn_confirmed,omitempty"`
	UserAgent        string `json:"-"`
	// ExpiresAt is internal token lifetime state and is deliberately excluded
	// from admin request decoding so callers cannot forge it.
	ExpiresAt int64 `json:"-"`
}

type UpstreamConnectionCreateParams struct {
	Name                string
	Provider            string
	AuthMode            string
	ManagementBaseURL   string
	ForwardingBaseURL   string
	Credential          UpstreamConnectionCredentialInput
	NotInCNConfirmed    bool
	RemoteUserID        string
	ProxyID             *int64
	SyncEnabled         bool
	SyncIntervalSeconds int
}

type UpstreamConnectionUpdateParams struct {
	ExpectedVersion     int64
	Name                *string
	Provider            *string
	AuthMode            *string
	ManagementBaseURL   *string
	ForwardingBaseURL   *string
	Credential          *UpstreamConnectionCredentialInput
	NotInCNConfirmed    *bool
	RemoteUserID        *string
	ProxyID             *int64
	ClearProxy          bool
	SyncEnabled         *bool
	SyncIntervalSeconds *int
}

type UpstreamConnectionProbePersistence struct {
	RemoteUserID      string
	Capabilities      map[string]any
	Status            string
	LastError         string
	SyncFailures      int
	Version           int64
	WalletObserved    bool
	WalletAmount      *float64
	WalletCurrency    string
	WalletUSD         *float64
	WalletUnlimited   bool
	WalletSource      string
	WalletReliability string
	WalletRaw         map[string]any
	WalletObservedAt  *time.Time
	GroupsObserved    bool
	Groups            []UpstreamGroup
	LastDiscoveredAt  *time.Time
	LastSyncedAt      *time.Time
	NextSyncAt        *time.Time
}

type UpstreamConnectionProbeFailure struct {
	Status       string
	LastError    string
	SyncFailures int
	Version      int64
	NextSyncAt   *time.Time
}

type UpstreamConnectionCredentialPersistence struct {
	CredentialEncrypted   string
	CredentialFingerprint string
	CredentialHint        string
	Version               int64
}

type upstreamConnectionCredential struct {
	Version          int    `json:"version"`
	Username         string `json:"username,omitempty"`
	Password         string `json:"password,omitempty"`
	AccessToken      string `json:"access_token,omitempty"`
	RefreshToken     string `json:"refresh_token,omitempty"`
	NotInCNConfirmed bool   `json:"not_in_cn_confirmed,omitempty"`
	UserAgent        string `json:"user_agent,omitempty"`
	ExpiresAt        int64  `json:"expires_at,omitempty"`
}

// UpstreamConnectionUsageStats is a read-only aggregate derived from usage_logs.
// AccountCost is the upstream account cost and UserCost is the customer/API-key charge.
type UpstreamConnectionUsageStats struct {
	Requests     int64   `json:"requests"`
	Tokens       int64   `json:"tokens"`
	AccountCost  float64 `json:"account_cost"`
	StandardCost float64 `json:"standard_cost"`
	UserCost     float64 `json:"user_cost"`
}

type UpstreamConnectionUsagePoint struct {
	Bucket time.Time `json:"bucket"`
	UpstreamConnectionUsageStats
}

// UpstreamConnectionAccountUsageBucket is the repository projection used to
// build both per-account and whole-connection hourly series in one query.
type UpstreamConnectionAccountUsageBucket struct {
	AccountID int64
	Bucket    time.Time
	UpstreamConnectionUsageStats
}

type UpstreamConnectionAccountUsage struct {
	BindingID          int64                          `json:"binding_id"`
	AccountID          int64                          `json:"account_id"`
	AccountName        string                         `json:"account_name"`
	RemoteTokenID      string                         `json:"remote_token_id"`
	RemoteTokenName    string                         `json:"remote_token_name"`
	RemoteGroupName    string                         `json:"remote_group_name"`
	ResolutionKind     string                         `json:"resolution_kind"`
	ObservedMultiplier *float64                       `json:"observed_multiplier"`
	Status             string                         `json:"status"`
	Stats              UpstreamConnectionUsageStats   `json:"stats"`
	Trend              []UpstreamConnectionUsagePoint `json:"trend"`
}

type UpstreamConnectionTodayUsage struct {
	ConnectionID int64                            `json:"connection_id"`
	Timezone     string                           `json:"timezone"`
	StartAt      time.Time                        `json:"start_at"`
	EndAt        time.Time                        `json:"end_at"`
	Summary      UpstreamConnectionUsageStats     `json:"summary"`
	Trend        []UpstreamConnectionUsagePoint   `json:"trend"`
	Accounts     []UpstreamConnectionAccountUsage `json:"accounts"`
}
