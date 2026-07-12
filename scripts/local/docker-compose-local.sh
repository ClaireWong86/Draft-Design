#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SOURCE_DIR="$ROOT_DIR/release/deployment/docker-compose"
WORK_DIR="${COZE_LOOP_LOCAL_COMPOSE_DIR:-/private/tmp/coze-loop-docker-compose}"
DOCKER_BIN="${DOCKER_BIN:-/Applications/Docker.app/Contents/Resources/bin/docker}"

if [[ ! -x "$DOCKER_BIN" ]]; then
  DOCKER_BIN="docker"
fi

COMPOSE_ARGS=(
  compose
  -f "$WORK_DIR/docker-compose.yml"
  --env-file "$WORK_DIR/.env"
  --profile "*"
)

usage() {
  cat <<'USAGE'
Usage:
  scripts/local/docker-compose-local.sh start
  scripts/local/docker-compose-local.sh stop
  scripts/local/docker-compose-local.sh status
  scripts/local/docker-compose-local.sh logs [service]
  scripts/local/docker-compose-local.sh refresh-config
  scripts/local/docker-compose-local.sh render-config
  scripts/local/docker-compose-local.sh restart-app
  scripts/local/docker-compose-local.sh doctor

This wrapper avoids Docker Desktop bind-mount hangs by copying compose files to
/private/tmp and mounting bootstrap/config files through named Docker volumes.

If .env.local contains OPENAI_API_KEY, ARK_API_KEY, DEEPSEEK_API_KEY, or
JOYBUILD_API_KEY, refresh-config/start writes a local model_config.yaml into the
temporary compose directory before syncing volumes. Optional .env.local values:
COZE_LOOP_OPENAI_BASE_URL, COZE_LOOP_OPENAI_MODEL, ARK_BASE_URL, ARK_MODEL,
DEEPSEEK_BASE_URL, DEEPSEEK_MODEL, JOYBUILD_BASE_URL, JOYBUILD_MODEL.
USAGE
}

run_docker() {
  "$DOCKER_BIN" "$@"
}

check_docker() {
  if ! run_docker info >/dev/null 2>&1; then
    echo "Docker Desktop is not ready. Start Docker Desktop first." >&2
    exit 1
  fi
}

