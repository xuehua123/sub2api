# AGENTS.md — Sub2API

> Fork: `bayma888/sub2api-bmai` ← upstream `Wei-Shaw/sub2api`

## Critical Architecture Rules

- **Strict layering** enforced by `.golangci.yml` depguard:
  - `service` **must NOT** import `repository`, `gorm`, or `redis`
  - `handler` **must NOT** import `repository`, `gorm`, or `redis`
  - Exceptions: `ops_aggregation_service.go`, `ops_alert_evaluator_service.go`, `ops_cleanup_service.go`, `ops_metrics_collector.go`, `ops_scheduled_report_service.go`, `wire.go`
- **Dependency injection**: Google Wire in `backend/cmd/server/wire.go` — after changing DI bindings run `go generate ./cmd/server` in `backend/`

## Critical Production Safety

- **Never build Docker images on production business servers** such as the Shanghai Sub2API host or the psydo host. Docker builds can saturate CPU, disk IO, memory, and network, causing API 502s and user request failures.
- Production deployment must use prebuilt images from local build, CI, or a dedicated build machine. A production host may only `docker pull`, switch image tags, restart services, run health checks, and roll back.
- Before restarting production services, record the currently running image tag/container state and keep a tested rollback path. If a build is needed, stop and move it off the production host.

## Mandatory Production Deployment Runbook

The production topology is role-based. Never treat the three Pipixia hosts as
three interchangeable application servers:

- **稳如狗** is the independent canary. It has its own data plane and receives
  the release first.
- **Canada** is the only active Pipixia application host, PostgreSQL primary,
  and Redis master. Exactly one blue-green application slot runs after a
  completed release.
- **United States** is an edge relay only. It forwards stale-DNS and optimized
  route traffic to Canada over WireGuard. It must not run Sub2API, PostgreSQL,
  or Redis and must not receive application/database secrets. Until its known
  compromise is followed by a verified reinstall and credential rotation,
  treat it as untrusted and deploy nothing to it.
- **France** is disaster recovery only. PostgreSQL and Redis remain replicas;
  the Sub2API application stays stopped. A release may be pre-pulled for future
  failover, but France must not run application startup migrations.

Apply this sequence for **every** production update:

1. Confirm the merge branch, staged tree, target upstream version, conflict
   resolutions, generated files, and working tree. Complete review and the
   configured backend/frontend/unit/integration/lint/security checks.
2. Merge the reviewed branch into the latest `origin/main`, push `main`, and
   wait for every configured CI gate. Deploy only the CI-built immutable image
   digest for that exact commit; never build on a production host.
3. Verify release tag, `backend/cmd/server/VERSION`, source revision, OCI labels,
   and image digest as one release unit. Any mismatch blocks deployment.
4. Review database migrations before starting an inactive slot. Blue and green
   share one database, so every migration must be backward compatible with the
   still-running old slot. An incompatible migration requires an explicit
   maintenance/expand-contract plan; blue-green alone cannot make it safe.
5. Record rollback state before each active deployment: container names and
   image digests, active/inactive ports, Compose state, Nginx upstream and
   backup, database migration state, and public health baseline.
6. Deploy **稳如狗 first** using its blue-green mechanism: pull the prebuilt
   digest, start only the inactive slot, verify local health/version/migrations,
   atomically switch Nginx, verify public health, drain old workers and long
   SSE/WebSocket connections, then stop the old slot. Never keep two slots
   running long term.
7. Observe 稳如狗 before touching Canada. Routine app-only releases require at
   least 5-10 minutes; releases involving migrations, billing, subscriptions,
   groups, or entitlements require about 30 minutes. Check `UsageLog`, balance
   and entitlement billing, subscription/group coverage, Grok/OpenAI errors,
   Nginx transport errors, and container restarts.
8. Before Canada, confirm Canada is still the writable primary/master, France
   PostgreSQL/Redis replication is healthy and caught up, the US relay is
   forwarding successfully, and only one Canada application slot is active.
9. Deploy **Canada** blue-green with the same immutable digest: prewarm the
   inactive `18080`/`28080` slot, verify it locally, atomically switch the
   dedicated Nginx upstream include, validate both direct domains and the US
   relay path, drain old workers/connections, then stop the old slot.
10. Observe Canada for 10-15 minutes for routine releases or about 30 minutes
    for critical business/migration releases. Confirm ongoing `UsageLog`
    writes, no negative/missing/duplicate billing records, entitlement user and
    group consistency, healthy PostgreSQL/Redis, and no transport-level 502/504.
11. Do **not** deploy the application to the US relay. Only verify Nginx/WireGuard
    forwarding, both old-domain health paths, latency, error rate, and that no
    Sub2API/PostgreSQL/Redis workload is running there.
12. On France, only pull/cache the same immutable image and record it as the DR
    candidate. Re-confirm `pg_is_in_recovery() = true`, WAL receiver streaming,
    Redis replica link up, and zero running Sub2API applications.

Rollback rules:

- A canary failure stops the release before Canada.
- A Canada application failure switches Nginx back to the recorded old slot;
  do not change DNS or database roles for an ordinary application rollback.
