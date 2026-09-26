package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

// Both detail and catalog reads use the same live source validation. Failed
// sources retain their own last result, but changed/unbound sources contribute
// no models or tags. No account usage timestamp participates in this identity.
const upstreamGroupModelReadJoinSQL = `
 LEFT JOIN upstream_group_model_snapshots m ON m.connection_id=c.id AND m.remote_key=` + upstreamGroupKeySQL + ` AND m.connection_version=c.version
 LEFT JOIN LATERAL (
  WITH sources AS MATERIALIZED (
   SELECT x.eligible,x.identity_valid,x.binding_status,x.binding_fresh_until,s.models,s.auto_tags,s.status,s.observed_at,s.fresh_until,
    (m.refresh_active AND (s.refresh_generation IS NULL OR s.refresh_generation<>m.refresh_generation)) AS batch_pending
   FROM upstream_group_model_current_sources x
   LEFT JOIN upstream_group_model_account_snapshots s ON s.connection_id=x.connection_id AND s.remote_key=x.remote_key
    AND s.account_id=x.account_id AND s.connection_version=x.connection_version AND s.source_fingerprint=x.source_fingerprint
   WHERE (m.coverage='bound_keys' OR m.refresh_active) AND x.connection_id=c.id AND x.remote_key=` + upstreamGroupKeySQL + ` AND x.connection_version=c.version
  )
  SELECT count(*)::int AS source_count,
   count(*) FILTER(WHERE eligible AND NOT batch_pending AND status='ready' AND fresh_until>NOW())::int AS ready_count,
   count(*) FILTER(WHERE eligible AND (batch_pending OR status IS NULL))::int AS pending_count,
   count(*) FILTER(WHERE NOT COALESCE(identity_valid,false) OR binding_status='error' OR (NOT batch_pending AND status='error'))::int AS failed_count,
   count(*) FILTER(WHERE identity_valid AND binding_status<>'error' AND COALESCE(status,'')<>'error'
    AND (NOT COALESCE(eligible,false) OR (NOT batch_pending AND status='ready' AND (fresh_until IS NULL OR fresh_until<=NOW()))))::int AS stale_count,
   min(observed_at) FILTER(WHERE identity_valid) AS observed_at,min(LEAST(fresh_until,binding_fresh_until)) FILTER(WHERE identity_valid) AS fresh_until,
   ARRAY(SELECT DISTINCT v FROM sources s CROSS JOIN LATERAL unnest(s.models) v WHERE s.identity_valid AND s.observed_at IS NOT NULL ORDER BY v) AS models,
   ARRAY(SELECT DISTINCT v FROM sources s CROSS JOIN LATERAL unnest(s.auto_tags) v WHERE s.identity_valid AND s.observed_at IS NOT NULL ORDER BY v) AS auto_tags
  FROM sources
 ) bs ON true
 LEFT JOIN LATERAL (
  SELECT
   CASE WHEN m.coverage='bound_keys' THEN bs.models ELSE COALESCE(m.models,'{}'::text[]) END AS models,
   CASE WHEN m.coverage='bound_keys' THEN bs.auto_tags ELSE COALESCE(m.auto_tags,'{}'::text[]) END AS auto_tags,
   COALESCE(m.source,'') AS source,COALESCE(m.coverage,'unknown') AS coverage,
   CASE WHEN m.coverage='bound_keys' THEN bs.observed_at ELSE m.observed_at END AS observed_at,
   CASE WHEN m.coverage='bound_keys' THEN bs.fresh_until ELSE m.fresh_until END AS fresh_until,
   CASE WHEN m.lease_until>NOW() THEN 'syncing'
    WHEN m.refresh_active THEN 'pending'
    WHEN m.coverage='bound_keys' THEN CASE
     WHEN bs.source_count=0 THEN 'unknown'
     WHEN bs.failed_count>0 THEN CASE WHEN bs.observed_at IS NULL THEN 'error' ELSE 'partial' END
     WHEN bs.pending_count>0 THEN 'pending'
     WHEN bs.ready_count<bs.source_count THEN 'stale' ELSE 'ready' END
    WHEN m.status='ready' AND m.fresh_until<=NOW() THEN 'stale' ELSE COALESCE(m.status,'unknown') END AS status,
   CASE WHEN m.refresh_active AND bs.pending_count>0 THEN 'pending_keys'
    WHEN m.refresh_active THEN ''
    WHEN m.coverage='bound_keys' THEN CASE
     WHEN bs.source_count=0 THEN 'no_bound_key'
     WHEN bs.failed_count>0 THEN 'partial_keys'
     WHEN bs.pending_count>0 THEN 'pending_keys' ELSE '' END
    ELSE COALESCE(m.error_code,'') END AS error_code,
   bs.source_count,bs.ready_count,bs.pending_count,bs.failed_count,bs.stale_count,
   COALESCE(m.lease_until>NOW(),false) OR COALESCE(m.refresh_active,false) OR (bs.pending_count>0 AND
    (COALESCE(m.refresh_active,false) OR (c.sync_enabled AND c.status NOT IN ('auth_error','needs_input','disabled')))) AS refresh_in_progress
 ) mr ON true
`

