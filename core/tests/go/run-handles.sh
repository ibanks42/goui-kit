# Handle and lifetime protocol tests: owned objects, temporary handles and
# next-frame sweeps, globals, transient kit values, view/panel cascades, and GC cleanup.
build handles-go handles

# Proves permanent HK_OBJ handles support field reads/writes and are tracked in live counts
# (0 -> 3) without creating any temporary handles.
GK_PROTO_BIN=handles-go run obj-objects 0 \
  'handles counts before temps=0 live=0' \
  'handles dock open=true size=240' \
  'handles dock written open=false size=300 resizing=true' \
  'handles position path=src/a.rs side=Modified line=7' \
  'handles matcher len=0 empty=true label=' \
  'handles counts made temps=0 live=3' \
  'handles done mode=obj-objects clicks=4'
# Proves unretained temporary handles are swept on the next frame (temps 1 -> 0, live 2 -> 1)
# while a handle promoted with gk.Retain survives and remains readable.
GK_PROTO_BIN=handles-go run obj-temps 0 \
  'handles temp placeholder="type here" editable=true multi_line=false' \
  'handles counts handed out temps=1 live=2' \
  'handles counts next frame temps=0 live=1' \
  'handles kept placeholder="type here"' \
  'handles done mode=obj-temps clicks=4'
# Proves dereferencing an unretained temporary handle after the next-frame sweep fails cleanly
# with an explanation of temporary lifetimes and exits 1 at the Go call site.
GK_PROTO_BIN=handles-go run obj-stale-temp 1 'handles reading the swept temporary' \
  'was released .*temporary: it lives until the next frame begins unless gk.[Rr]etain keeps it' 'at .*handles/main.go:[0-9]+'

# Proves GPUI Global singletons (Theme, ThemeRegistry) return the same handle identity across lookups
# and are tracked as permanent live handles rather than temporaries.
GK_PROTO_BIN=handles-go run obj-global 0 \
  'handles theme same=true dark=false name="[^"]+" radius_full=[0-9]+' 'handles registry ok=true' \
  'handles counts globals temps=0 live=2' 'handles done mode=obj-global clicks=4'
# Proves DialogState open/close handle state transitions round-trip false -> true -> false.
GK_PROTO_BIN=handles-go run obj-dialog 0 'handles dialog open=false' 'handles dialog open=true' \
  'handles dialog closed open=false'
# Proves ListState handle selection reads back row 120 and resets cleanly when cleared.
GK_PROTO_BIN=handles-go run obj-list 0 'handles list selected ok=true row=120' 'handles list cleared selected=false'
# Proves passing a clonable owned object twice into builders keeps the handle live (live=1)
# instead of consuming or duplicating it.
GK_PROTO_BIN=handles-go run obj-given-away 0 'handles counts installed twice temps=0 live=1' 'handles done mode=obj-given-away clicks=4'
# Proves transient kit values returned from constructors (tree entries, grid areas) decode
# correctly within the same turn.
GK_PROTO_BIN=handles-go run obj-values 0 \
  'handles tree entry 0 id=src depth=0 folder=true' 'handles tree entry 1 id=src/a.rs depth=1 folder=false' \
  'handles tree entries=4' 'handles grid area id=one col=1 row=[0-9]+ col_span=3' 'handles grid area id=two '
# Proves reusing a transient kit value after the turn that produced it fails with a clear misuse error and exits 1.
GK_PROTO_BIN=handles-go run obj-stale-value 1 'handles giving a value of an earlier turn' 'a kit value was used after the (procedure|function) that made it returned'
# Proves table column-descriptor lists can be inspected (n=2, Name/Size, width 140) and replaced with 3 columns.
GK_PROTO_BIN=handles-go run obj-columns 0 'handles columns n=2 first=Name second=Size width0=140' 'handles columns replaced n=3'

# Proves releasing a child view automatically cascades release to its child views, state, and focus handles,
# restoring counts from states=1/views=6/focus=1 back to states=0/views=1/focus=0.
GK_PROTO_BIN=handles-go run life-view 0 \
  'handles life before states=0 views=1 focus=0' 'handles life child made its state' \
  'handles life made states=1 views=6 focus=1' 'handles life released states=0 views=1 focus=0' \
  'handles done mode=life-view clicks=4'
# Proves explicitly releasing a state that was already cascade-released with its owning view exits 1
# with a diagnostic naming the view/panel cascade and the Go call site.
GK_PROTO_BIN=handles-go run life-view-again 1 'handles life releasing the child.s state again' \
  'gk.Release was given a handle that was released already, with the view \(or dock panel\) that made it' \
  'at .*handles/life.go:[0-9]+'
# Proves closing a docked panel cascades release to the panel's owned states, views, and focus handles
# while leaving the remaining dock panel intact (empty=false).
GK_PROTO_BIN=handles-go run life-panel 0 \
  'handles life panel 0 made its state' 'handles life docked states=4 views=7 focus=1' \
  'handles life closed states=2 views=2 focus=0' 'handles life dock empty=false' 'handles done mode=life-panel'
# Proves Go garbage collection finalizers release unreachable state and focus handles (6 released)
# while preserving handles still referenced by Go code (kept="kept").
GK_PROTO_BIN=handles-go run life-gc 0 \
  'handles life made states=4 views=1 focus=3' 'handles life collected states=1 views=1 focus=0' \
  'handles life gc released=6 kept="kept"' 'handles done mode=life-gc clicks=4'
