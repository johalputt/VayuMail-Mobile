// Package ui is the Gio root: window, theme, navigation router, and the
// single-threaded event loop. This is the only layer (besides platform/)
// allowed to import Gio (Constitutional Rule 4).
package ui

import (
	"context"
	"image"
	"log/slog"

	"gioui.org/app"
	"gioui.org/io/key"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	xtheme "gioui.org/x/pref/theme"

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

	inbox    *screens.Inbox
	thread   *screens.Thread
	compose  *screens.Compose
	setup    *screens.AccountSetup
	settings *screens.Settings
	search   *screens.Search

	events <-chan syncmanager.Event
	notify *mailNotifier
	ops    op.Ops
}

// New wires the UI over an already-started sync manager.
func New(ctx context.Context, w *app.Window, db *store.DB, mgr *syncmanager.Manager) *UI {
	dark, err := xtheme.IsDarkMode()
	if err != nil {
		slog.Debug("dark mode preference unavailable", "err", err)
	}
	th := theme.New(dark)
	st := state.New(ctx, db, mgr)
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
		Theme:    th,
		State:    st,
		Nav:      state.NewNav(state.ScreenInbox),
		Snack:    snack,
		Composer: widgets.NewComposer(),
		Keyring:  st.Keyring(),
	}

	ui := &UI{
		window:   w,
		th:       th,
		st:       st,
		nav:      env.Nav,
		env:      env,
		inbox:    screens.NewInbox(),
		thread:   screens.NewThread(),
		compose:  screens.NewCompose(),
		setup:    screens.NewAccountSetup(nil), // camera bridge: see COMPLIANCE-TRACKER.md
		settings: screens.NewSettings(),
		search:   screens.NewSearch(),
		events:   mgr.Events(),
		notify:   newMailNotifier(ctx, db),
	}
	st.Refresh() // SQLite on first paint: cached mail renders immediately.
	return ui
}

// Run is the Gio event loop (Section 3 of the architecture): drain sync
// events non-blockingly before every frame, draw, repeat. It returns when
// the window is destroyed.
func (ui *UI) Run() error {
	for {
		switch e := ui.window.Event().(type) {
		case app.FrameEvent:
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
			gtx := app.NewContext(&ui.ops, e)
			ui.layout(gtx)
			e.Frame(&ui.ops)

		case app.DestroyEvent:
			return e.Err
		}
	}
}

// layout routes to the active screen, running the push/pop slide
// transitions.
func (ui *UI) layout(gtx layout.Context) {
	widgets.FillMax(gtx, ui.th.Palette.Background)
	ui.handleBackKey(gtx)

	snap := ui.st.Snapshot()
	current := ui.nav.Current()

	// Force onboarding until an account exists.
	if len(snap.Accounts) == 0 && current != state.ScreenSetup {
		ui.nav.Replace(state.ScreenSetup)
		current = state.ScreenSetup
	}
	if len(snap.Accounts) > 0 && current == state.ScreenSetup {
		ui.nav.Replace(state.ScreenInbox)
		current = state.ScreenInbox
	}

	entering, leaving, progress, done := ui.nav.Transition(gtx.Now)
	width := gtx.Constraints.Max.X

	switch {
	case done:
		ui.layoutScreen(gtx, current)
	case entering:
		// Push: the new screen slides in from the right over its parent.
		offset := int(float32(width) * (1 - progress))
		ui.drawOffset(gtx, 0, func(gtx layout.Context) { ui.layoutScreen(gtx, ui.nav.Under()) })
		ui.drawOffset(gtx, offset, func(gtx layout.Context) { ui.layoutScreen(gtx, current) })
		gtx.Execute(op.InvalidateCmd{})
	default:
		// Pop: the old screen slides out to the right, revealing the new top.
		offset := int(float32(width) * progress)
		ui.drawOffset(gtx, 0, func(gtx layout.Context) { ui.layoutScreen(gtx, current) })
		ui.drawOffset(gtx, offset, func(gtx layout.Context) { ui.layoutScreen(gtx, leaving) })
		gtx.Execute(op.InvalidateCmd{})
	}

	// Snackbar stacks above every screen.
	snackGtx := gtx
	snackGtx.Constraints.Min = gtx.Constraints.Max
	ui.env.Snack.Layout(snackGtx, ui.th)
}

// handleBackKey maps the Android back button (and Escape on desktop)
// onto the navigation stack: close the drawer first, then pop; at the
// stack root it closes the window, matching platform convention.
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
	default:
		ui.inbox.Layout(gtx, ui.env)
	}
}
