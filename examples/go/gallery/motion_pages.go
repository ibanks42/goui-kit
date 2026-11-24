// Motion and element gallery pages: animations, chained tracks, transitions, springs,
// keyframes, presence, focus traps, drag-and-drop, image caches, scroll anchors, and virtual lists.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/ibanks42/goui-kit/gk"
	"github.com/ibanks42/goui-kit/gk/kit"
	"github.com/ibanks42/goui-kit/gk/motion"
	"github.com/ibanks42/goui-kit/gk/s"
	"github.com/ibanks42/goui-kit/gk/theme"
)

type motionPages struct {
	frames motion.Keyframes
	state  motion.TransitionState
	focus  gk.FocusHandle
	cache  gk.ImageCache
	pic    gk.RenderImage
	scroll gk.ScrollHandle
	anchor gk.ScrollAnchor
	scope  motion.TextSelectionScope
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
	{"elem-drop", elemDrop},
	{"elem-image", elemImage},
	{"elem-scope", elemScope},
	{"elem-vlist", elemVlist},
}

var (
	motionRed  = gk.HSLA(0, 0.7, 0.5, 1)
	motionBlue = gk.HSLA(0.6, 0.7, 0.5, 1)
)

var elemPNG = []byte{
	137, 80, 78, 71, 13, 10, 26, 10, 0, 0, 0, 13, 73, 72, 68, 82, 0, 0, 0, 4, 0, 0, 0, 4, 8, 6, 0, 0, 0, 169, 241, 158,
	126, 0, 0, 0, 26, 73, 68, 65, 84, 120, 218, 99, 248, 127, 130, 225, 63, 67, 195, 255, 255, 48, 154, 1, 153, 3, 166, 9,
	170, 0, 0, 141, 255, 42, 33, 250, 235, 186, 63, 0, 0, 0, 0, 73, 69, 78, 68, 174, 66, 96, 130,
}

const elemPNGPath = "gk-elem.png"

func motionPagesInit(a *App, cx *gk.Cx) {
	mp := &a.motion
	frames := []gk.KeyframeF32{{Offset: 0, Value: 40, Easing: gk.EasingLinear}, {Offset: 0.5, Value: 200, Easing: gk.EasingLinear},
		{Offset: 1, Value: 100, Easing: gk.EasingLinear}}
	k, err := motion.KeyframesTryNew(cx, frames).Result()
	if err != nil {
		panic(err)
	}
	mp.frames = k
	gk.Retain(cx, mp.frames)
	mp.state = motion.NewTransitionStateDefault(cx)
	mp.focus = gk.NewFocusHandle(cx)
	mp.cache = gk.NewImageCache(cx)
	mp.pic = gk.NewRenderImage(cx, elemChecker())
	mp.scroll = gk.NewScrollHandle(cx)
	mp.anchor = gk.NewScrollAnchor(cx, mp.scroll)
	mp.scope = motion.NewTextSelectionScopeID(cx)
	mp.rows = gk.NewRowSource(cx, 200, elemRow, gk.Initial(0))
	mp.hrows = gk.NewRowSource(cx, 8, elemRow, gk.Initial(0))
	_ = os.WriteFile(elemPNGPath, elemPNG, 0o644)
}

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
	return kit.Label(fmt.Sprintf("row %d", at.Row))
}

func motionToggle(a *App, label string) gk.Element {
	return kit.Button("toggle").Label(label).OnClick(func(cx *gk.Cx, _ gk.ClickEvent) { a.count++ })
}

func motionBox() gk.DivEl {
	return gk.Div(s.Relative).Size(gk.Px(40)).Bg(gk.HSLA(0.33, 0.6, 0.45, 1))
}

func motionEffect(a *App, cx *gk.Cx) gk.Element {
	e := motion.NewEffectTransition(300*time.Millisecond).Fade(0, 1).SlideX(-40, 0).Width(60, 180)
	return gk.Div().W(gk.Px(320)).H(gk.Px(80)).
		Child(kit.Effect(gk.Div(s.Relative).H(gk.Px(40)).Bg(motionBlue).Child(kit.Label("enter")), e, "enter"))
}

