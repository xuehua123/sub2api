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
readonly BOOTSTRAP_PENDING_FILE="${STATE_DIR}/state-bootstrap.pending"
readonly LOCK_FILE="${STATE_DIR}/stable-deploy.lock"
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
command -v flock >/dev/null 2>&1 || { echo "flock is required to serialize deployment state bootstrap" >&2; exit 1; }
command -v jq >/dev/null 2>&1 || { echo "jq is required to inspect existing Sub2API slots safely" >&2; exit 1; }

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
if [[ -L "$LOCK_FILE" || ( -e "$LOCK_FILE" && ! -f "$LOCK_FILE" ) ]]; then
  echo "Stable deployment lock path is unsafe: $LOCK_FILE" >&2
  exit 1
fi
if [[ ! -e "$LOCK_FILE" ]]; then
  (set -o noclobber; : > "$LOCK_FILE") 2>/dev/null || true
fi
if [[ -L "$LOCK_FILE" || ! -f "$LOCK_FILE" ]] ||
  [[ "$(stat -c '%u:%g:%a' -- "$LOCK_FILE")" != "${STATE_OWNER_UID}:${STATE_OWNER_GID}:600" ]]; then
  echo "Stable deployment lock must be owned by ${STATE_OWNER_UID}:${STATE_OWNER_GID} with mode 0600" >&2
  exit 1
fi
exec 9>>"$LOCK_FILE"
flock -n 9 || { echo "Another stable deployment or state bootstrap is already running" >&2; exit 1; }

for container in sub2api-blue sub2api-green; do
  container_inspect=""
  if ! container_inspect="$(docker container inspect "$container")"; then
    echo "Unable to inspect required stable slot: $container" >&2
    exit 1
  fi
  running_state="$(jq -er 'if length == 1 and (.[0].State.Running | type) == "boolean" then (if .[0].State.Running then "true" else "false" end) else error("invalid running state") end' <<< "$container_inspect")" || {
    echo "Unable to determine running state for stable slot: $container" >&2
    exit 1
  }
  if [[ "$running_state" != true ]]; then
    continue
  fi

  gate_safety="$(jq -er '
    (.[0].Config.Env // []) as $env |
    if (($env | type) != "array" or any($env[]; (type != "string"))) then
      error("invalid container environment")
    else . end |
    [$env[] | select(startswith("AFFILIATE_REFUND_REVERSAL_ENABLED="))] as $matches |
    if ($matches | length) == 1 and $matches[0] == "AFFILIATE_REFUND_REVERSAL_ENABLED=false" then "false"
    else "unsafe"
    end
  ' <<< "$container_inspect")" || {
    echo "Unable to inspect the affiliate reversal gate on $container" >&2
    exit 1
  }
  if [[ "$gate_safety" != false ]]; then
    echo "Refusing to bootstrap absent state while $container may have affiliate refund reversal enabled" >&2
    exit 1
  fi

  active_image_id="$(jq -er '.[0].Image | select(type == "string" and length > 0)' <<< "$container_inspect")" || {
    echo "Unable to determine the image for running stable slot: $container" >&2
    exit 1
  }
  image_inspect=""
  if ! image_inspect="$(docker image inspect "$active_image_id")"; then
    echo "Unable to inspect the image for running stable slot: $container" >&2
    exit 1
  fi
  payment_components_capability="$(jq -er '.[0].Config.Labels["org.sub2api.capability.payment-reversal-components"] // ""' <<< "$image_inspect")" || {
    echo "Unable to inspect payment reversal capability for running stable slot: $container" >&2
    exit 1
  }
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

if [[ -e "$AFFILIATE_REVERSAL_CONTRACT_FILE" || -L "$AFFILIATE_REVERSAL_CONTRACT_FILE" ||
  -e "$AFFILIATE_REVERSAL_CONTRACT_PENDING_FILE" || -L "$AFFILIATE_REVERSAL_CONTRACT_PENDING_FILE" ||
  -e "$PAYMENT_REVERSAL_COMPONENTS_CONTRACT_FILE" || -L "$PAYMENT_REVERSAL_COMPONENTS_CONTRACT_FILE" ||
  -e "$PAYMENT_REVERSAL_COMPONENTS_CONTRACT_PENDING_FILE" || -L "$PAYMENT_REVERSAL_COMPONENTS_CONTRACT_PENDING_FILE" ]]; then
  echo "Deployment contract state already exists; bootstrap will not overwrite it" >&2
  exit 1
fi

