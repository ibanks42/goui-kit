#!/bin/bash
# build.sh [what...]: build the Go example apps against the prebuilt library. No Rust is involved;
# this is all an app developer runs (with Go and a C compiler for cgo).
#   what: hello counter panels proto misuse ... (default: hello counter panels)
# GK_LIB=the directory holding libgpuikit.so (default lib/ship); OUT=output directory (default
# bin/); SUFFIX is added to each binary's name; GOBUILDFLAGS=extra flags for go build.
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
  CGO_LDFLAGS="-L$lib -Wl,-rpath,\$ORIGIN/../lib/ship" go build ${GOBUILDFLAGS:-} -o "$out/$what-go${SUFFIX:-}" "./$what"
done