func motionAnimation(a *App, cx *gk.Cx) gk.Element {
	return gk.VFlex(s.Gap2).W(gk.Px(320)).H(gk.Px(120)).
		Child(motion.WithAnimation(motionBox(), "slide", motion.Animation{Ms: 400, Ease: motion.Ease{Kind: motion.EaseInOut}},
			motion.Track{Prop: motion.Left, From: float32(0), To: float32(200)}, motion.Track{Prop: motion.Opacity, From: float32(0.2), To: float32(1)})).
		Child(motion.WithAnimation(gk.Div().Size(gk.Px(30)).Bg(motionRed), "colour",
			motion.Animation{Ms: 400, Ease: motion.Ease{Kind: motion.EaseCubicBezier, P: [4]float32{0.4, 0, 0.2, 1}}},
			motion.Track{Prop: motion.Bg, From: motionRed, To: motionBlue}, motion.Track{Prop: motion.Width, From: float32(30), To: float32(120)}))
}

func motionChain(a *App, cx *gk.Cx) gk.Element {
	anims := []motion.Animation{
		{Ms: 200, Ease: motion.Ease{Kind: motion.EaseOutCubic}},
		{Ms: 200, Ease: motion.Ease{Kind: motion.EaseBounce, Bounce: motion.EaseQuadratic}},
	}
	return gk.Div().W(gk.Px(320)).H(gk.Px(80)).
		Child(motion.WithAnimations(motionBox(), "chain", anims,
			motion.Track{Prop: motion.Left, From: float32(0), To: float32(150), Step: 1}, motion.Track{Prop: motion.Height, From: float32(40), To: float32(70), Step: 2}))
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
		Child(motion.Motion(gk.Div().W(gk.Px(120)).H(gk.Px(40)).Bg(motionRed),
			motion.Value{Prop: motion.Height, Src: motion.ToTransition{ID: "h", Target: pick[float32](open, 120, 40), Policy: motion.NewTransition(300 * time.Millisecond)}},
			motion.Value{Prop: motion.Bg, Src: motion.ToTransition{ID: "c", Target: pick(open, motionBlue, motionRed), Policy: motion.NewTransition(300 * time.Millisecond)}}))
}

func motionSpring(a *App, cx *gk.Cx) gk.Element {
	open := a.count%2 == 1
	return gk.VFlex(s.Gap2).W(gk.Px(320)).
		Child(motionToggle(a, "Toggle")).
		Child(motion.Motion(motionBox(), motion.Value{Prop: motion.Left,
			Src: motion.ToSpring{ID: "x", Target: pick[float32](open, 200, 0), Policy: motion.NewSpring(250 * time.Millisecond).WithDamping(0.5)}}))
}

func motionKeyframes(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(320)).H(gk.Px(60)).
		Child(motion.Motion(gk.Div().H(gk.Px(40)).Bg(motionBlue),
			motion.Value{Prop: motion.Width, Src: motion.AtKeyframes{ID: "kf", Frames: a.motion.frames, Timing: motion.NewTiming(600 * time.Millisecond)}}))
}

func motionPresence(a *App, cx *gk.Cx) gk.Element {
	shown := a.count%2 == 1
	return gk.VFlex(s.Gap2).W(gk.Px(320)).
		Child(motionToggle(a, "Show")).
		Child(motion.Motion(gk.Div().Size(gk.Px(60)).Bg(motionRed),
			motion.Value{Prop: motion.Opacity, Src: motion.AtPresence{Presence: motion.NewPresence("p", shown).Transition(motion.NewTransition(300 * time.Millisecond)),
				From: float32(0), To: float32(1)}},
			motion.Value{Prop: motion.Width, Src: motion.ToState{State: a.motion.state, Target: pick[float32](shown, 200, 60), Policy: motion.NewTransition(300 * time.Millisecond)}}))
}

