// The runtime's API for apps: package internal/rt holds the runtime, and these are its names apps
// use (docs/GO.md section 1). Types are aliases (their methods are rt's), functions call rt's.

package gk

import (
	"time"

	"gpuikit.dev/gk/internal/rt"
)

// ---- rt_app.go

// Action is an action of the app's action table, or a built-in one of GPUI or the kit.
type Action = rt.Action

// NewAction names an action and returns it: the same name (and data) gives the same action.
// data is the JSON a built-in action with fields takes ("" for none).
func NewAction(cx *Cx, name string, data ...string) Action { return rt.NewAction(cx, name, data...) }

// KitActionValue is one of the kit's own action values (gk.NewCancelDefault(),
// gk.NewSelectDownDefault()): a value node gk.ActionOf names as an Action.
type KitActionValue = rt.KitActionValue

// ActionOf names one of the kit's own action values as an Action, which menus, key bindings and
// Kbd take (PROTOCOLS.md 13.11). Make the value in the handler or init that calls; the same action
// value always gives the same Action.
func ActionOf(cx *Cx, v KitActionValue) Action { return rt.ActionOf(cx, v) }

// ActionHandler is what an element's OnAction takes: the action to listen for and the handler.
type ActionHandler = rt.ActionHandler

// Act pairs an action with its handler: gk.Div().OnAction(gk.Act(save, a.save)).
func Act(a Action, f func(cx *Cx, a Action)) ActionHandler { return rt.Act(a, f) }

// KeyBinding is keystrokes ("ctrl-s", "ctrl-k ctrl-c"), the action, and the key context it
// applies in (When: "Editor", "Editor && !Readonly"; "" for everywhere).
type KeyBinding = rt.KeyBinding

// BindKeys adds key bindings to the app's keymap. A later binding of the same keys wins.
func BindKeys(cx *Cx, bindings ...KeyBinding) { rt.BindKeys(cx, bindings...) }

// ClearKeyBindings removes every key binding, the kit's included.
func ClearKeyBindings(cx *Cx) { rt.ClearKeyBindings(cx) }

// ActionListener is an app-wide action listener (or a window-closed one); gk.Release ends it.
type ActionListener = rt.ActionListener

// OnAppAction runs f whenever the app action is dispatched and no element on the focus path
// took it (GPUI's global action listeners).
func OnAppAction(cx *Cx, a Action, f func(cx *Cx, a Action)) ActionListener {
	return rt.OnAppAction(cx, a, f)
}

// StopPropagation, in the handler of a click, a mouse or a key event, keeps the event from the
// elements around the one whose handler runs: a click on a checkbox in a list row then does
// not also select the row.
func StopPropagation(cx *Cx) { rt.StopPropagation(cx) }

// ShortcutOption changes a Shortcut.
type ShortcutOption = rt.ShortcutOption

// WhenIn limits a shortcut's keys to a key context: only while the focus is inside an element
// with .KeyContext(ctx).
func WhenIn(ctx string) ShortcutOption { return rt.WhenIn(ctx) }

// ShortcutHandler is what gk.Shortcut runs: a func(cx), or a click handler (the method a
// button's OnClick takes, so one method serves the button and its keys; it gets a ClickEvent of
// Kind ClickKeyboard).
type ShortcutHandler = rt.ShortcutHandler

// Shortcut is a keyboard shortcut in one call: it makes an app action, binds keys to it and runs
// f when they are pressed, wherever the focus is (an app-wide listener, so it keeps working
// after a dialog or a click moved the focus). It returns the action, for a menu item (the menu
// shows the keys) or for more keys (gk.BindKeys). Given in a view's init, f redraws that view.
//
//	a.newTask = gk.Shortcut(cx, "ctrl-n", a.addTask)              // func (a *App) addTask(cx *gk.Cx)
//	gk.Shortcut(cx, "ctrl-t", a.toggleTheme)                       // also a button's OnClick
//	gk.Shortcut(cx, "delete", a.deleteTask, gk.WhenIn("Tasks"))
func Shortcut[F ShortcutHandler](cx *Cx, keys string, f F, opts ...ShortcutOption) Action {
	return rt.Shortcut[F](cx, keys, f, opts...)
}

// DispatchAction dispatches an action to the focused element of the turn's window (as its key
// binding would), or app-wide in a turn without a window. It runs after the handler.
func DispatchAction(cx *Cx, a Action) { rt.DispatchAction(cx, a) }

// ActionAvailable reports whether something on the focus path, or app-wide, takes the action.
func ActionAvailable(cx *Cx, a Action) bool { return rt.ActionAvailable(cx, a) }

// Keystrokes gives the keys of the action's binding that wins ("ctrl-s"), and false when it has
// none. keyContext: only bindings of that context ("" for any).
func Keystrokes(cx *Cx, a Action, keyContext string) (string, bool) {
	return rt.Keystrokes(cx, a, keyContext)
}

// Timer fires a handler between frames, never in the middle of one. gk.Release cancels it.
type Timer = rt.Timer

// After runs f once, d from now.
func After(cx *Cx, d time.Duration, f func(cx *Cx)) Timer { return rt.After(cx, d, f) }

// Every runs f every d until the timer is released.
func Every(cx *Cx, d time.Duration, f func(cx *Cx)) Timer { return rt.Every(cx, d, f) }

// ClipboardWrite puts text on the clipboard.
func ClipboardWrite(cx *Cx, text string) { rt.ClipboardWrite(cx, text) }

// ClipboardRead gives the clipboard's text, and false when it holds none.
func ClipboardRead(cx *Cx) (string, bool) { return rt.ClipboardRead(cx) }

// Post runs f on the UI thread, in a WAKE turn, with a cx. Any goroutine may call it; it is the
// only function of this package that may be called off the UI thread. However fast posts come,
// the library makes at most one WAKE turn per turn of its main loop, and the views they mark are
// described once, before the next frame.
//
//	go func() { items := load(path); gk.Post(func(cx *gk.Cx) { a.items = items; gk.Refresh(cx) }) }()
func Post(f func(cx *Cx)) { rt.Post(f) }

// Go runs work on a new goroutine and done with its result on the UI thread.
//
//	gk.Go(cx, func() []Article { return parse(path) }, func(cx *gk.Cx, as []Article) { a.articles = as; gk.Refresh(cx) })
func Go[T any](cx *Cx, work func() T, done func(cx *Cx, v T)) { rt.Go[T](cx, work, done) }

// DoneStatus says what happened to a request.
type DoneStatus = rt.DoneStatus

const DoneAnswered = rt.DoneAnswered

const DoneCancelled = rt.DoneCancelled

const DoneFailed = rt.DoneFailed

// PathsDone is what a file dialog answers.
type PathsDone = rt.PathsDone

// PromptDone is what a prompt answers: the index of the button chosen.
type PromptDone = rt.PromptDone

// Completion is a request that answers later. gk.Release cancels it while it waits.
type Completion = rt.Completion

// PathPrompt is what a file dialog may choose. Prompt: the text of its button, "" for the platform's.
type PathPrompt = rt.PathPrompt

// PromptForPaths shows the platform's dialog for opening files or directories; f gets the answer.
func PromptForPaths(cx *Cx, p PathPrompt, f func(cx *Cx, d PathsDone)) Completion {
	return rt.PromptForPaths(cx, p, f)
}

