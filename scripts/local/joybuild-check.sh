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

check_openai() {
  echo "Checking JoyBuild OpenAI-compatible endpoint: $runtime_base_url/chat/completions"
  if ! response="$(
    /usr/bin/curl -sS --connect-timeout 10 --max-time 30 \
      -X POST "$runtime_base_url/chat/completions" \
      -H "Authorization: Bearer $api_key" \
      -H "Content-Type: application/json" \
      --data "{\"model\":\"$model\",\"messages\":[{\"role\":\"user\",\"content\":\"ping\"}],\"max_tokens\":8}"
  )"; then
    echo "WARN: cannot connect to OpenAI-compatible endpoint"
    return 1
  fi

  JOYBUILD_CHECK_RESPONSE="$response" /usr/bin/python3 - "$model" <<'PY'
import json
import os
import sys

model = sys.argv[1]
data = json.loads(os.environ["JOYBUILD_CHECK_RESPONSE"])

err = data.get("error")
if err:
    msg = err.get("message") if isinstance(err, dict) else str(err)
    print(f"WARN: OpenAI-compatible check failed: {msg}")
    sys.exit(1)

if not (data.get("choices") or []):
    print("WARN: OpenAI-compatible endpoint returned no choices")
    sys.exit(1)

print(f"OK: JoyBuild OpenAI-compatible model responded: {model}")
PY
}

check_native_responses() {
  echo "Checking JoyBuild native endpoint: $base_url/v1/responses (tire-ai-diagnosis protocol)"
  if ! response="$(
    /usr/bin/curl -sS --connect-timeout 10 --max-time 30 \
      -X POST "$base_url/v1/responses" \
      -H "Authorization: Bearer $api_key" \
      -H "Content-Type: application/json" \
      -H "Trace-Id: prompt-loop-joybuild-check" \
      --data "{\"model\":\"$model\",\"contents\":[{\"role\":\"user\",\"parts\":[{\"text\":\"ping\"}]}],\"generationConfig\":{\"maxOutputTokens\":8}}"
  )"; then
    echo "FAIL: cannot connect to JoyBuild /v1/responses"
    return 1
  fi

  JOYBUILD_CHECK_RESPONSE="$response" /usr/bin/python3 - "$model" <<'PY'
import json
import os
import sys

model = sys.argv[1]
data = json.loads(os.environ["JOYBUILD_CHECK_RESPONSE"])

err = data.get("error")
if err:
    msg = err.get("message") if isinstance(err, dict) else str(err)
    print(f"FAIL: native /v1/responses error: {msg}")
    sys.exit(1)

# Accept common JoyBuild/Gemini-style success shapes without dumping payload.
if data.get("candidates") or data.get("output") or data.get("response") or data.get("output_text"):
    print(f"OK: JoyBuild native model responded: {model}")
    sys.exit(0)

# Some gateways return text fields at top level.
if any(isinstance(data.get(k), str) and data.get(k).strip() for k in ("text", "content", "result")):
    print(f"OK: JoyBuild native model responded: {model}")
    sys.exit(0)

print("FAIL: native /v1/responses returned unrecognized payload")
sys.exit(1)
PY
}

openai_ok=0
native_ok=0

if check_openai; then
  openai_ok=1
fi

if check_native_responses; then
  native_ok=1
fi

if [[ "$openai_ok" -eq 1 ]]; then
  exit 0
fi

if [[ "$native_ok" -eq 1 ]]; then
  echo "NOTE: Native JoyBuild (/v1/responses) works — same as tire-ai-diagnosis."
  echo "NOTE: Prompt Loop should mount JoyBuild as protocol=joybuild (/v1/responses)."
  exit 0
fi

echo "FAIL: JoyBuild checks failed for both OpenAI-compatible and native protocols"
exit 1
