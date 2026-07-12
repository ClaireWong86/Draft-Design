#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SOURCE_ENV="${TIRE_AI_DIAGNOSIS_ENV:-/Users/wangdujuan/Projects/tire-ai-diagnosis/services/api/.env}"
TARGET_ENV="$ROOT_DIR/.env.local"
NAMES=(ARK_API_KEY ARK_BASE_URL ARK_MODEL)

if [[ ! -f "$SOURCE_ENV" ]]; then
  echo "Source env file not found: $SOURCE_ENV" >&2
  exit 1
fi

for name in "${NAMES[@]}"; do
  if ! /usr/bin/grep -q "^$name=" "$SOURCE_ENV"; then
    echo "Missing $name in $SOURCE_ENV" >&2
    exit 1
  fi
done

touch "$TARGET_ENV"
chmod 600 "$TARGET_ENV"

tmp_file="$(mktemp)"
cp "$TARGET_ENV" "$tmp_file"

for name in "${NAMES[@]}"; do
  value="$(awk -F= -v key="$name" '$1 == key { sub(/^[^=]*=/, ""); print; found=1; exit } END { exit found ? 0 : 1 }' "$SOURCE_ENV")"
  if /usr/bin/grep -q "^$name=" "$tmp_file"; then
    awk -v key="$name" -v value="$value" 'BEGIN { updated=0 } $0 ~ "^" key "=" { print key "=" value; updated=1; next } { print } END { if (!updated) print key "=" value }' "$tmp_file" >"$tmp_file.next"
  else
    cp "$tmp_file" "$tmp_file.next"
    printf '%s=%s\n' "$name" "$value" >>"$tmp_file.next"
  fi
  mv "$tmp_file.next" "$tmp_file"
done

mv "$tmp_file" "$TARGET_ENV"
chmod 600 "$TARGET_ENV"

echo "Imported Ark config into .env.local: ARK_API_KEY, ARK_BASE_URL, ARK_MODEL"
