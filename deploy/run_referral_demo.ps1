param(
    [string]$ComposeFile = "docker-compose.dev.yml",
    [string]$BindHost = "127.0.0.1",
    [int]$ServerPort = 8080,
    [string]$AdminEmail = "admin@sub2api.local",
    [string]$AdminPassword = "Admin123456!",
    [string]$PostgresUser = "sub2api",
    [string]$PostgresPassword = "sub2api_demo_pass",
    [string]$PostgresDb = "sub2api",
    [string]$JwtSecret = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
    [string]$TotpKey = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
    [switch]$SkipBuild,
    [switch]$ResetData
)

$ErrorActionPreference = 'Stop'

function Invoke-ApiJson {
    param(
        [string]$Method,
        [string]$Uri,
        [object]$Body = $null,
        [hashtable]$Headers = @{}
    )

    $params = @{
        Method      = $Method
        Uri         = $Uri
        Headers     = $Headers
        ContentType = 'application/json'
    }
    if ($null -ne $Body) {
        $params.Body = ($Body | ConvertTo-Json -Depth 20)
    }
    $response = Invoke-RestMethod @params
    if ($null -ne $response -and $null -ne $response.code) {
        if ($response.code -ne 0) {
            throw "API error: $($response.message)"
        }
        return $response.data
    }
    return $response
}

function Wait-HttpHealthy {
    param(
        [string]$Url,
        [int]$TimeoutSeconds = 600
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        try {
            Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 5 | Out-Null
            return
        } catch {
            Start-Sleep -Seconds 3
        }
    }
    throw "Timed out waiting for $Url"
}

function Ensure-EnvFile {
    param([string]$EnvPath)

    $content = @"
BIND_HOST=$BindHost
SERVER_PORT=$ServerPort
SERVER_MODE=debug
RUN_MODE=standard
POSTGRES_USER=$PostgresUser
POSTGRES_PASSWORD=$PostgresPassword
POSTGRES_DB=$PostgresDb
REDIS_PASSWORD=
REDIS_DB=0
ADMIN_EMAIL=$AdminEmail
ADMIN_PASSWORD=$AdminPassword
JWT_SECRET=$JwtSecret
TOTP_ENCRYPTION_KEY=$TotpKey
TZ=Asia/Shanghai
"@
    Set-Content -Path $EnvPath -Value $content -Encoding ascii
}

function Get-AuthToken {
    param([string]$BaseUrl)

    $response = Invoke-ApiJson -Method POST -Uri "$BaseUrl/api/v1/auth/login" -Body @{
        email = $AdminEmail
        password = $AdminPassword
    }
    return $response.access_token
}

function Ensure-ReferralSettings {
    param(
        [string]$BaseUrl,
        [string]$Token
    )

    $headers = @{ Authorization = "Bearer $Token" }
    $settings = Invoke-ApiJson -Method GET -Uri "$BaseUrl/api/v1/admin/settings" -Headers $headers
    $settings.referral_enabled = $true
    $settings.referral_level1_enabled = $true
    $settings.referral_level2_enabled = $true
    $settings.referral_level1_rate = 0.15
    $settings.referral_level2_rate = 0.05
    $settings.referral_reward_mode = "every_paid_order"
    $settings.referral_settlement_delay_days = 7
    $settings.referral_bind_before_first_paid_only = $true
    $settings.referral_allow_manual_input = $true
    $settings.referral_withdraw_enabled = $true
    $settings.referral_withdraw_min_amount = 10
    $settings.referral_withdraw_max_amount = 5000
    $settings.referral_withdraw_daily_limit = 5
    $settings.referral_withdraw_fee_rate = 0.02
    $settings.referral_withdraw_fixed_fee = 0
    $settings.referral_withdraw_manual_review_required = $true
    $settings.referral_refund_reverse_enabled = $true
    $settings.referral_negative_carry_enabled = $true
    $settings.referral_settlement_currency = "CNY"
    $settings.referral_withdraw_methods_enabled = @("alipay", "wechat", "bank")
    Invoke-ApiJson -Method PUT -Uri "$BaseUrl/api/v1/admin/settings" -Headers $headers -Body $settings | Out-Null
}

