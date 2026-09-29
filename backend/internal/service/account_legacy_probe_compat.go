package service

// Legacy keys are retained only for repository compatibility during upgrades.
// Cost monitoring and rate synchronization belong to UpstreamConnectionService.
const (
	UpstreamBillingProbeExtraKey           = "upstream_billing_probe"
	UpstreamBillingProbeEnabledExtraKey    = "upstream_billing_probe_enabled"
	UpstreamBillingRateSyncEnabledExtraKey = "upstream_billing_rate_sync_enabled"
)
