#!/bin/bash
# docs.sh: the guides cannot rot. docs/GETTING-STARTED.md, docs/API.md (Odin) and
# docs/GETTING-STARTED-GO.md, docs/API-GO.md (Go) show code only as excerpts of programs under docs/
# or examples/ (gates/docs-snippets.py checks every block against its program, and every call an
# API guide names in its text against its snippet program); this builds every one of those programs
# against its package and the ship library (odin build; go build with cgo), runs each
# tutorial step of both languages offscreen (the diag library) for a few frames with a screenshot,
# drives each tutorial's last step through gates/docs-tutorial.script, and checks that the
# component indexes of docs/API.md and docs/API-GO.md are the ones the generated packages give
# (gen/gk_apidoc.py --check). Images in gates/out/docs/. Prints one line "GATE docs guides ...".
#   gates/docs.sh
here=$(cd "$(dirname "$0")" && pwd); repo=$(dirname "$here")
out=$here/out/docs; rm -rf "$out"; mkdir -p "$out"
odin=${ODIN:-odin}
root=${GK_LIB_ROOT:-$repo/lib}; bin=${GK_BIN:-$repo/bin}; mkdir -p "$bin"
fail() { echo "GATE docs guides FAIL $*"; exit 1; }
guides_odin="$repo/docs/GETTING-STARTED.md $repo/docs/API.md"
guides_go="$repo/docs/GETTING-STARTED-GO.md $repo/docs/API-GO.md"
python3 "$here/docs-snippets.py" $guides_odin $guides_go || fail "a snippet is not in its program"
python3 "$repo/gen/gk_apidoc.py" --check || fail "a component index (docs/API.md or docs/API-GO.md) is stale"
# cited GUIDES...: the folders of the programs outside docs/ the guides cut a snippet from
# (examples/odin/expenses, examples/go/expenses)
cited() {
  grep -ho '<!-- from: [^ ]* -->' "$@" | awk '{print $3}' | grep -v '^docs/' | xargs -r -n1 dirname \
    | sort -u | sed "s|^|$repo/|; s|$|/|"
}
progs=()
# every Odin tutorial step and snippet program, and every other program an Odin guide cites
for d in "$repo"/docs/tutorial/*/ "$repo"/docs/snippets/*/ $(cited $guides_odin); do
  [ -f "$d/main.odin" ] || continue
  name=docs-$(basename "$(dirname "$d")")-$(basename "$d")
  if ! "$odin" build "$d" -o:minimal -define:GK_SHARED=true -collection:gpuikit="$repo/odin" \
      "-extra-linker-flags:-L$root/ship" -out:"$bin/$name" > "$out/$name.build.log" 2>&1; then
    head -20 "$out/$name.build.log"; fail "$d does not build"
  fi
  progs+=("$name")
done
nodin=${#progs[@]}
# the same for Go: each folder is a main package inside a module whose go.mod replaces
# gpuikit.dev with ../../go (docs/tutorial-go, docs/snippets-go, examples/go)
. "$repo/go/env.sh"
for d in "$repo"/docs/tutorial-go/*/ "$repo"/docs/snippets-go/*/ $(cited $guides_go); do
  [ -f "$d/main.go" ] || continue
  name=docs-go-$(basename "$(dirname "$d")")-$(basename "$d")
  if ! (cd "$d" && CGO_LDFLAGS="-L$root/ship -Wl,-rpath,$root/ship" go build -o "$bin/$name" .) \
      > "$out/$name.build.log" 2>&1; then
    head -20 "$out/$name.build.log"; fail "$d does not build"
  fi
  progs+=("$name")
done
ngo=$((${#progs[@]} - nodin))
echo "docs: ${#progs[@]} programs build ($nodin Odin, $ngo Go)"
export LD_LIBRARY_PATH=$root/diag
unset WAYLAND_DISPLAY DISPLAY
n=0
for name in "${progs[@]}"; do
  case "$name" in docs-tutorial-*|docs-go-tutorial-go-*) ;; *) continue ;; esac
  printf 'size 520 560\nframe\nsleep 500\nframe\nadvance 500\nshot %s.png\n' "$name" > "$out/$name.script"
  ( cd "$out" && GK_SCRIPT=$out/$name.script timeout 60 "$bin/$name" > "$name.log" 2>&1 ) \
    || { tail -5 "$out/$name.log"; fail "$name stops when run"; }
  [ -f "$out/$name.png" ] || fail "$name drew no image"
  n=$((n + 1))
done
# each tutorial's last step under the scripted session
want=$(grep -c '^shot ' "$here/docs-tutorial.script")
sessions=""
for lang in odin go; do
  pre=docs-tutorial-; [ $lang = go ] && pre=docs-go-tutorial-go-
  last=$(printf '%s\n' "${progs[@]}" | grep "^$pre" | sort | tail -1)
  s=$out/session-$lang; mkdir -p "$s"
  ( cd "$s" && GK_SCRIPT=$here/docs-tutorial.script timeout 120 "$bin/$last" > log.txt 2>&1 ) \
    || { tail -10 "$s/log.txt"; fail "the $lang tutorial's session stops ($last)"; }
  shots=$(ls "$s"/*.png 2>/dev/null | wc -l)
  [ "$shots" = "$want" ] || fail "the $lang tutorial's session saved $shots of $want images"
  sessions="$sessions $lang $shots"
done
echo "GATE docs guides PASS ${#progs[@]} programs build ($nodin Odin, $ngo Go), $n tutorial steps run, the last steps' sessions (images:$sessions), both API indexes current"
