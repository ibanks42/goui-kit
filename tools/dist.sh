#!/bin/bash
# Assemble a self-contained platform distribution directory and compressed archive
# under dist/ from prebuilt `ship` libraries in lib/.
#
# Usage: tools/dist.sh [LIB_ROOT]
#   LIB_ROOT defaults to `lib`; expects `$LIB_ROOT/ship/` and `$LIB_ROOT/licences/`.
#
# Environment variables:
#   GK_PLATFORM   Target archive platform: `linux-x86_64`, `macos-universal`, or
#                 `windows-x86_64`. Auto-detected from `uname -s`/`uname -m` if unset.
#   GK_DIST_OUT   Output directory for the staged folder and archive (default: `dist`).
#   GK_LICENCES   Directory containing `THIRD_PARTY.md` and `texts/` (default: `$LIB_ROOT/licences`).
#   GK_COMMIT     Fallback source commit hash written into `VERSIONS` when building
#                 outside a git checkout (for example in a CI tarball job).
#
# Layout of the produced archive (`gpuikit-<ver>-<platform>/`):
#   lib/          ship shared and static libraries plus linker helper scripts/import libs
#   windows/      (Windows only) DPI/Common-Controls v6 manifest and .rc resource script
#   go/           self-contained Go module `gpuikit.dev` (`go.mod`, `go.sum`, `gk/**`)
#   include/      C ABI headers (`gpuikit.h`, `gk_ops.h`) for non-Go consumers
#   examples/go/  six starter apps plus a standalone `examples/go/build.sh` script
#   docs/         Go guides (`GETTING-STARTED-GO.md`, `API-GO.md`, `GO.md`), tutorial, snippets
#   licences/     `THIRD_PARTY.md` and verbatim third-party crate license texts
#   THIRD_PARTY.md top-level copy of the license summary for visibility
#   README.md     quick-start instructions customized for the target platform
#   VERSIONS      pin versions, `ApiHash`, minimum OS/glibc, dynamic deps, and SHA-256 digests
#
# Archive format choices:
#   - Linux / macOS: `.tar.zst` (fast multi-threaded decompression via `zstd -T0`).
#   - Windows: `.zip` (built with Python's stdlib `zipfile` so no external `zip` binary is needed).
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd); cd "$repo"
root=${1:-lib}
lic=${GK_LICENCES:-$root/licences}
dst=${GK_DIST_OUT:-dist}
py=$(command -v python3 || command -v python)
# Auto-detect the target distribution platform from host OS/arch when GK_PLATFORM is unset.
if [ -z "${GK_PLATFORM:-}" ]; then
  case "$(uname -s)/$(uname -m)" in
    Linux/x86_64) GK_PLATFORM=linux-x86_64 ;;
    Darwin/*) GK_PLATFORM=macos-universal ;;
    MINGW*/x86_64|MSYS*/x86_64|CYGWIN*/x86_64) GK_PLATFORM=windows-x86_64 ;;
    *) echo "dist.sh: no download for $(uname -s) $(uname -m); set GK_PLATFORM" >&2; exit 2 ;;
  esac
fi
# Select per-platform artifacts:
# - `libs`: files copied from `$root/ship/` into `$out/lib/`.
# - `main`: static archive whose SHA-256 is recorded as `library sha256` in VERSIONS.
# - `probe`: shared library inspected for OS/glibc minimums, NEEDED deps, and `shared library sha256`.
# - `ext`: executable suffix shown in generated README examples.
case "$GK_PLATFORM" in
  linux-x86_64) libs=(libgpuikit.so libgpuikit_static.a libgpuikit_sys.so); main=libgpuikit_static.a; probe=libgpuikit.so; ext=.bin ;;
  macos-universal) libs=(libgpuikit.dylib libgpuikit_static.a); main=libgpuikit_static.a; probe=libgpuikit.dylib; ext=.bin ;;
  windows-x86_64) libs=(gpuikit.dll libgpuikit_static.a libgpuikit_sys.a); main=libgpuikit_static.a; probe=gpuikit.dll; ext=.exe ;;
  *) echo "dist.sh: GK_PLATFORM is linux-x86_64, macos-universal or windows-x86_64" >&2; exit 2 ;;
