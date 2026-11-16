#!/bin/bash
# rebuild.sh [N]: the edit-loop gate. Copies the counter example to scratch, changes one line
# (the button's label), rebuilds against the prebuilt shared library, N times (default 5),
# and prints each wall time. No Rust is involved and no tracked file is touched.
#
# Pass: the median is under 0.60 s on the reference machine (DESIGN.md section 14), the one the
# requirement was measured on. On any other machine the limit is scaled by that machine's speed,
# measured in the same run: milestone 1's package (commit 3a8f188, 0.30 s on the reference
# machine) is
# rebuilt the same way, alternating with this one, and the limit is 0.60 s x (its median here
# / 0.30 s), never below 0.60 s. That is the same as: at most twice milestone 1's rebuild
# time on the same machine. The baseline package is GK_REBUILD_BASE (a directory holding
# odin/gk and examples/odin/counter of that commit), or taken from git when this is a checkout.
# GK_REBUILD_BREAKDOWN=1: after the verdict, one more edit of each package built with the
# compiler's -show-timings, its table printed as "rebuild-timings WHICH | ..." (parse, type
# check, code generation, link), with the size of each package (files, lines, bytes of
# odin/gk, and of its generated gen_*.odin files) and of the library it links against (bytes,
# dynamic symbols): where the time of an edit goes. Not part of the verdict.
#   gates/rebuild.sh
set -euo pipefail
n=${1:-5}
here=$(cd "$(dirname "$0")" && pwd); repo=$(dirname "$here")
base=${GK_SCRATCH:-${TMPDIR:-/tmp}/gk-gates}; mkdir -p "$base"
tmp=$(mktemp -d "$base/rebuild.XXXXXX")
trap 'rm -rf "$tmp"; rmdir "$base" 2>/dev/null || true' EXIT
M1=3a8f188 M1_REF=0.30 LIMIT=0.60
odin=${ODIN:-odin}
lib=${GK_LIB:-$repo/lib/ship}
mkdir -p "$tmp/now" "$tmp/m1"
cp -r "$repo/examples/odin/counter" "$tmp/now/counter"
ln -s "$repo/odin" "$tmp/now/odin"
if [ -n "${GK_REBUILD_BASE:-}" ]; then
  cp -r "$GK_REBUILD_BASE/odin" "$GK_REBUILD_BASE/examples/odin/counter" "$tmp/m1/"
elif git -C "$repo" rev-parse --verify -q "$M1^{commit}" >/dev/null; then
  git -C "$repo" archive "$M1" odin/gk examples/odin/counter | tar -x -C "$tmp/m1"
  mv "$tmp/m1/examples/odin/counter" "$tmp/m1/counter"
else
  echo "rebuild.sh: no baseline package (GK_REBUILD_BASE or a git checkout with $M1)"; exit 2
fi
build() { # which(now|m1) i
  sed -i "s/\"Add[0-9]*\"/\"Add$2\"/" "$tmp/$1/counter/main.odin"
  "$odin" build "$tmp/$1/counter" -o:minimal -collection:gpuikit="$tmp/$1/odin" \
    "-extra-linker-flags:-L$lib -Wl,-rpath,\$ORIGIN/../lib/ship" -out:"$tmp/$1/counter.bin"
}
build now 0; build m1 0   # warm: the first build is not an edit
now=(); m1=()
for i in $(seq "$n"); do
  for w in now m1; do
    start=$(date +%s.%N); build "$w" "$i"; t=$(echo "$(date +%s.%N) - $start" | bc)
    if [ "$w" = now ]; then now+=("$t"); echo "rebuild odin edit=$i seconds=$t"; else m1+=("$t"); echo "rebuild milestone-1 edit=$i seconds=$t"; fi
  done
done
med() { printf '%s\n' "$@" | sort -n | sed -n "$(( ($# + 1) / 2 ))p"; }
a=$(med "${now[@]}"); b=$(med "${m1[@]}")
limit=$(echo "l = $LIMIT * $b / $M1_REF; if (l < $LIMIT) l = $LIMIT; l" | bc -l)
verdict=FAIL; [ "$(echo "$a < $limit" | bc)" = 1 ] && verdict=PASS
printf 'GATE rebuild odin %s median %.2f s after a one-line edit (limit %.2f s: 0.60 s on the reference machine, scaled by milestone 1'"'"'s package, %.2f s here against %.2f s there; %.2f x milestone 1)\n' \
  "$verdict" "$a" "$limit" "$b" "$M1_REF" "$(echo "$a / $b" | bc -l)"
if [ "${GK_REBUILD_BREAKDOWN:-}" = 1 ]; then
  for w in now m1; do
    pkg=$tmp/$w/odin/gk
    printf 'rebuild-size %s package files=%s lines=%s bytes=%s generated_lines=%s app_lines=%s\n' "$w" \
      "$(ls "$pkg"/*.odin | wc -l)" "$(cat "$pkg"/*.odin | wc -l)" "$(cat "$pkg"/*.odin | wc -c)" \
      "$(cat "$pkg"/gen_*.odin 2>/dev/null | wc -l)" "$(cat "$tmp/$w/counter"/*.odin | wc -l)"
    sed -i "s/\"Add[0-9]*\"/\"Add99\"/" "$tmp/$w/counter/main.odin"
    "$odin" build "$tmp/$w/counter" -o:minimal -collection:gpuikit="$tmp/$w/odin" -show-timings \
      "-extra-linker-flags:-L$lib -Wl,-rpath,\$ORIGIN/../lib/ship" -out:"$tmp/$w/counter.bin" 2>&1 |
      sed "s/^/rebuild-timings $w | /"
  done
  printf 'rebuild-size library bytes=%s dynamic_symbols=%s\n' "$(stat -c %s "$lib/libgpuikit.so")" \
    "$(nm -D --defined-only "$lib/libgpuikit.so" 2>/dev/null | wc -l)"
fi
[ $verdict = PASS ]
