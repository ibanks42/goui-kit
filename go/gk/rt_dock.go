package gk

// Dock panels as Go views (docs/PROTOCOLS.md section 11.7), ported from the first part
// of odin/gk/rt_dock.odin. A panel is an ordinary view the app made with gk.NewView, put in a dock by the
// generated DockArea and DockLayout methods:
//
//	files := gk.NewPanel(cx, gk.NewView(nil, a.filesView), gk.PanelData{Name: "files", Title: "Files"})
//	a.dock = gk.NewDockAreaSkinned(cx, "main")
//	a.dock.SetCenter(cx, gk.NewDockLayoutTabs().PanelView(files))
//	gk.DockView(a.dock)                         // in a view function: the dock as an element
//
// The dock asks a panel its name, title and flags while it draws: the library answers from the
// data the app gave (gk.SetPanel changes it). What the dock tells a panel (active, zoomed, added
// to a tab group, removed) reaches the app's handler (gk.PanelEvents) as a PanelEvent, after the
// dock's update. Saved layouts and the pane tree are not in Go yet (gen/out/go-report.md).

// PanelData is what the dock reads of a panel. The flags are upstream's defaults when false.
type PanelData struct {
	Name        string         // panel_name: the name a saved layout restores it by
	TabName     Option[string] // the tab's label, else the title
	Title       string
	NotClosable bool
	NotZoomable bool
	Hidden      bool
	NoPadding   bool // inner_padding false
	NoTitleBar  bool // title_bar false
	Suffix      View // drawn at the end of the title bar (Panel::title_suffix)
	Toolbar     View // a view whose root's gk.Button children are the toolbar (Panel::toolbar_buttons)
	Menu        Built[PopupMenu]
	Info        string // JSON: the panel's own data in a saved layout ("" for none)
}

// PanelEventKind is what the dock told a panel.
type PanelEventKind uint8

const (
	PanelActive  PanelEventKind = 0 // Flag: made active (true) or not
	PanelZoomed  PanelEventKind = 1 // Flag: zoomed in (true) or out
	PanelAdded   PanelEventKind = 2 // added to a tab group
	PanelRemoved PanelEventKind = 3 // removed from the dock
)

// PanelEvent is the event of a panel's handler.
type PanelEvent struct {
	Kind PanelEventKind
	Flag bool
}

// PanelEmit is what a panel tells the dock (upstream's PanelEvent).
type PanelEmit uint8

const (
	PanelZoomIn        PanelEmit = 0
	PanelZoomOut       PanelEmit = 1
	PanelLayoutChanged PanelEmit = 2
)

func init() {
	registerShape(shapePanelEvent, shapeInfo{"func(cx *Cx, e PanelEvent)", func(h any, cx *Cx, r *reader, ret *tape) bool {
		f, ok := h.(func(*Cx, PanelEvent))
		if ok {
			f(cx, PanelEvent{PanelEventKind(r.u8()), r.bool()})
		}
		return ok
	}})
}

func putPanelData(o *tape, d PanelData) {
	o.str(d.Name)
	putOptStrOpt(o, d.TabName)
	o.str(d.Title)
	var flags uint8
	for i, b := range []bool{d.NotClosable, d.NotZoomable, d.Hidden, d.NoPadding, d.NoTitleBar} {
		if b {
			flags |= 1 << i
		}
	}
	o.u8(flags)
	o.u64(d.Suffix.h)
	o.u64(d.Toolbar.h)
	o.u64(d.Menu.h)
	o.str(d.Info)
}

func putOptStrOpt(o *tape, s Option[string]) {
	if v, ok := s.Get(); ok {
		o.u8(1)
		o.str(v)
		return
	}
	o.u8(0)
}

// PanelOption changes a panel made by NewPanel.
type PanelOption func(*panelOpts)

type panelOpts struct{ events func(cx *Cx, e PanelEvent) }

// PanelEvents gives a panel the handler of what the dock tells it.
func PanelEvents(f func(cx *Cx, e PanelEvent)) PanelOption {
	return func(o *panelOpts) { o.events = f }
}

// NewPanel makes a dock panel that draws v. In a handler or init (it needs the window).
func NewPanel(cx *Cx, v View, d PanelData, opts ...PanelOption) DockPanel {
	t := checkCx(cx)
	if vb.active {
		stop("gk.NewPanel in a view function: make panels in init or in a handler")
	}
	if d.Name == "" {
		stop("a panel needs a name (PanelData.Name): a saved layout restores panels by it")
	}
	if v.h == 0 {
		stop("gk.NewPanel was given no view: make it with gk.NewView(init, render)")
	}
	var po panelOpts
	for _, o := range opts {
		o(&po)
	}
	t.flush()
	var slot, gen uint32
	if po.events != nil {
		slot, gen = keepHandler(cx, po.events, shapePanelEvent)
	}
	o := newReq(cPanelNew)
	o.u64(v.h)
	putPanelData(o, d)
	o.u32(slot)
	o.u32(gen)
	r := reader{b: mustCall(*o)}
	return DockPanel{r.u64()}
}

// SetPanel changes what the dock reads of a panel; the dock draws it again.
func SetPanel(cx *Cx, p DockPanel, d PanelData) {
	o := newReq(cPanelSet)
	o.u64(p.h)
	putPanelData(o, d)
	checkCx(cx).deferOp(*o)
}

// EmitPanel tells the dock what upstream's panels emit (zoom in, zoom out, layout changed).
func EmitPanel(cx *Cx, p DockPanel, e PanelEmit) {
	o := newReq(cPanelEmit)
	o.u64(p.h)
	o.u8(uint8(e))
	checkCx(cx).deferOp(*o)
}

// NewDockAreaSkinned makes the kit's styled dock area (DockSkin::dock_area); gk.NewDockArea is
// the bare one. In init or a handler.
func NewDockAreaSkinned(cx *Cx, id string) DockArea {
	t := checkCx(cx)
	if vb.active {
		stop("gk.NewDockAreaSkinned in a view function: make it in init or in a handler")
	}
	t.flush()
	o := newReq(cDockAreaSkinned)
	o.str(id)
	o.u8(0)
	o.u64(0)
	r := reader{b: mustCall(*o)}
	return DockArea{r.u64()}
}

// DockView draws a dock area where the view puts it.
func DockView(d DockArea) Element {
	n := newNode(nDockView)
	nodeArgs(n).u64(d.h)
	return valueEl{n}
}
