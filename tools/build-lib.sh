#!/bin/bash
# build-lib.sh [VARIANT...]: build the library (and, on request, the Rust reference apps) from
# source, in the checkout's own target/ folder. The same script runs on a contributor's machine
# and on CI, for all three platforms:
#   Linux    x86-64, with the host's toolchain by default. GK_TARGET=x86_64-unknown-linux-gnu.2.28
#            links it with zig against glibc 2.28 instead (tools/zigbuild.sh), so it loads on any
#            distribution with that glibc or newer. Release builds do that (make lib-release, CI).
#   macOS    one universal library: both slices (arm64 and x86_64) are built and lipo joins them.
#            Needs Xcode and `rustup target add aarch64-apple-darwin x86_64-apple-darwin`.
#   Windows  x86-64 with MSVC; the Windows SDK's fxc.exe compiles the release shaders. Run it from
#            Git Bash or MSYS2.
#
# `build-lib.sh symbols ref-NAME` instead builds unstripped copies of the ship library and of one
# Rust reference app into dist/symbols, for the profiler (gates/profile.sh). Linux only.
#
# VARIANT: ship and/or diag (default both). ship is what apps link; diag adds offscreen rendering
# and scripted input, for the tests. Results, in the layout tools/dist.sh and the gates read:
#   lib/ship/  lib/diag/       libgpuikit.so | libgpuikit.dylib (universal, install name
#                              @rpath/libgpuikit.dylib) | gpuikit.dll, gpuikit.dll.lib (MSVC import
#                              library), libgpuikit.dll.a and gpuikit.def (MinGW import
#                              library, for Go's cgo; tools/win-implib.sh)
#   lib/VARIANT/libgpuikit_static.a   the same library, static, which Go apps link (universal on
#                              macOS; built for Rust's MinGW target on Windows); on Linux beside
#                              libgpuikit_sys.so and on Windows beside libgpuikit_sys.a, which name
#                              the system libraries it needs
#   lib/VARIANT/ref-*          with GK_REFS=1: the Rust reference apps the gates compare against
#   lib/licences/              THIRD_PARTY.md and texts/ for every crate linked in (tools/licences.py)
#
# Environment:
#   GK_LIB_OUT   where results go (default lib)
#   GK_REFS=1    Linux: also build the Rust reference apps (package gk-ref) into each variant's folder
#   GK_LICENCES=0  skip the licence texts
#   GK_JOBS      cargo's -j (default: cargo's own choice)
#   GK_ENGINE    the rendering engine of the ship variant: gpui-fast (default, the faster fork of
#                GPUI that GPUI-kit supports) or gpui (upstream GPUI: cargo --no-default-features).
#                The diag variant always uses gpui-fast: its retention counters exist only there.
#   GK_TARGET    Linux only: a zig target such as x86_64-unknown-linux-gnu.2.28 (default empty:
#                the host's toolchain). GK_ZIG names a zig binary; by default the pinned one is
#                downloaded once into target/toolchain/. cargo-zigbuild must be on PATH
#                (tools/zigbuild.sh fetch-cargo-zigbuild DIR fetches the pinned one).
#   GK_MAC_TARGETS  macOS slices (default "aarch64-apple-darwin x86_64-apple-darwin");
#                MACOSX_DEPLOYMENT_TARGET defaults to 11.0.
#
# A full build of both variants takes 5 to 15 minutes and about 4 GB of disk in target/; cargo
# uses up to 8 to 12 GB of memory at its peak (lower GK_JOBS if your machine has less).
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd); cd "$repo"
unset CARGO_TARGET_DIR CARGO_BUILD_TARGET_DIR
case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=macos; export MACOSX_DEPLOYMENT_TARGET=${MACOSX_DEPLOYMENT_TARGET:-11.0}
    read -r -a mac_targets <<< "${GK_MAC_TARGETS:-aarch64-apple-darwin x86_64-apple-darwin}" ;;
  MINGW*|MSYS*|CYGWIN*) os=windows ;;
  *) echo "build-lib.sh: unknown system $(uname -s)" >&2; exit 2 ;;
