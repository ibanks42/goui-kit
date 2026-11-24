#!/bin/bash
# dist.sh [LIBROOT]: assemble exactly what an app developer downloads, for one platform, into
# dist/gpuikit-VERSION-PLATFORM/ and an archive beside it. No Rust source, no toolchain: the
# prebuilt library, the Go module, the C header, the examples with a build script, the guides and
# the licence texts of every crate linked into the
# library. PLATFORM is GK_PLATFORM, or this machine's:
#   linux-x86_64    built by tools/build-lib.sh, linked by tools/zigbuild.sh against glibc 2.28
#                   for a release (make lib-release, CI); .tar.zst
#   macos-universal built by tools/build-lib.sh on a Mac: one universal library, arm64 and
#                   x86_64 (Apple silicon and Intel); .tar.zst
#   windows-x86_64  built by tools/build-lib.sh on Windows; .zip
# Every platform's download holds the same folders and files; only the library's files differ:
#   lib/libgpuikit.so                      Linux: the library (release, no grammars, no speech)
#   lib/libgpuikit_static.a                Linux: the same as a static library, which Go apps link
#                                          (each keeps only the component families it uses)
#   lib/libgpuikit_sys.so                  Linux: a linker script naming the system libraries it needs
#   lib/libgpuikit_static.a                macOS (universal) and Windows (MinGW's format) too; on
#   lib/libgpuikit_sys.a                   Windows beside the import library of the Windows API it links
#   lib/libgpuikit.dylib                   macOS: the same, universal (arm64 and x86_64 slices, each
#                                          with install name @rpath/libgpuikit.dylib)
#   lib/gpuikit.dll                        Windows: the same, and its import libraries:
#   lib/libgpuikit.dll.a, lib/gpuikit.def  MinGW's, which Go's cgo links (gcc)
#   windows/gpuikit.manifest, gpuikit.rc   Windows: the manifest a program embeds (DPI awareness)
#   go/                      the Go module gpuikit.dev (package gk, and s, theme, palette): an app's
#                            go.mod requires it and replaces it with this folder
#   include/*.h              the C interface, for other languages
#   README.md                where to start, for this platform (written here)
#   examples/go/             hello, counter, notes, todo, reader, expenses (module
#                            gpuikit.dev/examples) and build.sh
#   docs/                    the guides and the programs their snippets come from
#   VERSIONS                 what was built from what, what the library needs, the compilers tested
#   THIRD_PARTY.md, licences/ the crates linked into this platform's library, their licences and texts
# The build script is the same file on every platform (they look at uname). Only the ship
# library is shipped; the diagnostic one (lib/diag) is for this repository's own tests.
# LIBROOT (default lib) holds ship/ and licences/ (or GK_LICENCES names the licences
# folder), as tools/build-lib.sh leaves them. GK_DIST_OUT is the output folder (default dist).
# Prints every file with its size.
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd); cd "$repo"
root=${1:-lib}
lic=${GK_LICENCES:-$root/licences}
dst=${GK_DIST_OUT:-dist}
py=$(command -v python3 || command -v python)
if [ -z "${GK_PLATFORM:-}" ]; then
  case "$(uname -s)/$(uname -m)" in
    Linux/x86_64) GK_PLATFORM=linux-x86_64 ;;
    Darwin/*) GK_PLATFORM=macos-universal ;;
    MINGW*/x86_64|MSYS*/x86_64|CYGWIN*/x86_64) GK_PLATFORM=windows-x86_64 ;;
    *) echo "dist.sh: no download for $(uname -s) $(uname -m); set GK_PLATFORM" >&2; exit 2 ;;
  esac
