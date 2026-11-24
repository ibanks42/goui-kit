# Value-type, theme, scale, motion, date/calendar, scroll, drag, settings,
# questionnaire, background/gradient, matcher, and resize-handle protocol tests.
build values-go values

# Proves Band, Linear, Point, and Ordinal scales map domains to pixel/color ranges accurately,
# flag missing/out-of-range lookups, and resolve nearest band indices.
GK_PROTO_BIN=values-go run val-scales 0 \
  'values band width_ok=true step_ok=true order=true missing=true nearest=2' \
  'values linear t0=0 t50=100 t100=200' \
  'values point first=0 last=100 out_of_range=true' \
  'values ordinal found=true hue=0.66'
# Proves motion Keyframes interpolate midpoint=5.0, reject empty keyframes with an error,
# Discrete animations snap across the threshold (1 -> 2), and Sequences preserve start and step count.
GK_PROTO_BIN=values-go run val-motion 0 \
  'values keyframes ok=true len=2 mid=5.0' 'values keyframes empty ok=false err=true' \
  'values discrete before=1 after=2' 'values discrete switch ok=true' 'values sequence from=1 steps=1'
# Proves ThemeRegistry lists built-in themes, rejects invalid theme JSON, parses valid colors/configs,
# and switches between dark (font_size=17) and light modes cleanly.
GK_PROTO_BIN=values-go run val-theme 0 \
  'values registry themes=true dark=false' 'values registry bad json refused=true' \
  'values parse ok=true red=true bad_ok=false bad_err=true' 'values config made=true' \
  'values theme dark=true font_size=17' 'values theme light again dark=false'
# Proves ActionOf returns identical IDs for repeated lookups of the same built-in action and distinct IDs for others.
GK_PROTO_BIN=values-go run val-actions 0 'values actions same=true builtin=true other=true'
# Proves view-scoped pins survive unchanged frames (later=0), move into the deferred retirement queue
# on rebuild (later=1), and release on the following frame (later=0).
GK_PROTO_BIN=values-go run val-pins 0 \
  'values pins click=1 builds=1 pinned=1 later=0' 'values pins click=2 builds=1 pinned=1 later=0' \
  'values pins click=3 builds=2 pinned=1 later=1' 'values pins click=4 builds=2 pinned=1 later=0' \
  'values done mode=val-pins clicks=4 builds=2'
# Proves passing a value handle as an argument to its own builder is rejected with exit 1.
GK_PROTO_BIN=values-go run val-self-arg 1 'values giving a value to its own builder' \
  'a value was given as an argument to its own builder'
# Proves calling ActionOf inside a view function is rejected with exit 1 pointing at the Go call site.
GK_PROTO_BIN=values-go run val-action-in-view 1 'values naming an action in a view' \
  'gk.ActionOf in a view function' 'at .*values/actions.go:[0-9]+'

# Proves lazy clipboard copy closures run when triggered and write "copied by the app" to the clipboard.
GK_PROTO_BIN=values-go run val-clipboard 0 'values clipboard value asked' \
  'values clipboard click=[12] ok=true text="copied by the app"'
# Proves leap-day dates (2024-02-29), calendar grid weeks, date-range selection, DateTimePicker,
# and Time values encode and round-trip accurately.
GK_PROTO_BIN=values-go run val-dates 0 \
  'values calendar date=2024-02-29 today_ok=true weeks=[56] week_len=7' \
  'values calendar range applied=(true|false) start=2026-01-05 end=9' \
  'values picker date_time=2026-11-11 09:30:15' 'values time 23:59:00'
# Proves an impossible calendar date (2026-02-30) is rejected by validation and exits 1.
GK_PROTO_BIN=values-go run val-bad-date 1 'values writing February 30' \
  'the date 2026-02-30 does not exist'
# Proves ScrollHandle reports content taller than the 256px viewport and applies programmatic offset -200.
GK_PROTO_BIN=values-go run val-scroll 0 \
  'values scroll content_taller=true viewport_h=256 offset=0' 'values scroll offset=-200'
# Proves Questionnaire answer, item definition, and full answers-map readback match the chosen "blue" option.
GK_PROTO_BIN=values-go run val-readback 0 \
  'values answer set_failed=false ok=true choices=1 first=blue empty=false' \
  'values definition name=color required=true choices=2 second=blue' \
  'values answers len=1 get=true first=blue'

