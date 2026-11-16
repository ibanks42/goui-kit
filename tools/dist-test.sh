#!/bin/bash
# dist-test.sh DOWNLOAD [odin] [go]: what an app developer does first, from an unpacked download
# (dist/gpuikit-VERSION-PLATFORM, beside its -diag folder), on any of the three platforms: build
# the hello example with the download's own examples/build.sh and examples/go/build.sh, then run
# each offscreen under a gk-meter script (GK_SCRIPT) against the diagnostic library: 30 frames,
# a screenshot where the platform has an offscreen renderer (Linux: wgpu, macOS: Metal; gpui-fast
# has none on Windows, where the frames run without pixels), and the boundary counters. Default:
# both languages. Needs odin (ODIN, or on PATH) and go with a C compiler on PATH. Writes
# DOWNLOAD/../test-PLATFORM/ and exits non-zero on the first failure.
# macOS (the universal download): first both libraries' slices (lipo: exactly arm64 and x86_64,
# each with install name @rpath/libgpuikit.dylib); then hello for this Mac's architecture as above,
# and again for the other one (Apple silicon: x86_64, odin -target:darwin_amd64 and
# GOARCH=amd64 CC='clang -arch x86_64'; Intel: arm64), each binary checked to be that architecture
# alone (a program left from the native build cannot pass for it: the build scripts remove it
# first). The other architecture's hello runs the same way when this Mac can run it (x86_64 on
# Apple silicon with Rosetta 2); otherwise it is built and linked against the universal library
# only, and the log says so. GK_OTHER_ARCH=0 skips the other architecture.
set -euo pipefail
dl=$(cd "${1:?usage: dist-test.sh DOWNLOAD [odin] [go]}" && pwd); shift
[ $# -gt 0 ] || set -- odin go
diag=$dl-diag/lib
[ -d "$diag" ] || { echo "dist-test.sh: no diagnostic library at $diag" >&2; exit 1; }
case "$(uname -s)" in
  MINGW*|MSYS*|CYGWIN*) os=windows; ext=.exe; shot= ;;
  Darwin) os=macos; ext=.bin; shot=1 ;;
  *) os=linux; ext=.bin; shot=1 ;;
esac
if [ -n "${ODIN:-}" ]; then d=$(dirname "$ODIN"); [ $os != windows ] || d=$(cygpath -u "$d"); PATH=$d:$PATH; fi
out=$(dirname "$dl")/test-$(basename "$dl" | sed 's/^gpuikit-[^-]*-//')
rm -rf "$out"; mkdir -p "$out"
script=$out/hello.script
{ echo "size 480 320"; for _ in $(seq 30); do echo frame; done; [ -z "$shot" ] || echo "shot hello.png"; echo stat; } > "$script"
# A two-minute limit without timeout(1), which macOS lacks; and without a helper program between,
# since macOS drops DYLD_LIBRARY_PATH when it starts a system binary (perl, env).
limit() { "$@" & local pid=$! rc=0; ( sleep 120; kill $pid 2> /dev/null ) & local w=$!
  wait $pid || rc=$?; kill $w 2> /dev/null || true; return $rc; }