fi
case "$GK_PLATFORM" in
  # Apps link the library into the program (only the component families they use); the shared
  # library is built too (lib/ship) but not shipped: what the library needs is read from it.
  linux-x86_64) libs=(libgpuikit_static.a libgpuikit_sys.so); main=libgpuikit_static.a; probe=libgpuikit.so; ext=.bin ;;
  macos-universal) libs=(libgpuikit_static.a); main=libgpuikit_static.a; probe=libgpuikit.dylib; ext=.bin ;;
  windows-x86_64) libs=(libgpuikit_static.a libgpuikit_sys.a); main=libgpuikit_static.a; probe=gpuikit.dll; ext=.exe ;;
  *) echo "dist.sh: GK_PLATFORM is linux-x86_64, macos-universal or windows-x86_64" >&2; exit 2 ;;
esac
ver=$(grep -m1 '^version' Cargo.toml | cut -d'"' -f2)
name=gpuikit-$ver-$GK_PLATFORM
out=$dst/$name
sha256() { if command -v sha256sum > /dev/null; then sha256sum "$1"; else shasum -a 256 "$1"; fi | cut -c1-64; }
for f in "${libs[@]}"; do
  [ -f "$root/ship/$f" ] || { echo "dist.sh: $root/ship/$f is missing (make lib, or tools/build-lib.sh ship on $GK_PLATFORM)" >&2; exit 1; }
done
# The macOS download is named universal: the library must hold exactly the arm64 and x86_64 slices.
if [ "$GK_PLATFORM" = macos-universal ]; then
  got=$(lipo -archs "$root/ship/libgpuikit_static.a" | awk '{for (i = 1; i <= NF; i++) print $i}' | sort | tr '\n' ' ')
  [ "$got" = "arm64 x86_64 " ] || { echo "dist.sh: $root/ship/libgpuikit_static.a has the slices '$got', not arm64 and x86_64 (tools/build-lib.sh builds both)" >&2; exit 1; }
fi
# The licence texts of every crate linked into the library (tools/build-lib.sh writes them).
[ -f "$lic/THIRD_PARTY.md" ] || { echo "dist.sh: $lic/THIRD_PARTY.md is missing (make lib writes it)" >&2; exit 1; }
rm -rf "${dst:?}/$name" "$dst/$name".*
mkdir -p "$out/lib" "$out/go" "$out/include" "$out/examples/go" "$out/docs"
for f in "${libs[@]}"; do cp "$root/ship/$f" "$out/lib/"; done
if [ "$GK_PLATFORM" = windows-x86_64 ]; then
  mkdir -p "$out/windows"; cp core/windows/gpuikit.manifest core/windows/gpuikit.rc "$out/windows/"
fi
cp -r "$lic" "$out/licences"
cp "$lic/THIRD_PARTY.md" "$out/THIRD_PARTY.md"
# The Go module as the repository has it.
cp go/go.mod "$out/go/" && cp -r go/gk "$out/go/"
cp core/include/gpuikit.h core/include/gk_ops.h "$out/include/"
# The Go examples: one module whose go.mod replaces gpuikit.dev with ../../go, the download's go/.
cp examples/go/go.mod "$out/examples/go/"
cp -r examples/go/hello examples/go/counter examples/go/notes examples/go/todo examples/go/reader \
  examples/go/expenses "$out/examples/go/"
