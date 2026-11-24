// Public re-exports of the hand-written runtime API from github.com/ibanks42/goui-kit/gk/internal/rt.
// Type aliases preserve exact identity across sub-packages, while thin wrapper
// functions keep the public surface documented under package gk.

package gk

import (
	"time"

	"github.com/ibanks42/goui-kit/gk/internal/rt"
)

// Action identifies an application-defined or built-in kit action registered
// with GPUI for key bindings, menus, and element action handlers.
type Action = rt.Action

// NewAction registers a named application action with optional payload data.
func NewAction(cx *Cx, name string, data ...string) Action { return rt.NewAction(cx, name, data...) }

// KitActionValue is implemented by generated kit action value types accepted by ActionOf.
type KitActionValue = rt.KitActionValue

// ActionOf resolves a built-in kit action value into an Action handle outside a
// view function so it can be bound to keys or dispatched.
func ActionOf(cx *Cx, v KitActionValue) Action { return rt.ActionOf(cx, v) }

// ActionHandler pairs an Action with a callback for element-level OnAction builders.
type ActionHandler = rt.ActionHandler

// Act creates an ActionHandler binding action a to callback f.
func Act(a Action, f func(cx *Cx, a Action)) ActionHandler { return rt.Act(a, f) }

// KeyBinding maps a keystroke string to an Action, optionally scoped by When.
type KeyBinding = rt.KeyBinding

// BindKeys registers one or more keyboard bindings with the application.
func BindKeys(cx *Cx, bindings ...KeyBinding) { rt.BindKeys(cx, bindings...) }

// ClearKeyBindings removes all registered application key bindings.
func ClearKeyBindings(cx *Cx) { rt.ClearKeyBindings(cx) }

// ActionListener is a releasable handle to a global action, window-close, or
// app-quit callback.
type ActionListener = rt.ActionListener

// OnAppAction registers a global callback invoked when app-defined action a fires.
func OnAppAction(cx *Cx, a Action, f func(cx *Cx, a Action)) ActionListener {
	return rt.OnAppAction(cx, a, f)
}

// StopPropagation prevents the active UI event from bubbling to parent elements.
func StopPropagation(cx *Cx) { rt.StopPropagation(cx) }

// ShortcutOption configures optional settings on Shortcut and KbdForAction.
type ShortcutOption = rt.ShortcutOption

// WhenIn scopes a Shortcut or keycap hint to the given GPUI key context expression.
func WhenIn(ctx string) ShortcutOption { return rt.WhenIn(ctx) }

// ShortcutHandler constrains callbacks accepted by Shortcut to plain or click handlers.
type ShortcutHandler = rt.ShortcutHandler

// Shortcut registers a unique Action, attaches f as its global handler, and
// binds keys (when non-empty) to it.
func Shortcut[F ShortcutHandler](cx *Cx, keys string, f F, opts ...ShortcutOption) Action {
	return rt.Shortcut[F](cx, keys, f, opts...)
}

// DispatchAction dispatches action a through the currently focused element chain.
func DispatchAction(cx *Cx, a Action) { rt.DispatchAction(cx, a) }

// ActionAvailable reports whether any focused element or global listener handles a.
func ActionAvailable(cx *Cx, a Action) bool { return rt.ActionAvailable(cx, a) }

// Keystrokes returns the primary keystroke bound to a in keyContext, if any.
func Keystrokes(cx *Cx, a Action, keyContext string) (string, bool) {
	return rt.Keystrokes(cx, a, keyContext)
}

// Timer is a releasable handle to a one-shot or repeating UI-thread timer.
type Timer = rt.Timer

// After schedules f to run once on the UI thread after duration d.
func After(cx *Cx, d time.Duration, f func(cx *Cx)) Timer { return rt.After(cx, d, f) }

// Every schedules f to run repeatedly on the UI thread at interval d (at least 1 ms).
func Every(cx *Cx, d time.Duration, f func(cx *Cx)) Timer { return rt.Every(cx, d, f) }

// ClipboardWrite writes text to the system clipboard.
func ClipboardWrite(cx *Cx, text string) { rt.ClipboardWrite(cx, text) }

// ClipboardRead reads text from the system clipboard, returning false if none is available.
func ClipboardRead(cx *Cx) (string, bool) { return rt.ClipboardRead(cx) }

// Post schedules f to run on the UI thread during the next wake turn.
// Post is the only gk function that may be called from background goroutines;
// calling any other gk function off the UI thread stops the app with an *Error.
func Post(f func(cx *Cx)) { rt.Post(f) }

// Go executes work in a background goroutine and delivers its return value to
// done on the UI thread via Post. If work panics, Go captures the panic and
// call site, shuts the event loop down cleanly, and causes Run to re-panic
// with a *PanicError. Both work and done must be non-nil.
func Go[T any](cx *Cx, work func() T, done func(cx *Cx, v T)) { rt.Go[T](cx, work, done) }

// DoneStatus reports whether an asynchronous platform dialog was answered,
// canceled, or failed.
type DoneStatus = rt.DoneStatus

// DoneAnswered indicates the user completed the dialog with a result.
const DoneAnswered = rt.DoneAnswered

// DoneCancelled indicates the user dismissed or canceled the dialog.
const DoneCancelled = rt.DoneCancelled

// DoneFailed indicates the platform dialog failed with an error.
const DoneFailed = rt.DoneFailed

// PathsDone holds the outcome and chosen paths of PromptForPaths or PromptForNewPath.
type PathsDone = rt.PathsDone

// PromptDone holds the outcome and chosen button index of Prompt.
type PromptDone = rt.PromptDone

// Completion represents an in-flight platform dialog request that can be
// canceled with Release.
type Completion = rt.Completion

// PathPrompt configures a native file or directory open dialog.
type PathPrompt = rt.PathPrompt

// PromptForPaths opens a native file or directory picker and invokes f on completion.
func PromptForPaths(cx *Cx, p PathPrompt, f func(cx *Cx, d PathsDone)) Completion {
	return rt.PromptForPaths(cx, p, f)
}

