package gk

// The hand-written values and platform families (docs/PROTOCOLS.md 11.17, 11.30, 13.20, 13.31): drag and drop, an
// element's tooltip, image sources as bytes and pixels, the menu bar. The library side is
// core/src/values.rs and core/src/platform.rs; the generated builders (OnDrag, OnDrop, DragOver,
// CanDrop, Tooltip, gk.Img's sources) call the put functions here.
//
//	row := gk.Div().ID(gk.Key("row", i)).
//		OnDrag(gk.DragValue{ID: uint64(i), Text: name}, a.preview). // a.preview: a gk.View made in init
//		CanDrop(gk.DropAccept{IDs: []uint64{7, 8}}).                // only these values
//		DragOver(gk.NewStyleRefinementDefault().Bg(theme.Accent)).   // while one is over it
//		OnDrop(func(cx *gk.Cx, d gk.DragValue) { a.move(int(d.ID), i) })
//	gk.Div().ID("info").Tooltip(gk.NewKitTooltip("Saved to disk"))     // the kit's tooltip
//	gk.Div().ID("info").Tooltip(gk.NewKitTooltipElement(gk.HFlex().Child(..)))  // with an element of the app's
//	gk.Img(gk.ImageSourceImage{Value: gk.ImageBytes{Format: gk.ImageFormatPng, Bytes: png}})
//	gk.SetMenus(cx, gk.Menu{Name: "My App", Items: []gk.MenuItem{gk.MenuAction{Name: "Quit", Action: quit}}})

// --- drag and drop (PROTOCOLS.md 13.20) -----------------------------------------------------------

// DragValue is what an app drags: an element's OnDrag takes it with the view drawn under the
// pointer while it is dragged, a drop's handler gets it back, and DragOver's style applies while
// one is over the element. One type for every drag of the app: tell kinds apart by ID (say, the kind
// in the top 32 bits and an index below).
type DragValue struct {
	ID   uint64
	Text string
}

func putDragValue(o *tape, v DragValue) {
	o.u64(v.ID)
	o.str(v.Text)
}

func rdDragValue(r *reader) DragValue {
	id := r.u64()
	return DragValue{ID: id, Text: r.str()}
}

// DropAccept is what an element's CanDrop takes, as data (the closure gpui calls while a drag
// moves cannot run app code): the app's drag values (every one, or only those whose ID is in IDs)
// and files dragged in from outside the app. The zero value takes nothing.
type DropAccept struct {
	Values bool
	IDs    []uint64
	Paths  bool
}

func putDropAccept(o *tape, a DropAccept) {
	o.bool(a.Values || len(a.IDs) > 0)
	o.u32(uint32(len(a.IDs)))
	for _, id := range a.IDs {
		o.u64(id)
	}
	o.bool(a.Paths)
}

// --- an element's tooltip (PROTOCOLS.md 11.17) ----------------------------------------------------

// ViewOrTooltip is what an element's Tooltip and HoverableTooltip take: a view of the app's (any
// gk.View, drawn as the tooltip), or the kit's tooltip, gk.NewKitTooltip(text) or
// gk.NewKitTooltipElement(element) with its builders (KeyBinding, Action, the styles). The kit's
// value is described again each time the tooltip shows, from the view that gave it.
type ViewOrTooltip interface{ isViewOrTooltip() }

func (View) isViewOrTooltip() {}

func putViewOrTooltip(o *tape, v ViewOrTooltip) {
	switch x := v.(type) {
	case View:
		o.u8(0)
		putViewArg(o, x)
	case KitTooltip:
		o.u8(1)
		o.u32(takeValue(x.n))
	case nil:
		o.u8(0)
		o.u64(0)
	default:
		// Unreachable from outside the package (isViewOrTooltip is unexported); no %T, so v does not escape.
		stop("an unknown tooltip was given: give a gk.View, or gk.NewKitTooltip(text) or gk.NewKitTooltipElement(el)")
	}
}

// --- image sources as bytes and pixels (PROTOCOLS.md 13.31) ----------------------------------------