# The guides and the programs every snippet in them is cut from (gates/docs-go.sh builds them); the
# programs' go.mod files replace gpuikit.dev with ../../go as well.
cp docs/GETTING-STARTED-GO.md docs/API-GO.md docs/GO.md "$out/docs/"
cp -r docs/tutorial-go docs/snippets-go "$out/docs/"
cat > "$out/examples/go/build.sh" <<'B'
#!/bin/sh
# build.sh [APP]: build a Go app against the prebuilt library. Needs Go 1.27 or newer and a C
# compiler for cgo: gcc or clang on Linux (CC=clang), the Xcode command-line tools on macOS,
# MinGW-w64 gcc on Windows (MSYS2's ucrt64 or w64devkit), where this runs in Git Bash or MSYS2.
#   APP   an example's name (hello, counter, notes, todo, reader, expenses; default hello), built into
#         examples/go/APP.bin (APP.exe on Windows), or the path of any folder that holds a main
#         package of a module requiring gpuikit.dev (docs/tutorial-go/03-list, ../myapp), built into
#         FOLDER.bin (FOLDER.exe) beside it
# The library is linked into the binary (lib/libgpuikit_static.a), keeping only the component
# families the app calls into (-Wl,--gc-sections; -Wl,-dead_strip on macOS): one file, nothing to
# ship beside it (on Windows, the manifest: see below). Go's own
# settings pass through: on macOS GOARCH=amd64 CC='clang -arch x86_64' builds for Intel on Apple
# silicon (the library is universal), GOARCH=arm64 CC='clang -arch arm64' the other way.
set -e
here=$(cd "$(dirname "$0")" && pwd); lib=$(cd "$here/../../lib" && pwd); app=${1:-hello}
case "$(uname -s)" in MINGW*|MSYS*|CYGWIN*) win=1; ext=.exe ;; *) win=; ext=.bin ;; esac
case "$app" in
  */*) src=$(cd "${app%/}" && pwd); bin=$src$ext ;;
  *) src=$here/$app; bin=$here/$app$ext ;;
esac
cd "$src"
if [ -n "$win" ]; then
  # With lld (llvm-mingw, MSYS2's mingw-w64-ucrt-x86_64-lld) the linker drops the component families
  # the app does not use; GNU ld cannot on Windows (it keeps every function that has unwind data),
  # so without lld the program holds all of them. The manifest comes with package gk (a .syso);
  # -force:multipleres lets it replace the default one MinGW's gcc adds.
  if command -v ld.lld > /dev/null; then x="-fuse-ld=lld -Wl,--gc-sections,-Xlink=-force:multipleres"; else x="-Wl,--gc-sections"; fi
  CGO_ENABLED=1 CGO_LDFLAGS="-L$(cygpath -m "$lib")" go build -ldflags="-s -w '-extldflags=$x'" -o "$(cygpath -m "$bin")" .
elif [ "$(uname -s)" = Darwin ]; then
  CGO_ENABLED=1 CGO_LDFLAGS="-L$lib" go build -ldflags="-s -w -extldflags=-Wl,-dead_strip" -o "$bin" .
else
  CGO_ENABLED=1 CGO_LDFLAGS="-L$lib" go build -ldflags="-s -w -extldflags=-Wl,--gc-sections" -o "$bin" .
fi
echo "built $bin"
B
chmod +x "$out/examples/go/build.sh"
# What differs per platform in the README: the system, what to install, how an app links.
case "$GK_PLATFORM" in
  linux-x86_64) sys="Linux x86_64"; libdesc="the library"
    go_needs="a C compiler for cgo (gcc, or clang with CC=clang)"
    go_link='CGO_LDFLAGS="-L<this folder>/lib" go build -ldflags="-s -w -extldflags=-Wl,--gc-sections"'
    ship_note='An app is one file: the library is linked into it, with only the component families the app calls into (a Go app without the -ldflags still works, but holds all of them).' ;;
  macos-universal) sys="macOS, Apple silicon and Intel (universal)"
    libdesc="the library, universal (arm64 and x86_64), which an app links into itself"
    go_needs="the Xcode command-line tools (clang) for cgo"
    go_link='CGO_LDFLAGS="-L<this folder>/lib" go build -ldflags="-s -w -extldflags=-Wl,-dead_strip"'
    ship_note="An app is one file: the library is linked into it, with only the component families the app calls into. The build script builds for this Mac's architecture. For Intel on Apple silicon: GOARCH=amd64 CC='clang -arch x86_64' examples/go/build.sh APP (the other way round: GOARCH=arm64, -arch arm64); lipo -create joins the two programs into a universal one." ;;
  windows-x86_64) sys="Windows x86_64"; libdesc="the library, which Go apps link into themselves (MinGW's format)"
    go_needs="MinGW-w64 gcc and lld on PATH for cgo (MSYS2's ucrt64 with the mingw-w64-ucrt-x86_64-lld package, llvm-mingw, or WinLibs' LLVM build)"
    go_link="CGO_LDFLAGS=\"-L<this folder>/lib\" go build -ldflags=\"-s -w '-extldflags=-fuse-ld=lld -Wl,--gc-sections,-Xlink=-force:multipleres'\" (with lld on PATH; without it, -extldflags=-Wl,--gc-sections, and the program holds every component)"
    ship_note="An app is one .exe: the library is linked into it, with only the component families the app calls into. The manifest is linked in too: without it the program does not start (the library needs common controls 6) and the window is not DPI aware." ;;
esac
librows=$(for f in "${libs[@]}"; do
  case $f in
    ("$main") echo "| lib/$f | $libdesc; an app keeps only the component families it uses |" ;;
    (libgpuikit_sys.so) echo "| lib/$f | a linker script that names the system libraries the static library needs |" ;;
    (libgpuikit_sys.a) echo "| lib/$f | the import library of the Windows API the static library links |" ;;
    (*) echo "| lib/$f | an import library of gpuikit.dll |" ;;
  esac
done
[ "$GK_PLATFORM" != windows-x86_64 ] || echo "| windows/ | the manifest a program embeds, and the resource script that embeds it |")
kit=$("$py" -c "import json;print(json.load(open('api/coverage.json', encoding='utf-8'))['pin']['gpui-kit'])")
# The first file a newcomer opens: what this is and where to start.
cat > "$out/README.md" <<R
# gpuikit $ver for $sys

Desktop apps in **Go** with GPUI-kit $kit: GPUI's renderer and the kit's components, from one
prebuilt library behind a C interface. No Rust is needed.

## Go

Start with **docs/GETTING-STARTED-GO.md**: what to install, building the examples, and a
tutorial. Then **docs/API-GO.md**.
Needs Go 1.27 or newer and $go_needs.

    examples/go/build.sh hello && examples/go/hello$ext
    examples/go/build.sh ../myapp                    builds any app folder into ../myapp$ext

An app's go.mod requires the module in go/ and points at it:

    require gpuikit.dev v0.0.0
    replace gpuikit.dev => ../$name/go

and builds with $go_link as examples/go/build.sh does. $ship_note

| path | what |
|---|---|
$librows
| go/ | the Go module gpuikit.dev: import "gpuikit.dev/gk" (and gk/s, gk/theme, gk/palette) |
| examples/go/ | hello, counter, todo, reader, notes, expenses, and build.sh |
| docs/ | the guides, and the tutorial's programs in docs/tutorial-go/ |
| include/ | the C interface, for other languages |
| VERSIONS | what the library was built from, and what it needs to run |
| THIRD_PARTY.md, licences/ | the licences of everything inside the library |
R
# What the library needs on this platform, read from the shared library of the same build.
lf=$root/ship/$probe
case "$GK_PLATFORM" in
  linux-x86_64)
    needs="glibc $(objdump -T "$lf" | grep -o 'GLIBC_[0-9.]*' | sort -V | tail -1 | cut -d_ -f2) or newer (the newest symbol version the library refers to; it is linked against glibc 2.28), and $(objdump -p "$lf" | awk '/NEEDED/{print $2}' | grep -v 'libc.so\|libm.so\|ld-linux' | tr '\n' ' ')" ;;
  macos-universal)
    # Per slice: its minimum macOS (LC_BUILD_VERSION); the download needs the highest of them.
    archs=$(lipo -archs "$lf"); arch_line=; mins=
    for a in $archs; do
      m=$(otool -arch "$a" -l "$lf" | awk '/LC_BUILD_VERSION/{b=1} b&&/minos/&&!d{print $2; d=1}')
      arch_line="$arch_line${arch_line:+, }$a (minimum macOS ${m:-unknown})"; mins="$mins${m:+$m
}"
    done
    minos=$(printf '%s' "$mins" | sort -t. -k1,1n -k2,2n -k3,3n | tail -1)
    arch_line="
architectures: $arch_line"
    needs="macOS ${minos:-11.0} or newer, on Apple silicon or Intel (one universal library, arm64 and x86_64; the highest of the slices' minimums, LC_BUILD_VERSION), and the system's $(for a in $archs; do otool -arch "$a" -L "$lf" | tail -n +2; done | awk '{print $1}' | grep -v '@rpath' | sed 's|.*/||' | sort -u | tr '\n' ' ')" ;;
  windows-x86_64)
    od=$(command -v objdump || command -v llvm-objdump || true)
    dlls=$( [ -n "$od" ] && "$od" -p "$lf" | awk '/DLL Name:/{print $3}' | sort -u | tr '\n' ' ' || true)
    needs="Windows 10 or newer, x86-64, with Direct3D 11, and the system's ${dlls:-DLLs (not listed: no objdump on the machine that made this download)}" ;;
