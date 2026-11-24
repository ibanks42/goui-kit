#!/bin/bash
# Run `cargo check` on gk-capi in release mode, logging compiler diagnostics to
# gen/out/build.check.log in short message format for `gkgen.py quarantine rust`.
# Always exits 0 after printing summary lines so gen/round.sh can parse the log.
# Usage: gen/check.sh [JOBS]
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
