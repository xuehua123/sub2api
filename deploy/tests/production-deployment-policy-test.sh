#!/usr/bin/env bash

set -Eeuo pipefail

REPO_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
STABLE_SCRIPT="$REPO_ROOT/deploy/sub2api-deploy-stable.sh"
BOOTSTRAP_SCRIPT="$REPO_ROOT/deploy/sub2api-deploy-state-bootstrap.sh"
DEPLOYMENT_CONTRACT_DOC="$REPO_ROOT/deploy/PRODUCTION_DEPLOYMENT_CONTRACT.md"
CANARY_WORKFLOW="$REPO_ROOT/.github/workflows/deploy-shanghai.yml"
IMAGE_WORKFLOW="$REPO_ROOT/.github/workflows/docker-image.yml"
CAPABILITY_MANIFEST="$REPO_ROOT/deploy/image-capabilities.txt"
CONTRACT_TEST="$REPO_ROOT/deploy/tests/stable-deployment-contract-test.sh"
CUTOVER_SCRIPT="$REPO_ROOT/deploy/sub2api-nginx-bluegreen-cutover.sh"
CUTOVER_TEST="$REPO_ROOT/deploy/tests/nginx-bluegreen-cutover-test.sh"
AGENT_RULES="$REPO_ROOT/AGENTS.md"

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

bash -n "$STABLE_SCRIPT"
bash -n "$BOOTSTRAP_SCRIPT"
bash -n "$CUTOVER_SCRIPT"
bash -n "$CONTRACT_TEST"
bash -n "$CUTOVER_TEST"

grep -Fq 'blue) printf '\''%s'\'' 18080' "$STABLE_SCRIPT"
grep -Fq 'green) printf '\''%s'\'' 28080' "$STABLE_SCRIPT"
grep -Fq '127.0.0.1:18080:8080' "$STABLE_SCRIPT"
grep -Fq '127.0.0.1:28080:8080' "$STABLE_SCRIPT"

if grep -Fq '18082' "$STABLE_SCRIPT"; then
  echo "Stable deployment still contains the obsolete green port 18082" >&2
  exit 1
fi

if grep -Eqi 'pipixia|NEW_PIPIXIA|appleboy/ssh-action' "$CANARY_WORKFLOW"; then
  echo "Canary workflow must not contain a Pipixia application deployment path" >&2
  exit 1
fi

if grep -Eq 'docker( compose)? build' "$STABLE_SCRIPT" "$CANARY_WORKFLOW"; then
  echo "Production canary deployment must never build images on the host" >&2
  exit 1
fi

grep -Fq 'affiliate_refund_reversal_stage:' "$CANARY_WORKFLOW"
grep -Fq 'type: choice' "$CANARY_WORKFLOW"
grep -Fq -- '- disabled' "$CANARY_WORKFLOW"
grep -Fq -- '- enabled' "$CANARY_WORKFLOW"
grep -Fq '[[ "$DEPLOY_IMAGE_TAG" =~ ^sha256:[0-9a-f]{64}$ ]]' "$CANARY_WORKFLOW"
grep -Fq '[[ "$DEPLOY_IMAGE_TAG" =~ ^ghcr\.io/xuehua123/sub2api@sha256:[0-9a-f]{64}$ ]]' "$CANARY_WORKFLOW"
if grep -Fq 'sha-[0-9a-f]{7}' "$CANARY_WORKFLOW"; then
  echo "Production canary workflow must not accept mutable short-SHA image tags" >&2
  exit 1
fi
grep -Fq "printf 'sub2api-deploy-v2 %s\\n%s\\n%s\\n%s\\n'" "$CANARY_WORKFLOW"
grep -Fq '"$AFFILIATE_REFUND_REVERSAL_STAGE" |' "$CANARY_WORKFLOW"
grep -Fq 'STABLE_KNOWN_HOSTS: ${{ secrets.SHANGHAI_SSH_KNOWN_HOSTS }}' "$CANARY_WORKFLOW"
grep -Fq '[[ -z "$STABLE_KNOWN_HOSTS" ]]' "$CANARY_WORKFLOW"
grep -Fq 'ssh-keygen -l -f "$known_hosts_file"' "$CANARY_WORKFLOW"
grep -Fq 'StrictHostKeyChecking=yes' "$CANARY_WORKFLOW"
if grep -Fq 'StrictHostKeyChecking=accept-new' "$CANARY_WORKFLOW"; then
  echo "Production canary workflow must pin the SSH host key" >&2
  exit 1
