#!/bin/bash
# Build native `gk-capi` `ship` and/or `diag` libraries (and optional `ref-*` binaries) into `lib/`.
#
# Why two library variants exist:
# - `ship`: release library distributed to Go apps (minimal feature set; honors `GK_ENGINE`).
# - `diag`: release library with `--features diag` enabled (adds the headless offscreen
#   test runner, scripted frame stepping, pixel readback, and protocol diagnostic hooks used
#   by `gates/`). Always built against `gpui-fast`.
#
# Usage:
#   tools/build-lib.sh [ship] [diag]       build ship, diag, or both (default: both)
#   tools/build-lib.sh symbols ref-NAME    build unstripped library and ref binary into dist/symbols/
#
# Environment variables:
#   GK_ENGINE          `gpui-fast` (default) or `gpui` (passes `--no-default-features` for `ship`)
#   GK_REFS            set to 1 to also compile `gk-ref` reference binaries alongside `gk-capi`
#   GK_TARGET          Linux glibc-pinned target passed to `tools/zigbuild.sh`
#                      (e.g. `x86_64-unknown-linux-gnu.2.28` for portable release binaries)
#   GK_MAC_TARGETS     space-separated macOS Rust targets compiled and joined with `lipo`
#                      (default: `aarch64-apple-darwin x86_64-apple-darwin`)
#   MACOSX_DEPLOYMENT_TARGET  minimum macOS version embedded in `LC_BUILD_VERSION` (default: `11.0`)
#   GK_LIB_OUT         output directory (default: `lib`)
#   GK_JOBS            optional `cargo -j` parallelism cap
#   GK_LICENCES        set to 0 to skip running `tools/licences.py` (default: 1)
#
# Per-platform artifact outputs in `$GK_LIB_OUT/<variant>/`:
# - Linux:   `libgouikit.so`, `libgouikit_static.a`, `libgouikit_sys.so` (GNU ld script naming
#            X11/xkb/freetype SONAMEs for static cgo links), plus `ref-*` when `GK_REFS=1`.
# - macOS:   universal `libgouikit.dylib` (install name `@rpath/libgouikit.dylib`, ad-hoc
#            re-signed after `install_name_tool`) and universal `libgouikit_static.a`.
# - Windows: MSVC-built `gouikit.dll` + `gouikit.dll.lib`, GNU-ABI `libgouikit_static.a`
#            compiled with `-Zfunction-sections=yes` so MinGW/lld can dead-strip unused families,
#            and `libgouikit_sys.a` (Windows API import lib copied from `windows_x86_64_gnu`).
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd); cd "$repo"
# Clear any inherited target-dir overrides so outputs always land under the workspace `target/`
# directory where the copy steps below expect them.
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
  # Build unstripped release binaries (overriding Cargo's release strip setting) so
  # `tools/profile/sampler.c` and `tools/profile/symbolize.py` can resolve Rust symbols.
  ref=${2:?usage: build-lib.sh symbols ref-NAME}
  cargo rustc --release --locked -p gk-capi --lib -- -C strip=none
  cargo rustc --release --locked -p gk-ref --bin "$ref" -- -C strip=none
  rm -rf dist/symbols; mkdir -p dist/symbols; cp target/release/libgouikit.so "target/release/$ref" dist/symbols/
  ls -l dist/symbols; exit 0