// Configuration changes/new bindings are due immediately, even when the group
// job still has a six-hour success TTL. Existing failed keys keep their backoff.
const upstreamGroupModelSourceDueSQL = `EXISTS (
 SELECT 1 FROM upstream_group_model_current_sources x
 LEFT JOIN upstream_group_model_account_snapshots s ON s.connection_id=x.connection_id AND s.remote_key=x.remote_key AND s.account_id=x.account_id
  AND s.connection_version=x.connection_version AND s.source_fingerprint=x.source_fingerprint
 WHERE x.connection_id=m.connection_id AND x.remote_key=m.remote_key AND x.eligible
 AND (s.account_id IS NULL OR (m.refresh_active AND s.refresh_generation<>m.refresh_generation)
  OR (NOT m.refresh_active AND s.next_sync_at<=$1))
)`

func (r *upstreamConnectionRepository) GetGroupModels(ctx context.Context, ref service.UpstreamGroupReference) (*service.UpstreamGroupModels, error) {
	rows, err := clientFromContext(ctx, r.client).QueryContext(ctx, `SELECT mr.models,mr.auto_tags,mr.source,mr.coverage,mr.status,mr.error_code,mr.observed_at,mr.fresh_until,
 mr.source_count,mr.ready_count,mr.pending_count,mr.failed_count,mr.stale_count,mr.refresh_in_progress
 FROM upstream_groups g JOIN upstream_connections c ON c.id=g.connection_id `+upstreamGroupModelReadJoinSQL+`
 WHERE g.connection_id=$1 AND `+upstreamGroupKeySQL+`=$2`, ref.ConnectionID, ref.RemoteKey)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err = rows.Err(); err != nil {
			return nil, err
		}
		return nil, service.ErrUpstreamConnectionChanged
	}
	result := &service.UpstreamGroupModels{}
	err = rows.Scan(pq.Array(&result.Models), pq.Array(&result.AutoTags), &result.Source, &result.Coverage, &result.Status, &result.ErrorCode, &result.ObservedAt, &result.FreshUntil,
		&result.SourceCount, &result.ReadySourceCount, &result.PendingSourceCount, &result.FailedSourceCount, &result.StaleSourceCount, &result.RefreshInProgress)
	return result, err
}

