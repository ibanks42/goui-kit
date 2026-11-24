// Platform integration types and codecs: drag-and-drop values, tooltip unions,
// uploaded RGBA render images, and native application menus.

package rt

// DragValue carries an application-defined drag identifier and display text
// across in-app drag-and-drop gestures.
type DragValue struct {
	ID   uint64
	Text string
}

// PutDragValue encodes v onto o.
func PutDragValue(o *Tape, v DragValue) {
	o.U64(v.ID)
	o.Str(v.Text)
}

// RdDragValue decodes a DragValue from r.
func RdDragValue(r *Reader) DragValue {
	id := r.U64()
	return DragValue{ID: id, Text: r.Str()}
}

// DropAccept specifies which drag payloads (in-app values, specific IDs, or
// external file paths) a drop target accepts.
type DropAccept struct {
	Values bool
	IDs    []uint64
	Paths  bool
}

// PutDropAccept encodes a onto o.
func PutDropAccept(o *Tape, a DropAccept) {
	o.Bool(a.Values || len(a.IDs) > 0)
	o.U32(uint32(len(a.IDs)))
	for _, id := range a.IDs {
		o.U64(id)
	}
	o.Bool(a.Paths)
}

// ViewOrTooltip accepts either a View handle or a generated kit tooltip value.
type ViewOrTooltip interface{ isViewOrTooltip() }

func (View) isViewOrTooltip() {}

// PutViewOrTooltip encodes a View or kit tooltip value onto o.
func PutViewOrTooltip(o *Tape, v ViewOrTooltip) {
	switch x := v.(type) {
	case View:
		o.U8(0)
		PutViewArg(o, x)
	case value:
		o.U8(1)
		o.U32(TakeValue(x.valueNode()))
	case nil:
		o.U8(0)
		o.U64(0)
	default:
		Stop("an unknown tooltip was given: give a gk.View, or overlay.NewKitTooltip(text) or overlay.NewKitTooltipElement(el)")
	}
}

// ImageBytes pairs an encoded image format with its raw bytes.
type ImageBytes struct {
	Format ImageFormat
	Bytes  []byte
}

// PixelImage holds raw RGBA pixel data (Width * Height * 4 bytes) for uploading
// as a RenderImage.
type PixelImage struct {
	Width, Height uint32
	RGBA          []byte
}

// RenderImage is a handle to an uploaded GPU-ready raster image.
type RenderImage struct{ HCore }

// PutBytes encodes raw byte slice b onto o.
func PutBytes(o *Tape, b []byte) { o.Bytes(b) }

// PutRenderImage validates that i is non-zero and encodes its handle onto o.
func PutRenderImage(o *Tape, i RenderImage) {
	if i.h == 0 {
		Stop("a zero gk.RenderImage: make one with gk.NewRenderImage(cx, gk.PixelImage{..}) and keep it")
	}
	o.U64(i.h)
}

// NewRenderImage creates a RenderImage from RGBA pixel data outside a view function.
func NewRenderImage(cx *Cx, p PixelImage) RenderImage {
	t := CheckCx(cx)
	if Vb.Active {
		Stop("a view function may not make an image: make it once, in init or in a handler, and keep it")
	}
	if int(p.Width)*int(p.Height)*4 != len(p.RGBA) {
		Stop("a gk.PixelImage of %dx%d pixels holds %d bytes: it needs width x height x 4 (red, green, blue, alpha per pixel)", p.Width, p.Height, len(p.RGBA))
	}
	t.Flush()
	o := NewReq(CRenderImageNew)
	o.U32(p.Width)
	o.U32(p.Height)
	o.Bytes(p.RGBA)
	r := Reader{B: MustCall(*o)}
	return RenderImage{MkH(r.U64())}
}

// TestRenderImage queries image dimensions, first pixel, and ID in diagnostic tests.
func TestRenderImage(cx *Cx, i RenderImage, custom bool) (w, h, first uint32, id uint64) {
	CheckCx(cx).Flush()
	o := NewReq(CPlatformTest)
	o.U8(4)
	o.Bool(custom)
	PutRenderImage(o, i)
	r := Reader{B: MustCall(*o)}
	return r.U32(), r.U32(), r.U32(), r.U64()
}

// TestImagesShown returns diagnostic render counters for displayed images.
func TestImagesShown(cx *Cx) (times uint64, images uint32, dropped uint64) {
	CheckCx(cx).Flush()
	o := NewReq(CPlatformTest)
	o.U8(7)
	r := Reader{B: MustCall(*o)}
	return r.U64(), r.U32(), r.U64()
}