esac
engine=$(cat "$root/ship/ENGINE" 2>/dev/null || echo gpui-fast)
[ "$engine" = gpui ] && engine="upstream GPUI" || engine="gpui-fast $("$py" -c "import json;print(json.load(open('api/coverage.json', encoding='utf-8'))['pin'].get('gpui-fast', '?'))")"
hash=$(grep -m1 'const apiHash' go/gk/gen_ops.go | grep -o '0x[0-9a-f]*')
go_v=$(go version 2>/dev/null | awk '{print $3}' | sed 's/^go//' || true)
cat > "$out/VERSIONS" <<V
gpuikit $ver for $GK_PLATFORM
library: release, gpui-kit $("$py" -c "import json;p=json.load(open('api/coverage.json', encoding='utf-8'))['pin'];print(p['gpui-kit'], 'features', ' '.join(f for f in p['features'] if f != 'gpui-fast'))"), on $engine
interface hash: $hash (the library and the package refuse each other when it differs)
needs: $needs${arch_line:-}
licences: THIRD_PARTY.md (every crate linked into the library, its licence; texts in licences/texts)
library sha256: $(sha256 "$out/lib/$main")
source commit: $(git rev-parse --short HEAD 2>/dev/null || echo "${GK_COMMIT:-unknown} (a copy without git)")$(git diff --quiet 2>/dev/null || [ -n "${GK_COMMIT:-}" ] || echo ' with uncommitted changes')
tested with: Go ${go_v:-not found}
V
if [ "$GK_PLATFORM" = windows-x86_64 ]; then
  ( cd "$dst" && "$py" -m zipfile -c "$name.zip" "$name" )
  arc=$name.zip
