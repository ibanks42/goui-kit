// The platform and motion families' gallery pages, the Go twin of
// examples/odin/gallery/motion_pages.odin and of ref/src/bin/gallery/motion_pages.rs, call for
// call (docs/PROTOCOLS.md 13.30): gpui's with_animation and a chain of animations with the kit's
// easing curves, the kit's EffectTransition, and the kit's motion values (transition, spring,
// keyframes, presence, a TransitionState kept with the page's data) sampled while the library
// draws, the focus trap, and gpui's element leftovers (PROTOCOLS.md 13.31: drop targets, image
// caches, a scroll anchor, synthetic accessibility children, prepaint order, a text selection
// scope, virtual lists). Offscreen the clock moves only on `advance`, so every frame is at a fixed
// time and the pages compare byte for byte with Rust.
package main

import (
	"fmt"
	"os"
	"time"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/theme"
)

type motionPages struct {
	frames gk.Keyframes
	state  gk.TransitionState
	focus  gk.FocusHandle
	// the elem-* pages
	cache  gk.ImageCache
	pic    gk.RenderImage // made once from pixels: gpui keeps one texture for it
	scroll gk.ScrollHandle
	anchor gk.ScrollAnchor
	scope  gk.TextSelectionScope
	rows   gk.RowSource
	hrows  gk.RowSource
}

var motionPageTable = []Page{
	{"motion-animation", motionAnimation},
	{"motion-chain", motionChain},
	{"motion-effect", motionEffect},
	{"motion-transition", motionTransition},
	{"motion-spring", motionSpring},
	{"motion-keyframes", motionKeyframes},
	{"motion-presence", motionPresence},
	{"motion-focus-trap", motionFocusTrap},
	// PROTOCOLS.md 13.31: gpui's element leftovers
	{"elem-drop", elemDrop},
	{"elem-image", elemImage},
	{"elem-scope", elemScope},
	{"elem-vlist", elemVlist},
}

var (
	motionRed  = gk.HSLA(0, 0.7, 0.5, 1)
	motionBlue = gk.HSLA(0.6, 0.7, 0.5, 1)
)

// elemPNG is a 4 x 4 PNG; ImageSource's Path variant reads it from elemPNGPath (written where
// the gallery runs, as the Rust twin does).
var elemPNG = []byte{
	137, 80, 78, 71, 13, 10, 26, 10, 0, 0, 0, 13, 73, 72, 68, 82, 0, 0, 0, 4, 0, 0, 0, 4, 8, 6, 0, 0, 0, 169, 241, 158,
	126, 0, 0, 0, 26, 73, 68, 65, 84, 120, 218, 99, 248, 127, 130, 225, 63, 67, 195, 255, 255, 48, 154, 1, 153, 3, 166, 9,
	170, 0, 0, 141, 255, 42, 33, 250, 235, 186, 63, 0, 0, 0, 0, 73, 69, 78, 68, 174, 66, 96, 130,
}

const elemPNGPath = "gk-elem.png"

// The states in the order of the Odin twin's motion_pages_init.
func motionPagesInit(a *App, cx *gk.Cx) {
	mp := &a.motion
	frames := []gk.KeyframeF32{{Offset: 0, Value: 40, Easing: gk.EasingLinear}, {Offset: 0.5, Value: 200, Easing: gk.EasingLinear},
		{Offset: 1, Value: 100, Easing: gk.EasingLinear}}
	k, err := gk.KeyframesTryNew(cx, frames).Result()
	if err != nil {
		panic(err)
	}
	mp.frames = k
	gk.Retain(cx, mp.frames) // a fallible constructor hands out a temporary handle
	mp.state = gk.NewTransitionStateDefault(cx)
	mp.focus = gk.NewFocusHandle(cx)
	mp.cache = gk.NewImageCache(cx)
	mp.pic = gk.NewRenderImage(cx, elemChecker())
	mp.scroll = gk.NewScrollHandle(cx)
	mp.anchor = gk.NewScrollAnchor(cx, mp.scroll)
	mp.scope = gk.NewTextSelectionScopeID(cx)
	mp.rows = gk.NewRowSource(cx, 200, elemRow, gk.Initial(0))
	mp.hrows = gk.NewRowSource(cx, 8, elemRow, gk.Initial(0))
	_ = os.WriteFile(elemPNGPath, elemPNG, 0o644)
}

// elemChecker is a 4 x 4 checker of two colors, as RGBA pixels.
func elemChecker() gk.PixelImage {
	px := make([]byte, 4*4*4)
	for y := range 4 {
		for x := range 4 {
			c := [4]byte{40, 40, 200, 255}
			if (x+y)%2 == 0 {
				c = [4]byte{200, 40, 40, 255}
			}
			copy(px[(y*4+x)*4:], c[:])
		}
	}
	return gk.PixelImage{Width: 4, Height: 4, RGBA: px}
}

func elemRow(cx *gk.Cx, at gk.RowAt) gk.Element {
	return gk.Label(fmt.Sprintf("row %d", at.Row))
}

