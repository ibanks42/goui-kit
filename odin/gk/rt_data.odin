package gk

// The data protocol (docs/PROTOCOLS.md section 12): a chart, a plot shape or a scale that is
// generic over the type of one row of data gets the app's own rows as a table, and every
// closure upstream calls on a row while it draws is answered from that table instead:
//
//   gk.line_chart(gk.data(sales[:]))->x("month")->y("desktop")->tooltip_value("{} visits")
//
// gk.data takes a slice of any struct (or of plain numbers, one column named "value"). Each
// field is a column: numbers (any integer or float, bool as 0 and 1), strings, enums (by
// name), gk.Color or gk.Hsla, and Maybe of a number (a gap). A `field` builder names a column
// (`->x("month")`); a `format` builder takes a template over the numbers upstream hands the
// closure and the row's columns: "{}" the next number, "{1}" the second, "{:.2}" two
// decimals, "{:,}" with thousands separators, "{month}" a column, "{{" and "}}" braces.
// Names are checked here, at the builder, against the struct the data came from.

import "base:runtime"
import "core:fmt"
import "core:strings"

// Data is a slice of the app's rows, made by gk.data. The rows are copied into the tape when
// a builder takes it, and nothing keeps a pointer to them: a local array of the view procedure
// is fine (gk.bar_chart(gk.data(totals[:]))).
Data :: struct {
	ti:   ^runtime.Type_Info,
	rows: rawptr,
	n:    int,
	loc:  runtime.Source_Code_Location,
}

// data makes a data table of a slice of rows, for a builder that takes Data.
data :: proc(rows: []$T, loc := #caller_location) -> Data {
	return {type_info_of(T), raw_data(rows), len(rows), loc}
}

// Data_Kind is a column's kind on the wire, and what a field builder wants from one.
Data_Kind :: enum u8 {
	Num   = 0,
	Text  = 1,
	Color = 2,
	Opt   = 3,
	Num32 = 4, // an f32 (or f16) field: sent as f32, shown as text the way an f32 prints
}

@(private)
Data_Col :: struct {
	name:   string,
	kind:   Data_Kind,
	offset: uintptr,
	ti:     ^runtime.Type_Info,
	enum_:  bool,
}

@(private)
Data_Node :: struct {
	serial, node: u32,
	d:            Data,
}

// The tables given to the nodes of the build in progress, for the field and format checks.
@(private)
data_nodes: [dynamic]Data_Node

@(private)
data_kind_name := [Data_Kind]string {
	.Num   = "a number",
	.Text  = "text",
	.Color = "a colour",
	.Opt   = "a number or nothing (Maybe)",
	.Num32 = "a number",
}

@(private)
data_cols :: proc(d: Data) -> []Data_Col {
	cols := make([dynamic]Data_Col, 0, 8, context.temp_allocator)
	base := runtime.type_info_base(d.ti)
	if s, ok := base.variant.(runtime.Type_Info_Struct); ok && d.ti.id != Color && d.ti.id != Hsla {
		for i in 0 ..< int(s.field_count) {
			kind, is_enum := data_col_kind(s.types[i])
			if kind == nil {
				stop(d.loc, "gk.data: the field %s of %v is a %v, which a data table cannot carry (numbers, strings, enums, gk.Color, gk.Hsla, bool, Maybe of a number)", s.names[i], d.ti, s.types[i])
			}
			append(&cols, Data_Col{s.names[i], kind.?, s.offsets[i], s.types[i], is_enum})
		}
		return cols[:]
	}
	kind, is_enum := data_col_kind(d.ti)
	if kind == nil {
		stop(d.loc, "gk.data: a row of type %v cannot be data: give a slice of structs, or of numbers", d.ti)
	}
	append(&cols, Data_Col{"value", kind.?, 0, d.ti, is_enum})
	return cols[:]
}

@(private)
data_col_kind :: proc(ti: ^runtime.Type_Info) -> (Maybe(Data_Kind), bool) {
	if ti.id == Color || ti.id == Hsla {
		return .Color, false
	}
	base := runtime.type_info_base(ti)
	#partial switch v in base.variant {
	case runtime.Type_Info_Float:
		if base.size <= 4 {
			return Data_Kind.Num32, false
		}
		return Data_Kind.Num, false
	case runtime.Type_Info_Integer, runtime.Type_Info_Boolean:
		return .Num, false
	case runtime.Type_Info_String:
		return .Text, false
	case runtime.Type_Info_Enum:
		return .Text, true
	case runtime.Type_Info_Union:
		if len(v.variants) == 1 {
			#partial switch _ in runtime.type_info_base(v.variants[0]).variant {
			case runtime.Type_Info_Integer, runtime.Type_Info_Float:
				return .Opt, false
			}
		}
	}
	return nil, false
}

