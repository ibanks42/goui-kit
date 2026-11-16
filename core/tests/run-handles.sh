# run-handles.sh: the protocol tests (docs/PROTOCOLS.md section 13), sourced by
# core/tests/run.sh, which defines build and run. The app is core/tests/handles: library objects
# that are not entities behind handles of kind OBJ, driven by a gk-meter script offscreen.
build handles-odin "$here/handles"

# Owned objects: made (kept), read, written (buffered; the next read sees it), released.
GK_PROTO_BIN=handles-odin run obj-objects 0 \
  'handles counts before temps=0 live=0' \
  'handles dock open=true size=240' \
  'handles dock written open=false size=300 resizing=true' \
  'handles position path=src/a.rs side=Modified line=7' \
  'handles matcher len=0 empty=true label=' \
  'handles counts made temps=0 live=3' \
  'handles done mode=obj-objects clicks=4'
# A read hands out a temporary handle, swept when the next frame begins; gk.retain keeps one.
GK_PROTO_BIN=handles-odin run obj-temps 0 \
  'handles temp placeholder="type here" editable=true multi_line=false' \
  'handles counts handed out temps=1 live=2' \
  'handles counts next frame temps=0 live=1' \
  'handles kept placeholder="type here"' \
  'handles done mode=obj-temps clicks=4'
# Misuse: a temporary handle used after its frame stops with the message and the line (exit 2).
GK_PROTO_BIN=handles-odin run obj-stale-temp 2 'handles reading the swept temporary' \
  'was released .*temporary: it lives until the next frame begins unless gk.retain keeps it' 'at .*handles/main.odin:[0-9]+'
# The kit's theme global: one interned handle for both accessors, reads in place, in a view too.
GK_PROTO_BIN=handles-odin run obj-global 0 \
  'handles theme same=true dark=false name="[^"]+" radius_full=[0-9]+' 'handles registry ok=true' \
  'handles counts globals temps=0 live=2' 'handles done mode=obj-global clicks=4'
# A dialog's handle opened and closed from code.
GK_PROTO_BIN=handles-odin run obj-dialog 0 'handles dialog open=false' 'handles dialog open=true' \
  'handles dialog closed open=false'
# A list selected and scrolled from code (an IndexPath as its record; the scroll handle an object).
GK_PROTO_BIN=handles-odin run obj-list 0 'handles list selected ok=true row=120' 'handles list cleared selected=false'
# A method that takes its object by value gets a copy when the type is Clone: the handle stays.
GK_PROTO_BIN=handles-odin run obj-given-away 0 'handles counts installed twice temps=0 live=1' 'handles done mode=obj-given-away clicks=4'
# Builder values made in a handler (no view) and given to call ops: a tree's items, a grid's
# areas, each a tape of its own; read back through a read that hands out a library object.
GK_PROTO_BIN=handles-odin run obj-values 0 \
  'handles tree entry 0 id=src depth=0 folder=true' 'handles tree entry 1 id=src/a.rs depth=1 folder=false' \
  'handles tree entries=4' 'handles grid area id=one col=1 row=[0-9]+ col_span=3' 'handles grid area id=two '
# Misuse: a value kept past the turn that made it stops where it is given (exit 2).
GK_PROTO_BIN=handles-odin run obj-stale-value 2 'handles giving a value of an earlier turn' 'a kit value was used after the procedure that made it returned'
# A table's columns given as the kit's own Column values (each a value made outside a view), then
# replaced on the table state.
GK_PROTO_BIN=handles-odin run obj-columns 0 'handles columns n=2 first=Name second=Size width0=140' 'handles columns replaced n=3'
