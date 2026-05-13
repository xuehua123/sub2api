CREATE TABLE IF NOT EXISTS support_issues (
    id                       BIGSERIAL PRIMARY KEY,
    public_id                VARCHAR(32) NOT NULL,
    title                    VARCHAR(160) NOT NULL,
    description              TEXT NOT NULL,
    account_email            VARCHAR(255) NOT NULL,
    account_email_normalized VARCHAR(255) NOT NULL,
    account_email_masked     VARCHAR(255) NOT NULL,
    occurred_at              TIMESTAMPTZ NOT NULL,
    screenshot_text          TEXT NOT NULL,
    screenshot_language      VARCHAR(16) NOT NULL DEFAULT 'unknown',
    category                 VARCHAR(32) NOT NULL DEFAULT 'other',
    severity                 VARCHAR(32) NOT NULL DEFAULT 'question',
    status                   VARCHAR(32) NOT NULL DEFAULT 'open',
    model_name               VARCHAR(255) NOT NULL DEFAULT '',
    client_name              VARCHAR(120) NOT NULL DEFAULT '',
    http_status              INT,
    error_code               VARCHAR(120) NOT NULL DEFAULT '',
    api_key_suffix           VARCHAR(16) NOT NULL DEFAULT '',
    created_by_user_id       BIGINT REFERENCES users(id) ON DELETE SET NULL,
    resolved_by_user_id      BIGINT REFERENCES users(id) ON DELETE SET NULL,
    resolved_at              TIMESTAMPTZ,
    locked_at                TIMESTAMPTZ,
    last_comment_at          TIMESTAMPTZ,
    comment_count            INT NOT NULL DEFAULT 0,
    hidden_comment_count     INT NOT NULL DEFAULT 0,
    attachment_count         INT NOT NULL DEFAULT 0,
    search_text              TEXT NOT NULL DEFAULT '',
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS support_issue_comments (
    id                BIGSERIAL PRIMARY KEY,
    issue_id          BIGINT NOT NULL REFERENCES support_issues(id) ON DELETE CASCADE,
    author_user_id    BIGINT REFERENCES users(id) ON DELETE SET NULL,
    author_role       VARCHAR(16) NOT NULL,
    content           TEXT NOT NULL,
    hidden_at         TIMESTAMPTZ,
    hidden_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    hide_reason       VARCHAR(255) NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS support_issue_attachments (
    id                 BIGSERIAL PRIMARY KEY,
    issue_id           BIGINT REFERENCES support_issues(id) ON DELETE CASCADE,
    uploaded_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    file_path          VARCHAR(512) NOT NULL,
    file_url           VARCHAR(512) NOT NULL,
    file_name          VARCHAR(255) NOT NULL,
    mime_type          VARCHAR(100) NOT NULL,
    size_bytes         BIGINT NOT NULL,
    ocr_text           TEXT NOT NULL DEFAULT '',
    visibility         VARCHAR(16) NOT NULL DEFAULT 'public',
    hidden_at          TIMESTAMPTZ,
    hidden_by_user_id  BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS support_issue_events (
    id            BIGSERIAL PRIMARY KEY,
    issue_id      BIGINT NOT NULL REFERENCES support_issues(id) ON DELETE CASCADE,
    actor_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    event_type    VARCHAR(32) NOT NULL,
    from_status   VARCHAR(32),
    to_status     VARCHAR(32),
    metadata      JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_support_issues_public_id ON support_issues(public_id);
CREATE INDEX IF NOT EXISTS idx_support_issues_status_updated ON support_issues(status, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_support_issues_status_last_comment ON support_issues(status, last_comment_at DESC);
CREATE INDEX IF NOT EXISTS idx_support_issues_category_status ON support_issues(category, status);
CREATE INDEX IF NOT EXISTS idx_support_issues_created_by ON support_issues(created_by_user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_support_issues_email_norm ON support_issues(account_email_normalized);
CREATE INDEX IF NOT EXISTS idx_support_issues_occurred_at ON support_issues(occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_support_issues_http_status ON support_issues(http_status);
CREATE INDEX IF NOT EXISTS idx_support_issues_error_code ON support_issues(error_code);
CREATE INDEX IF NOT EXISTS idx_support_issue_comments_issue_created ON support_issue_comments(issue_id, created_at ASC);
CREATE INDEX IF NOT EXISTS idx_support_issue_comments_hidden ON support_issue_comments(issue_id, hidden_at);
CREATE INDEX IF NOT EXISTS idx_support_issue_attachments_issue ON support_issue_attachments(issue_id, visibility);
CREATE INDEX IF NOT EXISTS idx_support_issue_attachments_unbound_user ON support_issue_attachments(uploaded_by_user_id, issue_id);
CREATE INDEX IF NOT EXISTS idx_support_issue_events_issue_created ON support_issue_events(issue_id, created_at DESC);

DO $$
BEGIN
    BEGIN
        CREATE EXTENSION IF NOT EXISTS pg_trgm;
    EXCEPTION
        WHEN OTHERS THEN
            RAISE NOTICE 'pg_trgm extension not created: %', SQLERRM;
    END;

    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_trgm') THEN
        EXECUTE 'CREATE INDEX IF NOT EXISTS idx_support_issues_title_trgm
                 ON support_issues USING gin (title gin_trgm_ops)';
        EXECUTE 'CREATE INDEX IF NOT EXISTS idx_support_issues_screenshot_text_trgm
                 ON support_issues USING gin (screenshot_text gin_trgm_ops)';
        EXECUTE 'CREATE INDEX IF NOT EXISTS idx_support_issues_search_text_trgm
                 ON support_issues USING gin (search_text gin_trgm_ops)';
        EXECUTE 'CREATE INDEX IF NOT EXISTS idx_support_issue_comments_content_trgm
                 ON support_issue_comments USING gin (content gin_trgm_ops)';
    ELSE
        RAISE NOTICE 'skip support issue trigram indexes because pg_trgm is unavailable';
    END IF;
END
$$;
