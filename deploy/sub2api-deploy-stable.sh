#!/usr/bin/env bash

set -Eeuo pipefail

umask 077

readonly IMAGE_NAME="ghcr.io/xuehua123/sub2api"
readonly DEPLOY_DIR="/srv/sub2api-migration/incoming"
readonly STATE_DIR="/var/lib/sub2api-deploy"
readonly STATE_OWNER_UID="0"
readonly STATE_OWNER_GID="0"
readonly COMPOSE_FILE="${DEPLOY_DIR}/docker-compose.migration.yml"
readonly COMPOSE_ENV_FILE="${DEPLOY_DIR}/sub2api.env"
readonly NGINX_CONFIG="/etc/nginx/snippets/sub2api-active-upstream.conf"
readonly CUTOVER_SCRIPT="/usr/local/sbin/sub2api-nginx-bluegreen-cutover"
readonly CUTOVER_SCRIPT_PROTOCOL="sub2api-nginx-bluegreen-cutover-v2"
readonly CUTOVER_SCRIPT_SHA256="1d4f8b7b66d09e6c6968fc20173e58d46ed8967ddcc92eddf164b005e408b1e4"
readonly CUTOVER_OWNER_UID="0"
readonly CUTOVER_OWNER_GID="0"
readonly LOCK_FILE="${STATE_DIR}/stable-deploy.lock"
readonly PUBLIC_HEALTH_URL="https://api.wenrugouai.com/health"
readonly PUBLIC_HEALTH_RESOLVE="api.wenrugouai.com:443:40.160.58.167"
readonly NGINX_PID_FILE="/run/nginx.pid"
readonly DEPLOY_PROTOCOL="sub2api-deploy-v2"
readonly TRUSTED_CI_IMAGE_LABEL="org.sub2api.build.trusted-ci"
readonly AFFILIATE_REVERSAL_CAPABILITY_LABEL="org.sub2api.capability.affiliate-refund-reversal"
readonly PAYMENT_REVERSAL_COMPONENTS_CAPABILITY_LABEL="org.sub2api.capability.payment-reversal-components"
readonly ATTESTATION_REPOSITORY="xuehua123/sub2api"
readonly ATTESTATION_CERT_IDENTITY="https://github.com/xuehua123/sub2api/.github/workflows/docker-image.yml@refs/heads/main"
readonly MINIMUM_GH_VERSION="2.97.0"
readonly AFFILIATE_REVERSAL_STATE_FILE="${STATE_DIR}/affiliate-refund-reversal-state"
readonly AFFILIATE_REVERSAL_CONTRACT_FILE="${STATE_DIR}/affiliate-refund-reversal-contract"
readonly AFFILIATE_REVERSAL_CONTRACT_PENDING_FILE="${AFFILIATE_REVERSAL_CONTRACT_FILE}.pending"
readonly PAYMENT_REVERSAL_COMPONENTS_STATE_FILE="${STATE_DIR}/payment-reversal-components-state"
readonly PAYMENT_REVERSAL_COMPONENTS_CONTRACT_FILE="${STATE_DIR}/payment-reversal-components-contract"
readonly PAYMENT_REVERSAL_COMPONENTS_CONTRACT_PENDING_FILE="${PAYMENT_REVERSAL_COMPONENTS_CONTRACT_FILE}.pending"
readonly BOOTSTRAP_PENDING_FILE="${STATE_DIR}/state-bootstrap.pending"

if ! IFS= read -r deploy_request || [[ -z "$deploy_request" ]]; then
  echo "A versioned deployment request must be provided on stdin" >&2
  exit 2
fi

case "$deploy_request" in
  "${DEPLOY_PROTOCOL} "*)
    deploy_image_tag=${deploy_request#"${DEPLOY_PROTOCOL} "}
    ;;
  *)
    echo "Unsupported deployment protocol; expected ${DEPLOY_PROTOCOL}" >&2
    exit 2
    ;;
esac

if [[ -z "$deploy_image_tag" ]]; then
  echo "An immutable image tag must be provided in the deployment request" >&2
  exit 2
fi

if [[ "$deploy_image_tag" =~ ^sha256:[0-9a-f]{64}$ ]]; then
  deploy_image="${IMAGE_NAME}@${deploy_image_tag}"
elif [[ "$deploy_image_tag" =~ ^ghcr\.io/xuehua123/sub2api@sha256:[0-9a-f]{64}$ ]]; then
  deploy_image="$deploy_image_tag"
else
  echo "Only an exact ${IMAGE_NAME}@sha256 digest is allowed" >&2
  exit 2
fi

if ! IFS= read -r ghcr_username || [[ -z "$ghcr_username" ]]; then
  echo "A GHCR username must be provided on stdin" >&2
  exit 2
fi

if ! IFS= read -r ghcr_token || [[ -z "$ghcr_token" ]]; then
  echo "A GHCR token must be provided on stdin" >&2
  exit 2
fi

if ! IFS= read -r affiliate_refund_reversal_stage || [[ -z "$affiliate_refund_reversal_stage" ]]; then
  echo "An affiliate refund reversal stage must be provided on stdin" >&2
  exit 2
fi

unexpected_payload_line=""
if IFS= read -r unexpected_payload_line || [[ -n "$unexpected_payload_line" ]]; then
  echo "Unexpected trailing data in deployment request" >&2
  exit 2
fi

case "$affiliate_refund_reversal_stage" in
  disabled)
    affiliate_refund_reversal_enabled=false
    ;;
  enabled)
    affiliate_refund_reversal_enabled=true
    ;;
  *)
    echo "Affiliate refund reversal stage must be disabled or enabled" >&2
    exit 2
    ;;
esac

if [[ "$(id -u)" != "$STATE_OWNER_UID" || "$(id -g)" != "$STATE_OWNER_GID" ]]; then
  echo "Stable deployment must run as ${STATE_OWNER_UID}:${STATE_OWNER_GID}" >&2
  exit 1
fi

