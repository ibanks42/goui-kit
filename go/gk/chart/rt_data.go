package chart

import (
	"fmt"
	"gpuikit.dev/gk"
	"gpuikit.dev/gk/internal/rt"
	"reflect"
	"strings"
)

// Data wraps a Go slice of structs (or scalar values) as a columnar data table
// for chart and plot builders. Exported struct fields become named columns
// referenced by field name in chart builders and format templates.
type Data struct {
	rows reflect.Value
	typ  reflect.Type
	set  bool
}

// NewData wraps rows as a chart Data table. Supported field types are signed
// and unsigned integers, float32, float64, bool, string, fmt.Stringer,
// gk.Color, gk.Hsla, and optional numbers (*T or gk.Option[T]).
func NewData[T any](rows []T) Data {
	return Data{reflect.ValueOf(rows), reflect.TypeFor[T](), true}
}

type dataKind uint8

const (
	dataNum   dataKind = 0
	dataText  dataKind = 1
	dataColor dataKind = 2
	dataOpt   dataKind = 3
	dataNum32 dataKind = 4
)

var dataKindName = [...]string{"a number", "text", "a colour", "a number or nothing (gk.Option or a pointer)", "a number"}

type dataCol struct {
	name  string
	kind  dataKind
	index []int
}

var (
	stringerType = reflect.TypeFor[fmt.Stringer]()
	colorType    = reflect.TypeFor[gk.Color]()
	hslaType     = reflect.TypeFor[gk.Hsla]()
)

// dataColKind classifies a Go reflect.Type into one of the wire column kinds
// understood by the Rust chart data decoder.
func dataColKind(t reflect.Type) (dataKind, bool) {
	if t == colorType || t == hslaType {
		return dataColor, true
	}
	if t.Kind() != reflect.String && t.Implements(stringerType) {
		return dataText, true
	}
	switch t.Kind() {
	case reflect.Float32:
		return dataNum32, true
	case reflect.Float64, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Bool:
		return dataNum, true
	case reflect.String:
		return dataText, true
	case reflect.Pointer:
		if k, ok := dataColKind(t.Elem()); ok && (k == dataNum || k == dataNum32) {
			return dataOpt, true
		}
	case reflect.Struct:
		if strings.HasPrefix(t.Name(), "Option[") && t.PkgPath() == colorType.PkgPath() {
			if k, ok := dataColKind(reflect.New(t).Elem().Field(0).Type()); ok && (k == dataNum || k == dataNum32) {
				return dataOpt, true
			}
		}
	}
	return 0, false
}

// Per-row-type reflection cache avoiding repeated struct field inspection each frame.
var dataColsCache = map[reflect.Type][]dataCol{}

// dataCols returns the cached column schema for d, inspecting exported struct
// fields or synthesizing a single "Value" column for scalar slices.
func dataCols(d Data) []dataCol {
	if !d.set {
		rt.Stop("a zero chart.Data was given to a builder: make it with chart.NewData(rows)")
	}
	if cols, ok := dataColsCache[d.typ]; ok {
		return cols
	}
	var cols []dataCol
	if t := d.typ; t.Kind() == reflect.Struct && t != colorType && t != hslaType && !strings.HasPrefix(t.Name(), "Option[") {
		for _, f := range reflect.VisibleFields(t) {
			if !f.IsExported() || f.Anonymous {
				continue
			}
			kind, ok := dataColKind(f.Type)
			if !ok {
				rt.Stop("chart.NewData: the field %s of %v is a %v, which a data table cannot carry (numbers, strings, a type with a String method, gk.Color, gk.Hsla, bool, gk.Option or a pointer of a number)", f.Name, t, f.Type)
			}
			cols = append(cols, dataCol{f.Name, kind, f.Index})
		}
		if len(cols) == 0 {
			rt.Stop("chart.NewData: %v has no exported field a data table can carry: a column is an exported field (Total, not total) of a number, a string or a type with a String method", t)
		}
	} else {
		kind, ok := dataColKind(d.typ)
		if !ok {
			rt.Stop("chart.NewData: a row of type %v cannot be data: give a slice of structs, or of numbers", d.typ)
		}
		cols = []dataCol{{"Value", kind, nil}}
	}
	dataColsCache[d.typ] = cols
	return cols
}

func dataNumOf(v reflect.Value) float64 {
	switch v.Kind() {
	case reflect.Float32, reflect.Float64:
		return v.Float()
	case reflect.Bool:
		if v.Bool() {
			return 1
		}
		return 0
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(v.Uint())
	}
	return 0
}

// dataNode remembers which Data schema was attached to a chart node during the
// current view build so chained column and template builders validate column
// names and types immediately at the Go call site.
type dataNode struct {
	serial, node uint32
	d            Data
}

