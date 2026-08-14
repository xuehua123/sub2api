#!/usr/bin/env bash

set -Eeuo pipefail

REPO_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
INSTALLER="$REPO_ROOT/deploy/sub2api-install-stable-ovh.sh"
STABLE="$REPO_ROOT/deploy/sub2api-deploy-stable.sh"
CUTOVER="$REPO_ROOT/deploy/sub2api-nginx-bluegreen-cutover.sh"
BOOTSTRAP="$REPO_ROOT/deploy/sub2api-deploy-state-bootstrap.sh"

bash -n "$INSTALLER"
bash -n "$STABLE"
bash -n "$CUTOVER"

grep -Fq 'readonly DEPLOY_DIR="/srv/sub2api-migration/incoming"' "$STABLE"
grep -Fq 'readonly COMPOSE_FILE="${DEPLOY_DIR}/docker-compose.migration.yml"' "$STABLE"
grep -Fq 'readonly COMPOSE_ENV_FILE="${DEPLOY_DIR}/sub2api.env"' "$STABLE"
grep -Fq 'readonly NGINX_CONFIG="/etc/nginx/snippets/sub2api-active-upstream.conf"' "$STABLE"
grep -Fq 'docker compose --env-file "$COMPOSE_ENV_FILE" -p "sub2api-stable-${target_color}"' "$STABLE"
grep -Fq '  sub2api-net:' "$STABLE"
grep -Fq '    name: sub2api-net' "$STABLE"

cutover_hash=$(sha256sum "$CUTOVER" | awk '{print $1}')
deploy_hash=$(sha256sum "$STABLE" | awk '{print $1}')
bootstrap_hash=$(sha256sum "$BOOTSTRAP" | awk '{print $1}')
grep -Fq "readonly CUTOVER_SCRIPT_SHA256=\"$cutover_hash\"" "$STABLE"
grep -Fq "readonly DEPLOY_SCRIPT_SHA256=\"$deploy_hash\"" "$INSTALLER"
grep -Fq "readonly CUTOVER_SCRIPT_SHA256=\"$cutover_hash\"" "$INSTALLER"
grep -Fq "readonly BOOTSTRAP_SCRIPT_SHA256=\"$bootstrap_hash\"" "$INSTALLER"

grep -Fq 'readonly PUBLIC_CONFIG="/etc/nginx/sites-available/sub2api-public.conf"' "$INSTALLER"
grep -Fq 'readonly WG_CONFIG="/etc/nginx/sites-available/sub2api-wg.conf"' "$INSTALLER"
grep -Fq 'readonly ACTIVE_UPSTREAM_CONFIG="/etc/nginx/snippets/sub2api-active-upstream.conf"' "$INSTALLER"
grep -Fq 'PUBLIC_HEALTH_RESOLVE="api.wenrugouai.com:443:40.160.58.167"' "$INSTALLER"
grep -Fq 'docker compose --env-file "$COMPOSE_ENV_FILE" -p sub2api-stable-preflight -f "$COMPOSE_FILE" config --quiet' "$INSTALLER"
grep -Fq 'docker network inspect sub2api-net' "$INSTALLER"
grep -Fq 'docker volume inspect sub2api_data' "$INSTALLER"
grep -Fq 'source_snapshot="$(mktemp -d /run/sub2api-stable-install-source.XXXXXX)"' "$INSTALLER"
grep -Fq 'readonly INSTALL_LOCK_DIR="/run/sub2api-stable-install"' "$INSTALLER"
grep -Fq 'flock -n 8' "$INSTALLER"
grep -Fq 'flock -n 9' "$INSTALLER"
grep -Fq 'config --format json > "$compose_json"' "$INSTALLER"
grep -Fq '.services.sub2api.environment.DATABASE_HOST == "sub2api-postgres"' "$INSTALLER"
grep -Fq 'Compose environment file must be root-owned with mode 0600' "$INSTALLER"
grep -Fq 'readonly PUBLIC_ENABLED="/etc/nginx/sites-enabled/sub2api-public.conf"' "$INSTALLER"
grep -Fq 'readonly WG_ENABLED="/etc/nginx/sites-enabled/sub2api-wg.conf"' "$INSTALLER"
grep -Fq 'revalidate_precommit' "$INSTALLER"
grep -Fq 'Nginx reload did not create a new worker' "$INSTALLER"
grep -Fq 'cp --preserve=mode,ownership -- "$PUBLIC_CONFIG" "$backup_dir/sub2api-public.conf.before"' "$INSTALLER"
grep -Fq 'mv -Tf -- "$include_candidate" "$ACTIVE_UPSTREAM_CONFIG"' "$INSTALLER"
grep -Fq "trap 'rollback_install \$?' ERR" "$INSTALLER"
grep -Fq 'automatic rollback could not be verified; operator intervention is required' "$INSTALLER"

if grep -Eq 'StrictHostKeyChecking=no|docker system prune|docker image prune' "$INSTALLER" "$STABLE"; then
  echo "OVH deployment scripts contain a prohibited unsafe operation" >&2
  exit 1
fi

echo "OVH stable installer policy tests passed"
