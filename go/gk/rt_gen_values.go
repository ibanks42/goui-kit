package gk

// Values the generator's descriptor kinds carry that are not upstream records
// (docs/PROTOCOLS.md 9.14, 9.19, 11.21 to 11.26, 12.15, 13.9, 13.13, 13.19): jiff's calendar values,
// results that may fail, value cells, and the library values that travel by their fields.

// --- results read back (PROTOCOLS.md 13.9) -------------------------------------------------------

// Fallible is a Result<T, E> of the kit read back: Value when OK, the error's text when not.
// Result gives it as Go's (value, error).
type Fallible[T any] struct {
	OK    bool
	Value Option[T]
	Err   string
}

// Result is the value, or a *KitError with the error's text.
func (f Fallible[T]) Result() (T, error) {
	if !f.OK {
		var zero T
		return zero, &KitError{f.Err}
	}
	v, _ := f.Value.Get()
	return v, nil
}

func rdFallible[T any](r *reader, f func(r *reader) T) Fallible[T] {
	ok := r.bool()
	v := rdOpt(r, f)
	return Fallible[T]{OK: ok, Value: v, Err: r.str()}
}

// --- jiff's calendar values (PROTOCOLS.md 13.13) ---------------------------------------------------

// CivilDate is a calendar date (jiff's civil::Date): a date picker's and a calendar's dates. A date
// that does not exist (February 30), or a year outside -9999 to 9999, stops where it is written.
type CivilDate struct {
	Year  int32
	Month uint32 // 1 to 12
	Day   uint32 // 1 to the month's last day
}

// CivilTime is a time of day (jiff's civil::Time): a time field's time.
type CivilTime struct {
	Hour, Minute, Second uint32
	Nanosecond           uint32 // 0 to 999_999_999
}

// CivilDateTime is a date and a time of day (jiff's civil::DateTime).
type CivilDateTime struct {
	Date CivilDate
	Time CivilTime
}

// Weekday is a day of the week (jiff's civil::Weekday): a calendar's first day of the week.
type Weekday uint8

const (
	WeekdayMon Weekday = iota
	WeekdayTue
	WeekdayWed
	WeekdayThu
	WeekdayFri
	WeekdaySat
	WeekdaySun
)

// DaysInMonth is 28 to 31 for a month 1 to 12 (the proleptic Gregorian calendar, as jiff's), 0 for
// another month.
func DaysInMonth(year int32, month uint32) uint32 {
	switch month {
	case 1, 3, 5, 7, 8, 10, 12:
		return 31
	case 4, 6, 9, 11:
		return 30
	case 2:
		if year%4 == 0 && (year%100 != 0 || year%400 == 0) {
			return 29
		}
		return 28
	}
	return 0
}

func putCivilDate(o *tape, d CivilDate) {
	if d.Year < -9999 || d.Year > 9999 {
		stop("the date %d-%d-%d is outside the years -9999 to 9999 a date can hold", d.Year, d.Month, d.Day)
	}
	if d.Day < 1 || d.Day > DaysInMonth(d.Year, d.Month) {
		stop("the date %d-%02d-%02d does not exist (month 1 to 12, day 1 to the last day of the month)", d.Year, d.Month, d.Day)
	}
	o.u32(uint32(d.Year))
	o.u32(d.Month)
	o.u32(d.Day)
}

func putCivilTime(o *tape, t CivilTime) {
	if t.Hour > 23 || t.Minute > 59 || t.Second > 59 || t.Nanosecond > 999_999_999 {
		stop("the time %02d:%02d:%02d.%09d does not exist (hour 0 to 23, minute and second 0 to 59)", t.Hour, t.Minute, t.Second, t.Nanosecond)
	}
	o.u32(t.Hour)
	o.u32(t.Minute)
	o.u32(t.Second)
	o.u32(t.Nanosecond)
}

func putCivilDateTime(o *tape, v CivilDateTime) {
	putCivilDate(o, v.Date)
	putCivilTime(o, v.Time)
}

func putWeekday(o *tape, w Weekday) {
	if w > 6 {
		stop("gk.Weekday(%d) is not a day of the week", uint8(w))
	}
	o.u8(uint8(w))
}

func rdCivilDate(r *reader) CivilDate {
	y := int32(r.u32())
	m := r.u32()
	return CivilDate{y, m, r.u32()}
}

func rdCivilTime(r *reader) CivilTime {
	h, mi, s := r.u32(), r.u32(), r.u32()
	return CivilTime{h, mi, s, r.u32()}
}

func rdCivilDateTime(r *reader) CivilDateTime {
	d := rdCivilDate(r)
	return CivilDateTime{d, rdCivilTime(r)}
}

func rdWeekday(r *reader) Weekday { return Weekday(r.u8()) }

// --- value cells (PROTOCOLS.md 13.19) --------------------------------------------------------------

// CellValue is a type a value cell holds: what a settings field has.
type CellValue interface{ bool | float64 | string }