else
  ( cd "$dst" && tar -cf - "$name" | zstd -q -T0 -o "$name.tar.zst" )
  arc=$name.tar.zst
fi
echo "dist: $out"
# Every file with its size (the generated files and the licence texts summed), portably.
"$py" - "$dst" "$name" "$arc" <<'P'
import os, sys
dst, name, arc = sys.argv[1:]
out = os.path.join(dst, name)
def files(top):
    for d, _, fs in os.walk(top):
        for f in fs:
            p = os.path.join(d, f)
            yield os.path.relpath(p, out).replace(os.sep, "/"), os.path.getsize(p)
def mb(n): return f"{n / 1e6:.1f} MB"
gen_go = texts = 0; n_go = n_texts = 0
listed = []
for rel, size in files(out):
    base = rel.rsplit("/", 1)[-1]
    if rel.startswith("go/gk/") and base.startswith("gen_"): gen_go += size; n_go += 1
    elif rel.startswith("licences/texts/"): texts += size; n_texts += 1
    else: listed.append((rel, size))
for rel, size in sorted(listed): print(f"{size:10d}  ./{rel}")
crates = len([d for d in os.listdir(os.path.join(out, "licences", "texts")) if d != "_canonical"])
print(f"{texts:10d}  ./licences/texts/ ({n_texts} licence files of {crates} crates)")
print(f"{gen_go:10d}  ./go/gk/gen_*.go, ./go/gk/{{s,theme,palette}}/gen_*.go ({n_go} generated files)")
total = sum(s for _, s in files(out))
print(f"unpacked: {mb(total)}; download: {mb(os.path.getsize(os.path.join(dst, arc)))} ({arc})")
P
