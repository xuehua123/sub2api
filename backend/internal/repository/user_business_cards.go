package repository

// Only an explicit entitlement/order edge is used for actual card payment totals.
// Legacy and manually assigned cards without that edge remain unknown, not list-price estimates.
const userBusinessCardsSQL = `COALESCE((SELECT jsonb_agg(card ORDER BY expiry,id) FROM (
 SELECT e.id,e.expires_at AS expiry,jsonb_build_object('id',e.id,'name',e.name,'source','entitlement','expires_at',e.expires_at,
 'daily_limit',NULLIF(e.daily_limit_usd,0),'weekly_limit',NULLIF(e.weekly_limit_usd,0),'monthly_limit',NULLIF(e.monthly_limit_usd,0),
 'daily_remaining',CASE WHEN (e.daily_limit_usd IS NULL OR e.daily_limit_usd<=0) THEN NULL ELSE GREATEST(0,e.daily_limit_usd-CASE WHEN e.daily_window_start IS NULL OR ((e.daily_window_start AT TIME ZONE $8)::date < ($9 AT TIME ZONE $8)::date AND e.expires_at>e.starts_at+interval '24 hours') THEN 0 ELSE e.daily_usage_usd END) END,
 'weekly_remaining',CASE WHEN (e.weekly_limit_usd IS NULL OR e.weekly_limit_usd<=0) THEN NULL ELSE GREATEST(0,e.weekly_limit_usd-CASE WHEN COALESCE(e.weekly_window_start,e.starts_at)+interval '168 hours' <= $9 THEN 0 ELSE e.weekly_usage_usd END) END,
 'monthly_remaining',CASE WHEN (e.monthly_limit_usd IS NULL OR e.monthly_limit_usd<=0) THEN NULL ELSE GREATEST(0,e.monthly_limit_usd-CASE WHEN COALESCE(e.monthly_window_start,e.starts_at)+interval '720 hours' <= $9 THEN 0 ELSE e.monthly_usage_usd END) END,
 'paid_cny',(SELECT CASE WHEN COUNT(*)>0 AND COUNT(*)=COUNT(p.paid_cny) THEN SUM(p.paid_cny) END FROM orders p WHERE p.subscription_entitlement_id=e.id),
 'payments_count',(SELECT COUNT(*) FROM orders p WHERE p.subscription_entitlement_id=e.id)) AS card
 FROM subscription_entitlements e WHERE e.user_id=$10 AND e.deleted_at IS NULL AND e.status='active' AND e.starts_at <= $9 AND e.expires_at>$9
 UNION ALL
 SELECT -s.id,s.expires_at,jsonb_build_object('id',-s.id,'name',g.name,'source','legacy','expires_at',s.expires_at,'paid_cny',(SELECT CASE WHEN COUNT(*)>0 AND COUNT(*)=COUNT(p.paid_cny) THEN SUM(p.paid_cny) END FROM orders p WHERE p.user_id=s.user_id AND p.order_type='subscription' AND p.subscription_entitlement_id IS NULL AND p.subscription_group_id=s.group_id AND EXISTS(SELECT 1 FROM regexp_split_to_table(COALESCE(s.notes,''),E'[\r\n]+') line WHERE trim(line) IN ('payment_order_id='||p.id::text,'payment order '||p.id::text))),
 'payments_count',(SELECT COUNT(*) FROM orders p WHERE p.user_id=s.user_id AND p.order_type='subscription' AND p.subscription_entitlement_id IS NULL AND p.subscription_group_id=s.group_id AND EXISTS(SELECT 1 FROM regexp_split_to_table(COALESCE(s.notes,''),E'[\r\n]+') line WHERE trim(line) IN ('payment_order_id='||p.id::text,'payment order '||p.id::text))),
 'daily_limit',NULLIF(g.daily_limit_usd,0),'weekly_limit',NULLIF(g.weekly_limit_usd,0),'monthly_limit',NULLIF(g.monthly_limit_usd,0),
 'daily_remaining',CASE WHEN (g.daily_limit_usd IS NULL OR g.daily_limit_usd<=0) THEN NULL ELSE GREATEST(0,g.daily_limit_usd-CASE WHEN s.daily_window_start IS NULL OR ((s.daily_window_start AT TIME ZONE $8)::date < ($9 AT TIME ZONE $8)::date AND s.expires_at>s.starts_at+interval '24 hours') THEN 0 ELSE s.daily_usage_usd END) END,
 'weekly_remaining',CASE WHEN (g.weekly_limit_usd IS NULL OR g.weekly_limit_usd<=0) THEN NULL ELSE GREATEST(0,g.weekly_limit_usd-CASE WHEN COALESCE(s.weekly_window_start,s.starts_at)+interval '168 hours'<=$9 THEN 0 ELSE s.weekly_usage_usd END) END,
 'monthly_remaining',CASE WHEN (g.monthly_limit_usd IS NULL OR g.monthly_limit_usd<=0) THEN NULL ELSE GREATEST(0,g.monthly_limit_usd-CASE WHEN COALESCE(s.monthly_window_start,s.starts_at)+interval '720 hours'<=$9 THEN 0 ELSE s.monthly_usage_usd END) END)
 FROM user_subscriptions s JOIN groups g ON g.id=s.group_id WHERE s.user_id=$10 AND s.deleted_at IS NULL AND s.status='active' AND s.starts_at<=$9 AND s.expires_at>$9
 AND NOT EXISTS(SELECT 1 FROM subscription_entitlements e WHERE e.legacy_subscription_id=s.id AND e.deleted_at IS NULL)
) cards),'[]'::jsonb)`
