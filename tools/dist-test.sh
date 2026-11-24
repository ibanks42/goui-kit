#!/bin/bash
# dist-test.sh DOWNLOAD [go]: what an app developer does first, from an unpacked download
# (dist/gpuikit-VERSION-PLATFORM), on any of the three platforms: build the hello example with
# the download's own examples/go/build.sh, then run that program in a real window for 30 frames
# (GK_FRAMES=5,30: the library draws them and exits; Linux without a display runs it under
# xvfb-run). Needs go with a C compiler on PATH. Writes DOWNLOAD/../test-PLATFORM/ and exits non-zero on the first failure.
# macOS (the universal download): first the library's slices (lipo: exactly arm64 and x86_64);
# then hello for this Mac's architecture as above, and again for the other one (Apple silicon:
# x86_64, GOARCH=amd64 CC='clang -arch x86_64'; Intel: arm64), each binary checked to be that
# architecture alone. The other architecture's hello runs the same
# way when this Mac can run it (x86_64 on Apple silicon with Rosetta 2); otherwise it is built
# and linked against the universal library only, and the log says so. GK_OTHER_ARCH=0 skips the
# other architecture.
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
# A two-minute limit without timeout(1), which macOS lacks; and without a helper program between,
# since macOS drops DYLD_LIBRARY_PATH when it starts a system binary (perl, env).
limit() { "$@" & local pid=$! rc=0; ( sleep 120; kill $pid 2> /dev/null ) & local w=$!
  wait $pid || rc=$?; kill $w 2> /dev/null || true; return $rc; }
# Linux without a display: a virtual X server for the run (xvfb-run, from the xvfb package).
xvfb=()
if [ $os = linux ] && [ -z "${DISPLAY:-}${WAYLAND_DISPLAY:-}" ]; then
  command -v xvfb-run > /dev/null || { echo "dist-test.sh: no display and no xvfb-run" >&2; exit 1; }
  xvfb=(xvfb-run -a -s "-screen 0 1280x800x24")
fi
run() { # NAME BINARY: runs the program as built, in a window, for 30 frames
  local name=$1 bin=$2 dir=$out/$1 rc=0
  mkdir -p "$dir"
  # From a copy in its own folder, the way it is shipped: one file.
  cp "$bin" "$dir/"
  (cd "$dir" && GK_FRAMES=5,30 limit ${xvfb[@]+"${xvfb[@]}"} "./$(basename "$bin")") > "$dir/log" 2>&1 || rc=$?
  tail -5 "$dir/log"
  if [ $rc != 0 ]; then echo "dist-test: FAIL $name: exit $rc (log $dir/log)"; return 1; fi
  echo "dist-test: PASS $name: 30 frames in a window, exit 0"
}
# macOS: the universal library's slices, and which architecture is the other one here.
other=
if [ $os = macos ]; then
  for f in "$dl/lib/libgpuikit_static.a"; do
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
  # Rosetta 2 runs x86_64 code on Apple silicon; an Intel Mac cannot run arm64 at all. The probe
  # is a system binary run as x86_64: it starts only when Rosetta is installed.
  other_runs=
  if [ "$other" = x86_64 ] && /usr/bin/arch -x86_64 /usr/bin/true 2> /dev/null; then other_runs=1; fi
  if [ -z "$other" ]; then note="the other architecture is skipped (GK_OTHER_ARCH=0)"
  elif [ -n "$other_runs" ]; then note="hello is built and run for $other too (Rosetta 2)"
  elif [ "$other" = x86_64 ]; then note="hello is built and linked for $other too, not run (no Rosetta 2 here)"
  else note="hello is built and linked for $other too, not run (an Intel Mac cannot run arm64)"; fi
  echo "dist-test: this Mac is $here; $note"
fi
arch_of() { # BINARY ARCH: the binary is that one architecture alone
  local got; got=$(lipo -archs "$1" | awk '{for (i = 1; i <= NF; i++) printf "%s%s", (n++ ? " " : ""), $i}')
  [ "$got" = "$2" ] || { echo "dist-test: FAIL $1 is '$got', not $2"; return 1; }
}
for lang in "$@"; do
  case $lang in
    go) start=$(date +%s); "$dl/examples/go/build.sh" hello
      echo "dist-test: go hello built in $(( $(date +%s) - start )) s, $(( $(wc -c < "$dl/examples/go/hello$ext") / 1000000 )) MB, one file"
      [ $os != macos ] || arch_of "$dl/examples/go/hello$ext" "$here"
      run go "$dl/examples/go/hello$ext"
      if [ -n "$other" ]; then
        start=$(date +%s); GOARCH=$go_other CC="clang -arch $other" "$dl/examples/go/build.sh" hello
        echo "dist-test: go hello for $other built in $(( $(date +%s) - start )) s"
        arch_of "$dl/examples/go/hello$ext" "$other"
        if [ -n "$other_runs" ]; then
          run "go-$other" "$dl/examples/go/hello$ext"
        else echo "dist-test: LINKED go-$other: hello for $other links against the universal library (not run here)"; fi
      fi ;;
    *) echo "dist-test.sh: go is the only language" >&2; exit 2 ;;
  esac
done
