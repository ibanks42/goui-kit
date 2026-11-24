#!/bin/bash
# Go sample apps and tutorial gate (`gates/go-apps.sh [build|notes|todo|reader|expenses|tutorial]...`).
#
# What it gates:
#   Compilation, visual parity against Rust reference twins, state persistence, and self-determinism
#   for the four full sample applications (`notes`, `todo`, `reader`, `expenses`) and all 9 tutorial
#   steps (`docs/tutorial-go/01..09`). When `GK_STATIC=1`, builds static binaries linked with
#   `-Wl,--gc-sections` against `lib/diag` and logs their sizes in MB.
#
# How it measures and why the comparison is fair:
#   - Every app and its Rust twin run offscreen under `lib/diag` (display vars cleared, 120 s
#     timeout) driven by the exact same `.script` file so layout, input timing, and animation
#     advances are deterministic.
#   - `notes`, `todo` (across both initial and reload starts), `expenses`, and `tutorial-session`
#     (`tut-09-go` vs `ref-tutorial`) require every PNG to be byte-identical (`cmp -s`).
#   - `todo` also verifies `todo-go.json` with `gates/todo-check.py`.
#   - `reader` runs a 3000-article synthetic feed (`gates/reader-feed.py`), executes `ref-reader`
#     twice (`rust`, `rust2`) and `reader-go` once, and uses `gates/go-apps-cmp.py` to mask only
#     the wall-clock load-time and scrollbar boxes plus pixels where `rust` and `rust2` differ.
#   - Tutorial steps `01..09` each run twice (`go`, `go2`) to verify deterministic rendering.
#
# What PASS means (exit 0):
#   Every requested phase builds cleanly, exits 0, passes every image/state assertion, and emits
#   `GATE go-apps <phase> PASS` (or `GATE pixels notes-go PASS` for `notes`).

here=$(cd "$(dirname "$0")" && pwd); repo=$(dirname "$here")
root=${GK_LIB_ROOT:-$repo/lib}; bin=${GK_BIN:-$repo/bin}; mkdir -p "$bin"
if [ "${GK_STATIC:-}" = 1 ]; then
  # Static builds link lib/diag directly so offscreen scripts work, using --gc-sections to drop unused families.
  export GOBUILDFLAGS="-ldflags=-extldflags=-Wl,--gc-sections"
  golib=$root/diag; tag=" (static)"; out=$here/out/go-apps-static
else
  golib=$root/ship; tag=""; out=$here/out/go-apps
