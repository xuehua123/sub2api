-- SAFETY CONTRACT: all legacy payment writers MUST be stopped before this
-- migration executes. Legacy slots only update refund_amount and cannot retain
-- the refund/chargeback split. This migration is NOT shared-database blue-green
-- compatible with those writers; restarting one after migration is forbidden.
-- The migration changes accounting classification only. It never adjusts user
-- balances, subscriptions, entitlements, referral rewards, or affiliate quota.

-- Persist provider refunds and chargebacks independently. refund_amount remains
-- the backward-readable combined projection after the legacy writers are gone.
ALTER TABLE payment_orders
    ADD COLUMN IF NOT EXISTS provider_refund_amount DECIMAL(20,2) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS chargeback_amount DECIMAL(20,2) NOT NULL DEFAULT 0;

-- Legacy refund_amount did not distinguish its two components. Only successful
-- refund statuses represent settled money; REFUND_REQUESTED/REFUNDING/
-- REFUND_PENDING/REFUND_FAILED may contain a requested amount. Start settled
-- rows with the conservative provider-refund classification, then replace it
-- only when durable chargeback evidence can be classified without ambiguity.
UPDATE payment_orders
SET provider_refund_amount = LEAST(GREATEST(refund_amount, 0), GREATEST(amount, 0))
WHERE refund_amount > 0
  AND status IN ('PARTIALLY_REFUNDED', 'REFUNDED')
  AND provider_refund_amount = 0
  AND chargeback_amount = 0;

DO $migration$
DECLARE
    order_row RECORD;
    component_kind RECORD;
    audit_row RECORD;
    detail_json JSONB;
    event_count BIGINT;
    generic_count BIGINT;
    evidence_component NUMERIC;
    chargeback_evidence NUMERIC;
    refund_evidence NUMERIC;
    manual_refund_evidence NUMERIC;
    provider_evidence NUMERIC;
    recharge_count BIGINT;
    recharge_chargeback NUMERIC;
    chosen_chargeback NUMERIC;
    gateway_amount NUMERIC;
    audit_combined NUMERIC;
    audit_delta NUMERIC;
    scaled_amount NUMERIC;
    paid_base NUMERIC;
    semantic TEXT;
    manual_detail JSONB;
