#!/bin/bash
# Smoke-test an unpacked distribution folder (produced by `tools/dist.sh`) from a user's
# perspective: builds and runs the bundled Go `hello` example in both default shared mode
# (`CGO_ENABLED=0` purego) and static mode (`GK_STATIC=1` with `-tags gkstatic`).
#
# What this verifies on each platform:
# - Isolated runtime lookup: copies the built executable (and shared library in shared mode,
#   nothing else in static mode) into a fresh staging directory outside the archive so hidden
#   build-tree paths cannot mask missing libraries or broken rpaths.
# - Real window lifecycle: sets `GK_FRAMES=5,30` so the app opens a real window, renders
#   30 frames, and exits cleanly with status 0 within a 120-second watchdog timeout.
# - macOS universal check: verifies `libgpuikit_static.a` and `libgpuikit.dylib` each carry
#   both `arm64` and `x86_64` slices, checks the host binary's Mach-O slice via `lipo -archs`,
#   cross-compiles `hello` for the non-host architecture (using `GOARCH` + `CC="clang -arch ..."`
#   for static cgo), and executes the x86_64 build under Rosetta 2 when available on Apple Silicon.
# - Headless Linux fallback: wraps execution in `xvfb-run` if neither `DISPLAY` nor
#   `WAYLAND_DISPLAY` is set.
#
# Usage: tools/dist-test.sh DIST_DIR [go]
# Environment variables:
#   GK_OTHER_ARCH  set to 0 on macOS to skip cross-building/running the non-host architecture.
set -euo pipefail
dl=$(cd "${1:?usage: dist-test.sh DOWNLOAD [go]}" && pwd); shift
[ $# -gt 0 ] || set -- go
case "$(uname -s)" in
  MINGW*|MSYS*|CYGWIN*) os=windows; ext=.exe ;;
  Darwin) os=macos; ext=.bin ;;
  *) os=linux; ext=.bin ;;
esac
out=$(dirname "$dl")/test-$(basename "$dl" | sed 's/^gpuikit-[^-]*-//')
rm -rf "$out"; mkdir -p "$out"
# Run a command with a 120-second watchdog so a hung GUI window fails cleanly instead of blocking CI.
limit() { "$@" & local pid=$! rc=0; ( sleep 120; kill $pid 2> /dev/null ) & local w=$!
  wait $pid || rc=$?; kill $w 2> /dev/null || true; return $rc; }
xvfb=()
if [ $os = linux ] && [ -z "${DISPLAY:-}${WAYLAND_DISPLAY:-}" ]; then
  command -v xvfb-run > /dev/null || { echo "dist-test.sh: no display and no xvfb-run" >&2; exit 1; }
  xvfb=(xvfb-run -a -s "-screen 0 1280x800x24")
fi
# Copy only the built binary (and shared library in shared mode) into an isolated temp folder
# and run 30 frames to verify self-contained library loading and clean shutdown.
run() {
  local name=$1 bin=$2 so=${3:-} dir=$out/$1 rc=0
  mkdir -p "$dir"
  cp "$bin" "$dir/"
  [ -z "$so" ] || cp "$so" "$dir/"
  (cd "$dir" && GK_FRAMES=5,30 limit ${xvfb[@]+"${xvfb[@]}"} "./$(basename "$bin")") > "$dir/log" 2>&1 || rc=$?
  tail -5 "$dir/log"
  if [ $rc != 0 ]; then echo "dist-test: FAIL $name: exit $rc (log $dir/log)"; return 1; fi
  echo "dist-test: PASS $name: 30 frames in a window, exit 0"
}
other=
if [ $os = macos ]; then
  for f in "$dl/lib/libgpuikit_static.a" "$dl/lib/libgpuikit.dylib"; do
    lipo -info "$f"
    got=$(lipo -archs "$f" | awk '{for (i = 1; i <= NF; i++) print $i}' | sort | tr '\n' ' ')
    [ "$got" = "arm64 x86_64 " ] || { echo "dist-test: FAIL $f has the slices '$got', not arm64 and x86_64"; exit 1; }
  done
  here=$(uname -m)
  case $here in
    arm64) other=x86_64; go_other=amd64 ;;
    x86_64) other=arm64; go_other=arm64 ;;
    *) echo "dist-test: FAIL unknown Mac architecture $here"; exit 1 ;;
  esac
  [ "${GK_OTHER_ARCH:-1}" = 1 ] || other=
  other_runs=
  if [ "$other" = x86_64 ] && /usr/bin/arch -x86_64 /usr/bin/true 2> /dev/null; then other_runs=1; fi
  if [ -z "$other" ]; then note="the other architecture is skipped (GK_OTHER_ARCH=0)"
  elif [ -n "$other_runs" ]; then note="hello is built and run for $other too (Rosetta 2)"
  elif [ "$other" = x86_64 ]; then note="hello is built and linked for $other too, not run (no Rosetta 2 here)"
  else note="hello is built and linked for $other too, not run (an Intel Mac cannot run arm64)"; fi
  echo "dist-test: this Mac is $here; $note"
fi
arch_of() {
  local got; got=$(lipo -archs "$1" | awk '{for (i = 1; i <= NF; i++) printf "%s%s", (n++ ? " " : ""), $i}')
  [ "$got" = "$2" ] || { echo "dist-test: FAIL $1 is '$got', not $2"; return 1; }
}
for lang in "$@"; do
  case $lang in
    go)
      case $os in windows) so=$dl/lib/gpuikit.dll ;; macos) so=$dl/lib/libgpuikit.dylib ;; *) so=$dl/lib/libgpuikit.so ;; esac
      for mode in shared static; do
        st=; [ $mode = static ] && st=1
        h=$dl/examples/go/hello$ext
        rm -f "$h"
        start=$(date +%s); GK_STATIC=$st "$dl/examples/go/build.sh" hello
        echo "dist-test: go hello ($mode) built in $(( $(date +%s) - start )) s, $(( $(wc -c < "$h") / 1000000 )) MB$([ $mode = static ] && echo ', one file')"
        [ $os != macos ] || arch_of "$h" "$here"
        if [ $mode = static ]; then run go-static "$h"; else run go "$h" "$so"; fi
        if [ -n "$other" ]; then
          rm -f "$h"
          start=$(date +%s)
          if [ $mode = static ]; then GK_STATIC=1 GOARCH=$go_other CC="clang -arch $other" "$dl/examples/go/build.sh" hello
          else GOARCH=$go_other "$dl/examples/go/build.sh" hello; fi
          echo "dist-test: go hello ($mode) for $other built in $(( $(date +%s) - start )) s"
          arch_of "$h" "$other"
          if [ -n "$other_runs" ]; then
            if [ $mode = static ]; then run "go-static-$other" "$h"; else run "go-$other" "$h" "$so"; fi
          else echo "dist-test: BUILT go-$mode-$other: hello for $other (not run here)"; fi
        fi
      done ;;
    *) echo "dist-test.sh: go is the only language" >&2; exit 2 ;;
  esac
done
