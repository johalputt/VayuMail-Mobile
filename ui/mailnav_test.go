package ui

import "testing"

func TestMailNavTargetConsumedOnce(t *testing.T) {
	// A fresh set target is returned once with the right ids…
	SetMailNavTarget(7, 12)
	a, f, ok := consumeMailNavTarget()
	if !ok || a != 7 || f != 12 {
		t.Fatalf("consume = (%d, %d, %v), want (7, 12, true)", a, f, ok)
	}
	// …and never a second time (so the app doesn't yank the user back).
	if _, _, ok := consumeMailNavTarget(); ok {
		t.Fatal("target must be consumed exactly once")
	}
}

func TestMailNavWakeFires(t *testing.T) {
	woke := false
	setMailNavWake(func() { woke = true })
	t.Cleanup(func() { setMailNavWake(nil) })
	SetMailNavTarget(1, 0)
	if !woke {
		t.Fatal("setting a target should wake the frame loop")
	}
	_, _, _ = consumeMailNavTarget() // drain for other tests
}
