#!/bin/bash
# cross.sh: the Go front end compiled from Linux for the other platforms, macOS (arm64 and
# x86_64, the two slices of its universal library), Windows x86-64 and Linux arm64, in under a
# minute and without their SDKs, so a change that breaks them is seen before CI
# (.github/workflows/platforms.yml) builds and runs them there. Runs on Linux (make cross); prints
# GATE lines and exits non-zero on a failure.
#   go      examples/go/hello built the default way (CGO_ENABLED=0: the library is loaded at
#           start-up through purego, so no C compiler and no library are needed to build) with
#           GOOS=darwin GOARCH=arm64 and amd64, GOOS=windows GOARCH=amd64 and GOOS=linux
#           GOARCH=arm64; each binary is checked to be its platform and architecture alone.
#           Nothing is run; the binaries are thrown away.
#   vet     the whole Go module type-checked for each, the default way (CGO_ENABLED=0) and the
#           static way (-tags gkstatic, cgo run by zig cc for the target: go vet compiles the cgo
#           files but links nothing, so no library is needed)
# ZIG (default: the pinned zig of tools/zigbuild.sh, downloaded once into target/toolchain/).
# Scratch (zig's caches, the binaries) goes to GK_SCRATCH and is deleted.
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
macho() { python3 -I "$repo/tools/macho.py" "$@"; }
# kind FILE pe-amd64|elf-arm64: the binary's format and machine, read from its header.
kind() {
  python3 -I - "$1" "$2" <<'P'
import struct, sys
d = open(sys.argv[1], "rb").read(4096); want = sys.argv[2]
if want == "pe-amd64":
    pe = struct.unpack_from("<I", d, 0x3C)[0]
    ok = d[:2] == b"MZ" and d[pe:pe + 4] == b"PE\0\0" and struct.unpack_from("<H", d, pe + 4)[0] == 0x8664
else:
    ok = d[:4] == b"\x7fELF" and d[4] == 2 and struct.unpack_from("<H", d, 18)[0] == 183
print(sys.argv[1], want, "ok" if ok else "WRONG")
sys.exit(0 if ok else 1)
P
}

cd "$repo/examples/go" || exit 2
build() { # GOOS GOARCH: hello for one platform, without cgo
  env GOOS="$1" GOARCH="$2" CGO_ENABLED=0 go build -ldflags=-w -o "$tmp/hello-$1-$2" ./hello
}
windows() { build windows amd64 && kind "$tmp/hello-windows-amd64" pe-amd64; }
darwin() { build darwin "$1" && macho check "$tmp/hello-darwin-$1" "$2" --kind executable; }
linux_arm64() { build linux arm64 && kind "$tmp/hello-linux-arm64" elf-arm64; }
gate go-windows-amd64 windows
gate go-darwin-arm64 darwin arm64 arm64
gate go-darwin-amd64 darwin amd64 x86_64
gate go-linux-arm64 linux_arm64
# The whole Go module type-checks for each, both ways.
cd "$repo/go" || exit 2
gate vet-windows env GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go vet ./...
gate vet-darwin-arm64 env GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go vet ./...
gate vet-darwin-amd64 env GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go vet ./...
gate vet-static-windows env GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC="$zig cc -target x86_64-windows-gnu" go vet -tags gkstatic ./...
gate vet-static-darwin-arm64 env GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 CC="$zig cc -target aarch64-macos" go vet -tags gkstatic ./...
gate vet-static-darwin-amd64 env GOOS=darwin GOARCH=amd64 CGO_ENABLED=1 CC="$zig cc -target x86_64-macos" go vet -tags gkstatic ./...
exit $rc
