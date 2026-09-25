//go:build integration

package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type recordedBusinessSQL struct {
	sqlExecutor
	query string
	args  []any
	calls int
}

func (r *recordedBusinessSQL) QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	r.query = q
	r.args = args
	r.calls++
	return r.sqlExecutor.QueryContext(ctx, q, args...)
}
func TestUserBusinessBoundedAggregatePlan(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	ids := []int64{}
	for i := 0; i < 50; i++ {
		u := mustCreateUser(t, client, &service.User{Email: fmt.Sprintf("business-scale-%d@test.invalid", i)})
		ids = append(ids, u.ID)
	}
	key := mustCreateApiKey(t, client, &service.APIKey{UserID: ids[0], Key: "sk-business-scale", Name: "scale"})
	account := mustCreateAccount(t, client, &service.Account{Name: "scale"})
	start := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	_, err := tx.ExecContext(ctx, `INSERT INTO usage_logs(user_id,api_key_id,account_id,request_id,model,total_cost,actual_cost,created_at)
 SELECT ($1::bigint[])[1+(n%50)],$2,$3,'business-scale-'||n,'test',1,2,$4::timestamptz+n*interval '1 second' FROM generate_series(1,20000) n`, pq.Array(ids), key.ID, account.ID, start)
	require.NoError(t, err)
	recorder := &recordedBusinessSQL{sqlExecutor: tx}
	repo := &userBusinessRepository{db: recorder}
	q := service.UserBusinessQuery{UserBusinessParams: service.UserBusinessParams{Search: "business-scale-", Page: 1, PageSize: 20, Sort: "consumption", Order: "desc"}, Start: start, End: start.AddDate(0, 0, 1), Now: start.AddDate(0, 0, 2)}
	raw, err := repo.Report(ctx, q)
	require.NoError(t, err)
	require.Equal(t, 1, recorder.calls)
	var result struct {
		Total   int
		Items   []struct{ Consumption float64 }
		Summary struct{ Consumption float64 }
	}
	require.NoError(t, json.Unmarshal(raw, &result))
	require.Equal(t, 50, result.Total)
	require.Len(t, result.Items, 20)
	require.Equal(t, 40000.0, result.Summary.Consumption)
	rows, err := tx.QueryContext(ctx, "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) "+recorder.query, recorder.args...)
	require.NoError(t, err)
	require.True(t, rows.Next())
	var planRaw []byte
	require.NoError(t, rows.Scan(&planRaw))
	require.NoError(t, rows.Close())
	var plans []map[string]any
	require.NoError(t, json.Unmarshal(planRaw, &plans))
	t.Logf("50 users / 20,000 usage rows: one report query, EXPLAIN execution %.3f ms", plans[0]["Execution Time"])
}
