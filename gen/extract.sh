#!/bin/bash
# Extract public and hidden rustdoc JSON for gpui-fast and the gpui-kit crates into gen-json/.
# Runs two passes: `public` (normal rustdoc JSON) and `hidden` (with --document-hidden-items,
# needed by gkgen.py strip's GRAFTS table for hidden structs like Editor).
# Usage: gen/extract.sh [JOBS]
set -euo pipefail
cd "$(dirname "$0")/.."
# RUSTC_BOOTSTRAP=1 enables `-Z unstable-options --output-format json` on the pinned stable toolchain.
export RUSTC_BOOTSTRAP=1
pkgs=(-p gk-capi -p gpui-kit -p gpui-base -p gpui-component -p gpui-fast)
for pass in public hidden; do
  flags="-Z unstable-options --output-format json --cap-lints allow"
  [ "$pass" = hidden ] && flags="$flags --document-hidden-items"
  start=$(date +%s)
  RUSTDOCFLAGS="$flags" cargo doc ${1:+-j "$1"} --no-deps --locked "${pkgs[@]}"
  mkdir -p "gen-json/$pass"
  cp target/doc/gpui.json target/doc/gpui_base.json target/doc/gpui_component.json target/doc/gpui_kit.json "gen-json/$pass/"
  echo "extract: $pass pass $(( $(date +%s) - start )) s"
done
ls -l gen-json/*/
