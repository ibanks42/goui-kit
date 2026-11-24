#!/bin/bash
# go-misuse.sh: the Go front end's misuse gate. Run-time cases
# (examples/go/misuse, offscreen with the diag library): each must behave correctly or stop with
# its message; exit 0 with wrong content fails. Compile-time cases (examples/go/misuse_compile):
# each must fail to build, with the compiler's message naming the mistake. Prints
# "GATE misuse go ...". Needs bin/misuse-go, bin/misuse_rows-go, bin/misuse_data-go,
# bin/misuse_overlay-go, bin/misuse_values-go, bin/misuse_lists-go, bin/misuse_editor-go and
# bin/misuse_review-go (examples/go/build.sh misuse misuse_rows misuse_data misuse_overlay
# misuse_values misuse_lists misuse_editor misuse_review).
#
# Each case is a run or compile case, a comment naming the Go case that covers it, a comment
# saying why Go makes the mistake impossible, or a PENDING line (a Go binding is missing; not
# counted; listed in gates/go-pending.txt).
here=$(cd "$(dirname "$0")" && pwd); repo=$(dirname "$here")
. "$repo/go/env.sh"
out=$here/out/go/misuse
rm -rf "$out"; mkdir -p "$out"
export LD_LIBRARY_PATH=${GK_LIB_ROOT:-$repo/lib}/diag
unset WAYLAND_DISPLAY DISPLAY
pass=0; total=0
run() { # mode expected-exit pattern (the program is $NEG_BIN, default misuse)
  total=$((total + 1))
  bin=${NEG_BIN:-misuse}
  log=$out/$1.log
  [ "$bin" = misuse ] || log=$out/$bin-$1.log
  GK_NEG=$1 timeout 60 "${GK_BIN:-$repo/bin}/$bin-go" > "$log" 2>&1
  rc=$?
  if [ $rc -eq "$2" ] && grep -Eq -- "$3" "$log"; then
    pass=$((pass + 1)); echo "PASS $1: exit $rc; $(grep -Em1 -- "$3" "$log" | cut -c1-150)"
  else
    echo "FAIL $1: exit $rc (wanted $2 and /$3/)"; tail -5 "$log"
  fi
}
pending() { # case what-is-missing: not run, not counted (gates/go-pending.txt lists it)
  echo "GATE misuse $1 PENDING needs $2"
}
# A handler given to two builders, or kept past its turn: impossible in Go, a handler is a closure
# the collector keeps alive, so it may be given to any number of builders, in any turn; twice is
# the element form of the mistake. Lent text kept, or a pointer as handler data: impossible in Go,
# InputState.Value returns a Go string (a copy, kept by the collector) and a closure may capture
# any pointer.
run rune58 0 'done mode=rune58 clicks=3 selected="item 0 of build after 2 clicks"'
run keep 0 'done mode=keep clicks=3 selected="" kept="typed text"'
run cx 2 'a \*gk.Cx was used after the function it was given to returned'
run element 2 'an element was used after the view function that made it returned'
run twice 2 'an element was given to a second parent'
run self 2 'an element was made a child of itself'
run view 2 'a view function may not create views'
run usize 2 'a negative number \(-1\) was given where the kit takes a count'
run nil 2 'a nil function was given as a handler'
run nil-child 2 'a nil element was given to a builder'
run zero 2 'never made'
run built 2 'a builder chain was used after Build'
run utf8 2 'a string given to the kit is not UTF-8 \(byte 0xff at offset 2 of 4'
run nul 0 'gk-neg nul len=3 same=true'
run goroutine 2 'not on the UI thread: only gk.Post may be called there'
run release 2 'through a handle that cannot be used'
run row-write 2 'a view function may not write'
run data-column 2 'a builder names the column "month", which the rows of this data \(main.sale\) do not have: Month, Total'
run panel-name 2 'a panel needs a name \(PanelData.Name\)'
run app-panic 2 'gk: panic in app code: runtime error: index out of range'
run go-panic 2 'gk: panic in the work function of the gk.Go at .*misuse/main.go:[0-9]+: assignment to entry in nil map'
# Every run-time stop names the app's line.
total=$((total + 1))
if grep -lE 'at .*misuse/main.go:[0-9]+' "$out"/{cx,element,twice,self,view,nil,built,utf8,goroutine,row-write,data-column,panel-name,app-panic,go-panic,usize}.log > /dev/null 2>&1 &&
   [ "$(grep -lE 'at .*misuse/main.go:[0-9]+' "$out"/{cx,element,twice,self,view,nil,built,utf8,goroutine,row-write,data-column,panel-name,app-panic,go-panic,usize}.log | wc -l)" = 15 ]; then
  pass=$((pass + 1)); echo "PASS lines: the 15 stops in app code name main.go and the line"