function Ensure-User {
    param(
        [string]$BaseUrl,
        [string]$Token,
        [string]$Email,
        [string]$Password,
        [string]$Username,
        [double]$Balance = 0
    )

    $headers = @{ Authorization = "Bearer $Token" }
    try {
        $user = Invoke-ApiJson -Method POST -Uri "$BaseUrl/api/v1/admin/users" -Headers $headers -Body @{
            email = $Email
            password = $Password
            username = $Username
            balance = $Balance
            concurrency = 5
        }
        return $user
    } catch {
        $users = Invoke-ApiJson -Method GET -Uri "$BaseUrl/api/v1/admin/users?page=1&page_size=200&search=$([uri]::EscapeDataString($Email))" -Headers $headers
        $existing = $users.items | Where-Object { $_.email -eq $Email } | Select-Object -First 1
        if ($null -eq $existing) {
            throw
        }
        return $existing
    }
}

function Login-User {
    param(
        [string]$BaseUrl,
        [string]$Email,
        [string]$Password
    )

    $response = Invoke-ApiJson -Method POST -Uri "$BaseUrl/api/v1/auth/login" -Body @{
        email = $Email
        password = $Password
    }
    return $response.access_token
}

function Invoke-AdminRecharge {
    param(
        [string]$BaseUrl,
        [string]$Token,
        [int64]$UserId,
        [string]$ExternalOrderId,
        [double]$PaidAmount
    )

    $headers = @{
        Authorization   = "Bearer $Token"
        "Idempotency-Key" = "idem-$ExternalOrderId"
    }
    Invoke-ApiJson -Method POST -Uri "$BaseUrl/api/v1/admin/recharge-orders/credit" -Headers $headers -Body @{
        external_order_id = $ExternalOrderId
        provider = "local-demo"
        channel = "alipay"
        currency = "CNY"
        user_id = $UserId
        gross_amount = $PaidAmount
        discount_amount = 0
        paid_amount = $PaidAmount
        gift_balance_amount = 0
        credited_balance_amount = $PaidAmount
        metadata_json = "{`"seed`":`"referral-demo`"}"
        notes = "local demo seed"
    } | Out-Null
}

function Invoke-Psql {
    param(
        [string]$Sql,
        [string]$ContainerName
    )

    $tmp = Join-Path $PSScriptRoot "tmp_seed.sql"
    Set-Content -Path $tmp -Value $Sql -Encoding utf8
    try {
        docker cp $tmp "${ContainerName}:/tmp/seed.sql" | Out-Null
        docker exec $ContainerName psql -U $PostgresUser -d $PostgresDb -f /tmp/seed.sql | Out-Null
    } finally {
        if (Test-Path $tmp) {
            Remove-Item $tmp -Force
        }
    }
}

$deployDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$envPath = Join-Path $deployDir ".env"
$baseUrl = "http://$BindHost`:$ServerPort"
$composeArgs = @("-f", $ComposeFile)

Write-Host "[1/7] Preparing local Docker environment..."
if ($ResetData) {
    docker compose @composeArgs down | Out-Null
    foreach ($dir in @("data", "postgres_data", "redis_data")) {
        $full = Join-Path $deployDir $dir
        if (Test-Path $full) {
            Remove-Item -LiteralPath $full -Recurse -Force
        }
    }
}
Ensure-EnvFile -EnvPath $envPath
New-Item -ItemType Directory -Force -Path (Join-Path $deployDir "data"), (Join-Path $deployDir "postgres_data"), (Join-Path $deployDir "redis_data") | Out-Null

Write-Host "[2/7] Starting Docker services..."
if ($SkipBuild) {
    docker compose @composeArgs up -d | Out-Null
} else {
    docker compose @composeArgs up -d --build | Out-Null
}

Write-Host "[3/7] Waiting for application health..."
Wait-HttpHealthy -Url "$baseUrl/health"

Write-Host "[4/7] Logging in as admin..."
$adminToken = Get-AuthToken -BaseUrl $baseUrl
Ensure-ReferralSettings -BaseUrl $baseUrl -Token $adminToken

