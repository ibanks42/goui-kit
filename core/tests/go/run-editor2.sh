# Round-5/6/7 text and editor protocol tests: markdown plugins, link tooltips,
# block surfaces, history/conflicts/annotations, folds, editor_host leaf geometry,
# decorations, LSP completion highlighting, diagnostics, and overlay key routing.
build editor2-go editor2
# Isolate log files in a dedicated subdirectory because some mode names overlap.
editor2_out=$out; out=$out/editor2; mkdir -p "$out"

# Proves Markdown block and inline plugins receive parsed names, text, markdown source,
# and valid font/line/rem metrics across multiple parsed blocks and inline spans.
GK_PROTO_BIN=editor2-go run plugin 0 \
  'editor2 block row=[0-9]+ name=badge text="ready" markdown="```badge' \
  'editor2 inline row=[0-9]+ name=code text="inline" font_ok=true line_ok=true rem_ok=true' \
  'editor2 plugin parsed=[2-9][0-9]* blocks=[1-9][0-9]* inlines=[1-9][0-9]*'
# Proves Markdown link tooltip callbacks receive the hovered link URL and describe a tooltip view.
GK_PROTO_BIN=editor2-go run tooltip 0 \
  'editor2 tooltip row=0 url=https://example.com/docs' 'editor2 tooltip view_rows=1 described=[1-9]'
# Proves custom block surface overrides build and render cleanly to completion.
GK_PROTO_BIN=editor2-go run surface 0 'editor2 done mode=surface'

# Proves editor undo/redo history entries round-trip and match the expected snapshots ["b", "d"].
GK_PROTO_BIN=editor2-go run history 0 'editor2 history entries=\["b", "d"\]'
# Proves git conflict markers are parsed into a Conflict block with current/incoming labels and no base section.
GK_PROTO_BIN=editor2-go run conflict 0 'editor2 conflict n=1' \
  'editor2 conflict lines=.* current=.* incoming=.* base=false/.* current_label="ours" incoming_label="theirs" base_label=false'
# Proves line annotations preserve their id and path metadata when attached to an editor.
GK_PROTO_BIN=editor2-go run annotation 0 'editor2 annotation id="ann-1" path="src/a.rs"'
# Proves custom fold indicator callbacks receive the folded line index and describe their view.
GK_PROTO_BIN=editor2-go run fold 0 \
  'editor2 fold row=[0-9]+ line=0' 'editor2 fold described=[1-9]'
# Proves EntityIDOf accepts valid input/textarea/editor handles so ownership widgets draw cleanly.
GK_PROTO_BIN=editor2-go run owner 0 \
  'editor2 owner drawn' 'editor2 done mode=owner'
# Proves EntityIDOf rejects non-text handles (such as a non-editor state) and exits 1 with a clear error.
GK_PROTO_BIN=editor2-go run bad-owner 1 \
  'is not an input, a textarea or an editor'

# Proves range decorations can be created, appended, and cleared with exact [lo, hi] range readback.
GK_PROTO_BIN=editor2-go run decorations 0 'editor2 decorations made ranges=\[Range\{lo = 0, hi = 5\}\]' \
  'editor2 decorations appended ranges=\[Range\{lo = 0, hi = 5\}, Range\{lo = 6, hi = 11\}\]' \
  'editor2 decorations cleared ranges=\[\]'
# Proves custom editor syntax style resolvers are queried by token name (e.g. keyword).
GK_PROTO_BIN=editor2-go run editor-style 0 'editor2 style asked name=keyword'
# Proves display-map inspection reports line count, full text, visual rows, and initial unfolded state.
GK_PROTO_BIN=editor2-go run display-map 0 'editor2 display-map lines=[45] text="top \{\\nhidden\\n\}\\nlast\\n" rows=[45] folded=false hidden=false row1=1 folds=0'
# Proves unhandled editor key routing reports handled=false cleanly without crashing.
GK_PROTO_BIN=editor2-go run route 0 'editor2 route handled=false'
# Proves InputMask variants format raw digits into the masked output "123-456".
GK_PROTO_BIN=editor2-go run mask-variant 0 'editor2 mask-variant value="123-456"'

