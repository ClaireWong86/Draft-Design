#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ENV_FILE="$ROOT_DIR/.env.local"
CF_BIN="${CLOUDFLARED_BIN:-}"
CF_CONFIG_DIR="${CLOUDFLARED_CONFIG_DIR:-$HOME/.cloudflared}"
TUNNEL_NAME="${CF_TUNNEL_NAME:-prompt-loop}"
TUNNEL_HOSTNAME="${CF_TUNNEL_HOSTNAME:-}"
LOCAL_URL="${CF_TUNNEL_LOCAL_URL:-http://localhost:8082}"

usage() {
  cat <<USAGE
Usage:
  scripts/local/cloudflare-tunnel.sh install
  scripts/local/cloudflare-tunnel.sh login
  scripts/local/cloudflare-tunnel.sh create
  scripts/local/cloudflare-tunnel.sh route-dns
  scripts/local/cloudflare-tunnel.sh write-config
  scripts/local/cloudflare-tunnel.sh run
  scripts/local/cloudflare-tunnel.sh status
  scripts/local/cloudflare-tunnel.sh check
  scripts/local/cloudflare-tunnel.sh setup

Environment (.env.local or shell):
  CF_TUNNEL_NAME=prompt-loop
  CF_TUNNEL_HOSTNAME=promptloop.example.com
  CF_TUNNEL_LOCAL_URL=http://localhost:8082

Prerequisites:
  1. Cloudflare free account
  2. Domain DNS managed by Cloudflare
  3. Local Prompt Loop running on port 8082

Typical first-time setup:
  scripts/local/cloudflare-tunnel.sh install
  scripts/local/cloudflare-tunnel.sh login
  scripts/local/cloudflare-tunnel.sh create
  CF_TUNNEL_HOSTNAME=promptloop.example.com scripts/local/cloudflare-tunnel.sh route-dns
  scripts/local/cloudflare-tunnel.sh write-config
  scripts/local/cloudflare-tunnel.sh run
  scripts/local/cloudflare-tunnel.sh check
  scripts/local/cloudflare-tunnel.sh setup
USAGE
}

check_prerequisites() {
  local cf=""
  if cf="$(resolve_cloudflared 2>/dev/null)"; then
    :
  else
    cf=""
  fi
  echo "Local app URL: $LOCAL_URL"
  if /usr/bin/curl -fsSI "$LOCAL_URL/auth/login" >/dev/null 2>&1; then
    echo "OK: Prompt Loop reachable at $LOCAL_URL"
  else
    echo "WARN: Prompt Loop not reachable at $LOCAL_URL (run docker-compose-local.sh start)"
  fi
  if [[ -n "$cf" ]]; then
    echo "OK: cloudflared: $cf"
    "$cf" version 2>/dev/null | head -1 || true
  else
    echo "WARN: cloudflared not installed (run: $0 install)"
  fi
  if [[ -f "$CF_CONFIG_DIR/config.yml" ]]; then
    echo "OK: config at $CF_CONFIG_DIR/config.yml"
  else
    echo "WARN: missing $CF_CONFIG_DIR/config.yml (run: $0 write-config)"
  fi
  if [[ -n "$TUNNEL_HOSTNAME" ]] || env_value CF_TUNNEL_HOSTNAME >/dev/null 2>&1; then
    host="${TUNNEL_HOSTNAME:-$(env_value CF_TUNNEL_HOSTNAME)}"
    echo "Hostname: https://$host"
  else
    echo "WARN: CF_TUNNEL_HOSTNAME not set in .env.local"
  fi
}

print_setup() {
  cat <<SETUP
Cloudflare Named Tunnel — first-time setup
==========================================

1. Add to .env.local:
   CF_TUNNEL_NAME=prompt-loop
   CF_TUNNEL_HOSTNAME=promptloop.yourdomain.com
   CF_TUNNEL_LOCAL_URL=http://localhost:8082

2. Ensure domain DNS is on Cloudflare (free plan OK).

3. Run:
   scripts/local/cloudflare-tunnel.sh install
   scripts/local/cloudflare-tunnel.sh login
   scripts/local/cloudflare-tunnel.sh create
   CF_TUNNEL_HOSTNAME=promptloop.yourdomain.com scripts/local/cloudflare-tunnel.sh route-dns
   scripts/local/cloudflare-tunnel.sh write-config
   scripts/local/cloudflare-tunnel.sh run

4. Open https://\${CF_TUNNEL_HOSTNAME}/auth/login

SETUP
}

env_value() {
  local name="$1"
  [[ -f "$ENV_FILE" ]] || return 1
  awk -F= -v key="$name" '$1 == key { sub(/^[^=]*=/, ""); print; found=1; exit } END { exit found ? 0 : 1 }' "$ENV_FILE"
}

