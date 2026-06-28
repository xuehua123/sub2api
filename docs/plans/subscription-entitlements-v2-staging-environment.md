# Subscription Entitlements V2 Staging Environment

This document records non-sensitive staging environment facts for the
subscription entitlements v2 validation work. It is intentionally limited to
connection metadata, paths, safe command templates, test fixture ids, and
operational guardrails.

Do not add root passwords, tokens, API key secrets, database passwords,
provider credentials, payment secrets, private keys, production database
connection strings, or evidence files to this document.

Related references:

- Staging rollout runbook:
  `docs/plans/subscription-entitlements-v2-staging-rollout-runbook.md`
- Staging execution template:
  `docs/plans/subscription-entitlements-v2-staging-execution-template.md`
- Preflight SQL:
  `docs/plans/subscription-entitlements-v2-preflight-sql.md`
- Staging smoke helper:
  `tools/subscription_entitlements_v2_staging_smoke.ps1`

## Staging Basics

| Item | Value |
| --- | --- |
| Domain | `https://staging.xxxaicode.com` |
| Source IP | `38.54.104.97` |
| SSH user | `deploy` |
| SSH command | `ssh deploy@38.54.104.97` |
| Hostname | `sub2api-staging` |
| OS | Ubuntu 24.04.2 LTS |

Use the `deploy` user for staging operations. Do not use root login for normal
operations, and never record the root password in source control, evidence
files, or chat output.

## Server Directories

| Purpose | Path |
| --- | --- |
| Repository | `/opt/sub2api-staging/repo` |
| Deploy files | `/opt/sub2api-staging/deploy` |
| App data | `/opt/sub2api-staging/data` |
| Postgres data | `/opt/sub2api-staging/postgres_data` |
| Redis data | `/opt/sub2api-staging/redis_data` |
| Evidence | `/opt/sub2api-staging/evidence` |
| Secrets | `/opt/sub2api-staging/secrets` |
| Backups | `/opt/sub2api-staging/backups` |
| Temporary files | `/opt/sub2api-staging/tmp` |

Secrets stored under `/opt/sub2api-staging/secrets` must not be printed in
task reports. Evidence under `/opt/sub2api-staging/evidence` must not be
committed to this repository.

## Current Deployment Snapshot

| Item | Last Reported Value |
| --- | --- |
| Branch | `codex/subscription-entitlements-v2` |
| Staging commit | `499559140b43817f434642df5c22f801e2f48506` |
| Runtime | Docker Compose |
| App bind address | `127.0.0.1:8080` |
| Public proxy | Nginx |
| Nginx `server_name` | `staging.xxxaicode.com` |
| Required Nginx directive | `underscores_in_headers on;` |
| TLS | Let's Encrypt |

The staging commit is the last reported deployed commit. Before any operation,
verify the live repository state with the commands below instead of assuming it
is still current.

## Safe Command Templates

These commands are safe templates. They intentionally do not include secrets.

```bash
ssh deploy@38.54.104.97
```

```bash
cd /opt/sub2api-staging/repo
git status
git rev-parse --abbrev-ref HEAD
git rev-parse HEAD
```

The deployment uses Docker Compose from the repository deploy directory. Verify
the compose file exists before running compose commands:

```bash
cd /opt/sub2api-staging/repo/deploy
ls docker-compose.dev.yml
docker compose -f docker-compose.dev.yml ps
docker compose -f docker-compose.dev.yml logs --tail=100 sub2api
```

Health check:

```bash
curl -fsS https://staging.xxxaicode.com/health
```

Initial read-only smoke from a local workstation:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass `
  -File tools/subscription_entitlements_v2_staging_smoke.ps1 `
  -BaseUrl "https://staging.xxxaicode.com" `
  -AdminToken $env:SUB2API_STAGING_ADMIN_TOKEN `
  -ExpectV2Enabled false `
  -ExpectLegacyMappingEnabled false
```

Never paste token values into commands that will be saved in shell history or
task reports. Prefer environment variables or files under the staging secrets
directory with restrictive permissions.

## Flag Requirements

