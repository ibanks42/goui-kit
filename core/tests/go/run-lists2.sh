# Searchable select/combobox, styled table, input masks/validators, LSP providers,
# math expressions, Ask caches, diff parsing, inline tokens, lending, and highlighters.
build lists2-go lists2
# Isolate logs under $out/lists2 to avoid filename collisions with other families.
lists2_out=$out; out=$out/lists2; mkdir -p "$out"

# Proves Select confirm events, programmatic set_selected_value/set_selected_index/set_items,
# and clear behavior all stay consistent with the selected value and index readback.
GK_PROTO_BIN=lists2-go run select 0 \
  'lists2 select confirm has=true value="[a-z]+" now="[a-z]+" same=true' \
  'lists2 select set_selected_value value="cherry" ok=true row=2 index_ok=true' \
  'lists2 select set_selected_index value="mango"' \
  'lists2 select set_items value="mango" ok=true row=0' \
  'lists2 select cleared has=false' 'lists2 done mode=select events=1 '
# Proves local searchable Select filters options by query and confirms "mango".
GK_PROTO_BIN=lists2-go run search 0 'lists2 select confirm has=true value="mango" ' 'lists2 done mode=search events=1 '
# Proves async search requests forward the typed query "x" with a request id and confirm the answered "xenon" item.
GK_PROTO_BIN=lists2-go run search-requests 0 'lists2 search request id=[1-9][0-9]* query="x"' \
  'lists2 select confirm has=true value="xenon" ' 'lists2 done mode=search-requests events=1 requests=[1-9]'
# Proves grouped Select sources invoke group header and row builders for each group, confirm "apple",
# and do not redundantly re-render after confirmation (again=0).
GK_PROTO_BIN=lists2-go run rows 0 'lists2 header group=0' 'lists2 header group=1' \
  'lists2 row group=0 row=0 value=apple' 'lists2 row group=1 row=1 value=leek' \
  'lists2 select confirm has=true value="apple" ' 'lists2 done mode=rows events=1 .* again=0$'
# Proves multi-select Combobox supports confirm/change events, add (deduping duplicates),
# remove, programmatic set with query, and clear back to empty.
GK_PROTO_BIN=lists2-go run combobox 0 \
  'lists2 combobox confirm values=\["banana", "cherry"\]' 'lists2 combobox change values=\[\]' \
  'lists2 combobox initial values=\["banana", "cherry"\] n=2 first=Banana row=1' \
  'lists2 combobox add first=true again=false values=\["banana", "cherry", "apple"\]' \
  'lists2 combobox remove=true values=\["cherry", "apple"\]' \
  'lists2 combobox set values=\["apple", "mango"\] query="an"' \
  'lists2 combobox cleared n=0 has=false' 'lists2 done mode=combobox '

# Proves Table group headers describe their view, DumpTable exports headers and cell matrix,
# fit checks distinguish matching vs oversized tables, and the delegate identity equals the source handle.
GK_PROTO_BIN=lists2-go run table 0 'lists2 group view described' \
  'lists2 table dump headers=\["Id", "Name", "A"\] rows=\[\["0", "zero", "a0"\], \["1", "one", "a1"\], \["", "", ""\]\]' \
  'lists2 table fit=true nofit=false delegate_is_source=true' 'lists2 done mode=table '
# Proves SetSearchItems rejects a row source not created by NewSearchSource and exits 1 at the Go call site.
GK_PROTO_BIN=lists2-go run bad-source 1 'gk.SetSearchItems was given a row source gk.NewSearchSource did not make' 'at .*lists2/main.go:[0-9]+'
# Proves configuring group headers without a row builder is rejected with exit 1 at the Go call site.
GK_PROTO_BIN=lists2-go run bad-headers 1 'header rows need a row function' 'at .*lists2/main.go:[0-9]+'
# Proves displaying a released row source in a view fails cleanly with exit 1 instead of crashing.
GK_PROTO_BIN=lists2-go run released 1 'lists2 released source' 'a view shows a row source through a handle that cannot be used'

# Proves both pattern masks ("123-456") and number masks ("1,234,567") format input text as expected.
GK_PROTO_BIN=lists2-go run mask 0 'lists2 mask pattern="123-456" number="1,234,567"'
# Proves input validator callbacks run on each keystroke and reject disallowed characters leaving "abc".
GK_PROTO_BIN=lists2-go run validate 0 'lists2 validate value="abc" calls=[4-9]'
# Proves SelectableText exposes both full text ("hello world") and the selected substring ("world").
GK_PROTO_BIN=lists2-go run text 0 'lists2 text text="hello world" selected="world"'
# Proves editor display-map queries return valid line/row/wrap counts and unfold status on an empty buffer.
GK_PROTO_BIN=lists2-go run display-map 0 'lists2 display-map lines=[0-9]+ rows=[0-9]+ wraps=[0-9]+ text="" folded=false candidate=false'
# Proves all LSP provider callbacks (completions, document colors, semantic tokens, inline completions,
# code actions, and performing action "fix-1") dispatch and receive answers.
GK_PROTO_BIN=lists2-go run providers 0 'lists2 provider completions id=' 'lists2 provider colors id=' \
  'lists2 provider tokens id=' 'lists2 provider inline id=' 'lists2 provider code-actions id=' \
  'lists2 provider perform id=fix-1 ' 'lists2 done mode=providers '
