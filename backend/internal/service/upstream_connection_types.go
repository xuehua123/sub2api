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
)

const (
	UpstreamBindingStatusPending    = "pending"
	UpstreamBindingStatusReady      = "ready"
	UpstreamBindingStatusUnresolved = "unresolved"
	UpstreamBindingStatusError      = "error"
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
}

type UpstreamConnectionCredentialInput struct {
	Username     string `json:"username,omitempty"`
	Password     string `json:"password,omitempty"`
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	UserAgent    string `json:"-"`
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
	Version      int    `json:"version"`
	Username     string `json:"username,omitempty"`
	Password     string `json:"password,omitempty"`
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	UserAgent    string `json:"user_agent,omitempty"`
	ExpiresAt    int64  `json:"expires_at,omitempty"`
}