patch_compose_file() {
  perl -0pi -e '
    s#\./bootstrap/app:/coze-loop/bootstrap#app_bootstrap:/coze-loop/bootstrap#g;
    s#\./conf:/coze-loop/conf#app_conf:/coze-loop/conf#g;
    s#\./bootstrap/redis:/coze-loop-redis/bootstrap#redis_bootstrap:/coze-loop-redis/bootstrap#g;
    s#\./bootstrap/mysql:/coze-loop-mysql/bootstrap#mysql_bootstrap:/coze-loop-mysql/bootstrap#g;
    s#\./bootstrap/mysql-init:/coze-loop-mysql-init/bootstrap#mysql_init_bootstrap:/coze-loop-mysql-init/bootstrap#g;
    s#\./bootstrap/clickhouse:/coze-loop-clickhouse/bootstrap#clickhouse_bootstrap:/coze-loop-clickhouse/bootstrap#g;
    s#\./bootstrap/clickhouse-init:/coze-loop-clickhouse-init/bootstrap#clickhouse_init_bootstrap:/coze-loop-clickhouse-init/bootstrap#g;
    s#\./bootstrap/minio:/coze-loop-minio/bootstrap#minio_bootstrap:/coze-loop-minio/bootstrap#g;
    s#\./bootstrap/minio-init:/coze-loop-minio-init/bootstrap#minio_init_bootstrap:/coze-loop-minio-init/bootstrap#g;
    s#\./bootstrap/rmq-namesrv:/coze-loop-rmq-namesrv/bootstrap#rmq_namesrv_bootstrap:/coze-loop-rmq-namesrv/bootstrap#g;
    s#\./bootstrap/rmq-broker:/coze-loop-rmq-broker/bootstrap#rmq_broker_bootstrap:/coze-loop-rmq-broker/bootstrap#g;
    s#\./bootstrap/rmq-init:/coze-loop-rmq-init/bootstrap#rmq_init_bootstrap:/coze-loop-rmq-init/bootstrap#g;
    s#\./bootstrap/nginx:/coze-loop-nginx/bootstrap#nginx_bootstrap:/coze-loop-nginx/bootstrap#g;
    s#\./bootstrap/python-faas:/coze-loop-python-faas/bootstrap:ro#python_faas_bootstrap:/coze-loop-python-faas/bootstrap:ro#g;
    s#\./bootstrap/js-faas:/coze-loop-js-faas/bootstrap#js_faas_bootstrap:/coze-loop-js-faas/bootstrap#g;
  ' "$WORK_DIR/docker-compose.yml"

  cat >>"$WORK_DIR/docker-compose.yml" <<'VOLUMES'

# Local Docker Desktop workaround volumes. These avoid host bind mounts, which
# can hang on some Docker Desktop versions when starting containers.
VOLUMES

  perl -0pi -e '
    s#(  js_faas_workspace:\n    name: coze-loop_js_faas_workspace\n)#$1  app_bootstrap:\n    name: coze-loop_app_bootstrap\n  app_conf:\n    name: coze-loop_app_conf\n  redis_bootstrap:\n    name: coze-loop_redis_bootstrap\n  mysql_bootstrap:\n    name: coze-loop_mysql_bootstrap\n  mysql_init_bootstrap:\n    name: coze-loop_mysql_init_bootstrap\n  clickhouse_bootstrap:\n    name: coze-loop_clickhouse_bootstrap\n  clickhouse_init_bootstrap:\n    name: coze-loop_clickhouse_init_bootstrap\n  minio_bootstrap:\n    name: coze-loop_minio_bootstrap\n  minio_init_bootstrap:\n    name: coze-loop_minio_init_bootstrap\n  rmq_namesrv_bootstrap:\n    name: coze-loop_rmq_namesrv_bootstrap\n  rmq_broker_bootstrap:\n    name: coze-loop_rmq_broker_bootstrap\n  rmq_init_bootstrap:\n    name: coze-loop_rmq_init_bootstrap\n  nginx_bootstrap:\n    name: coze-loop_nginx_bootstrap\n  python_faas_bootstrap:\n    name: coze-loop_python_faas_bootstrap\n  js_faas_bootstrap:\n    name: coze-loop_js_faas_bootstrap\n#s;
  ' "$WORK_DIR/docker-compose.yml"
}

prepare_work_dir() {
  rm -rf "$WORK_DIR"
  mkdir -p "$(dirname "$WORK_DIR")"
  cp -R "$SOURCE_DIR" "$WORK_DIR"
  patch_compose_file
}

env_value() {
  local name="$1"
  local env_file="$ROOT_DIR/.env.local"

  [[ -f "$env_file" ]] || return 1
  awk -F= -v key="$name" '$1 == key { sub(/^[^=]*=/, ""); print; found=1; exit } END { exit found ? 0 : 1 }' "$env_file"
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

ark_chat_base_url() {
  local value="${1%/}"

  case "$value" in
    */api/v3) printf '%s' "$value" ;;
    *) printf '%s/api/v3' "$value" ;;
  esac
}

joybuild_openai_base_url() {
  local value="${1%/}"

  case "$value" in
    */v1) printf '%s' "$value" ;;
    *) printf '%s/v1' "$value" ;;
  esac
}

