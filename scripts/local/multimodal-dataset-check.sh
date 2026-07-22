#!/usr/bin/env bash
# End-to-end check: create evaluation set with MultiPart column, upload image, add item, read back.
set -euo pipefail

BASE_URL="${COZE_LOOP_BASE_URL:-http://localhost:8082}"
EMAIL="${COZE_LOOP_SMOKE_EMAIL:-codex-local-smoke@example.com}"
PASSWORD="${COZE_LOOP_SMOKE_PASSWORD:-Codex123456}"
FIXTURE=""

curl_json() {
  /usr/bin/curl -sS "$@"
}

require_code_zero() {
  local body="$1"
  local label="$2"
  local code
  code="$(printf '%s' "$body" | /usr/bin/python3 -c 'import json,sys; print(json.load(sys.stdin).get("code", ""))')"
  if [[ "$code" != "0" ]]; then
    echo "FAIL: $label returned code=$code"
    printf '%s\n' "$body"
    exit 1
  fi
}

if [[ ! -f "$FIXTURE" ]]; then
  FIXTURE="$(mktemp /tmp/multimodal-test.XXXXXX.png)"
  /usr/bin/python3 - <<'PY' "$FIXTURE"
import struct, sys, zlib
path = sys.argv[1]
width, height = 8, 8
raw = b"".join(b"\x00" + b"\xff\x00\x00" * width for _ in range(height))
def chunk(tag, data):
    return struct.pack(">I", len(data)) + tag + data + struct.pack(">I", zlib.crc32(tag + data) & 0xFFFFFFFF)
png = b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", width, height, 8, 2, 0, 0, 0)) + chunk(b"IDAT", zlib.compress(raw)) + chunk(b"IEND", b"")
open(path, "wb").write(png)
PY
  trap 'rm -f "$FIXTURE"' EXIT
fi

echo "Checking API: $BASE_URL"
if /usr/bin/curl -fsS "$BASE_URL/ping" >/dev/null 2>&1; then
  :
elif /usr/bin/curl -fsSI "$BASE_URL/auth/login" >/dev/null 2>&1; then
  :
else
  echo "FAIL: API not reachable at $BASE_URL"
  exit 1
fi

echo "Logging in as $EMAIL"
login_body="$(curl_json -X POST "$BASE_URL/api/foundation/v1/users/login_by_password" \
  -H 'Content-Type: application/json' \
  --data "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}")"
require_code_zero "$login_body" "login"
token="$(printf '%s' "$login_body" | /usr/bin/python3 -c 'import json,sys; print(json.load(sys.stdin).get("token", ""))')"
if [[ -z "$token" ]]; then
  echo "FAIL: login did not return session token"
  exit 1
fi

spaces_body="$(curl_json -X POST "$BASE_URL/api/foundation/v1/spaces/list" \
  -H 'Content-Type: application/json' \
  -H "Cookie: session_key=$token" \
  --data '{"page_number":1,"page_size":100}')"
require_code_zero "$spaces_body" "spaces/list"
space_id="$(printf '%s' "$spaces_body" | /usr/bin/python3 -c 'import json,sys; print(json.load(sys.stdin)["spaces"][0]["id"])')"

echo "Signing upload URL in space $space_id"
upload_key="${space_id}/multimodal-e2e-$(date +%s)-$(basename "$FIXTURE")"
sign_body="$(curl_json -X POST "$BASE_URL/api/foundation/v1/sign_upload_files" \
  -H 'Content-Type: application/json' \
  -H "Cookie: session_key=$token" \
  --data "{\"workspace_id\":\"$space_id\",\"keys\":[\"$upload_key\"],\"business_type\":\"evaluation\"}")"
require_code_zero "$sign_body" "sign_upload_files"
upload_url="$(printf '%s' "$sign_body" | /usr/bin/python3 -c 'import json,sys; print(json.load(sys.stdin).get("uris", [""])[0])')"
if [[ -z "$upload_url" ]]; then
  echo "FAIL: sign_upload_files returned empty uri"
  printf '%s\n' "$sign_body"
  exit 1
