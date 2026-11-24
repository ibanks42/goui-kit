#!/bin/bash
# go-apps.sh [PHASE...]: the Go apps against their Odin twins (docs/GO.md section 8). Each pair
# runs offscreen (the diag library) under the same gk-meter script and every image is compared
# byte for byte. Phases:
#   build     go build examples/go/{notes,todo,reader,expenses} and docs/tutorial-go/NN-* (bin/APP-go,
#             bin/tut-NN-go); the Odin twins (bin/APP-odin, bin/tut-NN-odin)
#   notes     gates/pixels.sh notes: Rust (ref-notes), Odin and Go under gates/notes.script
#   todo      gates/todo.script then gates/todo-reload.script, each app with a fresh state file;
#             gates/todo-check.py on what the Go app saved
#   reader    gates/reader.script over the same generated feed of 3,000 articles (gates/go-apps-cmp.py:
#             the load toast's time and what differs between two Odin runs are not counted)
#   expenses  gates/expenses.script
#   tutorial  every step a few frames (as gates/docs.sh), the last under gates/docs-tutorial.script
# Default: all of them. Prints lines starting with GATE. Images in gates/out/go-apps/.
# GK_STATIC=1: the Go and Odin programs are built the way an app is, against the static library
# (lib/diag's, for the offscreen runs), so each links only the component families it calls into;
# every scripted session then shows whether one is missing.
here=$(cd "$(dirname "$0")" && pwd); repo=$(dirname "$here")
root=${GK_LIB_ROOT:-$repo/lib}; bin=${GK_BIN:-$repo/bin}; mkdir -p "$bin"
if [ "${GK_STATIC:-}" = 1 ]; then
  export GOFLAGS="-mod=mod" GOBUILDFLAGS="-ldflags=-extldflags=-Wl,--gc-sections" GK_ODIN_SHARED=false
  golib=$root/diag; odinlib=$root/diag; odinshared=false; tag=" (static)"; out=$here/out/go-apps-static
else
  golib=$root/ship; odinlib=$root/ship; odinshared=true; tag=""; out=$here/out/go-apps
