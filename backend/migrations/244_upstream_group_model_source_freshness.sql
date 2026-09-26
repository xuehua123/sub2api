SET LOCAL lock_timeout = '5s';

-- Keep identity validity separate from observation freshness. Temporary probe
-- errors/expiry may prevent a new fetch but must not hide unchanged old results.
CREATE OR REPLACE VIEW upstream_group_model_current_sources AS
SELECT c.id AS connection_id,c.version AS connection_version,
 CASE WHEN g.remote_id<>'' THEN 'id:'||g.remote_id ELSE 'name:'||g.name END AS remote_key,
 a.id AS account_id,b.key_fingerprint,
 (b.status='ready' AND b.confidence='exact'
  AND b.resolution_kind IN ('fixed','inherited')
  AND b.fresh_until>NOW() AND COALESCE(jsonb_array_length(b.fallback_groups),0)=0
  AND a.parent_account_id IS NULL
  AND b.key_fingerprint='sha256:v1:'||encode(sha256(
   convert_to('upstream-api-key','UTF8')||decode('00','hex')||convert_to(COALESCE(
    NULLIF(btrim(a.credentials->>'api_key'),''),NULLIF(btrim(a.credentials->>'key'),''),
    NULLIF(btrim(a.credentials->>'token'),''),NULLIF(btrim(a.credentials->>'access_token'),''),''),'UTF8')),'hex')
 ) AS eligible,
 encode(sha256(convert_to(jsonb_build_array(
  a.platform,a.type,a.credentials,a.proxy_id,
  a.extra->'enable_tls_fingerprint',a.extra->'tls_fingerprint_profile_id',
  a.extra->'custom_base_url_enabled',a.extra->'anthropic_apikey_auth_scheme',
  a.extra->'openai_http_protocol',a.extra->'upstream_gzip_enabled',
  p.protocol,p.host,p.port,p.username,p.password,p.deleted_at,
  b.id,b.connection_id,b.remote_group_id,b.remote_group_name,b.resolution_kind,b.fallback_groups,b.key_fingerprint
 )::text,'UTF8')),'hex') AS source_fingerprint,
 (b.resolution_kind IN ('fixed','inherited')
  AND COALESCE(jsonb_array_length(b.fallback_groups),0)=0 AND a.parent_account_id IS NULL
  AND b.key_fingerprint='sha256:v1:'||encode(sha256(
   convert_to('upstream-api-key','UTF8')||decode('00','hex')||convert_to(COALESCE(
    NULLIF(btrim(a.credentials->>'api_key'),''),NULLIF(btrim(a.credentials->>'key'),''),
    NULLIF(btrim(a.credentials->>'token'),''),NULLIF(btrim(a.credentials->>'access_token'),''),''),'UTF8')),'hex')
 ) AS identity_valid,
 b.status AS binding_status,b.fresh_until AS binding_fresh_until
FROM upstream_groups g
JOIN upstream_connections c ON c.id=g.connection_id
JOIN upstream_account_bindings b ON b.connection_id=c.id
 AND ((b.remote_group_id<>'' AND b.remote_group_id=g.remote_id) OR (b.remote_group_id='' AND b.remote_group_name=g.name))
JOIN accounts a ON a.id=b.account_id AND a.deleted_at IS NULL
LEFT JOIN proxies p ON p.id=a.proxy_id;