fi
jobs=(); [ -z "${GK_JOBS:-}" ] || jobs=(-j "$GK_JOBS")
rd=target/release
build=(cargo build)
refs=(); [ "${GK_REFS:-0}" != 1 ] || refs=(-p gk-ref)
engine=${GK_ENGINE:-gpui-fast}
case "$engine" in gpui-fast|gpui) ;; *) echo "build-lib.sh: GK_ENGINE is gpui-fast or gpui" >&2; exit 2 ;; esac
export GK_ENGINE=$engine
if [ $os = linux ]; then
  export GK_TARGET=${GK_TARGET:-}
  if [ -n "$GK_TARGET" ]; then
    # On Linux release builds, routing through tools/zigbuild.sh links against an older glibc
    # symbol table (e.g. 2.28) instead of the build host's system glibc.
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
  if [ $os = macos ]; then
    for t in "${mac_targets[@]}"; do
      "${build[@]}" --release --locked ${jobs[@]+"${jobs[@]}"} --target "$t" -p gk-capi ${feats[@]+"${feats[@]}"}
    done
  else
    "${build[@]}" --release --locked ${jobs[@]+"${jobs[@]}"} -p gk-capi ${refs[@]+"${refs[@]}"} ${feats[@]+"${feats[@]}"}
  fi
  rm -rf "${out:?}/$v"; mkdir -p "$out/$v"
  # Record which GPUI engine produced this directory so tools/dist.sh can report it in VERSIONS.
  if [ "$v" = ship ]; then echo "$engine" > "$out/$v/ENGINE"; else echo gpui-fast > "$out/$v/ENGINE"; fi
  case $os in
    linux) cp $rd/libgouikit.so "$out/$v/"
      cp $rd/libgouikit.a "$out/$v/libgouikit_static.a"
      # Emit a thin GNU ld linker script rather than hard-coding -l flags in Go cgo directives:
      # `AS_NEEDED(...)` ensures static cgo binaries link the exact versioned X11/xkb/freetype
      # SONAMEs required by GPUI without requiring -dev symlinks on end-user machines.
      printf '%s\n' '/* GNU ld script: the system libraries libgouikit_static.a needs (tools/build-lib.sh). */' \
        'INPUT(AS_NEEDED(libxkbcommon.so.0 libxkbcommon-x11.so.0 libxcb.so.1 libfreetype.so.6))' > "$out/$v/libgouikit_sys.so"
      if [ ${#refs[@]} -gt 0 ]; then
        for f in $rd/ref-*; do case $f in *.d) ;; *) cp "$f" "$out/$v/" ;; esac; done
      fi
      # Print the highest GLIBC_* symbol version referenced so CI/maintainers immediately see
      # if a build accidentally pulled in a newer host glibc symbol.
      echo "build-lib: newest glibc symbol: $(objdump -T "$out/$v/libgouikit.so" | grep -o 'GLIBC_[0-9.]*' | sort -V | tail -1)" ;;
    macos)
      # Normalize each architecture slice's Mach-O install name to `@rpath/libgouikit.dylib`
      # (Cargo writes an absolute target path by default), ad-hoc re-sign each slice because
      # `install_name_tool` invalidates the arm64 linker signature, and join slices via `lipo`.
      sl=$out/$v.slices; rm -rf "$sl"; mkdir -p "$sl"
      archs=()
      for t in "${mac_targets[@]}"; do
        case $t in aarch64-*) a=arm64 ;; x86_64-*) a=x86_64 ;; *) echo "build-lib.sh: no macOS architecture for $t" >&2; exit 2 ;; esac
        cp "target/$t/release/libgouikit.dylib" "$sl/$a.dylib"; archs+=("$a")
        if [ "$(otool -D "$sl/$a.dylib" | tail -1)" != "@rpath/libgouikit.dylib" ]; then
          install_name_tool -id @rpath/libgouikit.dylib "$sl/$a.dylib"
          codesign --force --sign - "$sl/$a.dylib"
        fi
      done
      f=$out/$v/libgouikit.dylib
      lipo -create -output "$f" "$sl"/*.dylib
      rm -rf "$sl"
      statics=(); for t in "${mac_targets[@]}"; do statics+=("target/$t/release/libgouikit.a"); done
      lipo -create -output "$out/$v/libgouikit_static.a" "${statics[@]}"
      lipo -info "$f"
      # Verify the resulting universal dylib contains the exact set of requested architecture slices.
      got=$(lipo -archs "$f" | awk '{for (i = 1; i <= NF; i++) print $i}' | sort | tr '\n' ' '); need=$(printf '%s\n' "${archs[@]}" | sort | tr '\n' ' ')
      [ "$got" = "$need" ] || { echo "build-lib.sh: $f has the slices $got, not $need" >&2; exit 1; }
      for a in "${archs[@]}"; do
        echo "build-lib: $a: install name $(otool -arch "$a" -D "$f" | tail -1), minimum macOS $(otool -arch "$a" -l "$f" | awk '/LC_BUILD_VERSION/{b=1} b&&/minos/&&!d{print $2; d=1}')"
      done ;;
    windows)
      # Ship two complementary Windows binaries:
      # 1. `gouikit.dll` (and `.dll.lib`) built with the default MSVC toolchain for purego dynamic loading.
      # 2. `libgouikit_static.a` built with the pinned GNU-ABI toolchain (`x86_64-pc-windows-gnu`)
      #    because Go cgo on Windows links via MinGW/lld rather than MSVC. Setting
      #    `RUSTC_BOOTSTRAP=1` + `-Zfunction-sections=yes` puts each function in its own COMDAT
      #    section so `--gc-sections` can dead-strip unused component families.
      # 3. `libgouikit_sys.a`: the `windows-targets` import library required by static links.
      cp $rd/gouikit.dll $rd/gouikit.dll.lib "$out/$v/"
      gnu="$(sed -n 's/^channel = "\(.*\)"/\1/p' rust-toolchain.toml)-x86_64-pc-windows-gnu"
      RUSTC_BOOTSTRAP=1 RUSTFLAGS="${RUSTFLAGS:-} -Zfunction-sections=yes" \
        cargo "+$gnu" build --release --locked ${jobs[@]+"${jobs[@]}"} --target x86_64-pc-windows-gnu -p gk-capi ${feats[@]+"${feats[@]}"}
      cp target/x86_64-pc-windows-gnu/release/libgouikit.a "$out/$v/libgouikit_static.a"
      implib=$(find "${CARGO_HOME:-$HOME/.cargo}/registry/src" -path '*windows_x86_64_gnu-0.52*' -name libwindows.0.52.0.a | sort | tail -1)
      [ -n "$implib" ] || { echo "build-lib.sh: no libwindows.0.52.0.a under ${CARGO_HOME:-$HOME/.cargo}/registry/src" >&2; exit 1; }
      cp "$implib" "$out/$v/libgouikit_sys.a" ;;
  esac
  echo "build-lib: $v built in $(( $(date +%s) - start )) s"; ls -l "$out/$v"
done
if [ "${GK_LICENCES:-1}" = 1 ]; then
  # Collect third-party crate license notices matching the exact built target triple(s)
  # (on macOS, passes all universal slice triples so slice-specific crates are included).
  triple=$(rustc -vV | sed -n 's/^host: //p')
  [ $os != macos ] || triple="${mac_targets[*]}"
  rm -rf "${out:?}/licences"
  GK_LICENCE_TARGET=$triple "$py" tools/licences.py "$out/licences"
  echo "build-lib: licences for $triple in $out/licences"
fi