fi
. "$repo/go/env.sh"
mkdir -p "$out"
[ $# -gt 0 ] || set -- build notes todo reader expenses tutorial
apps="notes todo reader expenses"
rc=0

# Compare every PNG in $dir/$a byte-for-byte against $dir/$b and verify equal image counts.
same() {
  local dir=$1 a=$2 b=$3 name=$4 n=0 ok=0 diff=""
  for png in "$dir/$a"/*.png; do
    [ -e "$png" ] || break
    n=$((n + 1)); f=$(basename "$png")
    if cmp -s "$png" "$dir/$b/$f"; then ok=$((ok + 1)); else diff="$diff $f"; fi
  done
  local want; want=$(ls "$dir/$b"/*.png 2>/dev/null | wc -l)
  if [ $n -gt 0 ] && [ $ok = $n ] && [ "$want" = $n ]; then
    echo "GATE go-apps $name PASS $ok of $n images byte-identical, $a and $b"
  else
    echo "GATE go-apps $name FAIL $ok of $n images byte-identical ($b has $want), differ:$diff"; rc=1
  fi
}

# Run a Go app offscreen under lib/diag with GK_SCRIPT.
run() {
  local dir=$1 label=$2 script=$3 exe=$4; shift 4
  rm -rf "${dir:?}/$label"; mkdir -p "$dir/$label"
  ( cd "$dir/$label" && env LD_LIBRARY_PATH="$root/diag" GK_SCRIPT="$script" "$@" timeout 120 "$exe" > log.txt 2>&1 )
  local r=$?
  [ $r = 0 ] || { echo "go-apps: $label $(basename "$exe") exit $r"; tail -8 "$dir/$label/log.txt"; rc=1; }
  return $r
}

# Run a Rust reference app offscreen under lib/diag with --script.
runref() {
  local dir=$1 label=$2 script=$3 exe=$root/diag/$4; shift 4
  [ -x "$exe" ] || { echo "go-apps: $exe is missing (make lib builds the Rust twins)"; rc=1; return 1; }
  rm -rf "${dir:?}/$label"; mkdir -p "$dir/$label"
  ( cd "$dir/$label" && env LD_LIBRARY_PATH="$root/diag" "$@" timeout 120 "$exe" --script "$script" > log.txt 2>&1 )
  local r=$?
  [ $r = 0 ] || { echo "go-apps: $label $(basename "$exe") exit $r"; tail -8 "$dir/$label/log.txt"; rc=1; }
  return $r
}

unset WAYLAND_DISPLAY DISPLAY
for phase in "$@"; do
  case "$phase" in
  build)
    t0=$(date +%s.%N)
    if ! GK_LIB=$golib "$repo/examples/go/build.sh" $apps > "$out/build-go.log" 2>&1; then
      echo "GATE go-apps build FAIL the Go apps"; cat "$out/build-go.log"; rc=1; continue
    fi
    for d in "$repo"/docs/tutorial-go/*/; do
      n=$(basename "$d" | cut -c1-2)
      ( cd "$repo/docs/tutorial-go" && CGO_LDFLAGS="-L$golib" \
        go build ${GOBUILDFLAGS:-} -o "$bin/tut-$n-go" "./$(basename "$d")" ) >> "$out/build-go.log" 2>&1 \
        || { echo "GATE go-apps build FAIL docs/tutorial-go/$(basename "$d")"; cat "$out/build-go.log"; rc=1; continue 2; }
    done
    [ -z "$tag" ] || echo "go-apps: static sizes: $(for a in $apps; do printf '%s %s MB  ' "$a" "$(( $(stat -c %s "$bin/$a-go") / 1000000 ))"; done)"
    echo "GATE go-apps build$tag PASS 4 apps and 9 tutorial steps in Go in $(printf %.1f "$(echo "$(date +%s.%N) - $t0" | bc)") s" ;;
  notes)
    GK_PIXEL_CANDS=go "$here/pixels.sh" notes > "$out/pixels-notes.txt" 2>&1 || rc=1
    grep '^GATE' "$out/pixels-notes.txt"; grep -E 'DIFFERS|exit [1-9]' "$out/pixels-notes.txt" | head -20 ;;
  todo)
    d=$out/todo
    rm -f "$d-rust.json" "$d-go.json"
    runref "$d" rust "$here/todo.script" ref-todo TODO_FILE="$d-rust.json"
    runref "$d" rust-reload "$here/todo-reload.script" ref-todo TODO_FILE="$d-rust.json"
    cp "$d/rust-reload"/*.png "$d/rust/" 2>/dev/null
    run "$d" go "$here/todo.script" "$bin/todo-go" TODO_FILE="$d-go.json"
    run "$d" go-reload "$here/todo-reload.script" "$bin/todo-go" TODO_FILE="$d-go.json"
    cp "$d/go-reload"/*.png "$d/go/" 2>/dev/null
    same "$d" go rust todo
    python3 "$here/todo-check.py" "$d-go.json" > "$d/check-go.txt" 2>&1 \
      && echo "GATE go-apps todo-state PASS the Go app's saved state is what the session leaves" \
      || { echo "GATE go-apps todo-state FAIL"; cat "$d/check-go.txt"; rc=1; } ;;
  reader)
    d=$out/reader; mkdir -p "$d"
    python3 "$here/reader-feed.py" "$d/feed.md" 3000
    for l in rust rust2; do runref "$d" "$l" "$here/reader.script" ref-reader READER_FEED="$d/feed.md"; done
    run "$d" go "$here/reader.script" "$bin/reader-go" READER_FEED="$d/feed.md"
    # Mask the wall-clock load-time box and scrollbar region; also allow pixels where rust and rust2 differ.
    python3 "$here/go-apps-cmp.py" --labels rust,rust2,go "$d" reader "1500,130,1640,175;560,180,580,1320" > "$d/cmp.txt" 2>&1 || rc=1
    grep -v '^GATE' "$d/cmp.txt" | head -20; grep '^GATE' "$d/cmp.txt" ;;
  expenses)
    d=$out/expenses
    runref "$d" rust "$here/expenses.script" ref-expenses
    run "$d" go "$here/expenses.script" "$bin/expenses-go"
    same "$d" go rust expenses ;;
  tutorial)
    d=$out/tutorial
    for t in "$bin"/tut-0?-go; do
      n=$(basename "$t" | cut -c5-6)
      s=$d/step.script; mkdir -p "$d"
      printf 'size 520 560\nframe\nsleep 500\nframe\nadvance 500\nshot step.png\n' > "$s"
      for l in go go2; do run "$d/$n" "$l" "$s" "$bin/tut-$n-go"; done
      same "$d/$n" go go2 "tutorial-$n"
    done
    runref "$d/session" rust "$here/docs-tutorial.script" ref-tutorial
    run "$d/session" go "$here/docs-tutorial.script" "$bin/tut-09-go"
    same "$d/session" go rust tutorial-session ;;
  *) echo "go-apps.sh: unknown phase $phase" >&2; rc=2 ;;
  esac
done
exit $rc
