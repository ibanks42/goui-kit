#!/bin/bash
# misuse.sh: the Odin misuse gate (keeping a string made during a view and its relatives). Eighteen
# run-time cases (seven of them against the delegate protocol and InputState), offscreen with
# the diag library, and one case that must not compile.
# Each must behave correctly or stop with its message; exit 0 with wrong content fails.
# Needs bin/negative-odin, bin/negative_rows-odin and bin/negative_data-odin (examples/odin/build.sh negative negative_rows negative_data). Prints "GATE misuse ...".
#   gates/misuse.sh
here=$(cd "$(dirname "$0")" && pwd); repo=$(dirname "$here")
out=$here/out/misuse
rm -rf "$out"; mkdir -p "$out"
export LD_LIBRARY_PATH=${GK_LIB_ROOT:-$repo/lib}/diag
unset WAYLAND_DISPLAY DISPLAY
pass=0; total=0
ok() { pass=$((pass + 1)); echo "PASS $1"; }
run() { # mode expected-exit pattern-that-must-appear
  total=$((total + 1))
  GK_NEG=$1 timeout 60 "${GK_BIN:-$repo/bin}/${NEG_BIN:-negative-odin}" > "$out/$1.log" 2>&1
  rc=$?
  if [ $rc -eq "$2" ] && grep -q -- "$3" "$out/$1.log"; then
    ok "$1: exit $rc; $(grep -m1 -- "$3" "$out/$1.log" | cut -c1-150)"
  else
    echo "FAIL $1: exit $rc (wanted $2 and /$3/)"; tail -5 "$out/$1.log"
  fi
}
run rune58 0 'done mode=rune58 clicks=3 selected="item 0 of build after 2 clicks"'
run keep 0 'done mode=keep clicks=3 selected="" kept="typed text"'
run cast 3 'memory of turn [0-9]* (EVENT) was used after that turn ended'
run cx 3 'memory of turn [0-9]* (EVENT) was used after that turn ended'
run pointer 2 'holds a pointer'
run element 2 'an element was used after the view procedure that made it returned'
run self 2 'an element was made a child of itself'
run view 2 'a view procedure may not create views'
run usize 2 'a negative number (-1) was given where the kit takes a count'
run twice 2 'one handler was given to two builders'
run stored 2 'this handler was kept past the call that made it'
# The delegate protocol and InputState (examples/odin/negative_rows).
NEG_BIN=negative_rows-odin
run table-freed 2 'a view shows a row source through a handle that cannot be used'
run row-reenter 2 'a view procedure may not write: it only describes (gk.release)'
run row-handler 0 'gk-neg row-handler clicked text="row 0 of a list"'
run huge-count 2 'gk.table_source was given 3000000000 rows of 2 columns; a table holds at most'
run nul 0 'gk-neg nul len=3 same=true'
run utf8 2 'a string given to the kit is not UTF-8 (byte 0xff at offset 2 of 4'
run input-release 2 'a view shows a .*InputBaseState<.*InputMode> through a handle that cannot be used'
# The data protocol (examples/odin/negative_data, docs/PROTOCOLS.md section 12).
NEG_BIN=negative_data-odin
run data-ok 0 'gk-neg done mode=data-ok drawn=1'
run data-column 2 'names the column "dekstop", which the rows of this data (.*Month) do not have: month, desktop'
run data-kind 2 'reads the column "month" of .*Month as a number, and it holds text'
run data-number 2 'asks for number 2, and this builder hands it 1'
run data-brace 2 'has a { that is not closed'
run data-row 2 'names the column "month", and this builder has no row to read it from'
run data-field 2 'the field parts of .*Bad is a \[\]int, which a data table cannot carry'
# View-built closures (examples/odin/negative_overlay, docs/PROTOCOLS.md section 11.6).
NEG_BIN=negative_overlay-odin
run built-ok 0 'gk-neg done mode=built-ok built=[1-9][0-9]* active=true'
run built-root 2 'the procedure given to gk.built for a Dialog returned a node of type [0-9]*, not a Dialog'
run built-freed 2 'a view shows a Dialog (the view gk.built made) through a handle that cannot be used'
run built-view 2 'gk.built in a view procedure'
run built-none 2 'no view given: make it with gk.built'
run dock-node 2 'node id 424242 was never handed out by the dock'
run dock-register 2 'gk.register_panel in a view procedure'
# The values family (examples/odin/negative_values, docs/PROTOCOLS.md 13.8 to 13.12).
NEG_BIN=negative_values-odin
run val-ok 0 'gk-neg done mode=val-ok drawn=[0-9]* clicks=2'
run val-self 2 'a value was given as an argument to its own builder'
run val-self-view 2 'a value was given as an argument to its own builder'
run val-write-view 2 'a view procedure may not write'
run val-action-view 2 'gk.action_of in a view procedure'
run val-bad-time 2 'the time 24:00:00.000000000 does not exist'
run val-scroll-freed 2 'a view shows a .*ScrollHandle through a handle that cannot be used'
# (PROTOCOLS.md 11.17, 11.25).
run c6-rules-source 2 'gk.search_will_change was given a row source gk.search_source did not make'
run c6-rules-limits 2 'gk.search_will_change: min 3 and max 1 cannot both hold'
run c6-tooltip-kept 2 'was used after the view procedure that made it returned'
NEG_BIN=negative-odin
# The lists and editor family (examples/odin/negative_lists2, docs/PROTOCOLS.md 9.9 to 9.12).
NEG_BIN=negative_lists2-odin
run sel-ok 0 'gk-neg sel-ok value="b" ok=true'
run sel-foreign 2 'gk.search_items was given a row source gk.search_source did not make'
run sel-view 2 'a view procedure may not write'
run expr-syntax 2 'the expression "x <" does not parse'
run expr-arg 2 'the expression "z \* 2" uses z, but the closure is handed 2 argument'
run expr-func 2 'the expression "clamp(x, 1)" calls clamp with 2 argument(s); it takes 3'
run nonzero 2 'a count that must not be 0 was given 0'
run kept-view 2 'a view procedure may not write'
NEG_BIN=negative-odin
# (end of the misuse cases)
# (examples/odin/negative_editor, docs/PROTOCOLS.md 9.20 to 9.26).
NEG_BIN=negative_editor-odin
run ed-ok 0 'gk-neg done mode=ed-ok drawn=[1-9][0-9]* rows=[1-9]'
run ed-owner 2 'is not an input, a textarea or an editor'
run ed-owner-released 2 'a view shows a kit state (as an entity id) through a handle that cannot be used'
run ed-plugin-freed 2 'a view shows a row source through a handle that cannot be used'
# Round 6 (docs/PROTOCOLS.md 9.30 to 9.37): the editor host's held values and leaves, decorators, lent edits.
run ed6-ok 0 'gk-neg done mode=ed6-ok drawn=[1-9][0-9]* rows=[1-9][0-9]* leaves=[1-9][0-9]*'
run ed6-held-stale 2 'cannot be used'
run ed6-leaf-stale 2 'is temporary: it lives until the next frame begins'
run ed6-decorate-freed 2 'a view shows a row source through a handle that cannot be used'
run ed6-lent-view 2 'a value made in a view procedure was given to a call'
# Round 7 (docs/PROTOCOLS.md 9.38 to 9.40): a parsed Custom block placed with its flow; kept, and not kept.
run ed7-ok 0 'gk-neg done mode=ed7-ok drawn=[1-9][0-9]* rows=0 leaves=[1-9][0-9]*'
run ed7-custom-unretained 2 'is temporary\|cannot be used'
NEG_BIN=negative-odin
# Attacks on later protocols (examples/odin/negative_review3): actions, timers, windows, dock panels,
# temporary library objects.
NEG_BIN=negative_review3-odin
run rv-ok 0 'gk-neg done mode=rv-ok drawn=[0-9]* clicks=2 fired=1 closed=1 acted=[1-9] other=0'
run rv-timer-view 2 'a view procedure may not start timers'
run rv-timer-zero 2 'timer (gk.[a-z]*) of 0 ms: give'
run rv-window-stale 2 'was released'
run rv-keys-view 2 'a view procedure may not write'
run rv-action-forged 2 'names no action this library made'
run rv-temp-later 2 'temporary'
run rv-panel-released 2 'was released'
run rv-timer-state 2 'a handler takes'
NEG_BIN=negative-odin
# The compile-time case: lent text assigned to a string must be refused by the compiler.
total=$((total + 1))
if "${ODIN:-odin}" check "${SRC:-$repo/examples/odin}/negative_compile" \
     -define:GK_SHARED=true -collection:gpuikit="$repo/odin" > "$out/compile.log" 2>&1; then
  echo "FAIL compile: the program compiled"
elif grep -q "Temp_String" "$out/compile.log"; then
  ok "compile: refused; $(grep -m1 Temp_String "$out/compile.log" | cut -c1-150)"
else
  echo "FAIL compile: refused for another reason"; head -5 "$out/compile.log"
fi
echo "GATE misuse odin $([ $pass = $total ] && echo PASS || echo FAIL) $pass of $total cases"
[ $pass = $total ]