var dataNodes []dataNode

// putData encodes d's column headers and row cells onto o and associates d's
// row type with the active tape node for subsequent putField/putFormat checks.
func putData(o *rt.Tape, d Data) {
	cols := dataCols(d)
	n := d.rows.Len()
	if n > int(^uint32(0)) {
		rt.Stop("chart.NewData: %d rows is more than a data table can carry (at most %d): give a chart the rows it shows, or totals of them", n, ^uint32(0))
	}
	if rt.Vb.Active {
		if len(dataNodes) > 0 && dataNodes[0].serial != rt.Vb.Serial {
			dataNodes = dataNodes[:0]
		}
		dataNodes = append(dataNodes, dataNode{rt.Vb.Serial, rt.Vb.Cur, Data{typ: d.typ, set: true}})
	}
	o.U16(uint16(len(cols)))
	for _, c := range cols {
		o.U8(uint8(c.kind))
		o.Str(c.name)
	}
	o.U32(uint32(n))
	for i := range n {
		row := d.rows.Index(i)
		for _, c := range cols {
			v := row
			if c.index != nil {
				v = row.FieldByIndex(c.index)
			}
			switch c.kind {
			case dataNum:
				o.F64(dataNumOf(v))
			case dataNum32:
				o.F32(float32(dataNumOf(v)))
			case dataText:
				if v.Kind() != reflect.String && v.Type().Implements(stringerType) {
					o.Str(v.Interface().(fmt.Stringer).String())
				} else {
					o.Str(v.String())
				}
			case dataColor:
				if v.Type() == hslaType {
					rt.PutColor(o, v.Interface().(gk.Hsla).Color())
				} else {
					rt.PutColor(o, v.Interface().(gk.Color))
				}
			case dataOpt:
				var x reflect.Value
				if v.Kind() == reflect.Pointer {
					if !v.IsNil() {
						x = v.Elem()
					}
				} else if v.Field(1).Bool() {
					x = v.Field(0)
				}
				if x.IsValid() {
					o.U8(1)
					o.F64(dataNumOf(x))
				} else {
					o.U8(0)
					o.F64(0)
				}
			}
		}
	}
}

func dataOfCur() (Data, bool) {
	if !rt.Vb.Active {
		return Data{}, false
	}
	for i := len(dataNodes) - 1; i >= 0; i-- {
		if n := dataNodes[i]; n.serial == rt.Vb.Serial && n.node == rt.Vb.Cur {
			return n.d, true
		}
	}
	return Data{}, false
}

func dataFind(d Data, name string) (dataCol, bool) {
	for _, c := range dataCols(d) {
		if c.name == name {
			return c, true
		}
	}
	return dataCol{}, false
}

func dataNames(d Data) string {
	var names []string
	for _, c := range dataCols(d) {
		names = append(names, c.name)
	}
	return strings.Join(names, ", ")
}

// putField checks that column name exists on the active node's Data schema and
// matches the expected kind before encoding name onto o.
func putField(o *rt.Tape, name string, want dataKind) {
	if d, ok := dataOfCur(); ok {
		c, found := dataFind(d, name)
		if !found {
			rt.Stop("a builder names the column %q, which the rows of this data (%v) do not have: %s", name, d.typ, dataNames(d))
		}
		number := c.kind == dataNum || c.kind == dataNum32 || c.kind == dataOpt
		fits := c.kind == want || want == dataText && c.kind != dataColor || (want == dataNum || want == dataOpt) && number
		if !fits {
			rt.Stop("a builder reads the column %q of %v as %s, and it holds %s: name a column of the kind the builder takes (a band or label is text, a value is a number), or change the field's type", name, d.typ, dataKindName[want], dataKindName[c.kind])
		}
	}
	o.Str(name)
}

