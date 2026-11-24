# Text input, textarea, code editor, number/OTP input, clipboard paste, LSP
# completion providers, link clicks, and view-write-rule misuse tests.
build input-go input

# Proves single-line Input handles typing, forward/reversed selection, copy/paste,
# undo/redo, and Enter submission across 10 ordered input events.
GK_PROTO_BIN=input-go run input 0 \
  'input change value="h" cursor=1' 'input change value="hello" cursor=5' \
  'input selection lo=0 hi=5 text="hello" reversed=false' \
  'input change value="hellohello" cursor=10' \
  'input change value="hello" cursor=5' \
  'input selection lo=8 hi=10 text="lo" reversed=true' \
  'input enter secondary=false shift=false value="hellohel"' \
  'input done mode=input events=10 '

# Proves imperative methods on InputState, TextareaState, and EditorState (insert, replace,
# select_all, search/replace, language mode) and proves re-entrant entity writes defer
# cleanly while an outer lease is active and apply once the lease ends.
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

# Proves NumberInput increment/decrement buttons fire Step events and update the formatted value (6 -> 8).
GK_PROTO_BIN=input-go run number 0 \
  'number step action=Increment n=1' 'number step action=Increment n=2' 'number step action=Decrement n=3' \
  'number value="6"' 'number value="8"' 'input done mode=number .*steps=3 '
# Proves OtpInput emits Change on intermediate digits and Complete once all 4 digits are entered.
GK_PROTO_BIN=input-go run otp 0 'otp event=Change value="1"' 'otp event=Complete value="1234"'
# Proves Paste callbacks observe pasted clipboard entries and allow the text to append ("ab" -> "abab").
GK_PROTO_BIN=input-go run paste 0 'input paste n=1 entries=1 text="ab"' 'input change value="abab"'

# Proves rendering a released EditorState handle in a view fails cleanly with the Rust type name and exits 1.
GK_PROTO_BIN=input-go run released-editor 1 'input released-editor released' \
  'a view shows a .*InputBaseState<.*EditorMode>.* through a handle that cannot be used'
# Proves mutating kit state directly from inside a view function is rejected with exit 1 and the Go call site.
GK_PROTO_BIN=input-go run call-in-view 1 'a view function may not change kit state' 'at .*input/main.go:[0-9]+'

# Proves editor completion providers receive trigger character "." at offset 2 and inserting the reply mutates the document.
GK_PROTO_BIN=input-go run providers 0 \
  'provider completions id=1 offset=2 kind=2 trigger="\." text="a\."' \
  'editor change value="afirst\\n"' 'input done mode=providers .*asked=1$'
# Proves hyperlink clicks report the clicked URL and mouse button, and button-group clicks fire with count=1.
GK_PROTO_BIN=input-go run links 0 'link clicked url="https://example.com/x" button=Left' 'group button clicked count=1'
