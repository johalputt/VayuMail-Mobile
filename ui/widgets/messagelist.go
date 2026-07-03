package widgets

import (
	"fmt"
	"image"
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/widget"

	"github.com/johalputt/VayuMail-Mobile/internal/store"
	"github.com/johalputt/VayuMail-Mobile/ui/theme"
)

// ListActionKind describes what the user did to a row.
type ListActionKind int

// Row actions.
const (
	// ActionOpen: the row was tapped.
	ActionOpen ListActionKind = iota
	// ActionArchive: right swipe past threshold.
	ActionArchive
	// ActionDelete: left swipe past threshold.
	ActionDelete
)

// ListAction is one user interaction with a message row this frame.
type ListAction struct {
	Kind    ListActionKind
	Message store.Message
}

// MessageList is the virtualized inbox list: fixed 72pt rows, only the
// visible range plus a small buffer is laid out (layout.List computes the
// visible window from the scroll offset; off-screen rows are never
// measured or drawn).
type MessageList struct {
	list layout.List
	rows []rowState
	// Swipe enables archive/delete gestures (disabled in search results).
	Swipe bool
}

type rowState struct {
	click widget.Clickable
	swipe Swipeable
}

// NewMessageList constructs a vertical list with swipe enabled.
func NewMessageList() *MessageList {
	return &MessageList{
		list:  layout.List{Axis: layout.Vertical},
		Swipe: true,
	}
}

// Layout renders the list and returns any actions performed this frame.
func (ml *MessageList) Layout(gtx layout.Context, th *theme.Theme, msgs []store.Message) []ListAction {
	if len(ml.rows) < len(msgs) {
		ml.rows = append(ml.rows, make([]rowState, len(msgs)-len(ml.rows))...)
	}
	var actions []ListAction

	ml.list.Layout(gtx, len(msgs), func(gtx layout.Context, i int) layout.Dimensions {
		msg := msgs[i]
		row := &ml.rows[i]

		if row.click.Clicked(gtx) {
			actions = append(actions, ListAction{Kind: ActionOpen, Message: msg})
		}

		rowWidget := func(gtx layout.Context) layout.Dimensions {
			return row.click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return messageRow(gtx, th, msg)
			})
		}

		var dims layout.Dimensions
		if ml.Swipe {
			var result SwipeResult
			result, dims = row.swipe.Layout(gtx, th, rowWidget)
			switch result {
			case SwipeArchive:
				actions = append(actions, ListAction{Kind: ActionArchive, Message: msg})
			case SwipeDelete:
				actions = append(actions, ListAction{Kind: ActionDelete, Message: msg})
			case SwipeTap:
				// The drag gesture consumes taps before the row's Clickable,
				// so a tap arrives here as SwipeTap — treat it as open.
				actions = append(actions, ListAction{Kind: ActionOpen, Message: msg})
			}
		} else {
			dims = rowWidget(gtx)
		}

		sepGtx := gtx
		sepGtx.Constraints.Min = image.Pt(gtx.Constraints.Max.X, 0)
		sep := Separator(sepGtx, th, theme.SeparatorInset)
		return layout.Dimensions{Size: image.Pt(dims.Size.X, dims.Size.Y+sep.Size.Y)}
	})
	return actions
}

// messageRow draws one fixed-height row:
//
//	[16] [Avatar 40] [8] [ sender ... time / subject ... dot ] [16]
func messageRow(gtx layout.Context, th *theme.Theme, msg store.Message) layout.Dimensions {
	height := gtx.Dp(theme.RowHeight)
	gtx.Constraints = layout.Exact(image.Pt(gtx.Constraints.Max.X, height))

	return layout.Inset{Left: theme.MD, Right: theme.MD}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return Avatar(gtx, th, msg.FromName, msg.FromAddr)
				}),
				layout.Rigid(layout.Spacer{Width: theme.SM}.Layout),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return rowLine1(gtx, th, msg)
						}),
						layout.Rigid(layout.Spacer{Height: theme.XS}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return rowLine2(gtx, th, msg)
						}))
				}))
		})
}

func rowLine1(gtx layout.Context, th *theme.Theme, msg store.Message) layout.Dimensions {
	c := th.Palette.OnSurface
	if !msg.IsRead {
		c = th.Palette.OnBackground
	}
	sender := msg.FromName
	if sender == "" {
		sender = msg.FromAddr
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Baseline, Spacing: layout.SpaceBetween}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return th.Label(gtx, theme.BodyStrong, c, sender, 1)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return th.Label(gtx, theme.Caption, th.Palette.Subtle, RelativeTime(msg.Date, time.Now()), 1)
		}))
}

func rowLine2(gtx layout.Context, th *theme.Theme, msg store.Message) layout.Dimensions {
	subject := msg.Subject
	if subject == "" {
		subject = "(no subject)"
	}
	col := th.Palette.OnSurface
	if !msg.IsRead {
		col = th.Palette.OnBackground
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			l := widget.Label{MaxLines: 1, Truncator: "…", Alignment: text.Start}
			return l.Layout(gtx, th.Shaper, font.Font{Weight: theme.Body.Weight},
				theme.Body.Size, subject, theme.ColorOp(gtx, col))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if msg.IsRead {
				return layout.Dimensions{}
			}
			return layout.Inset{Left: theme.SM}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				d := gtx.Dp(theme.UnreadDotSize)
				defer clip.Ellipse{Max: image.Pt(d, d)}.Push(gtx.Ops).Pop()
				paint.ColorOp{Color: th.Palette.Unread}.Add(gtx.Ops)
				paint.PaintOp{}.Add(gtx.Ops)
				return layout.Dimensions{Size: image.Pt(d, d)}
			})
		}))
}

// RelativeTime renders a compact, human timestamp: clock time today,
// weekday within a week, day-month within a year, else day-month-year.
func RelativeTime(t, now time.Time) string {
	t = t.Local()
	if t.IsZero() {
		return ""
	}
	y1, m1, d1 := t.Date()
	y2, m2, d2 := now.Date()
	switch {
	case y1 == y2 && m1 == m2 && d1 == d2:
		return t.Format("15:04")
	case now.Sub(t) < 7*24*time.Hour:
		return t.Format("Mon")
	case y1 == y2:
		return t.Format("2 Jan")
	default:
		return t.Format("02/01/06")
	}
}

// unreadBadge renders the small unread-count pill used in the drawer.
func unreadBadge(gtx layout.Context, th *theme.Theme, count int) layout.Dimensions {
	if count == 0 {
		return layout.Dimensions{}
	}
	label := fmt.Sprintf("%d", count)
	if count > 99 {
		label = "99+"
	}
	return th.Label(gtx, theme.Micro, th.Palette.Accent, label, 1)
}
