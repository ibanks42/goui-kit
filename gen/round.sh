#!/bin/bash
# Iteratively run gen/check.sh and quarantine failing Rust items until gk-capi compiles
# or MAX_ROUNDS (default 4) is exhausted. Stops immediately if quarantine finds no new
# item to exclude (exit code != 3), which indicates an error outside generated items.
# Usage: gen/round.sh [MAX_ROUNDS]
set -uo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
cd "$repo"
rounds=${1:-4}
for n in $(seq 1 "$rounds"); do
  gen/check.sh || exit $?
  if grep -q "^check: cargo exit 0" gen/out/build.check.log; then echo "round $n: compiled"; exit 0; fi
  python3 gen/gkgen.py quarantine rust gen/out/build.check.log
  rc=$?
  echo "round $n: quarantine exit $rc"
  [ $rc -eq 3 ] || exit 1
done
exit 1
