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

Apply this sequence for **every** production update. Do not skip, reorder, or
replace steps with unverified cleanup automation, except for the narrowly
defined emergency Pipixia-first procedure below.

1. Confirm the merge branch's staged tree, version, conflict resolution, and
   working tree are clean. Commit and push the merge branch.
2. Merge the reviewed branch into the latest `origin/main` and push `main`.
3. Wait for every configured CI gate to complete. Deploy only a CI-built,
   immutable image SHA. A known baseline failure must be identified explicitly;
   it is never silently treated as a green gate.
4. On both production targets, record the running container names, image SHAs,
   published ports, compose state, and active proxy upstream as the rollback
   point.
5. Deploy **稳如狗 first**. The host only pulls the prebuilt SHA image. Start
   the inactive blue-green slot, wait for application health and migrations,
   atomically switch Nginx/proxy traffic to the healthy new slot, confirm no
   502s, then stop the old slot. Keep the old image as a rollback point, but do
   not leave two application instances running long term.
6. Observe 稳如狗 for about five minutes before touching the second target.
   Check migrations, `UsageLog` writes, balance/package-group/subscription
   entitlement billing, Grok/OpenAI errors, 5xx responses, and container logs.
7. Only when 稳如狗 is healthy, deploy **皮皮虾** with the same blue-green
   sequence: start the inactive slot, health check, atomically switch traffic,
   verify the new slot, stop the old slot, retain its image for rollback, and
   leave exactly one application instance running.
8. Any failed health check, migration, billing smoke test, elevated error rate,
   or unexpected 5xx requires immediate traffic rollback to the recorded old
   SHA. Do not continue the rollout.

### Emergency Pipixia-First Procedure

The normal 稳如狗-first order may be waived only when the project owner explicitly
authorizes an emergency, Pipixia-only rollout in the current task. This exception
does not relax any other production safeguards:

1. All configured CI gates must be green and the image must be the CI-built,
   immutable SHA for the reviewed `main` commit.
2. Before touching Pipixia, record its active container, image SHA, ports,
   compose state, Nginx upstream, and rollback configuration.
3. Use Pipixia's existing blue-green mechanism: start only the inactive slot,
   validate local and public health plus migrations, atomically cut Nginx over,
   verify no 502s, then stop the old slot. Retain its image and configuration
   backup as the rollback point; leave exactly one application instance running.
4. Check the requested billing and error paths after cutover. On any failed
   health check, migration, billing smoke test, elevated error rate, or 5xx,
   immediately restore the recorded old slot and proxy configuration.

Additional non-negotiable rules:

- Do not build images on either production server.
- Do not deploy a floating tag. Use an immutable SHA image only.
- Do not assume both targets share the same proxy implementation. Read and
  verify the target's existing cutover mechanism before operating it.
- Do not add generic Docker cleanup, container-ID matching, port inference, or
  topology-changing deployment logic directly in production. Such changes need
  an isolated test and an explicit user-approved rollout.
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

- `go.mod` declares **go 1.26.4** (source of truth)
- CI verifies `go1.26.4` exactly
- README badge says 1.25.7 — **stale**, ignore it

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
