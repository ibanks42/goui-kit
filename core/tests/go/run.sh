#!/bin/bash
# run.sh [MODE...]: the runtime's protocol tests of the families (docs/PROTOCOLS.md), in Go.
# Each family is one Go test app,
# core/tests/go/FAMILY (module gpuikit.dev/coretests), built against the diag library and run
# offscreen (no display, no compositor) in one mode per check; a check is the mode's exit code and
# the lines it printed. The core protocol tests in Go
# (examples/go/proto) are gates/go-proto.sh; `make go-proto-all`
# runs both.
#
# Prints one line per check and "GATE proto-families go PASS|FAIL n of m" at the end; exit 0 only
# if every check that ran passed.
#
#   core/tests/go/run.sh                   every family
#   core/tests/go/run.sh actions timers    those modes only
#   GK_FAMILIES="rows input" core/tests/go/run.sh   those families only
# GK_LIB_ROOT=the directory holding diag/libgpuikit.so (default lib); GK_BIN=where the test
# binaries go (default bin). Logs: gates/out/go-proto-families/MODE.log.
here=$(cd "$(dirname "$0")" && pwd); repo=$(cd "$here/../../.." && pwd)
. "$repo/go/env.sh"
lib=${GK_LIB_ROOT:-$repo/lib}/diag
bin=${GK_BIN:-$repo/bin}
out=$repo/gates/out/go-proto-families
rm -rf "$out"; mkdir -p "$out" "$bin"
export LD_LIBRARY_PATH=$lib
unset WAYLAND_DISPLAY DISPLAY
tags=gkshared

# build NAME DIR: go build core/tests/go/DIR into bin/NAME. A family that does not build fails
# each of its checks (they find no binary) instead of stopping the suite.
build() {
  name=$1; dir=$2
  rm -f "$bin/$name"
  if ! (cd "$here" && CGO_LDFLAGS="-L$lib -Wl,-rpath,\$ORIGIN/../lib/diag" go build -tags "$tags" -o "$bin/$name" "./$dir") \
      > "$out/build-$name.log" 2>&1; then
    echo "FAIL build $name: go build ./$dir"; head -20 "$out/build-$name.log"
  fi
}

pass=0; total=0
# check MODE EXIT PATTERN...: every pattern (grep -E) must appear in the mode's output.
check() {
  mode=$1; want=$2; shift 2
  log=$out/$mode.log
  if [ ! -e "$log" ]; then
    GK_PROTO=$mode timeout 60 "$bin/$GK_PROTO_BIN" > "$log" 2>&1
    echo $? > "$out/$mode.rc"
  fi
  rc=$(cat "$out/$mode.rc")
  total=$((total + 1))
  if [ "$rc" != "$want" ]; then
    echo "FAIL $mode: exit $rc, wanted $want"; tail -5 "$log"; return
  fi
  for pattern in "$@"; do
    if ! grep -Eq -- "$pattern" "$log"; then
      echo "FAIL $mode: exit $rc, but no line matches /$pattern/"; tail -5 "$log"; return
    fi
  done
  pass=$((pass + 1)); echo "PASS $mode: exit $rc; $(grep -Em1 -- "${1:-.}" "$log" | cut -c1-140)"
}
wanted() { [ ${#only[@]} -eq 0 ] && return 0; for m in "${only[@]}"; do [ "$m" = "$mode_name" ] && return 0; done; return 1; }
run() { mode_name=$1; if wanted; then check "$@"; fi; }
only=("$@")

# Each family: core/tests/go/run-FAMILY.sh, sourced here; it builds its app with `build NAME DIR`
# and checks it with `GK_PROTO_BIN=NAME run MODE EXIT PATTERN...`.
families=${GK_FAMILIES:-$(cd "$here" && ls run-*.sh | sed 's/^run-//; s/\.sh$//')}
for fam in $families; do
  [ -f "$here/run-$fam.sh" ] && . "$here/run-$fam.sh"
done

echo "GATE proto-families go $([ $pass = $total ] && echo PASS || echo FAIL) $pass of $total protocol tests"
[ $pass = $total ]
