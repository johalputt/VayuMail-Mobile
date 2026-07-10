package screens

import (
	"io"
	"strings"

	"gioui.org/io/clipboard"
	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/johalputt/VayuMail-Mobile/internal/store"
	"github.com/johalputt/VayuMail-Mobile/internal/syncmanager"
	"github.com/johalputt/VayuMail-Mobile/ui/state"
	"github.com/johalputt/VayuMail-Mobile/ui/theme"
	"github.com/johalputt/VayuMail-Mobile/ui/widgets"
)

// Thread shows one conversation with quoted-text folding, inline PGP
// status, tracking indicators, attachment downloads, snooze, one-tap
// unsubscribe, and a bottom action bar: reply, forward, archive, delete.
type Thread struct {
	view      *widgets.ThreadView
	backBtn   widget.Clickable
	snoozeBtn widget.Clickable
	unsubBtn  widget.Clickable

	replyBtn   widget.Clickable
	forwardBtn widget.Clickable
	archiveBtn widget.Clickable
	deleteBtn  widget.Clickable
}

// NewThread constructs the thread screen.
func NewThread() *Thread {
	return &Thread{view: widgets.NewThreadView()}
}

// Layout renders the conversation.
func (s *Thread) Layout(gtx layout.Context, env *Env) layout.Dimensions {
	th := env.Theme
	snap := env.State.Snapshot()

	var latest *store.Message
	if len(snap.Thread) > 0 {
		latest = &snap.Thread[len(snap.Thread)-1]
	}

	if s.backBtn.Clicked(gtx) {
		env.Nav.Pop(gtx.Now)
	}
	s.handleActions(gtx, env, snap, latest)

	title := "Conversation"
	if len(snap.Thread) > 0 {
		if subj := snap.Thread[0].Subject; subj != "" {
			title = subj
		}
	}

	trailing := []layout.Widget{}
	if latest != nil {
		if latest.IsList && latest.ListUnsubscribe != "" {
			trailing = append(trailing,
				iconBtn(th, &s.unsubBtn, widgets.IconEnvelope, th.Palette.Subtle))
		}
		trailing = append(trailing,
			iconBtn(th, &s.snoozeBtn, widgets.IconClock, th.Palette.OnBackground))
	}

	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return topBar(gtx, th,
				iconBtn(th, &s.backBtn, widgets.IconBack, th.Palette.OnBackground),
				title, trailing...)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if len(snap.Thread) == 0 {
				return emptyState(gtx, th, 0, false, "Nothing here.", "")
			}
			return s.view.Layout(gtx, th, snap.Thread)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if latest == nil {
				return layout.Dimensions{}
			}
			return s.actionBar(gtx, th)
		}))

	// Attachment downloads requested inside the view this frame.
	for _, req := range s.view.DownloadRequests() {
		env.State.DownloadAttachment(req.MessageID, req.Index)
	}
	return dims
}

// actionBar is the thumb-reachable bottom row of message actions.
func (s *Thread) actionBar(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	p := th.Palette
	item := func(click *widget.Clickable, icon widgets.Icon, label string, c ...bool) layout.FlexChild {
		destructive := len(c) > 0 && c[0]
		col := p.OnSurface
		if destructive {
			col = p.Destructive
		}
		return layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: theme.SM, Bottom: theme.SM + theme.XS}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								gtx.Constraints.Min.X = gtx.Constraints.Max.X
								return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return widgets.DrawIcon(gtx, icon, col, 22)
								})
							}),
							layout.Rigid(layout.Spacer{Height: theme.XS}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								gtx.Constraints.Min.X = gtx.Constraints.Max.X
								return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return th.Label(gtx, theme.Micro, col, label, 1)
								})
							}))
					})
			})
		})
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return widgets.Separator(gtx, th, 0)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				item(&s.replyBtn, widgets.IconReply, "Reply"),
				item(&s.forwardBtn, widgets.IconForward, "Forward"),
				item(&s.archiveBtn, widgets.IconArchive, "Archive"),
				item(&s.deleteBtn, widgets.IconTrash, "Delete", true))
		}))
}

// handleActions reacts to top-bar and action-bar taps.
func (s *Thread) handleActions(gtx layout.Context, env *Env, snap state.Snapshot, latest *store.Message) {
	if latest == nil {
		return
	}
	if s.replyBtn.Clicked(gtx) {
		env.Composer.PrefillReply(latest.FromAddr, latest.Subject)
		env.Nav.Push(state.ScreenCompose, gtx.Now)
	}
	if s.forwardBtn.Clicked(gtx) {
		env.Composer.PrefillForward(latest.Subject, latest.FromName, latest.FromAddr,
			latest.Date.Format("2 Jan 2006 15:04"), latest.BodyText)
		env.Nav.Push(state.ScreenCompose, gtx.Now)
	}
	if s.archiveBtn.Clicked(gtx) {
		archive := findFolder(snap.Folders, func(f store.Folder) bool { return f.IsArchive })
		if archive == nil {
			env.Snack.ShowInfo("No archive folder on this account")
		} else {
			env.State.Send(syncmanager.MoveCmd{MessageID: latest.ID, DestFolder: archive.FullName})
			env.Snack.ShowInfo("Archived")
			env.Nav.Pop(gtx.Now)
		}
	}
	if s.deleteBtn.Clicked(gtx) {
		id := latest.ID
		env.Dialog.Show(gtx.Now, "Delete this message?",
			"It moves to Trash on the server as well.", "Delete", true, func() {
				env.State.Send(syncmanager.DeleteCmd{MessageID: id})
			})
	}
	if s.snoozeBtn.Clicked(gtx) {
		env.State.Snooze(*latest)
		env.Nav.Pop(gtx.Now)
	}
	if s.unsubBtn.Clicked(gtx) {
		if url := env.State.Unsubscribe(*latest); url != "" {
			// HTTPS-only unsubscribe: put the link on the clipboard for
			// the user's browser.
			gtx.Execute(clipboard.WriteCmd{Type: "text/plain",
				Data: io.NopCloser(strings.NewReader(url))})
			env.Snack.ShowInfo("Unsubscribe link copied")
		}
	}
}