// PromptForNewPath opens a native save-file dialog in directory with an
// optional suggestedName and invokes f on completion.
func PromptForNewPath(cx *Cx, directory string, suggestedName string, f func(cx *Cx, d PathsDone)) Completion {
	return rt.PromptForNewPath(cx, directory, suggestedName, f)
}

// PromptLevel selects the severity icon for a native modal Prompt dialog.
type PromptLevel = rt.PromptLevel

// PromptInfo displays an informational alert icon.
const PromptInfo = rt.PromptInfo

// PromptWarning displays a warning alert icon.
const PromptWarning = rt.PromptWarning

// PromptCritical displays a critical-error alert icon.
const PromptCritical = rt.PromptCritical

// PromptButtonKind classifies a button in a native modal Prompt dialog.
type PromptButtonKind = rt.PromptButtonKind

// ButtonOther represents a general action button.
const ButtonOther = rt.ButtonOther

// ButtonOk represents the default confirmation button.
const ButtonOk = rt.ButtonOk

// ButtonCancel represents the cancellation button.
const ButtonCancel = rt.ButtonCancel

// PromptButton defines one button shown by Prompt.
type PromptButton = rt.PromptButton

// Prompt displays a native modal alert dialog and invokes f when answered or canceled.
func Prompt(cx *Cx, level PromptLevel, message string, detail string, answers []PromptButton, f func(cx *Cx, d PromptDone)) Completion {
	return rt.Prompt(cx, level, message, detail, answers, f)
}

// OpenURL opens url with the operating system's default application.
func OpenURL(cx *Cx, url string) { rt.OpenURL(cx, url) }

// PlatformAnswerPaths provides a scripted answer to an open or save dialog in
// headless diagnostic tests.
func PlatformAnswerPaths(cx *Cx, status DoneStatus, paths []string, newPath bool, errText string) bool {
	return rt.PlatformAnswerPaths(cx, status, paths, newPath, errText)
}

// PlatformAnswerPrompt provides a scripted answer to a modal Prompt in headless
// diagnostic tests.
func PlatformAnswerPrompt(cx *Cx, status DoneStatus, answer int) bool {
	return rt.PlatformAnswerPrompt(cx, status, answer)
}

// Built wraps a View whose render function returns a specific element or value
// type T, used for overlays such as dialogs, sheets, and popup menus.
type Built[T any] = rt.Built[T]

// NewBuilt creates a reusable Built[T] overlay view from render outside a view function.
func NewBuilt[T any](cx *Cx, render func(cx *Cx) T) Built[T] { return rt.NewBuilt[T](cx, render) }

// Builder accepts either a pre-created Built[T] handle or an inline closure
// func(*Cx) T when opening an overlay.
type Builder[T any] = rt.Builder[T]

// Info describes the loaded libgouikit binary's ABI version, interface hash,
// feature flags, and embedded crate versions.
type Info = rt.Info

// LibraryInfo returns metadata from the loaded native library.
func LibraryInfo() Info { return rt.LibraryInfo() }

// Error reports an application misuse of the Go bindings with its call site.
type Error = rt.Error

// PanicError wraps a panic raised by application code inside a UI callback or
// gk.Go worker, re-panicked by Run after shutting the native event loop down.
type PanicError = rt.PanicError

// LibraryError reports an internal panic caught inside the Rust core library.
type LibraryError = rt.LibraryError

// Run starts the native UI event loop on the locked OS thread and calls start
// on the initial turn (where apps typically create views and open windows).
//
// If GK_SCRIPT is set to a script file path, Run reads it and delegates to
// RunHeadless. On Linux, Run returns an error immediately if both DISPLAY and
// WAYLAND_DISPLAY are unset. Application panics inside callbacks or gk.Go
// workers cause Run to shut the event loop down and re-panic with *PanicError;
// API misuse returns *Error and Rust-side panics return *LibraryError.
func Run(start func(cx *Cx)) error { return rt.Run(start) }

// RunHeadless runs the application offscreen using the diagnostic library and
// drives window sizing, frames, scripted input, and screenshots from script.
func RunHeadless(script string, start func(cx *Cx)) error { return rt.RunHeadless(script, start) }

// TestRunRaw invokes the low-level library entry point with custom ABI fields
// to verify startup handshake checks in protocol tests.
func TestRunRaw(hashXor uint64, majorAdd uint16, minorAdd uint16, sizeDelta uint32, noCallback bool) (int32, string) {
	return rt.TestRunRaw(hashXor, majorAdd, minorAdd, sizeDelta, noCallback)
}

// TestExpectHash overrides the expected API hash checked before Run enters the loop.
func TestExpectHash(h uint64) { rt.TestExpectHash(h) }

// HighlightQuery holds language, text, and byte range for a custom highlighter.
type HighlightQuery = rt.HighlightQuery

// HighlightRun associates a byte range with a highlight style name.
type HighlightRun = rt.HighlightRun

// FoldQuery holds language and text for a custom code-folding callback.
type FoldQuery = rt.FoldQuery

// SyntaxQuery holds language, text, and cursor offset for syntax-context queries.
type SyntaxQuery = rt.SyntaxQuery

// LanguageAlias maps a short name or extension to a canonical language name.
type LanguageAlias = rt.LanguageAlias

// Highlighter holds the source handles for a custom syntax highlighter and folder.
type Highlighter = rt.Highlighter

// NewHighlighter registers a custom syntax highlighter and optional folder for
// the named languages.
func NewHighlighter(cx *Cx, styles func(cx *Cx, q HighlightQuery) []HighlightRun, folds func(cx *Cx, q FoldQuery) []rt.FoldRange, languages ...string) Highlighter {
	return rt.NewHighlighter(cx, styles, folds, languages...)
}

// NewLanguageProvider creates a language provider source with aliases and an
// optional syntax-context callback.
func NewLanguageProvider(cx *Cx, aliases []LanguageAlias, syntax func(cx *Cx, q SyntaxQuery) rt.SyntaxContext) RowSource {
	return rt.NewLanguageProvider(cx, aliases, syntax)
}

