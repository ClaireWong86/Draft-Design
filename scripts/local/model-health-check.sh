#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ENV_FILE="$ROOT_DIR/.env.local"
JSON_MODE=false
[[ "${1:-}" == "--json" ]] && JSON_MODE=true

env_value() {
  local name="$1"
  [[ -f "$ENV_FILE" ]] || return 1
  awk -F= -v key="$name" '$1 == key { sub(/^[^=]*=/, ""); print; found=1; exit } END { exit found ? 0 : 1 }' "$ENV_FILE"
}

check_openai() {
  local api_key base_url model
  api_key="$(env_value OPENAI_API_KEY || true)"
  base_url="$(env_value COZE_LOOP_OPENAI_BASE_URL || true)"
  model="$(env_value COZE_LOOP_OPENAI_MODEL || true)"
  base_url="${base_url:-https://api.openai.com/v1}"
  model="${model:-gpt-5.6-luna}"
  [[ -n "$api_key" ]] || return 0
  if response="$(
    /usr/bin/curl -sS --connect-timeout 10 --max-time 20 \
      -X POST "$base_url/chat/completions" \
      -H "Authorization: Bearer $api_key" \
      -H "Content-Type: application/json" \
      --data "{\"model\":\"$model\",\"messages\":[{\"role\":\"user\",\"content\":\"ping\"}],\"max_tokens\":4}"
  )" && printf '%s' "$response" | /usr/bin/python3 -c 'import json,sys; d=json.load(sys.stdin); sys.exit(0 if d.get("choices") else 1)' 2>/dev/null; then
    echo "openai|$model|ok|"
  else
    msg="$(printf '%s' "$response" | /usr/bin/python3 -c 'import json,sys; d=json.load(sys.stdin); print((d.get("error") or {}).get("message","failed")[:120])' 2>/dev/null || echo failed)"
    echo "openai|$model|fail|$msg"
  fi
}

check_ark() {
  local api_key base_url model
  api_key="$(env_value ARK_API_KEY || true)"
  base_url="$(env_value ARK_BASE_URL || true)"
  model="$(env_value ARK_MODEL || true)"
  base_url="${base_url:-https://ark.cn-beijing.volces.com}"
  model="${model:-doubao-seed-2-0-pro-260215}"
  base_url="${base_url%/}"
  case "$base_url" in */api/v3) ;; *) base_url="$base_url/api/v3" ;; esac
  [[ -n "$api_key" ]] || return 0
  if response="$(
    /usr/bin/curl -sS --connect-timeout 10 --max-time 20 \
      -X POST "$base_url/chat/completions" \
      -H "Authorization: Bearer $api_key" \
      -H "Content-Type: application/json" \
      --data "{\"model\":\"$model\",\"messages\":[{\"role\":\"user\",\"content\":\"ping\"}],\"max_tokens\":4}"
  )" && printf '%s' "$response" | /usr/bin/python3 -c 'import json,sys; d=json.load(sys.stdin); sys.exit(0 if d.get("choices") else 1)' 2>/dev/null; then
    echo "ark|$model|ok|"
  else
    msg="$(printf '%s' "$response" | /usr/bin/python3 -c 'import json,sys; d=json.load(sys.stdin); print((d.get("error") or {}).get("message","failed")[:120])' 2>/dev/null || echo failed)"
    echo "ark|$model|fail|$msg"
  fi
}

check_deepseek() {
  local api_key base_url model
  api_key="$(env_value DEEPSEEK_API_KEY || true)"
  base_url="$(env_value DEEPSEEK_BASE_URL || true)"
  model="$(env_value DEEPSEEK_MODEL || true)"
  base_url="${base_url:-https://api.deepseek.com}"
  model="${model:-deepseek-v4-pro}"
  [[ -n "$api_key" ]] || return 0
  if bash "$ROOT_DIR/scripts/local/deepseek-check.sh" >/dev/null 2>&1; then
    echo "deepseek|$model|ok|"
  else
    echo "deepseek|$model|fail|connectivity check failed"
  fi
}

check_joybuild() {
  local api_key model
  api_key="$(env_value JOYBUILD_API_KEY || true)"
  model="$(env_value JOYBUILD_MODEL || true)"
  model="${model:-Gemini-3.1-Flash-Lite}"
  [[ -n "$api_key" ]] || return 0
  if bash "$ROOT_DIR/scripts/local/joybuild-check.sh" >/dev/null 2>&1; then
    echo "joybuild|$model|ok|"
  else
    echo "joybuild|$model|fail|connectivity check failed"
  fi
}

results=()
while IFS= read -r line; do
  [[ -n "$line" ]] && results+=("$line")
done < <(
  check_openai
  check_ark
  check_deepseek
  check_joybuild
)

preferred="$(env_value COZE_LOOP_PREFERRED_MODELS || true)"
preferred="${preferred:-deepseek-v4-pro,Gemini-3.1-Flash-Lite}"

if $JSON_MODE; then
  printf '%s\n' "${results[@]}"
  exit 0
fi

echo "Model health:"
for row in "${results[@]}"; do
  IFS='|' read -r provider model status message <<<"$row"
  if [[ "$status" == "ok" ]]; then
    echo "  OK   $model ($provider)"
  else
    echo "  FAIL $model ($provider) - $message"
  fi
done
echo "Preferred order: $preferred"
