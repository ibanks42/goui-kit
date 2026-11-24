#!/bin/bash
# build.sh [what...]: build the Go example apps. No Rust is involved, and by default no C compiler
# either: the programs load the shared library at start-up (docs/GO.md section 8), so run them with
# GK_LIB=lib/ship (or LD_LIBRARY_PATH=lib/ship), as the gates do.
#   what: hello counter panels proto misuse ... (default: hello counter panels)
# GK_LIB=the directory holding the library (default lib/ship; with GK_STATIC=1 its
# libgpuikit_static.a is linked into each program, build tag gkstatic); OUT=output directory
# (default bin/); SUFFIX is added to each binary's name; GOBUILDFLAGS=extra flags for go build.
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
