#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ENV_FILE="$ROOT_DIR/.env.local"

env_value() {
  local name="$1"

  [[ -f "$ENV_FILE" ]] || return 1
  awk -F= -v key="$name" '$1 == key { sub(/^[^=]*=/, ""); print; found=1; exit } END { exit found ? 0 : 1 }' "$ENV_FILE"
}

env_or_file_value() {
  local name="$1"
  local fallback="$2"
  local value="${!name:-}"

  if [[ -z "$value" ]]; then
    value="$(env_value "$name" || true)"
  fi

  printf '%s' "${value:-$fallback}"
}

api_key="$(env_or_file_value OPENAI_API_KEY "")"
base_url="$(env_or_file_value COZE_LOOP_OPENAI_BASE_URL "https://api.openai.com/v1")"
model="$(env_or_file_value COZE_LOOP_OPENAI_MODEL "gpt-5.6-luna")"

if [[ -z "$api_key" ]]; then
  echo "FAIL: OPENAI_API_KEY is not configured in .env.local"
  exit 1
fi

echo "Checking OpenAI-compatible endpoint: $base_url"
if ! response="$(
  /usr/bin/curl -sS --connect-timeout 10 --max-time 20 \
    "$base_url/models" \
    -H "Authorization: Bearer $api_key" \
)"; then
  echo "FAIL: cannot connect to OpenAI-compatible endpoint"
  exit 1
fi

OPENAI_CHECK_RESPONSE="$response" /usr/bin/python3 - "$model" <<'PY'
import json
import os
import sys

model = sys.argv[1]
data = json.loads(os.environ["OPENAI_CHECK_RESPONSE"])

if data.get("error"):
    print(f"FAIL: {data['error'].get('message', 'OpenAI endpoint returned an error')}")
    sys.exit(1)

models = [item.get("id", "") for item in data.get("data", [])]
print(f"OK: endpoint returned {len(models)} model(s)")
if model:
    print(f"Configured model: {model}")
    print(f"Model listed by endpoint: {'yes' if model in models else 'no'}")
PY
