package migrations

import (
	"os"
	"strings"
	"testing"
)

func TestMigration158IsAdditiveAndCreatesBackfillControlTables(t *testing.T) {
	sqlBytes, err := os.ReadFile("158_legacy_entitlement_backfill_control_tables.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(sqlBytes)

	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS subscription_legacy_backfill_mappings",
		"CREATE TABLE IF NOT EXISTS api_key_legacy_backfill_snapshots",
		"legacy_group_id    BIGINT PRIMARY KEY",
		"api_key_id                       BIGINT PRIMARY KEY",
		"CREATE INDEX IF NOT EXISTS idx_subscription_legacy_backfill_mappings_runtime_key",
		"CREATE INDEX IF NOT EXISTS idx_api_key_legacy_backfill_snapshots_mapping_version",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("migration 158 missing %q", want)
		}
	}

	upper := strings.ToUpper(sql)
	for _, forbidden := range []string{
		"DROP TABLE",
		"DROP COLUMN",
		"TRUNCATE",
		"DELETE FROM",
		"UPDATE ",
		"INSERT INTO",
	} {
		if strings.Contains(upper, forbidden) {
			t.Fatalf("migration 158 must remain structural/additive only; found %s", forbidden)
		}
	}
}
