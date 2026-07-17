package screens

import (
	"fmt"
	"sync"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

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
	// stageTOTP asks for the authenticator code after a correct PIN when
	// the second factor is enrolled — for unlocking and for the sensitive
	// flows (change/disable) alike.
	stageTOTP
)

// lockOutcome carries an async verify/set result back to the UI thread;
// Layout folds it into the stage machine on the next frame.
type lockOutcome struct {
	ok         bool
	err        bool
	errMsg     string
	retryAfter time.Duration
	// totpNext: the PIN was right but the enrolled second factor still
	// gates the flow.
	totpNext bool
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

	// bioBtn triggers the biometric prompt; bioPrompted guards the one
	// automatic prompt shown when the unlock screen first appears, so it
	// isn't re-shown every frame or after the user declines.
	bioBtn      widget.Clickable
	bioPrompted bool

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
	l.bioPrompted = false
	if intent == LockIntentEnroll {
		l.stage = stageNew
	}
	l.mu.Lock()
	l.pending = nil
	l.mu.Unlock()
}

// promptBiometric fires the fingerprint/face prompt on the unlock flow.
// The result folds through the same async outcome mailbox the PIN uses.
func (l *Lock) promptBiometric(env *Env) {
	if l.busy {
		return
	}
	l.busy = true
	l.errText = ""
	env.State.UnlockWithBiometric(func(ok bool, totpNext bool) {
		o := &lockOutcome{ok: ok, totpNext: totpNext}
		if !ok {
			// A declined or failed biometric is not an error to shout about —
			// the PIN pad is right there. Keep the line quiet.
			o.errMsg = ""
		}
		l.deliver(o, env)
	})
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
	case l.stage == stageTOTP:
		return "Enter your authenticator code"
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

	// Offer biometrics on the unlock flow only. Auto-prompt once when the
	// gate first appears (the platform-native UX), and let the user re-trigger
	// it with the fingerprint key if they dismiss it.
	bioReady := l.intent == LockIntentUnlock && l.stage == stageCurrent &&
		env.State.BiometricUnlockReady()
	if bioReady && !l.bioPrompted && !l.busy && !waiting {
		l.bioPrompted = true
		l.promptBiometric(env)
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
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !bioReady {
					return layout.Dimensions{}
				}
				if l.bioBtn.Clicked(gtx) && !l.busy && !waiting {
					l.bioPrompted = true
					l.promptBiometric(env)
				}
				return layout.Inset{Top: theme.MD}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return l.bioBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return widgets.DrawIcon(gtx, widgets.IconFingerprint, p.Accent, 20)
							}),
							layout.Rigid(layout.Spacer{Width: theme.SM}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return th.Label(gtx, theme.Caption, p.Accent, "Use fingerprint", 1)
							}))
					})
				})
			}))
	})
	return dims
}

// applyKey folds a keypad action into the flow. UI thread only.
func (l *Lock) applyKey(gtx layout.Context, env *Env, action widgets.PinPadAction) {
	limit := 12
	if l.stage == stageTOTP {
		limit = 6
	}
	switch {
	case action.Digit != 0 && len(l.pin) < limit:
		l.errText = ""
		l.pin += string(action.Digit)
		// Authenticator codes are always six digits — submit on the last.
		if l.stage == stageTOTP && len(l.pin) == 6 && !l.busy {
			l.submit(gtx, env)
		}
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

	case l.stage == stageTOTP:
		l.busy = true
		l.errText = ""
		unlockOn := l.intent == LockIntentUnlock
		env.State.VerifyTOTP(pin, unlockOn, func(ok bool, retryAfter time.Duration) {
			l.deliver(&lockOutcome{ok: ok, retryAfter: retryAfter, errMsg: "Wrong code"}, env)
		})

	default: // verifying the current PIN
		l.busy = true
		l.errText = ""
		env.State.VerifyPIN(pin, func(ok bool, retryAfter time.Duration, totpNext bool) {
			l.deliver(&lockOutcome{ok: ok, retryAfter: retryAfter, totpNext: totpNext, errMsg: "Wrong PIN"}, env)
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
		// A silent failure (a declined/failed biometric carries no message
		// and no lockout) doesn't shake the pad — the user just uses the PIN.
		if o.errMsg != "" || o.retryAfter > 0 {
			l.shake.Start(gtx.Now, 400*time.Millisecond)
		}
		return
	}
	l.errText = ""
	if o.totpNext {
		// Correct PIN, second factor enrolled: same pad, six digits.
		l.stage = stageTOTP
		return
	}
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
