#!/bin/bash
# Go guides, snippets, API index, and tutorial gate (`gates/docs-go.sh`).
#
# What it gates:
#   Accuracy and runnability of `docs/GETTING-STARTED-GO.md`, `docs/API-GO.md`, and `docs/GO.md`,
#   every tutorial step in `docs/tutorial-go/*/`, every snippet program in `docs/snippets-go/*/`,
#   and every external example cited with `<!-- from: PATH -->`.
#
# How it measures:
#   1. Runs `gates/docs-snippets.py` to confirm every fenced `go` block and inline API call span
#      exists verbatim in its backing Go source file.
#   2. Runs `gen/gk_apidoc.py` in check mode to ensure the generated component index in `API-GO.md`
#      matches the current Go packages.
#   3. Compiles every tutorial, snippet, and cited program and runs each offscreen on `lib/diag`
#      (60 s timeout) to verify it renders a 520x560 PNG without crashing.
#   4. Drives the final tutorial step through `gates/docs-tutorial.script` (120 s timeout) and
#      verifies every scripted `shot` PNG is produced.
#
# What PASS means (exit 0):
#   All snippets match, the API-GO.md index is current, every program compiles and draws an image,
#   and the final tutorial session saves every expected shot (`GATE docs guides-go PASS`).

here=$(cd "$(dirname "$0")" && pwd); repo=$(dirname "$here")
out=$here/out/docs-go; rm -rf "$out"; mkdir -p "$out"
root=${GK_LIB_ROOT:-$repo/lib}; bin=${GK_BIN:-$repo/bin}; mkdir -p "$bin"
fail() { echo "GATE docs guides-go FAIL $*"; exit 1; }
guides="$repo/docs/GETTING-STARTED-GO.md $repo/docs/API-GO.md $repo/docs/GO.md"
python3 "$here/docs-snippets.py" $guides || fail "a snippet is not in its program"
python3 -c 'import sys; sys.path.insert(0, sys.argv[1]); import gk_apidoc as m
sys.exit(m.write(m.GO_DOC, m.GO_BEGIN, m.go_render(m.go_read_package()), True))' "$repo/gen" \
  || fail "the component index of docs/API-GO.md is stale (python3 gen/gk_apidoc.py)"

# Collect external program directories referenced by `<!-- from: PATH -->` outside docs/.
cited=$(grep -ho '<!-- from: [^ ]* -->' $guides | awk '{print $3}' | grep -v '^docs/' | xargs -r -n1 dirname | sort -u | sed "s|^|$repo/|; s|$|/|")
. "$repo/go/env.sh"
progs=()
for d in "$repo"/docs/tutorial-go/*/ "$repo"/docs/snippets-go/*/ $cited; do
  [ -f "$d/main.go" ] || continue
  name=docs-go-$(basename "$(dirname "$d")")-$(basename "$d")
  if ! (cd "$d" && go build -o "$bin/$name" .) \
      > "$out/$name.build.log" 2>&1; then
    head -20 "$out/$name.build.log"; fail "$d does not build"
  fi
  progs+=("$name")
done
echo "docs-go: ${#progs[@]} programs build"

# Render one offscreen frame from every built program to confirm it starts and paints cleanly.
export LD_LIBRARY_PATH=$root/diag
unset WAYLAND_DISPLAY DISPLAY
n=0
for name in "${progs[@]}"; do
  printf 'size 520 560\nframe\nsleep 500\nframe\nadvance 500\nshot %s.png\n' "$name" > "$out/$name.script"
  ( cd "$out" && GK_SCRIPT=$out/$name.script timeout 60 "$bin/$name" > "$name.log" 2>&1 ) \
    || { tail -5 "$out/$name.log"; fail "$name stops when run"; }
  [ -f "$out/$name.png" ] || fail "$name drew no image"
  n=$((n + 1))
done

# Drive the final tutorial step through the full scripted user session.
want=$(grep -c '^shot ' "$here/docs-tutorial.script")
last=$(printf '%s\n' "${progs[@]}" | grep '^docs-go-tutorial-go-' | sort | tail -1)
s=$out/session; mkdir -p "$s"
( cd "$s" && GK_SCRIPT=$here/docs-tutorial.script timeout 120 "$bin/$last" > log.txt 2>&1 ) \
  || { tail -10 "$s/log.txt"; fail "the tutorial's session stops ($last)"; }
shots=$(ls "$s"/*.png 2>/dev/null | wc -l)
[ "$shots" = "$want" ] || fail "the tutorial's session saved $shots of $want images"
blocks=$(grep -c '^```go' $guides | awk -F: '{n += $2} END {print n}')
echo "GATE docs guides-go PASS $blocks Go blocks in 3 guides found in their programs, ${#progs[@]} programs build and $n run offscreen, the last step's session ($shots images), the API-GO.md index current"
