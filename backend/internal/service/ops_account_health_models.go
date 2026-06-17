package service

import "time"

const (
	OpsAccountHealthWindow1m  = "1m"
	OpsAccountHealthWindow5m  = "5m"
	OpsAccountHealthWindow10m = "10m"
	OpsAccountHealthWindow30m = "30m"
	OpsAccountHealthWindow1h  = "1h"
)

const (
	OpsAccountHealthActionKeepOpen    = "keep_open"
	OpsAccountHealthActionWatch       = "watch"
	OpsAccountHealthActionCloseNow    = "close_now"
	OpsAccountHealthActionCanOpen     = "can_open"
	OpsAccountHealthActionNeedsProbe  = "needs_probe"
	OpsAccountHealthActionKeepClosed  = "keep_closed"
	OpsAccountHealthActionUnavailable = "unavailable"
)

const (
	OpsAccountHealthNotifyNone      = "none"
	OpsAccountHealthNotifyDigest    = "digest"
	OpsAccountHealthNotifyImmediate = "immediate"
)

type OpsAccountHealthFilter struct {
	Platform string
	GroupID  *int64

	RecentLimit int

	StartTime time.Time
	EndTime   time.Time
}

type OpsAccountHealthWindowStats struct {
	Window string `json:"window"`

	RequestCount       int64 `json:"request_count"`
	SuccessCount       int64 `json:"success_count"`
	ErrorCount         int64 `json:"error_count"`
	UpstreamErrorCount int64 `json:"upstream_error_count"`
	Status429Count     int64 `json:"status_429_count"`
	Status529Count     int64 `json:"status_529_count"`

	SuccessRatePercent       float64  `json:"success_rate_percent"`
	ErrorRatePercent         float64  `json:"error_rate_percent"`
	UpstreamErrorRatePercent float64  `json:"upstream_error_rate_percent"`
	AvgDurationMs            *float64 `json:"avg_duration_ms,omitempty"`
}

type OpsAccountHealthFirstTokenStats struct {
	Window      string   `json:"window"`
	SampleCount int64    `json:"sample_count"`
	AvgMs       *float64 `json:"avg_ms,omitempty"`
}

type OpsAccountHealthSample struct {
	Kind      string    `json:"kind"`
	CreatedAt time.Time `json:"created_at"`

	RequestID string `json:"request_id,omitempty"`
	Model     string `json:"model,omitempty"`

	DurationMs *int   `json:"duration_ms,omitempty"`
	StatusCode *int   `json:"status_code,omitempty"`
	Message    string `json:"message,omitempty"`
}

type OpsAccountHealthMetrics struct {
	AccountID         int64
	Windows           map[string]*OpsAccountHealthWindowStats
	Recent            []*OpsAccountHealthSample
	FirstToken5m      *OpsAccountHealthFirstTokenStats
	FirstTokenWindows map[string]*OpsAccountHealthFirstTokenStats
}

type OpsAccountHealthProbe struct {
	Status       string     `json:"status"`
	CheckedAt    *time.Time `json:"checked_at,omitempty"`
	LatencyMs    *int64     `json:"latency_ms,omitempty"`
	ModelID      string     `json:"model_id,omitempty"`
	ErrorMessage string     `json:"error_message,omitempty"`

	RequestCount       int64                     `json:"request_count,omitempty"`
	SuccessCount       int64                     `json:"success_count,omitempty"`
	ErrorCount         int64                     `json:"error_count,omitempty"`
	SuccessRatePercent float64                   `json:"success_rate_percent,omitempty"`
	ErrorRatePercent   float64                   `json:"error_rate_percent,omitempty"`
	AvgLatencyMs       *float64                  `json:"avg_latency_ms,omitempty"`
	Recent             []*OpsAccountHealthSample `json:"recent,omitempty"`
}

type OpsAccountHealthRecommendation struct {
	Action        string `json:"action"`
	Severity      string `json:"severity"`
	Title         string `json:"title"`
	Reason        string `json:"reason"`
	NotifyMode    string `json:"notify_mode"`
	Immediate     bool   `json:"immediate"`
	RecoveryReady bool   `json:"recovery_ready"`
}