validate_root_owned_nonwritable_directory() {
  local path=$1
  local metadata
  local mode
  local owner_uid
  local owner_gid

  if [[ -L "$path" || ! -d "$path" ]]; then
    echo "Deployment directory is not a regular directory: $path" >&2
    return 1
  fi
  metadata="$(stat -c '%u:%g:%a' -- "$path")"
  IFS=: read -r owner_uid owner_gid mode <<< "$metadata"
  if [[ "$owner_uid" != "$STATE_OWNER_UID" || "$owner_gid" != "$STATE_OWNER_GID" ]] ||
    [[ ! "$mode" =~ ^[0-7]{3,4}$ ]] || (( (8#$mode & 0022) != 0 )); then
    echo "Deployment directory must be root-owned and not group/other writable: $path" >&2
    return 1
  fi
}

validate_root_owned_nonwritable_file() {
  local path=$1
  local metadata
  local mode
  local owner_uid
  local owner_gid

  if [[ -L "$path" || ! -f "$path" ]]; then
    echo "Deployment file is not a regular non-symlink file: $path" >&2
    return 1
  fi
  metadata="$(stat -c '%u:%g:%a' -- "$path")"
  IFS=: read -r owner_uid owner_gid mode <<< "$metadata"
  if [[ "$owner_uid" != "$STATE_OWNER_UID" || "$owner_gid" != "$STATE_OWNER_GID" ]] ||
    [[ ! "$mode" =~ ^[0-7]{3,4}$ ]] || (( (8#$mode & 0022) != 0 )); then
    echo "Deployment file must be root-owned and not group/other writable: $path" >&2
    return 1
  fi
}

validate_root_owned_nonwritable_directory "$DEPLOY_DIR"
validate_root_owned_nonwritable_file "$COMPOSE_FILE"
validate_root_owned_nonwritable_file "$COMPOSE_ENV_FILE"
if [[ "$(stat -c '%u:%g:%a' -- "$COMPOSE_ENV_FILE")" != "${STATE_OWNER_UID}:${STATE_OWNER_GID}:600" ]]; then
  echo "Compose environment file must be root-owned with mode 0600: $COMPOSE_ENV_FILE" >&2
  exit 1
fi
[[ -f "$NGINX_CONFIG" ]] || { echo "Stable nginx config not found: $NGINX_CONFIG" >&2; exit 1; }
[[ -x "$CUTOVER_SCRIPT" ]] || { echo "Blue-green cutover script not found: $CUTOVER_SCRIPT" >&2; exit 1; }
if [[ -L "$CUTOVER_SCRIPT" ]] ||
  [[ "$(stat -c '%u:%g:%a' -- "$CUTOVER_SCRIPT")" != "${CUTOVER_OWNER_UID}:${CUTOVER_OWNER_GID}:755" ]]; then
  echo "Blue-green cutover script must be a root-owned, non-symlink executable with mode 0755" >&2
  exit 1
fi
if [[ "$(sha256sum -- "$CUTOVER_SCRIPT" | awk '{print $1}')" != "$CUTOVER_SCRIPT_SHA256" ]]; then
  echo "Blue-green cutover script does not match the reviewed repository version" >&2
  exit 1
fi
if [[ "$("$CUTOVER_SCRIPT" --protocol)" != "$CUTOVER_SCRIPT_PROTOCOL" ]]; then
  echo "Blue-green cutover script protocol mismatch" >&2
  exit 1
fi
command -v gh >/dev/null 2>&1 || { echo "GitHub CLI with artifact attestation support is required" >&2; exit 1; }
gh_version="$(gh --version | awk 'NR == 1 { print $3 }')"
IFS=. read -r gh_major gh_minor gh_patch <<< "$gh_version"
if [[ ! "$gh_version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] ||
  ((gh_major < 2 || (gh_major == 2 && gh_minor < 97))); then
  echo "GitHub CLI ${MINIMUM_GH_VERSION} or newer is required for secure attestation identity verification" >&2
  exit 1
fi
if ! gh attestation verify --help 2>&1 | grep -F -- '--source-digest' >/dev/null; then
  echo "Installed GitHub CLI does not support required attestation policy flags" >&2
  exit 1
fi

validate_secure_state_directory() {
  if [[ -L "$STATE_DIR" || ! -d "$STATE_DIR" ]]; then
    echo "Deployment state path is not a regular directory: $STATE_DIR" >&2
    return 1
  fi
  if [[ "$(stat -c '%u:%g:%a' -- "$STATE_DIR")" != "${STATE_OWNER_UID}:${STATE_OWNER_GID}:700" ]]; then
    echo "Deployment state directory must be owned by ${STATE_OWNER_UID}:${STATE_OWNER_GID} with mode 0700: $STATE_DIR" >&2
    return 1
  fi
}

validate_secure_state_file() {
  local path=$1
  if [[ -L "$path" || ! -f "$path" ]]; then
    echo "Deployment state is not a regular file: $path" >&2
    return 1
  fi
  if [[ "$(stat -c '%u:%g:%a' -- "$path")" != "${STATE_OWNER_UID}:${STATE_OWNER_GID}:600" ]]; then
    echo "Deployment state file must be owned by ${STATE_OWNER_UID}:${STATE_OWNER_GID} with mode 0600: $path" >&2
    return 1
  fi
}

write_affiliate_reversal_state() {
  local state=$1
  local state_temp

  case "$state" in
    absent|pending|activated) ;;
    *) return 2 ;;
  esac

  state_temp="$(mktemp "${AFFILIATE_REVERSAL_STATE_FILE}.tmp.XXXXXX")"
  {
    echo "state_version=1"
    echo "affiliate_refund_reversal_state=$state"
    echo "updated_at=$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  } > "$state_temp"
  chmod 600 "$state_temp"
  sync -f "$state_temp"
  mv -f -- "$state_temp" "$AFFILIATE_REVERSAL_STATE_FILE"
  sync -f "$STATE_DIR"
}

write_payment_reversal_components_state() {
  local state=$1
  local state_temp

  case "$state" in
    absent|pending|activated) ;;
    *) return 2 ;;
  esac

  state_temp="$(mktemp "${PAYMENT_REVERSAL_COMPONENTS_STATE_FILE}.tmp.XXXXXX")"
  {
    echo "state_version=1"
    echo "payment_reversal_components_state=$state"
    echo "updated_at=$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  } > "$state_temp"
  chmod 600 "$state_temp"
  sync -f "$state_temp"
  mv -f -- "$state_temp" "$PAYMENT_REVERSAL_COMPONENTS_STATE_FILE"
  sync -f "$STATE_DIR"
}

if [[ -L "$STATE_DIR" || ! -d "$STATE_DIR" ]]; then
  echo "Deployment state directory must be provisioned by the reviewed bootstrap command: $STATE_DIR" >&2
  exit 1
fi
validate_secure_state_directory
if [[ -e "$BOOTSTRAP_PENDING_FILE" || -L "$BOOTSTRAP_PENDING_FILE" ]]; then
  echo "Deployment state bootstrap transaction is incomplete; finish reviewed bootstrap recovery before deploying" >&2
  exit 1
fi
if [[ ! -e "$AFFILIATE_REVERSAL_STATE_FILE" && ! -L "$AFFILIATE_REVERSAL_STATE_FILE" ]]; then
  echo "Deployment state directory exists without its monotonic contract state; run the reviewed bootstrap only after the required operator precheck" >&2
  exit 1
fi
validate_secure_state_file "$AFFILIATE_REVERSAL_STATE_FILE"
if [[ ! -e "$PAYMENT_REVERSAL_COMPONENTS_STATE_FILE" && ! -L "$PAYMENT_REVERSAL_COMPONENTS_STATE_FILE" ]]; then
  echo "Deployment state directory exists without its payment reversal component contract state; run the reviewed bootstrap only after the required operator precheck" >&2
  exit 1
fi
validate_secure_state_file "$PAYMENT_REVERSAL_COMPONENTS_STATE_FILE"

if [[ -L "$LOCK_FILE" || ( -e "$LOCK_FILE" && ! -f "$LOCK_FILE" ) ]]; then
  echo "Stable deployment lock path is unsafe: $LOCK_FILE" >&2
  exit 1
fi
if [[ ! -e "$LOCK_FILE" ]]; then
  (set -o noclobber; : > "$LOCK_FILE") 2>/dev/null || true
fi
validate_secure_state_file "$LOCK_FILE"
exec 9>>"$LOCK_FILE"
flock -n 9 || { echo "Another stable deployment is already running" >&2; exit 1; }

persistent_contract_state=""
persistent_state_version=""
persistent_state_version_seen=false
persistent_contract_state_seen=false
load_persistent_contract_state() {
  local key
  local value

  while IFS='=' read -r key value; do
    case "$key" in
      state_version)
        [[ "$persistent_state_version_seen" == false ]] || { echo "Duplicate state_version in deployment state" >&2; return 1; }
        persistent_state_version_seen=true
        persistent_state_version=$value
        ;;
      affiliate_refund_reversal_state)
        [[ "$persistent_contract_state_seen" == false ]] || { echo "Duplicate affiliate_refund_reversal_state in deployment state" >&2; return 1; }
        persistent_contract_state_seen=true
        persistent_contract_state=$value
        ;;
      updated_at)
        ;;
      "")
        ;;
      *)
        echo "Unknown field in deployment state: $key" >&2
        return 1
        ;;
    esac
  done < "$AFFILIATE_REVERSAL_STATE_FILE"

  if [[ "$persistent_state_version" != 1 ]] ||
    [[ ! "$persistent_contract_state" =~ ^(absent|pending|activated)$ ]]; then
    echo "Deployment contract state is invalid or incomplete" >&2
    return 1
  fi
}

load_persistent_contract_state

contract_state=absent
contract_version=""
contract_stage=""
contract_image_id=""
contract_revision=""
contract_capability_label=""
contract_capability_value=""
contract_version_seen=false
contract_state_seen=false
contract_stage_seen=false
contract_image_id_seen=false
contract_revision_seen=false
contract_capability_label_seen=false
contract_capability_value_seen=false

load_affiliate_reversal_contract() {
  local key
  local value

  validate_secure_state_file "$AFFILIATE_REVERSAL_CONTRACT_FILE"

  while IFS='=' read -r key value; do
    case "$key" in
      contract_version)
        [[ "$contract_version_seen" == false ]] || { echo "Duplicate contract_version in affiliate refund reversal contract" >&2; return 1; }
        contract_version_seen=true
        contract_version=$value
        ;;
      state)
        [[ "$contract_state_seen" == false ]] || { echo "Duplicate state in affiliate refund reversal contract" >&2; return 1; }
        contract_state_seen=true
        contract_state=$value
        ;;
      stage)
        [[ "$contract_stage_seen" == false ]] || { echo "Duplicate stage in affiliate refund reversal contract" >&2; return 1; }
        contract_stage_seen=true
        contract_stage=$value
        ;;
      image_id)
        [[ "$contract_image_id_seen" == false ]] || { echo "Duplicate image_id in affiliate refund reversal contract" >&2; return 1; }
        contract_image_id_seen=true
        contract_image_id=$value
        ;;
      revision)
        [[ "$contract_revision_seen" == false ]] || { echo "Duplicate revision in affiliate refund reversal contract" >&2; return 1; }
        contract_revision_seen=true
        contract_revision=$value
        ;;
      capability_label)
        [[ "$contract_capability_label_seen" == false ]] || { echo "Duplicate capability_label in affiliate refund reversal contract" >&2; return 1; }
        contract_capability_label_seen=true
        contract_capability_label=$value
        ;;
      capability_value)
        [[ "$contract_capability_value_seen" == false ]] || { echo "Duplicate capability_value in affiliate refund reversal contract" >&2; return 1; }
        contract_capability_value_seen=true
        contract_capability_value=$value
        ;;
      prepared_at)
        ;;
      activated_at)
        ;;
      "")
        ;;
      *)
        echo "Unknown field in affiliate refund reversal contract: $key" >&2
        return 1
        ;;
    esac
  done < "$AFFILIATE_REVERSAL_CONTRACT_FILE"

  if [[ "$contract_version" != 1 || "$contract_state" != activated || "$contract_stage" != enabled ||
    "$contract_capability_label" != "$AFFILIATE_REVERSAL_CAPABILITY_LABEL" || "$contract_capability_value" != 1 ||
    ! "$contract_image_id" =~ ^sha256:[0-9a-f]{64}$ || ! "$contract_revision" =~ ^[0-9a-f]{40,64}$ ]]; then
    echo "Affiliate refund reversal contract is invalid or incomplete" >&2
    return 1
  fi
}

