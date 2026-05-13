# Sub2API Issue Center Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a public, structured, searchable issue center inside Sub2API so users must report problems with enough evidence, admins can resolve them once, and future users can precisely search the public archive.

**Architecture:** Implement Issue Center as a native Sub2API capability, not as a separate service. Follow the existing backend layering (`repository -> service -> handler -> routes`) and existing Vue/Vite frontend API/view patterns. Store issue data in Ent-backed tables, use Postgres exact/GIN/trigram indexes when available, and provide SQLite-compatible fallbacks.

**Tech Stack:** Go + Gin + Ent + Wire + PostgreSQL/SQLite migrations; Vue 3 + Vite + TypeScript + Pinia-style API modules; pnpm; existing Sub2API auth/admin middleware.

---

## 1. Product Capability Contract

### Capability

Sub2API users and admins gain a public Issue Center. A logged-in user can create a structured public problem report with screenshot evidence, account email, occurrence time, error text, model/client context, and severity. Other logged-in users can comment. The reporter or an admin can mark the issue as resolved; resolved issues are locked and cannot receive more comments. Admins can moderate sensitive content, update statuses, reopen issues, and use complete private fields for diagnosis. All public issues are searchable with exact query syntax.

### Fixed Rules

- Issue content is public by default.
- Full account email is private and admin-only; public responses return a masked email.
- Authenticated users can create issues and comments.
- Anonymous visitors may browse and search public issues only if existing frontend auth flow allows public routes; MVP should choose public read endpoints but hide creation/comment controls until login.
- Reporter and admin can resolve an issue.
- Only admin can reopen, force lock, hide comments, hide attachments, edit moderation fields, or change a resolved issue back to an active state.
- Resolved or closed issues are locked. Locked issues reject new comments.
- Hidden comments and hidden attachments are excluded from public responses but remain visible in admin responses with moderation metadata.
- Search must prefer exact structured filters over fuzzy matching.
- AI/semantic search is out of scope for MVP.
- OCR is out of scope for MVP; user-entered screenshot text is required. OCR can be added in a later phase.

### Non-Goals

- Do not build a full private helpdesk.
- Do not import Discourse, Apache Answer, Zammad, or another service.
- Do not add voting, reputation, badges, or gamified community features.
- Do not allow users to edit other users' issues/comments.
- Do not expose full emails, API keys, payment IDs, or tokens publicly.
- Do not build direct production log lookup into MVP. Add admin shortcut fields now so log-linking can be added later.

### Actors

- `visitor`: can list/search/view public issue content.
- `user`: can create issues, comment on unlocked issues, resolve own issue, and view public data.
- `reporter`: the user who created an issue; can add comments and resolve their issue.
- `admin`: can view private fields, change status, moderate content, reopen/lock, and resolve any issue.

### Issue States

Use exactly these statuses in MVP:

- `open`: newly submitted and waiting for admin/user/community response.
- `needs_info`: admin or community needs more evidence from reporter.
- `in_progress`: admin has acknowledged and is investigating.
- `resolved`: reporter or admin says the issue is solved; locked.
- `closed`: admin closed as duplicate/invalid/spam; locked.

Allowed transitions:

| From | To | Actor | Notes |
| --- | --- | --- | --- |
| `open` | `needs_info` | admin | Ask reporter for missing details. |
| `open` | `in_progress` | admin | Admin accepted the issue. |
| `open` | `resolved` | reporter/admin | Lock immediately. |
| `open` | `closed` | admin | Lock immediately. |
| `needs_info` | `open` | reporter/admin | Reporter supplied info or admin reopens. |
| `needs_info` | `in_progress` | admin | Admin starts investigation. |
| `needs_info` | `resolved` | reporter/admin | Lock immediately. |
| `needs_info` | `closed` | admin | Lock immediately. |
| `in_progress` | `needs_info` | admin | More information needed. |
| `in_progress` | `resolved` | reporter/admin | Lock immediately. |
| `in_progress` | `closed` | admin | Lock immediately. |
| `resolved` | `open` | admin | Reopen only. Clear `locked_at`. |
| `closed` | `open` | admin | Reopen only. Clear `locked_at`. |

## 2. Repository Context

### Must Follow

- Work inside `sub2api`, not a separate project.
- Backend routes are registered in `backend/internal/server/router.go` via route modules in `backend/internal/server/routes`.
- Admin routes live under `/api/v1/admin`.
- User routes live under `/api/v1`.
- Existing backend layering is strict:
  - `handler` must not import `repository`, Ent, Gorm, or Redis.
  - `service` must not import `repository`, Ent, Gorm, or Redis.
  - `repository` owns Ent/SQL access.
- Wire provider sets are in:
  - `backend/internal/repository/wire.go`
  - `backend/internal/service/wire.go`
  - `backend/internal/handler/wire.go`
  - `backend/cmd/server/wire.go`
- Ent schema files are in `backend/ent/schema/`.
- Generated Ent code in `backend/ent/` must be committed after schema changes.
- Migrations live in `backend/migrations/`.
- Frontend API modules live in:
  - `frontend/src/api/*.ts`
  - `frontend/src/api/admin/*.ts`
- Frontend routes are in `frontend/src/router/index.ts`.
- Sidebar menu is in `frontend/src/components/layout/AppSidebar.vue`.
- Frontend package manager is pnpm only.

### Commands

Run backend commands from `backend/`:

```bash
go generate ./ent
go generate ./cmd/server
go test -tags=unit ./...
go test -tags=integration ./...
golangci-lint run ./...
```

