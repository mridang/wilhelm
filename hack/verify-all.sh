#!/usr/bin/env bash
# Build and lint every wilhelm module (root + each CRD subpackage).
set -uo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"

fail=0
modules=$(find . -name go.mod -not -path './.go/*' 2>/dev/null \
  | xargs -n1 dirname | sort -u)

for d in $modules; do
  printf '\n==== %s ====\n' "$d"
  # Capture exit status through the pipe explicitly — plain `|` discards it.
  if ! (set -o pipefail; cd "$d" && go build ./... 2>&1 | tail -5); then
    fail=1
    continue
  fi
  if ! (set -o pipefail; cd "$d" && golangci-lint run ./... 2>&1 | grep -v '^level=' | tail -5); then
    fail=1
  fi
done

printf '\n==== summary ====\n'
if [[ $fail -eq 0 ]]; then
  echo "all modules build + lint clean"
else
  echo "FAILED"
fi
exit $fail
