#!/bin/bash
# Standalone panic probe for generated conformance pages (`gates/conformance-probe.sh`).
#
# What it checks (diagnostic helper for maintaining `gates/conformance-skip.txt`):
#   Some upstream GPUI-kit widgets panic when rendered in isolation without parent state or context.
#   This helper iterates through every page ID in `ref/gallery/conf.txt` and renders two 800x600
#   offscreen frames in `ref-gallery` (`GALLERY_START=<id>`, 20 s timeout) on `lib/diag`.
#
# Output:
#   Prints each failing page to stdout in `gates/conformance-skip.txt` format
#   (`<id> | panics in the reference: <reason>`) and prints a summary count to stderr.

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