// data_num reads a number of any integer, float or bool type as an f64.
@(private)
data_num :: proc(p: rawptr, ti: ^runtime.Type_Info) -> f64 {
	base := runtime.type_info_base(ti)
	#partial switch v in base.variant {
	case runtime.Type_Info_Float:
		switch base.size {
		case 2:
			return f64((cast(^f16)p)^)
		case 4:
			return f64((cast(^f32)p)^)
		case:
			return (cast(^f64)p)^
		}
	case runtime.Type_Info_Boolean:
		return (cast(^u8)p)^ != 0 ? 1 : 0
	case runtime.Type_Info_Integer:
		if v.signed {
			switch base.size {
			case 1:
				return f64((cast(^i8)p)^)
			case 2:
				return f64((cast(^i16)p)^)
			case 4:
				return f64((cast(^i32)p)^)
			case 8:
				return f64((cast(^i64)p)^)
			case:
				return f64((cast(^i128)p)^)
			}
		}
		switch base.size {
		case 1:
			return f64((cast(^u8)p)^)
		case 2:
			return f64((cast(^u16)p)^)
		case 4:
			return f64((cast(^u32)p)^)
		case 8:
			return f64((cast(^u64)p)^)
		case:
			return f64((cast(^u128)p)^)
		}
	}
	return 0
}

// put_data writes a data table: u16 columns, (u8 kind, str name) each, u32 rows, then the rows
// one after the other, each column's value in its kind (num f64, text str, colour as put_color,
// opt u8 presence and f64).
@(private)
put_data :: proc(o: ^Tape, d: Data) {
	if d.n < 0 || d.n > int(max(u32)) {
		stop(d.loc, "gk.data: %d rows is more than a data table can carry", d.n)
	}
	cols := data_cols(d)
	if build.active { // rows given to a call op (Pie::arcs, Stack::data) belong to no node of a view
		if len(data_nodes) > 0 && data_nodes[0].serial != build.serial {
			clear(&data_nodes)
		}
		// the type and the line only: the rows may be the app's local array, gone when its view
		// procedure returns (keeping the pointer made Odin warn at every gk.data(local[:]))
		append(&data_nodes, Data_Node{build.serial, build.cur, {ti = d.ti, n = d.n, loc = d.loc}})
	}
	put_u16(o, u16(len(cols)))
	for c in cols {
		put_u8(o, u8(c.kind))
		put_str(o, c.name)
	}
	put_u32(o, u32(d.n))
	size := d.ti.size
	for i in 0 ..< d.n {
		row := rawptr(uintptr(d.rows) + uintptr(i * size))
		for c in cols {
			p := rawptr(uintptr(row) + c.offset)
			switch c.kind {
			case .Num:
				put_f64(o, data_num(p, c.ti))
			case .Num32:
				put_f32(o, f32(data_num(p, c.ti)))
			case .Text:
				if c.enum_ {
					put_str(o, fmt.tprint(any{p, c.ti.id}))
				} else {
					put_str(o, (cast(^string)p)^)
				}
			case .Color:
				if c.ti.id == Hsla {
					h := (cast(^Hsla)p)^
					put_color(o, Color{0, h.h, h.s, h.l, h.a})
				} else {
					put_color(o, (cast(^Color)p)^)
				}
			case .Opt:
				u := runtime.type_info_base(c.ti).variant.(runtime.Type_Info_Union)
				tag: i64
				if u.tag_type != nil {
					runtime.mem_copy_non_overlapping(&tag, rawptr(uintptr(p) + u.tag_offset), u.tag_type.size)
				}
				if tag == 0 {
					put_u8(o, 0)
					put_f64(o, 0)
				} else {
					put_u8(o, 1)
					put_f64(o, data_num(p, u.variants[0]))
				}
			}
		}
	}
}

// The table the node being written was given, if it was given one already.
@(private)
data_of_cur :: proc() -> (Data, bool) {
	if !build.active {
		return {}, false
	}
	#reverse for n in data_nodes {
		if n.serial == build.serial && n.node == build.cur {
			return n.d, true
		}
	}
	return {}, false
}

@(private)
data_find :: proc(d: Data, name: string) -> (Data_Col, bool) {
	for c in data_cols(d) {
		if c.name == name {
			return c, true
		}
	}
	return {}, false
}

@(private)
data_names :: proc(d: Data) -> string {
	b := strings.builder_make(context.temp_allocator)
	for c, i in data_cols(d) {
		if i > 0 {
			strings.write_string(&b, ", ")
		}
		strings.write_string(&b, c.name)
	}
	return strings.to_string(b)
}

