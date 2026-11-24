# Row/item callback context tests for calendar, time field, tree, popover,
# command palette, hover card, combobox, navigation stack, and wrong-kind checks.
build items-go items

# Proves Calendar item builders receive 28..42 day cells and 7 weekday headers, and that
# the handed-item context clears to 0 after rendering completes.
GK_PROTO_BIN=items-go run items-calendar 0 \
  'items calendar described=[0-9]+ days=(2[89]|3[0-9]|4[0-2]) weekdays=7 active=[01] after=0' \
  'items done mode=items-calendar'
# Proves TimeField segment renderers receive Hour and Minute segment descriptors with current values.
GK_PROTO_BIN=items-go run items-time-field 0 \
  'items segment=Hour value=[0-9]+ selected=false' 'items segment=Minute value=[0-9]+' \
  'items time field described=[2-9] hours=1 minutes=1'
# Proves Tree row renderers receive index, label, depth, folder, and selection fields for Alpha and Beta.
GK_PROTO_BIN=items-go run items-tree 0 \
  'items tree row ix=0 label=Alpha depth=0 folder=false selected=false' \
  'items tree row ix=1 label=Beta depth=0 folder=false selected=false' 'items done mode=items-tree described=2 '

# Proves Popover content item closures observe is_open=true when opened.
GK_PROTO_BIN=items-go run items-popover 0 'items popover content is_open=true' 'items done mode=items-popover described=[12] '
# Proves Command header item closures receive matched count, loading, and selection state.
GK_PROTO_BIN=items-go run items-command 0 'items command header matched=0 loading=false selected=false' \
  'items done mode=items-command described=2 '
# Proves HoverCard content item closures observe is_open=true when hovered open.
GK_PROTO_BIN=items-go run items-hover 0 'items hover content is_open=true' 'items done mode=items-hover described=[1-9]'
# Proves a closed popover does not invoke its content view (described=0) even though the outer widget built once.
GK_PROTO_BIN=items-go run items-close 0 'items done mode=items-close described=0 builds=1 '
# Proves Combobox trigger item closures report selected count, first label, placeholder, and open state.
GK_PROTO_BIN=items-go run items-combobox 0 'items combobox trigger selected=1 first=Pear placeholder=Fruit open=false'
# Proves NavigationStack page item closures receive page index 0, transition phase, and progress=1.0.
GK_PROTO_BIN=items-go run items-nav 0 'items nav page index=0 phase=[A-Za-z_]+ progress=1.0'

# Proves passing a mismatched handed-item context type (TimeFieldSegment) to overlay.Items fails cleanly with exit 1.
GK_PROTO_BIN=items-go run items-wrong-kind 1 'overlay.Items was given a .*TimeFieldSegment'
