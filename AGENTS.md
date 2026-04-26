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

- `go.mod` declares **go 1.26.1** (source of truth)
- CI verifies `go1.26.1` exactly
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