func motionFocusTrap(a *App, cx *gk.Cx) gk.Element {
	return motion.FocusTrap(gk.Div(s.Flex, s.Gap2, s.P2, s.Border1).W(gk.Px(320)).TrackFocus(a.motion.focus).
		Child(kit.Button("one").Label("One")).Child(kit.Button("two").Label("Two")), "trap", a.motion.focus)
}

func elemDrop(a *App, cx *gk.Cx) gk.Element {
	dropped := func(cx *gk.Cx, d gk.DragValue) { a.count++ }
	src := gk.Div().ID("src").W(gk.Px(100)).H(gk.Px(60)).Bg(theme.Muted).
		OnDrag(gk.DragValue{ID: 7, Text: "seven"}, gk.View{}).
		ExternalDragPayload(gk.ExternalDragPayload{Files: []gk.ExternalFile{{Path: "/tmp/seven.txt"}}}).Child(kit.Label("seven"))
	takes7 := gk.Div().ID("t7").Group("targets").W(gk.Px(100)).H(gk.Px(60)).Bg(theme.Secondary).
		CanDrop(gk.DropAccept{IDs: []uint64{7}}).OnDrop(dropped).
		OnDragMove(func(cx *gk.Cx, e gk.DragMoveEvent) { a.flag = true }).
		Child(gk.Div().Size(gk.Px(20)).Bg(motionRed).GroupDragOver("targets", gk.NewStyleRefinementDefault().Bg(motionBlue)))
	takes8 := gk.Div().ID("t8").W(gk.Px(100)).H(gk.Px(60)).Bg(theme.Secondary).
		CanDrop(gk.DropAccept{IDs: []uint64{8}}).OnDrop(dropped)
	return gk.VFlex(s.Gap2).
		Child(gk.HFlex(s.Gap4).Child(src).Child(takes7).Child(takes8)).
		Child(kit.Label(fmt.Sprintf("drops %d moved %v", a.count, a.flag)))
}

func elemImage(a *App, cx *gk.Cx) gk.Element {
	pic := a.motion.pic
	return gk.HFlex(s.Gap2).ImageCache(gk.RetainAllImages{ID: "elem-images"}).
		Child(gk.Img(gk.ImageSourceRender{Value: pic}).Size(gk.Px(32))).
		Child(gk.Img(gk.ImageSourceCustom{Value: pic}).Size(gk.Px(32)).ImageCache(a.motion.cache)).
		Child(gk.Img(gk.ImageSourceImage{Value: gk.ImageBytes{Format: gk.ImageFormatPng, Bytes: elemPNG}}).Size(gk.Px(32))).
		Child(gk.Img(gk.ImageSourceResource{Value: gk.ResourcePath{Value: elemPNGPath}}).Size(gk.Px(32))).
		Child(gk.Div().ImageCache(a.motion.cache).Child(kit.Avatar().Src(gk.ImageSourceRender{Value: pic})))
}

func elemScope(a *App, cx *gk.Cx) gk.Element {
	scrolled := gk.Div().ID("scrolled").H(gk.Px(60)).W(gk.Px(200)).OverflowYScroll().TrackScroll(a.motion.scroll).
		Child(gk.Div().H(gk.Px(40)).Child(kit.Label("above"))).
		Child(gk.Div().ID("anchored").AnchorScroll(gk.Some(a.motion.anchor)).Child(kit.Label("anchored")))
	tree := gk.Div().ID("chart").A11ySyntheticChildren([]gk.A11yChild{{Key: "q1", Role: gk.RoleListItem, Label: "Q1: 12"}, {Key: "q2", Role: gk.RoleListItem, Label: "Q2: 30"}}).
		Child(kit.Label("chart"))
	ordered := gk.Div(s.Relative).H(gk.Px(30)).WithDynamicPrepaintOrder([]int{1, 0}).
		Child(gk.Div(s.Absolute).Size(gk.Px(30)).Bg(motionRed)).Child(gk.Div(s.Absolute).Left(gk.Px(10)).Size(gk.Px(30)).Bg(motionBlue))
	return gk.VFlex(s.Gap2).
		Child(motion.TextSelectionScopeOf(gk.Div().Child(kit.Label("selectable in its scope")), a.motion.scope)).
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
