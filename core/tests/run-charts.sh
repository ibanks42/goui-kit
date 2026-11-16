# run-charts.sh: the charts' calculators over the app's rows (docs/PROTOCOLS.md 12.15), sourced by
# core/tests/run.sh, which defines build and run. The app is core/tests/charts, offscreen.
build charts-odin "$here/charts"

# A pie over three rows: one arc per row, each its row's index, the angles summing to a turn, the
# rows' order kept (upstream does not sort); a gap column gives no arc for the gap.
GK_PROTO_BIN=charts-odin run pie 0 \
  'overlay-charts pie arcs=3 rows=0,1,2 values=10,30,20' 'overlay-charts pie sweep=6.283[0-9] order=0,1,2' \
  'overlay-charts pie gap arcs=2 end=3.142' 'overlay-charts done mode=pie'
# A stack of two keys over two rows: value("") reads the column each key names, so the pears band
# sits on the apples one; a named column is read for every key.
GK_PROTO_BIN=charts-odin run stack 0 \
  'overlay-charts stack series=2 keys=apples,pears' 'overlay-charts stack pears jan=1..3 feb=3..7 rows=0,1' \
  'overlay-charts stack named feb=3..6' 'overlay-charts done mode=stack'
