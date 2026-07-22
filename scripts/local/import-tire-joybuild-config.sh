#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SOURCE_ENV="${TIRE_AI_DIAGNOSIS_ENV:-/Users/wangdujuan10/tire-ai-diagnosis/services/api/.env}"
SOURCE_EXAMPLE="${TIRE_AI_DIAGNOSIS_ENV_EXAMPLE:-/Users/wangdujuan10/tire-ai-diagnosis/services/api/.env.example}"
TARGET_ENV="$ROOT_DIR/.env.local"

env_value_from_file() {
  local file="$1"
  local name="$2"

  [[ -f "$file" ]] || return 1
  awk -F= -v key="$name" '
    $1 == key {
      sub(/^[^=]*=/, "")
      print
      found = 1
      exit
    }
    END { exit found ? 0 : 1 }
  ' "$file"
}

example_value_from_file() {
  local file="$1"
  local name="$2"

  [[ -f "$file" ]] || return 1
  awk -v key="$name" '
    $0 ~ "^#?[[:space:]]*" key "=" {
      line = $0
      sub(/^#?[[:space:]]*/, "", line)
      sub(/^[^=]*=/, "", line)
      print line
      found = 1
      exit
    }
    END { exit found ? 0 : 1 }
  ' "$file"
}

resolve_value() {
  local name="$1"
  local fallback="$2"
  local value=""

  value="$(env_value_from_file "$SOURCE_ENV" "$name" || true)"
  if [[ -z "$value" ]]; then
    value="$(example_value_from_file "$SOURCE_EXAMPLE" "$name" || true)"
  fi
  if [[ -z "$value" ]]; then
    value="$fallback"
  fi
  printf '%s' "$value"
}

if [[ ! -f "$SOURCE_ENV" && ! -f "$SOURCE_EXAMPLE" ]]; then
  echo "Source env files not found:" >&2
  echo "  $SOURCE_ENV" >&2
  echo "  $SOURCE_EXAMPLE" >&2
  exit 1
fi

api_key="$(resolve_value JOYBUILD_API_KEY "")"
base_url="$(resolve_value JOYBUILD_BASE_URL "http://ai-api.jdcloud.com")"
model="$(resolve_value JOYBUILD_MODEL "Gemini-3.1-Flash-Lite")"
base_url="${base_url%/}"

if [[ -z "$api_key" || "$api_key" == *"***"* ]]; then
  echo "FAIL: JOYBUILD_API_KEY is not configured in tire-ai-diagnosis." >&2
  echo "Add these to $SOURCE_ENV first:" >&2
  echo "  JOYBUILD_BASE_URL=http://ai-api.jdcloud.com" >&2
  echo "  JOYBUILD_API_KEY=pk-..." >&2
  echo "  JOYBUILD_MODEL=Gemini-3.1-Flash-Lite" >&2
  exit 1
fi

touch "$TARGET_ENV"
chmod 600 "$TARGET_ENV"

tmp_file="$(mktemp)"
cp "$TARGET_ENV" "$tmp_file"

upsert_env() {
  local name="$1"
  local value="$2"

  if /usr/bin/grep -q "^$name=" "$tmp_file"; then
    awk -v key="$name" -v value="$value" 'BEGIN { updated=0 } $0 ~ "^" key "=" { print key "=" value; updated=1; next } { print } END { if (!updated) print key "=" value }' "$tmp_file" >"$tmp_file.next"
  else
    cp "$tmp_file" "$tmp_file.next"
    printf '%s=%s\n' "$name" "$value" >>"$tmp_file.next"
  fi
  mv "$tmp_file.next" "$tmp_file"
}

upsert_env JOYBUILD_API_KEY "$api_key"
upsert_env JOYBUILD_BASE_URL "$base_url"
upsert_env JOYBUILD_MODEL "$model"

mv "$tmp_file" "$TARGET_ENV"
chmod 600 "$TARGET_ENV"

echo "Imported JoyBuild config into .env.local: JOYBUILD_API_KEY, JOYBUILD_BASE_URL, JOYBUILD_MODEL"
