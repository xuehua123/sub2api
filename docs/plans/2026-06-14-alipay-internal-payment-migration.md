# Alipay Internal Payment Migration Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Move payment intake from the external `sub2-payment-page` service to Sub2API's built-in `/purchase` page for Alipay only.

**Architecture:** Sub2API becomes the only checkout and fulfillment system. The external payment page is kept read-only during cutover, then disabled after pending orders are drained. No historical order import, no coupon migration, and no WeChat migration are in scope.

**Tech Stack:** Sub2API Go backend, Vue frontend, Ent/PostgreSQL, built-in payment provider instances, official Alipay provider.

---

## Scope Lock

In scope:
- Built-in `/purchase` page.
- Official Alipay provider only.
- Balance recharge through built-in payment orders.
- Subscription plan purchase through built-in payment orders and V2 entitlements.
- Existing current subscription plan data in Sub2API.
- Backup and rollback plan.

Out of scope:
- WeChat Pay migration.
- External `sub2-payment-page` UI.
- Historical external orders.
- External coupons or payment discount coupons.
- Docker image builds on production hosts.
- Production deploy until local/staging verification passes.

## Target State

- Users buy from `https://<sub2api-domain>/purchase`.
- Alipay notify URL points to `https://<sub2api-domain>/api/v1/payment/webhook/alipay`.
- Alipay return URL points to `https://<sub2api-domain>/payment/result`.
- Only Alipay is visible/enabled on the payment page.
- External payment page stops accepting new orders.
- External secrets and Sub2API admin API key used by external page are rotated after cutover.

## Migration Mapping

External `sub2-payment-page` -> Sub2API:

- `gateway.alipayAppId` -> payment provider config `appId`
- `gateway.alipayPrivateKey` -> payment provider config `privateKey`
- `gateway.alipayPublicKey` -> payment provider config `publicKey`
- `gateway.callbackBaseUrl` -> replace with Sub2API public HTTPS origin
- `topup.exchange_rate` -> `payment_balance_recharge_multiplier = 1 / exchange_rate`
- `packages[]` -> existing/new Sub2API subscription plans

Do not migrate:

- `gateway.wxmchid`, `gateway.wxappid`, `gateway.wxapiv3key`, `gateway.wxcertserial`, `gateway.wxapiclientkey`
- `gateway.sub2apiUrl`, `gateway.sub2apiAdminKey`
- `coupons[]`
- external `payment_orders`

## Phase 1: Read-Only Inventory

1. Identify which production host actually runs `sub2-payment-page`.
2. Record container/service name, exposed domain, reverse proxy config, and current app directory.
3. Back up external payment page files and data:
   - app directory
   - `config.json` if used
   - SQLite `orders.db` if used
   - Postgres `payment_runtime_config` if used
   - Postgres `payment_orders` if used
   - nginx/caddy config for payment domain
4. Extract only these values into a local encrypted/operator-only note, not into git:
   - Alipay App ID
   - Alipay application private key
   - Alipay public key
   - external top-up exchange rate
   - external package list for comparison

Expected result:
- We know the exact Alipay credentials and old package pricing.
- No secret is pasted into chat, committed, or logged.

## Phase 2: Configure Sub2API Built-In Payment

1. Open Sub2API admin UI.
2. Go to `系统设置 -> 支付`.
3. Enable payment.
4. Enable only Alipay as the user-facing method.
5. Disable WeChat until a separate migration is planned.
6. If balance recharge remains available, set:
   - minimum amount
   - maximum amount
   - daily limit
   - `balance_recharge_multiplier`
7. If old external exchange rate is `0.35`, set built-in multiplier to `2.8571` because old logic credited `paid_amount / exchange_rate`.
8. Create or update one provider instance:
   - provider key: `alipay`
   - name: `Alipay Official`
   - supported types: `alipay`
   - payment mode:
     - `qrcode` if the merchant has face-to-face/precreate enabled
     - `redirect` if desktop QR creation fails and page pay should be used
   - `appId`: external `gateway.alipayAppId`
   - `privateKey`: external `gateway.alipayPrivateKey`
   - `publicKey`: external `gateway.alipayPublicKey`
   - notify base URL: Sub2API public HTTPS origin
   - return base URL: Sub2API public HTTPS origin

