#!/bin/bash
# Build one or more Go example programs into bin/<name>-go.
# Defaults to: hello counter panels.
#
# Environment variables:
#   GK_LIB        Directory containing libgouikit (default: lib/ship).
#   OUT           Output directory for binaries (default: bin).
#   SUFFIX        Optional suffix appended to each output binary name.
#   GK_STATIC=1   Link libgouikit_static.a via cgo (-tags=gkstatic) instead of purego.
#   GOBUILDFLAGS  Extra flags passed to `go build`.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../.." && pwd)
. "$repo/go/env.sh"
lib=${GK_LIB:-$repo/lib/ship}
out=${OUT:-$repo/bin}
mkdir -p "$out"
[ $# -gt 0 ] || set -- hello counter panels
cd "$here"
for what in "$@"; do
  CGO_LDFLAGS="-L$lib" go build ${GOBUILDFLAGS:-} -o "$out/$what-go${SUFFIX:-}" "./$what"
done