// PromptForNewPath shows the platform's dialog for saving a new file, starting in directory.
func PromptForNewPath(cx *Cx, directory string, suggestedName string, f func(cx *Cx, d PathsDone)) Completion {
	return rt.PromptForNewPath(cx, directory, suggestedName, f)
}

// PromptLevel is how serious a prompt is.
type PromptLevel = rt.PromptLevel

const PromptInfo = rt.PromptInfo

const PromptWarning = rt.PromptWarning

const PromptCritical = rt.PromptCritical

// PromptButtonKind marks a prompt's button as the OK or Cancel one.
type PromptButtonKind = rt.PromptButtonKind

const ButtonOther = rt.ButtonOther

const ButtonOk = rt.ButtonOk

const ButtonCancel = rt.ButtonCancel

// PromptButton is one answer of a prompt.
type PromptButton = rt.PromptButton

// Prompt asks a question in the turn's window with the platform's dialog where it has one and
// GPUI's own otherwise; f gets the index of the button chosen.
func Prompt(cx *Cx, level PromptLevel, message string, detail string, answers []PromptButton, f func(cx *Cx, d PromptDone)) Completion {
	return rt.Prompt(cx, level, message, detail, answers, f)
}

// OpenURL has the platform open a URL with the user's browser. It runs after the handler.
func OpenURL(cx *Cx, url string) { rt.OpenURL(cx, url) }

// PlatformAnswerPaths answers the oldest waiting file dialog (offscreen, or with
// GK_SCRIPTED_PLATFORM=1: tests). It reports whether one was waiting.
func PlatformAnswerPaths(cx *Cx, status DoneStatus, paths []string, newPath bool, errText string) bool {
	return rt.PlatformAnswerPaths(cx, status, paths, newPath, errText)
}

// PlatformAnswerPrompt answers the oldest waiting prompt with the index of a button.
func PlatformAnswerPrompt(cx *Cx, status DoneStatus, answer int) bool {
	return rt.PlatformAnswerPrompt(cx, status, answer)
}

// ---- rt_built.go

// Built is the view behind a view-built closure whose function describes a T (a DialogEl, a
// SheetEl, a PopupMenu). Make it once (in init) and give it to the call that opens the overlay
// each time; gk.Release(cx, b.View) forgets it.
type Built[T any] = rt.Built[T]

// NewBuilt makes the view that describes a T with render. Only in a handler or init: a view
// function may not make views.
func NewBuilt[T any](cx *Cx, render func(cx *Cx) T) Built[T] { return rt.NewBuilt[T](cx, render) }

// Builder is what an overlay opens with: a Built[T] made once with NewBuilt, or the function that
// describes the overlay, given right there (overlay.OpenDialog(cx, func(cx *gk.Cx) overlay.DialogEl {
// ... })). A function gets one view per place in the app's code that opens it, released with the
// view whose handler opened it.
type Builder[T any] = rt.Builder[T]

// ---- rt_capi.go

// Info is what the library reports about itself (any goroutine may ask).
type Info = rt.Info

// LibraryInfo reads the library's gk_info.
func LibraryInfo() Info { return rt.LibraryInfo() }

// Error is a mistake in app code the package found (an element used after its view returned, a
// released handle, a write in a view, ...), with the app's line. gk panics with it inside the
// turn; the boundary with the library recovers it, the app is shut down, and Run returns it.
// Nothing unwinds into the library.
type Error = rt.Error

// PanicError is a panic in app code (a handler, a view, init, a posted function, gk.Go's work
// function): once the library has shut down, Run panics again with it. Value is what the app
// panicked with (Unwrap gives it when it is an error), Stack the stack where it panicked.
type PanicError = rt.PanicError

// LibraryError is a fault inside the library, not in the app's Go code: Run returns it once the
// library has shut down.
type LibraryError = rt.LibraryError

// Run enters the library and returns when the app quits. Call it from main; start runs in the
// first turn (open a window there). With GK_SCRIPT=file in the environment the app runs
// offscreen under that gk-meter script (RunHeadless; it needs the diagnostic library).
//
// Run returns nil when the app quit, a *gk.Error when a mistake in app code stopped it (the
// message names the app's line), a *gk.LibraryError for a fault in the library, or an error
// when it could not start. A panic in app code shuts the app down and panics again from Run
// with a *gk.PanicError. Nothing exits the process: main's deferred functions run.
//
//	if err := gk.Run(start); err != nil {
//		log.Fatal(err)
//	}
func Run(start func(cx *Cx)) error { return rt.Run(start) }

// RunHeadless runs the app offscreen under a gk-meter script (the diagnostic library only). It
// returns and panics as Run does.
func RunHeadless(script string, start func(cx *Cx)) error { return rt.RunHeadless(script, start) }

// TestRunRaw calls gk_run with a gk_app changed as the runtime's version test asks: the
// library must refuse each with GK_ERR_VERSION. It returns the status and the library's message.
func TestRunRaw(hashXor uint64, majorAdd uint16, minorAdd uint16, sizeDelta uint32, noCallback bool) (int32, string) {
	return rt.TestRunRaw(hashXor, majorAdd, minorAdd, sizeDelta, noCallback)
}

// TestExpectHash makes the package expect another interface hash (the version test's other half).
func TestExpectHash(h uint64) { rt.TestExpectHash(h) }

// ---- rt_editor_lang.go

// HighlightQuery is what a highlighter is asked: the editor's language, its whole text, and the
// byte range to style.
type HighlightQuery = rt.HighlightQuery

// HighlightRun styles a byte range by a semantic name ("keyword", "string", "comment", ..), which
// the editor's resolver turns into a style (the kit's editor: the theme's syntax colors). Text no
// run covers is unstyled.
type HighlightRun = rt.HighlightRun

// FoldQuery is what a highlighter's folds function is asked: the language and the whole text.
type FoldQuery = rt.FoldQuery

// SyntaxQuery is what a language provider's syntax function is asked: where in the syntax an
// offset of the text sits (for pairing quotes and brackets, indenting).
type SyntaxQuery = rt.SyntaxQuery

// LanguageAlias names a language by another name ("rs" for "rust").
type LanguageAlias = rt.LanguageAlias

// Highlighter is a syntax highlighter made of the app's functions. Give Src where upstream takes
// an InputHighlighterFactory (editor.SetHighlighterFactory(cx, hl.Src)).
type Highlighter = rt.Highlighter

// NewHighlighter makes a highlighter for editors of languages (none: every language) whose runs
// styles gives, and whose fold ranges folds gives (nil: none). In init or a handler; release it
// with gk.Release(cx, hl.Src) once no editor uses it.
func NewHighlighter(cx *Cx, styles func(cx *Cx, q HighlightQuery) []HighlightRun, folds func(cx *Cx, q FoldQuery) []rt.FoldRange, languages ...string) Highlighter {
	return rt.NewHighlighter(cx, styles, folds, languages...)
}

// NewLanguageProvider makes the app's language service: aliases of language names, and where in
// the syntax an offset sits (syntax; nil: always code). Give the result where upstream takes an
// Rc<dyn LanguageProvider>. In init or a handler.
func NewLanguageProvider(cx *Cx, aliases []LanguageAlias, syntax func(cx *Cx, q SyntaxQuery) rt.SyntaxContext) RowSource {
	return rt.NewLanguageProvider(cx, aliases, syntax)
}

// ---- rt_emit.go

// Element is an element being described (gk.Div(), gk.Button("ok"), ...).
type Element = rt.Element

// Style is one style token (package s: s.Gap3, s.SizeFull).
type Style = rt.Style

