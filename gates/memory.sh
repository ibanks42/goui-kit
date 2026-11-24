#!/bin/bash
# memory.sh [--quick]: the memory gate. Long scripted runs of the apps of a front end (GK_FRONT:
# odin, the default, or go: bin/gallery-go, bin/panels-go, bin/counter-go) and their Rust
# references with the resident set sampled every 20 ms (gates/memory.py); an app that leaks
# per frame, per event or per re-described view grows where the Rust reference stays flat.
#   gallery   offscreen, the page script of gates/gallery.py walked CYCLES times: every page
#             is built, clicked and typed into again and again
#   panels    a real window at HZ: 60 panels, 2 re-described before every frame
#   counter   a real window at HZ: a click on Add every 10 frames (an EVENT turn, a new row)
# Defaults: about 100,000 gallery frames (as many whole walks of the pages as that takes; about
# 7 s, offscreen a frame takes 0.07 ms), FRAMES=6000, HZ=1000; --quick: 30,000 and 2000.
# GALLERY_FRAMES and CYCLES override the first.
# Prints "memory ..." rows and one "GATE memory NAME" line per run. Exit 0 = all pass.
#   gates/memory.sh
# --selftest: only the gallery run, with the front end's gallery told to leak 512 bytes on every
# view build (GALLERY_LEAK, which the Odin and the Go gallery both read); passes when the verdict
# FAILS, that is when the gate sees the leak.
#   GK_FRONT=go gates/memory.sh [--quick|--selftest]   the Go apps (rows memory-go, memory-selftest-go)
here=$(cd "$(dirname "$0")" && pwd); repo=$(dirname "$here")
target=${GALLERY_FRAMES:-100000}; frames=${FRAMES:-6000}; hz=${HZ:-1000}
[ "${1:-}" = --quick ] && { target=${GALLERY_FRAMES:-30000}; frames=${FRAMES:-2000}; }
selftest=0; [ "${1:-}" = --selftest ] && { selftest=1; export GALLERY_LEAK=512; }
root=${GK_LIB_ROOT:-$repo/lib}; bin=${GK_BIN:-$repo/bin}
front=${GK_FRONT:-odin}; sfx=""; speed=-speed
[ "$front" = odin ] || { sfx=-$front; speed=""; }
out=$here/out/memory$sfx; rm -rf "$out"; mkdir -p "$out/gallery" "$out/panels" "$out/counter"
pages=$repo/ref/gallery/pages.txt
cycles=${CYCLES:-$(( target / $(python3 "$here/gallery.py" frames "$pages" --cycles 1) + 1 ))}
fail=0

# gallery, offscreen (the diag library)
ref=${GK_REF:-$root/diag}/ref-gallery; [ -x "$ref" ] || ref=$root/diag/ref-gallery
# The Go gallery is built here when missing or older than its sources (as gates/gallery.sh does).
if [ "$front" = go ] && { [ ! -x "$bin/gallery-go" ] ||
   [ -n "$(find "$repo/examples/go/gallery" "$repo/go/gk" -newer "$bin/gallery-go" -name '*.go' -print -quit)" ]; }; then
  GK_LIB=$root/ship OUT=$bin "$repo/examples/go/build.sh" gallery > "$out/build-gallery.log" 2>&1 ||
    { tail -5 "$out/build-gallery.log"; echo "GATE memory gallery$sfx FAIL the Go gallery does not build"; exit 1; }
fi
if [ -x "$ref" ] && [ -x "$bin/gallery-$front" ]; then
  python3 "$here/gallery.py" script "$pages" --cycles "$cycles" > "$out/gallery/cycles.script"
  n=$(python3 "$here/gallery.py" frames "$pages" --cycles "$cycles")
  ( unset WAYLAND_DISPLAY DISPLAY GALLERY_START; export LD_LIBRARY_PATH=$root/diag; cd "$out/gallery"
    python3 "$here/memory.py" sample rust.csv -- "$ref" --script cycles.script || echo "memory gallery rust: exit $?"
    GK_SCRIPT=cycles.script python3 "$here/memory.py" sample $front.csv -- "$bin/gallery-$front" || echo "memory gallery $front: exit $?" )
  if [ $selftest = 1 ]; then
    if GK_FRONT=$front python3 "$here/memory.py" verdict gallery-leak-512 "$n" "$out/gallery" > "$out/selftest.txt"; then
      echo "GATE memory selftest$sfx FAIL a leak of 512 bytes a view build was not seen: $(grep '^GATE' "$out/selftest.txt" | cut -d' ' -f5-)"; exit 1
    fi
    # The verdict must fail for the leak, not because a run did not happen.
    if ! grep -q 'app grows more than the Rust reference' "$out/selftest.txt" || grep -q 'samples (the run failed' "$out/selftest.txt"; then
      echo "GATE memory selftest$sfx FAIL the leaking run did not complete: $(grep '^GATE' "$out/selftest.txt" | cut -d' ' -f5-)"; exit 1
    fi
    echo "GATE memory selftest$sfx PASS a leak of 512 bytes a view build is refused: $(grep '^GATE' "$out/selftest.txt" | cut -d' ' -f5-)"; exit 0
  fi
  GK_FRONT=$front python3 "$here/memory.py" verdict gallery$sfx "$n" "$out/gallery" || fail=1
else
  echo "GATE memory gallery$sfx FAIL $ref or $bin/gallery-$front is missing"; fail=1
fi

# panels and counter, real windows in one private compositor session (the ship library)
GK_TIMEOUT=$(( frames * 3 / (hz / 2) + 30 ))
"$here/display.sh" --hz "$hz" bash -c '
  here=$1; out=$2; root=$3; bin=$4; frames=$5; t=$6; front=$7; speed=$8
  export LD_LIBRARY_PATH=$root/ship GK_FRAMES=60,$frames
  # A run that is killed by the timeout received no frame callback from gamescope (the stall
  # gates/fps.sh describes, seen in the Rust reference and in the Odin app alike): it is
  # repeated, up to three times, and counted in stalls.txt.
  run() { # scene who csv command...
    local scene=$1 who=$2 csv=$3 rc; shift 3
    for try in 1 2 3; do
      timeout "$t" python3 "$here/memory.py" sample "$csv" -- "$@"; rc=$?
      [ $rc -eq 124 ] || break
      echo "$scene $who try=$try" >> "$out/stalls.txt"; echo "memory $scene $who: stalled (try $try)"
    done
    [ $rc -eq 0 ] || echo "memory $scene $who: exit $rc"
  }
  cd "$out/panels"
  run panels rust rust.csv "$root/ship/ref-panels" 60 64 2
  run panels $front $front.csv "$bin/panels-$front$speed" 60 64 2
  cd "$out/counter"; export GK_CLICK=40,32,90,10
  run counter rust rust.csv "$root/ship/ref-counter"
  run counter $front $front.csv "$bin/counter-$front$speed"
' _ "$here" "$out" "$root" "$bin" "$frames" "$GK_TIMEOUT" "$front" "$speed"
GK_FRONT=$front python3 "$here/memory.py" verdict panels-60-64-2$sfx "$frames" "$out/panels" || fail=1
GK_FRONT=$front python3 "$here/memory.py" verdict counter-clicks$sfx "$frames" "$out/counter" || fail=1
exit $fail
