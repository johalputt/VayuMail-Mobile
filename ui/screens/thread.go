package screens

import (
	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/johalputt/VayuMail-Mobile/ui/state"
	"github.com/johalputt/VayuMail-Mobile/ui/widgets"
)

// Thread shows one conversation with quoted-text folding and inline PGP
// status.
type Thread struct {
	view     *widgets.ThreadView
	backBtn  widget.Clickable
	replyBtn widget.Clickable
}

// NewThread constructs the thread screen.
func NewThread() *Thread {
	return &Thread{view: widgets.NewThreadView()}
}

// Layout renders the conversation.
func (s *Thread) Layout(gtx layout.Context, env *Env) layout.Dimensions {
	th := env.Theme
	snap := env.State.Snapshot()

	if s.backBtn.Clicked(gtx) {
		env.Nav.Pop(gtx.Now)
	}
	if s.replyBtn.Clicked(gtx) && len(snap.Thread) > 0 {
		last := snap.Thread[len(snap.Thread)-1]
		env.Composer.PrefillReply(last.FromAddr, last.Subject)
		env.Nav.Push(state.ScreenCompose, gtx.Now)
	}

	title := "Conversation"
	if len(snap.Thread) > 0 {
		if subj := snap.Thread[0].Subject; subj != "" {
			title = subj
		}
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return topBar(gtx, th,
				iconBtn(th, &s.backBtn, widgets.IconBack, th.Palette.OnBackground),
				title,
				iconBtn(th, &s.replyBtn, widgets.IconCompose, th.Palette.OnBackground),
			)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if len(snap.Thread) == 0 {
				return emptyState(gtx, th, 0, false, "Nothing here.", "")
			}
			return s.view.Layout(gtx, th, snap.Thread)
		}))
}