// Color is an HSLA colour or a colour of the kit's theme or palette, resolved by the library each
// time it draws (package theme: theme.Background; package palette: palette.Blue500).
type Color = rt.Color

// HSLA is a colour by hue, saturation, lightness and alpha, each 0 to 1.
func HSLA(h float32, s float32, l float32, a float32) Color { return rt.HSLA(h, s, l, a) }

// ThemeColor is the theme's colour number n (package theme names them all).
func ThemeColor(n uint16) Color { return rt.ThemeColor(n) }

// PaletteColor is the kit's fixed colour number n (package palette names them all).
func PaletteColor(n uint16) Color { return rt.PaletteColor(n) }

// Hsla is a colour the library hands back (in an event, a read's result).
type Hsla = rt.Hsla

// Fill is what .Bg and the plot's paths and quads take (gpui's Fill and Background): a Color, or
// a LinearGradient. PROTOCOLS.md 11.38.
//
//	.Bg(theme.Background)
//	.Bg(gk.NewLinearGradient(90, gk.NewLinearColorStop(cyan, 0), gk.NewLinearColorStop(violet, 1)))
type Fill = rt.Fill

// NewLinearColorStop is gpui's linear_color_stop: c at percentage (0 to 1) of a gradient
// (gk.LinearColorStop is the kit's record of it).
func NewLinearColorStop(c Color, percentage float32) rt.LinearColorStop {
	return rt.NewLinearColorStop(c, percentage)
}

// LinearGradient is gpui's linear_gradient: from one stop to the other along Angle (degrees; 0 is
// up, 90 left to right, increasing clockwise), mixed in sRGB or, with Oklab, in the Oklab space.
type LinearGradient = rt.LinearGradient

// NewLinearGradient is gpui's linear_gradient(angle, from, to).
func NewLinearGradient(angle float32, from rt.LinearColorStop, to rt.LinearColorStop) LinearGradient {
	return rt.NewLinearGradient(angle, from, to)
}

// Length is what `impl Into<Length>`, DefiniteLength and AbsoluteLength accept.
type Length = rt.Length

// Px is a length in logical pixels.
func Px(v float32) Length { return rt.Px(v) }

// Rems is a length in rems (the root font size).
func Rems(v float32) Length { return rt.Rems(v) }

// Relative is a fraction of the parent's size (0.5 is half).
func Relative(v float32) Length { return rt.Relative(v) }

// Auto is the automatic length.
func Auto() Length { return rt.Auto() }

// ID is what an element id (`impl Into<ElementId>`) accepts: a string, an int, or gk.Key(name, n).
type ID = rt.ID

// NamedID is an id made of a name and a number: gk.Key("row", i).
type NamedID = rt.NamedID

// Key is an element id of a name and a number, for elements made in a loop: gk.Key("row", i).
func Key(name string, n int) NamedID { return rt.Key(name, n) }

// Range is a range of indices or byte offsets, Lo included, Hi not.
type Range = rt.Range

// ---- rt_gen_editor.go

// Font is gpui's Font as the binding carries it: a family, a weight (0: normal, 400) and a style;
// its features and fallbacks are gpui's defaults. DisplayMap and Prewrap take one.
type Font = rt.Font

// NewFont is a font of family at the normal weight and style.
func NewFont(family string) Font { return rt.NewFont(family) }

// ExprCheck returns "" when text is an expression of at most nargs arguments, else what is wrong.
func ExprCheck(text string, nargs int) string { return rt.ExprCheck(text, nargs) }

// MarkdownMatch is the kind of Markdown node a data plugin claims. The first six are blocks, the
// rest inline. Code and the JSX elements also match on Lang: a fenced code block's language, an
// element's name ("" matches every one).
type MarkdownMatch = rt.MarkdownMatch

const MarkdownMatchCode = rt.MarkdownMatchCode

const MarkdownMatchMath = rt.MarkdownMatchMath

const MarkdownMatchYaml = rt.MarkdownMatchYaml

const MarkdownMatchToml = rt.MarkdownMatchToml

const MarkdownMatchHTML = rt.MarkdownMatchHTML

const MarkdownMatchMdxElement = rt.MarkdownMatchMdxElement

const MarkdownMatchInlineCode = rt.MarkdownMatchInlineCode

const MarkdownMatchInlineMath = rt.MarkdownMatchInlineMath

const MarkdownMatchInlineHTML = rt.MarkdownMatchInlineHTML

const MarkdownMatchMdxInlineElement = rt.MarkdownMatchMdxInlineElement

// MarkdownPlugin is a data plugin: its name (the node's name), the nodes it claims, the row source
// that draws them, and an inline node's baseline in pixels from its top (0: upstream's). The library
// claims the nodes of the rule while upstream parses and draws each node as a row of Rows: a block's
// row reads a gk.MarkdownNodeRead, an inline node's a gk.MarkdownNodeRead and a
// gk.InlineRenderContext (make Rows with gk.Items).
type MarkdownPlugin = rt.MarkdownPlugin

// MarkdownPluginAny is what Plugin takes: one of the kit's plugins (gk.NewFrontmatterPlugin()) or
// a data plugin (gk.MarkdownPlugin).
type MarkdownPluginAny = rt.MarkdownPluginAny

// EntityID is the entity behind an input, textarea or editor handle, where upstream takes a
// gpui::EntityId (InputBase's SemanticOwner): gk.EntityIDOf(a.editor).
type EntityID = rt.EntityID

// EntityIDOf is the entity of a state handle.
func EntityIDOf(state rt.Handled) EntityID { return rt.EntityIDOf(state) }

// ---- rt_gen_items.go

// KitView is a view the kit hands an item function (a tooltip's content): place it as a child,
// once, in the element the function returns.
type KitView = rt.KitView

// Ask is a data closure's function, kept by the library with its cache of answers: K the closure's
// arguments (one argument, or a Tuple of them), V its result. gk.Release(cx, a.Src) ends it.
type Ask[K any, V any] = rt.Ask[K, V]

// NewAsk keeps f as a data closure's function: the library asks it for each key it draws without
// an answer, between frames, and caches what it returns. Needs a window: call it in init or in a
// handler, and keep the gk.Ask. The key and value types are the closure's: an ask of other types
// does not fit the builder, and one of types no closure of the kit has stops here.
func NewAsk[K any, V any](cx *Cx, f func(cx *Cx, key K) V) Ask[K, V] { return rt.NewAsk[K, V](cx, f) }

// TestAsks is the library's counts of data closure answers (gets, keys asked, answers cached).
func TestAsks(cx *Cx) (int, int, int) { return rt.TestAsks(cx) }

// TestLends is the library's counts of objects lent by kit state and given back (PROTOCOLS.md 9.17).
func TestLends(cx *Cx) (int, int) { return rt.TestLends(cx) }

// ---- rt_gen_motion.go

// Transformation is a scale (1 is the svg's size), a translation in pixels and a rotation in
// radians, about the svg's centre, and an alpha that multiplies the svg's own (none is 1, opaque;
// unlike an element's opacity it applies to the svg alone). Make one with gk.NewTransformation: the
// zero value has a zero scale, which draws nothing.
type Transformation = rt.Transformation

// NewTransformation is the identity: scale 1, no translation, no rotation, opaque.
func NewTransformation() Transformation { return rt.NewTransformation() }

// SvgEasing is how a value of a TimedTransformation changes over one pass (gpui's Easing of an svg,
// not gk.Easing of an animation). Heartbeat is two quick beats then a rest: it reaches To at the
// first beat and is back at From by the end of the period.
type SvgEasing = rt.SvgEasing

