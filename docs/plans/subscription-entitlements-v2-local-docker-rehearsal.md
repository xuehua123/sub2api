# Subscription Entitlements V2 Local Docker Rehearsal Plan

This plan describes how to rehearse subscription entitlements v2 locally with
Docker, synthetic data, and optional tiny anonymized samples. It must not be
used as a production extraction or production enablement runbook.

Related references:

- Staging rollout runbook:
  `docs/plans/subscription-entitlements-v2-staging-rollout-runbook.md`
- Staging execution template:
  `docs/plans/subscription-entitlements-v2-staging-execution-template.md`
- Preflight SQL:
  `docs/plans/subscription-entitlements-v2-preflight-sql.md`
- Local rehearsal helper draft:
  `tools/subscription_entitlements_v2_local_rehearsal.ps1`

## Existing Local Docker Setup

Current repository support:

- Root `Dockerfile` builds frontend and backend into an embedded frontend image.
- `backend/Dockerfile` builds only the Go server and is less suitable for full
  UI rehearsal.
- `deploy/docker-compose.dev.yml` builds from local source and starts:
  - `sub2api`
  - `postgres`
  - `redis`
- `deploy/docker-compose.local.yml` runs the published image with local data
  directories.
- `deploy/.env.example` documents environment variables.
- `deploy/README.md` documents Docker auto setup. With `AUTO_SETUP=true`, first
  run connects to PostgreSQL/Redis, applies SQL migrations from
  `backend/migrations/*.sql`, creates an admin user, and writes config.
- `deploy/run_referral_demo.ps1` is an existing local demo script pattern, but
  it is referral-specific and not safe to reuse directly for entitlement v2.
- `.gitignore` already ignores:
  - `.env`, `*.env`, `*.local`
  - `deploy/config.yaml`
  - `tmp/`
  - `deploy/docker-compose.override.yml`

Recommended rehearsal base: `deploy/docker-compose.dev.yml`, because it builds
the current branch locally and keeps PostgreSQL/Redis isolated from staging and
production.

## Local Rehearsal Flow

Use only local Docker resources. Do not log in to production, do not connect to
production databases, and do not build images on production business servers.

1. Prepare local environment values outside git:
   - `POSTGRES_PASSWORD`
   - `JWT_SECRET`
   - `TOTP_ENCRYPTION_KEY`
   - `ADMIN_EMAIL`
   - `ADMIN_PASSWORD`
2. Start local compose from the repository:

   ```powershell
   cd deploy
   docker compose -f docker-compose.dev.yml up -d --build
   docker compose -f docker-compose.dev.yml logs -f sub2api
   ```

3. Wait for health:

   ```powershell
   Invoke-WebRequest -UseBasicParsing http://127.0.0.1:8080/health
   ```

4. Confirm migrations applied through auto setup:

   ```powershell
   docker compose -f docker-compose.dev.yml exec -T postgres `
     psql -U $env:POSTGRES_USER -d $env:POSTGRES_DB `
     -c "SELECT filename FROM schema_migrations ORDER BY filename DESC LIMIT 10;"
   ```

5. Confirm both flags are initially false:
   - `subscription_entitlements_v2_enabled=false`
   - `sub2_payment_page_legacy_mapping_enabled=false`

6. Seed synthetic entitlement data. Prefer local admin APIs where practical, and
   use SQL only for fixture rows that have no ergonomic admin API setup path.

7. Enable v2 locally only:
   - `subscription_entitlements_v2_enabled=true`
   - keep `sub2_payment_page_legacy_mapping_enabled=false`

8. Rehearse:
   - API key group selection.
   - Same-entitlement group switching.
   - User `/entitlements`.
   - Minimal `/subscriptions` alias.
   - Admin usage entitlement filters.
   - Gateway quota success, quota exceeded, and balance fallback where local
     upstream stubbing is available.

9. Roll back locally:
   - set both flags false.
   - leave schema in place.
   - clear local test data by dropping local compose volumes/directories only.

## Synthetic Seed Design

