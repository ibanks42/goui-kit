# run-values.sh: the protocol tests (docs/PROTOCOLS.md sections
# 13.8 to 13.12), sourced by core/tests/run.sh, which defines build and run. The app is
# core/tests/values, driven by a gk-meter script offscreen with the diag library.
build values-odin "$here/values"

# The plot scales instantiated as library objects: a builder chain in one crossing, reads, the
# generic trait Scale's methods through each instance, Option results.
GK_PROTO_BIN=values-odin run val-scales 0 \
  'values band width_ok=true step_ok=true order=true missing=true nearest=2' \
  'values linear t0=0 t50=100 t100=200' \
  'values point first=0 last=100 out_of_range=true' \
  'values ordinal found=true hue=0.66'
# Motion values over f32: a fallible constructor (Result<Self, E> as gk.Fallible), samples, a
# sequence made of steps (objects inside an Into-free list argument of a builder chain).
GK_PROTO_BIN=values-odin run val-motion 0 \
  'values keyframes ok=true len=2 mid=5.0' 'values keyframes empty ok=false err=true' \
  'values discrete before=1 after=2' 'values discrete switch ok=true' 'values sequence from=1 steps=1'
# The theme: the registry's themes (a map read back), a config applied (an Rc object), the mode
# changed (Option<&mut Window>), a field written and read, Result<T, E> and Result<(), E> read back.
GK_PROTO_BIN=values-odin run val-theme 0 \
  'values registry themes=true dark=false' 'values registry bad json refused=true' \
  'values parse ok=true red=true bad_ok=false bad_err=true' 'values config made=true' \
  'values theme dark=true font_size=17' 'values theme light again dark=false'
# The kit's own action values named as actions.
GK_PROTO_BIN=values-odin run val-actions 0 'values actions same=true builtin=true other=true'
# A temporary handle placed in a view: pinned, redrawn frames later without a new build (the
# theme change redraws every window), released after a newer build.
GK_PROTO_BIN=values-odin run val-pins 0 \
  'values pins click=1 builds=1 pinned=1 later=0' 'values pins click=2 builds=1 pinned=1 later=0' \
  'values pins click=3 builds=2 pinned=1 later=1' 'values pins click=4 builds=2 pinned=1 later=0' \
  'values done mode=val-pins clicks=4 builds=2'
# Misuse: a value given as an argument to its own builder stops there (exit 2).
GK_PROTO_BIN=values-odin run val-self-arg 2 'values giving a value to its own builder' \
  'a value was given as an argument to its own builder'
# Misuse: naming an action in a view procedure stops at its line (exit 2).
GK_PROTO_BIN=values-odin run val-action-in-view 2 'values naming an action in a view' \
  'gk.action_of in a view procedure' 'at .*values/main.odin:[0-9]+'
# The clipboard's value closure: a handler that returns text, run by the copy button's click.
GK_PROTO_BIN=values-odin run val-clipboard 0 'values clipboard value asked' \
  'values clipboard click=[12] ok=true text="copied by the app"'
# Round 3 (PROTOCOLS.md 13.13 to 13.16). chrono's calendar values both ways: a calendar's date and
# range, a date picker's date with time, a time field's time.
GK_PROTO_BIN=values-odin run val-dates 0 \
  'values calendar date=2024-02-29 today_ok=true weeks=[56] week_len=7' \
  'values calendar range applied=(true|false) start=2026-01-05 end=9' \
  'values picker date_time=2026-11-11 09:30:15' 'values time 23:59:00'
# Misuse: a date that does not exist stops where it is written (exit 2).
GK_PROTO_BIN=values-odin run val-bad-date 2 'values writing February 30' \
  'the date 2026-02-30 does not exist'
# gpui's ScrollHandle as a library object: tracked by a div, its ScrollbarHandle methods.
GK_PROTO_BIN=values-odin run val-scroll 0 \
  'values scroll content_taller=true viewport_h=256 offset=0' 'values scroll offset=-200'
# Kit values read back with their reads, and objects inside a state's event.
GK_PROTO_BIN=values-odin run val-readback 0 \
  'values answer set_failed=false ok=true choices=1 first=blue empty=false' \
  'values definition name=color required=true choices=2 second=blue' \
  'values answers len=1 get=true first=blue'