Run frontend commands from `frontend/`:

```bash
pnpm install
pnpm run lint:check
pnpm run typecheck
pnpm run test:run
pnpm run build
```

Root checks:

```bash
make test
make secret-scan
```

On Windows, run the raw backend/frontend commands if `make` is unavailable.

## 3. Data Model

### Ent Schemas To Create

Create:

- `backend/ent/schema/support_issue.go`
- `backend/ent/schema/support_issue_comment.go`
- `backend/ent/schema/support_issue_attachment.go`
- `backend/ent/schema/support_issue_event.go`

### Table: `support_issues`

Fields:

| Field | Type | Required | Public? | Notes |
| --- | --- | --- | --- | --- |
| `id` | bigint | yes | yes | Ent int64 ID. |
| `public_id` | string | yes | yes | Stable display ID like `ISS-000001`; unique. |
| `title` | string(160) | yes | yes | Required; trim spaces. |
| `description` | text | yes | yes | Required; Markdown/plain text allowed; render escaped/sanitized. |
| `account_email` | string(255) | yes | admin only | Raw email for admin diagnosis. |
| `account_email_normalized` | string(255) | yes | no | Lowercase trimmed email for exact search. |
| `account_email_masked` | string(255) | yes | yes | Public masked email. |
| `occurred_at` | timestamptz | yes | yes | User-provided occurrence time. |
| `screenshot_text` | text | yes | yes | Required manual transcription of screenshot error text. |
| `screenshot_language` | string(16) | yes | yes | `zh`, `en`, `mixed`, `unknown`. |
| `category` | string(32) | yes | yes | See category enum below. |
| `severity` | string(32) | yes | yes | See severity enum below. |
| `status` | string(32) | yes | yes | `open`, `needs_info`, `in_progress`, `resolved`, `closed`. |
| `model_name` | string(255) | no | yes | Optional but recommended. |
| `client_name` | string(120) | no | yes | Claude Code, Cherry Studio, LobeHub, custom script, etc. |
| `http_status` | int | no | yes | 400/401/429/500/etc. |
| `error_code` | string(120) | no | yes | Upstream or Sub2API error code. |
| `api_key_suffix` | string(16) | no | admin only by default | Last 6 chars; can public-mask as `****abcd12` if shown. |
| `created_by_user_id` | bigint | yes | no | FK to users. |
| `resolved_by_user_id` | bigint | no | admin public as display role only | FK to users. |
| `resolved_at` | timestamptz | no | yes | Set when resolved. |
| `locked_at` | timestamptz | no | yes | Set when resolved/closed or force-locked. |
| `last_comment_at` | timestamptz | no | yes | Sort by activity. |
| `comment_count` | int | yes | yes | Denormalized visible comment count. |
| `hidden_comment_count` | int | yes | admin only | Moderation count. |
| `attachment_count` | int | yes | yes | Visible attachments. |
| `search_text` | text | yes | no | Normalized combined search text. |
| `created_at` | timestamptz | yes | yes | Default now. |
| `updated_at` | timestamptz | yes | yes | Update default now. |

Category enum:

- `login`
- `payment`
- `api_call`
- `model_unavailable`
- `api_key`
- `balance`
- `subscription`
- `channel`
- `other`

Severity enum:

- `blocked`: cannot use at all.
- `partial`: some requests/features fail.
- `intermittent`: occasional failure.
- `question`: usage question.

### Table: `support_issue_comments`

Fields:

| Field | Type | Required | Public? | Notes |
| --- | --- | --- | --- | --- |
| `id` | bigint | yes | yes | Ent int64 ID. |
| `issue_id` | bigint | yes | yes | FK. |
| `author_user_id` | bigint | yes | public display only | FK to users. |
| `author_role` | string(16) | yes | yes | `admin` or `user` snapshot. |
| `content` | text | yes | yes if not hidden | Required. |
| `hidden_at` | timestamptz | no | no | Hidden comments excluded from public. |
| `hidden_by_user_id` | bigint | no | admin only | Moderator. |
| `hide_reason` | string(255) | no | admin only | Sensitive info, spam, abuse. |
| `created_at` | timestamptz | yes | yes | Default now. |
| `updated_at` | timestamptz | yes | yes | Update default now. |

### Table: `support_issue_attachments`

Fields:

| Field | Type | Required | Public? | Notes |
| --- | --- | --- | --- | --- |
| `id` | bigint | yes | yes | Ent int64 ID. |
| `issue_id` | bigint | yes | yes | FK. |
| `uploaded_by_user_id` | bigint | yes | no | FK. |
| `file_path` | string(512) | yes | no | Server path or object key. |
| `file_url` | string(512) | yes | yes if visible | Served URL. |
| `file_name` | string(255) | yes | yes | Original name sanitized. |
| `mime_type` | string(100) | yes | yes | Allow image PNG/JPEG/WebP/GIF only in MVP. |
| `size_bytes` | bigint | yes | yes | Enforce limit. |
| `ocr_text` | text | no | future | Empty in MVP. |
| `visibility` | string(16) | yes | yes | `public`, `hidden`. |
| `hidden_at` | timestamptz | no | no | Admin moderation. |
| `hidden_by_user_id` | bigint | no | admin only | Moderator. |
| `created_at` | timestamptz | yes | yes | Default now. |

### Table: `support_issue_events`

Fields:

