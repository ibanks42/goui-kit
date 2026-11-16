#!/bin/bash
# gallery.sh [--record]: the gallery pixel gate. Runs the Rust reference gallery twice and the Odin
# gallery once, offscreen (gpui's HeadlessAppContext with the real wgpu renderer; no display),
# under one generated script that walks every page of ref/gallery/pages.txt and every step of
# each page, and compares the PNGs byte for byte, page by page. Reference and candidate are
# rendered in the same run on the same machine and driver; the second reference run shows
# which pages are comparable at all. Prints one line per page and "GATE gallery pixels ...".
# A page that gates/verified.json lists as identical must still be identical (the ratchet);
# --record rewrites that file from a passing run, after a page was added to the Odin app.
#   gates/gallery.sh
# Needs GK_REF/ref-gallery (default: lib root/diag), bin/gallery-odin and the diag library.
# Results: gates/out/gallery/{rust,rust2,odin}/*.png, result.json.
here=$(cd "$(dirname "$0")" && pwd); repo=$(dirname "$here")
pages=$repo/ref/gallery/pages.txt
out=$here/out/gallery
lib=${GK_LIB_ROOT:-$repo/lib}/diag
ref=${GK_REF:-$lib}/ref-gallery; [ -x "$ref" ] || ref=$lib/ref-gallery
odin=${GK_BIN:-$repo/bin}/gallery-odin
export LD_LIBRARY_PATH=$lib
unset WAYLAND_DISPLAY DISPLAY GALLERY_START
rm -rf "$out"; mkdir -p "$out"
python3 "$here/gallery.py" script "$pages" > "$out/gallery.script"
for x in "$ref" "$odin"; do [ -x "$x" ] || { echo "GATE gallery pixels FAIL $x is missing"; exit 1; }; done
"$odin" --list > "$out/odin-pages.txt"
fail=0
run() { # name command...
  local name=$1; shift
  mkdir -p "$out/$name"
  local start=$(date +%s.%N)
  (cd "$out/$name" && timeout 180 "$@" > log.txt 2>&1); local rc=$?
  echo "$name: exit $rc, $(ls "$out/$name" | grep -c '\.png$') images, $(echo "$(date +%s.%N) - $start" | bc | cut -c1-5) s"
  [ $rc -eq 0 ] || { tail -5 "$out/$name/log.txt"; fail=1; }
}
run rust "$ref" --script "$out/gallery.script"
run rust2 "$ref" --script "$out/gallery.script"
run odin env GK_SCRIPT="$out/gallery.script" "$odin"
python3 "$here/gallery.py" compare "$pages" "$out" "$out/odin-pages.txt" ${1:+"$1"} || fail=1
exit $fail
