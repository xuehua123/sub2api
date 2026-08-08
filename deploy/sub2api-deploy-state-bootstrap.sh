#!/usr/bin/env bash

set -Eeuo pipefail

umask 077

readonly STATE_DIR="/var/lib/sub2api-deploy"
readonly STATE_OWNER_UID="0"
readonly STATE_OWNER_GID="0"
readonly AFFILIATE_REVERSAL_STATE_FILE="${STATE_DIR}/affiliate-refund-reversal-state"
readonly AFFILIATE_REVERSAL_CONTRACT_FILE="${STATE_DIR}/affiliate-refund-reversal-contract"
readonly AFFILIATE_REVERSAL_CONTRACT_PENDING_FILE="${AFFILIATE_REVERSAL_CONTRACT_FILE}.pending"
readonly PAYMENT_REVERSAL_COMPONENTS_STATE_FILE="${STATE_DIR}/payment-reversal-components-state"
readonly PAYMENT_REVERSAL_COMPONENTS_CONTRACT_FILE="${STATE_DIR}/payment-reversal-components-contract"
readonly PAYMENT_REVERSAL_COMPONENTS_CONTRACT_PENDING_FILE="${PAYMENT_REVERSAL_COMPONENTS_CONTRACT_FILE}.pending"
readonly DATABASE_CONFIRMATION="--operator-confirm-database-has-no-affiliate-reversals"

if [[ "$#" -ne 1 || "$1" != "$DATABASE_CONFIRMATION" ]]; then
  echo "Usage: $0 ${DATABASE_CONFIRMATION}" >&2
  echo "This command does not query the production database. Before running it, an operator must verify that no affiliate reversal has ever been persisted (including non-zero reversed_amount or equivalent reversal ledger state)." >&2
  exit 2
fi

if [[ "$(id -u)" != "$STATE_OWNER_UID" || "$(id -g)" != "$STATE_OWNER_GID" ]]; then
  echo "Deployment state bootstrap must run as ${STATE_OWNER_UID}:${STATE_OWNER_GID}" >&2
  exit 1
fi

command -v docker >/dev/null 2>&1 || { echo "Docker is required to inspect existing Sub2API slots" >&2; exit 1; }

for container in sub2api-blue sub2api-green; do
  if [[ "$(docker inspect --format '{{.State.Running}}' "$container" 2>/dev/null || true)" != true ]]; then
    continue
  fi

  gate_count=0
  gate_value=""
  while IFS= read -r env_line; do
    case "$env_line" in
      AFFILIATE_REFUND_REVERSAL_ENABLED=*)
        gate_count=$((gate_count + 1))
        gate_value=${env_line#AFFILIATE_REFUND_REVERSAL_ENABLED=}
        ;;
    esac
  done < <(docker inspect --format '{{range .Config.Env}}{{println .}}{{end}}' "$container")

  if [[ "$gate_count" -gt 1 || "$gate_value" == true ]]; then
    echo "Refusing to bootstrap absent state while $container may have affiliate refund reversal enabled" >&2
    exit 1
  fi

  active_image_id="$(docker inspect --format '{{.Image}}' "$container")"
  payment_components_capability="$(docker image inspect --format '{{ index .Config.Labels "org.sub2api.capability.payment-reversal-components" }}' "$active_image_id")"
  case "$payment_components_capability" in
    1)
      echo "Refusing to bootstrap absent state while $container has payment reversal components enabled" >&2
      exit 1
      ;;
    0|""|"<no value>") ;;
    *)
      echo "Refusing to bootstrap with an ambiguous payment reversal component capability on $container" >&2
      exit 1
      ;;
  esac
done

if [[ -L "$STATE_DIR" || ( -e "$STATE_DIR" && ! -d "$STATE_DIR" ) ]]; then
  echo "Deployment state path is unsafe: $STATE_DIR" >&2
  exit 1
fi
if [[ ! -e "$STATE_DIR" ]]; then
  install -d -o "$STATE_OWNER_UID" -g "$STATE_OWNER_GID" -m 0700 -- "$STATE_DIR"
fi
if [[ "$(stat -c '%u:%g:%a' -- "$STATE_DIR")" != "${STATE_OWNER_UID}:${STATE_OWNER_GID}:700" ]]; then
  echo "Deployment state directory must be owned by ${STATE_OWNER_UID}:${STATE_OWNER_GID} with mode 0700: $STATE_DIR" >&2
  exit 1
fi

if [[ -e "$AFFILIATE_REVERSAL_STATE_FILE" || -L "$AFFILIATE_REVERSAL_STATE_FILE" ||
  -e "$AFFILIATE_REVERSAL_CONTRACT_FILE" || -L "$AFFILIATE_REVERSAL_CONTRACT_FILE" ||
  -e "$AFFILIATE_REVERSAL_CONTRACT_PENDING_FILE" || -L "$AFFILIATE_REVERSAL_CONTRACT_PENDING_FILE" ||
  -e "$PAYMENT_REVERSAL_COMPONENTS_STATE_FILE" || -L "$PAYMENT_REVERSAL_COMPONENTS_STATE_FILE" ||
  -e "$PAYMENT_REVERSAL_COMPONENTS_CONTRACT_FILE" || -L "$PAYMENT_REVERSAL_COMPONENTS_CONTRACT_FILE" ||
  -e "$PAYMENT_REVERSAL_COMPONENTS_CONTRACT_PENDING_FILE" || -L "$PAYMENT_REVERSAL_COMPONENTS_CONTRACT_PENDING_FILE" ]]; then
  echo "Deployment state already exists; bootstrap is a one-time operation and will not overwrite it" >&2
  exit 1
fi

state_temp="$(mktemp "${STATE_DIR}/affiliate-refund-reversal-state.bootstrap.XXXXXX")"
payment_state_temp=""
cleanup() {
  rm -f -- "$state_temp"
  if [[ -n "$payment_state_temp" ]]; then
    rm -f -- "$payment_state_temp"
  fi
}
trap cleanup EXIT
payment_state_temp="$(mktemp "${STATE_DIR}/payment-reversal-components-state.bootstrap.XXXXXX")"

{
  echo "state_version=1"
  echo "affiliate_refund_reversal_state=absent"
  echo "updated_at=$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
} > "$state_temp"
{
  echo "state_version=1"
  echo "payment_reversal_components_state=absent"
  echo "updated_at=$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
} > "$payment_state_temp"
chmod 600 "$state_temp"
chmod 600 "$payment_state_temp"
sync -f "$state_temp"
sync -f "$payment_state_temp"
mv -- "$state_temp" "$AFFILIATE_REVERSAL_STATE_FILE"
mv -- "$payment_state_temp" "$PAYMENT_REVERSAL_COMPONENTS_STATE_FILE"
sync -f "$STATE_DIR"

trap - EXIT
echo "Deployment state bootstrapped as absent after explicit external database precheck confirmation."
