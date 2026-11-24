#!/bin/bash
# Memory-growth gate (`gates/memory.sh [--quick|--selftest]`).
#
# What it gates:
#   Steady-state resident memory (RSS) growth of Go apps relative to their Rust references across
#   three sustained workloads, plus a `--selftest` mode that proves the gate rejects a real leak.
#
# How it measures and why the comparison is fair:
#   1. Gallery cycles (offscreen on `lib/diag`): generates a multi-cycle script covering all pages
#      in `ref/gallery/pages.txt` (default target ~100,000 frames; ~30,000 with `--quick`),
#      rebuilds `gallery-go` if stale, and samples RSS via `gates/memory.py sample` for both
#      `ref-gallery` and `gallery-go`.
#   2. Panels (`panels 60 64 2`) and clicking counter (`GK_CLICK=40,32,90,10`) on `lib/ship`: runs
#      60 warmup + 6,000 measured frames (`2,000` with `--quick`) inside one `display.sh --hz 1000`
#      session for Rust and Go in turn. Retries up to 3 times on timeout (exit 124) if the headless
#      compositor stalls, logging stalls to `stalls.txt`.
#   3. Comparing Go's second-half RSS growth minus Rust's cancels out shared GPUI/driver growth.
#   4. In `--selftest` mode, `GALLERY_LEAK=512` leaks 512 bytes per view build in `gallery-go`;
#      PASS requires `memory.py verdict` to fail specifically for excess growth after a full run.
#
# What PASS means (exit 0):
#   In normal mode, `gates/memory.py verdict` exits 0 for `gallery`, `panels-60-64-2`, and
#   `counter-clicks` (excess growth <= `SLACK_KB`, default 2048 kB). In `--selftest`, the injected
#   512-byte leak is caught and rejected.

here=$(cd "$(dirname "$0")" && pwd); repo=$(dirname "$here")
target=${GALLERY_FRAMES:-100000}; frames=${FRAMES:-6000}; hz=${HZ:-1000}
[ "${1:-}" = --quick ] && { target=${GALLERY_FRAMES:-30000}; frames=${FRAMES:-2000}; }
selftest=0; [ "${1:-}" = --selftest ] && { selftest=1; export GALLERY_LEAK=512; }
root=${GK_LIB_ROOT:-$repo/lib}; bin=${GK_BIN:-$repo/bin}
front=go; sfx=-$front
out=$here/out/memory$sfx; rm -rf "$out"; mkdir -p "$out/gallery" "$out/panels" "$out/counter"
pages=$repo/ref/gallery/pages.txt
# Compute enough full gallery cycles to reach or exceed the target frame count.
cycles=${CYCLES:-$(( target / $(python3 "$here/gallery.py" frames "$pages" --cycles 1) + 1 ))}
fail=0

# Rebuild gallery-go if missing or older than any gallery or gk package source file.
ref=${GK_REF:-$root/diag}/ref-gallery; [ -x "$ref" ] || ref=$root/diag/ref-gallery
if { [ ! -x "$bin/gallery-go" ] ||
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
    # Selftest passes only if the verdict fails specifically for excess growth after a full run.
    if GK_FRONT=$front python3 "$here/memory.py" verdict gallery-leak-512 "$n" "$out/gallery" > "$out/selftest.txt"; then
      echo "GATE memory selftest$sfx FAIL a leak of 512 bytes a view build was not seen: $(grep '^GATE' "$out/selftest.txt" | cut -d' ' -f5-)"; exit 1
    fi
    if ! grep -q 'app grows more than the Rust reference' "$out/selftest.txt" || grep -q 'samples (the run failed' "$out/selftest.txt"; then
      echo "GATE memory selftest$sfx FAIL the leaking run did not complete: $(grep '^GATE' "$out/selftest.txt" | cut -d' ' -f5-)"; exit 1
    fi
    echo "GATE memory selftest$sfx PASS a leak of 512 bytes a view build is refused: $(grep '^GATE' "$out/selftest.txt" | cut -d' ' -f5-)"; exit 0
  fi
  GK_FRONT=$front python3 "$here/memory.py" verdict gallery$sfx "$n" "$out/gallery" || fail=1
else
  echo "GATE memory gallery$sfx FAIL $ref or $bin/gallery-$front is missing"; fail=1
fi

# Budget timeout assuming at least half the requested refresh rate, plus 30 s startup slack.
GK_TIMEOUT=$(( frames * 3 / (hz / 2) + 30 ))
"$here/display.sh" --hz "$hz" bash -c '
  here=$1; out=$2; root=$3; bin=$4; frames=$5; t=$6; front=$7
  export LD_LIBRARY_PATH=$root/ship GK_FRAMES=60,$frames
  # Retry up to 3 times if timeout exits 124 (for instance, a headless compositor stall).
  run() {
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
  run panels $front $front.csv "$bin/panels-$front" 60 64 2
  cd "$out/counter"; export GK_CLICK=40,32,90,10
  run counter rust rust.csv "$root/ship/ref-counter"
  run counter $front $front.csv "$bin/counter-$front"
' _ "$here" "$out" "$root" "$bin" "$frames" "$GK_TIMEOUT" "$front"
GK_FRONT=$front python3 "$here/memory.py" verdict panels-60-64-2$sfx "$frames" "$out/panels" || fail=1
GK_FRONT=$front python3 "$here/memory.py" verdict counter-clicks$sfx "$frames" "$out/counter" || fail=1
exit $fail
