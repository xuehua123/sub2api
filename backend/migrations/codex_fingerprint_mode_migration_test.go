package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration224EnforcesExplicitOffForLiveOpenAIOAuthAccounts(t *testing.T) {
	content, err := FS.ReadFile("224_codex_fingerprint_mode_explicit_off.sql")
	require.NoError(t, err)

	sql := string(content)
	for _, want := range []string{
		"SET LOCAL lock_timeout = '5s';",
		"CREATE OR REPLACE FUNCTION public.enforce_openai_oauth_codex_fingerprint_mode_explicit_off()",
		"BEFORE INSERT OR UPDATE OF platform, type, extra, deleted_at",
		"accounts_enforce_codex_fingerprint_mode_explicit_off",
		"NEW.platform IS DISTINCT FROM 'openai'",
		"NEW.type IS DISTINCT FROM 'oauth'",
		"NEW.deleted_at IS NOT NULL",
		"jsonb_typeof(NEW.extra) IS DISTINCT FROM 'object'",
		"must be a JSON object before enforcing codex_fingerprint_mode",
		"fingerprint_mode #>> '{}' IN ('off', 'device', 'session', 'full')",
		"'{codex_fingerprint_mode}'",
		"'\"off\"'::jsonb",
		"WITH updated_accounts AS",
		"RETURNING id",
		"INSERT INTO scheduler_outbox (event_type, account_id)",
		"SELECT 'account_changed', id",
	} {
		require.Contains(t, sql, want)
	}
}
