#!/bin/bash
# readme.sh: the example in README.md is the example that builds. The first ```odin block of
# README.md must be examples/odin/hello/main.odin byte for byte; that app is built against
# the ship library and run for a few frames in a private compositor.
#   gates/readme.sh
here=$(cd "$(dirname "$0")" && pwd); repo=$(dirname "$here")
out=$here/out/readme; rm -rf "$out"; mkdir -p "$out"
awk '/^```odin$/{f=1;next} /^```$/{if(f)exit} f' "$repo/README.md" > "$out/readme.odin"
if ! cmp -s "$out/readme.odin" "$repo/examples/odin/hello/main.odin"; then
  echo "GATE readme example FAIL the Odin block in README.md is not examples/odin/hello/main.odin"; exit 1
fi
lib=${GK_LIB_ROOT:-$repo/lib}/ship; bin=${GK_BIN:-$repo/bin}; mkdir -p "$bin"
if ! "${ODIN:-odin}" build "$repo/examples/odin/hello" -o:minimal -define:GK_SHARED=true -collection:gpuikit="$repo/odin" \
    "-extra-linker-flags:-L$lib" -out:"$bin/hello-odin" > "$out/build.log" 2>&1; then
  echo "GATE readme example FAIL it does not build"; head -5 "$out/build.log"; exit 1
fi
LD_LIBRARY_PATH=$lib GK_FRAMES=5,30 "$here/display.sh" timeout 60 "$bin/hello-odin" > "$out/run.log" 2>&1; rc=$?
if [ $rc -eq 0 ] && grep -q '^gk-meter' "$out/run.log"; then
  echo "GATE readme example PASS $(wc -l < "$out/readme.odin") lines, as in README.md; builds and draws 30 frames"
else
  echo "GATE readme example FAIL exit $rc when run"; tail -5 "$out/run.log"; exit 1
fi
