# Issue Center AI Execution Prompt

Use this prompt for any AI agent that will implement the Sub2API Issue Center. The agent must treat this document and `docs/plans/2026-05-13-issue-center-implementation-plan.md` as the project lead's instructions.

---

## Copy-Paste Prompt

You are implementing the Sub2API Issue Center under direct project-lead supervision.

This is not exploratory work. Follow the plan exactly, keep changes scoped, review your own code seriously, fix failures, commit cleanly, and push the implementation branch.

### Mission

Build the native Sub2API Issue Center described in:

```text
docs/plans/2026-05-13-issue-center-implementation-plan.md
```

The feature must let Sub2API users create public, structured issue reports with screenshot evidence, account email, occurrence time, screenshot/error text, category, severity, comments, precise search, admin moderation, and resolved/locked status behavior.

Do not build a separate project. Do not integrate Discourse, Apache Answer, Zammad, or any external helpdesk. Implement this inside the existing `sub2api` application.

### Authority And Priorities

Follow instructions in this priority order:

1. Existing repository rules in `AGENTS.md`.
2. The implementation plan in `docs/plans/2026-05-13-issue-center-implementation-plan.md`.
3. Existing Sub2API architecture and code patterns.
4. This prompt.
5. Your own judgment.

If there is a conflict, stop and report the conflict before continuing.

### Branch Discipline

Start from `main` and create a dedicated branch.

Run:

```bash
git switch main
git pull --ff-only origin main
git switch -c feature/issue-center
```

If `feature/issue-center` already exists locally:

```bash
git switch feature/issue-center
git pull --ff-only origin feature/issue-center
```

If the remote branch does not exist yet, continue locally and push it after the first commit.

Never commit directly to `main`.
Never force push.
Never push to `upstream`.
Push only to `origin`.

### Architecture Rules

Respect the existing Sub2API backend layering:

- `handler` must not import `repository`, Ent, Gorm, or Redis.
- `service` must not import `repository`, Ent, Gorm, or Redis.
- `repository` owns Ent and SQL access.
- Use existing response helpers, pagination helpers, auth middleware patterns, DTO patterns, and route registration style.
- Ent schema changes require generated Ent code to be committed.
- Wire provider changes require generated Wire code to be committed.

Required backend shape:

```text
backend/internal/domain/support_issue.go
backend/internal/service/support_issue*.go
backend/internal/repository/support_issue_repo.go
backend/internal/handler/dto/support_issue.go
backend/internal/handler/support_issue_handler.go
backend/internal/handler/admin/support_issue_handler.go
backend/internal/server/routes/issues.go
backend/ent/schema/support_issue*.go
backend/migrations/138_add_support_issues.sql
```

Required frontend shape:

```text
frontend/src/types/issues.ts
frontend/src/api/issues.ts
frontend/src/api/admin/issues.ts
frontend/src/views/user/IssuesView.vue
frontend/src/views/user/IssueDetailView.vue
frontend/src/views/user/NewIssueView.vue
frontend/src/views/admin/AdminIssuesView.vue
frontend/src/views/admin/AdminIssueDetailView.vue
```

### Execution Order

Implement in this exact order. Do not jump to frontend before backend contracts exist.

1. Domain constants and exact search parser.
2. Ent schemas and SQL migration.
3. Repository layer.
4. Service layer.
5. User handler DTOs and handlers.
6. Admin handlers and route registration.
7. Screenshot attachment upload and serving.
8. Frontend types and API modules.
9. Public/user views.
10. Admin views.
11. Integration verification and full test pass.

After each major step:

- Run the smallest relevant tests.
- Fix failures immediately.
- Commit the step if tests pass.
- Do not batch huge unrelated changes into one commit.

### MVP Scope

Implement only MVP unless the plan explicitly says otherwise.

MVP includes:

- Public issue list and detail.
- Authenticated issue creation.
- Required structured fields.
- Screenshot upload.
- Comments.
- Status flow: `open`, `needs_info`, `in_progress`, `resolved`, `closed`.
- Reporter/admin resolve.
- Admin reopen/close/status update.
- Resolved/closed issues lock comments.
- Hidden comments/attachments excluded from public API.
- Admin moderation.
- Exact structured search.
- Email masking.
- Tests.

MVP does not include:

- OCR.
- AI semantic search.
- Voting/reputation.
- Full private helpdesk workflows.
- External community platform integration.
- Complex FAQ system.
- Production log lookup automation.