Write-Host "[5/7] Creating demo users..."
$demoPassword = "Demo123456!"
$users = @()
$users += Ensure-User -BaseUrl $baseUrl -Token $adminToken -Email "ref-alpha@example.com" -Password $demoPassword -Username "alpha"
$users += Ensure-User -BaseUrl $baseUrl -Token $adminToken -Email "ref-bravo@example.com" -Password $demoPassword -Username "bravo"
$users += Ensure-User -BaseUrl $baseUrl -Token $adminToken -Email "ref-charlie@example.com" -Password $demoPassword -Username "charlie"
$users += Ensure-User -BaseUrl $baseUrl -Token $adminToken -Email "ref-delta@example.com" -Password $demoPassword -Username "delta"
$users += Ensure-User -BaseUrl $baseUrl -Token $adminToken -Email "ref-echo@example.com" -Password $demoPassword -Username "echo"
$users += Ensure-User -BaseUrl $baseUrl -Token $adminToken -Email "ref-foxtrot@example.com" -Password $demoPassword -Username "foxtrot"
$users += Ensure-User -BaseUrl $baseUrl -Token $adminToken -Email "ref-golf@example.com" -Password $demoPassword -Username "golf"
$users += Ensure-User -BaseUrl $baseUrl -Token $adminToken -Email "ref-hotel@example.com" -Password $demoPassword -Username "hotel"
$users += Ensure-User -BaseUrl $baseUrl -Token $adminToken -Email "ref-india@example.com" -Password $demoPassword -Username "india"
$users += Ensure-User -BaseUrl $baseUrl -Token $adminToken -Email "ref-juliet@example.com" -Password $demoPassword -Username "juliet"
$users += Ensure-User -BaseUrl $baseUrl -Token $adminToken -Email "ref-kilo@example.com" -Password $demoPassword -Username "kilo"
$users += Ensure-User -BaseUrl $baseUrl -Token $adminToken -Email "ref-lima@example.com" -Password $demoPassword -Username "lima"

$alpha = $users[0]
$bravo = $users[1]
$charlie = $users[2]
$delta = $users[3]
$echo = $users[4]
$foxtrot = $users[5]
$golf = $users[6]
$hotel = $users[7]
$india = $users[8]
$juliet = $users[9]
$kilo = $users[10]
$lima = $users[11]

Write-Host "[6/7] Seeding referral graph and commission data..."
$sql = @"
INSERT INTO referral_codes (user_id, code, status, is_default)
VALUES
  ($($alpha.id), 'ALPHA001', 'active', true),
  ($($bravo.id), 'BRAVO001', 'active', true),
  ($($charlie.id), 'CHARLIE1', 'active', true),
  ($($delta.id), 'DELTA001', 'active', true),
  ($($echo.id), 'ECHO0001', 'active', true),
  ($($foxtrot.id), 'FOXTR001', 'active', true),
  ($($golf.id), 'GOLF0001', 'active', true),
  ($($hotel.id), 'HOTEL001', 'active', true),
  ($($india.id), 'INDIA001', 'active', true),
  ($($juliet.id), 'JULIET01', 'active', true),
  ($($kilo.id), 'KILO0001', 'active', true),
  ($($lima.id), 'LIMA0001', 'active', true)
ON CONFLICT (code) DO NOTHING;

INSERT INTO referral_relations (user_id, referrer_user_id, bind_source, bind_code, created_at, updated_at)
VALUES
  ($($bravo.id), $($alpha.id), 'link', 'ALPHA001', NOW() - INTERVAL '18 days', NOW() - INTERVAL '18 days'),
  ($($charlie.id), $($alpha.id), 'link', 'ALPHA001', NOW() - INTERVAL '17 days', NOW() - INTERVAL '17 days'),
  ($($delta.id), $($alpha.id), 'link', 'ALPHA001', NOW() - INTERVAL '16 days', NOW() - INTERVAL '16 days'),
  ($($echo.id), $($bravo.id), 'link', 'BRAVO001', NOW() - INTERVAL '15 days', NOW() - INTERVAL '15 days'),
  ($($foxtrot.id), $($bravo.id), 'link', 'BRAVO001', NOW() - INTERVAL '14 days', NOW() - INTERVAL '14 days'),
  ($($golf.id), $($charlie.id), 'link', 'CHARLIE1', NOW() - INTERVAL '13 days', NOW() - INTERVAL '13 days'),
  ($($hotel.id), $($charlie.id), 'link', 'CHARLIE1', NOW() - INTERVAL '12 days', NOW() - INTERVAL '12 days'),
  ($($india.id), $($delta.id), 'link', 'DELTA001', NOW() - INTERVAL '11 days', NOW() - INTERVAL '11 days'),
  ($($juliet.id), $($delta.id), 'link', 'DELTA001', NOW() - INTERVAL '10 days', NOW() - INTERVAL '10 days'),
  ($($kilo.id), $($echo.id), 'link', 'ECHO0001', NOW() - INTERVAL '9 days', NOW() - INTERVAL '9 days'),
  ($($lima.id), $($foxtrot.id), 'link', 'FOXTR001', NOW() - INTERVAL '8 days', NOW() - INTERVAL '8 days')