apply_openai_model_config() {
  local api_key
  local base_url
  local model
  local joybuild_api_key
  local joybuild_base_url
  local joybuild_model
  local ark_api_key
  local ark_base_url
  local ark_model
  local deepseek_api_key
  local deepseek_base_url
  local deepseek_model

  api_key="$(env_or_file_value OPENAI_API_KEY "")"
  base_url="$(env_or_file_value COZE_LOOP_OPENAI_BASE_URL "https://api.openai.com/v1")"
  model="$(env_or_file_value COZE_LOOP_OPENAI_MODEL "gpt-5.6-luna")"
  ark_api_key="$(env_or_file_value ARK_API_KEY "")"
  ark_base_url="$(ark_chat_base_url "$(env_or_file_value ARK_BASE_URL "https://ark.cn-beijing.volces.com")")"
  ark_model="$(env_or_file_value ARK_MODEL "doubao-seed-2-0-pro-260215")"
  deepseek_api_key="$(env_or_file_value DEEPSEEK_API_KEY "")"
  deepseek_base_url="$(env_or_file_value DEEPSEEK_BASE_URL "https://api.deepseek.com")"
  deepseek_model="$(env_or_file_value DEEPSEEK_MODEL "deepseek-v4-pro")"
  joybuild_api_key="$(env_or_file_value JOYBUILD_API_KEY "")"
  joybuild_base_url="$(joybuild_openai_base_url "$(env_or_file_value JOYBUILD_BASE_URL "http://ai-api.jdcloud.com")")"
  joybuild_model="$(env_or_file_value JOYBUILD_MODEL "Gemini-3.1-Flash-Lite")"

  if [[ -z "$api_key" && -z "$ark_api_key" && -z "$deepseek_api_key" && -z "$joybuild_api_key" ]]; then
    return 0
  fi

  cat >"$WORK_DIR/conf/model_config.yaml" <<EOF
models:
EOF

  if [[ -n "$api_key" ]]; then
    cat >>"$WORK_DIR/conf/model_config.yaml" <<EOF
  - id: 1
    name: "$model"
    desc: "OpenAI local development model"
    ability:
      max_context_tokens: 1050000
      max_input_tokens: 1050000
      max_output_tokens: 128000
      function_call: true
      json_mode: true
      multi_modal: true
      ability_multi_modal:
        image: true
        ability_image:
          url_enabled: true
          binary_enabled: true
          max_image_size: 20
          max_image_count: 20
    frame: "eino"
    protocol: "openai"
    protocol_config:
      base_url: "$base_url"
      api_key: "$api_key"
      model: "$model"
      protocol_config_openai:
        by_azure: false
        api_version: ""
        response_format_type: ""
        response_format_json_schema: ""
    scenario_configs:
      default:
        scenario: "default"
        quota:
          qpm: 0
          tpm: 0
        unavailable: false
      prompt_debug:
        scenario: "prompt_debug"
        quota:
          qpm: 0
          tpm: 0
        unavailable: false
      eval_target:
        scenario: "eval_target"
        quota:
          qpm: 0
          tpm: 0
        unavailable: false
      evaluator:
        scenario: "evaluator"
        quota:
          qpm: 0
          tpm: 0
        unavailable: false
    param_config:
      param_schemas:
        - name: "temperature"
          label: "temperature"
          desc: "Increasing temperature makes model output more diverse and creative, while decreasing it makes model output more focused on instructions."
          type: "float"
          min: "0"
          max: "1.0"
          default_val: "0.7"
        - name: "max_tokens"
          label: "max_tokens"
          desc: "Controls the maximum number of tokens in model output."
          type: "int"
          min: "1"
          max: "8192"
          default_val: "2048"
        - name: "top_p"
          label: "top_p"
          desc: "Selects the minimum token set with cumulative probability reaching top_p during generation."
          type: "float"
          min: "0.001"
          max: "1.0"
          default_val: "0.7"
        - name: "frequency_penalty"
          label: "frequency_penalty"
          desc: "Penalizes repeated tokens."
          type: "float"
          min: "0"
          max: "2.0"
          default_val: "0"
        - name: "presence_penalty"
          label: "presence_penalty"
          desc: "Penalizes tokens that have appeared, increasing content diversity."
          type: "float"
          min: "0"
          max: "2.0"
          default_val: "0"
EOF
  fi

  if [[ -n "$ark_api_key" ]]; then
    cat >>"$WORK_DIR/conf/model_config.yaml" <<EOF
  - id: 2
    name: "$ark_model"
    desc: "Ark model imported from tire-ai-diagnosis ARK_* variables"
    ability:
      max_context_tokens: 128000
      max_input_tokens: 128000
      max_output_tokens: 8192
      function_call: true
      json_mode: true
      multi_modal: true
      ability_multi_modal:
        image: true
        ability_image:
          url_enabled: true
          binary_enabled: true
          max_image_size: 20
          max_image_count: 20
    frame: "eino"
    protocol: "ark"
    protocol_config:
      base_url: "$ark_base_url"
      api_key: "$ark_api_key"
      model: "$ark_model"
      timeout_ms: 180000
      protocol_config_ark:
        region: "cn-beijing"
        access_key: ""
        secret_key: ""
        retry_times: 2
        custom_headers: {}
    scenario_configs:
      default:
        scenario: "default"
        quota:
          qpm: 0
          tpm: 0
        unavailable: false
      prompt_debug:
        scenario: "prompt_debug"
        quota:
          qpm: 0
          tpm: 0
        unavailable: false
      eval_target:
        scenario: "eval_target"
        quota:
          qpm: 0
          tpm: 0
        unavailable: false
      evaluator:
        scenario: "evaluator"
        quota:
          qpm: 0
          tpm: 0
        unavailable: false
    param_config:
      param_schemas:
        - name: "temperature"
          label: "temperature"
          desc: "Increasing temperature makes model output more diverse and creative, while decreasing it makes model output more focused on instructions."
          type: "float"
          min: "0"
          max: "1.0"
          default_val: "0.7"
        - name: "max_tokens"
          label: "max_tokens"
          desc: "Controls the maximum number of tokens in model output."
          type: "int"
          min: "1"
          max: "8192"
          default_val: "2048"
        - name: "top_p"
          label: "top_p"
          desc: "Selects the minimum token set with cumulative probability reaching top_p during generation."
          type: "float"
          min: "0.001"
          max: "1.0"
          default_val: "0.7"
        - name: "frequency_penalty"
          label: "frequency_penalty"
          desc: "Penalizes repeated tokens."
          type: "float"
          min: "0"
          max: "2.0"
          default_val: "0"
        - name: "presence_penalty"
          label: "presence_penalty"
          desc: "Penalizes tokens that have appeared, increasing content diversity."
          type: "float"
          min: "0"
          max: "2.0"
          default_val: "0"
EOF
  fi

  if [[ -n "$deepseek_api_key" ]]; then
    cat >>"$WORK_DIR/conf/model_config.yaml" <<EOF
  - id: 3
    name: "$deepseek_model"
    desc: "DeepSeek model configured from DEEPSEEK_* variables"
    ability:
      max_context_tokens: 1000000
      max_input_tokens: 1000000
      max_output_tokens: 384000
      function_call: true
      json_mode: true
      multi_modal: false
    frame: "eino"
    protocol: "deepseek"
    protocol_config:
      base_url: "$deepseek_base_url"
      api_key: "$deepseek_api_key"
      model: "$deepseek_model"
      timeout_ms: 180000
      protocol_config_deepseek:
        response_format_type: ""
    scenario_configs:
      default:
        scenario: "default"
        quota:
          qpm: 0
          tpm: 0
        unavailable: false
      prompt_debug:
        scenario: "prompt_debug"
        quota:
          qpm: 0
          tpm: 0
        unavailable: false
      eval_target:
        scenario: "eval_target"
        quota:
          qpm: 0
          tpm: 0
        unavailable: false
      evaluator:
        scenario: "evaluator"
        quota:
          qpm: 0
          tpm: 0
        unavailable: false
    param_config:
      param_schemas:
        - name: "temperature"
          label: "temperature"
          desc: "Increasing temperature makes model output more diverse and creative, while decreasing it makes model output more focused on instructions."
          type: "float"
          min: "0"
          max: "1.0"
          default_val: "0.7"
        - name: "max_tokens"
          label: "max_tokens"
          desc: "Controls the maximum number of tokens in model output."
          type: "int"
          min: "1"
          max: "8192"
          default_val: "2048"
        - name: "top_k"
          label: "top_k"
          desc: "Samples from the top k tokens with the highest probability."
          type: "int"
          min: "1"
          max: "100"
          default_val: "50"
        - name: "top_p"
          label: "top_p"
          desc: "Selects the minimum token set with cumulative probability reaching top_p during generation."
          type: "float"
          min: "0.001"
          max: "1.0"
          default_val: "0.7"
        - name: "frequency_penalty"
          label: "frequency_penalty"
          desc: "Penalizes repeated tokens."
          type: "float"
          min: "0"
          max: "2.0"
          default_val: "0"
        - name: "presence_penalty"
          label: "presence_penalty"
          desc: "Penalizes tokens that have appeared, increasing content diversity."
          type: "float"
          min: "0"
          max: "2.0"
          default_val: "0"
EOF
  fi

  if [[ -n "$joybuild_api_key" ]]; then
    cat >>"$WORK_DIR/conf/model_config.yaml" <<EOF
  - id: 4
    name: "$joybuild_model"
    desc: "JoyBuild model configured from tire-ai-diagnosis style JOYBUILD_* variables"
    ability:
      max_context_tokens: 65536
      max_input_tokens: 65536
      max_output_tokens: 8192
      function_call: true
      json_mode: true
      multi_modal: true
      ability_multi_modal:
        image: true
        ability_image:
          url_enabled: true
          binary_enabled: true
          max_image_size: 20
          max_image_count: 20
    frame: "eino"
    protocol: "openai"
    protocol_config:
      base_url: "$joybuild_base_url"
      api_key: "$joybuild_api_key"
      model: "$joybuild_model"
      timeout_ms: 180000
      protocol_config_openai:
        by_azure: false
        api_version: ""
        response_format_type: ""
        response_format_json_schema: ""
    scenario_configs:
      default:
        scenario: "default"
        quota:
          qpm: 0
          tpm: 0
        unavailable: false
      prompt_debug:
        scenario: "prompt_debug"
        quota:
          qpm: 0
          tpm: 0
        unavailable: false
      eval_target:
        scenario: "eval_target"
        quota:
          qpm: 0
          tpm: 0
        unavailable: false
      evaluator:
        scenario: "evaluator"
        quota:
          qpm: 0
          tpm: 0
        unavailable: false
    param_config:
      param_schemas:
        - name: "temperature"
          label: "temperature"
          desc: "Increasing temperature makes model output more diverse and creative, while decreasing it makes model output more focused on instructions."
          type: "float"
          min: "0"
          max: "1.0"
          default_val: "0.7"
        - name: "max_tokens"
          label: "max_tokens"
          desc: "Controls the maximum number of tokens in model output."
          type: "int"
          min: "1"
          max: "8192"
          default_val: "2048"
        - name: "top_p"
          label: "top_p"
          desc: "Selects the minimum token set with cumulative probability reaching top_p during generation."
          type: "float"
          min: "0.001"
          max: "1.0"
          default_val: "0.7"
        - name: "frequency_penalty"
          label: "frequency_penalty"
          desc: "Penalizes repeated tokens."
          type: "float"
          min: "0"
          max: "2.0"
          default_val: "0"
        - name: "presence_penalty"
          label: "presence_penalty"
          desc: "Penalizes tokens that have appeared, increasing content diversity."
          type: "float"
          min: "0"
          max: "2.0"
          default_val: "0"
EOF
  fi
}

