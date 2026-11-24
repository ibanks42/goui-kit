package rt

import (
	"runtime"
	"sync"
	"weak"
)

// Handle lifetimes (docs/GO.md section 4). gk.Release gives a handle back at once; two rules
// give back what the app forgot:
//
//   - A view owns what is made in its turns (its init, its handlers, its frame handler): kit
//     state, library objects, focus handles, row sources, views and overlays opened with a
//     function. When the view is released they are released with it, the newest first. A dock
//     panel owns its view, so a panel the user closes (removed from the dock and not added back)
//     releases its view and all the view made (dock.KeepWhenClosed keeps it).
//   - A kit state, library object or focus handle that the app can no longer reach is released
//     when the collector finds it: runtime.AddCleanup on the handle value posts its handle, and
//     the next WAKE turn on the UI thread releases it (the library is never called from the
//     cleanup's goroutine). One the app handed to the library outside a view (an argument of a
//     call, or of a value given to one: a panel given to a dock) may be held there, and is kept
//     until it is released or its owner is.
//
// Releasing a handle twice, or one its owner released, stops as before (the library's message);
// a stale handle stops where it is used.

// life is what every copy of a tracked handle's value points at; the collector finding it
// unreachable means no copy is left.
type life struct {
	h uint64
	_ *life // a pointer: never a tiny allocation, whose cleanup could wait for its neighbors
}

const (
	ownState  = iota // kit state, a library object, a focus handle: released through C_RELEASE
	ownView          // a view: releaseView
	ownSource        // a row source: releaseSource
)

type ownEntry struct {
	h    uint64
	kind uint8
}

var (
	ownerOf = map[uint64]uint64{}     // handle -> the handle that owns it (a view, a panel)
	ownedBy = map[uint64][]ownEntry{} // owner -> what it owns, oldest first

	lives  = map[uint64]weak.Pointer[life]{} // tracked handles
	fresh  = map[uint64]*life{}              // made this turn: held until the turn ends
	pinned = map[uint64]*life{}              // handed to the library: kept until released

	gone = map[uint64]struct{}{} // released by their owner or the collector: a gk.Release of one stops

	gcMu    sync.Mutex
	gcQueue []uint64 // handles the collector found unreachable, for the UI thread
)

// own records that the turn's view owns h (nothing when no view's turn runs).
func own(h uint64, kind uint8) {
	if cur == nil || cur.owner == nil || h == 0 || cur.owner.h == h {
		return
	}
	adopt(cur.owner.h, h, kind)
}

func adopt(owner, h uint64, kind uint8) {
	disown(h)
	ownerOf[h] = owner
	ownedBy[owner] = append(ownedBy[owner], ownEntry{h, kind})
}

// disown forgets h's owner: released by the app, it is not released again with its owner.
func disown(h uint64) {
	o, ok := ownerOf[h]
	if !ok {
		return
	}
	delete(ownerOf, h)
	list := ownedBy[o]
	for i, e := range list {
		if e.h == h {
			ownedBy[o] = append(list[:i], list[i+1:]...)
			break
		}
	}
	if len(ownedBy[o]) == 0 {
		delete(ownedBy, o)
	}
}

// checkNotGone stops a gk.Release of a handle the runtime released already with its owner.
func checkNotGone(h uint64) {
	if _, ok := gone[h]; ok {
		Stop("gk.Release was given a handle that was released already, with the view (or dock panel) that made it: what a view makes in its turns goes with it (docs/GO.md section 4). Release a handle once, and not after its view")
	}
}

// AdoptView makes owner (a dock panel) the owner of view v: releasing the panel releases the view.
func AdoptView(owner uint64, v View) { adopt(owner, v.h, ownView) }

// Made records a handle the hand-written packages made (a dock panel): owned by the turn's view,
// released when no copy of its value is left.
func Made(h uint64) {
	track(h)
	own(h, ownState)
}

// Tracked reports whether h is a handle the runtime follows that was not released yet.
func Tracked(h uint64) bool {
	_, ok := lives[h]
	return ok
}

// releaseOwned releases what h owns, the newest first; each also releases what it owns.
func releaseOwned(t *turn, h uint64) {
	list := ownedBy[h]
	if len(list) == 0 {
		return
	}
	delete(ownedBy, h)
	for i := len(list) - 1; i >= 0; i-- {
		e := list[i]
		delete(ownerOf, e.h)
		gone[e.h] = struct{}{}
		switch e.kind {
		case ownView:
			if views.get(e.h) != nil {
				releaseView(t, View{HCore{h: e.h}})
			}
		case ownSource:
			if sources[e.h] != nil {
				releaseSource(t, RowSource{HCore{h: e.h}})
			}
		default:
			releaseOwned(t, e.h)
			forget(e.h)
			keptRelease(e.h)
			releaseHandle(t, e.h)
		}
	}
}

// track makes h a handle the collector releases when no copy of its value is left. The handle
// value is made from it right away (MkH finds it in fresh until the turn ends).
func track(h uint64) {
	if h == 0 {
		return
	}
	l := &life{h: h}
	lives[h] = weak.Make(l)
	fresh[h] = l
	runtime.AddCleanup(l, collected, h)
}

// lifeOf is the life of h, for a handle value made from the number (MkH): every copy shares it.
func lifeOf(h uint64) *life {
	if len(lives) == 0 {
		return nil
	}
	if l := fresh[h]; l != nil {
		return l
	}
	if w, ok := lives[h]; ok {
		return w.Value()
	}
	return nil
}

// forget stops tracking h (released by the app or with its owner).
func forget(h uint64) {
	delete(lives, h)
	delete(fresh, h)
	delete(pinned, h)
}

// Held is a handle given to the library as an argument (generated code): outside a view's
// description the library may keep it, so the collector does not release it.
func Held(h uint64) uint64 {
	if !Vb.Active && len(lives) > 0 {
		if l := lifeOf(h); l != nil {
			pinned[h] = l
		}
	}
	return h
}

// collected runs on the cleanup goroutine: the handle goes to the UI thread, never to the library.
func collected(h uint64) {
	gcMu.Lock()
	first := len(gcQueue) == 0
	gcQueue = append(gcQueue, h)
	gcMu.Unlock()
	if first {
		postWake()
	}
}

// releaseCollected releases, in a WAKE turn, the handles the collector found unreachable that
// are still tracked (one released meanwhile is not).
func releaseCollected(t *turn) {
	gcMu.Lock()
	q := gcQueue
	gcQueue = nil
	gcMu.Unlock()
	for _, h := range q {
		w, ok := lives[h]
		if !ok || w.Value() != nil || pinned[h] != nil {
			continue
		}
		forget(h)
		disown(h)
		gone[h] = struct{}{}
		releaseOwned(t, h)
		keptRelease(h)
		releaseHandle(t, h)
		gcReleased++
	}
}

// endTurnLife lets go of the handles made this turn: their values hold them from here on.
func endTurnLife() {
	if len(fresh) > 0 {
		clear(fresh)
	}
}

var gcReleased int

// TestLife returns how many handles the collector has released and how many are tracked (tests).
func TestLife() (released, tracked int) { return gcReleased, len(lives) }

// TestLiveCounts returns the library's live kit states, views and focus handles (tests).
func TestLiveCounts(cx *Cx) (states, views, focus int) {
	CheckCx(cx).Flush()
	r := Reader{B: MustCall(*NewReq(CObjTestTemps))}
	r.U32()
	r.U32()
	return int(r.U32()), int(r.U32()), int(r.U32())
}