ON CONFLICT (user_id) DO NOTHING;

INSERT INTO referral_relation_histories (user_id, new_referrer_user_id, new_bind_code, change_source, created_at)
VALUES
  ($($bravo.id), $($alpha.id), 'ALPHA001', 'link', NOW() - INTERVAL '18 days'),
  ($($charlie.id), $($alpha.id), 'ALPHA001', 'link', NOW() - INTERVAL '17 days'),
  ($($delta.id), $($alpha.id), 'ALPHA001', 'link', NOW() - INTERVAL '16 days')
ON CONFLICT DO NOTHING;
"@
Invoke-Psql -Sql $sql -ContainerName "sub2api-postgres-dev"

$orders = @(
    @{ UserId = $echo.id; Amount = 120.0 },
    @{ UserId = $foxtrot.id; Amount = 88.0 },
    @{ UserId = $golf.id; Amount = 66.0 },
    @{ UserId = $hotel.id; Amount = 188.0 },
    @{ UserId = $india.id; Amount = 256.0 },
    @{ UserId = $juliet.id; Amount = 99.0 },
    @{ UserId = $kilo.id; Amount = 138.0 },
    @{ UserId = $lima.id; Amount = 168.0 },
    @{ UserId = $echo.id; Amount = 72.0 },
    @{ UserId = $foxtrot.id; Amount = 45.0 },
    @{ UserId = $golf.id; Amount = 54.0 },
    @{ UserId = $hotel.id; Amount = 73.0 }
)

$index = 1
foreach ($order in $orders) {
    Invoke-AdminRecharge -BaseUrl $baseUrl -Token $adminToken -UserId $order.UserId -ExternalOrderId ("demo-order-{0:D3}" -f $index) -PaidAmount $order.Amount
    $index++
}

$settleSql = @"
WITH to_settle AS (
  SELECT id, user_id, recharge_order_id, reward_amount, currency
  FROM commission_rewards
  WHERE status = 'pending' AND MOD(id, 2) = 0
)
INSERT INTO commission_ledgers (user_id, reward_id, recharge_order_id, entry_type, bucket, amount, currency, created_at)
SELECT user_id, id, recharge_order_id, 'reward_pending_to_available', 'pending', -reward_amount, currency, NOW() - INTERVAL '9 days'
FROM to_settle;

WITH to_settle AS (
  SELECT id, user_id, recharge_order_id, reward_amount, currency
  FROM commission_rewards
  WHERE status = 'pending' AND MOD(id, 2) = 0
)
INSERT INTO commission_ledgers (user_id, reward_id, recharge_order_id, entry_type, bucket, amount, currency, created_at)
SELECT user_id, id, recharge_order_id, 'reward_pending_to_available', 'available', reward_amount, currency, NOW() - INTERVAL '9 days'
FROM to_settle;

UPDATE commission_rewards
SET status = 'available',
    available_at = NOW() - INTERVAL '8 days',
    updated_at = NOW()
WHERE status = 'pending' AND MOD(id, 2) = 0;
"@
Invoke-Psql -Sql $settleSql -ContainerName "sub2api-postgres-dev"

$bravoToken = Login-User -BaseUrl $baseUrl -Email $bravo.email -Password $demoPassword
$charlieToken = Login-User -BaseUrl $baseUrl -Email $charlie.email -Password $demoPassword
$alphaToken = Login-User -BaseUrl $baseUrl -Email $alpha.email -Password $demoPassword

$bravoAccount = Invoke-ApiJson -Method POST -Uri "$baseUrl/api/v1/user/referral/payout-accounts" -Headers @{ Authorization = "Bearer $bravoToken" } -Body @{
    method = "alipay"
    account_name = "Bravo Demo"
    account_no = "bravo@example.com"
    is_default = $true
}
$charlieAccount = Invoke-ApiJson -Method POST -Uri "$baseUrl/api/v1/user/referral/payout-accounts" -Headers @{ Authorization = "Bearer $charlieToken" } -Body @{
    method = "bank"
    account_name = "Charlie Demo"
    account_no = "6222020011223344"
    bank_name = "Demo Bank"
    is_default = $true
}
$alphaAccount = Invoke-ApiJson -Method POST -Uri "$baseUrl/api/v1/user/referral/payout-accounts" -Headers @{ Authorization = "Bearer $alphaToken" } -Body @{
    method = "wechat"
    account_name = "Alpha Demo"
    account_no = "alpha-wechat"
    is_default = $true
}

