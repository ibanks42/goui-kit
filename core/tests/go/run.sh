#!/bin/bash
# Master runner for Go protocol-family tests under core/tests/go/.
# Sources each run-*.sh family (or $GK_FAMILIES), optionally filters individual
# modes via positional arguments, and prints a GATE summary line.
here=$(cd "$(dirname "$0")" && pwd); repo=$(cd "$here/../../.." && pwd)
. "$repo/go/env.sh"
lib=${GK_LIB_ROOT:-$repo/lib}/diag
bin=${GK_BIN:-$repo/bin}
out=$repo/gates/out/go-proto-families
rm -rf "$out"; mkdir -p "$out" "$bin"
export LD_LIBRARY_PATH=$lib
# Force headless execution even when invoked from a graphical desktop session.
unset WAYLAND_DISPLAY DISPLAY

# Builds a Go test package into $bin/$name, logging failures under $out.
build() {
  name=$1; dir=$2
  rm -f "$bin/$name"
  if ! (cd "$here" && go build -o "$bin/$name" "./$dir") \
      > "$out/build-$name.log" 2>&1; then
    echo "FAIL build $name: go build ./$dir"; head -20 "$out/build-$name.log"
  fi
}

pass=0; total=0
# Runs $GK_PROTO_BIN with GK_PROTO=$mode, verifies exit code $want, and checks
# that every extended regex argument matches the captured log.
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

families=${GK_FAMILIES:-$(cd "$here" && ls run-*.sh | sed 's/^run-//; s/\.sh$//')}
for fam in $families; do
  [ -f "$here/run-$fam.sh" ] && . "$here/run-$fam.sh"
done

echo "GATE proto-families go $([ $pass = $total ] && echo PASS || echo FAIL) $pass of $total protocol tests"
[ $pass = $total ]
