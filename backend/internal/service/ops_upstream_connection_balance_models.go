package service

import "time"

type OpsUpstreamConnectionBalanceSettings struct {
	Enabled             bool                                             `json:"enabled"`
	DefaultThresholdUSD float64                                          `json:"default_threshold_usd"`
	RateLimitPerHour    int                                              `json:"rate_limit_per_hour"`
	Notification        OpsUpstreamConnectionBalanceNotificationSettings `json:"notification"`
}

type OpsUpstreamConnectionBalanceNotificationSettings struct {
	EnterpriseWeChatEnabled    bool   `json:"enterprise_wechat_enabled"`
	EnterpriseWeChatWebhookURL string `json:"enterprise_wechat_webhook_url,omitempty"`
	MentionAllOnLowBalance     bool   `json:"mention_all_on_low_balance"`
}

type OpsUpstreamConnectionBalanceOverride struct {
	Enabled      bool     `json:"enabled"`
	ThresholdUSD *float64 `json:"threshold_usd,omitempty"`
}

type OpsUpstreamConnectionBalanceAlertState struct {
	Enabled       bool       `json:"enabled"`
	ThresholdUSD  float64    `json:"threshold_usd"`
	UsesDefault   bool       `json:"uses_default_threshold"`
	Eligible      bool       `json:"eligible"`
	SnapshotFresh bool       `json:"snapshot_fresh"`
	Low           bool       `json:"low"`
	NotifiedAt    *time.Time `json:"notified_at,omitempty"`
}

type OpsUpstreamConnectionBalanceItem struct {
	ConnectionID        int64                                  `json:"connection_id"`
	Name                string                                 `json:"name"`
	Provider            string                                 `json:"provider"`
	Status              string                                 `json:"status"`
	LastError           string                                 `json:"last_error,omitempty"`
	SyncEnabled         bool                                   `json:"sync_enabled"`
	SyncIntervalSeconds int                                    `json:"sync_interval_seconds"`
	BindingCount        int                                    `json:"binding_count"`
	BoundAccountIDs     []int64                                `json:"bound_account_ids"`
	WalletAmount        *float64                               `json:"wallet_amount,omitempty"`
	WalletCurrency      string                                 `json:"wallet_currency,omitempty"`
	WalletUSD           *float64                               `json:"wallet_usd,omitempty"`
	WalletUnlimited     bool                                   `json:"wallet_unlimited"`
	WalletSource        string                                 `json:"wallet_source,omitempty"`
	WalletReliability   string                                 `json:"wallet_reliability,omitempty"`
	WalletObservedAt    *time.Time                             `json:"wallet_observed_at,omitempty"`
	Alert               OpsUpstreamConnectionBalanceAlertState `json:"alert"`
}

type OpsUpstreamConnectionBalanceSummary struct {
	TotalConnections      int `json:"total_connections"`
	MonitoredConnections  int `json:"monitored_connections"`
	LowBalanceConnections int `json:"low_balance_connections"`
	FailedConnections     int `json:"failed_connections"`
	StaleConnections      int `json:"stale_connections"`
	UnlimitedConnections  int `json:"unlimited_connections"`
}

type OpsUpstreamConnectionBalanceListResponse struct {
	GeneratedAt time.Time                            `json:"generated_at"`
	Settings    OpsUpstreamConnectionBalanceSettings `json:"settings"`
	Items       []OpsUpstreamConnectionBalanceItem   `json:"items"`
	Total       int64                                `json:"total"`
	Page        int                                  `json:"page"`
	PageSize    int                                  `json:"page_size"`
	Summary     OpsUpstreamConnectionBalanceSummary  `json:"summary"`
}

type OpsUpstreamConnectionBalanceMonitorFilter struct {
	Page       int
	PageSize   int
	Search     string
	Status     string
	OnlyLow    bool
	OnlyFailed bool
}

type OpsUpstreamConnectionBalanceAlertUpdate struct {
	Enabled             *bool    `json:"enabled"`
	ThresholdUSD        *float64 `json:"threshold_usd"`
	UseDefaultThreshold *bool    `json:"use_default_threshold"`
}
