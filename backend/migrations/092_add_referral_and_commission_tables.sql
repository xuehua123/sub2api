-- Migration: 092_add_referral_and_commission_tables
-- Referral codes, relations, recharge orders, commission rewards/ledgers, withdrawals.

CREATE TABLE IF NOT EXISTS referral_codes (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    code VARCHAR(64) NOT NULL UNIQUE,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_referral_codes_user_id
        FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE INDEX IF NOT EXISTS idx_referral_codes_user_id ON referral_codes(user_id);
CREATE INDEX IF NOT EXISTS idx_referral_codes_status ON referral_codes(status);
CREATE INDEX IF NOT EXISTS idx_referral_codes_is_default ON referral_codes(is_default);

CREATE TABLE IF NOT EXISTS referral_relations (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL UNIQUE,
    referrer_user_id BIGINT NOT NULL,
    bind_source VARCHAR(32) NOT NULL,
    bind_code VARCHAR(64),
    locked_at TIMESTAMPTZ,
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_referral_relations_user_id
        FOREIGN KEY (user_id) REFERENCES users(id),
    CONSTRAINT fk_referral_relations_referrer_user_id
        FOREIGN KEY (referrer_user_id) REFERENCES users(id)
);

CREATE INDEX IF NOT EXISTS idx_referral_relations_referrer_user_id ON referral_relations(referrer_user_id);
CREATE INDEX IF NOT EXISTS idx_referral_relations_bind_source ON referral_relations(bind_source);

CREATE TABLE IF NOT EXISTS referral_relation_histories (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    old_referrer_user_id BIGINT,
    new_referrer_user_id BIGINT,
    old_bind_code VARCHAR(64),
    new_bind_code VARCHAR(64),
    change_source VARCHAR(32) NOT NULL,
    changed_by BIGINT,
    reason TEXT,
    metadata_json TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_referral_relation_histories_user_id
        FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE INDEX IF NOT EXISTS idx_referral_relation_histories_user_id ON referral_relation_histories(user_id);
CREATE INDEX IF NOT EXISTS idx_referral_relation_histories_changed_by ON referral_relation_histories(changed_by);
CREATE INDEX IF NOT EXISTS idx_referral_relation_histories_created_at ON referral_relation_histories(created_at);

CREATE TABLE IF NOT EXISTS recharge_orders (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    external_order_id VARCHAR(128) NOT NULL,
    provider VARCHAR(32) NOT NULL,
    channel VARCHAR(32),
    currency VARCHAR(3) NOT NULL DEFAULT 'CNY',
    gross_amount DECIMAL(20,8) NOT NULL DEFAULT 0,
    discount_amount DECIMAL(20,8) NOT NULL DEFAULT 0,
    paid_amount DECIMAL(20,8) NOT NULL DEFAULT 0,
    gift_balance_amount DECIMAL(20,8) NOT NULL DEFAULT 0,
    credited_balance_amount DECIMAL(20,8) NOT NULL DEFAULT 0,
    refunded_amount DECIMAL(20,8) NOT NULL DEFAULT 0,
    chargeback_amount DECIMAL(20,8) NOT NULL DEFAULT 0,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    paid_at TIMESTAMPTZ,
    credited_at TIMESTAMPTZ,
    refunded_at TIMESTAMPTZ,
    chargeback_at TIMESTAMPTZ,
    idempotency_key VARCHAR(128),
    metadata_json TEXT,
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_recharge_orders_external_provider UNIQUE (external_order_id, provider),
    CONSTRAINT fk_recharge_orders_user_id
        FOREIGN KEY (user_id) REFERENCES users(id),
    CONSTRAINT chk_recharge_orders_currency CHECK (currency = 'CNY')
);

CREATE INDEX IF NOT EXISTS idx_recharge_orders_user_id ON recharge_orders(user_id);
CREATE INDEX IF NOT EXISTS idx_recharge_orders_status ON recharge_orders(status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_recharge_orders_idempotency_key ON recharge_orders(idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_recharge_orders_paid_at ON recharge_orders(paid_at);
CREATE INDEX IF NOT EXISTS idx_recharge_orders_refunded_at ON recharge_orders(refunded_at);

CREATE TABLE IF NOT EXISTS commission_rewards (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    source_user_id BIGINT NOT NULL,
    recharge_order_id BIGINT NOT NULL,
    level INT NOT NULL,
    rate_snapshot DECIMAL(10,4) NOT NULL DEFAULT 0,
    base_amount_snapshot DECIMAL(20,8) NOT NULL DEFAULT 0,
    reward_amount DECIMAL(20,8) NOT NULL DEFAULT 0,
    currency VARCHAR(3) NOT NULL DEFAULT 'CNY',
    reward_mode_snapshot VARCHAR(32) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    available_at TIMESTAMPTZ,
    frozen_at TIMESTAMPTZ,
    paid_at TIMESTAMPTZ,
    reversed_at TIMESTAMPTZ,
    rule_snapshot_json TEXT,
    relation_snapshot_json TEXT,
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_commission_rewards_order_user_level UNIQUE (recharge_order_id, user_id, level),
    CONSTRAINT fk_commission_rewards_user_id
        FOREIGN KEY (user_id) REFERENCES users(id),
    CONSTRAINT fk_commission_rewards_source_user_id
        FOREIGN KEY (source_user_id) REFERENCES users(id),
    CONSTRAINT fk_commission_rewards_recharge_order_id
        FOREIGN KEY (recharge_order_id) REFERENCES recharge_orders(id),
    CONSTRAINT chk_commission_rewards_currency CHECK (currency = 'CNY'),
    CONSTRAINT chk_commission_rewards_level CHECK (level = 1)
);

CREATE INDEX IF NOT EXISTS idx_commission_rewards_user_id ON commission_rewards(user_id);
CREATE INDEX IF NOT EXISTS idx_commission_rewards_source_user_id ON commission_rewards(source_user_id);
CREATE INDEX IF NOT EXISTS idx_commission_rewards_status ON commission_rewards(status);
CREATE INDEX IF NOT EXISTS idx_commission_rewards_available_at ON commission_rewards(available_at);

CREATE TABLE IF NOT EXISTS commission_withdrawals (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    withdrawal_no VARCHAR(64) NOT NULL UNIQUE,
    amount DECIMAL(20,8) NOT NULL DEFAULT 0,
    fee_amount DECIMAL(20,8) NOT NULL DEFAULT 0,
    net_amount DECIMAL(20,8) NOT NULL DEFAULT 0,
    currency VARCHAR(3) NOT NULL DEFAULT 'CNY',
    status VARCHAR(32) NOT NULL DEFAULT 'pending_review',
    payout_method VARCHAR(32) NOT NULL,
    payout_account_snapshot_json TEXT,
    reviewed_by BIGINT,
    reviewed_at TIMESTAMPTZ,
    paid_by BIGINT,
    paid_at TIMESTAMPTZ,
    reject_reason TEXT,
    remark TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_commission_withdrawals_user_id
        FOREIGN KEY (user_id) REFERENCES users(id),
    CONSTRAINT chk_commission_withdrawals_currency CHECK (currency = 'CNY')
);

CREATE INDEX IF NOT EXISTS idx_commission_withdrawals_user_id ON commission_withdrawals(user_id);
CREATE INDEX IF NOT EXISTS idx_commission_withdrawals_status ON commission_withdrawals(status);
CREATE INDEX IF NOT EXISTS idx_commission_withdrawals_created_at ON commission_withdrawals(created_at);

CREATE TABLE IF NOT EXISTS commission_withdrawal_items (
    id BIGSERIAL PRIMARY KEY,
    withdrawal_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    reward_id BIGINT NOT NULL,
    recharge_order_id BIGINT NOT NULL,
    allocated_amount DECIMAL(20,8) NOT NULL DEFAULT 0,
    fee_allocated_amount DECIMAL(20,8) NOT NULL DEFAULT 0,
    net_allocated_amount DECIMAL(20,8) NOT NULL DEFAULT 0,
    currency VARCHAR(3) NOT NULL DEFAULT 'CNY',
    status VARCHAR(32) NOT NULL DEFAULT 'frozen',
    freeze_ledger_id BIGINT,
    return_ledger_id BIGINT,
    paid_ledger_id BIGINT,
    reverse_ledger_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_commission_withdrawal_items_withdrawal_reward UNIQUE (withdrawal_id, reward_id),
    CONSTRAINT fk_commission_withdrawal_items_withdrawal_id
        FOREIGN KEY (withdrawal_id) REFERENCES commission_withdrawals(id),
    CONSTRAINT fk_commission_withdrawal_items_user_id
        FOREIGN KEY (user_id) REFERENCES users(id),
    CONSTRAINT fk_commission_withdrawal_items_reward_id
        FOREIGN KEY (reward_id) REFERENCES commission_rewards(id),
    CONSTRAINT fk_commission_withdrawal_items_recharge_order_id
        FOREIGN KEY (recharge_order_id) REFERENCES recharge_orders(id),
    CONSTRAINT chk_commission_withdrawal_items_currency CHECK (currency = 'CNY'),
    CONSTRAINT chk_commission_withdrawal_items_amount CHECK (allocated_amount > 0)
);

CREATE INDEX IF NOT EXISTS idx_commission_withdrawal_items_user_id ON commission_withdrawal_items(user_id);
CREATE INDEX IF NOT EXISTS idx_commission_withdrawal_items_reward_id ON commission_withdrawal_items(reward_id);
CREATE INDEX IF NOT EXISTS idx_commission_withdrawal_items_status ON commission_withdrawal_items(status);

CREATE TABLE IF NOT EXISTS commission_ledgers (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    reward_id BIGINT,
    recharge_order_id BIGINT,
    withdrawal_id BIGINT,
    withdrawal_item_id BIGINT,
    entry_type VARCHAR(64) NOT NULL,
    bucket VARCHAR(32) NOT NULL,
    amount DECIMAL(20,8) NOT NULL DEFAULT 0,
    currency VARCHAR(3) NOT NULL DEFAULT 'CNY',
    idempotency_key VARCHAR(128),
    operator_user_id BIGINT,
    remark TEXT,
    metadata_json TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_commission_ledgers_user_id
        FOREIGN KEY (user_id) REFERENCES users(id),
    CONSTRAINT fk_commission_ledgers_reward_id
        FOREIGN KEY (reward_id) REFERENCES commission_rewards(id),
    CONSTRAINT fk_commission_ledgers_recharge_order_id
        FOREIGN KEY (recharge_order_id) REFERENCES recharge_orders(id),
    CONSTRAINT fk_commission_ledgers_withdrawal_id
        FOREIGN KEY (withdrawal_id) REFERENCES commission_withdrawals(id),
    CONSTRAINT fk_commission_ledgers_withdrawal_item_id
        FOREIGN KEY (withdrawal_item_id) REFERENCES commission_withdrawal_items(id),
    CONSTRAINT chk_commission_ledgers_currency CHECK (currency = 'CNY')
);

CREATE INDEX IF NOT EXISTS idx_commission_ledgers_user_id ON commission_ledgers(user_id);
CREATE INDEX IF NOT EXISTS idx_commission_ledgers_reward_id ON commission_ledgers(reward_id);
CREATE INDEX IF NOT EXISTS idx_commission_ledgers_recharge_order_id ON commission_ledgers(recharge_order_id);
CREATE INDEX IF NOT EXISTS idx_commission_ledgers_withdrawal_id ON commission_ledgers(withdrawal_id);
CREATE INDEX IF NOT EXISTS idx_commission_ledgers_withdrawal_item_id ON commission_ledgers(withdrawal_item_id);
CREATE INDEX IF NOT EXISTS idx_commission_ledgers_entry_type ON commission_ledgers(entry_type);
CREATE INDEX IF NOT EXISTS idx_commission_ledgers_bucket ON commission_ledgers(bucket);
CREATE INDEX IF NOT EXISTS idx_commission_ledgers_created_at ON commission_ledgers(created_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_commission_ledgers_idempotency_key ON commission_ledgers(idempotency_key) WHERE idempotency_key IS NOT NULL;

CREATE TABLE IF NOT EXISTS commission_payout_accounts (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    method VARCHAR(32) NOT NULL,
    account_name VARCHAR(128) NOT NULL,
    account_no_masked VARCHAR(128),
    account_no_encrypted TEXT,
    bank_name VARCHAR(128),
    qr_image_url VARCHAR(512),
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_commission_payout_accounts_user_id
        FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE INDEX IF NOT EXISTS idx_commission_payout_accounts_user_id ON commission_payout_accounts(user_id);
CREATE INDEX IF NOT EXISTS idx_commission_payout_accounts_method ON commission_payout_accounts(method);
CREATE INDEX IF NOT EXISTS idx_commission_payout_accounts_status ON commission_payout_accounts(status);
CREATE INDEX IF NOT EXISTS idx_commission_payout_accounts_is_default ON commission_payout_accounts(is_default);
