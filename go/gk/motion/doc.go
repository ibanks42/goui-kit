// Package motion provides declarative animations, spring physics, keyframes,
// presence transitions, focus traps, and cross-element text selection scopes.
//
// Wrap animatable elements with [WithAnimation] or [WithAnimations] for
// keyframed property tracks, or with [Motion] to drive properties ([Prop])
// from transitions ([ToTransition], [ToState]), springs ([ToSpring]),
// keyframes ([AtKeyframes]), or presence state ([AtPresence]). Use [FocusTrap]
// to confine Tab navigation and [NewTextSelectionScopeID] with
// [TextSelectionScopeOf] to share text selection across sibling elements.
package motion
