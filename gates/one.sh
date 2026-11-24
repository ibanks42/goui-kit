#!/bin/bash
# Single-run launcher for frame-metered binaries (`gates/one.sh <rust|go> <ship|diag> <scene> [args...]`).
#
# Purpose:
#   Provides a uniform invocation wrapper for `fps.sh` and `counter.sh` so Rust references
#   (`$GK_LIB_ROOT/<variant>/ref-<scene>`) and Go binaries (`$GK_BIN/<scene>-go`) run against the
#   same shared library (`LD_LIBRARY_PATH=$lib`), the same warmup/measured frame counts
#   (`GK_FRAMES`, default `60,300`), and the same timeout (`GK_TIMEOUT`, default 90 s).
#   Writes per-frame CSV telemetry to `GK_STATS` (default `gates/out/frames/<name>.csv`).
#
# Exit code: 2 for unknown binary or missing executable; otherwise the child's exit status.

bin=$1; variant=$2; scene=$3; shift 3
repo=$(cd "$(dirname "$0")/.." && pwd)
lib=${GK_LIB_ROOT:-$repo/lib}/$variant
case "$bin" in
  rust) exe=$lib/ref-$scene ;;
  go) exe=${GK_BIN:-$repo/bin}/$scene-go ;;
  *) echo "one.sh: unknown binary $bin" >&2; exit 2 ;;
esac
[ -x "$exe" ] || { echo "one.sh: $exe is missing (make lib / examples/go/build.sh)" >&2; exit 2; }
name=$scene$(printf -- '-%s' "$@")-$bin-$variant${GK_TAG:+-$GK_TAG}
export LD_LIBRARY_PATH=$lib
export GK_FRAMES=${GK_FRAMES:-60,300} GK_LABEL=${GK_LABEL:-$bin}
if [ -z "${GK_STATS:-}" ]; then mkdir -p "$repo/gates/out/frames"; export GK_STATS=$repo/gates/out/frames/$name.csv; fi
timeout "${GK_TIMEOUT:-90}" "$exe" "$@"
rc=$?
[ $rc -eq 0 ] || echo "one.sh: $name exited $rc" >&2
exit $rc
