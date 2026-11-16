package gk

// The data protocol (docs/PROTOCOLS.md section 12), the port of odin/gk/rt_data.odin: a chart, a
// plot shape or a scale that is generic over the type of one row of data gets the app's own rows
// as a table, and every closure upstream calls on a row while it draws is answered from that
// table instead:
//
//	type sale struct { Month string; Desktop float64 }
//	gk.LineChart(gk.NewData(sales)).X("Month").Y("Desktop").TooltipValue("{} visits")
//
// gk.NewData takes a slice of any struct (or of plain numbers, one column named "Value"). Each
// exported field is a column, named as the field is: numbers (any integer or float, bool as 0
// and 1), strings, a type with a String method (an enum, by name), gk.Color or gk.Hsla, and
// gk.Option or a pointer of a number (a gap). A `field` builder names a column (.X("Month")); a
// `format` builder takes a template over the numbers upstream hands the closure and the row's
// columns: "{}" the next number, "{1}" the second, "{:.2}" two decimals, "{:,}" with thousands
// separators, "{Month}" a column, "{{" and "}}" braces. Names are checked at the builder,
// against the struct the data came from.

import (
	"fmt"
	"reflect"
	"strings"
)

// Data is a slice of the app's rows, made by gk.NewData. The rows are copied into the tape when
// a builder takes it; nothing keeps them.
type Data struct {
	rows reflect.Value
	typ  reflect.Type
	set  bool
}

// NewData makes a data table of a slice of rows, for a builder that takes Data.
func NewData[T any](rows []T) Data {
	return Data{reflect.ValueOf(rows), reflect.TypeFor[T](), true}
}

type dataKind uint8

const (
	dataNum   dataKind = 0
	dataText  dataKind = 1
	dataColor dataKind = 2
	dataOpt   dataKind = 3
	dataNum32 dataKind = 4 // a float32 field: sent as f32
)

var dataKindName = [...]string{"a number", "text", "a colour", "a number or nothing (gk.Option or a pointer)", "a number"}

type dataCol struct {
	name  string
	kind  dataKind
	index []int
}

var (
	stringerType = reflect.TypeFor[fmt.Stringer]()
	colorType    = reflect.TypeFor[Color]()
	hslaType     = reflect.TypeFor[Hsla]()
)

// dataColKind is the kind of a column of Go type t (ok false: none).
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

var dataColsCache = map[reflect.Type][]dataCol{}

