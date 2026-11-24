# Virtualized List, DataTable, and Tree protocol tests covering row paging,
# search, sections, empty/loading states, sorting, column reorder, and scrolling.
build rows-go rows

# Proves virtualized List fires Select, Confirm, RightClick, and Cancel events, syncs ListState
# selection readback, renders both cached (placeholder_frames: 0) and paging frames, and avoids extra renders.
GK_PROTO_BIN=rows-go run list 0 \
  'rows list Select has=true section=0 row=0 ' 'rows list Confirm has=false .*secondary=false' \
  'rows list Select has=true section=0 row=1 ' 'rows list RightClick has=true section=0 row=[0-9]+ ' \
  'rows list Cancel ' 'rows list-state ListEventSelect' 'rows list-state ListEventConfirm' \
  'rows list-state read selected_index ok=true row=0' 'placeholder_frames: 0 ' 'placeholder_frames: 1 ' 'rows_turns: [1-9],' \
  'rows done mode=list .* again=0$'
# Proves List search forwards query "77" asynchronously and answers with all 280 matching rows.
GK_PROTO_BIN=rows-go run list-search 0 'rows initial view described' \
  'rows list Search .*id=[1-9][0-9]* query="77"' 'rows search answered query="77" matches=280' \
  'rows done mode=list-search '
# Proves sectioned Lists invoke section header and footer callbacks for sections 0 and 1.
GK_PROTO_BIN=rows-go run list-sections 0 \
  'rows header section=0' 'rows footer section=0' 'rows header section=1' 'rows footer section=1' \
  'rows done mode=list-sections '
# Proves List empty-state view, loading-state view, and LoadMore trigger each fire when reached.
GK_PROTO_BIN=rows-go run list-states 0 \
  'rows empty view described n=1' 'rows loading view described n=1' 'rows list LoadMore ' \
  'rows done mode=list-states '

# Proves 100,000-row DataTable supports keyboard jumps (0 -> 1 -> 99999 -> 50000), column sort,
# column selection, double-click, column reorder, and width changes while paging rows on demand.
GK_PROTO_BIN=rows-go run table 0 \
  'rows table Select_Row row=0$' 'rows table Select_Row row=1$' 'rows table Select_Row row=99999$' \
  'rows table VisibleRows row=0 ' 'rows table VisibleRows row=999[0-9][0-9] ' \
  'placeholder_frames: 0 ' 'placeholder_frames: 1 ' \
  'rows table Sort row=0 col=0 to=0 sort=Descending' 'rows table state EventSelectColumn\{Value:0\}' 'rows table-state read selected_row ok=false' \
  'rows table-state scroll_to_row 50000' 'rows table VisibleRows row=500[0-9][0-9] ' 'rows table Double_Clicked_Row ' \
  'rows table MoveColumn row=0 col=1 ' 'rows table Column_Widths ' 'rows done mode=table '
# Proves smooth table scrolling binds rows ahead so rows 80..99 appear with zero placeholder frames (again=0).
GK_PROTO_BIN=rows-go run table-scroll 0 \
  'rows table VisibleRows row=(8[0-9]|9[0-9]) ' 'placeholder_frames: 0 ' 'rows done mode=table-scroll .*again=0$'

# Proves Tree folder expansion flips row 3 ("docs") from expanded=false to expanded=true,
# exposes child row 4 ("docs/c") at depth 1, and emits TreeEventExpanded.
GK_PROTO_BIN=rows-go run tree 0 \
  'rows tree row=3 id=docs depth=0 folder=true expanded=false' \
  'rows tree row=3 id=docs depth=0 folder=true expanded=true' 'rows tree row=4 id=docs/c depth=1' \
  'rows tree-state TreeEventExpanded\{Value:"docs"\} selected ok=true ix=3' \
  'rows done mode=tree '
# Proves rendering a released row source fails cleanly with exit 1 instead of dereferencing stale state.
GK_PROTO_BIN=rows-go run released 1 'rows released source' \
  'a view shows a row source through a handle that cannot be used'
