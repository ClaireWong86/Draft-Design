#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ENV_FILE="$ROOT_DIR/.env.local"
PROXY_JS="$ROOT_DIR/scripts/local/joybuild-openai-proxy.mjs"

env_value() {
  local name="$1"

  [[ -f "$ENV_FILE" ]] || return 1
  awk -F= -v key="$name" '$1 == key { sub(/^[^=]*=/, ""); print; found=1; exit } END { exit found ? 0 : 1 }' "$ENV_FILE"
}

export JOYBUILD_API_KEY="${JOYBUILD_API_KEY:-$(env_value JOYBUILD_API_KEY || true)}"
export JOYBUILD_BASE_URL="${JOYBUILD_BASE_URL:-$(env_value JOYBUILD_BASE_URL || true)}"
export JOYBUILD_MODEL="${JOYBUILD_MODEL:-$(env_value JOYBUILD_MODEL || true)}"
export JOYBUILD_PROXY_PORT="${JOYBUILD_PROXY_PORT:-18081}"
export JOYBUILD_PROXY_HOST="${JOYBUILD_PROXY_HOST:-127.0.0.1}"

JOYBUILD_BASE_URL="${JOYBUILD_BASE_URL:-http://ai-api.jdcloud.com}"
JOYBUILD_MODEL="${JOYBUILD_MODEL:-Gemini-3.1-Flash-Lite}"

if [[ -z "${JOYBUILD_API_KEY:-}" ]]; then
  echo "FAIL: JOYBUILD_API_KEY is not configured (.env.local or env)"
  exit 1
fi

if ! command -v node >/dev/null 2>&1; then
  echo "FAIL: node is required to run the JoyBuild OpenAI proxy"
  exit 1
fi

if curl -fsS "http://127.0.0.1:${JOYBUILD_PROXY_PORT}/health" >/dev/null 2>&1; then
  echo "JoyBuild proxy already healthy on :${JOYBUILD_PROXY_PORT}"
  exit 0
fi

echo "Starting JoyBuild OpenAI proxy on ${JOYBUILD_PROXY_HOST}:${JOYBUILD_PROXY_PORT}"
exec node "$PROXY_JS"