| Field | Type | Required | Public? | Notes |
| --- | --- | --- | --- | --- |
| `id` | bigint | yes | admin only | Audit ID. |
| `issue_id` | bigint | yes | admin only | FK. |
| `actor_user_id` | bigint | yes | admin only | FK. |
| `event_type` | string(32) | yes | admin only | `created`, `commented`, `status_changed`, `locked`, `reopened`, `comment_hidden`, `attachment_hidden`. |
| `from_status` | string(32) | no | admin only | Previous status. |
| `to_status` | string(32) | no | admin only | Next status. |
| `metadata` | jsonb/text | yes | admin only | JSON. Use JSONB for Postgres. |
| `created_at` | timestamptz | yes | admin only | Default now. |

### Required Indexes

Base indexes:

```sql
CREATE UNIQUE INDEX idx_support_issues_public_id ON support_issues(public_id);
CREATE INDEX idx_support_issues_status_updated ON support_issues(status, updated_at DESC);
CREATE INDEX idx_support_issues_status_last_comment ON support_issues(status, last_comment_at DESC);
CREATE INDEX idx_support_issues_category_status ON support_issues(category, status);
CREATE INDEX idx_support_issues_created_by ON support_issues(created_by_user_id, created_at DESC);
CREATE INDEX idx_support_issues_email_norm ON support_issues(account_email_normalized);
CREATE INDEX idx_support_issues_occurred_at ON support_issues(occurred_at DESC);
CREATE INDEX idx_support_issues_http_status ON support_issues(http_status);
CREATE INDEX idx_support_issues_error_code ON support_issues(error_code);
CREATE INDEX idx_support_issue_comments_issue_created ON support_issue_comments(issue_id, created_at ASC);
CREATE INDEX idx_support_issue_comments_hidden ON support_issue_comments(issue_id, hidden_at);
CREATE INDEX idx_support_issue_attachments_issue ON support_issue_attachments(issue_id, visibility);
CREATE INDEX idx_support_issue_events_issue_created ON support_issue_events(issue_id, created_at DESC);
```

Postgres best-effort search indexes:

```sql
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX IF NOT EXISTS idx_support_issues_title_trgm
  ON support_issues USING gin (title gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_support_issues_error_text_trgm
  ON support_issues USING gin (screenshot_text gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_support_issues_search_text_trgm
  ON support_issues USING gin (search_text gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_support_issue_comments_content_trgm
  ON support_issue_comments USING gin (content gin_trgm_ops);
```

Follow the existing style from `backend/migrations/065_add_search_trgm_indexes.sql`: try `pg_trgm`, then create trigram indexes only when the extension exists.

## 4. Backend Domain And Service Design

### Create Domain Constants

Create `backend/internal/domain/support_issue.go`.

Include:

- status constants.
- category constants.
- severity constants.
- screenshot language constants.
- event type constants.
- validation helper functions.
- allowed status transition function.

Required validation:

- title length 4-160 after trim.
- description length 10-12000 after trim.
- email parse via `net/mail`, lower-case normalized.
- occurred_at required and not more than 30 days in the future.
- screenshot_text length 2-8000 after trim.
- category/severity/status/language must be known values.
- `api_key_suffix`, if present, must be 4-16 alphanumeric-ish characters after stripping spaces.
- `http_status`, if present, must be 100-599.

### Create Service Port And DTO Types

Create `backend/internal/service/support_issue.go`.

Types:

```go
type SupportIssue struct { ... }
type SupportIssueComment struct { ... }
type SupportIssueAttachment struct { ... }
type SupportIssueEvent struct { ... }

type CreateSupportIssueInput struct { ... }
type ListSupportIssueFilters struct { ... }
type SearchSupportIssueQuery struct { ... }
type AddSupportIssueCommentInput struct { ... }
type UpdateSupportIssueStatusInput struct { ... }
type HideSupportIssueCommentInput struct { ... }
type HideSupportIssueAttachmentInput struct { ... }

type SupportIssueRepository interface { ... }
```

Repository interface should include:

```go
CreateIssue(ctx context.Context, issue *SupportIssue, attachments []SupportIssueAttachment, event SupportIssueEvent) error
GetIssue(ctx context.Context, id int64, includeHidden bool) (*SupportIssue, error)
ListIssues(ctx context.Context, params pagination.PaginationParams, filters ListSupportIssueFilters) ([]SupportIssue, *pagination.PaginationResult, error)
SearchIssues(ctx context.Context, params pagination.PaginationParams, query SearchSupportIssueQuery) ([]SupportIssue, *pagination.PaginationResult, error)
AddComment(ctx context.Context, comment *SupportIssueComment, event SupportIssueEvent) error
UpdateStatus(ctx context.Context, issueID int64, nextStatus string, actorUserID int64, actorIsAdmin bool, event SupportIssueEvent) (*SupportIssue, error)
HideComment(ctx context.Context, input HideSupportIssueCommentInput, event SupportIssueEvent) error
HideAttachment(ctx context.Context, input HideSupportIssueAttachmentInput, event SupportIssueEvent) error
ListEvents(ctx context.Context, issueID int64) ([]SupportIssueEvent, error)
```

Service methods:

```go
Create(ctx context.Context, actor AuthActor, input CreateSupportIssueInput) (*SupportIssue, error)
ListPublic(ctx context.Context, params pagination.PaginationParams, filters ListSupportIssueFilters) (...)
SearchPublic(ctx context.Context, params pagination.PaginationParams, rawQuery string, filters ListSupportIssueFilters) (...)
GetPublic(ctx context.Context, issueID int64) (*SupportIssue, error)
AddComment(ctx context.Context, actor AuthActor, issueID int64, content string) (*SupportIssueComment, error)
Resolve(ctx context.Context, actor AuthActor, issueID int64) (*SupportIssue, error)
AdminList(...)
AdminGet(...)
AdminUpdateStatus(...)
AdminHideComment(...)
AdminHideAttachment(...)
AdminReopen(...)
AdminListEvents(...)
SuggestSimilar(ctx context.Context, actor AuthActor, input SuggestSimilarInput) ([]SupportIssue, error)
```