// put_field writes the name of the column a field builder reads, checked against the node's
// table when it has one: the column must exist and be of a kind the builder can use.
@(private)
put_field :: proc(o: ^Tape, name: string, want: Data_Kind) {
	if d, ok := data_of_cur(); ok {
		c, found := data_find(d, name)
		if !found {
			stop(d.loc, "a builder names the column \"%s\", which the rows of this data (%v) do not have: %s", name, d.ti, data_names(d))
		}
		number := c.kind == .Num || c.kind == .Num32 || c.kind == .Opt
		fits := c.kind == want || want == .Text && c.kind != .Color || (want == .Num || want == .Opt) && number
		if !fits {
			stop(d.loc, "a builder reads the column \"%s\" of %v as %s, and it holds %s", name, d.ti, data_kind_name[want], data_kind_name[c.kind])
		}
	}
	put_str(o, name)
}

// put_format writes a template, checked: braces close, a number's index is one the closure
// has (`nums`), a column is one of the node's table and the closure has a row to read it from.
@(private)
put_format :: proc(o: ^Tape, template: string, nums: int, row: bool) {
	d, have := data_of_cur()
	loc := d.loc if have else runtime.Source_Code_Location{}
	next := 0
	i := 0
	for i < len(template) {
		ch := template[i]
		if ch == '}' {
			if i + 1 < len(template) && template[i + 1] == '}' {
				i += 2
				continue
			}
			stop(loc, "the template \"%s\" has a %s that closes nothing (write %s for a brace)", template, "}", "}}")
		}
		if ch != '{' {
			i += 1
			continue
		}
		if i + 1 < len(template) && template[i + 1] == '{' {
			i += 2
			continue
		}
		end := strings.index_byte(template[i:], '}')
		if end < 0 {
			stop(loc, "the template \"%s\" has a %s that is not closed (write %s for a brace)", template, "{", "{{")
		}
		inner := template[i + 1:i + end]
		name := inner
		spec := ""
		if c := strings.index_byte(inner, ':'); c >= 0 {
			name, spec = inner[:c], inner[c + 1:]
		}
		if !data_spec_ok(spec) {
			stop(loc, "the template \"%s\" has the format \"%s\": write nothing, \",\", \".N\" or \",.N\"", template, spec)
		}
		switch {
		case name == "":
			if next >= nums {
				stop(loc, "the template \"%s\" asks for number %d, and this builder hands it %d", template, next + 1, nums)
			}
			next += 1
		case name[0] >= '0' && name[0] <= '9':
			n := 0
			for c in transmute([]u8)name {
				if c < '0' || c > '9' {
					stop(loc, "the template \"%s\" has %s%s%s: a number's index is digits only", template, "{", inner, "}")
				}
				n = n * 10 + int(c - '0')
			}
			if n >= nums {
				stop(loc, "the template \"%s\" asks for number %d, and this builder hands it %d", template, n, nums)
			}
		case:
			if !row {
				stop(loc, "the template \"%s\" names the column \"%s\", and this builder has no row to read it from", template, name)
			}
			if have {
				if _, found := data_find(d, name); !found {
					stop(loc, "the template \"%s\" names the column \"%s\", which the rows of this data (%v) do not have: %s", template, name, d.ti, data_names(d))
				}
			}
		}
		i += end + 1
	}
	put_str(o, template)
}