esac
out=${GK_LIB_OUT:-lib}
py=$(command -v python3 || command -v python)
[ $# -gt 0 ] || set -- ship diag
if [ "$1" = symbols ]; then
  ref=${2:?usage: build-lib.sh symbols ref-NAME}
  cargo rustc --release --locked -p gk-capi --lib -- -C strip=none
  cargo rustc --release --locked -p gk-ref --bin "$ref" -- -C strip=none
  rm -rf dist/symbols; mkdir -p dist/symbols; cp target/release/libgpuikit.so "target/release/$ref" dist/symbols/
  ls -l dist/symbols; exit 0
fi
jobs=(); [ -z "${GK_JOBS:-}" ] || jobs=(-j "$GK_JOBS")
rd=target/release
build=(cargo build)
refs=(); [ "${GK_REFS:-0}" != 1 ] || refs=(-p gk-ref)
engine=${GK_ENGINE:-gpui-fast}
case "$engine" in gpui-fast|gpui) ;; *) echo "build-lib.sh: GK_ENGINE is gpui-fast or gpui" >&2; exit 2 ;; esac
export GK_ENGINE=$engine   # tools/licences.py reads it
if [ $os = linux ]; then
  export GK_TARGET=${GK_TARGET:-}
  if [ -n "$GK_TARGET" ]; then
    [ -n "${GK_ZIG:-}" ] || GK_ZIG=$(tools/zigbuild.sh fetch-zig "${GK_ZIG_HOME:-$repo/target/toolchain}")
    export GK_ZIG
    rd=$(tools/zigbuild.sh outdir); build=(tools/zigbuild.sh build)
    echo "build-lib: Linux for $GK_TARGET with $("$GK_ZIG" version 2>/dev/null | sed 's/^/zig /') and $(cargo-zigbuild --version)"
  fi
