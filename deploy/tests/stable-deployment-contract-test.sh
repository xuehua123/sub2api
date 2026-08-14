#!/usr/bin/env bash

set -Eeuo pipefail

REPO_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
STABLE_SCRIPT="$REPO_ROOT/deploy/sub2api-deploy-stable.sh"
BOOTSTRAP_SCRIPT="$REPO_ROOT/deploy/sub2api-deploy-state-bootstrap.sh"
TEST_ROOT=$(mktemp -d)

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

cleanup() {
  rm -rf -- "$TEST_ROOT"
}
trap cleanup EXIT

MOCK_BIN="$TEST_ROOT/bin"
mkdir -p "$MOCK_BIN"

cat > "$MOCK_BIN/curl" <<'EOF'
#!/usr/bin/env bash
last_argument=${!#}
if [[ "$last_argument" == https://api.wenrugouai.com/health ]]; then
  printf '%s' "${MOCK_PUBLIC_HEALTH_STATUS:-200}"
else
  printf '%s' 200
fi
EOF

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
if [[ "${1:-}" == -s && "${2:-}" == reload ]]; then
  reload_count=0
  [[ ! -f "$MOCK_STATE_DIR/nginx_reload_count" ]] || reload_count=$(cat "$MOCK_STATE_DIR/nginx_reload_count")
  printf '%s' "$((reload_count + 1))" > "$MOCK_STATE_DIR/nginx_reload_count"
fi
exit 0
EOF

cat > "$MOCK_BIN/pgrep" <<'EOF'
#!/usr/bin/env bash
reload_count=0
[[ ! -f "$MOCK_STATE_DIR/nginx_reload_count" ]] || reload_count=$(cat "$MOCK_STATE_DIR/nginx_reload_count")
printf '%s\n' "$((200 + reload_count))"
EOF

cat > "$MOCK_BIN/sync" <<'EOF'
#!/usr/bin/env bash
exit 0
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

cat > "$MOCK_BIN/sha256sum" <<'EOF'
#!/usr/bin/env bash
if [[ -x /usr/bin/sha256sum ]]; then
  exec /usr/bin/sha256sum "$@"
fi
if [[ "${1:-}" == -- ]]; then
  shift
fi
exec /usr/bin/shasum -a 256 "$@"
EOF

cat > "$MOCK_BIN/jq" <<'EOF'
#!/usr/bin/env python3
import json
import sys

expression = sys.argv[-1]
try:
    data = json.load(sys.stdin)
    item = data[0]
    if "State.Running" in expression:
        value = item["State"]["Running"]
        if not isinstance(value, bool):
            raise ValueError("invalid running state")
        print("true" if value else "false")
    elif "AFFILIATE_REFUND_REVERSAL_ENABLED=" in expression:
        values = item["Config"].get("Env") or []
        if not isinstance(values, list) or not all(isinstance(value, str) for value in values):
            raise ValueError("invalid env")
        matches = [value for value in values if value.startswith("AFFILIATE_REFUND_REVERSAL_ENABLED=")]
        if matches == ["AFFILIATE_REFUND_REVERSAL_ENABLED=false"]:
            print("false")
        else:
            print("unsafe")
    elif ".[0].Image" in expression:
        value = item.get("Image")
        if not isinstance(value, str) or not value:
            raise ValueError("invalid image")
        print(value)
    elif "org.sub2api.capability.payment-reversal-components" in expression:
        value = item.get("Config", {}).get("Labels", {}).get(
            "org.sub2api.capability.payment-reversal-components", ""
        )
        print(value)
    else:
        raise ValueError("unsupported expression")
except Exception as exc:
    print(str(exc), file=sys.stderr)
    sys.exit(1)
EOF

cat > "$MOCK_BIN/install" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
mode=700
path=${!#}
for ((index = 1; index <= $#; index++)); do
  if [[ "${!index}" == -m ]]; then
    mode_index=$((index + 1))
    mode=${!mode_index}
  fi
done
mkdir -p "$path"
/bin/chmod "$mode" "$path"
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

cat > "$MOCK_BIN/gh" <<'EOF'
#!/usr/bin/env bash
if [[ "${1:-}" == --version ]]; then
  printf 'gh version %s (mock)\n' "${MOCK_GH_VERSION:-2.97.0}"
  exit 0
fi
if [[ "$*" == *'attestation verify --help'* ]]; then
  printf '%s\n' '  --source-digest string'
  exit 0
fi
if [[ "${1:-}" == attestation ]]; then
  [[ "${MOCK_ATTESTATION_VALID:-true}" == true ]]
  exit
fi
if [[ "${1:-}" == api ]]; then
  endpoint=${2:-}
  case "$endpoint" in
    */git/ref/tags/*)
      printf '%s\t%s\n' "${MOCK_TAG_OBJECT_TYPE:-commit}" "${MOCK_TAG_OBJECT_SHA:-$MOCK_CANDIDATE_REVISION}"
      ;;
    */git/tags/*)
      printf '%s\t%s\n' "${MOCK_PEELED_TAG_OBJECT_TYPE:-commit}" "${MOCK_PEELED_TAG_OBJECT_SHA:-$MOCK_CANDIDATE_REVISION}"
      ;;
    */releases/tags/*)
      [[ "${MOCK_RELEASE_API_FAIL:-false}" != true ]] || exit 1
      printf '%s\t%s\t%s\n' \
        "${MOCK_RELEASE_TAG:-v${MOCK_CANDIDATE_VERSION:-0.1.172}}" \
        "${MOCK_RELEASE_DRAFT:-false}" \
        "${MOCK_RELEASE_PRERELEASE:-false}"
      ;;
    *) exit 98 ;;
  esac
  exit 0
fi
exit 99
EOF

cat > "$MOCK_BIN/docker" <<'EOF'
#!/usr/bin/env bash

set -Eeuo pipefail

state_value() {
  local color=$1
  local field=$2
  local default_value=$3
  local path="${MOCK_STATE_DIR}/${color}_${field}"
  if [[ -f "$path" ]]; then
    cat "$path"
  else
    printf '%s' "$default_value"
  fi
}

set_state() {
  printf '%s' "$3" > "${MOCK_STATE_DIR}/$1_$2"
}

image_revision() {
  local image=$1
  local color
  if [[ "$image" == "${MOCK_CANDIDATE_REF:-}" || "$image" == "${MOCK_CANDIDATE_ID:-}" ]]; then
    printf '%s' "$MOCK_CANDIDATE_REVISION"
    return
  fi
  for color in blue green; do
    if [[ "$(state_value "$color" image_id '')" == "$image" ]]; then
      state_value "$color" revision '<no value>'
      return
    fi
  done
  printf '%s' '<no value>'
}

image_payment_reversal_components() {
  local image=$1
  local color
  if [[ "$image" == "${MOCK_CANDIDATE_REF:-}" || "$image" == "${MOCK_CANDIDATE_ID:-}" ]]; then
    printf '%s' "${MOCK_CANDIDATE_PAYMENT_COMPONENTS:-0}"
    return
  fi
  for color in blue green; do
    if [[ "$(state_value "$color" image_id '')" == "$image" ]]; then
      state_value "$color" payment_components '<no value>'
      return
    fi
  done
  printf '%s' '<no value>'
}

if [[ "${1:-}" == --config ]]; then
  shift 2
  if [[ "${1:-}" == login ]]; then
    cat >/dev/null
    exit 0
  fi
fi

command=${1:-}
shift || true

case "$command" in
  container)
    [[ "${1:-}" == inspect ]] || exit 89
    container=${2:-}
    color=${container#sub2api-}
    [[ "${MOCK_CONTAINER_INSPECT_FAIL:-}" != "$color" ]] || exit 88
    [[ "$(state_value "$color" exists true)" == true ]] || exit 1
    running="$(state_value "$color" running false)"
    image_id="$(state_value "$color" image_id '')"
    gate_value="$(state_value "$color" gate '')"
    MOCK_JSON_RUNNING="$running" MOCK_JSON_IMAGE="$image_id" MOCK_JSON_GATE="$gate_value" \
      MOCK_JSON_INVALID_ENV="${MOCK_CONTAINER_INSPECT_INVALID_ENV:-}" MOCK_JSON_COLOR="$color" \
      python3 - <<'PY'
import json
import os

gate = os.environ.get("MOCK_JSON_GATE", "")
if os.environ.get("MOCK_JSON_INVALID_ENV") == os.environ.get("MOCK_JSON_COLOR"):
    env = {"invalid": True}
elif gate:
    env = ["AFFILIATE_REFUND_REVERSAL_ENABLED=" + gate]
else:
    env = []
print(json.dumps([{
    "State": {"Running": os.environ["MOCK_JSON_RUNNING"] == "true"},
    "Config": {"Env": env},
    "Image": os.environ.get("MOCK_JSON_IMAGE", ""),
}]))
PY
    ;;
  inspect)
    [[ "${1:-}" == --format ]] || exit 90
    template=$2
    container=$3
    color=${container#sub2api-}
    case "$template" in
      *'range .Config.Env'*)
        gate_value="$(state_value "$color" gate '')"
        if [[ -n "$gate_value" ]]; then
          printf 'AFFILIATE_REFUND_REVERSAL_ENABLED=%s\n' "$gate_value"
        fi
        ;;
      *'.State.Running'*)
        state_value "$color" running false
        ;;
      *'.State.Health'*)
        state_value "$color" health none
        ;;
      *'.Config.Image'*)
        state_value "$color" image_ref ''
        ;;
      *'.Image'*)
        state_value "$color" image_id ''
        ;;
      *)
        echo "Unsupported docker inspect template: $template" >&2
        exit 91
        ;;
    esac
    ;;
  image)
    [[ "${1:-}" == inspect ]] || exit 92
    if [[ "${2:-}" != --format ]]; then
      image=${2:-}
      [[ "${MOCK_IMAGE_INSPECT_FAIL:-false}" != true ]] || exit 92
      capability="$(image_payment_reversal_components "$image")"
      MOCK_JSON_CAPABILITY="$capability" python3 - <<'PY'
import json
import os
print(json.dumps([{"Config": {"Labels": {
    "org.sub2api.capability.payment-reversal-components": os.environ.get("MOCK_JSON_CAPABILITY", "")
}}}]))
PY
      exit 0
    fi
    template=$3
    image=$4
    case "$template" in
      *'range .RepoDigests'*)
        printf '%s\n' "$MOCK_CANDIDATE_REF"
        ;;
      *'join .RepoDigests'*)
        printf '%s' "$image"
        ;;
      *'.Id'*)
        printf '%s' "$MOCK_CANDIDATE_ID"
        ;;
      *'org.opencontainers.image.revision'*)
        image_revision "$image"
        ;;
      *'org.opencontainers.image.version'*)
        printf '%s' "${MOCK_CANDIDATE_VERSION:-0.1.172}"
        ;;
      *'org.sub2api.build.trusted-ci'*)
        printf '%s' "$MOCK_CANDIDATE_TRUSTED_CI"
        ;;
      *'org.sub2api.capability.affiliate-refund-reversal'*)
        printf '%s' "$MOCK_CANDIDATE_CAPABILITY"
        ;;
      *'org.sub2api.capability.payment-reversal-components'*)
        image_payment_reversal_components "$image"
        ;;
      *)
        echo "Unsupported docker image inspect template: $template" >&2
        exit 93
        ;;
    esac
    ;;
  compose)
    candidate=''
    compose_env_file=''
    compose_project=''
    previous=''
    action=''
    for argument in "$@"; do
      if [[ "$previous" == -f ]]; then
        candidate=$argument
      elif [[ "$previous" == --env-file ]]; then
        compose_env_file=$argument
      elif [[ "$previous" == -p ]]; then
        compose_project=$argument
      fi
      case "$argument" in
        pull|up) action=$argument ;;
      esac
      previous=$argument
    done
    [[ -n "$candidate" && -n "$action" ]] || exit 94
    [[ -f "$compose_env_file" ]] || exit 97
    [[ "$compose_project" =~ ^sub2api-stable-(blue|green)$ ]] || exit 98
    grep -Fq '  sub2api-net:' "$candidate" || exit 99
    grep -Fq '    name: sub2api-net' "$candidate" || exit 100
    if [[ "$action" == pull ]]; then
      exit 0
    fi
    container=$(awk '$1 == "container_name:" { print $2 }' "$candidate")
    image_ref=$(awk '$1 == "image:" { print $2 }' "$candidate")
    gate_value=$(awk '$1 == "AFFILIATE_REFUND_REVERSAL_ENABLED:" { gsub(/"/, "", $2); print $2 }' "$candidate")
    color=${container#sub2api-}
    if [[ "${MOCK_REQUIRE_OLD_STOPPED:-false}" == true ]]; then
      if [[ "$color" == blue ]]; then old_color=green; else old_color=blue; fi
      [[ "$(state_value "$old_color" running false)" != true ]] || exit 96
    fi
    set_state "$color" running true
    set_state "$color" exists true
    set_state "$color" health "${MOCK_CANDIDATE_HEALTH:-healthy}"
    set_state "$color" image_ref "$image_ref"
    set_state "$color" image_id "$MOCK_CANDIDATE_ID"
    set_state "$color" revision "$MOCK_CANDIDATE_REVISION"
    set_state "$color" gate "$gate_value"
    set_state "$color" payment_components "${MOCK_CANDIDATE_PAYMENT_COMPONENTS:-0}"
    ;;
  run)
    printf 'Sub2API %s (commit: %s, built: 2026-08-08T00:00:00Z)\n' \
      "${MOCK_BINARY_VERSION:-${MOCK_CANDIDATE_VERSION:-0.1.172}}" \
      "${MOCK_BINARY_COMMIT:-$MOCK_CANDIDATE_REVISION}"
    ;;
  rm)
    container=''
    for argument in "$@"; do
      case "$argument" in
        -*) ;;
        *) container=$argument ;;
      esac
    done
    if [[ -n "$container" ]]; then
      color=${container#sub2api-}
      set_state "$color" running false
      set_state "$color" exists false
    fi
    ;;
  stop)
    container=${!#}
    color=${container#sub2api-}
    set_state "$color" running false
    ;;
  start)
    container=$1
    color=${container#sub2api-}
    set_state "$color" running true
    set_state "$color" exists true
    ;;
  info)
    exit 0
    ;;
  logs)
    ;;
  *)
    echo "Unsupported docker command: $command $*" >&2
    exit 95
    ;;
esac
EOF

chmod +x "$MOCK_BIN"/*

OLD_IMAGE_ID="sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
OLD_REVISION="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
RELEASE_IMAGE_ID="sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
RELEASE_REVISION="bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
NEXT_IMAGE_ID="sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
NEXT_REVISION="cccccccccccccccccccccccccccccccccccccccc"

new_scenario() {
  local name=$1
  local active_gate=${2:-}
  local root="$TEST_ROOT/$name"
  local platform="$root/platform"
  local deploy_state="$root/deploy-state"
  local state="$root/state"
  local nginx_config="$root/platform.conf"
  local cutover="$root/cutover"
  local lock_file="$root/deploy.lock"

  mkdir -p "$platform" "$state" "$deploy_state"
  chmod 0700 "$deploy_state"
  printf '%s\n' \
    'state_version=1' \
    'affiliate_refund_reversal_state=absent' \
    'updated_at=2026-08-08T00:00:00Z' > "$deploy_state/affiliate-refund-reversal-state"
  chmod 0600 "$deploy_state/affiliate-refund-reversal-state"
  printf '%s\n' \
    'state_version=1' \
    'payment_reversal_components_state=absent' \
    'updated_at=2026-08-08T00:00:00Z' > "$deploy_state/payment-reversal-components-state"
  chmod 0600 "$deploy_state/payment-reversal-components-state"
  printf '%s\n' 'services: {}' > "$platform/docker-compose.migration.yml"
  printf '%s\n' '# fixture intentionally contains no secrets' > "$platform/sub2api.env"
  chmod 0600 "$platform/sub2api.env"
  printf '%s\n' 'proxy_pass http://127.0.0.1:18080;' > "$nginx_config"
  printf '%s' true > "$state/blue_running"
  printf '%s' true > "$state/blue_exists"
  printf '%s' healthy > "$state/blue_health"
  printf '%s' 'ghcr.io/xuehua123/sub2api:sha-aaaaaaa' > "$state/blue_image_ref"
  printf '%s' "$OLD_IMAGE_ID" > "$state/blue_image_id"
  printf '%s' "$OLD_REVISION" > "$state/blue_revision"
  if [[ -n "$active_gate" ]]; then
    printf '%s' "$active_gate" > "$state/blue_gate"
  fi
  printf '%s' false > "$state/green_running"
  printf '%s' true > "$state/green_exists"
  printf '%s\n' 100 > "$state/nginx.pid"

  cat > "$cutover" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
if [[ "${1:-}" == --protocol ]]; then
  printf '%s\n' 'sub2api-nginx-bluegreen-cutover-v2'
  exit 0
fi
active_color=${1#sub2api-}
if [[ "${MOCK_LEAVE_OLD_RUNNING:-false}" != true ]]; then
  printf '%s' false > "${MOCK_STATE_DIR}/${active_color}_running"
fi
printf 'proxy_pass http://%s;\n' "$4" > "$6"
EOF
  chmod +x "$cutover"

  sed \
    -e "s|readonly DEPLOY_DIR=\"/srv/sub2api-migration/incoming\"|readonly DEPLOY_DIR=\"$platform\"|" \
    -e "s|readonly STATE_DIR=\"/var/lib/sub2api-deploy\"|readonly STATE_DIR=\"$deploy_state\"|" \
    -e "s|readonly STATE_OWNER_UID=\"0\"|readonly STATE_OWNER_UID=\"$(id -u)\"|" \
    -e "s|readonly STATE_OWNER_GID=\"0\"|readonly STATE_OWNER_GID=\"$(id -g)\"|" \
    -e "s|readonly NGINX_CONFIG=\"/etc/nginx/snippets/sub2api-active-upstream.conf\"|readonly NGINX_CONFIG=\"$nginx_config\"|" \
    -e "s|readonly NGINX_PID_FILE=\"/run/nginx.pid\"|readonly NGINX_PID_FILE=\"$state/nginx.pid\"|" \
    -e "s|readonly CUTOVER_SCRIPT=\"/usr/local/sbin/sub2api-nginx-bluegreen-cutover\"|readonly CUTOVER_SCRIPT=\"$cutover\"|" \
    -e "s|readonly CUTOVER_SCRIPT_SHA256=\"[0-9a-f]*\"|readonly CUTOVER_SCRIPT_SHA256=\"$(sha256_file "$cutover")\"|" \
    -e "s|readonly CUTOVER_OWNER_UID=\"0\"|readonly CUTOVER_OWNER_UID=\"$(id -u)\"|" \
    -e "s|readonly CUTOVER_OWNER_GID=\"0\"|readonly CUTOVER_OWNER_GID=\"$(id -g)\"|" \
    "$STABLE_SCRIPT" > "$root/subject.sh"
  chmod +x "$root/subject.sh"
  printf '%s' "$root"
}

RUN_STATUS=0
RUN_LOG=''
run_deploy() {
  local root=$1
  local image_tag=$2
  local stage=$3
  local candidate_image_id=$4
  local candidate_revision=$5
  local candidate_capability=$6
  local leave_old_running=${7:-false}
  local candidate_health=${8:-healthy}
  local public_health_status=${9:-200}
  local candidate_trusted_ci=${10:-1}
  local attestation_valid=${11:-true}
  local gh_version=${12:-2.97.0}
  local candidate_payment_components=${13:-0}
  local candidate_ref
  if [[ "$image_tag" == sha256:* ]]; then
    candidate_ref="ghcr.io/xuehua123/sub2api@${image_tag}"
  else
    candidate_ref="$image_tag"
  fi

  RUN_LOG="$root/run-${stage}-${image_tag}.log"
  set +e
  printf 'sub2api-deploy-v2 %s\n%s\n%s\n%s\n' "$image_tag" test-user test-token "$stage" |
    env \
      PATH="$MOCK_BIN:$PATH" \
      MOCK_STATE_DIR="$root/state" \
      MOCK_CANDIDATE_REF="$candidate_ref" \
      MOCK_CANDIDATE_ID="$candidate_image_id" \
      MOCK_CANDIDATE_REVISION="$candidate_revision" \
      MOCK_CANDIDATE_VERSION="${MOCK_CANDIDATE_VERSION:-0.1.172}" \
      MOCK_BINARY_VERSION="${MOCK_BINARY_VERSION:-${MOCK_CANDIDATE_VERSION:-0.1.172}}" \
      MOCK_BINARY_COMMIT="${MOCK_BINARY_COMMIT:-$candidate_revision}" \
      MOCK_TAG_OBJECT_TYPE="${MOCK_TAG_OBJECT_TYPE:-commit}" \
      MOCK_TAG_OBJECT_SHA="${MOCK_TAG_OBJECT_SHA:-$candidate_revision}" \
      MOCK_PEELED_TAG_OBJECT_TYPE="${MOCK_PEELED_TAG_OBJECT_TYPE:-commit}" \
      MOCK_PEELED_TAG_OBJECT_SHA="${MOCK_PEELED_TAG_OBJECT_SHA:-$candidate_revision}" \
      MOCK_RELEASE_TAG="${MOCK_RELEASE_TAG:-v${MOCK_CANDIDATE_VERSION:-0.1.172}}" \
      MOCK_RELEASE_DRAFT="${MOCK_RELEASE_DRAFT:-false}" \
      MOCK_RELEASE_PRERELEASE="${MOCK_RELEASE_PRERELEASE:-false}" \
      MOCK_RELEASE_API_FAIL="${MOCK_RELEASE_API_FAIL:-false}" \
      MOCK_CANDIDATE_CAPABILITY="$candidate_capability" \
      MOCK_CANDIDATE_PAYMENT_COMPONENTS="$candidate_payment_components" \
      MOCK_CANDIDATE_TRUSTED_CI="$candidate_trusted_ci" \
      MOCK_ATTESTATION_VALID="$attestation_valid" \
      MOCK_GH_VERSION="$gh_version" \
      MOCK_LEAVE_OLD_RUNNING="$leave_old_running" \
      MOCK_CANDIDATE_HEALTH="$candidate_health" \
      MOCK_PUBLIC_HEALTH_STATUS="$public_health_status" \
      MOCK_REQUIRE_OLD_STOPPED="${MOCK_REQUIRE_OLD_STOPPED:-false}" \
      bash "$root/subject.sh" > "$RUN_LOG" 2>&1
  RUN_STATUS=$?
  set -e
}

assert_success() {
  if [[ "$RUN_STATUS" -ne 0 ]]; then
    cat "$RUN_LOG" >&2
    echo "Expected deployment to succeed" >&2
    exit 1
  fi
}

assert_failure() {
  local expected_message=$1
  if [[ "$RUN_STATUS" -eq 0 ]]; then
    cat "$RUN_LOG" >&2
    echo "Expected deployment to fail" >&2
    exit 1
  fi
  if ! grep -Fq "$expected_message" "$RUN_LOG"; then
    cat "$RUN_LOG" >&2
    echo "Expected deployment failure message: $expected_message" >&2
    exit 1
  fi
}

new_bootstrap_scenario() {
  local name=$1
  local active_gate=${2-false}
  local payment_components=${3:-'<no value>'}
  local root="$TEST_ROOT/bootstrap-$name"
  mkdir -p "$root/state"
  printf '%s' true > "$root/state/blue_running"
  printf '%s' true > "$root/state/blue_exists"
  printf '%s' "$active_gate" > "$root/state/blue_gate"
  printf '%s' "$OLD_IMAGE_ID" > "$root/state/blue_image_id"
  printf '%s' "$payment_components" > "$root/state/blue_payment_components"
  printf '%s' false > "$root/state/green_running"
  printf '%s' true > "$root/state/green_exists"
  sed \
    -e "s|readonly STATE_DIR=\"/var/lib/sub2api-deploy\"|readonly STATE_DIR=\"$root/deploy-state\"|" \
    -e "s|readonly STATE_OWNER_UID=\"0\"|readonly STATE_OWNER_UID=\"$(id -u)\"|" \
    -e "s|readonly STATE_OWNER_GID=\"0\"|readonly STATE_OWNER_GID=\"$(id -g)\"|" \
    "$BOOTSTRAP_SCRIPT" > "$root/bootstrap.sh"
  chmod +x "$root/bootstrap.sh"
  printf '%s' "$root"
}

assert_bootstrap_rejected_without_state() {
  local root=$1
  [[ -d "$root/deploy-state" ]]
  [[ -f "$root/deploy-state/stable-deploy.lock" ]]
  [[ "$(find "$root/deploy-state" -type f ! -name stable-deploy.lock -print | wc -l)" -eq 0 ]]
}

# Missing state is never inferred as a virgin host. A separate one-time
# bootstrap requires an explicit external database reversal precheck and also
# proves that no running slot has the gate enabled.
bootstrap_root=$(new_bootstrap_scenario success false)
env PATH="$MOCK_BIN:$PATH" MOCK_STATE_DIR="$bootstrap_root/state" \
  bash "$bootstrap_root/bootstrap.sh" --operator-confirm-database-has-no-affiliate-reversals >/dev/null
grep -Fq 'affiliate_refund_reversal_state=absent' "$bootstrap_root/deploy-state/affiliate-refund-reversal-state"
grep -Fq 'payment_reversal_components_state=absent' "$bootstrap_root/deploy-state/payment-reversal-components-state"
[[ "$(stat -c '%a' "$bootstrap_root/deploy-state/affiliate-refund-reversal-state" 2>/dev/null || stat -f '%Lp' "$bootstrap_root/deploy-state/affiliate-refund-reversal-state")" == 600 ]]
if env PATH="$MOCK_BIN:$PATH" MOCK_STATE_DIR="$bootstrap_root/state" \
  bash "$bootstrap_root/bootstrap.sh" --operator-confirm-database-has-no-affiliate-reversals >/dev/null 2>&1; then
  echo "Bootstrap unexpectedly overwrote existing deployment state" >&2
  exit 1
fi

bootstrap_gate_root=$(new_bootstrap_scenario gate-enabled true)
if env PATH="$MOCK_BIN:$PATH" MOCK_STATE_DIR="$bootstrap_gate_root/state" \
  bash "$bootstrap_gate_root/bootstrap.sh" --operator-confirm-database-has-no-affiliate-reversals > "$bootstrap_gate_root/run.log" 2>&1; then
  echo "Bootstrap unexpectedly accepted a running gate=true slot" >&2
  exit 1
fi
grep -Fq 'may have affiliate refund reversal enabled' "$bootstrap_gate_root/run.log"
assert_bootstrap_rejected_without_state "$bootstrap_gate_root"

bootstrap_whitespace_gate_root=$(new_bootstrap_scenario whitespace-gate-enabled $'  true\t')
if env PATH="$MOCK_BIN:$PATH" MOCK_STATE_DIR="$bootstrap_whitespace_gate_root/state" \
  bash "$bootstrap_whitespace_gate_root/bootstrap.sh" --operator-confirm-database-has-no-affiliate-reversals > "$bootstrap_whitespace_gate_root/run.log" 2>&1; then
  echo "Bootstrap unexpectedly accepted a whitespace-padded gate=true slot" >&2
  exit 1
fi
grep -Fq 'may have affiliate refund reversal enabled' "$bootstrap_whitespace_gate_root/run.log"
assert_bootstrap_rejected_without_state "$bootstrap_whitespace_gate_root"

bootstrap_newline_gate_root=$(new_bootstrap_scenario newline-gate-enabled $'\ntrue\n')
if env PATH="$MOCK_BIN:$PATH" MOCK_STATE_DIR="$bootstrap_newline_gate_root/state" \
  bash "$bootstrap_newline_gate_root/bootstrap.sh" --operator-confirm-database-has-no-affiliate-reversals > "$bootstrap_newline_gate_root/run.log" 2>&1; then
  echo "Bootstrap unexpectedly accepted a newline-padded gate=true slot" >&2
  exit 1
fi
grep -Fq 'may have affiliate refund reversal enabled' "$bootstrap_newline_gate_root/run.log"
assert_bootstrap_rejected_without_state "$bootstrap_newline_gate_root"

bootstrap_inspect_failure_root=$(new_bootstrap_scenario inspect-failure false)
if env PATH="$MOCK_BIN:$PATH" MOCK_STATE_DIR="$bootstrap_inspect_failure_root/state" MOCK_CONTAINER_INSPECT_FAIL=blue \
  bash "$bootstrap_inspect_failure_root/bootstrap.sh" --operator-confirm-database-has-no-affiliate-reversals > "$bootstrap_inspect_failure_root/run.log" 2>&1; then
  echo "Bootstrap unexpectedly treated a Docker inspect failure as an absent slot" >&2
  exit 1
fi
grep -Fq 'Unable to inspect required stable slot' "$bootstrap_inspect_failure_root/run.log"
assert_bootstrap_rejected_without_state "$bootstrap_inspect_failure_root"

bootstrap_missing_gate_root=$(new_bootstrap_scenario missing-gate '')
if env PATH="$MOCK_BIN:$PATH" MOCK_STATE_DIR="$bootstrap_missing_gate_root/state" \
  bash "$bootstrap_missing_gate_root/bootstrap.sh" --operator-confirm-database-has-no-affiliate-reversals > "$bootstrap_missing_gate_root/run.log" 2>&1; then
  echo "Bootstrap unexpectedly accepted a running slot without an explicit gate=false" >&2
  exit 1
fi
grep -Fq 'may have affiliate refund reversal enabled' "$bootstrap_missing_gate_root/run.log"
assert_bootstrap_rejected_without_state "$bootstrap_missing_gate_root"

bootstrap_payment_root=$(new_bootstrap_scenario payment-components false 1)
if env PATH="$MOCK_BIN:$PATH" MOCK_STATE_DIR="$bootstrap_payment_root/state" \
  bash "$bootstrap_payment_root/bootstrap.sh" --operator-confirm-database-has-no-affiliate-reversals > "$bootstrap_payment_root/run.log" 2>&1; then
  echo "Bootstrap unexpectedly accepted a running payment-component-capable slot" >&2
  exit 1
fi
grep -Fq 'has payment reversal components enabled' "$bootstrap_payment_root/run.log"
assert_bootstrap_rejected_without_state "$bootstrap_payment_root"

# A lone state file without the bootstrap transaction journal can mean that a
# previously initialized monotonic state was lost. It must never be inferred
# as an interrupted first bootstrap.
bootstrap_lone_state_root=$(new_bootstrap_scenario lone-state false)
mkdir -p "$bootstrap_lone_state_root/deploy-state"
chmod 0700 "$bootstrap_lone_state_root/deploy-state"
printf '%s\n' \
  'state_version=1' \
  'affiliate_refund_reversal_state=absent' \
  'updated_at=2026-08-14T00:00:00Z' > "$bootstrap_lone_state_root/deploy-state/affiliate-refund-reversal-state"
chmod 0600 "$bootstrap_lone_state_root/deploy-state/affiliate-refund-reversal-state"
if env PATH="$MOCK_BIN:$PATH" MOCK_STATE_DIR="$bootstrap_lone_state_root/state" \
  bash "$bootstrap_lone_state_root/bootstrap.sh" --operator-confirm-database-has-no-affiliate-reversals > "$bootstrap_lone_state_root/run.log" 2>&1; then
  echo "Bootstrap unexpectedly repaired a lone state without its transaction journal" >&2
  exit 1
fi
grep -Fq 'incomplete without a matching bootstrap transaction' "$bootstrap_lone_state_root/run.log"
[[ ! -e "$bootstrap_lone_state_root/deploy-state/payment-reversal-components-state" ]]

# A crash after the first no-clobber state commit is recoverable only while the
# matching pending journal is still present and both hashes match exactly.
bootstrap_resume_root=$(new_bootstrap_scenario resume false)
mkdir -p "$bootstrap_resume_root/deploy-state"
chmod 0700 "$bootstrap_resume_root/deploy-state"
resume_updated_at=2026-08-14T00:00:00Z
printf '%s\n' \
  'state_version=1' \
  'affiliate_refund_reversal_state=absent' \
  "updated_at=$resume_updated_at" > "$bootstrap_resume_root/deploy-state/affiliate-refund-reversal-state"
resume_payment="$bootstrap_resume_root/payment.expected"
printf '%s\n' \
  'state_version=1' \
  'payment_reversal_components_state=absent' \
  "updated_at=$resume_updated_at" > "$resume_payment"
chmod 0600 "$bootstrap_resume_root/deploy-state/affiliate-refund-reversal-state"
cat > "$bootstrap_resume_root/deploy-state/state-bootstrap.pending" <<EOF
bootstrap_version=1
transaction_id=0123456789abcdef0123456789abcdef
updated_at=$resume_updated_at
affiliate_sha256=$(sha256_file "$bootstrap_resume_root/deploy-state/affiliate-refund-reversal-state")
payment_sha256=$(sha256_file "$resume_payment")
EOF
chmod 0600 "$bootstrap_resume_root/deploy-state/state-bootstrap.pending"
env PATH="$MOCK_BIN:$PATH" MOCK_STATE_DIR="$bootstrap_resume_root/state" \
  bash "$bootstrap_resume_root/bootstrap.sh" --operator-confirm-database-has-no-affiliate-reversals >/dev/null
cmp -s "$resume_payment" "$bootstrap_resume_root/deploy-state/payment-reversal-components-state"
[[ ! -e "$bootstrap_resume_root/deploy-state/state-bootstrap.pending" ]]

bootstrap_no_confirmation_root=$(new_bootstrap_scenario no-confirmation false)
if env PATH="$MOCK_BIN:$PATH" MOCK_STATE_DIR="$bootstrap_no_confirmation_root/state" \
  bash "$bootstrap_no_confirmation_root/bootstrap.sh" > "$bootstrap_no_confirmation_root/run.log" 2>&1; then
  echo "Bootstrap unexpectedly skipped the external database precheck confirmation" >&2
  exit 1
fi
grep -Fq 'This command does not query the production database' "$bootstrap_no_confirmation_root/run.log"

bootstrap_pending_root=$(new_scenario bootstrap-pending false)
printf '%s\n' 'bootstrap_version=1' > "$bootstrap_pending_root/deploy-state/state-bootstrap.pending"
chmod 0600 "$bootstrap_pending_root/deploy-state/state-bootstrap.pending"
run_deploy "$bootstrap_pending_root" "$RELEASE_IMAGE_ID" disabled "$RELEASE_IMAGE_ID" "$RELEASE_REVISION" 1
assert_failure 'Deployment state bootstrap transaction is incomplete'

# The protocol prefix occupies the first line without moving the contract stage
# from line four. A legacy three-line receiver treats line one as an image and
# must therefore reject the new payload instead of silently ignoring the stage.
legacy_receiver="$TEST_ROOT/legacy-receiver.sh"
cat > "$legacy_receiver" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
IFS= read -r image_tag
case "$image_tag" in
  sha-[0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f]) ;;
  *) exit 2 ;;
esac
IFS= read -r username
IFS= read -r token
[[ -n "$username" && -n "$token" ]]
EOF
chmod +x "$legacy_receiver"
if printf 'sub2api-deploy-v2 sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\nuser\ntoken\nenabled\n' | "$legacy_receiver"; then
  echo "Legacy deployment receiver silently accepted the versioned payload" >&2
  exit 1
fi
[[ "$(printf 'sub2api-deploy-v2 sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\nuser\ntoken\nenabled\n' | sed -n '4p')" == enabled ]]

# The receiver rejects extra payload lines even if the first four fields are
# otherwise valid, preventing newline injection from replacing the approved
# contract stage.
trailing_payload_root=$(new_scenario trailing-payload false)
RUN_LOG="$trailing_payload_root/run-trailing-payload.log"
set +e
printf 'sub2api-deploy-v2 sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\nuser\ntoken\ndisabled\nenabled\n' |
  env PATH="$MOCK_BIN:$PATH" MOCK_STATE_DIR="$trailing_payload_root/state" bash "$trailing_payload_root/subject.sh" > "$RUN_LOG" 2>&1
RUN_STATUS=$?
set -e
assert_failure 'Unexpected trailing data in deployment request'

# Manual rebuilds are isolated from release tags and carry trusted-ci=0. Even
# with an exact digest and the affiliate gate disabled, production rejects them.
untrusted_image_root=$(new_scenario untrusted-image false)
run_deploy "$untrusted_image_root" "$RELEASE_IMAGE_ID" disabled "$RELEASE_IMAGE_ID" "$RELEASE_REVISION" 1 false healthy 200 0
assert_failure 'Candidate image does not declare org.sub2api.build.trusted-ci=1'

# OCI labels are self-declared. Production trust comes from a GitHub/Sigstore
# attestation bound to this digest, repository, workflow, branch, and commit.
unattested_image_root=$(new_scenario unattested-image false)
run_deploy "$unattested_image_root" "$RELEASE_IMAGE_ID" disabled "$RELEASE_IMAGE_ID" "$RELEASE_REVISION" 1 false healthy 200 1 false
assert_failure 'Candidate image lacks trusted main-branch GitHub build provenance'

invalid_oci_version_root=$(new_scenario invalid-oci-version false)
MOCK_CANDIDATE_VERSION=not-a-version
run_deploy "$invalid_oci_version_root" "$RELEASE_IMAGE_ID" disabled "$RELEASE_IMAGE_ID" "$RELEASE_REVISION" 1
unset MOCK_CANDIDATE_VERSION
assert_failure 'Candidate image ID, OCI revision, or OCI version metadata is incomplete or invalid'

binary_version_mismatch_root=$(new_scenario binary-version-mismatch false)
MOCK_BINARY_VERSION=0.1.171
run_deploy "$binary_version_mismatch_root" "$RELEASE_IMAGE_ID" disabled "$RELEASE_IMAGE_ID" "$RELEASE_REVISION" 1
unset MOCK_BINARY_VERSION
assert_failure 'Candidate binary version/commit does not match OCI metadata'

binary_commit_mismatch_root=$(new_scenario binary-commit-mismatch false)
MOCK_BINARY_COMMIT="$OLD_REVISION"
run_deploy "$binary_commit_mismatch_root" "$RELEASE_IMAGE_ID" disabled "$RELEASE_IMAGE_ID" "$RELEASE_REVISION" 1
unset MOCK_BINARY_COMMIT
assert_failure 'Candidate binary version/commit does not match OCI metadata'

wrong_release_tag_root=$(new_scenario wrong-release-tag false)
MOCK_TAG_OBJECT_SHA="$OLD_REVISION"
run_deploy "$wrong_release_tag_root" "$RELEASE_IMAGE_ID" disabled "$RELEASE_IMAGE_ID" "$RELEASE_REVISION" 1
unset MOCK_TAG_OBJECT_SHA
assert_failure 'Release tag v0.1.172 does not resolve to candidate revision'

draft_release_root=$(new_scenario draft-release false)
MOCK_RELEASE_DRAFT=true
run_deploy "$draft_release_root" "$RELEASE_IMAGE_ID" disabled "$RELEASE_IMAGE_ID" "$RELEASE_REVISION" 1
unset MOCK_RELEASE_DRAFT
assert_failure 'Published GitHub release is missing or inconsistent for v0.1.172'

prerelease_root=$(new_scenario prerelease false)
MOCK_RELEASE_PRERELEASE=true
run_deploy "$prerelease_root" "$RELEASE_IMAGE_ID" disabled "$RELEASE_IMAGE_ID" "$RELEASE_REVISION" 1
unset MOCK_RELEASE_PRERELEASE
assert_failure 'Published GitHub release is missing or inconsistent for v0.1.172'

missing_release_root=$(new_scenario missing-release false)
MOCK_RELEASE_API_FAIL=true
run_deploy "$missing_release_root" "$RELEASE_IMAGE_ID" disabled "$RELEASE_IMAGE_ID" "$RELEASE_REVISION" 1
unset MOCK_RELEASE_API_FAIL
assert_failure 'Published GitHub release is missing or inconsistent for v0.1.172'

annotated_release_tag_root=$(new_scenario annotated-release-tag false)
MOCK_TAG_OBJECT_TYPE=tag
MOCK_TAG_OBJECT_SHA="$OLD_REVISION"
run_deploy "$annotated_release_tag_root" "$RELEASE_IMAGE_ID" disabled "$RELEASE_IMAGE_ID" "$RELEASE_REVISION" 1
unset MOCK_TAG_OBJECT_TYPE MOCK_TAG_OBJECT_SHA
assert_success

outdated_gh_root=$(new_scenario outdated-gh false)
run_deploy "$outdated_gh_root" "$RELEASE_IMAGE_ID" disabled "$RELEASE_IMAGE_ID" "$RELEASE_REVISION" 1 false healthy 200 1 true 2.96.0
assert_failure 'GitHub CLI 2.97.0 or newer is required'

prerelease_gh_root=$(new_scenario prerelease-gh false)
run_deploy "$prerelease_gh_root" "$RELEASE_IMAGE_ID" disabled "$RELEASE_IMAGE_ID" "$RELEASE_REVISION" 1 false healthy 200 1 true 2.97.0-rc.1
assert_failure 'GitHub CLI 2.97.0 or newer is required'

# Mutable short-SHA tags never reach registry login or deployment state setup.
mutable_tag_root=$(new_scenario mutable-tag false)
run_deploy "$mutable_tag_root" sha-bbbbbbb disabled "$RELEASE_IMAGE_ID" "$RELEASE_REVISION" 1
assert_failure 'Only an exact ghcr.io/xuehua123/sub2api@sha256 digest is allowed'

missing_state_root=$(new_scenario missing-state false)
rm -rf -- "$missing_state_root/deploy-state"
run_deploy "$missing_state_root" "$RELEASE_IMAGE_ID" disabled "$RELEASE_IMAGE_ID" "$RELEASE_REVISION" 1
assert_failure 'Deployment state directory must be provisioned by the reviewed bootstrap command'

unsafe_lock_root=$(new_scenario unsafe-lock false)
lock_victim="$unsafe_lock_root/lock-victim"
printf '%s' preserved > "$lock_victim"
ln -s "$lock_victim" "$unsafe_lock_root/deploy-state/stable-deploy.lock"
run_deploy "$unsafe_lock_root" "$RELEASE_IMAGE_ID" disabled "$RELEASE_IMAGE_ID" "$RELEASE_REVISION" 1
assert_failure 'Stable deployment lock path is unsafe'
[[ "$(cat "$lock_victim")" == preserved ]]

writable_platform_root=$(new_scenario writable-platform false)
chmod 0777 "$writable_platform_root/platform"
run_deploy "$writable_platform_root" "$RELEASE_IMAGE_ID" disabled "$RELEASE_IMAGE_ID" "$RELEASE_REVISION" 1
assert_failure 'Deployment directory must be root-owned and not group/other writable'

compose_symlink_root=$(new_scenario compose-symlink false)
compose_victim="$compose_symlink_root/compose-victim"
printf '%s' preserved > "$compose_victim"
rm -f -- "$compose_symlink_root/platform/docker-compose.migration.yml"
ln -s "$compose_victim" "$compose_symlink_root/platform/docker-compose.migration.yml"
run_deploy "$compose_symlink_root" "$RELEASE_IMAGE_ID" disabled "$RELEASE_IMAGE_ID" "$RELEASE_REVISION" 1
assert_failure 'Deployment file is not a regular non-symlink file'
[[ "$(cat "$compose_victim")" == preserved ]]

env_symlink_root=$(new_scenario env-symlink false)
env_victim="$env_symlink_root/env-victim"
printf '%s' preserved > "$env_victim"
rm -f -- "$env_symlink_root/platform/sub2api.env"
ln -s "$env_victim" "$env_symlink_root/platform/sub2api.env"
run_deploy "$env_symlink_root" "$RELEASE_IMAGE_ID" disabled "$RELEASE_IMAGE_ID" "$RELEASE_REVISION" 1
assert_failure 'Deployment file is not a regular non-symlink file'
[[ "$(cat "$env_victim")" == preserved ]]

env_mode_root=$(new_scenario env-mode false)
chmod 0644 "$env_mode_root/platform/sub2api.env"
run_deploy "$env_mode_root" "$RELEASE_IMAGE_ID" disabled "$RELEASE_IMAGE_ID" "$RELEASE_REVISION" 1
assert_failure 'Compose environment file must be root-owned with mode 0600'

# Migration 197 changes the payment refund writer contract. The first capable
# image must persist pending state and stop the old incapable writer before the
# candidate starts and runs migrations. This transition is independent from
# the affiliate gate, which remains explicitly false.
payment_components_root=$(new_scenario payment-components false)
MOCK_REQUIRE_OLD_STOPPED=true
run_deploy "$payment_components_root" "$RELEASE_IMAGE_ID" disabled "$RELEASE_IMAGE_ID" "$RELEASE_REVISION" 1 false healthy 200 1 true 2.97.0 1
unset MOCK_REQUIRE_OLD_STOPPED
assert_success
payment_contract="$payment_components_root/deploy-state/payment-reversal-components-contract"
grep -Fq 'state=activated' "$payment_contract"
grep -Fq "image_id=$RELEASE_IMAGE_ID" "$payment_contract"
grep -Fq 'capability_value=1' "$payment_contract"
grep -Fq 'payment_reversal_components_state=activated' "$payment_components_root/deploy-state/payment-reversal-components-state"
[[ "$(cat "$payment_components_root/state/blue_running")" == false ]]
[[ "$(cat "$payment_components_root/state/green_running")" == true ]]
[[ "$(cat "$payment_components_root/state/green_gate")" == false ]]

# Once the later payment-components contract capability is activated, images
# without that capability are no longer rollback targets.
run_deploy "$payment_components_root" "$OLD_IMAGE_ID" disabled "$OLD_IMAGE_ID" "$OLD_REVISION" 1 false healthy 200 1 true 2.97.0 0
assert_failure 'Payment reversal components are permanently activated; incapable images are rejected'

rm -f -- "$payment_contract"
run_deploy "$payment_components_root" "$NEXT_IMAGE_ID" disabled "$NEXT_IMAGE_ID" "$NEXT_REVISION" 1 false healthy 200 1 true 2.97.0 1
assert_failure 'Activated payment reversal component state conflicts with contract files'

# A failed first start remains pending and leaves the old writer stopped. The
# ordinary deploy command cannot infer or downgrade this state.
payment_components_failure_root=$(new_scenario payment-components-failure false)
run_deploy "$payment_components_failure_root" "$RELEASE_IMAGE_ID" disabled "$RELEASE_IMAGE_ID" "$RELEASE_REVISION" 1 false unhealthy 200 1 true 2.97.0 1
assert_failure 'An irreversible deployment activation remains pending; forward operator recovery is required.'
grep -Fq 'state=pending' "$payment_components_failure_root/deploy-state/payment-reversal-components-contract.pending"
grep -Fq 'payment_reversal_components_state=pending' "$payment_components_failure_root/deploy-state/payment-reversal-components-state"
[[ "$(cat "$payment_components_failure_root/state/blue_running")" == false ]]
run_deploy "$payment_components_failure_root" "$RELEASE_IMAGE_ID" disabled "$RELEASE_IMAGE_ID" "$RELEASE_REVISION" 1 false healthy 200 1 true 2.97.0 1
assert_failure 'An incomplete payment reversal component activation requires forward operator recovery'

# Real upgrade path: deploy the capable image disabled, then activate that exact
# image only after the older slot has stopped.
two_stage_root=$(new_scenario two-stage)
run_deploy "$two_stage_root" "$RELEASE_IMAGE_ID" disabled "$RELEASE_IMAGE_ID" "$RELEASE_REVISION" 1
assert_success
[[ ! -e "$two_stage_root/deploy-state/affiliate-refund-reversal-contract" ]]
grep -Fq 'affiliate_refund_reversal_state=absent' "$two_stage_root/deploy-state/affiliate-refund-reversal-state"
[[ "$(cat "$two_stage_root/state/blue_running")" == false ]]
[[ "$(cat "$two_stage_root/state/green_gate")" == false ]]
[[ -z "$(find "$two_stage_root/deploy-state" -name 'sub2api-stable-*.yml.*' -print -quit)" ]]

run_deploy "$two_stage_root" "$RELEASE_IMAGE_ID" enabled "$RELEASE_IMAGE_ID" "$RELEASE_REVISION" 1
assert_success
contract="$two_stage_root/deploy-state/affiliate-refund-reversal-contract"
grep -Fq 'state=activated' "$contract"
grep -Fq 'stage=enabled' "$contract"
grep -Fq "image_id=$RELEASE_IMAGE_ID" "$contract"
grep -Fq "revision=$RELEASE_REVISION" "$contract"
grep -Fq 'capability_value=1' "$contract"
[[ ! -e "${contract}.pending" ]]
grep -Fq 'affiliate_refund_reversal_state=activated' "$two_stage_root/deploy-state/affiliate-refund-reversal-state"
[[ "$(cat "$two_stage_root/state/green_running")" == false ]]
[[ "$(cat "$two_stage_root/state/blue_gate")" == true ]]
[[ -z "$(find "$two_stage_root/deploy-state" -name 'sub2api-stable-*.yml.*' -print -quit)" ]]
rollback_state=$(find "$two_stage_root/deploy-state" -name 'rollback-state-enabled-*.txt.*' -print | head -n 1)
grep -Fq 'stage=enabled' "$rollback_state"
grep -Fq "candidate_image_id=$RELEASE_IMAGE_ID" "$rollback_state"
grep -Fq "candidate_revision=$RELEASE_REVISION" "$rollback_state"
grep -Fq 'old_slot_stopped=true' "$rollback_state"

# The persisted contract makes disabled rollback and unlabelled images illegal.
run_deploy "$two_stage_root" "$OLD_IMAGE_ID" disabled "$OLD_IMAGE_ID" "$OLD_REVISION" '<no value>'
assert_failure 'Affiliate refund reversal is permanently activated; disabled deployments are rejected'
run_deploy "$two_stage_root" "$NEXT_IMAGE_ID" enabled "$NEXT_IMAGE_ID" "$NEXT_REVISION" '<no value>'
assert_failure 'Candidate image does not declare org.sub2api.capability.affiliate-refund-reversal=1'

# Future releases remain possible, but only as capable images with the gate on.
run_deploy "$two_stage_root" "$NEXT_IMAGE_ID" enabled "$NEXT_IMAGE_ID" "$NEXT_REVISION" 1
assert_success
grep -Fq "image_id=$RELEASE_IMAGE_ID" "$contract"
[[ "$(cat "$two_stage_root/state/blue_running")" == false ]]
[[ "$(cat "$two_stage_root/state/green_gate")" == true ]]

# Before activation, enabled cannot switch to a different image ID.
mismatch_root=$(new_scenario mismatch false)
run_deploy "$mismatch_root" "$RELEASE_IMAGE_ID" enabled "$RELEASE_IMAGE_ID" "$RELEASE_REVISION" 1
assert_failure 'Initial activation must use the exact image ID already running with the gate disabled'
[[ ! -e "$mismatch_root/deploy-state/affiliate-refund-reversal-contract" ]]

# An unset active gate is not equivalent to an explicit false during activation.
unset_gate_root=$(new_scenario unset-gate)
run_deploy "$unset_gate_root" "$OLD_IMAGE_ID" enabled "$OLD_IMAGE_ID" "$OLD_REVISION" 1
assert_failure 'Initial activation requires the active container gate to be explicitly false'
[[ ! -e "$unset_gate_root/deploy-state/affiliate-refund-reversal-contract" ]]

# Initial activation stops the gate=false slot before the gate=true candidate
# starts. Even a cutover helper that does not stop the old slot cannot leave a
# downgrade target running after reversals may begin.
old_running_root=$(new_scenario old-running false)
run_deploy "$old_running_root" "$OLD_IMAGE_ID" enabled "$OLD_IMAGE_ID" "$OLD_REVISION" 1 true
assert_success
grep -Fq 'state=activated' "$old_running_root/deploy-state/affiliate-refund-reversal-contract"
[[ "$(cat "$old_running_root/state/blue_running")" == false ]]
[[ "$(cat "$old_running_root/state/green_gate")" == true ]]

# In reversible stages, a post-cutover inconsistency invokes the reviewed
# helper in recovery mode, switches Nginx back, and removes the failed target.
safe_rollback_root=$(new_scenario safe-rollback false)
run_deploy "$safe_rollback_root" "$RELEASE_IMAGE_ID" disabled "$RELEASE_IMAGE_ID" "$RELEASE_REVISION" 1 true
assert_failure 'Deployment failed after cutover and the previous slot was restored.'
[[ "$(cat "$safe_rollback_root/state/blue_running")" == true ]]
[[ "$(cat "$safe_rollback_root/state/green_running")" == false ]]
grep -Fq 'proxy_pass http://127.0.0.1:18080;' "$safe_rollback_root/platform.conf"
rollback_record=$(find "$safe_rollback_root/deploy-state" -name 'rollback-state-disabled-*.txt.*' -print | head -n 1)
grep -Fq 'rollback_restored_container=sub2api-blue' "$rollback_record"

# Losing an activated contract is not interpreted as a virgin host. The
# independent monotonic state record makes the next deployment fail closed.
lost_contract_root=$(new_scenario lost-contract false)
run_deploy "$lost_contract_root" "$OLD_IMAGE_ID" enabled "$OLD_IMAGE_ID" "$OLD_REVISION" 1
assert_success
rm -f -- "$lost_contract_root/deploy-state/affiliate-refund-reversal-contract"
run_deploy "$lost_contract_root" "$NEXT_IMAGE_ID" enabled "$NEXT_IMAGE_ID" "$NEXT_REVISION" 1
assert_failure 'Activated deployment state conflicts with affiliate refund reversal contract files'

# The state directory itself is a trust boundary, not just the file mode.
insecure_state_root=$(new_scenario insecure-state false)
run_deploy "$insecure_state_root" "$RELEASE_IMAGE_ID" disabled "$RELEASE_IMAGE_ID" "$RELEASE_REVISION" 1
assert_success
chmod 0755 "$insecure_state_root/deploy-state"
run_deploy "$insecure_state_root" "$RELEASE_IMAGE_ID" disabled "$RELEASE_IMAGE_ID" "$RELEASE_REVISION" 1
assert_failure 'Deployment state directory must be owned'

# Once a gate=true process may have started, even a pre-cutover health failure
# keeps the pending marker because database recovery could already have run.
health_failure_root=$(new_scenario health-failure false)
run_deploy "$health_failure_root" "$OLD_IMAGE_ID" enabled "$OLD_IMAGE_ID" "$OLD_REVISION" 1 false unhealthy
assert_failure 'An irreversible deployment activation remains pending; forward operator recovery is required.'
[[ ! -e "$health_failure_root/deploy-state/affiliate-refund-reversal-contract" ]]
grep -Fq 'state=pending' "$health_failure_root/deploy-state/affiliate-refund-reversal-contract.pending"

# The contract remains pending when the new slot is locally healthy but public
# health fails after the old slot has stopped. It must never be promoted based
# on a stale pre-drain response.
public_health_failure_root=$(new_scenario public-health-failure false)
run_deploy "$public_health_failure_root" "$OLD_IMAGE_ID" enabled "$OLD_IMAGE_ID" "$OLD_REVISION" 1 false healthy 503
assert_failure 'Public health check failed after the old active slot stopped'
[[ ! -e "$public_health_failure_root/deploy-state/affiliate-refund-reversal-contract" ]]
grep -Fq 'state=pending' "$public_health_failure_root/deploy-state/affiliate-refund-reversal-contract.pending"

# Comments or unrelated text containing a port are not valid Nginx routing
# evidence. The deployment must require an exact proxy_pass directive.
invalid_nginx_root=$(new_scenario invalid-nginx false)
printf '%s\n' '# proxy_pass http://127.0.0.1:18080;' > "$invalid_nginx_root/platform.conf"
run_deploy "$invalid_nginx_root" "$OLD_IMAGE_ID" disabled "$OLD_IMAGE_ID" "$OLD_REVISION" 1
assert_failure 'Nginx config does not route exclusively to the active upstream 127.0.0.1:18080'

echo "stable deployment contract behavior tests passed"
