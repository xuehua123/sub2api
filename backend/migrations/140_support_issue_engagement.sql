ALTER TABLE support_issues
    ADD COLUMN IF NOT EXISTS hidden_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS hidden_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS hide_reason VARCHAR(255) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS last_viewed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS view_count INT NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS support_issue_views (
    id              BIGSERIAL PRIMARY KEY,
    issue_id        BIGINT NOT NULL REFERENCES support_issues(id) ON DELETE CASCADE,
    viewer_user_id  BIGINT REFERENCES users(id) ON DELETE SET NULL,
    viewer_hash     VARCHAR(64) NOT NULL,
    viewed_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_support_issues_hidden_at ON support_issues(hidden_at);
CREATE INDEX IF NOT EXISTS idx_support_issues_view_count ON support_issues(view_count DESC);
CREATE INDEX IF NOT EXISTS idx_support_issues_last_viewed_at ON support_issues(last_viewed_at DESC);
CREATE INDEX IF NOT EXISTS idx_support_issue_views_issue_viewed ON support_issue_views(issue_id, viewed_at DESC);
CREATE INDEX IF NOT EXISTS idx_support_issue_views_viewer_issue_viewed ON support_issue_views(viewer_hash, issue_id, viewed_at DESC);
CREATE INDEX IF NOT EXISTS idx_support_issue_views_user_issue_viewed ON support_issue_views(viewer_user_id, issue_id, viewed_at DESC);
