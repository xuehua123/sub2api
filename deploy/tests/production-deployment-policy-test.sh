#!/usr/bin/env bash

set -Eeuo pipefail

REPO_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
STABLE_SCRIPT="$REPO_ROOT/deploy/sub2api-deploy-stable.sh"
CANARY_WORKFLOW="$REPO_ROOT/.github/workflows/deploy-shanghai.yml"
AGENT_RULES="$REPO_ROOT/AGENTS.md"

grep -Fq 'blue) printf '\''%s'\'' 18080' "$STABLE_SCRIPT"
grep -Fq 'green) printf '\''%s'\'' 28080' "$STABLE_SCRIPT"
grep -Fq '127.0.0.1:18080:8080' "$STABLE_SCRIPT"
grep -Fq '127.0.0.1:28080:8080' "$STABLE_SCRIPT"

if grep -Fq '18082' "$STABLE_SCRIPT"; then
  echo "Stable deployment still contains the obsolete green port 18082" >&2
  exit 1
fi

if grep -Eqi 'pipixia|NEW_PIPIXIA|appleboy/ssh-action' "$CANARY_WORKFLOW"; then
  echo "Canary workflow must not contain a Pipixia application deployment path" >&2
  exit 1
fi

grep -Fq '**Canada** is the only active Pipixia application host' "$AGENT_RULES"
grep -Fq '**United States** is an edge relay only' "$AGENT_RULES"
grep -Fq '**France** is disaster recovery only' "$AGENT_RULES"

echo "production deployment policy tests passed"
