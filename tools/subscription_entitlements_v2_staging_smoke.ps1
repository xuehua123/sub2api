<#
.SYNOPSIS
Read-only and semi-automatic staging smoke helper for subscription entitlements v2.

.DESCRIPTION
This helper is parameterized and intentionally avoids real payment creation,
redeem mutation, gateway billing, or flag updates. It can verify read-only API
surfaces and prints the manual checklist for state-changing staging validation.
#>

[CmdletBinding()]
param(
    [string]$BaseUrl,
    [string]$AdminToken,
    [string]$UserToken,
    [long]$EntitlementId = 0,
    [long]$ExpectedAliasLegacySubscriptionId = 0,
    [ValidateSet("skip", "true", "false")]
    [string]$ExpectV2Enabled = "skip",
    [ValidateSet("skip", "true", "false")]
    [string]$ExpectLegacyMappingEnabled = "skip",
    [switch]$DryRun,
    [switch]$Help
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Show-Usage {
    @"
Subscription entitlements v2 staging smoke helper

Usage:
  powershell -NoProfile -ExecutionPolicy Bypass -File tools/subscription_entitlements_v2_staging_smoke.ps1 -DryRun
  powershell -NoProfile -ExecutionPolicy Bypass -File tools/subscription_entitlements_v2_staging_smoke.ps1 -BaseUrl https://staging.example.com -AdminToken <token> -ExpectV2Enabled false -ExpectLegacyMappingEnabled false
  powershell -NoProfile -ExecutionPolicy Bypass -File tools/subscription_entitlements_v2_staging_smoke.ps1 -BaseUrl https://staging.example.com -AdminToken <token> -UserToken <token> -EntitlementId 123 -ExpectV2Enabled true -ExpectLegacyMappingEnabled false
  powershell -NoProfile -ExecutionPolicy Bypass -File tools/subscription_entitlements_v2_staging_smoke.ps1 -BaseUrl https://staging.example.com -UserToken <token> -EntitlementId 123 -ExpectedAliasLegacySubscriptionId 456

The script does not:
  - create real payments
  - redeem codes
  - send billable gateway requests
  - change feature flags
  - print tokens

Use docs/plans/subscription-entitlements-v2-staging-rollout-runbook.md for the
manual payment, redeem, API key, gateway billing, and rollback checks.
"@ | Write-Host
}

function Write-Section {
    param([string]$Title)
    Write-Host ""
    Write-Host ("== " + $Title + " ==")
}

function Write-Check {
    param(
        [string]$Name,
        [string]$Status,
        [string]$Detail = ""
    )
    $line = ("[{0}] {1}" -f $Status, $Name)
    if ($Detail -ne "") {
        $line = $line + " - " + $Detail
    }
    Write-Host $line
}

function Require-BaseUrl {
    if ([string]::IsNullOrWhiteSpace($BaseUrl)) {
        throw "BaseUrl is required for HTTP checks. Use -DryRun for checklist-only mode."
    }
}

function Get-ApiData {
    param([object]$Response)
    if ($null -eq $Response) {
        return $null
    }
    $prop = $Response.PSObject.Properties["data"]
    if ($null -ne $prop) {
        return $prop.Value
    }
    return $Response
}

function Get-ObjectPropertyValue {
    param(
        [object]$Object,
        [string]$Name
    )
    if ($null -eq $Object) {
        return $null
    }
    $prop = $Object.PSObject.Properties[$Name]
    if ($null -eq $prop) {
        return $null
    }
    return $prop.Value
}

function As-Array {
    param([object]$Value)
    if ($null -eq $Value) {
        return @()
    }
    if ($Value -is [System.Array]) {
        return $Value
    }
    return @($Value)
}

function New-QueryString {
    param([hashtable]$Query)
    if ($null -eq $Query -or $Query.Count -eq 0) {
        return ""
    }

    $parts = @()
    foreach ($key in $Query.Keys) {
        $value = $Query[$key]
        if ($null -eq $value) {
            continue
        }
        $text = [string]$value
        if ($text -eq "") {
            continue
        }
        $parts += ([uri]::EscapeDataString([string]$key) + "=" + [uri]::EscapeDataString($text))
    }
    if ($parts.Count -eq 0) {
        return ""
    }
    return "?" + ($parts -join "&")
}

function Invoke-StagingRequest {
    param(
        [ValidateSet("GET", "POST", "PUT", "PATCH", "DELETE")]
        [string]$Method,
        [string]$Path,
        [string]$Token,
        [hashtable]$Query
    )

    Require-BaseUrl
    if ([string]::IsNullOrWhiteSpace($Token)) {
        throw "A token is required for $Method $Path."
    }

    $root = $BaseUrl.TrimEnd("/")
    if ($Path.StartsWith("/")) {
        $requestPath = $Path
    } else {
        $requestPath = "/" + $Path
    }
    $uri = $root + $requestPath + (New-QueryString -Query $Query)
    $headers = @{
        "Authorization" = "Bearer $Token"
        "Accept" = "application/json"
    }

    Write-Check -Name "$Method $requestPath" -Status "RUN"
    return Invoke-RestMethod -Method $Method -Uri $uri -Headers $headers -ContentType "application/json"
}

function Test-ExpectedBool {
    param(
        [object]$Object,
        [string]$Name,
        [string]$Expected
    )
    if ($Expected -eq "skip") {
        return
    }
    $actual = Get-ObjectPropertyValue -Object $Object -Name $Name
    if ($null -eq $actual) {
        throw "Settings response does not include $Name."
    }
    $expectedBool = ($Expected -eq "true")
    if ([bool]$actual -ne $expectedBool) {
        throw "$Name expected $expectedBool but got $actual."
    }
    Write-Check -Name "$Name is $expectedBool" -Status "PASS"
}

function Test-NoForbiddenFields {
    param(
        [object[]]$Items,
        [string[]]$ForbiddenFields
    )
    foreach ($item in $Items) {
        foreach ($field in $ForbiddenFields) {
            $prop = $item.PSObject.Properties[$field]
            if ($null -ne $prop) {
                throw "Forbidden field '$field' appeared in entitlement response."
            }
        }
    }
    Write-Check -Name "entitlement DTO forbidden fields absent" -Status "PASS"
}

function Show-ManualChecklist {
    Write-Section "Manual staging checks"
    $checks = @(
        "v2=false legacy subscription, balance, and API key requests still work.",
        "v2=true native plan payment creates or extends entitlement and writes fulfillment history.",
        "Redeem or create-and-redeem creates or extends entitlement exactly once.",
        "API key create/edit can select an entitlement group.",
        "API key same-entitlement group switch succeeds; cross-entitlement silent switch does not happen.",
        "Successful quota request increments entitlement usage and writes billing_source=entitlement_quota.",
        "Quota exceeded returns HTTP 429 and does not write a successful usage log.",
        "balance_fallback success deducts balance and writes billing_source=entitlement_balance_fallback.",
        "Fallback insufficient balance rolls back usage log, dedup, entitlement usage, and balance changes.",
        "Admin usage filters by entitlement_id and shows billing_source.",
        "/entitlements does not expose source, notes, plan_snapshot, or fulfillment fields.",
        "/subscriptions alias returns only real legacy_subscription_id aliases and never fakes id.",
        "Closing both flags restores legacy paths."
    )
    foreach ($check in $checks) {
        Write-Check -Name $check -Status "TODO"
    }
}

if ($Help) {
    Show-Usage
    exit 0
}

Write-Section "Subscription entitlements v2 staging smoke"

if ($DryRun) {
    Write-Check -Name "dry run mode" -Status "PASS" -Detail "No HTTP requests will be sent."
    Show-ManualChecklist
    exit 0
}

if ([string]::IsNullOrWhiteSpace($AdminToken) -and [string]::IsNullOrWhiteSpace($UserToken)) {
    Show-Usage
    throw "Provide AdminToken and/or UserToken for HTTP checks, or pass -DryRun."
}

if (-not [string]::IsNullOrWhiteSpace($AdminToken)) {
    Write-Section "Admin read-only checks"
    $settingsResponse = Invoke-StagingRequest -Method GET -Path "/api/v1/admin/settings" -Token $AdminToken
    $settings = Get-ApiData -Response $settingsResponse
    Test-ExpectedBool -Object $settings -Name "subscription_entitlements_v2_enabled" -Expected $ExpectV2Enabled
    Test-ExpectedBool -Object $settings -Name "sub2_payment_page_legacy_mapping_enabled" -Expected $ExpectLegacyMappingEnabled

    if ($EntitlementId -gt 0) {
        $usageQuery = @{ entitlement_id = $EntitlementId; page = 1; page_size = 20 }
        $null = Invoke-StagingRequest -Method GET -Path "/api/v1/admin/usage" -Token $AdminToken -Query $usageQuery
        Write-Check -Name "admin usage accepts entitlement_id filter" -Status "PASS"
        $null = Invoke-StagingRequest -Method GET -Path "/api/v1/admin/usage/stats" -Token $AdminToken -Query @{ entitlement_id = $EntitlementId }
        Write-Check -Name "admin usage stats accepts entitlement_id filter" -Status "PASS"
    } else {
        Write-Check -Name "admin usage entitlement_id checks" -Status "SKIP" -Detail "Pass -EntitlementId to run."
    }
}

if (-not [string]::IsNullOrWhiteSpace($UserToken)) {
    Write-Section "User read-only checks"
    $entitlementsResponse = Invoke-StagingRequest -Method GET -Path "/api/v1/entitlements" -Token $UserToken
    $entitlements = As-Array -Value (Get-ApiData -Response $entitlementsResponse)
    Test-NoForbiddenFields -Items $entitlements -ForbiddenFields @(
        "source_id",
        "source_external_id",
        "source_redeem_code_id",
        "assigned_by",
        "notes",
        "plan_snapshot",
        "fulfillments",
        "fulfillment_history"
    )
    Write-Check -Name "user entitlement count" -Status "INFO" -Detail ([string]$entitlements.Count)

    $null = Invoke-StagingRequest -Method GET -Path "/api/v1/entitlements/active" -Token $UserToken
    Write-Check -Name "active entitlements endpoint reachable" -Status "PASS"

    if ($EntitlementId -gt 0) {
        $null = Invoke-StagingRequest -Method GET -Path ("/api/v1/entitlements/{0}/progress" -f $EntitlementId) -Token $UserToken
        Write-Check -Name "entitlement progress endpoint reachable" -Status "PASS"
    } else {
        Write-Check -Name "entitlement progress endpoint" -Status "SKIP" -Detail "Pass -EntitlementId to run."
    }

    $subscriptionsResponse = Invoke-StagingRequest -Method GET -Path "/api/v1/subscriptions" -Token $UserToken
    $subscriptions = As-Array -Value (Get-ApiData -Response $subscriptionsResponse)
    $aliasCount = 0
    $expectedAliasFound = $false
    foreach ($sub in $subscriptions) {
        $entitlementAliasID = Get-ObjectPropertyValue -Object $sub -Name "entitlement_id"
        if ($null -ne $entitlementAliasID) {
            $aliasCount += 1
            $legacyID = Get-ObjectPropertyValue -Object $sub -Name "id"
            if ($null -eq $legacyID -or [string]$legacyID -eq "") {
                throw "Subscription alias with entitlement_id is missing legacy id."
            }
            if ($EntitlementId -gt 0 -and [int64]$entitlementAliasID -eq $EntitlementId) {
                $expectedAliasFound = $true
                if ($ExpectedAliasLegacySubscriptionId -gt 0 -and [int64]$legacyID -ne $ExpectedAliasLegacySubscriptionId) {
                    throw ("Subscription alias for entitlement {0} expected legacy id {1} but got {2}." -f $EntitlementId, $ExpectedAliasLegacySubscriptionId, $legacyID)
                }
            }
        }
    }
    if ($ExpectedAliasLegacySubscriptionId -gt 0 -and $EntitlementId -le 0) {
        throw "ExpectedAliasLegacySubscriptionId requires EntitlementId so the script can match the alias row."
    }
    if ($ExpectedAliasLegacySubscriptionId -gt 0 -and -not $expectedAliasFound) {
        throw ("No subscription alias row found for entitlement {0}." -f $EntitlementId)
    }
    Write-Check -Name "subscription alias rows with entitlement_id have legacy id" -Status "PASS" -Detail ([string]$aliasCount)
    if ($ExpectedAliasLegacySubscriptionId -gt 0) {
        Write-Check -Name "subscription alias uses expected legacy subscription id" -Status "PASS" -Detail ([string]$ExpectedAliasLegacySubscriptionId)
    }
}

Show-ManualChecklist
Write-Section "Result"
Write-Check -Name "read-only smoke completed" -Status "PASS"
