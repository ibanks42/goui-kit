#!/bin/bash
# alone.sh [FAMILY [MODE...]]: one family's protocol tests without the rest of core/tests/run.sh
# (the same build, check and run helpers; FAMILY default lists2: core/tests/run-FAMILY.sh).
# GK_LIB_ROOT, GK_BIN, ODIN as for run.sh. Prints "GATE proto-FAMILY ... PASS|FAIL n of m".
here=$(cd "$(dirname "$0")/.." && pwd); repo=$(cd "$here/../.." && pwd)
family=${1:-lists2}; shift || true
odin=${ODIN:-odin}
lib=${GK_LIB_ROOT:-$repo/lib}/diag
bin=${GK_BIN:-$repo/bin}
out=$repo/gates/out/proto
rm -rf "$out"; mkdir -p "$out" "$bin"
export LD_LIBRARY_PATH=$lib
unset WAYLAND_DISPLAY DISPLAY
build() { # output-name [source-dir] extra-flags...
  name=$1; shift
  dir=$here/proto
  if [ $# -gt 0 ] && [ -d "$1" ]; then dir=$1; shift; fi
  "$odin" build "$dir" -o:minimal -define:GK_GUARD=true "$@" -define:GK_SHARED=true -collection:gpuikit="$repo/odin" \
    "-extra-linker-flags:-L$lib -Wl,-rpath,\$ORIGIN/../lib/diag" -out:"$bin/$name" || exit 2
}
pass=0; total=0
check() {
  mode=$1; want=$2; shift 2
  log=$out/$mode.log
  if [ ! -e "$log" ]; then
    GK_PROTO=$mode timeout 60 "$bin/${GK_PROTO_BIN:-proto-odin}" > "$log" 2>&1
    echo $? > "$out/$mode.rc"
  fi
  rc=$(cat "$out/$mode.rc")
  total=$((total + 1))
  if [ "$rc" != "$want" ]; then
    echo "FAIL $mode: exit $rc, wanted $want"; tail -8 "$log"; return
  fi
  for pattern in "$@"; do
    if ! grep -Eq -- "$pattern" "$log"; then
      echo "FAIL $mode: exit $rc, but no line matches /$pattern/"; tail -8 "$log"; return
    fi
  done
  pass=$((pass + 1)); echo "PASS $mode: exit $rc; $(grep -Em1 -- "${1:-.}" "$log" | cut -c1-140)"
}
wanted() { [ $# -eq 0 ] && return 0; for m in "$@"; do [ "$m" = "$mode_name" ] && return 0; done; return 1; }
run() { mode_name=$1; if wanted "${only[@]}"; then check "$@"; fi; }
only=("$@")
. "$here/run-$family.sh"
echo "GATE proto-$family $([ $pass = $total ] && echo PASS || echo FAIL) $pass of $total protocol tests"
[ $pass = $total ]
