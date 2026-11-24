// Handle lifetime management: view-scoped ownership cascades and GC cleanup.
//
// Rust handles held by Go code are freed through three cooperating mechanisms:
//  1. Explicit gk.Release(cx, h): frees the handle immediately and cascades to
//     every handle it owns.
//  2. View/panel ownership (ownerOf / ownedBy): any state, child view, or row
//     source created during a view's init or event turn is owned by that view
//     (and dock.NewPanel adopts its view). Releasing the owner releases all of
//     its owned handles in reverse creation order (LIFO).
//  3. Garbage collection (lives / fresh / pinned + runtime.AddCleanup): state
//     handles embed a *life pointer inside HCore. When all Go copies become
//     unreachable, Go's cleanup callback pushes the handle onto gcQueue and
//     wakes the UI thread; KWake drains gcQueue via releaseCollected.

package rt

import (
	"runtime"
	"sync"
	"weak"
)

// life is the heap-allocated GC token embedded inside HCore for tracked handles.
// The unused self-typed pointer field ensures Go allocates a distinct non-zero
// object with pointer-bearing memory so runtime.AddCleanup always tracks it.
type life struct {
	h uint64
	_ *life
}

// Ownership categories recorded in ownedBy so releasing a parent view cascades
// cleanly through child views, row sources, and kit states.
const (
	ownState = iota
	ownView
	ownSource
)

type ownEntry struct {
	h    uint64
	kind uint8
}

// Handle ownership and GC lifetime tracking tables. All maps are accessed only
// on the UI thread; gcQueue is guarded by gcMu because collected() runs on a
// Go GC cleanup goroutine.
var (
	ownerOf = map[uint64]uint64{}
	ownedBy = map[uint64][]ownEntry{}

	// lives holds weak references to every GC-tracked handle's life token.
	// fresh pins newly created tokens until the end of the top-level turn so a
	// handle cannot be collected before the app stores it in state or returns it.
	// pinned permanently retains tokens for handles handed to Rust outside a
	// view build (via Held) so GC cannot free objects Rust still references.
	lives  = map[uint64]weak.Pointer[life]{}
	fresh  = map[uint64]*life{}
	pinned = map[uint64]*life{}

	// gone records handles already released by an owner cascade or GC cleanup so
	// a later explicit gk.Release reports an informative double-release error.
	gone = map[uint64]struct{}{}

	gcMu    sync.Mutex
	gcQueue []uint64
)

// own attaches handle h to the currently active turn's owning view, if any.
func own(h uint64, kind uint8) {
	if cur == nil || cur.owner == nil || h == 0 || cur.owner.h == h {
		return
	}
	adopt(cur.owner.h, h, kind)
}

// adopt assigns handle h to owner, removing any previous ownership link first.
func adopt(owner, h uint64, kind uint8) {
	disown(h)
	ownerOf[h] = owner
	ownedBy[owner] = append(ownedBy[owner], ownEntry{h, kind})
}

// disown detaches handle h from its owning view without releasing it.
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

// checkNotGone stops if h was already released by an earlier cascade or GC cleanup.
func checkNotGone(h uint64) {
	if _, ok := gone[h]; ok {
		Stop("gk.Release was given a handle that was released already, with the view (or dock panel) that made it: what a view makes in its turns goes with it (docs/GO.md section 4). Release a handle once, and not after its view")
	}
}

// AdoptView reparents view v so releasing owner also releases v.
func AdoptView(owner uint64, v View) { adopt(owner, v.h, ownView) }

// Made registers a newly created Rust state handle for GC cleanup and attaches
// it to the current turn's owning view.
func Made(h uint64) {
	track(h)
	own(h, ownState)
}

// Tracked reports whether handle h is currently tracked for GC cleanup.
func Tracked(h uint64) bool {
	_, ok := lives[h]
	return ok
}

// releaseOwned recursively releases every handle owned by h in reverse creation
// order (LIFO) so child resources tear down before the handles they reference.
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

// track creates a GC-tracked life token for h, pins it for the current turn,
// and installs a runtime cleanup that enqueues h onto gcQueue when unreachable.
func track(h uint64) {
	if h == 0 {
		return
	}
	l := &life{h: h}
	lives[h] = weak.Make(l)
	fresh[h] = l
	runtime.AddCleanup(l, collected, h)
}

// lifeOf returns the live *life token for h, or nil if h is untracked or gone.
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

// forget removes handle h from all GC-tracking maps.
func forget(h uint64) {
	delete(lives, h)
	delete(fresh, h)
	delete(pinned, h)
}

// Held pins a tracked handle when it is passed to the library outside a view
// build (for example, stored in a Rust object), preventing premature GC cleanup.
func Held(h uint64) uint64 {
	if !Vb.Active && len(lives) > 0 {
		heldSlow(h)
	}
	return h
}

func heldSlow(h uint64) {
	if l := lifeOf(h); l != nil {
		pinned[h] = l
	}
}

// collected is the runtime.AddCleanup callback. It runs on a GC goroutine,
// queues h for UI-thread release, and wakes the event loop if the queue was empty.
func collected(h uint64) {
	gcMu.Lock()
	first := len(gcQueue) == 0
	gcQueue = append(gcQueue, h)
	gcMu.Unlock()
	if first {
		postWake()
	}
}

// releaseCollected drains gcQueue on the UI thread during KWake turns and
// releases every handle whose weak token is actually dead and not pinned.
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

// endTurnLife drops the per-turn strong references in fresh so handles that the
// app did not keep become eligible for garbage collection.
func endTurnLife() {
	if len(fresh) > 0 {
		clear(fresh)
	}
}

var gcReleased int

// TestLife returns the count of GC-released handles and currently tracked handles.
func TestLife() (released, tracked int) { return gcReleased, len(lives) }

// TestLiveCounts queries the diagnostic library for live Rust state, view, and
// focus handle counts.
func TestLiveCounts(cx *Cx) (states, views, focus int) {
	CheckCx(cx).Flush()
	r := Reader{B: MustCall(*NewReq(CObjTestTemps))}
	r.U32()
	r.U32()
	return int(r.U32()), int(r.U32()), int(r.U32())
}