// Cell is a value the library holds for the kit to read while it draws: a settings field's value
// (gk.NewSettingFieldBoolSwitch(dark, setDark)), a field's dirty test. The page reads the cell each
// time it draws; the app writes it between frames with gk.CellSet and every window redraws.
// A kept handle: gk.Release frees it.
type Cell[T CellValue] struct{ h uint64 }

func (c Cell[T]) handle() uint64 { return c.h }

// NewCell makes a value cell holding value (immediate; not in a view function).
func NewCell[T CellValue](cx *Cx, value T) Cell[T] {
	t := checkCx(cx)
	if vb.active {
		stop("gk.NewCell in a view function: make the cell in init or a handler and keep it")
	}
	t.flush()
	o := newReq(cCellNew)
	putCellValue(o, value)
	r := reader{b: mustCall(*o)}
	return Cell[T]{r.u64()}
}

// CellSet writes the cell (buffered, with the turn's writes; every window redraws then).
func CellSet[T CellValue](cx *Cx, c Cell[T], value T) {
	if c.h == 0 {
		stop("gk.CellSet on a zero Cell (make it with gk.NewCell)")
	}
	o := newReq(cCellSet)
	o.u64(c.h)
	putCellValue(o, value)
	checkCx(cx).deferOp(*o)
}

// CellGet reads the cell's value now (after the turn's writes so far; allowed in a view function).
func CellGet[T CellValue](cx *Cx, c Cell[T]) T {
	checkCx(cx).flush()
	o := newReq(cCellGet)
	o.u64(c.h)
	r := reader{b: mustCall(*o)}
	n := int(r.u32())
	in := reader{b: r.b[r.i : r.i+n]}
	var v T
	switch p := any(&v).(type) {
	case *bool:
		*p = in.bool()
	case *float64:
		*p = in.f64()
	case *string:
		*p = in.str()
	}
	return v
}

func putCellValue[T CellValue](o *tape, value T) {
	var body tape
	switch x := any(value).(type) {
	case bool:
		body.bool(x)
	case float64:
		body.f64(x)
	case string:
		body.str(x)
	default:
		stop("a gk.Cell holds a bool, a float64 or a string, not a %T", value)
	}
	o.bytes(body)
}

// --- library values that travel by their fields ----------------------------------------------------

// LspPosition is an LSP position: a line and a character (UTF-16 units) in it, from 0
// (cursor_position, PROTOCOLS.md 9.13).
type LspPosition struct {
	Line      uint32
	Character uint32
}

func putLspPosition(o *tape, p LspPosition) {
	o.u32(p.Line)
	o.u32(p.Character)
}

func rdLspPosition(r *reader) LspPosition {
	l := r.u32()
	return LspPosition{l, r.u32()}
}

// CompletionItem is one entry of the completion menu: Kind 0 for none, else LSP's
// CompletionItemKind (3 function, 6 variable, 14 keyword ...). InsertText defaults to Label.
type CompletionItem struct {
	Label      string
	Kind       uint8
	Detail     Option[string]
	InsertText Option[string]
}

func putCompletionItem(o *tape, c CompletionItem) {
	o.str(c.Label)
	o.u8(c.Kind)
	putOptString(o, c.Detail)
	putOptString(o, c.InsertText)
}

func rdCompletionItem(r *reader) CompletionItem {
	l := r.str()
	k := r.u8()
	d := rdOpt(r, (*reader).str)
	return CompletionItem{l, k, d, rdOpt(r, (*reader).str)}
}

func putOptString(o *tape, s Option[string]) {
	if v, ok := s.Get(); ok {
		o.u8(1)
		o.str(v)
	} else {
		o.u8(0)
	}
}

// CodeBlock is a fenced code block as a highlighter gets it: its language (the word after the
// fence), its code, and the rest of the fence's info string.
type CodeBlock struct {
	Lang Option[string]
	Code string
	Meta Option[string]
}

func putCodeBlock(o *tape, b CodeBlock) {
	putOptString(o, b.Lang)
	o.str(b.Code)
	putOptString(o, b.Meta)
}

func rdCodeBlock(r *reader) CodeBlock {
	l := rdOpt(r, (*reader).str)
	c := r.str()
	return CodeBlock{l, c, rdOpt(r, (*reader).str)}
}

func putSearchItem(o *tape, it SearchItem) {
	o.str(it.Value)
	o.str(it.Title)
	putOptString(o, it.Display)
	o.str(it.Keywords)
	o.bool(it.Disabled)
}

func rdSearchItem(r *reader) SearchItem {
	v, t := r.str(), r.str()
	d := rdOpt(r, (*reader).str)
	k := r.str()
	return SearchItem{Value: v, Title: t, Display: d, Keywords: k, Disabled: r.bool()}
}

// ArcData is one slice of a pie (upstream's ArcData): the row it stands for and its angles.
type ArcData struct {
	Row        int
	Index      int // its place in the pie's order
	Value      float32
	StartAngle float32
	EndAngle   float32
	PadAngle   float32
}

func rdArcData(r *reader) ArcData {
	row, i := int(r.u64()), int(r.u64())
	v, s, e := r.f32(), r.f32(), r.f32()
	return ArcData{row, i, v, s, e, r.f32()}
}

