#!/bin/bash
# all.sh [quick]: every gate, one after the other, and one summary table at the end
# (gates/out/summary.md). No Rust is built: it needs the library and the Rust reference apps in
# lib/ship and lib/diag (make lib), the Odin compiler and Go. Linux only: the gates draw offscreen
# and in a private display (gates/display.sh).
#
#   quick   the tier to run after every change, under two minutes: the Odin builds, the
#           rebuild, misuse, pixel and gallery gates, the counter click, frame cost at
#           1,000 Hz only (5 rounds per scene), the short memory runs, coverage
#   (none)  the full tier, about 20 minutes: frame cost at 60, 240 and 1,000 Hz with 10
#           rounds per scene, the counter with a click on every frame at 60 Hz, the long
#           memory runs and the memory gate's self-test
#
# Exit 0 only when every gate passed or failed as gates/expected.txt says it does today.
# To judge a library built elsewhere: GK_LIB_ROOT=other-lib GK_BIN=other-bin gates/all.sh quick
# (GK_REF=dir names where the Rust references are, when not in $GK_LIB_ROOT/diag). The run
# works on a copy of the library taken at its start, so rebuilding lib/ meanwhile does not
# disturb it.
here=$(cd "$(dirname "$0")" && pwd); repo=$(dirname "$here")
tier=full; rounds=10; rates="60 240 1000"; mem=""
[ "${1:-}" = quick ] && { tier=quick; rounds=5; rates="1000"; mem=--quick; }
# GK_RATES overrides the refresh rates of the frame-cost gate (a machine whose display paces
# at 60 Hz only, gates/display.sh with GK_DISPLAY=xvfb).
rates=${GK_RATES:-$rates}
export GK_LIB_ROOT=${GK_LIB_ROOT:-$repo/lib} GK_BIN=${GK_BIN:-$repo/bin}
log=$here/out/all.log; mkdir -p "$here/out"; : > "$log"; : > "$here/out/times.txt"
# The run judges one library: a copy taken now, so a rebuild of lib/ during the run cannot mix
# two libraries. The copy (about 0.5 GB) is deleted at the end.
rm -rf "$here/out/fps"   # only this run's scenes are in the table gates/status.py writes
snap=$here/out/lib-snapshot; rm -rf "$snap"; mkdir -p "$snap"
cp -a --reflink=auto "$GK_LIB_ROOT/ship" "$GK_LIB_ROOT/diag" "$snap/" || { echo "all.sh: $GK_LIB_ROOT has no ship and diag library (make lib)"; exit 1; }
if [ -n "${GK_REF:-}" ]; then cp -a "$GK_REF"/ref-* "$snap/diag/"; unset GK_REF; fi
trap 'rm -rf "$snap"' EXIT
from=$GK_LIB_ROOT
export GK_LIB_ROOT=$snap GK_LIB=$snap/ship OUT=$GK_BIN
began=$(date +%s.%N)
step() { # name command...
  local name=$1 t=$(date +%s.%N); shift
  echo "== $name: $*" | tee -a "$log"
  "$@" 2>&1 | tee -a "$log"
  echo "$name $(echo "$(date +%s.%N) - $t" | bc)" >> "$here/out/times.txt"
}
echo "machine: $(free -g | awk 'NR==2{print "available " $7 " GB"}'), load$(uptime | sed 's/.*load average://'); tier $tier" | tee -a "$log"
echo "judged: library $from/ship/libgpuikit.so of $(date -r "$from/ship/libgpuikit.so" '+%-d %b %H:%M') (sha256 $(sha256sum "$snap/ship/libgpuikit.so" | cut -c1-12)), copied at $(date +%H:%M); Odin package and examples at commit $(git -C "$repo" rev-parse --short HEAD 2>/dev/null || echo "${GK_COMMIT:-unknown} (a copy without git)")$(git -C "$repo" diff --quiet -- odin examples 2>/dev/null || [ -n "${GK_COMMIT:-}" ] || echo ' with uncommitted changes')" | tee -a "$log"
step build bash -c '"$0" panels counter negative negative_rows negative_data negative_overlay negative_values negative_lists2 negative_review3 negative_editor gallery writerule table && OPT=-o:speed SUFFIX=-speed "$0" panels counter gallery table; "$0" notes; "$0" files; "$0" paint' "$repo/examples/odin/build.sh"
step rebuild "$here/rebuild.sh"
step misuse "$here/misuse.sh"
step proto "$repo/core/tests/run.sh"
step gensample "$here/gensample.sh"
step pixels bash -c '"$0" counter; "$0" writerule; "$0" notes; "$0" files; "$0" paint' "$here/pixels.sh"
step todo "$here/todo.sh"
step reader "$here/reader.sh"
step docs "$here/docs.sh"
step conformance python3 "$here/conformance.py" --check
# GK_GALLERY_RECORD=1: the gallery gate records gates/verified.json from a passing run (--record).
step gallery "$here/gallery.sh" ${GK_GALLERY_RECORD:+--record}
step counter "$here/counter.sh"
step readme "$here/readme.sh"
for hz in $rates; do
  step "fps-$hz" bash -c '
    "$0" --hz "$1" --rounds "$2" --zero-crossings panels 60 64 0
    "$0" --hz "$1" --rounds "$2" panels 60 64 2' "$here/fps.sh" "$hz" "$rounds"
done
# A click on every frame: one EVENT turn and one re-described view per frame, against the same
# listener in Rust. At 60 Hz, where the meter's click lands between two frames. Twice the
# rounds of the other scenes: its frames are the shortest, so the machine's noise is the
# largest share of them.
[ $tier = full ] && step fps-click env GK_CLICK=40,32,61,1 GK_NAME=counter-click-every-frame \
  "$here/fps.sh" --hz 60 --rounds "$((rounds * 2))" counter
# A table of 100,000 rows scrolled by 24 px before every frame (the rows family, PROTOCOLS.md
# 9.8): the kit's DataTable over a Rust delegate against the same table whose cells are views
# over a row source, bound ahead in batches. At the highest rate this machine paces.
step fps-table env GK_SCROLL=450,300,-24 GK_NAME=table-scroll \
  "$here/fps.sh" --hz "${rates##* }" --rounds "$rounds" table
# The Go front end (docs/GO.md): its builds, the README's app, the protocol tests and misuse
# cases in Go, counter and panels pixels against Rust, the edit loop; frame cost in the full tier
# (judged at 1.15 x Rust).
step go "$here/go.sh" build hello proto misuse pixels rebuild $([ $tier = full ] && echo fps)
# The Go apps and the tutorial against their Odin twins (gates/go-apps.sh).
step go-apps "$here/go-apps.sh"
# The same Go apps built the way an app is: the static library, each linking only the
# component families it calls into (docs/GO.md section 8); the scripted sessions find a missing one.
step go-apps-static env GK_STATIC=1 "$here/go-apps.sh" build notes todo reader expenses tutorial
step memory "$here/memory.sh" $mem
[ $tier = full ] && step memory-selftest "$here/memory.sh" --selftest
step coverage "$here/coverage.sh"
echo "all $(echo "$(date +%s.%N) - $began" | bc)" >> "$here/out/times.txt"
python3 "$here/summary.py" "$tier" "$log" "$here/expected.txt" "$here/out/times.txt" | tee "$here/out/summary.md"
exit "${PIPESTATUS[0]}"