esac
ver=$(grep -m1 '^version' Cargo.toml | cut -d'"' -f2)
name=gpuikit-$ver-$GK_PLATFORM
out=$dst/$name
sha256() { if command -v sha256sum > /dev/null; then sha256sum "$1"; else shasum -a 256 "$1"; fi | cut -c1-64; }
# Pre-flight validation: verify every required library exists, confirm macOS binaries are
# genuinely universal (both arm64 and x86_64 slices), and require THIRD_PARTY.md before staging.
for f in "${libs[@]}"; do
  [ -f "$root/ship/$f" ] || { echo "dist.sh: $root/ship/$f is missing (make lib, or tools/build-lib.sh ship on $GK_PLATFORM)" >&2; exit 1; }
done
if [ "$GK_PLATFORM" = macos-universal ]; then
  for f in libgpuikit_static.a libgpuikit.dylib; do
    got=$(lipo -archs "$root/ship/$f" | awk '{for (i = 1; i <= NF; i++) print $i}' | sort | tr '\n' ' ')
    [ "$got" = "arm64 x86_64 " ] || { echo "dist.sh: $root/ship/$f has the slices '$got', not arm64 and x86_64 (tools/build-lib.sh builds both)" >&2; exit 1; }
  done
fi
[ -f "$lic/THIRD_PARTY.md" ] || { echo "dist.sh: $lic/THIRD_PARTY.md is missing (make lib writes it)" >&2; exit 1; }
rm -rf "${dst:?}/$name" "$dst/$name".*
mkdir -p "$out/lib" "$out/go" "$out/include" "$out/examples/go" "$out/docs"
for f in "${libs[@]}"; do cp "$root/ship/$f" "$out/lib/"; done
# Copy Windows manifest and resource script alongside lib/ for users embedding DPI/Common-Controls metadata.
if [ "$GK_PLATFORM" = windows-x86_64 ]; then
  mkdir -p "$out/windows"; cp core/windows/gpuikit.manifest core/windows/gpuikit.rc "$out/windows/"
fi
cp -r "$lic" "$out/licences"
cp "$lic/THIRD_PARTY.md" "$out/THIRD_PARTY.md"
cp go/go.mod go/go.sum "$out/go/" && cp -r go/gk "$out/go/"
cp core/include/gpuikit.h core/include/gk_ops.h "$out/include/"
cp examples/go/go.mod examples/go/go.sum "$out/examples/go/"
cp -r examples/go/hello examples/go/counter examples/go/notes examples/go/todo examples/go/reader \
  examples/go/expenses "$out/examples/go/"
