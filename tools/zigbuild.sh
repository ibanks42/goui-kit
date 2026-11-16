#!/bin/bash
# zigbuild.sh ACTION [ARGS...]: how the released Linux library is linked, in one place for
# tools/build-lib.sh (GK_TARGET set; make lib-release and CI, .github/workflows/platforms.yml). cargo-zigbuild with a pinned zig links against the glibc of
# GK_TARGET (default x86_64-unknown-linux-gnu.2.28), so the library loads on any distribution with
# that glibc or newer.
#
#   ACTION
#   fetch-zig DIR             put the pinned zig into DIR/zig-x86_64-linux-VERSION/ (once; the
#                             archive's sha256 is checked) and print the zig binary's path
#   fetch-cargo-zigbuild DIR  put the pinned cargo-zigbuild release binary into DIR/ (sha256
#                             checked) and print DIR; for a machine that has none (CI)
#   build ARGS...             cargo zigbuild --target GK_TARGET ARGS... with the link setup below
#   rustc ARGS...             cargo-zigbuild rustc, the same (ARGS may end in -- RUSTC_ARGS)
#   outdir                    print cargo's output folder for GK_TARGET (target/TRIPLE/release)
#
# build and rustc need GK_ZIG (the zig binary) and cargo-zigbuild on PATH, and run in the
# workspace. GK_ZIG_WORK is where the link folder and cargo-zigbuild's cache live (default
# target/zigbuild in the workspace; the paths are part of cargo's fingerprints, so keep it the
# same between builds). A cargo-zigbuild other than the pinned version below works, and the log
# names it.
set -euo pipefail
ZIG_VERSION=0.14.1
ZIG_SHA256=24aeeec8af16c381934a6cd7d95c807a8cb2cf7df9fa40d359aa884195c4716c
CARGO_ZIGBUILD_VERSION=0.24.1
CARGO_ZIGBUILD_SHA256=32581c8e13f27a807bb244aaaac870522f5b51263c766cf77b1a99771ce9fa5c
target=${GK_TARGET:-x86_64-unknown-linux-gnu.2.28}
triple=${target%.*}; triple=${triple%.*}   # x86_64-unknown-linux-gnu.2.28 -> x86_64-unknown-linux-gnu

link_setup() {
  : "${GK_ZIG:?zigbuild.sh: GK_ZIG names the zig binary (zigbuild.sh fetch-zig DIR prints it)}"
  local work have
  work=${GK_ZIG_WORK:-$PWD/target/zigbuild}
  have=$(cargo-zigbuild --version 2>/dev/null | awk '{print $2}') ||
    { echo "zigbuild.sh: cargo-zigbuild is not on PATH (zigbuild.sh fetch-cargo-zigbuild DIR)" >&2; exit 2; }
  [ "$have" = "$CARGO_ZIGBUILD_VERSION" ] ||
    echo "zigbuild.sh: note: cargo-zigbuild $have here, $CARGO_ZIGBUILD_VERSION pinned in tools/zigbuild.sh (and used by CI)" >&2
  export CARGO_ZIGBUILD_ZIG_PATH=$GK_ZIG CARGO_ZIGBUILD_CACHE_DIR=$work/zigbuild-cache
  # The system libraries the link names (freetype and fontconfig through pkg-config, xkbcommon
  # and xcb through #[link]) come from a folder that holds only them: with pkg-config's
  # -L /usr/lib/x86_64-linux-gnu the linker took the host's libm and libc over zig's glibc 2.28
  # and the library needed glibc 2.43 again (measured).
  linkdir=$work/zig-link; mkdir -p "$linkdir"
  local l
  for l in freetype fontconfig xkbcommon xkbcommon-x11 xcb; do
    [ -e "/usr/lib/x86_64-linux-gnu/lib$l.so" ] ||
      { echo "zigbuild.sh: /usr/lib/x86_64-linux-gnu/lib$l.so is missing (its -dev package)" >&2; exit 2; }
    ln -sf "/usr/lib/x86_64-linux-gnu/lib$l.so" "$linkdir/lib$l.so"
  done
  local t=target.x86_64-unknown-linux-gnu
  links=(--config "$t.freetype.rustc-link-search=[\"native=$linkdir\"]" --config "$t.freetype.rustc-link-lib=[\"freetype\"]"
    --config "$t.fontconfig.rustc-link-search=[\"native=$linkdir\"]" --config "$t.fontconfig.rustc-link-lib=[\"fontconfig\"]")
}

sha_ok() { echo "$2  $1" | sha256sum -c - >&2; }   # FILE SHA256 (stdout carries only the printed path)

action=${1:?usage: zigbuild.sh fetch-zig DIR | fetch-cargo-zigbuild DIR | build ARGS | rustc ARGS | outdir}; shift
case "$action" in
  fetch-zig)
    dir=${1:?zigbuild.sh fetch-zig DIR}; zig=$dir/zig-x86_64-linux-$ZIG_VERSION/zig
    if [ ! -x "$zig" ]; then
      mkdir -p "$dir"
      curl -fsSL --retry 3 "https://ziglang.org/download/$ZIG_VERSION/zig-x86_64-linux-$ZIG_VERSION.tar.xz" -o "$dir/zig.tar.xz"
      (cd "$dir" && sha_ok zig.tar.xz "$ZIG_SHA256" && tar xf zig.tar.xz && rm zig.tar.xz)
    fi
    echo "$zig" ;;
  fetch-cargo-zigbuild)
    dir=${1:?zigbuild.sh fetch-cargo-zigbuild DIR}
    if [ "$("$dir/cargo-zigbuild" --version 2>/dev/null | awk '{print $2}')" != "$CARGO_ZIGBUILD_VERSION" ]; then
      mkdir -p "$dir"; a=cargo-zigbuild-x86_64-unknown-linux-gnu.tar.xz
      curl -fsSL --retry 3 "https://github.com/rust-cross/cargo-zigbuild/releases/download/v$CARGO_ZIGBUILD_VERSION/$a" -o "$dir/$a"
      (cd "$dir" && sha_ok "$a" "$CARGO_ZIGBUILD_SHA256" && tar xf "$a" && rm "$a")
      # The archive may hold the binary in a folder of its own.
      [ -x "$dir/cargo-zigbuild" ] || mv "$(find "$dir" -type f -name cargo-zigbuild | head -1)" "$dir/"
      rm -rf "$dir"/cargo-zigbuild-*/
    fi
    echo "$dir" ;;
  build) link_setup; exec cargo zigbuild --target "$target" "${links[@]}" "$@" ;;
  rustc) link_setup; exec cargo-zigbuild rustc --target "$target" "${links[@]}" "$@" ;;
  outdir) echo "target/$triple/release" ;;
  *) echo "zigbuild.sh: ACTION is fetch-zig, fetch-cargo-zigbuild, build, rustc or outdir" >&2; exit 2 ;;
esac