// TestDropAccept evaluates a DropAccept filter against id in diagnostic tests.
func TestDropAccept(cx *Cx, a DropAccept, id uint64) (takesValue, takesFiles bool) {
	CheckCx(cx).Flush()
	o := NewReq(CPlatformTest)
	o.U8(1)
	PutDropAccept(o, a)
	o.U64(id)
	r := Reader{B: MustCall(*o)}
	return r.Bool(), r.Bool()
}

// OsAction tags a native menu item with a standard platform editing role.
type OsAction uint8

// Standard platform roles for MenuAction.OsAction.
const (
	OsActionNone      OsAction = 0
	OsActionCut       OsAction = 1
	OsActionCopy      OsAction = 2
	OsActionPaste     OsAction = 3
	OsActionSelectAll OsAction = 4
	OsActionUndo      OsAction = 5
	OsActionRedo      OsAction = 6
)

// Menu describes one top-level or nested native application menu.
type Menu struct {
	Name     string
	Items    []MenuItem
	Disabled bool
}

// MenuItem is implemented by MenuSeparator, MenuAction, MenuSubmenu, and
// MenuServices.
type MenuItem interface{ isMenuItem() }

// MenuSeparator inserts a visual divider between menu items.
type MenuSeparator struct{}

// MenuAction is a clickable menu item bound to an Action.
type MenuAction struct {
	Name     string
	Action   Action
	OsAction OsAction
	Checked  bool
	Disabled bool
}

// MenuSubmenu nests a child Menu inside a parent menu.
type MenuSubmenu struct{ Menu Menu }

// MenuServices marks the platform services submenu (used on macOS).
type MenuServices struct{ Name string }

func (MenuSeparator) isMenuItem() {}

func (MenuAction) isMenuItem() {}

func (MenuSubmenu) isMenuItem() {}

func (MenuServices) isMenuItem() {}

func putMenu(o *Tape, m Menu, depth int) {
	if depth > 16 {
		Stop("gk.SetMenus: menus nested more than 16 deep")
	}
	o.Str(m.Name)
	o.Bool(m.Disabled)
	o.U32(uint32(len(m.Items)))
	for _, it := range m.Items {
		switch x := it.(type) {
		case MenuSeparator:
			o.U8(0)
		case MenuAction:
			if x.Action.id == 0 {
				Stop("gk.SetMenus: the item %q has no action (make one with gk.NewAction, or use the one gk.Shortcut returns)", x.Name)
			}
			o.U8(1)
			o.Str(x.Name)
			o.U32(x.Action.id)
			o.U8(uint8(x.OsAction))
			o.Bool(x.Checked)
			o.Bool(x.Disabled)
		case MenuSubmenu:
			o.U8(2)
			putMenu(o, x.Menu, depth+1)
		case MenuServices:
			o.U8(3)
			o.Str(x.Name)
		default:
			Stop("gk.SetMenus: a nil or unknown menu item in %q (give gk.MenuSeparator{} for a separator)", m.Name)
		}
	}
}

// SetMenus installs the application's native menu bar.
func SetMenus(cx *Cx, menus ...Menu) {
	t := CheckCx(cx)
	o := NewReq(CSetMenus)
	o.U32(uint32(len(menus)))
	for _, m := range menus {
		putMenu(o, m, 0)
	}
	t.DeferOp(*o)
}

func rdMenu(r *Reader) Menu {
	var m Menu
	m.Name = r.Str()
	m.Disabled = r.Bool()
	n := int(r.U32())
	m.Items = make([]MenuItem, n)
	for i := range m.Items {
		switch r.U8() {
		case 0:
			m.Items[i] = MenuSeparator{}
		case 1:
			var a MenuAction
			a.Name = r.Str()
			a.Action = Action{r.U32()}
			a.OsAction = OsAction(r.U8())
			a.Checked = r.Bool()
			a.Disabled = r.Bool()
			m.Items[i] = a
		case 2:
			m.Items[i] = MenuSubmenu{rdMenu(r)}
		case 3:
			m.Items[i] = MenuServices{r.Str()}
		}
	}
	return m
}

// Menus returns the currently installed application menus.
func Menus(cx *Cx) []Menu {
	CheckCx(cx).Flush()
	r := Reader{B: MustCall(*NewReq(CMenusGet))}
	out := make([]Menu, int(r.U32()))
	for i := range out {
		out[i] = rdMenu(&r)
	}
	return out
}