# (QuestionnaireEvent::AnswerChanged is emitted only on the user's input, not by set_answer: mode
# val-quest-events below drives it with clicks.)
# Kit state a read hands out: one handle per entity.
GK_PROTO_BIN=values-odin run val-handed 0 'values handed hue_max=[0-9]+ same_handle=true value_kind='
# Ambient reads: locale, ActiveTheme::theme.
GK_PROTO_BIN=values-odin run val-ambient 0 'values ambient locale=[a-zA-Z_-]+ theme_font=true'
# A tooltip closure that returns a view: the app's view, handed back when the pointer rests on it.
GK_PROTO_BIN=values-odin run val-tooltip 0 'values tooltip view described n=1' 'values done mode=val-tooltip'
# Round 4 (PROTOCOLS.md 13.19, 13.20). Value cells: made, read, written between frames, read again.
GK_PROTO_BIN=values-odin run val-cells 0 'values cells made b=true s=first f=1.5' \
  'values cells again b=false s=second f=2.25'
# One table of handed-out entities: the values path and the dock's path give one handle.
GK_PROTO_BIN=values-odin run val-one-handle 0 'values one handle same=true again=true distinct=1 nonzero=true'
# box_shadow read back as a record whose colour is a gk.Color, then drawn by shadow().
GK_PROTO_BIN=values-odin run val-box-shadow 0 'values box shadow theme=0 h=0.50 l=0.25 x=1 y=2 blur=3 spread=4'
# Drag and drop of the library's drag value: a drag from the source onto the target runs the drop
# handler with the value; the preview is the app's view.
GK_PROTO_BIN=values-odin run val-drag 0 'values drop id=7 text=row 7'
# A settings page over value cells: the switch's click runs the field's set_value handler with the
# new value, which writes the cell; the page reads it while it draws; the app's fragment is drawn and
# its handler runs (and sees the cell written).
GK_PROTO_BIN=values-odin run val-settings 0 'values settings switch set=true was=false' \
  'values settings fragment click dark=true'
# The questionnaire driven by simulated input (PROTOCOLS.md 13.15): a click on a choice emits
# AnswerChanged (its object read back), a click on Submit emits Submit.
GK_PROTO_BIN=values-odin run val-quest-events 0 'values quest answer_changed item=color choice=blue' 'values quest submit'
# (PROTOCOLS.md 11.18 to 11.26; core/tests/values/comp6.odin).
# try_parse_background read back as gk.Background_Value: a colour, a gradient (its stops' hues and
# percentage), a text that is neither (the error).
GK_PROTO_BIN=values-odin run c6-background 0 \
  'values c6 background solid ok=true kind=Solid l=0.50 a=1.00' \
  'values c6 background gradient ok=true kind=Linear_Gradient angle=90 from_h=0.00 to_h=0.667 to_pct=1.00' \
  'values c6 background bad ok=false has=false err=true'
# A calendar's disabled matcher of every variant, given in the state's builder chain and by
# set_disabled_matcher, read back by disabled_matcher_ref.
GK_PROTO_BIN=values-odin run c6-matcher 0 \
  'values c6 matcher interval interval before=2026-11-12 has_before=true has_after=false' \
  'values c6 matcher range range from=2026-1-1 to=2026-1-31' \
  'values c6 matcher custom custom n=2 first=2026-12-24' 'values c6 matcher weekdays days n=2'
# A questionnaire item's validator as rules: two choices where one is allowed is refused with the
# app's message, one is valid; the definition's validator read back.
GK_PROTO_BIN=values-odin run c6-q-rules 0 'values c6 rules two message="color: one only"' \
  'values c6 rules one valid' 'values c6 rules validator color=true plain=false'
# A resizable group whose divider the app draws: the row reads the snapshot of the kit's context, and
# is described again when the pointer is over the divider (Hovered).
GK_PROTO_BIN=values-odin run c6-resize 0 \
  'values c6 handle row=0 axis=Horizontal edge=[A-Za-z]* has_edge=false active=false state=Idle' \
  'values c6 handle row=0 axis=Horizontal edge=[A-Za-z]* has_edge=false active=false state=Hovered'
# Colours and value nodes in a constructor's builder chain (PROTOCOLS.md 11.27, the rule
# of 9.31): each value read back from the state or object the chain made.
GK_PROTO_BIN=values-odin run c6-chains 0 'values c6 chains picker has=true h=0.50' 'values c6 chains grid areas=2' \
  'values c6 chains tree entries=true,true,false' \
  'values c6 chains scale b_h=0.75 unknown=true unknown_l=0.50 unknown_s=0.00'
