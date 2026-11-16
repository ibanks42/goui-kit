#!/bin/bash
# run.sh [MODE...]: the runtime's own tests, one per protocol of docs/PROTOCOLS.md. Each runs
# the test app core/tests/proto offscreen (the diag library; no display, no compositor) in
# one mode and checks its exit code and the lines it printed. Prints one line per check and
# "GATE proto ... PASS|FAIL n of m" at the end; exit 0 only if every check passed.
#   core/tests/run.sh            (builds the test app first: Odin only)
# GK_LIB_ROOT=the directory holding diag/libgpuikit.so (default lib); GK_BIN=where the test
# binaries go (default bin); ODIN=the compiler. Logs: gates/out/proto/MODE.log.
here=$(cd "$(dirname "$0")" && pwd); repo=$(cd "$here/../.." && pwd)
odin=${ODIN:-odin}
lib=${GK_LIB_ROOT:-$repo/lib}/diag
bin=${GK_BIN:-$repo/bin}
out=$repo/gates/out/proto
rm -rf "$out"; mkdir -p "$out" "$bin"
export LD_LIBRARY_PATH=$lib
unset WAYLAND_DISPLAY DISPLAY

build() { # output-name [source-dir] extra-flags...
  name=$1; shift
  dir=$here/proto
  if [ $# -gt 0 ] && [ -d "$1" ]; then dir=$1; shift; fi
  "$odin" build "$dir" -o:minimal -define:GK_GUARD=true "$@" -define:GK_SHARED=true -collection:gpuikit="$repo/odin" \
    "-extra-linker-flags:-L$lib -Wl,-rpath,\$ORIGIN/../lib/diag" -out:"$bin/$name" || exit 2
}
build proto-odin
# The same app built against a different interface hash: the binding's half of the version check.
build proto-odin-stale -define:GK_TEST_API_HASH=1

pass=0; total=0
# check MODE EXIT PATTERN...: every pattern (grep -E) must appear in the mode's output.
check() {
  mode=$1; want=$2; shift 2
  log=$out/$mode.log
  if [ ! -e "$log" ]; then
    GK_PROTO=$mode timeout 60 "$bin/${GK_PROTO_BIN:-proto-odin}" > "$log" 2>&1
    echo $? > "$out/$mode.rc"
  fi
  rc=$(cat "$out/$mode.rc")
  total=$((total + 1))
  if [ "$rc" != "$want" ]; then
    echo "FAIL $mode: exit $rc, wanted $want"; tail -5 "$log"; return
  fi
  for pattern in "$@"; do
    if ! grep -Eq -- "$pattern" "$log"; then
      echo "FAIL $mode: exit $rc, but no line matches /$pattern/"; tail -5 "$log"; return
    fi
  done
  pass=$((pass + 1)); echo "PASS $mode: exit $rc; $(grep -Em1 -- "${1:-.}" "$log" | cut -c1-140)"
}
wanted() { [ $# -eq 0 ] && return 0; for m in "$@"; do [ "$m" = "$mode_name" ] && return 0; done; return 1; }
run() { mode_name=$1; if wanted "${only[@]}"; then check "$@"; fi; }
only=("$@")

# Handlers of every shape, fired by the real component or by the library's test hook.
run handlers 0 \
  'proto click n=1 button=Left count=1 kind=Mouse x=36 y=32' \
  'proto returns true=1 false=0' \
  'proto value back="payload/7"' \
  'proto value back="with-data/8/9/row-9"' \
  'proto value default=""' \
  'proto toggle checked=true' 'proto toggle checked=false' \
  'proto hover inside=true' 'proto hover inside=false' \
  'proto row id=5 name="row 5" button=Left' \
  'proto mouse button=Left count=1' \
  'proto key key="k" char="k" has_char=true held=false' \
  'proto key key="enter" .*mods=.*\{Shift\}' \
  'proto done mode=handlers views=[0-9]+ .*clicks=1'
# A handler that does not fit the closure stops at registration, naming both types and the line.
run shape 2 'this handler takes a bool, but the event of the builder it is given to is a Click_Event' 'at .*proto/main.odin:[0-9]+'
run state-type 2 'a handler takes \^Other but its view.s state is App' 'at .*proto/main.odin:[0-9]+'
# Kit state behind a handle: a subscription, a dependency, read-your-writes, unsubscribe.
run state 0 \
  'proto echo view=1 frame=0 value="abc"' \
  'proto input change value="abcx"' 'proto input change value="abcxy"' \
  'proto echo view=[0-9]+ frame=0 value="abcxy"' \
  'proto input enter secondary=false shift=false' \
  'proto read-after-write value="written"' \
  'proto unsubscribed' \
  'proto echo view=[0-9]+ frame=0 value="writtenz"' \
  'proto done mode=state .*events=3 '
run stale 2 'handle 0x[0-9a-f]+ \(state\) was released' 'at .*proto/main.odin:[0-9]+'
# A buffered write to a released handle is found after the handler returned: it still stops.
run stale-write 2 'proto stale-write buffered' 'a change a handler asked for could not be made: handle 0x[0-9a-f]+ \(state\) was released' 'GK_DEBUG=1'
run wrong-type 2 'handle 0x[0-9a-f]+ is a rows, not a state' 'at .*proto/main.odin:[0-9]+'
# A handler called from inside the state's own update: its read reports busy and its write
# lands when that update has ended (not before: the caller still reads the old value); a
# queued event runs after the update and sees the write.
run lease 0 \
  'proto lease busy=true' \
  'proto lease ret=1 value_after="abc"' \
  'proto queued not yet' \
  'proto queued row=42 ok=true value="written inside the update"'
# The write rule: a per-frame handler's write is what the view describes in the same frame.
run write-rule 0 \
  'proto echo view=[0-9]+ frame=1 value="frame 1"' \
  'proto echo view=[0-9]+ frame=2 value="frame 2"' \
  'proto echo view=[0-9]+ frame=3 value="frame 3"' \
  'frame_turns: [34],'
# A frame handler alone (nothing dirty, nothing observed) runs before every frame, and a quiet
# frame costs that one turn and no description.
run frames 0 'proto done mode=frames views=1 .*frame=4 ' 'frame_turns: 4,' 'views_turns: 1,'
run view-write 2 'a view procedure may not write' 'at .*proto/main.odin:[0-9]+'
# App state: a model marks the views that read it, and only those; observers run.
run model 0 \
  'proto side view run=1 count=0' \
  'proto model set count=1 observer_saw=1' \
  'proto side view run=2 count=1' \
  'proto model set count=2 observer_saw=2' \
  'proto side view run=3 count=2' \
  'proto done mode=model views=1 sides=3 '
# Background posts: 20,001 posts from another thread over half a second and ten frames, in 20
# bursts 25 ms apart. Every one is delivered, in at most one WAKE turn per burst (between 5 and
# 17 turns in all, depending on how fast the UI thread drains each burst),
# and the view they all mark is described at most once per frame. A second thread posts one
# message holding a slice of records with strings and a dynamic array (copied when posted).
run post 0 \
  'proto post batch items=3 first="item 0" last="item 2" nums=4 sum=10' \
  'proto done mode=post .*count=20000 seen=20000 ticks=20001 sent=20001' \
  'wake_turns: ([2-9]|1[0-9]|2[0-2]),' \
  'views_turns: ([2-9]|1[01]),'
# Panics stop at the boundary: the app's exits 2 with its line, the library's exits 1.
run panic-odin 2 "the app.s own panic" 'at .*proto/main.odin:[0-9]+'
run panic-call 1 'gk: the library panicked: test panic asked for by the app' 'in the call at .*proto/main.odin:[0-9]+'
run panic-later 1 'gk: the library panicked: test panic asked for by the app.*while applying buffered writes'
run thread 2 'was called from a thread that is not the UI thread'
# Versions: the library refuses a gk_app from another generation, and the binding a library.
run version 0 \
  'proto version hash rc=8 message="the binding and the library are from different versions' \
  'proto version major rc=8' 'proto version newer-minor rc=8' \
  'proto version size rc=8 message="the binding passed a gk_app of [0-9]+ bytes' \
  'proto version callback rc=8'
# Kit state that a view still shows after it was released stops the program with the message
# when the view is drawn (exit 2); the library used to draw nothing in its place, exit 0.
run released-shown 2 'proto released-shown released' \
  'a view shows a .*InputBaseState.* through a handle that cannot be used: handle 0x[0-9a-f]+ \(state\) was released'
# Posted messages are freed when they are delivered (their strings are the procedure's), and
# the binding warns past 16 MB held: 300,000 distinct lines of 100 bytes, queued at once,
# twice. The second round reuses what the first freed. They used to be interned for good.
run post-memory 0 'gk: warning: the binding holds (1[6-9]|[2-9][0-9]) MB' \
  'proto post-memory round=2 ' 'proto post-memory second round grew_mb=[0-9] held_kb=[0-9]$' 'proto done mode=post-memory .*'
# Handler values of a view described on every frame with new strings are freed with its old
# handler tables: 600 builds of 100 handlers, what the binding holds stays under 100 kB.
run handler-memory 0 'proto handler-memory done frames=60[01] .*held_kb=([0-9]|[1-9][0-9])$'
# Row sources (section 9): the first rows are described before the first frame; a jump to row
# 5000 draws placeholders for one frame, one ROWS turn binds the rows and the next frame shows
# them; a row's handler fires with its data; rows already described are kept, never described
# again; a count drops rows; a request is answered by the app.
run rows 0 \
  'proto row clicked row=5000' \
  'proto source request id=1 text="ping"' \
  'proto answered ok=true text="pong ping" bound=20' \
  'rows_turns: 1,' 'placeholder_frames: 1 ' 'rows_missing: [1-9][0-9]?,' \
  'proto rows described=[0-9]+ again=0'
GK_PROTO_BIN=proto-odin-stale run version-lib 1 'gk: the library and this package are from different versions: the library is ABI [0-9.]+ with interface hash'

# The delegate families' own tests (docs/PROTOCOLS.md section 9.8): each family adds
# core/tests/run-FAMILY.sh, sourced here, which builds its test app with `build NAME DIR` and
# checks it with `GK_PROTO_BIN=NAME run MODE EXIT PATTERN...`.
for extra in "$here"/run-*.sh; do
  [ -f "$extra" ] && . "$extra"
done

echo "GATE proto odin $([ $pass = $total ] && echo PASS || echo FAIL) $pass of $total protocol tests"
[ $pass = $total ]