Define a small `AuthActor` struct in service or handler DTO layer:

```go
type SupportIssueActor struct {
    UserID int64
    Email string
    Role string
    IsAdmin bool
}
```

Handlers convert middleware auth subject/user to this actor.

### Service Business Logic

Service owns:

- trimming and validating inputs.
- email normalization and masking.
- checking reporter/admin permission for resolve.
- checking admin permission for admin operations.
- checking issue lock before comments.
- status transition validation.
- maintaining `resolved_at` and `locked_at`.
- generating `public_id`.
- constructing `search_text`.
- creating event records for every mutation.

Repository owns:

- Ent queries.
- transactions.
- dialect-specific search SQL.
- pagination.
- updating denormalized counters.

### Public ID

Use `ISS-` plus zero-padded database ID after creation if possible:

1. Create issue with temporary `public_id` equal to a UUID or `PENDING-<uuid>`.
2. After Ent returns ID, update `public_id` to `ISS-%06d`.
3. Wrap both steps in a transaction.

Alternative acceptable implementation:

- Use `ISS-<unix-ts>-<short-random>` if avoiding second update.

Prefer `ISS-%06d` because users can search exact IDs easily.

## 5. Search Design

### Query Syntax

Support these filters:

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

- Plain tokens use AND semantics.
- Quoted phrases require phrase/substring match.
- Structured filters are exact or normalized prefix/contains depending on field:
  - `id`: exact.
  - `email`: exact if contains `@`, domain/substring if no exact full email.
  - `status`, `category`, `severity`, `lang`, `http_status`: exact.
  - `model`, `client`, `error_code`: case-insensitive contains.
  - `key`: exact suffix match against `api_key_suffix`, admin only. Public search must ignore `key:` or return bad request.
  - `has:image`: attachment_count > 0.
  - `time`: date or date range against `occurred_at`.
- Public search never returns raw email or key suffix.
- Admin search can search raw email and key suffix.

### Search Parser

Create `backend/internal/service/support_issue_search.go`.

Test parser independently. It should output:

```go
type ParsedSupportIssueSearch struct {
    IssueID *int64
    PublicID string
    Email string
    Status []string
    Category []string
    Severity []string
    Model []string
    Client []string
    HTTPStatus []int
    ErrorCode []string
    APIKeySuffix string
    ScreenshotLanguage []string
    HasImage *bool
    OccurredFrom *time.Time
    OccurredTo *time.Time
    TitlePhrases []string
    ErrorPhrases []string
    ExactPhrases []string
    Terms []string
}
```

Parser tests:

- `id:123` parses ID.
- `id:ISS-000123` parses public ID.
- `time:2026-05-01..2026-05-13` parses inclusive local date range.
- `"rate limit" code:429 status:open` keeps exact phrase plus filters.
- unclosed quote returns a validation error.
- unknown field returns a validation error with a helpful message.

### Ranking

Repository should sort by:

1. Exact ID/public ID match.
2. Exact email match.
3. Exact `http_status`/`error_code`.
4. Exact phrase in title.
5. Exact phrase in `screenshot_text`.
6. Plain tokens in title.
7. Plain tokens in `search_text`.
8. `last_comment_at DESC NULLS LAST`, then `created_at DESC`.

MVP can implement this with simpler ordering if it still prioritizes exact filters and deterministic newest activity.

## 6. Backend API Contract

### Public/User Endpoints

Register these under `/api/v1/issues`.

| Method | Path | Auth | Handler |
| --- | --- | --- | --- |
| `GET` | `/issues` | public read | list/search public issues |
| `GET` | `/issues/:id` | public read | get public issue detail |
| `POST` | `/issues` | user | create issue |
| `POST` | `/issues/:id/comments` | user | add comment |
| `PATCH` | `/issues/:id/resolve` | reporter/admin | resolve and lock |
| `POST` | `/issues/search-suggestions` | user | suggest similar existing issues |

`GET /issues` query:

```text
q
status
category
severity
has_image
sort_by
sort_order
page
page_size
```

Default page size: 20. Max page size: 100.

`POST /issues` JSON:

```json
{
  "title": "Claude Code returns 401",
  "description": "Requests started failing after 20:30. Balance is positive.",
  "account_email": "user@example.com",
  "occurred_at": "2026-05-13T20:30:00+08:00",
  "screenshot_text": "401 unauthorized",
  "screenshot_language": "en",
  "category": "api_call",
  "severity": "blocked",
  "model_name": "claude-sonnet-4-5",
  "client_name": "Claude Code",
  "http_status": 401,
  "error_code": "unauthorized",
  "api_key_suffix": "ab12cd",
  "attachment_ids": [1]
}
```

`POST /issues/:id/comments` JSON:

```json
{
  "content": "I have the same issue on Cherry Studio since 21:00."
}
```

### Attachment Upload

MVP route:

```text
POST /api/v1/issues/attachments
```

Auth: user.

Request: multipart form with `file`.

Rules:

- Max file size: 5 MB per file.
- Allowed MIME: `image/png`, `image/jpeg`, `image/webp`, `image/gif`.
- Return temporary attachment ID that can be used in issue creation.
- Do not expose local filesystem paths.
- If existing project already has a file upload/storage pattern, reuse it.

