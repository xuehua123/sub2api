package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCommissionRewardsSettlementReadyIndexMigration(t *testing.T) {
	const name = "185_add_commission_rewards_settlement_ready_index_notx.sql"
	content, err := FS.ReadFile(name)
	require.NoError(t, err, "production migrations must include settlement-ready composite index")

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_commission_rewards_status_available_at_id")
	require.Contains(t, strings.ToLower(sql), "on commission_rewards (status, available_at, id)")
	require.True(t, strings.HasSuffix(name, "_notx.sql"), "CONCURRENTLY indexes must use non-transactional migration")
}

func TestCommissionRewardsSettlementIndexNameMatchesEntStorageKey(t *testing.T) {
	// Keep Ent StorageKey and SQL migration index name in lockstep so schema tools
	// cannot invent a second index (commissionreward_status_available_at_id).
	const want = "idx_commission_rewards_status_available_at_id"
	content, err := FS.ReadFile("185_add_commission_rewards_settlement_ready_index_notx.sql")
	require.NoError(t, err)
	require.Contains(t, string(content), want)
}
