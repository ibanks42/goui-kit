#!/bin/bash
# Standalone offscreen session gate for the Go reader app (`gates/reader.sh [script]`).
# Gates: building reader-go and running a full scripted session over a 3000-article feed.
# Method: builds examples/go/reader, generates feed.md via gates/reader-feed.py, and executes
# reader-go offscreen on lib/diag with GK_SCRIPT (default gates/reader.script, 120 s timeout).
# PASS: build succeeds and reader-go exits 0 (`GATE reader session-go PASS`).

here=$(cd "$(dirname "$0")" && pwd); repo=$(dirname "$here")
front=go; gate="reader session-$front"
script=$(realpath "${1:-$here/reader.script}")
out=$here/out/reader-$front
"$repo/examples/$front/build.sh" reader || { echo "GATE $gate FAIL the app does not build"; exit 1; }
rm -rf "$out"; mkdir -p "$out"; cd "$out"
python3 "$here/reader-feed.py" "$out/feed.md" 3000
export LD_LIBRARY_PATH=${GK_LIB_ROOT:-$repo/lib}/diag
unset WAYLAND_DISPLAY DISPLAY
READER_FEED=$out/feed.md GK_SCRIPT=$script timeout 120 "${GK_BIN:-$repo/bin}/reader-$front" > log.txt 2>&1
rc=$?
echo "reader: exit $rc, $(ls *.png 2>/dev/null | wc -l) images"
[ $rc -eq 0 ] || { tail -20 log.txt; echo "GATE $gate FAIL exit $rc"; exit 1; }
echo "GATE $gate PASS $(ls *.png | wc -l) images"