// Element is implemented by every renderable element handle built inside a view function.
type Element = rt.Element

// Style is a nullary GPUI style token opcode (see package github.com/ibanks42/goui-kit/gk/s).
type Style = rt.Style

// Color represents either a theme/palette color token or an explicit HSLA color.
type Color = rt.Color

// HSLA creates an explicit Color from hue, saturation, lightness, and alpha channels.
func HSLA(h float32, s float32, l float32, a float32) Color { return rt.HSLA(h, s, l, a) }

// ThemeColor constructs a Color backed by theme token ID n.
func ThemeColor(n uint16) Color { return rt.ThemeColor(n) }

// PaletteColor constructs a Color backed by palette token ID n.
func PaletteColor(n uint16) Color { return rt.PaletteColor(n) }

// Hsla holds explicit hue, saturation, lightness, and alpha color channels.
type Hsla = rt.Hsla

// Fill is implemented by solid Color values and LinearGradient background fills.
type Fill = rt.Fill

// NewLinearColorStop creates a gradient stop with color c at percentage (0 to 1).
func NewLinearColorStop(c Color, percentage float32) rt.LinearColorStop {
	return rt.NewLinearColorStop(c, percentage)
}

// LinearGradient defines a two-stop linear gradient background fill.
type LinearGradient = rt.LinearGradient

// NewLinearGradient constructs a LinearGradient at angle degrees between from and to.
func NewLinearGradient(angle float32, from rt.LinearColorStop, to rt.LinearColorStop) LinearGradient {
	return rt.NewLinearGradient(angle, from, to)
}

// Length represents a CSS-like layout length in pixels, rems, relative fraction, or auto.
type Length = rt.Length

// Px returns an absolute length in logical pixels.
func Px(v float32) Length { return rt.Px(v) }

// Rems returns an absolute length in root-em units.
func Rems(v float32) Length { return rt.Rems(v) }

// Relative returns a fraction (0 to 1) of the parent's dimension.
func Relative(v float32) Length { return rt.Relative(v) }

// Auto returns an automatic layout length.
func Auto() Length { return rt.Auto() }

// ID accepts an element identifier: string, int, int64, uint64, or NamedID.
type ID = rt.ID

// NamedID pairs a string name with an integer index as a composite element ID.
type NamedID = rt.NamedID

// Key creates a composite NamedID from name and index n.
func Key(name string, n int) NamedID { return rt.Key(name, n) }

// Range is a half-open non-negative integer range [Lo, Hi).
type Range = rt.Range

// Font describes a font family, numeric weight, and slant style.
type Font = rt.Font

// NewFont creates a Font with the given family name.
func NewFont(family string) Font { return rt.NewFont(family) }

// ExprCheck validates an expression string for nargs parameters, returning an
// empty string on success or a diagnostic message on error.
func ExprCheck(text string, nargs int) string { return rt.ExprCheck(text, nargs) }

// MarkdownMatch selects which markdown block or inline construct a plugin matches.
type MarkdownMatch = rt.MarkdownMatch

// MarkdownMatchCode matches fenced code blocks.
const MarkdownMatchCode = rt.MarkdownMatchCode

// MarkdownMatchMath matches display math blocks.
const MarkdownMatchMath = rt.MarkdownMatchMath

// MarkdownMatchYaml matches YAML frontmatter blocks.
const MarkdownMatchYaml = rt.MarkdownMatchYaml

// MarkdownMatchToml matches TOML frontmatter blocks.
const MarkdownMatchToml = rt.MarkdownMatchToml

// MarkdownMatchHTML matches HTML blocks.
const MarkdownMatchHTML = rt.MarkdownMatchHTML

// MarkdownMatchMdxElement matches block MDX elements.
const MarkdownMatchMdxElement = rt.MarkdownMatchMdxElement

// MarkdownMatchInlineCode matches inline code spans.
const MarkdownMatchInlineCode = rt.MarkdownMatchInlineCode

// MarkdownMatchInlineMath matches inline math spans.
const MarkdownMatchInlineMath = rt.MarkdownMatchInlineMath

// MarkdownMatchInlineHTML matches inline HTML spans.
const MarkdownMatchInlineHTML = rt.MarkdownMatchInlineHTML

// MarkdownMatchMdxInlineElement matches inline MDX elements.
const MarkdownMatchMdxInlineElement = rt.MarkdownMatchMdxInlineElement

// MarkdownPlugin registers a custom RowSource renderer for matched markdown nodes.
type MarkdownPlugin = rt.MarkdownPlugin

// MarkdownPluginAny is the marker interface satisfied by MarkdownPlugin.
type MarkdownPluginAny = rt.MarkdownPluginAny

// EntityID identifies a GPUI state entity passed to text and editor APIs.
type EntityID = rt.EntityID

// EntityIDOf extracts the EntityID of a handled kit state.
func EntityIDOf(state rt.Handled) EntityID { return rt.EntityIDOf(state) }

// KitView is a placeholder element referencing a kit-provided child view slot.
type KitView = rt.KitView

// Ask is a typed synchronous request/response callback channel backed by a RowSource.
type Ask[K any, V any] = rt.Ask[K, V]

// NewAsk creates a typed Ask[K, V] channel outside a view function.
func NewAsk[K any, V any](cx *Cx, f func(cx *Cx, key K) V) Ask[K, V] { return rt.NewAsk[K, V](cx, f) }

// TestAsks returns diagnostic counters for Ask gets, requests, and answers.
func TestAsks(cx *Cx) (int, int, int) { return rt.TestAsks(cx) }

// TestLends returns diagnostic counters for lent handles issued and returned.
func TestLends(cx *Cx) (int, int) { return rt.TestLends(cx) }

// Transformation defines a 2D scale, translation, rotation, and optional opacity
// transform for SVG elements.
type Transformation = rt.Transformation

// NewTransformation returns the identity SVG Transformation.
func NewTransformation() Transformation { return rt.NewTransformation() }

