#!/bin/bash
# pixels.sh [APP [SCRIPT]]: the pixel gate for one app (default counter; also writerule: the
# write-rule case, whose every image must show a per-frame handler's write in the frame it
# was made in). Runs the app offscreen (gpui's
# HeadlessAppContext with the real wgpu renderer; no display, no compositor) as the Rust
# reference and as the Odin app under the same gk-meter script, with the diag library, and
# compares the PNGs byte for byte. Reference and candidate are always rendered in the same
# run, on the same machine and driver. Prints one line starting with "GATE pixels".
#   gates/pixels.sh
here=$(cd "$(dirname "$0")" && pwd); repo=$(dirname "$here")
app=${1:-counter}
script=${2:-$here/$app.script}
out=$here/out/pixels/$app
lib=${GK_LIB_ROOT:-$repo/lib}/diag
ref=${GK_REF:-$lib}/ref-$app
[ -x "$ref" ] || ref=$lib/ref-$app
export LD_LIBRARY_PATH=$lib
unset WAYLAND_DISPLAY DISPLAY
fail=0; same=0; total=0
# GK_PIXEL_CANDS: the front ends compared with Rust (default odin; gates/go.sh passes go).
cands=${GK_PIXEL_CANDS:-odin}
for b in rust $cands; do
  rm -rf "$out/$b"; mkdir -p "$out/$b"; cd "$out/$b"
  case "$b" in
    rust) timeout 120 "$ref" --script "$script" ${GK_PIXEL_ARGS:-} > log.txt 2>&1 ;;
    *) GK_SCRIPT=$script timeout 120 "${GK_BIN:-$repo/bin}/$app-$b" ${GK_PIXEL_ARGS:-} > log.txt 2>&1 ;;
  esac
  rc=$?
  echo "$b: exit $rc, $(ls *.png 2>/dev/null | wc -l) images"
  [ $rc -eq 0 ] || { tail -5 log.txt; fail=1; }
done
cd "$out"
for png in rust/*.png; do
  [ -e "$png" ] || { fail=1; break; }
  n=$(basename "$png")
  for c in $cands; do
    total=$((total + 1))
    if cmp -s "rust/$n" "$c/$n"; then
      same=$((same + 1)); echo "$c $n: identical ($(stat -c %s "$png") bytes, sha256 $(sha256sum "$png" | cut -c1-12))"
    else
      echo "$c $n: DIFFERS"; fail=1
    fi
  done
done
[ $total -gt 0 ] || fail=1
echo "GATE pixels $app${GK_PIXEL_CANDS:+-${cands// /-}} $([ $fail = 0 ] && echo PASS || echo FAIL) $same of $total images byte-identical to Rust"
exit $fail
