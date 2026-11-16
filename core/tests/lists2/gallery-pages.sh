#!/bin/bash
# gallery-pages.sh [PREFIX]: the gallery gate for one track's pages only (ref/gallery/pages.txt
# lines whose id starts with PREFIX, default "lists2-"), offscreen with the diag library: the Rust
# reference twice and the Odin gallery once, each started at the first of those pages
# (GALLERY_START) and walked through them with the steps of pages.txt; PNGs compared byte for
# byte. Builds bin/gallery-odin first. GK_LIB_ROOT, GK_BIN, ODIN as for gates/gallery.sh.
# Prints one line per page and "GATE gallery-PREFIX PASS|FAIL n of m pages identical".
here=$(cd "$(dirname "$0")/../../.." && pwd)
prefix=${1:-lists2-}
lib=${GK_LIB_ROOT:-$here/lib}/diag
bin=${GK_BIN:-$here/bin}
odin=${ODIN:-odin}
out=$here/gates/out/gallery-$prefix
rm -rf "$out"; mkdir -p "$out" "$bin"
export LD_LIBRARY_PATH=$lib
unset WAYLAND_DISPLAY DISPLAY
"$odin" build "$here/examples/odin/gallery" -o:minimal -collection:gpuikit="$here/odin" \
  "-extra-linker-flags:-L$lib -Wl,-rpath,\$ORIGIN/../lib/diag" -out:"$bin/gallery-odin" || { echo "GATE gallery-$prefix FAIL build"; exit 1; }
grep -E "^$prefix" "$here/ref/gallery/pages.txt" > "$out/pages.txt"
first=$(head -1 "$out/pages.txt" | cut -d'|' -f1 | tr -d ' ')
python3 -c "
import sys; sys.path.insert(0, '$here/gates'); import gallery
gallery.script('$out/pages.txt', 0)" > "$out/script"
fail=0
for name in rust rust2 odin; do
  mkdir -p "$out/$name"
  if [ $name = odin ]; then cmd=(env GK_SCRIPT="$out/script" "$bin/gallery-odin"); else cmd=("$lib/ref-gallery" --script "$out/script"); fi
  (cd "$out/$name" && GALLERY_START=$first timeout 180 "${cmd[@]}" > log.txt 2>&1) || { echo "$name failed"; tail -5 "$out/$name/log.txt"; fail=1; }
done
n=0; ok=0
while IFS='|' read -r id rest; do
  id=$(echo "$id" | tr -d ' '); n=$((n + 1))
  state=identical; bad=""
  for f in "$out"/rust/[0-9][0-9][0-9]-"$id"-[0-9]*.png; do
    b=$(basename "$f")
    cmp -s "$f" "$out/rust2/$b" || state=unstable
    cmp -s "$f" "$out/odin/$b" || { [ $state = identical ] && state=DIFFERS; bad="$bad $b"; }
  done
  ls "$out"/rust/[0-9][0-9][0-9]-"$id"-[0-9]*.png > /dev/null 2>&1 || state=missing
  [ $state = identical ] && ok=$((ok + 1))
  echo "$id: $state$bad"
done < "$out/pages.txt"
echo "GATE gallery-$prefix $([ $ok = $n ] && [ $fail = 0 ] && echo PASS || echo FAIL) $ok of $n pages identical"
[ $ok = $n ] && [ $fail = 0 ]