const SvgEasingLinear = rt.SvgEasingLinear

const SvgEasingQuadratic = rt.SvgEasingQuadratic

const SvgEasingEaseInOut = rt.SvgEasingEaseInOut

const SvgEasingEaseOutQuint = rt.SvgEasingEaseOutQuint

const SvgEasingHeartbeat = rt.SvgEasingHeartbeat

// TimedTransformation is gpui's TimedTransformation: the svg moves from From to To over every
// Period, forever, drawn by the library without a turn of the app (it replaces
// WithTransformation; layout and hit testing keep the svg's place). Each value has its own easing;
// Alternate goes back from To to From within each period instead of jumping; MaxFPS (0: no limit)
// caps the frames a second asked for while nothing else needs one.
type TimedTransformation = rt.TimedTransformation

// NewTimedTransform moves from `from` to `to` every period, every value with one easing.
func NewTimedTransform(period time.Duration, from Transformation, to Transformation, easing SvgEasing) TimedTransformation {
	return rt.NewTimedTransform(period, from, to, easing)
}

// NewTimedRotation turns `from` clockwise, one full turn every period (TimedTransformation::rotation).
func NewTimedRotation(period time.Duration, from Transformation) TimedTransformation {
	return rt.NewTimedRotation(period, from)
}

// ---- rt_gen_platform.go

// ExternalFile is a file (or a directory) handed to the platform when a drag leaves the window.
type ExternalFile = rt.ExternalFile

// ExternalDragPayload is what ExternalDragPayload takes: the files a drag of the app's value
// becomes outside the window (none: the drag stays inside the app).
type ExternalDragPayload = rt.ExternalDragPayload

// A11yChild is one synthetic accessibility node A11ySyntheticChildren adds under the element: its
// key (its id is derived from it and the element's, as upstream does), its role and its label.
type A11yChild = rt.A11yChild

// DragMoveEvent is gpui's DragMoveEvent of the library's drag value: where the pointer is (as a
// MouseEvent), the bounds of the element listening, and the value being dragged.
type DragMoveEvent = rt.DragMoveEvent

// ImageCache is an image cache the library keeps (gpui's RetainAllImageCache): the images drawn
// under an element given it stay loaded until it is released. gk.Release frees it.
type ImageCache = rt.ImageCache

// RetainAllImages is gpui's retain-all cache kept by the element id given (retain_all(id)).
type RetainAllImages = rt.RetainAllImages

// ImageCacheProvider is what Div's ImageCache takes: an ImageCache the app made, or a
// RetainAllImages kept by id (nil: one kept by the id "images").
type ImageCacheProvider = rt.ImageCacheProvider

// NewImageCache makes an image cache (gpui's RetainAllImageCache::new). Keep it in the app's state.
func NewImageCache(cx *Cx) ImageCache { return rt.NewImageCache(cx) }

// ScrollAnchor keeps an element in view of a scroll handle (gpui's ScrollAnchor): an element given
// it with AnchorScroll is scrolled to by gk.ScrollAnchorTo. gk.Release frees it.
type ScrollAnchor = rt.ScrollAnchor

// NewScrollAnchor makes an anchor for a scroll handle (ScrollAnchor::for_handle).
func NewScrollAnchor(cx *Cx, handle rt.ScrollHandle) ScrollAnchor {
	return rt.NewScrollAnchor(cx, handle)
}

// ScrollAnchorTo scrolls the anchor's handle so the element given the anchor is in view
// (ScrollAnchor::scroll_to), with the turn's window.
func ScrollAnchorTo(cx *Cx, a ScrollAnchor) { rt.ScrollAnchorTo(cx, a) }

// ---- rt_gen_values.go

// Fallible is a Result<T, E> of the kit read back: Value when OK, the error's text when not.
// Result gives it as Go's (value, error).
type Fallible[T any] = rt.Fallible[T]

// CivilDate is a calendar date (jiff's civil::Date): a date picker's and a calendar's dates. A date
// that does not exist (February 30), or a year outside -9999 to 9999, stops where it is written.
type CivilDate = rt.CivilDate

// CivilTime is a time of day (jiff's civil::Time): a time field's time.
type CivilTime = rt.CivilTime

// CivilDateTime is a date and a time of day (jiff's civil::DateTime).
type CivilDateTime = rt.CivilDateTime

// Weekday is a day of the week (jiff's civil::Weekday): a calendar's first day of the week.
type Weekday = rt.Weekday

const WeekdayMon = rt.WeekdayMon

const WeekdayTue = rt.WeekdayTue

const WeekdayWed = rt.WeekdayWed

const WeekdayThu = rt.WeekdayThu

const WeekdayFri = rt.WeekdayFri

const WeekdaySat = rt.WeekdaySat

const WeekdaySun = rt.WeekdaySun

// DaysInMonth is 28 to 31 for a month 1 to 12 (the proleptic Gregorian calendar, as jiff's), 0 for
// another month.
func DaysInMonth(year int32, month uint32) uint32 { return rt.DaysInMonth(year, month) }

// CellValue is a type a value cell holds: what a settings field has.
type CellValue = rt.CellValue

// Cell is a value the library holds for the kit to read while it draws: a settings field's value
// (gk.NewSettingFieldBoolSwitch(dark, setDark)), a field's dirty test. The page reads the cell each
// time it draws; the app writes it between frames with gk.CellSet and every window redraws.
// A kept handle: gk.Release frees it.
type Cell[T CellValue] = rt.Cell[T]

// NewCell makes a value cell holding value (immediate; not in a view function).
func NewCell[T CellValue](cx *Cx, value T) Cell[T] { return rt.NewCell[T](cx, value) }

// CellSet writes the cell (buffered, with the turn's writes; every window redraws then).
func CellSet[T CellValue](cx *Cx, c Cell[T], value T) { rt.CellSet[T](cx, c, value) }

// CellGet reads the cell's value now (after the turn's writes so far; allowed in a view function).
func CellGet[T CellValue](cx *Cx, c Cell[T]) T { return rt.CellGet[T](cx, c) }

// LspPosition is an LSP position: a line and a character (UTF-16 units) in it, from 0
// (cursor_position, PROTOCOLS.md 9.13).
type LspPosition = rt.LspPosition

// CompletionItem is one entry of the completion menu: Kind 0 for none, else LSP's
// CompletionItemKind (3 function, 6 variable, 14 keyword ...). InsertText defaults to Label.
type CompletionItem = rt.CompletionItem

// CodeBlock is a fenced code block as a highlighter gets it: its language (the word after the
// fence), its code, and the rest of the fence's info string.
type CodeBlock = rt.CodeBlock

// ArcData is one slice of a pie (upstream's ArcData): the row it stands for and its angles.
type ArcData = rt.ArcData

// StackPoint is one row of one series of a stack: its band from Y0 to Y1.
type StackPoint = rt.StackPoint

// StackSeries is one key's series of a stack (upstream's StackSeries).
type StackSeries = rt.StackSeries

// QuestionnaireValidator is QuestionnaireItemDefinition::validator read back: present when the
// item has one.
type QuestionnaireValidator = rt.QuestionnaireValidator