Prefer synthetic data. It is deterministic, auditable, and avoids production
privacy risk.

Minimum fixture:

| Fixture | Purpose |
| --- | --- |
| Admin user | Toggle settings and create local fixtures. |
| Test user A | Entitlement quota success and multi-group API key selection. |
| Test user B | Balance fallback success. |
| Test user C | Balance fallback insufficient balance. |
| Standard group | Verify balance/standard path is not bound to entitlement. |
| Subscription group 1 | First authorized entitlement group. |
| Subscription group 2 | Second authorized entitlement group for multi-group coverage. |
| Subscription group 3 | Non-covered subscription group for negative coverage tests. |
| Subscription plan | Explicit access scope with group 1 and group 2 grants. |
| Plan grants | `subscription_plan_groups` rows, stable `sort_order`. |
| Entitlement A | Active, covers group 1 and group 2, `overage_policy='block'`. |
| Entitlement B | Active, same user/group coverage for explicit selection UI test. |
| Fallback entitlement | Active, small limit, `overage_policy='balance_fallback'`. |
| API key A | Bound to Entitlement A and group 1. |
| API key fallback | Bound to fallback entitlement. |
| Usage fixtures | Optional local usage rows with `billing_source` values for admin UI. |

Recommended local values:

- Synthetic email domain: `example.test`.
- Synthetic external ids: `local-rehearsal-*`.
- Synthetic notes: avoid copying production notes; use short fixture labels.
- API key values: generate fresh local-only values and never copy production API
  keys.
- Passwords: use local-only values generated during rehearsal and keep them out
  of committed files.

Seed invariants:

- Entitlement group order must be stable: `sort_order`, then `group_id`.
- API keys bound to standard/balance groups must have
  `subscription_entitlement_id=NULL`.
- Entitlement-only rows without `legacy_subscription_id` must appear in
  `/entitlements` and not in `/subscriptions`.
- Alias rehearsal requires a real `user_subscriptions.id` linked through
  `subscription_entitlements.legacy_subscription_id`.
- Fallback success must deduct balance and write
  `billing_source='entitlement_balance_fallback'`.
- Fallback insufficient balance must roll back usage log, billing dedup,
  entitlement usage, and balance changes.

## Optional Tiny Anonymized Sample Plan

Only use this if synthetic seed cannot reproduce a staging-only issue. The
preferred route remains synthetic seed.

Rules:

1. Export from a read-only replica or staging copy, not directly from
   production primary.
2. Export only whitelisted tables and columns.
3. Export only the minimum rows needed for a single user/group/entitlement case.
4. Save raw exports only under an ignored local directory such as
   `tmp/subscription-entitlements-v2-rehearsal/`.
5. Run anonymization before import into local Docker.
6. Delete raw exports after anonymized import is verified.

Whitelisted tables:

| Table | Allowed Columns | Required Redaction |
| --- | --- | --- |
| `users` | `id`, `role`, `balance`, `concurrency`, `status`, timestamps | Replace `email`, `password_hash`; do not export auth identities. |
| `groups` | ids, names, platform, subscription type, limits, sort/status | Replace names if they reveal customers or vendors. |
| `user_allowed_groups` | `user_id`, `group_id` | Keep only sampled users. |
| `api_keys` | id, user/group/status, entitlement id, timestamps | Replace `key`, `name`; remove last-used metadata if sensitive. |
| `subscription_plans` | ids, group id, access scope, limits, policy, price fields | Replace product/name/description if sensitive. |
| `subscription_plan_groups` | plan/group/sort/enabled | Keep only sampled plans. |
| `user_subscriptions` | ids, user/group/status/windows/usage/times | Keep only alias pair rows. |
| `subscription_entitlements` | ids, owner, plan, legacy id, group, limits, usage, windows, policy | Clear `source_*`, `assigned_by`, `notes`, `plan_snapshot`. |
| `subscription_entitlement_groups` | entitlement/group/sort/enabled | Keep only sampled entitlements. |
| `subscription_entitlement_fulfillments` | entitlement/user/plan/source type/times | Prefer synthetic rows; otherwise clear `source_*` and notes. |
| `usage_logs` | ids, user/key/account/group/subscription/entitlement, cost, model, billing source, created_at | Replace request ids, IP/user agent fields, model if sensitive. |
| `settings` | selected feature flags only | Do not export payment, OAuth, security, or provider secrets. |

