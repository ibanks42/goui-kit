# Go protocol tests

This directory contains the Go-side protocol test suites driven against the diagnostic build of `libgouikit` (`lib/diag`). Each suite selects individual test scenarios via the `GK_PROTO` environment variable and verifies exit codes and structured stdout assertions.

## Running the suite

Run every family:

```sh
core/tests/go/run.sh
```

Run a subset of families or specific modes:

```sh
GK_FAMILIES="app attack" core/tests/go/run.sh
GK_FAMILIES=values core/tests/go/run.sh val-scales val-dates
```

`run.sh` unsets `DISPLAY` and `WAYLAND_DISPLAY` so most tests run headlessly through `gk.RunHeadless`. Modes that require a real display (`overlay`'s `window-real` and `platform`'s `real-*` modes) launch through `gates/display.sh`.

## Test families

| Family (`run-<name>.sh`) | Package | Coverage |
| --- | --- | --- |
| `app` | `./app` | Actions, keybindings, timers, multiple windows, clipboard, ambient calls, close vetoes, quit hooks, reduce-motion, and window icons. |
| `attack` | `./attack` | Raw cgo C ABI stress tests: truncated frames, unknown/empty/null ops, forged/wrong-kind/double-released handles, off-thread calls, stale callback tokens, and invalid view replies. |
| `charts` | `./charts` | Pie arc calculation and stacked series geometry helpers. |
| `editor2` | `./editor2` | Markdown plugins/surfaces/tooltips, history, merge conflicts, folds, `editor_host` leaf geometry, text decorations, display maps, LSP completion highlighting, diagnostics, and overlay key routing. |
| `gen` | `./gen` | Generated smoke checks exercising generated node shapes and state constructors. |
| `handles` | `./handles` | Owned objects, next-frame temporary sweeps, `gk.Retain`, globals, transient values, cascading view/panel release, and GC cleanup. |
| `input` | `./input` | Input, textarea, code editor, number/OTP inputs, clipboard paste, LSP completion providers, link clicks, and view-write-rule checks. |
| `items` | `./items` | Typed `overlay.Items` callbacks for calendar, time field, tree, popover, command palette, hover card, combobox, navigation stack, and wrong-kind rejection. |
| `lists2` | `./lists2` | Select/combobox delegates, custom rows/headers, styled tables, input masks/validators, LSP providers, expression VM, `Ask` caches, diff parsing, inline tokens, state lending, and highlighters. |
| `overlay` | `./overlay` | Dialogs, alert dialogs, sheets, questionnaires, dropdown redraw survival, dock layouts/renderers/persistence, navigation stacks, toasts, popovers, position observers, and real X11 window titles. |
| `platform` | `./platform` | Scripted and real file/prompt dialogs, menus, URL opening, render images/caches, drag-and-drop, virtual lists, and context menus. |
| `review3` | `./review3` | Lifetime and reentrancy regressions: timers outliving views, closing own window, freed dock panels, reentrant action dispatch, and retained dock handles. |
| `rows` | `./rows` | Virtualized list, table, and tree row sources: paging, search, sections, empty/loading/load-more states, sorting, column moves, and scrolling. |
| `runerr` | `./runerr` | `go test` checks confirming API misuse (`*gk.Error`) and panics (`*gk.PanicError`) unwind cleanly while running defers. |
| `values` | `./values` | Scales, motion curves, themes, actions, value pins, clipboard, dates/matchers, scroll handles, cells, settings, drag-and-drop, questionnaire rules, CSS backgrounds, and resize handles. |

Generated files in this tree (`app/gen_ambient.go`, `gen/main.go`, `gensample/main.go`, `input/main.go`) are produced by the code generator and should not be edited by hand.