Response:

```json
{
  "id": 1,
  "file_url": "/api/v1/issues/attachments/1/file",
  "file_name": "error.png",
  "mime_type": "image/png",
  "size_bytes": 102400
}
```

Serving route:

```text
GET /api/v1/issues/attachments/:id/file
```

Public only if attachment visibility is `public` and issue is public.

### Admin Endpoints

Register these under `/api/v1/admin/issues`.

| Method | Path | Handler |
| --- | --- | --- |
| `GET` | `/admin/issues` | admin list/search with private fields |
| `GET` | `/admin/issues/:id` | admin detail |
| `PATCH` | `/admin/issues/:id/status` | set status |
| `POST` | `/admin/issues/:id/reopen` | reopen locked issue |
| `POST` | `/admin/issues/:id/lock` | lock without changing status if needed |
| `POST` | `/admin/issues/:id/comments/:comment_id/hide` | hide comment |
| `POST` | `/admin/issues/:id/attachments/:attachment_id/hide` | hide attachment |
| `GET` | `/admin/issues/:id/events` | audit trail |

`PATCH /admin/issues/:id/status` JSON:

```json
{
  "status": "needs_info",
  "reason": "Need exact screenshot text and timestamp."
}
```

`POST hide` JSON:

```json
{
  "reason": "Contains API key"
}
```

### Response DTOs

Public issue:

```json
{
  "id": 123,
  "public_id": "ISS-000123",
  "title": "Claude Code returns 401",
  "description": "...",
  "account_email_masked": "u***@example.com",
  "occurred_at": "2026-05-13T20:30:00+08:00",
  "screenshot_text": "401 unauthorized",
  "screenshot_language": "en",
  "category": "api_call",
  "severity": "blocked",
  "status": "open",
  "model_name": "claude-sonnet-4-5",
  "client_name": "Claude Code",
  "http_status": 401,
  "error_code": "unauthorized",
  "resolved_at": null,
  "locked_at": null,
  "comment_count": 2,
  "attachment_count": 1,
  "created_at": "...",
  "updated_at": "...",
  "last_comment_at": "...",
  "comments": [],
  "attachments": []
}
```

Admin issue extends public issue with:

```json
{
  "account_email": "user@example.com",
  "account_email_normalized": "user@example.com",
  "api_key_suffix": "ab12cd",
  "created_by_user_id": 99,
  "resolved_by_user_id": null,
  "hidden_comment_count": 1
}
```

## 7. Backend Files To Modify

Create:

- `backend/internal/domain/support_issue.go`
- `backend/internal/service/support_issue.go`
- `backend/internal/service/support_issue_service.go`
- `backend/internal/service/support_issue_search.go`
- `backend/internal/repository/support_issue_repo.go`
- `backend/internal/handler/dto/support_issue.go`
- `backend/internal/handler/support_issue_handler.go`
- `backend/internal/handler/admin/support_issue_handler.go`
- `backend/internal/server/routes/issues.go`
- `backend/ent/schema/support_issue.go`
- `backend/ent/schema/support_issue_comment.go`
- `backend/ent/schema/support_issue_attachment.go`
- `backend/ent/schema/support_issue_event.go`
- `backend/migrations/138_add_support_issues.sql`

Modify:

- `backend/internal/repository/wire.go`: add `NewSupportIssueRepository`.
- `backend/internal/service/wire.go`: add `NewSupportIssueService`.
- `backend/internal/handler/wire.go`: add user/admin support issue handlers to provider set and structs.
- `backend/internal/handler/handler.go`: add `SupportIssue *SupportIssueHandler` and `Admin.SupportIssue *admin.SupportIssueHandler`.
- `backend/internal/server/router.go`: call `routes.RegisterIssueRoutes(...)`.
- `backend/internal/server/routes/admin.go`: either call `registerSupportIssueRoutes(admin, h)` or keep issue admin routes inside new `routes/issues.go`.
- `backend/cmd/server/wire.go`: usually unchanged unless new cleanup dependency is needed.
- `backend/cmd/server/wire_gen.go`: generated by `go generate ./cmd/server`.

## 8. Frontend Design

### Routes

Add to `frontend/src/router/index.ts`:

```text
/issues
/issues/new
/issues/:id
/admin/issues
/admin/issues/:id
```

Route metadata:

- `/issues`: `requiresAuth: false`, title `Issue Center`.
- `/issues/:id`: `requiresAuth: false`.
- `/issues/new`: `requiresAuth: true`, `requiresAdmin: false`.
- `/admin/issues`: `requiresAuth: true`, `requiresAdmin: true`.
- `/admin/issues/:id`: `requiresAuth: true`, `requiresAdmin: true`.

### Frontend API Files

Create:

- `frontend/src/api/issues.ts`
- `frontend/src/api/admin/issues.ts`

Modify:

- `frontend/src/api/index.ts`: export `issuesAPI`.
- `frontend/src/api/admin/index.ts`: export `adminIssuesAPI`.
- `frontend/src/types/index.ts`: add issue types or import from dedicated issue types file.

Recommended dedicated type file:

- `frontend/src/types/issues.ts`

Then export from `frontend/src/types/index.ts`.

### Views

Create:

- `frontend/src/views/user/IssuesView.vue`
- `frontend/src/views/user/IssueDetailView.vue`
- `frontend/src/views/user/NewIssueView.vue`
- `frontend/src/views/admin/AdminIssuesView.vue`
- `frontend/src/views/admin/AdminIssueDetailView.vue`