Never export:

- `accounts.credentials`
- OAuth tokens or refresh tokens
- provider API keys
- payment provider config secrets
- `security_secrets`
- raw `payment_orders` identifiers unless fully synthetic
- raw `redeem_codes.code`
- raw `source_external_id`
- user notes or admin notes
- logs containing Authorization headers

Redaction rules:

- Email: `user_<id>@example.test`.
- API key value: generate a fresh local-only value.
- Password hash: replace with a known local hash or recreate users via admin API.
- Order ids/trade nos/source external ids: `local-order-<stable-id>`.
- Redeem codes: `LOCAL-REDEEM-<stable-id>`.
- Notes: `redacted-local-sample`.
- Provider credentials: `{}` or `NULL`.
- Tokens/secrets/private keys: do not export; if accidentally present, discard
  the sample and start over.

Risk checklist:

- A "small" export can still leak tokens if columns are not whitelisted.
- Notes/source fields often contain external order ids or user-provided text.
- Payment and OAuth tables are high risk and should not be exported for this
  rehearsal.
- Local imports must never be pointed at staging or production DSNs.
- Do not commit raw or anonymized sample files.

## Local Rehearsal Helper Draft

The helper draft is intentionally conservative:

- Default mode is dry-run.
- It accepts only local base URLs for executable mode.
- It prints commands and fixture intent.
- It can write a synthetic seed SQL template under ignored `tmp/`.
- It does not connect to production.
- It does not include real tokens, passwords, provider keys, or domains.

Use:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass `
  -File tools/subscription_entitlements_v2_local_rehearsal.ps1 -Help

powershell -NoProfile -ExecutionPolicy Bypass `
  -File tools/subscription_entitlements_v2_local_rehearsal.ps1 -DryRun

powershell -NoProfile -ExecutionPolicy Bypass `
  -File tools/subscription_entitlements_v2_local_rehearsal.ps1 `
  -DryRun -WriteSeedTemplate
```

## What Local Docker Can Validate

Local rehearsal can validate:

- Migration order and idempotency in a clean PostgreSQL container.
- Feature flag behavior for v2 off/on.
- Synthetic entitlement, plan, group, and API key relationships.
- API key entitlement group selection in the UI.
- Same-entitlement group switching.
- User `/entitlements` and `/subscriptions` alias behavior.
- Usage log attribution and admin usage filters when synthetic requests are
  recorded.
- Gateway quota and fallback behavior if a local-safe upstream/account fixture
  is available.

Local rehearsal cannot replace:

- Real payment provider callbacks.
- Real external payment page behavior.
- Real production domain HTTPS, cookies, CSP, and callback URLs.
- Production nginx/header behavior.
- Production traffic concurrency and long-running streaming behavior.
- Provider-side OAuth, quota, or billing behavior.

## Files And Ignore Rules

Already ignored and safe for local artifacts:

- `tmp/`
- `.env`, `*.env`, `.env.local`, `*.local`
- `deploy/config.yaml`
- `deploy/docker-compose.override.yml`

Use these ignored locations for local evidence and generated templates:

- `tmp/subscription-entitlements-v2-rehearsal/`
- `deploy/.env`
- `deploy/config.yaml`

If an operator prefers `local-evidence/`, add it to `.gitignore` before writing
any evidence there. This plan intentionally uses `tmp/` so no new ignore rule is
required.

## Proposed Follow-Up Files

Current task adds:

- `docs/plans/subscription-entitlements-v2-local-docker-rehearsal.md`
- `tools/subscription_entitlements_v2_local_rehearsal.ps1`

Possible later, after review:

- `tools/subscription_entitlements_v2_local_seed.ps1`
- `backend/internal/integration/entitlement_rehearsal_seed.sql`