// SvgEasing selects the interpolation curve for TimedTransformation channels.
type SvgEasing = rt.SvgEasing

// SvgEasingLinear interpolates linearly.
const SvgEasingLinear = rt.SvgEasingLinear

// SvgEasingQuadratic uses quadratic easing.
const SvgEasingQuadratic = rt.SvgEasingQuadratic

// SvgEasingEaseInOut uses symmetric ease-in-out.
const SvgEasingEaseInOut = rt.SvgEasingEaseInOut

// SvgEasingEaseOutQuint uses quintic ease-out.
const SvgEasingEaseOutQuint = rt.SvgEasingEaseOutQuint

// SvgEasingHeartbeat uses a heartbeat pulse curve.
const SvgEasingHeartbeat = rt.SvgEasingHeartbeat

// TimedTransformation animates an SVG element between From and To over Period.
type TimedTransformation = rt.TimedTransformation

// NewTimedTransform creates a TimedTransformation using easing for all channels.
func NewTimedTransform(period time.Duration, from Transformation, to Transformation, easing SvgEasing) TimedTransformation {
	return rt.NewTimedTransform(period, from, to, easing)
}

// NewTimedRotation creates a continuous linear full-turn rotation starting at from.
func NewTimedRotation(period time.Duration, from Transformation) TimedTransformation {
	return rt.NewTimedRotation(period, from)
}

// ExternalFile describes one filesystem entry dragged from outside the app.
type ExternalFile = rt.ExternalFile

// ExternalDragPayload carries external files dropped onto an element.
type ExternalDragPayload = rt.ExternalDragPayload

// A11yChild describes a child accessibility node with key, role, and label.
type A11yChild = rt.A11yChild

// DragMoveEvent carries pointer state, bounds, and DragValue during a drag move.
type DragMoveEvent = rt.DragMoveEvent

// ImageCache is a handle to a shared GPUI image cache.
type ImageCache = rt.ImageCache

// RetainAllImages selects a named GPUI retain-all image cache provider.
type RetainAllImages = rt.RetainAllImages

// ImageCacheProvider is implemented by ImageCache and RetainAllImages.
type ImageCacheProvider = rt.ImageCacheProvider

// NewImageCache allocates an ImageCache handle outside a view function.
func NewImageCache(cx *Cx) ImageCache { return rt.NewImageCache(cx) }

// ScrollAnchor is a handle used to scroll an anchored element into view.
type ScrollAnchor = rt.ScrollAnchor

// NewScrollAnchor creates a ScrollAnchor attached to handle outside a view function.
func NewScrollAnchor(cx *Cx, handle rt.ScrollHandle) ScrollAnchor {
	return rt.NewScrollAnchor(cx, handle)
}

// ScrollAnchorTo scrolls the container of a so the anchored element is visible.
func ScrollAnchorTo(cx *Cx, a ScrollAnchor) { rt.ScrollAnchorTo(cx, a) }

// Fallible represents a Rust Result<T, E> decoded across the C ABI.
type Fallible[T any] = rt.Fallible[T]

// CivilDate represents a calendar date without a time zone.
type CivilDate = rt.CivilDate

// CivilTime represents a wall-clock time without a time zone.
type CivilTime = rt.CivilTime

// CivilDateTime combines a CivilDate and a CivilTime.
type CivilDateTime = rt.CivilDateTime

// Weekday identifies a day of the week (Monday through Sunday).
type Weekday = rt.Weekday

// WeekdayMon is Monday.
const WeekdayMon = rt.WeekdayMon

// WeekdayTue is Tuesday.
const WeekdayTue = rt.WeekdayTue

// WeekdayWed is Wednesday.
const WeekdayWed = rt.WeekdayWed

// WeekdayThu is Thursday.
const WeekdayThu = rt.WeekdayThu

// WeekdayFri is Friday.
const WeekdayFri = rt.WeekdayFri

// WeekdaySat is Saturday.
const WeekdaySat = rt.WeekdaySat

// WeekdaySun is Sunday.
const WeekdaySun = rt.WeekdaySun

// DaysInMonth returns the number of days in (year, month) in the Gregorian calendar.
func DaysInMonth(year int32, month uint32) uint32 { return rt.DaysInMonth(year, month) }

// CellValue constrains scalar types stored in a Rust-side Cell.
type CellValue = rt.CellValue

// Cell is a handle to a Rust-managed mutable scalar cell (bool, float64, or string).
type Cell[T CellValue] = rt.Cell[T]

// NewCell creates a Rust-side Cell initialized to value outside a view function.
func NewCell[T CellValue](cx *Cx, value T) Cell[T] { return rt.NewCell[T](cx, value) }

// CellSet updates the value stored in Cell c.
func CellSet[T CellValue](cx *Cx, c Cell[T], value T) { rt.CellSet[T](cx, c, value) }

// CellGet reads the current value stored in Cell c.
func CellGet[T CellValue](cx *Cx, c Cell[T]) T { return rt.CellGet[T](cx, c) }

// LspPosition identifies a zero-based line and UTF-16 character column.
type LspPosition = rt.LspPosition

// CompletionItem represents one editor autocomplete suggestion.
type CompletionItem = rt.CompletionItem

// CodeBlock holds a fenced markdown code block's language, source, and metadata.
type CodeBlock = rt.CodeBlock

// ArcData holds the computed geometry of one pie or donut chart slice.
type ArcData = rt.ArcData

// StackPoint holds the baseline, top, and row index of one point in a stacked series.
type StackPoint = rt.StackPoint

// StackSeries holds one stacked chart series key, index, and points.
type StackSeries = rt.StackSeries

// QuestionnaireValidator is a marker struct for questionnaire validator objects.
type QuestionnaireValidator = rt.QuestionnaireValidator

// QuestionnaireRules configures declarative validation rules for a questionnaire field.
type QuestionnaireRules = rt.QuestionnaireRules

// BackgroundKind identifies which variant a BackgroundValue holds.
type BackgroundKind = rt.BackgroundKind

