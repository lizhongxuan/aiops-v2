#!/usr/bin/env sh
set -eu

: "${AIOPS_HTTP_ADDR:=:8080}"
: "${AIOPS_GRPC_ADDR:=:18090}"
: "${AIOPS_DATA_DIR:=/var/lib/aiops}"
: "${AIOPS_WEB_DIST_DIR:=/app/web/dist}"
: "${AIOPS_DEBUG_MODEL_INPUT_TRACE_DIR:=/var/lib/aiops/model-input-traces}"

export AIOPS_HTTP_ADDR
export AIOPS_GRPC_ADDR
export AIOPS_DATA_DIR
export AIOPS_WEB_DIST_DIR
export AIOPS_DEBUG_MODEL_INPUT_TRACE_DIR

mkdir -p "$AIOPS_DATA_DIR"

json_escape() {
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

write_llm_config_from_env() {
  target="${AIOPS_DATA_DIR%/}/llm-config.json"
  mode="${AIOPS_BOOTSTRAP_LLM_CONFIG:-if-missing}"

  if [ -s "$target" ] && [ "$mode" != "overwrite" ]; then
    echo "[aiops-v2] keep existing LLM config: $target"
    return 0
  fi

  provider="${AIOPS_LLM_PROVIDER:-openai}"
  model="${AIOPS_LLM_MODEL:-gpt-5.4}"
  compact_model="${AIOPS_LLM_COMPACT_MODEL:-gpt-5.4-mini}"
  api_key="${AIOPS_LLM_API_KEY:-}"
  base_url="${AIOPS_LLM_BASE_URL:-}"
  fallback_provider="${AIOPS_LLM_FALLBACK_PROVIDER:-}"
  fallback_model="${AIOPS_LLM_FALLBACK_MODEL:-}"
  fallback_api_key="${AIOPS_LLM_FALLBACK_API_KEY:-}"

  if [ -z "$api_key" ] && [ -z "$base_url" ] && [ -z "${AIOPS_LLM_MODEL:-}" ] && [ -z "${AIOPS_LLM_PROVIDER:-}" ]; then
    return 0
  fi

  tmp="${target}.tmp"
  cat > "$tmp" <<EOF
{
  "provider": "$(json_escape "$provider")",
  "model": "$(json_escape "$model")",
  "apiKey": "$(json_escape "$api_key")",
  "baseURL": "$(json_escape "$base_url")",
  "fallbackProvider": "$(json_escape "$fallback_provider")",
  "fallbackModel": "$(json_escape "$fallback_model")",
  "fallbackApiKey": "$(json_escape "$fallback_api_key")",
  "compactModel": "$(json_escape "$compact_model")"
}
EOF
  mv "$tmp" "$target"
  echo "[aiops-v2] bootstrapped LLM config: $target"
}

copy_llm_config_file() {
  source="${AIOPS_LLM_CONFIG_FILE:-}"
  target="${AIOPS_DATA_DIR%/}/llm-config.json"
  mode="${AIOPS_BOOTSTRAP_LLM_CONFIG:-if-missing}"

  if [ -z "$source" ] || [ ! -f "$source" ]; then
    return 1
  fi
  if [ -s "$target" ] && [ "$mode" != "overwrite" ]; then
    echo "[aiops-v2] keep existing LLM config: $target"
    return 0
  fi
  cp "$source" "$target"
  echo "[aiops-v2] copied LLM config from $source"
  return 0
}

copy_llm_config_file || write_llm_config_from_env

exec "$@"