validate_existing_absent_state() {
  local path=$1
  local state_key=$2
  if [[ -L "$path" || ! -f "$path" ]] ||
    [[ "$(stat -c '%u:%g:%a' -- "$path")" != "${STATE_OWNER_UID}:${STATE_OWNER_GID}:600" ]] ||
    [[ "$(wc -l < "$path")" -ne 3 ]] ||
    [[ "$(grep -Fxc 'state_version=1' "$path")" -ne 1 ]] ||
    [[ "$(grep -Fxc "${state_key}=absent" "$path")" -ne 1 ]] ||
    [[ "$(grep -Ec '^updated_at=[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$' "$path")" -ne 1 ]]; then
    echo "Existing bootstrap state is invalid and requires operator review: $path" >&2
    return 1
  fi
}

affiliate_state_exists=false
payment_state_exists=false
if [[ -e "$AFFILIATE_REVERSAL_STATE_FILE" || -L "$AFFILIATE_REVERSAL_STATE_FILE" ]]; then
  validate_existing_absent_state "$AFFILIATE_REVERSAL_STATE_FILE" affiliate_refund_reversal_state
  affiliate_state_exists=true
fi
if [[ -e "$PAYMENT_REVERSAL_COMPONENTS_STATE_FILE" || -L "$PAYMENT_REVERSAL_COMPONENTS_STATE_FILE" ]]; then
  validate_existing_absent_state "$PAYMENT_REVERSAL_COMPONENTS_STATE_FILE" payment_reversal_components_state
  payment_state_exists=true
fi

state_temp=""
payment_state_temp=""
pending_temp=""
cleanup() {
  if [[ -n "$state_temp" ]]; then
    rm -f -- "$state_temp"
  fi
  if [[ -n "$payment_state_temp" ]]; then
    rm -f -- "$payment_state_temp"
  fi
  if [[ -n "$pending_temp" ]]; then
    rm -f -- "$pending_temp"
  fi
}
trap cleanup EXIT

pending_exists=false
if [[ -e "$BOOTSTRAP_PENDING_FILE" || -L "$BOOTSTRAP_PENDING_FILE" ]]; then
  pending_exists=true
fi

bootstrap_version=""
bootstrap_transaction_id=""
bootstrap_updated_at=""
bootstrap_affiliate_sha256=""
bootstrap_payment_sha256=""

load_bootstrap_pending() {
  local key
  local value
  local version_seen=false
  local transaction_seen=false
  local updated_at_seen=false
  local affiliate_hash_seen=false
  local payment_hash_seen=false

  if [[ -L "$BOOTSTRAP_PENDING_FILE" || ! -f "$BOOTSTRAP_PENDING_FILE" ]] ||
    [[ "$(stat -c '%u:%g:%a' -- "$BOOTSTRAP_PENDING_FILE")" != "${STATE_OWNER_UID}:${STATE_OWNER_GID}:600" ]]; then
    echo "Bootstrap transaction journal is unsafe: $BOOTSTRAP_PENDING_FILE" >&2
    return 1
  fi
  while IFS='=' read -r key value; do
    case "$key" in
      bootstrap_version)
        [[ "$version_seen" == false ]] || return 1
        version_seen=true
        bootstrap_version=$value
        ;;
      transaction_id)
        [[ "$transaction_seen" == false ]] || return 1
        transaction_seen=true
        bootstrap_transaction_id=$value
        ;;
      updated_at)
        [[ "$updated_at_seen" == false ]] || return 1
        updated_at_seen=true
        bootstrap_updated_at=$value
        ;;
      affiliate_sha256)
        [[ "$affiliate_hash_seen" == false ]] || return 1
        affiliate_hash_seen=true
        bootstrap_affiliate_sha256=$value
        ;;
      payment_sha256)
        [[ "$payment_hash_seen" == false ]] || return 1
        payment_hash_seen=true
        bootstrap_payment_sha256=$value
        ;;
      *)
        echo "Unknown field in bootstrap transaction journal: $key" >&2
        return 1
        ;;
    esac
  done < "$BOOTSTRAP_PENDING_FILE"
  if [[ "$bootstrap_version" != 1 || ! "$bootstrap_transaction_id" =~ ^[0-9a-f]{32}$ ||
    ! "$bootstrap_updated_at" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$ ||
    ! "$bootstrap_affiliate_sha256" =~ ^[0-9a-f]{64}$ ||
    ! "$bootstrap_payment_sha256" =~ ^[0-9a-f]{64}$ ]]; then
    echo "Bootstrap transaction journal is invalid or incomplete" >&2
    return 1
  fi
}