fill_volume() {
  local volume="$1"
  local source="$2"
  local container="coze-loop-fill-${volume//_/-}"

  run_docker volume create "$volume" >/dev/null
  run_docker rm -f "$container" >/dev/null 2>&1 || true
  run_docker create --name "$container" -v "$volume":/target redis:8.2.0 sh -c 'sleep 3600' >/dev/null
  run_docker start "$container" >/dev/null
  run_docker exec "$container" sh -c 'find /target -mindepth 1 -maxdepth 1 -exec rm -rf {} +'
  run_docker cp "$source/." "$container:/target/"
  run_docker rm -f "$container" >/dev/null
}

apply_model_health() {
  local config_file="$WORK_DIR/conf/model_config.yaml"
  if [[ -f "$config_file" ]]; then
    if /usr/bin/python3 "$ROOT_DIR/scripts/local/apply-model-health.py" "$config_file"; then
      :
    else
      echo "WARN: model health check failed; keeping generated model_config.yaml as-is" >&2
    fi
  fi
}

refresh_config() {
  check_docker
  prepare_work_dir
  apply_openai_model_config
  apply_model_health

  run_docker image inspect redis:8.2.0 >/dev/null 2>&1 || run_docker pull redis:8.2.0

  fill_volume coze-loop_app_bootstrap "$WORK_DIR/bootstrap/app"
  fill_volume coze-loop_app_conf "$WORK_DIR/conf"
  fill_volume coze-loop_redis_bootstrap "$WORK_DIR/bootstrap/redis"
  fill_volume coze-loop_mysql_bootstrap "$WORK_DIR/bootstrap/mysql"
  fill_volume coze-loop_mysql_init_bootstrap "$WORK_DIR/bootstrap/mysql-init"
  fill_volume coze-loop_clickhouse_bootstrap "$WORK_DIR/bootstrap/clickhouse"
  fill_volume coze-loop_clickhouse_init_bootstrap "$WORK_DIR/bootstrap/clickhouse-init"
  fill_volume coze-loop_minio_bootstrap "$WORK_DIR/bootstrap/minio"
  fill_volume coze-loop_minio_init_bootstrap "$WORK_DIR/bootstrap/minio-init"
  fill_volume coze-loop_rmq_namesrv_bootstrap "$WORK_DIR/bootstrap/rmq-namesrv"
  fill_volume coze-loop_rmq_broker_bootstrap "$WORK_DIR/bootstrap/rmq-broker"
  fill_volume coze-loop_rmq_init_bootstrap "$WORK_DIR/bootstrap/rmq-init"
  fill_volume coze-loop_nginx_bootstrap "$WORK_DIR/bootstrap/nginx"
  fill_volume coze-loop_python_faas_bootstrap "$WORK_DIR/bootstrap/python-faas"
  fill_volume coze-loop_js_faas_bootstrap "$WORK_DIR/bootstrap/js-faas"
}

