package state

import (
	"time"
)

// Screen identifies one full-screen view.
type Screen int

// All screens in the app.
const (
	ScreenInbox Screen = iota
	ScreenThread
	ScreenCompose
	ScreenSetup
	ScreenSettings
	ScreenSearch
	// ScreenLock hosts PIN enrollment/change/disable flows pushed from
	// Settings. The launch/idle unlock gate is drawn by the root instead
	// and never enters this stack.
	ScreenLock
	// ScreenTalk is the VayuTalk conversation list (ephemeral E2E chat).
	ScreenTalk
	// ScreenTalkRoom is one VayuTalk conversation with its compose bar.
	ScreenTalkRoom
	// ScreenTalkVerify shows a peer's safety number for out-of-band
	// verification.
	ScreenTalkVerify
)

// Motion constants from the design spec: push 200ms ease-out cubic,
// pop 150ms ease-in cubic. No bounce, no spring.
const (
	pushDuration = 200 * time.Millisecond
	popDuration  = 150 * time.Millisecond
)

// Nav is the navigation stack with slide-transition state.
type Nav struct {
	stack []Screen

	// transition state
	animStart time.Time
	pushing   bool
	animating bool
	// leaving is the screen sliding out during a pop.
	leaving Screen
}

// NewNav starts on the given root screen.
func NewNav(root Screen) *Nav {
	return &Nav{stack: []Screen{root}}
}

// Current returns the active screen.
func (n *Nav) Current() Screen {
	return n.stack[len(n.stack)-1]
}

// Depth returns the stack depth (1 = at root).
func (n *Nav) Depth() int { return len(n.stack) }

// Under returns the screen below the top of the stack — the one revealed
// while the top screen animates in. At the root it returns the root.
func (n *Nav) Under() Screen {
	if len(n.stack) < 2 {
		return n.stack[0]
	}
	return n.stack[len(n.stack)-2]
}

// Push navigates forward with the slide-in-from-right transition.
func (n *Nav) Push(s Screen, now time.Time) {
	if n.Current() == s {
		return
	}
	n.stack = append(n.stack, s)
	n.animStart = now
	n.pushing = true
	n.animating = true
}

// Pop navigates back with the slide-out-to-right transition. Returns
// false at the stack root (the platform back action should then exit or
// background the app).
func (n *Nav) Pop(now time.Time) bool {
	if len(n.stack) <= 1 {
		return false
	}
	n.leaving = n.Current()
	n.stack = n.stack[:len(n.stack)-1]
	n.animStart = now
	n.pushing = false
	n.animating = true
	return true
}

// Replace swaps the whole stack for a new root without animation — used
// when onboarding completes.
func (n *Nav) Replace(root Screen) {
	n.stack = n.stack[:0]
	n.stack = append(n.stack, root)
	n.animating = false
}

// Transition reports the in-flight animation: which screen is entering
// or leaving and its horizontal progress in [0,1] (1 = settled). done is
// true when no animation is running. The frame loop must request another
// frame while done is false.
func (n *Nav) Transition(now time.Time) (entering bool, other Screen, progress float32, done bool) {
	if !n.animating {
		return false, 0, 1, true
	}
	elapsed := now.Sub(n.animStart)
	d := pushDuration
	if !n.pushing {
		d = popDuration
	}
	if elapsed >= d {
		n.animating = false
		return n.pushing, n.leaving, 1, true
	}
	t := float32(elapsed) / float32(d)
	if n.pushing {
		// Ease-out cubic.
		t = 1 - (1-t)*(1-t)*(1-t)
	} else {
		// Ease-in cubic.
		t = t * t * t
	}
	return n.pushing, n.leaving, t, false
}
