# Production deployment contract

These commands are production safety boundaries. Install the reviewed scripts
as root-owned, non-symlink files; the stable deploy script verifies the exact
cutover helper hash before every run.

The OVH stable host uses `/srv/sub2api-migration/incoming`,
`docker-compose.migration.yml`, and `sub2api.env`. The environment file must be
`root:root` mode `0600`; the deployment tree, resolved Nginx configuration, and
every parent directory must be root-owned, non-symlink, and not writable by
group or others.

The reviewed OVH v2 package is installed with
`sub2api-install-stable-ovh.sh`. Run `--preflight` first, then `--install`. The
installer snapshots the package into a root-only directory, holds the deploy
and cutover locks, verifies the rendered shared PostgreSQL/Redis/network/volume
contract, migrates all three live Nginx routes to one shared upstream include,
and atomically replaces the fixed forced-command entry. Existing scripts are
archived under `/root`; do not delete that archive until a successful release
has completed its observation window.

## One-time state bootstrap

The ordinary deploy command never creates missing monotonic state. Before the
first deployment with this contract:

1. Confirm that every active blue/green container has the explicit literal
   `AFFILIATE_REFUND_REVERSAL_ENABLED=false` and does not have the
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

The bootstrap uses `state-bootstrap.pending` as a crash-recovery journal. A
matching journal may complete the same first bootstrap transaction. A lone
state file without that journal is treated as historical state loss and always
requires operator review. Ordinary deployment also refuses to run while the
journal exists.

## Migration 197 expand phase

Migration 197 installs a deferred legacy-writer bridge before enforcing the new
refund/chargeback projection. Images in this expand phase retain
`org.sub2api.capability.payment-reversal-components=0`, so the inactive slot may
start, migrate, and prewarm while the legacy slot continues serving. The bridge
reconciles a legacy `refund_amount` delta from the audit row committed in the
same transaction. A later reviewed contract release may remove the bridge and
set the capability to `1` only after every legacy writer has been retired.

Entitlement-scoped usage-cleanup tasks use the `pending_v2` state. New workers
claim both `pending` and `pending_v2`; legacy workers claim only `pending`, so
they cannot execute a task whose `entitlement_id` they do not understand.

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
