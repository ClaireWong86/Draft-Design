#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SOURCE_DIR="$ROOT_DIR/release/deployment/docker-compose"
WORK_DIR="${COZE_LOOP_LOCAL_COMPOSE_DIR:-/private/tmp/coze-loop-docker-compose}"
DOCKER_BIN="${DOCKER_BIN:-/Applications/Docker.app/Contents/Resources/bin/docker}"

if [[ ! -x "$DOCKER_BIN" ]]; then
  DOCKER_BIN="docker"
fi

# Lightweight image used to seed bootstrap/config named volumes. Kept small and
# already present locally so volume seeding works offline (e.g. under Podman).
FILL_IMAGE="${COZE_LOOP_FILL_IMAGE:-alpine:latest}"

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
    (restarts app+nginx; syncs frontend from coze-loop:local-fix when present)
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
    echo "Container runtime is not ready. Start Podman Desktop or Docker Desktop first." >&2
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

patch_rmq_heap() {
  # RocketMQ's runbroker.sh/runserver.sh default to multi-GB JVM heaps (broker
  # -Xmx8g, namesrv -Xmx4g), which OOM-kills the broker on small local VMs.
  # Inject JAVA_OPT_EXT (appended after the defaults, so the last -Xmx wins) to
  # cap the heap. Local-only: applied to the temp compose, never the source.
  perl -0pi -e '
    s#(    container_name: "coze-loop-rmq-broker"\n)#$1    environment:\n      JAVA_OPT_EXT: "-server -Xms512m -Xmx'"${COZE_LOOP_RMQ_BROKER_XMX:-1g}"' -Xmn256m"\n#;
    s#(    container_name: "coze-loop-rmq-namesrv"\n)#$1    environment:\n      JAVA_OPT_EXT: "-server -Xms256m -Xmx'"${COZE_LOOP_RMQ_NAMESRV_XMX:-512m}"' -Xmn128m"\n#;
  ' "$WORK_DIR/docker-compose.yml"
}