// QuestionnaireRules is a questionnaire item's validator as rules (with_validator, PROTOCOLS.md
// 11.26), checked by the library while the questionnaire updates: a limit of 0 is none; a length
// counts the free answer's characters; OneOf / NotOneOf (empty: any) the free answer must or must
// not be; SameAs / DiffersFrom name another item whose answer this one must equal or differ from;
// Message replaces the error ("{item}" is the item's name).
type QuestionnaireRules = rt.QuestionnaireRules

// BackgroundKind is the kind of a gpui Background read back.
type BackgroundKind = rt.BackgroundKind

const BackgroundKindSolid = rt.BackgroundKindSolid

const BackgroundKindLinearGradient = rt.BackgroundKindLinearGradient

const BackgroundKindPatternSlash = rt.BackgroundKindPatternSlash

const BackgroundKindCheckerboard = rt.BackgroundKindCheckerboard

// BackgroundColorSpace is the colour space a gradient interpolates in.
type BackgroundColorSpace = rt.BackgroundColorSpace

const BackgroundColorSpaceSrgb = rt.BackgroundColorSpaceSrgb

const BackgroundColorSpaceOklab = rt.BackgroundColorSpaceOklab

// BackgroundStop is one stop of a linear gradient read back.
type BackgroundStop = rt.BackgroundStop

// BackgroundValue is gpui's Background read back (try_parse_background, PROTOCOLS.md 11.23): its
// kind, colour space, solid colour (solid, slash pattern, checkerboard), the gradient's angle in
// degrees or the pattern's height, and a linear gradient's two stops.
type BackgroundValue = rt.BackgroundValue

// IntervalMatcher is a calendar matcher's interval (PROTOCOLS.md 11.21), both ways:
// gk.MatcherInterval{Value: gk.IntervalMatcher{Before: gk.Some(d)}} disables the days before
// Before and after After; none is no bound.
type IntervalMatcher = rt.IntervalMatcher

// RangeMatcher is a calendar matcher's range (PROTOCOLS.md 11.21), both ways: the days from From to
// To, both included; none is no bound.
type RangeMatcher = rt.RangeMatcher

// ---- rt_handler.go

// KeyMods is a set of modifier keys.
type KeyMods = rt.KeyMods

const ModControl = rt.ModControl

const ModAlt = rt.ModAlt

const ModShift = rt.ModShift

const ModPlatform = rt.ModPlatform

const ModFunction = rt.ModFunction

// PointerButton is a mouse button.
type PointerButton = rt.PointerButton

const ButtonLeft = rt.ButtonLeft

const ButtonRight = rt.ButtonRight

const ButtonMiddle = rt.ButtonMiddle

const ButtonBack = rt.ButtonBack

const ButtonForward = rt.ButtonForward

const ButtonNone = rt.ButtonNone

// ClickKind is what made a click.
type ClickKind = rt.ClickKind

const ClickMouse = rt.ClickMouse

const ClickKeyboard = rt.ClickKeyboard

const ClickTouch = rt.ClickTouch

// ClickEvent is GPUI's ClickEvent.
type ClickEvent = rt.ClickEvent

// KeyEvent is GPUI's KeyDownEvent or KeyUpEvent.
type KeyEvent = rt.KeyEvent

// MouseEvent is GPUI's MouseDownEvent, MouseUpEvent or MouseMoveEvent.
type MouseEvent = rt.MouseEvent

// TestPayload is the payload of the runtime's own test shape.
type TestPayload = rt.TestPayload

// NewHandler keeps a handler that returns a bool and belongs to no tape, and returns its slot
// (the runtime's tests fire it with TestFire).
func NewHandler(f func(cx *Cx) bool) uint32 { return rt.NewHandler(f) }

// ---- rt_linkall.go

// LinkAll links every GPUI-kit component family into the program and registers them all.
//
// A Go program built against the static library (the default) links a family only when
// it calls into it: gk.Button links the button family, gk.NewInputState the input family, and the
// linker leaves out the rest (with -ldflags=-extldflags=-Wl,--gc-sections, -Wl,-dead_strip on macOS;
// docs/GO.md section 8).
// LinkAll is the escape hatch for a program that wants everything, at the size of everything. It
// lives in a file of its own so that a program that never calls it does not link every family.
func LinkAll() { rt.LinkAll() }

// ---- rt_model.go

// Model holds a value of type T and the views that read it.
type Model[T any] = rt.Model[T]

// NewModel is a Model holding v (the zero Model holds T's zero value).
func NewModel[T any](v T) Model[T] { return rt.NewModel[T](v) }

// ---- rt_platform.go

// DragValue is what an app drags: an element's OnDrag takes it with the view drawn under the
// pointer while it is dragged, a drop's handler gets it back, and DragOver's style applies while
// one is over the element. One type for every drag of the app: tell kinds apart by ID (say, the kind
// in the top 32 bits and an index below).
type DragValue = rt.DragValue

// DropAccept is what an element's CanDrop takes, as data (the closure gpui calls while a drag
// moves cannot run app code): the app's drag values (every one, or only those whose ID is in IDs)
// and files dragged in from outside the app. The zero value takes nothing.
type DropAccept = rt.DropAccept

// ViewOrTooltip is what an element's Tooltip and HoverableTooltip take: a view of the app's (any
// gk.View, drawn as the tooltip), or the kit's tooltip, gk.NewKitTooltip(text) or
// gk.NewKitTooltipElement(element) with its builders (KeyBinding, Action, the styles). The kit's
// value is described again each time the tooltip shows, from the view that gave it.
type ViewOrTooltip = rt.ViewOrTooltip

// ImageBytes is an encoded image (a PNG, a JPEG, ..., gpui's Image) for gk.ImageSourceImage:
//
//	gk.Img(gk.ImageSourceImage{Value: gk.ImageBytes{Format: gk.ImageFormatPng, Bytes: data}})
type ImageBytes = rt.ImageBytes

// PixelImage is an image as pixels, for gk.NewRenderImage: Width x Height pixels of 4 bytes each,
// red, green, blue, alpha, row by row.
type PixelImage = rt.PixelImage

// RenderImage is an image the library made from pixels (gpui's RenderImage), shown by
// gk.ImageSourceRender and gk.ImageSourceCustom. Make it once, with gk.NewRenderImage, and keep it
// while a view shows it: gpui keeps each image's texture on the graphics card until the image is
// released, so pixels made into a new image on every frame would be a new texture on every frame.
// gk.Release frees it in every window; a view that names it after that stops with a message. New
// pixels are a new image: make it, show it, release the old one.
type RenderImage = rt.RenderImage

// NewRenderImage makes an image of p's pixels, converted once to the format gpui draws. Make it
// in init or in a handler, not in a view function; gk.Release frees it.
func NewRenderImage(cx *Cx, p PixelImage) RenderImage { return rt.NewRenderImage(cx, p) }

// TestRenderImage gives the library's image behind i (custom: through ImageSourceCustom's loader):
// its size, its first pixel as gpui keeps it (BGRA, a little-endian uint32) and gpui's id of it,
// the same for as long as i is kept. For tests.
func TestRenderImage(cx *Cx, i RenderImage, custom bool) (uint32, uint32, uint32, uint64) {
	return rt.TestRenderImage(cx, i, custom)
}

// TestImagesShown tells how many times views have handed gpui an image (gk.ImageSourceRender or
// gk.ImageSourceCustom), how many distinct images (gpui's ids, one texture each) those were, and
// how many times a release dropped an image's texture from a window. For tests; the diagnostic
// library only.
func TestImagesShown(cx *Cx) (uint64, uint32, uint64) { return rt.TestImagesShown(cx) }