- A migration incompatibility is not repaired by merely restoring the old
  image. Use the reviewed forward-fix/database recovery plan.
- Canada host loss uses the separate France promotion and ingress failover
  procedure. Disaster-recovery promotion is never part of a normal release.

Additional non-negotiable rules:

- Routine releases do not modify DNS.
- `.github/workflows/deploy-shanghai.yml` is a 稳如狗 canary workflow only.
  Canada promotion remains an explicit AI/operator blue-green action until a
  separately reviewed automation is approved; never add US or France app
  deployment back to that workflow.
- Distinguish provider/channel 5xx returned by the application from Nginx,
  WireGuard, database, Redis, or process transport failures. Provider 5xx are
  recorded; transport failures block or roll back the release.
- Do not add generic Docker cleanup, container-ID guessing, port inference, or
  topology-changing automation directly in production. Test it in isolation
  and obtain explicit rollout approval first.
- Prompt Audit remains disabled by default. Do not configure audit nodes or
  blocking, and do not enable raw prompt storage in production.

## Release Version Integrity

Treat the release tag, source version, CI image metadata, and runtime version
as one deployable unit. A successful build is not sufficient when these values
disagree.

1. Before merging an upstream release, record its tag, exact commit SHA, and
   declared semantic version.
2. After conflict resolution, verify `backend/cmd/server/VERSION` exactly
   matches the target release version. If the upstream tag itself contains a
   stale version file, explicitly include the upstream version-sync commit or a
   narrowly scoped fork version-sync commit **after** the target tag content is
   present; never pre-bump a branch that has not yet merged the release.
3. Before production deployment, verify the CI-built immutable image reports
   the same version and source revision in its OCI labels/build metadata, and
   that the server runtime displays that version after startup.
4. A mismatch among the target tag, `VERSION`, CI image metadata, release
   notes, or runtime display is a release blocker. State the discrepancy and
   its resolution in the final review/deployment report, together with the
   merge commit and immutable image digest.

## Exact Commands

### Backend (run from `backend/`)

```bash
go generate ./ent          # regenerate Ent after schema changes
go generate ./cmd/server   # regenerate Wire after DI changes
go test -tags=unit ./...           # unit tests
go test -tags=integration ./...    # integration tests (uses testcontainers)
golangci-lint run ./...            # lint (v2.9)
CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o bin/server ./cmd/server
```

### Frontend (run from `frontend/`)

```bash
pnpm install               # MUST use pnpm, NOT npm
pnpm run build             # outputs to ../backend/internal/web/dist/
pnpm run lint:check        # ESLint
pnpm run typecheck         # vue-tsc --noEmit
pnpm run test:run          # Vitest
```

### Root Makefile

```bash
make build                 # build backend + frontend
make test                  # backend tests + frontend lint+typecheck
make secret-scan           # python3 tools/secret_scan.py
```

### Build for production

```bash
# Frontend first, then backend with embed tag
cd frontend && pnpm install && pnpm run build
cd ../backend && go build -tags embed -o sub2api ./cmd/server
```

Without `-tags embed`, the binary will NOT serve the frontend UI.

## Go Version

- `go.mod` declares **go 1.26.6** (source of truth)
- CI verifies `go1.26.6` exactly
- README badges must match the `go.mod` version.

## Testing Quirks

- Tests are split by build tags: `unit`, `integration`, `e2e`
- Integration tests use `testcontainers-go` (PostgreSQL + Redis containers)
- When adding methods to an interface, **all stubs/mocks** implementing it must be updated across test files — compilation will fail otherwise

## Ent ORM

- Schema definitions in `backend/ent/schema/`
- Generated code in `backend/ent/` — **must be committed**
- Regenerate: `cd backend && go generate ./ent`
- Features enabled: `sql/upsert`, `intercept`, `sql/execquery`, `sql/lock`
- ID type: `int64`

## Frontend Gotchas

- **pnpm only** — `pnpm-lock.yaml` must be committed on any dependency change; CI uses `--frozen-lockfile`
- If `node_modules` was previously created by npm, delete it before `pnpm install` (EPERM errors)
- Frontend build output goes to `backend/internal/web/dist/` (embedded into backend binary)

## Simple Mode

- `RUN_MODE=simple` hides SaaS billing features
- In production also requires `SIMPLE_MODE_CONFIRM=true` or startup fails

## Windows Quirks (from DEV_GUIDE.md)

- No `make` command — use raw commands directly
- psql `$` in bcrypt hashes gets eaten by PowerShell — write SQL to file, use `psql -f`
- psql cannot handle Chinese file paths — copy to ASCII path first
- Use `127.0.0.1` instead of `localhost` for psql (IPv6 issue)

## PR Checklist

- [ ] `go test -tags=unit ./...` passes (in `backend/`)
- [ ] `go test -tags=integration ./...` passes
- [ ] `golangci-lint run ./...` clean
- [ ] `pnpm-lock.yaml` committed if frontend deps changed
- [ ] All interface stubs updated in tests
- [ ] Ent generated code committed if schema changed

## Nginx Note

When reverse-proxying Sub2API, add `underscores_in_headers on;` to nginx `http` block — nginx drops `session_id` header by default, breaking sticky sessions.
