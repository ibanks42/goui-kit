#!/bin/bash
# build.sh [what...]: build the Odin example apps against the prebuilt library. No Rust is
# involved; this is all an app developer runs. About half a second each.
#   what: panels counter negative gallery writerule (default: panels counter)
# ODIN=the compiler (default: odin on PATH; tools/install-odin.sh fetches one);
# OPT=its flags (default -o:minimal, the build an edit loop uses; the frame-cost gate also
# builds -o:speed with SUFFIX=-speed); GK_LIB=the directory holding libgpuikit.so (default
# lib/ship); OUT=output directory (default bin/); SRC=the directory holding the example
# directories (default: this one). GK_LDFLAGS=extra linker flags.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../.." && pwd)
odin=${ODIN:-odin}
lib=${GK_LIB:-$repo/lib/ship}
out=${OUT:-$repo/bin}
opt=${OPT:--o:minimal}
src=${SRC:-$here}
mkdir -p "$out"
[ $# -gt 0 ] || set -- panels counter
for what in "$@"; do
  extra=()
  # The negative suite needs the guard pages a debug build has; here they are switched on
  # in the ordinary build.
  case "$what" in negative|negative_rows) extra=(-define:GK_GUARD=true) ;; esac
  # GK_ODIN_SHARED (default true): link the shared library, so a test can run the program against
  # lib/ship or lib/diag; false links the static library, as an app does (docs/DESIGN.md section 16).
  "$odin" build "$src/$what" $opt "${extra[@]}" -define:GK_SHARED=${GK_ODIN_SHARED:-true} -collection:gpuikit="$repo/odin" \
    "-extra-linker-flags:-L$lib -Wl,-rpath,\$ORIGIN/../lib/ship ${GK_LDFLAGS:-}" -out:"$out/$what-odin${SUFFIX:-}"
done