func motionToggle(a *App, label string) gk.Element {
	return gk.Button("toggle").Label(label).OnClick(func(cx *gk.Cx, _ gk.ClickEvent) { a.count++ })
}

func motionBox() gk.DivEl {
	return gk.Div(s.Relative).Size(gk.Px(40)).Bg(gk.HSLA(0.33, 0.6, 0.45, 1))
}

func motionEffect(a *App, cx *gk.Cx) gk.Element {
	e := gk.NewEffectTransition(300*time.Millisecond).Fade(0, 1).SlideX(-40, 0).Width(60, 180)
	return gk.Div().W(gk.Px(320)).H(gk.Px(80)).
		Child(gk.Effect(gk.Div(s.Relative).H(gk.Px(40)).Bg(motionBlue).Child(gk.Label("enter")), e, "enter"))
}

func motionAnimation(a *App, cx *gk.Cx) gk.Element {
	return gk.VFlex(s.Gap2).W(gk.Px(320)).H(gk.Px(120)).
		Child(gk.WithAnimation(motionBox(), "slide", gk.Animation{Ms: 400, Ease: gk.Ease{Kind: gk.EaseInOut}},
			gk.Track{Prop: gk.MotionLeft, From: float32(0), To: float32(200)}, gk.Track{Prop: gk.MotionOpacity, From: float32(0.2), To: float32(1)})).
		Child(gk.WithAnimation(gk.Div().Size(gk.Px(30)).Bg(motionRed), "colour",
			gk.Animation{Ms: 400, Ease: gk.Ease{Kind: gk.EaseCubicBezier, P: [4]float32{0.4, 0, 0.2, 1}}},
			gk.Track{Prop: gk.MotionBg, From: motionRed, To: motionBlue}, gk.Track{Prop: gk.MotionWidth, From: float32(30), To: float32(120)}))
}

func motionChain(a *App, cx *gk.Cx) gk.Element {
	anims := []gk.Animation{
		{Ms: 200, Ease: gk.Ease{Kind: gk.EaseOutCubic}},
		{Ms: 200, Ease: gk.Ease{Kind: gk.EaseBounce, Bounce: gk.EaseQuadratic}},
	}
	return gk.Div().W(gk.Px(320)).H(gk.Px(80)).
		Child(gk.WithAnimations(motionBox(), "chain", anims,
			gk.Track{Prop: gk.MotionLeft, From: float32(0), To: float32(150), Step: 1}, gk.Track{Prop: gk.MotionHeight, From: float32(40), To: float32(70), Step: 2}))
}

func pick[T any](c bool, a, b T) T {
	if c {
		return a
	}
	return b
}

func motionTransition(a *App, cx *gk.Cx) gk.Element {
	open := a.count%2 == 1
	return gk.VFlex(s.Gap2).W(gk.Px(320)).
		Child(motionToggle(a, "Toggle")).
		Child(gk.Motion(gk.Div().W(gk.Px(120)).H(gk.Px(40)).Bg(motionRed),
			gk.MotionValue{Prop: gk.MotionHeight, Src: gk.ToTransition{ID: "h", Target: pick[float32](open, 120, 40), Policy: gk.NewTransition(300 * time.Millisecond)}},
			gk.MotionValue{Prop: gk.MotionBg, Src: gk.ToTransition{ID: "c", Target: pick(open, motionBlue, motionRed), Policy: gk.NewTransition(300 * time.Millisecond)}}))
}

func motionSpring(a *App, cx *gk.Cx) gk.Element {
	open := a.count%2 == 1
	return gk.VFlex(s.Gap2).W(gk.Px(320)).
		Child(motionToggle(a, "Toggle")).
		Child(gk.Motion(motionBox(), gk.MotionValue{Prop: gk.MotionLeft,
			Src: gk.ToSpring{ID: "x", Target: pick[float32](open, 200, 0), Policy: gk.NewSpring(250 * time.Millisecond).WithDamping(0.5)}}))
}

func motionKeyframes(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(320)).H(gk.Px(60)).
		Child(gk.Motion(gk.Div().H(gk.Px(40)).Bg(motionBlue),
			gk.MotionValue{Prop: gk.MotionWidth, Src: gk.AtKeyframes{ID: "kf", Frames: a.motion.frames, Timing: gk.NewTiming(600 * time.Millisecond)}}))
}

func motionPresence(a *App, cx *gk.Cx) gk.Element {
	shown := a.count%2 == 1
	return gk.VFlex(s.Gap2).W(gk.Px(320)).
		Child(motionToggle(a, "Show")).
		Child(gk.Motion(gk.Div().Size(gk.Px(60)).Bg(motionRed),
			gk.MotionValue{Prop: gk.MotionOpacity, Src: gk.AtPresence{Presence: gk.NewPresence("p", shown).Transition(gk.NewTransition(300 * time.Millisecond)),
				From: float32(0), To: float32(1)}},
			gk.MotionValue{Prop: gk.MotionWidth, Src: gk.ToState{State: a.motion.state, Target: pick[float32](shown, 200, 60), Policy: gk.NewTransition(300 * time.Millisecond)}}))
}

