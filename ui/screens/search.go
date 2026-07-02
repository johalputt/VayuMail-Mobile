package screens

import (
	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/johalputt/VayuMail-Mobile/internal/syncmanager"
	"github.com/johalputt/VayuMail-Mobile/ui/state"
	"github.com/johalputt/VayuMail-Mobile/ui/widgets"
)

// Search is the full-screen FTS5 search: a bar and a result list.
type Search struct {
	bar     *widgets.SearchBar
	results *widgets.MessageList
	backBtn widget.Clickable
}

// NewSearch constructs the search screen.
func NewSearch() *Search {
	results := widgets.NewMessageList()
	results.Swipe = false
	return &Search{
		bar:     widgets.NewSearchBar(),
		results: results,
	}
}

// Layout renders the search screen.
func (s *Search) Layout(gtx layout.Context, env *Env) layout.Dimensions {
	th := env.Theme
	snap := env.State.Snapshot()

	if s.backBtn.Clicked(gtx) {
		s.bar.Clear()
		env.State.SetSearch("")
		env.Nav.Pop(gtx.Now)
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return widgets.IconButton(gtx, th, &s.backBtn, widgets.IconBack, th.Palette.OnBackground)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					changed, dims := s.bar.Layout(gtx, th)
					if changed {
						env.State.SetSearch(s.bar.Query())
					}
					return dims
				}))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return widgets.Separator(gtx, th, 0)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if s.bar.Query() == "" {
				return layout.Dimensions{Size: gtx.Constraints.Max}
			}
			if len(snap.SearchResults) == 0 {
				// The message is complete: no secondary text.
				return emptyState(gtx, th, 0, false, "Nothing found.", "")
			}
			for _, action := range s.results.Layout(gtx, th, snap.SearchResults) {
				if action.Kind == widgets.ActionOpen {
					msg := action.Message
					if !msg.IsRead {
						env.State.Send(syncmanager.MarkCmd{MessageID: msg.ID, Flag: `\Seen`, Set: true})
					}
					env.State.OpenThread(msg.ThreadID)
					env.Nav.Push(state.ScreenThread, gtx.Now)
				}
			}
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}))
}
