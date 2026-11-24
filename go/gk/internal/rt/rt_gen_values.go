package rt

// Fallible represents a Rust Result<T, E> decoded across the C ABI.
type Fallible[T any] struct {
	OK    bool
	Value Option[T]
	Err   string
}

// Result converts f into a standard Go (value, error) pair, wrapping Err as *KitError.
func (f Fallible[T]) Result() (T, error) {
	if !f.OK {
		var zero T
		return zero, &KitError{f.Err}
	}
	v, _ := f.Value.Get()
	return v, nil
}

// RdFallible decodes a Fallible[T] from r using f to decode the success value.
func RdFallible[T any](r *Reader, f func(r *Reader) T) Fallible[T] {
	ok := r.Bool()
	v := RdOpt(r, f)
	return Fallible[T]{OK: ok, Value: v, Err: r.Str()}
}

// CivilDate represents a calendar date without a time zone (year -9999..9999,
// month 1..12, day 1..DaysInMonth).
type CivilDate struct {
	Year  int32
	Month uint32
	Day   uint32
}

// CivilTime represents a wall-clock time without a time zone.
type CivilTime struct {
	Hour, Minute, Second uint32
	Nanosecond           uint32
}

// CivilDateTime combines a CivilDate and a CivilTime.
type CivilDateTime struct {
	Date CivilDate
	Time CivilTime
}

// Weekday identifies a day of the week (Monday = 0 through Sunday = 6).
type Weekday uint8

// Day-of-week values matching Rust jiff::civil::Weekday ordering.
const (
	WeekdayMon Weekday = iota
	WeekdayTue
	WeekdayWed
	WeekdayThu
	WeekdayFri
	WeekdaySat
	WeekdaySun
)

// DaysInMonth returns the number of calendar days in (year, month) using the
// proleptic Gregorian leap-year rules, or 0 for an invalid month.
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

// PutCivilDate validates that d is a real Gregorian date and encodes it onto o.
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

// PutCivilTime validates that t is a real wall-clock time and encodes it onto o.
func PutCivilTime(o *Tape, t CivilTime) {
	if t.Hour > 23 || t.Minute > 59 || t.Second > 59 || t.Nanosecond > 999_999_999 {
		Stop("the time %02d:%02d:%02d.%09d does not exist (hour 0 to 23, minute and second 0 to 59)", t.Hour, t.Minute, t.Second, t.Nanosecond)
	}
	o.U32(t.Hour)
	o.U32(t.Minute)
	o.U32(t.Second)
	o.U32(t.Nanosecond)
}

// PutCivilDateTime validates and encodes v onto o.
func PutCivilDateTime(o *Tape, v CivilDateTime) {
	PutCivilDate(o, v.Date)
	PutCivilTime(o, v.Time)
}

// PutWeekday validates w (0..6) and encodes it onto o.
func PutWeekday(o *Tape, w Weekday) {
	if w > 6 {
		Stop("gk.Weekday(%d) is not a day of the week", uint8(w))
	}
	o.U8(uint8(w))
}

// RdCivilDate decodes a CivilDate from r.
func RdCivilDate(r *Reader) CivilDate {
	y := int32(r.U32())
	m := r.U32()
	return CivilDate{y, m, r.U32()}
}

// RdCivilTime decodes a CivilTime from r.
func RdCivilTime(r *Reader) CivilTime {
	h, mi, s := r.U32(), r.U32(), r.U32()
	return CivilTime{h, mi, s, r.U32()}
}

// RdCivilDateTime decodes a CivilDateTime from r.
func RdCivilDateTime(r *Reader) CivilDateTime {
	d := RdCivilDate(r)
	return CivilDateTime{d, RdCivilTime(r)}
}

func rdWeekday(r *Reader) Weekday { return Weekday(r.U8()) }

// CellValue constrains the scalar types stored in a Rust-side Cell.
type CellValue interface{ bool | float64 | string }

// Cell is a handle to a Rust-managed mutable scalar cell shared with kit
// controls that read or write a value directly (such as settings fields).
type Cell[T CellValue] struct{ HCore }

func (c Cell[T]) handle() uint64 { return c.h }

// NewCell creates a Rust-side Cell initialized to value outside a view function.
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

// CellSet updates the value stored in Cell c.
func CellSet[T CellValue](cx *Cx, c Cell[T], value T) {
	if c.h == 0 {
		Stop("gk.CellSet on a zero Cell (make it with gk.NewCell)")
	}
	o := NewReq(CCellSet)
	o.U64(c.h)
	putCellValue(o, value)
	CheckCx(cx).DeferOp(*o)
}

// CellGet reads the current value stored in Cell c.
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

// LspPosition identifies a zero-based line and UTF-16 character column in an
// editor document, matching the Language Server Protocol Position struct.
type LspPosition struct {
	Line      uint32
	Character uint32
}

// PutLspPosition encodes p onto o.
func PutLspPosition(o *Tape, p LspPosition) {
	o.U32(p.Line)
	o.U32(p.Character)
}

// RdLspPosition decodes an LspPosition from r.
func RdLspPosition(r *Reader) LspPosition {
	l := r.U32()
	return LspPosition{l, r.U32()}
}

