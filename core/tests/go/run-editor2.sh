# run-editor2.sh: the protocol tests (docs/PROTOCOLS.md 9.20 to 9.40), sourced by core/tests/go/run.sh,
# which defines build and run. The app is core/tests/go/editor2, offscreen under gk-meter scripts. Odin
# twin: core/tests/run-editor2.sh. The logs go to a folder of their own ($out/editor2): some modes' names
# are other families' too, and run.sh keys a log by its mode's name.
build editor2-go editor2
editor2_out=$out; out=$out/editor2; mkdir -p "$out"

# Markdown plugins (9.23): a data plugin claims ```badge code blocks (each drawn as a row of the
# app's source, reading the node), another inline code (its row reads the node and the inherited
# typography); the kit's frontmatter plugin beside them. The library counts the nodes it claimed.
GK_PROTO_BIN=editor2-go run plugin 0 \
  'editor2 block row=[0-9]+ name=badge text="ready" markdown="```badge' \
  'editor2 inline row=[0-9]+ name=code text="inline" font_ok=true line_ok=true rem_ok=true' \
  'editor2 plugin parsed=[2-9][0-9]* blocks=[1-9][0-9]* inlines=[1-9][0-9]*'
# A link's tooltip (9.22): the URL's row is described when the mouse is on the link; the same URL
# again keeps its row.
GK_PROTO_BIN=editor2-go run tooltip 0 \
  'editor2 tooltip row=0 url=https://example.com/docs' 'editor2 tooltip view_rows=1 described=[1-9]'
# A block surface (9.20): the app's Div around the blockquote's blocks (pixels: gallery editor-surface).
GK_PROTO_BIN=editor2-go run surface 0 'editor2 done mode=surface'
# A predicate as a key set (9.21): History::retain keeps the entries the app names.
GK_PROTO_BIN=editor2-go run history 0 'editor2 history entries=\["b", "d"\]'
# An object a read-back value hands out (9.25): a merge conflict of a working file, its seven reads.
GK_PROTO_BIN=editor2-go run conflict 0 'editor2 conflict n=1' \
  'editor2 conflict lines=.* current=.* incoming=.* base=false/.* current_label="ours" incoming_label="theirs" base_label=false'
# An ElementId read back (9.24): its text.
GK_PROTO_BIN=editor2-go run annotation 0 'editor2 annotation id="ann-1" path="src/a.rs"'
# A fold's placeholder (9.22): upstream calls the closure without a window; the row of the folded
# line is described when it is laid out.
GK_PROTO_BIN=editor2-go run fold 0 \
  'editor2 fold row=[0-9]+ line=0' 'editor2 fold described=[1-9]'
# An EntityId argument (9.24): the entity behind an editor handle; any other handle stops.
GK_PROTO_BIN=editor2-go run owner 0 \
  'editor2 owner drawn' 'editor2 done mode=owner'
GK_PROTO_BIN=editor2-go run bad-owner 2 \
  'is not an input, a textarea or an editor'
# A record with a colour in a call op, decoded in place (9.26): an editor's decoration collection.
GK_PROTO_BIN=editor2-go run decorations 0 'editor2 decorations made ranges=\[Range\{lo = 0, hi = 5\}\]' \
  'editor2 decorations appended ranges=\[Range\{lo = 0, hi = 5\}, Range\{lo = 6, hi = 11\}\]' \
  'editor2 decorations cleared ranges=\[\]'
# The app's editor style (9.26): its highlight resolver is an ask the editor consults by name.
GK_PROTO_BIN=editor2-go run editor-style 0 'editor2 style asked name=keyword'
# Reads through a borrow (9.27): the editor's DisplayMap, which cannot be copied, read in place.
GK_PROTO_BIN=editor2-go run display-map 0 'editor2 display-map lines=[45] text="top \{\\nhidden\\n\}\\nlast\\n" rows=[45] folded=false hidden=false row1=1 folds=0'
# An action in a call op (9.26): routed to the editor's overlays, none open.
GK_PROTO_BIN=editor2-go run route 0 'editor2 route handled=false'
# A variant made by its constructor (9.29): MaskPattern's Pattern, typed through.
GK_PROTO_BIN=editor2-go run mask-variant 0 'editor2 mask-variant value="123-456"'

