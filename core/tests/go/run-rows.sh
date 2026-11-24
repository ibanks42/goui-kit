# run-rows.sh: the rows family's protocol tests (docs/PROTOCOLS.md section 9.5), sourced by
# core/tests/go/run.sh, which defines build and run: the kit's list, table and tree over row
# sources, offscreen. Each check is one mode of core/tests/go/rows: its exit code and the lines it
# printed. examples/go/proto's mode list (gates/go-proto.sh) is
# an older port of the first check, with "proto" lines.
build rows-go rows
# A list: the first frame complete; a click selects and confirms, Down selects the next row, a
# right click and Escape reach the app, all as queued events; a far scroll draws placeholders
# for exactly one frame and one ROWS turn binds them; back at the top nothing is described
# again.
# Go: the delegate's kind is RightClick; the state's events print as Go values
# (ListEventSelect{...}).
GK_PROTO_BIN=rows-go run list 0 \
  'rows list Select has=true section=0 row=0 ' 'rows list Confirm has=false .*secondary=false' \
  'rows list Select has=true section=0 row=1 ' 'rows list RightClick has=true section=0 row=[0-9]+ ' \
  'rows list Cancel ' 'rows list-state ListEventSelect' 'rows list-state ListEventConfirm' \
  'rows list-state read selected_index ok=true row=0' 'placeholder_frames: 0 ' 'placeholder_frames: 1 ' 'rows_turns: [1-9],' \
  'rows done mode=list .* again=0$'
# A searchable list: the app's initial view while the query is empty; typing sends
# perform_search as a request; the app filters, sets the count and answers; the list draws the
# matches.
GK_PROTO_BIN=rows-go run list-search 0 'rows initial view described' \
  'rows list Search .*id=[1-9][0-9]* query="77"' 'rows search answered query="77" matches=280' \
  'rows done mode=list-search '
# Sections with header and footer rows (rows HEADER and FOOTER of each section).
GK_PROTO_BIN=rows-go run list-sections 0 \
  'rows header section=0' 'rows footer section=0' 'rows header section=1' 'rows footer section=1' \
  'rows done mode=list-sections '
# The views the app gives as data: empty, loading; load_more when scrolled near the end.
# Go: the kind is LoadMore.
GK_PROTO_BIN=rows-go run list-states 0 \
  'rows empty view described n=1' 'rows loading view described n=1' 'rows list LoadMore ' \
  'rows done mode=list-states '
# A table of 100,000 rows whose cells are views: a click and Down select rows, End jumps to the
# last row with one placeholder frame, the generated scroll_to_row jumps to row 50,000, a click on
# the sort icon sorts (the app invalidates and the cells are described again; the click also
# selects the column, so the selected row it reads is none), a double click, a column moved and one resized, visible ranges.
# Go: the delegate's kinds are VisibleRows and MoveColumn; the
# state's event prints as a Go value (TableEventSelectColumn{Value:0}).
GK_PROTO_BIN=rows-go run table 0 \
  'rows table Select_Row row=0$' 'rows table Select_Row row=1$' 'rows table Select_Row row=99999$' \
  'rows table VisibleRows row=0 ' 'rows table VisibleRows row=999[0-9][0-9] ' \
  'placeholder_frames: 0 ' 'placeholder_frames: 1 ' \
  'rows table Sort row=0 col=0 to=0 sort=Descending' 'rows table state TableEventSelectColumn\{Value:0\}' 'rows table-state read selected_row ok=false' \
  'rows table-state scroll_to_row 50000' 'rows table VisibleRows row=500[0-9][0-9] ' 'rows table Double_Clicked_Row ' \
  'rows table MoveColumn row=0 col=1 ' 'rows table Column_Widths ' 'rows done mode=table '
# A table scrolled by 24 px before each of 120 frames: rows are bound ahead in batches, so no
# frame draws a placeholder, and the visible rows reach row 80 and more.
GK_PROTO_BIN=rows-go run table-scroll 0 \
  'rows table VisibleRows row=(8[0-9]|9[0-9]) ' 'placeholder_frames: 0 ' 'rows done mode=table-scroll .*again=0$'
# A tree: each entry a row view that reads its entry; expanding a folder describes the rows again.
# Go: the state's event prints as a Go value (TreeEventExpanded{Value:"docs"}).
GK_PROTO_BIN=rows-go run tree 0 \
  'rows tree row=3 id=docs depth=0 folder=true expanded=false' \
  'rows tree row=3 id=docs depth=0 folder=true expanded=true' 'rows tree row=4 id=docs/c depth=1' \
  'rows tree-state TreeEventExpanded\{Value:"docs"\} selected ok=true ix=3' \
  'rows done mode=tree '
# A row source released while a view still draws its list stops the program (exit 2).
GK_PROTO_BIN=rows-go run released 2 'rows released source' \
  'a view shows a row source through a handle that cannot be used'