@(private)
data_spec_ok :: proc(spec: string) -> bool {
	s := spec
	if len(s) > 0 && s[0] == ',' {
		s = s[1:]
	}
	if len(s) == 0 {
		return true
	}
	if s[0] != '.' || len(s) == 1 || len(s) > 3 {
		return false
	}
	for c in transmute([]u8)s[1:] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// plot is a plot described as a list of parts (docs/PROTOCOLS.md section 12): an axis, a grid
// and shapes over the app's rows, painted in this order in the plot's bounds, which fill its
// parent. It stands where a Rust app implements the kit's Plot trait and paints each part.
//
//   gk.plot(gk.plot_axis()->x(200)->x_label(..), gk.bar_shape()->data(gk.data(rows[:]))->..)
plot :: proc(parts: ..Plot_Part, loc := #caller_location) -> Element {
	return plot_with({}, ..parts, loc = loc)
}

// Plot_Options answer the kit's Plot trait for a described plot (PROTOCOLS.md 12.14): `id` keeps
// the plot's state across frames and opts in to appear motion and hover (Plot::id), `interactive`
// false keeps that state without hover tracking (Plot::interactive), `appear` draws the plot in
// again whenever it changes (Plot::appear_generation; 0 is none).
Plot_Options :: struct {
	id:              Maybe(Id),
	not_interactive: bool,
	appear:          u64,
}

// plot_with is gk.plot with options: gk.plot_with({id = "sales", appear = 2}, parts..).
plot_with :: proc(opts: Plot_Options, parts: ..Plot_Part, loc := #caller_location) -> Element {
	e := new_node(N_PLOT, loc)
	o := node_args()
	put_u32(o, u32(len(parts)))
	for p in parts {
		put_plot_part(o, p)
	}
	if id, ok := opts.id.?; ok {
		put_u8(o, 1)
		put_id(o, id)
	} else {
		put_u8(o, 0)
	}
	put_u8(o, u8(!opts.not_interactive))
	put_u64(o, opts.appear)
	return e
}

// --- the binding's own plot parts (docs/PROTOCOLS.md 12.17): paths and quads ----------------------
//
// What a Rust plot's paint draws with gpui's PathBuilder, paint_path and paint_quad, in a colour or a
// gradient (a gk.Fill). Positions are pixels from the plot's top left.
//
//   gk.plot(gk.grid()->x(xs)->stroke(line),
//           gk.plot_area(points, gk.hsla(0.6, 0.8, 0.7, 0.12)),
//           gk.plot_line(points, 2.5, gk.linear_gradient(90, gk.linear_color_stop(cyan, 0), gk.linear_color_stop(violet, 1))),
//           gk.plot_quad({x, y, 10, 10}, gk.hsla(0, 0, 1, 1), {corner_radius = 5}))

// Plot_Path is a polyline stroked at a width or a polygon filled (node PLOT_PATH).
Plot_Path :: struct {
	using nd: Value,
}

// plot_line is a polyline through points, stroked width pixels wide with fill. Fewer than two points
// paint nothing.
plot_line :: proc(points: [][2]f32, width: f32, fill: Fill, loc := #caller_location) -> Plot_Path {
	return plot_path(0, width, points, fill, loc)
}

// plot_area is the polygon through points, closed back to the first, filled with fill.
plot_area :: proc(points: [][2]f32, fill: Fill, loc := #caller_location) -> Plot_Path {
	return plot_path(1, 0, points, fill, loc)
}

@(private)
plot_path :: proc(kind: u8, width: f32, points: [][2]f32, fill: Fill, loc: runtime.Source_Code_Location) -> Plot_Path {
	p := Plot_Path{new_value(N_PLOT_PATH, loc)}
	o := node_args()
	put_u8(o, kind)
	put_f32(o, width)
	put_u32(o, u32(len(points)))
	for pt in points {
		put_f32(o, pt[0])
		put_f32(o, pt[1])
	}
	put_fill(o, fill)
	return p
}

// Plot_Quad is a rectangle a plot paints (node PLOT_QUAD).
Plot_Quad :: struct {
	using nd: Value,
}

// Quad_Style is a quad's corners and border; the zero value is square and without a border.
Quad_Style :: struct {
	corner_radius: f32,
	border_width:  f32,
	border_color:  Color,
}

// plot_quad is the rectangle {x, y, width, height} filled with fill, with a style's corners and border.
plot_quad :: proc(r: [4]f32, fill: Fill, style := Quad_Style{}, loc := #caller_location) -> Plot_Quad {
	q := Plot_Quad{new_value(N_PLOT_QUAD, loc)}
	o := node_args()
	put_f32(o, r[0])
	put_f32(o, r[1])
	put_f32(o, r[2])
	put_f32(o, r[3])
	put_f32(o, style.corner_radius)
	put_fill(o, fill)
	put_f32(o, style.border_width)
	put_color(o, style.border_color)
	return q
}

// --- docs/PROTOCOLS.md 12.15: the pie and stack layouts over the app's rows --------------
//
//   pie := gk.pie(cx)->value("visits")->pad_angle(0.02)->build()     // Pie<Datum>: a kept object
//   arcs := pie->arcs(cx, gk.data(rows[:]))                          // []gk.Arc_Data, one per row with a value
//   st := gk.stack(cx)->data(gk.data(rows[:]))->keys({"apples", "pears"})->value("")->build()
//   for s in st->series(cx) { .. s.points[i].y0, y1, row .. }        // value(""): the column the key names
//
// A row is its index in the rows the app passed.

// Arc_Data is one slice of a pie (upstream's ArcData): the row it stands for and its angles.
Arc_Data :: struct {
	row:         int,
	index:       int, // its place in the pie's order
	value:       f32,
	start_angle: f32,
	end_angle:   f32,
	pad_angle:   f32,
}

// Stack_Point is one row of one series of a stack: its band from y0 to y1.
Stack_Point :: struct {
	y0, y1: f32,
	row:    int,
}

// Stack_Series is one key's series of a stack (upstream's StackSeries).
Stack_Series :: struct {
	key:    string,
	index:  int,
	points: []Stack_Point,
}