# Proves block editor_host parses Markdown into 3 blocks, exposes sized leaf geometry and hit-testing
# (index=3, caret=3), delivers host events, and preserves parsed Markdown table cells and alignments.
GK_PROTO_BIN=editor2-go run host 0 'editor2 host parsed ok=true n=3' \
  'editor2 leaf key=1 len=4 empty=false sized=true line=true caret=true' \
  'editor2 leaf key=2 len=11 empty=false sized=true line=true caret=true' \
  'editor2 leaf key=3 len=5 empty=false sized=true' 'editor2 leaf key=4 len=6 empty=false sized=true' \
  'editor2 leaf hit index=3 caret=3 same=true atom=false' \
  'editor2 host hosts=[1-9][0-9]* leaves=[1-9][0-9]* delivered=[1-3] events=[1-3]$' \
  'editor2 host table rows=1 cells=2 first="cell" aligns=\["Left", "Right"\]'
# Proves block-host style chains inherit parent text styling and draw cleanly.
GK_PROTO_BIN=editor2-go run chain 0 'editor2 chain inherit=true' 'editor2 chain drawn'
# Proves block decorators wrap parsed blocks and run their describe callbacks multiple times.
GK_PROTO_BIN=editor2-go run decorate 0 'editor2 decorate decorated=[2-9][0-9]* described=[2-9][0-9]*'

# Proves LSP completion providers receive a lent edit handle exactly once for in-place text edits.
GK_PROTO_BIN=editor2-go run lsp 0 'editor2 lsp lent=1'
# Proves editor diagnostics present their popover without error.
GK_PROTO_BIN=editor2-go run diagnostic 0 \
  'editor2 diagnostic presented' 'editor2 done mode=diagnostic'
# Proves overlay key routing reports routed=false when no overlay has claimed the key event.
GK_PROTO_BIN=editor2-go run overlay-keys 0 'editor2 overlay-keys routed=false'
# Proves incremental text-edit hooks report exact byte offsets for both insertion (2..2->3) and deletion (2..3->2).
GK_PROTO_BIN=editor2-go run edits 0 'editor2 edits start=2 old_end=2 new_end=3 text="abc"' \
  'editor2 edits start=2 old_end=3 new_end=2 text="ab"'

# Proves custom block layouts place all 4 leaves, fire geometry events, and honor custom block gaps.
GK_PROTO_BIN=editor2-go run custom 0 'editor2 custom parsed ok=true n=2' \
  'editor2 custom leaves=4 events=[1-9][0-9]* placed=true gap_with=[0-9]+ gap_without=[0-9]+'
# Proves an overlay firing on Enter consumes the key (taken=true, no newline added) and clears routing afterward.
GK_PROTO_BIN=editor2-go run overlay-fire 0 \
  'editor2 overlay-fire presented at=3 text="abx"' 'editor2 overlay-fire value="abx" routed_after=false taken=true'
# Proves an overlay declining Enter leaves the key for the editor (taken=false, newline inserted as "abx\n").
GK_PROTO_BIN=editor2-go run overlay-control 0 \
  'editor2 overlay-control presented at=3 text="abx"' 'editor2 overlay-control value="abx\\n" routed_after=false taken=false'
# Proves LSP completion re-queries the provider on subsequent triggers and renders both "first" and "second" labels.
GK_PROTO_BIN=editor2-go run lsp-ask 0 'editor2 lsp-ask asked query="" label="first"' \
  'editor2 lsp-ask asked query="" label="second"' 'editor2 lsp-ask asked=[2-9]'
# Proves diagnostic popover visibility flips from true when shown to false after clearing diagnostics.
GK_PROTO_BIN=editor2-go run diagnostic-shown 0 \
  'editor2 diagnostic-shown popover=true' 'editor2 diagnostic-shown cleared popover=false'

out=$editor2_out
