#!/bin/bash
# conformance-probe.sh: find the generated conformance pages whose Rust reference cannot be
# drawn on its own. Runs ref-gallery once per page of ref/gallery/conf.txt (GALLERY_START=id,
# two offscreen frames, about 0.15 s each) and prints, for every page that does not exit 0,
# a line for gates/conformance-skip.txt: "id | panics in the reference: MESSAGE". Such a
# page shows an element that only works inside a parent or with state the page does not
# make (a ResizablePanel outside its group); it needs a hand-written page instead.
#   gates/conformance-probe.sh >> gates/conformance-skip.txt
# then gates/conformance.py and a rebuild of the reference.
here=$(cd "$(dirname "$0")" && pwd); repo=$(dirname "$here")
lib=${GK_LIB_ROOT:-$repo/lib}/diag
ref=${GK_REF:-$lib}/ref-gallery; [ -x "$ref" ] || ref=$lib/ref-gallery
export LD_LIBRARY_PATH=$lib
unset WAYLAND_DISPLAY DISPLAY
tmp=$(mktemp -d "${GK_SCRATCH:-${TMPDIR:-/tmp}/gk-gates}/probe.XXXXXX"); trap 'rm -rf "$tmp"' EXIT
printf 'size 800 600\nframe\nframe\n' > "$tmp/probe.script"
n=0; bad=0
while IFS='|' read -r id _; do
  id=$(echo "$id" | xargs); case "$id" in ''|\#*) continue ;; esac
  n=$((n + 1))
  if ! GALLERY_START=$id timeout 20 "$ref" --script "$tmp/probe.script" > "$tmp/log" 2>&1; then
    bad=$((bad + 1))
    why=$(grep -A1 'panicked at' "$tmp/log" | tail -1 | cut -c1-160)
    echo "$id | panics in the reference: ${why:-no message (exit or timeout)}"
  fi
done < "$repo/ref/gallery/conf.txt"
echo "conformance-probe: $bad of $n pages cannot be drawn alone" >&2