BEGIN
    -- Chargeback evidence on a non-settled order cannot be reconciled to the
    -- settled refund projection and therefore blocks the migration.
    IF EXISTS (
        SELECT 1
        FROM payment_orders AS p
        JOIN payment_audit_logs AS pal ON pal.order_id = p.id::text
        WHERE (
                LEFT(pal.action, CHAR_LENGTH('CHARGEBACK_EVENT_')) = 'CHARGEBACK_EVENT_'
                OR pal.action = 'EXTERNAL_CHARGEBACK_SYNCED'
              )
          AND (p.status NOT IN ('PARTIALLY_REFUNDED', 'REFUNDED') OR p.refund_amount <= 0)
    ) THEN
        RAISE EXCEPTION 'migration 197 found chargeback evidence without a positive settled refund projection';
    END IF;

    FOR order_row IN
        SELECT p.id,
               p.amount::numeric AS amount,
               p.pay_amount::numeric AS pay_amount,
               p.refund_amount::numeric AS refund_amount,
               p.out_trade_no,
               p.payment_type
        FROM payment_orders AS p
        WHERE p.status IN ('PARTIALLY_REFUNDED', 'REFUNDED')
          AND p.refund_amount > 0
          AND (
                EXISTS (
                    SELECT 1
                    FROM payment_audit_logs AS pal
                    WHERE pal.order_id = p.id::text
                      AND (
                            LEFT(pal.action, CHAR_LENGTH('CHARGEBACK_EVENT_')) = 'CHARGEBACK_EVENT_'
                            OR pal.action = 'EXTERNAL_CHARGEBACK_SYNCED'
                          )
                )
                OR EXISTS (
                    SELECT 1
                    FROM recharge_orders AS r
                    WHERE r.external_order_id = p.out_trade_no
                      AND r.provider = CASE
                          WHEN p.payment_type IN ('stripe', 'card', 'link') THEN 'stripe'
                          WHEN p.payment_type LIKE 'alipay%' THEN 'alipay'
                          WHEN p.payment_type LIKE 'wxpay%' THEN 'wxpay'
                          ELSE p.payment_type
                      END
                      AND r.chargeback_amount > 0
                )
              )
        ORDER BY p.id
    LOOP
        IF order_row.amount <= 0
           OR order_row.refund_amount <= 0
           OR order_row.refund_amount > order_row.amount THEN
            RAISE EXCEPTION
                'migration 197 invalid settled projection for payment order %: amount=% refund_amount=%',
                order_row.id, order_row.amount, order_row.refund_amount;
        END IF;

        paid_base := CASE
            WHEN order_row.pay_amount > 0 THEN order_row.pay_amount
            ELSE order_row.amount
        END;
        chargeback_evidence := NULL;
        refund_evidence := NULL;

        -- Event-specific audit rows are the authoritative sequence. The generic
        -- action is a strictly validated fallback for older rows that predate
        -- event-specific markers; when both exist it is validated but not added
        -- a second time.
        FOR component_kind IN
            SELECT *
            FROM (VALUES
                ('chargeback', 'CHARGEBACK_EVENT_', 'EXTERNAL_CHARGEBACK_SYNCED', 'chargeback'),
                ('refund',     'REFUND_EVENT_',     'EXTERNAL_REFUND_SYNCED',     'refunded')
            ) AS kinds(component_name, event_prefix, generic_action, expected_status)
        LOOP
            SELECT COUNT(*)
            INTO event_count
            FROM payment_audit_logs AS pal
            WHERE pal.order_id = order_row.id::text
              AND LEFT(pal.action, CHAR_LENGTH(component_kind.event_prefix)) = component_kind.event_prefix;

            SELECT COUNT(*)
            INTO generic_count
            FROM payment_audit_logs AS pal
            WHERE pal.order_id = order_row.id::text
              AND pal.action = component_kind.generic_action;

            IF generic_count > 1 THEN
                RAISE EXCEPTION
                    'migration 197 ambiguous % generic evidence for payment order %',
                    component_kind.component_name, order_row.id;
            END IF;

            evidence_component := NULL;
            IF event_count > 0 OR generic_count > 0 THEN
                evidence_component := 0;
            END IF;

            FOR audit_row IN
                SELECT pal.id,
                       pal.action,
                       pal.detail,
                       LEFT(pal.action, CHAR_LENGTH(component_kind.event_prefix)) = component_kind.event_prefix AS is_event
                FROM payment_audit_logs AS pal
                WHERE pal.order_id = order_row.id::text
                  AND (
                        LEFT(pal.action, CHAR_LENGTH(component_kind.event_prefix)) = component_kind.event_prefix
                        OR pal.action = component_kind.generic_action
                      )
                ORDER BY pal.created_at, pal.id
            LOOP
                BEGIN
                    detail_json := audit_row.detail::jsonb;
                EXCEPTION WHEN OTHERS THEN
                    RAISE EXCEPTION
                        'migration 197 invalid JSON in % audit % for payment order %: %',
                        component_kind.component_name, audit_row.id, order_row.id, SQLERRM;
                END;

                IF JSONB_TYPEOF(detail_json) <> 'object'
                   OR NOT detail_json ? 'gatewayAmount'
                   OR JSONB_TYPEOF(detail_json -> 'gatewayAmount') <> 'number'
                   OR NOT detail_json ? 'amountSemantic'
                   OR JSONB_TYPEOF(detail_json -> 'amountSemantic') <> 'string'
                   OR NOT detail_json ? 'status'
                   OR JSONB_TYPEOF(detail_json -> 'status') <> 'string'
                   OR NOT detail_json ? 'refundAmountTotal'
                   OR JSONB_TYPEOF(detail_json -> 'refundAmountTotal') <> 'number'
                   OR NOT detail_json ? 'creditedDelta'
                   OR JSONB_TYPEOF(detail_json -> 'creditedDelta') <> 'number' THEN
                    RAISE EXCEPTION
                        'migration 197 malformed % audit % for payment order %',
                        component_kind.component_name, audit_row.id, order_row.id;
                END IF;

                BEGIN
                    gateway_amount := (detail_json ->> 'gatewayAmount')::numeric;
                    audit_combined := (detail_json ->> 'refundAmountTotal')::numeric;
                    audit_delta := (detail_json ->> 'creditedDelta')::numeric;
                EXCEPTION WHEN OTHERS THEN
                    RAISE EXCEPTION
                        'migration 197 non-numeric % audit % for payment order %: %',
                        component_kind.component_name, audit_row.id, order_row.id, SQLERRM;
                END;

                semantic := detail_json ->> 'amountSemantic';
                IF semantic NOT IN ('delta', 'total')
                   OR (detail_json ->> 'status') IS DISTINCT FROM component_kind.expected_status
                   OR gateway_amount <= 0
                   OR audit_combined < 0
                   OR audit_combined > order_row.refund_amount
                   OR audit_delta < 0
                   OR audit_delta > order_row.refund_amount THEN
                    RAISE EXCEPTION
                        'migration 197 inconsistent % audit % for payment order %',
                        component_kind.component_name, audit_row.id, order_row.id;
                END IF;

                scaled_amount := CASE
                    WHEN ABS(paid_base - order_row.amount) > 0.01
                        THEN ROUND(gateway_amount * order_row.amount / paid_base, 2)
                    ELSE ROUND(gateway_amount, 2)
                END;
                scaled_amount := LEAST(GREATEST(scaled_amount, 0), order_row.amount);

                -- Ignore the generic duplicate when event-specific rows exist,
                -- but only after validating every field above.
                IF audit_row.is_event OR event_count = 0 THEN
                    IF semantic = 'total' THEN
                        evidence_component := GREATEST(evidence_component, scaled_amount);
                    ELSE
                        evidence_component := LEAST(order_row.amount, evidence_component + scaled_amount);
                    END IF;
                END IF;
            END LOOP;

            IF component_kind.component_name = 'chargeback' THEN
                chargeback_evidence := evidence_component;
            ELSE
                refund_evidence := evidence_component;
            END IF;
        END LOOP;

        -- A manual provider refund is already recorded in credited-order units.
        -- It may duplicate an external confirmation, so both sources must agree.
        SELECT COUNT(*)
        INTO generic_count
        FROM payment_audit_logs AS pal
        WHERE pal.order_id = order_row.id::text
          AND pal.action = 'REFUND_SUCCESS';
        IF generic_count > 1 THEN
            RAISE EXCEPTION 'migration 197 ambiguous manual refund evidence for payment order %', order_row.id;
        END IF;
        manual_refund_evidence := NULL;
        IF generic_count = 1 THEN
            SELECT pal.detail::jsonb
            INTO manual_detail
            FROM payment_audit_logs AS pal
            WHERE pal.order_id = order_row.id::text
              AND pal.action = 'REFUND_SUCCESS';
            IF JSONB_TYPEOF(manual_detail) <> 'object'
               OR NOT manual_detail ? 'refundAmount'
               OR JSONB_TYPEOF(manual_detail -> 'refundAmount') <> 'number' THEN
                RAISE EXCEPTION 'migration 197 malformed manual refund evidence for payment order %', order_row.id;
            END IF;
            manual_refund_evidence := ROUND((manual_detail ->> 'refundAmount')::numeric, 2);
            IF manual_refund_evidence <= 0 OR manual_refund_evidence > order_row.amount THEN
                RAISE EXCEPTION 'migration 197 inconsistent manual refund evidence for payment order %', order_row.id;
            END IF;
        END IF;

        IF refund_evidence IS NOT NULL
           AND manual_refund_evidence IS NOT NULL
           AND refund_evidence <> manual_refund_evidence THEN
            RAISE EXCEPTION
                'migration 197 conflicting provider refund evidence for payment order %: external=% manual=%',
                order_row.id, refund_evidence, manual_refund_evidence;
        END IF;
        provider_evidence := COALESCE(refund_evidence, manual_refund_evidence);

        -- Referral settlement is an independent cross-check, not a prerequisite:
        -- users without a recharge/referral row are classified from audit data.
        SELECT COUNT(*),
               MAX(CASE
                   WHEN r.paid_amount > 0 THEN LEAST(
                       GREATEST(ROUND(order_row.amount * r.chargeback_amount / r.paid_amount, 2), 0),
                       order_row.amount
                   )
                   ELSE NULL
               END)
        INTO recharge_count, recharge_chargeback
        FROM recharge_orders AS r
        WHERE r.external_order_id = order_row.out_trade_no
          AND r.provider = CASE
              WHEN order_row.payment_type IN ('stripe', 'card', 'link') THEN 'stripe'
              WHEN order_row.payment_type LIKE 'alipay%' THEN 'alipay'
              WHEN order_row.payment_type LIKE 'wxpay%' THEN 'wxpay'
              ELSE order_row.payment_type
          END
          AND r.chargeback_amount > 0;

        IF recharge_count > 1 OR (recharge_count = 1 AND recharge_chargeback IS NULL) THEN
            RAISE EXCEPTION 'migration 197 ambiguous referral chargeback evidence for payment order %', order_row.id;
        END IF;
        IF chargeback_evidence IS NOT NULL
           AND recharge_chargeback IS NOT NULL
           AND chargeback_evidence <> recharge_chargeback THEN
            RAISE EXCEPTION
                'migration 197 conflicting chargeback evidence for payment order %: audit=% referral=%',
                order_row.id, chargeback_evidence, recharge_chargeback;
        END IF;
        chosen_chargeback := COALESCE(chargeback_evidence, recharge_chargeback);
        IF chosen_chargeback IS NULL OR chosen_chargeback <= 0 THEN
            RAISE EXCEPTION 'migration 197 could not classify chargeback evidence for payment order %', order_row.id;
        END IF;

        IF chargeback_evidence IS NOT NULL THEN
            IF provider_evidence IS NULL THEN
                IF chosen_chargeback <> order_row.refund_amount THEN
                    RAISE EXCEPTION
                        'migration 197 missing provider refund evidence for mixed reversal on payment order %',
                        order_row.id;
                END IF;
                provider_evidence := 0;
            END IF;
            IF ROUND(provider_evidence + chosen_chargeback, 2) <> ROUND(order_row.refund_amount, 2) THEN
                RAISE EXCEPTION
                    'migration 197 reversal evidence does not match settled projection for payment order %: provider=% chargeback=% projection=%',
                    order_row.id, provider_evidence, chosen_chargeback, order_row.refund_amount;
            END IF;
        ELSE
            provider_evidence := ROUND(order_row.refund_amount - chosen_chargeback, 2);
        END IF;

        IF provider_evidence < 0
           OR chosen_chargeback < 0
           OR ROUND(provider_evidence + chosen_chargeback, 2) <> ROUND(order_row.refund_amount, 2) THEN
            RAISE EXCEPTION
                'migration 197 derived components do not match settled projection for payment order %',
                order_row.id;
        END IF;

        UPDATE payment_orders
        SET provider_refund_amount = provider_evidence,
            chargeback_amount = chosen_chargeback
        WHERE id = order_row.id;
    END LOOP;

    IF EXISTS (
        SELECT 1
        FROM payment_orders AS p
        WHERE p.status IN ('PARTIALLY_REFUNDED', 'REFUNDED')
          AND ROUND(p.provider_refund_amount + p.chargeback_amount, 2) <> ROUND(p.refund_amount, 2)
    ) THEN
        RAISE EXCEPTION 'migration 197 left settled payment reversal components inconsistent with refund_amount';
    END IF;
