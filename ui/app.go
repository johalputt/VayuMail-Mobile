// Package ui is the Gio root: window, theme, navigation router, and the
// single-threaded event loop. This is the only layer (besides platform/)
// allowed to import Gio (Constitutional Rule 4).
package ui

import (
	"context"
	"image"
	"io"
	"time"

	"gioui.org/app"
	"gioui.org/io/key"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/johalputt/VayuMail-Mobile/internal/applock"
	appcrypto "github.com/johalputt/VayuMail-Mobile/internal/crypto"
	"github.com/johalputt/VayuMail-Mobile/internal/store"
	"github.com/johalputt/VayuMail-Mobile/internal/syncmanager"
	"github.com/johalputt/VayuMail-Mobile/ui/screens"
	"github.com/johalputt/VayuMail-Mobile/ui/state"
	"github.com/johalputt/VayuMail-Mobile/ui/theme"
	"github.com/johalputt/VayuMail-Mobile/ui/widgets"
)

// UI owns the window-level state and screen instances.
type UI struct {
	window *app.Window
	th     *theme.Theme
	st     *state.AppState
	nav    *state.Nav
	env    *screens.Env

	inbox      *screens.Inbox
	thread     *screens.Thread
	compose    *screens.Compose
	setup      *screens.AccountSetup
	settings   *screens.Settings
	search     *screens.Search
	lockSetup  *screens.Lock
	lockGate   *screens.Lock
	talk       *screens.Talk
	talkRoom   *screens.TalkRoom
	talkVerify *screens.TalkVerify

	events    <-chan syncmanager.Event
	notify    *mailNotifier
	lastFrame time.Time
}

// New wires the UI over an already-started sync manager. dark is the
// platform theme preference, probed off the UI thread during startup
// (see cmd/vayumail: the probe must never block the first frame). ks is
// the same keystore the sync manager uses — the app-lock verifier lives
// in it too (Rule 6: never in SQLite). pickFile opens the platform file
// picker for composer attachments; nil when no picker is available.
func New(ctx context.Context, w *app.Window, db *store.DB, mgr *syncmanager.Manager, ks appcrypto.Keystore, dark bool, pickFile func() (io.ReadCloser, error)) *UI {
	th := theme.New(dark)
	st := state.New(ctx, db, mgr, applock.New(ks, db))
	st.SetInvalidate(w.Invalidate)

	snack := &widgets.Snackbar{}
	st.Notify = func(msg string) {
		snack.ShowInfo(msg)
		w.Invalidate()
	}
	st.NotifyUndo = func(msg string, onUndo, onCommit func()) {
		snack.Show(msg, onUndo, onCommit)
		w.Invalidate()
	}

	env := &screens.Env{
		Theme:      th,
		State:      st,
		Nav:        state.NewNav(state.ScreenInbox),
		Snack:      snack,
		Composer:   widgets.NewComposer(),
		Dialog:     &widgets.Dialog{},
		Keyring:    st.Keyring(),
		PickFile:   pickFile,
		Invalidate: w.Invalidate,
	}
	env.LockSetup = screens.NewLock(screens.LockIntentEnroll)
	// Live key status for the composer's security readout.
	env.Composer.HasKey = func(addr string) bool { return st.Keyring().HasKeyFor(addr) }

	// VayuTalk: the ephemeral-chat holder shares the keyring (for E2E) and
	// the keystore (for the on-demand mailbox credential) with the mail
	// engine. It stays idle until the Talk screen binds it to an account.
	chatState := state.NewChatState(db, st.Keyring(), ks, w.Invalidate, st.Notify)
	st.Chat = chatState

	ui := &UI{
		window:     w,
		th:         th,
		st:         st,
		nav:        env.Nav,
		env:        env,
		inbox:      screens.NewInbox(),
		thread:     screens.NewThread(),
		compose:    screens.NewCompose(),
		setup:      screens.NewAccountSetup(),
		settings:   screens.NewSettings(),
		search:     screens.NewSearch(),
		lockSetup:  env.LockSetup,
		lockGate:   screens.NewLock(screens.LockIntentUnlock),
		talk:       screens.NewTalk(),
		talkRoom:   screens.NewTalkRoom(),
		talkVerify: screens.NewTalkVerify(),
		events:     mgr.Events(),
		notify:     newMailNotifier(ctx, db),
	}
	ui.notify.enabled = st.NotificationsEnabled
	ui.notify.preview = st.NotifyPreviewEnabled
	// Incoming VayuTalk messages post a content-free notification (privacy:
	// never the sender or the text), gated by the notifications setting.
	chatState.OnIncoming = ui.notify.notifyChat
	st.Refresh() // SQLite on first paint: cached mail renders immediately.
	return ui
}

// Frame renders one UI frame into the boot loop's context (Section 3 of
// the architecture): it drains sync events non-blockingly, applies the
// idle auto-lock, then draws.
func (ui *UI) Frame(gtx layout.Context) {
	// A long gap between frames means the app was backgrounded or the
	// device idle — re-arm the lock before drawing anything.
	if !ui.lastFrame.IsZero() {
		ui.st.MaybeAutoLock(gtx.Now.Sub(ui.lastFrame))
	}
	ui.lastFrame = gtx.Now

	// Drain eventCh non-blockingly before drawing.
drain:
	for {
		select {
		case ev := <-ui.events:
			ui.st.Apply(ev)
			ui.notify.observe(ev)
		default:
			break drain
		}
	}
	ui.layout(gtx)
}