fi
. "$repo/go/env.sh"
mkdir -p "$out"
odin=${ODIN:-odin}
[ $# -gt 0 ] || set -- build notes todo reader expenses tutorial
apps="notes todo reader expenses"
rc=0

# same DIR A B: compare every image of DIR/A with DIR/B; prints a GATE line named $name.
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

# run DIR LABEL SCRIPT EXE [ENV...]: one offscreen run in DIR/LABEL.
run() {
  local dir=$1 label=$2 script=$3 exe=$4; shift 4
  rm -rf "${dir:?}/$label"; mkdir -p "$dir/$label"
  ( cd "$dir/$label" && env LD_LIBRARY_PATH="$root/diag" GK_SCRIPT="$script" "$@" timeout 120 "$exe" > log.txt 2>&1 )
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
      ( cd "$repo/docs/tutorial-go" && CGO_LDFLAGS="-L$golib -Wl,-rpath,$root/ship" \
        go build ${GOBUILDFLAGS:-} -o "$bin/tut-$n-go" "./$(basename "$d")" ) >> "$out/build-go.log" 2>&1 \
        || { echo "GATE go-apps build FAIL docs/tutorial-go/$(basename "$d")"; cat "$out/build-go.log"; rc=1; continue 2; }
    done
    echo "go-apps: Go built in $(printf %.1f "$(echo "$(date +%s.%N) - $t0" | bc)") s"
    GK_LIB=$odinlib "$repo/examples/odin/build.sh" $apps > "$out/build-odin.log" 2>&1 \
      || { echo "GATE go-apps build FAIL the Odin apps"; tail -20 "$out/build-odin.log"; rc=1; continue; }
    for d in "$repo"/docs/tutorial/*/; do
      n=$(basename "$d" | cut -c1-2)
      "$odin" build "$d" -o:minimal -define:GK_SHARED=$odinshared -collection:gpuikit="$repo/odin" "-extra-linker-flags:-L$odinlib" \
        -out:"$bin/tut-$n-odin" >> "$out/build-odin.log" 2>&1 \
        || { echo "GATE go-apps build FAIL $d"; tail -20 "$out/build-odin.log"; rc=1; continue 2; }
    done
    [ -z "$tag" ] || echo "go-apps: static sizes: $(for a in $apps; do printf '%s %s / %s MB  ' "$a" "$(( $(stat -c %s "$bin/$a-go") / 1000000 ))" "$(( $(stat -c %s "$bin/$a-odin") / 1000000 ))"; done)(Go / Odin)"
    echo "GATE go-apps build$tag PASS 4 apps and 9 tutorial steps in Go and in Odin" ;;
  notes)
    GK_PIXEL_CANDS="odin go" "$here/pixels.sh" notes > "$out/pixels-notes.txt" 2>&1 || rc=1
    grep '^GATE' "$out/pixels-notes.txt"; grep -E 'DIFFERS|exit [1-9]' "$out/pixels-notes.txt" | head -20 ;;
  todo)
    d=$out/todo
    for l in odin go; do
      rm -f "$d-$l.json"
      run "$d" "$l" "$here/todo.script" "$bin/todo-$l" TODO_FILE="$d-$l.json"
      run "$d" "$l-reload" "$here/todo-reload.script" "$bin/todo-$l" TODO_FILE="$d-$l.json"
      cp "$d/$l-reload"/*.png "$d/$l/" 2>/dev/null
    done
    same "$d" odin go todo
    python3 "$here/todo-check.py" "$d-go.json" > "$d/check-go.txt" 2>&1 \
      && echo "GATE go-apps todo-state PASS the Go app's saved state is what the session leaves" \
      || { echo "GATE go-apps todo-state FAIL"; cat "$d/check-go.txt"; rc=1; } ;;
  reader)
    d=$out/reader; mkdir -p "$d"
    python3 "$here/reader-feed.py" "$d/feed.md" 3000
    # The Odin app runs twice: what differs between its own two runs is the session's noise. Two
    # boxes (image pixels) hold wall-clock time: the load toast says how long the load took
    # ("in 7 ms"; the box runs on over " ms", which moves when the number's width does: "1" is
    # narrower than "2"), and the list's scrollbar fades on a real timer (gates/go-apps-cmp.py).
    for l in odin odin2 go; do run "$d" "$l" "$here/reader.script" "$bin/reader-${l%2}" READER_FEED="$d/feed.md"; done
    python3 "$here/go-apps-cmp.py" "$d" reader "1500,130,1640,175;560,180,580,1320" > "$d/cmp.txt" 2>&1 || rc=1
    grep -v '^GATE' "$d/cmp.txt" | head -20; grep '^GATE' "$d/cmp.txt" ;;
  expenses)
    d=$out/expenses
    for l in odin go; do run "$d" "$l" "$here/expenses.script" "$bin/expenses-$l"; done
    same "$d" odin go expenses ;;
  tutorial)
    d=$out/tutorial
    for t in "$bin"/tut-0?-go; do
      n=$(basename "$t" | cut -c5-6)
      s=$d/step.script; mkdir -p "$d"
      printf 'size 520 560\nframe\nsleep 500\nframe\nadvance 500\nshot step.png\n' > "$s"
      for l in odin go; do run "$d/$n" "$l" "$s" "$bin/tut-$n-$l"; done
      same "$d/$n" odin go "tutorial-$n"
    done
    for l in odin go; do run "$d/session" "$l" "$here/docs-tutorial.script" "$bin/tut-09-$l"; done
    same "$d/session" odin go tutorial-session ;;
  *) echo "go-apps.sh: unknown phase $phase" >&2; rc=2 ;;
  esac
done
exit $rc
