#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
exec /usr/bin/python3 "$ROOT_DIR/scripts/local/seed_tire_prompt.py" "$@"