Expected callback URLs:
- `https://<sub2api-domain>/api/v1/payment/webhook/alipay`
- `https://<sub2api-domain>/payment/result`

## Phase 3: Configure Alipay Open Platform

1. In Alipay Open Platform, confirm the same app ID is used.
2. Confirm the app public key uploaded to Alipay matches the private key configured in Sub2API.
3. Confirm RSA2 signing is used.
4. Confirm IP whitelist, if enabled, includes the Sub2API server egress IP.
5. Configure/confirm async notify URL:
   - `https://<sub2api-domain>/api/v1/payment/webhook/alipay`
6. Configure/confirm return URL:
   - `https://<sub2api-domain>/payment/result`
7. Confirm product permissions:
   - face-to-face payment for QR/precreate mode, or
   - computer website payment / mobile website payment for redirect flow.

## Phase 4: Subscription Plan Alignment

1. In `订单管理 -> 订阅套餐`, compare built-in plans with old external `packages[]`.
2. Keep the current V2 entitlement plan model as source of truth.
3. For each public plan, verify:
   - price
   - original price
   - validity days/unit
   - authorized groups
   - daily/weekly/monthly limit
   - overage policy
   - for sale status
   - sort order
4. Do not create plans only because they existed in external page if they are obsolete.
5. Do not import external coupon configuration.

## Phase 5: Local/Staging Verification

1. Confirm `/purchase` loads and defaults to subscription tab.
2. Confirm only Alipay appears as a payment method.
3. Create a small balance recharge order.
4. Verify Alipay order creation returns either QR content or pay URL.
5. Complete a real low-value payment.
6. Verify:
   - internal `payment_orders` status becomes `COMPLETED`
   - user balance increases according to multiplier
   - recharge/referral order, if enabled, is recorded
7. Create a low-value subscription purchase using an internal test plan.
8. Verify:
   - internal order becomes `COMPLETED`
   - V2 `subscription_entitlements` row is created/extended correctly
   - API Key can select/use the purchased entitlement
   - `/subscriptions` shows the entitlement correctly
9. Test cancellation/expiry of unpaid order.
10. Test payment result page by returning from Alipay.

## Phase 6: Cutover

1. Announce a short payment maintenance window.
2. Disable new orders on external `sub2-payment-page`.
3. Keep external payment callback service alive long enough for already-created unpaid orders to either complete or expire.
4. Switch navigation and public links to Sub2API `/purchase`.
5. Confirm nginx routes `/purchase`, `/payment/result`, and `/api/v1/payment/webhook/alipay` to Sub2API.
6. Run one final low-value Alipay payment on production.
7. Confirm payment and entitlement fulfillment.

## Phase 7: Post-Cutover Cleanup

1. Export external unpaid/paid order snapshot for archive only.
2. After all external pending orders are drained, stop external payment page.
3. Remove external payment page from public reverse proxy.
4. Rotate the Sub2API admin API key that external payment page used.
5. Rotate SSH passwords or replace password login with key-only login.
6. Keep external backups for audit, but do not import them into Sub2API.
7. Monitor payment failures, webhook logs, and entitlement assignment for 24-48 hours.

## Rollback

Rollback before production payments are accepted:
- Re-enable external payment page.
- Point sidebar/public recharge links back to external page.
- Disable Sub2API payment in `系统设置 -> 支付`.

Rollback after successful internal payments:
- Prefer forward fix.
- Do not delete internal orders.
- If Alipay provider config fails, disable Alipay provider and temporarily re-enable external page.
- Manually reconcile any paid-but-unfulfilled order from Sub2API admin orders.

## Final Recommendation

Use Sub2API built-in payment as the only checkout. Migrate Alipay credentials and current sale plans only. Do not migrate external order history or coupons. Do not migrate WeChat now. Keep external service alive only as a temporary callback drain during cutover, then remove it and rotate the keys it used.