// StackPoint is one row of one series of a stack: its band from Y0 to Y1.
type StackPoint struct {
	Y0, Y1 float32
	Row    int
}

// StackSeries is one key's series of a stack (upstream's StackSeries).
type StackSeries struct {
	Key    string
	Index  int
	Points []StackPoint
}

func rdStackSeries(r *reader) StackSeries {
	k := r.str()
	i := int(r.u64())
	return StackSeries{k, i, rdList(r, func(r *reader) StackPoint {
		y0, y1 := r.f32(), r.f32()
		return StackPoint{y0, y1, int(r.u64())}
	})}
}

// QuestionnaireValidator is QuestionnaireItemDefinition::validator read back: present when the
// item has one.
type QuestionnaireValidator struct{}

func rdQuestionnaireValidator(r *reader) QuestionnaireValidator { return QuestionnaireValidator{} }

// QuestionnaireRules is a questionnaire item's validator as rules (with_validator, PROTOCOLS.md
// 11.26), checked by the library while the questionnaire updates: a limit of 0 is none; a length
// counts the free answer's characters; OneOf / NotOneOf (empty: any) the free answer must or must
// not be; SameAs / DiffersFrom name another item whose answer this one must equal or differ from;
// Message replaces the error ("{item}" is the item's name).
type QuestionnaireRules struct {
	MinChoices  uint32
	MaxChoices  uint32
	MinLength   uint32
	MaxLength   uint32
	OneOf       []string
	NotOneOf    []string
	SameAs      string
	DiffersFrom string
	Message     string
}

func putQuestionnaireRules(o *tape, q QuestionnaireRules) {
	o.u32(q.MinChoices)
	o.u32(q.MaxChoices)
	o.u32(q.MinLength)
	o.u32(q.MaxLength)
	for _, l := range [][]string{q.OneOf, q.NotOneOf} {
		o.u32(uint32(len(l)))
		for _, s := range l {
			o.str(s)
		}
	}
	o.str(q.SameAs)
	o.str(q.DiffersFrom)
	o.str(q.Message)
}

// BackgroundKind is the kind of a gpui Background read back.
type BackgroundKind uint8

const (
	BackgroundKindSolid BackgroundKind = iota
	BackgroundKindLinearGradient
	BackgroundKindPatternSlash
	BackgroundKindCheckerboard
)

// BackgroundColorSpace is the colour space a gradient interpolates in.
type BackgroundColorSpace uint8

const (
	BackgroundColorSpaceSrgb BackgroundColorSpace = iota
	BackgroundColorSpaceOklab
)

// BackgroundStop is one stop of a linear gradient read back.
type BackgroundStop struct {
	Color      Hsla
	Percentage float32 // 0 to 1
}

// BackgroundValue is gpui's Background read back (try_parse_background, PROTOCOLS.md 11.23): its
// kind, colour space, solid colour (solid, slash pattern, checkerboard), the gradient's angle in
// degrees or the pattern's height, and a linear gradient's two stops.
type BackgroundValue struct {
	Kind          BackgroundKind
	ColorSpace    BackgroundColorSpace
	Solid         Hsla
	AngleOrHeight float32
	Stops         [2]BackgroundStop
}

func rdBackgroundValue(r *reader) BackgroundValue {
	var v BackgroundValue
	v.Kind = BackgroundKind(r.u8())
	v.ColorSpace = BackgroundColorSpace(r.u8())
	v.Solid = rdHsla(r)
	v.AngleOrHeight = r.f32()
	for i := range v.Stops {
		c := rdHsla(r)
		v.Stops[i] = BackgroundStop{c, r.f32()}
	}
	return v
}

// IntervalMatcher is a calendar matcher's interval (PROTOCOLS.md 11.21), both ways:
// gk.MatcherInterval{Value: gk.IntervalMatcher{Before: gk.Some(d)}} disables the days before
// Before and after After; none is no bound.
type IntervalMatcher struct {
	Before Option[CivilDate]
	After  Option[CivilDate]
}

// RangeMatcher is a calendar matcher's range (PROTOCOLS.md 11.21), both ways: the days from From to
// To, both included; none is no bound.
type RangeMatcher struct {
	From Option[CivilDate]
	To   Option[CivilDate]
}

func putOptCivilDate(o *tape, d Option[CivilDate]) {
	if v, ok := d.Get(); ok {
		o.u8(1)
		putCivilDate(o, v)
		return
	}
	o.u8(0)
}

func putIntervalMatcher(o *tape, m IntervalMatcher) {
	putOptCivilDate(o, m.Before)
	putOptCivilDate(o, m.After)
}

func rdIntervalMatcher(r *reader) IntervalMatcher {
	a := rdOpt(r, rdCivilDate)
	return IntervalMatcher{a, rdOpt(r, rdCivilDate)}
}

func putRangeMatcher(o *tape, m RangeMatcher) {
	putOptCivilDate(o, m.From)
	putOptCivilDate(o, m.To)
}

func rdRangeMatcher(r *reader) RangeMatcher {
	a := rdOpt(r, rdCivilDate)
	return RangeMatcher{a, rdOpt(r, rdCivilDate)}
}
