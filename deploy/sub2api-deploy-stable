#!/usr/bin/env bash

set -Eeuo pipefail

umask 077

readonly IMAGE_NAME="ghcr.io/xuehua123/sub2api"
readonly DEPLOY_DIR="/opt/platform"
readonly COMPOSE_FILE="docker-compose.yml"
readonly LOCK_FILE="/var/lock/sub2api-stable-deploy.lock"

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

if [[ ! -f "${DEPLOY_DIR}/${COMPOSE_FILE}" ]]; then
  echo "Stable compose file not found: ${DEPLOY_DIR}/${COMPOSE_FILE}" >&2
  exit 1
fi

exec 9>"$LOCK_FILE"
if ! flock -n 9; then
  echo "Another stable deployment is already running" >&2
  exit 1
fi

cd "$DEPLOY_DIR"

mapfile -t services < <(
  docker compose -f "$COMPOSE_FILE" config --format json |
    python3 -c 'import json, sys
config = json.load(sys.stdin)
prefixes = ("ghcr.io/xuehua123/sub2api:", "ghcr.io/xuehua123/sub2api@")
for name, service in config.get("services", {}).items():
    image = service.get("image", "")
    if image == "ghcr.io/xuehua123/sub2api" or image.startswith(prefixes):
        print(name)'
)

if [[ "${#services[@]}" -eq 0 ]]; then
  echo "No Sub2API application services found in ${DEPLOY_DIR}/${COMPOSE_FILE}" >&2
  exit 1
fi

timestamp="$(date +%Y%m%d-%H%M%S)"
safe_tag="$(printf '%s' "$deploy_image_tag" | tr '/:@' '---')"
backup="${COMPOSE_FILE}.bak-${timestamp}-pre-${safe_tag}"
rollback_state="rollback-state-${timestamp}-pre-${safe_tag}.txt"

cp --preserve=mode,ownership,timestamps "$COMPOSE_FILE" "$backup"

{
  echo "timestamp=$timestamp"
  echo "deploy_image=$deploy_image"
  echo "compose_backup=$backup"
  for service in "${services[@]}"; do
    container_id="$(docker compose -f "$COMPOSE_FILE" ps -q "$service" || true)"
    echo "service=$service"
    echo "container_id=$container_id"
    if [[ -n "$container_id" ]]; then
      echo "container_image=$(docker inspect --format '{{.Config.Image}}' "$container_id")"
      echo "container_image_id=$(docker inspect --format '{{.Image}}' "$container_id")"
    fi
  done
} > "$rollback_state"

rollback() {
  trap - ERR INT TERM
  echo "Deployment failed; restoring $backup" >&2
  cp --preserve=mode,ownership,timestamps "$backup" "$COMPOSE_FILE"
  for service in "${services[@]}"; do
    docker compose -f "$COMPOSE_FILE" up -d --no-deps "$service" || true
  done
  docker compose -f "$COMPOSE_FILE" ps || true
}
trap rollback ERR INT TERM

DEPLOY_IMAGE="$deploy_image" COMPOSE_PATH="${DEPLOY_DIR}/${COMPOSE_FILE}" python3 - <<'PY'
from pathlib import Path
import os
import re

path = Path(os.environ["COMPOSE_PATH"])
image = os.environ["DEPLOY_IMAGE"]
lines = path.read_text().splitlines()
pattern = re.compile(
    r"^(\s*image:\s*)ghcr\.io/xuehua123/sub2api(?::[^\s#]+|@[^\s#]+)?(\s*(?:#.*)?)$"
)
replaced = 0
output = []
for line in lines:
    match = pattern.match(line)
    if match:
        line = f"{match.group(1)}{image}{match.group(2)}"
        replaced += 1
    output.append(line)
if replaced == 0:
    raise SystemExit("No Sub2API image entries found in compose file")
path.write_text("\n".join(output) + "\n")
print(f"Updated {replaced} Sub2API image entries")
PY

docker compose -f "$COMPOSE_FILE" pull "${services[@]}"

for service in "${services[@]}"; do
  docker compose -f "$COMPOSE_FILE" up -d --no-deps "$service"
  container_id="$(docker compose -f "$COMPOSE_FILE" ps -q "$service")"
  [[ -n "$container_id" ]]

  for ((attempt = 0; attempt < 90; attempt++)); do
    running="$(docker inspect --format '{{.State.Running}}' "$container_id")"
    health="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$container_id")"

    if [[ "$running" == "true" && "$health" == "healthy" ]]; then
      break
    fi
    if [[ "$running" != "true" || "$health" == "unhealthy" ]]; then
      docker logs --tail 150 "$container_id" >&2 || true
      false
    fi
    if [[ "$health" == "none" && "$attempt" -ge 5 ]]; then
      break
    fi
    sleep 2
  done

  if [[ "$attempt" -ge 90 ]]; then
    docker logs --tail 150 "$container_id" >&2 || true
    false
  fi

  actual_image="$(docker inspect --format '{{.Config.Image}}' "$container_id")"
  if [[ "$actual_image" != "$deploy_image" ]]; then
    echo "Unexpected image for $service: $actual_image" >&2
    false
  fi
done

trap - ERR INT TERM
docker compose -f "$COMPOSE_FILE" ps "${services[@]}"
echo "Rollback state: ${DEPLOY_DIR}/${rollback_state}"
echo "Compose backup: ${DEPLOY_DIR}/${backup}"
