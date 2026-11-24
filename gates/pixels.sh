#!/bin/bash
# Offscreen pixel-identity gate for one app (`gates/pixels.sh [app] [script]`).
#
# What it gates:
#   Byte-for-byte PNG equality of every candidate in `GK_PIXEL_CANDS` (default `go`) against the
#   pure-Rust reference `ref-<app>` (default app: `counter`, default script: `gates/<app>.script`).
#
# How it measures and why the comparison is fair:
#   - Runs `ref-<app>` (`--script`) and each candidate (`GK_SCRIPT`) offscreen against the same
#     `lib/diag` build with `WAYLAND_DISPLAY` and `DISPLAY` unset (120 s timeout each) so both
#     render through the exact same headless renderer, fonts, and scripted input sequence.
#   - Compares every PNG emitted by Rust against each candidate with `cmp -s`, logging byte size
#     and sha256 prefix for matches or `DIFFERS` on any pixel mismatch.
#
# What PASS means (exit 0):
#   Every run exits 0, Rust produces at least one PNG, and every candidate image is byte-identical.

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
cands=${GK_PIXEL_CANDS:-go}
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
