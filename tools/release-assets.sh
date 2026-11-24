#!/bin/bash
# Release pre-flight gate run on the collected release directory before publishing assets.
#
# Invariants checked across all three platform archives (`linux-x86_64`, `macos-universal`,
# `windows-x86_64`):
# 1. Version matches `Cargo.toml` (the same source `tools/dist.sh` reads).
# 2. Directory contains the exact three expected archive filenames and nothing else
#    (apart from any pre-existing `SHA256SUMS` which is overwritten at the end).
# 3. Every archive's `VERSIONS` header matches `goui-kit <VERSION> for <PLATFORM>` and its
#    `interface hash` matches `const ApiHash` in `go/gk/internal/rt/gen_ops.go` (ensuring
#    all three platform builds share the exact same wire ABI).
# 4. Both `library sha256` (static archive) and `shared library sha256` (shared library)
#    recorded in `VERSIONS` match the actual streamed bytes inside the archive.
# 5. For `macos-universal`, validates both `libgouikit_static.a` and `libgouikit.dylib`
#    through `tools/macho.py check` (pure Python, so this release check runs on Linux CI
#    without needing macOS `lipo`/`otool`) to confirm `arm64` + `x86_64` slices and the
#    `@rpath/libgouikit.dylib` install name.
# 6. When optional `COMMIT` is passed, rejects `uncommitted`/`unknown` stamps and confirms
#    each archive's `source commit` is a prefix of `COMMIT`.
# 7. Writes `SHA256SUMS` covering all three verified archives.
#
# Usage: tools/release-assets.sh VERSION DIR [COMMIT]
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
ver=${1:?usage: release-assets.sh VERSION DIR [COMMIT]}; dir=${2:?DIR}; commit=${3:-}
fail() { echo "release-assets: FAIL: $*" >&2; exit 1; }
# Cross-check the requested release tag version against Cargo.toml and read the expected C/Go ApiHash.
want=$(grep -m1 '^version' "$repo/Cargo.toml" | cut -d'"' -f2)
[ "$ver" = "$want" ] || fail "version $ver, but tools/dist.sh writes $want (Cargo.toml): tag v$want, or change the version first"
hash=$(grep -m1 'const ApiHash' "$repo/go/gk/internal/rt/gen_ops.go" | grep -o '0x[0-9a-f]*')
[ -n "$hash" ] || fail "no ApiHash in go/gk/internal/rt/gen_ops.go"
platforms=(linux-x86_64 macos-universal windows-x86_64)
expected=()
for p in "${platforms[@]}"; do
  case $p in windows-*) e=zip ;; *) e=tar.zst ;; esac
  expected+=("goui-kit-$ver-$p.$e")
done
# Ensure the release folder holds the exact expected archive set (ignoring any existing SHA256SUMS).
have=$(for f in "$dir"/* "$dir"/.[!.]*; do
  if [ -e "$f" ] && [ "${f##*/}" != SHA256SUMS ]; then echo "${f##*/}"; fi
done | sort)
want_list=$(printf '%s\n' "${expected[@]}" | sort)
[ "$have" = "$want_list" ] || fail "$dir holds
$have
but a release is exactly
$want_list"
# Stream a single member out of a .zip or .tar.zst release archive to stdout without unpacking
# to disk (uses `python3 -I` for zip so user site-packages cannot interfere).
member() {
  case $1 in
    *.zip) python3 -I -c 'import sys, zipfile, shutil; shutil.copyfileobj(zipfile.ZipFile(sys.argv[1]).open(sys.argv[2]), sys.stdout.buffer)' "$1" "$2" ;;
    *) tar --zstd -xOf "$1" "$2" ;;
  esac
}
for p in "${platforms[@]}"; do
  case $p in
    linux-*) e=tar.zst; lib=libgouikit_static.a; so=libgouikit.so ;;
    macos-*) e=tar.zst; lib=libgouikit_static.a; so=libgouikit.dylib ;;
    windows-*) e=zip; lib=libgouikit_static.a; so=gouikit.dll ;;
  esac
  name=goui-kit-$ver-$p; arc=$dir/$name.$e
  v=$(member "$arc" "$name/VERSIONS") || fail "$name.$e has no $name/VERSIONS"
  [ "$(head -1 <<< "$v")" = "goui-kit $ver for $p" ] || fail "$name/VERSIONS begins '$(head -1 <<< "$v")'"
  h=$(sed -n 's/^interface hash: \(0x[0-9a-f]*\).*/\1/p' <<< "$v")
  [ "$h" = "$hash" ] || fail "$name/VERSIONS states interface hash '${h:-none}', the source $hash"
  s=$(sed -n 's/^library sha256: \([0-9a-f]*\).*/\1/p' <<< "$v")
  got=$(member "$arc" "$name/lib/$lib" | sha256sum | cut -c1-64) || fail "$name.$e has no $name/lib/$lib"
  [ "$s" = "$got" ] || fail "$name/VERSIONS states library sha256 '${s:-none}', the archive's $lib has $got"
  s=$(sed -n 's/^shared library sha256: \([0-9a-f]*\).*/\1/p' <<< "$v")
  sgot=$(member "$arc" "$name/lib/$so" | sha256sum | cut -c1-64) || fail "$name.$e has no $name/lib/$so"
  [ "$s" = "$sgot" ] || fail "$name/VERSIONS states shared library sha256 '${s:-none}', the archive's $so has $sgot"
  slices=
  if [ "$p" = macos-universal ]; then
    a=$(sed -n 's/^architectures: //p' <<< "$v")
    case "$a" in *arm64*x86_64*|*x86_64*arm64*) ;; *) fail "$name/VERSIONS says architectures '${a:-none}', not arm64 and x86_64" ;; esac
    chk=$(member "$arc" "$name/lib/$lib" | python3 -I "$repo/tools/macho.py" check - arm64 x86_64 --kind archive 2>&1) \
      || fail "$name/lib/$lib: $chk"
    chk2=$(member "$arc" "$name/lib/$so" | python3 -I "$repo/tools/macho.py" check - arm64 x86_64 --kind dylib --id @rpath/libgouikit.dylib 2>&1) \
      || fail "$name/lib/$so: $chk2"
    slices=", universal: $chk; $chk2"
  fi
  if [ -n "$commit" ]; then
    c=$(sed -n 's/^source commit: //p' <<< "$v")
    case "$c" in
      *uncommitted*|*unknown*|"") fail "$name/VERSIONS says source commit '$c'" ;;
    esac
    case "$commit" in "$c"*) ;; *) fail "$name/VERSIONS says source commit $c, the release is $commit" ;; esac
  fi
  echo "release-assets: $name: interface hash $h, $lib sha256 ${got:0:16}, $so sha256 ${sgot:0:16}${commit:+, commit $c}$slices"
done
(cd "$dir" && sha256sum "${expected[@]}" > SHA256SUMS)
echo "release-assets: all three present and consistent; SHA256SUMS:"; cat "$dir/SHA256SUMS"