case "$persistent_contract_state" in
  absent)
    if [[ -e "$AFFILIATE_REVERSAL_CONTRACT_FILE" || -L "$AFFILIATE_REVERSAL_CONTRACT_FILE" ||
      -e "$AFFILIATE_REVERSAL_CONTRACT_PENDING_FILE" || -L "$AFFILIATE_REVERSAL_CONTRACT_PENDING_FILE" ]]; then
      echo "Absent deployment state conflicts with affiliate refund reversal contract files" >&2
      exit 1
    fi
    ;;
  pending)
    if [[ -e "$AFFILIATE_REVERSAL_CONTRACT_FILE" || -L "$AFFILIATE_REVERSAL_CONTRACT_FILE" ]] ||
      [[ ! -e "$AFFILIATE_REVERSAL_CONTRACT_PENDING_FILE" && ! -L "$AFFILIATE_REVERSAL_CONTRACT_PENDING_FILE" ]]; then
      echo "Pending deployment state conflicts with affiliate refund reversal contract files" >&2
      exit 1
    fi
    validate_secure_state_file "$AFFILIATE_REVERSAL_CONTRACT_PENDING_FILE"
    echo "An incomplete affiliate refund reversal activation requires operator review: $AFFILIATE_REVERSAL_CONTRACT_PENDING_FILE" >&2
    exit 1
    ;;
  activated)
    if [[ -e "$AFFILIATE_REVERSAL_CONTRACT_PENDING_FILE" || -L "$AFFILIATE_REVERSAL_CONTRACT_PENDING_FILE" ]] ||
      [[ ! -e "$AFFILIATE_REVERSAL_CONTRACT_FILE" && ! -L "$AFFILIATE_REVERSAL_CONTRACT_FILE" ]]; then
      echo "Activated deployment state conflicts with affiliate refund reversal contract files" >&2
      exit 1
    fi
    load_affiliate_reversal_contract
    ;;