Default staging state must keep both flags disabled:

- `subscription_entitlements_v2_enabled=false`
- `sub2_payment_page_legacy_mapping_enabled=false`

Only during Task 15-D or a Task 15-D revalidation is it acceptable to enable
the main staging v2 flag:

- `subscription_entitlements_v2_enabled=true`

Do not enable `sub2_payment_page_legacy_mapping_enabled` unless a separate
Task 15-E legacy mapping validation has been explicitly approved.

Production flags must not be changed from this staging workflow.

## Existing Staging Test Fixture IDs

These ids are non-sensitive staging fixtures prepared for entitlement v2
validation. They are useful for staging smoke, sandbox upstream account setup,
and future gateway quota/fallback checks.

| Fixture | ID / Value |
| --- | --- |
| Normal user | `user_id=2` |
| Fallback enough-balance user | `user_id=3` |
| Fallback insufficient-balance user | `user_id=4` |
| Subscription group A | `group_id=2` |
| Subscription group B | `group_id=3` |
| Standard/balance group | `group_id=4` |
| Negative mismatch group | `group_id=5` |
| Block-policy plan | `plan_id=1` |
| Balance-fallback plan | `plan_id=2` |
| Alias-pair entitlement | `entitlement_id=1`, `legacy_subscription_id=1` |
| Entitlement-only fixture | `entitlement_id=2` |
| Fallback enough-balance entitlement | `entitlement_id=3` |
| Fallback insufficient-balance entitlement | `entitlement_id=4` |
| Bound alias API key | `api_key_id=1` |
| Standard group API key | `api_key_id=2` |
| Quick switch API key | `api_key_id=3` |
| Fallback enough-balance API key | `api_key_id=4` |
| Fallback insufficient-balance API key | `api_key_id=5` |

API key secrets, user passwords, admin tokens, user tokens, and provider keys
are stored only in the staging secrets area and must not be written here.

## Evidence Rules

Save evidence under a task-specific directory:

```text
/opt/sub2api-staging/evidence/<timestamp>-<task>
```

Examples:

```text
/opt/sub2api-staging/evidence/20260613-132256-task15d
/opt/sub2api-staging/evidence/YYYYMMDD-HHMM-task15d-fix2
```

Evidence may include redacted command output, preflight SQL results, HTTP
status summaries, and non-sensitive row ids. Evidence must not include tokens,
API key secrets, provider credentials, database dumps, raw user private data,
or unredacted payment/provider responses.

Do not commit evidence files to this repository.

## Sandbox Or Mock Upstream Setup Notes

Task 15-D gateway quota and fallback checks require a staging-only upstream
account or mock provider because staging previously had no available accounts.

Acceptable options, in priority order:

1. Existing project mock or test provider.
2. Local or staging-hosted OpenAI-compatible mock upstream returning fixed
   successful responses.
3. Staging-only sandbox provider key with strict limits.

Forbidden options:

- Production provider keys.
- Production upstream accounts.
- Production payment secrets.
- Production database or Redis connections.
- Any setup that prints provider keys or API key secrets in logs.

Bind any sandbox or mock account only to staging groups intended for testing,
for example `group_id=2` and `group_id=3`. Keep `group_id=4` as the
standard/balance comparison group unless the current task explicitly changes
that setup.

## Prohibited Actions

- Do not log in to production.
- Do not connect to production Postgres or Redis.
- Do not use production provider, payment, webhook, OAuth, or API credentials.
- Do not modify `sub2-payment-page`.
- Do not build Docker images on production business servers.
- Do not submit or commit secrets, evidence, database dumps, `.env` files, or
  provider responses containing credentials.
- Do not write root passwords, tokens, API key secrets, provider keys, private
  keys, or database passwords to documentation.
- Do not enable production v2 flags from this staging workflow.

## Pre-Task Checklist

Before any staging operation, record these non-sensitive facts in the task
report:

- Current host and SSH user.
- Current branch and commit.
- Current flag values.
- Evidence directory path.
- Whether the operation uses mock, sandbox, or no upstream provider.
- Confirmation that no production credentials or production services are used.