write_expected_state_temps() {
  local updated_at=$1
  state_temp="$(mktemp "${STATE_DIR}/affiliate-refund-reversal-state.bootstrap.XXXXXX")"
  payment_state_temp="$(mktemp "${STATE_DIR}/payment-reversal-components-state.bootstrap.XXXXXX")"
  {
    echo "state_version=1"
    echo "affiliate_refund_reversal_state=absent"
    echo "updated_at=${updated_at}"
  } > "$state_temp"
  {
    echo "state_version=1"
    echo "payment_reversal_components_state=absent"
    echo "updated_at=${updated_at}"
  } > "$payment_state_temp"
  chmod 600 "$state_temp" "$payment_state_temp"
  sync -f "$state_temp"
  sync -f "$payment_state_temp"
}

if [[ "$pending_exists" == true ]]; then
  load_bootstrap_pending
  write_expected_state_temps "$bootstrap_updated_at"
  [[ "$(sha256sum -- "$state_temp" | awk '{print $1}')" == "$bootstrap_affiliate_sha256" ]] || {
    echo "Bootstrap affiliate state does not match its transaction journal" >&2
    exit 1
  }
  [[ "$(sha256sum -- "$payment_state_temp" | awk '{print $1}')" == "$bootstrap_payment_sha256" ]] || {
    echo "Bootstrap payment state does not match its transaction journal" >&2
    exit 1
  }
  if [[ "$affiliate_state_exists" == true ]]; then
    [[ "$(sha256sum -- "$AFFILIATE_REVERSAL_STATE_FILE" | awk '{print $1}')" == "$bootstrap_affiliate_sha256" ]] || {
      echo "Existing affiliate state does not match the pending bootstrap transaction" >&2
      exit 1
    }
  else
    ln -- "$state_temp" "$AFFILIATE_REVERSAL_STATE_FILE"
  fi
  if [[ "$payment_state_exists" == true ]]; then
    [[ "$(sha256sum -- "$PAYMENT_REVERSAL_COMPONENTS_STATE_FILE" | awk '{print $1}')" == "$bootstrap_payment_sha256" ]] || {
      echo "Existing payment state does not match the pending bootstrap transaction" >&2
      exit 1
    }
  else
    ln -- "$payment_state_temp" "$PAYMENT_REVERSAL_COMPONENTS_STATE_FILE"
  fi
else
  if [[ "$affiliate_state_exists" == true || "$payment_state_exists" == true ]]; then
    echo "Deployment state is incomplete without a matching bootstrap transaction; operator review is required" >&2
    exit 1
  fi
  bootstrap_updated_at="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  bootstrap_transaction_id="$(head -c 16 /dev/urandom | od -An -tx1 | tr -d ' \n')"
  write_expected_state_temps "$bootstrap_updated_at"
  bootstrap_affiliate_sha256="$(sha256sum -- "$state_temp" | awk '{print $1}')"
  bootstrap_payment_sha256="$(sha256sum -- "$payment_state_temp" | awk '{print $1}')"
  pending_temp="$(mktemp "${STATE_DIR}/state-bootstrap.pending.XXXXXX")"
  {
    echo "bootstrap_version=1"
    echo "transaction_id=${bootstrap_transaction_id}"
    echo "updated_at=${bootstrap_updated_at}"
    echo "affiliate_sha256=${bootstrap_affiliate_sha256}"
    echo "payment_sha256=${bootstrap_payment_sha256}"
  } > "$pending_temp"
  chmod 600 "$pending_temp"
  sync -f "$pending_temp"
  ln -- "$pending_temp" "$BOOTSTRAP_PENDING_FILE"
  sync -f "$STATE_DIR"
  ln -- "$state_temp" "$AFFILIATE_REVERSAL_STATE_FILE"
  ln -- "$payment_state_temp" "$PAYMENT_REVERSAL_COMPONENTS_STATE_FILE"
fi

validate_existing_absent_state "$AFFILIATE_REVERSAL_STATE_FILE" affiliate_refund_reversal_state
validate_existing_absent_state "$PAYMENT_REVERSAL_COMPONENTS_STATE_FILE" payment_reversal_components_state
[[ "$(sha256sum -- "$AFFILIATE_REVERSAL_STATE_FILE" | awk '{print $1}')" == "$bootstrap_affiliate_sha256" ]]
[[ "$(sha256sum -- "$PAYMENT_REVERSAL_COMPONENTS_STATE_FILE" | awk '{print $1}')" == "$bootstrap_payment_sha256" ]]
sync -f "$STATE_DIR"
rm -f -- "$BOOTSTRAP_PENDING_FILE"
sync -f "$STATE_DIR"

trap - EXIT
cleanup
echo "Deployment state bootstrapped as absent after explicit external database precheck confirmation."
