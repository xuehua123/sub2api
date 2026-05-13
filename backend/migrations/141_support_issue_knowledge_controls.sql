ALTER TABLE support_issues
    ADD COLUMN IF NOT EXISTS pinned_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS pinned_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS pinned_reason VARCHAR(255) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS solution_comment_id BIGINT REFERENCES support_issue_comments(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS related_issue_id BIGINT REFERENCES support_issues(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS related_issue_reason VARCHAR(255) NOT NULL DEFAULT '';

ALTER TABLE support_issue_comments
    ADD COLUMN IF NOT EXISTS related_issue_id BIGINT REFERENCES support_issues(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_support_issues_pinned_at ON support_issues(pinned_at DESC);
CREATE INDEX IF NOT EXISTS idx_support_issues_solution_comment_id ON support_issues(solution_comment_id);
CREATE INDEX IF NOT EXISTS idx_support_issues_related_issue_id ON support_issues(related_issue_id);
CREATE INDEX IF NOT EXISTS idx_support_issue_comments_related_issue_id ON support_issue_comments(related_issue_id);