# Proves handed values preserve handle identity and kind metadata when passed through value callbacks.
GK_PROTO_BIN=values-go run val-handed 0 'values handed hue_max=[0-9]+ same_handle=true value_kind='
# Proves ambient locale and theme-font getters return valid non-empty settings inside views.
GK_PROTO_BIN=values-go run val-ambient 0 'values ambient locale=[a-zA-Z_-]+ theme_font=true'
# Proves custom Tooltip views describe once when hovered.
GK_PROTO_BIN=values-go run val-tooltip 0 'values tooltip view described n=1' 'values done mode=val-tooltip'
# Proves mutable ValueCell handles hold and update bool, string, and float values across turns.
GK_PROTO_BIN=values-go run val-cells 0 \
  'values cells made b=true s=first f=1.5' 'values cells again b=false s=second f=2.25'
# Proves repeated lookups of the same GPUI entity return a single deduplicated non-zero handle (distinct=1).
GK_PROTO_BIN=values-go run val-one-handle 0 \
  'values one handle same=true again=true distinct=1 nonzero=true'
# Proves BoxShadow wire encoding round-trips theme tag, HSLA color, offset (1,2), blur (3), and spread (4).
GK_PROTO_BIN=values-go run val-box-shadow 0 'values box shadow theme=0 h=0.50 l=0.25 x=1 y=2 blur=3 spread=4'
# Proves drag payload values preserve id=7 and label "row 7" across drop delivery.
GK_PROTO_BIN=values-go run val-drag 0 'values drop id=7 text=row 7'
# Proves settings switch toggles report old/new state (false -> true) and settings fragment clicks see dark=true.
GK_PROTO_BIN=values-go run val-settings 0 'values settings switch set=true was=false' \
  'values settings fragment click dark=true'
# Proves Questionnaire emits AnswerChanged for item=color/choice=blue followed by Submit.
GK_PROTO_BIN=values-go run val-quest-events 0 'values quest answer_changed item=color choice=blue' 'values quest submit'

# Proves Background wire codec round-trips Solid and Linear_Gradient stops/angles and rejects malformed inputs.
GK_PROTO_BIN=values-go run c6-background 0 \
  'values c6 background solid ok=true kind=Solid l=0.50 a=1.00' \
  'values c6 background gradient ok=true kind=Linear_Gradient angle=90 from_h=0.00 to_h=0.667 to_pct=1.00' \
  'values c6 background bad ok=false has=false err=true'
# Proves all four DateMatcher variants (interval, range, custom date list, weekdays) encode and read back accurately.
GK_PROTO_BIN=values-go run c6-matcher 0 \
  'values c6 matcher interval interval before=2026-11-12 has_before=true has_after=false' \
  'values c6 matcher range range from=2026-1-1 to=2026-1-31' \
  'values c6 matcher custom custom n=2 first=2026-12-24' 'values c6 matcher weekdays days n=2'
# Proves QuestionnaireRules custom validators reject 2 choices with "color: one only", accept 1 choice,
# and report validator presence per item (color=true, plain=false).
GK_PROTO_BIN=values-go run c6-q-rules 0 'values c6 rules two message="color: one only"' \
  'values c6 rules one valid' 'values c6 rules validator color=true plain=false'
# Proves ResizeHandle item contexts report axis, edge, active flag, and transition from Idle to Hovered.
GK_PROTO_BIN=values-go run c6-resize 0 \
  'values c6 handle row=0 axis=Horizontal edge=[A-Za-z]* has_edge=false active=false state=Idle' \
  'values c6 handle row=0 axis=Horizontal edge=[A-Za-z]* has_edge=false active=false state=Hovered'
# Proves chained value builders for ColorPicker, Grid areas, Tree entries, and Ordinal scales
# (including unknown fallback colors) preserve every configured property.
GK_PROTO_BIN=values-go run c6-chains 0 'values c6 chains picker has=true h=0.50' 'values c6 chains grid areas=2' \
  'values c6 chains tree entries=true,true,false' \
  'values c6 chains scale b_h=0.75 unknown=true unknown_l=0.50 unknown_s=0.00'
