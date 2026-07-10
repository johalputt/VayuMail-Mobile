package screens

import (
	"fmt"
	"sync"
	"time"

	"gioui.org/layout"
	"gioui.org/op"

	"github.com/johalputt/VayuMail-Mobile/ui/anim"
	"github.com/johalputt/VayuMail-Mobile/ui/theme"
	"github.com/johalputt/VayuMail-Mobile/ui/widgets"
)

// LockIntent selects what the lock screen is doing.
type LockIntent int

// Lock screen purposes.
const (
	// LockIntentUnlock gates the app at launch/idle.
	LockIntentUnlock LockIntent = iota
	// LockIntentEnroll sets a first PIN (enter, then confirm).
	LockIntentEnroll
	// LockIntentChange replaces the PIN (current, new, confirm).
	LockIntentChange
	// LockIntentDisable removes the PIN (current, then off).
	LockIntentDisable
)

// lockStage is the step within an intent.
type lockStage int

const (
	stageCurrent lockStage = iota
	stageNew
	stageConfirm
)

// lockOutcome carries an async verify/set result back to the UI thread;
// Layout folds it into the stage machine on the next frame.
type lockOutcome struct {
	ok         bool
	err        bool
	errMsg     string
	retryAfter time.Duration
}

// Lock is the PIN screen: dots, keypad, shake-on-error, lockout
// countdown. All PIN work runs off-thread; Layout only reads UI state
// and folds queued outcomes (Rule 5).
type Lock struct {
	pad   widgets.PinPad
	shake anim.Anim

	intent LockIntent
	stage  lockStage
	pin    string
	newPin string
	busy   bool

	errText   string
	lockedTil time.Time
	finished  bool

	mu      sync.Mutex
	pending *lockOutcome
}

// NewLock builds a lock screen for the given purpose.
func NewLock(intent LockIntent) *Lock {
	l := &Lock{}
	l.Begin(intent)
	return l
}

// Begin re-arms the screen for a fresh run of its intent. UI thread only.
func (l *Lock) Begin(intent LockIntent) {
	l.intent = intent
	l.pin, l.newPin = "", ""
	l.busy, l.finished = false, false
	l.errText = ""
	l.stage = stageCurrent
	if intent == LockIntentEnroll {
		l.stage = stageNew
	}
	l.mu.Lock()
	l.pending = nil
	l.mu.Unlock()
}

// title is the prompt for the current stage.
func (l *Lock) title() string {
	switch {
	case l.intent == LockIntentUnlock:
		return "Enter your PIN"
	case l.stage == stageCurrent && l.intent == LockIntentDisable:
		return "Enter PIN to turn off app lock"
	case l.stage == stageCurrent:
		return "Enter your current PIN"
	case l.stage == stageNew:
		return "Choose a PIN (4–12 digits)"
	default:
		return "Repeat the new PIN"
	}
}

// Layout renders the lock screen.
func (l *Lock) Layout(gtx layout.Context, env *Env) layout.Dimensions {
	th := env.Theme
	p := th.Palette

	l.foldOutcome(gtx, env)
	if l.finished {
		l.finished = false
		env.Nav.Pop(gtx.Now)
	}

	wait := time.Until(l.lockedTil)
	waiting := wait > 0
	if waiting {
		gtx.Execute(op.InvalidateCmd{})
	}

	dims := layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return widgets.DrawIcon(gtx, widgets.IconLock, p.Accent, 32)
			}),
			layout.Rigid(layout.Spacer{Height: theme.MD}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return th.Label(gtx, theme.Title, p.OnBackground, l.title(), 1)
			}),
			layout.Rigid(layout.Spacer{Height: theme.SM}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				switch {
				case waiting:
					return th.Label(gtx, theme.Caption, p.Warning,
						"Too many attempts — try again in "+countdown(wait), 1)
				case l.errText != "":
					return th.Label(gtx, theme.Caption, p.Destructive, l.errText, 1)
				case l.busy:
					return th.Label(gtx, theme.Caption, p.Subtle, "Checking…", 1)
				default:
					// Reserve the line so the pad never jumps.
					return th.Label(gtx, theme.Caption, p.Subtle, " ", 1)
				}
			}),
			layout.Rigid(layout.Spacer{Height: theme.LG}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return widgets.PinDots(gtx, th, len(l.pin), &l.shake)
			}),
			layout.Rigid(layout.Spacer{Height: theme.XL}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if l.busy || waiting {
					gtx = gtx.Disabled()
				}
				action, padDims := l.pad.Layout(gtx, th, len(l.pin) >= 4)
				l.applyKey(gtx, env, action)
				return padDims
			}))
	})
	return dims
}