// CompletionItem represents one autocomplete suggestion passed to or read from
// an editor completion provider.
type CompletionItem struct {
	Label      string
	Kind       uint8
	Detail     Option[string]
	InsertText Option[string]
}

// PutCompletionItem encodes c onto o.
func PutCompletionItem(o *Tape, c CompletionItem) {
	o.Str(c.Label)
	o.U8(c.Kind)
	putOptString(o, c.Detail)
	putOptString(o, c.InsertText)
}

// RdCompletionItem decodes a CompletionItem from r.
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

// CodeBlock holds a fenced markdown code block's optional language tag, source
// text, and optional metadata string.
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

// RdCodeBlock decodes a CodeBlock from r.
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

// RdSearchItem decodes a SearchItem from r.
func RdSearchItem(r *Reader) SearchItem {
	v, t := r.Str(), r.Str()
	d := RdOpt(r, (*Reader).Str)
	k := r.Str()
	return SearchItem{Value: v, Title: t, Display: d, Keywords: k, Disabled: r.Bool()}
}

// ArcData holds the computed geometry of one pie or donut chart arc slice.
type ArcData struct {
	Row        int
	Index      int
	Value      float32
	StartAngle float32
	EndAngle   float32
	PadAngle   float32
}

// RdArcData decodes an ArcData from r.
func RdArcData(r *Reader) ArcData {
	row, i := int(r.U64()), int(r.U64())
	v, s, e := r.F32(), r.F32(), r.F32()
	return ArcData{row, i, v, s, e, r.F32()}
}

// StackPoint holds the baseline (Y0), top (Y1), and source row index of one
// point in a stacked chart series.
type StackPoint struct {
	Y0, Y1 float32
	Row    int
}

// StackSeries holds one stacked series key, series index, and its StackPoints.
type StackSeries struct {
	Key    string
	Index  int
	Points []StackPoint
}

// RdStackSeries decodes a StackSeries from r.
func RdStackSeries(r *Reader) StackSeries {
	k := r.Str()
	i := int(r.U64())
	return StackSeries{k, i, RdList(r, func(r *Reader) StackPoint {
		y0, y1 := r.F32(), r.F32()
		return StackPoint{y0, y1, int(r.U64())}
	})}
}

// QuestionnaireValidator is a marker struct returned by questionnaire APIs that
// expose opaque Rust validator objects.
type QuestionnaireValidator struct{}

// RdQuestionnaireValidator decodes an empty QuestionnaireValidator marker.
func RdQuestionnaireValidator(r *Reader) QuestionnaireValidator { return QuestionnaireValidator{} }

// QuestionnaireRules configures declarative answer validation rules for a
// questionnaire field.
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

// PutQuestionnaireRules encodes q onto o.
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

// BackgroundKind identifies which variant a decoded BackgroundValue holds.
type BackgroundKind uint8

// Variant tags for BackgroundValue.Kind.
const (
	BackgroundKindSolid BackgroundKind = iota
	BackgroundKindLinearGradient
	BackgroundKindPatternSlash
	BackgroundKindCheckerboard
)

// BackgroundColorSpace identifies the interpolation color space of a gradient background.
type BackgroundColorSpace uint8

// Color spaces for BackgroundValue.ColorSpace.
const (
	BackgroundColorSpaceSrgb BackgroundColorSpace = iota
	BackgroundColorSpaceOklab
)

// BackgroundStop holds one decoded color stop of a BackgroundValue gradient.
type BackgroundStop struct {
	Color      Hsla
	Percentage float32
}

// BackgroundValue is a decoded GPUI Background returned from style/theme queries.
type BackgroundValue struct {
	Kind          BackgroundKind
	ColorSpace    BackgroundColorSpace
	Solid         Hsla
	AngleOrHeight float32
	Stops         [2]BackgroundStop
}

// RdBackgroundValue decodes a BackgroundValue from r.
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

// IntervalMatcher matches calendar dates strictly before Before or after After.
type IntervalMatcher struct {
	Before Option[CivilDate]
	After  Option[CivilDate]
}

// RangeMatcher matches calendar dates inside an optional [From, To] interval.
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

// PutIntervalMatcher encodes m onto o.
func PutIntervalMatcher(o *Tape, m IntervalMatcher) {
	putOptCivilDate(o, m.Before)
	putOptCivilDate(o, m.After)
}

// RdIntervalMatcher decodes an IntervalMatcher from r.
func RdIntervalMatcher(r *Reader) IntervalMatcher {
	a := RdOpt(r, RdCivilDate)
	return IntervalMatcher{a, RdOpt(r, RdCivilDate)}
}

// PutRangeMatcher encodes m onto o.
func PutRangeMatcher(o *Tape, m RangeMatcher) {
	putOptCivilDate(o, m.From)
	putOptCivilDate(o, m.To)
}

// RdRangeMatcher decodes a RangeMatcher from r.
func RdRangeMatcher(r *Reader) RangeMatcher {
	a := RdOpt(r, RdCivilDate)
	return RangeMatcher{a, RdOpt(r, RdCivilDate)}
}