### Sidebar

Modify `frontend/src/components/layout/AppSidebar.vue`.

Add a user nav item:

```text
问题中心 -> /issues
```

Add an admin nav item:

```text
问题处理 -> /admin/issues
```

Use existing icon pattern in the file. Do not add a new icon library unless the project already has one available and used.

### UX Requirements

`IssuesView`:

- Compact operational layout, not a landing page.
- Search bar at top.
- Filter controls: status, category, severity, has image, sort.
- Results list with status badge, category, severity, masked email, occurred time, comment count, last activity.
- Empty state with button to submit issue.

`NewIssueView`:

- Structured form with required fields.
- File upload for screenshots.
- Show privacy warning near upload.
- On title/error text blur or before submit, call suggestions API.
- Show similar issues with links.
- User must confirm "not duplicate" when suggestions exist.
- Validate client-side before API call but rely on server validation.

`IssueDetailView`:

- Show issue fields clearly.
- Show visible attachments.
- Show comments in chronological order.
- If locked, hide comment box and show lock reason/status.
- If current user is reporter and not locked, show "Mark resolved".
- If current user is anonymous, show login prompt to comment.

`AdminIssuesView`:

- Same as public list, plus full email search, key suffix search, status controls.
- Show private fields in table columns where useful.

`AdminIssueDetailView`:

- Show full email, user ID, API key suffix, raw status controls.
- Show hidden comments/attachments with moderation metadata.
- Show event timeline.
- Buttons: set status, reopen, lock, hide comment, hide attachment.

### Frontend Types

Create types matching DTOs:

```ts
export type SupportIssueStatus = 'open' | 'needs_info' | 'in_progress' | 'resolved' | 'closed'
export type SupportIssueCategory = 'login' | 'payment' | 'api_call' | 'model_unavailable' | 'api_key' | 'balance' | 'subscription' | 'channel' | 'other'
export type SupportIssueSeverity = 'blocked' | 'partial' | 'intermittent' | 'question'
export type ScreenshotLanguage = 'zh' | 'en' | 'mixed' | 'unknown'
```

Use `BasePaginationResponse<T>` already in `frontend/src/types/index.ts`.

## 9. Implementation Tasks

### Task 1: Domain Constants And Search Parser Tests

**Files:**

- Create: `backend/internal/domain/support_issue.go`
- Create: `backend/internal/service/support_issue_search.go`
- Test: `backend/internal/service/support_issue_search_test.go`

**Steps:**

1. Write unit tests for search parsing.
2. Implement parser and validation helpers.
3. Run:

```bash
cd backend
go test -tags=unit ./internal/service -run SupportIssueSearch
```

Expected: parser tests pass.

Commit:

```bash
git add backend/internal/domain/support_issue.go backend/internal/service/support_issue_search.go backend/internal/service/support_issue_search_test.go
git commit -m "feat: add support issue search parser"
```

### Task 2: Ent Schemas And Migration

**Files:**

- Create four Ent schema files under `backend/ent/schema/`.
- Create `backend/migrations/138_add_support_issues.sql`.
- Generated: `backend/ent/**`.

**Steps:**

1. Write Ent schemas.
2. Write SQL migration with Postgres best-effort `pg_trgm` section.
3. Run:

```bash
cd backend
go generate ./ent
```

4. Run:

```bash
go test -tags=unit ./ent/...
```

Commit:

```bash
git add backend/ent backend/migrations/138_add_support_issues.sql
git commit -m "feat: add support issue persistence schema"
```

### Task 3: Repository Layer

**Files:**

- Create: `backend/internal/repository/support_issue_repo.go`
- Modify: `backend/internal/repository/wire.go`
- Test: `backend/internal/repository/support_issue_repo_integration_test.go`

**Steps:**

1. Write integration tests using existing repository integration style.
2. Cover create issue with attachments/events.
3. Cover list filters.
4. Cover public hidden content exclusion.
5. Cover status update transaction.
6. Cover search exact filters.
7. Implement repository.
8. Run:

```bash
cd backend
go test -tags=integration ./internal/repository -run SupportIssue
```

Commit:

```bash
git add backend/internal/repository/support_issue_repo.go backend/internal/repository/wire.go backend/internal/repository/support_issue_repo_integration_test.go
git commit -m "feat: add support issue repository"
```

### Task 4: Service Layer

**Files:**

- Create: `backend/internal/service/support_issue.go`
- Create: `backend/internal/service/support_issue_service.go`
- Modify: `backend/internal/service/wire.go`
- Test: `backend/internal/service/support_issue_service_test.go`

**Steps:**

1. Write unit tests with fake repository.
2. Cover create validation.
3. Cover email masking.
4. Cover reporter resolve permission.
5. Cover non-reporter resolve rejection.
6. Cover admin reopen.
7. Cover comment rejected when locked.
8. Implement service.
9. Run:

```bash
cd backend
go test -tags=unit ./internal/service -run SupportIssue
```

Commit:

```bash
git add backend/internal/service/support_issue*.go backend/internal/service/wire.go backend/internal/service/support_issue_service_test.go
git commit -m "feat: add support issue service"
```

### Task 5: Handler DTOs And User Handlers

**Files:**

- Create: `backend/internal/handler/dto/support_issue.go`
- Create: `backend/internal/handler/support_issue_handler.go`
- Modify: `backend/internal/handler/handler.go`
- Modify: `backend/internal/handler/wire.go`
- Test: `backend/internal/handler/support_issue_handler_test.go`

