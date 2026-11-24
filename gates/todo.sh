#!/bin/bash
# todo.sh [SCRIPT]: runs the todo example (examples/odin/todo) offscreen under a gk-meter script
# (default gates/todo.script, then gates/todo-reload.script on what it saved) with a fresh state
# file, keeps the images in gates/out/todo/, and checks what the session must have left behind in
# the saved state (gates/todo-check.py). No Rust twin: the images are looked at by eye. Builds the
# app first. Prints one line "GATE todo session ...". Another SCRIPT: that script alone, no checks.
# GK_FRONT=go: the Go app (examples/go/todo, bin/todo-go); the line is then "GATE todo session-go".
here=$(cd "$(dirname "$0")" && pwd); repo=$(dirname "$here")
front=${GK_FRONT:-odin}; gate="todo session"; [ "$front" = odin ] || gate="todo session-$front"
script=$(realpath "${1:-$here/todo.script}")
out=$here/out/todo${GK_FRONT:+-$front}
"$repo/examples/$front/build.sh" todo || { echo "GATE $gate FAIL the app does not build"; exit 1; }
rm -rf "$out"; mkdir -p "$out"; cd "$out"
export LD_LIBRARY_PATH=${GK_LIB_ROOT:-$repo/lib}/diag
unset WAYLAND_DISPLAY DISPLAY
TODO_FILE=$out/todo.json GK_SCRIPT=$script timeout 120 "${GK_BIN:-$repo/bin}/todo-$front" > log.txt 2>&1
rc=$?
echo "todo: exit $rc, $(ls *.png 2>/dev/null | wc -l) images"
[ $rc -eq 0 ] || { tail -20 log.txt; echo "GATE $gate FAIL exit $rc"; exit 1; }
if [ -z "${1:-}" ]; then
  # a second start, which loads the state the first one saved
  TODO_FILE=$out/todo.json GK_SCRIPT=$here/todo-reload.script timeout 120 "${GK_BIN:-$repo/bin}/todo-$front" > log2.txt 2>&1
  rc=$?
  echo "todo (reload): exit $rc, $(ls *.png 2>/dev/null | wc -l) images"
  [ $rc -eq 0 ] || { tail -20 log2.txt; echo "GATE $gate FAIL the second start: exit $rc"; exit 1; }
fi
if [ -z "${1:-}" ]; then
  python3 "$here/todo-check.py" "$out/todo.json" || { echo "GATE $gate FAIL the saved state (todo-check.py)"; exit 1; }
fi
echo "GATE $gate PASS $(ls *.png | wc -l) images, two starts, the saved state as the session left it"
