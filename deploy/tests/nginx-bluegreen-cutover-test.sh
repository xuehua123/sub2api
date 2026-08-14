#!/usr/bin/env bash

set -Eeuo pipefail

REPO_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
CUTOVER_SOURCE="$REPO_ROOT/deploy/sub2api-nginx-bluegreen-cutover.sh"
TEST_ROOT=$(mktemp -d)
trap 'rm -rf -- "$TEST_ROOT"' EXIT

MOCK_BIN="$TEST_ROOT/bin"
MOCK_STATE="$TEST_ROOT/state"
DEPLOY_STATE="$TEST_ROOT/deploy-state"
mkdir -p "$MOCK_BIN" "$MOCK_STATE" "$DEPLOY_STATE"
chmod 0700 "$DEPLOY_STATE"

cat > "$MOCK_BIN/flock" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF

cat > "$MOCK_BIN/sleep" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF

cat > "$MOCK_BIN/nginx" <<'EOF'
#!/usr/bin/env bash
if [[ "${MOCK_NGINX_TEST_FAIL:-false}" == true && "${1:-}" == -t ]]; then
  exit 1
fi
if [[ "${1:-}" == -s && "${2:-}" == reload ]]; then
  reload_count=0
  [[ ! -f "$MOCK_STATE_DIR/nginx_reload_count" ]] || reload_count=$(cat "$MOCK_STATE_DIR/nginx_reload_count")
  reload_count=$((reload_count + 1))
  printf '%s' "$reload_count" > "$MOCK_STATE_DIR/nginx_reload_count"
  if [[ "${MOCK_NGINX_RELOAD_FAIL_ON_CALL:-0}" -eq "$reload_count" ]]; then
    exit 1
  fi
fi
exit 0
EOF

cat > "$MOCK_BIN/pgrep" <<'EOF'
#!/usr/bin/env bash
reload_count=0
[[ ! -f "$MOCK_STATE_DIR/nginx_reload_count" ]] || reload_count=$(cat "$MOCK_STATE_DIR/nginx_reload_count")
if [[ "${MOCK_NGINX_NO_WORKER_TURNOVER:-false}" == true ]]; then
  printf '%s\n' 200
else
  printf '%s\n' "$((200 + reload_count))"
fi
EOF

cat > "$MOCK_BIN/stat" <<'EOF'
#!/usr/bin/env bash
path=${!#}
perl -e '
  @value = stat($ARGV[0]);
  die "stat failed" unless @value;
  printf "%d:%d:%o", $value[4], $value[5], $value[2] & 07777;
' "$path"
EOF

cat > "$MOCK_BIN/chown" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF

cat > "$MOCK_BIN/chmod" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF

cat > "$MOCK_BIN/cp" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
arguments=()
for argument in "$@"; do
  case "$argument" in
    --preserve=*) ;;
    --) ;;
    *) arguments+=("$argument") ;;
  esac
done
exec /bin/cp -p "${arguments[@]}"
EOF

