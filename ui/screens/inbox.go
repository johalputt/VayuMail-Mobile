package screens

import (
	"time"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/johalputt/VayuMail-Mobile/internal/store"
	"github.com/johalputt/VayuMail-Mobile/internal/syncmanager"
	"github.com/johalputt/VayuMail-Mobile/ui/state"
	"github.com/johalputt/VayuMail-Mobile/ui/theme"
	"github.com/johalputt/VayuMail-Mobile/ui/widgets"
)

// Inbox is the primary view: the virtualized message list with swipe
// actions, the folder drawer, and the top bar.
type Inbox struct {
	list   *widgets.MessageList
	drawer *widgets.FolderDrawer

	menuBtn    widget.Clickable
	searchBtn  widget.Clickable
	composeBtn widget.Clickable

	// pendingHidden holds message IDs swiped away but not yet committed;
	// they are hidden immediately and restored on undo.
	pendingHidden map[int64]bool
}

// NewInbox constructs the inbox screen.
func NewInbox() *Inbox {
	return &Inbox{
		list:          widgets.NewMessageList(),
		drawer:        widgets.NewFolderDrawer(),
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
	if s.composeBtn.Clicked(gtx) {
		env.Composer.Reset()
		env.Nav.Push(state.ScreenCompose, gtx.Now)
	}

	title := snap.CurrentFolder.Name
	if title == "" {
		title = "Inbox"
	}

	visible := s.visibleMessages(snap.Messages)

	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return topBar(gtx, th,
				iconBtn(th, &s.menuBtn, widgets.IconMenu, th.Palette.OnBackground),
				title,
				iconBtn(th, &s.searchBtn, widgets.IconSearch, th.Palette.OnBackground),
				iconBtn(th, &s.composeBtn, widgets.IconCompose, th.Palette.OnBackground),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if snap.Online {
				return layout.Dimensions{}
			}
			return layout.Inset{Left: theme.LG, Top: theme.XS, Bottom: theme.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return th.Label(gtx, theme.Caption, th.Palette.Subtle, "Offline — showing cached mail", 1)
				})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if len(visible) == 0 {
				return emptyState(gtx, th, widgets.IconEnvelope, true,
					"All clear.", "New messages will appear here.")
			}
			for _, action := range s.list.Layout(gtx, th, visible) {
				s.handleAction(gtx, env, snap, action)
			}
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}))

	// Drawer stacks above everything.
	drawerGtx := gtx
	drawerGtx.Constraints = layout.Exact(gtx.Constraints.Max)
	if a := s.drawer.Layout(drawerGtx, th, snap.Folders, snap.Unread, snap.CurrentFolder.ID); a.FolderID != 0 {
		env.State.SelectFolder(a.FolderID)
	} else if a.Settings {
		env.Nav.Push(state.ScreenSettings, gtx.Now)
	}
	return dims
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