// applyKey folds a keypad action into the flow. UI thread only.
func (l *Lock) applyKey(gtx layout.Context, env *Env, action widgets.PinPadAction) {
	switch {
	case action.Digit != 0 && len(l.pin) < 12:
		l.errText = ""
		l.pin += string(action.Digit)
	case action.Backspace && len(l.pin) > 0:
		l.pin = l.pin[:len(l.pin)-1]
	case action.Submit && len(l.pin) >= 4 && !l.busy:
		l.submit(gtx, env)
	}
}

// submit advances the stage machine with the entered PIN.
func (l *Lock) submit(gtx layout.Context, env *Env) {
	pin := l.pin
	switch {
	case l.stage == stageNew:
		l.newPin = pin
		l.pin = ""
		l.stage = stageConfirm

	case l.stage == stageConfirm:
		if pin != l.newPin {
			l.pin = ""
			l.stage = stageNew
			l.errText = "PINs didn't match — start over"
			l.shake.Start(gtx.Now, 400*time.Millisecond)
			return
		}
		l.busy = true
		l.errText = ""
		env.State.SetPIN(pin, func(err error) {
			l.deliver(&lockOutcome{ok: err == nil, err: err != nil, errMsg: "Could not set PIN"}, env)
		})

	default: // verifying the current PIN
		l.busy = true
		l.errText = ""
		env.State.VerifyPIN(pin, func(ok bool, retryAfter time.Duration) {
			l.deliver(&lockOutcome{ok: ok, retryAfter: retryAfter, errMsg: "Wrong PIN"}, env)
		})
	}
}

// deliver queues an async outcome for the next frame. Any goroutine.
func (l *Lock) deliver(o *lockOutcome, env *Env) {
	l.mu.Lock()
	l.pending = o
	l.mu.Unlock()
	if env.Invalidate != nil {
		env.Invalidate()
	}
}

// foldOutcome applies a queued async result on the UI thread.
func (l *Lock) foldOutcome(gtx layout.Context, env *Env) {
	l.mu.Lock()
	o := l.pending
	l.pending = nil
	l.mu.Unlock()
	if o == nil {
		return
	}
	l.busy = false
	l.pin = ""
	if !o.ok {
		l.errText = o.errMsg
		if o.retryAfter > 0 {
			l.lockedTil = time.Now().Add(o.retryAfter)
		}
		l.shake.Start(gtx.Now, 400*time.Millisecond)
		return
	}
	l.errText = ""
	switch {
	case l.stage == stageConfirm:
		// New PIN stored.
		env.Snack.ShowInfo("App lock on")
		l.finished = true
	case l.intent == LockIntentChange:
		l.stage = stageNew
	case l.intent == LockIntentDisable:
		l.busy = true
		env.State.RemovePIN(func(err error) {
			l.deliver(&lockOutcome{ok: err == nil, err: err != nil, errMsg: "Could not turn off app lock"}, env)
		})
		// The next successful outcome lands in stageCurrent with intent
		// Disable and busy=false — route it out below on the next fold.
		l.intent = lockIntentDisableDone
	case l.intent == lockIntentDisableDone:
		env.Snack.ShowInfo("App lock off")
		l.finished = true
	default:
		// Unlock: AppState already flipped Locked off; nothing to pop.
		l.Begin(LockIntentUnlock)
	}
}

// lockIntentDisableDone is the internal second half of Disable.
const lockIntentDisableDone LockIntent = -1

// countdown formats a lockout wait compactly.
func countdown(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	sec := int(d.Seconds() + 0.999)
	if sec >= 60 {
		return fmt.Sprintf("%d:%02d min", sec/60, sec%60)
	}
	return fmt.Sprintf("%d s", sec)
}