cat > "$MOCK_BIN/readlink" <<'EOF'
#!/usr/bin/env bash
path=${!#}
perl -MCwd=realpath -e 'print realpath($ARGV[0])' "$path"
EOF

cat > "$MOCK_BIN/mv" <<'EOF'
#!/usr/bin/env bash
arguments=()
for argument in "$@"; do
  [[ "$argument" == -- ]] || arguments+=("$argument")
done
exec /bin/mv "${arguments[@]}"
EOF

cat > "$MOCK_BIN/rm" <<'EOF'
#!/usr/bin/env bash
arguments=()
for argument in "$@"; do
  [[ "$argument" == -- ]] || arguments+=("$argument")
done
exec /bin/rm "${arguments[@]}"
EOF

cat > "$MOCK_BIN/curl" <<'EOF'
#!/usr/bin/env bash
url=${!#}
case "$url" in
  https://*) printf '%s' "${MOCK_PUBLIC_STATUS:-200}" ;;
  *:18080/*)
    [[ "$(cat "$MOCK_STATE_DIR/blue_running")" == true ]] && printf '%s' 200 || printf '%s' 503
    ;;
  *:28080/*)
    [[ "$(cat "$MOCK_STATE_DIR/green_running")" == true ]] && printf '%s' 200 || printf '%s' 503
    ;;
  *) printf '%s' 503 ;;
esac
EOF

cat > "$MOCK_BIN/docker" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
command=$1
shift
case "$command" in
  container)
    [[ "${1:-}" == inspect ]]
    container=${2:-}
    color=${container#sub2api-}
    [[ "$(cat "$MOCK_STATE_DIR/${color}_exists")" == true ]]
    ;;
  inspect)
    [[ "$1" == --format ]]
    template=$2
    container=$3
    color=${container#sub2api-}
    case "$template" in
      *'.State.Running'*) cat "$MOCK_STATE_DIR/${color}_running" ;;
      *'.State.Health'*) printf '%s' healthy ;;
      *) exit 90 ;;
    esac
    ;;
  stop)
    container=${!#}
    color=${container#sub2api-}
    printf '%s' false > "$MOCK_STATE_DIR/${color}_running"
    [[ "${MOCK_STOP_FAIL:-false}" != true ]]
    ;;
  start)
    container=$1
    color=${container#sub2api-}
    printf '%s' true > "$MOCK_STATE_DIR/${color}_running"
    printf '%s' true > "$MOCK_STATE_DIR/${color}_exists"
    ;;
  rm)
    container=${!#}
    color=${container#sub2api-}
    [[ "${MOCK_RM_FAIL:-false}" != true ]] || exit 1
    printf '%s' false > "$MOCK_STATE_DIR/${color}_running"
    printf '%s' false > "$MOCK_STATE_DIR/${color}_exists"
    ;;
  *) exit 91 ;;
esac
EOF

chmod +x "$MOCK_BIN"/*

SUBJECT="$TEST_ROOT/cutover.sh"
NGINX_PID_FILE="$TEST_ROOT/nginx.pid"
printf '%s\n' 100 > "$NGINX_PID_FILE"
sed \
  -e "s|readonly STATE_DIR=\"/var/lib/sub2api-deploy\"|readonly STATE_DIR=\"$DEPLOY_STATE\"|" \
  -e "s|readonly STATE_OWNER_UID=\"0\"|readonly STATE_OWNER_UID=\"$(id -u)\"|" \
  -e "s|readonly STATE_OWNER_GID=\"0\"|readonly STATE_OWNER_GID=\"$(id -g)\"|" \
  -e "s|readonly NGINX_PID_FILE=\"/run/nginx.pid\"|readonly NGINX_PID_FILE=\"$NGINX_PID_FILE\"|" \
  "$CUTOVER_SOURCE" > "$SUBJECT"
chmod +x "$SUBJECT"

reset_state() {
  printf '%s' true > "$MOCK_STATE/blue_running"
  printf '%s' true > "$MOCK_STATE/green_running"
  printf '%s' true > "$MOCK_STATE/blue_exists"
  printf '%s' true > "$MOCK_STATE/green_exists"
}

write_active_config() {
  local path=$1
  cat > "$path" <<'EOF'
server {
  location / {
    proxy_pass http://127.0.0.1:18080; # stable application slot
  }
}
EOF
}

run_cutover() {
  local config=$1
  local public_status=${2:-200}
  local stop_fail=${3:-false}
  local rollback_policy=${4:-allow}
  local reload_fail_on_call=${5:-0}
  local no_worker_turnover=${6:-false}
  local rm_fail=${7:-false}
  RUN_LOG="$TEST_ROOT/run-$(basename "$config").log"
  rm -f -- "$MOCK_STATE/nginx_reload_count"
  set +e
  env \
    PATH="$MOCK_BIN:$PATH" \
    MOCK_STATE_DIR="$MOCK_STATE" \
    MOCK_PUBLIC_STATUS="$public_status" \
    MOCK_STOP_FAIL="$stop_fail" \
    MOCK_NGINX_RELOAD_FAIL_ON_CALL="$reload_fail_on_call" \
    MOCK_NGINX_NO_WORKER_TURNOVER="$no_worker_turnover" \
    MOCK_RM_FAIL="$rm_fail" \
    bash "$SUBJECT" \
      sub2api-blue sub2api-green \
      127.0.0.1:18080 127.0.0.1:28080 \
      https://api.wenrugouai.com/health "$config" "$rollback_policy" > "$RUN_LOG" 2>&1
  RUN_STATUS=$?
  set -e
}

[[ "$(bash "$SUBJECT" --protocol)" == sub2api-nginx-bluegreen-cutover-v2 ]]

success_config="$TEST_ROOT/success.conf"
reset_state
write_active_config "$success_config"
run_cutover "$success_config"
[[ "$RUN_STATUS" -eq 0 ]]
grep -Fq 'proxy_pass http://127.0.0.1:28080;' "$success_config"
[[ "$(cat "$MOCK_STATE/blue_running")" == false ]]
[[ "$(cat "$MOCK_STATE/green_running")" == true ]]
find "$TEST_ROOT" -name 'success.conf.pre-cutover-*' -type f | grep -q .

# Public validation happens before the old slot is stopped. A failure restores
# the exact previous Nginx file and leaves the old slot healthy.
public_failure_config="$TEST_ROOT/public-failure.conf"
reset_state
write_active_config "$public_failure_config"
run_cutover "$public_failure_config" 503
[[ "$RUN_STATUS" -ne 0 ]]
grep -Fq 'proxy_pass http://127.0.0.1:18080;' "$public_failure_config"
[[ "$(cat "$MOCK_STATE/blue_running")" == true ]]
grep -Fq 'automatic rollback could not be verified' "$RUN_LOG"

# A rollback is not complete until the target container is actually gone.
target_removal_failure_config="$TEST_ROOT/target-removal-failure.conf"
reset_state
write_active_config "$target_removal_failure_config"
run_cutover "$target_removal_failure_config" 200 true allow 0 false true
[[ "$RUN_STATUS" -ne 0 ]]
grep -Fq 'proxy_pass http://127.0.0.1:18080;' "$target_removal_failure_config"
[[ "$(cat "$MOCK_STATE/blue_running")" == true ]]
[[ "$(cat "$MOCK_STATE/green_running")" == true ]]
grep -Fq 'automatic rollback could not remove the target' "$RUN_LOG"

# A successful reload command is not enough: without a new worker, health can
# still be served by the old configuration and the old slot must stay alive.
no_worker_turnover_config="$TEST_ROOT/no-worker-turnover.conf"
reset_state
write_active_config "$no_worker_turnover_config"
run_cutover "$no_worker_turnover_config" 200 false allow 0 true
[[ "$RUN_STATUS" -ne 0 ]]
grep -Fq 'proxy_pass http://127.0.0.1:18080;' "$no_worker_turnover_config"
[[ "$(cat "$MOCK_STATE/blue_running")" == true ]]
grep -Fq 'automatic rollback could not be verified' "$RUN_LOG"

# If graceful stop itself fails after stopping the old container, rollback
# restarts it, reverses Nginx, verifies public health, and removes the target.
stop_failure_config="$TEST_ROOT/stop-failure.conf"
reset_state
write_active_config "$stop_failure_config"
run_cutover "$stop_failure_config" 200 true
[[ "$RUN_STATUS" -ne 0 ]]
grep -Fq 'proxy_pass http://127.0.0.1:18080;' "$stop_failure_config"
[[ "$(cat "$MOCK_STATE/blue_running")" == true ]]
[[ "$(cat "$MOCK_STATE/green_running")" == false ]]
grep -Fq 'previous slot was restored' "$RUN_LOG"

# A failed rollback reload means public 200 may still be served by the target.
# The helper must keep that target alive instead of creating an outage.
rollback_reload_failure_config="$TEST_ROOT/rollback-reload-failure.conf"
reset_state
write_active_config "$rollback_reload_failure_config"
run_cutover "$rollback_reload_failure_config" 200 true allow 2
[[ "$RUN_STATUS" -ne 0 ]]
grep -Fq 'proxy_pass http://127.0.0.1:18080;' "$rollback_reload_failure_config"
[[ "$(cat "$MOCK_STATE/blue_running")" == true ]]
[[ "$(cat "$MOCK_STATE/green_running")" == true ]]
grep -Fq 'automatic rollback could not be verified' "$RUN_LOG"

# Initial irreversible activation has no legal gate=false rollback target. The
# caller stops that slot first; a later failure leaves routing on the enabled
# candidate and requires forward recovery.
irreversible_config="$TEST_ROOT/irreversible.conf"
reset_state
printf '%s' false > "$MOCK_STATE/blue_running"
write_active_config "$irreversible_config"
run_cutover "$irreversible_config" 503 false forbid
[[ "$RUN_STATUS" -ne 0 ]]
grep -Fq 'proxy_pass http://127.0.0.1:28080;' "$irreversible_config"
[[ "$(cat "$MOCK_STATE/blue_running")" == false ]]
[[ "$(cat "$MOCK_STATE/green_running")" == true ]]
grep -Fq 'automatic downgrade is forbidden' "$RUN_LOG"

echo "nginx blue-green cutover behavior tests passed"