type OpsAccountHealthItem struct {
	AccountID   int64    `json:"account_id"`
	AccountName string   `json:"account_name"`
	Platform    string   `json:"platform"`
	GroupID     int64    `json:"group_id"`
	GroupName   string   `json:"group_name"`
	Tags        []string `json:"tags"`

	Status string `json:"status"`

	IsOpened            bool `json:"is_opened"`
	IsSchedulable       bool `json:"is_schedulable"`
	IsAvailable         bool `json:"is_available"`
	IsRateLimited       bool `json:"is_rate_limited"`
	IsOverloaded        bool `json:"is_overloaded"`
	IsTempUnschedulable bool `json:"is_temp_unschedulable"`
	HasError            bool `json:"has_error"`
	ProbeAutoDisabled   bool `json:"probe_auto_disabled"`

	ProbeModelID        string `json:"probe_model_id,omitempty"`
	ProbeModelEffective string `json:"probe_model_effective,omitempty"`

	RateLimitResetAt       *time.Time `json:"rate_limit_reset_at,omitempty"`
	RateLimitRemainingSec  *int64     `json:"rate_limit_remaining_sec,omitempty"`
	OverloadUntil          *time.Time `json:"overload_until,omitempty"`
	OverloadRemainingSec   *int64     `json:"overload_remaining_sec,omitempty"`
	TempUnschedulableUntil *time.Time `json:"temp_unschedulable_until,omitempty"`
	ErrorMessage           string     `json:"error_message,omitempty"`

	Windows           map[string]*OpsAccountHealthWindowStats     `json:"windows"`
	Recent            []*OpsAccountHealthSample                   `json:"recent"`
	FirstToken5m      *OpsAccountHealthFirstTokenStats            `json:"first_token_5m,omitempty"`
	FirstTokenWindows map[string]*OpsAccountHealthFirstTokenStats `json:"first_token_windows,omitempty"`
	Probe             *OpsAccountHealthProbe                      `json:"probe,omitempty"`
	Recommendation    OpsAccountHealthRecommendation              `json:"recommendation"`
}

type OpsAccountHealthResponse struct {
	Enabled     bool                     `json:"enabled"`
	GeneratedAt time.Time                `json:"generated_at"`
	Items       []*OpsAccountHealthItem  `json:"items"`
	Settings    OpsAccountHealthSettings `json:"settings"`
}

type OpsAccountHealthProbeAutoState struct {
	AccountID         int64 `json:"account_id"`
	ProbeAutoDisabled bool  `json:"probe_auto_disabled"`
}

type OpsAccountHealthProbeModelState struct {
	AccountID           int64  `json:"account_id"`
	ProbeModelID        string `json:"probe_model_id,omitempty"`
	ProbeModelEffective string `json:"probe_model_effective"`
	GlobalProbeModelID  string `json:"global_probe_model_id"`
	DefaultProbeModelID string `json:"default_probe_model_id"`
	InheritsGlobalModel bool   `json:"inherits_global_model"`
	HasAccountOverride  bool   `json:"has_account_override"`
}

type OpsAccountHealthSettings struct {
	Enabled bool   `json:"enabled"`
	Mode    string `json:"mode"`

	Burst    OpsAccountHealthBurstSettings    `json:"burst"`
	Degrade  OpsAccountHealthDegradeSettings  `json:"degrade"`
	Recovery OpsAccountHealthRecoverySettings `json:"recovery"`
	Probe    OpsAccountHealthProbeSettings    `json:"probe"`

	Notification OpsAccountHealthNotificationSettings `json:"notification"`

	RateLimitPerHour int `json:"rate_limit_per_hour"`
}

type OpsAccountHealthNotificationSettings struct {
	EnterpriseWeChatEnabled    bool   `json:"enterprise_wechat_enabled"`
	EnterpriseWeChatWebhookURL string `json:"enterprise_wechat_webhook_url,omitempty"`
	MentionAllOnImmediate      bool   `json:"mention_all_on_immediate"`
}

type OpsAccountHealthProbeSettings struct {
	Enabled         bool   `json:"enabled"`
	IntervalMinutes int    `json:"interval_minutes"`
	MaxPerRun       int    `json:"max_per_run"`
	TimeoutSeconds  int    `json:"timeout_seconds"`
	ModelID         string `json:"model_id,omitempty"`
	Mode            string `json:"mode,omitempty"`
	Prompt          string `json:"prompt,omitempty"`
}

type OpsAccountHealthBurstSettings struct {
	Enabled                  bool    `json:"enabled"`
	WindowMinutes            int     `json:"window_minutes"`
	MinRequests              int     `json:"min_requests"`
	ErrorRatePercent         float64 `json:"error_rate_percent"`
	UpstreamErrorRatePercent float64 `json:"upstream_error_rate_percent"`
	CooldownMinutes          int     `json:"cooldown_minutes"`
	BypassDigest             bool    `json:"bypass_digest"`
}

type OpsAccountHealthDegradeSettings struct {
	Enabled                  bool    `json:"enabled"`
	WindowMinutes            int     `json:"window_minutes"`
	MinRequests              int     `json:"min_requests"`
	SuccessRateMinPercent    float64 `json:"success_rate_min_percent"`
	ErrorRatePercent         float64 `json:"error_rate_percent"`
	UpstreamErrorRatePercent float64 `json:"upstream_error_rate_percent"`
	CooldownMinutes          int     `json:"cooldown_minutes"`
}

type OpsAccountHealthRecoverySettings struct {
	Enabled               bool    `json:"enabled"`
	WindowMinutes         int     `json:"window_minutes"`
	MinRequests           int     `json:"min_requests"`
	SuccessRateMinPercent float64 `json:"success_rate_min_percent"`
	NotifyOpenedAccounts  bool    `json:"notify_opened_accounts"`
	NotifyClosedAccounts  bool    `json:"notify_closed_accounts"`
	CooldownMinutes       int     `json:"cooldown_minutes"`
}
