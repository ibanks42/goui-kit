#!/bin/bash
# shellcheck disable=SC2329  # stubs, darwin and macho run through gate()
# cross.sh: the Go front end compiled from Linux for the other platforms, macOS (arm64
# and x86_64, the two slices of its universal library) and Windows x86-64, in under a minute and
# without their SDKs, so a change that breaks them is seen before CI
# (.github/workflows/platforms.yml) builds and runs them there. Runs on Linux
# (make cross); prints GATE lines and exits non-zero on a failure.
#   go     examples/go/hello built with cgo by zig cc for darwin/arm64, darwin/amd64 and
#          windows/amd64, linked against stand-ins of the library made here: for Windows the MinGW
#          import library tools/win-implib.sh makes from the header (the one the Windows download
#          ships), for macOS one universal dylib (an arm64 and an x86_64 slice, minimum macOS 11.0,
#          joined by tools/macho.py as lipo joins the real one) of the seven functions doing
#          nothing (zig carries macOS's C headers and libSystem's stubs), so each Go build takes its
#          own slice of the one file, as on a Mac; each binary is checked to be its architecture
#          alone. Nothing is run; the binaries are thrown away.
# ZIG (default: the pinned zig of tools/zigbuild.sh, downloaded
# once into target/toolchain/). Scratch (zig's caches, the stand-ins) goes to GK_SCRATCH and is deleted.
set -uo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
. "$repo/go/env.sh"
zig=${ZIG:-$("$repo/tools/zigbuild.sh" fetch-zig "$repo/target/toolchain")}
out=$repo/gates/out/cross
rm -rf "$out"; mkdir -p "$out"
base=${GK_SCRATCH:-${TMPDIR:-/tmp}}
mkdir -p "$base" && tmp=$(mktemp -d "$base/cross.XXXXXX") && [ -n "$tmp" ] || { echo "cross.sh: no scratch folder in $base" >&2; exit 2; }
trap 'rm -rf "$tmp"' EXIT
export ZIG_GLOBAL_CACHE_DIR=$tmp/zig-global ZIG_LOCAL_CACHE_DIR=$tmp/zig-local
rc=0
gate() { # NAME CMD...: one check, its output in gates/out/cross/NAME.log
  local name=$1; shift
  if "$@" > "$out/$name.log" 2>&1; then echo "GATE cross $name PASS"
  else echo "GATE cross $name FAIL (gates/out/cross/$name.log)"; tail -15 "$out/$name.log"; rc=1; fi
}

# The stand-ins: Windows' import library from the header, a macOS dylib with the header's functions.
win=$tmp/windows mac=$tmp/macos
mkdir -p "$win" "$mac"
gate implib-windows env ZIG="$zig" "$repo/tools/win-implib.sh" "$win"
# zig's linker looks for gpuikit.lib, not MinGW's libgpuikit.dll.a (gcc's ld takes the latter):
# the same import library under the name zig wants, in the stand-in folder only.
cp "$win/libgpuikit.dll.a" "$win/gpuikit.lib" 2> /dev/null
cat > "$mac/stub.c" <<'C'
#include "gpuikit.h"
const gk_info *gk_info_get(void) { return 0; }
int32_t gk_run(const gk_app *app) { (void)app; return 0; }
int32_t gk_call(const uint8_t *req, size_t len, const uint8_t **res, size_t *res_len) { (void)req; (void)len; (void)res; (void)res_len; return 0; }
int32_t gk_reply(gk_cb *cb, const uint8_t *out, size_t len) { (void)cb; (void)out; (void)len; return 0; }
int32_t gk_post(uint64_t token) { (void)token; return 0; }
const char *gk_last_error(size_t *len) { (void)len; return 0; }
int32_t gk_headless_run(const gk_app *app, const uint8_t *script, size_t len) { (void)app; (void)script; (void)len; return 0; }
C
# Go's runtime/cgo links -lresolv on macOS, which zig's libSystem stubs lack: an empty one.
echo 'int gk_cross_resolv_stub;' > "$mac/resolv.c"
macho() { python3 -I "$repo/tools/macho.py" "$@"; }
stubs() { # each stand-in once per slice (minimum macOS 11.0, as tools/build-lib.sh builds), then universal
  local a
  for a in aarch64 x86_64; do
    "$zig" cc -target "$a-macos.11.0" -shared -I "$repo/core/include" -Wl,-install_name,@rpath/libgpuikit.dylib \
      -o "$mac/gpuikit-$a.dylib" "$mac/stub.c" || return 1
    "$zig" cc -target "$a-macos.11.0" -shared -Wl,-install_name,/usr/lib/libresolv.9.dylib \
      -o "$mac/resolv-$a.dylib" "$mac/resolv.c" || return 1
  done
  macho fat "$mac/libgpuikit.dylib" "$mac/gpuikit-aarch64.dylib" "$mac/gpuikit-x86_64.dylib" &&
    macho fat "$mac/libresolv.dylib" "$mac/resolv-aarch64.dylib" "$mac/resolv-x86_64.dylib" &&
    rm -f "$mac"/gpuikit-*.dylib "$mac"/resolv-*.dylib &&
    macho check "$mac/libgpuikit.dylib" arm64 x86_64 --id @rpath/libgpuikit.dylib --minos 11.0
}
gate stub-macos stubs

cd "$repo/examples/go" || exit 2
gate go-windows env GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC="$zig cc -target x86_64-windows-gnu" \
  CGO_LDFLAGS="-L$win" go build -o "$tmp/hello.exe" ./hello
darwin() { # GOARCH ZIG-ARCH SLICE: hello for one slice, against the one universal stand-in
  env GOOS=darwin GOARCH="$1" CGO_ENABLED=1 CC="$zig cc -target $2-macos" \
    CGO_LDFLAGS="-L$mac -Wl,-rpath,@executable_path" go build -ldflags=-w -o "$tmp/hello-darwin-$1" ./hello &&
    macho check "$tmp/hello-darwin-$1" "$3" --kind executable   # -w: no DWARF, which needs Apple's dsymutil
}
gate go-darwin-arm64 darwin arm64 aarch64 arm64
gate go-darwin-amd64 darwin amd64 x86_64 x86_64
# The whole Go module type-checks for each (go vet runs cgo with the same compilers).
cd "$repo/go" || exit 2
gate vet-windows env GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC="$zig cc -target x86_64-windows-gnu" go vet ./...
gate vet-darwin-arm64 env GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 CC="$zig cc -target aarch64-macos" go vet ./...
gate vet-darwin-amd64 env GOOS=darwin GOARCH=amd64 CGO_ENABLED=1 CC="$zig cc -target x86_64-macos" go vet ./...
exit $rc