esac

payment_persistent_state=""
payment_state_version=""
payment_state_version_seen=false
payment_persistent_state_seen=false
while IFS='=' read -r key value; do
  case "$key" in
    state_version)
      [[ "$payment_state_version_seen" == false ]] || { echo "Duplicate state_version in payment reversal component state" >&2; exit 1; }
      payment_state_version_seen=true
      payment_state_version=$value
      ;;
    payment_reversal_components_state)
      [[ "$payment_persistent_state_seen" == false ]] || { echo "Duplicate payment_reversal_components_state" >&2; exit 1; }
      payment_persistent_state_seen=true
      payment_persistent_state=$value
      ;;
    updated_at|"") ;;
    *) echo "Unknown field in payment reversal component state: $key" >&2; exit 1 ;;
  esac
done < "$PAYMENT_REVERSAL_COMPONENTS_STATE_FILE"
if [[ "$payment_state_version" != 1 ]] ||
  [[ ! "$payment_persistent_state" =~ ^(absent|pending|activated)$ ]]; then
  echo "Payment reversal component state is invalid or incomplete" >&2
  exit 1
fi

payment_contract_state=absent
payment_contract_version=""
payment_contract_image_id=""
payment_contract_revision=""
payment_contract_capability_label=""
payment_contract_capability_value=""
load_payment_reversal_components_contract() {
  local key
  local value
  local version_seen=false
  local state_seen=false
  local image_seen=false
  local revision_seen=false
  local label_seen=false
  local value_seen=false

  validate_secure_state_file "$PAYMENT_REVERSAL_COMPONENTS_CONTRACT_FILE"
  while IFS='=' read -r key value; do
    case "$key" in
      contract_version)
        [[ "$version_seen" == false ]] || { echo "Duplicate payment contract_version" >&2; return 1; }
        version_seen=true
        payment_contract_version=$value
        ;;
      state)
        [[ "$state_seen" == false ]] || { echo "Duplicate payment contract state" >&2; return 1; }
        state_seen=true
        payment_contract_state=$value
        ;;
      image_id)
        [[ "$image_seen" == false ]] || { echo "Duplicate payment contract image_id" >&2; return 1; }
        image_seen=true
        payment_contract_image_id=$value
        ;;
      revision)
        [[ "$revision_seen" == false ]] || { echo "Duplicate payment contract revision" >&2; return 1; }
        revision_seen=true
        payment_contract_revision=$value
        ;;
      capability_label)
        [[ "$label_seen" == false ]] || { echo "Duplicate payment contract capability_label" >&2; return 1; }
        label_seen=true
        payment_contract_capability_label=$value
        ;;
      capability_value)
        [[ "$value_seen" == false ]] || { echo "Duplicate payment contract capability_value" >&2; return 1; }
        value_seen=true
        payment_contract_capability_value=$value
        ;;
      prepared_at|activated_at|"") ;;
      *) echo "Unknown field in payment reversal component contract: $key" >&2; return 1 ;;
    esac
  done < "$PAYMENT_REVERSAL_COMPONENTS_CONTRACT_FILE"

  if [[ "$payment_contract_version" != 1 || "$payment_contract_state" != activated ||
    "$payment_contract_capability_label" != "$PAYMENT_REVERSAL_COMPONENTS_CAPABILITY_LABEL" ||
    "$payment_contract_capability_value" != 1 ||
    ! "$payment_contract_image_id" =~ ^sha256:[0-9a-f]{64}$ ||
    ! "$payment_contract_revision" =~ ^[0-9a-f]{40,64}$ ]]; then
    echo "Payment reversal component contract is invalid or incomplete" >&2
    return 1
  fi
}

case "$payment_persistent_state" in
  absent)
    if [[ -e "$PAYMENT_REVERSAL_COMPONENTS_CONTRACT_FILE" || -L "$PAYMENT_REVERSAL_COMPONENTS_CONTRACT_FILE" ||
      -e "$PAYMENT_REVERSAL_COMPONENTS_CONTRACT_PENDING_FILE" || -L "$PAYMENT_REVERSAL_COMPONENTS_CONTRACT_PENDING_FILE" ]]; then
      echo "Absent payment reversal component state conflicts with contract files" >&2
      exit 1
    fi
    ;;
  pending)
    if [[ -e "$PAYMENT_REVERSAL_COMPONENTS_CONTRACT_FILE" || -L "$PAYMENT_REVERSAL_COMPONENTS_CONTRACT_FILE" ]] ||
      [[ ! -e "$PAYMENT_REVERSAL_COMPONENTS_CONTRACT_PENDING_FILE" && ! -L "$PAYMENT_REVERSAL_COMPONENTS_CONTRACT_PENDING_FILE" ]]; then
      echo "Pending payment reversal component state conflicts with contract files" >&2
      exit 1
    fi
    validate_secure_state_file "$PAYMENT_REVERSAL_COMPONENTS_CONTRACT_PENDING_FILE"
    echo "An incomplete payment reversal component activation requires forward operator recovery: $PAYMENT_REVERSAL_COMPONENTS_CONTRACT_PENDING_FILE" >&2
    exit 1
    ;;
  activated)
    if [[ -e "$PAYMENT_REVERSAL_COMPONENTS_CONTRACT_PENDING_FILE" || -L "$PAYMENT_REVERSAL_COMPONENTS_CONTRACT_PENDING_FILE" ]] ||
      [[ ! -e "$PAYMENT_REVERSAL_COMPONENTS_CONTRACT_FILE" && ! -L "$PAYMENT_REVERSAL_COMPONENTS_CONTRACT_FILE" ]]; then
      echo "Activated payment reversal component state conflicts with contract files" >&2
      exit 1
    fi
    load_payment_reversal_components_contract
    ;;
esac

if [[ "$contract_state" == activated && "$affiliate_refund_reversal_stage" != enabled ]]; then
  echo "Affiliate refund reversal is permanently activated; disabled deployments are rejected" >&2
  exit 1
fi

docker_config="$(mktemp -d)"
cleanup() {
  unset ghcr_token
  if [[ -n "${candidate:-}" ]]; then
    rm -f -- "$candidate"
  fi
  rm -rf -- "$docker_config"
}
trap cleanup EXIT

printf '%s' "$ghcr_token" |
  docker --config "$docker_config" login ghcr.io \
    --username "$ghcr_username" \
    --password-stdin

color_port() {
  case "$1" in
    blue) printf '%s' 18080 ;;
    green) printf '%s' 28080 ;;
    *) return 2 ;;
  esac
}