// TestDropAccept tells what a DropAccept takes, as the library decodes it: the app's drag value of
// that id, and files from outside. For tests.
func TestDropAccept(cx *Cx, a DropAccept, id uint64) (bool, bool) {
	return rt.TestDropAccept(cx, a, id)
}

// OsAction is the operating system's own meaning of a menu item (macOS fills the item in).
type OsAction = rt.OsAction

const OsActionNone = rt.OsActionNone

const OsActionCut = rt.OsActionCut

const OsActionCopy = rt.OsActionCopy

const OsActionPaste = rt.OsActionPaste

const OsActionSelectAll = rt.OsActionSelectAll

const OsActionUndo = rt.OsActionUndo

const OsActionRedo = rt.OsActionRedo

// Menu is a menu of the menu bar, or a submenu.
type Menu = rt.Menu

// MenuItem is one of MenuSeparator, MenuAction, MenuSubmenu, MenuServices.
type MenuItem = rt.MenuItem

// MenuSeparator is a line between items.
type MenuSeparator = rt.MenuSeparator

// MenuAction is an item that dispatches an action (gk.NewAction or gk.Shortcut's); the menu shows
// its key binding.
type MenuAction = rt.MenuAction

// MenuSubmenu is an item that opens another menu.
type MenuSubmenu = rt.MenuSubmenu

// MenuServices is the operating system's Services menu (macOS; elsewhere nothing).
type MenuServices = rt.MenuServices

// SetMenus sets the app's menu bar (App::set_menus): the platform's where it has one (macOS), and
// always the kit's AppMenuBar. Items dispatch their action to the focused element, as its key
// binding would. It runs after the handler.
//
//	quit := gk.Shortcut(cx, "cmd-q", a.quit)
//	gk.SetMenus(cx, gk.Menu{Name: "My App", Items: []gk.MenuItem{gk.MenuAction{Name: "Quit", Action: quit}}})
func SetMenus(cx *Cx, menus ...Menu) { rt.SetMenus(cx, menus...) }

// Menus gives the menu bar as the platform holds it (App::get_menus).
func Menus(cx *Cx) []Menu { return rt.Menus(cx) }

// ---- rt_select.go

// SearchItem is one item of a select or combobox (upstream's SearchableListItem as data).
type SearchItem = rt.SearchItem

// SearchGroup is a titled group of items (upstream's SearchableGroup).
type SearchGroup = rt.SearchGroup

// SearchRequest is the event of a source made with gk.SearchRequests: the query changed.
type SearchRequest = rt.SearchRequest

// SearchOption changes a source made by NewSearchSource.
type SearchOption = rt.SearchOption

// SearchRow has the kit draw each item's row with row (at: the group and the item's index in it)
// instead of its title.
func SearchRow(row func(cx *Cx, at RowAt) Element) SearchOption { return rt.SearchRow(row) }

// SearchHeaders has the row function also describe each group's header row (at.Row is the header's marker, HeaderRow).
func SearchHeaders() SearchOption { return rt.SearchHeaders() }

// SearchRequests makes searches requests to the app (SearchRequest).
func SearchRequests(f func(cx *Cx, e SearchRequest)) SearchOption { return rt.SearchRequests(f) }

// NewSearchSource makes the row source of a select or combobox over a flat list of items. Needs
// a window: init or a handler.
func NewSearchSource(cx *Cx, items []SearchItem, opts ...SearchOption) RowSource {
	return rt.NewSearchSource(cx, items, opts...)
}

// NewSearchSourceGroups is NewSearchSource over titled groups.
func NewSearchSourceGroups(cx *Cx, groups []SearchGroup, opts ...SearchOption) RowSource {
	return rt.NewSearchSourceGroups(cx, groups, opts...)
}

// SetSearchItems replaces the items of a select's or combobox's source (a buffered write). A
// state made over it before shows them after .SetItems(cx, src).
func SetSearchItems(cx *Cx, src RowSource, items []SearchItem) { rt.SetSearchItems(cx, src, items) }

// SetSearchGroups is SetSearchItems for titled groups.
func SetSearchGroups(cx *Cx, src RowSource, groups []SearchGroup) {
	rt.SetSearchGroups(cx, src, groups)
}

// AnswerSearch answers a SearchRequest once the items are set.
func AnswerSearch(cx *Cx, src RowSource, id uint64) { rt.AnswerSearch(cx, src, id) }

// SearchRules is what a select's or combobox's selection may become, given in advance: upstream
// asks its delegate (SearchableListDelegate::on_will_change) while the list handles a click, where
// app code may not run, so the library applies upstream's change and then these rules. Blocked
// values are never added; Locked values are never taken out once selected; a change that would
// leave fewer than Min or more than Max values selected (0: no limit) is refused whole.
type SearchRules = rt.SearchRules

// SearchWillChange gives a select's or combobox's source its rules (buffered; replaces the
// source's rules; the zero SearchRules is upstream's default alone).
func SearchWillChange(cx *Cx, src RowSource, rules SearchRules) { rt.SearchWillChange(cx, src, rules) }

// ---- rt_source.go

// RowAt is the row a row view describes: its section (0 without sections) and its row.
type RowAt = rt.RowAt

// RowSource is a handle to a row source, which lives in the library. The kit's list, table,
// tree and select states take one.
type RowSource = rt.RowSource

// SourceTest is the event of the row sources' own test request.
type SourceTest = rt.SourceTest

// MaxRows is the most rows a section of a row source holds.
const MaxRows = rt.MaxRows

// SourceOption changes a row source made by NewRowSource.
type SourceOption = rt.SourceOption

// Initial is how many rows get their views at once, so the first frame that draws the source is
// complete (default 64).
func Initial(n int) SourceOption { return rt.Initial(n) }

// Capacity is how many row views are kept; far ones are evicted (default 512).
func Capacity(n int) SourceOption { return rt.Capacity(n) }

// CountFunc gives the source a count function: gk.Refresh takes the number of rows from it.
func CountFunc(f func() int) SourceOption { return rt.CountFunc(f) }

// TestSourceEvents is sourceEvents for the runtime's tests: the source's test requests.
func TestSourceEvents(h func(cx *Cx, e SourceTest)) SourceOption { return rt.TestSourceEvents(h) }

// NewRowSource makes a source of count rows (one section) whose rows row describes. Needs a
// window: call it in a view's init function or in a handler.
func NewRowSource(cx *Cx, count int, row func(cx *Cx, at RowAt) Element, opts ...SourceOption) RowSource {
	return rt.NewRowSource(cx, count, row, opts...)
}

// SetCount changes the number of rows of section 0. Rows at and past the new count are dropped;
// rows below it keep their views (invalidate them if their content moved).
func SetCount(cx *Cx, src RowSource, count int) { rt.SetCount(cx, src, count) }

// SetSectionCount is SetCount for one section.
func SetSectionCount(cx *Cx, src RowSource, section int, count int) {
	rt.SetSectionCount(cx, src, section, count)
}

// Invalidate has rows lo to hi (not included) of section 0 described again before the next frame.
func Invalidate(cx *Cx, src RowSource, lo int, hi int) { rt.Invalidate(cx, src, lo, hi) }

// Refresh is for after the app's data changed: every view is described again before the next
// frame, and so is every row a list, table or tree shows; a source made with CountFunc first
// takes its number of rows from it. A description that did not change costs a compare.
func Refresh(cx *Cx) { rt.Refresh(cx) }