fi
if [[ "$upload_url" != http://* && "$upload_url" != https://* ]]; then
  upload_url="${BASE_URL%/}${upload_url}"
fi

echo "Uploading fixture to object storage"
upload_status="$(/usr/bin/curl -sS -o /dev/null -w '%{http_code}' -X PUT --data-binary @"$FIXTURE" "$upload_url")"
if [[ "$upload_status" != "200" && "$upload_status" != "204" ]]; then
  echo "FAIL: object upload HTTP $upload_status"
  exit 1
fi

suffix="${FIXTURE##*.}"
name="multimodal-e2e.${suffix}"
set_name="multimodal-e2e-$(date +%s)"

echo "Creating evaluation set with MultiPart input column"
create_body="$(curl_json -X POST "$BASE_URL/api/evaluation/v1/evaluation_sets" \
  -H 'Content-Type: application/json' \
  -H "Cookie: session_key=$token" \
  --data @- <<EOF
{
  "workspace_id": "$space_id",
  "name": "$set_name",
  "description": "multimodal e2e check",
  "evaluation_set_schema": {
    "workspace_id": "$space_id",
    "field_schemas": [
      {
        "name": "input",
        "content_type": "MultiPart",
        "description": "text + image input"
      },
      {
        "name": "reference_output",
        "content_type": "Text",
        "text_schema": "{\\"type\\":\\"string\\"}"
      }
    ]
  }
}
EOF
)"
require_code_zero "$create_body" "create evaluation set"
eval_set_id="$(printf '%s' "$create_body" | /usr/bin/python3 -c 'import json,sys; print(json.load(sys.stdin).get("evaluation_set_id", ""))')"

get_body="$(curl_json -X GET "$BASE_URL/api/evaluation/v1/evaluation_sets/$eval_set_id?workspace_id=$space_id" \
  -H "Cookie: session_key=$token")"
require_code_zero "$get_body" "get evaluation set"
field_keys="$(printf '%s' "$get_body" | /usr/bin/python3 -c '
import json, sys
data = json.load(sys.stdin)
fields = data.get("evaluation_set", {}).get("evaluation_set_version", {}).get("evaluation_set_schema", {}).get("field_schemas", [])
keys = {f.get("name"): f.get("key", "") for f in fields if f.get("name")}
print(keys.get("input", ""))
print(keys.get("reference_output", ""))
')"
input_key="$(printf '%s\n' "$field_keys" | sed -n '1p')"
ref_output_key="$(printf '%s\n' "$field_keys" | sed -n '2p')"
if [[ -z "$input_key" ]]; then
  echo "FAIL: could not resolve input column key"
  printf '%s\n' "$get_body"
  exit 1
fi

echo "Adding multipart item (text + image) to evaluation set $eval_set_id"
batch_body="$(curl_json -X POST "$BASE_URL/api/evaluation/v1/evaluation_sets/$eval_set_id/items/batch_create" \
  -H 'Content-Type: application/json' \
  -H "Cookie: session_key=$token" \
  --data @- <<EOF
{
  "workspace_id": "$space_id",
  "evaluation_set_id": "$eval_set_id",
  "skip_invalid_items": true,
  "allow_partial_add": true,
  "items": [
    {
      "workspace_id": "$space_id",
      "evaluation_set_id": "$eval_set_id",
      "turns": [
        {
          "field_data_list": [
            {
              "key": "$input_key",
              "name": "input",
              "content": {
                "content_type": "MultiPart",
                "multi_part": [
                  {
                    "content_type": "Text",
                    "text": "tire tread wear pattern"
                  },
                  {
                    "content_type": "Image",
                    "image": {
                      "name": "$name",
                      "uri": "$upload_key",
                      "storage_provider": 5
                    }
                  }
                ]
              }
            },
            {
              "key": "${ref_output_key:-reference_output}",
              "name": "reference_output",
              "content": {
                "content_type": "Text",
                "text": "normal wear"
              }
            }
          ]
        }
      ]
    }
  ]
}
EOF
)"
require_code_zero "$batch_body" "batch_create items"
added_count="$(printf '%s' "$batch_body" | /usr/bin/python3 -c 'import json,sys; print(len(json.load(sys.stdin).get("added_items") or {}))')"
if [[ "$added_count" != "1" ]]; then
  echo "FAIL: expected 1 added item, got $added_count"
  printf '%s\n' "$batch_body"
  exit 1
fi

list_body="$(curl_json -X POST "$BASE_URL/api/evaluation/v1/evaluation_sets/$eval_set_id/items/list" \
  -H 'Content-Type: application/json' \
  -H "Cookie: session_key=$token" \
  --data "{\"workspace_id\":\"$space_id\",\"evaluation_set_id\":\"$eval_set_id\",\"page_number\":1,\"page_size\":10}")"
require_code_zero "$list_body" "list items"

/usr/bin/python3 - <<'PY' "$list_body" "$upload_key"
import json, sys
body = json.loads(sys.argv[1])
expected_uri = sys.argv[2]
items = body.get("items") or []
if not items:
    raise SystemExit("FAIL: list items returned empty")
turn = (items[0].get("turns") or [{}])[0]
multipart = None
for fd in turn.get("field_data_list") or []:
    content = fd.get("content") or {}
    if content.get("content_type") == "MultiPart":
        multipart = content.get("multi_part") or []
        break
if not multipart:
    raise SystemExit("FAIL: item has no MultiPart content")
text_parts = [p for p in multipart if p.get("content_type") == "Text" and p.get("text")]
image_parts = [p for p in multipart if p.get("content_type") == "Image" and (p.get("image") or {}).get("uri")]
if not text_parts:
    raise SystemExit("FAIL: missing text part in MultiPart item")
if not image_parts:
    raise SystemExit("FAIL: missing image part in MultiPart item")
if image_parts[0]["image"]["uri"] != expected_uri:
    raise SystemExit(f"FAIL: image uri mismatch: {image_parts[0]['image']['uri']!r} != {expected_uri!r}")
print("OK: multipart item round-trip verified")
print(f"  text: {text_parts[0]['text']!r}")
print(f"  image uri: {expected_uri}")
PY

echo "Multimodal dataset e2e check passed (evaluation_set_id=$eval_set_id)."
