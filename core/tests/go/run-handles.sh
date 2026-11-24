# run-handles.sh: the protocol tests (docs/PROTOCOLS.md section 13), sourced by
# core/tests/go/run.sh, which defines build and run. The app is core/tests/go/handles: library
# objects that are not entities behind handles of kind OBJ, driven by a gk-meter script offscreen.
build handles-go handles

# Owned objects: made (kept), read, written (buffered; the next read sees it), released.
GK_PROTO_BIN=handles-go run obj-objects 0 \
  'handles counts before temps=0 live=0' \
  'handles dock open=true size=240' \
  'handles dock written open=false size=300 resizing=true' \
  'handles position path=src/a.rs side=Modified line=7' \
  'handles matcher len=0 empty=true label=' \
  'handles counts made temps=0 live=3' \
  'handles done mode=obj-objects clicks=4'
# A read hands out a temporary handle, swept when the next frame begins; gk.Retain keeps one.
GK_PROTO_BIN=handles-go run obj-temps 0 \
  'handles temp placeholder="type here" editable=true multi_line=false' \
  'handles counts handed out temps=1 live=2' \
  'handles counts next frame temps=0 live=1' \
  'handles kept placeholder="type here"' \
  'handles done mode=obj-temps clicks=4'
# Misuse: a temporary handle used after its frame stops with the message and the line (gk.Run returns it: exit 1).
# Go: the app's line is main.go's (the library's message may name gk.retain or gk.Retain).
GK_PROTO_BIN=handles-go run obj-stale-temp 1 'handles reading the swept temporary' \
  'was released .*temporary: it lives until the next frame begins unless gk.[Rr]etain keeps it' 'at .*handles/main.go:[0-9]+'
# The kit's theme global: one interned handle for both accessors, reads in place, in a view too.
GK_PROTO_BIN=handles-go run obj-global 0 \
  'handles theme same=true dark=false name="[^"]+" radius_full=[0-9]+' 'handles registry ok=true' \
  'handles counts globals temps=0 live=2' 'handles done mode=obj-global clicks=4'
# A dialog's handle opened and closed from code.
GK_PROTO_BIN=handles-go run obj-dialog 0 'handles dialog open=false' 'handles dialog open=true' \
  'handles dialog closed open=false'
# A list selected and scrolled from code (an IndexPath as its record; the scroll handle an object).
GK_PROTO_BIN=handles-go run obj-list 0 'handles list selected ok=true row=120' 'handles list cleared selected=false'
# A method that takes its object by value gets a copy when the type is Clone: the handle stays.
GK_PROTO_BIN=handles-go run obj-given-away 0 'handles counts installed twice temps=0 live=1' 'handles done mode=obj-given-away clicks=4'
# Builder values made in a handler (no view) and given to call ops: a tree's items, a grid's
# areas, each a tape of its own; read back through a read that hands out a library object.
GK_PROTO_BIN=handles-go run obj-values 0 \
  'handles tree entry 0 id=src depth=0 folder=true' 'handles tree entry 1 id=src/a.rs depth=1 folder=false' \
  'handles tree entries=4' 'handles grid area id=one col=1 row=[0-9]+ col_span=3' 'handles grid area id=two '
# Misuse: a value kept past the turn that made it stops where it is given (gk.Run returns it: exit 1).
# Go: the message may say "the function that made it" or "the procedure".
GK_PROTO_BIN=handles-go run obj-stale-value 1 'handles giving a value of an earlier turn' 'a kit value was used after the (procedure|function) that made it returned'
# A table's columns given as the kit's own Column values (each a value made outside a view), then
# replaced on the table state.
GK_PROTO_BIN=handles-go run obj-columns 0 'handles columns n=2 first=Name second=Size width0=140' 'handles columns replaced n=3'
# Lifetimes (docs/GO.md section 4, core/tests/go/handles/life.go). What a view makes in its turns
# (here its click handler: kit state, a focus handle, a row source and its rows, a view) is
# released with the view.
GK_PROTO_BIN=handles-go run life-view 0 \
  'handles life before states=0 views=1 focus=0' 'handles life child made its state' \
  'handles life made states=1 views=6 focus=1' 'handles life released states=0 views=1 focus=0' \
  'handles done mode=life-view clicks=4'
# Misuse: releasing what a view released with itself stops at the app's line (exit 1).
GK_PROTO_BIN=handles-go run life-view-again 1 'handles life releasing the child.s state again' \
  'gk.Release was given a handle that was released already, with the view \(or dock panel\) that made it' \
  'at .*handles/life.go:[0-9]+'
# A dock panel the dock removed (as when the user closes its tab), and not added back, is released
# with its view and what the view made; the other panel stays.
GK_PROTO_BIN=handles-go run life-panel 0 \
  'handles life panel 0 made its state' 'handles life docked states=4 views=7 focus=1' \
  'handles life closed states=2 views=2 focus=0' 'handles life dock empty=false' 'handles done mode=life-panel'
# Kit state and focus handles the app drops are released once the collector finds them (runtime.GC
# in the handler; the release runs on the UI thread, in a WAKE turn); the one kept is not.
GK_PROTO_BIN=handles-go run life-gc 0 \
  'handles life made states=4 views=1 focus=3' 'handles life collected states=1 views=1 focus=0' \
  'handles life gc released=6 kept="kept"' 'handles done mode=life-gc clicks=4'