native() { if [ $os = windows ]; then cygpath -m "$1"; else echo "$1"; fi; }
run() { # NAME BINARY: runs it offscreen (a build linked against the diagnostic library), in its own folder
  local name=$1 bin=$2 dir=$out/$1 rc=0
  mkdir -p "$dir"
  case $os in
    # Windows loads gpuikit.dll from the program's folder first: a copy of the program beside the
    # diagnostic DLL.
    windows) cp "$bin" "$dir/"
      # The manifest the program needs to start (common controls 6), beside it (a Go program has it
      # linked in already; then this copy is not read).
      cp "$dl/windows/gpuikit.manifest" "$dir/$(basename "$bin").manifest"
      (cd "$dir" && GK_SCRIPT=$(native "$script") limit "./$(basename "$bin")") > "$dir/log" 2>&1 || rc=$? ;;
    macos) (cd "$dir" && GK_SCRIPT=$script limit "$bin") > "$dir/log" 2>&1 || rc=$? ;;
    linux) (cd "$dir" && GK_SCRIPT=$script limit "$bin") > "$dir/log" 2>&1 || rc=$? ;;
  esac
  tail -5 "$dir/log"
  if [ $rc != 0 ]; then echo "dist-test: FAIL $name: exit $rc (log $dir/log)"; return 1; fi
  if [ -n "$shot" ] && [ ! -s "$dir/hello.png" ]; then echo "dist-test: FAIL $name: no hello.png"; return 1; fi
  echo "dist-test: PASS $name: 30 frames offscreen, exit 0${shot:+, hello.png $(wc -c < "$dir/hello.png") bytes}"
}
# macOS: the universal library's slices, and which architecture is the other one here.
other=
if [ $os = macos ]; then
  for f in "$dl/lib/libgpuikit_static.a" "$diag/libgpuikit_static.a"; do
    lipo -info "$f"
    got=$(lipo -archs "$f" | awk '{for (i = 1; i <= NF; i++) print $i}' | sort | tr '\n' ' ')
    [ "$got" = "arm64 x86_64 " ] || { echo "dist-test: FAIL $f has the slices '$got', not arm64 and x86_64"; exit 1; }
  done
  here=$(uname -m)
  case $here in
    arm64) other=x86_64; odin_other=darwin_amd64; go_other=amd64 ;;
    x86_64) other=arm64; odin_other=darwin_arm64; go_other=arm64 ;;
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
    odin) start=$(date +%s); "$dl/examples/build.sh" hello
      echo "dist-test: odin hello built in $(( $(date +%s) - start )) s, $(( $(wc -c < "$dl/examples/hello$ext") / 1000000 )) MB, one file"
      [ $os != macos ] || arch_of "$dl/examples/hello$ext" "$here"
      # The library is linked into the program: the offscreen run uses a second build of hello,
      # linked the same way against the diagnostic download's static library.
      odindiag() { # OUT [ODIN-FLAG...]
        local o=$1; shift
        if [ $os = windows ]; then
          odin build "$(native "$dl/examples/hello")" -o:minimal -linker:"${ODIN_LINKER:-lld}" -collection:gpuikit="$(native "$dl/odin")" \
            "-extra-linker-flags:/LIBPATH:$(native "$diag")" -out:"$(native "$o")" "$@"
        else
          odin build "$dl/examples/hello" -o:minimal -collection:gpuikit="$dl/odin" "-extra-linker-flags:-L$diag" -out:"$o" "$@"
        fi
      }
      odindiag "$out/hello-odin-diag$ext"
      run odin "$out/hello-odin-diag$ext"
      if [ -n "$other" ]; then
        start=$(date +%s); "$dl/examples/build.sh" hello "-target:$odin_other"
        echo "dist-test: odin hello for $other built in $(( $(date +%s) - start )) s"
        arch_of "$dl/examples/hello$ext" "$other"
        if [ -n "$other_runs" ]; then
          odindiag "$out/hello-odin-diag-$other$ext" "-target:$odin_other"
          run "odin-$other" "$out/hello-odin-diag-$other$ext"
        else echo "dist-test: LINKED odin-$other: hello for $other links against the universal library (not run here)"; fi
      fi ;;
    go) start=$(date +%s); "$dl/examples/go/build.sh" hello
      echo "dist-test: go hello built in $(( $(date +%s) - start )) s, $(( $(wc -c < "$dl/examples/go/hello$ext") / 1000000 )) MB, one file"
      [ $os != macos ] || arch_of "$dl/examples/go/hello$ext" "$here"
      # A Go program has the library linked into it, so it cannot be pointed at the diagnostic one at
      # run time: the offscreen run uses a second build of hello, linked the same way against the
      # diagnostic download's static library.
      case $os in macos) strip_flag=-Wl,-dead_strip ;; *) strip_flag=-Wl,--gc-sections ;; esac
      godiag() { # OUT [ENV...]: hello linked against the diagnostic library
        local o=$1; shift
        (cd "$dl/examples/go/hello" && env CGO_ENABLED=1 CGO_LDFLAGS="-L$(native "$diag")" "$@" \
          go build -ldflags="-s -w -extldflags=$strip_flag" -o "$(native "$o")" .)
      }
      godiag "$out/hello-diag$ext"
      run go "$out/hello-diag$ext"
      if [ -n "$other" ]; then
        start=$(date +%s); GOARCH=$go_other CC="clang -arch $other" "$dl/examples/go/build.sh" hello
        echo "dist-test: go hello for $other built in $(( $(date +%s) - start )) s"
        arch_of "$dl/examples/go/hello$ext" "$other"
        if [ -n "$other_runs" ]; then
          godiag "$out/hello-diag-$other$ext" GOARCH=$go_other CC="clang -arch $other"
          run "go-$other" "$out/hello-diag-$other$ext"
        else echo "dist-test: LINKED go-$other: hello for $other links against the universal library (not run here)"; fi
      fi ;;
    *) echo "dist-test.sh: odin or go" >&2; exit 2 ;;
  esac
done