func motionFocusTrap(a *App, cx *gk.Cx) gk.Element {
	return gk.FocusTrap(gk.Div(s.Flex, s.Gap2, s.P2, s.Border1).W(gk.Px(320)).TrackFocus(a.motion.focus).
		Child(gk.Button("one").Label("One")).Child(gk.Button("two").Label("Two")), "trap", a.motion.focus)
}

// --- PROTOCOLS.md 13.31: gpui's element leftovers. The drop targets take data where Rust gives
// closures: can_drop's ids, the files a drag becomes outside the window; the images are bytes and
// pixels.

func elemDrop(a *App, cx *gk.Cx) gk.Element {
	dropped := func(cx *gk.Cx, d gk.DragValue) { a.count++ }
	src := gk.Div().ID("src").W(gk.Px(100)).H(gk.Px(60)).Bg(theme.Muted).
		OnDrag(gk.DragValue{ID: 7, Text: "seven"}, gk.View{}).
		ExternalDragPayload(gk.ExternalDragPayload{Files: []gk.ExternalFile{{Path: "/tmp/seven.txt"}}}).Child(gk.Label("seven"))
	takes7 := gk.Div().ID("t7").Group("targets").W(gk.Px(100)).H(gk.Px(60)).Bg(theme.Secondary).
		CanDrop(gk.DropAccept{IDs: []uint64{7}}).OnDrop(dropped).
		OnDragMove(func(cx *gk.Cx, e gk.DragMoveEvent) { a.flag = true }).
		Child(gk.Div().Size(gk.Px(20)).Bg(motionRed).GroupDragOver("targets", gk.NewStyleRefinementDefault().Bg(motionBlue)))
	takes8 := gk.Div().ID("t8").W(gk.Px(100)).H(gk.Px(60)).Bg(theme.Secondary).
		CanDrop(gk.DropAccept{IDs: []uint64{8}}).OnDrop(dropped)
	return gk.VFlex(s.Gap2).
		Child(gk.HFlex(s.Gap4).Child(src).Child(takes7).Child(takes8)).
		Child(gk.Label(fmt.Sprintf("drops %d moved %v", a.count, a.flag)))
}

func elemImage(a *App, cx *gk.Cx) gk.Element {
	pic := a.motion.pic
	return gk.HFlex(s.Gap2).ImageCache(gk.RetainAllImages{ID: "elem-images"}).
		Child(gk.Img(gk.ImageSourceRender{Value: pic}).Size(gk.Px(32))).
		Child(gk.Img(gk.ImageSourceCustom{Value: pic}).Size(gk.Px(32)).ImageCache(a.motion.cache)).
		Child(gk.Img(gk.ImageSourceImage{Value: gk.ImageBytes{Format: gk.ImageFormatPng, Bytes: elemPNG}}).Size(gk.Px(32))).
		Child(gk.Img(gk.ImageSourceResource{Value: gk.ResourcePath{Value: elemPNGPath}}).Size(gk.Px(32))).
		Child(gk.Div().ImageCache(a.motion.cache).Child(gk.Avatar().Src(gk.ImageSourceRender{Value: pic})))
}

func elemScope(a *App, cx *gk.Cx) gk.Element {
	scrolled := gk.Div().ID("scrolled").H(gk.Px(60)).W(gk.Px(200)).OverflowYScroll().TrackScroll(a.motion.scroll).
		Child(gk.Div().H(gk.Px(40)).Child(gk.Label("above"))).
		Child(gk.Div().ID("anchored").AnchorScroll(gk.Some(a.motion.anchor)).Child(gk.Label("anchored")))
	tree := gk.Div().ID("chart").A11ySyntheticChildren([]gk.A11yChild{{Key: "q1", Role: gk.RoleListItem, Label: "Q1: 12"}, {Key: "q2", Role: gk.RoleListItem, Label: "Q2: 30"}}).
		Child(gk.Label("chart"))
	ordered := gk.Div(s.Relative).H(gk.Px(30)).WithDynamicPrepaintOrder([]int{1, 0}).
		Child(gk.Div(s.Absolute).Size(gk.Px(30)).Bg(motionRed)).Child(gk.Div(s.Absolute).Left(gk.Px(10)).Size(gk.Px(30)).Bg(motionBlue))
	return gk.VFlex(s.Gap2).
		Child(gk.TextSelectionScopeOf(gk.Div().Child(gk.Label("selectable in its scope")), a.motion.scope)).
		Child(scrolled).Child(tree).Child(ordered)
}

func elemVlist(a *App, cx *gk.Cx) gk.Element {
	sizes := make([][2]float32, 200)
	for i := range sizes {
		sizes[i] = [2]float32{60, float32(20 + (i%3)*8)}
	}
	return gk.VFlex(s.Gap2).
		Child(gk.VVirtualList("v", a.motion.rows, sizes).W(gk.Px(200)).H(gk.Px(160))).
		Child(gk.HVirtualList("h", a.motion.hrows, sizes[:8]).W(gk.Px(400)).H(gk.Px(24)))
}