resolve_cloudflared() {
  if [[ -n "$CF_BIN" && -x "$CF_BIN" ]]; then
    printf '%s' "$CF_BIN"
    return 0
  fi
  if command -v cloudflared >/dev/null 2>&1; then
    command -v cloudflared
    return 0
  fi
  local arch dest
  arch="$(uname -m)"
  case "$arch" in
    arm64) dest="$ROOT_DIR/.tools/cloudflared-darwin-arm64" ;;
    x86_64) dest="$ROOT_DIR/.tools/cloudflared-darwin-amd64" ;;
    *) echo "Unsupported architecture: $arch" >&2; exit 1 ;;
  esac
  if [[ -x "$dest" ]]; then
    printf '%s' "$dest"
    return 0
  fi
  echo "cloudflared is not installed. Run: scripts/local/cloudflare-tunnel.sh install" >&2
  exit 1
}

install_cloudflared() {
  mkdir -p "$ROOT_DIR/.tools"
  local arch url dest
  arch="$(uname -m)"
  case "$arch" in
    arm64)
      url="https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-darwin-arm64.tgz"
      dest="$ROOT_DIR/.tools/cloudflared-darwin-arm64"
      ;;
    x86_64)
      url="https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-darwin-amd64.tgz"
      dest="$ROOT_DIR/.tools/cloudflared-darwin-amd64"
      ;;
    *)
      echo "Unsupported architecture: $arch" >&2
      exit 1
      ;;
  esac
  tmp="$(mktemp -d)"
  /usr/bin/curl -fsSL "$url" -o "$tmp/cloudflared.tgz"
  tar -xzf "$tmp/cloudflared.tgz" -C "$tmp"
  install -m 0755 "$tmp/cloudflared" "$dest"
  rm -rf "$tmp"
  echo "Installed cloudflared: $dest"
}

tunnel_uuid() {
  local cf="$1"
  "$cf" tunnel list 2>/dev/null | awk -v name="$TUNNEL_NAME" '$0 ~ name { print $1; exit }'
}

cmd="${1:-}"
case "$cmd" in
  install)
    install_cloudflared
    ;;
  login)
    cf="$(resolve_cloudflared)"
    "$cf" tunnel login
    ;;
  create)
    cf="$(resolve_cloudflared)"
    if tunnel_uuid "$cf" >/dev/null; then
      echo "Tunnel already exists: $TUNNEL_NAME"
      "$cf" tunnel list | grep "$TUNNEL_NAME" || true
      exit 0
    fi
    "$cf" tunnel create "$TUNNEL_NAME"
    ;;
  route-dns)
    cf="$(resolve_cloudflared)"
    if [[ -z "$TUNNEL_HOSTNAME" ]]; then
      TUNNEL_HOSTNAME="$(env_value CF_TUNNEL_HOSTNAME || true)"
    fi
    if [[ -z "$TUNNEL_HOSTNAME" ]]; then
      echo "Set CF_TUNNEL_HOSTNAME=promptloop.example.com" >&2
      exit 1
    fi
    "$cf" tunnel route dns "$TUNNEL_NAME" "$TUNNEL_HOSTNAME"
    echo "DNS route created: https://$TUNNEL_HOSTNAME"
    ;;
  write-config)
    cf="$(resolve_cloudflared)"
    uuid="$(tunnel_uuid "$cf")"
    if [[ -z "$uuid" ]]; then
      echo "Tunnel not found: $TUNNEL_NAME" >&2
      exit 1
    fi
    if [[ -z "$TUNNEL_HOSTNAME" ]]; then
      TUNNEL_HOSTNAME="$(env_value CF_TUNNEL_HOSTNAME || true)"
    fi
    mkdir -p "$CF_CONFIG_DIR"
    cred="$CF_CONFIG_DIR/$uuid.json"
    if [[ ! -f "$cred" ]]; then
      echo "Credentials file not found: $cred" >&2
      exit 1
    fi
    cat >"$CF_CONFIG_DIR/config.yml" <<EOF
tunnel: $uuid
credentials-file: $cred

ingress:
  - hostname: ${TUNNEL_HOSTNAME:-promptloop.example.com}
    service: $LOCAL_URL
  - service: http_status:404
EOF
    echo "Wrote $CF_CONFIG_DIR/config.yml"
    ;;
  run)
    cf="$(resolve_cloudflared)"
    if [[ ! -f "$CF_CONFIG_DIR/config.yml" ]]; then
      echo "Missing $CF_CONFIG_DIR/config.yml. Run write-config first." >&2
      exit 1
    fi
    echo "Starting named tunnel. Public URL depends on CF_TUNNEL_HOSTNAME."
    exec "$cf" tunnel --config "$CF_CONFIG_DIR/config.yml" run
    ;;
  status)
    cf="$(resolve_cloudflared)"
    "$cf" tunnel list || true
    if [[ -f "$CF_CONFIG_DIR/config.yml" ]]; then
      echo "---"
      sed -n '1,20p' "$CF_CONFIG_DIR/config.yml"
    fi
    ;;
  check)
    if [[ -z "$TUNNEL_HOSTNAME" ]]; then
      TUNNEL_HOSTNAME="$(env_value CF_TUNNEL_HOSTNAME || true)"
    fi
    check_prerequisites
    ;;
  setup)
    print_setup
    ;;
  *)
    usage
    exit 1
    ;;
esac
