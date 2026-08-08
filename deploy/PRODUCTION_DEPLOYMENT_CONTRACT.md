# Production deployment contract

These commands are production safety boundaries. Install the reviewed scripts
as root-owned, non-symlink files; the stable deploy script verifies the exact
cutover helper hash before every run.

`/opt/platform`, its `docker-compose.yml`, the resolved Nginx configuration and
its parent directory must be root-owned and not writable by group or others.

## One-time state bootstrap

The ordinary deploy command never creates missing monotonic state. Before the
first deployment with this contract:

1. Confirm that no active blue/green container has
   `AFFILIATE_REFUND_REVERSAL_ENABLED=true` or the
   `org.sub2api.capability.payment-reversal-components=1` image label.
2. Independently query the production database and confirm that no affiliate
   reversal has ever been persisted, including non-zero `reversed_amount` or
   equivalent reversal ledger state. The bootstrap script deliberately has no
   database credentials and does not perform this check itself.
3. Run, exactly once:

   ```bash
   sudo /usr/local/sbin/sub2api-deploy-state-bootstrap \
     --operator-confirm-database-has-no-affiliate-reversals
   ```

If the state directory or either state file later disappears, deployment must
stop. Do not recreate `absent` state during incident handling.

## First migration-197-capable image

The first image labeled
`org.sub2api.capability.payment-reversal-components=1` is a maintenance
transition even while the affiliate gate remains disabled. Deployment records
pending state and gracefully stops the old writer before starting the candidate
and allowing migration 197 to run. Automatic downgrade is forbidden. After
activation, every future image must retain this capability.

The same stop-before-start boundary is also mandatory for usage-cleanup
workers in this release. New tasks may contain `entitlement_id`; a legacy
worker ignores that JSON field and would delete the wider date range. Do not
create or execute an entitlement-scoped cleanup task until every legacy app and
worker on that data plane has stopped, and never restart a legacy worker after
the candidate becomes available.

## Affiliate refund reversal activation

Deploy the same capable immutable digest first with stage `disabled`, observe
it, and then deploy that exact image with stage `enabled`. The old gate-false
slot is stopped before gate-true starts. After activation, gate-false images are
not legal rollback targets.

## Pending recovery

Both irreversible contracts intentionally fail closed in `pending`. A normal
deployment cannot clear or overwrite pending state. Preserve the pending files,
containers, image IDs, revisions, Nginx configuration, and rollback record;
then perform reviewed forward recovery with the exact recorded capable digest.
Do not delete state files, set them back to `absent`, enable the old writer, or
use a gate-false image as a rollback shortcut.
