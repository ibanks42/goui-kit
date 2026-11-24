// Package dock provides dockable workspace areas, split pane trees, tabbed
// panel groups, layout serialization, skinning, and custom chrome renderers.
//
// Create panels with [NewPanel] (which adopts its content [gk.View] and
// automatically releases it when closed unless [KeepWhenClosed] is passed) and
// mount a dock [Area] inside a view tree using [View]. Layouts can be saved
// and restored via [AreaState], [PanelState], and factories registered with
// [RegisterPanel]. Applications that want custom dock chrome can override
// individual slots using [NewRenderer], [Hooks], [RenderAnswer], and [Slot].
package dock
