#!/bin/bash
# round.sh [ROUNDS]: the check-and-quarantine loop (DESIGN.md 13, stage 5). Each round:
# `gkgen.py gen` has written core/src/gen; gen/check.sh runs cargo check on it; then
# `gkgen.py quarantine rust` charges every compiler error to the API item that caused it and
# regenerates without that item. Stops when a round compiles or nothing could be charged.
# At most ROUNDS (default 4). Each check is a cargo check of the library: a few minutes the
# first time, and several GB of memory.
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
  [ $rc -eq 3 ] || exit 1   # nothing could be charged to an item: a generator bug, not an item
done
exit 1