// putFormat validates placeholder braces, numeric indices (0..nums-1), format
// specifiers ("", ",", ".N", ",.N"), and optional row column names before
// encoding template onto o.
func putFormat(o *rt.Tape, template string, nums int, row bool) {
	d, have := dataOfCur()
	next := 0
	for i := 0; i < len(template); {
		ch := template[i]
		if ch == '}' {
			if i+1 < len(template) && template[i+1] == '}' {
				i += 2
				continue
			}
			rt.Stop("the template %q has a } that closes nothing (write }} for a brace)", template)
		}
		if ch != '{' {
			i++
			continue
		}
		if i+1 < len(template) && template[i+1] == '{' {
			i += 2
			continue
		}
		end := strings.IndexByte(template[i:], '}')
		if end < 0 {
			rt.Stop("the template %q has a { that is not closed (write {{ for a brace)", template)
		}
		inner := template[i+1 : i+end]
		name, spec, _ := strings.Cut(inner, ":")
		if !dataSpecOK(spec) {
			rt.Stop("the template %q has the format %q: write nothing, \",\", \".N\" or \",.N\"", template, spec)
		}
		switch {
		case name == "":
			if next >= nums {
				rt.Stop("the template %q asks for number %d, and this builder hands it %d", template, next+1, nums)
			}
			next++
		case name[0] >= '0' && name[0] <= '9':
			n := 0
			for _, c := range []byte(name) {
				if c < '0' || c > '9' {
					rt.Stop("the template %q has {%s}: a number's index is digits only", template, inner)
				}
				n = n*10 + int(c-'0')
			}
			if n >= nums {
				rt.Stop("the template %q asks for number %d, and this builder hands it %d", template, n, nums)
			}
		default:
			if !row {
				rt.Stop("the template %q names the column %q, and this builder has no row to read it from", template, name)
			}
			if have {
				if _, found := dataFind(d, name); !found {
					rt.Stop("the template %q names the column %q, which the rows of this data (%v) do not have: %s", template, name, d.typ, dataNames(d))
				}
			}
		}
		i += end + 1
	}
	o.Str(template)
}

func dataSpecOK(spec string) bool {
	s := strings.TrimPrefix(spec, ",")
	if s == "" {
		return true
	}
	if s[0] != '.' || len(s) == 1 || len(s) > 3 {
		return false
	}
	for _, c := range []byte(s[1:]) {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// Plot builds an interactive canvas element composed of one or more plot parts.
func Plot(parts ...gk.PlotPart) gk.Element { return PlotWith(PlotOptions{}, parts...) }

// PlotOptions configures identity, interactivity, and appear-animation timing
// for PlotWith.
type PlotOptions struct {
	ID             gk.ID
	NotInteractive bool
	Appear         uint64
}

// PlotWith builds a plot element from parts using the settings in opts.
func PlotWith(opts PlotOptions, parts ...gk.PlotPart) gk.Element {
	n := rt.NewNode(rt.NPlot)
	o := rt.NodeArgs(n)
	o.U32(uint32(len(parts)))
	for _, p := range parts {
		if p == nil || rt.ValueOf(p) == nil {
			rt.Stop("a nil or zero plot part was given to chart.Plot: make each part with its constructor (chart.NewGrid(), chart.NewPlotLine(..))")
		}
		o.U32(rt.TakeValue(rt.ValueOf(p)))
	}
	if opts.ID != nil {
		o.U8(1)
		rt.PutID(o, opts.ID)
	} else {
		o.U8(0)
	}
	o.Bool(!opts.NotInteractive)
	o.U64(opts.Appear)
	return rt.ValueEl{ElCore: rt.MkElCore(n)}
}

// PlotPath is a polyline or filled polygon plot part accepted by Plot.
type PlotPath struct{ rt.ValPlotPart }

// NewPlotLine creates a stroked polyline PlotPath through points with stroke
// width and fill.
func NewPlotLine(points [][2]float32, width float32, fill gk.Fill) PlotPath {
	return plotPath(0, width, points, fill)
}

// NewPlotArea creates a filled polygon PlotPath through points with fill.
func NewPlotArea(points [][2]float32, fill gk.Fill) PlotPath {
	return plotPath(1, 0, points, fill)
}

func plotPath(kind uint8, width float32, points [][2]float32, fill gk.Fill) PlotPath {
	n := rt.NewValue(rt.NPlotPath)
	o := rt.NodeArgs(n)
	o.U8(kind)
	o.F32(width)
	o.U32(uint32(len(points)))
	for _, p := range points {
		o.F32(p[0])
		o.F32(p[1])
	}
	rt.PutFill(o, fill)
	return PlotPath{ValPlotPart: rt.MkValPlotPart(n)}
}

// PlotQuad is a styled rectangle plot part accepted by Plot.
type PlotQuad struct{ rt.ValPlotPart }

// QuadStyle configures optional rounded corners and border styling for NewPlotQuad.
type QuadStyle struct {
	CornerRadius float32
	BorderWidth  float32
	BorderColor  gk.Color
}

// NewPlotQuad creates a rectangle plot part at (x, y) with the given size,
// fill, and border/corner style.
func NewPlotQuad(x, y, width, height float32, fill gk.Fill, style QuadStyle) PlotQuad {
	n := rt.NewValue(rt.NPlotQuad)
	o := rt.NodeArgs(n)
	o.F32(x)
	o.F32(y)
	o.F32(width)
	o.F32(height)
	o.F32(style.CornerRadius)
	rt.PutFill(o, fill)
	o.F32(style.BorderWidth)
	rt.PutColor(o, style.BorderColor)
	return PlotQuad{ValPlotPart: rt.MkValPlotPart(n)}
}
