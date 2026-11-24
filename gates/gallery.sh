#!/bin/bash
# Gallery pixel-identity gate (`gates/gallery.sh [go] [--record]`).
#
# What it gates:
#   Pixel-for-pixel equivalence of every hand-written gallery page (`ref/gallery/pages.txt`) and
#   generated one-function conformance page (`ref/gallery/conf.txt`) rendered by `gallery-go`
#   against the pure-Rust reference `ref-gallery`.
#
# How it measures and why the comparison is fair:
#   - Automatically rebuilds `bin/gallery-go` when missing or older than Go sources (disable with
#     `GK_GALLERY_BUILD=0`).
#   - Generates a single deterministic offscreen script (`gallery.script`) via `gates/gallery.py`
#     and runs `ref-gallery` twice (`rust` and `rust2`) plus `gallery-go` once on `lib/diag`
#     (display vars cleared, 180 s timeout per run).
#   - Comparing `rust` against `rust2` catches non-deterministic upstream pages before judging Go.
#   - Delegates comparison, `gates/gallery-go-pending.txt` handling, regression checks against
#     `gates/verified.json`, and optional `--record` updates to `gates/gallery.py compare`.
#
# What PASS means (exit 0):
#   All three runs exit 0 and `gallery.py compare` exits 0 (zero differing pages, zero unstable
#   Rust pages, at least one identical page, and zero regressions vs `gates/verified.json`).

here=$(cd "$(dirname "$0")" && pwd); repo=$(dirname "$here")
cand=go
[ "${1:-}" = go ] && shift
pages=$repo/ref/gallery/pages.txt
out=$here/out/gallery-go; pending=(--pending "$here/gallery-go-pending.txt")
lib=${GK_LIB_ROOT:-$repo/lib}/diag
ref=${GK_REF:-$lib}/ref-gallery; [ -x "$ref" ] || ref=$lib/ref-gallery
exe=${GK_BIN:-$repo/bin}/gallery-$cand

# Rebuild gallery-go automatically if missing or stale against Go sources.
if [ "${GK_GALLERY_BUILD:-1}" = 1 ] &&
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
run() {
  local name=$1; shift
  mkdir -p "$out/$name"
  local start=$(date +%s.%N)
  (cd "$out/$name" && timeout 180 "$@" > log.txt 2>&1); local rc=$?
  echo "$name: exit $rc, $(ls "$out/$name" | grep -c '\.png$') images, $(echo "$(date +%s.%N) - $start" | bc | cut -c1-5) s"
  [ $rc -eq 0 ] || { tail -5 "$out/$name/log.txt"; fail=1; }
}

# Run Rust twice to detect non-deterministic pages before comparing against the Go candidate.
run rust "$ref" --script "$out/gallery.script"
run rust2 "$ref" --script "$out/gallery.script"
run $cand env GK_SCRIPT="$out/gallery.script" "$exe"
python3 "$here/gallery.py" compare "$pages" "$out" "$out/$cand-pages.txt" --candidate $cand "${pending[@]}" ${1:+"$1"} || fail=1
exit $fail
