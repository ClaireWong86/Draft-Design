#!/usr/bin/env bash
# Protocol-break + Goodcase judge regression (unit level).
# Expect: fenced/prose JSON and type breaks hard-zero to 0; dynamic batch sizing ok.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
export PATH="/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin:${PATH:-}"

cd "$ROOT/backend"
echo "Running Goodcase judge / protocol / dynamic-batch tests..."
go test -gcflags="all=-N -l" -count=1 \
  -run 'Test.*(Judge|Goodcase|Estimate|Guard|Validate|Merge|Refusal|JSON|Protocol)' \
  ./modules/evaluation/application/
echo "PASS: protocol hard checks + dynamic batch sizing"
