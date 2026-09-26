package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

const upstreamGroupKeySQL = "CASE WHEN g.remote_id <> '' THEN 'id:' || g.remote_id ELSE 'name:' || g.name END"
const upstreamGroupCatalogSQL = `WITH catalog AS NOT MATERIALIZED (
 SELECT c.id AS connection_id,c.name AS connection_name,c.management_base_url,c.provider,
 CASE WHEN g.remote_id <> '' THEN 'id:' || g.remote_id ELSE 'name:' || g.name END AS remote_key,
 g.remote_id,g.name,g.rate_multiplier,g.source,g.confidence,g.observed_at,g.fresh_until,
 ARRAY(SELECT DISTINCT tag FROM unnest(COALESCE(a.tags,'{}'::text[]) || ARRAY(SELECT v FROM unnest(mr.auto_tags) v WHERE NOT(v=ANY(COALESCE(a.excluded_auto_tags,'{}'::text[]))))) tag ORDER BY tag) AS tags,
 COALESCE(a.favorite,false) AS favorite,
 mr.auto_tags,COALESCE(a.excluded_auto_tags,'{}'::text[]) AS excluded_auto_tags,
 mr.models,mr.coverage AS model_coverage,mr.observed_at AS models_observed_at,mr.status AS model_status,
 ARRAY(SELECT DISTINCT b.account_id FROM upstream_account_bindings b JOIN accounts ac ON ac.id=b.account_id AND ac.deleted_at IS NULL
 WHERE b.connection_id=c.id AND ((b.remote_group_id<>'' AND b.remote_group_id=g.remote_id) OR (b.remote_group_id='' AND b.remote_group_name=g.name))) AS account_ids,
 CASE WHEN c.status IN ('auth_error','degraded','needs_input') THEN 'error'
 WHEN g.fresh_until IS NULL THEN 'unknown' WHEN g.fresh_until <= $1 THEN 'stale' ELSE 'fresh' END AS freshness
 FROM upstream_groups g JOIN upstream_connections c ON c.id=g.connection_id
 LEFT JOIN upstream_group_annotations a ON a.connection_id=c.id AND a.remote_key=CASE WHEN g.remote_id<>'' THEN 'id:' || g.remote_id ELSE 'name:' || g.name END
 ` + upstreamGroupModelReadJoinSQL + `
), filtered AS (
 SELECT * FROM catalog WHERE
 ($2='' OR strpos(lower(name || ' ' || remote_id || ' ' || connection_name || ' ' || management_base_url || ' ' || array_to_string(tags,' ') || ' ' || array_to_string(models,' ')),lower($2))>0)
 AND (cardinality($3::bigint[])=0 OR connection_id=ANY($3::bigint[]))
 AND ($4='' OR provider=$4) AND ($5='' OR $5=ANY(tags))
 AND ($6='' OR ($6='bound' AND cardinality(account_ids)>0) OR ($6='unbound' AND cardinality(account_ids)=0))
 AND ($7='' OR freshness=$7) AND (NOT $8 OR favorite)
 AND ($9::double precision IS NULL OR rate_multiplier >= $9) AND ($10::double precision IS NULL OR rate_multiplier <= $10)
) `