cp docs/GETTING-STARTED-GO.md docs/API-GO.md docs/GO.md "$out/docs/"
cp -r docs/tutorial-go docs/snippets-go "$out/docs/"
# Write a standalone example build script inside the distribution archive (separate from the
# repo's examples/go/build.sh, because a downloaded archive has no Rust workspace or diag build).
# Modes:
# - Default (GK_STATIC!=1): purego dynamic load with `CGO_ENABLED=0`, symlinking (or copying on
#   Windows) the shared library next to the binary so it starts without setting GK_LIB.
# - Static (GK_STATIC=1): cgo build with `-tags gkstatic` and platform dead-stripping flags
#   (`--gc-sections` on ELF/PE, `-dead_strip` on Mach-O). On Windows, prefers `ld.lld` with
#   `-Xlink=-force:multipleres` so Go's embedded `.syso` manifest coexists with cgo resources
#   and unused component families are actually dead-stripped.
cat > "$out/examples/go/build.sh" <<'B'
set -e
here=$(cd "$(dirname "$0")" && pwd); lib=$(cd "$here/../../lib" && pwd); app=${1:-hello}
case "$(uname -s)" in MINGW*|MSYS*|CYGWIN*) win=1; ext=.exe ;; *) win=; ext=.bin ;; esac
case "${GOOS:-}" in windows) ext=.exe ;; linux|darwin) ext=.bin ;; esac
case "$app" in
  */*) src=$(cd "${app%/}" && pwd); bin=$src$ext ;;
  *) src=$here/$app; bin=$here/$app$ext ;;
esac
cd "$src"
if [ "${GK_STATIC:-}" != 1 ]; then
  CGO_ENABLED=0 go build -ldflags="-s -w" -o "$bin" .
  dir=$(dirname "$bin")
  case "${GOOS:-$(go env GOOS)}" in windows) so=gpuikit.dll ;; darwin) so=libgpuikit.dylib ;; *) so=libgpuikit.so ;; esac
  if [ -f "$lib/$so" ] && [ ! -e "$dir/$so" ]; then
    if [ -n "$win" ]; then cp "$lib/$so" "$dir/"; else ln -s "$lib/$so" "$dir/$so"; fi
  fi
elif [ -n "$win" ]; then
  if command -v ld.lld > /dev/null; then x="-fuse-ld=lld -Wl,--gc-sections,-Xlink=-force:multipleres"; else x="-Wl,--gc-sections"; fi
  CGO_ENABLED=1 CGO_LDFLAGS="-L$(cygpath -m "$lib")" go build -tags gkstatic -ldflags="-s -w '-extldflags=$x'" -o "$(cygpath -m "$bin")" .
elif [ "$(uname -s)" = Darwin ]; then
  CGO_ENABLED=1 CGO_LDFLAGS="-L$lib" go build -tags gkstatic -ldflags="-s -w -extldflags=-Wl,-dead_strip" -o "$bin" .
else
  CGO_ENABLED=1 CGO_LDFLAGS="-L$lib" go build -tags gkstatic -ldflags="-s -w -extldflags=-Wl,--gc-sections" -o "$bin" .
fi
echo "built $bin"
B
chmod +x "$out/examples/go/build.sh"
case "$GK_PLATFORM" in
  linux-x86_64) sys="Linux x86_64"; libdesc="the static library"; so=libgpuikit.so
    go_static='CGO_LDFLAGS="-L<this folder>/lib" go build -tags gkstatic -ldflags="-s -w -extldflags=-Wl,--gc-sections"'
    static_needs="a C compiler for cgo (gcc, or clang with CC=clang)"
    ship_note="" ;;
  macos-universal) sys="macOS, Apple silicon and Intel (universal)"; so=libgpuikit.dylib
    libdesc="the static library, universal (arm64 and x86_64)"
    go_static='CGO_LDFLAGS="-L<this folder>/lib" go build -tags gkstatic -ldflags="-s -w -extldflags=-Wl,-dead_strip"'
    static_needs="the Xcode command-line tools (clang) for cgo"
    ship_note="The library is universal: GOARCH=amd64 examples/go/build.sh APP builds for Intel on Apple silicon (GOARCH=arm64 the other way round), and lipo -create joins the two programs into a universal one. In an app bundle the library goes in Contents/Frameworks." ;;
  windows-x86_64) sys="Windows x86_64"; libdesc="the static library (MinGW's format)"; so=gpuikit.dll
    go_static="CGO_LDFLAGS=\"-L<this folder>/lib\" go build -tags gkstatic -ldflags=\"-s -w '-extldflags=-fuse-ld=lld -Wl,--gc-sections,-Xlink=-force:multipleres'\" (with lld on PATH; without it, -extldflags=-Wl,--gc-sections, and the program holds every component)"
    static_needs="MinGW-w64 gcc and lld on PATH for cgo (MSYS2's ucrt64 with the mingw-w64-ucrt-x86_64-lld package, llvm-mingw, or WinLibs' LLVM build)"
    ship_note="The manifest is linked into the .exe (package gk carries it): without it the program does not start (the library needs common controls 6) and the window is not DPI aware. No C compiler is needed: GOOS=windows go build cross-compiles from any system." ;;
esac
librows=$(for f in "${libs[@]}"; do
  case $f in
    ("$so") echo "| lib/$f | the library, which an app loads at start-up (put it beside the program) |" ;;
    ("$main") echo "| lib/$f | $libdesc, for an app built with -tags gkstatic, which links it into itself and keeps only the component families it uses |" ;;
    (libgpuikit_sys.so) echo "| lib/$f | a linker script that names the system libraries the static library needs |" ;;
    (libgpuikit_sys.a) echo "| lib/$f | the import library of the Windows API the static library links |" ;;
    (*) echo "| lib/$f | an import library of gpuikit.dll |" ;;
  esac
done
[ "$GK_PLATFORM" != windows-x86_64 ] || echo "| windows/ | the manifest a program embeds, and the resource script that embeds it |")
kit=$("$py" -c "import json;print(json.load(open('api/coverage.json', encoding='utf-8'))['pin']['gpui-kit'])")
cat > "$out/README.md" <<R

Desktop apps in **Go** with GPUI-kit $kit: GPUI's renderer and the kit's components, from one
prebuilt library behind a C interface. No Rust is needed.

Start with **docs/GETTING-STARTED-GO.md**: what to install, building the examples, and a
tutorial. Then **docs/API-GO.md**.
Needs Go 1.27 or newer, and no C compiler: an app loads lib/$so when it starts.

    examples/go/build.sh hello && examples/go/hello$ext
    examples/go/build.sh ../myapp                    builds any app folder into ../myapp$ext

An app's go.mod requires the module in go/ and points at it:

    require gpuikit.dev v0.0.0
    replace gpuikit.dev => ../$name/go

then \`go mod tidy\` (it fetches purego, the one module the package needs) and \`go build\`.
The program looks for $so in GK_LIB (the folder that holds it), beside itself, in its lib/ or
../lib/ folder, then where the system loader looks; ship it beside the program. $ship_note

To link the library into the program instead (one file, only the component families it uses;
needs $static_needs): $go_static, or GK_STATIC=1 examples/go/build.sh.

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
# Inspect the built shared library to populate `VERSIONS` with facts verified by
# `tools/release-assets.sh`:
# - Linux: highest `GLIBC_*` symbol version from dynamic symbols plus non-libc NEEDED SONAMEs.
# - macOS: per-slice `LC_BUILD_VERSION` `minos` (recording the maximum across slices) and
#   non-@rpath linked frameworks/dylibs from `otool -L`.
# - Windows: imported system DLLs via `objdump -p` / `llvm-objdump -p` when available.
# - Interface hash (`ApiHash` from `go/gk/internal/rt/gen_ops.go`) and SHA-256 digests of
#   both the static and shared libraries (checked again by release-assets.sh before upload).
lf=$root/ship/$probe
case "$GK_PLATFORM" in
  linux-x86_64)
    needs="glibc $(objdump -T "$lf" | grep -o 'GLIBC_[0-9.]*' | sort -V | tail -1 | cut -d_ -f2) or newer (the newest symbol version the library refers to; it is linked against glibc 2.28), and $(objdump -p "$lf" | awk '/NEEDED/{print $2}' | grep -v 'libc.so\|libm.so\|ld-linux' | tr '\n' ' ')" ;;
  macos-universal)
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
hash=$(grep -m1 'const ApiHash' go/gk/internal/rt/gen_ops.go | grep -o '0x[0-9a-f]*')
go_v=$(go version 2>/dev/null | awk '{print $3}' | sed 's/^go//' || true)
cat > "$out/VERSIONS" <<V
gpuikit $ver for $GK_PLATFORM
library: release, gpui-kit $("$py" -c "import json;p=json.load(open('api/coverage.json', encoding='utf-8'))['pin'];print(p['gpui-kit'], 'features', ' '.join(f for f in p['features'] if f != 'gpui-fast'))"), on $engine
interface hash: $hash (the library and the package refuse each other when it differs)
needs: $needs${arch_line:-}
licences: THIRD_PARTY.md (every crate linked into the library, its licence; texts in licences/texts)
library sha256: $(sha256 "$out/lib/$main")
shared library sha256: $(sha256 "$out/lib/$probe")
source commit: $(git rev-parse --short HEAD 2>/dev/null || echo "${GK_COMMIT:-unknown} (a copy without git)")$(git diff --quiet 2>/dev/null || [ -n "${GK_COMMIT:-}" ] || echo ' with uncommitted changes')
tested with: Go ${go_v:-not found}
V
# Pack the staged directory into a zip (Windows) or zstd tarball (Linux/macOS) and print a size manifest.
if [ "$GK_PLATFORM" = windows-x86_64 ]; then
  ( cd "$dst" && "$py" -m zipfile -c "$name.zip" "$name" )
  arc=$name.zip
else
  ( cd "$dst" && tar -cf - "$name" | zstd -q -T0 -o "$name.tar.zst" )
  arc=$name.tar.zst
fi
echo "dist: $out"
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
    if rel.startswith("go/gk/") and os.path.basename(rel).startswith("gen_"): gen_go += size; n_go += 1
    elif rel.startswith("licences/texts/"): texts += size; n_texts += 1
    else: listed.append((rel, size))
for rel, size in sorted(listed): print(f"{size:10d}  ./{rel}")
crates = len([d for d in os.listdir(os.path.join(out, "licences", "texts")) if d != "_canonical"])
print(f"{texts:10d}  ./licences/texts/ ({n_texts} licence files of {crates} crates)")
print(f"{gen_go:10d}  ./go/gk/**/gen_*.go ({n_go} generated files)")
total = sum(s for _, s in files(out))
print(f"unpacked: {mb(total)}; download: {mb(os.path.getsize(os.path.join(dst, arc)))} ({arc})")
P
