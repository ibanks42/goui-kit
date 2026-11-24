package rt

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

func RdFallible[T any](r *Reader, f func(r *Reader) T) Fallible[T] {
	ok := r.Bool()
	v := RdOpt(r, f)
	return Fallible[T]{OK: ok, Value: v, Err: r.Str()}
}

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

func PutCivilDate(o *Tape, d CivilDate) {
	if d.Year < -9999 || d.Year > 9999 {
		Stop("the date %d-%d-%d is outside the years -9999 to 9999 a date can hold", d.Year, d.Month, d.Day)
	}
	if d.Day < 1 || d.Day > DaysInMonth(d.Year, d.Month) {
		Stop("the date %d-%02d-%02d does not exist (month 1 to 12, day 1 to the last day of the month)", d.Year, d.Month, d.Day)
	}
	o.U32(uint32(d.Year))
	o.U32(d.Month)
	o.U32(d.Day)
}

func PutCivilTime(o *Tape, t CivilTime) {
	if t.Hour > 23 || t.Minute > 59 || t.Second > 59 || t.Nanosecond > 999_999_999 {
		Stop("the time %02d:%02d:%02d.%09d does not exist (hour 0 to 23, minute and second 0 to 59)", t.Hour, t.Minute, t.Second, t.Nanosecond)
	}
	o.U32(t.Hour)
	o.U32(t.Minute)
	o.U32(t.Second)
	o.U32(t.Nanosecond)
}

func PutCivilDateTime(o *Tape, v CivilDateTime) {
	PutCivilDate(o, v.Date)
	PutCivilTime(o, v.Time)
}

func PutWeekday(o *Tape, w Weekday) {
	if w > 6 {
		Stop("gk.Weekday(%d) is not a day of the week", uint8(w))
	}
	o.U8(uint8(w))
}

func RdCivilDate(r *Reader) CivilDate {
	y := int32(r.U32())
	m := r.U32()
	return CivilDate{y, m, r.U32()}
}

func RdCivilTime(r *Reader) CivilTime {
	h, mi, s := r.U32(), r.U32(), r.U32()
	return CivilTime{h, mi, s, r.U32()}
}

func RdCivilDateTime(r *Reader) CivilDateTime {
	d := RdCivilDate(r)
	return CivilDateTime{d, RdCivilTime(r)}
}

func rdWeekday(r *Reader) Weekday { return Weekday(r.U8()) }

// CellValue is a type a value cell holds: what a settings field has.
type CellValue interface{ bool | float64 | string }

// Cell is a value the library holds for the kit to read while it draws: a settings field's value
// (gk.NewSettingFieldBoolSwitch(dark, setDark)), a field's dirty test. The page reads the cell each
// time it draws; the app writes it between frames with gk.CellSet and every window redraws.
// A kept handle: gk.Release frees it.
type Cell[T CellValue] struct{ HCore }

func (c Cell[T]) handle() uint64 { return c.h }

// NewCell makes a value cell holding value (immediate; not in a view function).
func NewCell[T CellValue](cx *Cx, value T) Cell[T] {
	t := CheckCx(cx)
	if Vb.Active {
		Stop("gk.NewCell in a view function: make the cell in init or a handler and keep it")
	}
	t.Flush()
	o := NewReq(CCellNew)
	putCellValue(o, value)
	r := Reader{B: MustCall(*o)}
	return Cell[T]{MkH(r.U64())}
}

// CellSet writes the cell (buffered, with the turn's writes; every window redraws then).
func CellSet[T CellValue](cx *Cx, c Cell[T], value T) {
	if c.h == 0 {
		Stop("gk.CellSet on a zero Cell (make it with gk.NewCell)")
	}
	o := NewReq(CCellSet)
	o.U64(c.h)
	putCellValue(o, value)
	CheckCx(cx).DeferOp(*o)
}

// CellGet reads the cell's value now (after the turn's writes so far; allowed in a view function).
func CellGet[T CellValue](cx *Cx, c Cell[T]) T {
	CheckCx(cx).Flush()
	o := NewReq(CCellGet)
	o.U64(c.h)
	r := Reader{B: MustCall(*o)}
	n := int(r.U32())
	in := Reader{B: r.B[r.i : r.i+n]}
	var v T
	switch p := any(&v).(type) {
	case *bool:
		*p = in.Bool()
	case *float64:
		*p = in.F64()
	case *string:
		*p = in.Str()
	}
	return v
}

func putCellValue[T CellValue](o *Tape, value T) {
	var body Tape
	switch x := any(value).(type) {
	case bool:
		body.Bool(x)
	case float64:
		body.F64(x)
	case string:
		body.Str(x)
	default:
		Stop("a gk.Cell holds a bool, a float64 or a string, not a %T", value)
	}
	o.Bytes(body)
}

// LspPosition is an LSP position: a line and a character (UTF-16 units) in it, from 0
// (cursor_position, PROTOCOLS.md 9.13).
type LspPosition struct {
	Line      uint32
	Character uint32
}

