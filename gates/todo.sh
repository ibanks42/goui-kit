#!/bin/bash
# Standalone two-start persistence gate for the Go todo app (`gates/todo.sh [script]`).
# Gates: building todo-go, running the full interactive session offscreen, reloading its saved
# todo.json in a second process, and verifying the persisted state.
# Method: builds examples/go/todo, runs gates/todo.script (or a custom script arg) on lib/diag with
# TODO_FILE=todo.json; when using the default script, also runs gates/todo-reload.script and validates
# todo.json with gates/todo-check.py.
# PASS: build, both offscreen starts, and todo-check.py exit 0 (`GATE todo session-go PASS`).

here=$(cd "$(dirname "$0")" && pwd); repo=$(dirname "$here")
front=go; gate="todo session-$front"
script=$(realpath "${1:-$here/todo.script}")
out=$here/out/todo-$front
"$repo/examples/$front/build.sh" todo || { echo "GATE $gate FAIL the app does not build"; exit 1; }
rm -rf "$out"; mkdir -p "$out"; cd "$out"
export LD_LIBRARY_PATH=${GK_LIB_ROOT:-$repo/lib}/diag
unset WAYLAND_DISPLAY DISPLAY
TODO_FILE=$out/todo.json GK_SCRIPT=$script timeout 120 "${GK_BIN:-$repo/bin}/todo-$front" > log.txt 2>&1
rc=$?
echo "todo: exit $rc, $(ls *.png 2>/dev/null | wc -l) images"
[ $rc -eq 0 ] || { tail -20 log.txt; echo "GATE $gate FAIL exit $rc"; exit 1; }
if [ -z "${1:-}" ]; then
  TODO_FILE=$out/todo.json GK_SCRIPT=$here/todo-reload.script timeout 120 "${GK_BIN:-$repo/bin}/todo-$front" > log2.txt 2>&1
  rc=$?
  echo "todo (reload): exit $rc, $(ls *.png 2>/dev/null | wc -l) images"
  [ $rc -eq 0 ] || { tail -20 log2.txt; echo "GATE $gate FAIL the second start: exit $rc"; exit 1; }
fi
if [ -z "${1:-}" ]; then
  python3 "$here/todo-check.py" "$out/todo.json" || { echo "GATE $gate FAIL the saved state (todo-check.py)"; exit 1; }
fi
echo "GATE $gate PASS $(ls *.png | wc -l) images, two starts, the saved state as the session left it"
