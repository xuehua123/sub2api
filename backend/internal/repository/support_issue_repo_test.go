package repository

import "testing"

func TestSupportIssueCommentAuthorDisplayName(t *testing.T) {
	t.Run("keeps emoji username", func(t *testing.T) {
		got := supportIssueCommentAuthorDisplayName("🚀皮皮虾", "user@example.com")
		if got != "🚀皮皮虾" {
			t.Fatalf("display name = %q, want emoji username", got)
		}
	})

	t.Run("masks fallback email middle with three asterisks", func(t *testing.T) {
		got := supportIssueCommentAuthorDisplayName("", "LoveJsXuehua@Example.COM")
		if got != "lovej***ehua@example.com" {
			t.Fatalf("display name = %q, want lovej***ehua@example.com", got)
		}
	})
}
