#!/bin/bash
# extract.sh [JOBS]: stage 1 of the generator, run when the pinned gpui-kit version changes. Asks
# rustdoc for its JSON output (RUSTC_BOOTSTRAP=1: the JSON format is unstable; format 61 with the
# Rust of rust-toolchain.toml) for the crates the binding covers, with exactly the library's
# default feature set (gpui-fast). Two passes: the public API, and the same with
# --document-hidden-items (the #[doc(hidden)] items rustdoc otherwise leaves out).
# Output: gen-json/{public,hidden}/{gpui,gpui_base,gpui_component,gpui_kit}.json in the
# repository, for `python3 gen/gkgen.py strip gen-json/public` (which writes api/rustdoc).
# Uses the checkout's own target/ (about 3 GB) and a few GB of memory; a few minutes.
set -euo pipefail
cd "$(dirname "$0")/.."
export RUSTC_BOOTSTRAP=1
# gk-capi is listed so its default features (gpui-fast) apply to the kit, as in the library.
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