// Answer answers a request a row source made (SourceTest's id) with a string.
func Answer(cx *Cx, src RowSource, id uint64, text string) { rt.Answer(cx, src, id, text) }

// ScrollTo scrolls the core list of a source so that row is at the top. A buffered write.
func ScrollTo(cx *Cx, src RowSource, row int) { rt.ScrollTo(cx, src, row) }

// TestSourceRequest has the library ask the source's events handler a question (SourceTest).
func TestSourceRequest(cx *Cx, src RowSource, text string) { rt.TestSourceRequest(cx, src, text) }

// TestSourceTake returns the answer to TestSourceRequest once it has arrived.
func TestSourceTake(cx *Cx, src RowSource) (string, bool) { return rt.TestSourceTake(cx, src) }

// TestSourceRows is the number of rows of a source that have a view now.
func TestSourceRows(src RowSource) int { return rt.TestSourceRows(src) }

// Rows is a handle to the rows of a list kept in the library (strings only), for UniformList.
type Rows = rt.Rows

// NewRows makes an empty list of rows in the library.
func NewRows(cx *Cx) Rows { return rt.NewRows(cx) }

// UniformListEl is a list of equal-height rows (GPUI's uniform_list over Rows, or the runtime's
// list over a RowSource).
type UniformListEl = rt.UniformListEl

// UniformList is GPUI's uniform_list over rows kept in the library.
func UniformList(id string, rows Rows, st ...Style) UniformListEl {
	return rt.UniformList(id, rows, st...)
}

// SourceList is the shared core's own list: a uniform list over a row source whose rows are
// rowHeight high (the kit's lists, tables and trees draw sources too).
func SourceList(id string, src RowSource, rowHeight float32, st ...Style) UniformListEl {
	return rt.SourceList(id, src, rowHeight, st...)
}

// Text is a string as an element, for a builder that takes any element where a Rust app would
// pass a string. For a child, the parent's own Text method is shorter.
func Text(s string) Element { return rt.Text(s) }

// VirtualListEl is gpui-kit's virtual list over a row source (docs/PROTOCOLS.md 13.31): rows of
// different sizes, each a view described by the source's row function, only those in view drawn.
type VirtualListEl = rt.VirtualListEl

// VVirtualList lays a source's rows out top to bottom, row i sizes[i][1] high (the width is the
// list's). One size per row: rows beyond the sizes are not drawn, sizes beyond the rows neither. A
// row without a view yet is a gap of its size for one frame (the rows protocol).
//
//	gk.VVirtualList("log", a.rows, a.heights, s.SizeFull)
func VVirtualList(id string, src RowSource, sizes [][2]float32, st ...Style) VirtualListEl {
	return rt.VVirtualList(id, src, sizes, st...)
}

// HVirtualList lays a source's rows out left to right, row i sizes[i][0] wide.
func HVirtualList(id string, src RowSource, sizes [][2]float32, st ...Style) VirtualListEl {
	return rt.HVirtualList(id, src, sizes, st...)
}

// ---- rt_state.go

// Window is a handle to a window.
type Window = rt.Window

// WindowStyle is how a window sits among others.
type WindowStyle = rt.WindowStyle

const WindowNormal = rt.WindowNormal

const WindowPopup = rt.WindowPopup

const WindowFloating = rt.WindowFloating

// WindowConfig is every option OpenWindowWith takes; zero values are GPUI's defaults.
type WindowConfig = rt.WindowConfig

// OpenWindow opens a window titled title, width by height logical pixels, whose root (inside
// the kit's Root) is the view.
//
//	gk.OpenWindow(cx, "Groceries", 520, 560, gk.NewView(a.init, a.view))
func OpenWindow(cx *Cx, title string, width float32, height float32, root View) Window {
	return rt.OpenWindow(cx, title, width, height, root)
}

// OpenWindowWith opens a window with every option (a minimum size, a position, a style, ...).
func OpenWindowWith(cx *Cx, c WindowConfig, root View) Window { return rt.OpenWindowWith(cx, c, root) }

// CloseWindow closes a window (after the handler); a zero Window is the turn's own.
func CloseWindow(cx *Cx, w Window) { rt.CloseWindow(cx, w) }

// SetWindowTitle changes a window's title.
func SetWindowTitle(cx *Cx, w Window, title string) { rt.SetWindowTitle(cx, w, title) }

// ActivateWindow brings a window to the front.
func ActivateWindow(cx *Cx, w Window) { rt.ActivateWindow(cx, w) }

// QuitOnLastWindow sets whether closing the last window quits the app (the default).
func QuitOnLastWindow(cx *Cx, yes bool) { rt.QuitOnLastWindow(cx, yes) }

// WindowInfo is a window's title, bounds and whether it is the active one.
type WindowInfo = rt.WindowInfo

// GetWindowInfo reads a window's title and bounds; a zero Window is the turn's own.
func GetWindowInfo(cx *Cx, w Window) WindowInfo { return rt.GetWindowInfo(cx, w) }

// Windows lists the open windows, in the order they were opened.
func Windows(cx *Cx) []Window { return rt.Windows(cx) }

// OnWindowClosed runs f when the window has closed; gk.Release ends it.
func OnWindowClosed(cx *Cx, w Window, f func(cx *Cx, w Window)) ActionListener {
	return rt.OnWindowClosed(cx, w, f)
}

// OnWindowShouldClose asks f before the user closes the window (its close button, Alt-F4, the
// taskbar; Window::on_window_should_close): f returns false to keep it open, say to ask about
// unsaved edits first, and closes it later with gk.CloseWindow, which does not ask again. Several
// handlers are asked in the order given; the first false keeps the window. A zero Window is the
// turn's own. gk.Release ends it. PROTOCOLS.md 11.38.
//
//	gk.OnWindowShouldClose(cx, gk.Window{}, func(cx *gk.Cx) bool {
//		if !a.dirty { return true }
//		gk.OpenDialog(cx, a.confirmClose) // its OK calls gk.CloseWindow(cx, a.win)
//		return false
//	})
func OnWindowShouldClose(cx *Cx, w Window, f func(cx *Cx) bool) ActionListener {
	return rt.OnWindowShouldClose(cx, w, f)
}

// OnAppQuit runs f once as the app quits (App::on_app_quit), however it quits: gk.Quit, the last
// window closed, the platform's quit (Cmd-Q on macOS). It runs with no window and must not wait
// long: gpui gives quit handlers a moment, then exits. Start what must outlive the app (a
// detached process) from here. gk.Release ends it. PROTOCOLS.md 11.38.
func OnAppQuit(cx *Cx, f func(cx *Cx)) ActionListener { return rt.OnAppQuit(cx, f) }

// ReduceMotion reports whether the app should draw non-essential motion still (gpui's
// App::reduce_motion: the system's setting once gk.BaseApplySystemReduceMotion follows it, or what
// gk.SetReduceMotion set). A view function may read it.
func ReduceMotion(cx *Cx) bool { return rt.ReduceMotion(cx) }

// SetReduceMotion sets the reduce-motion flag (App::set_reduce_motion); every window redraws. It
// runs after the handler.
func SetReduceMotion(cx *Cx, reduce bool) { rt.SetReduceMotion(cx, reduce) }

// TestShouldClose asks the window's should-close handlers as the platform does when the user closes
// it, and reports whether it may close. For tests (offscreen there is no close button to press).
func TestShouldClose(cx *Cx, w Window) bool { return rt.TestShouldClose(cx, w) }