// layout routes to the active screen, running the push/pop slide
// transitions, then stacks the lock gate, dialog, and snackbar overlays.
func (ui *UI) layout(gtx layout.Context) {
	widgets.FillMax(gtx, ui.th.Palette.Background)

	snap := ui.st.Snapshot()
	if snap.Locked {
		// The gate replaces the whole frame: no mail pixels render while
		// locked, so app switchers screenshot nothing sensitive.
		ui.lockGate.Layout(gtx, ui.env)
		return
	}

	ui.handleBackKey(gtx)
	current := ui.nav.Current()

	// Force onboarding until an account exists; release the forced setup
	// root once one does. A setup screen pushed from Settings/drawer
	// (depth > 1) is user navigation and stays.
	if len(snap.Accounts) == 0 && current != state.ScreenSetup {
		ui.nav.Replace(state.ScreenSetup)
		current = state.ScreenSetup
	}
	if len(snap.Accounts) > 0 && current == state.ScreenSetup && ui.nav.Depth() == 1 {
		ui.nav.Replace(state.ScreenInbox)
		current = state.ScreenInbox
	}

	entering, leaving, progress, done := ui.nav.Transition(gtx.Now)
	width := gtx.Constraints.Max.X

	switch {
	case done:
		ui.layoutScreen(gtx, current)
	case entering:
		// Push: the parent recedes with a slight parallax and dim while
		// the new screen slides in from the right.
		offset := int(float32(width) * (1 - progress))
		ui.drawParallaxUnder(gtx, progress, func(gtx layout.Context) { ui.layoutScreen(gtx, ui.nav.Under()) })
		ui.drawOffset(gtx, offset, func(gtx layout.Context) { ui.layoutScreen(gtx, current) })
		gtx.Execute(op.InvalidateCmd{})
	default:
		// Pop: the old screen slides out to the right, revealing the new
		// top as it returns from its receded position.
		offset := int(float32(width) * progress)
		ui.drawParallaxUnder(gtx, 1-progress, func(gtx layout.Context) { ui.layoutScreen(gtx, current) })
		ui.drawOffset(gtx, offset, func(gtx layout.Context) { ui.layoutScreen(gtx, leaving) })
		gtx.Execute(op.InvalidateCmd{})
	}

	// Dialog and snackbar stack above every screen.
	overlayGtx := gtx
	overlayGtx.Constraints.Min = gtx.Constraints.Max
	ui.env.Dialog.Layout(overlayGtx, ui.th)
	ui.env.Snack.Layout(overlayGtx, ui.th)
}

// handleBackKey maps the Android back button (and Escape on desktop)
// onto the navigation stack: dialog first, then the drawer, then pop; at
// the stack root it closes the window, matching platform convention.
func (ui *UI) handleBackKey(gtx layout.Context) {
	for {
		ev, ok := gtx.Event(
			key.Filter{Name: key.NameBack},
			key.Filter{Name: key.NameEscape})
		if !ok {
			break
		}
		e, ok := ev.(key.Event)
		if !ok || e.State != key.Press {
			continue
		}
		if ui.env.Dialog.Visible() {
			ui.env.Dialog.Dismiss()
			continue
		}
		if ui.inbox.CloseDrawer(gtx.Now) {
			continue
		}
		if !ui.nav.Pop(gtx.Now) {
			ui.window.Perform(system.ActionClose)
		}
	}
}

func (ui *UI) drawOffset(gtx layout.Context, x int, f func(layout.Context)) {
	defer op.Offset(image.Pt(x, 0)).Push(gtx.Ops).Pop()
	f(gtx)
}

// drawParallaxUnder draws the screen beneath a transition, shifted left
// by up to 18% and dimmed as the top screen covers it (t = coverage).
func (ui *UI) drawParallaxUnder(gtx layout.Context, t float32, f func(layout.Context)) {
	shift := -int(0.18 * t * float32(gtx.Constraints.Max.X))
	func() {
		defer op.Offset(image.Pt(shift, 0)).Push(gtx.Ops).Pop()
		f(gtx)
	}()
	if t > 0 {
		paint.FillShape(gtx.Ops,
			theme.WithAlpha(ui.th.Palette.Shadow, uint8(t*float32(ui.th.Palette.Shadow.A))),
			clip.Rect{Max: gtx.Constraints.Max}.Op())
	}
}

// layoutScreen draws one screen by ID.
func (ui *UI) layoutScreen(gtx layout.Context, s state.Screen) {
	gtx.Constraints.Min = gtx.Constraints.Max
	switch s {
	case state.ScreenThread:
		ui.thread.Layout(gtx, ui.env)
	case state.ScreenCompose:
		ui.compose.Layout(gtx, ui.env)
	case state.ScreenSetup:
		ui.setup.Layout(gtx, ui.env)
	case state.ScreenSettings:
		ui.settings.Layout(gtx, ui.env)
	case state.ScreenSearch:
		ui.search.Layout(gtx, ui.env)
	case state.ScreenLock:
		ui.lockSetup.Layout(gtx, ui.env)
	case state.ScreenTalk:
		ui.talk.Layout(gtx, ui.env)
	case state.ScreenTalkRoom:
		ui.talkRoom.Layout(gtx, ui.env)
	case state.ScreenTalkVerify:
		ui.talkVerify.Layout(gtx, ui.env)
	default:
		ui.inbox.Layout(gtx, ui.env)
	}
}