### Search Requirements

Search must be exact and structured first. Support:

```text
id:123
id:ISS-000123
email:user@example.com
email:gmail.com
status:open
status:resolved
category:payment
type:payment
severity:blocked
model:claude
client:claude-code
code:401
code:429
error:insufficient_quota
key:ab12cd
lang:en
lang:zh
has:image
time:2026-05-13
time:2026-05-01..2026-05-13
title:"余额未到账"
error:"Your account is temporarily unavailable"
"exact phrase"
```

Rules:

- Plain terms use AND semantics.
- Quoted phrases are exact phrase/substring matches.
- Public search must not expose raw email or API key suffix.
- Public search must reject or ignore `key:`.
- Admin search may use full email and key suffix.
- Exact ID/public ID match must outrank fuzzy text results.

### Privacy And Security Requirements

Public API must never return:

- full `account_email`
- `account_email_normalized`
- raw `api_key_suffix`
- hidden comments
- hidden attachments
- local filesystem paths
- admin-only moderation metadata

Admin API may return private diagnostic fields behind admin auth only.

Screenshot upload must:

- require login
- limit size
- allow image MIME types only
- sanitize file names
- prevent path traversal
- store files outside frontend source
- serve only visible/public attachments

Frontend must not render user text with unsafe `v-html`.

### Review Requirements

Before every commit, perform a serious self-review.

Review checklist:

- Does this change follow `AGENTS.md`?
- Does it preserve backend layering?
- Are public/private DTOs separated correctly?
- Can a public route leak email, key suffix, hidden content, local paths, or admin metadata?
- Are status transitions enforced on the server?
- Are locked issues unable to receive comments?
- Are reporter/admin permissions enforced server-side?
- Are search filters parsed deterministically?
- Are migrations idempotent and compatible with Postgres and SQLite expectations?
- Did Ent/Wire generated files update when required?
- Did frontend types match backend DTOs?
- Are tests meaningful, not just snapshot noise?
- Did you avoid unrelated refactors?

If review finds a problem:

1. Do not commit yet.
2. Fix it.
3. Re-run relevant tests.
4. Review again.

### Fix Requirements

When tests fail:

- Read the first real failure, not just the final summary.
- Group related failures.
- Fix root causes, not symptoms.
- Do not delete tests to make the suite pass.
- Do not loosen security or privacy requirements to make tests pass.
- If an interface changed, update all stubs/mocks.
- If generated code is stale, regenerate it.

Backend generation commands:

```bash
cd backend
go generate ./ent
go generate ./cmd/server
```

Backend tests:

```bash
cd backend
go test -tags=unit ./...
go test -tags=integration ./...
golangci-lint run ./...
```

Frontend tests:

```bash
cd frontend
pnpm install
pnpm run lint:check
pnpm run typecheck
pnpm run test:run
pnpm run build
```

### Commit Requirements

Use focused commits. Good commit examples:

```text
feat: add support issue search parser
feat: add support issue persistence schema
feat: add support issue repository
feat: add support issue service
feat: expose support issue APIs
feat: add support issue screenshot uploads
feat: add support issue frontend API
feat: add public issue center views
feat: add admin issue center views
test: verify support issue center flows
fix: lock resolved support issues against comments
```

Before each commit:

```bash
git status --short
git diff --check
```

Then stage only relevant files:

```bash
git add <files>
git commit -m "<message>"
```

Do not commit:

- local `.env`
- logs
- coverage output
- build artifacts unless the repo already expects them
- unrelated formatting churn
- unrelated user changes

### Push Requirements

After the feature is implemented, reviewed, fixed, and tests pass:

```bash
git status --short
git push -u origin feature/issue-center
```

If the branch already tracks origin:

```bash
git push origin feature/issue-center
```

Never use:

```bash
git push --force
git push upstream
```

### Final Report Required

When done, report:

```text
Branch:
Commits:
Files changed:
Backend tests run:
Frontend tests run:
Known limitations:
Open questions:
Push status:
```

If anything could not be completed, say exactly what blocked it and what remains.

### Stop Conditions

Stop and ask for project-lead guidance if:

- A requirement conflicts with existing Sub2API architecture.
- A migration could destroy existing data.
- A public/private data leak risk cannot be resolved cleanly.
- Tests require external services unavailable locally.
- The branch has unrelated dirty changes you did not create.
- A production deployment/build is requested on a production host.

You are expected to be careful, exact, and accountable. Do not improvise a different product.
