#!/bin/bash
# one.sh BIN VARIANT SCENE [ARGS...]: run one binary of a scene and print its gk-meter line.
#   BIN      rust | go                  (the Rust reference; the Go app)
#   VARIANT  ship | diag                (which library, and which build of the reference)
#   SCENE    panels | counter | ...     (bin/SCENE-go against lib/VARIANT/ref-SCENE)
# Needs a display (gates/display.sh). GK_FRAMES defaults to 60,300. The per-frame CSV goes to
# $GK_STATS (default gates/out/frames/SCENE-ARGS-BIN-VARIANT[-TAG].csv). Killed after
# $GK_TIMEOUT seconds (default 90); exit 124 then means the run stalled.
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