# Round 6 (docs/PROTOCOLS.md 9.30 to 9.37, core/tests/go/editor2/round6.go).
# The editor host (9.30): the app's blocks and parsed ones (held values) drawn by gk.BlockHost; each
# keyed leaf's geometry after the frame, its reads; delivered again only when the leaves change; a
# parsed table's rows read back as nested lists.
GK_PROTO_BIN=editor2-go run host 0 'editor2 host parsed ok=true n=3' \
  'editor2 leaf key=1 len=4 empty=false sized=true line=true caret=true' \
  'editor2 leaf key=2 len=11 empty=false sized=true line=true caret=true' \
  'editor2 leaf key=3 len=5 empty=false sized=true' 'editor2 leaf key=4 len=6 empty=false sized=true' \
  'editor2 leaf hit index=3 caret=3 same=true atom=false' \
  'editor2 host hosts=[1-9][0-9]* leaves=[1-9][0-9]* delivered=[1-3] events=[1-3]$' \
  'editor2 host table rows=1 cells=2 first="cell" aligns=\["Left", "Right"\]'
# A value node in a builder chain of a call op (9.31): decoded with the call's context.
GK_PROTO_BIN=editor2-go run chain 0 'editor2 chain inherit=true' 'editor2 chain drawn'
# Decorators kept for later (9.32): a number input's buttons get the app's style and rows.
GK_PROTO_BIN=editor2-go run decorate 0 'editor2 decorate decorated=[2-9][0-9]* described=[2-9][0-9]*'
# An edit of a value lent by &mut (9.33): the editor's Lsp, a completion highlighter set in place.
GK_PROTO_BIN=editor2-go run lsp 0 'editor2 lsp lent=1'
# A record holding a value node in a call op (9.34): the record's fields as the call's arguments.
GK_PROTO_BIN=editor2-go run diagnostic 0 \
  'editor2 diagnostic presented' 'editor2 done mode=diagnostic'
# An answer given in advance (9.36): the overlay actions the app takes; none open, nothing routed.
GK_PROTO_BIN=editor2-go run overlay-keys 0 'editor2 overlay-keys routed=false'
# An event payload made of its reads (9.35): the edits and the text after each.
GK_PROTO_BIN=editor2-go run edits 0 'editor2 edits start=2 old_end=2 new_end=3 text="abc"' \
  'editor2 edits start=2 old_end=3 new_end=2 text="ab"'
# Actions read back (9.37) have no run here: upstream hands on_context_menu's handler an empty
# NativeMenu (the app builds the items), and under the offscreen driver the handler did not fire after
# a right click (mode context-menu of round6.go prints events=0; PROTOCOLS.md 9.37).

# Round 7 (docs/PROTOCOLS.md 9.38 to 9.40, core/tests/go/editor2/round7.go).
# A parsed Custom block (9.39): the front matter the kit's plugin claims, a held value, placed among the
# app's blocks with its flow from FlowPosition::at over a list holding it; the leaf after it lies lower
# than in a host without it.
GK_PROTO_BIN=editor2-go run custom 0 'editor2 custom parsed ok=true n=2' \
  'editor2 custom leaves=4 events=[1-9][0-9]* placed=true gap_with=[0-9]+ gap_without=[0-9]+'
# Answers given in advance, fired by a key (9.36, 9.40): the completion menu the app presented is open,
# Enter is the app's: the overlay closes, the editor does nothing with it; afterwards Enter routes nowhere.
# Without the key set Enter is the editor's: a new line. (The base Editor: the kit's code editor installs
# its own overlay handler when it renders.)
GK_PROTO_BIN=editor2-go run overlay-fire 0 \
  'editor2 overlay-fire presented at=3 text="abx"' 'editor2 overlay-fire value="abx" routed_after=false taken=true'
GK_PROTO_BIN=editor2-go run overlay-control 0 \
  'editor2 overlay-control presented at=3 text="abx"' 'editor2 overlay-control value="abx\\n" routed_after=false taken=false'
# The Lsp's completion highlighter consulted by the open menu (9.33, 9.40): each item's label asked.
GK_PROTO_BIN=editor2-go run lsp-ask 0 'editor2 lsp-ask asked query="" label="first"' \
  'editor2 lsp-ask asked query="" label="second"' 'editor2 lsp-ask asked=[2-9]'
# A presented diagnostic (9.34, 9.40) read back as the editor's popover, then cleared (pixels: gallery
# editor7-diagnostic).
GK_PROTO_BIN=editor2-go run diagnostic-shown 0 \
  'editor2 diagnostic-shown popover=true' 'editor2 diagnostic-shown cleared popover=false'

out=$editor2_out