func dataCols(d Data) []dataCol {
	if !d.set {
		stop("a zero gk.Data was given to a builder: make it with gk.NewData(rows)")
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
				stop("gk.NewData: the field %s of %v is a %v, which a data table cannot carry (numbers, strings, a type with a String method, gk.Color, gk.Hsla, bool, gk.Option or a pointer of a number)", f.Name, t, f.Type)
			}
			cols = append(cols, dataCol{f.Name, kind, f.Index})
		}
		if len(cols) == 0 {
			stop("gk.NewData: %v has no exported field a data table can carry: a column is an exported field (Total, not total) of a number, a string or a type with a String method", t)
		}
	} else {
		kind, ok := dataColKind(d.typ)
		if !ok {
			stop("gk.NewData: a row of type %v cannot be data: give a slice of structs, or of numbers", d.typ)
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

// The tables given to the nodes of the build in progress, for the field and format checks.
type dataNode struct {
	serial, node uint32
	d            Data
}

var dataNodes []dataNode

// putData writes a data table: u16 columns, (u8 kind, str name) each, u32 rows, then the rows one
// after the other, each column's value in its kind (num f64, num32 f32, text str, colour as
// putColor, opt u8 presence and f64).
func putData(o *tape, d Data) {
	cols := dataCols(d)
	n := d.rows.Len()
	if n > int(^uint32(0)) {
		stop("gk.NewData: %d rows is more than a data table can carry (at most %d): give a chart the rows it shows, or totals of them", n, ^uint32(0))
	}
	if vb.active {
		if len(dataNodes) > 0 && dataNodes[0].serial != vb.serial {
			dataNodes = dataNodes[:0]
		}
		dataNodes = append(dataNodes, dataNode{vb.serial, vb.cur, Data{typ: d.typ, set: true}})
	}
	o.u16(uint16(len(cols)))
	for _, c := range cols {
		o.u8(uint8(c.kind))
		o.str(c.name)
	}
	o.u32(uint32(n))
	for i := range n {
		row := d.rows.Index(i)
		for _, c := range cols {
			v := row
			if c.index != nil {
				v = row.FieldByIndex(c.index)
			}
			switch c.kind {
			case dataNum:
				o.f64(dataNumOf(v))
			case dataNum32:
				o.f32(float32(dataNumOf(v)))
			case dataText:
				if v.Kind() != reflect.String && v.Type().Implements(stringerType) {
					o.str(v.Interface().(fmt.Stringer).String())
				} else {
					o.str(v.String())
				}
			case dataColor:
				if v.Type() == hslaType {
					putColor(o, v.Interface().(Hsla).Color())
				} else {
					putColor(o, v.Interface().(Color))
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
					o.u8(1)
					o.f64(dataNumOf(x))
				} else {
					o.u8(0)
					o.f64(0)
				}
			}
		}
	}
}

// dataOfCur is the table the node being written was given, if it was given one already.
func dataOfCur() (Data, bool) {
	if !vb.active {
		return Data{}, false
	}
	for i := len(dataNodes) - 1; i >= 0; i-- {
		if n := dataNodes[i]; n.serial == vb.serial && n.node == vb.cur {
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

// putField writes the name of the column a field builder reads, checked against the node's table
// when it has one: the column must exist and be of a kind the builder can use.
func putField(o *tape, name string, want dataKind) {
	if d, ok := dataOfCur(); ok {
		c, found := dataFind(d, name)
		if !found {
			stop("a builder names the column %q, which the rows of this data (%v) do not have: %s", name, d.typ, dataNames(d))
		}
		number := c.kind == dataNum || c.kind == dataNum32 || c.kind == dataOpt
		fits := c.kind == want || want == dataText && c.kind != dataColor || (want == dataNum || want == dataOpt) && number
		if !fits {
			stop("a builder reads the column %q of %v as %s, and it holds %s: name a column of the kind the builder takes (a band or label is text, a value is a number), or change the field's type", name, d.typ, dataKindName[want], dataKindName[c.kind])
		}
	}
	o.str(name)
}

// putFormat writes a template, checked: braces close, a number's index is one the closure has
// (nums), a column is one of the node's table and the closure has a row to read it from.
func putFormat(o *tape, template string, nums int, row bool) {
	d, have := dataOfCur()
	next := 0
	for i := 0; i < len(template); {
		ch := template[i]
		if ch == '}' {
			if i+1 < len(template) && template[i+1] == '}' {
				i += 2
				continue
			}
			stop("the template %q has a } that closes nothing (write }} for a brace)", template)
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
			stop("the template %q has a { that is not closed (write {{ for a brace)", template)
		}
		inner := template[i+1 : i+end]
		name, spec, _ := strings.Cut(inner, ":")
		if !dataSpecOK(spec) {
			stop("the template %q has the format %q: write nothing, \",\", \".N\" or \",.N\"", template, spec)
		}
		switch {
		case name == "":
			if next >= nums {
				stop("the template %q asks for number %d, and this builder hands it %d", template, next+1, nums)
			}
			next++
		case name[0] >= '0' && name[0] <= '9':
			n := 0
			for _, c := range []byte(name) {
				if c < '0' || c > '9' {
					stop("the template %q has {%s}: a number's index is digits only", template, inner)
				}
				n = n*10 + int(c-'0')
			}
			if n >= nums {
				stop("the template %q asks for number %d, and this builder hands it %d", template, n, nums)
			}
		default:
			if !row {
				stop("the template %q names the column %q, and this builder has no row to read it from", template, name)
			}
			if have {
				if _, found := dataFind(d, name); !found {
					stop("the template %q names the column %q, which the rows of this data (%v) do not have: %s", template, name, d.typ, dataNames(d))
				}
			}
		}
		i += end + 1
	}
	o.str(template)
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

// --- described plots (docs/PROTOCOLS.md 12.6, 12.14, 12.17), the port of odin/gk/rt_data.odin -------

// Plot is a plot described as a list of parts: an axis, a grid, the kit's shapes over the app's
// rows, and the binding's own paths and quads, painted in this order in the plot's bounds, which
// fill its parent. It stands where a Rust app implements the kit's Plot trait and paints each part
// (and calls gpui's paint_path and paint_quad).
//
//	gk.Plot(gk.NewGrid().X(xs).Y(ys).Stroke(line),
//		gk.NewPlotArea(points, gk.HSLA(0.6, 0.8, 0.7, 0.12)),
//		gk.NewPlotLine(points, 2.5, gk.NewLinearGradient(90, gk.NewLinearColorStop(cyan, 0), gk.NewLinearColorStop(violet, 1))))
func Plot(parts ...PlotPart) Element { return PlotWith(PlotOptions{}, parts...) }

// PlotOptions answer the kit's Plot trait for a described plot: ID keeps the plot's state across
// frames and opts in to appear motion and hover (Plot::id), NotInteractive keeps that state without
// hover tracking (Plot::interactive), Appear draws the plot in again whenever it changes
// (Plot::appear_generation; 0 is none).
type PlotOptions struct {
	ID             ID // nil: none
	NotInteractive bool
	Appear         uint64
}

// PlotWith is gk.Plot with options: gk.PlotWith(gk.PlotOptions{ID: "sales", Appear: 2}, parts...).
func PlotWith(opts PlotOptions, parts ...PlotPart) Element {
	n := newNode(nPlot)
	o := nodeArgs(n)
	o.u32(uint32(len(parts)))
	for _, p := range parts {
		if p == nil || p.valueNode() == nil {
			stop("a nil or zero plot part was given to gk.Plot: make each part with its constructor (gk.NewGrid(), gk.NewPlotLine(..))")
		}
		o.u32(takeValue(p.valueNode()))
	}
	if opts.ID != nil {
		o.u8(1)
		putID(o, opts.ID)
	} else {
		o.u8(0)
	}
	o.bool(!opts.NotInteractive)
	o.u64(opts.Appear)
	return valueEl{n}
}

// PlotPath is a path a plot paints (node PLOT_PATH): a polyline stroked at a width, or a polygon
// filled, in a colour or a gradient (gpui's PathBuilder and paint_path). Points are pixels from the
// plot's top left.
type PlotPath struct{ n *node }

func (p PlotPath) valueNode() *node { return p.n }

// NewPlotLine is a polyline through points, stroked width pixels wide with fill (a colour or a
// gk.LinearGradient). Fewer than two points paint nothing.
func NewPlotLine(points [][2]float32, width float32, fill Fill) PlotPath {
	return plotPath(0, width, points, fill)
}

// NewPlotArea is the polygon through points, closed back to the first, filled with fill.
func NewPlotArea(points [][2]float32, fill Fill) PlotPath {
	return plotPath(1, 0, points, fill)
}

func plotPath(kind uint8, width float32, points [][2]float32, fill Fill) PlotPath {
	n := newValue(nPlotPath)
	o := nodeArgs(n)
	o.u8(kind)
	o.f32(width)
	o.u32(uint32(len(points)))
	for _, p := range points {
		o.f32(p[0])
		o.f32(p[1])
	}
	putFill(o, fill)
	return PlotPath{n}
}

// PlotQuad is a rectangle a plot paints (node PLOT_QUAD; gpui's paint_quad): its bounds in pixels
// from the plot's top left, a fill (a colour or a gradient), round corners and a border.
type PlotQuad struct{ n *node }

func (q PlotQuad) valueNode() *node { return q.n }

// QuadStyle is a quad's corners and border; the zero value is square and without a border.
type QuadStyle struct {
	CornerRadius float32
	BorderWidth  float32
	BorderColor  Color
}

// NewPlotQuad is the rectangle x, y, width by height, filled with fill, with a style's corners
// and border: gk.NewPlotQuad(10, 10, 4, 4, theme.Primary, gk.QuadStyle{CornerRadius: 2}).
func NewPlotQuad(x, y, width, height float32, fill Fill, style QuadStyle) PlotQuad {
	n := newValue(nPlotQuad)
	o := nodeArgs(n)
	o.f32(x)
	o.f32(y)
	o.f32(width)
	o.f32(height)
	o.f32(style.CornerRadius)
	putFill(o, fill)
	o.f32(style.BorderWidth)
	putColor(o, style.BorderColor)
	return PlotQuad{n}
}
