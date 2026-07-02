package screens

import (
	"io"
	"strings"

	"gioui.org/io/clipboard"
	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/johalputt/VayuMail-Mobile/internal/store"
	"github.com/johalputt/VayuMail-Mobile/ui/state"
	"github.com/johalputt/VayuMail-Mobile/ui/widgets"
)

// Thread shows one conversation with quoted-text folding, inline PGP
// status, tracking indicators, attachment downloads, snooze, and
// one-tap unsubscribe for list mail.
type Thread struct {
	view      *widgets.ThreadView
	backBtn   widget.Clickable
	replyBtn  widget.Clickable
	snoozeBtn widget.Clickable
	unsubBtn  widget.Clickable
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
	if s.replyBtn.Clicked(gtx) && latest != nil {
		env.Composer.PrefillReply(latest.FromAddr, latest.Subject)
		env.Nav.Push(state.ScreenCompose, gtx.Now)
	}
	if s.snoozeBtn.Clicked(gtx) && latest != nil {
		env.State.Snooze(*latest)
		env.Nav.Pop(gtx.Now)
	}
	if s.unsubBtn.Clicked(gtx) && latest != nil {
		if url := env.State.Unsubscribe(*latest); url != "" {
			// HTTPS-only unsubscribe: put the link on the clipboard for
			// the user's browser.
			gtx.Execute(clipboard.WriteCmd{Type: "text/plain",
				Data: io.NopCloser(strings.NewReader(url))})
			env.Snack.ShowInfo("Unsubscribe link copied")
		}
	}

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
	trailing = append(trailing,
		iconBtn(th, &s.replyBtn, widgets.IconCompose, th.Palette.OnBackground))

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
		}))

	// Attachment downloads requested inside the view this frame.
	for _, req := range s.view.DownloadRequests() {
		env.State.DownloadAttachment(req.MessageID, req.Index)
	}
	return dims
}
