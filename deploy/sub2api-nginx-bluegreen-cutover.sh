#!/usr/bin/env bash

set -Eeuo pipefail

umask 077

readonly PROTOCOL_VERSION="sub2api-nginx-bluegreen-cutover-v2"
readonly STATE_DIR="/var/lib/sub2api-deploy"
readonly STATE_OWNER_UID="0"
readonly STATE_OWNER_GID="0"
readonly LOCK_FILE="${STATE_DIR}/nginx-bluegreen-cutover.lock"

if [[ "${1:-}" == --protocol ]]; then
  [[ "$#" -eq 1 ]] || exit 2
  printf '%s\n' "$PROTOCOL_VERSION"
  exit 0
fi

if [[ "$#" -ne 7 ]]; then
  echo "Usage: $0 <active-container> <target-container> <active-upstream> <target-upstream> <public-health-url> <nginx-config> <rollback-policy>" >&2
  exit 2
fi

if [[ "$(id -u)" != "$STATE_OWNER_UID" || "$(id -g)" != "$STATE_OWNER_GID" ]]; then
  echo "Nginx cutover must run as ${STATE_OWNER_UID}:${STATE_OWNER_GID}" >&2
  exit 1
fi
if [[ -L "$STATE_DIR" || ! -d "$STATE_DIR" ]] ||
  [[ "$(stat -c '%u:%g:%a' -- "$STATE_DIR")" != "${STATE_OWNER_UID}:${STATE_OWNER_GID}:700" ]]; then
  echo "Deployment state directory must be root-owned, non-symlink, and mode 0700: $STATE_DIR" >&2
  exit 1
fi
if [[ -L "$LOCK_FILE" || ( -e "$LOCK_FILE" && ! -f "$LOCK_FILE" ) ]]; then
  echo "Nginx cutover lock path is unsafe: $LOCK_FILE" >&2
  exit 1
fi
if [[ ! -e "$LOCK_FILE" ]]; then
  (set -o noclobber; : > "$LOCK_FILE") 2>/dev/null || true
fi
if [[ -L "$LOCK_FILE" || ! -f "$LOCK_FILE" ]] ||
  [[ "$(stat -c '%u:%g:%a' -- "$LOCK_FILE")" != "${STATE_OWNER_UID}:${STATE_OWNER_GID}:600" ]]; then
  echo "Nginx cutover lock must be root-owned, non-symlink, and mode 0600: $LOCK_FILE" >&2
  exit 1
fi

active_container=$1
target_container=$2
active_upstream=$3
target_upstream=$4
public_health_url=$5
requested_nginx_config=$6
rollback_policy=$7

case "$rollback_policy" in
  allow|forbid|recover) ;;
  *) echo "Rollback policy must be allow, forbid, or recover" >&2; exit 2 ;;
esac

case "${active_container}:${active_upstream}" in
  sub2api-blue:127.0.0.1:18080|sub2api-green:127.0.0.1:28080) ;;
  *) echo "Active container and upstream do not match a stable slot" >&2; exit 2 ;;
esac
case "${target_container}:${target_upstream}" in
  sub2api-blue:127.0.0.1:18080|sub2api-green:127.0.0.1:28080) ;;
  *) echo "Target container and upstream do not match a stable slot" >&2; exit 2 ;;
esac
if [[ "$active_container" == "$target_container" || "$active_upstream" == "$target_upstream" ]]; then
  echo "Active and target slots must be different" >&2
  exit 2