container_healthy_on_port() {
  local color=$1
  local container="sub2api-${color}"
  local port
  port="$(color_port "$color")"

  [[ "$(docker inspect --format '{{.State.Running}}' "$container" 2>/dev/null || true)" == true ]] || return 1
  [[ "$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$container")" == healthy ]] || return 1
  [[ "$(curl --noproxy '*' -sS -o /dev/null -w '%{http_code}' --max-time 10 "http://127.0.0.1:${port}/health")" == 200 ]]
}

container_affiliate_reversal_value() {
  local container=$1
  local env_line
  local value=""
  local count=0

  while IFS= read -r env_line; do
    case "$env_line" in
      AFFILIATE_REFUND_REVERSAL_ENABLED=*)
        value=${env_line#AFFILIATE_REFUND_REVERSAL_ENABLED=}
        count=$((count + 1))
        ;;
    esac
  done < <(docker inspect --format '{{range .Config.Env}}{{println .}}{{end}}' "$container")

  [[ "$count" -eq 1 ]] || return 1
  printf '%s' "$value"
}

image_label_value() {
  local image=$1
  local label=$2
  docker image inspect --format "{{ index .Config.Labels \"${label}\" }}" "$image"
}

nginx_proxy_pass_count() {
  local upstream=$1
  awk -v expected="http://${upstream};" '
    {
      line = $0
      sub(/[[:space:]]*#.*/, "", line)
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", line)
      if (line ~ /^proxy_pass[[:space:]]+/) {
        field_count = split(line, fields, /[[:space:]]+/)
        if (field_count == 2 && fields[2] == expected) {
          matches++
        }
      }
    }
    END { print matches + 0 }
  ' "$NGINX_CONFIG"
}

nginx_routes_to_only_slot() {
  local expected_upstream=$1
  local stale_upstream=$2
  [[ "$(nginx_proxy_pass_count "$expected_upstream")" -eq 1 ]] &&
    [[ "$(nginx_proxy_pass_count "$stale_upstream")" -eq 0 ]]
}

public_health_ok() {
  [[ "$(curl --noproxy '*' --resolve "$PUBLIC_HEALTH_RESOLVE" -sS -o /dev/null -w '%{http_code}' --max-time 10 "$PUBLIC_HEALTH_URL")" == 200 ]]
}

reload_nginx_verified() {
  local master_pid
  local current_master_pid
  local workers_before
  local workers_after
  local worker

  nginx -t || return 1
  [[ -f "$NGINX_PID_FILE" && ! -L "$NGINX_PID_FILE" ]] || return 1
  master_pid="$(tr -d '[:space:]' < "$NGINX_PID_FILE")"
  [[ "$master_pid" =~ ^[1-9][0-9]*$ ]] || return 1
  workers_before="$(pgrep -P "$master_pid" 2>/dev/null | sort -n || true)"
  [[ -n "$workers_before" ]] || return 1
  nginx -s reload || return 1
  for ((attempt = 0; attempt < 10; attempt++)); do
    current_master_pid="$(tr -d '[:space:]' < "$NGINX_PID_FILE")"
    [[ "$current_master_pid" == "$master_pid" ]] || return 1
    workers_after="$(pgrep -P "$master_pid" 2>/dev/null | sort -n || true)"
    while IFS= read -r worker; do
      [[ -n "$worker" ]] || continue
      if ! grep -Fxq -- "$worker" <<< "$workers_before"; then return 0; fi
    done <<< "$workers_after"
    sleep 1
  done
  echo "Nginx reload did not create a new worker" >&2
  return 1
}

remove_target_container_verified() {
  if docker container inspect "$target_container" >/dev/null 2>&1; then
    docker rm -f "$target_container" >/dev/null 2>&1 || return 1
  else
    docker info >/dev/null 2>&1 || return 1
  fi
  if docker container inspect "$target_container" >/dev/null 2>&1; then
    echo "Target container still exists after removal: $target_container" >&2
    return 1
  fi
  return 0
}

active_color=""
for color in blue green; do
  if container_healthy_on_port "$color"; then
    if [[ -n "$active_color" ]]; then
      echo "Both blue and green Sub2API instances are healthy; refusing ambiguous cutover" >&2
      exit 1
    fi
    active_color=$color
  fi
done

if [[ -z "$active_color" ]]; then
  echo "No healthy active Sub2API blue-green instance was found" >&2
  exit 1
fi

if [[ "$active_color" == blue ]]; then
  target_color=green
else
  target_color=blue
fi

active_container="sub2api-${active_color}"
target_container="sub2api-${target_color}"
active_port="$(color_port "$active_color")"
target_port="$(color_port "$target_color")"
active_upstream="127.0.0.1:${active_port}"
target_upstream="127.0.0.1:${target_port}"

if ! nginx_routes_to_only_slot "$active_upstream" "$target_upstream"; then
  echo "Nginx config does not route exclusively to the active upstream $active_upstream" >&2
  exit 1
fi

# The inactive slot must be stopped before the new candidate is evaluated.
if [[ "$(docker inspect --format '{{.State.Running}}' "$target_container" 2>/dev/null || true)" == true ]]; then
  echo "Inactive target container $target_container is unexpectedly running" >&2
  exit 1
fi

safe_tag="$(printf '%s' "$deploy_image_tag" | tr '/:@' '---')"
candidate="$(mktemp "${STATE_DIR}/sub2api-stable-${target_color}-${affiliate_refund_reversal_stage}-${safe_tag}.yml.XXXXXX")"
rollback_state=""

if [[ "$target_color" == blue ]]; then
  cat > "$candidate" <<EOF
services:
  sub2api:
    container_name: sub2api-blue
    image: ${deploy_image}
    ports: !override
      - "127.0.0.1:18080:8080"
    environment:
      AFFILIATE_REFUND_REVERSAL_ENABLED: "${affiliate_refund_reversal_enabled}"

networks:
  sub2api-net:
    external: true
    name: sub2api-net
EOF
else
  cat > "$candidate" <<EOF
services:
  sub2api:
    container_name: sub2api-green
    image: ${deploy_image}
    ports: !override
      - "127.0.0.1:28080:8080"
    environment:
      AFFILIATE_REFUND_REVERSAL_ENABLED: "${affiliate_refund_reversal_enabled}"

networks:
  sub2api-net:
    external: true
    name: sub2api-net
EOF
fi

compose=(docker compose --env-file "$COMPOSE_ENV_FILE" -p "sub2api-stable-${target_color}" -f "$COMPOSE_FILE" -f "$candidate")
DOCKER_CONFIG="$docker_config" "${compose[@]}" pull sub2api

candidate_image_id="$(docker image inspect --format '{{.Id}}' "$deploy_image")"
candidate_repo_digests="$(docker image inspect --format '{{range .RepoDigests}}{{println .}}{{end}}' "$deploy_image")"
candidate_revision="$(image_label_value "$deploy_image" org.opencontainers.image.revision)"
candidate_version="$(image_label_value "$deploy_image" org.opencontainers.image.version)"
candidate_trusted_ci="$(image_label_value "$deploy_image" "$TRUSTED_CI_IMAGE_LABEL")"
candidate_capability="$(image_label_value "$deploy_image" "$AFFILIATE_REVERSAL_CAPABILITY_LABEL")"
candidate_payment_reversal_components="$(image_label_value "$deploy_image" "$PAYMENT_REVERSAL_COMPONENTS_CAPABILITY_LABEL")"
active_image_ref="$(docker inspect --format '{{.Config.Image}}' "$active_container")"
active_image_id="$(docker inspect --format '{{.Image}}' "$active_container")"
active_repo_digests="$(docker image inspect --format '{{join .RepoDigests ","}}' "$active_image_id")"
active_revision="$(image_label_value "$active_image_id" org.opencontainers.image.revision)"
active_payment_reversal_components="$(image_label_value "$active_image_id" "$PAYMENT_REVERSAL_COMPONENTS_CAPABILITY_LABEL")"

if [[ ! "$candidate_image_id" =~ ^sha256:[0-9a-f]{64}$ || ! "$candidate_revision" =~ ^[0-9a-f]{40,64}$ ]] ||
  [[ ! "$candidate_version" =~ ^[0-9]+(\.[0-9]+){2,}([+-][0-9A-Za-z.-]+)?$ ]]; then
  echo "Candidate image ID, OCI revision, or OCI version metadata is incomplete or invalid" >&2
  exit 1
fi
if ! grep -Fxq "$deploy_image" <<< "$candidate_repo_digests"; then
  echo "Pulled image does not retain the requested registry digest: $deploy_image" >&2
  exit 1
fi

if [[ "$candidate_trusted_ci" != 1 ]]; then
  echo "Candidate image does not declare ${TRUSTED_CI_IMAGE_LABEL}=1" >&2
  exit 1
fi

if [[ ! "$candidate_payment_reversal_components" =~ ^[01]$ ]]; then
  echo "Candidate image has an invalid ${PAYMENT_REVERSAL_COMPONENTS_CAPABILITY_LABEL} label" >&2
  exit 1
fi
if [[ "$payment_contract_state" == activated && "$candidate_payment_reversal_components" != 1 ]]; then
  echo "Payment reversal components are permanently activated; incapable images are rejected" >&2
  exit 1
fi

if [[ "$contract_state" == activated || "$affiliate_refund_reversal_stage" == enabled ]]; then
  if [[ "$candidate_capability" != 1 ]]; then
    echo "Candidate image does not declare ${AFFILIATE_REVERSAL_CAPABILITY_LABEL}=1" >&2
    exit 1
  fi
fi

if ! DOCKER_CONFIG="$docker_config" GH_TOKEN="$ghcr_token" gh attestation verify \
  "oci://${deploy_image}" \
  --repo "$ATTESTATION_REPOSITORY" \
  --bundle-from-oci \
  --predicate-type https://slsa.dev/provenance/v1 \
  --cert-identity "$ATTESTATION_CERT_IDENTITY" \
  --cert-oidc-issuer https://token.actions.githubusercontent.com \
  --source-ref refs/heads/main \
  --source-digest "$candidate_revision" \
  --deny-self-hosted-runners >/dev/null; then
  echo "Candidate image lacks trusted main-branch GitHub build provenance" >&2
  exit 1
fi

release_tag="v${candidate_version}"
if ! tag_object="$(DOCKER_CONFIG="$docker_config" GH_TOKEN="$ghcr_token" gh api \
  "repos/${ATTESTATION_REPOSITORY}/git/ref/tags/${release_tag}" \
  --jq '[.object.type, .object.sha] | @tsv')"; then
  echo "Release tag ${release_tag} is missing" >&2
  exit 1
fi
IFS=$'\t' read -r tag_object_type release_tag_commit <<< "$tag_object"
tag_depth=0
while [[ "$tag_object_type" == tag && "$tag_depth" -lt 8 ]]; do
  if ! tag_object="$(DOCKER_CONFIG="$docker_config" GH_TOKEN="$ghcr_token" gh api \
    "repos/${ATTESTATION_REPOSITORY}/git/tags/${release_tag_commit}" \
    --jq '[.object.type, .object.sha] | @tsv')"; then
    echo "Annotated release tag ${release_tag} could not be peeled" >&2
    exit 1
  fi
  IFS=$'\t' read -r tag_object_type release_tag_commit <<< "$tag_object"
  tag_depth=$((tag_depth + 1))
done
if [[ "$tag_object_type" != commit || "$release_tag_commit" != "$candidate_revision" ]]; then
  echo "Release tag ${release_tag} does not resolve to candidate revision ${candidate_revision}" >&2
  exit 1
fi

if ! release_record="$(DOCKER_CONFIG="$docker_config" GH_TOKEN="$ghcr_token" gh api \
  "repos/${ATTESTATION_REPOSITORY}/releases/tags/${release_tag}" \
  --jq '[.tag_name, (.draft | tostring), (.prerelease | tostring)] | @tsv')"; then
  echo "Published GitHub release is missing or inconsistent for ${release_tag}" >&2
  exit 1
fi
IFS=$'\t' read -r published_release_tag published_release_draft published_release_prerelease <<< "$release_record"
if [[ "$published_release_tag" != "$release_tag" || "$published_release_draft" != false ||
  "$published_release_prerelease" != false ]]; then
  echo "Published GitHub release is missing or inconsistent for ${release_tag}" >&2
  exit 1
fi

binary_version_output="$(docker run --rm --entrypoint /app/sub2api "$deploy_image" --version 2>&1)"
binary_build_records=()
while IFS= read -r binary_build_record; do
  [[ -n "$binary_build_record" ]] && binary_build_records+=("$binary_build_record")
done < <(
  sed -nE 's/^.*Sub2API[[:space:]]+([^[:space:]]+)[[:space:]]+\(commit:[[:space:]]+([0-9a-f]{40,64}),[[:space:]]+built:[[:space:]]+[^)]*\).*$/\1\t\2/p' <<< "$binary_version_output"
)
if [[ "${#binary_build_records[@]}" -ne 1 ]]; then
  echo "Candidate binary did not report one parseable version record" >&2
  exit 1
fi
IFS=$'\t' read -r candidate_binary_version candidate_binary_commit <<< "${binary_build_records[0]}"
if [[ "$candidate_binary_version" != "$candidate_version" || "$candidate_binary_commit" != "$candidate_revision" ]]; then
  echo "Candidate binary version/commit does not match OCI metadata" >&2
  exit 1
fi
unset ghcr_token

active_affiliate_refund_reversal_value="<unset>"
if resolved_active_affiliate_refund_reversal_value="$(container_affiliate_reversal_value "$active_container")"; then
  active_affiliate_refund_reversal_value=$resolved_active_affiliate_refund_reversal_value
fi

if [[ "$contract_state" == absent && "$affiliate_refund_reversal_stage" == enabled ]]; then
  if [[ "$active_image_id" != "$candidate_image_id" ]]; then
    echo "Initial activation must use the exact image ID already running with the gate disabled" >&2
    exit 1
  fi
  if [[ "$active_affiliate_refund_reversal_value" != false ]]; then
    echo "Initial activation requires the active container gate to be explicitly false" >&2
    exit 1
  fi
elif [[ "$contract_state" == activated && "$active_affiliate_refund_reversal_value" != true ]]; then
  echo "Activated contract requires the active container gate to be explicitly true" >&2
  exit 1
fi
if [[ "$payment_contract_state" == absent && "$active_payment_reversal_components" == 1 ]]; then
  echo "Active image already has payment reversal components but the monotonic contract is absent; operator review is required" >&2
  exit 1
fi

rollback_state="$(mktemp "${STATE_DIR}/rollback-state-${affiliate_refund_reversal_stage}-${safe_tag}.txt.XXXXXX")"
{
  echo "timestamp=$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  echo "stage=$affiliate_refund_reversal_stage"
  echo "contract_state_before=$contract_state"
  echo "deploy_image=$deploy_image"
  echo "candidate_image_id=$candidate_image_id"
  echo "candidate_repo_digest=$deploy_image"
  echo "candidate_revision=$candidate_revision"
  echo "candidate_version=$candidate_version"
  echo "candidate_release_tag=$release_tag"
  echo "candidate_release_commit=$release_tag_commit"
  echo "candidate_trusted_ci=$candidate_trusted_ci"
  echo "candidate_capability=$candidate_capability"
  echo "candidate_payment_reversal_components=$candidate_payment_reversal_components"
  echo "active_container=$active_container"
  echo "active_image=$active_image_ref"
  echo "active_image_id=$active_image_id"
  echo "active_repo_digests=$active_repo_digests"
  echo "active_revision=$active_revision"
  echo "active_payment_reversal_components=$active_payment_reversal_components"
  echo "active_affiliate_refund_reversal_enabled=$active_affiliate_refund_reversal_value"
  echo "target_container=$target_container"
  echo "candidate=$candidate"
  echo "nginx_config=$NGINX_CONFIG"
} > "$rollback_state"

prepared_contract_pending=false
prepared_payment_components_pending=false
cutover_started=false

attempt_safe_cutover_rollback() {
  local rollback_health_verified=false

  if container_healthy_on_port "$active_color" &&
    nginx_routes_to_only_slot "$active_upstream" "$target_upstream"; then
    if reload_nginx_verified && public_health_ok && remove_target_container_verified; then
      return 0
    fi
    return 1
  fi

  docker start "$active_container" >/dev/null 2>&1 || true
  for ((rollback_attempt = 0; rollback_attempt < 60; rollback_attempt++)); do
    if container_healthy_on_port "$active_color"; then
      rollback_health_verified=true
      break
    fi
    sleep 1
  done
  [[ "$rollback_health_verified" == true ]] || return 1
  nginx_routes_to_only_slot "$target_upstream" "$active_upstream" || return 1

  if ! "$CUTOVER_SCRIPT" \
    "$target_container" \
    "$active_container" \
    "$target_upstream" \
    "$active_upstream" \
    "$PUBLIC_HEALTH_URL" \
    "$NGINX_CONFIG" \
    recover; then
    return 1
  fi

  if container_healthy_on_port "$active_color" &&
    nginx_routes_to_only_slot "$active_upstream" "$target_upstream" && public_health_ok; then
    remove_target_container_verified || return 1
    return 0
  fi
  return 1
}

abort_deployment() {
  local status=$1
  trap - ERR INT TERM
  set +e
  if [[ "$cutover_started" != true ]]; then
    if ! remove_target_container_verified; then
      echo "Deployment failed and candidate cleanup could not be verified; operator intervention is required." >&2
    fi
  fi
  if [[ "$prepared_contract_pending" == true || "$prepared_payment_components_pending" == true ]]; then
    echo "An irreversible deployment activation remains pending; forward operator recovery is required." >&2
  elif [[ "$cutover_started" == true ]]; then
    if attempt_safe_cutover_rollback; then
      {
        echo "rollback_completed_at=$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
        echo "rollback_restored_container=$active_container"
      } >> "$rollback_state"
      echo "Deployment failed after cutover and the previous slot was restored." >&2
    else
      echo "Cutover started and automatic rollback could not be verified. Operator review is required." >&2
    fi
  fi
  exit "$status"
}
trap 'abort_deployment $?' ERR
trap 'abort_deployment 130' INT
trap 'abort_deployment 143' TERM

if [[ "$contract_state" == absent && "$affiliate_refund_reversal_stage" == enabled ]]; then
  contract_temp="$(mktemp "${AFFILIATE_REVERSAL_CONTRACT_FILE}.tmp.XXXXXX")"
  {
    echo "contract_version=1"
    echo "state=pending"
    echo "stage=enabled"
    echo "image_id=$candidate_image_id"
    echo "revision=$candidate_revision"
    echo "capability_label=$AFFILIATE_REVERSAL_CAPABILITY_LABEL"
    echo "capability_value=1"
    echo "prepared_at=$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  } > "$contract_temp"
  chmod 600 "$contract_temp"
  sync -f "$contract_temp"
  mv -f -- "$contract_temp" "$AFFILIATE_REVERSAL_CONTRACT_PENDING_FILE"
  sync -f "$STATE_DIR"
  write_affiliate_reversal_state pending
  prepared_contract_pending=true
fi

if [[ "$payment_contract_state" == absent && "$candidate_payment_reversal_components" == 1 ]]; then
  payment_contract_temp="$(mktemp "${PAYMENT_REVERSAL_COMPONENTS_CONTRACT_FILE}.tmp.XXXXXX")"
  {
    echo "contract_version=1"
    echo "state=pending"
    echo "image_id=$candidate_image_id"
    echo "revision=$candidate_revision"
    echo "capability_label=$PAYMENT_REVERSAL_COMPONENTS_CAPABILITY_LABEL"
    echo "capability_value=1"
    echo "prepared_at=$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  } > "$payment_contract_temp"
  chmod 600 "$payment_contract_temp"
  sync -f "$payment_contract_temp"
  mv -f -- "$payment_contract_temp" "$PAYMENT_REVERSAL_COMPONENTS_CONTRACT_PENDING_FILE"
  sync -f "$STATE_DIR"
  write_payment_reversal_components_state pending
  prepared_payment_components_pending=true
fi

cutover_rollback_policy=allow
if [[ "$prepared_contract_pending" == true || "$prepared_payment_components_pending" == true ]]; then
  cutover_rollback_policy=forbid
  # Starting either an affiliate gate=true process or a future contract image
  # that removes migration 197's legacy-writer bridge can make old writers
  # unsafe. Stop the old slot before contract migrations or irreversible state.
  docker stop --time 120 "$active_container" >/dev/null
  if [[ "$(docker inspect --format '{{.State.Running}}' "$active_container" 2>/dev/null || true)" == true ]]; then
    echo "Irreversible activation could not stop the gate=false slot" >&2
    false
  fi
fi

remove_target_container_verified
"${compose[@]}" up -d --no-deps --force-recreate sub2api

for ((attempt = 0; attempt < 90; attempt++)); do
  running="$(docker inspect --format '{{.State.Running}}' "$target_container" 2>/dev/null || true)"
  health="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$target_container" 2>/dev/null || true)"

  if [[ "$running" == true && "$health" == healthy ]] &&
    [[ "$(curl --noproxy '*' -sS -o /dev/null -w '%{http_code}' --max-time 10 "http://${target_upstream}/health")" == 200 ]]; then
    break
  fi
  if [[ "$running" != true || "$health" == unhealthy ]]; then
    docker logs --tail 150 "$target_container" >&2 || true
    false
  fi
  sleep 2
done

if [[ "$attempt" -ge 90 ]]; then
  docker logs --tail 150 "$target_container" >&2 || true
  false
fi

actual_image="$(docker inspect --format '{{.Config.Image}}' "$target_container")"
if [[ "$actual_image" != "$deploy_image" ]]; then
  echo "Unexpected image for $target_container: $actual_image" >&2
  false
fi

actual_image_id="$(docker inspect --format '{{.Image}}' "$target_container")"
if [[ "$actual_image_id" != "$candidate_image_id" ]]; then
  echo "Unexpected image ID for $target_container: $actual_image_id" >&2
  false
fi

if ! target_affiliate_refund_reversal_value="$(container_affiliate_reversal_value "$target_container")" ||
  [[ "$target_affiliate_refund_reversal_value" != "$affiliate_refund_reversal_enabled" ]]; then
  echo "Candidate container did not receive AFFILIATE_REFUND_REVERSAL_ENABLED=$affiliate_refund_reversal_enabled" >&2
  false
fi

cutover_started=true
"$CUTOVER_SCRIPT" \
  "$active_container" \
  "$target_container" \
  "$active_upstream" \
  "$target_upstream" \
  "$PUBLIC_HEALTH_URL" \
  "$NGINX_CONFIG" \
  "$cutover_rollback_policy"

for ((stop_attempt = 0; stop_attempt < 30; stop_attempt++)); do
  if [[ "$(docker inspect --format '{{.State.Running}}' "$active_container" 2>/dev/null || true)" != true ]]; then
    break
  fi
  sleep 1
done

if [[ "$(docker inspect --format '{{.State.Running}}' "$active_container" 2>/dev/null || true)" == true ]]; then
  echo "Old active container $active_container is still running after cutover" >&2
  false
fi
if ! nginx_routes_to_only_slot "$target_upstream" "$active_upstream"; then
  echo "Nginx config does not route exclusively to the new active upstream $target_upstream after cutover" >&2
  false
fi
if ! container_healthy_on_port "$target_color"; then
  echo "New active container $target_container is not healthy after cutover" >&2
  false
fi

public_health_verified=false
for ((public_health_attempt = 0; public_health_attempt < 10; public_health_attempt++)); do
  if public_health_ok; then
    public_health_verified=true
    break
  fi
  sleep 2
done
if [[ "$public_health_verified" != true ]]; then
  echo "Public health check failed after the old active slot stopped: $PUBLIC_HEALTH_URL" >&2
  false
fi

if [[ "$prepared_contract_pending" == true ]]; then
  activated_contract_temp="$(mktemp "${AFFILIATE_REVERSAL_CONTRACT_FILE}.tmp.XXXXXX")"
  {
    echo "contract_version=1"
    echo "state=activated"
    echo "stage=enabled"
    echo "image_id=$candidate_image_id"
    echo "revision=$candidate_revision"
    echo "capability_label=$AFFILIATE_REVERSAL_CAPABILITY_LABEL"
    echo "capability_value=1"
    echo "activated_at=$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  } > "$activated_contract_temp"
  chmod 600 "$activated_contract_temp"
  sync -f "$activated_contract_temp"
  mv -f -- "$activated_contract_temp" "$AFFILIATE_REVERSAL_CONTRACT_PENDING_FILE"
  sync -f "$STATE_DIR"
  mv -f -- "$AFFILIATE_REVERSAL_CONTRACT_PENDING_FILE" "$AFFILIATE_REVERSAL_CONTRACT_FILE"
  sync -f "$STATE_DIR"
  write_affiliate_reversal_state activated
  prepared_contract_pending=false
  contract_state=activated
fi

if [[ "$prepared_payment_components_pending" == true ]]; then
  activated_payment_contract_temp="$(mktemp "${PAYMENT_REVERSAL_COMPONENTS_CONTRACT_FILE}.tmp.XXXXXX")"
  {
    echo "contract_version=1"
    echo "state=activated"
    echo "image_id=$candidate_image_id"
    echo "revision=$candidate_revision"
    echo "capability_label=$PAYMENT_REVERSAL_COMPONENTS_CAPABILITY_LABEL"
    echo "capability_value=1"
    echo "activated_at=$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  } > "$activated_payment_contract_temp"
  chmod 600 "$activated_payment_contract_temp"
  sync -f "$activated_payment_contract_temp"
  mv -f -- "$activated_payment_contract_temp" "$PAYMENT_REVERSAL_COMPONENTS_CONTRACT_PENDING_FILE"
  sync -f "$STATE_DIR"
  mv -f -- "$PAYMENT_REVERSAL_COMPONENTS_CONTRACT_PENDING_FILE" "$PAYMENT_REVERSAL_COMPONENTS_CONTRACT_FILE"
  sync -f "$STATE_DIR"
  write_payment_reversal_components_state activated
  prepared_payment_components_pending=false
  payment_contract_state=activated
fi

{
  echo "cutover_completed_at=$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  echo "old_slot_stopped=true"
  echo "contract_state_after=$contract_state"
  echo "payment_reversal_components_state_after=$payment_contract_state"
} >> "$rollback_state"

trap - ERR INT TERM
echo "Blue-green deployment complete: image=$deploy_image stage=$affiliate_refund_reversal_stage rollback_state=$rollback_state"