// BackgroundKindSolid is a solid color background.
const BackgroundKindSolid = rt.BackgroundKindSolid

// BackgroundKindLinearGradient is a linear gradient background.
const BackgroundKindLinearGradient = rt.BackgroundKindLinearGradient

// BackgroundKindPatternSlash is a diagonal slash pattern background.
const BackgroundKindPatternSlash = rt.BackgroundKindPatternSlash

// BackgroundKindCheckerboard is a checkerboard pattern background.
const BackgroundKindCheckerboard = rt.BackgroundKindCheckerboard

// BackgroundColorSpace selects the interpolation color space of a gradient background.
type BackgroundColorSpace = rt.BackgroundColorSpace

// BackgroundColorSpaceSrgb interpolates in sRGB space.
const BackgroundColorSpaceSrgb = rt.BackgroundColorSpaceSrgb

// BackgroundColorSpaceOklab interpolates in Oklab space.
const BackgroundColorSpaceOklab = rt.BackgroundColorSpaceOklab

// BackgroundStop holds one color stop of a BackgroundValue gradient.
type BackgroundStop = rt.BackgroundStop

// BackgroundValue is a decoded GPUI Background returned from style/theme queries.
type BackgroundValue = rt.BackgroundValue

// IntervalMatcher matches calendar dates before Before or after After.
type IntervalMatcher = rt.IntervalMatcher

// RangeMatcher matches calendar dates within an optional [From, To] range.
type RangeMatcher = rt.RangeMatcher

// KeyMods is a bitmask of keyboard modifier keys held during an input event.
type KeyMods = rt.KeyMods

// ModControl indicates the Control modifier key.
const ModControl = rt.ModControl

// ModAlt indicates the Alt/Option modifier key.
const ModAlt = rt.ModAlt

// ModShift indicates the Shift modifier key.
const ModShift = rt.ModShift

// ModPlatform indicates the platform command modifier (Command on macOS, Control elsewhere).
const ModPlatform = rt.ModPlatform

// ModFunction indicates the Function (Fn) modifier key.
const ModFunction = rt.ModFunction

// PointerButton identifies a mouse or pointer button.
type PointerButton = rt.PointerButton

// ButtonLeft is the primary mouse button.
const ButtonLeft = rt.ButtonLeft

// ButtonRight is the secondary mouse button.
const ButtonRight = rt.ButtonRight

// ButtonMiddle is the middle mouse button.
const ButtonMiddle = rt.ButtonMiddle

// ButtonBack is the browser-back mouse button.
const ButtonBack = rt.ButtonBack

// ButtonForward is the browser-forward mouse button.
const ButtonForward = rt.ButtonForward

// ButtonNone indicates no pointer button (for example, keyboard activation).
const ButtonNone = rt.ButtonNone

// ClickKind distinguishes mouse, keyboard, and touch click activations.
type ClickKind = rt.ClickKind

// ClickMouse indicates a mouse click.
const ClickMouse = rt.ClickMouse

// ClickKeyboard indicates keyboard activation (Enter or Space).
const ClickKeyboard = rt.ClickKeyboard

// ClickTouch indicates touch activation.
const ClickTouch = rt.ClickTouch

// ClickEvent carries pointer coordinates, button, modifiers, click count, and
// activation kind for click handlers.
type ClickEvent = rt.ClickEvent

// KeyEvent carries key name, optional typed character, modifiers, and repeat
// state for keyboard handlers.
type KeyEvent = rt.KeyEvent

// MouseEvent carries pointer coordinates, button, modifiers, and click count.
type MouseEvent = rt.MouseEvent

// TestPayload is a diagnostic event payload used by protocol tests.
type TestPayload = rt.TestPayload

// NewHandler stores a bool-returning diagnostic handler and returns its slot index.
func NewHandler(f func(cx *Cx) bool) uint32 { return rt.NewHandler(f) }

// LinkAll ensures all component families are linked into static builds (or
// loads the shared library eagerly in dynamic builds).
func LinkAll() { rt.LinkAll() }

// Model holds an observable Go value of type T. Calling Model.Get inside a
// view's render function subscribes that view to future writes; Model.Set,
// Model.Update, and Model.Touch mark all subscribed views dirty and run any
// callbacks attached with Model.Observe. Use Model.Peek to read without
// subscribing.
type Model[T any] = rt.Model[T]

// NewModel creates a Model initialized to v.
func NewModel[T any](v T) Model[T] { return rt.NewModel[T](v) }

// DragValue carries an application-defined drag ID and text across drag gestures.
type DragValue = rt.DragValue

// DropAccept specifies which drag payloads a drop target accepts.
type DropAccept = rt.DropAccept

// ViewOrTooltip accepts either a View handle or a generated kit tooltip value.
type ViewOrTooltip = rt.ViewOrTooltip

// ImageBytes pairs an image format with encoded image bytes.
type ImageBytes = rt.ImageBytes

// PixelImage holds raw RGBA pixel bytes (Width * Height * 4) for NewRenderImage.
type PixelImage = rt.PixelImage

// RenderImage is a handle to an uploaded GPU-ready raster image.
type RenderImage = rt.RenderImage

// NewRenderImage uploads PixelImage p to the library outside a view function.
func NewRenderImage(cx *Cx, p PixelImage) RenderImage { return rt.NewRenderImage(cx, p) }

// TestRenderImage queries image dimensions, first pixel, and ID in diagnostic tests.
func TestRenderImage(cx *Cx, i RenderImage, custom bool) (uint32, uint32, uint32, uint64) {
	return rt.TestRenderImage(cx, i, custom)
}

// TestImagesShown returns diagnostic render counters for displayed images.
func TestImagesShown(cx *Cx) (uint64, uint32, uint64) { return rt.TestImagesShown(cx) }

// TestDropAccept evaluates a DropAccept filter against id in diagnostic tests.
func TestDropAccept(cx *Cx, a DropAccept, id uint64) (bool, bool) {
	return rt.TestDropAccept(cx, a, id)
}