// OnFrame asks for a FRAME turn before every frame of the turn's window: the only way app code
// runs on every frame, and it is opt-in. What f writes is in place before the views are
// described for that same frame.
func OnFrame(cx *Cx, f func(cx *Cx)) { rt.OnFrame(cx, f) }

// Quit ends the app.
func Quit(cx *Cx) { rt.Quit(cx) }

// NewFocusHandle makes a GPUI FocusHandle: give it to TrackFocus on an element, and move the
// keyboard focus there with gk.Focus.
func NewFocusHandle(cx *Cx) rt.FocusHandle { return rt.NewFocusHandle(cx) }

// Focus moves keyboard focus to the element that tracks the handle.
func Focus(cx *Cx, f rt.FocusHandle) { rt.Focus(cx, f) }

// Subscription is a handler subscribed to the events of kit state; gk.Release ends it.
type Subscription = rt.Subscription

// Release gives back anything held by handle: kit state, a library object, a subscription, a
// focus handle, a view, a row source, a timer, a request that waits, a listener. The handle is
// stale from then on; using it stops with a message, never worse.
func Release(cx *Cx, x any) { rt.Release(cx, x) }

// Retain keeps a temporary library object (one a read, a call or an event handed out) until
// gk.Release. Retain(cx, obj) returns nothing: the handle is the same.
func Retain(cx *Cx, x rt.Handled) { rt.Retain(cx, x) }

// TestFire has the library fire a kept handler synchronously and returns what it received.
func TestFire(cx *Cx, slot uint32) int32 { return rt.TestFire(cx, slot) }

// TestFireValue fires a handler of the test shape with (text, n) through the library and
// returns the string that came back.
func TestFireValue(cx *Cx, h func(cx *Cx, p TestPayload) string, text string, n uint32) string {
	return rt.TestFireValue(cx, h, text, n)
}

// TestFireNoValue fires a handler that returns nothing where the test shape expects a string:
// the library gets the default.
func TestFireNoValue(cx *Cx, h func(cx *Cx), text string, n uint32) string {
	return rt.TestFireNoValue(cx, h, text, n)
}

// TestFireInUpdate fires a kept handler from inside a state's own update, as a delegate would.
func TestFireInUpdate(cx *Cx, s rt.Handled, slot uint32) int32 {
	return rt.TestFireInUpdate(cx, s, slot)
}

// TestFireQueued raises an event with payload row from inside a state's own update; the handler
// runs when that update has ended.
func TestFireQueued(cx *Cx, s rt.Handled, h func(cx *Cx, row int), row int) {
	rt.TestFireQueued(cx, s, h, row)
}

// TestPanic makes the library panic: at once, inside the call, or later, when buffered writes
// are applied outside any call.
func TestPanic(cx *Cx, later bool) { rt.TestPanic(cx, later) }

// TestFireShape fires a handler of shape with a payload already on the wire, through the
// library, and returns whether the library decoded it into its Rust type and what the handler
// returned, encoded (the generated shape test).
func TestFireShape(cx *Cx, h any, shape uint16, payload []byte) (bool, []byte) {
	return rt.TestFireShape(cx, h, shape, payload)
}

// Handle returns the raw handle of x (tests only: a fake of another kind shows the check).
func Handle(x rt.Handled) uint64 { return rt.Handle(x) }

// ---- rt_tests.go

// TestObjCounts returns the temporary handles waiting for the next frame and the live library
// objects (core/tests/go/handles).
func TestObjCounts(cx *Cx) (int, int) { return rt.TestObjCounts(cx) }

// TestLiveCounts returns the library's live kit states, views and focus handles (tests).
func TestLiveCounts(cx *Cx) (states, views, focus int) { return rt.TestLiveCounts(cx) }

// TestLife returns how many handles the collector has released and how many the runtime follows
// (tests).
func TestLife() (released, tracked int) { return rt.TestLife() }

// TestValuePins returns the temporary handles pinned by views and the ones waiting for the next
// sweep (core/tests/go/values).
func TestValuePins(cx *Cx) (int, int) { return rt.TestValuePins(cx) }

// TestEntityHandles reads one entity back through the library's two paths (the values path and
// the dock's) and again, and returns the three handles and how many distinct handles the table
// holds for it (core/tests/go/values).
func TestEntityHandles(cx *Cx) (uint64, uint64, uint64, uint64) { return rt.TestEntityHandles(cx) }

// TestDragPayload has the library decode an external drag payload as an element's builder would and
// returns its files (core/tests/go/platform).
func TestDragPayload(cx *Cx, p ExternalDragPayload) []ExternalFile { return rt.TestDragPayload(cx, p) }

// TestPrepaintOrder has the library decode a dynamic prepaint order and returns it
// (core/tests/go/platform).
func TestPrepaintOrder(cx *Cx, order []int) []int { return rt.TestPrepaintOrder(cx, order) }

// PlatformLog is what the scripted platform recorded (core/tests/go/platform): the URLs opened, the
// requests made, answers delivered and discarded, requests waiting, and whether the platform is
// scripted.
type PlatformLog = rt.PlatformLog

// GetPlatformLog reads the platform's log.
func GetPlatformLog(cx *Cx) PlatformLog { return rt.GetPlatformLog(cx) }

// TestEditorText returns the library's counts of the editor family: Markdown nodes the data plugins
// claimed, and rows made for the distinct values of view closures (core/tests/go/editor2).
func TestEditorText(cx *Cx) (int, int) { return rt.TestEditorText(cx) }

// TestEditorHostCounts returns editor hosts decoded, leaves collected, leaf events delivered,
// decorators run and lent values edited (core/tests/go/editor2).
func TestEditorHostCounts(cx *Cx) (int, int, int, int, int) { return rt.TestEditorHostCounts(cx) }

// TestKeptCount returns the handlers the runtime keeps for the library (subscriptions, timers,
// handlers given to call ops).
func TestKeptCount() int { return rt.TestKeptCount() }

// ---- rt_turn.go

// Cx is the context of one turn, given to every init function, handler, view function and
// posted function. It is valid until that call returns; one kept longer stops the program on use.
type Cx = rt.Cx

// ---- rt_view.go

// View is an app view: a function that describes its elements, retained by gpui-fast as a view
// of its own. Embed one in another with .ChildView(v): the parent stays retained when the child
// is described again, which is the main tool for a fast app.
type View = rt.View

// NewView makes a view: render describes it whenever it is dirty, and init, if not nil, runs
// once when a window builds this view as its root (kit state that needs a window is made there).
// Call it in the start function, in an init function or in a handler, never in a view function.
//
//	a.root = gk.NewView(a.init, a.view)
func NewView(init func(cx *Cx), render func(cx *Cx) Element) View { return rt.NewView(init, render) }

// Notify marks a view dirty: it is described again before the next frame. An unchanged
// description costs no redraw.
func Notify(cx *Cx, v View) { rt.Notify(cx, v) }

// Quiet, called in a handler, keeps the handler's view from being marked for redraw when it
// returns (a hover that changes nothing on screen).
func Quiet(cx *Cx) { rt.Quiet(cx) }

// ---- rt_wire.go

// KitError is an error an upstream call that may fail returned.
type KitError = rt.KitError

// Option is an optional value: Some(v), or the zero Option for none.
type Option[T any] = rt.Option[T]

// Some is an Option holding v.
func Some[T any](v T) Option[T] { return rt.Some[T](v) }

// None is the empty Option (the zero value is the same).
func None[T any]() Option[T] { return rt.None[T]() }
