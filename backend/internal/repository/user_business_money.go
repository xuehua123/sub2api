package repository

// Payment orders are authoritative. Recharge ledger rows are used only for their
// CNY settlement snapshots, or as standalone payments when no payment order exists.
const userBusinessMoneyCTE = `,
ledger_source AS MATERIALIZED (
 SELECT r.*,CASE WHEN pg_input_is_valid(COALESCE(metadata_json,''),'jsonb') THEN metadata_json::jsonb ELSE '{}'::jsonb END AS meta
 FROM recharge_orders r WHERE paid_at < $8 AND ($9::bigint=0 OR user_id=$9)
 AND ((paid_at >= $1 AND paid_at < $2) OR refunded_at >= $1 OR chargeback_at >= $1)
), ledger AS MATERIALIZED (
 SELECT r.*,CASE
 WHEN r.meta->>'order_type' IN ('balance','subscription') THEN r.meta->>'order_type'
 WHEN r.provider='sub2apipay' AND r.meta->>'redeem_type' IN ('balance','subscription') THEN r.meta->>'redeem_type'
 WHEN r.provider='sub2apipay' AND r.channel IN ('balance','subscription') THEN r.channel
 WHEN r.provider='sub2apipay' THEN 'unknown'
 ELSE 'balance' END AS business_order_type
 FROM ledger_source r
), orders AS MATERIALIZED (
 SELECT p.*,COALESCE(NULLIF(UPPER(TRIM(p.provider_snapshot->>'currency')),''),'CNY') AS currency,
 CASE WHEN COALESCE(NULLIF(UPPER(TRIM(p.provider_snapshot->>'currency')),''),'CNY')='CNY' THEN p.pay_amount
 ELSE (SELECT r.paid_amount FROM recharge_orders r
 WHERE r.currency='CNY' AND r.user_id=p.user_id AND (p.out_trade_no='' OR r.external_order_id=p.out_trade_no)
 AND (CASE WHEN pg_input_is_valid(COALESCE(r.metadata_json,''),'jsonb') THEN r.metadata_json::jsonb ELSE '{}'::jsonb END)->>'payment_order_id'=p.id::text
 ORDER BY r.id DESC LIMIT 1) END AS paid_cny
 FROM payment_orders p WHERE p.paid_at IS NOT NULL AND p.paid_at < $8 AND ($9::bigint=0 OR p.user_id=$9)
 AND ($9::bigint>0 OR (p.paid_at >= $1 AND p.paid_at<$2) OR p.refund_at >= $1
 OR EXISTS(SELECT 1 FROM payment_audit_logs ev WHERE ev.order_id=p.id::text AND ev.created_at >= $1 AND ev.created_at<$2
 AND (ev.action='REFUND_SUCCESS' OR ev.action='EXTERNAL_REFUND_SYNCED' OR ev.action='EXTERNAL_CHARGEBACK_SYNCED' OR ev.action LIKE 'REFUND_EVENT_%' OR ev.action LIKE 'CHARGEBACK_EVENT_%')))
 AND (p.status IN ('PAID','COMPLETED','RECHARGING','REFUNDED','PARTIALLY_REFUNDED','REFUND_REQUESTED','REFUND_FAILED','REFUNDING','REFUND_PENDING')
 OR (p.status='FAILED' AND EXISTS(SELECT 1 FROM payment_audit_logs paid WHERE paid.order_id=p.id::text AND paid.action='ORDER_PAID')))
), audit_checkpoints AS MATERIALIZED (
 SELECT p.id, a.created_at AS at,a.id AS audit_id,
 CASE WHEN COALESCE(d.value->>'refundAmountTotal',d.value->>'refundAmount','') ~ '^[0-9]+([.][0-9]+)?$'
 THEN LEAST(p.amount,COALESCE(d.value->>'refundAmountTotal',d.value->>'refundAmount')::numeric) ELSE NULL END AS cumulative
 FROM orders p JOIN payment_audit_logs a ON a.order_id=p.id::text
 CROSS JOIN LATERAL (SELECT CASE WHEN pg_input_is_valid(a.detail,'jsonb') THEN a.detail::jsonb ELSE '{}'::jsonb END AS value) d
 WHERE a.created_at<$2 AND (a.action='REFUND_SUCCESS' OR a.action='EXTERNAL_REFUND_SYNCED' OR a.action='EXTERNAL_CHARGEBACK_SYNCED' OR a.action LIKE 'REFUND_EVENT_%' OR a.action LIKE 'CHARGEBACK_EVENT_%')
), checkpoints AS (
 SELECT id,at,audit_id,cumulative,cumulative IS NULL AS uncertain FROM audit_checkpoints
 UNION ALL
 SELECT p.id,p.refund_at,9223372036854775807::bigint,LEAST(p.amount,
 CASE WHEN p.status IN ('COMPLETED','PARTIALLY_REFUNDED','REFUNDED','REFUND_FAILED') THEN GREATEST(p.refund_amount,p.provider_refund_amount+p.chargeback_amount) ELSE p.provider_refund_amount+p.chargeback_amount END),true
 FROM orders p WHERE p.refund_at IS NOT NULL AND p.refund_at<$2
 AND LEAST(p.amount,CASE WHEN p.status IN ('COMPLETED','PARTIALLY_REFUNDED','REFUNDED','REFUND_FAILED') THEN GREATEST(p.refund_amount,p.provider_refund_amount+p.chargeback_amount) ELSE p.provider_refund_amount+p.chargeback_amount END)
 > COALESCE((SELECT MAX(a.cumulative) FROM audit_checkpoints a WHERE a.id=p.id),0)
), refund_steps AS (
 SELECT *,GREATEST(0,cumulative-COALESCE(MAX(cumulative) OVER(PARTITION BY id ORDER BY at,audit_id ROWS BETWEEN UNBOUNDED PRECEDING AND 1 PRECEDING),0)) AS delta
 FROM checkpoints
), cash_events AS MATERIALIZED (
 SELECT p.user_id,p.id AS order_id,p.paid_at AS at,p.order_type,COALESCE(p.paid_cny,0) AS paid,0::numeric AS refund,
 p.paid_cny IS NULL AS uncertain,p.paid_cny IS NOT NULL AND p.currency<>'CNY' AS converted
 FROM orders p WHERE p.paid_at >= $1 AND p.paid_at < $2
 UNION ALL
 SELECT p.user_id,p.id,r.at,p.order_type,0,
CASE WHEN p.amount>0 AND p.paid_cny IS NOT NULL AND r.cumulative IS NOT NULL THEN ROUND(r.cumulative*p.paid_cny/p.amount,2)-ROUND((r.cumulative-r.delta)*p.paid_cny/p.amount,2) ELSE 0 END,
 r.uncertain OR p.paid_cny IS NULL OR p.amount<=0,p.currency<>'CNY'
 FROM refund_steps r JOIN orders p ON p.id=r.id WHERE r.at >= $1 AND (r.delta>0 OR r.uncertain)
 UNION ALL
 SELECT p.user_id,p.id,$1::timestamptz,p.order_type,0,0,true,p.currency<>'CNY'
 FROM orders p WHERE EXISTS(SELECT 1 FROM audit_checkpoints bad WHERE bad.id=p.id AND bad.cumulative IS NULL AND bad.at<$1
 AND NOT EXISTS(SELECT 1 FROM audit_checkpoints good WHERE good.id=p.id AND good.cumulative IS NOT NULL AND good.at>=bad.at AND good.at<$1))
 AND EXISTS(SELECT 1 FROM refund_steps current WHERE current.id=p.id AND current.at >= $1)
 UNION ALL
 SELECT r.user_id,NULL,r.paid_at,r.business_order_type,CASE WHEN r.currency='CNY' THEN r.paid_amount ELSE 0 END,0,(r.currency<>'CNY' OR r.business_order_type='unknown'),false
 FROM ledger r WHERE r.paid_at >= $1 AND r.paid_at<$2 AND r.status NOT IN ('pending','failed','cancelled')
 AND NOT EXISTS(SELECT 1 FROM payment_orders p WHERE p.user_id=r.user_id AND (r.meta->>'payment_order_id'=p.id::text OR (p.out_trade_no<>'' AND p.out_trade_no=r.external_order_id)))
 UNION ALL
 -- Standalone ledgers retain a cumulative reversal and its latest timestamp only.
 -- Exact period attribution requires the whole payment lifetime in the selected range.
 SELECT r.user_id,NULL,ev.at,r.business_order_type,0,CASE WHEN r.currency='CNY' THEN ev.amount ELSE 0 END,(r.currency<>'CNY' OR r.paid_at<$1),false
 FROM ledger r CROSS JOIN LATERAL (VALUES(r.refunded_at,r.refunded_amount),(r.chargeback_at,r.chargeback_amount)) ev(at,amount)
 WHERE ev.at >= $1 AND ev.at < $2 AND ev.amount>0
 AND NOT EXISTS(SELECT 1 FROM payment_orders p WHERE p.user_id=r.user_id AND (r.meta->>'payment_order_id'=p.id::text OR (p.out_trade_no<>'' AND p.out_trade_no=r.external_order_id)))
 UNION ALL
 SELECT r.user_id,NULL,$1::timestamptz,r.business_order_type,0,0,true,false
 FROM ledger r WHERE r.paid_at<$2 AND ((r.refunded_amount>0 AND r.refunded_at>=$2) OR (r.chargeback_amount>0 AND r.chargeback_at>=$2))
 AND NOT EXISTS(SELECT 1 FROM payment_orders p WHERE p.user_id=r.user_id AND (r.meta->>'payment_order_id'=p.id::text OR (p.out_trade_no<>'' AND p.out_trade_no=r.external_order_id)))
), cash AS MATERIALIZED (
 SELECT user_id,SUM(paid) AS paid,SUM(paid) FILTER(WHERE order_type='balance') AS balance_paid,
 SUM(paid) FILTER(WHERE order_type='subscription') AS subscription_paid,SUM(refund) AS refund,
 COUNT(*) FILTER(WHERE uncertain) AS uncertain_count
 FROM cash_events GROUP BY user_id
) `
