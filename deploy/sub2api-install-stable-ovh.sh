#!/usr/bin/env bash

set -Eeuo pipefail

umask 077

readonly PACKAGE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
readonly DEPLOY_SCRIPT_NAME="sub2api-deploy-stable.sh"
readonly CUTOVER_SCRIPT_NAME="sub2api-nginx-bluegreen-cutover.sh"
readonly BOOTSTRAP_SCRIPT_NAME="sub2api-deploy-state-bootstrap.sh"
readonly DEPLOY_SCRIPT_SHA256="ee76ee63b61c65dddbd56833e13d4a37391cfdc007044e55ca60cf2a6c4287d5"
readonly CUTOVER_SCRIPT_SHA256="1d4f8b7b66d09e6c6968fc20173e58d46ed8967ddcc92eddf164b005e408b1e4"
readonly BOOTSTRAP_SCRIPT_SHA256="a5c90693eeafc565d4601238fdefe979f481d3bd8a2ebf8a0b61d4975b14cf06"
readonly DEPLOY_SCRIPT="/usr/local/sbin/sub2api-deploy-stable"
readonly CUTOVER_SCRIPT="/usr/local/sbin/sub2api-nginx-bluegreen-cutover"
readonly BOOTSTRAP_SCRIPT="/usr/local/sbin/sub2api-deploy-state-bootstrap"
readonly DEPLOY_DIR="/srv/sub2api-migration/incoming"
readonly COMPOSE_FILE="${DEPLOY_DIR}/docker-compose.migration.yml"
readonly COMPOSE_ENV_FILE="${DEPLOY_DIR}/sub2api.env"
readonly STATE_DIR="/var/lib/sub2api-deploy"
readonly INSTALL_LOCK_DIR="/run/sub2api-stable-install"
readonly INSTALL_LOCK_FILE="${INSTALL_LOCK_DIR}/installer.lock"
readonly DEPLOY_LOCK_FILE="${STATE_DIR}/stable-deploy.lock"
readonly CUTOVER_LOCK_FILE="${STATE_DIR}/nginx-bluegreen-cutover.lock"
readonly PUBLIC_CONFIG="/etc/nginx/sites-available/sub2api-public.conf"
readonly WG_CONFIG="/etc/nginx/sites-available/sub2api-wg.conf"
readonly PUBLIC_ENABLED="/etc/nginx/sites-enabled/sub2api-public.conf"
readonly WG_ENABLED="/etc/nginx/sites-enabled/sub2api-wg.conf"
readonly ACTIVE_UPSTREAM_CONFIG="/etc/nginx/snippets/sub2api-active-upstream.conf"
readonly PUBLIC_HEALTH_URL="https://api.wenrugouai.com/health"
readonly PUBLIC_HEALTH_RESOLVE="api.wenrugouai.com:443:40.160.58.167"
readonly WG_HEALTH_URL="http://10.254.254.2/health"
readonly NGINX_PID_FILE="/run/nginx.pid"

mode=${1:---install}
case "$mode" in
  --preflight|--install) ;;
  *) echo "Usage: $0 [--preflight|--install]" >&2; exit 2 ;;
esac
[[ "$#" -le 1 ]] || { echo "Usage: $0 [--preflight|--install]" >&2; exit 2; }

if [[ "$(id -u)" != 0 || "$(id -g)" != 0 ]]; then
  echo "OVH stable installer must run as root" >&2
  exit 1
fi

for command_name in awk cmp curl docker flock gh grep jq nginx pgrep readlink sha256sum stat sync; do
  command -v "$command_name" >/dev/null 2>&1 || {
    echo "Required command is missing: $command_name" >&2
    exit 1
  }
done