else
  echo "FAIL lines: a stop does not name main.go:LINE"; grep -L 'misuse/main.go:' "$out"/*.log
fi
# The delegate protocol and InputState (examples/go/misuse_rows; its nul and utf8 cases are the
# nul and utf8 modes above).
NEG_BIN=misuse_rows
run table-freed 2 'a view shows a row source through a handle that cannot be used'
run row-reenter 2 'a view function may not write: it only describes \(gk.Release\)'
run row-handler 0 'gk-neg row-handler clicked text="row 0 of a list"'
run huge-count 2 'gk.NewTableSource was given 3000000000 rows of 2 columns; a table holds at most'
run input-release 2 'a view shows a .*InputBaseState<.*InputMode> through a handle that cannot be used'
# The data protocol (examples/go/misuse_data). A column
# is an exported field, named as the field is (Month, Desktop).
NEG_BIN=misuse_data
run data-ok 0 'gk-neg done mode=data-ok drawn=1'
run data-column 2 'names the column "Dekstop", which the rows of this data \(main.month\) do not have: Month, Desktop'
run data-kind 2 'reads the column "Month" of main.month as a number, and it holds text'
run data-number 2 'asks for number 2, and this builder hands it 1'
run data-brace 2 'has a \{ that is not closed'
run data-row 2 'names the column "Month", and this builder has no row to read it from'
run data-field 2 'the field Parts of main.bad is a \[\]int, which a data table cannot carry'
# View-built closures (examples/go/misuse_overlay). A function returning a node that is not a
# Dialog does not compile in Go: compile built_root below.
NEG_BIN=misuse_overlay
run built-ok 0 'gk-neg done mode=built-ok built=[1-9][0-9]* active=true'
run built-freed 2 'a view shows a Dialog \(the view gk.NewBuilt made\) through a handle that cannot be used'
run built-view 2 'gk.NewBuilt in a view function'
run built-none 2 'no view given: make it with gk.NewBuilt'
run dock-node 2 'node id 424242 was never handed out by the dock'
run dock-register 2 'gk.RegisterPanel in a view function'
# The values family (examples/go/misuse_values).
NEG_BIN=misuse_values
run val-ok 0 'gk-neg done mode=val-ok drawn=[0-9]* clicks=2'
run val-self 2 'a value was given as an argument to its own builder'
run val-self-view 2 'a value was given as an argument to its own builder'
run val-write-view 2 'a view function may not write'
run val-action-view 2 'gk.ActionOf in a view function'
run val-bad-time 2 'the time 24:00:00.000000000 does not exist'
run val-scroll-freed 2 'a view shows a .*ScrollHandle through a handle that cannot be used'
run c6-rules-source 2 'gk.SearchWillChange was given a row source gk.NewSearchSource did not make'
run c6-rules-limits 2 'min 3 and max 1 cannot both hold'
run c6-tooltip-kept 2 'was used after the function that made it returned'
# The lists family (examples/go/misuse_lists).
NEG_BIN=misuse_lists
run sel-ok 0 'gk-neg sel-ok value="b" ok=true'
run sel-foreign 2 'gk.SetSearchItems was given a row source gk.NewSearchSource did not make'
run sel-view 2 'a view function may not write'
run expr-syntax 2 'the expression "x <" does not parse'
run expr-arg 2 'the expression "z \* 2" uses z, but the closure is handed 2 argument'
run expr-func 2 'the expression "clamp\(x, 1\)" calls clamp with 2 argument\(s\); it takes 3'
run nonzero 2 '0 was given where the kit takes a number that is never 0'
run kept-view 2 'a view function may not write'
# The editor family (examples/go/misuse_editor, docs/PROTOCOLS.md 9.20 to 9.40).
NEG_BIN=misuse_editor
run ed-ok 0 'gk-neg done mode=ed-ok drawn=[1-9][0-9]* rows=[1-9]'
run ed-owner 2 'is not an input, a textarea or an editor'
run ed-owner-released 2 'a view shows a kit state \(as an entity id\) through a handle that cannot be used'
run ed-plugin-freed 2 'a view shows a row source through a handle that cannot be used'
# Round 6 (9.30 to 9.37): the editor host's held values and leaves, decorators, lent edits.
run ed6-ok 0 'gk-neg done mode=ed6-ok drawn=[1-9][0-9]* rows=[1-9][0-9]* leaves=[1-9][0-9]*'
run ed6-held-stale 2 'cannot be used'
run ed6-leaf-stale 2 'is temporary: it lives until the next frame begins'
run ed6-decorate-freed 2 'a view shows a row source through a handle that cannot be used'
run ed6-lent-view 2 'a value made in a view function was given to a call'
# Round 7 (9.38 to 9.40): a parsed Custom block placed with its flow; kept, and not kept.
run ed7-ok 0 'gk-neg done mode=ed7-ok drawn=[1-9][0-9]* rows=0 leaves=[1-9][0-9]*'
run ed7-custom-unretained 2 'is temporary|cannot be used'
# Attacks on later protocols (examples/go/misuse_review). An action id the library never made
# does not compile in Go: compile action_forged below. A timer whose handler takes another state
# than the view's: impossible in Go, a handler is a closure and has no state type.
NEG_BIN=misuse_review
run rv-ok 0 'gk-neg done mode=rv-ok drawn=[0-9]* clicks=2 fired=1 closed=1 acted=[1-9]'
run rv-timer-view 2 'a view function may not start timers'
run rv-timer-zero 2 'a timer of 0s: give 1 ms'
run rv-window-stale 2 '\(window\) was released'
run rv-keys-view 2 'a view function may not write'
run rv-temp-later 2 'temporary: it lives until the next frame begins'
run rv-panel-released 2 'was released'
NEG_BIN=misuse
compile() { # case pattern
  total=$((total + 1))
  (cd "$repo/examples/go" && go build -o /dev/null "./misuse_compile/$1") > "$out/compile-$1.log" 2>&1
  rc=$?
  if [ $rc -ne 0 ] && grep -Eq -- "$2" "$out/compile-$1.log"; then
    pass=$((pass + 1)); echo "PASS compile-$1: does not build; $(grep -Em1 -- "$2" "$out/compile-$1.log" | sed 's/.*main.go:/main.go:/' | cut -c1-150)"
  else
    echo "FAIL compile-$1: exit $rc (wanted a build error matching /$2/)"; tail -5 "$out/compile-$1.log"
  fi
}
compile handler_type 'cannot use func\(cx \*gk.Cx, e gk.ClickEvent\) \{.*\} .* as func\(cx \*gk.Cx, e bool\) value'
compile not_element 'cannot use 42 .* as gk.Element value'
compile token_color 'cannot use s.Gap2 .* as gk.Color value'
compile el_types 'cannot use gk.Button\("b"\) .* as gk.LabelEl value'
compile no_method 'OnClick undefined'
compile value_child 'cannot use gk.NewStyleRefinementDefault\(\) .* as gk.Element value'
compile effect_target 'gk.Text\("x"\) .* does not implement gk.AnimatableElement'
compile shortcut_type 'does not satisfy gk.ShortcutHandler'
# The port round (PROTOCOLS.md 11.38): .Bg takes a gk.Fill (a colour or a gradient); a gradient fits
# nowhere a single colour goes.
compile fill_token 'cannot use s.Gap2 .* as gk.Fill value.*does not implement gk.Fill'
compile gradient_color 'cannot use g .* as gk.Color value'
# Lent text assigned to a string: impossible in Go, InputState.Value returns a string (a copy),
# so keeping it is correct (the keep mode above).
compile built_root 'cannot use gk.NewBuilt\(cx, .*\) .*gk.Built\[gk.LabelEl\].* as gk.Built\[gk.DialogEl\] value'
compile action_forged 'cannot refer to unexported field id in struct literal of type gk.Action'
# No display and no script: GPUI would wait without a word; gk.Run says so (exit 1).
total=$((total + 1))
timeout 30 "${GK_BIN:-$repo/bin}/hello-go" > "$out/no-display.log" 2>&1; rc=$?
if [ $rc = 1 ] && grep -q 'the app has no display to open its window on' "$out/no-display.log"; then
  pass=$((pass + 1)); echo "PASS no-display: exit 1; $(head -1 "$out/no-display.log" | cut -c1-120)"
else
  echo "FAIL no-display: exit $rc (wanted 1 and the no-display message)"; head -3 "$out/no-display.log"
fi
# Without cgo (CGO_ENABLED=0, or no C compiler) the first error says what to do.
total=$((total + 1))
(cd "$repo/examples/go" && CGO_ENABLED=0 go build -o /dev/null ./hello) > "$out/compile-nocgo.log" 2>&1
if [ $? -ne 0 ] && grep -v '^#' "$out/compile-nocgo.log" | head -1 | grep -q 'undefined: gk_needs_cgo__install_gcc_or_clang_and_build_with_CGO_ENABLED_1'; then
  pass=$((pass + 1)); echo "PASS compile-nocgo: CGO_ENABLED=0 stops first at $(grep -v '^#' "$out/compile-nocgo.log" | head -1 | sed 's/.*gk\//gk\//' | cut -c1-120)"
else
  echo "FAIL compile-nocgo: the first error is not the cgo one"; head -5 "$out/compile-nocgo.log"
fi
compile cx_handler 'cannot use func\(e gk.ClickEvent\) \{.*\} .* as func\(cx \*gk.Cx, e gk.ClickEvent\) value'
echo "GATE misuse go $([ $pass = $total ] && echo PASS || echo FAIL) $pass of $total cases"
[ $pass = $total ]
