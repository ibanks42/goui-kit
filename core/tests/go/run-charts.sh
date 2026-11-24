# Chart helper layout tests covering pie arc calculation and stacked series geometry.
build charts-go charts

# Proves PieArcs preserves row order and values, spans a full 2*pi circle (6.283 rad),
# and respects half-circle end angles and pad gaps when configured.
GK_PROTO_BIN=charts-go run pie 0 \
  'overlay-charts pie arcs=3 rows=0,1,2 values=10,30,20' 'overlay-charts pie sweep=6.283[0-9] order=0,1,2' \
  'overlay-charts pie gap arcs=2 end=3.142' 'overlay-charts done mode=pie'
# Proves StackSeries accumulates y0..y1 intervals across keys (Apples then Pears) and
# supports named-series lookups with matching stacked baselines.
GK_PROTO_BIN=charts-go run stack 0 \
  'overlay-charts stack series=2 keys=Apples,Pears' 'overlay-charts stack pears jan=1..3 feb=3..7 rows=0,1' \
  'overlay-charts stack named feb=3..6' 'overlay-charts done mode=stack'
