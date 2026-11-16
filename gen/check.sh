#!/bin/bash
# check.sh [JOBS]: "rustc is the judge". cargo check of the library with the
# generated Rust in core/src/gen, in the release profile (an item that exists only in a
# debug build of the kit must not pass), with the compiler's errors one per line in
# gen/out/build.check.log (which gen/round.sh hands to `gkgen.py quarantine rust`). Prints
# the wall time. Always exits 0: the log says whether it compiled.
set -uo pipefail
cd "$(dirname "$0")/.."
mkdir -p gen/out
start=$(date +%s)
cargo check --release --locked ${1:+-j "$1"} -p gk-capi --message-format short > gen/out/build.check.log 2>&1
echo "check: cargo exit $?" >> gen/out/build.check.log
echo "check: $(( $(date +%s) - start )) s"
grep -E "^check:|error: could not compile|Finished" gen/out/build.check.log
echo "check: $(grep -c ': error' gen/out/build.check.log) error lines"
exit 0
