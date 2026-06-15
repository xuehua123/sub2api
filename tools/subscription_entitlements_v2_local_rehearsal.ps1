<#
.SYNOPSIS
Dry-run local Docker rehearsal helper for subscription entitlements v2.

.DESCRIPTION
This helper drafts the local rehearsal commands and an optional synthetic seed
SQL template. It does not connect to production, does not export data, does not
enable remote flags, and does not print secrets.
#>

[CmdletBinding()]
param(
    [string]$ComposeFile = "deploy/docker-compose.dev.yml",
    [string]$BaseUrl = "http://127.0.0.1:8080",
    [string]$EvidenceDir = "tmp/subscription-entitlements-v2-rehearsal",
    [switch]$DryRun,
    [switch]$WriteSeedTemplate,
    [switch]$ExecuteLocal,
    [switch]$Help
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Show-Usage {
    @"
Subscription entitlements v2 local rehearsal helper

Default behavior is dry-run. Nothing is executed unless -ExecuteLocal is passed.

Examples:
  powershell -NoProfile -ExecutionPolicy Bypass -File tools/subscription_entitlements_v2_local_rehearsal.ps1 -Help
  powershell -NoProfile -ExecutionPolicy Bypass -File tools/subscription_entitlements_v2_local_rehearsal.ps1 -DryRun
  powershell -NoProfile -ExecutionPolicy Bypass -File tools/subscription_entitlements_v2_local_rehearsal.ps1 -DryRun -WriteSeedTemplate

Safety:
  - Local URLs only.
  - No production connection.
  - No real token, DB dump, or provider key output.
  - Seed templates are written under ignored tmp/ by default.
"@ | Write-Host
}

function Assert-LocalBaseUrl {
    param([string]$Url)
    if ($Url -notmatch '^https?://(127\.0\.0\.1|localhost)(:\d+)?/?$') {
        throw "BaseUrl must be local for this rehearsal helper: $Url"
    }
}

function Write-Step {
    param(
        [string]$Title,
        [string]$Command
    )
    Write-Host ""
    Write-Host ("== " + $Title + " ==")
    Write-Host $Command
}

function New-SyntheticSeedSql {
    @"
-- Synthetic subscription entitlements v2 rehearsal seed template.
-- This file is intentionally local-only. Review before execution.
-- Replace placeholder password hashes and API key values locally.

BEGIN;

INSERT INTO settings (key, value, updated_at)
VALUES
  ('subscription_entitlements_v2_enabled', 'false', NOW()),
  ('sub2_payment_page_legacy_mapping_enabled', 'false', NOW())
ON CONFLICT (key) DO UPDATE
SET value = EXCLUDED.value,
    updated_at = NOW();

INSERT INTO groups (
    name, description, rate_multiplier, is_exclusive, status,
    platform, subscription_type, daily_limit_usd, weekly_limit_usd,
    monthly_limit_usd, default_validity_days, created_at, updated_at
)
VALUES
  ('local-standard-balance', 'local rehearsal standard group', 1.0, FALSE, 'active',
   'openai', 'standard', NULL, NULL, NULL, 30, NOW(), NOW()),
  ('local-subscription-a', 'local rehearsal entitlement group A', 1.0, FALSE, 'active',
   'openai', 'subscription', 0.10, 0.50, 1.00, 30, NOW(), NOW()),
  ('local-subscription-b', 'local rehearsal entitlement group B', 1.0, FALSE, 'active',
   'openai', 'subscription', 0.10, 0.50, 1.00, 30, NOW(), NOW()),
  ('local-subscription-negative', 'local rehearsal non-covered subscription group', 1.0, FALSE, 'active',
   'openai', 'subscription', 0.10, 0.50, 1.00, 30, NOW(), NOW())
ON CONFLICT (name) DO UPDATE
SET description = EXCLUDED.description,
    platform = EXCLUDED.platform,
    subscription_type = EXCLUDED.subscription_type,
    updated_at = NOW();

-- Prefer creating users and API keys through local admin APIs.
-- If using direct SQL, replace these placeholders locally and never commit the
-- filled file:
--   <local-bcrypt-password-hash>
--   <local-api-key-secret>

COMMIT;
"@
}

function Write-SeedTemplate {
    param([string]$TargetDir)
    New-Item -ItemType Directory -Force -Path $TargetDir | Out-Null
    $seedPath = Join-Path $TargetDir "synthetic_entitlement_seed.template.sql"
    Set-Content -Path $seedPath -Value (New-SyntheticSeedSql) -Encoding ascii
    Write-Host ("Wrote seed template: " + $seedPath)
}

if ($Help) {
    Show-Usage
    exit 0
}

Assert-LocalBaseUrl -Url $BaseUrl

if (-not $ExecuteLocal) {
    $DryRun = $true
}

Write-Host "Subscription entitlements v2 local rehearsal"
Write-Host ("Mode: " + ($(if ($ExecuteLocal) { "execute-local" } else { "dry-run" })))
Write-Host ("BaseUrl: " + $BaseUrl)
Write-Host ("ComposeFile: " + $ComposeFile)
Write-Host ("EvidenceDir: " + $EvidenceDir)

Write-Step -Title "Prepare local env" -Command @"
Copy deploy/.env.example to deploy/.env and set local-only values:
  POSTGRES_PASSWORD=[REDACTED]
  JWT_SECRET=[REDACTED]
  TOTP_ENCRYPTION_KEY=[REDACTED]
  ADMIN_EMAIL=admin@sub2api.local
  ADMIN_PASSWORD=[REDACTED]
"@

Write-Step -Title "Start local Docker compose" -Command @"
docker compose -f $ComposeFile up -d --build
"@

Write-Step -Title "Wait for health" -Command @"
Invoke-WebRequest -UseBasicParsing $BaseUrl/health
"@

Write-Step -Title "Verify migrations" -Command @"
docker compose -f $ComposeFile exec -T postgres psql -U `$env:POSTGRES_USER -d `$env:POSTGRES_DB -c "SELECT filename FROM schema_migrations ORDER BY filename DESC LIMIT 10;"
"@

Write-Step -Title "Seed synthetic data" -Command @"
Review the generated synthetic seed template, then import only into the local Docker PostgreSQL container.
Do not import production dumps.
"@

Write-Step -Title "Run local smoke" -Command @"
powershell -NoProfile -ExecutionPolicy Bypass -File tools/subscription_entitlements_v2_staging_smoke.ps1 -BaseUrl $BaseUrl -AdminToken [REDACTED] -UserToken [REDACTED] -EntitlementId <local-entitlement-id> -ExpectedAliasLegacySubscriptionId <local-legacy-subscription-id> -ExpectV2Enabled true -ExpectLegacyMappingEnabled false
"@

if ($WriteSeedTemplate) {
    Write-SeedTemplate -TargetDir $EvidenceDir
}

if ($ExecuteLocal) {
    throw "ExecuteLocal is intentionally not implemented in this draft. Review the plan before adding executable steps."
}

Write-Host ""
Write-Host "Dry-run complete. No Docker command or HTTP request was executed."