render_config() {
  prepare_work_dir
  apply_openai_model_config
  apply_model_health
  echo "Rendered compose config: $WORK_DIR"
  echo "Rendered model config: $WORK_DIR/conf/model_config.yaml"
}

start() {
  refresh_config
  run_docker "${COMPOSE_ARGS[@]}" up -d
  run_docker "${COMPOSE_ARGS[@]}" ps
}

stop() {
  check_docker
  [[ -f "$WORK_DIR/docker-compose.yml" ]] || prepare_work_dir
  run_docker "${COMPOSE_ARGS[@]}" down
}

status() {
  check_docker
  [[ -f "$WORK_DIR/docker-compose.yml" ]] || prepare_work_dir
  run_docker "${COMPOSE_ARGS[@]}" ps
}

logs() {
  check_docker
  [[ -f "$WORK_DIR/docker-compose.yml" ]] || prepare_work_dir
  run_docker "${COMPOSE_ARGS[@]}" logs -f --tail=100 "$@"
}

restart_app() {
  check_docker
  [[ -f "$WORK_DIR/docker-compose.yml" ]] || prepare_work_dir
  run_docker "${COMPOSE_ARGS[@]}" restart app
}

doctor() {
  check_docker
  run_docker info --format 'Docker server: {{.ServerVersion}}
Storage driver: {{.Driver}}
Operating system: {{.OperatingSystem}}
Architecture: {{.Architecture}}'
  echo
  echo "Expected app URL: http://localhost:8082"
  if run_docker info --format '{{.Driver}}' | grep -q '^overlayfs$'; then
    echo "Warning: Docker is using overlayfs/containerd snapshotter."
    echo "If containers with volumes hang in Created state, disable UseContainerdSnapshotter in Docker Desktop settings."
  fi
}

case "${1:-}" in
  start) start ;;
  stop) stop ;;
  status) status ;;
  logs) shift; logs "$@" ;;
  refresh-config) refresh_config ;;
  render-config) render_config ;;
  restart-app) restart_app ;;
  doctor) doctor ;;
  -h|--help|help|"") usage ;;
  *) echo "Unknown command: $1" >&2; usage; exit 1 ;;
esac