// OsAction tags a native menu item with a standard platform editing role.
type OsAction = rt.OsAction

// OsActionNone attaches no special platform editing role.
const OsActionNone = rt.OsActionNone

// OsActionCut maps to the system Cut menu action.
const OsActionCut = rt.OsActionCut

// OsActionCopy maps to the system Copy menu action.
const OsActionCopy = rt.OsActionCopy

// OsActionPaste maps to the system Paste menu action.
const OsActionPaste = rt.OsActionPaste

// OsActionSelectAll maps to the system Select All menu action.
const OsActionSelectAll = rt.OsActionSelectAll

// OsActionUndo maps to the system Undo menu action.
const OsActionUndo = rt.OsActionUndo

// OsActionRedo maps to the system Redo menu action.
const OsActionRedo = rt.OsActionRedo

// Menu describes one top-level or nested native application menu.
type Menu = rt.Menu

// MenuItem is implemented by MenuSeparator, MenuAction, MenuSubmenu, and MenuServices.
type MenuItem = rt.MenuItem

// MenuSeparator inserts a divider line between menu items.
type MenuSeparator = rt.MenuSeparator

// MenuAction defines a clickable menu item bound to an Action.
type MenuAction = rt.MenuAction

// MenuSubmenu nests a child Menu inside a parent menu.
type MenuSubmenu = rt.MenuSubmenu

// MenuServices marks the platform services submenu (used on macOS).
type MenuServices = rt.MenuServices

// SetMenus installs the application's native menu bar.
func SetMenus(cx *Cx, menus ...Menu) { rt.SetMenus(cx, menus...) }

// Menus returns the currently installed application menus.
func Menus(cx *Cx) []Menu { return rt.Menus(cx) }

// SearchItem describes one selectable entry in a searchable list or select.
type SearchItem = rt.SearchItem

// SearchGroup groups SearchItems under a section heading.
type SearchGroup = rt.SearchGroup

// SearchRequest carries a query string and correlation ID for async search.
type SearchRequest = rt.SearchRequest

// SearchOption configures optional callbacks on NewSearchSource and NewSearchSourceGroups.
type SearchOption = rt.SearchOption

// SearchRow sets a custom row render callback for items in a search source.
func SearchRow(row func(cx *Cx, at RowAt) Element) SearchOption { return rt.SearchRow(row) }

// SearchHeaders enables custom rendering of group header rows via SearchRow.
func SearchHeaders() SearchOption { return rt.SearchHeaders() }

// SearchRequests installs an asynchronous query handler on a search source.
func SearchRequests(f func(cx *Cx, e SearchRequest)) SearchOption { return rt.SearchRequests(f) }

// NewSearchSource creates a flat searchable RowSource initialized with items.
func NewSearchSource(cx *Cx, items []SearchItem, opts ...SearchOption) RowSource {
	return rt.NewSearchSource(cx, items, opts...)
}

// NewSearchSourceGroups creates a grouped searchable RowSource initialized with groups.
func NewSearchSourceGroups(cx *Cx, groups []SearchGroup, opts ...SearchOption) RowSource {
	return rt.NewSearchSourceGroups(cx, groups, opts...)
}

// SetSearchItems replaces the items of a flat search RowSource.
func SetSearchItems(cx *Cx, src RowSource, items []SearchItem) { rt.SetSearchItems(cx, src, items) }

// SetSearchGroups replaces the groups of a grouped search RowSource.
func SetSearchGroups(cx *Cx, src RowSource, groups []SearchGroup) {
	rt.SetSearchGroups(cx, src, groups)
}

// AnswerSearch marks asynchronous search request id on src as fulfilled.
func AnswerSearch(cx *Cx, src RowSource, id uint64) { rt.AnswerSearch(cx, src, id) }

// SearchRules constrains selection counts and locked/blocked values on a search source.
type SearchRules = rt.SearchRules

// SearchWillChange installs selection validation rules on search source src.
func SearchWillChange(cx *Cx, src RowSource, rules SearchRules) { rt.SearchWillChange(cx, src, rules) }

// RowAt identifies one item by Section and Row index inside a RowSource.
type RowAt = rt.RowAt

// RowSource is a handle to a virtualized row/item provider whose visible items
// are materialized on demand as child views.
type RowSource = rt.RowSource

// SourceTest is a diagnostic event payload for testing RowSource requests.
type SourceTest = rt.SourceTest

// MaxRows is the maximum number of rows supported in a single RowSource section.
const MaxRows = rt.MaxRows

// SourceOption configures optional settings on NewRowSource and derived sources.
type SourceOption = rt.SourceOption

// Initial sets the number of row views pre-allocated when a RowSource is created.
func Initial(n int) SourceOption { return rt.Initial(n) }

// Capacity sets the maximum number of cached row views retained by a RowSource.
func Capacity(n int) SourceOption { return rt.Capacity(n) }

// CountFunc registers a function consulted by Refresh to sync a RowSource's count.
func CountFunc(f func() int) SourceOption { return rt.CountFunc(f) }

// TestSourceEvents attaches a diagnostic SourceTest handler to a RowSource.
func TestSourceEvents(h func(cx *Cx, e SourceTest)) SourceOption { return rt.TestSourceEvents(h) }

// NewRowSource creates a virtualized RowSource with count rows in section 0.
func NewRowSource(cx *Cx, count int, row func(cx *Cx, at RowAt) Element, opts ...SourceOption) RowSource {
	return rt.NewRowSource(cx, count, row, opts...)
}

// SetCount updates the row count of section 0 in src.
func SetCount(cx *Cx, src RowSource, count int) { rt.SetCount(cx, src, count) }

// SetSectionCount updates the row count of section in src.
func SetSectionCount(cx *Cx, src RowSource, section int, count int) {
	rt.SetSectionCount(cx, src, section, count)
}

// Invalidate marks cached row views in section 0 with indices in [lo, hi) dirty.
func Invalidate(cx *Cx, src RowSource, lo int, hi int) { rt.Invalidate(cx, src, lo, hi) }

