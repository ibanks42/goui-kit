#!/bin/bash
# Cross-compilation and `go vet` gate from Linux (`gates/cross.sh`).
#
# What it gates:
#   Verifies that the Go front end stays portable to Windows (amd64), macOS (arm64 and amd64),
#   and Linux (arm64) without needing target hardware:
#   - 4 executable builds of `examples/go/hello` with `CGO_ENABLED=0` (purego dynamic loader path).
#   - 3 `go vet ./...` passes on `go/` with `CGO_ENABLED=0` for Windows and macOS.
#   - 3 `go vet -tags gkstatic ./...` passes on `go/` with `CGO_ENABLED=1` using `zig cc` as the
#     cross C compiler so cgo headers and link directives are checked on non-Linux targets too.
#
# How it verifies outputs:
#   Checks executable magic and architecture headers directly (PE x86_64 machine 0x8664, Mach-O
#   arm64/x86_64 via `tools/macho.py`, 64-bit ELF AArch64 machine 183) inside a scratch directory.
#
# What PASS means (exit 0):
#   All 10 `GATE cross <target>` checks succeed; failures save logs under `gates/out/cross/`.

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
gate() {
  local name=$1; shift
  if "$@" > "$out/$name.log" 2>&1; then echo "GATE cross $name PASS"
  else echo "GATE cross $name FAIL (gates/out/cross/$name.log)"; tail -15 "$out/$name.log"; rc=1; fi
}
macho() { python3 -I "$repo/tools/macho.py" "$@"; }

# Verify PE (x86_64 = 0x8664) or 64-bit ELF (AArch64 e_machine = 183) binary magic and architecture.
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
build() {
  env GOOS="$1" GOARCH="$2" CGO_ENABLED=0 go build -ldflags=-w -o "$tmp/hello-$1-$2" ./hello
}
windows() { build windows amd64 && kind "$tmp/hello-windows-amd64" pe-amd64; }
darwin() { build darwin "$1" && macho check "$tmp/hello-darwin-$1" "$2" --kind executable; }
linux_arm64() { build linux arm64 && kind "$tmp/hello-linux-arm64" elf-arm64; }
gate go-windows-amd64 windows
gate go-darwin-arm64 darwin arm64 arm64
gate go-darwin-amd64 darwin amd64 x86_64
gate go-linux-arm64 linux_arm64

# Vet go/ for purego (CGO_ENABLED=0) and static cgo (-tags gkstatic using zig cc as the cross C compiler).
cd "$repo/go" || exit 2
gate vet-windows env GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go vet ./...
gate vet-darwin-arm64 env GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go vet ./...
gate vet-darwin-amd64 env GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go vet ./...
gate vet-static-windows env GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC="$zig cc -target x86_64-windows-gnu" go vet -tags gkstatic ./...
gate vet-static-darwin-arm64 env GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 CC="$zig cc -target aarch64-macos" go vet -tags gkstatic ./...
gate vet-static-darwin-amd64 env GOOS=darwin GOARCH=amd64 CGO_ENABLED=1 CC="$zig cc -target x86_64-macos" go vet -tags gkstatic ./...
exit $rc