**Steps:**

1. Write handler tests for bad request, unauthorized create, public list, create, comment, resolve.
2. Implement DTO conversion with public/private field separation.
3. Implement handlers using existing `response.Success`, `response.ErrorFrom`, etc.
4. Run:

```bash
cd backend
go test -tags=unit ./internal/handler -run SupportIssue
```

Commit:

```bash
git add backend/internal/handler/dto/support_issue.go backend/internal/handler/support_issue_handler.go backend/internal/handler/handler.go backend/internal/handler/wire.go backend/internal/handler/support_issue_handler_test.go
git commit -m "feat: add user support issue handlers"
```

### Task 6: Admin Handlers And Routes

**Files:**

- Create: `backend/internal/handler/admin/support_issue_handler.go`
- Create: `backend/internal/server/routes/issues.go`
- Modify: `backend/internal/server/router.go`
- Modify: `backend/internal/server/routes/admin.go` if needed.
- Modify: `backend/internal/handler/handler.go`
- Modify: `backend/internal/handler/wire.go`
- Test: `backend/internal/server/api_contract_test.go` or new route test.

**Steps:**

1. Write route/API contract tests for user and admin issue endpoints.
2. Implement admin handler.
3. Register routes.
4. Run Wire:

```bash
cd backend
go generate ./cmd/server
```

5. Run:

```bash
go test -tags=unit ./internal/server -run Issue
go test -tags=unit ./cmd/server
```

Commit:

```bash
git add backend/internal/handler/admin/support_issue_handler.go backend/internal/server/routes/issues.go backend/internal/server/router.go backend/internal/server/routes/admin.go backend/internal/handler/handler.go backend/internal/handler/wire.go backend/cmd/server/wire_gen.go
git commit -m "feat: expose support issue APIs"
```

### Task 7: Attachment Upload

**Files:**

- Modify: `backend/internal/service/support_issue*.go`
- Modify: `backend/internal/repository/support_issue_repo.go`
- Modify: `backend/internal/handler/support_issue_handler.go`
- Test: handler/repository tests.

**Steps:**

1. Check if project has an existing upload/storage helper. Reuse if present.
2. Add upload endpoint with MIME and size validation.
3. Store files under a configurable local directory in MVP, such as `data/support-issue-attachments`.
4. Generate safe server file names.
5. Serve only visible public attachments.
6. Run handler tests.

Commit:

```bash
git add backend/internal/service backend/internal/repository backend/internal/handler
git commit -m "feat: add support issue screenshot uploads"
```

### Task 8: Frontend Types And API Modules

**Files:**

- Create: `frontend/src/types/issues.ts`
- Modify: `frontend/src/types/index.ts`
- Create: `frontend/src/api/issues.ts`
- Create: `frontend/src/api/admin/issues.ts`
- Modify: `frontend/src/api/index.ts`
- Modify: `frontend/src/api/admin/index.ts`
- Test: `frontend/src/api/__tests__/issues.spec.ts`

**Steps:**

1. Add frontend types.
2. Add user API module.
3. Add admin API module.
4. Add unit tests with mocked `apiClient`.
5. Run:

```bash
cd frontend
pnpm run test:run -- issues
pnpm run typecheck
```

Commit:

```bash
git add frontend/src/types frontend/src/api
git commit -m "feat: add support issue frontend API"
```

### Task 9: Public/User Views

**Files:**

- Create: `frontend/src/views/user/IssuesView.vue`
- Create: `frontend/src/views/user/IssueDetailView.vue`
- Create: `frontend/src/views/user/NewIssueView.vue`
- Modify: `frontend/src/router/index.ts`
- Modify: `frontend/src/components/layout/AppSidebar.vue`
- Test: `frontend/src/views/user/__tests__/IssuesView.spec.ts`
- Test: `frontend/src/views/user/__tests__/IssueDetailView.spec.ts`
- Test: `frontend/src/views/user/__tests__/NewIssueView.spec.ts`

**Steps:**

1. Build list/search/filter UI.
2. Build create form with duplicate suggestion flow.
3. Build detail/comment/resolve flow.
4. Add routes.
5. Add sidebar entry.
6. Test locked issue hides comment form.
7. Test unauthenticated users see login prompt for comment/create.
8. Run:

```bash
cd frontend
pnpm run test:run -- IssuesView IssueDetailView NewIssueView
pnpm run typecheck
pnpm run lint:check
```

Commit:

```bash
git add frontend/src/views/user frontend/src/router/index.ts frontend/src/components/layout/AppSidebar.vue
git commit -m "feat: add public support issue views"
```

### Task 10: Admin Views

**Files:**

- Create: `frontend/src/views/admin/AdminIssuesView.vue`
- Create: `frontend/src/views/admin/AdminIssueDetailView.vue`
- Modify: `frontend/src/router/index.ts`
- Modify: `frontend/src/components/layout/AppSidebar.vue`
- Test: `frontend/src/views/admin/__tests__/AdminIssuesView.spec.ts`
- Test: `frontend/src/views/admin/__tests__/AdminIssueDetailView.spec.ts`

**Steps:**

1. Build admin list/search page.
2. Build admin detail/moderation page.
3. Add event timeline.
4. Add hide/reopen/status actions.
5. Test full email visible only in admin view.
6. Test status action calls correct API.
7. Run:

```bash
cd frontend
pnpm run test:run -- AdminIssuesView AdminIssueDetailView
pnpm run typecheck
pnpm run lint:check
```

Commit:

```bash
git add frontend/src/views/admin frontend/src/router/index.ts frontend/src/components/layout/AppSidebar.vue
git commit -m "feat: add admin support issue views"
```

### Task 11: End-To-End Verification

**Files:**

- Add integration test if existing e2e pattern fits: `backend/internal/integration/e2e_support_issue_test.go`.
- Optional frontend integration test: `frontend/src/__tests__/integration/support-issues.spec.ts`.

**Steps:**

1. Register/login test user.
2. Create issue.
3. List public issues and verify masked email.
4. Add comment.
5. Resolve as reporter.
6. Verify comment after resolve returns error.
7. Admin reopen.
8. Admin hide comment.
9. Public detail excludes hidden comment.
10. Run:

```bash
cd backend
go test -tags=integration ./internal/integration -run SupportIssue
```

Run full checks:

```bash
cd backend
go test -tags=unit ./...
go test -tags=integration ./...
golangci-lint run ./...
cd ../frontend
pnpm run lint:check
pnpm run typecheck
pnpm run test:run
pnpm run build
```

Commit:

```bash
git add backend frontend
git commit -m "test: verify support issue center flows"
```

## 10. Error Handling

Add domain/service errors with stable codes:

- `SUPPORT_ISSUE_NOT_FOUND`
- `SUPPORT_ISSUE_LOCKED`
- `SUPPORT_ISSUE_INVALID_STATUS`
- `SUPPORT_ISSUE_INVALID_TRANSITION`
- `SUPPORT_ISSUE_PERMISSION_DENIED`
- `SUPPORT_ISSUE_TITLE_INVALID`
- `SUPPORT_ISSUE_DESCRIPTION_INVALID`
- `SUPPORT_ISSUE_EMAIL_INVALID`
- `SUPPORT_ISSUE_SCREENSHOT_TEXT_REQUIRED`
- `SUPPORT_ISSUE_SEARCH_INVALID`
- `SUPPORT_ISSUE_ATTACHMENT_INVALID_TYPE`
- `SUPPORT_ISSUE_ATTACHMENT_TOO_LARGE`

Use existing project error helpers in `backend/internal/pkg/errors` and response helpers.

## 11. Security And Privacy Checklist

- Public API never returns `account_email`.
- Public API never returns raw `api_key_suffix` unless explicitly masked; default is omit.
- Admin API returns private fields only behind admin middleware.
- Attachment file names are sanitized.
- Attachment paths cannot escape storage directory.
- Upload endpoint enforces MIME and size.
- Markdown/user text is rendered safely on frontend; no `v-html` for user content unless sanitized.
- Comments reject empty/too long content.
- Resolved/closed/locked issues reject comments server-side.
- Hidden content is excluded server-side, not only hidden in UI.
- All admin moderation actions create events.
- Add rate limiting if existing middleware has a convenient per-route limiter; otherwise leave a TODO and keep auth-required create/comment in MVP.

## 12. Rollout Plan

Phase 1 MVP:

- Tables and APIs.
- Public issue list/detail.
- User create/comment/resolve.
- Admin status/moderation.
- Exact structured search.
- Screenshot upload.
- Email masking.

Phase 2:

- OCR extraction into `ocr_text`.
- Duplicate issue merge.
- FAQ marker and FAQ filter.
- Admin shortcuts to related usage/payment/log pages.
- Sensitive token detection before upload/comment.

Phase 3:

- Auto category suggestion.
- Auto title rewrite.
- Auto extraction of HTTP status/error code/model/client from screenshot text.
- Similar resolved issue recommendations while typing.

## 13. Acceptance Criteria

MVP is done when:

- A logged-in user cannot submit an issue without title, description, account email, occurrence time, screenshot text, screenshot language, category, and severity.
- Public list/detail show masked email only.
- Admin list/detail show full email.
- User can comment on open/needs_info/in_progress issues.
- Commenting on resolved/closed/locked issue returns a clear API error.
- Reporter can resolve own issue.
- Non-reporter user cannot resolve someone else's issue.
- Admin can resolve/reopen/close/hide comments/hide attachments.
- Every mutation writes a support issue event.
- Search supports `id`, `email`, `status`, `category`, `code`, `model`, `time`, quoted phrases, and plain AND terms.
- Public search ignores or rejects `key:`; admin search supports it.
- Backend unit tests pass.
- Backend integration tests for issue flow pass.
- Frontend typecheck, lint, tests, and build pass.

## 14. Open Decisions

Resolve before implementation or choose defaults below:

1. Public read access:
   - Recommended default: public read/search without login; create/comment require login.
2. Attachment storage:
   - Recommended default: local storage under `data/support-issue-attachments`; make path configurable later.
3. Maximum attachments per issue:
   - Recommended default: 5.
4. Maximum comments per issue:
   - Recommended default: no hard count, but each comment max 8000 chars.
5. Whether public detail should show reporter display name:
   - Recommended default: no; show masked email only.

## 15. AI Implementation Notes

- Do not start by building frontend screens. Start with domain/search tests, then persistence, then service, then handlers, then UI.
- Keep commits small and runnable.
- After adding Ent schema, always run `go generate ./ent` and include generated code.
- After changing Wire provider structs, always run `go generate ./cmd/server` and include `wire_gen.go`.
- If an interface changes and tests fail, update all stubs/mocks instead of weakening the interface.
- Prefer existing pagination helpers from `backend/internal/pkg/pagination`.
- Reuse existing response helpers and auth subject extraction patterns.
- Keep MVP exact and boring. Fancy AI duplicate detection can wait.