// Refresh syncs all CountFunc row sources and marks every live view dirty.
func Refresh(cx *Cx) { rt.Refresh(cx) }

// Answer sends a string reply to diagnostic request id on src.
func Answer(cx *Cx, src RowSource, id uint64, text string) { rt.Answer(cx, src, id, text) }

// ScrollTo scrolls a list backed by src to row.
func ScrollTo(cx *Cx, src RowSource, row int) { rt.ScrollTo(cx, src, row) }

// TestSourceRequest sends a synthetic request to src in diagnostic tests.
func TestSourceRequest(cx *Cx, src RowSource, text string) { rt.TestSourceRequest(cx, src, text) }

// TestSourceTake reads and consumes the last answer recorded for src in tests.
func TestSourceTake(cx *Cx, src RowSource) (string, bool) { return rt.TestSourceTake(cx, src) }

// TestSourceRows returns the number of currently bound row views on src.
func TestSourceRows(src RowSource) int { return rt.TestSourceRows(src) }

// Rows is a handle to a plain string list backing UniformList.
type Rows = rt.Rows

// NewRows allocates an empty Rows store outside a view function.
func NewRows(cx *Cx) Rows { return rt.NewRows(cx) }

// UniformListEl is the element handle returned by UniformList and SourceList.
type UniformListEl = rt.UniformListEl

// UniformList renders a fixed-height scrollable list backed by Rows.
func UniformList(id string, rows Rows, st ...Style) UniformListEl {
	return rt.UniformList(id, rows, st...)
}

// SourceList renders a fixed-row-height virtualized list backed by RowSource src.
func SourceList(id string, src RowSource, rowHeight float32, st ...Style) UniformListEl {
	return rt.SourceList(id, src, rowHeight, st...)
}

// Text creates a leaf text element displaying s.
func Text(s string) Element { return rt.Text(s) }

// VirtualListEl is the element handle returned by VVirtualList and HVirtualList.
type VirtualListEl = rt.VirtualListEl

// VVirtualList renders a vertically scrolling variable-size virtual list.
func VVirtualList(id string, src RowSource, sizes [][2]float32, st ...Style) VirtualListEl {
	return rt.VVirtualList(id, src, sizes, st...)
}

// HVirtualList renders a horizontally scrolling variable-size virtual list.
func HVirtualList(id string, src RowSource, sizes [][2]float32, st ...Style) VirtualListEl {
	return rt.HVirtualList(id, src, sizes, st...)
}

// Window is a handle to an open GPUI window.
type Window = rt.Window

// WindowStyle selects native window chrome and floating presentation behavior.
type WindowStyle = rt.WindowStyle

// WindowNormal creates a standard titled desktop window.
const WindowNormal = rt.WindowNormal

// WindowPopup creates a borderless popup window.
const WindowPopup = rt.WindowPopup

// WindowFloating creates an always-on-top floating window.
const WindowFloating = rt.WindowFloating

// WindowConfig configures size, title, style, position, flags, AppID, and icon
// when opening a window with OpenWindowWith.
type WindowConfig = rt.WindowConfig

// OpenWindow opens a standard window displaying root with the given title and size.
func OpenWindow(cx *Cx, title string, width float32, height float32, root View) Window {
	return rt.OpenWindow(cx, title, width, height, root)
}

// OpenWindowWith opens a window configured by c displaying root.
func OpenWindowWith(cx *Cx, c WindowConfig, root View) Window { return rt.OpenWindowWith(cx, c, root) }

// CloseWindow closes window w.
func CloseWindow(cx *Cx, w Window) { rt.CloseWindow(cx, w) }

// SetWindowTitle updates the title of window w.
func SetWindowTitle(cx *Cx, w Window, title string) { rt.SetWindowTitle(cx, w, title) }

// ActivateWindow raises and focuses window w.
func ActivateWindow(cx *Cx, w Window) { rt.ActivateWindow(cx, w) }

// QuitOnLastWindow controls whether the app exits when its last window closes.
func QuitOnLastWindow(cx *Cx, yes bool) { rt.QuitOnLastWindow(cx, yes) }

// WindowInfo describes a window's current title, bounds, and active state.
type WindowInfo = rt.WindowInfo

// GetWindowInfo returns the current WindowInfo for window w.
func GetWindowInfo(cx *Cx, w Window) WindowInfo { return rt.GetWindowInfo(cx, w) }

// Windows returns handles for all currently open application windows.
func Windows(cx *Cx) []Window { return rt.Windows(cx) }

// OnWindowClosed registers f to run after window w closes.
func OnWindowClosed(cx *Cx, w Window, f func(cx *Cx, w Window)) ActionListener {
	return rt.OnWindowClosed(cx, w, f)
}

// OnWindowShouldClose registers a close-veto callback for window w; returning
// false cancels the close.
func OnWindowShouldClose(cx *Cx, w Window, f func(cx *Cx) bool) ActionListener {
	return rt.OnWindowShouldClose(cx, w, f)
}

// OnAppQuit registers f to run when the application is quitting.
func OnAppQuit(cx *Cx, f func(cx *Cx)) ActionListener { return rt.OnAppQuit(cx, f) }

// ReduceMotion reports whether the reduced-motion accessibility preference is active.
func ReduceMotion(cx *Cx) bool { return rt.ReduceMotion(cx) }

// SetReduceMotion overrides the reduced-motion accessibility preference.
func SetReduceMotion(cx *Cx, reduce bool) { rt.SetReduceMotion(cx, reduce) }

// TestShouldClose invokes window w's OnWindowShouldClose callback in tests.
func TestShouldClose(cx *Cx, w Window) bool { return rt.TestShouldClose(cx, w) }

// OnFrame installs or clears (when f is nil) a per-frame callback on the
// current turn's window.
func OnFrame(cx *Cx, f func(cx *Cx)) { rt.OnFrame(cx, f) }

// Quit requests a clean shutdown of the application event loop.
func Quit(cx *Cx) { rt.Quit(cx) }

