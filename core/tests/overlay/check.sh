#!/bin/bash
# check.sh: type-check the track's Odin (the package, the overlay test, the gallery, negative_overlay).
here=$(cd "$(dirname "$0")/../../.." && pwd)
for d in core/tests/overlay examples/odin/gallery examples/odin/negative_overlay; do
  echo "== $d"; "${ODIN:-odin}" check "$here/$d" -collection:gpuikit="$here/odin" 2>&1 | head -40
done
