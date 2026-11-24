# run-input.sh: the input family's protocol tests (docs/PROTOCOLS.md sections 9.6 and 9.8),
# sourced by core/tests/go/run.sh, which defines build and run. The app is core/tests/go/input:
# the three instantiations of InputBaseState<M> (InputState, EditorState, TextareaState, made by
# the generator's `[instantiate] states` rule), OtpState, and every handler shape the family's
# elements take, driven by gk-meter scripts offscreen.
build input-go input

# Events of an InputState: a change per typed key and enter (focus and blur are not raised
# by the offscreen driver, whose window is never activated); the selection (a read, no event
# upstream) through select all and shift-left; copy, paste, undo and redo by key.
GK_PROTO_BIN=input-go run input 0 \
  'input change value="h" cursor=1' 'input change value="hello" cursor=5' \
  'input selection lo=0 hi=5 text="hello" reversed=false' \
  'input change value="hellohello" cursor=10' \
  'input change value="hello" cursor=5' \
  'input selection lo=8 hi=10 text="lo" reversed=true' \
  'input enter secondary=false shift=false value="hellohel"' \
  'input done mode=input events=10 '
# Reads, buffered writes (read your own write), calls that return a value at once, and a read
# of a state on lease, on all three instantiations.
GK_PROTO_BIN=input-go run calls 0 \
  'calls input value="abc def" selected=0..3 text="abc" single=true multi=false' \
  'calls input insert value="abcX def" cursor=4' \
  'calls input replace value="YX def" cursor=1' \
  'calls input select_all 0..6' \
  'calls input editable=false' \
  'calls textarea multi=true single=false value="one two\\none three"' \
  'calls textarea next_match 8\.\.11' \
  'calls textarea replaced=2 value="1 two\\n1 three"' \
  'calls editor code=true multi=true language="rust" value="fn main' \
  'calls lease busy=true' 'calls lease ret=1 value="YX def"' \
  'calls done' 'calls lease later value="after the lease"'
# The handler shape (StepAction) of base::NumberInput::on_step, fired by up and down in the
# focused number input; on_step replaces upstream's stepping, so the handler steps the value.
GK_PROTO_BIN=input-go run number 0 \
  'number step action=Increment n=1' 'number step action=Increment n=2' 'number step action=Decrement n=3' \
  'number value="6"' 'number value="8"' 'input done mode=number .*steps=3 '
# OtpState: its events, one change per digit and complete at the last.
GK_PROTO_BIN=input-go run otp 0 'otp event=Change value="1"' 'otp event=Complete value="1234"'
# The handler shape (ClipboardItem) -> bool of Input::on_paste: it sees the copied text and,
# returning false, lets the input paste.
GK_PROTO_BIN=input-go run paste 0 'input paste n=1 entries=1 text="ab"' 'input change value="abab"'
# A released EditorState that a view still shows stops the program (gk.Run returns it: exit 1), as InputState.
GK_PROTO_BIN=input-go run released-editor 1 'input released-editor released' \
  'a view shows a .*InputBaseState<.*EditorMode>.* through a handle that cannot be used'
# A view function may not run an update that returns a value.
# Go: "a view function", and the app's line in main.go.
GK_PROTO_BIN=input-go run call-in-view 1 'a view function may not change kit state' 'at .*input/main.go:[0-9]+'
# The editor's providers over a row source (section 9.6): a typed trigger character asks for
# completions as a queued event with a request id; the app's answer fills the menu of the
# styled editor (gk.CodeEditor) and enter takes its first item, which replaces the query
# (the trigger character; upstream also inserts the newline).
GK_PROTO_BIN=input-go run providers 0 \
  'provider completions id=1 offset=2 kind=2 trigger="\." text="a\."' \
  'editor change value="afirst\\n"' 'input done mode=providers .*asked=1$'
# The handler shapes (&str, &ClickEvent) of TextView::on_link_click and the click of an
# InputGroupButton, on a markdown text view and an input group.
GK_PROTO_BIN=input-go run links 0 'link clicked url="https://example.com/x" button=Left' 'group button clicked count=1'