// NewFocusHandle allocates a GPUI focus handle outside a view function.
func NewFocusHandle(cx *Cx) rt.FocusHandle { return rt.NewFocusHandle(cx) }

// Focus moves keyboard focus to f.
func Focus(cx *Cx, f rt.FocusHandle) { rt.Focus(cx, f) }

// Subscription is a releasable handle to a kit state event subscription.
type Subscription = rt.Subscription

// Release explicitly frees a handle (View, kit state, RowSource, Timer,
// Completion, Subscription, or ActionListener). Releasing a View or panel
// cascades to all child handles created during its init or event turns. Must
// not be called inside a view render function or twice on the same handle.
func Release(cx *Cx, x any) { rt.Release(cx, x) }

// Retain promotes a frame-temporary handle (such as LeafGeometry returned by
// editor.BlockHost) into a long-lived handle that survives past the next frame.
func Retain(cx *Cx, x rt.Handled) { rt.Retain(cx, x) }

// TestFire invokes kept handler slot from Rust in protocol tests.
func TestFire(cx *Cx, slot uint32) int32 { return rt.TestFire(cx, slot) }

// TestFireValue fires a value-returning TestPayload callback round-trip in tests.
func TestFireValue(cx *Cx, h func(cx *Cx, p TestPayload) string, text string, n uint32) string {
	return rt.TestFireValue(cx, h, text, n)
}

// TestFireNoValue fires a zero-argument callback through the value test hook.
func TestFireNoValue(cx *Cx, h func(cx *Cx), text string, n uint32) string {
	return rt.TestFireNoValue(cx, h, text, n)
}

// TestFireInUpdate invokes slot while state s is actively borrowed in Rust.
func TestFireInUpdate(cx *Cx, s rt.Handled, slot uint32) int32 {
	return rt.TestFireInUpdate(cx, s, slot)
}

// TestFireQueued queues an integer callback on state s in protocol tests.
func TestFireQueued(cx *Cx, s rt.Handled, h func(cx *Cx, row int), row int) {
	rt.TestFireQueued(cx, s, h, row)
}

// TestPanic triggers a deliberate Rust panic to test LibraryError recovery.
func TestPanic(cx *Cx, later bool) { rt.TestPanic(cx, later) }

// TestFireShape dispatches raw payload bytes for shape through Rust in tests.
func TestFireShape(cx *Cx, h any, shape uint16, payload []byte) (bool, []byte) {
	return rt.TestFireShape(cx, h, shape, payload)
}

// Handle extracts the raw 64-bit Rust handle from x.
func Handle(x rt.Handled) uint64 { return rt.Handle(x) }

// TestObjCounts returns the counts of temporary and live Rust object handles.
func TestObjCounts(cx *Cx) (int, int) { return rt.TestObjCounts(cx) }

// TestLiveCounts returns live Rust counts of state, view, and focus handles.
func TestLiveCounts(cx *Cx) (states, views, focus int) { return rt.TestLiveCounts(cx) }

// TestLife returns GC-released and currently GC-tracked handle counts.
func TestLife() (released, tracked int) { return rt.TestLife() }

// TestValuePins returns diagnostic counts of pinned and deferred-cleanup values.
func TestValuePins(cx *Cx) (int, int) { return rt.TestValuePins(cx) }

// TestEntityHandles exercises entity-handle identity round-trips in tests.
func TestEntityHandles(cx *Cx) (uint64, uint64, uint64, uint64) { return rt.TestEntityHandles(cx) }

// TestDragPayload round-trips an ExternalDragPayload through the diagnostic library.
func TestDragPayload(cx *Cx, p ExternalDragPayload) []ExternalFile { return rt.TestDragPayload(cx, p) }

// TestPrepaintOrder exercises prepaint callback ordering in diagnostic tests.
func TestPrepaintOrder(cx *Cx, order []int) []int { return rt.TestPrepaintOrder(cx, order) }

// PlatformLog records diagnostic URL open and dialog request counters.
type PlatformLog = rt.PlatformLog

// GetPlatformLog returns the current diagnostic PlatformLog snapshot.
func GetPlatformLog(cx *Cx) PlatformLog { return rt.GetPlatformLog(cx) }

// TestEditorText returns diagnostic counts of parsed blocks and editor view rows.
func TestEditorText(cx *Cx) (int, int) { return rt.TestEditorText(cx) }

// TestEditorHostCounts returns diagnostic counts for BlockHost resources and events.
func TestEditorHostCounts(cx *Cx) (int, int, int, int, int) { return rt.TestEditorHostCounts(cx) }

// TestKeptCount returns the number of currently live long-lived handler slots.
func TestKeptCount() int { return rt.TestKeptCount() }

// Cx is the UI-thread execution context passed to init, view, frame, and event
// callbacks. A *Cx is tied to the exact turn that received it and invalidated as
// soon as that callback returns; storing and reusing it later fails with *Error.
type Cx = rt.Cx

// View is a handle to a GPUI view backed by a Go render function and optional
// init callback. Embed a child View inside a parent element with .Child(v).
type View = rt.View

// NewView creates a view with an optional init callback (run once when the
// view mounts in a window) and a required render function (run whenever the
// view is marked dirty). Must be called outside a view render function.
func NewView(init func(cx *Cx), render func(cx *Cx) Element) View { return rt.NewView(init, render) }

// Notify marks view v dirty in the current turn so its render function runs on
// the next frame. Must be called outside a view render function.
func Notify(cx *Cx, v View) { rt.Notify(cx, v) }

// Quiet suppresses the automatic dirty mark that event dispatch normally places
// on the handler's owning view after the handler returns.
func Quiet(cx *Cx) { rt.Quiet(cx) }

// KitError wraps an error message returned by an underlying Rust gpui-kit operation.
type KitError = rt.KitError

// Option represents an optional value of type T.
type Option[T any] = rt.Option[T]

// Some returns a present Option holding v.
func Some[T any](v T) Option[T] { return rt.Some[T](v) }

// None returns an empty Option of type T.
func None[T any]() Option[T] { return rt.None[T]() }
