# run-items.sh: item functions (docs/PROTOCOLS.md 9.41), sourced by core/tests/go/run.sh, which defines
# build and run. The app is core/tests/go/items, offscreen with the diag library.
build items-go items
# An element the kit built for each day, decorated by the app's function with the record of the
# day's state; the rows are described in the first frames and kept (none described after them).
GK_PROTO_BIN=items-go run items-calendar 0 \
  'items calendar described=[0-9]+ days=(2[89]|3[0-9]|4[0-2]) weekdays=7 active=[01] after=0' \
  'items done mode=items-calendar'
# A time field's segments: the handed segment placed inside the app's own element.
GK_PROTO_BIN=items-go run items-time-field 0 \
  'items segment=Hour value=[0-9]+ selected=false' 'items segment=Minute value=[0-9]+' \
  'items time field described=[2-9] hours=1 minutes=1'
# A tree's entries: the index (the row's key), the entry's reads and its interaction state as records.
GK_PROTO_BIN=items-go run items-tree 0 \
  'items tree row ix=0 label=Alpha depth=0 folder=false selected=false' \
  'items tree row ix=1 label=Beta depth=0 folder=false selected=false' 'items done mode=items-tree described=2 '
# A popover's content over the popover's own state (handed with its own context). Upstream calls the
# closure twice a frame (two rows, each described once).
GK_PROTO_BIN=items-go run items-popover 0 'items popover content is_open=true' 'items done mode=items-popover described=[12] '
# A command palette's header and footer over the palette's state (handed with the app's context).
GK_PROTO_BIN=items-go run items-command 0 'items command header matched=0 loading=false selected=false' \
  'items done mode=items-command described=2 '
# A closure that returns one element type: a base hover card's content (a StatefulDivEl), opened by
# the pointer.
GK_PROTO_BIN=items-go run items-hover 0 'items hover content is_open=true' 'items done mode=items-hover described=[1-9]'
# FnOnce(Button) -> E: the dialog's close button built from the app's ops.
GK_PROTO_BIN=items-go run items-close 0 'items done mode=items-close described=0 builds=1 '
# A generic context read as a record (the binding's search delegate's items): the selection and the
# placeholder the app gave.
GK_PROTO_BIN=items-go run items-combobox 0 'items combobox trigger selected=1 first=Pear placeholder=Fruit open=false'
# A nav stack's page: the handed page with the record of its reads (settled: progress 1).
GK_PROTO_BIN=items-go run items-nav 0 'items nav page index=0 phase=[A-Za-z_]+ progress=1.0'
# Misuse: a time field's function given to a calendar stops where the builder is given it (exit 2).
# Go: the message names gk.Items and Go's function types; the kinds are matched loosely.
GK_PROTO_BIN=items-go run items-wrong-kind 2 'gk.Items was given a .*TimeFieldSegment'
