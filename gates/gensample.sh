#!/bin/bash
# gensample.sh: the sample by generation, against the real runtime. core/tests/gensample is
# written by `make gen`: one view procedure that calls every bound constructor and builder
# of the generated package once (26,000 calls). Built and run offscreen with the diag
# library, it shows that every generated procedure exists, that no entry of a method table
# is nil (the Odin compiler has turned a cast of a procedure in a global initialiser into
# nil without a word), and that the runtime accepts every op and every handler registration.
# The elements are not attached to the view's root, so the library does not decode them:
# what the library makes of each is the gallery's business.
#   gates/gensample.sh
here=$(cd "$(dirname "$0")" && pwd); repo=$(dirname "$here")
odin=${ODIN:-odin}
lib=${GK_LIB_ROOT:-$repo/lib}/diag
bin=${GK_BIN:-$repo/bin}
out=$here/out/gensample; rm -rf "$out"; mkdir -p "$out" "$bin"
export LD_LIBRARY_PATH=$lib
unset WAYLAND_DISPLAY DISPLAY
start=$(date +%s.%N)
if ! "$odin" build "$repo/core/tests/gensample" -o:minimal -define:GK_SHARED=true -collection:gpuikit="$repo/odin" \
    "-extra-linker-flags:-L$lib" -out:"$bin/gensample-odin" > "$out/build.log" 2>&1; then
  echo "GATE gensample odin FAIL the sample does not compile: $(grep -c Error "$out/build.log") errors"; head -5 "$out/build.log"; exit 1
fi
built=$(echo "$(date +%s.%N) - $start" | bc | cut -c1-4)
timeout 120 "$bin/gensample-odin" > "$out/run.log" 2>&1; rc=$?
line=$(grep -m1 '^gensample: [0-9]* calls' "$out/run.log")
if [ $rc -eq 0 ] && grep -q '^gensample: done runs=[1-9]' "$out/run.log"; then
  echo "GATE gensample odin PASS ${line#gensample: }; built in $built s"
else
  echo "GATE gensample odin FAIL exit $rc"; tail -5 "$out/run.log"; exit 1
fi
