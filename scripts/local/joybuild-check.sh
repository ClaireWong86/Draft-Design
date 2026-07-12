#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ENV_FILE="$ROOT_DIR/.env.local"

env_value() {
  local name="$1"

  [[ -f "$ENV_FILE" ]] || return 1
  awk -F= -v key="$name" '$1 == key { sub(/^[^=]*=/, ""); print; found=1; exit } END { exit found ? 0 : 1 }' "$ENV_FILE"
}

api_key="$(env_value JOYBUILD_API_KEY || true)"
base_url="$(env_value JOYBUILD_BASE_URL || true)"
model="$(env_value JOYBUILD_MODEL || true)"

base_url="${base_url:-http://ai-api.jdcloud.com}"
model="${model:-Gemini-3.1-Flash-Lite}"
base_url="${base_url%/}"
case "$base_url" in
  */v1) runtime_base_url="$base_url" ;;
  *) runtime_base_url="$base_url/v1" ;;
esac

if [[ -z "$api_key" ]]; then
  echo "FAIL: JOYBUILD_API_KEY is not configured in .env.local"
  exit 1
fi

echo "Checking JoyBuild OpenAI-compatible endpoint: $runtime_base_url"
if ! response="$(
  /usr/bin/curl -sS --connect-timeout 10 --max-time 30 \
    -X POST "$runtime_base_url/chat/completions" \
    -H "Authorization: Bearer $api_key" \
    -H "Content-Type: application/json" \
    --data "{\"model\":\"$model\",\"messages\":[{\"role\":\"user\",\"content\":\"ping\"}],\"max_tokens\":8}"
)"; then
  echo "FAIL: cannot connect to JoyBuild endpoint"
  exit 1
fi

JOYBUILD_CHECK_RESPONSE="$response" /usr/bin/python3 - "$model" <<'PY'
import json
import os
import sys

model = sys.argv[1]
data = json.loads(os.environ["JOYBUILD_CHECK_RESPONSE"])

if data.get("error"):
    print(f"FAIL: {data['error'].get('message', 'JoyBuild endpoint returned an error')}")
    sys.exit(1)

choices = data.get("choices") or []
if not choices:
    print("FAIL: JoyBuild endpoint returned no choices")
    sys.exit(1)

print(f"OK: JoyBuild model responded: {model}")
PY
