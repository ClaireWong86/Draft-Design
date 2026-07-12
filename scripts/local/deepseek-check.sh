#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ENV_FILE="$ROOT_DIR/.env.local"

env_value() {
  local name="$1"

  [[ -f "$ENV_FILE" ]] || return 1
  awk -F= -v key="$name" '$1 == key { sub(/^[^=]*=/, ""); print; found=1; exit } END { exit found ? 0 : 1 }' "$ENV_FILE"
}

api_key="$(env_value DEEPSEEK_API_KEY || true)"
base_url="$(env_value DEEPSEEK_BASE_URL || true)"
model="$(env_value DEEPSEEK_MODEL || true)"

base_url="${base_url:-https://api.deepseek.com}"
model="${model:-deepseek-v4-pro}"
base_url="${base_url%/}"

if [[ -z "$api_key" ]]; then
  echo "FAIL: DEEPSEEK_API_KEY is not configured in .env.local"
  exit 1
fi

echo "Checking DeepSeek chat endpoint: $base_url"
if ! response="$(
  /usr/bin/curl -sS --connect-timeout 10 --max-time 30 \
    -X POST "$base_url/chat/completions" \
    -H "Authorization: Bearer $api_key" \
    -H "Content-Type: application/json" \
    --data "{\"model\":\"$model\",\"messages\":[{\"role\":\"user\",\"content\":\"ping\"}],\"max_tokens\":8}"
)"; then
  echo "FAIL: cannot connect to DeepSeek endpoint"
  exit 1
fi

DEEPSEEK_CHECK_RESPONSE="$response" /usr/bin/python3 - "$model" <<'PY'
import json
import os
import sys

model = sys.argv[1]
data = json.loads(os.environ["DEEPSEEK_CHECK_RESPONSE"])

if data.get("error"):
    print(f"FAIL: {data['error'].get('message', 'DeepSeek endpoint returned an error')}")
    sys.exit(1)

choices = data.get("choices") or []
if not choices:
    print("FAIL: DeepSeek endpoint returned no choices")
    sys.exit(1)

print(f"OK: DeepSeek model responded: {model}")
PY