fi
grep -Fq 'readonly DEPLOY_PROTOCOL="sub2api-deploy-v2"' "$STABLE_SCRIPT"
grep -Fq 'readonly STATE_DIR="/var/lib/sub2api-deploy"' "$STABLE_SCRIPT"
grep -Fq 'affiliate_refund_reversal_state=' "$STABLE_SCRIPT"
grep -Fq 'validate_secure_state_directory' "$STABLE_SCRIPT"
grep -Fq 'validate_secure_state_file' "$STABLE_SCRIPT"
grep -Fq 'must be provisioned by the reviewed bootstrap command' "$STABLE_SCRIPT"
grep -Fq -- '--operator-confirm-database-has-no-affiliate-reversals' "$BOOTSTRAP_SCRIPT"
grep -Fq 'This command does not query the production database' "$BOOTSTRAP_SCRIPT"
grep -Fq 'deployment cannot clear or overwrite pending state' "$DEPLOYMENT_CONTRACT_DOC"
grep -Fq 'Do not recreate `absent` state during incident handling' "$DEPLOYMENT_CONTRACT_DOC"
grep -Fq 'candidate_repo_digest=$deploy_image' "$STABLE_SCRIPT"
grep -Fq 'active_repo_digests=$active_repo_digests' "$STABLE_SCRIPT"
grep -Fq 'readonly MINIMUM_GH_VERSION="2.97.0"' "$STABLE_SCRIPT"
grep -Fq 'gh attestation verify' "$STABLE_SCRIPT"
grep -Fq -- '--bundle-from-oci' "$STABLE_SCRIPT"
grep -Fq -- '--predicate-type https://slsa.dev/provenance/v1' "$STABLE_SCRIPT"
grep -Fq -- '--cert-identity "$ATTESTATION_CERT_IDENTITY"' "$STABLE_SCRIPT"
grep -Fq -- '--cert-oidc-issuer https://token.actions.githubusercontent.com' "$STABLE_SCRIPT"
grep -Fq -- '--source-ref refs/heads/main' "$STABLE_SCRIPT"
grep -Fq -- '--source-digest "$candidate_revision"' "$STABLE_SCRIPT"
grep -Fq -- '--deny-self-hosted-runners' "$STABLE_SCRIPT"
grep -Fq 'readonly CUTOVER_SCRIPT_PROTOCOL="sub2api-nginx-bluegreen-cutover-v2"' "$STABLE_SCRIPT"
grep -Fq 'Blue-green cutover script does not match the reviewed repository version' "$STABLE_SCRIPT"
expected_cutover_hash=$(sed -n 's/^readonly CUTOVER_SCRIPT_SHA256="\([0-9a-f]\{64\}\)"$/\1/p' "$STABLE_SCRIPT")
actual_cutover_hash=$(sha256_file "$CUTOVER_SCRIPT")
[[ -n "$expected_cutover_hash" && "$expected_cutover_hash" == "$actual_cutover_hash" ]]
grep -Fq 'AFFILIATE_REFUND_REVERSAL=1' "$CAPABILITY_MANIFEST"
grep -Fq 'PAYMENT_REVERSAL_COMPONENTS=0' "$CAPABILITY_MANIFEST"
grep -Fq 'github.event_name }}" = "workflow_run"' "$IMAGE_WORKFLOW"
grep -Fq 'affiliate_refund_reversal_capability=0' "$IMAGE_WORKFLOW"
grep -Fq 'affiliate_refund_reversal_capability=1' "$IMAGE_WORKFLOW"
grep -Fq 'payment_reversal_components_capability=0' "$IMAGE_WORKFLOW"
grep -Fq 'payment_reversal_components_capability=1' "$IMAGE_WORKFLOW"
grep -Fq 'trusted_release=false' "$IMAGE_WORKFLOW"
grep -Fq 'trusted_ci_label=0' "$IMAGE_WORKFLOW"
grep -Fq 'trusted_ci_label=1' "$IMAGE_WORKFLOW"
grep -Fq 'manual-${short_sha}-${GITHUB_RUN_ID}' "$IMAGE_WORKFLOW"
grep -Fq "steps.source.outputs.trusted_release == 'true'" "$IMAGE_WORKFLOW"
grep -Fq 'org.sub2api.build.trusted-ci=${{ steps.source.outputs.trusted_ci_label }}' "$IMAGE_WORKFLOW"
grep -Fq 'org.sub2api.capability.affiliate-refund-reversal=${{ steps.source.outputs.affiliate_refund_reversal_capability }}' "$IMAGE_WORKFLOW"
grep -Fq 'org.sub2api.capability.payment-reversal-components=${{ steps.source.outputs.payment_reversal_components_capability }}' "$IMAGE_WORKFLOW"
grep -Fq 'legacy workers claim only `pending`' "$DEPLOYMENT_CONTRACT_DOC"
grep -Fq 'id-token: write' "$IMAGE_WORKFLOW"
grep -Fq 'attestations: write' "$IMAGE_WORKFLOW"
grep -Fq 'uses: actions/attest@1e69f48acb82d1966a394da916b4c1698aa569d6 # v4.2.2' "$IMAGE_WORKFLOW"
if grep -E '^[[:space:]]*uses:' "$IMAGE_WORKFLOW" |
  grep -Ev '@[0-9a-f]{40}([[:space:]]+#.*)?$' >/dev/null; then
  echo "Every image workflow action must be pinned to a reviewed full commit SHA" >&2
  exit 1
