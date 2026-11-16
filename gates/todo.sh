#!/bin/bash
# todo.sh [SCRIPT]: runs the todo example (examples/odin/todo) offscreen under a gk-meter script
# (default gates/todo.script, then gates/todo-reload.script on what it saved) with a fresh state
# file, keeps the images in gates/out/todo/, and checks what the session must have left behind in
# the saved state (gates/todo-check.py). No Rust twin: the images are looked at by eye. Builds the
# app first. Prints one line "GATE todo session ...". Another SCRIPT: that script alone, no checks.
here=$(cd "$(dirname "$0")" && pwd); repo=$(dirname "$here")
script=$(realpath "${1:-$here/todo.script}")
out=$here/out/todo
"$repo/examples/odin/build.sh" todo || { echo "GATE todo session FAIL the app does not build"; exit 1; }
rm -rf "$out"; mkdir -p "$out"; cd "$out"
export LD_LIBRARY_PATH=${GK_LIB_ROOT:-$repo/lib}/diag
unset WAYLAND_DISPLAY DISPLAY
TODO_FILE=$out/todo.json GK_SCRIPT=$script timeout 120 "${GK_BIN:-$repo/bin}/todo-odin" > log.txt 2>&1
rc=$?
echo "todo: exit $rc, $(ls *.png 2>/dev/null | wc -l) images"
[ $rc -eq 0 ] || { tail -20 log.txt; echo "GATE todo session FAIL exit $rc"; exit 1; }
if [ -z "${1:-}" ]; then
  # a second start, which loads the state the first one saved
  TODO_FILE=$out/todo.json GK_SCRIPT=$here/todo-reload.script timeout 120 "${GK_BIN:-$repo/bin}/todo-odin" > log2.txt 2>&1
  rc=$?
  echo "todo (reload): exit $rc, $(ls *.png 2>/dev/null | wc -l) images"
  [ $rc -eq 0 ] || { tail -20 log2.txt; echo "GATE todo session FAIL the second start: exit $rc"; exit 1; }
fi
if [ -z "${1:-}" ]; then
  python3 "$here/todo-check.py" "$out/todo.json" || { echo "GATE todo session FAIL the saved state (todo-check.py)"; exit 1; }
fi
echo "GATE todo session PASS $(ls *.png | wc -l) images, two starts, the saved state as the session left it"