fi
if [[ ! "$public_health_url" =~ ^https://[^[:space:]]+/health$ ]]; then
  echo "Public health URL must use HTTPS and end in /health" >&2
  exit 2
fi
if [[ ! -f "$requested_nginx_config" ]]; then
  echo "Nginx configuration is not a regular file: $requested_nginx_config" >&2
  exit 2
fi
nginx_config=$(readlink -f -- "$requested_nginx_config")
if [[ -z "$nginx_config" || ! -f "$nginx_config" ]]; then
  echo "Nginx configuration target cannot be resolved: $requested_nginx_config" >&2
  exit 2
fi
nginx_dir=$(dirname -- "$nginx_config")
nginx_base=$(basename -- "$nginx_config")

validate_root_owned_nonwritable_path() {
  local path=$1
  local expected_type=$2
  local metadata
  local owner_uid
  local owner_gid
  local mode

  case "$expected_type" in
    directory) [[ -d "$path" && ! -L "$path" ]] ;;
    file) [[ -f "$path" && ! -L "$path" ]] ;;
    *) return 2 ;;
  esac || {
    echo "Nginx cutover path is not a regular ${expected_type}: $path" >&2
    return 1
  }
  metadata="$(stat -c '%u:%g:%a' -- "$path")"
  IFS=: read -r owner_uid owner_gid mode <<< "$metadata"
  if [[ "$owner_uid" != "$STATE_OWNER_UID" || "$owner_gid" != "$STATE_OWNER_GID" ]] ||
    [[ ! "$mode" =~ ^[0-7]{3,4}$ ]] || (( (8#$mode & 0022) != 0 )); then
    echo "Nginx cutover path must be root-owned and not group/other writable: $path" >&2
    return 1
  fi
}

validate_root_owned_nonwritable_path "$nginx_dir" directory
validate_root_owned_nonwritable_path "$nginx_config" file

exec 9>>"$LOCK_FILE"
flock -n 9 || { echo "Another Nginx cutover is already running" >&2; exit 1; }

container_healthy() {
  local container=$1
  local upstream=$2
  [[ "$(docker inspect --format '{{.State.Running}}' "$container" 2>/dev/null || true)" == true ]] || return 1
  [[ "$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$container")" == healthy ]] || return 1
  [[ "$(curl --noproxy '*' -sS -o /dev/null -w '%{http_code}' --max-time 10 "http://${upstream}/health")" == 200 ]]
}

public_health_ok() {
  [[ "$(curl --noproxy '*' -sS -o /dev/null -w '%{http_code}' --max-time 10 "$public_health_url")" == 200 ]]
}

if [[ "$rollback_policy" == allow ]]; then
  container_healthy "$active_container" "$active_upstream" || {
    echo "Active slot is not healthy before cutover: $active_container" >&2
    exit 1
  }
elif [[ "$rollback_policy" == forbid ]] &&
  [[ "$(docker inspect --format '{{.State.Running}}' "$active_container" 2>/dev/null || true)" == true ]]; then
  echo "Fail-closed cutover requires the irreversible old slot to be stopped first" >&2
  exit 1
fi
container_healthy "$target_container" "$target_upstream" || {
  echo "Target slot is not healthy before cutover: $target_container" >&2
  exit 1
}

timestamp=$(date -u '+%Y%m%dT%H%M%SZ')
backup=$(mktemp "${nginx_dir}/${nginx_base}.pre-cutover-${timestamp}.XXXXXX")
candidate=$(mktemp "${nginx_dir}/.${nginx_base}.candidate.XXXXXX")
rollback_candidate=""
switch_committed=false
old_stop_started=false

cleanup() {
  rm -f -- "$candidate"
  if [[ -n "$rollback_candidate" ]]; then
    rm -f -- "$rollback_candidate"
  fi
}
trap cleanup EXIT

cp --preserve=mode,ownership -- "$nginx_config" "$backup"

if ! awk -v active="http://${active_upstream};" -v target="http://${target_upstream};" '
  {
    original = $0
    line = $0
    sub(/[[:space:]]*#.*/, "", line)
    gsub(/^[[:space:]]+|[[:space:]]+$/, "", line)
    if (line ~ /^proxy_pass[[:space:]]+/) {
      field_count = split(line, fields, /[[:space:]]+/)
      if (field_count == 2 && fields[2] == active) {
        position = index(original, active)
        if (position == 0) exit 43
        original = substr(original, 1, position - 1) target substr(original, position + length(active))
        active_matches++
      } else if (field_count == 2 && fields[2] == target) {
        target_matches++
      }
    }
    print original
  }
  END {
    if (active_matches != 1 || target_matches != 0) {
      print "Expected exactly one active proxy_pass and no target proxy_pass" > "/dev/stderr"
      exit 42
    }
  }
' "$nginx_config" > "$candidate"; then
  echo "Nginx configuration did not contain one unambiguous active route" >&2
  exit 1
fi

chown --reference="$nginx_config" -- "$candidate"
chmod --reference="$nginx_config" -- "$candidate"

rollback_cutover() {
  local original_status=$1
  local rollback_reload_verified=false
  trap - ERR INT TERM
  set +e

  if [[ "$rollback_policy" == forbid ]]; then
    echo "Cutover failed after an irreversible activation; automatic downgrade is forbidden" >&2
    exit "$original_status"
  fi
  if [[ "$rollback_policy" == recover ]]; then
    echo "Recovery cutover failed; returning to the known-bad slot is forbidden" >&2
    exit "$original_status"
  fi

  if [[ "$old_stop_started" == true ]] &&
    [[ "$(docker inspect --format '{{.State.Running}}' "$active_container" 2>/dev/null || true)" != true ]]; then
    docker start "$active_container" >/dev/null
    for ((attempt = 0; attempt < 30; attempt++)); do
      container_healthy "$active_container" "$active_upstream" && break
      sleep 1
    done
  fi

  if [[ "$switch_committed" == true ]]; then
    rollback_candidate=$(mktemp "${nginx_dir}/.${nginx_base}.rollback.XXXXXX")
    cp --preserve=mode,ownership -- "$backup" "$rollback_candidate"
    mv -f -- "$rollback_candidate" "$nginx_config"
    rollback_candidate=""
    if nginx -t && nginx -s reload; then
      rollback_reload_verified=true
    fi
  fi

  if [[ "$rollback_reload_verified" == true ]] &&
    container_healthy "$active_container" "$active_upstream" && public_health_ok; then
    docker rm -f "$target_container" >/dev/null 2>&1 || true
    echo "Cutover failed and the previous slot was restored: $active_container" >&2
  else
    echo "Cutover failed and automatic rollback could not be verified; operator intervention is required" >&2
  fi
  exit "$original_status"
}

trap 'rollback_cutover $?' ERR
trap 'rollback_cutover 130' INT
trap 'rollback_cutover 143' TERM

mv -f -- "$candidate" "$nginx_config"
switch_committed=true
nginx -t
nginx -s reload

public_health_verified=false
for ((attempt = 0; attempt < 10; attempt++)); do
  if container_healthy "$target_container" "$target_upstream" && public_health_ok; then
    public_health_verified=true
    break
  fi
  sleep 2
done
if [[ "$public_health_verified" != true ]]; then
  echo "Public health did not converge on the target slot" >&2
  false
fi

if [[ "$rollback_policy" != forbid ]]; then
  # Nginx no longer assigns new connections to the old slot. Docker sends
  # SIGTERM and gives the server time to drain long-lived SSE/WebSocket workers.
  old_stop_started=true
  docker stop --time 120 "$active_container" >/dev/null
  if [[ "$(docker inspect --format '{{.State.Running}}' "$active_container" 2>/dev/null || true)" == true ]]; then
    echo "Old slot remained running after its graceful stop window" >&2
    false
  fi
fi

trap - ERR INT TERM
echo "Nginx cutover complete: active=$target_container previous=$active_container backup=$backup"