END
$migration$;

COMMENT ON COLUMN payment_orders.provider_refund_amount IS
    'Cumulative credited amount reversed by provider refunds only';
COMMENT ON COLUMN payment_orders.chargeback_amount IS
    'Cumulative credited amount reversed by chargebacks only';
COMMENT ON COLUMN payment_orders.refund_amount IS
    'Backward-compatible projection of provider_refund_amount + chargeback_amount';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'chk_payment_orders_reversal_components'
          AND conrelid = 'payment_orders'::regclass
    ) THEN
        ALTER TABLE payment_orders
            ADD CONSTRAINT chk_payment_orders_reversal_components
            CHECK (
                provider_refund_amount >= 0
                AND chargeback_amount >= 0
                AND provider_refund_amount + chargeback_amount <= amount
            ) NOT VALID;
    END IF;
END $$;

ALTER TABLE payment_orders
    VALIDATE CONSTRAINT chk_payment_orders_reversal_components;

-- Once the historical split has been classified, settled rows must never
-- accept a legacy projection-only update. This turns the rollout contract into
-- a database-enforced invariant: accidentally restarting an old writer fails
-- its transaction instead of silently assigning the projection gap to the
-- wrong reversal component.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'chk_payment_orders_reversal_projection'
          AND conrelid = 'payment_orders'::regclass
    ) THEN
        ALTER TABLE payment_orders
            ADD CONSTRAINT chk_payment_orders_reversal_projection
            CHECK (
                status NOT IN ('PARTIALLY_REFUNDED', 'REFUNDED')
                OR provider_refund_amount + chargeback_amount = refund_amount
            ) NOT VALID;
    END IF;
END $$;

ALTER TABLE payment_orders
    VALIDATE CONSTRAINT chk_payment_orders_reversal_projection;
