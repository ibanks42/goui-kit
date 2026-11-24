#!/bin/bash
# gallery.sh [go|odin] [--record]: the gallery pixel gate. Runs the Rust reference gallery twice and
# the candidate gallery once (go: examples/go/gallery, bin/gallery-go; odin, the default for now:
# examples/odin/gallery, bin/gallery-odin), offscreen (gpui's HeadlessAppContext with the real wgpu
# renderer; no display), under one generated script that walks every page of ref/gallery/pages.txt
# and every step of each page, and compares the PNGs byte for byte, page by page. Reference and
# candidate are rendered in the same run on the same machine and driver; the second reference run
# shows which pages are comparable at all. Prints one line per page and
# "GATE gallery[-go] pixels ...". A page that gates/verified.json lists as identical for that
# front end must still be identical (the ratchet); --record rewrites the front end's entry from a
# passing run, after a page was added to its app. For Go, the pages of
# gates/gallery-go-pending.txt (waiting for a Go binding) are reported as pending, not failed.
#   gates/gallery.sh go        make gallery-go
#   gates/gallery.sh           (Odin) make gallery
# Needs GK_REF/ref-gallery (default: lib root/diag), the diag library and bin/gallery-go (built here
# when missing or older than its sources; GK_GALLERY_BUILD=0 never builds) or bin/gallery-odin.
# Results: gates/out/gallery[-go]/{rust,rust2,go|odin}/*.png, result.json.
here=$(cd "$(dirname "$0")" && pwd); repo=$(dirname "$here")
cand=odin
case "${1:-}" in go|odin) cand=$1; shift ;; esac
pages=$repo/ref/gallery/pages.txt
out=$here/out/gallery; pending=()
[ $cand = go ] && { out=$here/out/gallery-go; pending=(--pending "$here/gallery-go-pending.txt"); }
lib=${GK_LIB_ROOT:-$repo/lib}/diag
ref=${GK_REF:-$lib}/ref-gallery; [ -x "$ref" ] || ref=$lib/ref-gallery
exe=${GK_BIN:-$repo/bin}/gallery-$cand
if [ $cand = go ] && [ "${GK_GALLERY_BUILD:-1}" = 1 ] &&
   { [ ! -x "$exe" ] || [ -n "$(find "$repo/examples/go/gallery" "$repo/go/gk" -newer "$exe" -name '*.go' -print -quit)" ]; }; then
  echo "gallery: building $exe"
  GK_LIB=${GK_LIB_ROOT:-$repo/lib}/ship OUT=$(dirname "$exe") "$repo/examples/go/build.sh" gallery ||
    { echo "GATE gallery-go pixels FAIL the Go gallery does not build"; exit 1; }
fi
export LD_LIBRARY_PATH=$lib
unset WAYLAND_DISPLAY DISPLAY GALLERY_START
rm -rf "$out"; mkdir -p "$out"
python3 "$here/gallery.py" script "$pages" > "$out/gallery.script"
for x in "$ref" "$exe"; do [ -x "$x" ] || { echo "GATE gallery pixels FAIL $x is missing"; exit 1; }; done
"$exe" --list > "$out/$cand-pages.txt"
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
run $cand env GK_SCRIPT="$out/gallery.script" "$exe"
python3 "$here/gallery.py" compare "$pages" "$out" "$out/$cand-pages.txt" --candidate $cand "${pending[@]}" ${1:+"$1"} || fail=1
exit $fail
