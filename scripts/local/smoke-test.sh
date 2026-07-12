#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${COZE_LOOP_BASE_URL:-http://localhost:8082}"
OPENAPI_URL="${COZE_LOOP_OPENAPI_URL:-http://localhost:8888}"
EMAIL="${COZE_LOOP_SMOKE_EMAIL:-codex-local-smoke@example.com}"
PASSWORD="${COZE_LOOP_SMOKE_PASSWORD:-Codex123456}"

curl_json() {
  /usr/bin/curl -sS "$@"
}

json_field() {
  /usr/bin/python3 -c "import json,sys; print(json.load(sys.stdin).get('$1', ''))"
}

require_code_zero() {
  local body="$1"
  local label="$2"
  local code

  code="$(printf '%s' "$body" | json_field code)"
  if [[ "$code" != "0" ]]; then
    echo "FAIL: $label returned code=$code"
    printf '%s\n' "$body"
    exit 1
  fi
}

echo "Checking web app: $BASE_URL/auth/login"
/usr/bin/curl -fsSI "$BASE_URL/auth/login" >/dev/null
echo "OK: web app is reachable"

echo "Checking OpenAPI ping: $OPENAPI_URL/ping"
ping_body="$(curl_json "$OPENAPI_URL/ping")"
if [[ "$ping_body" != '{"message":"pong"}' ]]; then
  echo "FAIL: unexpected ping response"
  printf '%s\n' "$ping_body"
  exit 1
fi
echo "OK: OpenAPI ping responded"

echo "Registering smoke user if needed: $EMAIL"
register_body="$(curl_json -X POST "$BASE_URL/api/foundation/v1/users/register" \
  -H 'Content-Type: application/json' \
  --data "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}")"
register_code="$(printf '%s' "$register_body" | json_field code)"
if [[ "$register_code" != "0" && "$register_code" != "602002002" ]]; then
  echo "FAIL: register returned code=$register_code"
  printf '%s\n' "$register_body"
  exit 1
fi
echo "OK: smoke user is available"

echo "Logging in"
login_body="$(curl_json -X POST "$BASE_URL/api/foundation/v1/users/login_by_password" \
  -H 'Content-Type: application/json' \
  --data "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}")"
require_code_zero "$login_body" "login"
token="$(printf '%s' "$login_body" | /usr/bin/python3 -c 'import json,sys; print(json.load(sys.stdin).get("token", ""))')"
if [[ -z "$token" ]]; then
  echo "FAIL: login did not return a session token"
  exit 1
fi
echo "OK: login succeeded"

echo "Checking personal space"
spaces_body="$(curl_json -X POST "$BASE_URL/api/foundation/v1/spaces/list" \
  -H 'Content-Type: application/json' \
  -H "Cookie: session_key=$token" \
  --data '{"page_number":1,"page_size":100}')"
require_code_zero "$spaces_body" "spaces/list"
space_count="$(printf '%s' "$spaces_body" | /usr/bin/python3 -c 'import json,sys; print(len(json.load(sys.stdin).get("spaces", [])))')"
if [[ "$space_count" == "0" ]]; then
  echo "FAIL: spaces/list returned no spaces"
  printf '%s\n' "$spaces_body"
  exit 1
fi
echo "OK: found $space_count space(s)"

space_id="$(printf '%s' "$spaces_body" | /usr/bin/python3 -c 'import json,sys; print(json.load(sys.stdin)["spaces"][0]["id"])')"

echo "Checking model list"
models_body="$(curl_json -X POST "$BASE_URL/api/llm/v1/models/list" \
  -H 'Content-Type: application/json' \
  -H "Cookie: session_key=$token" \
  --data "{\"workspace_id\":\"$space_id\",\"page_token\":\"0\",\"page_size\":20,\"scenario\":\"scenario_prompt_debug\"}")"
require_code_zero "$models_body" "models/list"
model_names="$(printf '%s' "$models_body" | /usr/bin/python3 -c 'import json,sys; print(", ".join(m.get("name", "") for m in json.load(sys.stdin).get("models", [])))')"
if [[ -z "$model_names" ]]; then
  echo "FAIL: models/list returned no models"
  printf '%s\n' "$models_body"
  exit 1
fi
echo "OK: found model(s): $model_names"

echo "Smoke test passed."