gh_version="$(gh --version | awk 'NR == 1 { print $3 }')"
IFS=. read -r gh_major gh_minor gh_patch <<< "$gh_version"
if [[ ! "$gh_version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] ||
  ((gh_major < 2 || (gh_major == 2 && gh_minor < 97))); then
  echo "GitHub CLI 2.97.0 or newer is required" >&2
  exit 1
fi
if ! gh attestation verify --help 2>&1 | grep -Fq -- '--source-digest'; then
  echo "Installed GitHub CLI does not support required attestation policy flags" >&2
  exit 1
fi

validate_root_file() {
  local path=$1
  local metadata
  local owner_uid
  local owner_gid
  local file_mode

  if [[ -L "$path" || ! -f "$path" ]]; then
    echo "Expected a regular non-symlink file: $path" >&2
    return 1
  fi
  metadata="$(stat -c '%u:%g:%a' -- "$path")"
  IFS=: read -r owner_uid owner_gid file_mode <<< "$metadata"
  if [[ "$owner_uid" != 0 || "$owner_gid" != 0 ]] ||
    [[ ! "$file_mode" =~ ^[0-7]{3,4}$ ]] || (( (8#$file_mode & 0022) != 0 )); then
    echo "File must be root-owned and not group/other writable: $path" >&2
    return 1
  fi
}

validate_root_directory() {
  local path=$1
  local metadata
  local owner_uid
  local owner_gid
  local directory_mode

  if [[ -L "$path" || ! -d "$path" ]]; then
    echo "Expected a regular non-symlink directory: $path" >&2
    return 1
  fi
  metadata="$(stat -c '%u:%g:%a' -- "$path")"
  IFS=: read -r owner_uid owner_gid directory_mode <<< "$metadata"
  if [[ "$owner_uid" != 0 || "$owner_gid" != 0 ]] ||
    [[ ! "$directory_mode" =~ ^[0-7]{3,4}$ ]] || (( (8#$directory_mode & 0022) != 0 )); then
    echo "Directory must be root-owned and not group/other writable: $path" >&2
    return 1
  fi
}

validate_root_directory_chain() {
  local current=$1
  local parent
  [[ "$current" == /* ]] || return 2
  while true; do
    validate_root_directory "$current" || return 1
    [[ "$current" == / ]] && break
    parent="$(dirname -- "$current")"
    [[ "$parent" != "$current" ]] || return 1
    current=$parent
  done
}

validate_root_file_and_parents() {
  local path=$1
  validate_root_file "$path" || return 1
  validate_root_directory_chain "$(dirname -- "$path")" || return 1
}

prepare_lock_file() {
  local path=$1
  if [[ -L "$path" || ( -e "$path" && ! -f "$path" ) ]]; then
    echo "Installer lock path is unsafe: $path" >&2
    return 1
  fi
  if [[ ! -e "$path" ]]; then
    (set -o noclobber; : > "$path") 2>/dev/null || true
  fi
  if [[ -L "$path" || ! -f "$path" || "$(stat -c '%u:%g:%a' -- "$path")" != 0:0:600 ]]; then
    echo "Installer lock must be root-owned, non-symlink, and mode 0600: $path" >&2
    return 1
  fi
}

validate_root_directory_chain "$PACKAGE_DIR"
for package_file in "$DEPLOY_SCRIPT_NAME" "$CUTOVER_SCRIPT_NAME" "$BOOTSTRAP_SCRIPT_NAME"; do
  validate_root_file "${PACKAGE_DIR}/${package_file}"
done

source_snapshot="$(mktemp -d /run/sub2api-stable-install-source.XXXXXX)"
chmod 0700 "$source_snapshot"
install -o root -g root -m 0600 -- "${PACKAGE_DIR}/${DEPLOY_SCRIPT_NAME}" "${source_snapshot}/${DEPLOY_SCRIPT_NAME}"
install -o root -g root -m 0600 -- "${PACKAGE_DIR}/${CUTOVER_SCRIPT_NAME}" "${source_snapshot}/${CUTOVER_SCRIPT_NAME}"
install -o root -g root -m 0600 -- "${PACKAGE_DIR}/${BOOTSTRAP_SCRIPT_NAME}" "${source_snapshot}/${BOOTSTRAP_SCRIPT_NAME}"
readonly DEPLOY_SCRIPT_SOURCE="${source_snapshot}/${DEPLOY_SCRIPT_NAME}"
readonly CUTOVER_SCRIPT_SOURCE="${source_snapshot}/${CUTOVER_SCRIPT_NAME}"
readonly BOOTSTRAP_SCRIPT_SOURCE="${source_snapshot}/${BOOTSTRAP_SCRIPT_NAME}"

public_candidate=""
wg_candidate=""
include_candidate=""
deploy_candidate=""
cutover_candidate=""
bootstrap_candidate=""
compose_json=""
nginx_dump=""
cleanup_ephemeral() {
  rm -rf -- "$source_snapshot"
  for path in "$public_candidate" "$wg_candidate" "$include_candidate" \
    "$deploy_candidate" "$cutover_candidate" "$bootstrap_candidate" "$compose_json" "$nginx_dump"; do
    [[ -z "$path" ]] || rm -f -- "$path"
  done
}
trap cleanup_ephemeral EXIT

for source in "$DEPLOY_SCRIPT_SOURCE" "$CUTOVER_SCRIPT_SOURCE" "$BOOTSTRAP_SCRIPT_SOURCE"; do
  bash -n "$source"
done
[[ "$(sha256sum -- "$DEPLOY_SCRIPT_SOURCE" | awk '{print $1}')" == "$DEPLOY_SCRIPT_SHA256" ]] || {
  echo "Stable deploy source does not match the reviewed repository hash" >&2
  exit 1
}
[[ "$(sha256sum -- "$CUTOVER_SCRIPT_SOURCE" | awk '{print $1}')" == "$CUTOVER_SCRIPT_SHA256" ]] || {
  echo "Cutover source does not match the reviewed repository hash" >&2
  exit 1
}
[[ "$(sha256sum -- "$BOOTSTRAP_SCRIPT_SOURCE" | awk '{print $1}')" == "$BOOTSTRAP_SCRIPT_SHA256" ]] || {
  echo "State bootstrap source does not match the reviewed repository hash" >&2
  exit 1
}
grep -Fq "readonly CUTOVER_SCRIPT_SHA256=\"${CUTOVER_SCRIPT_SHA256}\"" "$DEPLOY_SCRIPT_SOURCE" || {
  echo "Stable deploy source does not pin the reviewed cutover hash" >&2
  exit 1
}
[[ "$(bash "$CUTOVER_SCRIPT_SOURCE" --protocol)" == sub2api-nginx-bluegreen-cutover-v2 ]] || {
  echo "Cutover source protocol mismatch" >&2
  exit 1
}

if [[ -L "$INSTALL_LOCK_DIR" || ( -e "$INSTALL_LOCK_DIR" && ! -d "$INSTALL_LOCK_DIR" ) ]]; then
  echo "OVH installer lock directory is unsafe: $INSTALL_LOCK_DIR" >&2
  exit 1
fi
if [[ ! -e "$INSTALL_LOCK_DIR" ]]; then
  install -d -o root -g root -m 0700 -- "$INSTALL_LOCK_DIR"
fi
[[ "$(stat -c '%u:%g:%a' -- "$INSTALL_LOCK_DIR")" == 0:0:700 ]] || {
  echo "OVH installer lock directory must be root-owned with mode 0700" >&2
  exit 1
}
validate_root_directory_chain "$INSTALL_LOCK_DIR"
prepare_lock_file "$INSTALL_LOCK_FILE"
exec 7>>"$INSTALL_LOCK_FILE"
flock -n 7 || { echo "Another OVH installer is already running" >&2; exit 1; }

if [[ -L "$STATE_DIR" || ( -e "$STATE_DIR" && ! -d "$STATE_DIR" ) ]]; then
  echo "Deployment state path is unsafe: $STATE_DIR" >&2
  exit 1
fi
if [[ ! -e "$STATE_DIR" ]]; then
  install -d -o root -g root -m 0700 -- "$STATE_DIR"
fi
[[ "$(stat -c '%u:%g:%a' -- "$STATE_DIR")" == 0:0:700 ]] || {
  echo "Deployment state directory must be root-owned with mode 0700" >&2
  exit 1
}
prepare_lock_file "$DEPLOY_LOCK_FILE"
prepare_lock_file "$CUTOVER_LOCK_FILE"
exec 8>>"$DEPLOY_LOCK_FILE"
flock -n 8 || { echo "A stable deployment or bootstrap is already running" >&2; exit 1; }
exec 9>>"$CUTOVER_LOCK_FILE"
flock -n 9 || { echo "An Nginx cutover is already running" >&2; exit 1; }

validate_root_file_and_parents "$PUBLIC_CONFIG"
validate_root_file_and_parents "$WG_CONFIG"
validate_root_file_and_parents "$COMPOSE_FILE"
validate_root_file_and_parents "$COMPOSE_ENV_FILE"
validate_root_directory_chain /usr/local/sbin
validate_root_directory_chain /etc/nginx/sites-enabled
validate_root_directory_chain /etc/nginx/snippets
[[ "$(stat -c '%u:%g:%a' -- "$COMPOSE_ENV_FILE")" == 0:0:600 ]] || {
  echo "Compose environment file must be root-owned with mode 0600: $COMPOSE_ENV_FILE" >&2
  exit 1
}

validate_enabled_links() {
  [[ -L "$PUBLIC_ENABLED" && "$(readlink -f -- "$PUBLIC_ENABLED")" == "$PUBLIC_CONFIG" ]] || {
    echo "Public Nginx enabled link does not resolve to the reviewed site file" >&2
    return 1
  }
  [[ -L "$WG_ENABLED" && "$(readlink -f -- "$WG_ENABLED")" == "$WG_CONFIG" ]] || {
    echo "WireGuard Nginx enabled link does not resolve to the reviewed site file" >&2
    return 1
  }
}

compose_json="$(mktemp /run/sub2api-stable-compose.XXXXXX)"
chmod 0600 "$compose_json"
validate_compose_contract() {
  docker network inspect sub2api-net >/dev/null || return 1
  docker volume inspect sub2api_data >/dev/null || return 1
  docker compose --env-file "$COMPOSE_ENV_FILE" -p sub2api-stable-preflight -f "$COMPOSE_FILE" config --quiet || return 1
  docker compose --env-file "$COMPOSE_ENV_FILE" -p sub2api-stable-preflight -f "$COMPOSE_FILE" config --format json > "$compose_json" || return 1
  jq -e '
    def has_network($service; $network):
      (.services[$service].networks // {}) as $networks |
      if ($networks | type) == "object" then ($networks | has($network))
      elif ($networks | type) == "array" then (($networks | index($network)) != null)
      else false end;
    . as $root |
    (.services | has("sub2api")) and
    (.services | has("sub2api-postgres")) and
    (.services | has("sub2api-redis")) and
    has_network("sub2api"; "sub2api-net") and
    has_network("sub2api-postgres"; "sub2api-net") and
    has_network("sub2api-redis"; "sub2api-net") and
    (.networks["sub2api-net"].name == "sub2api-net") and
    (.volumes.sub2api_data.name == "sub2api_data") and
    any(.services.sub2api.volumes[]?; .type == "volume" and .source == "sub2api_data" and .target == "/app/data") and
    (.services.sub2api.environment.DATABASE_HOST == "sub2api-postgres") and
    ((.services.sub2api.environment.DATABASE_PORT | tostring) == "5432") and
    (.services.sub2api.environment.REDIS_HOST == "sub2api-redis") and
    ((.services.sub2api.environment.REDIS_PORT | tostring) == "6379") and
    ((.services.sub2api.environment.TOTP_ENCRYPTION_KEY | type) == "string") and
    (["DATABASE_PASSWORD", "JWT_SECRET", "REDIS_PASSWORD"] |
      all(.[]; ($root.services.sub2api.environment[.] | type) == "string" and
        ($root.services.sub2api.environment[.] | length) > 0))
  ' "$compose_json" >/dev/null || {
    echo "Rendered OVH Compose topology does not satisfy the reviewed shared data-plane contract" >&2
    return 1
  }
}
validate_compose_contract

color_port() {
  case "$1" in
    blue) printf '%s' 18080 ;;
    green) printf '%s' 28080 ;;
    *) return 2 ;;
  esac
}

slot_inspect() {
  local color=$1
  docker container inspect --format '{{.Id}}|{{.Image}}|{{.State.Running}}|{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "sub2api-${color}"
}

container_healthy() {
  local color=$1
  local state
  local container_id
  local image_id
  local running
  local health
  local port
  state="$(slot_inspect "$color")" || return 1
  IFS='|' read -r container_id image_id running health <<< "$state"
  [[ -n "$container_id" && -n "$image_id" && "$running" == true && "$health" == healthy ]] || return 1
  port="$(color_port "$color")"
  [[ "$(curl --noproxy '*' -sS -o /dev/null -w '%{http_code}' --max-time 10 "http://127.0.0.1:${port}/health")" == 200 ]]
}

discover_active_color() {
  local found=""
  local color
  local state
  local container_id
  local image_id
  local running
  local health
  for color in blue green; do
    state="$(slot_inspect "$color")" || {
      echo "Unable to inspect required stable slot: sub2api-${color}" >&2
      return 1
    }
    IFS='|' read -r container_id image_id running health <<< "$state"
    case "$running" in
      true)
        [[ "$health" == healthy ]] && container_healthy "$color" || {
          echo "Running stable slot is not healthy: sub2api-${color}" >&2
          return 1
        }
        [[ -z "$found" ]] || { echo "Both stable slots are running" >&2; return 1; }
        found=$color
        ;;
      false) ;;
      *) echo "Stable slot has an invalid running state: sub2api-${color}" >&2; return 1 ;;
    esac
  done
  [[ -n "$found" ]] || { echo "No healthy stable slot was found" >&2; return 1; }
  printf '%s' "$found"
}

active_color="$(discover_active_color)"
if [[ "$active_color" == blue ]]; then target_color=green; else target_color=blue; fi
active_port="$(color_port "$active_color")"
target_port="$(color_port "$target_color")"
active_proxy="proxy_pass http://127.0.0.1:${active_port};"
target_proxy="proxy_pass http://127.0.0.1:${target_port};"
include_line="include ${ACTIVE_UPSTREAM_CONFIG};"
active_state="$(slot_inspect "$active_color")"
IFS='|' read -r active_container_id active_image_id _ _ <<< "$active_state"

active_container="sub2api-${active_color}"
[[ "$(docker inspect --format '{{if index .NetworkSettings.Networks "sub2api-net"}}true{{else}}false{{end}}' "$active_container")" == true ]] || {
  echo "Active slot is not connected to sub2api-net" >&2
  exit 1
}
[[ "$(docker inspect --format '{{range .Mounts}}{{if and (eq .Destination "/app/data") (eq .Name "sub2api_data")}}true{{end}}{{end}}' "$active_container")" == true ]] || {
  echo "Active slot does not use the shared sub2api_data volume" >&2
  exit 1
}

count_exact_directive() {
  local file=$1
  local directive=$2
  awk -v expected="$directive" '
    {
      line=$0
      sub(/[[:space:]]*#.*/, "", line)
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", line)
      if (line == expected) count++
    }
    END { print count + 0 }
  ' "$file"
}

verify_nginx_loaded() {
  local phase=$1
  nginx_dump="$(mktemp /run/sub2api-stable-nginx-dump.XXXXXX)" || return 1
  chmod 0600 "$nginx_dump" || return 1
  nginx -T > "$nginx_dump" 2>&1 || return 1
  grep -Fq "# configuration file ${PUBLIC_ENABLED}:" "$nginx_dump" || return 1
  grep -Fq "# configuration file ${WG_ENABLED}:" "$nginx_dump" || return 1
  if [[ "$phase" == before ]]; then
    if grep -Fq "# configuration file ${ACTIVE_UPSTREAM_CONFIG}:" "$nginx_dump"; then return 1; fi
  else
    grep -Fq "# configuration file ${ACTIVE_UPSTREAM_CONFIG}:" "$nginx_dump" || return 1
  fi
  rm -f -- "$nginx_dump" || return 1
  nginx_dump=""
}

verify_nginx_files_before() {
  validate_enabled_links || return 1
  [[ ! -e "$ACTIVE_UPSTREAM_CONFIG" && ! -L "$ACTIVE_UPSTREAM_CONFIG" ]] || {
    echo "Shared upstream include already exists; refusing a non-idempotent bootstrap" >&2
    return 1
  }
  [[ "$(count_exact_directive "$PUBLIC_CONFIG" "$active_proxy")" -eq 1 ]] || return 1
  [[ "$(count_exact_directive "$WG_CONFIG" "$active_proxy")" -eq 2 ]] || return 1
  [[ "$(count_exact_directive "$PUBLIC_CONFIG" "$target_proxy")" -eq 0 ]] || return 1
  [[ "$(count_exact_directive "$WG_CONFIG" "$target_proxy")" -eq 0 ]] || return 1
  [[ "$(count_exact_directive "$PUBLIC_CONFIG" "$include_line")" -eq 0 ]] || return 1
  [[ "$(count_exact_directive "$WG_CONFIG" "$include_line")" -eq 0 ]] || return 1
  verify_nginx_loaded before || return 1
}

verify_nginx_files_after() {
  validate_enabled_links || return 1
  validate_root_file_and_parents "$ACTIVE_UPSTREAM_CONFIG" || return 1
  [[ "$(count_exact_directive "$PUBLIC_CONFIG" "$active_proxy")" -eq 0 ]] || return 1
  [[ "$(count_exact_directive "$WG_CONFIG" "$active_proxy")" -eq 0 ]] || return 1
  [[ "$(count_exact_directive "$PUBLIC_CONFIG" "$target_proxy")" -eq 0 ]] || return 1
  [[ "$(count_exact_directive "$WG_CONFIG" "$target_proxy")" -eq 0 ]] || return 1
  [[ "$(count_exact_directive "$PUBLIC_CONFIG" "$include_line")" -eq 1 ]] || return 1
  [[ "$(count_exact_directive "$WG_CONFIG" "$include_line")" -eq 2 ]] || return 1
  [[ "$(count_exact_directive "$ACTIVE_UPSTREAM_CONFIG" "$active_proxy")" -eq 1 ]] || return 1
  [[ "$(count_exact_directive "$ACTIVE_UPSTREAM_CONFIG" "$target_proxy")" -eq 0 ]] || return 1
  verify_nginx_loaded after || return 1
}

public_health_ok() {
  [[ "$(curl --noproxy '*' --resolve "$PUBLIC_HEALTH_RESOLVE" -sS -o /dev/null -w '%{http_code}' --max-time 10 "$PUBLIC_HEALTH_URL")" == 200 ]]
}

wg_health_ok() {
  [[ "$(curl --noproxy '*' -H 'Host: api.wenrugouai.com' -sS -o /dev/null -w '%{http_code}' --max-time 10 "$WG_HEALTH_URL")" == 200 ]]
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

verify_nginx_files_before
public_health_ok || { echo "Direct OVH public health baseline failed" >&2; exit 1; }
wg_health_ok || { echo "WireGuard health baseline failed" >&2; exit 1; }

public_hash="$(sha256sum -- "$PUBLIC_CONFIG" | awk '{print $1}')"
wg_hash="$(sha256sum -- "$WG_CONFIG" | awk '{print $1}')"
compose_hash="$(sha256sum -- "$COMPOSE_FILE" | awk '{print $1}')"
env_hash="$(sha256sum -- "$COMPOSE_ENV_FILE" | awk '{print $1}')"

rewrite_nginx_config() {
  local source=$1
  local destination=$2
  local expected_count=$3
  awk -v active="$active_proxy" -v replacement="$include_line" -v expected_count="$expected_count" '
    {
      original=$0
      line=$0
      sub(/[[:space:]]*#.*/, "", line)
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", line)
      if (line == active) {
        position=index(original, active)
        original=substr(original, 1, position - 1) replacement substr(original, position + length(active))
        matches++
      }
      print original
    }
    END { if (matches != expected_count) exit 42 }
  ' "$source" > "$destination"
  chown --reference="$source" -- "$destination"
  chmod --reference="$source" -- "$destination"
}

public_candidate="$(mktemp /etc/nginx/sites-available/.sub2api-public.conf.ovh.XXXXXX)"
wg_candidate="$(mktemp /etc/nginx/sites-available/.sub2api-wg.conf.ovh.XXXXXX)"
include_candidate="$(mktemp /etc/nginx/snippets/.sub2api-active-upstream.conf.ovh.XXXXXX)"
rewrite_nginx_config "$PUBLIC_CONFIG" "$public_candidate" 1
rewrite_nginx_config "$WG_CONFIG" "$wg_candidate" 2
printf '%s\n' "$active_proxy" > "$include_candidate"
chown root:root "$include_candidate"
chmod 0644 "$include_candidate"

revalidate_precommit() {
  [[ "$(discover_active_color)" == "$active_color" ]] || return 1
  local current_state
  local current_container_id
  local current_image_id
  current_state="$(slot_inspect "$active_color")" || return 1
  IFS='|' read -r current_container_id current_image_id _ _ <<< "$current_state"
  [[ "$current_container_id" == "$active_container_id" && "$current_image_id" == "$active_image_id" ]] || return 1
  [[ "$(sha256sum -- "$PUBLIC_CONFIG" | awk '{print $1}')" == "$public_hash" ]] || return 1
  [[ "$(sha256sum -- "$WG_CONFIG" | awk '{print $1}')" == "$wg_hash" ]] || return 1
  [[ "$(sha256sum -- "$COMPOSE_FILE" | awk '{print $1}')" == "$compose_hash" ]] || return 1
  [[ "$(sha256sum -- "$COMPOSE_ENV_FILE" | awk '{print $1}')" == "$env_hash" ]] || return 1
  verify_nginx_files_before || return 1
  validate_compose_contract || return 1
  container_healthy "$active_color" || return 1
  public_health_ok || return 1
  wg_health_ok || return 1
}

revalidate_precommit || {
  echo "OVH topology or configuration changed during installer preflight" >&2
  exit 1
}

if [[ "$mode" == --preflight ]]; then
  echo "OVH stable installer preflight passed: active=${active_color} target=${target_color}"
  exit 0
fi

timestamp="$(date -u '+%Y%m%dT%H%M%SZ')"
backup_dir="$(mktemp -d "/root/sub2api-stable-install-${timestamp}.XXXXXX")"
chmod 0700 "$backup_dir"

deploy_existed=false
cutover_existed=false
bootstrap_existed=false
for record in \
  "DEPLOY_SCRIPT:$DEPLOY_SCRIPT:deploy_existed" \
  "CUTOVER_SCRIPT:$CUTOVER_SCRIPT:cutover_existed" \
  "BOOTSTRAP_SCRIPT:$BOOTSTRAP_SCRIPT:bootstrap_existed"; do
  IFS=: read -r label path flag <<< "$record"
  if [[ -e "$path" || -L "$path" ]]; then
    validate_root_file "$path"
    cp --preserve=mode,ownership -- "$path" "$backup_dir/$(basename "$path").before"
    printf -v "$flag" '%s' true
  fi
done
cp --preserve=mode,ownership -- "$PUBLIC_CONFIG" "$backup_dir/sub2api-public.conf.before"
cp --preserve=mode,ownership -- "$WG_CONFIG" "$backup_dir/sub2api-wg.conf.before"

deploy_candidate="$(mktemp /usr/local/sbin/.sub2api-deploy-stable.ovh.XXXXXX)"
cutover_candidate="$(mktemp /usr/local/sbin/.sub2api-nginx-bluegreen-cutover.ovh.XXXXXX)"
bootstrap_candidate="$(mktemp /usr/local/sbin/.sub2api-deploy-state-bootstrap.ovh.XXXXXX)"
install -o root -g root -m 0755 -- "$DEPLOY_SCRIPT_SOURCE" "$deploy_candidate"
install -o root -g root -m 0755 -- "$CUTOVER_SCRIPT_SOURCE" "$cutover_candidate"
install -o root -g root -m 0755 -- "$BOOTSTRAP_SCRIPT_SOURCE" "$bootstrap_candidate"

nginx_committed=false
scripts_committed=false

restore_path() {
  local destination=$1
  local backup=$2
  local existed=$3
  local restore_candidate
  if [[ "$existed" == true ]]; then
    restore_candidate="$(mktemp "$(dirname "$destination")/.restore.$(basename "$destination").XXXXXX")" || return 1
    cp --preserve=mode,ownership -- "$backup" "$restore_candidate" || return 1
    mv -Tf -- "$restore_candidate" "$destination" || return 1
    cmp -s -- "$backup" "$destination" || return 1
    [[ "$(stat -c '%u:%g:%a' -- "$backup")" == "$(stat -c '%u:%g:%a' -- "$destination")" ]] || return 1
  else
    rm -f -- "$destination" || return 1
    [[ ! -e "$destination" && ! -L "$destination" ]] || return 1
  fi
}

rollback_install() {
  local status=$1
  local rollback_ok=true
  trap - ERR INT TERM
  set +e
  if [[ "$scripts_committed" == true ]]; then
    restore_path "$DEPLOY_SCRIPT" "$backup_dir/$(basename "$DEPLOY_SCRIPT").before" "$deploy_existed" || rollback_ok=false
    restore_path "$CUTOVER_SCRIPT" "$backup_dir/$(basename "$CUTOVER_SCRIPT").before" "$cutover_existed" || rollback_ok=false
    restore_path "$BOOTSTRAP_SCRIPT" "$backup_dir/$(basename "$BOOTSTRAP_SCRIPT").before" "$bootstrap_existed" || rollback_ok=false
  fi
  if [[ "$nginx_committed" == true ]]; then
    restore_path "$PUBLIC_CONFIG" "$backup_dir/sub2api-public.conf.before" true || rollback_ok=false
    restore_path "$WG_CONFIG" "$backup_dir/sub2api-wg.conf.before" true || rollback_ok=false
    rm -f -- "$ACTIVE_UPSTREAM_CONFIG" || rollback_ok=false
    [[ ! -e "$ACTIVE_UPSTREAM_CONFIG" && ! -L "$ACTIVE_UPSTREAM_CONFIG" ]] || rollback_ok=false
    reload_nginx_verified || rollback_ok=false
    verify_nginx_files_before || rollback_ok=false
    container_healthy "$active_color" || rollback_ok=false
    public_health_ok || rollback_ok=false
    wg_health_ok || rollback_ok=false
  fi
  if [[ "$rollback_ok" == true ]]; then
    echo "OVH stable installer failed; previous files were restored and verified. Evidence: $backup_dir" >&2
  else
    echo "OVH stable installer failed and automatic rollback could not be verified; operator intervention is required. Evidence: $backup_dir" >&2
  fi
  exit "$status"
}
trap 'rollback_install $?' ERR
trap 'rollback_install 130' INT
trap 'rollback_install 143' TERM

nginx_committed=true
mv -Tf -- "$include_candidate" "$ACTIVE_UPSTREAM_CONFIG"
include_candidate=""
mv -Tf -- "$public_candidate" "$PUBLIC_CONFIG"
public_candidate=""
mv -Tf -- "$wg_candidate" "$WG_CONFIG"
wg_candidate=""
reload_nginx_verified
verify_nginx_files_after
container_healthy "$active_color"
public_health_ok
wg_health_ok

scripts_committed=true
mv -Tf -- "$cutover_candidate" "$CUTOVER_SCRIPT"
cutover_candidate=""
mv -Tf -- "$bootstrap_candidate" "$BOOTSTRAP_SCRIPT"
bootstrap_candidate=""
mv -Tf -- "$deploy_candidate" "$DEPLOY_SCRIPT"
deploy_candidate=""

[[ "$(stat -c '%u:%g:%a' -- "$DEPLOY_SCRIPT")" == 0:0:755 ]]
[[ "$(stat -c '%u:%g:%a' -- "$CUTOVER_SCRIPT")" == 0:0:755 ]]
[[ "$(stat -c '%u:%g:%a' -- "$BOOTSTRAP_SCRIPT")" == 0:0:755 ]]
[[ "$(sha256sum -- "$DEPLOY_SCRIPT" | awk '{print $1}')" == "$DEPLOY_SCRIPT_SHA256" ]]
[[ "$(sha256sum -- "$CUTOVER_SCRIPT" | awk '{print $1}')" == "$CUTOVER_SCRIPT_SHA256" ]]
[[ "$(sha256sum -- "$BOOTSTRAP_SCRIPT" | awk '{print $1}')" == "$BOOTSTRAP_SCRIPT_SHA256" ]]
[[ "$(bash "$CUTOVER_SCRIPT" --protocol)" == sub2api-nginx-bluegreen-cutover-v2 ]]
bash -n "$DEPLOY_SCRIPT" "$BOOTSTRAP_SCRIPT"

trap - ERR INT TERM
echo "OVH stable deployment scripts installed: active=${active_color} backup=${backup_dir}"