# Proves step expression formulas evaluate both step-by ("20") and value-dependent ("12") increments.
GK_PROTO_BIN=lists2-go run expr 0 'lists2 expr step_by="20" by_value="12"'
# Proves a syntax error in a step expression (ternary `?` missing `:`) fails at compile time with exit 1.
GK_PROTO_BIN=lists2-go run bad-expr 1 'the expression "x < 10 \? 1" does not parse: a \? without its :'

# Proves programmatic selection and scrolling work across List, Table, and Tree and resolve
# target row handles (row 500/700/150 selected, 900/0 handles reachable).
GK_PROTO_BIN=lists2-go run drive 0 'lists2 drive before list=false table=false tree=false' \
  'lists2 drive selected list=500 table=700 tree=150 ok=true' 'lists2 drive scrolled list=true table=true tree=true' \
  'lists2 drive handle list900=true table0=true'
# Proves kept-handle pools retain added handles (3) and drop them cleanly when released.
GK_PROTO_BIN=lists2-go run kept 0 'lists2 kept added=3 dropped=true'
# Proves semantic status and syntax theme color helpers map created->success, deleted->error,
# and border->error consistently.
GK_PROTO_BIN=lists2-go run status-colors 0 \
  'lists2 status error_opaque=(true|false) error_not_warning=(true|false) created_is_success=true deleted_is_error=true border_is_error=true' \
  'lists2 status syntax keyword=false index0=false'
# Proves the Ask cache routes both syntax highlight and image URI requests, caches answers (2/2),
# and signals when more requests arrive.
GK_PROTO_BIN=lists2-go run ask 0 'lists2 ask highlight lang=demo code="proc x' 'lists2 ask image uri=pic.png' \
  'lists2 ask counts asked=2 answered=2 highlight=1 image=1 gets_more=true'

# Proves unified diff parsing succeeds for a 2-hunk diff, rejects malformed diffs with an error string,
# and constructs a DiffState handle.
GK_PROTO_BIN=lists2-go run diff 0 'lists2 diff count=2 ok=true bad_ok=false bad_err="[^"]+"' 'lists2 diff state made'
# Proves inline token context callbacks report the byte range [6, 11], label "World", line height,
# and update selected from false to true when selected.
GK_PROTO_BIN=lists2-go run ctx-token 0 \
  'lists2 ctx token row=0 range=Range\{lo = 6, hi = 11\} selected=false disabled=false readonly=false label="World" height_ok=true' \
  'lists2 ctx token row=0 range=Range\{lo = 6, hi = 11\} selected=true '
# Proves temporary handle lending exposes an empty collection inside the callback, persists mutations
# (len=2), and balances lent=1 with given_back=1.
GK_PROTO_BIN=lists2-go run lend 0 'lists2 lend got=true empty=true' 'lists2 lend after len=2 lent=1 given_back=1'
# Proves custom syntax highlighter callbacks answer both highlight-range and fold-range requests for lang=demo.
GK_PROTO_BIN=lists2-go run highlight 0 'lists2 highlight lang=demo range=Range\{lo = [0-9]+, hi = [0-9]+\} text="main :: proc' \
  'lists2 highlight folds lang=demo lines=3' 'lists2 highlight counts styles=true folds=true asked=[1-9][0-9]* answered=[1-9][0-9]* gets_more=true'
# Proves registering a custom language triggers syntax queries for lang=demo with valid byte offsets.
GK_PROTO_BIN=lists2-go run languages 0 'lists2 syntax asked lang=demo offset=[0-9]+' 'lists2 languages syntax_asked=true'
# Proves retaining and using a lent handle after its callback returned fails as released with exit 1.
GK_PROTO_BIN=lists2-go run bad-lend 1 'lists2 bad-lend using the lent handle' 'was released' 'at .*lists2/round4.go:[0-9]+'
# Proves miscellaneous round-4 helpers: diff parse-error location/message, prefix highlight range [0, 2],
# and virtual scroll offset moving negative to draw distant rows.
GK_PROTO_BIN=lists2-go run small 0 'lists2 small parse-error ok=true line=1 message="hunk has no file header" good_has_error=false' \
  'lists2 small highlight_prefix=\[Range\{lo = 0, hi = 2\}\]' 'lists2 small scroll before y=0' 'lists2 small scroll after y=-[1-9][0-9.]* drawn_far=true'
# Proves will-change hooks intercept or observe value transitions and commit ["banana"].
GK_PROTO_BIN=lists2-go run will-change 0 'lists2 will-change values=\["banana"\]'
out=$lists2_out
