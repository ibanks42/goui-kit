// Package gk provides the core Go bindings for GPUI and gpui-kit.
//
// # Application Lifecycle and Threading
//
// Call [Run] from main to start the native event loop on the locked main OS
// thread. The start callback receives a [*Cx] context used to create initial
// views ([NewView]), open windows ([OpenWindow], [OpenWindowWith]), bind
// keyboard shortcuts ([Shortcut], [BindKeys]), and configure menus ([SetMenus]).
// For offscreen testing with the diagnostic library, use [RunHeadless] or set
// the GK_SCRIPT environment variable.
//
// Every function in gk and its sub-packages that accepts a [*Cx] must run on
// the UI thread during the callback that received that [*Cx]. Do not store a
// [*Cx] past the return of the callback. Background goroutines must use [Post]
// or [Go] to deliver results back onto the UI thread.
//
// # Views, Elements, and Reactive State
//
// A view's render function describes its UI tree by constructing elements
// ([Div], [Text], [UniformList], [SourceList], and widgets from sub-packages)
// and chaining builder methods. Elements and kit values are transient: they
// live only for the single render call that created them and may be attached to
// at most one parent. View functions must be pure descriptions and cannot
// create views, open windows, start timers, or mutate kit state.
//
// Application state can be stored in [Model] values (which automatically mark
// reading views dirty on [Model.Set], [Model.Update], or [Model.Touch]), or
// updated explicitly in event handlers followed by [Notify] or [Refresh].
//
// # Handle Lifetimes
//
// Handles created inside a view's init or event turns (kit states, focus
// handles, child views, row sources, timers, and completions) are owned by that
// view and released automatically when the view is released. Most state handles
// are also tracked by Go's garbage collector via runtime.AddCleanup and freed
// on the UI thread when unreachable; call [Release] to free resources eagerly.
//
// # Sub-packages and Library Loading
//
// Component families live in sibling packages so applications import only what
// they use:
//   - gpuikit.dev/gk/s: nullary GPUI style tokens
//   - gpuikit.dev/gk/theme and gpuikit.dev/gk/palette: theme and fixed colors
//   - gpuikit.dev/gk/kit: everyday UI widgets and menus
//   - gpuikit.dev/gk/base: low-level layout, questionnaire, and resize primitives
//   - gpuikit.dev/gk/overlay: dialogs, alert dialogs, sheets, popovers, tooltips, hover cards, notifications
//   - gpuikit.dev/gk/table: virtualized tables, lists, searchable lists, selects, comboboxes, trees
//   - gpuikit.dev/gk/editor: text inputs, code editors, diffs, markdown plugins, block hosts
//   - gpuikit.dev/gk/dock: dockable workspace areas, split panes, tab groups, custom renderers
//   - gpuikit.dev/gk/chart: data tables, charts, and composable plots
//   - gpuikit.dev/gk/motion: animations, springs, keyframes, focus traps, text selection scopes
//
// By default (CGO_ENABLED=0), the runtime loads libgpuikit dynamically at
// startup via purego (searching GK_LIB, directories beside the executable, and
// the OS dynamic loader path). Building with -tags gkstatic and CGO_ENABLED=1
// links libgpuikit_static.a statically via cgo and keeps only the imported
// component families.
package gk
