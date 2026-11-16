#!/bin/bash
# install-odin.sh [DIR]: download the Odin compiler release this project is tested with from
# github.com/odin-lang/Odin/releases and unpack it into DIR (default ~/.local/opt/odin). No sudo.
# Afterwards, put DIR on your PATH, or point the build scripts at it with ODIN=DIR/odin.
#
# The release is ODIN_RELEASE (default dev-2026-11, the one CI uses). The compiler is one binary
# plus its base, core and vendor libraries, about 250 MB unpacked. On Linux and macOS it links
# with clang, so clang must be installed. To remove it, delete DIR.
#
# Already have Odin? A newer monthly release usually works too; this script only saves you looking
# up which one the project was tested with.
set -euo pipefail
release=${ODIN_RELEASE:-dev-2026-11}
dest=${1:-$HOME/.local/opt/odin}
case "$(uname -s)-$(uname -m)" in
  Linux-x86_64) asset=linux-amd64 ;;
  Linux-aarch64) asset=linux-arm64 ;;
  Darwin-arm64) asset=macos-arm64 ;;
  Darwin-x86_64) asset=macos-amd64 ;;
  MINGW*-*|MSYS*-*|CYGWIN*-*) asset=windows-amd64 ;;
  *) echo "install-odin: no Odin release for $(uname -s) $(uname -m); see https://odin-lang.org/docs/install/" >&2; exit 2 ;;
esac
if [ -x "$dest/odin" ] && "$dest/odin" version 2>/dev/null | grep -q "$release"; then
  echo "install-odin: $("$dest/odin" version) is already in $dest"; exit 0
fi
api=https://api.github.com/repos/odin-lang/Odin/releases/tags/$release
url=$(curl -fsSL "$api" | python3 -c '
import json, sys
want = sys.argv[1]
for a in json.load(sys.stdin)["assets"]:
    if want in a["name"]:
        print(a["browser_download_url"]); break
' "$asset") || true
[ -n "$url" ] || { echo "install-odin: release $release has no $asset download; see https://github.com/odin-lang/Odin/releases" >&2; exit 1; }
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
echo "install-odin: downloading $url"
curl -fL --retry 3 -o "$tmp/${url##*/}" "$url"
# Some releases wrap the tarball in a zip: unpack until the compiler is there.
cd "$tmp"
for _ in 1 2; do
  for a in *.zip *.tar.gz; do
    [ -f "$a" ] || continue
    case $a in *.zip) unzip -q -o "$a" ;; *) tar xzf "$a" ;; esac
    rm -f "$a"
  done
done
odin=$(find "$tmp" -type f \( -name odin -o -name odin.exe \) | head -1)
[ -n "$odin" ] || { echo "install-odin: no odin binary in the download" >&2; exit 1; }
rm -rf "$dest"; mkdir -p "$(dirname "$dest")"
mv "$(dirname "$odin")" "$dest"
chmod +x "$dest"/odin*
command -v clang >/dev/null || [ "$asset" = windows-amd64 ] || echo "install-odin: clang is missing; Odin links with it" >&2
echo "install-odin: $("$dest/odin" version) in $dest"
echo "install-odin: add it to your PATH, e.g.  export PATH=\"$dest:\$PATH\""