func (r *upstreamConnectionRepository) ListGroupCatalog(ctx context.Context, p service.UpstreamGroupCatalogParams, now time.Time) (*service.UpstreamGroupCatalogResult, error) {
	client := clientFromContext(ctx, r.client)
	result := &service.UpstreamGroupCatalogResult{Items: []service.UpstreamGroupCatalogItem{}, Tags: []string{}, Page: p.Page, PageSize: p.PageSize}
	ids := p.ConnectionIDs
	if ids == nil {
		ids = []int64{}
	}
	args := []any{now, p.Search, pq.Array(ids), p.Provider, p.Tag, p.Binding, p.Freshness, p.Favorites, p.MinRate, p.MaxRate}
	order := map[string]string{"name_asc": "name ASC", "name_desc": "name DESC", "rate_asc": "rate_multiplier ASC NULLS LAST", "rate_desc": "rate_multiplier DESC NULLS LAST", "bindings_desc": "cardinality(account_ids) DESC", "observed_desc": "observed_at DESC NULLS LAST", "connection_asc": "connection_name ASC"}[p.Sort]
	if order == "" {
		order = "favorite DESC, name ASC"
	}
	if p.GroupByConnection {
		order = "connection_name ASC,connection_id ASC," + order
	}
	// Count and page share one materialized filtered projection and one database
	// snapshot. JSON is limited to the requested page, never the full model lists.
	query := upstreamGroupCatalogSQL + `SELECT (SELECT count(*) FROM filtered),
 COALESCE((SELECT jsonb_agg(to_jsonb(page)) FROM (
 SELECT connection_id,connection_name,management_base_url,provider,remote_key,remote_id,name,rate_multiplier,source,confidence,observed_at,fresh_until,
 tags,favorite,account_ids,cardinality(account_ids) AS binding_count,freshness,auto_tags,excluded_auto_tags,
 cardinality(models) AS model_count,models[1:3] AS model_preview,model_status,model_coverage,models_observed_at
 FROM filtered ORDER BY ` + order + `,connection_id ASC,remote_key ASC LIMIT $11 OFFSET $12) page),'[]'::jsonb),
 ARRAY(SELECT DISTINCT unnest(tags) AS tag FROM catalog ORDER BY tag)`
	rows, err := client.QueryContext(ctx, query, append(args, p.PageSize, (p.Page-1)*p.PageSize)...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if rows.Next() {
		var page []byte
		if err = rows.Scan(&result.Total, &page, pq.Array(&result.Tags)); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(page, &result.Items); err != nil {
			return nil, err
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, rows.Close()
}
func (r *upstreamConnectionRepository) UpdateGroupAnnotations(ctx context.Context, p service.UpstreamGroupAnnotationUpdate) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// Lock connections in a stable order, matching the synchronizer's lock order.
	ids := make([]int64, 0, len(p.Groups))
	for _, g := range p.Groups {
		ids = append(ids, g.ConnectionID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for i, id := range ids {
		if i > 0 && ids[i-1] == id {
			continue
		}
		rows, e := tx.Client().QueryContext(ctx, "SELECT id FROM upstream_connections WHERE id=$1 FOR UPDATE", id)
		if e != nil {
			return e
		}
		exists := rows.Next()
		e = rows.Err()
		if closeErr := rows.Close(); e == nil {
			e = closeErr
		}
		if e != nil {
			return e
		}
		if !exists {
			return service.ErrUpstreamConnectionNotFound
		}
	}
	for _, g := range p.Groups {
		rows, e := tx.Client().QueryContext(ctx, "SELECT id FROM upstream_groups g WHERE connection_id=$1 AND "+upstreamGroupKeySQL+"=$2", g.ConnectionID, g.RemoteKey)
		if e != nil {
			return e
		}
		exists := rows.Next()
		e = rows.Err()
		if closeErr := rows.Close(); e == nil {
			e = closeErr
		}
		if e != nil {
			return e
		}
		if !exists {
			return service.ErrUpstreamConnectionChanged
		}
		add := p.AddTags
		if add == nil {
			add = []string{}
		}
		remove := p.RemoveTags
		if remove == nil {
			remove = []string{}
		}
		_, e = tx.Client().ExecContext(ctx, `INSERT INTO upstream_group_annotations(connection_id,remote_key,tags,favorite,excluded_auto_tags)
 VALUES($1,$2,ARRAY(SELECT DISTINCT v FROM unnest($3::text[]) v WHERE NOT(v=ANY($4::text[])) ORDER BY v),COALESCE($5,false),CASE WHEN $6 THEN '{}'::text[] ELSE $4::text[] END)
 ON CONFLICT(connection_id,remote_key) DO UPDATE SET
 tags=ARRAY(SELECT DISTINCT v FROM unnest(upstream_group_annotations.tags || $3::text[]) v WHERE NOT(v=ANY($4::text[])) ORDER BY v),
 excluded_auto_tags=CASE WHEN $6 THEN '{}'::text[] ELSE ARRAY(SELECT DISTINCT v FROM unnest(upstream_group_annotations.excluded_auto_tags || $4::text[]) v WHERE NOT(v=ANY($3::text[])) OR v=ANY($4::text[]) ORDER BY v) END,
 favorite=COALESCE($5,upstream_group_annotations.favorite),updated_at=NOW()`, g.ConnectionID, g.RemoteKey, pq.Array(add), pq.Array(remove), p.Favorite, p.ResetAutoTags)
		if e != nil {
			return e
		}
	}
	return tx.Commit()
}
func (r *upstreamConnectionRepository) GetConnectionDailyCosts(ctx context.Context, ids []int64, start, end time.Time, timezoneName string) ([]service.UpstreamConnectionDailyCost, error) {
	if ids == nil {
		ids = []int64{}
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	// One bounded aggregation supplies daily points and all connection totals. Keep
	// the timestamp predicate unwrapped so (account_id, created_at) remains usable.
	rows, err := clientFromContext(ctx, r.client).QueryContext(ctx, upstreamDailyCostsSQL, pq.Array(ids), start, end, timezoneName)
	if err != nil {
		return nil, fmt.Errorf("query upstream historical costs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	items := []service.UpstreamConnectionDailyCost{}
	for rows.Next() {
		var item service.UpstreamConnectionDailyCost
		if err := rows.Scan(&item.ConnectionID, &item.Name, &item.Day, &item.Requests, &item.AccountCost); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, rows.Close()
}

const upstreamDailyCostsSQL = `WITH selected_connections AS MATERIALIZED (
 SELECT id,name FROM upstream_connections WHERE cardinality($1::bigint[])=0 OR id=ANY($1::bigint[])
), aggregates AS MATERIALIZED (
 SELECT b.connection_id,(u.created_at AT TIME ZONE $4)::date AS day,
 COUNT(*) AS requests,SUM(COALESCE(u.account_stats_cost,u.total_cost)*COALESCE(u.account_rate_multiplier,1)) AS cost
 FROM selected_connections c
 JOIN upstream_account_bindings b ON b.connection_id=c.id
 JOIN accounts a ON a.id=b.account_id AND a.deleted_at IS NULL
 JOIN usage_logs u ON u.account_id=a.id AND u.created_at >= $2 AND u.created_at < $3
 GROUP BY GROUPING SETS ((b.connection_id), ((u.created_at AT TIME ZONE $4)::date))
)
SELECT c.id,c.name,'' AS day,COALESCE(d.requests,0),COALESCE(d.cost,0)
FROM selected_connections c LEFT JOIN aggregates d ON d.connection_id=c.id
UNION ALL
SELECT 0,'',to_char(day,'YYYY-MM-DD'),requests,COALESCE(cost,0)
FROM aggregates WHERE connection_id IS NULL
ORDER BY 1,3`
