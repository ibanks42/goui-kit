#!/bin/bash
# Manage pinned Zig and cargo-zigbuild toolchains and run glibc-targeted Linux Rust builds.
#
# Subcommands:
#   fetch-zig DIR             download and SHA-256 verify Zig into DIR, printing the zig path
#   fetch-cargo-zigbuild DIR  download and SHA-256 verify cargo-zigbuild into DIR
#   build ARGS...             invoke `cargo zigbuild --target $GK_TARGET` with link overrides
#   rustc ARGS...             invoke `cargo-zigbuild rustc --target $GK_TARGET` with link overrides
#   outdir                    print target/<triple>/release for the configured GK_TARGET
#
# Environment variables:
#   GK_TARGET     glibc-pinned Rust target (default: x86_64-unknown-linux-gnu.2.28)
#   GK_ZIG        path to the zig executable (required for build/rustc)
#   GK_ZIG_WORK   scratch directory for cargo-zigbuild cache and system-lib symlink shim
set -euo pipefail
# Pinned release versions and SHA-256 checksums shared with CI.
ZIG_VERSION=0.14.1
ZIG_SHA256=24aeeec8af16c381934a6cd7d95c807a8cb2cf7df9fa40d359aa884195c4716c
CARGO_ZIGBUILD_VERSION=0.24.1
CARGO_ZIGBUILD_SHA256=32581c8e13f27a807bb244aaaac870522f5b51263c766cf77b1a99771ce9fa5c
target=${GK_TARGET:-x86_64-unknown-linux-gnu.2.28}
# Strip the trailing .MAJOR.MINOR glibc suffix to obtain Cargo's output target triple.
triple=${target%.*}; triple=${triple%.*}

# Stage a clean directory containing symlinks only to the five dynamic system libraries
# we link against, preventing Zig's linker from picking up host glibc or static archives in /usr/lib.
link_setup() {
  : "${GK_ZIG:?zigbuild.sh: GK_ZIG names the zig binary (zigbuild.sh fetch-zig DIR prints it)}"
  local work have
  work=${GK_ZIG_WORK:-$PWD/target/zigbuild}
  have=$(cargo-zigbuild --version 2>/dev/null | awk '{print $2}') ||
    { echo "zigbuild.sh: cargo-zigbuild is not on PATH (zigbuild.sh fetch-cargo-zigbuild DIR)" >&2; exit 2; }
  [ "$have" = "$CARGO_ZIGBUILD_VERSION" ] ||
    echo "zigbuild.sh: note: cargo-zigbuild $have here, $CARGO_ZIGBUILD_VERSION pinned in tools/zigbuild.sh (and used by CI)" >&2
  export CARGO_ZIGBUILD_ZIG_PATH=$GK_ZIG CARGO_ZIGBUILD_CACHE_DIR=$work/zigbuild-cache
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

sha_ok() { echo "$2  $1" | sha256sum -c - >&2; }

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
      [ -x "$dir/cargo-zigbuild" ] || mv "$(find "$dir" -type f -name cargo-zigbuild | head -1)" "$dir/"
      rm -rf "$dir"/cargo-zigbuild-*/
    fi
    echo "$dir" ;;
  build) link_setup; exec cargo zigbuild --target "$target" "${links[@]}" "$@" ;;
  rustc) link_setup; exec cargo-zigbuild rustc --target "$target" "${links[@]}" "$@" ;;
  outdir) echo "target/$triple/release" ;;
  *) echo "zigbuild.sh: ACTION is fetch-zig, fetch-cargo-zigbuild, build, rustc or outdir" >&2; exit 2 ;;
esac
