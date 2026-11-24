# run-lists2.sh: the protocol tests of the searchable lists and the
# table delegate's data (docs/PROTOCOLS.md sections 9.9 and 9.10), sourced by core/tests/go/run.sh.
# The app is core/tests/go/lists2: the kit's Select and Combobox over the library's search delegate
# and a table's texts and looks, offscreen under gk-meter scripts. Odin twin: core/tests/run-lists2.sh.
build lists2-go lists2
# Logs of their own: run.sh keeps a mode's log by its name, and rows, table, providers and released
# are also modes of the rows and input apps (their logs were read for these checks).
lists2_out=$out; out=$out/lists2; mkdir -p "$out"

# A select over items given as data: the menu opened, Down and Enter confirm an item (the
# state's Confirm event, the value read back the same); the generated writes and reads:
# SetSelectedValue, SetSelectedIndex (row 4 is mango), new items set by SetItems keep the
# selected value at its new row, a cleared selection.
GK_PROTO_BIN=lists2-go run select 0 \
  'lists2 select confirm has=true value="[a-z]+" now="[a-z]+" same=true' \
  'lists2 select set_selected_value value="cherry" ok=true row=2 index_ok=true' \
  'lists2 select set_selected_index value="mango"' \
  'lists2 select set_items value="mango" ok=true row=0' \
  'lists2 select cleared has=false' 'lists2 done mode=select events=1 '
# Searching filters in the library as upstream's SearchableVec does: "an" keeps Banana and
# Mango, the cursor on the first, so Down and Enter take Mango (unfiltered, Down would reach
# Banana).
GK_PROTO_BIN=lists2-go run search 0 'lists2 select confirm has=true value="mango" ' 'lists2 done mode=search events=1 '
# A source with search requests: the query goes to the app, which answers with items of its own
# (gk.SetSearchItems, SetItems, gk.AnswerSearch); Enter takes the first of them.
GK_PROTO_BIN=lists2-go run search-requests 0 'lists2 search request id=[1-9][0-9]* query="x"' \
  'lists2 select confirm has=true value="xenon" ' 'lists2 done mode=search-requests events=1 requests=[1-9]'
# Groups whose items and headers are row views: each described once when the menu draws it.
GK_PROTO_BIN=lists2-go run rows 0 'lists2 header group=0' 'lists2 header group=1' \
  'lists2 row group=0 row=0 value=apple' 'lists2 row group=1 row=1 value=leek' \
  'lists2 select confirm has=true value="apple" ' 'lists2 done mode=rows events=1 .* again=0$'
# A multiple combobox starting with Banana: in the open menu Down and Enter add Cherry and the
# menu reports the selection (Confirm); then the generated calls: add (once), remove, set
# values, a query, clear (a Change with no values).
GK_PROTO_BIN=lists2-go run combobox 0 \
  'lists2 combobox confirm values=\["banana", "cherry"\]' 'lists2 combobox change values=\[\]' \
  'lists2 combobox initial values=\["banana", "cherry"\] n=2 first=Banana row=1' \
  'lists2 combobox add first=true again=false values=\["banana", "cherry", "apple"\]' \
  'lists2 combobox remove=true values=\["cherry", "apple"\]' \
  'lists2 combobox set values=\["apple", "mango"\] query="an"' \
  'lists2 combobox cleared n=0 has=false' 'lists2 done mode=combobox '
# A table's texts (DumpRange reads them through cell_text), a fit width (FitCol), group
# headers with an app view, header and row styles, and its delegate read back as its source.
GK_PROTO_BIN=lists2-go run table 0 'lists2 group view described' \
  'lists2 table dump headers=\["Id", "Name", "A"\] rows=\[\["0", "zero", "a0"\], \["1", "one", "a1"\], \["", "", ""\]\]' \
  'lists2 table fit=true nofit=false delegate_is_source=true' 'lists2 done mode=table '
# Misuse: items for a source that is not a select's; header rows without a row function; a
# released source whose rows a menu still draws.
# Go: the runtime's messages name the Go calls (gk.SetSearchItems, gk.NewSearchSource,
# gk.SearchHeaders: "a row function"), and the app's line is in main.go.
GK_PROTO_BIN=lists2-go run bad-source 2 'gk.SetSearchItems was given a row source gk.NewSearchSource did not make' 'at .*lists2/main.go:[0-9]+'
GK_PROTO_BIN=lists2-go run bad-headers 2 'header rows need a row function' 'at .*lists2/main.go:[0-9]+'
GK_PROTO_BIN=lists2-go run released 2 'lists2 released source' 'a view shows a row source through a handle that cannot be used'
# Options the binding could not give before (docs/PROTOCOLS.md 9.12): a mask as upstream's string
# pattern (an Into member From<&str>) and the Number variant with a char separator.
GK_PROTO_BIN=lists2-go run mask 0 'lists2 mask pattern="123-456" number="1,234,567"'
# A handler given to a state's builder (a kept handler): the validator refuses the x.
GK_PROTO_BIN=lists2-go run validate 0 'lists2 validate value="abc" calls=[4-9]'
# The editor's text read back as a string (a Rope).
GK_PROTO_BIN=lists2-go run text 0 'lists2 text text="hello world" selected="world"'
# A DisplayMap made from a font (gk.Font): its reads and writes.
GK_PROTO_BIN=lists2-go run display-map 0 'lists2 display-map lines=[0-9]+ rows=[0-9]+ wraps=[0-9]+ text="" folded=false candidate=false'
# The editor's providers over the request channel (docs/PROTOCOLS.md 9.11): completions on a
# trigger character, document colors and semantic tokens on edits, an inline completion after
# its debounce, code actions on ctrl-. and the chosen one performed by the app.
GK_PROTO_BIN=lists2-go run providers 0 'lists2 provider completions id=' 'lists2 provider colors id=' \
  'lists2 provider tokens id=' 'lists2 provider inline id=' 'lists2 provider code-actions id=' \
  'lists2 provider perform id=fix-1 ' 'lists2 done mode=providers '