// ImageBytes is an encoded image (a PNG, a JPEG, ..., gpui's Image) for gk.ImageSourceImage:
//
//	gk.Img(gk.ImageSourceImage{Value: gk.ImageBytes{Format: gk.ImageFormatPng, Bytes: data}})
type ImageBytes struct {
	Format ImageFormat
	Bytes  []byte
}

// PixelImage is an image as pixels, for gk.NewRenderImage: Width x Height pixels of 4 bytes each,
// red, green, blue, alpha, row by row.
type PixelImage struct {
	Width, Height uint32
	RGBA          []byte
}

// RenderImage is an image the library made from pixels (gpui's RenderImage), shown by
// gk.ImageSourceRender and gk.ImageSourceCustom. Make it once, with gk.NewRenderImage, and keep it
// while a view shows it: gpui keeps each image's texture on the graphics card until the image is
// released, so pixels made into a new image on every frame would be a new texture on every frame.
// gk.Release frees it in every window; a view that names it after that stops with a message. New
// pixels are a new image: make it, show it, release the old one.
type RenderImage struct{ h uint64 }

func (i RenderImage) handle() uint64 { return i.h }

func putBytes(o *tape, b []byte) { o.bytes(b) }

func putRenderImage(o *tape, i RenderImage) {
	if i.h == 0 {
		stop("a zero gk.RenderImage: make one with gk.NewRenderImage(cx, gk.PixelImage{..}) and keep it")
	}
	o.u64(i.h)
}

// NewRenderImage makes an image of p's pixels, converted once to the format gpui draws. Make it
// in init or in a handler, not in a view function; gk.Release frees it.
func NewRenderImage(cx *Cx, p PixelImage) RenderImage {
	t := checkCx(cx)
	if vb.active {
		stop("a view function may not make an image: make it once, in init or in a handler, and keep it")
	}
	if int(p.Width)*int(p.Height)*4 != len(p.RGBA) {
		stop("a gk.PixelImage of %dx%d pixels holds %d bytes: it needs width x height x 4 (red, green, blue, alpha per pixel)", p.Width, p.Height, len(p.RGBA))
	}
	t.flush()
	o := newReq(cRenderImageNew)
	o.u32(p.Width)
	o.u32(p.Height)
	o.bytes(p.RGBA)
	r := reader{b: mustCall(*o)}
	return RenderImage{r.u64()}
}

// TestRenderImage gives the library's image behind i (custom: through ImageSourceCustom's loader):
// its size, its first pixel as gpui keeps it (BGRA, a little-endian uint32) and gpui's id of it,
// the same for as long as i is kept. For tests.
func TestRenderImage(cx *Cx, i RenderImage, custom bool) (w, h, first uint32, id uint64) {
	checkCx(cx).flush()
	o := newReq(cPlatformTest)
	o.u8(4)
	o.bool(custom)
	putRenderImage(o, i)
	r := reader{b: mustCall(*o)}
	return r.u32(), r.u32(), r.u32(), r.u64()
}

// TestImagesShown tells how many times views have handed gpui an image (gk.ImageSourceRender or
// gk.ImageSourceCustom), how many distinct images (gpui's ids, one texture each) those were, and
// how many times a release dropped an image's texture from a window. For tests; the diagnostic
// library only.
func TestImagesShown(cx *Cx) (times uint64, images uint32, dropped uint64) {
	checkCx(cx).flush()
	o := newReq(cPlatformTest)
	o.u8(7)
	r := reader{b: mustCall(*o)}
	return r.u64(), r.u32(), r.u64()
}

// TestDropAccept tells what a DropAccept takes, as the library decodes it: the app's drag value of
// that id, and files from outside. For tests.
func TestDropAccept(cx *Cx, a DropAccept, id uint64) (takesValue, takesFiles bool) {
	checkCx(cx).flush()
	o := newReq(cPlatformTest)
	o.u8(1)
	putDropAccept(o, a)
	o.u64(id)
	r := reader{b: mustCall(*o)}
	return r.bool(), r.bool()
}

// --- the menu bar (PROTOCOLS.md 11.30) -----------------------------------------------------------------

// OsAction is the operating system's own meaning of a menu item (macOS fills the item in).
type OsAction uint8