fi
for v in "$@"; do
  case "$v" in
    ship) feats=(); [ "$engine" = gpui ] && feats=(--no-default-features) ;;
    diag) feats=(--features diag) ;;
    *) echo "build-lib.sh: VARIANT is ship or diag" >&2; exit 2 ;;
  esac
  start=$(date +%s)
  # ${a[@]+"${a[@]}"}: macOS's /bin/bash is 3.2, where an empty array under set -u is unbound.
  if [ $os = macos ]; then
    for t in "${mac_targets[@]}"; do
      "${build[@]}" --release --locked ${jobs[@]+"${jobs[@]}"} --target "$t" -p gk-capi ${feats[@]+"${feats[@]}"}
    done
  else
    "${build[@]}" --release --locked ${jobs[@]+"${jobs[@]}"} -p gk-capi ${refs[@]+"${refs[@]}"} ${feats[@]+"${feats[@]}"}
  fi
  rm -rf "${out:?}/$v"; mkdir -p "$out/$v"
  # Which rendering engine this variant runs on, for tools/dist.sh's VERSIONS file.
  if [ "$v" = ship ]; then echo "$engine" > "$out/$v/ENGINE"; else echo gpui-fast > "$out/$v/ENGINE"; fi
  case $os in
    linux) cp $rd/libgpuikit.so "$out/$v/"
      # The static library, for Go apps that opt into the component families they use (docs/GO.md
      # section 8), and a linker script naming the system libraries it needs by their run-time
      # names, so linking it needs no -dev packages (the same libraries the shared one loads).
      cp $rd/libgpuikit.a "$out/$v/libgpuikit_static.a"
      printf '%s\n' '/* GNU ld script: the system libraries libgpuikit_static.a needs (tools/build-lib.sh). */' \
        'INPUT(AS_NEEDED(libxkbcommon.so.0 libxkbcommon-x11.so.0 libxcb.so.1 libfreetype.so.6))' > "$out/$v/libgpuikit_sys.so"
      if [ ${#refs[@]} -gt 0 ]; then
        for f in $rd/ref-*; do case $f in *.d) ;; *) cp "$f" "$out/$v/" ;; esac; done
      fi
      echo "build-lib: newest glibc symbol: $(objdump -T "$out/$v/libgpuikit.so" | grep -o 'GLIBC_[0-9.]*' | sort -V | tail -1)" ;;
    macos)
      # One slice per target, then one universal file: an app built for either architecture links
      # and loads the same libgpuikit.dylib.
      sl=$out/$v.slices; rm -rf "$sl"; mkdir -p "$sl"
      archs=()
      for t in "${mac_targets[@]}"; do
        case $t in aarch64-*) a=arm64 ;; x86_64-*) a=x86_64 ;; *) echo "build-lib.sh: no macOS architecture for $t" >&2; exit 2 ;; esac
        cp "target/$t/release/libgpuikit.dylib" "$sl/$a.dylib"; archs+=("$a")
        # An app finds the library through its own rpath (beside it, or in Contents/Frameworks of
        # a bundle): the install name must be @rpath/, which rustc normally writes already.
        if [ "$(otool -D "$sl/$a.dylib" | tail -1)" != "@rpath/libgpuikit.dylib" ]; then
          install_name_tool -id @rpath/libgpuikit.dylib "$sl/$a.dylib"
          codesign --force --sign - "$sl/$a.dylib"   # arm64 code must stay signed after the edit
        fi
      done
      f=$out/$v/libgpuikit.dylib
      lipo -create -output "$f" "$sl"/*.dylib
      rm -rf "$sl"
      # The static library, universal too, for Go apps (docs/GO.md section 8).
      statics=(); for t in "${mac_targets[@]}"; do statics+=("target/$t/release/libgpuikit.a"); done
      lipo -create -output "$out/$v/libgpuikit_static.a" "${statics[@]}"
      lipo -info "$f"
      got=$(lipo -archs "$f" | awk '{for (i = 1; i <= NF; i++) print $i}' | sort | tr '\n' ' '); need=$(printf '%s\n' "${archs[@]}" | sort | tr '\n' ' ')
      [ "$got" = "$need" ] || { echo "build-lib.sh: $f has the slices $got, not $need" >&2; exit 1; }
      for a in "${archs[@]}"; do
        echo "build-lib: $a: install name $(otool -arch "$a" -D "$f" | tail -1), minimum macOS $(otool -arch "$a" -l "$f" | awk '/LC_BUILD_VERSION/{b=1} b&&/minos/&&!d{print $2; d=1}')"
      done ;;
    windows)
      cp $rd/gpuikit.dll $rd/gpuikit.dll.lib "$out/$v/"
      tools/win-implib.sh "$out/$v"
      # The static library for Go apps (docs/GO.md section 8). Go's cgo links with MinGW-w64, which
      # cannot link MSVC's objects, so this one is built for Rust's MinGW target, by the toolchain
      # hosted on MinGW (with the MSVC-hosted one, gpui's build script compiles its resources with
      # MSVC's rc.exe, in a format MinGW's linker cannot read). Needs MinGW-w64's gcc, dlltool and
      # windres on PATH. libgpuikit_sys.a is the import library of the Windows API it links
      # (windows-targets' libwindows.0.52.0.a). Rust's MinGW target puts each object's code in one
      # section, so no linker could drop a single function; -Zfunction-sections (an unstable option,
      # hence RUSTC_BOOTSTRAP) gives each function its own, which an app's linker drops when nothing
      # uses it (with lld: GNU ld keeps every function that has unwind data on Windows).
      gnu="$(sed -n 's/^channel = "\(.*\)"/\1/p' rust-toolchain.toml)-x86_64-pc-windows-gnu"
      RUSTC_BOOTSTRAP=1 RUSTFLAGS="${RUSTFLAGS:-} -Zfunction-sections=yes" \
        cargo "+$gnu" build --release --locked ${jobs[@]+"${jobs[@]}"} --target x86_64-pc-windows-gnu -p gk-capi ${feats[@]+"${feats[@]}"}
      cp target/x86_64-pc-windows-gnu/release/libgpuikit.a "$out/$v/libgpuikit_static.a"
      implib=$(find "${CARGO_HOME:-$HOME/.cargo}/registry/src" -path '*windows_x86_64_gnu-0.52*' -name libwindows.0.52.0.a | sort | tail -1)
      [ -n "$implib" ] || { echo "build-lib.sh: no libwindows.0.52.0.a under ${CARGO_HOME:-$HOME/.cargo}/registry/src" >&2; exit 1; }
      cp "$implib" "$out/$v/libgpuikit_sys.a" ;;
  esac
  echo "build-lib: $v built in $(( $(date +%s) - start )) s"; ls -l "$out/$v"
done
if [ "${GK_LICENCES:-1}" = 1 ]; then
  triple=$(rustc -vV | sed -n 's/^host: //p')
  # macOS: every slice's crate graph (x86_64's has a few crates arm64's lacks).
  [ $os != macos ] || triple="${mac_targets[*]}"
  rm -rf "${out:?}/licences"
  GK_LICENCE_TARGET=$triple "$py" tools/licences.py "$out/licences"
  echo "build-lib: licences for $triple in $out/licences"
fi
