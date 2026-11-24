#!/bin/bash
# docs-go.sh: the Go guides cannot rot. docs/GETTING-STARTED-GO.md, docs/API-GO.md and
# docs/GO.md show code only as excerpts of programs under docs/ or examples/ (gates/docs-snippets.py checks every block against its program,
# and every call docs/API-GO.md names in its text against its snippet program); README.md's Go
# block is gates/readme-go.sh's. This builds every program those guides cut from (every folder of
# docs/tutorial-go and docs/snippets-go, and the examples they cite) with go build against the
# package (the default build, which loads the library at start-up), runs each of them offscreen (the diag library) for a few frames with a screenshot,
# drives the tutorial's last step through gates/docs-tutorial.script, and checks that the
# component index of docs/API-GO.md is the one the generated package gives (gen/gk_apidoc.py).
# Images in gates/out/docs-go/. Prints one line "GATE docs guides-go ...".
#   gates/docs-go.sh
here=$(cd "$(dirname "$0")" && pwd); repo=$(dirname "$here")
out=$here/out/docs-go; rm -rf "$out"; mkdir -p "$out"
root=${GK_LIB_ROOT:-$repo/lib}; bin=${GK_BIN:-$repo/bin}; mkdir -p "$bin"
fail() { echo "GATE docs guides-go FAIL $*"; exit 1; }
guides="$repo/docs/GETTING-STARTED-GO.md $repo/docs/API-GO.md $repo/docs/GO.md"
python3 "$here/docs-snippets.py" $guides || fail "a snippet is not in its program"
python3 -c 'import sys; sys.path.insert(0, sys.argv[1]); import gk_apidoc as m
sys.exit(m.write(m.GO_DOC, m.GO_BEGIN, m.go_render(m.go_read_package()), True))' "$repo/gen" \
  || fail "the component index of docs/API-GO.md is stale (python3 gen/gk_apidoc.py)"
# Every ```go block of a guide follows a <!-- from: --> line (docs-snippets.py); the programs
# outside docs/ that one cites (examples/go/expenses).
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
export LD_LIBRARY_PATH=$root/diag
unset WAYLAND_DISPLAY DISPLAY
# Every program runs: a few frames offscreen and an image (the tutorial's steps, the snippet
# programs, the cited examples alike).
n=0
for name in "${progs[@]}"; do
  printf 'size 520 560\nframe\nsleep 500\nframe\nadvance 500\nshot %s.png\n' "$name" > "$out/$name.script"
  ( cd "$out" && GK_SCRIPT=$out/$name.script timeout 60 "$bin/$name" > "$name.log" 2>&1 ) \
    || { tail -5 "$out/$name.log"; fail "$name stops when run"; }
  [ -f "$out/$name.png" ] || fail "$name drew no image"
  n=$((n + 1))
done
# the tutorial's last step under the scripted session
want=$(grep -c '^shot ' "$here/docs-tutorial.script")
last=$(printf '%s\n' "${progs[@]}" | grep '^docs-go-tutorial-go-' | sort | tail -1)
s=$out/session; mkdir -p "$s"
( cd "$s" && GK_SCRIPT=$here/docs-tutorial.script timeout 120 "$bin/$last" > log.txt 2>&1 ) \
  || { tail -10 "$s/log.txt"; fail "the tutorial's session stops ($last)"; }
shots=$(ls "$s"/*.png 2>/dev/null | wc -l)
[ "$shots" = "$want" ] || fail "the tutorial's session saved $shots of $want images"
blocks=$(grep -c '^```go' $guides | awk -F: '{n += $2} END {print n}')
echo "GATE docs guides-go PASS $blocks Go blocks in 3 guides found in their programs, ${#progs[@]} programs build and $n run offscreen, the last step's session ($shots images), the API-GO.md index current"
