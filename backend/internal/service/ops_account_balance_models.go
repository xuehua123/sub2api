package service

import (
	"context"
	"time"
)

const (
	AccountBalanceProbeMethodAuto     = "auto"
	AccountBalanceProbeMethodDisabled = "disabled"
	// AccountBalanceProbeMethodUpstreamManagement queries an upstream user's
	// management profile. Unlike an API key usage endpoint, it represents the
	// upstream account wallet.
	AccountBalanceProbeMethodUpstreamManagement = "upstream_management"
	AccountBalanceProbeMethodNewAPITokenUsage   = "newapi_token_usage"
	AccountBalanceProbeMethodSub2APIUsage       = "sub2api_usage"
	AccountBalanceProbeMethodOpenAIBilling      = "openai_billing"

	AccountBalanceProbeStatusUnknown     = "unknown"
	AccountBalanceProbeStatusOK          = "ok"
	AccountBalanceProbeStatusFailed      = "failed"
	AccountBalanceProbeStatusUnsupported = "unsupported"
	AccountBalanceProbeStatusSkipped     = "skipped"
)

const (
	accountBalanceProbeMethodExtraKey          = "balance_probe_method"
	accountBalanceProbeEnabledExtraKey         = "balance_probe_enabled"
	accountBalanceProbeThresholdUSDExtraKey    = "balance_probe_threshold_usd"
	accountBalanceProbeDetectedMethodExtraKey  = "balance_probe_detected_method"
	accountBalanceProbeStatusExtraKey          = "balance_probe_status"
	accountBalanceProbeErrorExtraKey           = "balance_probe_error"
	accountBalanceProbeCheckedAtExtraKey       = "balance_probe_checked_at"
	accountBalanceProbeBalanceUSDExtraKey      = "balance_probe_balance_usd"
	accountBalanceProbeBalanceAmountExtraKey   = "balance_probe_balance_amount"
	accountBalanceProbeBalanceCurrencyExtraKey = "balance_probe_balance_currency"
	accountBalanceProbeUnlimitedExtraKey       = "balance_probe_unlimited"
	accountBalanceProbeEndpointExtraKey        = "balance_probe_endpoint"
	accountBalanceProbeTotalUsedUSDExtraKey    = "balance_probe_total_used_usd"
	accountBalanceProbeGrantedUSDExtraKey      = "balance_probe_total_granted_usd"
	accountBalanceProbeNotifiedAtExtraKey      = "balance_probe_notified_at"
)

type OpsAccountBalanceSettings struct {
	Enabled      bool                                  `json:"enabled"`
	Probe        OpsAccountBalanceProbeSettings        `json:"probe"`
	Notification OpsAccountBalanceNotificationSettings `json:"notification"`

	DefaultThresholdUSD float64 `json:"default_threshold_usd"`
	RateLimitPerHour    int     `json:"rate_limit_per_hour"`
}

type OpsAccountBalanceProbeSettings struct {
	IntervalMinutes int      `json:"interval_minutes"`
	MaxPerRun       int      `json:"max_per_run"`
	TimeoutSeconds  int      `json:"timeout_seconds"`
	OnlySchedulable bool     `json:"only_schedulable"`
	MethodOrder     []string `json:"method_order"`
}

type OpsAccountBalanceNotificationSettings struct {
	EnterpriseWeChatEnabled    bool   `json:"enterprise_wechat_enabled"`
	EnterpriseWeChatWebhookURL string `json:"enterprise_wechat_webhook_url,omitempty"`
	MentionAllOnLowBalance     bool   `json:"mention_all_on_low_balance"`
}

type OpsAccountBalanceState struct {
	AccountID       int64      `json:"account_id"`
	Method          string     `json:"method"`
	Enabled         bool       `json:"enabled"`
	ThresholdUSD    *float64   `json:"threshold_usd,omitempty"`
	DetectedMethod  string     `json:"detected_method,omitempty"`
	Status          string     `json:"status"`
	Error           string     `json:"error,omitempty"`
	BalanceUSD      *float64   `json:"balance_usd,omitempty"`
	BalanceAmount   *float64   `json:"balance_amount,omitempty"`
	BalanceCurrency string     `json:"balance_currency,omitempty"`
	Unlimited       bool       `json:"unlimited"`
	Endpoint        string     `json:"endpoint,omitempty"`
	TotalUsedUSD    *float64   `json:"total_used_usd,omitempty"`
	TotalGrantedUSD *float64   `json:"total_granted_usd,omitempty"`
	CheckedAt       *time.Time `json:"checked_at,omitempty"`
	NotifiedAt      *time.Time `json:"notified_at,omitempty"`
}

type OpsAccountBalanceAccountItem struct {
	AccountID    int64                  `json:"account_id"`
	AccountName  string                 `json:"account_name"`
	Platform     string                 `json:"platform"`
	Type         string                 `json:"type"`
	Status       string                 `json:"status"`
	Schedulable  bool                   `json:"schedulable"`
	GroupIDs     []int64                `json:"group_ids,omitempty"`
	BalanceProbe OpsAccountBalanceState `json:"balance_probe"`
}

type OpsAccountBalanceListResponse struct {
	GeneratedAt time.Time                      `json:"generated_at"`
	Settings    OpsAccountBalanceSettings      `json:"settings"`
	Items       []OpsAccountBalanceAccountItem `json:"items"`
	Total       int64                          `json:"total"`
	Page        int                            `json:"page"`
	PageSize    int                            `json:"page_size"`
	Summary     OpsAccountBalanceSummary       `json:"summary"`
}

type OpsAccountBalanceSummary struct {
	TotalAccounts     int `json:"total_accounts"`
	KnownBalanceCount int `json:"known_balance_count"`
	LowBalanceCount   int `json:"low_balance_count"`
	FailedCount       int `json:"failed_count"`
	UnsupportedCount  int `json:"unsupported_count"`
	UnlimitedCount    int `json:"unlimited_count"`
	DisabledCount     int `json:"disabled_count"`
	DueCount          int `json:"due_count"`
}

type OpsAccountBalanceMonitorFilter struct {
	Page            int
	PageSize        int
	Platform        string
	Status          string
	ProbeStatus     string
	Search          string
	Method          string
	OnlyDue         bool
	OnlyLow         bool
	OnlyFailed      bool
	OnlySchedulable bool
	SortBy          string
	SortOrder       string
}

type OpsAccountBalanceProbeConfigUpdate struct {
	Method              *string  `json:"method"`
	Enabled             *bool    `json:"enabled"`
	ThresholdUSD        *float64 `json:"threshold_usd"`
	UseDefaultThreshold *bool    `json:"use_default_threshold"`
}

type OpsAccountBalanceProbeResult struct {
	AccountID int64                           `json:"account_id"`
	State     OpsAccountBalanceState          `json:"state"`
	Attempts  []OpsAccountBalanceProbeAttempt `json:"attempts"`
}

type OpsAccountBalanceProbeAttempt struct {
	Method   string `json:"method"`
	Endpoint string `json:"endpoint"`
	Status   string `json:"status"`
	Message  string `json:"message,omitempty"`
}

type opsAccountBalanceMethodResult struct {
	Method          string
	Endpoint        string
	BalanceUSD      *float64
	BalanceAmount   *float64
	BalanceCurrency string
	Unlimited       bool
	TotalUsedUSD    *float64
	TotalGrantedUSD *float64
}

type upstreamAccountBalanceFetcher interface {
	FetchAccountBalance(ctx context.Context, account *Account) (opsAccountBalanceMethodResult, error)
}
