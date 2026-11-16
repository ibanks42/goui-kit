#!/bin/bash
# reader.sh [SCRIPT]: runs the reader example (examples/odin/reader) offscreen under a gk-meter script
# (default gates/reader.script) over a generated feed of 3,000 articles (gates/reader-feed.py), keeps
# the images in gates/out/reader/ and checks the log for what the session must have done. No Rust
# twin: the images are looked at by eye. Builds the app first. Prints one line "GATE reader session ...".
here=$(cd "$(dirname "$0")" && pwd); repo=$(dirname "$here")
script=$(realpath "${1:-$here/reader.script}")
out=$here/out/reader
"$repo/examples/odin/build.sh" reader || { echo "GATE reader session FAIL the app does not build"; exit 1; }
rm -rf "$out"; mkdir -p "$out"; cd "$out"
python3 "$here/reader-feed.py" "$out/feed.md" 3000
export LD_LIBRARY_PATH=${GK_LIB_ROOT:-$repo/lib}/diag
unset WAYLAND_DISPLAY DISPLAY
READER_FEED=$out/feed.md GK_SCRIPT=$script timeout 120 "${GK_BIN:-$repo/bin}/reader-odin" > log.txt 2>&1
rc=$?
echo "reader: exit $rc, $(ls *.png 2>/dev/null | wc -l) images"
[ $rc -eq 0 ] || { tail -20 log.txt; echo "GATE reader session FAIL exit $rc"; exit 1; }
echo "GATE reader session PASS $(ls *.png | wc -l) images"
