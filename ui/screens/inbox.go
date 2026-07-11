package screens

import (
	"fmt"
	"image"
	"time"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/johalputt/VayuMail-Mobile/internal/store"
	"github.com/johalputt/VayuMail-Mobile/internal/syncmanager"
	"github.com/johalputt/VayuMail-Mobile/ui/state"
	"github.com/johalputt/VayuMail-Mobile/ui/theme"
	"github.com/johalputt/VayuMail-Mobile/ui/widgets"
)

// Inbox is the primary view: the virtualized message list with swipe
// actions, the folder drawer with the account switcher, the sync
// progress hairline, and the floating compose button.
type Inbox struct {
	list    *widgets.MessageList
	drawer  *widgets.FolderDrawer
	fab     *widgets.FAB
	syncBar widgets.SyncBar
	pull    widgets.PullRefresh

	menuBtn    widget.Clickable
	searchBtn  widget.Clickable
	refreshBtn widget.Clickable

	// pendingHidden holds message IDs swiped away but not yet committed;
	// they are hidden immediately and restored on undo.
	pendingHidden map[int64]bool
}

// NewInbox constructs the inbox screen.
func NewInbox() *Inbox {
	return &Inbox{
		list:          widgets.NewMessageList(),
		drawer:        widgets.NewFolderDrawer(),
		fab:           &widgets.FAB{},
		pendingHidden: make(map[int64]bool),
	}
}

// CloseDrawer closes the folder drawer if it is open, reporting whether
// it consumed the action — the first stop for the platform back button.
func (s *Inbox) CloseDrawer(now time.Time) bool {
	if s.drawer.IsOpen() {
		s.drawer.Close(now)
		return true
	}
	return false
}

// Layout renders the inbox.
func (s *Inbox) Layout(gtx layout.Context, env *Env) layout.Dimensions {
	th := env.Theme
	snap := env.State.Snapshot()

	if s.menuBtn.Clicked(gtx) {
		s.drawer.Open(gtx.Now)
	}
	if s.searchBtn.Clicked(gtx) {
		env.Nav.Push(state.ScreenSearch, gtx.Now)
	}
	if s.fab.Clicked(gtx) {
		env.Composer.Reset()
		env.Nav.Push(state.ScreenCompose, gtx.Now)
	}
	if s.refreshBtn.Clicked(gtx) {
		env.State.SyncNow()
		env.Snack.ShowInfo("Syncing…")
	}

	title := snap.CurrentFolder.Name
	if title == "" {
		title = "Inbox"
	}
	if n := snap.Unread[snap.CurrentFolder.ID]; n > 0 {
		title = fmt.Sprintf("%s · %d", title, n)
	}

	// Arm the entrance cascade when the folder identity changes.
	s.list.BeginEntrance(fmt.Sprintf("%d/%d", snap.SelectedAccount, snap.CurrentFolder.ID), gtx.Now)

	visible := s.visibleMessages(snap.Messages)
	syncing := snap.ManualSyncing ||
		(snap.SyncTotal > 0 && snap.SyncDone < snap.SyncTotal)

	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return topBar(gtx, th,
				iconBtn(th, &s.menuBtn, widgets.IconMenu, th.Palette.OnBackground),
				title,
				s.refreshIcon(th, syncing),
				iconBtn(th, &s.searchBtn, widgets.IconSearch, th.Palette.OnBackground),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.syncBar.Layout(gtx, th, snap.SyncDone, snap.SyncTotal)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return statusBanner(gtx, th, snap)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			triggered, dims := s.pull.Layout(gtx, th, s.list.AtTop(), syncing,
				func(gtx layout.Context) layout.Dimensions {
					if len(visible) == 0 {
						return emptyState(gtx, th, widgets.IconEnvelope, true,
							"All clear.", "New messages will appear here.")
					}
					for _, action := range s.list.Layout(gtx, th, visible) {
						s.handleAction(gtx, env, snap, action)
					}
					return layout.Dimensions{Size: gtx.Constraints.Max}
				})
			if triggered {
				env.State.SyncNow()
			}
			return dims
		}))

	// FAB floats bottom-right above the list.
	layout.SE.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(theme.FABMargin).Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				return s.fab.Layout(gtx, th, widgets.IconCompose)
			})
	})

	// Drawer stacks above everything.
	drawerGtx := gtx
	drawerGtx.Constraints = layout.Exact(gtx.Constraints.Max)
	a := s.drawer.Layout(drawerGtx, th, snap.Accounts, snap.SelectedAccount,
		snap.Folders, snap.Unread, snap.CurrentFolder.ID)
	switch {
	case a.FolderID != 0:
		env.State.SelectFolder(a.FolderID)
	case a.AccountID != 0:
		env.State.SelectAccount(a.AccountID)
	case a.AddAccount:
		env.Nav.Push(state.ScreenSetup, gtx.Now)
	case a.Settings:
		env.Nav.Push(state.ScreenSettings, gtx.Now)
	case a.Talk:
		if c := env.State.Chat; c != nil {
			if acct, ok := env.State.CurrentAccount(); ok {
				c.EnsureStarted(acct)
			}
		}
		env.Nav.Push(state.ScreenTalk, gtx.Now)
	}
	return dims
}

