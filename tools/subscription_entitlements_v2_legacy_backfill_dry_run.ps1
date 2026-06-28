<#
.SYNOPSIS
Read-only dry-run helper for legacy subscription -> entitlement v2 backfill.

.DESCRIPTION
Runs the pre-write sections of
docs/plans/subscription-entitlements-v2-legacy-backfill-dry-run.sql through psql
inside a read-only transaction, redacts sensitive output, and optionally writes
evidence files. It never performs writes and does not enable feature flags.
#>

[CmdletBinding()]
param(
    [Alias("Env")]
    [ValidateSet("local", "staging", "production")]
    [string]$Environment = "staging",

    [switch]$DryRun,
    [switch]$Help,
    [switch]$ValidateSqlOnly,
    [switch]$RedactionSelfTest,
    [switch]$WritePostWriteReconciliationSql,

    [string]$SqlPath = "docs/plans/subscription-entitlements-v2-legacy-backfill-dry-run.sql",
    [string]$ConnectionString,
    [string]$ConnectionStringEnvVar = "DATABASE_URL",
    [string]$OutputDir,
    [string]$PsqlPath = "psql",
    [int[]]$Sections
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$writeVerbPattern = "\b(INSERT|UPDATE|DELETE|ALTER|CREATE|DROP|TRUNCATE|MERGE|GRANT|REVOKE|COPY)\b"
$defaultExecutableSections = @(1, 2, 3, 4, 5, 6)
$postWriteSection = 7

function Show-Usage {
    @"
Legacy subscription entitlement backfill dry-run

Usage:
  powershell -NoProfile -ExecutionPolicy Bypass -File tools/subscription_entitlements_v2_legacy_backfill_dry_run.ps1 -Help
  powershell -NoProfile -ExecutionPolicy Bypass -File tools/subscription_entitlements_v2_legacy_backfill_dry_run.ps1 -DryRun
  powershell -NoProfile -ExecutionPolicy Bypass -File tools/subscription_entitlements_v2_legacy_backfill_dry_run.ps1 -ValidateSqlOnly
  powershell -NoProfile -ExecutionPolicy Bypass -File tools/subscription_entitlements_v2_legacy_backfill_dry_run.ps1 -RedactionSelfTest
  powershell -NoProfile -ExecutionPolicy Bypass -File tools/subscription_entitlements_v2_legacy_backfill_dry_run.ps1 -Environment staging -DryRun -OutputDir tmp/legacy-backfill-evidence
  powershell -NoProfile -ExecutionPolicy Bypass -File tools/subscription_entitlements_v2_legacy_backfill_dry_run.ps1 -Environment production -DryRun -OutputDir <secure-evidence-dir>

Connection:
  Provide a connection string with -ConnectionString or set the environment
  variable named by -ConnectionStringEnvVar. The connection string is never
  printed. If no DB connection is supplied, the script prints the run plan and
  refuses SQL execution.

Safety:
  - Default mode is dry-run.
  - Production requires explicit -DryRun.
  - SQL is scanned for write/DDL verbs before execution.
  - psql execution is wrapped in BEGIN READ ONLY ... ROLLBACK.
  - Post-write reconciliation SQL can be written to evidence, but is not run.
  - Output is redacted before printing or saving.
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
    $line = "[{0}] {1}" -f $Status, $Name
    if ($Detail -ne "") {
        $line = $line + " - " + $Detail
    }
    Write-Host $line
}

function Redact-Text {
    param([string]$Text)
    if ($null -eq $Text) {
        return ""
    }

    $redacted = $Text
    $redacted = $redacted -replace '(?i)(postgres(?:ql)?://[^:\s/@]+:)([^@\s]+)(@)', '${1}[REDACTED]${3}'
    $redacted = $redacted -replace '(?i)\b(authorization\s*:\s*bearer\s+)[^\s,;]+', '${1}[REDACTED]'
    $redacted = $redacted -replace '(?i)\b(password|passwd|pwd|token|api[_-]?key|secret|jwt|totp[_-]?encryption[_-]?key)\s*[:=]\s*[^,\s;]+', '${1}=[REDACTED]'
    $redacted = $redacted -replace '(?i)\b(source_external_id|payment_source_id|notes)\s*[:=]\s*[^,\r\n;]+', '${1}=[REDACTED]'
    $redacted = $redacted -replace '\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b', '[REDACTED_EMAIL]'
    $redacted = $redacted -replace '\b(sk|sess|key|tok|pk|rk)-[A-Za-z0-9_\-]{8,}\b', '$1-[REDACTED]'
    $redacted = $redacted -replace '\beyJ[A-Za-z0-9_\-]{20,}\.[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}\b', '[REDACTED_JWT]'
    return $redacted
}

function Test-Redaction {
    $fakePassword = "fake-redaction-" + "password"
    $fakeApiKey = "key-" + "fake-redaction-sample"
    $fakeToken = "tok-" + "fake-redaction-sample"
    $fakeBearer = "sk-" + "fake-redaction-sample"
    $fakeSource = "trade-" + "secret-123"
    $fakeEmail = "user" + "@" + "example.com"
    $sampleLines = @(
        "Authorization: Bearer " + $fakeBearer,
        "postgres" + "://reader:" + $fakePassword + "@db.example.invalid:5432/sub2api",
        "api" + "_key=" + $fakeApiKey,
        "tok" + "en=" + $fakeToken,
        "pass" + "word=" + $fakePassword,
        "no" + "tes=private customer note",
        "source" + "_external_id=" + $fakeSource,
        $fakeEmail
    )
    $sample = $sampleLines -join [Environment]::NewLine
    $redacted = Redact-Text -Text $sample
    $forbidden = @(
        $fakeBearer,
        $fakePassword,
        $fakeApiKey,
        $fakeToken,
        "private customer note",
        $fakeSource,
        $fakeEmail
    )
    foreach ($value in $forbidden) {
        if ($redacted.Contains($value)) {
            throw "redaction self-test failed; sensitive sample remained: $value"
        }
    }
    Write-Check -Name "redaction self-test" -Status "PASS"
}

function Get-ResolvedSqlPath {
    param([string]$Path)
    $resolved = Resolve-Path -LiteralPath $Path -ErrorAction Stop
    return $resolved.Path
}

function Get-SqlSections {
    param([string]$Path)
    $lines = Get-Content -LiteralPath $Path
    $sections = @()
    $currentNumber = $null
    $currentTitle = $null
    $buffer = @()

    foreach ($line in $lines) {
        if ($line -match '^--\s+(\d+)\.\s+(.+?)\s*$') {
            if ($null -ne $currentNumber) {
                $sections += [pscustomobject]@{
                    Number = [int]$currentNumber
                    Title = [string]$currentTitle
                    Lines = $buffer
                }
            }
            $currentNumber = [int]$Matches[1]
            $currentTitle = [string]$Matches[2]
            $buffer = @($line)
        } elseif ($null -ne $currentNumber) {
            $buffer += $line
        }
    }

    if ($null -ne $currentNumber) {
        $sections += [pscustomobject]@{
            Number = [int]$currentNumber
            Title = [string]$currentTitle
            Lines = $buffer
        }
    }
    return $sections
}

function Assert-SqlReadOnly {
    param(
        [string]$Sql,
        [string]$Label
    )
    $lineNumber = 0
    foreach ($line in ($Sql -split "`r?`n")) {
        $lineNumber += 1
        $trimmed = $line.Trim()
        if ($trimmed -eq "" -or $trimmed.StartsWith("--")) {
            continue
        }
        if ($trimmed.StartsWith("\")) {
            throw ("SQL contains psql meta-command in {0} at line {1}: {2}" -f $Label, $lineNumber, $trimmed)
        }
        if ($trimmed -match $script:writeVerbPattern) {
            throw ("SQL contains write/DDL verb in {0} at line {1}: {2}" -f $Label, $lineNumber, $trimmed)
        }
    }
}

function Get-SectionSql {
    param(
        [object[]]$AllSections,
        [int[]]$Numbers
    )
    $selected = @()
    foreach ($section in $AllSections) {
        if ($Numbers -contains [int]$section.Number) {
            $selected += ($section.Lines -join [Environment]::NewLine)
        }
    }
    return ($selected -join ([Environment]::NewLine + [Environment]::NewLine))
}

function Get-ConnectionString {
    if (-not [string]::IsNullOrWhiteSpace($ConnectionString)) {
        return $ConnectionString
    }
    $envItem = Get-Item -Path ("Env:" + $ConnectionStringEnvVar) -ErrorAction SilentlyContinue
    if ($null -eq $envItem -or [string]::IsNullOrWhiteSpace($envItem.Value)) {
        return ""
    }
    return [string]$envItem.Value
}

function Show-RunPlan {
    param(
        [object[]]$AllSections,
        [int[]]$ExecutableSectionNumbers
    )
    Write-Section "Dry-run plan"
    Write-Check -Name "environment" -Status "INFO" -Detail $Environment
    Write-Check -Name "sql file" -Status "INFO" -Detail $SqlPath
    foreach ($section in $AllSections) {
        $status = "SKIP"
        if ($ExecutableSectionNumbers -contains [int]$section.Number) {
            $status = "RUN"
        } elseif ([int]$section.Number -eq $postWriteSection) {
            $status = "DRAFT"
        }
        Write-Check -Name ("section {0}: {1}" -f $section.Number, $section.Title) -Status $status
    }
}

function Write-PostWriteSql {
    param(
        [object[]]$AllSections,
        [string]$TargetDir
    )
    $postWriteSql = Get-SectionSql -AllSections $AllSections -Numbers @($postWriteSection)
    if ([string]::IsNullOrWhiteSpace($postWriteSql)) {
        Write-Check -Name "post-write reconciliation SQL" -Status "SKIP" -Detail "section not found"
        return
    }
    Assert-SqlReadOnly -Sql $postWriteSql -Label "post-write reconciliation"
    if ([string]::IsNullOrWhiteSpace($TargetDir)) {
        Write-Section "Post-write reconciliation SQL draft"
        Write-Host $postWriteSql
        return
    }
    New-Item -ItemType Directory -Force -Path $TargetDir | Out-Null
    $target = Join-Path $TargetDir "post-write-reconciliation-draft.sql"
    Set-Content -LiteralPath $target -Value $postWriteSql -Encoding UTF8
    Write-Check -Name "post-write reconciliation SQL draft" -Status "PASS" -Detail $target
}

function Invoke-PsqlReadOnly {
    param(
        [string]$Sql,
        [string]$DbConnectionString,
        [string]$EvidenceDir
    )

    $null = Get-Command -Name $PsqlPath -ErrorAction Stop

    $wrappedSql = @"
\set ON_ERROR_STOP on
BEGIN READ ONLY;
SET LOCAL statement_timeout = '120s';
$Sql
ROLLBACK;
"@

    Assert-SqlReadOnly -Sql $Sql -Label "executable dry-run sections"

    $tempFile = [System.IO.Path]::GetTempFileName()
    try {
        Set-Content -LiteralPath $tempFile -Value $wrappedSql -Encoding UTF8
        $args = @(
            "-X",
            "--set=ON_ERROR_STOP=1",
            "--csv",
            "--file=$tempFile",
            "--dbname=$DbConnectionString"
        )
        $output = & $PsqlPath @args 2>&1
        $exitCode = $LASTEXITCODE
        $redactedOutput = Redact-Text -Text (($output | ForEach-Object { [string]$_ }) -join [Environment]::NewLine)

        if (-not [string]::IsNullOrWhiteSpace($EvidenceDir)) {
            New-Item -ItemType Directory -Force -Path $EvidenceDir | Out-Null
            $outputPath = Join-Path $EvidenceDir "legacy-backfill-dry-run-output.csv"
            Set-Content -LiteralPath $outputPath -Value $redactedOutput -Encoding UTF8
            Write-Check -Name "redacted dry-run output" -Status "PASS" -Detail $outputPath
        } else {
            Write-Section "Redacted psql output"
            Write-Host $redactedOutput
        }

        if ($exitCode -ne 0) {
            throw "psql dry-run failed with exit code $exitCode. Output has been redacted."
        }
        Write-Check -Name "psql read-only dry-run" -Status "PASS"
    } finally {
        Remove-Item -LiteralPath $tempFile -Force -ErrorAction SilentlyContinue
    }
}

if ($Help) {
    Show-Usage
    exit 0
}

if ($RedactionSelfTest) {
    Test-Redaction
    exit 0
}

if ($Environment -eq "production" -and -not $PSBoundParameters.ContainsKey("DryRun")) {
    throw "production requires explicit -DryRun; refusing to continue"
}

$resolvedSqlPath = Get-ResolvedSqlPath -Path $SqlPath
$allSections = Get-SqlSections -Path $resolvedSqlPath
if ($allSections.Count -eq 0) {
    throw "no numbered SQL sections found in $resolvedSqlPath"
}

$executableSections = $defaultExecutableSections
if ($null -ne $Sections -and $Sections.Count -gt 0) {
    foreach ($sectionNumber in $Sections) {
        if ($sectionNumber -eq $postWriteSection) {
            throw "section $postWriteSection is post-write reconciliation SQL and is never executed by this dry-run tool"
        }
    }
    $executableSections = $Sections
}

$executableSql = Get-SectionSql -AllSections $allSections -Numbers $executableSections
Assert-SqlReadOnly -Sql $executableSql -Label "dry-run SQL"

if ($ValidateSqlOnly) {
    Write-Check -Name "dry-run SQL read-only scan" -Status "PASS" -Detail $resolvedSqlPath
    exit 0
}

if (-not $DryRun) {
    $DryRun = $true
}

Write-Section "Legacy subscription entitlement backfill dry-run"
Show-RunPlan -AllSections $allSections -ExecutableSectionNumbers $executableSections

if ($WritePostWriteReconciliationSql) {
    Write-PostWriteSql -AllSections $allSections -TargetDir $OutputDir
}

$dbConnectionString = Get-ConnectionString
if ([string]::IsNullOrWhiteSpace($dbConnectionString)) {
    Write-Check -Name "database connection" -Status "REFUSED" -Detail ("set -" + "ConnectionString or environment variable " + $ConnectionStringEnvVar + " to execute read-only SQL")
    Write-Check -Name "write path" -Status "NOT_IMPLEMENTED" -Detail "this tool is dry-run only"
    exit 0
}

Invoke-PsqlReadOnly -Sql $executableSql -DbConnectionString $dbConnectionString -EvidenceDir $OutputDir
Write-Section "Result"
Write-Check -Name "legacy backfill dry-run completed" -Status "PASS"