$charlieRewards = Invoke-ApiJson -Method GET -Uri "$baseUrl/api/v1/admin/referral/commission-rewards?page=1&page_size=20&user_id=$($charlie.id)" -Headers @{ Authorization = "Bearer $adminToken" }
if ($charlieRewards.items.Count -gt 0) {
    Invoke-ApiJson -Method POST -Uri "$baseUrl/api/v1/admin/referral/commission-adjustments" -Headers @{ Authorization = "Bearer $adminToken" } -Body @{
        reward_id = $charlieRewards.items[0].id
        amount = 15
        remark = "seed extra available commission for charlie"
    } | Out-Null
}

$bravoWithdrawal = Invoke-ApiJson -Method POST -Uri "$baseUrl/api/v1/user/referral/withdrawals" -Headers @{ Authorization = "Bearer $bravoToken" } -Body @{
    amount = 10
    payout_method = "alipay"
    payout_account_id = $bravoAccount.id
    remark = "demo pending then paid"
}
$charlieWithdrawal = Invoke-ApiJson -Method POST -Uri "$baseUrl/api/v1/user/referral/withdrawals" -Headers @{ Authorization = "Bearer $charlieToken" } -Body @{
    amount = 10
    payout_method = "bank"
    payout_account_id = $charlieAccount.id
    remark = "demo reject"
}
$alphaWithdrawal = Invoke-ApiJson -Method POST -Uri "$baseUrl/api/v1/user/referral/withdrawals" -Headers @{ Authorization = "Bearer $alphaToken" } -Body @{
    amount = 12
    payout_method = "wechat"
    payout_account_id = $alphaAccount.id
    remark = "demo pending review"
}

Invoke-ApiJson -Method POST -Uri "$baseUrl/api/v1/admin/referral/withdrawals/$($bravoWithdrawal.withdrawal.id)/approve" -Headers @{ Authorization = "Bearer $adminToken" } -Body @{ remark = "demo approve" } | Out-Null
Invoke-ApiJson -Method POST -Uri "$baseUrl/api/v1/admin/referral/withdrawals/$($bravoWithdrawal.withdrawal.id)/mark-paid" -Headers @{ Authorization = "Bearer $adminToken" } -Body @{ remark = "demo paid" } | Out-Null
Invoke-ApiJson -Method POST -Uri "$baseUrl/api/v1/admin/referral/withdrawals/$($charlieWithdrawal.withdrawal.id)/reject" -Headers @{ Authorization = "Bearer $adminToken" } -Body @{ reason = "demo reject" } | Out-Null

$relationUpdate = Invoke-ApiJson -Method PUT -Uri "$baseUrl/api/v1/admin/referral/relations/$($juliet.id)" -Headers @{ Authorization = "Bearer $adminToken" } -Body @{
    code = "CHARLIE1"
    reason = "demo admin override"
    notes = "seeded for relation history"
}

$rewards = Invoke-ApiJson -Method GET -Uri "$baseUrl/api/v1/admin/referral/commission-rewards?page=1&page_size=50" -Headers @{ Authorization = "Bearer $adminToken" }
if ($rewards.items.Count -gt 0) {
    Invoke-ApiJson -Method POST -Uri "$baseUrl/api/v1/admin/referral/commission-adjustments" -Headers @{ Authorization = "Bearer $adminToken" } -Body @{
        reward_id = $rewards.items[0].id
        amount = 3.5
        remark = "demo manual adjustment"
    } | Out-Null
}

Write-Host "[7/7] Demo environment is ready."
Write-Host ""
Write-Host "Open: $baseUrl"
Write-Host "Admin: $AdminEmail / $AdminPassword"
Write-Host "Demo users:"
Write-Host "  $($alpha.email) / $demoPassword"
Write-Host "  $($bravo.email) / $demoPassword"
Write-Host "  $($charlie.email) / $demoPassword"
Write-Host ""
Write-Host "Suggested pages:"
Write-Host "  $baseUrl/login"
Write-Host "  $baseUrl/admin/referral"
Write-Host "  $baseUrl/admin/referral/withdrawals"
Write-Host "  $baseUrl/referral"