// refreshIcon spins while a counted sync is in flight — the visible
// "quick sync" affordance. The rotation runs only during the sync.
func (s *Inbox) refreshIcon(th *theme.Theme, syncing bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if !syncing {
			return widgets.IconButton(gtx, th, &s.refreshBtn, widgets.IconRefresh, th.Palette.OnBackground)
		}
		gtx.Execute(op.InvalidateCmd{})
		angle := float32(gtx.Now.UnixMilli()%1200) / 1200 * 2 * 3.14159
		return s.refreshBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			target := gtx.Dp(theme.TouchTarget)
			gtx.Constraints = layout.Exact(image.Pt(target, target))
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				macro := op.Record(gtx.Ops)
				dims := widgets.DrawIcon(gtx, widgets.IconRefresh, th.Palette.Accent, 24)
				call := macro.Stop()
				origin := f32.Pt(float32(dims.Size.X)/2, float32(dims.Size.Y)/2)
				defer op.Affine(f32.Affine2D{}.Rotate(origin, angle)).Push(gtx.Ops).Pop()
				call.Add(gtx.Ops)
				return dims
			})
		})
	}
}

// statusBanner surfaces offline and auth-failure states inline.
func statusBanner(gtx layout.Context, th *theme.Theme, snap state.Snapshot) layout.Dimensions {
	msg := ""
	c := th.Palette.Subtle
	switch {
	case snap.AuthError:
		msg = "Sign-in failed — check this account's password in Settings"
		c = th.Palette.Warning
	case !snap.Online:
		msg = "Offline — showing cached mail"
	}
	if msg == "" {
		return layout.Dimensions{}
	}
	return layout.Inset{Left: theme.LG, Top: theme.XS, Bottom: theme.XS}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return th.Label(gtx, theme.Caption, c, msg, 1)
		})
}

// visibleMessages filters out rows hidden by an uncommitted swipe.
func (s *Inbox) visibleMessages(msgs []store.Message) []store.Message {
	if len(s.pendingHidden) == 0 {
		return msgs
	}
	out := make([]store.Message, 0, len(msgs))
	for _, m := range msgs {
		if !s.pendingHidden[m.ID] {
			out = append(out, m)
		}
	}
	return out
}

// handleAction reacts to a row interaction: open navigates, archive and
// delete hide the row and arm the snackbar's undo/commit pair.
func (s *Inbox) handleAction(gtx layout.Context, env *Env, snap state.Snapshot, action widgets.ListAction) {
	msg := action.Message
	switch action.Kind {
	case widgets.ActionOpen:
		if !msg.IsRead {
			env.State.Send(syncmanager.MarkCmd{MessageID: msg.ID, Flag: `\Seen`, Set: true})
		}
		env.State.OpenThread(msg.ThreadID)
		env.Nav.Push(state.ScreenThread, gtx.Now)

	case widgets.ActionArchive:
		archive := findFolder(snap.Folders, func(f store.Folder) bool { return f.IsArchive })
		if archive == nil {
			env.Snack.ShowInfo("No archive folder on this account")
			return
		}
		s.armUndo(env, msg.ID, "Archived",
			syncmanager.MoveCmd{MessageID: msg.ID, DestFolder: archive.FullName})

	case widgets.ActionDelete:
		s.armUndo(env, msg.ID, "Deleted",
			syncmanager.DeleteCmd{MessageID: msg.ID})
	}
}

// armUndo hides the row now and commits the command only when the
// snackbar expires; undo simply unhides.
func (s *Inbox) armUndo(env *Env, msgID int64, label string, cmd syncmanager.Cmd) {
	s.pendingHidden[msgID] = true
	env.Snack.Show(label,
		func() { // undo
			delete(s.pendingHidden, msgID)
			env.State.Refresh()
		},
		func() { // commit
			delete(s.pendingHidden, msgID)
			env.State.Send(cmd)
		})
}

func findFolder(folders []store.Folder, match func(store.Folder) bool) *store.Folder {
	for i := range folders {
		if match(folders[i]) {
			return &folders[i]
		}
	}
	return nil
}