func PutLspPosition(o *Tape, p LspPosition) {
	o.U32(p.Line)
	o.U32(p.Character)
}

func RdLspPosition(r *Reader) LspPosition {
	l := r.U32()
	return LspPosition{l, r.U32()}
}

// CompletionItem is one entry of the completion menu: Kind 0 for none, else LSP's
// CompletionItemKind (3 function, 6 variable, 14 keyword ...). InsertText defaults to Label.
type CompletionItem struct {
	Label      string
	Kind       uint8
	Detail     Option[string]
	InsertText Option[string]
}

func PutCompletionItem(o *Tape, c CompletionItem) {
	o.Str(c.Label)
	o.U8(c.Kind)
	putOptString(o, c.Detail)
	putOptString(o, c.InsertText)
}

func RdCompletionItem(r *Reader) CompletionItem {
	l := r.Str()
	k := r.U8()
	d := RdOpt(r, (*Reader).Str)
	return CompletionItem{l, k, d, RdOpt(r, (*Reader).Str)}
}

func putOptString(o *Tape, s Option[string]) {
	if v, ok := s.Get(); ok {
		o.U8(1)
		o.Str(v)
	} else {
		o.U8(0)
	}
}

// CodeBlock is a fenced code block as a highlighter gets it: its language (the word after the
// fence), its code, and the rest of the fence's info string.
type CodeBlock struct {
	Lang Option[string]
	Code string
	Meta Option[string]
}

func putCodeBlock(o *Tape, b CodeBlock) {
	putOptString(o, b.Lang)
	o.Str(b.Code)
	putOptString(o, b.Meta)
}

func RdCodeBlock(r *Reader) CodeBlock {
	l := RdOpt(r, (*Reader).Str)
	c := r.Str()
	return CodeBlock{l, c, RdOpt(r, (*Reader).Str)}
}

func putSearchItem(o *Tape, it SearchItem) {
	o.Str(it.Value)
	o.Str(it.Title)
	putOptString(o, it.Display)
	o.Str(it.Keywords)
	o.Bool(it.Disabled)
}

func RdSearchItem(r *Reader) SearchItem {
	v, t := r.Str(), r.Str()
	d := RdOpt(r, (*Reader).Str)
	k := r.Str()
	return SearchItem{Value: v, Title: t, Display: d, Keywords: k, Disabled: r.Bool()}
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

func RdArcData(r *Reader) ArcData {
	row, i := int(r.U64()), int(r.U64())
	v, s, e := r.F32(), r.F32(), r.F32()
	return ArcData{row, i, v, s, e, r.F32()}
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

func RdStackSeries(r *Reader) StackSeries {
	k := r.Str()
	i := int(r.U64())
	return StackSeries{k, i, RdList(r, func(r *Reader) StackPoint {
		y0, y1 := r.F32(), r.F32()
		return StackPoint{y0, y1, int(r.U64())}
	})}
}

// QuestionnaireValidator is QuestionnaireItemDefinition::validator read back: present when the
// item has one.
type QuestionnaireValidator struct{}

func RdQuestionnaireValidator(r *Reader) QuestionnaireValidator { return QuestionnaireValidator{} }

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

func PutQuestionnaireRules(o *Tape, q QuestionnaireRules) {
	o.U32(q.MinChoices)
	o.U32(q.MaxChoices)
	o.U32(q.MinLength)
	o.U32(q.MaxLength)
	for _, l := range [][]string{q.OneOf, q.NotOneOf} {
		o.U32(uint32(len(l)))
		for _, s := range l {
			o.Str(s)
		}
	}
	o.Str(q.SameAs)
	o.Str(q.DiffersFrom)
	o.Str(q.Message)
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

func RdBackgroundValue(r *Reader) BackgroundValue {
	var v BackgroundValue
	v.Kind = BackgroundKind(r.U8())
	v.ColorSpace = BackgroundColorSpace(r.U8())
	v.Solid = RdHsla(r)
	v.AngleOrHeight = r.F32()
	for i := range v.Stops {
		c := RdHsla(r)
		v.Stops[i] = BackgroundStop{c, r.F32()}
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

func putOptCivilDate(o *Tape, d Option[CivilDate]) {
	if v, ok := d.Get(); ok {
		o.U8(1)
		PutCivilDate(o, v)
		return
	}
	o.U8(0)
}

func PutIntervalMatcher(o *Tape, m IntervalMatcher) {
	putOptCivilDate(o, m.Before)
	putOptCivilDate(o, m.After)
}

func RdIntervalMatcher(r *Reader) IntervalMatcher {
	a := RdOpt(r, RdCivilDate)
	return IntervalMatcher{a, RdOpt(r, RdCivilDate)}
}

func PutRangeMatcher(o *Tape, m RangeMatcher) {
	putOptCivilDate(o, m.From)
	putOptCivilDate(o, m.To)
}

func RdRangeMatcher(r *Reader) RangeMatcher {
	a := RdOpt(r, RdCivilDate)
	return RangeMatcher{a, RdOpt(r, RdCivilDate)}
}