func (r *upstreamConnectionRepository) ListDueGroupModels(ctx context.Context, now time.Time, limit int) ([]service.UpstreamGroupReference, error) {
	rows, err := clientFromContext(ctx, r.client).QueryContext(ctx, `SELECT g.connection_id,`+upstreamGroupKeySQL+`
 FROM upstream_groups g JOIN upstream_connections c ON c.id=g.connection_id
 LEFT JOIN upstream_group_model_snapshots m ON m.connection_id=c.id AND m.remote_key=`+upstreamGroupKeySQL+`
 WHERE ((c.sync_enabled AND c.status NOT IN ('auth_error','needs_input','disabled'))
  OR (m.refresh_active AND m.connection_version=c.version))
 AND (m.lease_until IS NULL OR m.lease_until<=$1)
 AND (m.next_sync_at IS NULL OR m.next_sync_at<=$1 OR m.connection_version<>c.version OR m.refresh_active OR (m.coverage='bound_keys' AND `+upstreamGroupModelSourceDueSQL+`))
 ORDER BY m.last_attempt_at ASC NULLS FIRST,g.connection_id,g.id LIMIT $2`, now, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	refs := []service.UpstreamGroupReference{}
	for rows.Next() {
		var ref service.UpstreamGroupReference
		if err = rows.Scan(&ref.ConnectionID, &ref.RemoteKey); err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, rows.Err()
}

func (r *upstreamConnectionRepository) ClaimGroupModels(ctx context.Context, ref service.UpstreamGroupReference, version int64, token string, now time.Time, force bool) (bool, bool, error) {
	// $1 is the timestamp in the reusable source-due predicate.
	rows, err := clientFromContext(ctx, r.client).QueryContext(ctx, `INSERT INTO upstream_group_model_snapshots AS m(connection_id,remote_key,connection_version,lease_token,lease_until,last_attempt_at,refresh_generation,refresh_active)
 SELECT $2,$3,$4,$5,$1::timestamptz+INTERVAL '65 seconds',$1,CASE WHEN $6 THEN 1 ELSE 0 END,$6 FROM upstream_connections c
 WHERE c.id=$2 AND c.version=$4 AND EXISTS(SELECT 1 FROM upstream_groups g WHERE g.connection_id=c.id AND `+upstreamGroupKeySQL+`=$3)
 ON CONFLICT(connection_id,remote_key) DO UPDATE SET lease_token=$5,lease_until=$1::timestamptz+INTERVAL '65 seconds',last_attempt_at=$1,
 refresh_generation=CASE WHEN $6 AND (NOT m.refresh_active OR m.connection_version<>$4) THEN m.refresh_generation+1 ELSE m.refresh_generation END,
 refresh_active=CASE WHEN $6 THEN true WHEN m.connection_version<>$4 THEN false ELSE m.refresh_active END
 WHERE (m.lease_until IS NULL OR m.lease_until<=$1) AND (m.last_attempt_at IS NULL OR m.last_attempt_at<=$1::timestamptz-INTERVAL '30 seconds')
 AND ($6 OR m.next_sync_at<=$1 OR m.connection_version<>$4 OR m.refresh_active OR (m.coverage='bound_keys' AND `+upstreamGroupModelSourceDueSQL+`)) RETURNING refresh_active`, now, ref.ConnectionID, ref.RemoteKey, version, token, force)
	if err != nil {
		return false, false, err
	}
	defer func() { _ = rows.Close() }()
	claimed := rows.Next()
	var manualBatch bool
	if claimed {
		err = rows.Scan(&manualBatch)
	}
	if err == nil {
		err = rows.Err()
	}
	return claimed, manualBatch, err
}

func (r *upstreamConnectionRepository) ListGroupModelAccountSources(ctx context.Context, ref service.UpstreamGroupReference, version int64, now time.Time, limit int, force bool) ([]service.UpstreamGroupModelAccountSource, error) {
	if limit < 1 || limit > 8 {
		limit = 8
	}
	rows, err := clientFromContext(ctx, r.client).QueryContext(ctx, `SELECT x.source_fingerprint,x.key_fingerprint,
 jsonb_build_object('ID',a.id,'Platform',a.platform,'Type',a.type,'Credentials',a.credentials,'Extra',a.extra,
  'ProxyID',a.proxy_id,'Concurrency',a.concurrency,'Proxy',CASE WHEN p.id IS NOT NULL THEN
    jsonb_build_object('ID',p.id,'Protocol',p.protocol,'Host',p.host,'Port',p.port,'Username',COALESCE(p.username,''),'Password',COALESCE(p.password,''),'Status',p.status) ELSE NULL END)
 FROM upstream_group_model_current_sources x
 JOIN upstream_group_model_snapshots m ON m.connection_id=x.connection_id AND m.remote_key=x.remote_key
 JOIN accounts a ON a.id=x.account_id LEFT JOIN proxies p ON p.id=a.proxy_id
 LEFT JOIN upstream_group_model_account_snapshots s ON s.connection_id=x.connection_id AND s.remote_key=x.remote_key AND s.account_id=x.account_id
  AND s.connection_version=x.connection_version AND s.source_fingerprint=x.source_fingerprint
 WHERE x.connection_id=$1 AND x.remote_key=$2 AND x.connection_version=$3 AND x.eligible
 AND (s.account_id IS NULL OR (m.refresh_active AND s.refresh_generation<>m.refresh_generation)
  OR (NOT m.refresh_active AND ($6 OR s.next_sync_at<=$4)))
 ORDER BY s.last_attempt_at ASC NULLS FIRST,x.account_id LIMIT $5`, ref.ConnectionID, ref.RemoteKey, version, now, limit, force)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	sources := []service.UpstreamGroupModelAccountSource{}
	for rows.Next() {
		var source service.UpstreamGroupModelAccountSource
		var account []byte
		if err = rows.Scan(&source.Fingerprint, &source.KeyFingerprint, &account); err != nil {
			return nil, err
		}
		source.Account = &service.Account{}
		if err = json.Unmarshal(account, source.Account); err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}
	return sources, rows.Err()
}

func (r *upstreamConnectionRepository) SaveGroupModels(ctx context.Context, ref service.UpstreamGroupReference, version int64, token string, s service.UpstreamGroupModels, now time.Time) (bool, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.Client().QueryContext(ctx, `SELECT id FROM upstream_connections WHERE id=$1 AND version=$2 FOR UPDATE`, ref.ConnectionID, version)
	if err != nil {
		return false, err
	}
	exists := rows.Next()
	rowErr := rows.Err()
	_ = rows.Close()
	if rowErr != nil {
		return false, rowErr
	}
	if !exists {
		return false, nil
	}
	// Check ownership before touching source rows; an expired/stolen lease must
	// never allow an older worker to overwrite another worker's account results.
	rows, err = tx.Client().QueryContext(ctx, `SELECT coverage,connection_version,refresh_generation FROM upstream_group_model_snapshots m
 WHERE connection_id=$1 AND remote_key=$2 AND lease_token=$3
 AND EXISTS(SELECT 1 FROM upstream_groups g WHERE g.connection_id=$1 AND `+upstreamGroupKeySQL+`=$2) FOR UPDATE`, ref.ConnectionID, ref.RemoteKey, token)
	if err != nil {
		return false, err
	}
	var previousCoverage string
	var previousVersion int64
	var refreshGeneration int64
	exists = rows.Next()
	if exists {
		err = rows.Scan(&previousCoverage, &previousVersion, &refreshGeneration)
	} else {
		err = rows.Err()
	}
	_ = rows.Close()
	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}

	successfulAccounts := 0
	for _, result := range s.AccountResults {
		if result.Models == nil {
			result.Models = []string{}
		}
		if result.AutoTags == nil {
			result.AutoTags = []string{}
		}
		status := "error"
		if result.Success {
			status = "ready"
		}
		rows, err = tx.Client().QueryContext(ctx, `INSERT INTO upstream_group_model_account_snapshots AS old
   (connection_id,remote_key,account_id,connection_version,source_fingerprint,models,auto_tags,status,observed_at,fresh_until,last_attempt_at,next_sync_at,failures,refresh_generation)
  SELECT $1,$2,$3,$4,$5,$6,$7,$8,CASE WHEN $9 THEN $10::timestamptz END,
   CASE WHEN $9 THEN $10::timestamptz+INTERVAL '6 hours' END,$10,
   $10::timestamptz+CASE WHEN $9 THEN INTERVAL '6 hours' ELSE INTERVAL '5 minutes' END,CASE WHEN $9 THEN 0 ELSE 1 END,$11
  FROM upstream_group_model_current_sources x WHERE x.connection_id=$1 AND x.remote_key=$2 AND x.account_id=$3
   AND x.connection_version=$4 AND x.source_fingerprint=$5 AND x.eligible
  ON CONFLICT(connection_id,remote_key,account_id) DO UPDATE SET
   connection_version=$4,source_fingerprint=$5,status=$8,last_attempt_at=$10,refresh_generation=$11,
   models=CASE WHEN $9 THEN $6::text[] WHEN old.source_fingerprint=$5 AND old.connection_version=$4 THEN old.models ELSE '{}'::text[] END,
   auto_tags=CASE WHEN $9 THEN $7::text[] WHEN old.source_fingerprint=$5 AND old.connection_version=$4 THEN old.auto_tags ELSE '{}'::text[] END,
   observed_at=CASE WHEN $9 THEN $10 WHEN old.source_fingerprint=$5 AND old.connection_version=$4 THEN old.observed_at END,
   fresh_until=CASE WHEN $9 THEN $10::timestamptz+INTERVAL '6 hours' WHEN old.source_fingerprint=$5 AND old.connection_version=$4 THEN old.fresh_until END,
   failures=CASE WHEN $9 THEN 0 WHEN old.source_fingerprint=$5 AND old.connection_version=$4 THEN LEAST(old.failures+1,20) ELSE 1 END,
   next_sync_at=$10::timestamptz+CASE WHEN $9 THEN INTERVAL '6 hours' ELSE
    LEAST(3600,300*POWER(2,LEAST(CASE WHEN old.source_fingerprint=$5 AND old.connection_version=$4 THEN old.failures ELSE 0 END,4)))*INTERVAL '1 second' END
  RETURNING account_id`, ref.ConnectionID, ref.RemoteKey, result.AccountID, version, result.Fingerprint, pq.Array(result.Models), pq.Array(result.AutoTags), status, result.Success, now, refreshGeneration)
		if err != nil {
			return false, err
		}
		applied := rows.Next()
		rowErr = rows.Err()
		_ = rows.Close()
		if rowErr != nil {
			return false, rowErr
		}
		if applied && result.Success {
			successfulAccounts++
		}
	}
	bound := s.Coverage == "bound_keys"
	if bound && previousCoverage == "published" && previousVersion == version && successfulAccounts == 0 {
		bound = false
		s.Status = "error"
		s.ErrorCode = "unavailable"
	}
	publish := s.Coverage == "published" && s.Status == "ready"
	// A batch ends once each currently eligible source has been attempted, not
	// only when each succeeds. Failed sources then follow their ordinary backoff.
	batchPending := false
	if s.Coverage == "bound_keys" {
		rows, err = tx.Client().QueryContext(ctx, `SELECT EXISTS(
 SELECT 1 FROM upstream_group_model_current_sources x
 LEFT JOIN upstream_group_model_account_snapshots a ON a.connection_id=x.connection_id AND a.remote_key=x.remote_key
  AND a.account_id=x.account_id AND a.connection_version=x.connection_version AND a.source_fingerprint=x.source_fingerprint
 WHERE x.connection_id=$1 AND x.remote_key=$2 AND x.connection_version=$3 AND x.eligible
 AND (a.refresh_generation IS NULL OR a.refresh_generation<>$4))`, ref.ConnectionID, ref.RemoteKey, version, refreshGeneration)
		if err != nil {
			return false, err
		}
		if rows.Next() {
			err = rows.Scan(&batchPending)
		} else {
			err = rows.Err()
		}
		_ = rows.Close()
		if err != nil {
			return false, err
		}
	}
	if s.Models == nil {
		s.Models = []string{}
	}
	if s.AutoTags == nil {
		s.AutoTags = []string{}
	}
	rows, err = tx.Client().QueryContext(ctx, `UPDATE upstream_group_model_snapshots m SET
 models=CASE WHEN $5 THEN $6::text[] WHEN $13 OR m.connection_version<>$3 THEN '{}'::text[] ELSE m.models END,
 auto_tags=CASE WHEN $5 THEN $7::text[] WHEN $13 OR m.connection_version<>$3 THEN '{}'::text[] ELSE m.auto_tags END,
 source=CASE WHEN $13 THEN 'bound_keys' WHEN $5 THEN $8 WHEN m.connection_version<>$3 THEN '' ELSE m.source END,
 coverage=CASE WHEN $13 THEN 'bound_keys' WHEN $5 THEN 'published' WHEN m.connection_version<>$3 THEN 'unknown' ELSE m.coverage END,
 status=$9,error_code=$10,
 observed_at=CASE WHEN $5 THEN $11 WHEN $13 OR m.connection_version<>$3 THEN NULL ELSE m.observed_at END,
 fresh_until=CASE WHEN $5 THEN $11::timestamptz+INTERVAL '6 hours' WHEN $13 OR m.connection_version<>$3 THEN NULL ELSE m.fresh_until END,
 refresh_active=(m.refresh_active AND $14),
 next_sync_at=CASE WHEN m.refresh_active AND $14 THEN $11::timestamptz+INTERVAL '30 seconds'
  WHEN $13 THEN COALESCE((SELECT MIN(COALESCE(a.next_sync_at,$11::timestamptz+INTERVAL '30 seconds'))
   FROM upstream_group_model_current_sources x LEFT JOIN upstream_group_model_account_snapshots a
    ON a.connection_id=x.connection_id AND a.remote_key=x.remote_key AND a.account_id=x.account_id AND a.source_fingerprint=x.source_fingerprint AND a.connection_version=x.connection_version
   WHERE x.connection_id=$1 AND x.remote_key=$2 AND x.eligible),$11::timestamptz+INTERVAL '5 minutes')
  WHEN $5 THEN $11::timestamptz+INTERVAL '6 hours' ELSE $11::timestamptz+LEAST(3600,300*POWER(2,LEAST(m.failures,4)))*INTERVAL '1 second' END,
 failures=CASE WHEN $5 OR $12 THEN 0 ELSE LEAST(m.failures+1,20) END,
 connection_version=$3,lease_token='',lease_until=NULL
 WHERE m.connection_id=$1 AND m.remote_key=$2 AND m.lease_token=$4 RETURNING connection_id`,
		ref.ConnectionID, ref.RemoteKey, version, token, publish, pq.Array(s.Models), pq.Array(s.AutoTags), s.Source, s.Status, s.ErrorCode, now, successfulAccounts > 0, bound, batchPending)
	if err != nil {
		return false, err
	}
	saved := rows.Next()
	rowErr = rows.Err()
	_ = rows.Close()
	if rowErr != nil {
		return false, rowErr
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return saved, nil
}
