#!/usr/bin/env bash

set -Eeuo pipefail

umask 077

readonly IMAGE_NAME="ghcr.io/xuehua123/sub2api"
readonly DEPLOY_DIR="/opt/platform"
readonly COMPOSE_FILE="${DEPLOY_DIR}/docker-compose.yml"
readonly NGINX_CONFIG="/etc/nginx/sites-enabled/platform.conf"
readonly CUTOVER_SCRIPT="/usr/local/sbin/sub2api-nginx-bluegreen-cutover"
readonly LOCK_FILE="/var/lock/sub2api-stable-deploy.lock"
readonly PUBLIC_HEALTH_URL="https://api.wenrugouai.com/health"

if ! IFS= read -r deploy_image_tag || [[ -z "$deploy_image_tag" ]]; then
  echo "An immutable image tag must be provided on stdin" >&2
  exit 2
fi

case "$deploy_image_tag" in
  sha-[0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f])
    deploy_image="${IMAGE_NAME}:${deploy_image_tag}"
    ;;
  sha256:*)
    deploy_image="${IMAGE_NAME}@${deploy_image_tag}"
    ;;
  "${IMAGE_NAME}":sha-[0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f])
    deploy_image="$deploy_image_tag"
    ;;
  "${IMAGE_NAME}"@sha256:*)
    deploy_image="$deploy_image_tag"
    ;;
  *)
    echo "Only immutable ${IMAGE_NAME}:sha-* tags or sha256 digests are allowed" >&2
    exit 2
    ;;
esac

if ! IFS= read -r ghcr_username || [[ -z "$ghcr_username" ]]; then
  echo "A GHCR username must be provided on stdin" >&2
  exit 2
fi

if ! IFS= read -r ghcr_token || [[ -z "$ghcr_token" ]]; then
  echo "A GHCR token must be provided on stdin" >&2
  exit 2
fi

[[ -f "$COMPOSE_FILE" ]] || { echo "Stable compose file not found: $COMPOSE_FILE" >&2; exit 1; }
[[ -f "$NGINX_CONFIG" ]] || { echo "Stable nginx config not found: $NGINX_CONFIG" >&2; exit 1; }
[[ -x "$CUTOVER_SCRIPT" ]] || { echo "Blue-green cutover script not found: $CUTOVER_SCRIPT" >&2; exit 1; }

exec 9>"$LOCK_FILE"
flock -n 9 || { echo "Another stable deployment is already running" >&2; exit 1; }

docker_config="$(mktemp -d)"
cleanup() {
  unset ghcr_token
  rm -rf -- "$docker_config"
}
trap cleanup EXIT

printf '%s' "$ghcr_token" |
  docker --config "$docker_config" login ghcr.io \
    --username "$ghcr_username" \
    --password-stdin
unset ghcr_token

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

if ! grep -Fq "$active_upstream" "$NGINX_CONFIG"; then
  echo "Nginx config does not reference the active upstream $active_upstream" >&2
  exit 1
fi

# The inactive slot must be fully disposable before the new candidate is started.
if [[ "$(docker inspect --format '{{.State.Running}}' "$target_container" 2>/dev/null || true)" == true ]]; then
  echo "Inactive target container $target_container is unexpectedly running" >&2
  exit 1
fi
docker rm -f "$target_container" >/dev/null 2>&1 || true

safe_tag="$(printf '%s' "$deploy_image_tag" | tr '/:@' '---')"
candidate="${DEPLOY_DIR}/sub2api-stable-${target_color}-${safe_tag}.yml"
rollback_state="${DEPLOY_DIR}/rollback-state-$(date +%Y%m%d-%H%M%S)-pre-${safe_tag}.txt"

if [[ "$target_color" == blue ]]; then
  cat > "$candidate" <<EOF
services:
  sub2api:
    container_name: sub2api-blue
    image: ${deploy_image}
    ports: !override
      - "127.0.0.1:18080:8080"

networks:
  shanghai-net:
    external: true
    name: platform_shanghai-net
EOF
else
  cat > "$candidate" <<EOF
services:
  sub2api:
    container_name: sub2api-green
    image: ${deploy_image}
    ports: !override
      - "127.0.0.1:28080:8080"

networks:
  shanghai-net:
    external: true
    name: platform_shanghai-net
EOF
fi

{
  echo "timestamp=$(date -Is)"
  echo "deploy_image=$deploy_image"
  echo "active_container=$active_container"
  echo "active_image=$(docker inspect --format '{{.Config.Image}}' "$active_container")"
  echo "target_container=$target_container"
  echo "candidate=$candidate"
  echo "nginx_config=$NGINX_CONFIG"
} > "$rollback_state"

compose=(docker compose -p "platform-${target_color}" -f "$COMPOSE_FILE" -f "$candidate")
rollback_target() {
  trap - ERR INT TERM
  docker rm -f "$target_container" >/dev/null 2>&1 || true
}
trap rollback_target ERR INT TERM

DOCKER_CONFIG="$docker_config" "${compose[@]}" pull sub2api
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

"$CUTOVER_SCRIPT" \
  "$active_container" \
  "$target_container" \
  "$active_upstream" \
  "$target_upstream" \
  "$PUBLIC_HEALTH_URL" \
  "$NGINX_CONFIG"

trap - ERR INT TERM
echo "Blue-green deployment complete: image=$deploy_image rollback_state=$rollback_state"