const (
	OsActionNone      OsAction = 0
	OsActionCut       OsAction = 1
	OsActionCopy      OsAction = 2
	OsActionPaste     OsAction = 3
	OsActionSelectAll OsAction = 4
	OsActionUndo      OsAction = 5
	OsActionRedo      OsAction = 6
)

// Menu is a menu of the menu bar, or a submenu.
type Menu struct {
	Name     string
	Items    []MenuItem
	Disabled bool
}

// MenuItem is one of MenuSeparator, MenuAction, MenuSubmenu, MenuServices.
type MenuItem interface{ isMenuItem() }

// MenuSeparator is a line between items.
type MenuSeparator struct{}

// MenuAction is an item that dispatches an action (gk.NewAction or gk.Shortcut's); the menu shows
// its key binding.
type MenuAction struct {
	Name     string
	Action   Action
	OsAction OsAction
	Checked  bool
	Disabled bool
}

// MenuSubmenu is an item that opens another menu.
type MenuSubmenu struct{ Menu Menu }

// MenuServices is the operating system's Services menu (macOS; elsewhere nothing).
type MenuServices struct{ Name string }

func (MenuSeparator) isMenuItem() {}
func (MenuAction) isMenuItem()    {}
func (MenuSubmenu) isMenuItem()   {}
func (MenuServices) isMenuItem()  {}

func putMenu(o *tape, m Menu, depth int) {
	if depth > 16 {
		stop("gk.SetMenus: menus nested more than 16 deep")
	}
	o.str(m.Name)
	o.bool(m.Disabled)
	o.u32(uint32(len(m.Items)))
	for _, it := range m.Items {
		switch x := it.(type) {
		case MenuSeparator:
			o.u8(0)
		case MenuAction:
			if x.Action.id == 0 {
				stop("gk.SetMenus: the item %q has no action (make one with gk.NewAction, or use the one gk.Shortcut returns)", x.Name)
			}
			o.u8(1)
			o.str(x.Name)
			o.u32(x.Action.id)
			o.u8(uint8(x.OsAction))
			o.bool(x.Checked)
			o.bool(x.Disabled)
		case MenuSubmenu:
			o.u8(2)
			putMenu(o, x.Menu, depth+1)
		case MenuServices:
			o.u8(3)
			o.str(x.Name)
		default:
			stop("gk.SetMenus: a nil or unknown menu item in %q (give gk.MenuSeparator{} for a separator)", m.Name)
		}
	}
}

// SetMenus sets the app's menu bar (App::set_menus): the platform's where it has one (macOS), and
// always the kit's AppMenuBar. Items dispatch their action to the focused element, as its key
// binding would. It runs after the handler.
//
//	quit := gk.Shortcut(cx, "cmd-q", a.quit)
//	gk.SetMenus(cx, gk.Menu{Name: "My App", Items: []gk.MenuItem{gk.MenuAction{Name: "Quit", Action: quit}}})
func SetMenus(cx *Cx, menus ...Menu) {
	t := checkCx(cx)
	o := newReq(cSetMenus)
	o.u32(uint32(len(menus)))
	for _, m := range menus {
		putMenu(o, m, 0)
	}
	t.deferOp(*o)
}

func rdMenu(r *reader) Menu {
	var m Menu
	m.Name = r.str()
	m.Disabled = r.bool()
	n := int(r.u32())
	m.Items = make([]MenuItem, n)
	for i := range m.Items {
		switch r.u8() {
		case 0:
			m.Items[i] = MenuSeparator{}
		case 1:
			var a MenuAction
			a.Name = r.str()
			a.Action = Action{r.u32()}
			a.OsAction = OsAction(r.u8())
			a.Checked = r.bool()
			a.Disabled = r.bool()
			m.Items[i] = a
		case 2:
			m.Items[i] = MenuSubmenu{rdMenu(r)}
		case 3:
			m.Items[i] = MenuServices{r.str()}
		}
	}
	return m
}

// Menus gives the menu bar as the platform holds it (App::get_menus).
func Menus(cx *Cx) []Menu {
	checkCx(cx).flush()
	r := reader{b: mustCall(*newReq(cMenusGet))}
	out := make([]Menu, int(r.u32()))
	for i := range out {
		out[i] = rdMenu(&r)
	}
	return out
}