fi
grep -Fq 'subject-digest: ${{ steps.image.outputs.digest }}' "$IMAGE_WORKFLOW"
grep -Fq 'push-to-registry: true' "$IMAGE_WORKFLOW"
grep -Fq 'id: image' "$IMAGE_WORKFLOW"
grep -Fq 'steps.image.outputs.digest' "$IMAGE_WORKFLOW"
grep -Fq 'immutable_image="${IMAGE_NAME}@${image_digest}"' "$IMAGE_WORKFLOW"
grep -Fq 'refusing to move it to ${source_sha}' "$IMAGE_WORKFLOW"
if grep -Fq -- '-F force=true' "$IMAGE_WORKFLOW"; then
  echo "Version release tags must never be moved with force" >&2
  exit 1
fi
if grep -Fq 'org.sub2api.capability.affiliate-refund-reversal=1' "$IMAGE_WORKFLOW"; then
  echo "Capability label must not be granted unconditionally to manual ref builds" >&2
  exit 1
fi
if grep -Fq 'org.sub2api.capability.payment-reversal-components=1' "$IMAGE_WORKFLOW"; then
  echo "Payment writer capability must not be granted unconditionally to manual ref builds" >&2
  exit 1
fi
grep -Fq -- '- AFFILIATE_REFUND_REVERSAL_ENABLED=${AFFILIATE_REFUND_REVERSAL_ENABLED:-false}' "$REPO_ROOT/deploy/docker-compose.yml"
grep -Fq 'AFFILIATE_REFUND_REVERSAL_ENABLED: "${affiliate_refund_reversal_enabled}"' "$STABLE_SCRIPT"
grep -Fq 'candidate_image_id' "$STABLE_SCRIPT"
grep -Fq 'candidate_revision' "$STABLE_SCRIPT"
grep -Fq 'candidate_version' "$STABLE_SCRIPT"
grep -Fq 'docker run --rm --entrypoint /app/sub2api' "$STABLE_SCRIPT"
grep -Fq 'releases/tags/${release_tag}' "$STABLE_SCRIPT"
grep -Fq 'PAYMENT_REVERSAL_COMPONENTS_CAPABILITY_LABEL' "$STABLE_SCRIPT"
grep -Fq 'write_payment_reversal_components_state pending' "$STABLE_SCRIPT"
grep -Fq 'candidate_trusted_ci' "$STABLE_SCRIPT"
grep -Fq 'old_slot_stopped=true' "$STABLE_SCRIPT"
grep -Fq 'mv -f -- "$AFFILIATE_REVERSAL_CONTRACT_PENDING_FILE" "$AFFILIATE_REVERSAL_CONTRACT_FILE"' "$STABLE_SCRIPT"

grep -Fq '**Canada** is the only active Pipixia application host' "$AGENT_RULES"
grep -Fq '**United States** is an edge relay only' "$AGENT_RULES"
grep -Fq '**France** is disaster recovery only' "$AGENT_RULES"

bash "$CONTRACT_TEST"
bash "$CUTOVER_TEST"

echo "production deployment policy tests passed"