# Closures that map numbers to a number, as expressions the library evaluates (docs/PROTOCOLS.md
# 9.12): StepBy steps 9 -> 10 -> 20, and NumberStep's ByValue variant ("y == 1 ? x : 0.5": an
# increment doubles) 3 -> 6 -> 12. An expression that does not parse stops where it is given.
GK_PROTO_BIN=lists2-go run expr 0 'lists2 expr step_by="20" by_value="12"'
GK_PROTO_BIN=lists2-go run bad-expr 2 'the expression "x < 10 \? 1" does not parse: a \? without its :'
# Round 3 (docs/PROTOCOLS.md 9.13, 9.14).
# A list, a table and a tree selected and scrolled from code (generated state calls, and the list's
# scroll handle): the rows scrolled to are described, the selections read back.
GK_PROTO_BIN=lists2-go run drive 0 'lists2 drive before list=false table=false tree=false' \
  'lists2 drive selected list=500 table=700 tree=150 ok=true' 'lists2 drive scrolled list=true table=true tree=true' \
  'lists2 drive handle list900=true table0=true'
# Handlers given to calls (a builder's Validate, SetValidator twice) are dropped with their state.
GK_PROTO_BIN=lists2-go run kept 0 'lists2 kept added=3 dropped=true'
# A library object with no constructor but Default (StatusColors, SyntaxColors) and its reads. The
# colors themselves are upstream's: the component theme's own red and yellow, transparent in this
# test app, whose component theme is not synced from the base theme.
GK_PROTO_BIN=lists2-go run status-colors 0 \
  'lists2 status error_opaque=(true|false) error_not_warning=(true|false) created_is_success=true deleted_is_error=true border_is_error=true' \
  'lists2 status syntax keyword=false index0=false'
# Data closures answered between frames: a code block's highlights and an image's source, each
# asked once, cached, then answered from the cache.
GK_PROTO_BIN=lists2-go run ask 0 'lists2 ask highlight lang=odin code="proc x' 'lists2 ask image uri=pic.png' \
  'lists2 ask counts asked=2 answered=2 highlight=1 image=1 gets_more=true'
# A static that makes several values (DiffFile::parse): its count, a patch that does not parse
# (the kit's error), and each file by its index given to a diff state (PROTOCOLS.md 9.15).
GK_PROTO_BIN=lists2-go run diff 0 'lists2 diff count=2 ok=true bad_ok=false bad_err="[^"]+"' 'lists2 diff state made'
# Round 4 (docs/PROTOCOLS.md 9.16 to 9.18).
# An element closure over a context: the input's token drawn by a row of the app's, which reads the
# token's context (a copy behind a temporary handle); selecting the text hands the row a context
# that is selected, and the row is described again.
GK_PROTO_BIN=lists2-go run ctx-token 0 \
  'lists2 ctx token row=0 range=Range\{lo = 6, hi = 11\} selected=false disabled=false readonly=false label="World" height_ok=true' \
  'lists2 ctx token row=0 range=Range\{lo = 6, hi = 11\} selected=true '
# An object lent by kit state: the editor's diagnostics, two pushed with the turn's writes, given
# back when they have landed (the editor's own diagnostics read back in the next turn).
GK_PROTO_BIN=lists2-go run lend 0 'lists2 lend got=true empty=true' 'lists2 lend after len=2 lent=1 given_back=1'
# A delegate answered between frames: the editor's highlighter asks the app's styles and folds
# functions with the language, the text and a range, answered from the cache afterwards.
GK_PROTO_BIN=lists2-go run highlight 0 'lists2 highlight lang=odin range=Range\{lo = [0-9]+, hi = [0-9]+\} text="main :: proc' \
  'lists2 highlight folds lang=odin lines=3' 'lists2 highlight counts styles=true folds=true asked=[1-9][0-9]* answered=[1-9][0-9]* gets_more=true'
# The app's language service: the syntax function is asked where a typed quote sits.
GK_PROTO_BIN=lists2-go run languages 0 'lists2 syntax asked lang=odin offset=[0-9]+' 'lists2 languages syntax_asked=true'
# Misuse: a lent object used after the turn that lent it.
# Go: the app's line is in round4.go (Odin round4.odin).
GK_PROTO_BIN=lists2-go run bad-lend 2 'lists2 bad-lend using the lent handle' 'was released' 'at .*lists2/round4.go:[0-9]+'
# Odin's bad-ctx (gk.row_context for a row no closure drew, exit 2) has no Go twin: an item function
# is handed its context's values (gk.Items), so there is no read of a context to misuse; not counted.
# Small rules (PROTOCOLS.md 9.19): a patch that does not parse gives the kit's DiffParseError (its line
# and message), a good one none; BaseHighlightPrefix over an LSP item; a tree's scroll handle is gpui's
# ScrollHandle, scrolled from code: the rows it scrolls to are described.
GK_PROTO_BIN=lists2-go run small 0 'lists2 small parse-error ok=true line=1 message="hunk has no file header" good_has_error=false' \
  'lists2 small highlight_prefix=\[Range\{lo = 0, hi = 2\}\]' 'lists2 small scroll before y=0' 'lists2 small scroll after y=-[1-9][0-9.]* drawn_far=true'
# (PROTOCOLS.md 11.25): the combobox mode's Down and Enter would add
# Cherry, which the rules the app gave in advance block; the selection stays Banana.
GK_PROTO_BIN=lists2-go run will-change 0 'lists2 will-change values=\["banana"\]'
out=$lists2_out