prepare_work_dir() {
  rm -rf "$WORK_DIR"
  mkdir -p "$(dirname "$WORK_DIR")"
  cp -R "$SOURCE_DIR" "$WORK_DIR"
  patch_compose_file
  patch_rmq_heap
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

joybuild_native_base_url() {
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
  local joybuild_models_raw
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
  joybuild_base_url="$(joybuild_native_base_url "$(env_or_file_value JOYBUILD_BASE_URL "http://ai-api.jdcloud.com")")"
  joybuild_model="$(env_or_file_value JOYBUILD_MODEL "Gemini-3.1-Flash-Lite")"
  joybuild_models_raw="$(env_or_file_value JOYBUILD_MODELS "")"
  if [[ -z "$joybuild_models_raw" ]]; then
    joybuild_models_raw="$joybuild_model"
  fi

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
    # Native joybuild protocol (see backend .../llmimpl/eino/joybuild.go), matching
    # tire-ai-diagnosis (VLM_PROVIDER=joybuild -> /v1/responses). Requires the
    # source-built app image (coze-loop:local-fix); the official image lacks this
    # adapter. base_url already ends with /v1, so the adapter posts to /v1/responses.
    # JOYBUILD_MODELS (comma-separated) emits multiple entries; JOYBUILD_MODEL is fallback.
    local joybuild_id=4
    local _jb_m
    IFS=',' read -ra _joybuild_model_list <<< "$joybuild_models_raw"
    for _jb_m in "${_joybuild_model_list[@]}"; do
      _jb_m="${_jb_m#"${_jb_m%%[![:space:]]*}"}"
      _jb_m="${_jb_m%"${_jb_m##*[![:space:]]}"}"
      [[ -z "$_jb_m" ]] && continue
    cat >>"$WORK_DIR/conf/model_config.yaml" <<EOF
  - id: $joybuild_id
    name: "$_jb_m"
    desc: "JoyBuild native (/v1/responses) direct to $joybuild_base_url"
    ability:
      max_context_tokens: 65536
      max_input_tokens: 65536
      max_output_tokens: 8192
      function_call: false
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
    protocol: "joybuild"
    protocol_config:
      base_url: "$joybuild_base_url"
      api_key: "$joybuild_api_key"
      model: "$_jb_m"
      timeout_ms: 180000
      protocol_config_joybuild: {}
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
      joybuild_id=$((joybuild_id + 1))
    done
  fi
}

fill_volume() {
  local volume="$1"
  local source="$2"

  run_docker volume create "$volume" >/dev/null 2>&1 || true
  # Copy source into the named volume via a throwaway container. Using a bind
  # mount + cp -a (instead of `docker cp` into a long-lived container) keeps this
  # fast and reliable on both Docker Desktop and Podman. Also restore +x on
  # entrypoint/shell scripts, which some runtimes drop during the copy.
  run_docker run --rm \
    -v "$volume":/target \
    -v "$source":/source:ro \
    "$FILL_IMAGE" \
    sh -c 'find /target -mindepth 1 -maxdepth 1 -exec rm -rf {} + 2>/dev/null; cp -a /source/. /target/ && find /target -type f -name "*.sh" -exec chmod +x {} + 2>/dev/null; true'
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

work_env_value() {
  local name="$1"
  local envf="$WORK_DIR/.env"

  [[ -f "$envf" ]] || return 1
  awk -F= -v key="$name" '$1 == key { sub(/^[^=]*=/, ""); print; found=1; exit } END { exit found ? 0 : 1 }' "$envf"
}

# Point the app service at a locally built backend image so source hotfixes (e.g.
# the SignUpload EscapedPath fix) take effect. Defaults to coze-loop:local-fix
# when that image exists; each COZE_LOOP_APP_IMAGE_* can be overridden via .env.local.
apply_app_image_override() {
  local envf="$WORK_DIR/.env"
  [[ -f "$envf" ]] || return 0

  local registry repository name tag
  registry="$(work_env_value COZE_LOOP_APP_IMAGE_REGISTRY || echo docker.io)"
  repository="$(work_env_value COZE_LOOP_APP_IMAGE_REPOSITORY || echo cozedev)"
  name="$(work_env_value COZE_LOOP_APP_IMAGE_NAME || echo coze-loop)"
  tag="$(work_env_value COZE_LOOP_APP_IMAGE_TAG || echo 1.5.1)"

  # Auto-select the local backend-fix image when present (and user hasn't pinned one).
  if [[ -z "$(env_value COZE_LOOP_APP_IMAGE_TAG || true)" ]] &&
     run_docker image inspect coze-loop:local-fix >/dev/null 2>&1; then
    registry="docker.io"
    repository="library"
    name="coze-loop"
    tag="local-fix"
  fi

  # Explicit .env.local overrides win.
  registry="$(env_or_file_value COZE_LOOP_APP_IMAGE_REGISTRY "$registry")"
  repository="$(env_or_file_value COZE_LOOP_APP_IMAGE_REPOSITORY "$repository")"
  name="$(env_or_file_value COZE_LOOP_APP_IMAGE_NAME "$name")"
  tag="$(env_or_file_value COZE_LOOP_APP_IMAGE_TAG "$tag")"

  perl -0pi -e 's/^COZE_LOOP_APP_IMAGE_(REGISTRY|REPOSITORY|NAME|TAG)=.*\n//mg' "$envf"
  # Ensure the file ends with a newline so appended vars are not glued onto the last line.
  [[ -n "$(tail -c1 "$envf")" ]] && printf '\n' >>"$envf"
  {
    echo "COZE_LOOP_APP_IMAGE_REGISTRY=$registry"
    echo "COZE_LOOP_APP_IMAGE_REPOSITORY=$repository"
    echo "COZE_LOOP_APP_IMAGE_NAME=$name"
    echo "COZE_LOOP_APP_IMAGE_TAG=$tag"
  } >>"$envf"

  echo "App image: $registry/$repository/$name:$tag"
}

refresh_config() {
  check_docker
  prepare_work_dir
  apply_app_image_override
  apply_openai_model_config
  apply_model_health

  run_docker image inspect "$FILL_IMAGE" >/dev/null 2>&1 || run_docker pull "$FILL_IMAGE"

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
  apply_app_image_override
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
  sync_frontend_resources
  run_docker "${COMPOSE_ARGS[@]}" restart app nginx
}

# Copy /coze-loop/resources from the local-fix image into the nginx data volume.
# App/nginx both mount this volume; restarting alone does not refresh stale UI.
sync_frontend_resources() {
  local image="${COZE_LOOP_LOCAL_FIX_IMAGE:-coze-loop:local-fix}"
  local volume
  volume="$(work_env_value COZE_LOOP_NGINX_DATA_VOLUME_NAME || echo coze-loop-nginx-data)"

  if ! run_docker image inspect "$image" >/dev/null 2>&1; then
    return 0
  fi

  echo "Syncing frontend resources from $image into volume $volume"
  run_docker compose -f "$WORK_DIR/docker-compose.yml" --env-file "$WORK_DIR/.env" stop app nginx >/dev/null 2>&1 || true
  # Use shared SELinux label (:z) so app + nginx can both read the volume on Podman.
  # Private :Z from a one-off sync container causes nginx 403 (Permission denied).
  run_docker run --rm \
    -v "${volume}:/target:z" \
    --entrypoint sh \
    "$image" \
    -c 'rm -rf /target/* && cp -a /coze-loop/resources/. /target/ && chmod -R a+rX /target'
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
