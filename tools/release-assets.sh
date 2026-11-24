#!/bin/bash
# release-assets.sh VERSION DIR [COMMIT]: check the files a release publishes and write
# DIR/SHA256SUMS. Run by the release job of .github/workflows/platforms.yml on the three
# platforms' downloads (tools/dist.sh), before `gh release create`; runs anywhere with bash,
# GNU tar with zstd, sha256sum and python3 (no Rust).
#
# Checks, and exits non-zero on the first failure:
#   - VERSION is the version tools/dist.sh writes (Cargo.toml's workspace version);
#   - DIR holds exactly these three files, the names other projects download by:
#       gpuikit-VERSION-linux-x86_64.tar.zst
#       gpuikit-VERSION-macos-universal.tar.zst
#       gpuikit-VERSION-windows-x86_64.zip
#   - each download's VERSIONS says `gpuikit VERSION for PLATFORM`, states the interface hash of
#     this checkout (go/gk/gen_ops.go's apiHash: the three agree with each other and with the
#     source), and the sha256 of the library inside that archive;
#   - macOS: the library is a universal static archive of exactly an arm64 and an x86_64 slice
#     (tools/macho.py reads it, as there is no lipo here), and VERSIONS's `architectures:` line
#     names both;
#   - with COMMIT: every VERSIONS names a source commit COMMIT starts with, without uncommitted
#     changes (the three were built from the commit being released).
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
ver=${1:?usage: release-assets.sh VERSION DIR [COMMIT]}; dir=${2:?DIR}; commit=${3:-}
fail() { echo "release-assets: FAIL: $*" >&2; exit 1; }
want=$(grep -m1 '^version' "$repo/Cargo.toml" | cut -d'"' -f2)
[ "$ver" = "$want" ] || fail "version $ver, but tools/dist.sh writes $want (Cargo.toml): tag v$want, or change the version first"
hash=$(grep -m1 'const ApiHash' "$repo/go/gk/internal/rt/gen_ops.go" | grep -o '0x[0-9a-f]*')
[ -n "$hash" ] || fail "no ApiHash in go/gk/internal/rt/gen_ops.go"
platforms=(linux-x86_64 macos-universal windows-x86_64)
expected=()
for p in "${platforms[@]}"; do
  case $p in windows-*) e=zip ;; *) e=tar.zst ;; esac
  expected+=("gpuikit-$ver-$p.$e")
done
# Every name in DIR but SHA256SUMS (each pass ends in a true status: set -e and pipefail).
have=$(for f in "$dir"/* "$dir"/.[!.]*; do
  if [ -e "$f" ] && [ "${f##*/}" != SHA256SUMS ]; then echo "${f##*/}"; fi
done | sort)
want_list=$(printf '%s\n' "${expected[@]}" | sort)
[ "$have" = "$want_list" ] || fail "$dir holds
$have
but a release is exactly
$want_list"
# One file of an archive to stdout: ARCHIVE PATH.
member() {
  case $1 in
    *.zip) python3 -I -c 'import sys, zipfile, shutil; shutil.copyfileobj(zipfile.ZipFile(sys.argv[1]).open(sys.argv[2]), sys.stdout.buffer)' "$1" "$2" ;;
    *) tar --zstd -xOf "$1" "$2" ;;
  esac
}
for p in "${platforms[@]}"; do
  case $p in
    linux-*|macos-*) e=tar.zst; lib=libgpuikit_static.a ;;
    windows-*) e=zip; lib=libgpuikit_static.a ;;
  esac
  name=gpuikit-$ver-$p; arc=$dir/$name.$e
  v=$(member "$arc" "$name/VERSIONS") || fail "$name.$e has no $name/VERSIONS"
  [ "$(head -1 <<< "$v")" = "gpuikit $ver for $p" ] || fail "$name/VERSIONS begins '$(head -1 <<< "$v")'"
  h=$(sed -n 's/^interface hash: \(0x[0-9a-f]*\).*/\1/p' <<< "$v")
  [ "$h" = "$hash" ] || fail "$name/VERSIONS states interface hash '${h:-none}', the source $hash"
  s=$(sed -n 's/^library sha256: \([0-9a-f]*\).*/\1/p' <<< "$v")
  got=$(member "$arc" "$name/lib/$lib" | sha256sum | cut -c1-64) || fail "$name.$e has no $name/lib/$lib"
  [ "$s" = "$got" ] || fail "$name/VERSIONS states library sha256 '${s:-none}', the archive's $lib has $got"
  slices=
  if [ "$p" = macos-universal ]; then
    a=$(sed -n 's/^architectures: //p' <<< "$v")
    case "$a" in *arm64*x86_64*|*x86_64*arm64*) ;; *) fail "$name/VERSIONS says architectures '${a:-none}', not arm64 and x86_64" ;; esac
    chk=$(member "$arc" "$name/lib/$lib" | python3 -I "$repo/tools/macho.py" check - arm64 x86_64 --kind archive 2>&1) \
      || fail "$name/lib/$lib: $chk"
    slices=", universal: $chk"
  fi
  if [ -n "$commit" ]; then
    c=$(sed -n 's/^source commit: //p' <<< "$v")
    case "$c" in
      *uncommitted*|*unknown*|"") fail "$name/VERSIONS says source commit '$c'" ;;
    esac
    case "$commit" in "$c"*) ;; *) fail "$name/VERSIONS says source commit $c, the release is $commit" ;; esac
  fi
  echo "release-assets: $name: interface hash $h, $lib sha256 ${got:0:16}${commit:+, commit $c}$slices"
done
(cd "$dir" && sha256sum "${expected[@]}" > SHA256SUMS)
echo "release-assets: all three present and consistent; SHA256SUMS:"; cat "$dir/SHA256SUMS"
